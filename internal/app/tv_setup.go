package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

const (
	tvSetupServiceType         = "_portico-setup._tcp.local."
	tvSetupProtocol            = 1
	tvSetupSessionTTL          = 2 * time.Minute
	tvSetupGrantTTL            = 2 * time.Minute
	tvSetupCodeAttempts        = 10
	tvSetupGrantInfo           = "Portico Nearby TV Setup Grant v1"
	tvSetupGrantKeyBytes       = 32
	tvSetupCodeAlphabet        = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	tvSetupNormalizedCodeSize  = 8
	tvSetupMaxActiveTotal      = 5000
	tvSetupMaxActivePerIP      = 12
	tvSetupMaxActivePerInstall = 3
	tvSetupTerminalRetention   = 24 * time.Hour
)

type tvSetupSessionRecord struct {
	ID                   string
	InstallationID       string
	Code                 string
	Status               string
	UserID               string
	DevicePublicKey      string
	GrantSecretHash      string
	EncryptedGrantJSON   string
	DeviceName           string
	Platform             string
	AppVersion           string
	ServerHint           string
	AuthModeHint         string
	EndpointURL          string
	ClientIP             string
	UserAgent            string
	NativeRefreshTokenID string
	ExpiresAt            string
	RedeemedAt           string
	CreatedAt            string
	UpdatedAt            string
}

var (
	errTVSetupGrantInvalid  = errors.New("TV setup grant is invalid")
	errTVSetupNotAuthorized = errors.New("TV setup session is not authorized")
	errTVSetupGrantExpired  = errors.New("TV setup grant has expired")
	errTVSetupGrantUsed     = errors.New("TV setup grant has already been used")
	errTVSetupAdmissionBusy = errors.New("TV setup admission capacity is exhausted")
	errTVSetupDeviceBusy    = errors.New("too many active TV setup sessions for this device")
)

type tvSetupGrantPayload struct {
	SetupSessionID string `json:"setupSessionId"`
	GrantSecret    string `json:"grantSecret"`
	UserID         string `json:"userId"`
	AuthProvider   string `json:"authProvider"`
	IssuedAt       string `json:"issuedAt"`
	ExpiresAt      string `json:"expiresAt"`
}

func (s *Server) handleTVSetupSessions(w http.ResponseWriter, r *http.Request) {
	quickConnectNoStore(w)
	switch r.Method {
	case http.MethodPost:
		var req TVSetupSessionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if _, err := decodeTVSetupPublicKey(req.DevicePublicKey); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_device_public_key", "A valid TV setup public key is required.")
			return
		}
		installationID := normalizeUntrustedNativeInstallationID(req.InstallationID)
		if !s.allowTVSetupPublicRequest(w, r, "start", installationID) {
			return
		}
		setupSessionID, err := nativeSecureRandomID(s.nativeCredentialEntropyReader(), "psu")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tv_setup_entropy_unavailable", "Unable to securely create a TV setup session.")
			return
		}
		if installationID == "" {
			installationID = "server:" + setupSessionID
		}
		now := time.Now().UTC()
		session := tvSetupSessionRecord{
			ID:              setupSessionID,
			InstallationID:  installationID,
			Status:          "pending",
			DevicePublicKey: strings.TrimSpace(req.DevicePublicKey),
			DeviceName:      cleanTVSetupLabel(req.DeviceName, "Portico TV"),
			Platform:        cleanTVSetupLabel(req.Platform, "tv"),
			AppVersion:      cleanTVSetupLabel(req.AppVersion, ""),
			ServerHint:      cleanTVSetupLabel(req.ServerHint, ""),
			AuthModeHint:    s.tvSetupAuthModeHint(req.AuthModeHint),
			EndpointURL:     cleanTVSetupEndpoint(req.EndpointURL),
			ClientIP:        clientIPFromRequest(r),
			UserAgent:       strings.TrimSpace(r.UserAgent()),
			ExpiresAt:       now.Add(tvSetupSessionTTL).Format(time.RFC3339),
			CreatedAt:       now.Format(time.RFC3339),
			UpdatedAt:       now.Format(time.RFC3339),
		}
		created := false
		for attempt := 0; attempt < tvSetupCodeAttempts; attempt++ {
			setupCode, err := randomTVSetupCode(s.nativeCredentialEntropyReader())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "tv_setup_entropy_unavailable", "Unable to securely create a TV setup session.")
				return
			}
			session.Code = setupCode
			if err := s.createTVSetupSessionContext(r.Context(), session); err == nil {
				created = true
				break
			} else if errors.Is(err, errTVSetupDeviceBusy) {
				w.Header().Set("Retry-After", "60")
				writeError(w, http.StatusTooManyRequests, "tv_setup_device_busy", "Too many TV setup sessions are already active for this device.")
				return
			} else if errors.Is(err, errTVSetupAdmissionBusy) {
				w.Header().Set("Retry-After", "30")
				writeError(w, http.StatusServiceUnavailable, "tv_setup_capacity_reached", "TV setup is temporarily at capacity.")
				return
			} else if !isTVSetupCodeConflict(err) {
				writeError(w, http.StatusInternalServerError, "tv_setup_session_failed", "Unable to create TV setup session.")
				return
			}
		}
		if !created {
			w.Header().Set("Retry-After", "2")
			writeError(w, http.StatusServiceUnavailable, "tv_setup_unavailable", "TV setup is temporarily unavailable.")
			return
		}
		s.recordLog("info", "Nearby TV Setup session created", map[string]string{"setupSessionId": session.ID, "device": session.DeviceName, "platform": session.Platform})
		writeJSON(w, http.StatusCreated, tvSetupSessionResponse(session, nil))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
	}
}

