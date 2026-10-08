package hosted

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/networking"
)

type cleanupTransport interface {
	ClaimCleanup(context.Context, networking.Intent, networking.SignedProof) (networking.PolicyEnvelope, error)
}
type claimCleanup struct {
	keys      *networking.ProtectedKeys
	transport cleanupTransport
}
type cleanupJob struct {
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	DeletionID  string    `json:"deletionId"`
	ServerID    string    `json:"serverId"`
	AccountID   string    `json:"accountId"`
	ProfileIDs  []string  `json:"profileIds"`
	Revision    int64     `json:"revision"`
	Disposition string    `json:"disposition"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type cleanupResult struct {
	Kind                 string       `json:"kind"`
	RequestID            string       `json:"requestId"`
	OperationID          string       `json:"operationId"`
	ServerID             string       `json:"serverId"`
	ClaimGeneration      string       `json:"claimGeneration"`
	CredentialGeneration string       `json:"credentialGeneration"`
	Retired              bool         `json:"retired"`
	Jobs                 []cleanupJob `json:"jobs"`
	IssuedAt             time.Time    `json:"issuedAt"`
	ExpiresAt            time.Time    `json:"expiresAt"`
}

// ConfigureClaimCleanup installs a durable deny-only queue. Neither the normal
// credential nor current installed-claim admission is required to drain it.
func (s *Service) ConfigureClaimCleanup(keys *networking.ProtectedKeys) error {
	if s.current == nil || keys == nil {
		return networking.ErrInvalid
	}
	// The queue tables come from migration 0040.
	s.current.cleanup = &claimCleanup{keys: keys, transport: s.current.transport}
	return nil
}

var cleanupID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (s *Service) verifyCleanup(ctx context.Context, w networking.CleanupWire, envelope networking.PolicyEnvelope) (cleanupResult, error) {
	var r cleanupResult
	invalid := identity.ErrUnauthorized
	raw, e := s.verifyEnvelope(ctx, envelope)
	if e != nil {
		return r, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil {
		return r, invalid
	}
	canonical, _ := json.Marshal(r)
	if !bytes.Equal(canonical, raw) {
		return r, invalid
	}
	expires, e := time.Parse(time.RFC3339Nano, w.ExpiresAt)
	if e != nil {
		return r, invalid
	}
	issued, _ := time.Parse(time.RFC3339Nano, w.IssuedAt)
	now := time.Now().UTC()
	if r.Kind != "portico.claim.cleanup.result" || r.RequestID != w.RequestID || r.OperationID != w.OperationID || r.ServerID != w.ServerID || r.ClaimGeneration != w.ClaimGeneration || r.CredentialGeneration != w.CredentialGeneration || !r.ExpiresAt.Equal(expires) || !r.ExpiresAt.After(now) || r.IssuedAt.Before(issued.Add(-5*time.Second)) || r.IssuedAt.After(now.Add(5*time.Second)) || !r.IssuedAt.Before(r.ExpiresAt) || len(r.Jobs) > 10 {
		return r, invalid
	}
	seen := map[string]bool{}
	for _, j := range r.Jobs {
		if j.Kind != "portico.account-erasure.v1" || !cleanupID.MatchString(j.ID) || !cleanupID.MatchString(j.DeletionID) || !cleanupID.MatchString(j.AccountID) || j.ServerID != w.ServerID || j.Revision != 1 || j.Disposition != "delete_owned_personal_resources" || !j.IssuedAt.Equal(r.IssuedAt) || !j.ExpiresAt.Equal(r.ExpiresAt) || len(j.ProfileIDs) > 200 || seen[j.ID] || j.ID == w.AcknowledgeJobID {
			return r, invalid
		}
		seen[j.ID] = true
		profiles := map[string]bool{}
		for _, p := range j.ProfileIDs {
			if !cleanupID.MatchString(p) || profiles[p] {
				return r, invalid
			}
			profiles[p] = true
		}
	}
	return r, nil
}
func cleanupDigest(j cleanupJob) string {
	j.IssuedAt = time.Time{}
	j.ExpiresAt = time.Time{}
	raw, _ := json.Marshal(j)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func (s *Service) applyCleanupJob(ctx context.Context, tx *sql.Tx, v networking.Intent, j cleanupJob) error {
	digest := cleanupDigest(j)
	var previous string
	e := tx.QueryRowContext(ctx, `SELECT target_digest FROM hosted_claim_cleanup_receipts WHERE operation_id=? AND job_id=?`, v.OperationID, j.ID).Scan(&previous)
	if e == nil {
		if previous != digest {
			return networking.ErrStale
		}
		return nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	// This is an account-erasure job: remove account-wide recording inheritance
	// as well as exact profile grants. Claim retirement alone is not this authority.
	var grants bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='dvr_recording_grants')`).Scan(&grants); e != nil {
		return e
	}
	if grants {
		if _, e = tx.ExecContext(ctx, `DELETE FROM dvr_recording_grants WHERE authority='hosted' AND account_id=?`, j.AccountID); e != nil {
			return e
		}
	}
	for _, profile := range j.ProfileIDs {
		viewer := identity.Viewer{Authority: "hosted", AccountID: j.AccountID, ProfileID: profile}
		if e = s.revokeRestrictedFamilies(tx, j.AccountID, profile); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE session_hash IN(SELECT hash FROM authorization_access WHERE authority='hosted' AND account_id=? AND profile_id=?)`, j.AccountID, profile); e != nil {
			return e
		}
		if e = identity.EraseProfileSavedDataTx(ctx, tx, viewer); e != nil {
			return e
		}
		key := identity.PersonalKey(viewer)
		for _, q := range []string{`DELETE FROM download_grants WHERE profile_key=?`, `DELETE FROM download_progress_marks WHERE profile_key=?`, `DELETE FROM download_receipts WHERE profile_key=?`, `DELETE FROM download_preparations WHERE profile_key=?`} {
			if _, e = tx.ExecContext(ctx, q, key); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM hosted_profile_revisions WHERE account_id=? AND profile_id=?`, j.AccountID, profile); e != nil {
			return e
		}
	}
	if e = eraseLinkedAccountTx(ctx, tx, j.AccountID); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO hosted_claim_cleanup_receipts(operation_id,job_id,revision,target_digest) VALUES(?,?,?,?)`, v.OperationID, j.ID, j.Revision, digest)
	return e
}

// eraseLinkedAccountTx removes the account a deleted Portico Account had here
// (migration 0120): its profiles' saved data, its sessions and the account.
// The owner's account is only unlinked, because it is also this server's
// recovery account; Hosted retires the claim of an owner that deletes itself.
// The journal trigger then tells Hosted's index, which already forgot it.
func eraseLinkedAccountTx(ctx context.Context, tx *sql.Tx, hostedAccount string) error {
	var account, role string
	e := tx.QueryRowContext(ctx, `SELECT l.account_id,m.role FROM account_portico_links l JOIN direct_memberships m ON m.account_id=l.account_id WHERE l.hosted_account_id=?`, hostedAccount).Scan(&account, &role)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if role == identity.TierOwner {
		_, e = tx.ExecContext(ctx, `DELETE FROM account_portico_links WHERE account_id=?`, account)
		return e
	}
	if e = identity.RevokeFamiliesMatchingTx(ctx, tx, identity.RevokedMembershipRemoved, `authority='local' AND account_id=?`, account); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE account_id=? AND state<>'stopped'`, account); e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM direct_profiles WHERE account_id=?`, account)
	if e != nil {
		return e
	}
	var profiles []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		profiles = append(profiles, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, profile := range profiles {
		if e = identity.EraseProfileSavedDataTx(ctx, tx, identity.Viewer{Authority: "local", AccountID: account, ProfileID: profile}); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, account)
	return e
}

// One exchange per pass bounds work. Applied jobs are acknowledged on the next
// pass, including after restart, only after the erasure transaction committed.
func (s *Service) cleanupControlStep(ctx context.Context) (time.Time, error) {
	if s.current.cleanup == nil {
		return time.Time{}, nil
	}
	now := time.Now().UTC()
	var operation string
	var next int64
	var attempts int
	e := s.db.QueryRowContext(ctx, `SELECT i.operation_id,COALESCE(c.next_at,0),COALESCE(c.attempts,0) FROM networking_claim_intents i LEFT JOIN hosted_claim_cleanup_schedule c ON c.operation_id=i.operation_id WHERE i.claim_generation<>'' AND COALESCE(c.complete,0)=0 ORDER BY COALESCE(c.next_at,0),i.operation_id LIMIT 1`).Scan(&operation, &next, &attempts)
	if errors.Is(e, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if e != nil {
		return now.Add(time.Minute), e
	}
	if now.UnixMilli() < next {
		return time.UnixMilli(next), nil
	}
	v, e := s.current.store.Load(ctx, operation)
	if e != nil {
		return now.Add(time.Minute), e
	}
	var ack string
	e = s.db.QueryRowContext(ctx, `SELECT job_id FROM hosted_claim_cleanup_receipts WHERE operation_id=? AND acknowledged=0 ORDER BY job_id LIMIT 1`, operation).Scan(&ack)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return now.Add(time.Minute), e
	}
	w, proof, e := s.current.cleanup.keys.CleanupProof(ctx, v, s.origin, ack)
	var result cleanupResult
	if e == nil {
		var envelope networking.PolicyEnvelope
		envelope, e = s.current.cleanup.transport.ClaimCleanup(ctx, v, proof)
		if e == nil {
			result, e = s.verifyCleanup(ctx, w, envelope)
		}
	}
	due := now.Add(7 * 24 * time.Hour)
	if e != nil {
		attempts = min(attempts+1, 8)
		due = networking.RetryAt(now, 5*time.Second, attempts, 8, networking.ControlRetryAt(e))
		save := s.current.store.WithCleanupTransaction(ctx, v, false, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO hosted_claim_cleanup_schedule(operation_id,next_at,attempts) VALUES(?,?,?) ON CONFLICT(operation_id) DO UPDATE SET next_at=excluded.next_at,attempts=excluded.attempts`, operation, due.UnixMilli(), attempts)
			return err
		})
		if save != nil {
			return due, save
		}
		return due, e
	}
	if len(result.Jobs) > 0 {
		due = now
	}
	e = s.current.store.WithCleanupTransaction(ctx, v, result.Retired, func(ctx context.Context, tx *sql.Tx) error {
		// Nothing is acknowledged, nor the claim retired, if any target fails erasure.
		for _, j := range result.Jobs {
			if e := s.applyCleanupJob(ctx, tx, v, j); e != nil {
				return e
			}
		}
		if ack != "" {
			if _, e := tx.ExecContext(ctx, `UPDATE hosted_claim_cleanup_receipts SET acknowledged=1 WHERE operation_id=? AND job_id=?`, operation, ack); e != nil {
				return e
			}
		}
		var pending int
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM hosted_claim_cleanup_receipts WHERE operation_id=? AND acknowledged=0`, operation).Scan(&pending); e != nil {
			return e
		}
		if pending > 0 {
			due = now
		}
		complete := result.Retired && len(result.Jobs) == 0 && pending == 0
		_, e := tx.ExecContext(ctx, `INSERT INTO hosted_claim_cleanup_schedule(operation_id,next_at,attempts,retired,complete) VALUES(?,?,0,?,?) ON CONFLICT(operation_id) DO UPDATE SET next_at=excluded.next_at,attempts=0,retired=MAX(retired,excluded.retired),complete=excluded.complete`, operation, due.UnixMilli(), result.Retired, complete)
		return e
	})
	if e != nil {
		return now.Add(time.Minute), e
	}
	// Another original operation may already be due (e.g. a successor claim).
	var earliest sql.NullInt64
	e = s.db.QueryRowContext(ctx, `SELECT MIN(COALESCE(c.next_at,0)) FROM networking_claim_intents i LEFT JOIN hosted_claim_cleanup_schedule c ON c.operation_id=i.operation_id WHERE i.claim_generation<>'' AND COALESCE(c.complete,0)=0`).Scan(&earliest)
	if e != nil {
		return due, e
	}
	if earliest.Valid {
		return time.UnixMilli(earliest.Int64), nil
	}
	return time.Time{}, nil
}

// Active claims have no cleanup timer: ordinary policy/check-in traffic and
// terminal events announce work. Original-key retries remain durable.
func (s *Service) cleanupNeeded(ctx context.Context) (bool, error) {
	if s.current.cleanup == nil {
		return false, nil
	}
	var needed bool
	e := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM networking_claim_intents i LEFT JOIN hosted_claim_cleanup_schedule c ON c.operation_id=i.operation_id WHERE i.claim_generation<>'' AND COALESCE(c.complete,0)=0 AND (i.stage<>'installed' OR COALESCE(c.attempts,0)>0 OR COALESCE(c.retired,0)=1 OR EXISTS(SELECT 1 FROM hosted_claim_cleanup_receipts r WHERE r.operation_id=i.operation_id AND acknowledged=0) OR EXISTS(SELECT 1 FROM networking_claim_authority a WHERE a.operation_id=i.operation_id AND a.state<>'active')))`).Scan(&needed)
	return needed, e
}
func (s *Service) requestClaimCleanup(ctx context.Context) error {
	if s.current.cleanup == nil {
		return nil
	}
	v, e := s.current.store.InstalledIntent(ctx)
	if e != nil {
		return e
	}
	return s.current.store.WithCleanupTransaction(ctx, v, false, func(ctx context.Context, tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO hosted_claim_cleanup_schedule(operation_id,next_at,attempts) VALUES(?,0,1) ON CONFLICT(operation_id) DO UPDATE SET next_at=0,attempts=MAX(attempts,1)`, v.OperationID)
		return e
	})
}
