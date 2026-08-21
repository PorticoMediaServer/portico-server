package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const profileAccountAuthenticationTTL = 5 * time.Minute

const profileBrowserBindingCookieName = "portico_profile_auth_binding"

var (
	errInvalidProfileAccountAuthentication = errors.New("profile account authentication is invalid or expired")
	errProfileAccountAuthenticationUsed    = errors.New("profile account authentication has already been used")
)

type ProfileDeviceDescriptor struct {
	InstallationID string `json:"installationId,omitempty"`
	DeviceName     string `json:"deviceName"`
	App            string `json:"app"`
	Platform       string `json:"platform"`
}

type LocalProfileAccountAuthenticationRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	Purpose  string `json:"purpose"`
	ProfileDeviceDescriptor
}

type ProfileAvatar struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

type SelectableProfile struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Avatar         *ProfileAvatar      `json:"avatar,omitempty"`
	IsPrimary      bool                `json:"isPrimary"`
	IsAccountAdmin bool                `json:"isAccountAdmin"`
	HasPIN         bool                `json:"hasPIN"`
	PINRevision    int64               `json:"pinRevision"`
	SortOrder      int                 `json:"sortOrder"`
	Policy         ProfileRestrictions `json:"policy"`
}

type ProfileDirectory struct {
	Authority       string              `json:"authority"`
	AccountID       string              `json:"accountId"`
	ServerID        string              `json:"serverId"`
	ProfilesAllowed bool                `json:"profilesAllowed"`
	Profiles        []SelectableProfile `json:"profiles"`
}

type ProfileAccountAuthenticationResponse struct {
	AccountAuthenticationToken string           `json:"accountAuthenticationToken"`
	ExpiresAt                  string           `json:"expiresAt"`
	Directory                  ProfileDirectory `json:"directory"`
}

type LocalProfileSelectionRequest struct {
	AccountAuthenticationToken string `json:"accountAuthenticationToken"`
	ProfileID                  string `json:"profileId"`
	PIN                        string `json:"pin,omitempty"`
	AutomaticSelectionTrust    string `json:"automaticSelectionTrust,omitempty"`
}

