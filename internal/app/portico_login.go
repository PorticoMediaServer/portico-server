package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const porticoLoginRequestTTL = 10 * time.Minute

type porticoLoginRequest struct {
	ID                 string
	StateHash          string
	ReturnURL          string
	ServerID           string
	CallbackURL        string
	LocalOrigin        string
	InstallationID     string
	ExpiresAt          string
	RememberOnBrowser  bool
	ExchangeResultJSON string
}

type porticoLoginExchangeResult struct {
	AccessToken       string                         `json:"accessToken"`
	ExpiresAt         string                         `json:"expiresAt"`
	SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
}

func (s *Server) handlePorticoLoginStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remote_access_failed", "Unable to load Portico account settings.")
		return
	}
	credential := s.secretSetting(remoteAccessCredentialKey)
	if !settings.Enabled || settings.ClaimStatus != "claimed" || settings.ServerID == "" || credential == "" || settings.PreferredRemoteAuthMode != "portico" {
		writeError(w, http.StatusConflict, "portico_login_unavailable", "This server is not configured for Portico account sign-in.")
		return
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "identity_failed", "Unable to load server identity.")
		return
	}
	systemIdentity, _ := s.systemIdentity()
	localOrigin := s.requestPublicOrigin(r)
	if localOrigin == "" {
		writeError(w, http.StatusBadRequest, "invalid_origin", "Unable to determine the local server address.")
		return
	}
	// Keep the callback on the exact origin that began the transaction. A
	// persisted public-route health result cannot prove that this browser can
	// hairpin through that route; substituting it here can strand an otherwise
	// valid localhost/LAN login after Hosted authorization. Public-direct starts
	// already carry their public origin, so no substitution is needed.
	callbackURL := strings.TrimRight(localOrigin, "/") + "/api/auth/portico/callback"
	returnURL := s.safePorticoLoginReturnURL(r.URL.Query().Get("returnUrl"), localOrigin)
	installationID := normalizeUntrustedNativeInstallationID(r.URL.Query().Get("installationId"))
	if installationID == "" {
		installationID = randomID("install")
	}
	state := randomToken()
	now := time.Now().UTC()
	rememberOnBrowser := true
	if raw := strings.TrimSpace(r.URL.Query().Get("rememberOnBrowser")); raw != "" {
		parsed, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_remember_on_browser", "rememberOnBrowser must be true or false.")
			return
		}
		rememberOnBrowser = parsed
	}
	request := porticoLoginRequest{
		ID:                randomID("plogin"),
		StateHash:         hashToken(state),
		ReturnURL:         returnURL,
		ServerID:          settings.ServerID,
		CallbackURL:       callbackURL,
		LocalOrigin:       localOrigin,
		InstallationID:    installationID,
		ExpiresAt:         now.Add(porticoLoginRequestTTL).Format(time.RFC3339),
		RememberOnBrowser: rememberOnBrowser,
	}
	if err := s.createPorticoLoginRequest(r.Context(), request, r, now); err != nil {
		writeError(w, http.StatusInternalServerError, "portico_login_failed", "Unable to start Portico account sign-in.")
		return
	}
	authURL := s.porticoHostedLoginURL(settings.HostedBaseURL, map[string]string{
		"serverId":                   settings.ServerID,
		"serverName":                 firstNonEmpty(systemIdentity.FriendlyName, "Portico"),
		"callbackUrl":                callbackURL,
		"localOrigin":                localOrigin,
		"state":                      state,
		"serverPublicKeyFingerprint": identity.Fingerprint,
		"installationId":             installationID,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handlePorticoLoginCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	request, err := s.claimPorticoLoginRequestByState(r.Context(), state)
	if err != nil {
		s.redirectPorticoLoginResult(w, r, s.defaultPorticoLoginReturnURL(r), false, "auth.sign-in-failed")
		return
	}
	requestConsumed := false
	requestRetryable := false
	defer func() {
		if requestRetryable {
			_ = s.releasePorticoLoginRequest(context.Background(), request.ID)
		} else if !requestConsumed {
			_ = s.consumePorticoLoginRequest(context.Background(), request.ID)
		}
	}()
	if hostedError := strings.TrimSpace(r.URL.Query().Get("error")); hostedError != "" {
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.sign-in-failed")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.sign-in-failed")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil {
		requestRetryable = true
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "problem.server-unavailable")
		return
	}
	if settings.ServerID == "" || settings.ServerID != request.ServerID || settings.PreferredRemoteAuthMode != "portico" {
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "problem.server-unavailable")
		return
	}
	var exchange porticoLoginExchangeResult
	if strings.TrimSpace(request.ExchangeResultJSON) != "" {
		if err := json.Unmarshal([]byte(request.ExchangeResultJSON), &exchange); err != nil {
			s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.sign-in-failed")
			return
		}
	} else {
		exchange, err = s.exchangePorticoLoginCode(r.Context(), settings, code)
		if err != nil {
			requestRetryable = true
			s.recordLog("warn", "Portico account local login exchange failed", map[string]string{"error": err.Error()})
			s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "problem.cloud-unavailable")
			return
		}
		rawExchange, marshalErr := json.Marshal(exchange)
		if marshalErr != nil || s.savePorticoLoginExchangeResult(r.Context(), request.ID, string(rawExchange)) != nil {
			requestRetryable = true
			s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "problem.server-unavailable")
			return
		}
	}
	user, hostedDeviceID, err := s.porticoAttachmentForAccessToken(r.Context(), settings, exchange.AccessToken, exchange.SelectionEnvelope)
	if err != nil {
		var hostedErr *hostedHTTPError
		requestRetryable = !errors.As(err, &hostedErr) || (hostedErr.StatusCode != http.StatusUnauthorized && hostedErr.StatusCode != http.StatusForbidden)
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.profile-selection-failed")
		return
	}
	app, platform := classifyUserAgent(r.UserAgent())
	descriptor := ProfileDeviceDescriptor{
		InstallationID: request.InstallationID,
		DeviceName:     firstNonEmpty(platform, app, "Web browser"),
		App:            firstNonEmpty(app, "Portico Web"),
		Platform:       firstNonEmpty(platform, "Web"),
	}
	var localDeviceID string
	err = s.withUserTxTagged(r.Context(), []string{"devices"}, func(tx *sql.Tx) error {
		var deviceErr error
		localDeviceID, deviceErr = s.upsertProfileAuthenticationDeviceTx(tx, r, user.ID, descriptor, time.Now().UTC())
		return deviceErr
	})
	if err != nil {
		requestRetryable = true
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, localSessionErrorMessageID(err))
		return
	}
	rawEnvelope, err := json.Marshal(exchange.SelectionEnvelope)
	if err != nil {
		requestRetryable = true
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.profile-selection-failed")
		return
	}
	grant, err := s.issueHostedProfileSelectionGrantForPurposeContext(r.Context(), user.ID, rawEnvelope, hostedDeviceID, localDeviceID, request.InstallationID, "browser", time.Now().UTC())
	if err != nil {
		requestRetryable = true
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.profile-selection-failed")
		return
	}
	sessionToken, err := s.createSessionForProviderWithSessionOptions(w, r, user.ID, "portico", sessionCreateOptions{
		TrustDevice:             true,
		ProfileID:               grant.ProfileID,
		ProfileSelectionGrant:   grant.Token,
		ProfileSelectionPurpose: "browser",
		BoundDeviceID:           localDeviceID,
		BoundInstallationID:     request.InstallationID,
	})
	if err != nil {
		requestRetryable = true
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, localSessionErrorMessageID(err))
		return
	}
	if request.RememberOnBrowser {
		if err := s.rememberBrowserAccountForSession(r.Context(), w, r, user.ID, "portico", sessionToken); err != nil {
			requestRetryable = true
			s.discardBrowserSession(w, r.Context(), sessionToken)
			s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "auth.sign-in-failed")
			return
		}
	}
	if err := s.consumePorticoLoginRequest(r.Context(), request.ID); err != nil {
		requestRetryable = true
		s.discardBrowserSession(w, r.Context(), sessionToken)
		s.redirectPorticoLoginResult(w, r, request.ReturnURL, false, "problem.server-unavailable")
		return
	} else {
		requestConsumed = true
	}
	s.recordLog("info", "Portico account signed in locally", map[string]string{"user": user.Email})
	s.redirectPorticoLoginResult(w, r, request.ReturnURL, true, "")
}

