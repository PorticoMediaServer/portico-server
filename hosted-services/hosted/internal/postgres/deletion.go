package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"time"
)

func deletionSnapshot(ctx context.Context, tx *sql.Tx, account string) (identity.DeletionPreview, []string, []byte, error) {
	p := identity.DeletionPreview{OwnedServers: []identity.DeletionServer{}, AffectedServers: []identity.DeletionServer{}}
	profiles := []string{}
	rows, e := tx.QueryContext(ctx, `SELECT id FROM profiles WHERE account_id=$1 ORDER BY id`, account)
	if e != nil {
		return p, nil, nil, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return p, nil, nil, e
		}
		profiles = append(profiles, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return p, nil, nil, e
	}
	rows, e = tx.QueryContext(ctx, `SELECT id,name,COALESCE(owner_account_id=$1,false) FROM servers WHERE owner_account_id=$1 OR id IN(SELECT server_id FROM memberships WHERE account_id=$1 UNION SELECT server_id FROM server_members WHERE account_id=$1) ORDER BY id LIMIT 1001`, account)
	if e != nil {
		return p, nil, nil, e
	}
	for rows.Next() {
		var v identity.DeletionServer
		var owned bool
		if e = rows.Scan(&v.ID, &v.Name, &owned); e != nil {
			rows.Close()
			return p, nil, nil, e
		}
		p.AffectedServers = append(p.AffectedServers, v)
		if owned {
			p.OwnedServers = append(p.OwnedServers, v)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return p, nil, nil, e
	}
	if len(p.AffectedServers) > 1000 {
		return p, nil, nil, identity.ErrDeletionConflict
	}
	p.ProfileCount = len(profiles)
	p.CanDelete = true
	if len(p.AffectedServers) > 0 {
		p.RequiredDisposition = identity.DeletePersonalResources
	}
	raw, _ := json.Marshal(struct {
		Profiles []string
		Servers  []identity.DeletionServer
		Owned    []identity.DeletionServer
	}{profiles, p.AffectedServers, p.OwnedServers})
	return p, profiles, security.Hash(string(raw)), nil
}
func (s *Store) PrepareDeletion(ctx context.Context, account string, password, token []byte, expiry time.Time) (identity.DeletionPreview, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return identity.DeletionPreview{}, e
	}
	defer tx.Rollback()
	var stored []byte
	if e = tx.QueryRowContext(ctx, `SELECT password_hash FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&stored); e != nil {
		return identity.DeletionPreview{}, identity.ErrCredentials
	}
	if !identity.PasswordInstalled(stored) || !identity.PasswordInstalled(password) || !bytes.Equal(stored, password) {
		return identity.DeletionPreview{}, identity.ErrDeletionConflict
	}
	p, _, snapshot, e := deletionSnapshot(ctx, tx, account)
	if e != nil {
		return p, e
	}
	// A new preview supersedes earlier unaccepted capabilities for this account.
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_deletions WHERE account_id=$1 AND accepted_at IS NULL`, account); e != nil {
		return p, e
	}
	p.ExpiresAt = expiry
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_deletions(id,token_hash,account_id,password_hash,snapshot_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, security.RandomID("deletion_"), token, account, password, snapshot, expiry); e != nil {
		return p, e
	}
	return p, tx.Commit()
}
func (s *Store) AcceptDeletion(ctx context.Context, token []byte, disposition string) (identity.DeletionReceipt, error) {
	return s.acceptDeletion(ctx, token, disposition, nil, nil, "")
}
func (s *Store) acceptDeletion(ctx context.Context, token []byte, disposition string, accessHash []byte, securityService *AccountSecurity, confirmation string) (identity.DeletionReceipt, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return identity.DeletionReceipt{}, e
	}
	defer tx.Rollback()
	// Resolve ownership without taking the preview lock. PrepareDeletion and
	// credential changes lock the account first; acceptance uses that same order.
	var id, account string
	e = tx.QueryRowContext(ctx, `SELECT account_id FROM account_deletions WHERE token_hash=$1`, token).Scan(&account)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.DeletionReceipt{}, identity.ErrCredentials
	}
	if e != nil {
		return identity.DeletionReceipt{}, e
	}
	var password []byte
	accountErr := tx.QueryRowContext(ctx, `SELECT password_hash FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&password)
	if accountErr != nil && !errors.Is(accountErr, sql.ErrNoRows) {
		return identity.DeletionReceipt{}, accountErr
	}
	var expected, snapshot []byte
	var expires time.Time
	var accepted sql.NullTime
	var prior sql.NullString
	var family, mode sql.NullString
	var epoch, generation sql.NullInt64
	var captured []byte
	// The preliminary account lookup is not authority. Reselect the exact
	// token/account under lock, including when another accept just deleted it.
	e = tx.QueryRowContext(ctx, `SELECT id,password_hash,snapshot_hash,expires_at,accepted_at,disposition,expected_family_id,expected_account_epoch,expected_family_generation,expected_access_hash,expected_session_mode FROM account_deletions WHERE token_hash=$1 AND account_id=$2 FOR UPDATE`, token, account).Scan(&id, &expected, &snapshot, &expires, &accepted, &prior, &family, &epoch, &generation, &captured, &mode)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.DeletionReceipt{}, identity.ErrCredentials
	}
	if e != nil {
		return identity.DeletionReceipt{}, e
	}
	if securityService != nil && (!family.Valid || !epoch.Valid || !generation.Valid || !equalSecret(accessHash, captured)) {
		return identity.DeletionReceipt{}, identity.ErrCredentials
	}
	if accepted.Valid {
		if prior.String != disposition {
			return identity.DeletionReceipt{}, identity.ErrDeletionConflict
		}
		r, e := deletionReceipt(ctx, tx, id)
		if e != nil {
			return r, e
		}
		return r, tx.Commit()
	}
	if accountErr != nil {
		return identity.DeletionReceipt{}, identity.ErrCredentials
	}
	var checkedAt time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&checkedAt); e != nil {
		return identity.DeletionReceipt{}, e
	}
	if !expires.After(checkedAt) {
		return identity.DeletionReceipt{}, identity.ErrDeletionConflict
	}
	if securityService != nil {
		expectation := identity.CurrentFamilyExpectation{AccountID: account, FamilyID: family.String, AuthEpoch: epoch.Int64, FamilyGeneration: generation.Int64, TokenHash: captured, Mode: mode.String}
		if _, e = AuthorizeCurrentFamilyTx(ctx, tx, expectation); e != nil {
			return identity.DeletionReceipt{}, e
		}
		var email, username string
		if e = tx.QueryRowContext(ctx, `SELECT a.username,COALESCE((SELECT c.address FROM account_contact_emails c WHERE c.account_id=a.id AND c.is_primary),'') FROM accounts a WHERE a.id=$1`, account).Scan(&username, &email); e != nil {
			return identity.DeletionReceipt{}, e
		}
		want := email
		if want == "" {
			want = username
		}
		if typed := strings.TrimSpace(confirmation); typed == "" || !strings.EqualFold(typed, want) {
			return identity.DeletionReceipt{}, identity.ErrDeletionConfirmation
		}
	} else if !identity.PasswordInstalled(password) || !identity.PasswordInstalled(expected) || !bytes.Equal(password, expected) {
		return identity.DeletionReceipt{}, identity.ErrDeletionConflict
	}
	p, profiles, current, e := deletionSnapshot(ctx, tx, account)
	if e != nil {
		return identity.DeletionReceipt{}, e
	}
	if !bytes.Equal(snapshot, current) || disposition != p.RequiredDisposition {
		return identity.DeletionReceipt{}, identity.ErrDeletionConflict
	}
	for _, server := range p.AffectedServers {
		if e = prepareServerErasureTx(ctx, tx, id, account, profiles, disposition, server.ID); e != nil {
			return identity.DeletionReceipt{}, e
		}
	}
	// Approval family FKs and operation constraints must be released before
	// account/profile deletion. Keep only original-key cleanup authority and
	// opaque predecessor identifiers, never bearer credentials or display data.
	if _, e = tx.ExecContext(ctx, `DELETE FROM claim_approvals WHERE account_id=$1`, account); e != nil {
		return identity.DeletionReceipt{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE claim_operations SET state='cancelled',cancelled_at=COALESCE(cancelled_at,clock_timestamp()),server_name='Retired server' WHERE account_id=$1`, account); e != nil {
		return identity.DeletionReceipt{}, e
	}

	// Provider links are erased only on the path that first records the
	// deleted-identity markers which fence a re-link of the same subject. Without
	// those markers the links must stay, so account_external_identities' RESTRICT
	// foreign key aborts the deletion rather than silently unlinking a subject
	// that an in-flight assertion could immediately re-attach.
	if securityService != nil {
		if e = securityService.deletedIdentitiesTx(ctx, tx, account); e != nil {
			return identity.DeletionReceipt{}, e
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM account_external_identities WHERE account_id=$1`, account); e != nil {
			return identity.DeletionReceipt{}, e
		}
	}
	// Foreign keys plus the account lock serialize concurrent grants/credential
	// issuance. No content/source data exists in this hosted graph.
	statements := []string{
		`UPDATE account_mail_outbox SET status='cancelled',ciphertext=NULL,lease_id=NULL,lease_until=NULL WHERE account_id=$1 AND status IN('pending','sending')`,
		`DELETE FROM account_registrations WHERE account_id=$1`,
		`DELETE FROM account_security_receipts WHERE account_id=$1`,
		`DELETE FROM identity_transactions WHERE expected_account_id=$1 OR id IN(SELECT transaction_id FROM identity_completion_receipts WHERE account_id=$1) OR id IN(SELECT identity_transaction_id FROM account_mfa_challenges WHERE account_id=$1)`,
		`DELETE FROM restrictions WHERE account_id=$1`,
		`DELETE FROM attachment_tickets WHERE account_id=$1`,
		`DELETE FROM invitations WHERE owner_id=$1 OR recipient_id=$1`,
		`DELETE FROM memberships WHERE account_id=$1`,
		`DELETE FROM device_authorizations WHERE approved_account_id=$1`,
		`DELETE FROM hosted_devices WHERE account_id=$1`,
		`DELETE FROM account_sessions WHERE account_id=$1`,
		`DELETE FROM refresh_receipts WHERE family_id IN(SELECT id FROM session_families WHERE account_id=$1)`,
		`DELETE FROM refresh_tokens WHERE family_id IN(SELECT id FROM session_families WHERE account_id=$1)`,
		`DELETE FROM session_families WHERE account_id=$1`,
		`DELETE FROM security_audit WHERE actor_id=$1 OR target_id=$1 OR target_id IN(SELECT id FROM profiles WHERE account_id=$1)`,
		`DELETE FROM profiles WHERE account_id=$1`,
		`DELETE FROM accounts WHERE id=$1`,
	}
	for _, q := range statements {
		if _, e = tx.ExecContext(ctx, q, account); e != nil {
			return identity.DeletionReceipt{}, e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_deletions SET accepted_at=now(),password_hash=NULL,snapshot_hash=NULL,disposition=$2,receipt_expires_at=now()+interval '90 days',completed_at=CASE WHEN $3=0 THEN now() END WHERE id=$1`, id, disposition, len(p.AffectedServers)); e != nil {
		return identity.DeletionReceipt{}, e
	}
	if e = audit(ctx, tx, id, "account.erasure.accepted", id, 1); e != nil {
		return identity.DeletionReceipt{}, e
	}
	r, e := deletionReceipt(ctx, tx, id)
	if e != nil {
		return r, e
	}
	return r, tx.Commit()
}
func deletionReceipt(ctx context.Context, tx *sql.Tx, id string) (identity.DeletionReceipt, error) {
	r := identity.DeletionReceipt{AccountDeleted: true, ID: id, PendingServers: []string{}, Status: "pending-offline-servers"}
	var complete sql.NullTime
	e := tx.QueryRowContext(ctx, `SELECT accepted_at,completed_at,receipt_expires_at FROM account_deletions WHERE id=$1 AND accepted_at IS NOT NULL AND receipt_expires_at>clock_timestamp()`, id).Scan(&r.AcceptedAt, &complete, &r.ReceiptExpiresAt)
	if errors.Is(e, sql.ErrNoRows) {
		return r, identity.ErrCredentials
	}
	if e != nil {
		return r, e
	}
	if complete.Valid {
		r.Status = "complete"
	}
	rows, e := tx.QueryContext(ctx, `SELECT server_id FROM account_erasure_jobs WHERE deletion_id=$1 AND acknowledged_at IS NULL ORDER BY server_id`, id)
	if e != nil {
		return r, e
	}
	defer rows.Close()
	for rows.Next() {
		var server string
		if e = rows.Scan(&server); e != nil {
			return r, e
		}
		r.PendingServers = append(r.PendingServers, server)
	}
	return r, rows.Err()
}
func (s *Store) DeletionStatus(ctx context.Context, token []byte) (identity.DeletionReceipt, error) {
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return identity.DeletionReceipt{}, e
	}
	defer tx.Rollback()
	var id string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM account_deletions WHERE token_hash=$1 AND accepted_at IS NOT NULL`, token).Scan(&id); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			e = identity.ErrCredentials
		}
		return identity.DeletionReceipt{}, e
	}
	r, e := deletionReceipt(ctx, tx, id)
	if e != nil {
		return r, e
	}
	return r, tx.Commit()
}