func (s *Server) handleTVSetupSessionRoute(w http.ResponseWriter, r *http.Request) {
	quickConnectNoStore(w)
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	setupSessionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auth/tv-setup/sessions/"), "/")
	if setupSessionID == "" {
		writeError(w, http.StatusNotFound, "tv_setup_session_not_found", "TV setup session was not found.")
		return
	}
	if !s.allowTVSetupPublicRequest(w, r, "status", setupSessionID) {
		return
	}
	session, err := s.tvSetupSessionContext(r.Context(), setupSessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "tv_setup_session_not_found", "TV setup session was not found.")
		return
	}
	session = s.expireTVSetupSessionIfNeeded(r, session)
	encryptedGrant, _ := decodeTVSetupEncryptedGrant(session.EncryptedGrantJSON)
	writeJSON(w, http.StatusOK, tvSetupSessionResponse(session, encryptedGrant))
}

func (s *Server) handleTVSetupGrant(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !s.userCanApproveQuickConnect(user) {
		writeError(w, http.StatusForbidden, "tv_setup_approval_denied", "This account cannot approve TV setup requests.")
		return
	}
	var req TVSetupGrantRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	setupSessionID := strings.TrimSpace(req.SetupSessionID)
	session, err := s.tvSetupSessionContext(r.Context(), setupSessionID)
	if err != nil && setupSessionID == "" {
		session, err = s.tvSetupSessionByCodeContext(r.Context(), req.Code)
		setupSessionID = session.ID
	}
	if err != nil {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", setupSessionID, "warn", map[string]string{"reason": "not_found"})
		writeError(w, http.StatusNotFound, "tv_setup_session_not_found", "TV setup session was not found.")
		return
	}
	session = s.expireTVSetupSessionIfNeeded(r, session)
	if session.Status != "pending" {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": session.Status})
		writeError(w, http.StatusConflict, "tv_setup_not_pending", "TV setup session is no longer pending.")
		return
	}
	if !tvSetupCodeMatchesSession(req.Code, session.Code) {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": "code_mismatch", "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusForbidden, "tv_setup_code_mismatch", "The setup code does not match this TV.")
		return
	}
	if req.DevicePublicKey != "" && strings.TrimSpace(req.DevicePublicKey) != session.DevicePublicKey {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": "public_key_mismatch", "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusForbidden, "tv_setup_device_mismatch", "The selected TV setup session changed. Start setup again.")
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": "expired", "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusGone, "tv_setup_expired", "TV setup session has expired.")
		return
	}

	now := time.Now().UTC()
	grantExpires := now.Add(tvSetupGrantTTL)
	if expiresAt.Before(grantExpires) {
		grantExpires = expiresAt
	}
	grantSecret, err := randomNativeCredentialToken(s.nativeCredentialEntropyReader())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tv_setup_entropy_unavailable", "Unable to securely create a TV setup grant.")
		return
	}
	payload := tvSetupGrantPayload{
		SetupSessionID: session.ID,
		GrantSecret:    grantSecret,
		UserID:         user.ID,
		AuthProvider:   normalizeAuthProvider(user.AuthProvider),
		IssuedAt:       now.Format(time.RFC3339),
		ExpiresAt:      grantExpires.Format(time.RFC3339),
	}
	encryptedGrant, err := encryptTVSetupGrant(session.DevicePublicKey, session.ID, payload)
	if err != nil {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": "encrypt_failed", "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusBadRequest, "tv_setup_encrypt_failed", "Unable to encrypt setup grant for this TV.")
		return
	}
	encryptedGrantBytes, err := json.Marshal(encryptedGrant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tv_setup_grant_failed", "Unable to create TV setup grant.")
		return
	}
	if err := s.authorizeTVSetupSessionContext(r.Context(), session.ID, user.ID, hashToken(grantSecret), string(encryptedGrantBytes), grantExpires); err != nil {
		s.recordAudit(r, user, "tv_setup.grant_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": err.Error(), "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusConflict, "tv_setup_not_pending", "TV setup session could not be authorized.")
		return
	}
	s.recordAudit(r, user, "tv_setup.grant_created", "tv_setup", session.ID, "info", map[string]string{"device": session.DeviceName, "platform": session.Platform})
	writeJSON(w, http.StatusCreated, TVSetupGrantResponse{
		SetupSessionID: session.ID,
		Status:         "grant_ready",
		EncryptedGrant: encryptedGrant,
		ExpiresAt:      grantExpires.Format(time.RFC3339),
	})
}

