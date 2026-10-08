package postgres

import (
	"bytes"
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"time"

	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/security"
)

// Receipt/cleanup rows deliberately outlive account and claim erasure. They hold
// hashes and public material, never a server credential or TLS private key.
// All positive mutations lock account -> claim -> server -> certificate order.
const certificateColumns = `id,request_id,operation_id,server_id,account_id,claim_generation,credential_generation,token_hash,public_key,namespace,issuer,csr,state,provider_url,COALESCE(chain,''::bytea),not_before,not_after,created_at,next_attempt_at,attempts,lease_token,cancel_requested,revocation_done,error_code,creation_attempted`

type certificateScanner interface{ Scan(...any) error }

func scanCertificate(row certificateScanner) (certificates.Order, error) {
	var o certificates.Order
	var before, after sql.NullTime
	e := row.Scan(&o.ID, &o.RequestID, &o.Authority.OperationID, &o.Authority.ServerID, &o.Authority.AccountID, &o.Authority.ClaimGeneration, &o.Authority.CredentialGeneration, &o.Authority.TokenHash, &o.Authority.PublicKey, &o.Namespace, &o.Issuer, &o.CSR, &o.State, &o.ProviderURL, &o.Chain, &before, &after, &o.CreatedAt, &o.NextAttempt, &o.Attempts, &o.Lease, &o.CancelRequested, &o.RevocationDone, &o.ErrorCode, &o.CreateAttempted)
	if before.Valid {
		o.NotBefore = before.Time
	}
	if after.Valid {
		o.NotAfter = after.Time
	}
	return o, e
}
func sameCertificateAuthority(a, b directory.ServerAuthority) bool {
	return a.Valid() && b.Valid() && a.OperationID == b.OperationID && a.ServerID == b.ServerID && a.AccountID == b.AccountID && a.ClaimGeneration == b.ClaimGeneration && a.CredentialGeneration == b.CredentialGeneration && bytes.Equal(a.TokenHash, b.TokenHash) && bytes.Equal(a.PublicKey, b.PublicKey)
}
func (s *Store) SubmitCertificate(ctx context.Context, a directory.ServerAuthority, request, namespace, issuer string, csr []byte) (certificates.Order, error) {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return certificates.Order{}, e
	}
	defer tx.Rollback()
	ns, e := directNamespaceTx(ctx, tx, a.OperationID)
	if e != nil {
		return certificates.Order{}, e
	}
	if ns != namespace {
		return certificates.Order{}, certificates.ErrInvalid
	}
	old, e := scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE operation_id=$1 AND request_id=$2 FOR UPDATE`, a.OperationID, request))
	if e == nil {
		if !sameCertificateAuthority(a, old.Authority) || !bytes.Equal(csr, old.CSR) {
			return certificates.Order{}, certificates.ErrConflict
		}
		return old, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return certificates.Order{}, e
	}
	// Fence earlier credentials without erasing cleanup identities.
	_, e = tx.ExecContext(ctx, `UPDATE certificate_orders SET cancel_requested=true,state='cancelled',next_attempt_at=now(),updated_at=now() WHERE operation_id=$1 AND (claim_generation<>$2 OR credential_generation<>$3 OR token_hash<>$4) AND NOT cancel_requested`, a.OperationID, a.ClaimGeneration, a.CredentialGeneration, a.TokenHash)
	if e != nil {
		return certificates.Order{}, e
	}
	var active bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM certificate_orders WHERE operation_id=$1 AND (state IN ('queued','creating','ordering','finalizing','reconciliation_required') OR (cancel_requested AND creation_attempted AND provider_url='' AND NOT revocation_done)))`, a.OperationID).Scan(&active)
	if e != nil {
		return certificates.Order{}, e
	}
	if active {
		return certificates.Order{}, certificates.ErrConflict
	}
	// Hard provider-abuse budget survives new idempotency keys, restarts, errors and re-claims.
	// Existing idempotent receipts above are always returned, even before due time.
	//
	// The daily counts are keyed on the server and the account, not the claim operation: every
	// re-claim mints a fresh operation (and namespace), so an operation-keyed budget reset
	// itself for any server that was unclaimed and claimed in a loop. The renewal gate stays per
	// operation because a new namespace does need a new certificate. It sits below the
	// earliest moment a server renews (58% of the lifetime).
	var recent, recentAccount int
	var next sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT
  (SELECT count(*) FROM certificate_orders WHERE server_id=$2 AND created_at>now()-interval '24 hours'),
  (SELECT count(*) FROM certificate_orders WHERE account_id=$3 AND created_at>now()-interval '24 hours'),
  (SELECT max(CASE WHEN state='issued' AND NOT cancel_requested THEN not_before+(not_after-not_before)*0.55 END) FROM certificate_orders WHERE operation_id=$1)`, a.OperationID, a.ServerID, a.AccountID).Scan(&recent, &recentAccount, &next)
	if e != nil {
		return certificates.Order{}, e
	}
	if recent >= 5 || recentAccount >= 20 || next.Valid && time.Now().Before(next.Time) {
		return certificates.Order{}, certificates.ErrRateLimited
	}
	id := security.RandomID("cert_")
	_, e = tx.ExecContext(ctx, `INSERT INTO certificate_orders(id,request_id,operation_id,server_id,account_id,claim_generation,credential_generation,token_hash,public_key,namespace,issuer,csr,csr_hash,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'queued')`, id, request, a.OperationID, a.ServerID, a.AccountID, a.ClaimGeneration, a.CredentialGeneration, a.TokenHash, a.PublicKey, namespace, issuer, csr, certificates.Hash(csr))
	if e != nil {
		return certificates.Order{}, e
	}
	o, e := scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE id=$1`, id))
	if e != nil {
		return o, e
	}
	// The order exists the instant this commits; say so rather than leaving the workers to
	// discover it on their own clock.
	if e = notifyTx(ctx, tx, CertificateChannel); e != nil {
		return certificates.Order{}, e
	}
	return o, tx.Commit()
}
func (s *Store) ReadCertificate(ctx context.Context, a directory.ServerAuthority, id string) (certificates.Order, error) {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return certificates.Order{}, e
	}
	defer tx.Rollback()
	o, e := scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE id=$1 AND operation_id=$2`, id, a.OperationID))
	if errors.Is(e, sql.ErrNoRows) {
		return o, directory.ErrDenied
	}
	if e != nil {
		return o, e
	}
	if !sameCertificateAuthority(a, o.Authority) {
		return certificates.Order{}, directory.ErrDenied
	}
	return o, tx.Commit()
}
func (s *Store) CancelCertificate(ctx context.Context, a directory.ServerAuthority, id string) error {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	o, e := scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE id=$1 AND operation_id=$2 FOR UPDATE`, id, a.OperationID))
	if e != nil {
		return serverAuthorityError(e)
	}
	if !sameCertificateAuthority(a, o.Authority) {
		return directory.ErrDenied
	}
	// Do not steal an in-flight lease: late publication must precede cleanup.
	_, e = tx.ExecContext(ctx, `UPDATE certificate_orders SET cancel_requested=true,state='cancelled',next_attempt_at=now(),updated_at=now() WHERE id=$1`, id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ACMEAccount(ctx context.Context, issuer, directory string, candidate []byte) (certificates.Account, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return certificates.Account{}, e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO certificate_acme_accounts(issuer,directory_url,key_cipher) VALUES($1,$2,$3) ON CONFLICT(issuer) DO NOTHING`, issuer, directory, candidate)
	if e != nil {
		return certificates.Account{}, e
	}
	var a certificates.Account
	var actual string
	e = tx.QueryRowContext(ctx, `SELECT directory_url,key_cipher,account_url FROM certificate_acme_accounts WHERE issuer=$1`, issuer).Scan(&actual, &a.Cipher, &a.URL)
	if e != nil {
		return a, e
	}
	if actual != directory {
		return a, certificates.ErrConfiguration
	}
	return a, tx.Commit()
}
func (s *Store) SaveACMEAccountURL(ctx context.Context, issuer, url string) error {
	r, e := s.DB.ExecContext(ctx, `UPDATE certificate_acme_accounts SET account_url=$2 WHERE issuer=$1 AND (account_url='' OR account_url=$2)`, issuer, url)
	return certificateAffected(r, e)
}
func certificateAffected(r sql.Result, e error) error {
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return certificates.ErrLease
	}
	return nil
}

// Failed validation can still follow external issuance (including reused DNS
// authorizations with no challenge row). Such receipts always need deny-only
// cleanup, including failures persisted by an older worker.
// This short transaction locks ONLY a job. It commits before acquiring claim
// locks or doing network I/O. SKIP LOCKED and due-time ordering avoid hot-looping
// one failing account while starving other accounts.
func (s *Store) LeaseCertificate(ctx context.Context, issuer, lease string, cleanupOnly bool) (certificates.Order, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return certificates.Order{}, e
	}
	defer tx.Rollback()
	var id string
	if e = reconcileCertificateAuthorityTx(ctx, tx, issuer); e != nil {
		return certificates.Order{}, e
	}
	e = tx.QueryRowContext(ctx, certificateWorkSQL, issuer, cleanupOnly).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return certificates.Order{}, err
		}
		return certificates.Order{}, sql.ErrNoRows
	}
	if e != nil {
		return certificates.Order{}, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE certificate_orders SET lease_token=$2,lease_until=now()+interval '90 seconds',attempts=attempts+1,updated_at=now() WHERE id=$1`, id, lease)
	if e != nil {
		return certificates.Order{}, e
	}
	o, e := scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE id=$1`, id))
	if e != nil {
		return o, e
	}
	return o, tx.Commit()
}
func (s *Store) certificateLeaseTx(ctx context.Context, o certificates.Order, positive bool) (*sql.Tx, error) {
	var tx *sql.Tx
	var e error
	if positive {
		tx, e = s.beginServerOperation(ctx, o.Authority)
	} else {
		tx, e = s.DB.BeginTx(ctx, nil)
	}
	if e != nil {
		return nil, e
	}
	var cancelled bool
	var state string
	e = tx.QueryRowContext(ctx, `SELECT cancel_requested,state FROM certificate_orders WHERE id=$1 AND lease_token=$2 AND lease_until>now() FOR UPDATE`, o.ID, o.Lease).Scan(&cancelled, &state)
	if e != nil || positive && (cancelled || state == "cancelled" || state == "revoked" || state == "failed") {
		tx.Rollback()
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		return nil, certificates.ErrLease
	}
	return tx, nil
}
func (s *Store) CheckCertificateLease(ctx context.Context, o certificates.Order, positive bool) error {
	tx, e := s.certificateLeaseTx(ctx, o, positive)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	return tx.Commit()
}
func (s *Store) SetCertificateState(ctx context.Context, o certificates.Order, state, code string, next time.Time) error {
	positive := state == "queued" || state == "creating" || state == "ordering" || state == "finalizing"
	tx, e := s.certificateLeaseTx(ctx, o, positive)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE certificate_orders SET state=$3,error_code=$4,next_attempt_at=$5,updated_at=now(),authority_check_at=CASE WHEN $3='reconciliation_required' THEN now() ELSE authority_check_at END,creation_attempted=CASE WHEN $3='creating' THEN true WHEN $3='queued' THEN false ELSE creation_attempted END,cancel_requested=(cancel_requested OR $3 IN ('cancelled','failed')) WHERE id=$1 AND lease_token=$2`, o.ID, o.Lease, state, code, next)
	if e = certificateAffected(r, e); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) RecordProviderOrder(ctx context.Context, o certificates.Order, url string) error {
	tx, e := s.certificateLeaseTx(ctx, o, false)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// A provider receipt is deny-only evidence even after cancellation. Keeping it
	// must never revive positive authority. No differing URL can replace a receipt.
	r, e := tx.ExecContext(ctx, `UPDATE certificate_orders SET provider_url=$3,state=CASE WHEN cancel_requested THEN 'cancelled' ELSE 'ordering' END,updated_at=now() WHERE id=$1 AND lease_token=$2 AND (provider_url='' OR provider_url=$3)`, o.ID, o.Lease, url)
	if e = certificateAffected(r, e); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) SaveCertificateChain(ctx context.Context, o certificates.Order, chain []byte, leaf *x509.Certificate) error {
	tx, e := s.certificateLeaseTx(ctx, o, true)
	positive := e == nil
	if errors.Is(e, directory.ErrDenied) || errors.Is(e, certificates.ErrLease) {
		tx, e = s.certificateLeaseTx(ctx, o, false)
	}
	if e != nil {
		return e
	}
	defer tx.Rollback()
	state := "issued"
	if !positive {
		state = "cancelled"
	}
	r, e := tx.ExecContext(ctx, `UPDATE certificate_orders SET chain=$3,not_before=$4,not_after=$5,state=$6,authority_check_at=now(),cancel_requested=(cancel_requested OR $6='cancelled'),error_code='',next_attempt_at=now(),updated_at=now() WHERE id=$1 AND lease_token=$2`, o.ID, o.Lease, chain, leaf.NotBefore, leaf.NotAfter, state)
	if e = certificateAffected(r, e); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) CertificateChallenges(ctx context.Context, o certificates.Order) ([]certificates.Challenge, error) {
	if e := s.CheckCertificateLease(ctx, o, false); e != nil {
		return nil, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT authorization_url,challenge_url,namespace,txt_value,cleanup_done,cleanup_after FROM certificate_challenges WHERE order_id=$1 ORDER BY authorization_url`, o.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []certificates.Challenge
	for rows.Next() {
		var c certificates.Challenge
		if e = rows.Scan(&c.AuthorizationURL, &c.ChallengeURL, &c.Namespace, &c.Value, &c.CleanupDone, &c.CleanupAfter); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) OwnCertificateChallenge(ctx context.Context, o certificates.Order, c certificates.Challenge) error {
	tx, e := s.certificateLeaseTx(ctx, o, true)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if c.Namespace != o.Namespace {
		return certificates.ErrInvalid
	}
	// Reserve cleanup before sending an UPDATE. TSIG replay time plus lease/drain
	// time is allowed to pass before cleanup is considered final.
	r, e := tx.ExecContext(ctx, `INSERT INTO certificate_challenges(order_id,authorization_url,challenge_url,namespace,txt_value,cleanup_after) VALUES($1,$2,$3,$4,$5,now()+interval '6 minutes') ON CONFLICT(order_id,authorization_url) DO UPDATE SET cleanup_done=false,cleanup_after=now()+interval '6 minutes' WHERE certificate_challenges.challenge_url=EXCLUDED.challenge_url AND certificate_challenges.namespace=EXCLUDED.namespace AND certificate_challenges.txt_value=EXCLUDED.txt_value`, o.ID, c.AuthorizationURL, c.ChallengeURL, c.Namespace, c.Value)
	if e = certificateAffected(r, e); e != nil {
		return e
	}
	if e = updateCertificateCleanupDueTx(ctx, tx, o.ID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) FinishCertificateCleanup(ctx context.Context, o certificates.Order, c certificates.Challenge) error {
	tx, e := s.certificateLeaseTx(ctx, o, false)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE certificate_challenges SET cleanup_done=true WHERE order_id=$1 AND authorization_url=$2 AND namespace=$3 AND txt_value=$4 AND cleanup_after<=now()`, o.ID, c.AuthorizationURL, c.Namespace, c.Value)
	if e = certificateAffected(r, e); e != nil {
		return e
	}
	if e = updateCertificateCleanupDueTx(ctx, tx, o.ID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) FinishCertificateRevocation(ctx context.Context, o certificates.Order) error {
	tx, e := s.certificateLeaseTx(ctx, o, false)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE certificate_orders SET revocation_done=true,state='revoked',cancel_requested=true,updated_at=now() WHERE id=$1 AND lease_token=$2`, o.ID, o.Lease)
	if e = certificateAffected(r, e); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ReleaseCertificate(ctx context.Context, o certificates.Order, next time.Time, code string) error {
	r, e := s.DB.ExecContext(ctx, `UPDATE certificate_orders SET lease_token='',lease_until=NULL,next_attempt_at=$3,error_code=CASE WHEN state='reconciliation_required' THEN 'acceptance_uncertain' ELSE $4 END,updated_at=now() WHERE id=$1 AND lease_token=$2`, o.ID, o.Lease, next, code)
	return certificateAffected(r, e)
}
func (s *Store) ReconcileCertificateURL(ctx context.Context, id, issuer, url string) error {
	// Protected operator-only receipt recovery; cannot install or uncancel an order.
	r, e := s.DB.ExecContext(ctx, `UPDATE certificate_orders SET provider_url=$3,state=CASE WHEN cancel_requested THEN 'cancelled' ELSE 'ordering' END,error_code='',next_attempt_at=now(),updated_at=now() WHERE id=$1 AND issuer=$2 AND state='reconciliation_required' AND provider_url='' AND (lease_until IS NULL OR lease_until<now())`, id, issuer, url)
	return certificateAffected(r, e)
}

// ReconciliationCandidate is read by the operator's authenticated CLI only.
func (s *Store) ReconciliationCandidate(ctx context.Context, id string) (certificates.Order, error) {
	return scanCertificate(s.DB.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE id=$1 AND state='reconciliation_required'`, id))
}

