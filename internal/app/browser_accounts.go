package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	browserVaultCookieName = "portico_browser_vault"
	browserVaultTTL        = 180 * 24 * time.Hour
)

var (
	errBrowserVaultNotFound       = errors.New("browser account vault not found")
	errBrowserAccountNotFound     = errors.New("browser account not found")
	errBrowserAccountExpired      = errors.New("browser account expired")
	errBrowserAccountDisabled     = errors.New("browser account disabled")
	errBrowserAccessSchedule      = errors.New("browser account access schedule denied")
	errBrowserDeviceNotAllowed    = errors.New("browser account device not allowed")
	errBrowserMembershipInactive  = errors.New("browser account membership inactive")
	errBrowserAuthModeUnavailable = errors.New("browser account auth mode unavailable")
)

// BrowserAccountSummary is deliberately narrower than User. The browser vault
// is an authentication credential, but its list endpoint never discloses
// permissions, password state, libraries, e-mail addresses, or session data.
type BrowserAccountSummary struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	ProfileImageURL string `json:"profileImageUrl,omitempty"`
	AuthOrigin      string `json:"authOrigin"`
	AuthProvider    string `json:"authProvider"`
	LastUsedAt      string `json:"lastUsedAt"`
}

type BrowserAccountsResponse struct {
	Accounts          []BrowserAccountSummary `json:"accounts"`
	ActiveAccountID   string                  `json:"activeAccountId,omitempty"`
	AutomaticSignIn   bool                    `json:"automaticSignIn"`
	SelectionRequired bool                    `json:"selectionRequired"`
	CanAddAccount     bool                    `json:"canAddAccount"`
}

type BrowserAccountSwitchRequest struct {
	AccountID string `json:"accountId"`
	ProfileDeviceDescriptor
}

type BrowserAccountPreferencesRequest struct {
	AutomaticSignIn *bool `json:"automaticSignIn"`
}

type BrowserAccountRemoveRequest struct {
	AccountID string `json:"accountId"`
}

type BrowserAccountMutationResponse struct {
	OK                   bool `json:"ok"`
	VaultRevoked         bool `json:"vaultRevoked,omitempty"`
	ActiveAccountRemoved bool `json:"activeAccountRemoved,omitempty"`
}

type browserAccountVault struct {
	ID              string
	TokenHash       string
	DeviceID        string
	ActiveUserID    string
	AutomaticSignIn bool
	ExpiresAt       time.Time
}

type browserAccountSwitchResult struct {
	UserID         string
	Provider       string
	SessionToken   string
	SessionExpires time.Time
	VaultToken     string
	VaultExpires   time.Time
	ProfileAuth    *ProfileAccountAuthenticationResponse
	BrowserBinding string
	User           *User
}

func rememberBrowserAccount(value *bool) bool {
	return value == nil || *value
}

func (s *Server) discardBrowserSession(w http.ResponseWriter, ctx context.Context, sessionToken string) {
	if strings.TrimSpace(sessionToken) != "" {
		_, _ = s.execSecurityFenceWrite(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(sessionToken))
	}
	s.clearSessionCookies(ctx, w)
}

func (s *Server) handleBrowserAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	response, err := s.browserAccountsResponse(r.Context(), r)
	if errors.Is(err, errBrowserVaultNotFound) {
		s.clearBrowserVaultCookies(r.Context(), w)
		writeJSON(w, http.StatusOK, s.emptyBrowserAccountsResponse(r.Context()))
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "browser_accounts_failed", "Unable to load accounts remembered on this browser.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleBrowserAccountRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auth/browser-accounts/"), "/")
	switch path {
	case "switch":
		s.handleBrowserAccountSwitch(w, r)
	case "preferences":
		s.handleBrowserAccountPreferences(w, r)
	case "remove":
		s.handleBrowserAccountRemove(w, r)
	case "sign-out-all":
		s.handleBrowserAccountSignOutAll(w, r)
	default:
		parts := strings.Split(path, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] == "image" {
			s.handleBrowserAccountImage(w, r, parts[0])
			return
		}
		writeError(w, http.StatusNotFound, "browser_account_route_not_found", "Browser account route was not found.")
	}
}

