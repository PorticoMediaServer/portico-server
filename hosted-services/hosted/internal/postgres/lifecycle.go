package postgres

import (
	"bytes"
	"context"
	"portico.local/hosted/internal/identity"
)

func (s *Store) RenameAccount(ctx context.Context, account, name string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `UPDATE accounts SET display_name=$2 WHERE id=$1`, account, name)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return identity.ErrCredentials
	}
	if e = audit(ctx, tx, account, "account.rename", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ChangePassword(ctx context.Context, account string, old, next []byte) error {
	if !identity.PasswordInstalled(next) {
		return identity.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var stored []byte
	if e = tx.QueryRowContext(ctx, `SELECT password_hash FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&stored); e != nil {
		return e
	}
	if !identity.PasswordInstalled(old) || !identity.PasswordInstalled(stored) || !bytes.Equal(old, stored) {
		return identity.ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `UPDATE accounts SET password_hash=$2,auth_epoch=auth_epoch+1 WHERE id=$1`, account, next); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE session_families SET revoked_at=COALESCE(revoked_at,now()),revoke_reason=COALESCE(revoke_reason,'password_change') WHERE account_id=$1`, account); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE account_id=$1`, account); e != nil {
		return e
	}
	if e = audit(ctx, tx, account, "account.password_change", account, 0); e != nil {
		return e
	}
	return tx.Commit()
}
