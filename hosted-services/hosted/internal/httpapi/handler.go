package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"portico.local/hosted/internal/appstore"
	"portico.local/hosted/internal/buildinfo"
	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/sessions"
	"portico.local/hosted/packages/i18n"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Handler struct {
	ServerReturns      *ServerReturns
	AppAuthorizations  *AppAuthorizations
	AccessLog          func(string)
	requests           atomic.Uint64
	rejections         atomic.Uint64
	Certificates       *certificates.Service
	DirectRoutes       *directroute.Service
	OIDC               *postgres.OIDCLogin
	Security           *postgres.AccountSecurity
	AccountOrigin      string
	Claims             http.Handler
	Sessions           *sessions.Service
	DevelopmentCookies bool
	TrustedProxies     []netip.Prefix
	Devices            *device.Service
	Identity           *identity.Service
	Directory          *directory.Service
	Origins            map[string]bool
	Ready              func(context.Context) error
	// AppStore is the verified App Store Server Notifications key set. Nil leaves
	// the intake endpoint unregistered, which is what an unconfigured deployment
	// wants: a route that cannot verify must not exist.
	AppStore *appstore.KeySet
	budgets  attemptBudgets
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	h.oidcRoutes(mux)
	h.appAuthorizationRoutes(mux)
	h.serverReturnRoutes(mux)
	h.appleNotificationRoutes(mux)
	h.securityRoutes(mux)
	h.RegisterDirectRoutes(mux, h.DirectRoutes)
	h.certificateRoutes(mux)
	if h.Claims != nil {
		mux.Handle("POST /v1/servers/leave", h.Claims)
		mux.Handle("/v1/server-claims/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !h.allowAuth(r) {
				w.Header().Set("Retry-After", "60")
				fail(w, http.StatusTooManyRequests, "rate_limited", "Try again shortly.", true)
				return
			}
			h.Claims.ServeHTTP(w, r)
		}))
	}
	mux.HandleFunc("GET /v1/system", func(w http.ResponseWriter, r *http.Request) { write(w, 200, buildinfo.Current()) })
	h.lifecycleRoutes(mux)
	h.membershipLifecycleRoutes(mux)
	h.deletionRoutes(mux)
	h.deviceRoutes(mux)
	h.accountIdentityRoutes(mux)
	h.notificationRoutes(mux)
	h.exportRoutes(mux)
	h.refreshRoutes(mux)
	h.endpointRoutes(mux)
	normalSlots := make(chan struct{}, 128)
	claimStreamSlots := make(chan struct{}, 128)
	securitySlots := make(chan struct{}, 16)
	// Password work is CPU-bound and already limited to a few concurrent hashes. Without its own
	// waiting room, a sign-in burst (every client re-authenticating after an outage) queues for
	// those few hashes while holding general slots, and server check-ins are shed instead.
	passwordSlots := make(chan struct{}, 24)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]bool{"alive": true}) })
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		if h.Ready != nil && h.Ready(r.Context()) != nil {
			fail(w, http.StatusServiceUnavailable, "unavailable", "Service unavailable.", true)
			return
		}
		write(w, 200, map[string]bool{"ready": true})
	})
	mux.HandleFunc("GET /v1/keys", func(w http.ResponseWriter, r *http.Request) {
		id, key := h.Directory.Keys()
		write(w, 200, KeySet{Certificates: h.Directory.KeyCertificates(), KeyID: id, PublicKey: key, Root: h.Directory.TrustRoot()})
	})
	mux.HandleFunc("POST /v1/accounts", h.register)
	mux.HandleFunc("POST /v1/sessions", h.login)
	mux.HandleFunc("GET /v1/me", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) { write(w, 200, v) }))
	mux.HandleFunc("DELETE /v1/sessions/current", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		if e := h.Identity.Revoke(r.Context(), bearer(r)); e != nil {
			failure(w, e)
			return
		}
		// Bearer-scoped logout must not clear another browser family's cookie.
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /v1/servers", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		// The version is read before the list: a change committing in between
		// moves the version past this one, so the next check sees it.
		version := h.Directory.ServerListVersion(v.Account.ID)
		etag := `"` + version + `"`
		if r.URL.Query().Get("include") == "dormant" {
			// A different list: its own validator (INT M11).
			etag = `"d` + version + `"`
		}
		w.Header().Set("ETag", etag)
		if h.Directory.ServerListReady() && r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		items, e := h.Directory.List(r.Context(), v.Account.ID)
		if e != nil {
			failure(w, e)
			return
		}
		if r.URL.Query().Get("include") != "dormant" {
			visible := items[:0]
			for _, item := range items {
				if item.Status != "dormant" {
					visible = append(visible, item)
				}
			}
			items = visible
		}
		var ending *directory.MembershipEndingNotice
		if len(items) == 0 {
			ending, e = h.Directory.MembershipEndingNotice(r.Context(), v.Account.ID)
			if e != nil {
				failure(w, e)
				return
			}
		}
		write(w, 200, ServerList{Items: items, Version: version, Watch: h.Directory.WatchToken(v.Account.ID), MembershipEnding: ending})
	}))

	mux.HandleFunc("POST /v1/servers/{id}/heartbeat", h.server(func(w http.ResponseWriter, r *http.Request, authority directory.ServerAuthority) {
		var q directory.CheckInReport
		if !decode(w, r, &q) {
			return
		}
		result, e := h.Directory.ServerHeartbeat(r.Context(), authority, q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, result)
	}))
	h.memberIndexRoutes(mux)
	for _, add := range devRoutes {
		add(h, mux)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w, finish := h.observe(w, r, mux)
		defer finish()
		ip, e := h.clientAddress(r)
		if e != nil {
			fail(w, 400, "invalid_forwarding", "Invalid request forwarding.", false)
			return
		}
		r = withClientAddress(r, ip)
		r = r.WithContext(identity.WithDeviceContext(r.Context(), identity.DeviceContext{Name: r.Header.Get("X-Portico-Device-Name"), Platform: r.Header.Get("X-Portico-Platform"), App: r.Header.Get("X-Portico-App"), Version: r.Header.Get("X-Portico-Version"), InstallationID: r.Header.Get("X-Portico-Installation-ID"), IP: ip}))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		origin := r.Header.Get("Origin")
		if appExchange(r) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "Retry-After")
			w.Header().Set("Access-Control-Max-Age", "86400")
		} else if origin != "" && !h.oidcCallback(r) {
			if origin == "*" || origin == "null" || !h.Origins[origin] {
				fail(w, 403, "origin_denied", "Origin is not permitted.", false)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token, X-Portico-Device-Name, X-Portico-Platform, X-Portico-App, X-Portico-Version, X-Portico-Installation-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "Retry-After, X-CSRF-Token")
			// A browser otherwise forgets a preflight after about five seconds and doubles its
			// requests here. The real request is still origin-checked every time.
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			if origin == "" {
				fail(w, 403, "origin_denied", "Origin is required.", false)
				return
			}
			w.WriteHeader(204)
			return
		}
		if r.URL.RawQuery != "" && !h.oidcCallback(r) && !appAuthorizationQueryAllowed(r) && !(r.Method == "GET" && r.URL.Path == "/v1/servers" && len(r.URL.Query()) == 1 && r.URL.Query().Get("include") == "dormant") && !sessionReviewQueryAllowed(r) && !notificationQueryAllowed(r) && !(r.Method == http.MethodGet && r.URL.Path == "/v1/dns/addresses") && !(r.Method == http.MethodGet && devQueryPaths[r.URL.Path]) {
			fail(w, 400, "query_not_supported", "Query parameters are not supported.", false)
			return
		}
		slots := normalSlots
		if r.URL.Path == "/v1/server-claims/events" {
			slots = claimStreamSlots
		}
		if r.URL.Path == "/v1/servers/leave" || r.URL.Path == "/v1/server-claims/cancel" || r.URL.Path == "/v1/server-claims/ack" || strings.HasPrefix(r.URL.Path, "/v1/account/deletion") || strings.HasSuffix(r.URL.Path, "/sessions/logout-all") || strings.HasSuffix(r.URL.Path, "/sessions/revoke") || strings.HasSuffix(r.URL.Path, "/device-authorizations/cancel") || r.Method == http.MethodDelete {
			slots = securitySlots
		}
		if r.Method == http.MethodPost && passwordPath(r.URL.Path) {
			slots = passwordSlots
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			// Everyone shed in the same second must not return in the same second.
			w.Header().Set("Retry-After", strconv.Itoa(2+rand.IntN(9)))
			fail(w, 429, "capacity_limited", "Service is busy. Try again shortly.", true)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout(r))
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (h *Handler) allowAuth(r *http.Request) bool {
	// Both budgets count every attempt: 30 a minute per address (/64 for IPv6)
	// and 300 per wider network (/48, /24), so cycling addresses inside one
	// delegation still runs out.
	address := h.allowAttempt("auth:"+r.URL.Path+":"+budgetIP(peer(r)), 30)
	network := h.allowAttempt("authnet:"+r.URL.Path+":"+budgetNetwork(peer(r)), 300)
	return address && network
}
func (h *Handler) allowAttempt(key string, limit int) bool {
	return h.budgets.allow(key, limit, time.Now())
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) { h.registrationCreate(w, r) }
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.allowAuth(r) {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "Try again shortly.", true)
		return
	}
	var q SignInRequest
	if !decode(w, r, &q) {
		return
	}
	if q.SessionMode == "" {
		q.SessionMode = "native"
	}
	if !h.validSessionMode(w, r, q.SessionMode) {
		return
	}
	if !h.securityReady(w) || !h.credentialOrigin(w, r) {
		return
	}
	binding, ok := h.continuation(w, r, true)
	if !ok {
		return
	}
	response, e := h.Security.PasswordLoginBound(r.Context(), q.Username, q.Password, q.SessionMode, binding, q.RequestID, budgetIP(peer(r)))
	if e != nil {
		failure(w, e)
		return
	}
	if response.Challenge != nil {
		write(w, 202, response.Challenge)
	} else if response.Session != nil {
		h.sessionResponse(w, r, 200, *response.Session)
	} else {
		fail(w, 503, "unavailable", "Sign-in could not complete.", true)
	}
}
func bearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(header, "Bearer ")
}
func (h *Handler) account(next func(http.ResponseWriter, *http.Request, identity.Viewer)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "account") {
			return
		}
		v, e := h.Identity.Authenticate(r.Context(), bearer(r))
		if e != nil {
			failure(w, e)
			return
		}
		if !h.allowAttempt("account:"+v.Account.ID, 240) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "Try again shortly.", true)
			return
		}
		next(w, r, v)
	}
}
func (h *Handler) server(next func(http.ResponseWriter, *http.Request, directory.ServerAuthority)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "server") {
			return
		}
		authority, e := h.Directory.CaptureServerAuthority(r.Context(), bearer(r))
		if e != nil {
			failure(w, e)
			return
		}
		if authority.ServerID != r.PathValue("id") {
			failure(w, directory.ErrDenied)
			return
		}
		if !h.allowAttempt("server:"+authority.ServerID, 120) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rate_limited", "Try again shortly.", true)
			return
		}
		next(w, r, authority)
	}
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeLimit(w, r, target, 64<<10)
}