func (s *Server) handleTVSetupRedeem(w http.ResponseWriter, r *http.Request) {
	quickConnectNoStore(w)
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req TVSetupRedeemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	setupSessionID := strings.TrimSpace(req.SetupSessionID)
	if !s.allowTVSetupPublicRequest(w, r, "redeem", setupSessionID) {
		return
	}
	session, err := s.tvSetupSessionContext(r.Context(), setupSessionID)
	if err != nil {
		s.recordAudit(r, User{}, "tv_setup.redeem_failed", "tv_setup", setupSessionID, "warn", map[string]string{"reason": "not_found"})
		writeError(w, http.StatusNotFound, "tv_setup_session_not_found", "TV setup session was not found.")
		return
	}
	session = s.expireTVSetupSessionIfNeeded(r, session)
	if (session.Status != "grant_ready" && session.Status != "redeemed") || strings.TrimSpace(session.UserID) == "" {
		s.recordAudit(r, User{ID: session.UserID}, "tv_setup.redeem_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": session.Status, "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusForbidden, "tv_setup_not_authorized", "TV setup session is not authorized.")
		return
	}
	if hashToken(strings.TrimSpace(req.GrantSecret)) != session.GrantSecretHash {
		s.recordAudit(r, User{ID: session.UserID}, "tv_setup.redeem_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": "grant_mismatch", "device": session.DeviceName, "platform": session.Platform})
		writeError(w, http.StatusForbidden, "tv_setup_grant_invalid", "TV setup grant is invalid.")
		return
	}
	credentials, err := s.redeemTVSetupCredentials(r, session, req.GrantSecret)
	if err != nil {
		s.recordAudit(r, User{ID: session.UserID}, "tv_setup.redeem_failed", "tv_setup", session.ID, "warn", map[string]string{"reason": err.Error(), "device": session.DeviceName, "platform": session.Platform})
		if errors.Is(err, errDeviceNotTrusted) || errors.Is(err, errDeviceNotAllowed) || errors.Is(err, errActiveSessionLimit) || errors.Is(err, errAccessSchedule) || errors.Is(err, errNativeAccountDisabled) {
			s.writeNativeSessionCreationError(w, err)
		} else if errors.Is(err, errTVSetupGrantInvalid) {
			writeError(w, http.StatusForbidden, "tv_setup_grant_invalid", "TV setup grant is invalid.")
		} else if errors.Is(err, errTVSetupGrantExpired) || errors.Is(err, errTVSetupGrantUsed) || errors.Is(err, errTVSetupNotAuthorized) {
			writeError(w, http.StatusForbidden, "tv_setup_grant_used", "TV setup can no longer be finished with this grant.")
		} else if errors.Is(err, errInvalidNativeRefreshToken) {
			writeError(w, http.StatusForbidden, "tv_setup_receipt_unavailable", "This TV setup exchange can no longer be recovered.")
		} else {
			writeError(w, http.StatusInternalServerError, "tv_setup_redeem_failed", "Unable to finish TV setup.")
		}
		return
	}
	s.recordAudit(r, credentials.User, "tv_setup.redeemed", "tv_setup", session.ID, "info", map[string]string{"device": session.DeviceName, "platform": session.Platform})
	writeJSON(w, http.StatusOK, credentials)
}