func (s *Server) exchangePorticoLoginCode(ctx context.Context, settings RemoteAccessSettings, code string) (porticoLoginExchangeResult, error) {
	credential := s.secretSetting(remoteAccessCredentialKey)
	if credential == "" {
		return porticoLoginExchangeResult{}, errors.New("server credential is missing")
	}
	body, _ := json.Marshal(map[string]string{"code": code})
	var response porticoLoginExchangeResult
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/local-login/exchange"
	if err := s.hostedJSON(ctx, http.MethodPost, endpoint, credential, body, &response); err != nil {
		return porticoLoginExchangeResult{}, err
	}
	if strings.TrimSpace(response.AccessToken) == "" || strings.TrimSpace(response.SelectionEnvelope.AssertionID) == "" {
		return porticoLoginExchangeResult{}, errors.New("Portico exchange response missing access token or profile selection")
	}
	return response, nil
}

func (s *Server) createPorticoLoginRequest(ctx context.Context, request porticoLoginRequest, r *http.Request, now time.Time) error {
	_, err := s.execUserWrite(ctx, `
		INSERT INTO portico_login_requests
			(id, state_hash, status, return_url, server_id, callback_url, local_origin, installation_id, client_ip, user_agent, expires_at, created_at, updated_at, remember_on_browser)
		VALUES (?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		request.ID, request.StateHash, request.ReturnURL, request.ServerID, request.CallbackURL, request.LocalOrigin,
		request.InstallationID, clientIPFromRequest(r), strings.TrimSpace(r.UserAgent()), request.ExpiresAt, now.Format(time.RFC3339), now.Format(time.RFC3339), request.RememberOnBrowser)
	return err
}

func (s *Server) claimPorticoLoginRequestByState(ctx context.Context, state string) (porticoLoginRequest, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return porticoLoginRequest{}, sql.ErrNoRows
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var request porticoLoginRequest
	err := s.withUserTxTagged(ctx, []string{"portico_login_requests"}, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE portico_login_requests
			SET status = 'processing', updated_at = ?
			WHERE state_hash = ? AND status = 'pending' AND expires_at > ?`, now, hashToken(state), now)
		if err != nil {
			return err
		}
		if rowsAffected(result) != 1 {
			return sql.ErrNoRows
		}
		return tx.QueryRowContext(ctx, `
			SELECT id, state_hash, return_url, server_id, callback_url, local_origin, installation_id, expires_at, remember_on_browser, COALESCE(exchange_result_json, '')
			FROM portico_login_requests
			WHERE state_hash = ? AND status = 'processing'
			LIMIT 1`, hashToken(state)).Scan(
			&request.ID, &request.StateHash, &request.ReturnURL, &request.ServerID, &request.CallbackURL, &request.LocalOrigin, &request.InstallationID, &request.ExpiresAt, &request.RememberOnBrowser, &request.ExchangeResultJSON,
		)
	})
	return request, err
}