// ActiveLocalProfileSelectionRequest changes the viewing profile beneath an
// already-authenticated Local Auth account. The account session is the account
// proof; only the selected profile's PIN (when configured) is requested again.
// This deliberately cannot cross accounts or switch a hosted identity.
type ActiveLocalProfileSelectionRequest struct {
	ProfileID string `json:"profileId"`
	PIN       string `json:"pin,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
}

type ProfileSelectionResponse struct {
	SelectionGrant string `json:"token"`
	Authority      string `json:"authority"`
	AccountID      string `json:"accountId"`
	ServerID       string `json:"serverId"`
	ProfileID      string `json:"profileId"`
	PINRevision    int64  `json:"pinRevision"`
	InstallationID string `json:"installationId,omitempty"`
	ExpiresAt      string `json:"expiresAt"`
}

type BrowserProfileSessionRequest struct {
	SelectionGrant  string `json:"selectionGrant"`
	RememberBrowser *bool  `json:"rememberOnBrowser,omitempty"`
}

type NativeProfileSessionRequest struct {
	SelectionGrant string `json:"selectionGrant"`
	ProfileDeviceDescriptor
}

type profileAccountAuthenticationRecord struct {
	ID                 string
	AccountID          string
	AuthProvider       string
	Purpose            string
	DeviceID           string
	InstallationID     string
	BrowserBindingHash string
	ExpiresAt          time.Time
}

func (s *Server) handleLocalProfileAccountAuthentication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if s.porticoAccountMode() {
		writeProductError(w, http.StatusForbidden, "unsupported_auth_mode", "This server uses Portico Account sign-in.")
		return
	}
	var request LocalProfileAccountAuthenticationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	purpose := strings.ToLower(strings.TrimSpace(request.Purpose))
	if purpose != "browser" && purpose != "native" {
		writeProductError(w, http.StatusBadRequest, "invalid_profile_authentication_purpose", "Purpose must be browser or native.")
		return
	}
	installationID := normalizeUntrustedNativeInstallationID(request.InstallationID)
	if strings.TrimSpace(request.Login) == "" || request.Password == "" {
		writeProductError(w, http.StatusBadRequest, "credentials_required", "Login and password are required.")
		return
	}
	rateKey := loginRateKey("profile-account-auth", r, request.Login)
	if !s.allowLoginAttempt(rateKey) {
		w.Header().Set("Retry-After", "60")
		writeProductError(w, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts.")
		return
	}
	if !validProfileDeviceDescriptor(request.ProfileDeviceDescriptor) {
		writeProductError(w, http.StatusBadRequest, "device_identity_required", "A valid device name, app, and platform are required.")
		return
	}
	user, err := s.authenticateLocalNativeUser(r.Context(), request.Login, request.Password)
	if err != nil {
		s.recordLoginFailure(rateKey)
		writeProductError(w, http.StatusUnauthorized, "invalid_credentials", "Username/email or password is incorrect.")
		return
	}
	if installationID == "" {
		installationID, err = nativeSecureRandomID(s.nativeCredentialEntropyReader(), "install")
		if err != nil {
			writeProductError(w, http.StatusInternalServerError, "session_failed", "Unable to create a server device record.")
			return
		}
	}
	request.InstallationID = installationID
	response, browserBinding, err := s.createProfileAccountAuthentication(r, user, purpose, request.ProfileDeviceDescriptor)
	if err != nil {
		s.writeProfileAuthenticationError(w, err)
		return
	}
	s.clearLoginFailures(rateKey)
	if browserBinding != "" {
		s.setProfileBrowserBindingCookie(r.Context(), w, browserBinding, time.Now().UTC().Add(profileAccountAuthenticationTTL))
	}
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handleLocalProfileSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request LocalProfileSelectionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.AccountAuthenticationToken) == "" || strings.TrimSpace(request.ProfileID) == "" {
		writeProductError(w, http.StatusBadRequest, "profile_selection_required", "Choose a profile to continue.")
		return
	}
	rateKey := s.profileSelectionRateKey(r.Context(), r, request.AccountAuthenticationToken, request.ProfileID)
	if !s.allowLoginAttempt(rateKey) {
		writeProductError(w, http.StatusTooManyRequests, "rate_limited", "Too many profile selection attempts.")
		return
	}
	grant, err := s.consumeLocalProfileAccountAuthentication(r.Context(), request, s.profileBrowserBindingCookie(r), time.Now().UTC())
	if err != nil {
		s.recordLoginFailure(rateKey)
		s.writeProfileAuthenticationError(w, err)
		return
	}
	s.clearLoginFailures(rateKey)
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "profile_selection_failed", "Portico couldn't open this profile. Choose it again or try another profile.")
		return
	}
	writeJSON(w, http.StatusCreated, ProfileSelectionResponse{
		SelectionGrant: grant.Token, Authority: "local", AccountID: grant.AccountID, ServerID: serverID,
		ProfileID: grant.ProfileID, PINRevision: grant.PINRevision, InstallationID: grant.InstallationID,
		ExpiresAt: grant.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func (s *Server) handleActiveLocalProfileSelection(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if normalizeAuthProvider(user.AuthProvider) != "local" || strings.TrimSpace(user.AuthOrigin) != "local" {
		writeProductError(w, http.StatusForbidden, "unsupported_auth_mode", "This profile is managed by your Portico Account.")
		return
	}
	var request ActiveLocalProfileSelectionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	request.ProfileID = strings.TrimSpace(request.ProfileID)
	if request.ProfileID == "" {
		writeProductError(w, http.StatusBadRequest, "profile_selection_required", "Choose a profile to continue.")
		return
	}
	purpose := strings.ToLower(strings.TrimSpace(request.Purpose))
	if purpose == "" {
		purpose = "browser"
	}
	if purpose != "browser" && purpose != "native" {
		writeProductError(w, http.StatusBadRequest, "invalid_profile_selection_purpose", "Purpose must be browser or native.")
		return
	}
	accountID := accountIDForUser(user)
	deviceID := strings.TrimSpace(user.DeviceID)
	if accountID == "" || deviceID == "" {
		writeProductError(w, http.StatusUnauthorized, "interactive_session_required", "Open Portico from an active browser session and try again.")
		return
	}
	rateKey := strings.Join([]string{"active-profile-selection", accountID, request.ProfileID, deviceID, clientIPFromRequest(r)}, ":")
	if !s.allowLoginAttempt(rateKey) {
		writeProductError(w, http.StatusTooManyRequests, "rate_limited", "Too many profile selection attempts.")
		return
	}
	var installationID string
	_ = s.queryUserRow(r.Context(), `SELECT COALESCE(installation_id, '') FROM devices WHERE id = ? AND user_id = ? AND COALESCE(revoked_at, '') = ''`, deviceID, accountID).Scan(&installationID)
	now := time.Now().UTC()
	grant, err := s.issueLocalProfileSelectionGrantForPurposeContext(
		r.Context(), accountID, request.ProfileID, request.PIN, deviceID, installationID,
		purpose, randomID("session_profile_selection"), now,
	)
	if err != nil {
		s.recordLoginFailure(rateKey)
		s.writeProfileAuthenticationError(w, err)
		return
	}
	if purpose == "browser" {
		browserBinding, err := randomNativeCredentialToken(s.nativeCredentialEntropyReader())
		if err != nil {
			writeProductError(w, http.StatusInternalServerError, "profile_selection_failed", "Portico couldn't open this profile. Choose it again or try another profile.")
			return
		}
		bindingHash := hashToken(browserBinding)
		if _, err := s.execUserWrite(r.Context(), `UPDATE profile_selection_grants SET browser_binding_hash = ? WHERE token_hash = ?`, bindingHash, hashToken(grant.Token)); err != nil {
			writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "profile_selection_failed", "Portico couldn't open this profile. Choose it again or try another profile.")
			return
		}
		s.setProfileBrowserBindingCookie(r.Context(), w, browserBinding, grant.ExpiresAt)
	}
	s.clearLoginFailures(rateKey)
	serverID, err := s.profileDirectoryServerIDContext(r.Context())
	if err != nil {
		writeProductError(w, http.StatusInternalServerError, "profile_selection_failed", "Portico couldn't open this profile. Choose it again or try another profile.")
		return
	}
	writeJSON(w, http.StatusCreated, ProfileSelectionResponse{
		SelectionGrant: grant.Token, Authority: "local", AccountID: grant.AccountID, ServerID: serverID,
		ProfileID: grant.ProfileID, PINRevision: grant.PINRevision, InstallationID: grant.InstallationID,
		ExpiresAt: grant.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func (s *Server) handleBrowserProfileSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request BrowserProfileSessionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	record, err := s.profileSelectionGrantRecord(r.Context(), request.SelectionGrant, "local", "browser", time.Now().UTC())
	if err != nil {
		s.writeProfileAuthenticationError(w, err)
		return
	}
	if record.BrowserBindingHash == "" || !credentialDigestMatches(record.BrowserBindingHash, s.profileBrowserBindingCookie(r)) {
		s.writeProfileAuthenticationError(w, errInvalidProfileSelectionGrant)
		return
	}
	user, err := s.getUser(record.AccountID)
	if err != nil {
		s.writeProfileAuthenticationError(w, errInvalidProfileSelectionGrant)
		return
	}
	principal, err := s.resolveRequestPrincipalContext(r.Context(), user.ID, record.ProfileID)
	if err != nil {
		s.writeProfileAuthenticationError(w, errInvalidProfileSelectionGrant)
		return
	}
	selected := user
	applyRequestPrincipal(&selected, principal)
	s.enrichUserAuthContextContext(r.Context(), &selected, "local")
	token, err := s.createSessionForProviderWithSessionOptions(w, r, user.ID, "local", sessionCreateOptions{
		ProfileID:               record.ProfileID,
		ProfileSelectionGrant:   request.SelectionGrant,
		ProfileSelectionPurpose: "browser",
		BoundDeviceID:           record.DeviceID,
		BoundInstallationID:     record.InstallationID,
	})
	if err != nil {
		s.writeProfileAuthenticationError(w, err)
		return
	}
	if rememberBrowserAccount(request.RememberBrowser) {
		if err := s.rememberBrowserAccountForSession(r.Context(), w, r, user.ID, "local", token); err != nil {
			s.discardBrowserSession(w, r.Context(), token)
			writeProductError(w, http.StatusInternalServerError, "browser_account_enrollment_failed", "Signed in credentials were valid, but this account could not be remembered on the browser.")
			return
		}
	}
	s.clearProfileBrowserBindingCookie(r.Context(), w)
	writeJSON(w, http.StatusCreated, s.authResponseWithServerContext(r.Context(), true, false, &selected))
}

func (s *Server) handleNativeProfileSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var request NativeProfileSessionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !validProfileDeviceDescriptor(request.ProfileDeviceDescriptor) {
		writeProductError(w, http.StatusBadRequest, "device_identity_required", "A valid device name, app, and platform are required.")
		return
	}
	receiptKind := "local-profile-session"
	receiptProof := nativeAuthExchangeProof(receiptKind, request.SelectionGrant, request.DeviceName, request.App, request.Platform)
	if recovered, recoveryErr := s.recoverNativeAuthExchangeReceipt(r.Context(), receiptKind, receiptProof); recoveryErr == nil {
		s.setNativeAccessSessionCookie(r.Context(), w, recovered)
		writeJSON(w, http.StatusCreated, recovered)
		return
	} else if !errors.Is(recoveryErr, sql.ErrNoRows) {
		s.writeNativeSessionCreationError(w, recoveryErr)
		return
	}
	record, err := s.profileSelectionGrantRecord(r.Context(), request.SelectionGrant, "local", "native", time.Now().UTC())
	if err != nil {
		s.writeProfileAuthenticationError(w, err)
		return
	}
	user, err := s.getUser(record.AccountID)
	if err != nil {
		s.writeProfileAuthenticationError(w, errInvalidProfileSelectionGrant)
		return
	}
	s.enrichUserAuthContextContext(r.Context(), &user, "local")
	credentials, err := s.issueNativeSessionCredentials(r, user, "local", nativeDeviceDescriptor{
		InstallationID:          record.InstallationID,
		Name:                    request.DeviceName,
		App:                     request.App,
		Platform:                request.Platform,
		ProfileSelectionGrant:   request.SelectionGrant,
		ProfileSelectionPurpose: "native",
		ExchangeReceiptKind:     receiptKind,
		ExchangeReceiptProof:    receiptProof,
	})
	if err != nil {
		s.writeProfileAuthenticationError(w, err)
		return
	}
	s.setNativeAccessSessionCookie(r.Context(), w, credentials)
	writeJSON(w, http.StatusCreated, credentials)
}

func validProfileDeviceDescriptor(descriptor ProfileDeviceDescriptor) bool {
	return strings.TrimSpace(descriptor.DeviceName) != "" && len([]rune(strings.TrimSpace(descriptor.DeviceName))) <= 120 &&
		strings.TrimSpace(descriptor.App) != "" && len([]rune(strings.TrimSpace(descriptor.App))) <= 120 &&
		strings.TrimSpace(descriptor.Platform) != "" && len([]rune(strings.TrimSpace(descriptor.Platform))) <= 120
}

func credentialDigestMatches(expectedDigest, raw string) bool {
	expectedDigest = strings.TrimSpace(expectedDigest)
	raw = strings.TrimSpace(raw)
	if expectedDigest == "" || raw == "" {
		return false
	}
	actualDigest := hashToken(raw)
	return len(expectedDigest) == len(actualDigest) && subtle.ConstantTimeCompare([]byte(expectedDigest), []byte(actualDigest)) == 1
}

func (s *Server) profileBrowserBindingCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(profileBrowserBindingCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (s *Server) setProfileBrowserBindingCookie(ctx context.Context, w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     profileBrowserBindingCookieName,
		Value:    token,
		Path:     "/api/auth/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookieForContext(ctx),
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearProfileBrowserBindingCookie(ctx context.Context, w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     profileBrowserBindingCookieName,
		Value:    "",
		Path:     "/api/auth/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookieForContext(ctx),
		SameSite: http.SameSiteStrictMode,
	})
}

func shortCredentialDigest(value string) string {
	digest := hashToken(strings.TrimSpace(value))
	if len(digest) > 16 {
		return digest[:16]
	}
	return digest
}

func (s *Server) profileSelectionRateKey(ctx context.Context, r *http.Request, rawToken, profileID string) string {
	material := clientIPFromRequest(r) + "\n" + shortCredentialDigest(rawToken) + "\n" + strings.TrimSpace(profileID)
	var accountID, deviceID, installationID string
	if err := s.queryUserRow(ctx, `
		SELECT account_id, device_id, installation_id
		FROM profile_account_authentications WHERE token_hash = ? AND consumed_at = ''`, hashToken(strings.TrimSpace(rawToken))).
		Scan(&accountID, &deviceID, &installationID); err == nil {
		material = accountID + "\n" + strings.TrimSpace(profileID) + "\n" + deviceID + "\n" + installationID + "\n" + clientIPFromRequest(r)
	}
	return "profile-selection:" + shortCredentialDigest(material)
}

func (s *Server) createProfileAccountAuthentication(r *http.Request, user User, purpose string, descriptor ProfileDeviceDescriptor) (ProfileAccountAuthenticationResponse, string, error) {
	now := time.Now().UTC()
	rawToken, err := randomNativeCredentialToken(s.nativeCredentialEntropyReader())
	if err != nil {
		return ProfileAccountAuthenticationResponse{}, "", err
	}
	recordID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "pauth")
	if err != nil {
		return ProfileAccountAuthenticationResponse{}, "", err
	}
	directory, err := s.profileDirectoryContext(r.Context(), user.ID)
	if err != nil {
		return ProfileAccountAuthenticationResponse{}, "", err
	}
	browserBinding := ""
	browserBindingHash := ""
	if purpose == "browser" {
		browserBinding, err = randomNativeCredentialToken(s.nativeCredentialEntropyReader())
		if err != nil {
			return ProfileAccountAuthenticationResponse{}, "", err
		}
		browserBindingHash = hashToken(browserBinding)
	}
	var deviceID string
	err = s.withUserTxTagged(r.Context(), []string{"devices", "profile_account_authentications"}, func(tx *sql.Tx) error {
		var err error
		deviceID, err = s.upsertProfileAuthenticationDeviceTx(tx, r, user.ID, descriptor, now)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM profile_account_authentications WHERE account_id = ? AND (consumed_at <> '' OR expires_at <= ?)`, user.ID, now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO profile_account_authentications (
				id, token_hash, account_id, auth_provider, purpose, device_id, installation_id, browser_binding_hash, expires_at, consumed_at, created_at
			) VALUES (?, ?, ?, 'local', ?, ?, ?, ?, ?, '', ?)`,
			recordID, hashToken(rawToken), user.ID, purpose, deviceID, descriptor.InstallationID,
			browserBindingHash, now.Add(profileAccountAuthenticationTTL).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return ProfileAccountAuthenticationResponse{}, "", err
	}
	return ProfileAccountAuthenticationResponse{
		AccountAuthenticationToken: rawToken,
		ExpiresAt:                  now.Add(profileAccountAuthenticationTTL).Format(time.RFC3339Nano),
		Directory:                  directory,
	}, browserBinding, nil
}

func (s *Server) profileDirectoryServerIDContext(ctx context.Context) (string, error) {
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return "", err
	}
	return s.localServerIDContext(ctx, settings)
}

func (s *Server) profileDirectoryContext(ctx context.Context, accountID string) (ProfileDirectory, error) {
	profiles, err := s.listAccountProfilesContext(ctx, accountID)
	if err != nil {
		return ProfileDirectory{}, err
	}
	var allowProfiles int
	var authOrigin string
	if err := s.queryUserRow(ctx, `SELECT COALESCE(allow_account_profiles, 1), COALESCE(auth_origin, 'local') FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, accountID).Scan(&allowProfiles, &authOrigin); err != nil {
		return ProfileDirectory{}, err
	}
	result := make([]SelectableProfile, 0, len(profiles))
	for _, profile := range profiles {
		var avatar *ProfileAvatar
		if reference := strings.TrimSpace(profile.AvatarURL); reference != "" {
			avatar = &ProfileAvatar{Kind: "custom", Reference: reference}
		}
		result = append(result, SelectableProfile{
			ID: profile.ID, Name: profile.DisplayName, Avatar: avatar,
			IsPrimary: profile.IsPrimary, IsAccountAdmin: profile.IsPrimary,
			HasPIN: profile.PINRequired, PINRevision: profile.PINRevision,
			SortOrder: profile.SortOrder, Policy: profile.Restrictions,
		})
	}
	if len(result) == 0 {
		return ProfileDirectory{}, errProfileNotFound
	}
	serverID, err := s.profileDirectoryServerIDContext(ctx)
	if err != nil {
		return ProfileDirectory{}, err
	}
	authority := "local"
	if authOrigin == "portico" {
		authority = "hosted"
	}
	return ProfileDirectory{Authority: authority, AccountID: accountID, ServerID: serverID, ProfilesAllowed: allowProfiles == 1, Profiles: result}, nil
}