var _ certificates.Repository = (*Store)(nil)

func (s *Store) LookupCertificate(ctx context.Context, a directory.ServerAuthority, request string) (certificates.Order, error) {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return certificates.Order{}, e
	}
	defer tx.Rollback()
	o, e := scanCertificate(tx.QueryRowContext(ctx, `SELECT `+certificateColumns+` FROM certificate_orders WHERE operation_id=$1 AND request_id=$2`, a.OperationID, request))
	if errors.Is(e, sql.ErrNoRows) {
		return o, certificates.ErrNotFound
	}
	if e != nil {
		return o, e
	}
	if !sameCertificateAuthority(a, o.Authority) {
		return certificates.Order{}, directory.ErrDenied
	}
	return o, tx.Commit()
}

// OutstandingCertificateIssuers is startup configuration validation, not a UI
// projection. Never silently strand a previous issuer's cleanup/revocation work.
func (s *Store) OutstandingCertificateIssuers(ctx context.Context) ([]string, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT DISTINCT c.issuer FROM certificate_orders c WHERE
 c.state IN ('queued','creating','ordering','finalizing','reconciliation_required') OR
 (c.state='failed' AND NOT c.revocation_done) OR
 (c.cancel_requested AND NOT c.revocation_done) OR
 (c.state='issued' AND c.not_after>now()) OR
 EXISTS(SELECT 1 FROM certificate_challenges t WHERE t.order_id=c.id AND NOT t.cleanup_done)`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// certificateAdmissionLock serialises admissions so the count and the insert are one decision
// across every Hosted instance.
const certificateAdmissionLock = 0x706f727469636f02

// AdmitCertificateCreation spends one slot of the issuer's per-minute order budget. When the
// last minute is full it takes nothing, gives back the attempt the lease charged (waiting for
// quota is not a failure and must not inflate the order's later backoff) and returns how long
// until the oldest admission leaves the window.
func (s *Store) AdmitCertificateCreation(ctx context.Context, o certificates.Order, perMinute int) (time.Duration, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(certificateAdmissionLock)); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM certificate_order_admissions WHERE admitted_at<clock_timestamp()-interval '10 minutes'`); e != nil {
		return 0, e
	}
	var used int
	var oldest sql.NullTime
	var now time.Time
	e = tx.QueryRowContext(ctx, `SELECT count(*),min(admitted_at),clock_timestamp() FROM certificate_order_admissions WHERE issuer=$1 AND admitted_at>clock_timestamp()-interval '1 minute'`, o.Issuer).Scan(&used, &oldest, &now)
	if e != nil {
		return 0, e
	}
	if used >= perMinute {
		if _, e = tx.ExecContext(ctx, `UPDATE certificate_orders SET attempts=GREATEST(attempts-1,0) WHERE id=$1`, o.ID); e != nil {
			return 0, e
		}
		wait := time.Second
		if oldest.Valid {
			wait = max(oldest.Time.Add(time.Minute).Sub(now), time.Second)
		}
		return wait, tx.Commit()
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO certificate_order_admissions(issuer,order_id) VALUES($1,$2)`, o.Issuer, o.ID); e != nil {
		return 0, e
	}
	return 0, tx.Commit()
}
