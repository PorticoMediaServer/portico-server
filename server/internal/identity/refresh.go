package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
)

// HostedRefreshVerifier checks the current signed Hosted policy and member
// inside the same transaction as token rotation. A local account needs no
// external verifier; a Hosted account must supply one.
type HostedRefreshVerifier func(context.Context, *sql.Tx, Principal) (time.Time, error)

// RefreshSession exchanges a device-bound refresh credential for another 15
// minute access token and a new refresh credential. A retry with the same
// request ID receives the exact committed response, stored plainly, until the
// successor credential is first used (capped at the family horizon). Neither an access bearer nor an installation ID alone can refresh.
func (s *Service) RefreshSession(ctx context.Context, secret, installation, requestID string, verify HostedRefreshVerifier) (Envelope, error) {
	if len(secret) != 43 || !validInstallationID(installation) || !validFamilyID(requestID) {
		return Envelope{}, ErrUnauthorized
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if err != nil {
		return Envelope{}, err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	hash := Digest(secret)
	var family, device, expiry, deviceInstallation, state string
	var generation int64
	err = tx.QueryRowContext(ctx, `SELECT c.family_id,c.device_id,c.generation,c.expires_at,d.installation_id,d.approval_state FROM identity_refresh_credentials c JOIN identity_devices d ON d.id=c.device_id WHERE c.token_hash=?`, hash).
		Scan(&family, &device, &generation, &expiry, &deviceInstallation, &state)
	if errors.Is(err, sql.ErrNoRows) && migratedRefreshTx(ctx, tx, hash) {
		return Envelope{}, ErrSessionMigrated
	}
	if errors.Is(err, sql.ErrNoRows) {
		out, reused, replayErr := s.replayRefreshTx(ctx, tx, hash, installation, requestID, verify)
		if reused != "" {
			// A known predecessor outside its exact replay is evidence that the
			// refresh credential escaped. Commit the revocation before returning
			// an error; the deferred rollback would otherwise undo it.
			if err = RevokeFamilyTx(ctx, tx, reused, RevokedRefreshReuse); err != nil {
				return Envelope{}, err
			}
			if err = gated.Commit(); err != nil {
				return Envelope{}, err
			}
			return Envelope{}, ErrUnauthorized
		}
		return out, replayErr
	}
	if err != nil {
		return Envelope{}, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	refreshUntil, err := familyTime(expiry)
	if err != nil || !refreshUntil.After(now) || deviceInstallation != installation || state != "approved" {
		return Envelope{}, ErrUnauthorized
	}
	var p Principal
	p.ServerID = s.serverID
	var horizonRaw, currentHash string
	var revoked int
	err = tx.QueryRowContext(ctx, `SELECT f.account_id,f.profile_id,f.authority,f.role,f.epoch,f.authorization_horizon,f.revoked,t.token_hash FROM authorization_session_families f JOIN authorization_family_tokens t ON t.family_id=f.id AND t.generation=f.current_generation WHERE f.id=?`, family).
		Scan(&p.AccountID, &p.ProfileID, &p.Authority, &p.Role, &p.Epoch, &horizonRaw, &revoked, &currentHash)
	if err != nil || revoked != 0 {
		return Envelope{}, ErrUnauthorized
	}
	p.Hash = currentHash
	oldHorizon, err := familyTime(horizonRaw)
	if err != nil || !oldHorizon.After(now) {
		return Envelope{}, ErrUnauthorized
	}
	if err = s.checkFamilyPrincipalTx(ctx, tx, p); err != nil {
		return Envelope{}, err
	}
	horizon := now.Add(LocalSignInWindow)
	if p.Authority == "hosted" {
		if verify == nil {
			return Envelope{}, ErrUnauthorized
		}
		verified, err := verify(ctx, tx, p)
		if err != nil {
			return Envelope{}, err
		}
		horizon, err = s.hostedFamilyHorizonTx(ctx, tx, verified, now)
		if err != nil {
			return Envelope{}, err
		}
	}
	var currentGeneration int64
	if err = tx.QueryRowContext(ctx, `SELECT current_generation FROM authorization_session_families WHERE id=? AND revoked=0`, family).Scan(&currentGeneration); err != nil || currentGeneration < generation {
		return Envelope{}, ErrUnauthorized
	}
	access, nextRefresh := Token(), Token()
	accessUntil := accessExpiry(now, horizon).Format(time.RFC3339)
	horizonText := horizon.Format(time.RFC3339)
	if _, err = tx.ExecContext(ctx, `UPDATE authorization_family_tokens SET retired=1 WHERE token_hash=? AND retired=0`, currentHash); err != nil {
		return Envelope{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO authorization_family_tokens(token_hash,family_id,generation,expires_at,retired) VALUES(?,?,?,?,0)`, Digest(access), family, currentGeneration+1, accessUntil); err != nil {
		return Envelope{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE authorization_session_families SET current_generation=?,authorization_horizon=? WHERE id=? AND revoked=0`, currentGeneration+1, horizonText, family); err != nil {
		return Envelope{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE identity_refresh_credentials SET token_hash=?,generation=?,expires_at=? WHERE family_id=? AND token_hash=?`, Digest(nextRefresh), currentGeneration+1, horizonText, family, hash); err != nil {
		return Envelope{}, err
	}
	out := Envelope{AccessToken: access, RefreshToken: nextRefresh, DeviceID: device, InstallationID: deviceInstallation, ExpiresAt: accessUntil, Viewer: p.Viewer, SessionFamilyID: family, TokenGeneration: currentGeneration + 1, AuthorizationHorizon: horizonText}
	sealed, err := s.sealSetup(out, refreshAAD(family, hash, installation, requestID))
	if err != nil {
		return Envelope{}, err
	}
	// The exact-response receipt lives until the successor is first used (the
	// rotation below deletes the family's older receipts), capped at the family's
	// horizon. A device whose answer was lost in an outage of any length can replay
	// the same request and get the same credentials; once the legitimate device has
	// moved on, the predecessor is reuse and revokes the family as before.
	if _, err = tx.ExecContext(ctx, `DELETE FROM identity_refresh_receipts WHERE family_id=?`, family); err != nil {
		return Envelope{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO identity_refresh_receipts(predecessor_hash,family_id,request_id,ciphertext,expires_at) VALUES(?,?,?,?,?)`, hash, family, requestID, sealed, horizonText); err != nil {
		return Envelope{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO identity_refresh_predecessors(predecessor_hash,family_id) VALUES(?,?)`, hash, family); err != nil {
		return Envelope{}, err
	}
	if err = gated.Commit(); err != nil {
		return Envelope{}, err
	}
	return out, nil
}

func refreshAAD(family, predecessor, installation, request string) string {
	return "first-party-refresh.v1:" + family + ":" + predecessor + ":" + installation + ":" + request
}

// A nonempty reused family identifies a known predecessor used outside the
// exact-response replay window (after its successor was used). The caller must
// commit its revocation.
func (s *Service) replayRefreshTx(ctx context.Context, tx *sql.Tx, predecessor, installation, request string, verify HostedRefreshVerifier) (Envelope, string, error) {
	var family, storedRequest, until string
	var sealed []byte
	if err := tx.QueryRowContext(ctx, `SELECT family_id,request_id,ciphertext,expires_at FROM identity_refresh_receipts WHERE predecessor_hash=?`, predecessor).Scan(&family, &storedRequest, &sealed, &until); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return Envelope{}, "", err
		}
		if err = tx.QueryRowContext(ctx, `SELECT family_id FROM identity_refresh_predecessors WHERE predecessor_hash=?`, predecessor).Scan(&family); err == nil {
			return Envelope{}, family, ErrUnauthorized
		}
		if errors.Is(err, sql.ErrNoRows) {
			return Envelope{}, "", ErrUnauthorized
		}
		return Envelope{}, "", err
	}
	if storedRequest != request {
		return Envelope{}, family, ErrUnauthorized
	}
	expiry, err := familyTime(until)
	if err != nil || !expiry.After(time.Now()) {
		return Envelope{}, family, ErrUnauthorized
	}
	var out Envelope
	if err = s.openSetup(sealed, refreshAAD(family, predecessor, installation, request), &out); err != nil {
		return Envelope{}, family, ErrUnauthorized
	}
	// Receipts sealed before installationId was included still use the binding
	// in their authenticated data. Return it on an exact replay as well.
	out.InstallationID = installation
	var live bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_session_families f JOIN identity_refresh_credentials c ON c.family_id=f.id JOIN identity_devices d ON d.id=c.device_id JOIN authorization_family_tokens t ON t.family_id=f.id AND t.generation=f.current_generation WHERE f.id=? AND f.revoked=0 AND d.approval_state='approved' AND d.installation_id=? AND c.token_hash=? AND t.token_hash=?)`, family, installation, Digest(out.RefreshToken), Digest(out.AccessToken)).Scan(&live); err != nil || !live {
		return Envelope{}, "", ErrRenewalRetired
	}
	p := Principal{Viewer: out.Viewer, Hash: Digest(out.AccessToken)}
	if err = tx.QueryRowContext(ctx, `SELECT epoch,server_id FROM authorization_session_families WHERE id=?`, family).Scan(&p.Epoch, &p.ServerID); err != nil {
		return Envelope{}, "", ErrUnauthorized
	}
	if err = s.checkFamilyPrincipalTx(ctx, tx, p); err != nil {
		return Envelope{}, "", err
	}
	if p.Authority == "hosted" {
		if verify == nil {
			return Envelope{}, "", ErrUnauthorized
		}
		if _, err = verify(ctx, tx, p); err != nil {
			return Envelope{}, "", err
		}
	}
	return out, "", nil
}
