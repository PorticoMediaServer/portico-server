package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
)

// AuthorizeCurrentFamilyTx checks strict authority for new sensitive operations.
// The caller owns a bounded transaction and must take any browser binding lock
// before calling. This locks account UPDATE, then family SHARE, then access
// SHARE. Domain proof/approval locks follow; no external work belongs here.
// Ordinary access authentication deliberately remains on its existing path.
func AuthorizeCurrentFamilyTx(ctx context.Context, tx *sql.Tx, expected identity.CurrentFamilyExpectation) (identity.CurrentFamilyFacts, error) {
	var empty identity.CurrentFamilyFacts
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if tx == nil || !identity.ValidCurrentFamilyExpectation(expected) {
		return empty, identity.ErrCredentials
	}
	// Keep repository arguments independent from a caller's later slice reuse.
	hash := append([]byte(nil), expected.TokenHash...)
	var out identity.CurrentFamilyFacts
	err := tx.QueryRowContext(ctx, `SELECT id,auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, expected.AccountID).Scan(&out.AccountID, &out.AuthEpoch)
	if err != nil {
		return empty, currentFamilyError(ctx, err)
	}
	if out.AuthEpoch != expected.AuthEpoch {
		return empty, identity.ErrCredentials
	}
	var familyAccount string
	var familyEpoch int64
	var revoked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id,account_id,account_epoch,generation,mode,absolute_expires_at,idle_expires_at,revoked_at FROM session_families WHERE id=$1 FOR SHARE`, expected.FamilyID).Scan(
		&out.FamilyID, &familyAccount, &familyEpoch, &out.FamilyGeneration, &out.Mode, &out.AbsoluteExpiresAt, &out.IdleExpiresAt, &revoked)
	if err != nil {
		return empty, currentFamilyError(ctx, err)
	}
	if familyAccount != out.AccountID || familyEpoch != out.AuthEpoch || out.FamilyGeneration != expected.FamilyGeneration || out.Mode != expected.Mode || revoked.Valid {
		return empty, identity.ErrCredentials
	}
	var accessAccount string
	var accessFamily sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT account_id,family_id,expires_at FROM account_sessions WHERE token_hash=$1 FOR SHARE`, hash).Scan(&accessAccount, &accessFamily, &out.AccessExpiresAt)
	if err != nil {
		return empty, currentFamilyError(ctx, err)
	}
	if accessAccount != out.AccountID || !accessFamily.Valid || accessFamily.String != out.FamilyID {
		return empty, identity.ErrCredentials
	}
	// now() is transaction-start time and can predate a long lock wait.
	if err = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&out.CheckedAt); err != nil {
		return empty, currentFamilyError(ctx, err)
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if !out.UnexpiredAt(out.CheckedAt) {
		return empty, identity.ErrCredentials
	}
	return out, nil
}

func currentFamilyError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, sql.ErrNoRows) {
		return identity.ErrCredentials
	}
	return err
}