func validateTVSetupExchangeState(session tvSetupSessionRecord, now time.Time) error {
	if session.Status == "redeemed" {
		if session.NativeRefreshTokenID != "" && nativeExchangeReceiptRecoverable(session.RedeemedAt, now) {
			return nil
		}
		return errTVSetupGrantUsed
	}
	expiresAt, err := parseCredentialTime(session.ExpiresAt)
	if err != nil || !expiresAt.After(now) {
		return errTVSetupGrantExpired
	}
	if session.Status != "grant_ready" || strings.TrimSpace(session.UserID) == "" {
		return errTVSetupNotAuthorized
	}
	return nil
}

func (s *Server) redeemTVSetupCredentials(r *http.Request, session tvSetupSessionRecord, grantSecret string) (NativeSessionCredentials, error) {
	ctx := r.Context()
	now := time.Now().UTC()
	if hashToken(strings.TrimSpace(grantSecret)) != session.GrantSecretHash {
		return NativeSessionCredentials{}, errTVSetupGrantInvalid
	}
	if err := validateTVSetupExchangeState(session, now); err != nil {
		return NativeSessionCredentials{}, err
	}
	if session.Status == "redeemed" {
		record, err := s.nativeRefreshByIDContext(ctx, session.NativeRefreshTokenID)
		if err != nil {
			return NativeSessionCredentials{}, errInvalidNativeRefreshToken
		}
		return s.nativeCredentialReceiptResponse(ctx, session.UserID, record)
	}
	user, err := s.getUser(session.UserID)
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	var draft nativeCredentialDraft
	created := false
	err = s.withUserTxTagged(ctx, []string{"tv_setup_sessions", "sessions", "devices"}, func(tx *sql.Tx) error {
		var current tvSetupSessionRecord
		current.ID = session.ID
		if err := tx.QueryRow(`
			SELECT status, user_id, grant_secret_hash, expires_at, redeemed_at, native_refresh_token_id
			FROM tv_setup_sessions WHERE id = ?`, session.ID).Scan(
			&current.Status, &current.UserID, &current.GrantSecretHash, &current.ExpiresAt, &current.RedeemedAt, &current.NativeRefreshTokenID,
		); err != nil {
			return errTVSetupNotAuthorized
		}
		if current.GrantSecretHash != hashToken(strings.TrimSpace(grantSecret)) {
			return errTVSetupGrantInvalid
		}
		if err := validateTVSetupExchangeState(current, now); err != nil {
			return err
		}
		session.Status, session.UserID, session.ExpiresAt = current.Status, current.UserID, current.ExpiresAt
		session.RedeemedAt, session.NativeRefreshTokenID = current.RedeemedAt, current.NativeRefreshTokenID
		if current.Status == "redeemed" {
			if err := localNativeUserActiveTx(tx, current.UserID); err != nil {
				return err
			}
			draft.Record, err = nativeRefreshByIDTx(tx, current.NativeRefreshTokenID)
			return err
		}
		draft, err = s.prepareNativeExchangeCredentialTx(tx, r, user, "local", nativeDeviceDescriptor{
			InstallationID: session.InstallationID, Name: session.DeviceName, App: "Portico TV", Platform: session.Platform, Trust: true,
		}, now)
		if err != nil {
			return err
		}
		terminalAt := now.Format(time.RFC3339Nano)
		update, err := tx.Exec(`
			UPDATE tv_setup_sessions
			SET status = 'redeemed', redeemed_at = ?, updated_at = ?, native_refresh_token_id = ?
			WHERE id = ? AND status = 'grant_ready' AND native_refresh_token_id = '' AND grant_secret_hash = ? AND expires_at > ?`,
			terminalAt, terminalAt, draft.Record.ID, current.ID, current.GrantSecretHash, now.Format(time.RFC3339))
		if err != nil {
			return err
		}
		if affected, err := update.RowsAffected(); err != nil || affected != 1 {
			return errTVSetupGrantUsed
		}
		if err := insertNativeCredentialTx(tx, draft); err != nil {
			return err
		}
		session.Status, session.RedeemedAt, session.NativeRefreshTokenID = "redeemed", terminalAt, draft.Record.ID
		created = true
		return nil
	})
	if err != nil {
		return NativeSessionCredentials{}, err
	}
	if created {
		if err := s.runNativeExchangeAfterCommit(); err != nil {
			return NativeSessionCredentials{}, err
		}
	}
	return s.nativeCredentialReceiptResponse(ctx, session.UserID, draft.Record)
}