func (s *Server) accountRequiresExplicitProfileSelectionContext(ctx context.Context, accountID string) (bool, error) {
	var activeProfiles, primaryPINRequired int
	if err := s.queryUserRow(ctx, `
		SELECT COUNT(*), COALESCE(MAX(CASE WHEN is_primary = 1 THEN pin_required ELSE 0 END), 0)
		FROM profiles WHERE account_id = ? AND disabled_at = ''`, strings.TrimSpace(accountID)).Scan(&activeProfiles, &primaryPINRequired); err != nil {
		return false, err
	}
	return activeProfiles != 1 || primaryPINRequired == 1, nil
}

func (s *Server) upsertProfileAuthenticationDeviceTx(tx *sql.Tx, r *http.Request, accountID string, descriptor ProfileDeviceDescriptor, now time.Time) (string, error) {
	deviceID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "dev")
	if err != nil {
		return "", err
	}
	installationID := strings.TrimSpace(descriptor.InstallationID)
	serverGeneratedInstallation := installationID == ""
	if serverGeneratedInstallation {
		installationID = "server:" + deviceID
	}
	trusted := !s.requireTrustedDevicesContext(r.Context())
	_, err = tx.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, display_name, app, platform, user_agent, client_ip, trusted, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, installation_id) WHERE installation_id <> '' DO UPDATE SET
			name = excluded.name,
			app = excluded.app,
			platform = excluded.platform,
			user_agent = excluded.user_agent,
			client_ip = excluded.client_ip,
			last_seen_at = excluded.last_seen_at`,
		deviceID, accountID, installationID, truncateDeviceName(descriptor.DeviceName),
		truncateDeviceName(descriptor.App), truncateDeviceName(descriptor.Platform), strings.TrimSpace(r.UserAgent()),
		clientIPFromRequest(r), boolInt(trusted), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	var revokedAt string
	if serverGeneratedInstallation {
		err = tx.QueryRow(`SELECT id, COALESCE(revoked_at, '') FROM devices WHERE id = ? AND user_id = ?`, deviceID, accountID).Scan(&deviceID, &revokedAt)
	} else {
		err = tx.QueryRow(`SELECT id, COALESCE(revoked_at, '') FROM devices WHERE user_id = ? AND installation_id = ?`, accountID, installationID).Scan(&deviceID, &revokedAt)
	}
	if err != nil {
		return "", err
	}
	if revokedAt != "" {
		return "", errDeviceNotAllowed
	}
	return deviceID, nil
}

func (s *Server) consumeLocalProfileAccountAuthentication(ctx context.Context, request LocalProfileSelectionRequest, browserBinding string, now time.Time) (ProfileSelectionGrant, error) {
	var grant ProfileSelectionGrant
	var pinResult error
	serverID, err := s.profileDirectoryServerIDContext(ctx)
	if err != nil {
		return ProfileSelectionGrant{}, err
	}
	err = s.withUserTxTagged(ctx, []string{"profile_account_authentications", "profiles", "local_profile_pin_credentials", "profile_selection_grants", "automatic_profile_selection_trusts"}, func(tx *sql.Tx) error {
		record, err := profileAccountAuthenticationTx(tx, request.AccountAuthenticationToken, "local", now)
		if err != nil {
			return err
		}
		if record.Purpose == "browser" && (record.BrowserBindingHash == "" || !credentialDigestMatches(record.BrowserBindingHash, browserBinding)) {
			return errInvalidProfileAccountAuthentication
		}
		var origin string
		var pinRequired, primary, profilesAllowed int
		if err := tx.QueryRow(`
			SELECT p.origin, p.pin_required, p.is_primary, COALESCE(u.allow_account_profiles, 1)
			FROM profiles p JOIN users u ON u.id = p.account_id
			WHERE p.id = ? AND p.account_id = ? AND p.disabled_at = '' AND COALESCE(u.disabled_at, '') = ''`,
			strings.TrimSpace(request.ProfileID), record.AccountID).Scan(&origin, &pinRequired, &primary, &profilesAllowed); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errProfileNotFound
			}
			return err
		}
		if origin != "local" {
			return errHostedProfileLocalPIN
		}
		if profilesAllowed != 1 && primary != 1 {
			return errProfileNotAllowed
		}
		trustedAutomaticSelection := strings.TrimSpace(request.AutomaticSelectionTrust) != ""
		if trustedAutomaticSelection {
			if err := automaticProfileTrustTx(tx, request.AutomaticSelectionTrust, record, request.ProfileID, serverID, now); err != nil {
				return err
			}
		}
		if pinRequired == 1 && !trustedAutomaticSelection {
			if !validLocalProfilePIN(request.PIN) {
				return errInvalidProfilePIN
			}
			valid, err := verifyLocalProfilePINTx(tx, record.AccountID, request.ProfileID, request.PIN, now)
			if err != nil {
				return err
			}
			if !valid {
				pinResult = profilePINAttemptResultTx(tx, request.ProfileID, now)
				return nil
			}
		}
		grant, err = s.mintProfileSelectionGrantBoundTx(tx, record.AccountID, request.ProfileID, "local", record.Purpose, record.ID, record.DeviceID, record.InstallationID, now)
		if err != nil {
			return err
		}
		if record.Purpose == "browser" {
			if _, err := tx.Exec(`UPDATE profile_selection_grants SET browser_binding_hash = ? WHERE token_hash = ?`, record.BrowserBindingHash, hashToken(grant.Token)); err != nil {
				return err
			}
			grant.BrowserBindingHash = record.BrowserBindingHash
		}
		result, err := tx.Exec(`UPDATE profile_account_authentications SET consumed_at = ? WHERE id = ? AND consumed_at = ''`, now.Format(time.RFC3339Nano), record.ID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return errProfileAccountAuthenticationUsed
		}
		return nil
	})
	if err == nil && pinResult != nil {
		return ProfileSelectionGrant{}, pinResult
	}
	return grant, err
}

func profileAccountAuthenticationTx(tx *sql.Tx, rawToken, provider string, now time.Time) (profileAccountAuthenticationRecord, error) {
	var record profileAccountAuthenticationRecord
	var expiresAt, consumedAt string
	err := tx.QueryRow(`
		SELECT id, account_id, auth_provider, purpose, device_id, installation_id, browser_binding_hash, expires_at, consumed_at
		FROM profile_account_authentications WHERE token_hash = ?`, hashToken(strings.TrimSpace(rawToken))).Scan(
		&record.ID, &record.AccountID, &record.AuthProvider, &record.Purpose, &record.DeviceID, &record.InstallationID, &record.BrowserBindingHash, &expiresAt, &consumedAt)
	if err != nil || record.AuthProvider != normalizeAuthProvider(provider) || record.DeviceID == "" {
		return profileAccountAuthenticationRecord{}, errInvalidProfileAccountAuthentication
	}
	if consumedAt != "" {
		return profileAccountAuthenticationRecord{}, errProfileAccountAuthenticationUsed
	}
	record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !record.ExpiresAt.After(now) {
		return profileAccountAuthenticationRecord{}, errInvalidProfileAccountAuthentication
	}
	return record, nil
}

func (s *Server) profileSelectionGrantRecord(ctx context.Context, rawToken, provider, purpose string, now time.Time) (ProfileSelectionGrant, error) {
	var record ProfileSelectionGrant
	var expiresAt, consumedAt string
	err := s.queryUserRow(ctx, `
		SELECT account_id, profile_id, auth_provider, purpose, account_authentication_id, device_id, installation_id, pin_revision, browser_binding_hash, expires_at, consumed_at
		FROM profile_selection_grants WHERE token_hash = ?`, hashToken(strings.TrimSpace(rawToken))).Scan(
		&record.AccountID, &record.ProfileID, &record.AuthProvider, &record.Purpose, &record.SourceProofID,
		&record.DeviceID, &record.InstallationID, &record.PINRevision, &record.BrowserBindingHash, &expiresAt, &consumedAt)
	if err != nil || record.AuthProvider != normalizeAuthProvider(provider) || record.Purpose != strings.TrimSpace(purpose) ||
		record.SourceProofID == "" || record.DeviceID == "" {
		return ProfileSelectionGrant{}, errInvalidProfileSelectionGrant
	}
	if consumedAt != "" {
		return ProfileSelectionGrant{}, errProfileSelectionGrantConsumed
	}
	record.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil || !record.ExpiresAt.After(now) {
		return ProfileSelectionGrant{}, errInvalidProfileSelectionGrant
	}
	record.Token = rawToken
	return record, nil
}

func (s *Server) writeProfileAuthenticationError(w http.ResponseWriter, err error) {
	var retry *profilePINRetryAfterError
	if errors.As(err, &retry) {
		seconds := int64((retry.retryAfter + time.Second - 1) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	}
	switch {
	case errors.Is(err, errProfilePINLocked):
		writeProductError(w, http.StatusTooManyRequests, "profile_temporarily_locked", "Too many incorrect PIN attempts were made. Wait until the lock expires, then try again.")
	case errors.Is(err, errProfilePINBackoff):
		writeProductError(w, http.StatusTooManyRequests, "profile_pin_retry_later", "For your security, wait a moment before trying the profile PIN again.")
	case errors.Is(err, errInvalidProfilePIN), errors.Is(err, errProfilePINNotSet):
		writeProductError(w, http.StatusBadRequest, "profile_pin_required", "Enter the four-digit PIN for this profile.")
	case errors.Is(err, errProfileNotFound):
		writeProductError(w, http.StatusNotFound, "profile_not_found", "This profile may have been removed. Choose another profile and try again.")
	case errors.Is(err, errProfileNotAllowed):
		writeProductError(w, http.StatusForbidden, "profile_not_available_on_server", "This server does not allow account profiles for this membership.")
	case errors.Is(err, errProfileAccountAuthenticationUsed), errors.Is(err, errProfileSelectionGrantConsumed):
		writeProductError(w, http.StatusConflict, "profile_selection_failed", "Portico couldn't open this profile. Choose it again or try another profile.")
	case errors.Is(err, errInvalidProfileAccountAuthentication):
		writeProductError(w, http.StatusUnauthorized, "profile_selection_required", "Your sign-in expired before a profile was selected. Sign in again to continue.")
	case errors.Is(err, errInvalidProfileSelectionGrant), errors.Is(err, errProfileAccountMismatch), errors.Is(err, errHostedProfileLocalPIN):
		writeProductError(w, http.StatusUnauthorized, "profile_selection_failed", "Portico couldn't open this profile. Choose it again or try another profile.")
	case errors.Is(err, errDeviceNotTrusted):
		writeProductError(w, http.StatusForbidden, "device_not_trusted", "This server only allows trusted devices. Ask an owner to approve this device in Settings > Devices.")
	case errors.Is(err, errDeviceNotAllowed):
		writeProductError(w, http.StatusForbidden, "device_not_allowed", "This account is not allowed to use this device.")
	case errors.Is(err, errActiveSessionLimit):
		writeProductError(w, http.StatusForbidden, "active_session_limit", "This account has reached its maximum active session limit.")
	case errors.Is(err, errAccessSchedule):
		writeProductError(w, http.StatusForbidden, "access_schedule_blocked", "This account is outside its allowed access schedule.")
	default:
		writeProductError(w, http.StatusInternalServerError, "session_failed", "Unable to create the selected profile session.")
	}
}
