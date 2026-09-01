package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app/apiroute"
)

const apiKeyRecentAuthenticationWindow = 10 * time.Minute
const maxActiveAPIKeysPerOwner = 100

func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request, user User) {
	setAPIKeyManagementNoStore(w)
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
			var conflict errAPIKeyConflict
			if errors.As(err, &conflict) {
				writeError(w, http.StatusConflict, conflict.Code, conflict.Error())
				return
			}
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
	setAPIKeyManagementNoStore(w)
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

func setAPIKeyManagementNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
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
	err := s.withSecurityFenceTxTagged(ctx, []string{"api_keys"}, func(tx *sql.Tx) error {
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM api_keys
			WHERE user_id = ? AND lower(name) = lower(?) AND revoked_at = ''
			ORDER BY created_at DESC, id ASC LIMIT 1`, userID, name).Scan(&existingID)
		if err == nil {
			// An ambiguous create retry cannot safely replay a secret that the
			// server never persisted. Fail deterministically instead of minting a
			// second credential; the owner can reconcile from the active list,
			// revoke the existing key, and retry intentionally.
			return errAPIKeyConflict{Code: "api_key_name_conflict", Message: "An active API key already uses this name. Revoke it before creating a replacement."}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var activeCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE user_id = ? AND revoked_at = ''`, userID).Scan(&activeCount); err != nil {
			return err
		}
		if activeCount >= maxActiveAPIKeysPerOwner {
			return errAPIKeyConflict{Code: "api_key_limit_reached", Message: "Revoke an active API key before creating another."}
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO api_keys (id, user_id, name, token_hash, last_four, scopes_json, created_at, last_used_at, revoked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '', '')`,
			key.ID, userID, name, hashToken(token), key.LastFour, string(scopesJSON), now)
		return err
	})
	if err != nil {
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
	err = s.withSecurityFenceTxTagged(ctx, []string{"api_keys", "playback", "downloads", "playback-receivers", "authorization"}, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at = ''`, now, keyID)
		if err != nil {
			return err
		}
		if rowsAffected(result) == 0 {
			return errInvalidAPIKey("API key was not found.")
		}
		// Receiver controller grants are independently presented capabilities, so
		// mark them terminal in the same commit as their originating key.
		_, err = tx.ExecContext(ctx, `UPDATE playback_receiver_authorizations SET revoked_at = ? WHERE api_key_id = ? AND revoked_at = ''`, now, keyID)
		return err
	})
	if err != nil {
		return APIKey{}, err
	}
	s.forgetMediaGrantsForAPIKey(keyID)
	s.apiKeyUsageMu.Lock()
	delete(s.apiKeyUsagePending, keyID)
	s.apiKeyUsageMu.Unlock()
	s.apiKeyLimiterMu.Lock()
	delete(s.apiKeyAttempts, keyID)
	s.apiKeyLimiterMu.Unlock()
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
	// API keys do not carry an interactive profile selection. Bind them to the
	// account's active primary profile so browse/search/content policy uses the
	// same canonical principal as an owner session instead of a partially
	// enriched account row with zero-value profile restrictions.
	var profileID string
	if err := s.queryUserRow(ctx, `
		SELECT id
		FROM profiles
		WHERE account_id = ? AND is_primary = 1 AND disabled_at = ''
		ORDER BY sort_order ASC, id ASC
		LIMIT 1`, userID).Scan(&profileID); err != nil {
		return User{}, false
	}
	principal, err := s.resolveRequestPrincipalContext(ctx, userID, profileID)
	if err != nil {
		return User{}, false
	}
	applyRequestPrincipal(&user, principal)
	scopes := decodeAPIKeyScopes(scopesJSON)
	user.AuthProvider = "api_key"
	user.APIKeyID = keyID
	user.APIKeyScopes = scopes
	user.Permissions = applyAPIKeyScopes(user.Permissions, scopes)
	_ = r
	return user, true
}

// applyActiveAPIKeyIdentityContext restores the credential boundary on a user
// reconstructed from an API-key-derived capability. It is intentionally strict:
// a missing, revoked, disabled-account, or malformed key record invalidates the
// derived capability instead of silently widening it to account authority.
func (s *Server) applyActiveAPIKeyIdentityContext(ctx context.Context, user *User, keyID string) bool {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return true
	}
	if user == nil {
		return false
	}
	var scopesJSON string
	if err := s.queryUserRow(ctx, `
		SELECT k.scopes_json
		FROM api_keys k
		JOIN users u ON u.id = k.user_id AND COALESCE(u.disabled_at, '') = ''
		WHERE k.id = ? AND k.user_id = ? AND k.revoked_at = ''`, keyID, accountIDForUser(*user)).Scan(&scopesJSON); err != nil {
		return false
	}
	scopes := decodeAPIKeyScopes(scopesJSON)
	user.AuthProvider = "api_key"
	user.APIKeyID = keyID
	user.APIKeyScopes = scopes
	user.Permissions = applyAPIKeyScopes(user.Permissions, scopes)
	return true
}

