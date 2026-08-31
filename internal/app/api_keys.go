package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

const apiKeyRecentAuthenticationWindow = 10 * time.Minute

func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "owner_required", "Only the server owner can manage API keys.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := s.listAPIKeysContext(r.Context())
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "api_keys_failed", "Unable to list API keys.")
			return
		}
		writeJSON(w, http.StatusOK, ListResponse[APIKey]{Items: keys, Total: len(keys)})
	case http.MethodPost:
		if !s.requireRecentAPIKeyOwnerAuthentication(w, r, user) {
			return
		}
		var req APIKeyCreateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		key, token, err := s.createAPIKeyContext(r.Context(), user.ID, req)
		if err != nil {
			writeDatabaseAccessError(w, err, http.StatusBadRequest, "api_key_create_failed", err.Error())
			return
		}
		s.recordAudit(r, user, "api_key.created", "api_key", key.ID, "warn", map[string]string{"name": key.Name, "scopes": strings.Join(key.Scopes, ",")})
		writeJSON(w, http.StatusCreated, APIKeyCreateResponse{Key: key, Token: token})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET or POST for this endpoint.")
	}
}

func (s *Server) handleAPIKeyRoute(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "owner_required", "Only the server owner can manage API keys.")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use DELETE for this endpoint.")
		return
	}
	keyID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/auth/api-keys/"))
	if keyID == "" || strings.Contains(keyID, "/") {
		writeError(w, http.StatusNotFound, "not_found", "API key route was not found.")
		return
	}
	if !s.requireRecentAPIKeyOwnerAuthentication(w, r, user) {
		return
	}
	key, err := s.revokeAPIKeyContext(r.Context(), keyID)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusNotFound, "api_key_not_found", "API key was not found.")
		return
	}
	s.recordAudit(r, user, "api_key.revoked", "api_key", key.ID, "warn", map[string]string{"name": key.Name})
	writeJSON(w, http.StatusOK, key)
}

// API keys are long-lived bearer credentials. Interactive ownership alone is
// insufficient to mint or revoke them: the current app session must have been
// established recently. Local and Hosted owners use the same boundary, and a
// stale browser can recover by signing in again through its authoritative
// provider rather than sending an account password to this endpoint.
func (s *Server) requireRecentAPIKeyOwnerAuthentication(w http.ResponseWriter, r *http.Request, user User) bool {
	sessionID, err := s.currentSessionIDContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusUnauthorized, "recent_reauthentication_required", "Sign in again before changing API keys.")
		return false
	}
	var createdAt string
	if err := s.queryUserRow(r.Context(), `SELECT created_at FROM sessions WHERE id = ? AND user_id = ?`, sessionID, accountIDForUser(user)).Scan(&createdAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("API key recent authentication lookup failed", "error", err)
		}
		writeProductError(w, http.StatusUnauthorized, "recent_reauthentication_required", "Sign in again before changing API keys.")
		return false
	}
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		created, err = time.Parse(time.RFC3339Nano, createdAt)
	}
	if err != nil || time.Since(created) < 0 || time.Since(created) > apiKeyRecentAuthenticationWindow {
		writeProductError(w, http.StatusUnauthorized, "recent_reauthentication_required", "Sign in again before changing API keys.")
		return false
	}
	return true
}

func (s *Server) listAPIKeys() ([]APIKey, error) {
	return s.listAPIKeysContext(context.Background())
}

