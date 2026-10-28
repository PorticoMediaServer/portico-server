package compactcatalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// O-101 read batching. Capture and every normalize sweep used to run one
// multi-join point statement per raw object; a bounded window now runs one
// set-based statement and the per-row Go work stays exactly as it was.
//
// reading_publications.asset_id is UNIQUE, so one raw object still hydrates to
// at most one row: batching repeats the same index seeks, it does not join
// more. Nothing here is stored between steps: the map lives for one quantum.

// pathObjectBatchMax bounds one hydrated window. It equals the Reading raw
// budget, so a step never asks for more objects than it could consume.
const pathObjectBatchMax = readingPathBatch

// pathObjectBatch is pathObjectColumns over a json_each id list. json_each is
// the leading table (CROSS JOIN) so the planner seeks inventory_objects by id
// for each value instead of scanning it.
const pathObjectBatch = `SELECT ` + pathObjectColumns + ` FROM json_each(?) j CROSS JOIN inventory_objects o ON o.id=j.value` + pathObjectJoins + ` WHERE o.retired=0`

// readPathObjects hydrates a bounded batch with one statement per window. An
// absent id means the same thing the point read's sql.ErrNoRows meant to its
// callers; the one-id window keeps using the point read unchanged.
func readPathObjects(ctx context.Context, tx *sql.Tx, ids []string) (map[string]pathObject, error) {
	out := make(map[string]pathObject, min(len(ids), pathObjectBatchMax))
	for start := 0; start < len(ids); start += pathObjectBatchMax {
		chunk := ids[start:min(start+pathObjectBatchMax, len(ids))]
		if len(chunk) == 1 {
			v, err := readPathObject(ctx, tx, chunk[0])
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out[v.ID] = v
			continue
		}
		listed, err := json.Marshal(chunk)
		if err != nil {
			return nil, err
		}
		rows, err := tx.QueryContext(ctx, pathObjectBatch, string(listed))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var v pathObject
			if err = rows.Scan(v.scan()...); err != nil {
				rows.Close()
				return nil, err
			}
			v.finish()
			// QueryRow keeps a point read's first row; keep the batch's first
			// row per id so a duplicate can never change a decision.
			if _, seen := out[v.ID]; !seen {
				out[v.ID] = v
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// normalizePathObjects normalizes a bounded id batch from one hydration. The
// write order, per-object body and returned errors are those of
// ReadingNormalizeObjectTx called once per id.
func normalizePathObjects(ctx context.Context, tx *sql.Tx, ids []string) error {
	for start := 0; start < len(ids); start += pathObjectBatchMax {
		chunk := ids[start:min(start+pathObjectBatchMax, len(ids))]
		objects, err := readPathObjects(ctx, tx, chunk)
		if err != nil {
			return err
		}
		for _, id := range chunk {
			v, found := objects[id]
			if err := normalizePathObjectTx(ctx, tx, id, v, found); err != nil {
				return err
			}
		}
	}
	return nil
}

// pathObjectWorkStep drains consecutive 'object' work rows that share the head
// row's served stamp. Under the queue's ORDER BY served,kind,work_key they are
// exactly the next rows, so batching them records the same sequence as one-row
// steps. Each row keeps its own requested/captured guard: a row requested again
// while captured stays queued and is re-read on the next claim.
func pathObjectWorkStep(ctx context.Context, tx *sql.Tx, w pathWork, limit int, turn int64) (int, error) {
	peers, err := pathObjectPeers(ctx, tx, w, limit-1)
	if err != nil {
		return 0, err
	}
	group := append([]pathWork{w}, peers...)
	ids := make([]string, len(group))
	for i := range group {
		ids[i] = group[i].Key
	}
	if err = normalizePathObjects(ctx, tx, ids); err != nil {
		return 0, err
	}
	var retire []int64
	seen := map[int64]bool{}
	for i := range group {
		r := group[i]
		if r.Requested != r.Captured {
			// Same re-arm as a one-row step: zero the claim and keep the row.
			r.Captured = 0
			if _, err = tx.ExecContext(ctx, `UPDATE reading_path_dirty_work SET captured=?,after_object=?,after_asset=?,after_scope=?,phase=?,served=?,fence_object=?,fence_asset=?,fence_scope=? WHERE library_id=? AND kind=? AND work_key=?`, r.Captured, r.Object, r.Asset, r.Scope, r.Phase, turn, r.FenceObject, r.FenceAsset, r.FenceScope, r.Library, r.Kind, r.Key); err != nil {
				return 0, err
			}
			continue
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM reading_path_dirty_work WHERE library_id=? AND kind=? AND work_key=? AND requested=?`, r.Library, r.Kind, r.Key, r.Captured); err != nil {
			return 0, err
		}
		if !seen[r.Library] {
			seen[r.Library] = true
			retire = append(retire, r.Library)
		}
	}
	for _, library := range retire {
		if err = pathRetireInput(ctx, tx, library); err != nil {
			return 0, err
		}
	}
	return max(1, len(group)), nil
}

// pathObjectPeers reads the rest of the head row's object batch, in the queue's
// own served,kind,work_key order. Each row gets the same claim-time
// normalization the head row already had: a fresh capture takes the current
// request count, and object rows carry no cursor or fence.
func pathObjectPeers(ctx context.Context, tx *sql.Tx, w pathWork, limit int) ([]pathWork, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT library_id,work_key,requested,captured,after_object,after_asset,after_scope,phase,fence_object,fence_asset,fence_scope FROM reading_path_dirty_work WHERE kind='object' AND served=? AND (library_id<>? OR work_key<>?) ORDER BY served,kind,work_key LIMIT ?`, w.Served, w.Library, w.Key, limit)
	if err != nil {
		return nil, err
	}
	var out []pathWork
	for rows.Next() {
		r := pathWork{Kind: "object", Served: w.Served}
		if err = rows.Scan(&r.Library, &r.Key, &r.Requested, &r.Captured, &r.Object, &r.Asset, &r.Scope, &r.Phase, &r.FenceObject, &r.FenceAsset, &r.FenceScope); err != nil {
			rows.Close()
			return nil, err
		}
		if r.Captured == 0 {
			r.Captured = r.Requested
			r.Object = ""
			r.Asset = 0
			r.Scope = 0
			r.Phase = 0
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}