func (s *Server) createTVSetupSession(session tvSetupSessionRecord) error {
	return s.createTVSetupSessionContext(context.Background(), session)
}

func (s *Server) createTVSetupSessionContext(ctx context.Context, session tvSetupSessionRecord) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return s.withUserTxTagged(ctx, []string{"tv_setup_sessions"}, func(tx *sql.Tx) error {
		if err := pruneTVSetupSessionsTx(ctx, tx, time.Now().UTC()); err != nil {
			return err
		}
		var activeTotal, activeInstall, activeIP int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_setup_sessions WHERE status IN ('pending', 'grant_ready') AND expires_at > ?`, now).Scan(&activeTotal); err != nil {
			return err
		}
		if activeTotal >= tvSetupMaxActiveTotal {
			return errTVSetupAdmissionBusy
		}
		if session.InstallationID != "" {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_setup_sessions WHERE installation_id = ? AND status IN ('pending', 'grant_ready') AND expires_at > ?`, session.InstallationID, now).Scan(&activeInstall); err != nil {
				return err
			}
		}
		if session.ClientIP != "" {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tv_setup_sessions WHERE client_ip = ? AND status IN ('pending', 'grant_ready') AND expires_at > ?`, session.ClientIP, now).Scan(&activeIP); err != nil {
				return err
			}
		}
		if activeInstall >= tvSetupMaxActivePerInstall || activeIP >= tvSetupMaxActivePerIP {
			return errTVSetupDeviceBusy
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tv_setup_sessions
			SET status = 'expired', updated_at = ?
			WHERE code = ? AND status IN ('pending', 'grant_ready') AND expires_at <= ?`,
			now, session.Code, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO tv_setup_sessions (
				id, code, status, installation_id, device_public_key, device_name, platform, app_version,
				server_hint, auth_mode_hint, endpoint_url, client_ip, user_agent,
				expires_at, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			session.ID, session.Code, session.Status, session.InstallationID, session.DevicePublicKey, session.DeviceName, session.Platform, session.AppVersion,
			session.ServerHint, session.AuthModeHint, session.EndpointURL, session.ClientIP, session.UserAgent,
			session.ExpiresAt, session.CreatedAt, session.UpdatedAt)
		return err
	})
}