func (s *Server) listAPIKeysContext(ctx context.Context) ([]APIKey, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT k.id, k.name, k.user_id, COALESCE(u.username, ''), k.last_four, k.scopes_json, k.created_at, k.last_used_at, k.revoked_at
		FROM api_keys k
		LEFT JOIN users u ON u.id = k.user_id
		WHERE k.revoked_at = ''
		ORDER BY k.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []APIKey{}
	for rows.Next() {
		var key APIKey
		var scopesJSON string
		if err := rows.Scan(&key.ID, &key.Name, &key.UserID, &key.Username, &key.LastFour, &scopesJSON, &key.CreatedAt, &key.LastUsedAt, &key.RevokedAt); err != nil {
			return nil, err
		}
		key.Scopes = decodeAPIKeyScopes(scopesJSON)
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Server) createAPIKey(userID string, req APIKeyCreateRequest) (APIKey, string, error) {
	return s.createAPIKeyContext(context.Background(), userID, req)
}

func (s *Server) createAPIKeyContext(ctx context.Context, userID string, req APIKeyCreateRequest) (APIKey, string, error) {
	var role string
	if err := s.queryUserRow(ctx, `SELECT role FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, strings.TrimSpace(userID)).Scan(&role); err != nil || !strings.EqualFold(strings.TrimSpace(role), "owner") {
		return APIKey{}, "", errInvalidAPIKey("API keys can be issued only for the server owner.")
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 2 || len(name) > 80 {
		return APIKey{}, "", errInvalidAPIKey("API key name must be 2 to 80 characters.")
	}
	for _, raw := range req.Scopes {
		scope := strings.TrimSpace(raw)
		if !apiKeySafeScopes[scope] {
			return APIKey{}, "", errInvalidAPIKey("API key scope " + scope + " is not supported.")
		}
	}
	scopes := normalizeAPIKeyScopes(req.Scopes)
	token := "ptc_api_" + randomToken()
	now := time.Now().UTC().Format(time.RFC3339)
	key := APIKey{
		ID:        randomID("apikey"),
		Name:      name,
		UserID:    userID,
		LastFour:  token[len(token)-4:],
		Scopes:    scopes,
		CreatedAt: now,
	}
	scopesJSON, _ := json.Marshal(scopes)
	if _, err := s.execUserWrite(ctx, `
		INSERT INTO api_keys (id, user_id, name, token_hash, last_four, scopes_json, created_at, last_used_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', '')`,
		key.ID, userID, name, hashToken(token), key.LastFour, string(scopesJSON), now); err != nil {
		return APIKey{}, "", err
	}
	if user, err := s.getUser(userID); err == nil {
		key.Username = user.Username
	}
	return key, token, nil
}

func (s *Server) revokeAPIKey(keyID string) (APIKey, error) {
	return s.revokeAPIKeyContext(context.Background(), keyID)
}

func (s *Server) revokeAPIKeyContext(ctx context.Context, keyID string) (APIKey, error) {
	keys, err := s.listAPIKeysContext(ctx)
	if err != nil {
		return APIKey{}, err
	}
	var key APIKey
	found := false
	for _, candidate := range keys {
		if candidate.ID == keyID {
			key = candidate
			found = true
			break
		}
	}
	if !found {
		return APIKey{}, errInvalidAPIKey("API key was not found.")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.execSecurityFenceWrite(ctx, `UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at = ''`, now, keyID)
	if err != nil {
		return APIKey{}, err
	}
	if rowsAffected(result) == 0 {
		return APIKey{}, errInvalidAPIKey("API key was not found.")
	}
	key.RevokedAt = now
	return key, nil
}

func (s *Server) userForAPIKey(r *http.Request, token string) (User, bool) {
	hash := hashToken(token)
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	row := s.queryUserRow(ctx, `
		SELECT k.id, k.user_id, k.scopes_json
		FROM api_keys k
		JOIN users u ON u.id = k.user_id AND COALESCE(u.disabled_at, '') = ''
		WHERE k.token_hash = ? AND k.revoked_at = ''`, hash)
	var keyID string
	var userID string
	var scopesJSON string
	if err := row.Scan(&keyID, &userID, &scopesJSON); err != nil {
		return User{}, false
	}
	user, err := s.getUser(userID)
	if err != nil {
		return User{}, false
	}
	scopes := decodeAPIKeyScopes(scopesJSON)
	user.AuthProvider = "api_key"
	user.APIKeyID = keyID
	user.APIKeyScopes = scopes
	user.Permissions = applyAPIKeyScopes(user.Permissions, scopes)
	s.observeAPIKeyUse(keyID)
	_ = r
	return user, true
}

const apiKeyUsageFlushInterval = 30 * time.Second

// API-key usage is observational telemetry, not an authorization input. Keep
// revocation and scope checks on the read path, but coalesce last-used writes
// so a read-heavy client cannot turn every request into a SQLite write.
func (s *Server) observeAPIKeyUse(keyID string) {
	if s == nil || strings.TrimSpace(keyID) == "" {
		return
	}
	s.apiKeyUsageMu.Lock()
	if s.apiKeyUsagePending == nil {
		s.apiKeyUsagePending = map[string]time.Time{}
	}
	if len(s.apiKeyUsagePending) < 4096 || s.apiKeyUsagePending[keyID].IsZero() {
		s.apiKeyUsagePending[keyID] = time.Now().UTC()
	}
	s.apiKeyUsageMu.Unlock()
}

func (s *Server) runAPIKeyUsageWriter(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(apiKeyUsageFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flushAPIKeyUsage(ctx)
		}
	}
}

func (s *Server) flushAPIKeyUsage(ctx context.Context) {
	if s == nil || s.dbHandle() == nil {
		return
	}
	s.apiKeyUsageMu.Lock()
	pending := s.apiKeyUsagePending
	s.apiKeyUsagePending = map[string]time.Time{}
	s.apiKeyUsageMu.Unlock()
	if len(pending) == 0 {
		return
	}
	ids := make([]string, 0, len(pending))
	args := make([]any, 0, len(pending)*2+len(pending))
	caseParts := make([]string, 0, len(pending))
	for id, at := range pending {
		ids = append(ids, id)
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, id, at.Format(time.RFC3339Nano))
	}
	sort.Strings(ids)
	// Rebuild the CASE arguments in deterministic ID order. Determinism keeps
	// query traces and regression tests stable without making ordering an auth
	// decision.
	args = args[:0]
	caseParts = caseParts[:0]
	for _, id := range ids {
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, id, pending[id].Format(time.RFC3339Nano))
	}
	placeholders := make([]string, len(ids))
	for index, id := range ids {
		placeholders[index] = "?"
		args = append(args, id)
	}
	query := `UPDATE api_keys
		SET last_used_at = MAX(last_used_at, CASE id ` + strings.Join(caseParts, " ") + ` ELSE last_used_at END)
		WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	if _, err := s.execBackgroundWrite(ctx, query, args...); err != nil {
		s.apiKeyUsageMu.Lock()
		if s.apiKeyUsagePending == nil {
			s.apiKeyUsagePending = map[string]time.Time{}
		}
		for id, at := range pending {
			if existing := s.apiKeyUsagePending[id]; existing.Before(at) {
				s.apiKeyUsagePending[id] = at
			}
		}
		s.apiKeyUsageMu.Unlock()
		s.log.Warn("API key usage observation flush failed", "error", err)
	}
}

func (s *Server) allowAPIKeyRequest(user User) (bool, int) {
	if user.AuthProvider != "api_key" || user.APIKeyID == "" {
		return true, 0
	}
	const maxRequests = 600
	window := time.Minute
	now := time.Now().UTC()
	cutoff := now.Add(-window)
	s.apiKeyLimiterMu.Lock()
	defer s.apiKeyLimiterMu.Unlock()
	if s.apiKeyAttempts == nil {
		s.apiKeyAttempts = map[string][]time.Time{}
	}
	attempts := s.apiKeyAttempts[user.APIKeyID]
	kept := attempts[:0]
	for _, attempt := range attempts {
		if attempt.After(cutoff) {
			kept = append(kept, attempt)
		}
	}
	if len(kept) >= maxRequests {
		retryAfter := int(time.Until(kept[0].Add(window)).Seconds()) + 1
		if retryAfter < 1 {
			retryAfter = 1
		}
		s.apiKeyAttempts[user.APIKeyID] = kept
		return false, retryAfter
	}
	kept = append(kept, now)
	s.apiKeyAttempts[user.APIKeyID] = kept
	return true, 0
}

func apiKeyAllowsRequest(user User, r *http.Request) bool {
	if user.AuthProvider != "api_key" {
		return true
	}
	scopes := map[string]bool{}
	for _, scope := range user.APIKeyScopes {
		scopes[scope] = true
	}
	path := r.URL.Path
	if apiKeyOwnerOnlyPath(path) {
		return false
	}
	switch {
	case apiPathMatches(path, "/api/watch-with-friends"):
		return scopes["watchWithFriends"]
	case apiPathMatches(path, "/api/playback-sessions"), apiPathMatches(path, "/api/playback"):
		return scopes["playMedia"]
	case apiPathMatches(path, "/api/live-tv/play") ||
		strings.Contains(path, "/streams/") ||
		strings.Contains(path, "/hls/"):
		return scopes["playLiveTV"]
	case apiPathMatches(path, "/api/admin/library-channels"):
		return false
	case apiPathMatches(path, "/api/library-channels"):
		return scopes["viewLiveTV"] || scopes["playLiveTV"]
	case apiPathMatches(path, "/api/live-tv/sources"):
		if r.Method == http.MethodGet && (strings.Contains(path, "/guide") || strings.Contains(path, "/channels")) {
			return scopes["viewLiveTV"] || scopes["playLiveTV"]
		}
		return false
	case apiPathMatches(path, "/api/live-tv/channels"):
		return scopes["viewLiveTV"] || scopes["playLiveTV"]
	case apiPathMatches(path, "/api/client-logs"):
		return scopes["read"]
	case apiPathMatches(path, "/api/playlists"):
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return scopes["read"]
		}
		return scopes["playMedia"] || scopes["editMetadata"]
	case apiPathMatches(path, "/api/media"):
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return scopes["read"]
		}
		return apiKeyAllowsMediaMutation(path, scopes)
	case apiPathMatches(path, "/api/libraries") || apiPathMatches(path, "/api/filesystem"):
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return scopes["read"]
		}
		return false
	case apiPathMatches(path, "/api/activity") ||
		apiPathMatches(path, "/api/backups") ||
		apiPathMatches(path, "/api/tasks") ||
		apiPathMatches(path, "/api/system/storage") ||
		apiPathMatches(path, "/api/logs"):
		return false
	case apiPathMatches(path, "/api/live-tv"):
		return scopes["viewLiveTV"] || scopes["playLiveTV"]
	case apiPathMatches(path, "/api/admin/dvr"):
		return false
	case apiPathMatches(path, "/api/dvr"):
		if strings.Contains(path, "/stream") || strings.Contains(path, "/hls/") {
			return scopes["playMedia"] && (scopes["viewDVR"] || scopes["manageDVR"])
		}
		if r.Method == http.MethodDelete {
			return scopes["deleteDVRRecordings"] || scopes["manageDVR"]
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			return scopes["scheduleDVR"] || scopes["manageDVR"]
		}
		return scopes["viewDVR"] || scopes["manageDVR"]
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return scopes["read"]
	}
	return false
}

func apiKeyOwnerOnlyPath(path string) bool {
	if path == "/api/account" {
		return true
	}
	for _, prefix := range []string{
		"/api/users", "/api/settings", "/api/auth/api-keys", "/api/devices",
		"/api/remote-access", "/api/account/", "/api/backups", "/api/system",
		"/api/release", "/api/storage", "/api/filesystem", "/api/dashboard",
		"/api/activity", "/api/tasks", "/api/jobs", "/api/logs", "/api/audit",
		"/api/diagnostics", "/api/dlna", "/api/identity-reconciliation", "/api/admin/",
	} {
		if apiPathMatches(path, strings.TrimSuffix(prefix, "/")) {
			return true
		}
	}
	return false
}

func apiPathMatches(path, root string) bool {
	path = strings.TrimSuffix(strings.TrimSpace(path), "/")
	root = strings.TrimSuffix(strings.TrimSpace(root), "/")
	return root != "" && (path == root || strings.HasPrefix(path, root+"/"))
}

func apiKeyAllowsMediaMutation(path string, scopes map[string]bool) bool {
	switch {
	case strings.Contains(path, "/images"):
		return scopes["editMetadata"]
	case strings.Contains(path, "/lyrics"):
		return scopes["editMetadata"] || scopes["manageLyrics"]
	case strings.Contains(path, "/subtitles"):
		return scopes["editMetadata"] || scopes["manageSubtitles"]
	case strings.Contains(path, "/segments"):
		return scopes["editMetadata"]
	case strings.Contains(path, "/download"):
		return scopes["downloadMedia"]
	}
	return scopes["editMetadata"] || scopes["deleteMedia"]
}

var apiKeySafeScopes = map[string]bool{
	"read": true, "playMedia": true, "downloadMedia": true, "editMetadata": true,
	"manageLyrics": true, "manageSubtitles": true, "watchWithFriends": true,
	"viewLiveTV": true, "playLiveTV": true, "viewDVR": true, "scheduleDVR": true,
	"manageDVR": true, "deleteDVRRecordings": true, "deleteMedia": true, "transcode": true,
}

func normalizeAPIKeyScopes(input []string) []string {
	seen := map[string]bool{}
	scopes := []string{"read"}
	seen["read"] = true
	for _, raw := range input {
		scope := strings.TrimSpace(raw)
		if scope == "all" {
			for _, safeScope := range []string{"playMedia", "downloadMedia", "editMetadata", "manageLyrics", "manageSubtitles", "watchWithFriends", "viewLiveTV", "playLiveTV", "viewDVR", "scheduleDVR", "manageDVR", "deleteDVRRecordings", "deleteMedia", "transcode"} {
				if !seen[safeScope] {
					scopes = append(scopes, safeScope)
					seen[safeScope] = true
				}
			}
			continue
		}
		if !apiKeySafeScopes[scope] || seen[scope] {
			continue
		}
		scopes = append(scopes, scope)
		seen[scope] = true
	}
	return scopes
}

func decodeAPIKeyScopes(raw string) []string {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return []string{"read"}
	}
	return normalizeAPIKeyScopes(scopes)
}

func applyAPIKeyScopes(permissions map[string]bool, scopes []string) map[string]bool {
	scopeMap := map[string]bool{}
	for _, scope := range scopes {
		scopeMap[scope] = true
	}
	filtered := map[string]bool{}
	for key, value := range permissions {
		filtered[key] = value && scopeMap[key]
	}
	return filtered
}

type errInvalidAPIKey string

func (e errInvalidAPIKey) Error() string {
	return string(e)
}
