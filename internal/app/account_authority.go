package app

import (
	"context"
	"database/sql"
	"strings"
)

// revokeAccountAuthorityTx is the single terminal credential boundary used
// when an account is disabled, removed, or loses server membership. It never
// clears revocation markers and is safe to call repeatedly.
func (s *Server) revokeAccountAuthorityTx(ctx context.Context, tx *sql.Tx, accountID, now string) error {
	return s.revokeAccountAuthorityExceptSessionTx(ctx, tx, accountID, "", "", now)
}

// revokeAccountPolicyCredentialsTx advances an account's authorization fence
// after an owner changes permissions or policy. It terminates active and
// reusable credentials without terminally revoking the durable device records
// that device allowlists and later sign-ins still reference. Terminal account,
// membership, and explicit device revocation continue to use
// revokeAccountAuthorityTx.
func (s *Server) revokeAccountPolicyCredentialsTx(ctx context.Context, tx *sql.Tx, accountID, now string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM sessions WHERE user_id = ?`, []any{accountID}},
		{`UPDATE native_refresh_tokens SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, []any{now, accountID}},
		{`UPDATE api_keys SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, []any{now, accountID}},
		{`DELETE FROM profile_selection_grants WHERE account_id = ?`, []any{accountID}},
		{`DELETE FROM profile_account_authentications WHERE account_id = ?`, []any{accountID}},
		{`DELETE FROM local_profile_admin_proofs WHERE account_id = ?`, []any{accountID}},
		{`UPDATE automatic_profile_selection_trusts SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END, updated_at = ? WHERE account_id = ?`, []any{now, now, accountID}},
		{`UPDATE playback_media_grants SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE principal_user_id = ?`, []any{now, accountID}},
		{`DELETE FROM media_download_grants WHERE principal_user_id = ?`, []any{accountID}},
		{`DELETE FROM playback_prepared_handoffs WHERE user_id = ?`, []any{accountID}},
		{`UPDATE playback_session_continuation_credentials SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, []any{now, accountID}},
		{`DELETE FROM cast_bootstraps WHERE user_id = ?`, []any{accountID}},
		{`UPDATE cast_receiver_sessions SET status = 'revoked', stopped_at = CASE WHEN stopped_at = '' THEN ? ELSE stopped_at END WHERE user_id = ? AND status = 'active'`, []any{now, accountID}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return s.revokeBrowserEntriesForUserTx(ctx, tx, accountID, "", now)
}

// revokeAccountAuthorityExceptSessionTx is used after an interactive password
// rotation. The authenticated browser session and its device row may survive
// long enough to return success, but all reusable switching credentials,
// refresh families, API keys, other devices, grants, and profile trusts are
// terminated in the same transaction.
func (s *Server) revokeAccountAuthorityExceptSessionTx(ctx context.Context, tx *sql.Tx, accountID, preservedSessionID, preservedDeviceID, now string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	preservedSessionID = strings.TrimSpace(preservedSessionID)
	preservedDeviceID = strings.TrimSpace(preservedDeviceID)
	statements := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM sessions WHERE user_id = ? AND (? = '' OR id <> ?)`, []any{accountID, preservedSessionID, preservedSessionID}},
		{`UPDATE native_refresh_tokens SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, []any{now, accountID}},
		{`UPDATE api_keys SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, []any{now, accountID}},
		{`UPDATE devices SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END, trusted = 0 WHERE user_id = ? AND (? = '' OR id <> ?)`, []any{now, accountID, preservedDeviceID, preservedDeviceID}},
		{`UPDATE devices SET trusted = 0 WHERE user_id = ? AND id = ?`, []any{accountID, preservedDeviceID}},
		{`DELETE FROM profile_selection_grants WHERE account_id = ?`, []any{accountID}},
		{`DELETE FROM profile_account_authentications WHERE account_id = ?`, []any{accountID}},
		{`DELETE FROM local_profile_admin_proofs WHERE account_id = ?`, []any{accountID}},
		{`UPDATE automatic_profile_selection_trusts SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END, updated_at = ? WHERE account_id = ?`, []any{now, now, accountID}},
		{`UPDATE playback_media_grants SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE principal_user_id = ?`, []any{now, accountID}},
		{`DELETE FROM media_download_grants WHERE principal_user_id = ?`, []any{accountID}},
		{`DELETE FROM playback_prepared_handoffs WHERE user_id = ?`, []any{accountID}},
		{`UPDATE playback_session_continuation_credentials SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, []any{now, accountID}},
		{`DELETE FROM cast_bootstraps WHERE user_id = ?`, []any{accountID}},
		{`UPDATE cast_receiver_sessions SET status = 'revoked', stopped_at = CASE WHEN stopped_at = '' THEN ? ELSE stopped_at END WHERE user_id = ? AND status = 'active'`, []any{now, accountID}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return s.revokeBrowserEntriesForUserTx(ctx, tx, accountID, preservedSessionID, now)
}
