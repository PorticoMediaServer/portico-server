package playbackv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// containerCount is a container's size at one catalog revision (P16; migration
// 0067). A large shuffle needs the size before its first entry, and counting
// through the caller's item fence costs seconds at 1M keys. For a caller the
// fence admits everything for (Service.Unrestricted) the size is the
// container's own, the same for every such caller, so it is kept and reused
// until the catalog changes. The catalog revision is every library's
// revision: a container's keys are catalog rows (songs, episodes, book files,
// collection members, items), and every change to one moves its library's
// revision. Playlists aren't catalog rows, and a restricted caller's view is
// narrower: both keep the counting pass.
type containerCount struct {
	selector, catalog string
	keys              int64
}

// containerCountFor is the cache entry this caller and selector would use:
// with its stored size when one is current (keys > 0), or empty to be filled
// by the counting pass. Nil when the cache doesn't apply.
func (s *Service) containerCountFor(ctx context.Context, tx *sql.Tx, p identity.Principal, in SegmentInput, k selectorKeys) *containerCount {
	c := in.Source.Container
	if s.Unrestricted == nil || k.kind != "selector" || c == nil || c.Kind == "playlist" {
		return nil
	}
	if open, err := s.Unrestricted(ctx, tx, p); err != nil || !open {
		return nil
	}
	args, _ := json.Marshal(k.args)
	out := &containerCount{selector: c.Kind + ":" + string(args)}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(group_concat(library_id||':'||revision,','),'') FROM (SELECT library_id,revision FROM library_revisions ORDER BY library_id)`).Scan(&out.catalog); err != nil {
		return nil
	}
	var catalog string
	var keys int64
	err := tx.QueryRowContext(ctx, `SELECT catalog,keys FROM queue_v1_container_counts WHERE selector=?`, out.selector).Scan(&catalog, &keys)
	if err == nil && catalog == out.catalog {
		out.keys = keys
	}
	return out
}

// pick is countAndPick without the pass: the anchor's ordinal (a walk of the
// container's own keys, unfenced), or the seeded pick the pass would have
// made, read with one ordered seek. ok is false if the stored size and the
// keys disagree (then the pass runs).
func (c *containerCount) pick(ctx context.Context, tx *sql.Tx, k selectorKeys, seed int64, anchorItem string) (n, ordinal int64, key string, ok bool) {
	if anchorItem == "" {
		ordinal = reservoirOrdinal(seed, c.keys)
		if tx.QueryRowContext(ctx, k.query+` LIMIT 1 OFFSET ?`, append(append([]any{}, k.args...), ordinal)...).Scan(&key) != nil {
			return 0, 0, "", false
		}
		return c.keys, ordinal, key, true
	}
	rows, err := tx.QueryContext(ctx, k.query, k.args...)
	if err != nil {
		return 0, 0, "", false
	}
	defer rows.Close()
	for i := int64(0); rows.Next() && i < c.keys; i++ {
		var id string
		if rows.Scan(&id) != nil {
			return 0, 0, "", false
		}
		if id == anchorItem {
			return c.keys, i, id, true
		}
	}
	return 0, 0, "", false
}

// reservoirOrdinal is the pick the counting pass makes over n keys: the last
// ordinal reservoirPick accepts. It depends on the seed and n alone, so the
// cached path and the pass agree; each ordinal i is accepted with
// probability 1/(i+1), so walking down from n-1 takes n/2 steps on average
// (a few milliseconds at 1M).
func reservoirOrdinal(seed, n int64) int64 {
	for i := n - 1; i > 0; i-- {
		if reservoirPick(seed, i) {
			return i
		}
	}
	return 0
}

// storeContainerCount keeps a counted size, at background priority, after
// the counting pass's read has ended. A failure only costs the next shuffle
// its pass.
func (s *Service) storeContainerCount(ctx context.Context, c containerCount) {
	write, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := dbwork.ExecWrite(write, s.DB, dbwork.ClassBackgroundMedia, `INSERT INTO queue_v1_container_counts(selector,catalog,keys,counted_ms) VALUES(?,?,?,?) ON CONFLICT(selector) DO UPDATE SET catalog=excluded.catalog,keys=excluded.keys,counted_ms=excluded.counted_ms`, c.selector, c.catalog, c.keys, s.now().UnixMilli()); err != nil {
		log.Printf("A container's size could not be kept: %v", err)
	}
}