func (s *Server) pruneTVSetupSessionsContext(ctx context.Context, now time.Time) error {
	return s.withUserTxTagged(ctx, []string{"tv_setup_sessions"}, func(tx *sql.Tx) error {
		return pruneTVSetupSessionsTx(ctx, tx, now)
	})
}

func pruneTVSetupSessionsTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		UPDATE tv_setup_sessions SET status = 'expired', updated_at = ?
		WHERE status IN ('pending', 'grant_ready') AND expires_at <= ?`, nowText, nowText); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM tv_setup_sessions WHERE expires_at < ?`, now.UTC().Add(-tvSetupTerminalRetention).Format(time.RFC3339))
	return err
}

func (s *Server) allowTVSetupPublicRequest(w http.ResponseWriter, r *http.Request, kind, subject string) bool {
	policy := quickConnectRatePolicy{window: time.Minute, code: "tv_setup_rate_limited", detail: "TV setup requests are being sent too quickly."}
	switch kind {
	case "status":
		policy.limit = 120
	case "redeem":
		policy.limit = 10
	default:
		policy.limit = 6
	}
	now := time.Now().UTC()
	keys := []string{
		"tv-setup:" + kind + ":ip:" + clientIPFromRequest(r),
		"tv-setup:" + kind + ":subject:" + hashToken(strings.TrimSpace(subject)),
	}
	globalPolicy := policy
	globalPolicy.limit *= 20
	for index, key := range append(keys, "tv-setup:"+kind+":global") {
		candidate := policy
		if index == len(keys) {
			candidate = globalPolicy
		}
		allowed, retryAfter := s.tvSetupLimiter.allow(key, candidate, now)
		if allowed {
			continue
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeError(w, http.StatusTooManyRequests, candidate.code, candidate.detail)
		return false
	}
	return true
}

func (s *Server) tvSetupSession(setupSessionID string) (tvSetupSessionRecord, error) {
	return s.tvSetupSessionContext(context.Background(), setupSessionID)
}

func (s *Server) tvSetupSessionContext(ctx context.Context, setupSessionID string) (tvSetupSessionRecord, error) {
	setupSessionID = strings.TrimSpace(setupSessionID)
	if setupSessionID == "" {
		return tvSetupSessionRecord{}, sql.ErrNoRows
	}
	var session tvSetupSessionRecord
	err := s.queryUserRow(ctx, `
		SELECT id, code, status, user_id, installation_id, device_public_key, grant_secret_hash, encrypted_grant_json,
			device_name, platform, app_version, server_hint, auth_mode_hint, endpoint_url,
			client_ip, user_agent, native_refresh_token_id, expires_at, redeemed_at, created_at, updated_at
		FROM tv_setup_sessions
		WHERE id = ?`, setupSessionID).Scan(
		&session.ID, &session.Code, &session.Status, &session.UserID, &session.InstallationID, &session.DevicePublicKey, &session.GrantSecretHash, &session.EncryptedGrantJSON,
		&session.DeviceName, &session.Platform, &session.AppVersion, &session.ServerHint, &session.AuthModeHint, &session.EndpointURL,
		&session.ClientIP, &session.UserAgent, &session.NativeRefreshTokenID, &session.ExpiresAt, &session.RedeemedAt, &session.CreatedAt, &session.UpdatedAt,
	)
	return session, err
}

func (s *Server) tvSetupSessionByCode(code string) (tvSetupSessionRecord, error) {
	return s.tvSetupSessionByCodeContext(context.Background(), code)
}

func (s *Server) tvSetupSessionByCodeContext(ctx context.Context, code string) (tvSetupSessionRecord, error) {
	code = tvSetupCodeForLookup(code)
	if code == "" {
		return tvSetupSessionRecord{}, sql.ErrNoRows
	}
	var session tvSetupSessionRecord
	err := s.queryUserRow(ctx, `
		SELECT id, code, status, user_id, installation_id, device_public_key, grant_secret_hash, encrypted_grant_json,
			device_name, platform, app_version, server_hint, auth_mode_hint, endpoint_url,
			client_ip, user_agent, native_refresh_token_id, expires_at, redeemed_at, created_at, updated_at
		FROM tv_setup_sessions
		WHERE code = ? AND status = 'pending' AND expires_at > ?
		ORDER BY created_at DESC
		LIMIT 1`, code, time.Now().UTC().Format(time.RFC3339)).Scan(
		&session.ID, &session.Code, &session.Status, &session.UserID, &session.InstallationID, &session.DevicePublicKey, &session.GrantSecretHash, &session.EncryptedGrantJSON,
		&session.DeviceName, &session.Platform, &session.AppVersion, &session.ServerHint, &session.AuthModeHint, &session.EndpointURL,
		&session.ClientIP, &session.UserAgent, &session.NativeRefreshTokenID, &session.ExpiresAt, &session.RedeemedAt, &session.CreatedAt, &session.UpdatedAt,
	)
	return session, err
}

func (s *Server) authorizeTVSetupSession(setupSessionID string, userID string, grantSecretHash string, encryptedGrantJSON string, grantExpires time.Time) error {
	return s.authorizeTVSetupSessionContext(context.Background(), setupSessionID, userID, grantSecretHash, encryptedGrantJSON, grantExpires)
}

func (s *Server) authorizeTVSetupSessionContext(ctx context.Context, setupSessionID string, userID string, grantSecretHash string, encryptedGrantJSON string, grantExpires time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.execUserWrite(ctx, `
		UPDATE tv_setup_sessions
		SET status = 'grant_ready', user_id = ?, grant_secret_hash = ?, encrypted_grant_json = ?, expires_at = ?, updated_at = ?
		WHERE id = ? AND status = 'pending' AND expires_at > ?`,
		userID, grantSecretHash, encryptedGrantJSON, grantExpires.Format(time.RFC3339), now, setupSessionID, now)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return errors.New("setup session is no longer pending")
	}
	return nil
}

