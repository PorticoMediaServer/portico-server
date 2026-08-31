package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	nativeAccessTokenTTL     = 15 * time.Minute
	nativeRefreshTokenTTL    = 180 * 24 * time.Hour
	nativeExchangeReceiptTTL = 5 * time.Minute
	nativeCredentialKeySize  = 32
)

var (
	errInvalidNativeRefreshToken        = errors.New("invalid native refresh token")
	errNativeRefreshTokenReuse          = errors.New("native refresh token reuse")
	errInvalidNativeRotationKey         = errors.New("invalid native refresh rotation key")
	errNativeAccountDisabled            = errors.New("native account is disabled")
	errNativeMembershipInactive         = errors.New("native Portico membership is inactive")
	errNativeDeviceNotAllowed           = errors.New("native device is not allowed")
	errNativeSessionRevoked             = errors.New("native server session is revoked")
	errNativeAccessSchedule             = errors.New("native account access schedule is blocked")
	errNativeCredentialKeyInvalidLength = errors.New("native credential key has an invalid length")
)

type nativeDeviceDescriptor struct {
	InstallationID          string
	Name                    string
	App                     string
	Platform                string
	Trust                   bool
	ProfileSelectionGrant   string
	ProfileSelectionPurpose string
	ExchangeReceiptKind     string
	ExchangeReceiptProof    string
}

type nativeRefreshTokenRecord struct {
	ID              string
	FamilyID        string
	UserID          string
	ProfileID       string
	DeviceID        string
	AuthProvider    string
	TokenHash       string
	ReplacedByID    string
	RotationKeyHash string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	ConsumedAt      time.Time
	RevokedAt       time.Time
}

type nativeCredentialDraft struct {
	User              User
	Device            Device
	Record            nativeRefreshTokenRecord
	AccessToken       string
	AccessExpiresAt   time.Time
	ProfileIdentityID string
}

func (s *Server) handleNativeSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if s.porticoAccountMode() {
		writeError(w, http.StatusForbidden, "unsupported_auth_mode", "This server uses Portico Account sign-in. Create the native account session with Hosted Services.")
		return
	}
	var req NativeSessionCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Login) == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "credentials_required", "Login and password are required.")
		return
	}
	loginKey := loginRateKey("native-login", r, req.Login)
	if !s.allowLoginAttempt(loginKey) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts.")
		return
	}
	installationID := normalizeUntrustedNativeInstallationID(req.InstallationID)
	if strings.TrimSpace(req.DeviceName) == "" || strings.TrimSpace(req.App) == "" || strings.TrimSpace(req.Platform) == "" {
		writeError(w, http.StatusBadRequest, "device_identity_required", "A valid device name, app, and platform are required.")
		return
	}
	user, err := s.authenticateLocalNativeUser(r.Context(), req.Login, req.Password)
	if err != nil {
		if writeKDFUnavailable(w, err) {
			return
		}
		s.recordLoginFailure(loginKey)
		writeError(w, http.StatusUnauthorized, "bad_credentials", "Username/email or password is incorrect.")
		return
	}
	selectionRequired, err := s.accountRequiresExplicitProfileSelectionContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login_failed", "Unable to inspect the available profiles.")
		return
	}
	if selectionRequired {
		writeError(w, http.StatusConflict, "profile_selection_required", "Choose a profile before creating this session.")
		return
	}
	credentials, err := s.issueNativeSessionCredentials(r, user, "local", nativeDeviceDescriptor{
		InstallationID: installationID, Name: req.DeviceName, App: req.App, Platform: req.Platform,
	})
	if err != nil {
		s.writeNativeSessionCreationError(w, err)
		return
	}
	s.clearLoginFailures(loginKey)
	s.recordAudit(r, user, "native_session.created", "device", credentials.Device.ID, "info", map[string]string{"app": credentials.Device.App, "platform": credentials.Device.Platform})
	s.setNativeAccessSessionCookie(r.Context(), w, credentials)
	writeJSON(w, http.StatusCreated, credentials)
}

