package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type AccountPasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleAccountPassword(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if user.AuthProvider == "api_key" {
		writeError(w, http.StatusForbidden, "interactive_session_required", "Password changes require a signed-in app session.")
		return
	}
	if !selectedProfileMayManageAccount(user) {
		writeError(w, http.StatusForbidden, "primary_profile_required", "Switch to the primary profile to manage account settings.")
		return
	}

	rateKey := strings.Join([]string{"account-password", user.ID, clientIPFromRequest(r)}, ":")
	if !s.allowLoginAttempt(rateKey) {
		w.Header().Set("Retry-After", strconv.Itoa(600))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many password change attempts. Try again later.")
		return
	}

	var req AccountPasswordChangeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.CurrentPassword == "" || len([]byte(req.CurrentPassword)) > 72 {
		writeError(w, http.StatusBadRequest, "invalid_credentials", "Enter the current password.")
		return
	}
	if !validAccountPassword(req.NewPassword) {
		writeError(w, http.StatusBadRequest, "invalid_password", accountPasswordPolicyMessage)
		return
	}
	if req.CurrentPassword == req.NewPassword {
		writeError(w, http.StatusBadRequest, "password_unchanged", "Choose a password that is different from the current password.")
		return
	}

	var currentHash string
	if err := s.queryUserRow(r.Context(), `SELECT COALESCE(password_hash, '') FROM users WHERE id = ?`, user.ID).Scan(&currentHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "account_not_found", "Account was not found.")
			return
		}
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "password_change_failed", "Unable to inspect the current password.")
		return
	}
	if strings.TrimSpace(currentHash) == "" {
		verifyAccountPassword("", req.CurrentPassword)
		s.recordLoginFailure(rateKey)
		s.recordAudit(r, user, "account.password_change_failed", "user", user.ID, "warn", nil)
		writeError(w, http.StatusConflict, "local_password_unavailable", "This profile does not have a This Server password.")
		return
	}

	currentValid, _ := verifyAccountPassword(currentHash, req.CurrentPassword)
	if !currentValid {
		s.recordLoginFailure(rateKey)
		s.recordAudit(r, user, "account.password_change_failed", "user", user.ID, "warn", nil)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Current password is incorrect.")
		return
	}
	s.clearLoginFailures(rateKey)
	replacement, err := hashAccountPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_hash_failed", "Unable to update password.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sessionID, err := s.currentSessionIDContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusUnauthorized, "interactive_session_required", "Password changes require a current interactive session.")
		return
	}
	var deviceID string
	if err := s.queryUserRow(r.Context(), `SELECT COALESCE(device_id, '') FROM sessions WHERE id = ? AND user_id = ?`, sessionID, accountIDForUser(user)).Scan(&deviceID); err != nil {
		writeProductError(w, http.StatusUnauthorized, "interactive_session_required", "Password changes require a current interactive session.")
		return
	}
	err = s.withUserTxTagged(r.Context(), []string{"users", "sessions", "native_refresh_tokens", "api_keys", "devices", "profile-trusts"}, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(r.Context(), `
			UPDATE users SET password_hash = ?, updated_at = ?
			WHERE id = ? AND password_hash = ?`, replacement, now, accountIDForUser(user), currentHash)
		if err != nil {
			return err
		}
		if rowsAffected(result) == 0 {
			return errPasswordChangedConcurrently
		}
		return s.revokeAccountAuthorityExceptSessionTx(r.Context(), tx, accountIDForUser(user), sessionID, deviceID, now)
	})
	if err != nil && !errors.Is(err, errPasswordChangedConcurrently) {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "password_change_failed", "Unable to update password.")
		return
	}
	if errors.Is(err, errPasswordChangedConcurrently) {
		writeError(w, http.StatusConflict, "password_changed_retry", "The password changed in another session. Sign in again and retry.")
		return
	}
	s.clearBrowserVaultCookies(r.Context(), w)

	s.recordAudit(r, user, "account.password_changed", "user", user.ID, "info", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

var errPasswordChangedConcurrently = errors.New("password changed concurrently")
