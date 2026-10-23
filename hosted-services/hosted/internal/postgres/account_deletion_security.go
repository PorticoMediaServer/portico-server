package postgres

import (
	"context"
	"database/sql"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
)

func (s *AccountSecurity) prepareDeletionCurrentTx(ctx context.Context, tx *sql.Tx, expected identity.CurrentFamilyExpectation) (identity.DeletionPreview, error) {
	p, snapshot, e := deletionSnapshot(ctx, tx, expected.AccountID)
	if e != nil {
		return p, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_deletions WHERE account_id=$1 AND accepted_at IS NULL`, expected.AccountID); e != nil {
		return p, e
	}
	p.Token = security.RandomID("del_")
	e = tx.QueryRowContext(ctx, `INSERT INTO account_deletions(id,token_hash,account_id,snapshot_hash,expires_at,expected_family_id,expected_account_epoch,expected_family_generation,expected_access_hash,expected_session_mode) VALUES($1,$2,$3,$4,clock_timestamp()+interval '5 minutes',$5,$6,$7,$8,$9) RETURNING expires_at`, security.RandomID("deletion_"), security.Hash(p.Token), expected.AccountID, snapshot, expected.FamilyID, expected.AuthEpoch, expected.FamilyGeneration, expected.TokenHash, expected.Mode).Scan(&p.ExpiresAt)
	return p, e
}

// AcceptDeletion is the final, irreversible step. It needs the signed-in
// session that made the preview and the account's email typed as confirmation
// (the username for an account without an email), not a password (A80).
func (s *AccountSecurity) AcceptDeletion(ctx context.Context, access, token, disposition, confirmation string) (identity.DeletionReceipt, error) {
	if len(access) < 32 || len(access) > 128 || len(token) < 32 || len(token) > 128 {
		return identity.DeletionReceipt{}, identity.ErrCredentials
	}
	if disposition != "" || len(confirmation) > 320 {
		return identity.DeletionReceipt{}, identity.ErrInvalid
	}
	return s.Store.acceptDeletion(ctx, security.Hash(token), disposition, security.Hash(access), s, confirmation)
}
func (s *AccountSecurity) deletedIdentitiesTx(ctx context.Context, tx *sql.Tx, account string) error {
	rows, e := tx.QueryContext(ctx, `SELECT issuer,subject FROM account_external_identities WHERE account_id=$1`, account)
	if e != nil {
		return e
	}
	type pair struct{ issuer, subject string }
	subjects := []pair{}
	for rows.Next() {
		var p pair
		if e = rows.Scan(&p.issuer, &p.subject); e != nil {
			rows.Close()
			return e
		}
		subjects = append(subjects, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, p := range subjects {
		if _, e = tx.ExecContext(ctx, `INSERT INTO deleted_identity_markers(marker_key_id,subject_digest,deleted_at,expires_at) VALUES($1,$2,clock_timestamp(),clock_timestamp()+interval '15 minutes') ON CONFLICT(marker_key_id,subject_digest) DO UPDATE SET deleted_at=EXCLUDED.deleted_at,expires_at=EXCLUDED.expires_at`, s.markerKeyID(), s.digest("deleted-identity", p.issuer, p.subject)); e != nil {
			return e
		}
	}
	return nil
}

// LogoutAll still uses the existing epoch/family model, but may not be authorized
// by an old generation which ordinary access-token authentication still accepts.
func (s *AccountSecurity) LogoutAll(ctx context.Context, access string) error {
	tx, expected, e := s.authorize(ctx, access)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = revokeAccountTx(ctx, tx, expected.AccountID, "logout_all"); e != nil {
		return e
	}
	if e = audit(ctx, tx, expected.AccountID, "sessions.logout_all", expected.AccountID, expected.AuthEpoch); e != nil {
		return e
	}
	return tx.Commit()
}