func (s *Server) handlePorticoSessionAttachDecrypted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !s.porticoAccountMode() {
		writeError(w, http.StatusConflict, "portico_auth_unavailable", "This server is not configured for Portico Account sign-in.")
		return
	}
	if !s.allowLoginAttempt("portico-attach:" + clientIPFromRequest(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many Portico session attachment attempts.")
		return
	}
	var req PorticoSessionAttachRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	accessToken := strings.TrimSpace(req.AccessToken)
	installationID := normalizeUntrustedNativeInstallationID(req.InstallationID)
	if !strings.HasPrefix(accessToken, "ptc_clt_") {
		writeError(w, http.StatusUnauthorized, "server_session_revoked", "The Portico authorization grant is invalid or expired.")
		return
	}
	if !validProfileDeviceDescriptor(ProfileDeviceDescriptor{
		InstallationID: req.InstallationID, DeviceName: req.DeviceName, App: req.App, Platform: req.Platform,
	}) || strings.TrimSpace(req.SelectionEnvelope.AssertionID) == "" {
		writeError(w, http.StatusBadRequest, "device_identity_required", "A valid device name, app, and platform are required.")
		return
	}
	if installationID == "" {
		var generatedErr error
		installationID, generatedErr = nativeSecureRandomID(s.nativeCredentialEntropyReader(), "install")
		if generatedErr != nil {
			writeError(w, http.StatusInternalServerError, "session_failed", "Unable to create a server device record.")
			return
		}
	}
	rawEnvelope, err := json.Marshal(req.SelectionEnvelope)
	if err != nil {
		writeError(w, http.StatusBadRequest, "profile_selection_failed", "The selected Portico profile proof is invalid.")
		return
	}
	receiptKind := "portico-attach"
	receiptProof := nativeAuthExchangeProof(receiptKind, accessToken+"\x00"+string(rawEnvelope), req.DeviceName, req.App, req.Platform)
	if recovered, recoveryErr := s.recoverNativeAuthExchangeReceipt(r.Context(), receiptKind, receiptProof); recoveryErr == nil {
		s.setNativeAccessSessionCookie(r.Context(), w, recovered)
		writeJSON(w, http.StatusCreated, recovered)
		return
	} else if !errors.Is(recoveryErr, sql.ErrNoRows) {
		s.writeNativeSessionCreationError(w, recoveryErr)
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil || settings.ServerID == "" {
		writeError(w, http.StatusConflict, "portico_auth_unavailable", "This server is not connected to Portico Hosted Services.")
		return
	}
	if strings.TrimSpace(req.SelectionEnvelope.ServerID) != strings.TrimSpace(settings.ServerID) {
		writeError(w, http.StatusUnauthorized, "profile_selection_failed", "The selected Portico profile proof is not bound to this server.")
		return
	}
	user, hostedDeviceID, err := s.porticoAttachmentForAccessToken(r.Context(), settings, accessToken, req.SelectionEnvelope)
	if err != nil {
		// Keep authentication diagnostics stage-specific without recording the
		// bootstrap token, signed envelope, installation metadata, or profile
		// identity. This lets operators distinguish a Hosted outage from an
		// authorization rejection without turning logs into credential storage.
		s.log.Warn("Portico session attachment verification failed", "stage", "hosted-introspection", "error", err)
		s.recordLog("warn", "Portico session attachment verification failed", map[string]string{"stage": "hosted-introspection", "error": err.Error()})
		var hostedErr *hostedHTTPError
		switch {
		case errors.Is(err, errNativeMembershipInactive):
			writeError(w, http.StatusForbidden, "membership_inactive", "This Portico Account no longer has access to this server.")
		case errors.As(err, &hostedErr) && hostedErr.Code == "remote_access_disabled":
			writeError(w, http.StatusForbidden, "remote_access_disabled", "Remote access is disabled for this server.")
		case errors.As(err, &hostedErr) && hostedErr.Code == "membership_inactive":
			writeError(w, http.StatusForbidden, "membership_inactive", "This Portico Account no longer has access to this server.")
		case errors.As(err, &hostedErr) && (hostedErr.StatusCode == http.StatusUnauthorized || hostedErr.StatusCode == http.StatusForbidden):
			// This is a one-time first-attachment bootstrap, not a durable
			// server session. A rejected Hosted grant must return the profile
			// selection recovery state and must never masquerade as an expired
			// signed-in session.
			writeError(w, http.StatusUnauthorized, "profile_selection_failed", "The selected Portico profile proof is invalid or expired. Choose the profile again to continue.")
		default:
			writeError(w, http.StatusServiceUnavailable, "hosted_unavailable", "Portico Hosted Services could not verify this first-time server attachment. Try again when Hosted Services is available.")
		}
		return
	}
	descriptor := ProfileDeviceDescriptor{
		InstallationID: installationID, DeviceName: req.DeviceName, App: req.App, Platform: req.Platform,
	}
	var localDeviceID string
	err = s.withUserTxTagged(r.Context(), []string{"devices"}, func(tx *sql.Tx) error {
		var deviceErr error
		localDeviceID, deviceErr = s.upsertProfileAuthenticationDeviceTx(tx, r, user.ID, descriptor, time.Now().UTC())
		return deviceErr
	})
	if err != nil {
		s.writeProfileAuthenticationError(w, err)
		return
	}
	grant, err := s.issueHostedProfileSelectionGrantContext(r.Context(), user.ID, rawEnvelope, hostedDeviceID, localDeviceID, installationID, time.Now().UTC())
	if err != nil {
		s.log.Warn("Portico session attachment verification failed", "stage", "profile-selection", "error", err)
		s.recordLog("warn", "Portico session attachment verification failed", map[string]string{"stage": "profile-selection", "error": err.Error()})
		switch {
		case errors.Is(err, errHostedProfileSelectionExchangeUnavailable):
			writeError(w, http.StatusServiceUnavailable, "hosted_unavailable", "Portico Hosted Services could not confirm this profile selection. Try again when Hosted Services is available.")
		case errors.Is(err, errProfileNotAllowed):
			writeError(w, http.StatusForbidden, "profile_not_available_on_server", "This server does not allow account profiles for this membership.")
		case errors.Is(err, errHostedProfileSelectionAssertionReplayed):
			writeError(w, http.StatusConflict, "profile_selection_failed", "This profile selection has already been used. Choose the profile again to continue.")
		default:
			writeError(w, http.StatusUnauthorized, "profile_selection_failed", "The selected Portico profile could not be verified. Choose the profile again to continue.")
		}
		return
	}
	credentials, err := s.issueNativeSessionCredentials(r, user, "portico", nativeDeviceDescriptor{
		// Hosted device identity remains a Cloud assertion binding. The durable
		// The server-local device keeps optional caller continuity metadata when supplied.
		InstallationID:          installationID,
		Name:                    req.DeviceName,
		App:                     req.App,
		Platform:                req.Platform,
		Trust:                   true,
		ProfileSelectionGrant:   grant.Token,
		ProfileSelectionPurpose: "native",
		ExchangeReceiptKind:     receiptKind,
		ExchangeReceiptProof:    receiptProof,
	})
	if err != nil {
		s.writeNativeSessionCreationError(w, err)
		return
	}
	s.clearLoginFailures("portico-attach:" + clientIPFromRequest(r))
	s.recordAudit(r, user, "portico_session.attached", "device", credentials.Device.ID, "info", map[string]string{"app": credentials.Device.App, "platform": credentials.Device.Platform})
	s.setNativeAccessSessionCookie(r.Context(), w, credentials)
	writeJSON(w, http.StatusCreated, credentials)
}

func (s *Server) handleNativeSessionRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req NativeSessionRefreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rawRefresh := strings.TrimSpace(req.RefreshToken)
	if rawRefresh == "" {
		writeError(w, http.StatusBadRequest, "refresh_token_required", "Refresh token is required.")
		return
	}
	rateKey := "native-refresh:" + clientIPFromRequest(r) + ":" + hashToken(rawRefresh)[:16]
	if !s.allowLoginAttempt(rateKey) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many session refresh attempts.")
		return
	}
	credentials, err := s.rotateNativeSessionCredentials(r, rawRefresh, strings.TrimSpace(req.RotationKey))
	if err != nil {
		s.recordLoginFailure(rateKey)
		switch {
		case errors.Is(err, errInvalidNativeRotationKey):
			writeError(w, http.StatusBadRequest, "invalid_rotation_key", "A valid refresh rotation key is required.")
		case errors.Is(err, errNativeRefreshTokenReuse):
			writeError(w, http.StatusUnauthorized, "server_session_revoked", "This server session was revoked after refresh-token reuse.")
		case errors.Is(err, errNativeAccountDisabled):
			writeError(w, http.StatusForbidden, "account_disabled", "This account is disabled on this server.")
		case errors.Is(err, errNativeMembershipInactive):
			writeError(w, http.StatusForbidden, "membership_inactive", "This Portico Account no longer has access to this server.")
		case errors.Is(err, errNativeDeviceNotAllowed):
			writeError(w, http.StatusForbidden, "device_not_allowed", "This account is not allowed to use this device.")
		case errors.Is(err, errNativeAccessSchedule):
			writeError(w, http.StatusForbidden, "access_schedule_blocked", "This account is outside its allowed access schedule.")
		case errors.Is(err, errHostedProfileAccessRevoked):
			writeProductError(w, http.StatusForbidden, "profile_not_available_on_server", "This profile can no longer use this server.")
		case errors.Is(err, errHostedProfileDirectoryUnavailable):
			w.Header().Set("Retry-After", strconv.Itoa(hostedProfileRetryAfterSeconds))
			writeProductError(w, http.StatusServiceUnavailable, "server_unavailable", "Portico could not refresh this server's profile access. Try again shortly.")
		case errors.Is(err, errNativeSessionRevoked), errors.Is(err, errInvalidNativeRefreshToken):
			writeError(w, http.StatusUnauthorized, "server_session_revoked", "This server session is invalid, expired, or revoked.")
		default:
			writeError(w, http.StatusInternalServerError, "session_refresh_failed", "Unable to refresh this server session.")
		}
		return
	}
	s.clearLoginFailures(rateKey)
	s.recordAudit(r, credentials.User, "native_session.refreshed", "device", credentials.Device.ID, "info", nil)
	s.setNativeAccessSessionCookie(r.Context(), w, credentials)
	writeJSON(w, http.StatusOK, credentials)
}

