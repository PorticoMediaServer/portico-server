package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// ReasonRemoved marks a claim the viewer deliberately gave up, as opposed to
// one the server cancelled or expired underneath them.
const ReasonRemoved = "removed"

// LibraryGuard is the caller's visibility check. The routes layer owns library
// permission; this package only asks whether this viewer may see that library,
// so an unreadable library is rejected per target instead of failing a batch.
//
// It is handed the admitting transaction rather than left to open its own. The
// database is opened with a single connection, so a guard that queried outside
// this transaction would deadlock against the transaction that called it.
type LibraryGuard func(tx *sql.Tx, library string) error

// Submit admits one preparation request. It is idempotent on operationId: a
// replayed request returns the batch the first one produced, byte for byte,
// rather than admitting the work twice.
func (s *Service) Submit(ctx context.Context, p identity.Principal, r Request, guard LibraryGuard) (Batch, error) {
	out := Batch{Items: []Preparation{}, Rejected: []Rejection{}}
	if !validID.MatchString(r.OperationID) {
		return out, ErrInput
	}
	if e := preparationQuality(r.Quality); e != nil {
		return out, e
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	now := s.millis()
	key := operations.ViewerKey(p)
	scope := "downloads-prepare:" + key
	raw, digest, e := operations.Receipt(tx, scope, r.OperationID, r, now)
	if e != nil {
		return out, e
	}
	if raw != "" {
		if e = json.Unmarshal([]byte(raw), &out); e != nil {
			return out, e
		}
		out.Duplicate = true
		return out, nil
	}
	allowed, e := ProfileAllowsDownloads(tx, p.Viewer)
	if e != nil {
		return out, e
	}
	if !allowed {
		return out, ErrPolicy
	}
	targets, origin, e := expandTargets(ctx, tx, r)
	if e != nil {
		return out, e
	}
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	batch := identity.Digest(identity.Token())
	out.BatchID = batch
	refused := 0
	// SEC-02: a title that doesn't exist, one in a library the viewer isn't
	// given and one over its restrictions get the same answer: 404 for a
	// one-title request, an item_deleted rejection in a batch. (A one-title
	// request used to answer 404 for a restricted title and 201 with a
	// rejection for an absent one, which told the two apart.)
	unreachable := func(item string) bool {
		if r.MediaID != "" {
			return false
		}
		out.Rejected = append(out.Rejected, Rejection{item, ReasonItemDeleted})
		return true
	}
	for _, item := range targets {
		entity, err := entityid.Resolve(ctx, tx, item)
		if errors.Is(err, entityid.ErrNotFound) {
			if !unreachable(item) {
				return out, ErrNotFound
			}
			continue
		}
		if err != nil {
			return out, err
		}
		library, err := libraryOf(ctx, tx, item)
		if errors.Is(err, sql.ErrNoRows) {
			if !unreachable(item) {
				return out, ErrNotFound
			}
			continue
		}
		if err != nil {
			return out, err
		}
		if err = contentaccess.VisibleItemTx(ctx, tx, p, item); err != nil {
			if !errors.Is(err, identity.ErrContentRestricted) {
				return out, err
			}
			if !unreachable(item) {
				return out, ErrNotFound
			}
			continue
		}
		if guard != nil {
			if err = guard(tx, library); err != nil {
				if !unreachable(item) {
					return out, ErrNotFound
				}
				continue
			}
		}
		// A live claim on the same item and quality is the same claim. Returning
		// it keeps a client that retried a batch from accumulating duplicates.
		existing, err := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE profile_key=? AND item_id=? AND quality=? AND state IN('queued','running','ready','paused','failed')`, key, entity, r.Quality))
		if err == nil {
			out.Items = append(out.Items, s.publish(existing, settings.RetentionDays))
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		found, err := resolveSource(ctx, tx, item)
		if reason := sourceReason(found, err); reason != "" {
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return out, err
			}
			out.Rejected = append(out.Rejected, Rejection{item, reason})
			continue
		}
		estimate, estimated := estimateBytes(r.Quality, found)
		if err = storageRoom(tx, estimate); errors.Is(err, ErrStorageFull) {
			out.Rejected = append(out.Rejected, Rejection{item, ReasonStorageFull})
			refused++
			continue
		} else if err != nil {
			return out, err
		}
		id := identity.Digest(identity.Token())
		flag := 0
		if estimated {
			flag = 1
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO download_preparations(id,profile_key,authority,account_id,profile_id,item_id,library_id,quality,origin,batch_id,state,bytes_total,estimated,created_ms,updated_ms) VALUES(?,?,?,?,?,?,?,?,?,?,'queued',?,?,?,?)`,
			id, key, p.Authority, p.AccountID, p.ProfileID, entity, library, r.Quality, origin, batch, estimate, flag, now, now); err != nil {
			return out, err
		}
		fresh, err := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=?`, id))
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, s.publish(fresh, settings.RetentionDays))
		out.Accepted++
		principal, _ := json.Marshal(p)
		if _, err = tx.ExecContext(ctx, `INSERT INTO download_preparation_authority(preparation_id,principal) VALUES(?,?)`, id, string(principal)); err != nil {
			return out, err
		}

	}
	// A request that named one thing and got nothing but a full store should say
	// so plainly rather than answering an empty batch a client has to interpret.
	if out.Accepted == 0 && len(out.Items) == 0 && refused > 0 && refused == len(out.Rejected) {
		return out, ErrStorageFull
	}
	if len(out.Items) == 0 && len(out.Rejected) == 0 {
		return out, ErrNotFound
	}
	if e = operations.Audit(tx, now, operations.AccountKey(p), "downloads.prepare", batch, int64(out.Accepted)); e != nil {
		return out, e
	}
	if e = operations.SaveReceipt(tx, scope, r.OperationID, digest, out, now); e != nil {
		return out, e
	}
	if e = gated.Commit(); e != nil {
		return out, e
	}
	s.signal()
	return out, nil
}

// legal reports whether an action is offered from a state. The published
// actions list and this fence are the same table, so a client that renders the
// controls the server published never has an action refused as illegal.
func legal(state, action string) bool {
	for _, v := range actionsFor(state) {
		if v == action {
			return true
		}
	}
	return false
}

// Act applies one fenced action. Every action names the revision the client
// read; an action against a stale revision is refused with the current row so
// the client can re-render rather than retry blind.
func (s *Service) Act(ctx context.Context, p identity.Principal, id string, c Command) (Preparation, error) {
	var out Preparation
	if !validID.MatchString(id) || !validID.MatchString(c.OperationID) {
		return out, ErrInput
	}
	switch c.Action {
	case ActionPause, ActionResume, ActionCancel, ActionRetry, ActionRemove:
	default:
		return out, ErrInput
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	now := s.millis()
	key := operations.ViewerKey(p)
	scope := "downloads-action:" + key
	raw, digest, e := operations.Receipt(tx, scope, c.OperationID, []any{id, c}, now)
	if e != nil {
		return out, e
	}
	if raw != "" {
		return out, json.Unmarshal([]byte(raw), &out)
	}
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	current, e := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=? AND profile_key=?`, id, key))
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if current.Revision != c.ExpectedRevision {
		return s.publish(current, settings.RetentionDays), ErrConflict
	}
	if !legal(current.State, c.Action) {
		return s.publish(current, settings.RetentionDays), ErrConflict
	}
	switch c.Action {
	case ActionPause:
		_, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='paused',reason='',revision=revision+1,updated_ms=? WHERE id=?`, now, id)
	case ActionResume:
		_, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='queued',reason='',revision=revision+1,updated_ms=? WHERE id=?`, now, id)
	case ActionCancel, ActionRemove:
		reason := ReasonCancelled
		if c.Action == ActionRemove {
			reason = ReasonRemoved
		}
		// Giving up the claim also withdraws the offline authorization it
		// justified: a receipt outliving its preparation would let a client play
		// bytes the server no longer accounts for.
		if e = revokeForPreparation(tx, id, reason, now); e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM download_grants WHERE preparation_id=?`, id); e != nil {
			return out, e
		}
		_, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='cancelled',reason=?,hash_state=NULL,bytes_done=0,revision=revision+1,updated_ms=? WHERE id=?`, reason, now, id)
	case ActionRetry:
		var device string
		e = tx.QueryRowContext(ctx, `SELECT device_id FROM download_preparation_authority WHERE preparation_id=?`, id).Scan(&device)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		if device != "" {
			if _, _, e = s.requestAccess(ctx, tx, p, device); e != nil {
				return out, e
			}
		}
		principal, _ := json.Marshal(p)
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparation_authority SET principal=?,optimization_requested=0,optimization_operation='' WHERE preparation_id=?`, string(principal), id); e != nil {
			return out, e
		}
		if e = storageRoom(tx, current.Progress.BytesTotal); e != nil {
			return out, e
		}
		_, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='queued',reason='',artifact_kind='',artifact_ref='',artifact_digest='',artifact_container='',source_version_json='',hash_state=NULL,bytes_done=0,started_ms=0,ready_ms=0,revision=revision+1,updated_ms=? WHERE id=?`, now, id)
	}
	if e != nil {
		return out, e
	}
	updated, e := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=?`, id))
	if e != nil {
		return out, e
	}
	out = s.publish(updated, settings.RetentionDays)
	if e = operations.Audit(tx, now, operations.AccountKey(p), "downloads."+c.Action, id, out.Revision); e != nil {
		return out, e
	}
	if e = operations.SaveReceipt(tx, scope, c.OperationID, digest, out, now); e != nil {
		return out, e
	}
	if e = gated2.Commit(); e == nil {
		s.signal()
	}
	return out, e
}