func (s *Server) savePorticoLoginExchangeResult(ctx context.Context, requestID, rawExchange string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.execUserWrite(ctx, `UPDATE portico_login_requests SET exchange_result_json = ?, updated_at = ? WHERE id = ? AND status = 'processing'`, rawExchange, now, requestID)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) releasePorticoLoginRequest(ctx context.Context, requestID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.execUserWrite(ctx, `UPDATE portico_login_requests SET status = 'pending', updated_at = ? WHERE id = ? AND status = 'processing' AND expires_at > ?`, now, requestID, now)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) consumePorticoLoginRequest(ctx context.Context, requestID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.execUserWrite(ctx, `UPDATE portico_login_requests SET status = 'consumed', consumed_at = ?, updated_at = ? WHERE id = ? AND status = 'processing'`, now, now, requestID)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) safePorticoLoginReturnURL(raw string, localOrigin string) string {
	fallback := strings.TrimRight(localOrigin, "/") + "/"
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	base, err := url.Parse(fallback)
	if err != nil {
		return fallback
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	if !parsed.IsAbs() {
		if strings.HasPrefix(raw, "//") {
			return fallback
		}
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fallback
	}
	if parsed.User != nil || !strings.EqualFold(parsed.Host, base.Host) || parsed.Scheme != base.Scheme {
		return fallback
	}
	return parsed.String()
}

func (s *Server) defaultPorticoLoginReturnURL(r *http.Request) string {
	origin := s.requestPublicOrigin(r)
	if origin == "" {
		return "/"
	}
	return strings.TrimRight(origin, "/") + "/"
}

func (s *Server) redirectPorticoLoginResult(w http.ResponseWriter, r *http.Request, returnURL string, ok bool, messageID string) {
	target, err := url.Parse(strings.TrimSpace(returnURL))
	if err != nil || target.String() == "" {
		target, _ = url.Parse(s.defaultPorticoLoginReturnURL(r))
	}
	query := target.Query()
	if ok {
		query.Set("porticoLogin", "success")
		query.Del("porticoLoginError")
		query.Del("porticoLoginMessageId")
	} else {
		query.Set("porticoLogin", "error")
		query.Del("porticoLoginError")
		if strings.TrimSpace(messageID) != "" {
			query.Set("porticoLoginMessageId", messageID)
		}
	}
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (s *Server) porticoHostedLoginURL(hostedBaseURL string, params map[string]string) string {
	hosted := porticoHostedWebBaseURL(hostedBaseURL)
	values := url.Values{}
	for key, value := range params {
		if strings.TrimSpace(value) != "" {
			values.Set(key, value)
		}
	}
	return strings.TrimRight(hosted, "/") + "/#/local-login?" + values.Encode()
}

func porticoHostedWebBaseURL(hostedBaseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(hostedBaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "https://web.getportico.tv"
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "api.getportico.tv" || host == "getportico.tv" {
		parsed.Host = "web.getportico.tv"
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func localSessionErrorMessageID(err error) string {
	switch {
	case errors.Is(err, errDeviceNotTrusted):
		return "auth.device-session-required"
	case errors.Is(err, errDeviceNotAllowed):
		return "problem.forbidden"
	case errors.Is(err, errActiveSessionLimit):
		return "problem.rate-limited"
	case errors.Is(err, errAccessSchedule):
		return "problem.forbidden"
	case errors.Is(err, errBrowserAccountDisabled):
		return "problem.forbidden"
	default:
		return "auth.server-session-load-failed"
	}
}

func (r porticoLoginRequest) String() string {
	return fmt.Sprintf("%s:%s", r.ID, r.ServerID)
}