// Browsers cannot attach an Authorization header to ordinary image, media,
// stylesheet, or EventSource requests. Publish the same short-lived access
// credential used by the native session as a secure HttpOnly cookie so those
// resource requests retain the exact viewer/profile authorization boundary.
// Native clients ignore Set-Cookie and continue using bearer authentication.
func (s *Server) setNativeAccessSessionCookie(ctx context.Context, w http.ResponseWriter, credentials NativeSessionCredentials) {
	token := strings.TrimSpace(credentials.AccessToken)
	expires, err := parseCredentialTime(credentials.AccessExpiresAt)
	if token == "" || err != nil || !expires.After(time.Now().UTC()) {
		return
	}
	s.setSessionCookie(ctx, w, s.sessionCookieNameContext(ctx), token, expires)
}

func (s *Server) handleNativeSessionRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req NativeSessionRefreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rawRefresh := strings.TrimSpace(req.RefreshToken)
	if rawRefresh != "" {
		var userID, deviceID string
		_ = s.withSecurityFenceTxTagged(r.Context(), []string{"sessions", "devices"}, func(tx *sql.Tx) error {
			var familyID string
			if err := tx.QueryRow(`SELECT family_id, user_id, device_id FROM native_refresh_tokens WHERE token_hash = ?`, hashToken(rawRefresh)).Scan(&familyID, &userID, &deviceID); err != nil {
				return nil
			}
			return revokeNativeCredentialFamilyTx(tx, familyID, time.Now().UTC())
		})
		if userID != "" {
			s.recordAudit(r, User{ID: userID}, "native_session.revoked", "device", deviceID, "info", nil)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) authenticateLocalNativeUser(ctx context.Context, login, password string) (User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	var userID, passwordHash, disabledAt string
	if err := s.queryUserRow(ctx, `
		SELECT id, COALESCE(password_hash, ''), COALESCE(disabled_at, '')
		FROM users
		WHERE lower(username) = ? OR lower(email) = ?
		ORDER BY CASE WHEN lower(username) = ? THEN 0 ELSE 1 END
		LIMIT 1`, login, login, login).Scan(&userID, &passwordHash, &disabledAt); err != nil {
		_, kdfErr := verifyAccountPassword(ctx, kdfNativeLoginCompare, "", password)
		if kdfErr != nil {
			return User{}, kdfErr
		}
		return User{}, err
	}
	if disabledAt != "" {
		// Run the same expensive password verification as an active account, but
		// never upgrade a disabled account's hash or reveal the account state.
		_, kdfErr := verifyAccountPassword(ctx, kdfNativeLoginCompare, passwordHash, password)
		if kdfErr != nil {
			return User{}, kdfErr
		}
		return User{}, errors.New("invalid credentials")
	}
	valid, verifiedHash, err := s.verifyCanonicalPasswordSnapshot(ctx, kdfNativeLoginCompare, passwordHash, password)
	if err != nil {
		return User{}, err
	}
	if !valid {
		return User{}, errors.New("invalid credentials")
	}
	user, err := s.getUser(userID)
	if err != nil {
		return User{}, err
	}
	s.enrichUserAuthContext(&user, "local")
	user.verifiedPasswordHash = verifiedHash
	return user, nil
}

func (s *Server) issueNativeSessionCredentials(r *http.Request, user User, provider string, descriptor nativeDeviceDescriptor) (NativeSessionCredentials, error) {
	var draft nativeCredentialDraft
	err := s.withUserTxTagged(r.Context(), []string{"sessions", "devices", "native_auth_exchange_receipts"}, func(tx *sql.Tx) error {
		var err error
		draft, err = s.prepareNativeExchangeCredentialTx(tx, r, user, provider, descriptor, time.Now().UTC())
		if err != nil {
			return err
		}
		if err := insertNativeCredentialTx(tx, draft); err != nil {
			return err
		}
		if descriptor.ExchangeReceiptKind != "" && descriptor.ExchangeReceiptProof != "" {
			now := draft.Record.CreatedAt
			if _, err := tx.Exec(`DELETE FROM native_auth_exchange_receipts WHERE expires_at <= ?`, now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if _, err := tx.Exec(`
				INSERT INTO native_auth_exchange_receipts (kind, proof_hash, user_id, native_refresh_token_id, created_at, expires_at)
				VALUES (?, ?, ?, ?, ?, ?)`, descriptor.ExchangeReceiptKind, descriptor.ExchangeReceiptProof, draft.Record.UserID,
				draft.Record.ID, now.Format(time.RFC3339Nano), now.Add(nativeExchangeReceiptTTL).Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if descriptor.ExchangeReceiptKind != "" && descriptor.ExchangeReceiptProof != "" {
			if recovered, recoveryErr := s.recoverNativeAuthExchangeReceipt(r.Context(), descriptor.ExchangeReceiptKind, descriptor.ExchangeReceiptProof); recoveryErr == nil {
				return recovered, nil
			}
		}
		return NativeSessionCredentials{}, err
	}
	if err := s.runNativeExchangeAfterCommit(); err != nil {
		return NativeSessionCredentials{}, err
	}
	draft.Device.SessionCount = 1
	return s.nativeCredentialsResponse(r.Context(), draft.User, draft.Device, draft.Record)
}

func nativeAuthExchangeProof(kind, secret, deviceName, app, platform string) string {
	return hashToken(strings.Join([]string{
		strings.TrimSpace(kind),
		strings.TrimSpace(secret),
		truncateDeviceName(deviceName),
		truncateDeviceName(app),
		truncateDeviceName(platform),
	}, "\x00"))
}

func (s *Server) recoverNativeAuthExchangeReceipt(ctx context.Context, kind, proofHash string) (NativeSessionCredentials, error) {
	now := time.Now().UTC()
	var userID, refreshID, expiresAt string
	err := s.queryUserRow(ctx, `
		SELECT user_id, native_refresh_token_id, expires_at
		FROM native_auth_exchange_receipts
		WHERE kind = ? AND proof_hash = ? AND expires_at > ?`, strings.TrimSpace(kind), strings.TrimSpace(proofHash), now.Format(time.RFC3339Nano)).Scan(
		&userID, &refreshID, &expiresAt,
	)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	if parsed, err := parseCredentialTime(expiresAt); err != nil || !parsed.After(now) {
		return NativeSessionCredentials{}, sql.ErrNoRows
	}
	record, err := s.nativeRefreshByIDContext(ctx, refreshID)
	if err != nil {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	return s.nativeCredentialReceiptResponse(ctx, userID, record)
}

func (s *Server) prepareNativeExchangeCredentialTx(tx *sql.Tx, r *http.Request, user User, provider string, descriptor nativeDeviceDescriptor, now time.Time) (nativeCredentialDraft, error) {
	installationID := normalizeUntrustedNativeInstallationID(descriptor.InstallationID)
	var disabledAt, role, preferencesJSON, currentPasswordHash string
	var maxActiveSessions int
	if err := tx.QueryRow(`
		SELECT COALESCE(disabled_at, ''), role, preferences_json, COALESCE(max_active_sessions, 0), COALESCE(password_hash, '')
		FROM users WHERE id = ?`, user.ID).Scan(&disabledAt, &role, &preferencesJSON, &maxActiveSessions, &currentPasswordHash); err != nil {
		return nativeCredentialDraft{}, err
	}
	if disabledAt != "" {
		return nativeCredentialDraft{}, errNativeAccountDisabled
	}
	if user.verifiedPasswordHash != "" && currentPasswordHash != user.verifiedPasswordHash {
		return nativeCredentialDraft{}, errPasswordCredentialChanged
	}
	profileID := strings.TrimSpace(user.ProfileID)
	if profileID == "" {
		profileID = user.ID
	}
	deviceID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "dev")
	if err != nil {
		return nativeCredentialDraft{}, fmt.Errorf("create device id: %w", err)
	}
	if installationID == "" {
		installationID = "server:" + deviceID
	}
	name := truncateDeviceName(descriptor.Name)
	app := truncateDeviceName(descriptor.App)
	platform := truncateDeviceName(descriptor.Platform)
	trusted := descriptor.Trust || !s.requireTrustedDevicesContext(r.Context())
	device := Device{
		ID: deviceID, InstallationID: installationID, UserID: user.ID, User: user.DisplayName, Name: name, AutoName: name,
		App: app, Platform: platform, UserAgent: strings.TrimSpace(r.UserAgent()), ClientIP: clientIPFromRequest(r),
		Trusted: trusted, CreatedAt: now.Format(time.RFC3339), LastSeenAt: now.Format(time.RFC3339), Options: DeviceOptions{},
	}
	if _, err := tx.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, display_name, app, platform, user_agent, client_ip, trusted, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, installation_id) WHERE installation_id <> '' DO UPDATE SET
			name = excluded.name,
			app = excluded.app,
			platform = excluded.platform,
			user_agent = excluded.user_agent,
			client_ip = excluded.client_ip,
			last_seen_at = excluded.last_seen_at`,
		device.ID, device.UserID, device.InstallationID, device.AutoName, device.App, device.Platform, device.UserAgent, device.ClientIP,
		boolInt(device.Trusted), device.CreatedAt, device.LastSeenAt); err != nil {
		return nativeCredentialDraft{}, err
	}
	var storedTrusted int
	if err := tx.QueryRow(`
		SELECT id, trusted, COALESCE(revoked_at, ''), created_at, last_seen_at
		FROM devices WHERE user_id = ? AND installation_id = ?`, user.ID, installationID).
		Scan(&device.ID, &storedTrusted, &device.RevokedAt, &device.CreatedAt, &device.LastSeenAt); err != nil {
		return nativeCredentialDraft{}, err
	}
	device.Trusted = storedTrusted == 1
	if grant := strings.TrimSpace(descriptor.ProfileSelectionGrant); grant != "" {
		principal, err := s.consumeProfileSelectionGrantForPurposeTx(tx, grant, user.ID, provider, descriptor.ProfileSelectionPurpose, device.ID, installationID, now)
		if err != nil {
			return nativeCredentialDraft{}, err
		}
		if profileID != user.ID && profileID != principal.ProfileID {
			return nativeCredentialDraft{}, errInvalidProfileSelectionGrant
		}
		profileID = principal.ProfileID
	} else {
		if profileID != user.ID {
			return nativeCredentialDraft{}, errInvalidProfileSelectionGrant
		}
		required, err := profileRequiresSelectionGrantTx(tx, user.ID, profileID)
		if err != nil {
			return nativeCredentialDraft{}, err
		}
		if required {
			return nativeCredentialDraft{}, errInvalidProfileSelectionGrant
		}
		if _, err := resolveRequestPrincipalTx(tx, user.ID, profileID); err != nil {
			return nativeCredentialDraft{}, err
		}
	}
	selectedPrincipal, err := resolveRequestPrincipalTx(tx, user.ID, profileID)
	if err != nil {
		return nativeCredentialDraft{}, err
	}
	applyRequestPrincipal(&user, selectedPrincipal)
	if s.nativeExchangeAfterDeviceUpsert != nil {
		if err := s.nativeExchangeAfterDeviceUpsert(); err != nil {
			return nativeCredentialDraft{}, err
		}
	}
	if s.requireTrustedDevicesContext(r.Context()) && !device.Trusted {
		return nativeCredentialDraft{}, errDeviceNotTrusted
	}
	if device.RevokedAt != "" {
		return nativeCredentialDraft{}, errDeviceNotAllowed
	}
	if role != "owner" {
		policy := decodeUserDevicePolicy(preferencesJSON)
		switch policy.Mode {
		case "trusted":
			if !device.Trusted {
				return nativeCredentialDraft{}, errDeviceNotAllowed
			}
		case "allowlist":
			allowed := false
			for _, allowedID := range policy.AllowedDeviceIDs {
				if allowedID == device.ID {
					allowed = true
					break
				}
			}
			if !allowed {
				return nativeCredentialDraft{}, errDeviceNotAllowed
			}
		}
	}
	if !userAccessScheduleAllows(decodeUserAccessSchedule(preferencesJSON), now) {
		return nativeCredentialDraft{}, errAccessSchedule
	}
	if limit := normalizeMaxActiveSessions(maxActiveSessions); limit > 0 {
		var active int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ? AND expires_at > ?`, user.ID, now.Format(time.RFC3339)).Scan(&active); err != nil {
			return nativeCredentialDraft{}, err
		}
		if active >= limit {
			return nativeCredentialDraft{}, errActiveSessionLimit
		}
	}
	refreshID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "rft")
	if err != nil {
		return nativeCredentialDraft{}, fmt.Errorf("create refresh credential id: %w", err)
	}
	familyID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "rtfam")
	if err != nil {
		return nativeCredentialDraft{}, fmt.Errorf("create refresh family id: %w", err)
	}
	record := nativeRefreshTokenRecord{
		ID: refreshID, FamilyID: familyID, UserID: user.ID, ProfileID: profileID, DeviceID: device.ID,
		AuthProvider: normalizeAuthProvider(provider), CreatedAt: now, ExpiresAt: now.Add(nativeRefreshTokenTTL),
	}
	rawRefresh, err := s.nativeRefreshTokenValue(record)
	if err != nil {
		return nativeCredentialDraft{}, err
	}
	record.TokenHash = hashToken(rawRefresh)
	accessToken, accessExpiresAt, err := s.nativeAccessTokenValue(record)
	if err != nil {
		return nativeCredentialDraft{}, err
	}
	profileIdentityID := ""
	if err := tx.QueryRow(`SELECT id FROM profile_identities WHERE profile_id = ? AND provider = ? LIMIT 1`, profileID, record.AuthProvider).Scan(&profileIdentityID); err != nil {
		_ = tx.QueryRow(`SELECT id FROM profile_identities WHERE profile_id = ? ORDER BY CASE provider WHEN ? THEN 0 WHEN 'local' THEN 1 ELSE 2 END LIMIT 1`, profileID, record.AuthProvider).Scan(&profileIdentityID)
	}
	return nativeCredentialDraft{
		User: user, Device: device, Record: record, AccessToken: accessToken, AccessExpiresAt: accessExpiresAt, ProfileIdentityID: profileIdentityID,
	}, nil
}

