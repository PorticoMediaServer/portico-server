package playback

// V1-only session helpers (B8c). Previously these shared code with the /v2
// ControlService (control_occurrences.go, control_vod.go, queue_playback.go);
// the /v2 branches (playback_occurrences, playback_queue_candidates,
// playback_audio_staging, playback_viewer_leases, playback_preparation_intents)
// are gone. Only playback_sessions, playback_private_presentations and
// playback_physical_source_pins remain.

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/identity"
)

// checkSessionPolicy enforces owner transcoding policy and stream caps for v1
// sessions. Private presentations share their owner's slot.
func checkSessionPolicy(ctx context.Context, tx *sql.Tx, p identity.Principal, mode, replacing string) error {
	var transcode bool
	var accountCap, serverCap sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT transcoding_enabled,per_account_cap,server_cap FROM playback_owner_policy WHERE singleton=1`).Scan(&transcode, &accountCap, &serverCap)
	if err != nil {
		return err
	}
	if mode == "hls" && !transcode {
		return controlFault("transcoding_disabled", 422)
	}
	now := time.Now()
	for _, limit := range []struct {
		cap     sql.NullInt64
		account bool
		code    string
	}{
		{accountCap, true, "owner_account_cap"}, {serverCap, false, "owner_server_cap"},
	} {
		if !limit.cap.Valid {
			continue
		}
		n, err := countPlaybackSlots(ctx, tx, now, p.Authority, p.AccountID, limit.account, replacing)
		if err != nil {
			return err
		}
		if n >= limit.cap.Int64 {
			return controlFault(limit.code, 409)
		}
	}
	return nil
}

func countPlaybackSlots(ctx context.Context, tx *sql.Tx, now time.Time, authority, account string, scoped bool, replacing string) (int64, error) {
	query := `SELECT count(*) FROM playback_sessions ps JOIN authorization_access auth ON auth.hash=ps.session_hash
      WHERE ps.state NOT IN('stopped','ended','failed') AND ps.expires_at>? AND auth.expires_at>? AND auth.revoked=0 AND ps.id<>?
      AND NOT EXISTS(SELECT 1 FROM playback_private_presentations pp JOIN playback_sessions owner ON owner.id=pp.owner_session_id
        WHERE pp.session_id=ps.id AND owner.state NOT IN('stopped','ended','failed'))`
	stamp := now.UTC().Format(time.RFC3339)
	args := []any{stamp, stamp, replacing}
	if scoped {
		query += ` AND auth.authority=? AND ps.account_id=?`
		args = append(args, authority, account)
	}
	var n int64
	err := tx.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// terminateControlOccurrence ends a v1 session (replacement or stop). The /v2
// lease/preparation/occurrence rows no longer exist.
func terminateControlOccurrence(ctx context.Context, tx *sql.Tx, id, state string) error {
	_, e := tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE id=?`, id)
	return e
}

func checkSessionOccurrenceTx(ctx context.Context, tx *sql.Tx, id string) error {
	return checkSessionOccurrenceModeTx(ctx, tx, id, false)
}

func checkSessionOccurrenceModeTx(ctx context.Context, tx *sql.Tx, id string, initialAudio bool) error {
	var invalidPhysical int
	if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_physical_source_pins pin WHERE pin.session_id=? AND NOT EXISTS(SELECT 1 FROM library_sources source JOIN inventory_objects object ON object.source_id=source.id JOIN playback_sessions ps ON ps.asset_id=object.asset_id JOIN catalog_entities item ON item.id=ps.item_id JOIN catalog_libraries cl ON cl.id=item.library_id AND cl.library_id=source.library_id WHERE ps.id=pin.session_id AND source.id=pin.source_id AND source.incarnation=pin.incarnation AND source.generation=pin.configuration_generation AND source.enabled=1 AND object.root_incarnation=pin.incarnation AND object.retired=0 AND object.state='available')`, id).Scan(&invalidPhysical); e != nil {
		return e
	}
	if invalidPhysical != 0 {
		return identity.ErrUnauthorized
	}
	var privateUntil sql.NullInt64
	if e := tx.QueryRowContext(ctx, `SELECT expires_ms FROM playback_private_presentations WHERE session_id=?`, id).Scan(&privateUntil); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	} else if e == nil {
		if !initialAudio || privateUntil.Int64 <= time.Now().UnixMilli() {
			return identity.ErrUnauthorized
		}
		return nil
	}
	return nil
}

// RestartConversion discards converted output and moves the producer to the
// grid slot covering position.
func (s *Service) RestartConversion(ctx context.Context, id string, positionSeconds float64) {
	if s.hls == nil {
		return
	}
	s.hls.restart(ctx, id, positionSeconds)
}