func (s *Server) expireTVSetupSessionIfNeeded(r *http.Request, session tvSetupSessionRecord) tvSetupSessionRecord {
	if session.Status != "pending" && session.Status != "grant_ready" {
		return session
	}
	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil || time.Now().UTC().Before(expiresAt) {
		return session
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	if _, err := s.execUserWrite(ctx, `UPDATE tv_setup_sessions SET status = 'expired', updated_at = ? WHERE id = ? AND status IN ('pending', 'grant_ready')`, now, session.ID); err == nil {
		session.Status = "expired"
		session.UpdatedAt = now
		s.recordAudit(r, User{ID: session.UserID}, "tv_setup.expired", "tv_setup", session.ID, "warn", map[string]string{"device": session.DeviceName, "platform": session.Platform})
	}
	return session
}

func tvSetupSessionResponse(session tvSetupSessionRecord, encryptedGrant *TVSetupEncryptedGrant) TVSetupSessionResponse {
	status := session.Status
	if status == "" {
		status = "pending"
	}
	authMode := session.AuthModeHint
	if authMode == "" {
		authMode = "unknown"
	}
	return TVSetupSessionResponse{
		SetupSessionID:      session.ID,
		Code:                session.Code,
		Status:              status,
		ProtocolVersion:     tvSetupProtocolForSession(session.Code),
		Service:             tvSetupServiceType,
		DevicePublicKey:     session.DevicePublicKey,
		DeviceName:          session.DeviceName,
		Platform:            session.Platform,
		AppVersion:          session.AppVersion,
		ServerHint:          session.ServerHint,
		AuthModeHint:        authMode,
		EndpointURL:         session.EndpointURL,
		ExpiresAt:           session.ExpiresAt,
		PollIntervalSeconds: 2,
		EncryptedGrant:      encryptedGrant,
	}
}

func (s *Server) tvSetupAuthModeHint(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local":
		return "local"
	case "portico-account", "portico":
		return "portico-account"
	case "unknown":
		return "unknown"
	}
	if settings, err := s.remoteAccessSettings(); err == nil && settings.PreferredRemoteAuthMode == "portico" {
		return "portico-account"
	}
	return "local"
}