func insertNativeCredentialTx(tx *sql.Tx, draft nativeCredentialDraft) error {
	record := draft.Record
	if _, err := tx.Exec(`
		INSERT INTO native_refresh_tokens (id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.ID, record.FamilyID, record.UserID, record.ProfileID, record.DeviceID, record.AuthProvider,
		record.TokenHash, record.CreatedAt.Format(time.RFC3339Nano), record.ExpiresAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, profile_identity_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, nativeAccessSessionID(record.ID), record.UserID, record.ProfileID, draft.ProfileIdentityID,
		record.AuthProvider, record.DeviceID, hashToken(draft.AccessToken), draft.AccessExpiresAt.Format(time.RFC3339Nano),
		record.CreatedAt.Format(time.RFC3339Nano), record.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Server) nativeCredentialReceiptResponse(ctx context.Context, expectedUserID string, record nativeRefreshTokenRecord) (NativeSessionCredentials, error) {
	now := time.Now().UTC()
	var disabledAt string
	if err := s.queryUserRow(ctx, `SELECT COALESCE(disabled_at, '') FROM users WHERE id = ?`, expectedUserID).Scan(&disabledAt); err != nil || disabledAt != "" {
		return NativeSessionCredentials{}, errNativeAccountDisabled
	}
	if record.ID == "" || record.UserID != expectedUserID || !record.ConsumedAt.IsZero() || !record.RevokedAt.IsZero() || !record.ExpiresAt.After(now) {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	accessToken, accessExpiresAt, err := s.nativeAccessTokenValue(record)
	if err != nil || !accessExpiresAt.After(now) {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	var activeSession int
	if err := s.queryUserRow(ctx, `
		SELECT COUNT(*) FROM sessions
		WHERE id = ? AND user_id = ? AND profile_id = ? AND device_id = ? AND token_hash = ? AND expires_at > ?`,
		nativeAccessSessionID(record.ID), record.UserID, record.ProfileID, record.DeviceID, hashToken(accessToken), now.Format(time.RFC3339Nano)).Scan(&activeSession); err != nil || activeSession != 1 {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	user, err := s.getUser(record.UserID)
	if err != nil || !s.userAccessScheduleAllowsNowContext(ctx, record.UserID, now) {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	principal, err := s.resolveRequestPrincipalContext(ctx, record.UserID, record.ProfileID)
	if err != nil {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	applyRequestPrincipal(&user, principal)
	device, err := s.getDevice(record.DeviceID)
	if err != nil || device.UserID != record.UserID || device.RevokedAt != "" ||
		(s.requireTrustedDevicesContext(ctx) && !device.Trusted) || !s.userDevicePolicyAllowsContext(ctx, record.UserID, record.DeviceID) {
		return NativeSessionCredentials{}, errInvalidNativeRefreshToken
	}
	device.SessionCount = 1
	return s.nativeCredentialsResponse(ctx, user, device, record)
}

func nativeExchangeReceiptRecoverable(terminalAt string, now time.Time) bool {
	parsed, err := parseCredentialTime(strings.TrimSpace(terminalAt))
	return err == nil && now.Before(parsed.Add(nativeExchangeReceiptTTL))
}

func (s *Server) runNativeExchangeAfterCommit() error {
	if s != nil && s.nativeExchangeAfterCommit != nil {
		return s.nativeExchangeAfterCommit()
	}
	return nil
}

func localNativeUserActiveTx(tx *sql.Tx, userID string) error {
	var disabledAt string
	if err := tx.QueryRow(`SELECT COALESCE(disabled_at, '') FROM users WHERE id = ?`, userID).Scan(&disabledAt); err != nil {
		return err
	}
	if disabledAt != "" {
		return errNativeAccountDisabled
	}
	return nil
}

func porticoNativeMembershipActiveTx(tx *sql.Tx, userID string) error {
	var count int
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM remote_access_members
		WHERE local_user_id = ? AND status = 'active'`, userID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return errNativeMembershipInactive
	}
	return nil
}