// decodeLimit is decode with a larger body bound, for the one route whose
// request can be a server's whole member list.
func decodeLimit(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(w, 415, "content_type", "Use application/json.", false)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(target); e != nil {
		fail(w, 400, "invalid_json", "Invalid request.", false)
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		fail(w, 400, "invalid_json", "Exactly one request object is required.", false)
		return false
	}
	return true
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, message string, retry bool) {
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "retryable": retry}})
}
func failure(w http.ResponseWriter, e error) {
	if appGrantError(e) {
		fail(w, 400, "invalid_grant", i18n.AppSignIn["expired"], false)
		return
	}
	if errors.Is(e, postgres.ErrPreferenceInput) {
		fail(w, 400, "invalid_request", "Those preferences are not valid.", false)
		return
	}
	if errors.Is(e, postgres.ErrPreferenceChanged) {
		fail(w, 409, "preferences_changed", postgres.ErrPreferenceChanged.Error(), false)
		return
	}
	if errors.Is(e, identity.ErrOwnedServers) {
		fail(w, 409, "owned_servers", "Change owner, switch to local sign-in, or start over on every owned server before deleting your Portico Account.", false)
		return
	}
	if errors.Is(e, identity.ErrDeletionConflict) {
		fail(w, 409, "deletion_preview_changed", "Get a fresh deletion preview and confirm its disposition again.", false)
		return
	}
	if errors.Is(e, identity.ErrDeletionConfirmation) {
		fail(w, 400, "deletion_confirmation_mismatch", "Type the account's email address exactly to confirm deletion.", false)
		return
	}
	if errors.Is(e, sessions.ErrFamilyChanged) {
		fail(w, 409, "family_changed", "The browser session changed. Discard the previous session intent.", false)
		return
	}
	if errors.Is(e, sessions.ErrReuse) {
		fail(w, 401, "refresh_reuse_detected", "Session ended. Sign in again.", false)
		return
	}
	if errors.Is(e, sessions.ErrGrant) {
		fail(w, 401, "invalid_grant", "Session expired or was revoked. Sign in again.", false)
		return
	}
	if errors.Is(e, sessions.ErrConflict) {
		fail(w, 409, "refresh_conflict", "Refresh request conflicts with current state.", false)
		return
	}
	var protocol *device.ProtocolError
	if errors.As(e, &protocol) {
		status := 400
		retry := false
		message := "Device authorization is not available."
		switch protocol.Code {
		case "authorization_pending":
			retry = true
			message = "Waiting for Portico Account approval."
		case "slow_down", "rate_limited":
			status = 429
			retry = true
			message = "Wait before trying again."
		case "access_denied":
			status = 403
			message = "Device authorization was denied or cancelled."
		case "expired_token":
			message = "Device authorization has expired."
		}
		if protocol.Interval > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(protocol.Interval))
		}
		fail(w, status, protocol.Code, message, retry)
		return
	}
	if errors.Is(e, device.ErrInvalid) {
		fail(w, 400, "invalid_input", "Check the supplied values.", false)
		return
	}
	if errors.Is(e, device.ErrConflict) {
		fail(w, 409, "conflict", "Device authorization conflicts with current state.", false)
		return
	}
	switch {
	case errors.Is(e, postgres.ErrMembershipBusy):
		fail(w, 409, "membership_busy", "This server is finishing a certificate operation. Try the same transfer again.", true)
	case errors.Is(e, identity.ErrRateLimited):
		retry := 60
		var cool identity.CoolOff
		if errors.As(e, &cool) {
			retry = max(1, int(time.Until(cool.Until).Seconds()+0.999))
		}
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		fail(w, 429, "rate_limited", "Wait before trying again.", true)
	case errors.Is(e, identity.ErrChallenge):
		fail(w, 400, "challenge_invalid", "Verification is invalid or expired. Check the code or restart verification.", false)
	case errors.Is(e, identity.ErrRecentAuth):
		fail(w, 403, "recent_auth_required", "Confirm your current sign-in method again.", false)
	case errors.Is(e, identity.ErrLastMethod):
		fail(w, 409, "last_method", "Add another usable sign-in method before removing this one.", false)
	case errors.Is(e, identity.ErrProviderConflict):
		fail(w, 409, "provider_unavailable", "This provider identity cannot be used for this operation.", false)
	case errors.Is(e, identity.ErrMailUnavailable):
		fail(w, 503, "mail_unavailable", "Account email is not configured. Contact the operator.", false)
	case errors.Is(e, identity.ErrPushUnavailable):
		fail(w, 503, "push_unavailable", "Device notifications are not configured.", false)
	case errors.Is(e, identity.ErrOwnedServers):
		fail(w, 409, "owned_servers", "Resolve ownership of your servers before deleting your account.", false)
	case errors.Is(e, identity.ErrDeletionConflict):
		fail(w, 409, "deletion_conflict", "The preview changed, expired, or a server cleanup prerequisite is unresolved. Review a new preview.", false)
	case errors.Is(e, directory.ErrCapacity):
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "Try again shortly.", true)
	case errors.Is(e, identity.ErrCredentials):
		fail(w, 401, "invalid_credentials", "Sign-in credentials are not valid.", false)
	case errors.Is(e, directory.ErrDenied):
		fail(w, 403, "permission_denied", "This operation is not permitted.", false)
	case errors.Is(e, identity.ErrConflict), errors.Is(e, directory.ErrConflict):
		fail(w, 409, "conflict", "The operation conflicts with current state.", false)
	case errors.Is(e, identity.ErrInvalid), errors.Is(e, directory.ErrInvalid):
		fail(w, 400, "invalid_input", "Check the supplied values.", false)
	default:
		fail(w, 503, "unavailable", "The operation could not complete. Try again.", true)
	}
}

// passwordPath names the requests that verify or set a password.
func passwordPath(path string) bool {
	switch path {
	case "/v1/sessions", "/v1/accounts", "/v1/account/password", "/v1/account/recovery/complete", "/v1/account/security/proof":
		return true
	}
	return false
}

func budgetIP(raw string) string {
	ip, e := netip.ParseAddr(raw)
	if e != nil {
		return raw
	}
	ip = ip.Unmap()
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

func requestTimeout(r *http.Request) time.Duration {
	if r.URL.Path == "/v1/server-claims/events" {
		return 10 * time.Minute
	}
	return 15 * time.Second
}

func (h *Handler) RequestMetrics() (uint64, uint64) { return h.requests.Load(), h.rejections.Load() }