func (s *Server) handleBrowserAccountSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req BrowserAccountSwitchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.AccountID = strings.TrimSpace(req.AccountID)
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "browser_account_required", "Choose an account to continue.")
		return
	}
	installationID := normalizeUntrustedNativeInstallationID(req.InstallationID)
	if !validProfileDeviceDescriptor(req.ProfileDeviceDescriptor) {
		writeProductError(w, http.StatusBadRequest, "device_identity_required", "A valid device name, app, and platform are required.")
		return
	}
	if installationID == "" {
		var generatedErr error
		installationID, generatedErr = nativeSecureRandomID(s.nativeCredentialEntropyReader(), "install")
		if generatedErr != nil {
			writeProductError(w, http.StatusInternalServerError, "session_failed", "Unable to create a server device record.")
			return
		}
	}
	req.InstallationID = installationID
	result, err := s.switchBrowserAccount(r.Context(), r, req.AccountID, req.ProfileDeviceDescriptor)
	if err != nil {
		writeBrowserAccountSwitchError(w, err)
		return
	}

	// Clearing every server-scoped session name prevents a stale fallback
	// cookie from authenticating the prior user if the preferred cookie changes.
	s.clearSessionCookies(r.Context(), w)
	if result.SessionToken != "" {
		s.setSessionCookie(r.Context(), w, s.sessionCookieNameContext(r.Context()), result.SessionToken, result.SessionExpires)
	}
	s.clearBrowserVaultCookies(r.Context(), w)
	s.setBrowserVaultCookie(r.Context(), w, s.browserVaultCookieNameContext(r.Context()), result.VaultToken, result.VaultExpires)
	if result.ProfileAuth != nil {
		s.setProfileBrowserBindingCookie(r.Context(), w, result.BrowserBinding, time.Now().UTC().Add(profileAccountAuthenticationTTL))
		writeJSON(w, http.StatusOK, result.ProfileAuth)
		return
	}

	if result.User == nil {
		writeDatabaseAccessError(w, sql.ErrNoRows, http.StatusInternalServerError, "browser_account_switch_failed", "The account could not be loaded before switching.")
		return
	}
	s.recordLog("info", "Browser account switched", map[string]string{"user": result.User.Email, "provider": result.Provider})
	writeJSON(w, http.StatusOK, s.authResponseWithServerContext(r.Context(), true, false, result.User))
}

func (s *Server) handleBrowserAccountPreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
		return
	}
	var req BrowserAccountPreferencesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AutomaticSignIn == nil {
		writeError(w, http.StatusBadRequest, "automatic_sign_in_required", "Automatic sign in must be true or false.")
		return
	}
	vault, _, err := s.browserVaultForRequest(r.Context(), r)
	if err != nil {
		s.clearBrowserVaultCookies(r.Context(), w)
		writeError(w, http.StatusUnauthorized, "browser_vault_required", "A remembered browser account is required.")
		return
	}
	activeUserID, ok := s.activeSessionUserForVault(r.Context(), r, vault.ID)
	if !ok || activeUserID == "" || activeUserID != vault.ActiveUserID {
		writeError(w, http.StatusUnauthorized, "active_browser_account_required", "Sign in with an account remembered on this browser before changing this preference.")
		return
	}
	value := 0
	if *req.AutomaticSignIn {
		value = 1
	}
	if _, err := s.execUserWrite(r.Context(), `UPDATE browser_account_vaults SET automatic_sign_in = ?, last_seen_at = ? WHERE id = ? AND revoked_at = ''`, value, time.Now().UTC().Format(time.RFC3339), vault.ID); err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "browser_account_preferences_failed", "Unable to update automatic sign in.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"automaticSignIn": *req.AutomaticSignIn})
}