func (s *Server) rotateNativeSessionCredentials(r *http.Request, rawRefresh, rawRotationKey string) (NativeSessionCredentials, error) {
	now := time.Now().UTC()
	preflight, preflightErr := scanNativeRefreshToken(s.queryUserRow(r.Context(), `
		SELECT id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, replaced_by_id, rotation_key_hash,
			created_at, expires_at, consumed_at, revoked_at
		FROM native_refresh_tokens WHERE token_hash = ?`, hashToken(rawRefresh)))
	if preflightErr == nil && normalizeAuthProvider(preflight.AuthProvider) == "portico" {
		expectedRefresh, err := s.nativeRefreshTokenValue(preflight)
		if err == nil && subtle.ConstantTimeCompare([]byte(expectedRefresh), []byte(rawRefresh)) == 1 {
			if err := s.ensureHostedProfileDirectoryFreshness(r.Context(), preflight.UserID, now); err != nil {
				return NativeSessionCredentials{}, err
			}
		}
	}
	var rotated nativeRefreshTokenRecord
	var replay bool
	var terminalErr error
	err := s.withUserTxTagged(r.Context(), []string{"sessions", "devices"}, func(tx *sql.Tx) error {
		current, err := nativeRefreshByHashTx(tx, hashToken(rawRefresh))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				terminalErr = errInvalidNativeRefreshToken
				return nil
			}
			return err
		}
		expectedRefresh, err := s.nativeRefreshTokenValue(current)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(expectedRefresh), []byte(rawRefresh)) != 1 {
			terminalErr = errInvalidNativeRefreshToken
			return nil
		}
		if !current.RevokedAt.IsZero() || !current.ExpiresAt.After(now) {
			terminalErr = errInvalidNativeRefreshToken
			return nil
		}
		if err := localNativeUserActiveTx(tx, current.UserID); err != nil {
			if errors.Is(err, errNativeAccountDisabled) {
				_ = revokeNativeCredentialFamilyTx(tx, current.FamilyID, now)
				terminalErr = errNativeAccountDisabled
				return nil
			}
			return err
		}
		if normalizeAuthProvider(current.AuthProvider) == "portico" {
			if err := porticoNativeMembershipActiveTx(tx, current.UserID); err != nil {
				if errors.Is(err, errNativeMembershipInactive) {
					_ = revokeNativeCredentialFamilyTx(tx, current.FamilyID, now)
					terminalErr = errNativeMembershipInactive
					return nil
				}
				return err
			}
		}
		var deviceRevokedAt string
		if err := tx.QueryRow(`SELECT COALESCE(revoked_at, '') FROM devices WHERE id = ?`, current.DeviceID).Scan(&deviceRevokedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				_ = revokeNativeCredentialFamilyTx(tx, current.FamilyID, now)
				terminalErr = errNativeDeviceNotAllowed
				return nil
			}
			return err
		}
		if deviceRevokedAt != "" {
			_ = revokeNativeCredentialFamilyTx(tx, current.FamilyID, now)
			terminalErr = errNativeDeviceNotAllowed
			return nil
		}
		if !current.ConsumedAt.IsZero() {
			rotationMatches := validNativeRotationKey(rawRotationKey) && current.RotationKeyHash != "" &&
				subtle.ConstantTimeCompare([]byte(current.RotationKeyHash), []byte(hashToken(rawRotationKey))) == 1
			if rotationMatches && current.ReplacedByID != "" {
				candidate, candidateErr := nativeRefreshByIDTx(tx, current.ReplacedByID)
				if candidateErr == nil && candidate.FamilyID == current.FamilyID && candidate.RevokedAt.IsZero() && candidate.ExpiresAt.After(now) {
					rotated = candidate
					replay = true
					return nil
				}
			}
			if err := revokeNativeCredentialFamilyTx(tx, current.FamilyID, now); err != nil {
				return err
			}
			terminalErr = errNativeRefreshTokenReuse
			return nil
		}
		if !validNativeRotationKey(rawRotationKey) {
			terminalErr = errInvalidNativeRotationKey
			return nil
		}
		replacementID, idErr := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "rft")
		if idErr != nil {
			return fmt.Errorf("create replacement refresh credential id: %w", idErr)
		}
		rotated = nativeRefreshTokenRecord{
			ID: replacementID, FamilyID: current.FamilyID, UserID: current.UserID, ProfileID: current.ProfileID, DeviceID: current.DeviceID,
			AuthProvider: current.AuthProvider, CreatedAt: now, ExpiresAt: now.Add(nativeRefreshTokenTTL),
		}
		rawRotated, err := s.nativeRefreshTokenValue(rotated)
		if err != nil {
			return err
		}
		rotated.TokenHash = hashToken(rawRotated)
		if _, err := tx.Exec(`
			INSERT INTO native_refresh_tokens (id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, rotated.ID, rotated.FamilyID, rotated.UserID, rotated.ProfileID, rotated.DeviceID, rotated.AuthProvider,
			rotated.TokenHash, rotated.CreatedAt.Format(time.RFC3339Nano), rotated.ExpiresAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE native_refresh_tokens SET consumed_at = ?, replaced_by_id = ?, rotation_key_hash = ? WHERE id = ?`, now.Format(time.RFC3339Nano), rotated.ID, hashToken(rawRotationKey), current.ID)
		return err
	})
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	if terminalErr != nil {
		return NativeSessionCredentials{}, terminalErr
	}
	user, err := s.getUser(rotated.UserID)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	principal, err := s.resolveRequestPrincipalContext(r.Context(), rotated.UserID, rotated.ProfileID)
	if err != nil {
		_ = s.revokeNativeCredentialFamily(r.Context(), rotated.FamilyID, now)
		return NativeSessionCredentials{}, err
	}
	applyRequestPrincipal(&user, principal)
	if !s.userAccessScheduleAllowsNowContext(r.Context(), rotated.UserID, now) {
		return NativeSessionCredentials{}, errNativeAccessSchedule
	}
	device, err := s.getDevice(rotated.DeviceID)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	if device.RevokedAt != "" || (s.requireTrustedDevicesContext(r.Context()) && !device.Trusted) || !s.userDevicePolicyAllowsContext(r.Context(), rotated.UserID, rotated.DeviceID) {
		_ = s.revokeNativeCredentialFamily(r.Context(), rotated.FamilyID, now)
		return NativeSessionCredentials{}, errNativeDeviceNotAllowed
	}
	if err := s.ensureNativeAccessSession(r.Context(), user, rotated); err != nil {
		if replay && strings.Contains(strings.ToLower(err.Error()), "unique") {
			// The first concurrent refresh already persisted the deterministic access row.
		} else {
			return NativeSessionCredentials{}, err
		}
	}
	device.SessionCount = 1
	return s.nativeCredentialsResponse(r.Context(), user, device, rotated)
}