func encryptTVSetupGrant(devicePublicKey string, setupSessionID string, payload tvSetupGrantPayload) (TVSetupEncryptedGrant, error) {
	tvPublicKeyBytes, err := decodeTVSetupPublicKey(devicePublicKey)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	curve := ecdh.X25519()
	tvPublicKey, err := curve.NewPublicKey(tvPublicKeyBytes)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	serverPrivateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	sharedSecret, err := serverPrivateKey.ECDH(tvPublicKey)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	key, err := tvSetupGrantKey(sharedSecret, setupSessionID)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return TVSetupEncryptedGrant{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(setupSessionID))
	return TVSetupEncryptedGrant{
		Version:         1,
		Algorithm:       "X25519-HKDF-SHA256-AESGCM",
		ServerPublicKey: base64.RawURLEncoding.EncodeToString(serverPrivateKey.PublicKey().Bytes()),
		Nonce:           base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext:      base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func tvSetupGrantKey(sharedSecret []byte, setupSessionID string) ([]byte, error) {
	salt := sha256.Sum256([]byte("portico-tv-setup-v1\x00" + strings.TrimSpace(setupSessionID)))
	reader := hkdf.New(sha256.New, sharedSecret, salt[:], []byte(tvSetupGrantInfo))
	key := make([]byte, tvSetupGrantKeyBytes)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func decodeTVSetupEncryptedGrant(raw string) (*TVSetupEncryptedGrant, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var grant TVSetupEncryptedGrant
	if err := json.Unmarshal([]byte(raw), &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

func decodeTVSetupPublicKey(value string) ([]byte, error) {
	bytes, err := decodeBase64URLFlexible(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if len(bytes) != 32 {
		return nil, fmt.Errorf("unexpected public key size %d", len(bytes))
	}
	return bytes, nil
}

func decodeBase64URLFlexible(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("empty base64 value")
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func randomTVSetupCode(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("TV setup entropy reader is unavailable")
	}
	code := make([]byte, tvSetupNormalizedCodeSize)
	limit := big.NewInt(int64(len(tvSetupCodeAlphabet)))
	for index := range code {
		value, err := rand.Int(reader, limit)
		if err != nil {
			return "", err
		}
		code[index] = tvSetupCodeAlphabet[value.Int64()]
	}
	return formatTVSetupCode(string(code)), nil
}

func normalizeTVSetupCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	code := make([]byte, 0, tvSetupNormalizedCodeSize)
	dashes := 0
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch character {
		case ' ':
			continue
		case '-':
			dashes++
			if dashes > 1 {
				return ""
			}
			continue
		}
		if !strings.ContainsRune(tvSetupCodeAlphabet, rune(character)) {
			return ""
		}
		code = append(code, character)
	}
	if len(code) != tvSetupNormalizedCodeSize {
		return ""
	}
	return string(code)
}

func formatTVSetupCode(code string) string {
	if len(code) != tvSetupNormalizedCodeSize {
		return code
	}
	return code[:4] + "-" + code[4:]
}

func tvSetupProtocolForSession(code string) int {
	return tvSetupProtocol
}

func tvSetupCodeMatchesSession(provided string, stored string) bool {
	normalized := normalizeTVSetupCode(provided)
	return normalized != "" && normalized == normalizeTVSetupCode(stored)
}

func tvSetupCodeForLookup(value string) string {
	if normalized := normalizeTVSetupCode(value); normalized != "" {
		return formatTVSetupCode(normalized)
	}
	return ""
}

func isTVSetupCodeConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") && strings.Contains(message, "tv_setup_sessions.code")
}

func cleanTVSetupLabel(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func cleanTVSetupEndpoint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		value = value[:512]
	}
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return ""
}
