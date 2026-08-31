package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

var errCurrentAccountSession = errors.New("current account session cannot be revoked from this endpoint")

func (s *Server) handleAccountSessions(w http.ResponseWriter, r *http.Request, user User) {
	if user.AuthProvider == "api_key" {
		writeError(w, http.StatusForbidden, "interactive_session_required", "Account sessions are available only to signed-in app sessions.")
		return
	}
	if !selectedProfileMayManageAccount(user) {
		writeError(w, http.StatusForbidden, "primary_profile_required", "Switch to the primary profile to manage account settings.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.listAccountSessionsContext(r.Context(), user, s.currentSessionTokenHashes(r))
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "account_sessions_failed", "Unable to load account sessions.")
			return
		}
		writeJSON(w, http.StatusOK, ListResponse[AccountSession]{Items: sessions, Total: len(sessions)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
	}
}

func (s *Server) handleAccountSessionRoute(w http.ResponseWriter, r *http.Request, user User) {
	if user.AuthProvider == "api_key" {
		writeError(w, http.StatusForbidden, "interactive_session_required", "Account sessions are available only to signed-in app sessions.")
		return
	}
	if !selectedProfileMayManageAccount(user) {
		writeError(w, http.StatusForbidden, "primary_profile_required", "Switch to the primary profile to manage account settings.")
		return
	}
	sessionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/account/sessions/"), "/")
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeError(w, http.StatusNotFound, "not_found", "Account session route was not found.")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use DELETE for this endpoint.")
		return
	}
	if err := s.revokeAccountSessionContext(r.Context(), user, sessionID, s.currentSessionTokenHashes(r)); err != nil {
		switch {
		case errors.Is(err, errCurrentAccountSession):
			writeError(w, http.StatusConflict, "current_session", "Use Sign out to end the current session.")
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "account_session_not_found", "Account session was not found.")
		default:
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "account_session_revoke_failed", "Unable to sign out that session.")
		}
		return
	}
	s.recordAudit(r, user, "account.session.revoked", "session", sessionID, "warn", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listAccountSessionsContext(ctx context.Context, user User, currentHashes map[string]bool) ([]AccountSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.queryUserRead(ctx, `
		SELECT s.id, COALESCE(s.device_id, ''), COALESCE(NULLIF(d.display_name, ''), d.name, ''), COALESCE(d.app, ''), COALESCE(d.platform, ''),
			COALESCE(d.client_ip, ''), COALESCE(d.trusted, 0), COALESCE(NULLIF(s.auth_provider, ''), 'local'),
			s.token_hash, s.created_at, s.last_seen_at, s.expires_at
		FROM sessions s
		LEFT JOIN devices d ON d.id = s.device_id
		WHERE s.user_id = ? AND s.expires_at > ?
		ORDER BY s.last_seen_at DESC, s.created_at DESC`, user.ID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []AccountSession{}
	for rows.Next() {
		var session AccountSession
		var tokenHash string
		var trusted int
		if err := rows.Scan(&session.ID, &session.DeviceID, &session.DeviceName, &session.App, &session.Platform, &session.ClientIP, &trusted, &session.AuthProvider, &tokenHash, &session.CreatedAt, &session.LastSeenAt, &session.ExpiresAt); err != nil {
			return nil, err
		}
		session.AuthProvider = normalizeAuthProvider(session.AuthProvider)
		session.Trusted = trusted == 1
		session.Current = currentHashes[tokenHash]
		session.CanRevoke = !session.Current
		if strings.TrimSpace(session.DeviceName) == "" {
			session.DeviceName = firstNonEmpty(session.Platform, session.App, "Signed-in device")
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Server) revokeAccountSessionContext(ctx context.Context, user User, sessionID string, currentHashes map[string]bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return sql.ErrNoRows
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var tokenHash string
	if err := s.queryUserRow(ctx, `SELECT token_hash FROM sessions WHERE id = ? AND user_id = ?`, sessionID, user.ID).Scan(&tokenHash); err != nil {
		return err
	}
	if currentHashes[tokenHash] {
		return errCurrentAccountSession
	}
	if refreshID, ok := strings.CutPrefix(sessionID, "nativesess_"); ok {
		var familyID string
		if err := s.queryUserRow(ctx, `SELECT family_id FROM native_refresh_tokens WHERE id = ? AND user_id = ?`, refreshID, user.ID).Scan(&familyID); err != nil {
			return err
		}
		return s.revokeNativeCredentialFamily(ctx, familyID, time.Now().UTC())
	}
	result, err := s.execSecurityFenceWrite(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, sessionID, user.ID)
	if err != nil {
		return err
	}
	if rowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) currentSessionTokenHashes(r *http.Request) map[string]bool {
	hashes := map[string]bool{}
	if r == nil {
		return hashes
	}
	for _, cookie := range s.requestSessionCookies(r) {
		if cookie.Value == "" {
			continue
		}
		hashes[hashToken(cookie.Value)] = true
	}
	if header := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(header, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token != "" && !strings.HasPrefix(token, "ptc_clt_") && !strings.HasPrefix(token, "ptc_api_") {
			hashes[hashToken(token)] = true
		}
	}
	return hashes
}