func (s *Server) ensureNativeAccessSession(ctx context.Context, user User, record nativeRefreshTokenRecord) error {
	accessToken, accessExpiry, err := s.nativeAccessTokenValue(record)
	if err != nil {
		return err
	}
	profileIdentityID := s.profileIdentityIDForProviderContext(ctx, record.UserID, record.AuthProvider)
	_, err = s.execUserWrite(ctx, `
		INSERT OR IGNORE INTO sessions (id, user_id, profile_id, profile_identity_id, auth_provider, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, nativeAccessSessionID(record.ID), record.UserID, record.ProfileID, profileIdentityID,
		record.AuthProvider, record.DeviceID, hashToken(accessToken), accessExpiry.Format(time.RFC3339Nano),
		record.CreatedAt.Format(time.RFC3339Nano), record.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Server) nativeCredentialsResponse(ctx context.Context, user User, device Device, record nativeRefreshTokenRecord) (NativeSessionCredentials, error) {
	rawAccess, accessExpiry, err := s.nativeAccessTokenValue(record)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	rawRefresh, err := s.nativeRefreshTokenValue(record)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	settings, _ := s.loadSettingsContext(ctx)
	serverID, _ := s.publicServerIDForAuthProviderContext(ctx, settings, record.AuthProvider)
	publicAccountID, publicProfileID, err := s.publicViewerIdentityForUserContext(ctx, user, record.AuthProvider)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	serverIdentity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	publicUser := user
	publicUser.ProfileID = publicProfileID
	return NativeSessionCredentials{
		TokenType: "Bearer", AccessToken: rawAccess, AccessExpiresAt: accessExpiry.Format(time.RFC3339),
		RefreshToken: rawRefresh, RefreshExpiresAt: record.ExpiresAt.Format(time.RFC3339), User: publicUser, Device: device,
		Authority: viewerAuthorityForAuthProvider(record.AuthProvider),
		AccountID: publicAccountID, ProfileID: publicProfileID, AuthorizationRevision: s.authorizationRevisionForUserContext(ctx, user),
		ServerID: serverID, ServerFriendlyName: serverFriendlyNameFromSettings(settings),
		ServerPublicKey:            base64.RawStdEncoding.EncodeToString(serverIdentity.PublicKey),
		ServerPublicKeyFingerprint: serverIdentity.Fingerprint,
	}, nil
}

func (s *Server) nativeRefreshTokenValue(record nativeRefreshTokenRecord) (string, error) {
	key, err := s.nativeCredentialHMACKey()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	for _, value := range []string{"local-refresh-token-v2", record.ID, record.FamilyID, record.UserID, record.ProfileID, record.DeviceID, record.AuthProvider} {
		_, _ = mac.Write([]byte(value))
		_, _ = mac.Write([]byte{'\n'})
	}
	return "ptc_lrf_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Server) nativeAccessTokenValue(record nativeRefreshTokenRecord) (string, time.Time, error) {
	key, err := s.nativeCredentialHMACKey()
	if err != nil {
		return "", time.Time{}, err
	}
	mac := hmac.New(sha256.New, key)
	for _, value := range []string{"local-access-token-v2", record.ID, record.FamilyID, record.UserID, record.ProfileID, record.DeviceID, record.AuthProvider} {
		_, _ = mac.Write([]byte(value))
		_, _ = mac.Write([]byte{'\n'})
	}
	expires := record.CreatedAt.Add(nativeAccessTokenTTL)
	if record.ExpiresAt.Before(expires) {
		expires = record.ExpiresAt
	}
	return "ptc_loc_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), expires, nil
}

func (s *Server) nativeCredentialEntropyReader() io.Reader {
	if s != nil && s.nativeCredentialEntropy != nil {
		return s.nativeCredentialEntropy
	}
	return rand.Reader
}

func nativeSecureRandomID(reader io.Reader, prefix string) (string, error) {
	bytes := make([]byte, 12)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomNativeCredentialToken(reader io.Reader) (string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(reader, bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func normalizeNativeInstallationID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 16 || len(value) > 128 {
		return "", false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("._:-", char) {
			continue
		}
		return "", false
	}
	return value, true
}

func normalizeOptionalNativeInstallationID(value string) (string, bool) {
	if strings.TrimSpace(value) == "" {
		return "", true
	}
	return normalizeNativeInstallationID(value)
}

// Installation IDs are optional continuity hints supplied by an untrusted
// client. They are never authentication proof. Malformed or rotated values are
// discarded; server-issued tokens, grants, device records, and cryptographic
// setup proofs remain the authority for every session operation.
func normalizeUntrustedNativeInstallationID(value string) string {
	normalized, ok := normalizeOptionalNativeInstallationID(value)
	if !ok {
		return ""
	}
	return normalized
}

func (s *Server) nativeCredentialHMACKey() ([]byte, error) {
	dir := filepath.Join(s.cfg.AppDataDir, "secrets")
	path := filepath.Join(dir, "native-session-hmac.key")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create native credential key directory: %w", err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect native credential key directory: %w", err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 || !dirInfo.IsDir() {
		return nil, errors.New("native credential key directory must be a real directory")
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("native credential key directory must not be accessible by group or others")
	}
	if err := validateNativeCredentialOwner(dirInfo); err != nil {
		return nil, err
	}
	if key, err := readNativeCredentialKey(path); err == nil {
		return key, nil
	} else if !os.IsNotExist(err) {
		if !errors.Is(err, errNativeCredentialKeyInvalidLength) {
			return nil, err
		}
		// A concurrent first creator exposes the path before its write has
		// completed. Treat only the invalid-length state as transient; all
		// other invalid filesystem shapes remain fail-closed immediately.
		if key, waitErr := waitForNativeCredentialKey(path, 250*time.Millisecond); waitErr == nil {
			return key, nil
		}
		return nil, err
	}
	key := make([]byte, nativeCredentialKeySize)
	if _, err := io.ReadFull(s.nativeCredentialEntropyReader(), key); err != nil {
		return nil, fmt.Errorf("generate native credential key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			// O_EXCL publishes the path before the creator has finished writing
			// it. Wait in a bounded window for the creator to finish; never
			// accept or replace an invalid key.
			return waitForNativeCredentialKey(path, 2*time.Second)
		}
		return nil, fmt.Errorf("create native credential key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write native credential key: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close native credential key: %w", err)
	}
	pathInfo, pathErr := os.Lstat(path)
	dirAfter, dirErr := os.Lstat(dir)
	if pathErr != nil || dirErr != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || pathInfo.Mode().Perm()&0o077 != 0 || !os.SameFile(dirInfo, dirAfter) {
		_ = os.Remove(path)
		return nil, errors.New("created native credential key failed filesystem safety validation")
	}
	if err := validateNativeCredentialOwner(pathInfo); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return key, nil
}

func readNativeCredentialKey(path string) ([]byte, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("native credential key must not be a symlink")
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, errors.New("native credential key must be a regular file")
	}
	if pathInfo.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("native credential key must not be accessible by group or others")
	}
	if err := validateNativeCredentialOwner(pathInfo); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open native credential key: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, openedInfo) {
		return nil, errors.New("native credential key changed while opening")
	}
	key, err := io.ReadAll(io.LimitReader(file, nativeCredentialKeySize+1))
	if err != nil {
		return nil, fmt.Errorf("read native credential key: %w", err)
	}
	if len(key) != nativeCredentialKeySize {
		return nil, errNativeCredentialKeyInvalidLength
	}
	return key, nil
}

func waitForNativeCredentialKey(path string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		key, err := readNativeCredentialKey(path)
		if err == nil {
			return key, nil
		}
		lastErr = err
		if !os.IsNotExist(err) && !errors.Is(err, errNativeCredentialKeyInvalidLength) {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errNativeCredentialKeyInvalidLength
	}
	return nil, lastErr
}

func nativeRefreshByHashTx(tx *sql.Tx, tokenHash string) (nativeRefreshTokenRecord, error) {
	return scanNativeRefreshToken(tx.QueryRow(`
		SELECT id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, replaced_by_id, rotation_key_hash,
			created_at, expires_at, consumed_at, revoked_at
		FROM native_refresh_tokens WHERE token_hash = ?`, tokenHash))
}

func nativeRefreshByIDTx(tx *sql.Tx, id string) (nativeRefreshTokenRecord, error) {
	return scanNativeRefreshToken(tx.QueryRow(`
		SELECT id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, replaced_by_id, rotation_key_hash,
			created_at, expires_at, consumed_at, revoked_at
		FROM native_refresh_tokens WHERE id = ?`, id))
}

func (s *Server) nativeRefreshByIDContext(ctx context.Context, id string) (nativeRefreshTokenRecord, error) {
	return scanNativeRefreshToken(s.queryUserRow(ctx, `
		SELECT id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, replaced_by_id, rotation_key_hash,
			created_at, expires_at, consumed_at, revoked_at
		FROM native_refresh_tokens WHERE id = ?`, id))
}

type nativeRefreshRow interface {
	Scan(...any) error
}

func scanNativeRefreshToken(row nativeRefreshRow) (nativeRefreshTokenRecord, error) {
	var record nativeRefreshTokenRecord
	var createdAt, expiresAt, consumedAt, revokedAt string
	if err := row.Scan(&record.ID, &record.FamilyID, &record.UserID, &record.ProfileID, &record.DeviceID, &record.AuthProvider, &record.TokenHash,
		&record.ReplacedByID, &record.RotationKeyHash, &createdAt, &expiresAt, &consumedAt, &revokedAt); err != nil {
		return nativeRefreshTokenRecord{}, err
	}
	var err error
	record.CreatedAt, err = parseCredentialTime(createdAt)
	if err != nil {
		return nativeRefreshTokenRecord{}, err
	}
	record.ExpiresAt, err = parseCredentialTime(expiresAt)
	if err != nil {
		return nativeRefreshTokenRecord{}, err
	}
	if consumedAt != "" {
		record.ConsumedAt, _ = parseCredentialTime(consumedAt)
	}
	if revokedAt != "" {
		record.RevokedAt, _ = parseCredentialTime(revokedAt)
	}
	return record, nil
}

func validNativeRotationKey(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) >= 32
}

func parseCredentialTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Parse(time.RFC3339, value)
	}
	return parsed, nil
}

func revokeNativeCredentialFamilyTx(tx *sql.Tx, familyID string, now time.Time) error {
	timestamp := now.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`UPDATE native_refresh_tokens SET revoked_at = CASE WHEN revoked_at = '' THEN ? ELSE revoked_at END WHERE family_id = ?`, timestamp, familyID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM sessions WHERE id IN (SELECT 'nativesess_' || id FROM native_refresh_tokens WHERE family_id = ?)`, familyID)
	return err
}

