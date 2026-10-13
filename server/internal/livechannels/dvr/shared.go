package dvr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/mediaartifact"
	"portico.local/server/internal/supervise"
	"time"
)

func (s *Store) sharedCaptureTx(ctx context.Context, tx *sql.Tx, c *claim, r Recording, generation int64) (bool, bool, error) {
	if _, ok := s.driver.(SharedCaptureDriver); !ok {
		return false, false, nil
	}
	var leader string
	e := tx.QueryRowContext(ctx, `SELECT capture_recording_id FROM dvr_recording_shares WHERE recording_id=?`, r.ID).Scan(&leader)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return false, false, e
	}
	if leader == "" {
		e = tx.QueryRowContext(ctx, `SELECT id FROM dvr_recordings WHERE source_id=? AND channel_id=? AND programme_id=? AND id<>? AND state IN('preparing','recording','finalizing','completed','incomplete-playable') AND NOT EXISTS(SELECT 1 FROM dvr_recording_shares x WHERE x.recording_id=dvr_recordings.id) ORDER BY start_ms,id LIMIT 1`, r.Occurrence.SourceID, r.Occurrence.ChannelID, r.Occurrence.ProgrammeID, r.ID).Scan(&leader)
		if errors.Is(e, sql.ErrNoRows) {
			return false, false, nil
		}
		if e != nil {
			return false, false, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO dvr_recording_shares VALUES(?,?)`, r.ID, leader); e != nil {
			return false, false, e
		}
	}
	var state string
	if e = tx.QueryRowContext(ctx, `SELECT state FROM dvr_recordings WHERE id=?`, leader).Scan(&state); e != nil {
		return false, false, e
	}
	if state != "completed" && state != "incomplete-playable" && state != "pending-delete" {
		if capturing(state) {
			return true, true, nil
		}
		_, e = tx.ExecContext(ctx, `DELETE FROM dvr_recording_shares WHERE recording_id=?`, r.ID)
		return false, false, e
	}
	c.sharedFrom = leader
	c.request.Generation = generation + 1
	c.token, e = randomID()
	if e != nil {
		return false, false, e
	}
	c.request.ArtifactKey, e = mediaartifact.RetainedCaptureKey(r.ID, c.request.Generation)
	if e != nil {
		return false, false, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE dvr_recordings SET state='preparing',reason='',claim_token=?,claim_generation=?,artifact_id=?,lease_until_ms=?,owner_instance=?,revision=revision+1,updated_ms=? WHERE id=?`, c.token, c.request.Generation, c.request.ArtifactKey, s.now().Add(workerTTL).UnixMilli(), s.instance, s.now().UnixMilli(), r.ID)
	if e != nil {
		return false, false, e
	}
	e = touchTx(ctx, tx, c.request.Owner)
	return true, false, e
}
func (s *Store) runSharedClaim(parent context.Context, c *claim) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	supervise.Go("dvr.shared.renew", func() {
		defer close(done)
		tick := time.NewTicker(4 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if s.renew(ctx, c, true) != nil {
					cancel()
					return
				}
			}
		}
	})
	defer func() { cancel(); <-done }()

	var source CaptureRequest
	var cp CaptureCheckpoint
	var raw, state string
	e := s.db.QueryRowContext(ctx, `SELECT authority,account_id,profile_id,artifact_id,checkpoint_json,state FROM dvr_recordings WHERE id=?`, c.sharedFrom).Scan(&source.Owner.Authority, &source.Owner.AccountID, &source.Owner.ProfileID, &source.ArtifactKey, &raw, &state)
	source.Recording.ID = c.sharedFrom
	if e == nil {
		e = json.Unmarshal([]byte(raw), &cp)
	}
	if e == nil && state != "completed" && state != "incomplete-playable" && state != "pending-delete" {
		e = ErrConflict
	}
	if e == nil {
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			e = err
		} else {
			source.Recording, e = loadTx(ctx, tx, source.Owner, c.sharedFrom)
			tx.Rollback()
		}
	}
	if e == nil {
		c.checkpoint = cp
		cp.WriterRetired = true
		e = s.checkpoint(ctx, c, cp)
	}
	if e == nil {
		result, err := s.driver.(SharedCaptureDriver).Share(ctx, c.request, source, cp)
		e = err
		// Shared media keeps the leader's interval and gap evidence. It is never
		// made complete merely because this profile attached after capture ended.
		if e == nil {

			e = s.publish(ctx, c, result)
		}
	}
	if e != nil && ctx.Err() == nil {
		s.failWorker(c, "shared-capture-unavailable", false)
	}
}

// preventLastCaptureRemoval retains the source object until every pending
// profile attachment has acquired its independent immutable hard link.
func (s *Store) pendingSharedTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var busy bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dvr_recording_shares x JOIN dvr_recordings r ON r.id=x.recording_id WHERE x.capture_recording_id=? AND r.state NOT IN('completed','incomplete-playable','deleted','failed','cancelled','pending-delete'))`, id).Scan(&busy)
	return busy, e
}

// hasSharedCaptureTx includes finished leaders until the follower acquires its
// own path. Explicit cancellation and durable permission checks stay separate.
func (s *Store) hasSharedCaptureTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var shared bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dvr_recording_shares x JOIN dvr_recordings leader ON leader.id=x.capture_recording_id WHERE x.recording_id=? AND leader.state IN('preparing','recording','finalizing','completed','incomplete-playable','pending-delete'))`, id).Scan(&shared)
	return shared, err
}