func (s *Server) applyPlaybackSessionAPIKeyContext(ctx context.Context, user *User, sessionID string) bool {
	if user == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	var accountID, profileID, keyID string
	if err := s.queryUserRow(ctx, `SELECT user_id, profile_id, COALESCE(api_key_id, '') FROM playback_sessions WHERE id = ?`, strings.TrimSpace(sessionID)).Scan(&accountID, &profileID, &keyID); err != nil {
		return false
	}
	if accountID != accountIDForUser(*user) || profileID != viewerProfileID(*user) {
		return false
	}
	return s.applyActiveAPIKeyIdentityContext(ctx, user, keyID)
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

func (s *Server) observeAuthorizedAPIKeyUse(user User) {
	if user.AuthProvider == "api_key" && user.APIKeyID != "" {
		s.observeAPIKeyUse(user.APIKeyID)
	}
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
		WHERE revoked_at = '' AND id IN (` + strings.Join(placeholders, ",") + `)`
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

func (s *Server) enforceAPIKeyRequestLimit(w http.ResponseWriter, user User) bool {
	allowed, retryAfter := s.allowAPIKeyRequest(user)
	if allowed {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	writeError(w, http.StatusTooManyRequests, "api_key_rate_limited", "This API key has reached its request limit. Try again shortly.")
	return false
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
	route, routeKnown := apiroute.RouteFromRequest(r)
	if !routeKnown {
		// API keys are integration credentials: an unregistered operation must
		// fail closed instead of inheriting a method/path heuristic.
		return false
	}
	operation := route.OperationID
	switch {
	case apiKeyRouteDeclaresScope(route, scopes):
		return true
	case operation == "postClientLogs":
		return false
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
	case apiKeyDownloadPreparationOperation(operation):
		return scopes["downloadMedia"]
	case apiPathMatches(path, "/api/playlists"):
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return scopes["read"]
		}
		return scopes["playMedia"] || scopes["editMetadata"]
	case apiPathMatches(path, "/api/media"):
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			return apiKeyAllowsMediaReadOperation(operation, scopes)
		}
		return apiKeyAllowsMediaMutation(operation, scopes)
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
		return apiKeyAllowsDVROperation(operation, scopes)
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return scopes["read"]
	}
	return false
}

func apiKeyRouteDeclaresScope(route apiroute.Route, granted map[string]bool) bool {
	if len(route.APIKeyScopes) == 0 {
		return false
	}
	for _, scope := range route.APIKeyScopes {
		if !granted[scope] {
			return false
		}
	}
	return true
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

func apiKeyDownloadPreparationOperation(operation string) bool {
	switch operation {
	case "listDownloadPreparations", "createDownloadPreparation", "getDownloadPreparation",
		"updateDownloadPreparation", "removeDownloadPreparation", "createDownloadPreparationGrant":
		return true
	default:
		return false
	}
}

func apiKeyAllowsDVROperation(operation string, scopes map[string]bool) bool {
	view := scopes["viewDVR"] || scopes["manageDVR"]
	switch operation {
	case "postDvrRecordingsIdPlayback", "getDvrRecordingsIdStream", "headDvrRecordingsIdStream",
		"getDvrRecordingsIdHlsResource", "headDvrRecordingsIdHlsResource":
		return view && scopes["playMedia"]
	case "deleteDvrRecordingsId":
		return scopes["deleteDVRRecordings"] || scopes["manageDVR"]
	case "postDvrRecordings", "patchDvrRecordingsId", "postDvrRules", "patchDvrRulesId", "deleteDvrRulesId":
		return scopes["scheduleDVR"] || scopes["manageDVR"]
	case "getDvrRecordingGroups", "getDvrRecordings", "getDvrRecordingsId", "getDvrRules", "getDvrRulesId", "getDvrSchedule", "getDVRStatus":
		return view
	default:
		return false
	}
}

func apiKeyAllowsMediaReadOperation(operation string, scopes map[string]bool) bool {
	switch operation {
	case "getMediaIdDownload", "headMediaIdDownload", "getMediaIdDownloadOptions":
		return scopes["downloadMedia"]
	case "getMediaIdStream", "headMediaIdStream", "getMediaIdHlsResource", "headMediaIdHlsResource",
		"getMediaIdOptimized", "getMediaIdOptimizedProfile", "headMediaIdOptimizedProfile",
		"getMediaIdTrickplay", "getMediaIdTrickplaySetIdTilesM3u8", "getMediaIdTrickplaySetIdTilesTileIndexJpg":
		return scopes["playMedia"]
	case "getMediaIdImagesImageId", "getMediaIdLyricsSearch", "getMediaIdMatchCandidates":
		return scopes["editMetadata"]
	case "getMediaIdSubtitlesStreamId", "headMediaIdSubtitlesStreamId":
		return scopes["manageSubtitles"] || scopes["editMetadata"]
	default:
		return scopes["read"]
	}
}

func apiKeyAllowsMediaMutation(operation string, scopes map[string]bool) bool {
	switch operation {
	case "postMediaIdFavorite", "postMediaIdRating", "postMediaIdReaction", "postMediaIdWatched", "postMediaIdWatchlist", "postMediaBulkState":
		return scopes["playMedia"]
	case "deleteMediaId":
		return scopes["deleteMedia"]
	case "postMediaIdLyrics", "postMediaIdLyricsApply", "postMediaIdLyricsFetch", "deleteMediaIdLyricsLyricId":
		return scopes["manageLyrics"] || scopes["editMetadata"]
	case "postMediaIdSubtitles", "deleteMediaIdSubtitlesStreamId", "patchMediaIdSubtitlesStreamId":
		return scopes["manageSubtitles"] || scopes["editMetadata"]
	case "postMediaIdImages", "postMediaIdImagesOrder", "deleteMediaIdImagesImageId", "postMediaIdImagesImageIdPreferred",
		"patchMediaId", "postMediaBulkMetadata", "postMediaIdMatch", "postMediaIdSegments", "deleteMediaIdSegmentsSegmentId":
		return scopes["editMetadata"]
	case "postMediaIdOptimized", "deleteMediaIdOptimizedProfile":
		return scopes["transcode"]
	case "postMediaBulkJobs", "postMediaIdJobs":
		// Job bodies multiplex administrative mutations; without a dedicated
		// operation-level scope contract an API key must not receive them.
		return false
	default:
		return false
	}
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

type errAPIKeyConflict struct {
	Code    string
	Message string
}

func (e errAPIKeyConflict) Error() string { return e.Message }