func (s *Server) revokeNativeCredentialFamily(ctx context.Context, familyID string, now time.Time) error {
	return s.withSecurityFenceTxTagged(ctx, []string{"sessions", "devices"}, func(tx *sql.Tx) error {
		return revokeNativeCredentialFamilyTx(tx, familyID, now)
	})
}

func nativeAccessSessionID(refreshTokenID string) string {
	return "nativesess_" + refreshTokenID
}

func (s *Server) writeNativeSessionCreationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPasswordCredentialChanged):
		writeError(w, http.StatusUnauthorized, "credentials_changed", "The account password changed while Portico was signing in. Sign in again.")
	case errors.Is(err, errDeviceNotTrusted):
		writeError(w, http.StatusForbidden, "device_not_trusted", "This server only allows trusted devices. Ask an owner to approve this device in Settings > Devices.")
	case errors.Is(err, errDeviceNotAllowed):
		writeError(w, http.StatusForbidden, "device_not_allowed", "This account is not allowed to use this device.")
	case errors.Is(err, errActiveSessionLimit):
		writeError(w, http.StatusForbidden, "active_session_limit", "This account has reached its maximum active session limit.")
	case errors.Is(err, errAccessSchedule):
		writeError(w, http.StatusForbidden, "access_schedule_blocked", "This account is outside its allowed access schedule.")
	case errors.Is(err, errNativeAccountDisabled):
		writeError(w, http.StatusForbidden, "account_disabled", "This account is disabled.")
	default:
		writeError(w, http.StatusInternalServerError, "session_failed", "Unable to create native session.")
	}
}