func (s *Server) handleBrowserAccountRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req BrowserAccountRemoveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.AccountID = strings.TrimSpace(req.AccountID)
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "browser_account_required", "Choose an account to remove.")
		return
	}
	result, vaultToken, vaultExpires, err := s.removeBrowserAccount(r.Context(), r, req.AccountID)
	if err != nil {
		if errors.Is(err, errBrowserVaultNotFound) {
			s.clearBrowserVaultCookies(r.Context(), w)
			writeError(w, http.StatusUnauthorized, "browser_vault_required", "A remembered browser account is required.")
			return
		}
		if errors.Is(err, errBrowserAccountNotFound) {
			writeError(w, http.StatusNotFound, "browser_account_not_found", "That account is not remembered on this browser.")
			return
		}
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "browser_account_remove_failed", "Unable to remove the account from this browser.")
		return
	}
	if result.ActiveAccountRemoved {
		s.clearSessionCookies(r.Context(), w)
	}
	if result.VaultRevoked {
		s.clearBrowserVaultCookies(r.Context(), w)
	} else {
		s.clearBrowserVaultCookies(r.Context(), w)
		s.setBrowserVaultCookie(r.Context(), w, s.browserVaultCookieNameContext(r.Context()), vaultToken, vaultExpires)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleBrowserAccountSignOutAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	vault, _, err := s.browserVaultForRequest(r.Context(), r)
	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		err = s.withSecurityFenceTxTagged(r.Context(), []string{"sessions", "browser_accounts"}, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(r.Context(), `UPDATE browser_account_entries SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE vault_id = ?`, now, vault.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE browser_vault_id = ?`, vault.ID); err != nil {
				return err
			}
			_, err := tx.ExecContext(r.Context(), `UPDATE browser_account_vaults SET active_user_id = NULL, revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE id = ?`, now, vault.ID)
			return err
		})
	}
	if err != nil && !errors.Is(err, errBrowserVaultNotFound) {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "browser_account_sign_out_failed", "Unable to sign out accounts on this browser.")
		return
	}
	s.clearSessionCookies(r.Context(), w)
	s.clearBrowserVaultCookies(r.Context(), w)
	writeJSON(w, http.StatusOK, BrowserAccountMutationResponse{OK: true, VaultRevoked: true})
}

func (s *Server) handleBrowserAccountImage(w http.ResponseWriter, r *http.Request, escapedAccountID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	accountID, err := url.PathUnescape(escapedAccountID)
	if err != nil || strings.TrimSpace(accountID) == "" {
		writeError(w, http.StatusNotFound, "profile_image_not_found", "Profile image was not found.")
		return
	}
	vault, _, err := s.browserVaultForRequest(r.Context(), r)
	if err != nil {
		s.clearBrowserVaultCookies(r.Context(), w)
		writeError(w, http.StatusNotFound, "profile_image_not_found", "Profile image was not found.")
		return
	}
	var imagePath, profileImageURL, authOrigin, porticoUserID string
	err = s.queryUserRow(r.Context(), `
		SELECT COALESCE(u.profile_image_path, ''), COALESCE(u.profile_image_url, ''), COALESCE(u.auth_origin, 'local'), COALESCE(u.portico_user_id, '')
		FROM browser_account_entries e
		JOIN users u ON u.id = e.user_id
		WHERE e.vault_id = ? AND e.user_id = ? AND e.revoked_at = '' AND e.expires_at > ?`,
		vault.ID, accountID, time.Now().UTC().Format(time.RFC3339)).Scan(&imagePath, &profileImageURL, &authOrigin, &porticoUserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile_image_not_found", "Profile image was not found.")
		return
	}
	if strings.EqualFold(authOrigin, "portico") && profileImageURL != "" && porticoUserID != "" && s.serveHostedProfileImage(w, r, porticoUserID) {
		return
	}
	if imagePath == "" || !pathInsideRoot(imagePath, filepath.Join(s.cfg.AppDataDir, "profile-images")) {
		writeError(w, http.StatusNotFound, "profile_image_not_found", "Profile image was not found.")
		return
	}
	s.serveProfileImageFile(w, r, imagePath)
}

func (s *Server) browserAccountsResponse(ctx context.Context, r *http.Request) (BrowserAccountsResponse, error) {
	vault, _, err := s.browserVaultForRequest(ctx, r)
	if err != nil {
		return BrowserAccountsResponse{}, err
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT e.user_id, u.display_name, COALESCE(u.profile_image_path, ''), COALESCE(u.profile_image_url, ''),
			COALESCE(u.auth_origin, 'local'), e.auth_provider, e.last_used_at
		FROM browser_account_entries e
		JOIN users u ON u.id = e.user_id
		WHERE e.vault_id = ? AND e.revoked_at = '' AND e.expires_at > ?
		ORDER BY e.last_used_at DESC, u.display_name COLLATE NOCASE ASC`,
		vault.ID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return BrowserAccountsResponse{}, err
	}
	defer rows.Close()
	accounts := []BrowserAccountSummary{}
	for rows.Next() {
		var account BrowserAccountSummary
		var imagePath, imageURL string
		if err := rows.Scan(&account.ID, &account.DisplayName, &imagePath, &imageURL, &account.AuthOrigin, &account.AuthProvider, &account.LastUsedAt); err != nil {
			return BrowserAccountsResponse{}, err
		}
		if strings.TrimSpace(imagePath) != "" || strings.TrimSpace(imageURL) != "" {
			account.ProfileImageURL = "/api/auth/browser-accounts/" + url.PathEscape(account.ID) + "/image"
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return BrowserAccountsResponse{}, err
	}
	activeAccountID, ok := s.activeSessionUserForVault(ctx, r, vault.ID)
	if !ok || activeAccountID != vault.ActiveUserID {
		activeAccountID = ""
	}
	response := BrowserAccountsResponse{
		Accounts:          accounts,
		ActiveAccountID:   activeAccountID,
		AutomaticSignIn:   vault.AutomaticSignIn,
		SelectionRequired: len(accounts) > 0 && activeAccountID == "" && !vault.AutomaticSignIn,
		CanAddAccount:     s.canAddBrowserAccount(ctx),
	}
	return response, nil
}

func (s *Server) emptyBrowserAccountsResponse(ctx context.Context) BrowserAccountsResponse {
	return BrowserAccountsResponse{
		Accounts:        []BrowserAccountSummary{},
		AutomaticSignIn: true,
		CanAddAccount:   s.canAddBrowserAccount(ctx),
	}
}

func (s *Server) canAddBrowserAccount(ctx context.Context) bool {
	required, err := s.inspectSetupRequired(ctx, "/api/auth/browser-accounts")
	return err == nil && !required
}

func (s *Server) browserVaultForRequest(ctx context.Context, r *http.Request) (browserAccountVault, string, error) {
	if r == nil {
		return browserAccountVault{}, "", errBrowserVaultNotFound
	}
	fingerprint := s.browserVaultDeviceID(ctx, r)
	now := time.Now().UTC()
	for _, cookie := range s.requestBrowserVaultCookies(r) {
		var record browserAccountVault
		var automaticSignIn int
		var expiresAt string
		err := s.queryUserRow(ctx, `
			SELECT id, token_hash, device_id, COALESCE(active_user_id, ''), automatic_sign_in, expires_at
			FROM browser_account_vaults
			WHERE token_hash = ? AND revoked_at = '' AND expires_at > ?
			LIMIT 1`, hashToken(cookie.Value), now.Format(time.RFC3339)).Scan(
			&record.ID, &record.TokenHash, &record.DeviceID, &record.ActiveUserID, &automaticSignIn, &expiresAt,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return browserAccountVault{}, "", err
		}
		record.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
		if err != nil || !now.Before(record.ExpiresAt) || record.DeviceID != fingerprint {
			continue
		}
		record.AutomaticSignIn = automaticSignIn != 0
		return record, cookie.Value, nil
	}
	return browserAccountVault{}, "", errBrowserVaultNotFound
}

func (s *Server) activeSessionUserForVault(ctx context.Context, r *http.Request, vaultID string) (string, bool) {
	if r == nil || strings.TrimSpace(vaultID) == "" {
		return "", false
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, cookie := range s.requestSessionCookies(r) {
		var userID string
		if err := s.queryUserRow(ctx, `SELECT user_id FROM sessions WHERE token_hash = ? AND browser_vault_id = ? AND expires_at > ?`, hashToken(cookie.Value), vaultID, now).Scan(&userID); err == nil {
			return userID, true
		}
	}
	return "", false
}

func (s *Server) rememberBrowserAccountForSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID, provider, sessionToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	provider = normalizeAuthProvider(provider)
	now := time.Now().UTC()
	entryExpires := now.Add(browserVaultTTL)
	newVaultToken := randomToken()
	fingerprint := s.browserVaultDeviceID(ctx, r)
	var existingVaultToken string
	if _, token, err := s.browserVaultForRequest(ctx, r); err == nil {
		existingVaultToken = token
	}
	var vaultID string
	err := s.withUserTx(ctx, func(tx *sql.Tx) error {
		var sessionID, profileIdentityID, deviceID, sessionProvider string
		if err := tx.QueryRowContext(ctx, `
			SELECT id, COALESCE(profile_identity_id, ''), device_id, COALESCE(NULLIF(auth_provider, ''), 'local')
			FROM sessions WHERE token_hash = ? AND user_id = ?`, hashToken(sessionToken), userID).Scan(&sessionID, &profileIdentityID, &deviceID, &sessionProvider); err != nil {
			return err
		}
		if profileIdentityID == "" {
			return errors.New("profile identity is not available for remembered account")
		}
		provider = normalizeAuthProvider(sessionProvider)
		if existingVaultToken != "" {
			var storedFingerprint string
			if err := tx.QueryRowContext(ctx, `SELECT id, device_id FROM browser_account_vaults WHERE token_hash = ? AND revoked_at = '' AND expires_at > ?`, hashToken(existingVaultToken), now.Format(time.RFC3339)).Scan(&vaultID, &storedFingerprint); err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				vaultID = ""
			} else if storedFingerprint != fingerprint {
				vaultID = ""
			}
		}
		if vaultID == "" {
			vaultID = randomID("bvault")
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO browser_account_vaults
					(id, token_hash, device_id, active_user_id, automatic_sign_in, created_at, last_seen_at, expires_at)
				VALUES (?, ?, ?, ?, 1, ?, ?, ?)`,
				vaultID, hashToken(newVaultToken), fingerprint, userID, now.Format(time.RFC3339), now.Format(time.RFC3339), entryExpires.Format(time.RFC3339)); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				UPDATE browser_account_vaults
				SET token_hash = ?, active_user_id = ?, last_seen_at = ?, expires_at = ?, revoked_at = ''
				WHERE id = ?`, hashToken(newVaultToken), userID, now.Format(time.RFC3339), entryExpires.Format(time.RFC3339), vaultID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO browser_account_entries
				(vault_id, user_id, auth_provider, profile_identity_id, device_id, added_at, last_used_at, expires_at, revoked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')
			ON CONFLICT(vault_id, user_id) DO UPDATE SET
				auth_provider = excluded.auth_provider,
				profile_identity_id = excluded.profile_identity_id,
				device_id = excluded.device_id,
				last_used_at = excluded.last_used_at,
				expires_at = excluded.expires_at,
				revoked_at = ''`,
			vaultID, userID, provider, profileIdentityID, deviceID, now.Format(time.RFC3339), now.Format(time.RFC3339), entryExpires.Format(time.RFC3339)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE browser_vault_id = ? AND id <> ?`, vaultID, sessionID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE sessions SET browser_vault_id = ? WHERE id = ?`, vaultID, sessionID)
		return err
	})
	if err != nil {
		return err
	}
	s.clearBrowserVaultCookies(ctx, w)
	s.setBrowserVaultCookie(ctx, w, s.browserVaultCookieNameContext(ctx), newVaultToken, entryExpires)
	return nil
}

func (s *Server) switchBrowserAccount(ctx context.Context, r *http.Request, accountID string, descriptor ProfileDeviceDescriptor) (browserAccountSwitchResult, error) {
	vault, rawVaultToken, err := s.browserVaultForRequest(ctx, r)
	if err != nil {
		return browserAccountSwitchResult{}, err
	}
	// Load all response material before rotating the browser vault/session. Once
	// cookies are published the handler performs no fallible identity lookup,
	// avoiding a 500 response after the requested authentication change landed.
	candidateUser, err := s.getUser(accountID)
	if err != nil {
		return browserAccountSwitchResult{}, err
	}
	now := time.Now().UTC()
	newSessionToken := randomToken()
	newVaultToken := randomToken()
	profileAuthenticationToken, err := randomNativeCredentialToken(s.nativeCredentialEntropyReader())
	if err != nil {
		return browserAccountSwitchResult{}, err
	}
	profileAuthenticationID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "pauth")
	if err != nil {
		return browserAccountSwitchResult{}, err
	}
	browserBinding, err := randomNativeCredentialToken(s.nativeCredentialEntropyReader())
	if err != nil {
		return browserAccountSwitchResult{}, err
	}
	// Load chooser-safe data before the validating transaction, but never
	// return it unless the browser-vault entry passes every current account,
	// device, schedule, and auth-mode check below.
	profileDirectory, profileDirectoryErr := s.profileDirectoryContext(ctx, accountID)
	var userID, provider, installationID string
	var sessionExpires, vaultExpires time.Time
	profileAuthenticationRequired := false
	requireTrusted := s.requireTrustedDevicesContext(ctx)
	porticoMode := s.porticoAccountMode()
	err = s.withSecurityFenceTxTagged(ctx, []string{"sessions", "browser_accounts"}, func(tx *sql.Tx) error {
		var profileIdentityID, deviceID, entryExpiresAt, vaultExpiresAt, disabledAt, role, preferencesJSON string
		var porticoMembershipID string
		var trusted, maxActiveSessions int
		err := tx.QueryRowContext(ctx, `
			SELECT e.auth_provider, e.profile_identity_id, e.device_id, d.installation_id, e.expires_at, v.expires_at,
				COALESCE(u.disabled_at, ''), u.role, u.preferences_json, COALESCE(u.portico_membership_id, ''),
				d.trusted, COALESCE(u.max_active_sessions, 0)
			FROM browser_account_vaults v
			JOIN browser_account_entries e ON e.vault_id = v.id
			JOIN users u ON u.id = e.user_id
			JOIN devices d ON d.id = e.device_id AND d.user_id = e.user_id
			JOIN profile_identities pi ON pi.id = e.profile_identity_id AND pi.profile_id = e.user_id AND pi.provider = e.auth_provider
			WHERE v.id = ? AND v.token_hash = ? AND v.revoked_at = '' AND v.expires_at > ?
				AND e.user_id = ? AND e.revoked_at = ''`,
			vault.ID, hashToken(rawVaultToken), now.Format(time.RFC3339), accountID).Scan(
			&provider, &profileIdentityID, &deviceID, &installationID, &entryExpiresAt, &vaultExpiresAt,
			&disabledAt, &role, &preferencesJSON, &porticoMembershipID, &trusted, &maxActiveSessions,
		)
		if errors.Is(err, sql.ErrNoRows) {
			var expiresAt, revokedAt string
			lookupErr := tx.QueryRowContext(ctx, `SELECT expires_at, revoked_at FROM browser_account_entries WHERE vault_id = ? AND user_id = ?`, vault.ID, accountID).Scan(&expiresAt, &revokedAt)
			if lookupErr == nil && revokedAt == "" {
				expires, parseErr := time.Parse(time.RFC3339, expiresAt)
				if parseErr != nil || !now.Before(expires) {
					return errBrowserAccountExpired
				}
			}
			return errBrowserAccountNotFound
		}
		if err != nil {
			return err
		}
		entryExpiry, parseErr := time.Parse(time.RFC3339, entryExpiresAt)
		if parseErr != nil || !now.Before(entryExpiry) {
			return errBrowserAccountExpired
		}
		vaultExpiry, parseErr := time.Parse(time.RFC3339, vaultExpiresAt)
		if parseErr != nil || !now.Before(vaultExpiry) {
			return errBrowserVaultNotFound
		}
		if disabledAt != "" {
			return errBrowserAccountDisabled
		}
		provider = normalizeAuthProvider(provider)
		if (provider == "portico") != porticoMode {
			return errBrowserAuthModeUnavailable
		}
		if provider == "portico" {
			var memberships int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM remote_access_members
				WHERE local_user_id = ? AND status = 'active'
					AND (? = '' OR portico_membership_id = ?)`, accountID, porticoMembershipID, porticoMembershipID).Scan(&memberships); err != nil {
				return err
			}
			if memberships == 0 {
				return errBrowserMembershipInactive
			}
		}
		if trusted != 1 && requireTrusted {
			return errBrowserDeviceNotAllowed
		}
		if role != "owner" {
			policy := decodeUserDevicePolicy(preferencesJSON)
			switch policy.Mode {
			case "trusted":
				if trusted != 1 {
					return errBrowserDeviceNotAllowed
				}
			case "allowlist":
				allowed := false
				for _, allowedID := range policy.AllowedDeviceIDs {
					if allowedID == deviceID {
						allowed = true
						break
					}
				}
				if !allowed {
					return errBrowserDeviceNotAllowed
				}
			}
		}
		if !userAccessScheduleAllows(decodeUserAccessSchedule(preferencesJSON), now) {
			return errBrowserAccessSchedule
		}
		maxActiveSessions = normalizeMaxActiveSessions(maxActiveSessions)
		if maxActiveSessions > 0 {
			var active int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = ? AND expires_at > ? AND browser_vault_id <> ?`, accountID, now.Format(time.RFC3339), vault.ID).Scan(&active); err != nil {
				return err
			}
			if active >= maxActiveSessions {
				return errActiveSessionLimit
			}
		}
		sessionExpires = now.Add(s.browserSessionTTLContext(ctx))
		if entryExpiry.Before(sessionExpires) {
			sessionExpires = entryExpiry
		}
		if vaultExpiry.Before(sessionExpires) {
			sessionExpires = vaultExpiry
		}
		vaultExpires = vaultExpiry
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE browser_vault_id = ?`, vault.ID); err != nil {
			return err
		}
		selectionRequired, err := profileRequiresSelectionGrantTx(tx, accountID, accountID)
		if err != nil {
			return err
		}
		if selectionRequired {
			challengeInstallationID := normalizeUntrustedNativeInstallationID(descriptor.InstallationID)
			if provider != "local" || !validProfileDeviceDescriptor(descriptor) {
				return errInvalidProfileSelectionGrant
			}
			if profileDirectoryErr != nil {
				return profileDirectoryErr
			}
			descriptor.InstallationID = challengeInstallationID
			deviceID, err = s.upsertProfileAuthenticationDeviceTx(tx, r, accountID, descriptor, now)
			if err != nil {
				return err
			}
			installationID = challengeInstallationID
			profileAuthenticationRequired = true
			if _, err := tx.Exec(`
				DELETE FROM profile_account_authentications
				WHERE account_id = ? AND (
					(consumed_at <> '' OR expires_at <= ?)
					OR (purpose = 'browser' AND device_id = ?)
				)`, accountID, now.Format(time.RFC3339Nano), deviceID); err != nil {
				return err
			}
			if _, err := tx.Exec(`
				INSERT INTO profile_account_authentications (
					id, token_hash, account_id, auth_provider, purpose, device_id, installation_id, browser_binding_hash, expires_at, consumed_at, created_at
				) VALUES (?, ?, ?, 'local', 'browser', ?, ?, ?, ?, '', ?)`,
				profileAuthenticationID, hashToken(profileAuthenticationToken), accountID, deviceID, installationID,
				hashToken(browserBinding), now.Add(profileAccountAuthenticationTTL).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `
			INSERT INTO sessions
				(id, user_id, profile_id, profile_identity_id, auth_provider, device_id, browser_vault_id, token_hash, expires_at, created_at, last_seen_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			randomID("sess"), accountID, accountID, profileIdentityID, provider, deviceID, vault.ID, hashToken(newSessionToken), sessionExpires.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE browser_account_entries SET last_used_at = ? WHERE vault_id = ? AND user_id = ?`, now.Format(time.RFC3339), vault.ID, accountID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE browser_account_vaults
			SET token_hash = ?, active_user_id = ?, last_seen_at = ?
			WHERE id = ? AND token_hash = ? AND revoked_at = ''`,
			hashToken(newVaultToken), accountID, now.Format(time.RFC3339), vault.ID, hashToken(rawVaultToken))
		if err != nil {
			return err
		}
		if rowsAffected(result) == 0 {
			return errBrowserVaultNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET last_seen_at = ?, client_ip = CASE WHEN ? <> '' THEN ? ELSE client_ip END WHERE id = ? AND revoked_at = ''`, now.Format(time.RFC3339), clientIPFromRequest(r), clientIPFromRequest(r), deviceID); err != nil {
			return err
		}
		userID = accountID
		return nil
	})
	if err != nil {
		return browserAccountSwitchResult{}, err
	}
	result := browserAccountSwitchResult{
		UserID: userID, Provider: provider, VaultToken: newVaultToken, VaultExpires: vaultExpires,
	}
	if profileAuthenticationRequired {
		result.ProfileAuth = &ProfileAccountAuthenticationResponse{
			AccountAuthenticationToken: profileAuthenticationToken,
			ExpiresAt:                  now.Add(profileAccountAuthenticationTTL).Format(time.RFC3339Nano),
			Directory:                  profileDirectory,
		}
		result.BrowserBinding = browserBinding
		return result, nil
	}
	s.enrichUserAuthContextContext(ctx, &candidateUser, provider)
	result.User = &candidateUser
	result.SessionToken = newSessionToken
	result.SessionExpires = sessionExpires
	return result, nil
}

func (s *Server) removeBrowserAccount(ctx context.Context, r *http.Request, accountID string) (BrowserAccountMutationResponse, string, time.Time, error) {
	vault, rawVaultToken, err := s.browserVaultForRequest(ctx, r)
	if err != nil {
		return BrowserAccountMutationResponse{}, "", time.Time{}, err
	}
	now := time.Now().UTC()
	newVaultToken := randomToken()
	result := BrowserAccountMutationResponse{OK: true}
	var vaultExpires time.Time
	err = s.withSecurityFenceTxTagged(ctx, []string{"sessions", "browser_accounts"}, func(tx *sql.Tx) error {
		var activeUserID string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(active_user_id, '') FROM browser_account_vaults WHERE id = ? AND token_hash = ? AND revoked_at = '' AND expires_at > ?`, vault.ID, hashToken(rawVaultToken), now.Format(time.RFC3339)).Scan(&activeUserID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errBrowserVaultNotFound
			}
			return err
		}
		entryResult, err := tx.ExecContext(ctx, `UPDATE browser_account_entries SET revoked_at = ? WHERE vault_id = ? AND user_id = ? AND revoked_at = ''`, now.Format(time.RFC3339), vault.ID, accountID)
		if err != nil {
			return err
		}
		if rowsAffected(entryResult) == 0 {
			return errBrowserAccountNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE browser_vault_id = ? AND user_id = ?`, vault.ID, accountID); err != nil {
			return err
		}
		result.ActiveAccountRemoved = activeUserID == accountID
		var remaining int
		var maxExpiry string
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(expires_at), '') FROM browser_account_entries WHERE vault_id = ? AND revoked_at = '' AND expires_at > ?`, vault.ID, now.Format(time.RFC3339)).Scan(&remaining, &maxExpiry); err != nil {
			return err
		}
		if remaining == 0 {
			result.VaultRevoked = true
			_, err := tx.ExecContext(ctx, `UPDATE browser_account_vaults SET active_user_id = NULL, revoked_at = ? WHERE id = ?`, now.Format(time.RFC3339), vault.ID)
			return err
		}
		vaultExpires, err = time.Parse(time.RFC3339, maxExpiry)
		if err != nil {
			return err
		}
		activeValue := activeUserID
		if result.ActiveAccountRemoved {
			activeValue = ""
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE browser_account_vaults
			SET token_hash = ?, active_user_id = NULLIF(?, ''), last_seen_at = ?, expires_at = ?
			WHERE id = ?`, hashToken(newVaultToken), activeValue, now.Format(time.RFC3339), vaultExpires.Format(time.RFC3339), vault.ID)
		return err
	})
	if err != nil {
		return BrowserAccountMutationResponse{}, "", time.Time{}, err
	}
	return result, newVaultToken, vaultExpires, nil
}

func (s *Server) rotateBrowserVaultForRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, activeUserID string) error {
	vault, rawToken, err := s.browserVaultForRequest(ctx, r)
	if err != nil {
		return nil
	}
	if activeUserID != "" && vault.ActiveUserID != activeUserID {
		return nil
	}
	newToken := randomToken()
	result, err := s.execUserWrite(ctx, `UPDATE browser_account_vaults SET token_hash = ?, last_seen_at = ? WHERE id = ? AND token_hash = ? AND revoked_at = ''`, hashToken(newToken), time.Now().UTC().Format(time.RFC3339), vault.ID, hashToken(rawToken))
	if err != nil {
		return err
	}
	if rowsAffected(result) == 0 {
		return errBrowserVaultNotFound
	}
	s.clearBrowserVaultCookies(ctx, w)
	s.setBrowserVaultCookie(ctx, w, s.browserVaultCookieNameContext(ctx), newToken, vault.ExpiresAt)
	return nil
}

func (s *Server) revokeBrowserEntriesForUserTx(ctx context.Context, tx *sql.Tx, userID, preservedSessionID, now string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE browser_account_entries SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE user_id = ?`, now, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE browser_vault_id <> '' AND user_id = ? AND (? = '' OR id <> ?)`, userID, preservedSessionID, preservedSessionID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE browser_account_vaults
		SET active_user_id = CASE WHEN active_user_id = ? THEN NULL ELSE active_user_id END,
			revoked_at = CASE
				WHEN NOT EXISTS (
					SELECT 1 FROM browser_account_entries e
					WHERE e.vault_id = browser_account_vaults.id AND e.revoked_at = '' AND e.expires_at > ?
				) THEN CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END
				ELSE revoked_at
			END
		WHERE id IN (SELECT vault_id FROM browser_account_entries WHERE user_id = ?)`, userID, now, now, userID)
	return err
}

func (s *Server) revokeBrowserEntryForDeviceTx(ctx context.Context, tx *sql.Tx, deviceID, now string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE browser_account_entries SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE device_id = ?`, now, deviceID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE browser_account_vaults
		SET active_user_id = CASE
				WHEN active_user_id IN (SELECT user_id FROM browser_account_entries e WHERE e.vault_id = browser_account_vaults.id AND e.device_id = ?) THEN NULL
				ELSE active_user_id
			END,
			revoked_at = CASE
				WHEN NOT EXISTS (
					SELECT 1 FROM browser_account_entries e
					WHERE e.vault_id = browser_account_vaults.id AND e.revoked_at = '' AND e.expires_at > ?
				) THEN CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END
				ELSE revoked_at
			END
		WHERE id IN (SELECT vault_id FROM browser_account_entries WHERE device_id = ?)`, deviceID, now, now, deviceID)
	return err
}

func (s *Server) browserVaultDeviceID(ctx context.Context, r *http.Request) string {
	serverID := ""
	if identity, err := s.systemIdentityContext(ctx); err == nil {
		serverID = identity.ServerID
	}
	userAgent := ""
	platform := ""
	if r != nil {
		userAgent = strings.TrimSpace(r.UserAgent())
		platform = strings.TrimSpace(r.Header.Get("Sec-CH-UA-Platform"))
	}
	fingerprint := hashToken(serverID + "\n" + userAgent + "\n" + platform)
	return "browser_" + fingerprint[:24]
}

func (s *Server) browserVaultCookieNameContext(ctx context.Context) string {
	preferred := browserVaultCookieName
	if identity, err := s.systemIdentityContext(ctx); err == nil {
		if suffix := cookieNameSuffix(identity.ServerID); suffix != "" {
			preferred += "_" + suffix
		}
	}
	return preferred
}

func (s *Server) browserVaultCookieNamesContext(ctx context.Context) []string {
	preferred := s.browserVaultCookieNameContext(ctx)
	if preferred == browserVaultCookieName {
		return []string{preferred}
	}
	return []string{preferred, browserVaultCookieName}
}

func (s *Server) requestBrowserVaultCookies(r *http.Request) []*http.Cookie {
	if r == nil {
		return nil
	}
	cookies := make([]*http.Cookie, 0, 2)
	for _, name := range s.browserVaultCookieNamesContext(r.Context()) {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
			cookies = append(cookies, cookie)
		}
	}
	return cookies
}

func (s *Server) setBrowserVaultCookie(ctx context.Context, w http.ResponseWriter, name, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/api",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookieForContext(ctx),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearBrowserVaultCookies(ctx context.Context, w http.ResponseWriter) {
	for _, name := range s.browserVaultCookieNamesContext(context.Background()) {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/api",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   s.secureCookieForContext(ctx),
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func writeBrowserAccountSwitchError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBrowserVaultNotFound), errors.Is(err, errBrowserAccountNotFound):
		writeError(w, http.StatusNotFound, "browser_account_not_found", "That account is no longer remembered on this browser.")
	case errors.Is(err, errBrowserAccountExpired):
		writeError(w, http.StatusUnauthorized, "browser_account_expired", "That remembered account has expired. Sign in again to add it back.")
	case errors.Is(err, errBrowserAccountDisabled):
		writeError(w, http.StatusForbidden, "account_disabled", "That account is disabled.")
	case errors.Is(err, errBrowserAccessSchedule), errors.Is(err, errAccessSchedule):
		writeError(w, http.StatusForbidden, "access_schedule_denied", "That account is outside its allowed access schedule.")
	case errors.Is(err, errBrowserDeviceNotAllowed), errors.Is(err, errDeviceNotAllowed), errors.Is(err, errDeviceNotTrusted):
		writeError(w, http.StatusForbidden, "device_not_allowed", "That account is not allowed to use this browser device.")
	case errors.Is(err, errBrowserMembershipInactive):
		writeError(w, http.StatusForbidden, "membership_inactive", "That Portico Account no longer has access to this server.")
	case errors.Is(err, errBrowserAuthModeUnavailable):
		writeError(w, http.StatusConflict, "auth_mode_unavailable", "That sign-in method is no longer enabled on this server.")
	case errors.Is(err, errActiveSessionLimit):
		writeError(w, http.StatusForbidden, "active_session_limit", "That account has reached its maximum active session limit.")
	case errors.Is(err, errInvalidProfileSelectionGrant):
		writeProductError(w, http.StatusConflict, "profile_selection_required", "Choose a profile before opening this remembered account.")
	default:
		writeError(w, http.StatusInternalServerError, "browser_account_switch_failed", "Unable to switch accounts.")
	}
}
