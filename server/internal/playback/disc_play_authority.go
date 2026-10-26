package playback

import (
	"context"
	"database/sql"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/playbackdisc"
)

// checkActiveDiscSnapshotTx is ordinary accepted-presentation authority. It
// deliberately uses neither a preparation value nor its original POST context.
// Store.Resolve/ResolvePin additionally check the immutable source/receipt and
// original horizon. Only the established same-family continuation seam can
// follow a rotated token; no latest account/profile/family lookup is permitted.
func (s *Service) checkActiveDiscSnapshotTx(ctx context.Context, tx *sql.Tx, snapshot playbackdisc.Snapshot) error {
	if s == nil || ctx == nil || tx == nil || s.ContinueAuthorityTx == nil {
		return playbackdisc.ErrCurrent
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var principal identity.Principal
	var item, expires string
	err := tx.QueryRowContext(ctx, `SELECT ps.session_hash,s.account_id,s.profile_id,s.authority,s.role,s.epoch,pid(i.public_id),ps.expires_at
 FROM playback_sessions ps JOIN catalog_entities i ON i.id=ps.item_id JOIN authorization_access s ON s.hash=ps.session_hash
 WHERE ps.id=? AND ps.generation=? AND ps.asset_id=? AND ps.request_id=? AND ps.state NOT IN('stopped','ended','failed')`,
		snapshot.SessionID, snapshot.Generation, snapshot.Source.AssetID, snapshot.Operation.LegacyRequestID).Scan(&principal.Hash, &principal.AccountID, &principal.ProfileID, &principal.Authority, &principal.Role, &principal.Epoch, &item, &expires)
	if err != nil {
		return err
	}
	principal.ServerID, principal.DeviceID, principal.Grant = snapshot.Actor.ServerID, snapshot.Actor.DeviceID, snapshot.Actor.Grant
	if principal.AccountID != snapshot.Actor.AccountID || principal.ProfileID != snapshot.Actor.ProfileID || principal.Authority != snapshot.Actor.Authority || int64(principal.Epoch) != snapshot.Actor.Epoch {
		return playbackdisc.ErrCurrent
	}
	end, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !end.After(time.Now()) || snapshot.Actor.OriginalHorizonMs <= time.Now().UnixMilli() {
		return playbackdisc.ErrCurrent
	}
	// This is the normal already-playing continuation callback, not HTTP
	// authentication and not the initial-token preparation callback.
	principal, err = s.ContinueAuthorityTx(ctx, tx, principal, item)
	if err != nil {
		return err
	}
	if principal.ServerID != snapshot.Actor.ServerID || principal.AccountID != snapshot.Actor.AccountID || principal.ProfileID != snapshot.Actor.ProfileID || principal.Authority != snapshot.Actor.Authority || principal.DeviceID != snapshot.Actor.DeviceID || principal.Grant != snapshot.Actor.Grant || int64(principal.Epoch) != snapshot.Actor.Epoch {
		return playbackdisc.ErrCurrent
	}
	var family, device string
	var generation int64
	err = tx.QueryRowContext(ctx, `SELECT t.family_id,t.generation,d.device_id FROM authorization_family_tokens t
 JOIN authorization_session_families f ON f.id=t.family_id JOIN identity_device_families d ON d.family_id=f.id
 WHERE t.token_hash=? AND t.retired=0 AND t.generation=f.current_generation AND f.revoked=0`, principal.Hash).Scan(&family, &generation, &device)
	if err != nil {
		return err
	}
	if family != snapshot.Actor.FamilyID || device != snapshot.Actor.DeviceID || generation < snapshot.Actor.TokenGeneration {
		return playbackdisc.ErrCurrent
	}
	// A pending private candidate cannot become readable merely by possessing a
	// legacy ID. The actual accepted V1 occurrence must still be alive.
	var accepted bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_v1_sessions v JOIN catalog_entities i ON i.id=v.item_id
 WHERE v.media_session_id=? AND v.media_generation=? AND v.account_id=? AND v.profile_id=? AND v.authority=? AND v.device_id=?
 AND v.ended_ms=0 AND v.lease_expires_ms>? AND pid(i.public_id)=? AND v.queue_id=? AND v.entry_id=?)`,
		snapshot.SessionID, snapshot.Generation, snapshot.Actor.AccountID, snapshot.Actor.ProfileID, snapshot.Actor.Authority, snapshot.Actor.DeviceID,
		time.Now().UnixMilli(), item, snapshot.Operation.QueueID, snapshot.Operation.QueueEntryID).Scan(&accepted)
	if err != nil {
		return err
	}
	if !accepted {
		return playbackdisc.ErrCurrent
	}
	if err := recordingAdmittedTx(ctx, tx, item); err != nil {
		return err
	}
	return ctx.Err()
}
