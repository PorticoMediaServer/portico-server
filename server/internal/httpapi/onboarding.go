package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"portico.local/server/internal/identity"
	"strconv"
	"strings"
	"time"
)

// Forwarded addresses are authoritative only behind explicitly configured proxies.
// A proxy without one unambiguous address cannot establish LAN authority.
func (d Dependencies) privateSetupPeer(r *http.Request) bool {
	if len(d.TrustedProxies) == 0 && (r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "") {
		return false
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(peer)
	if err != nil {
		return false
	}
	for _, prefix := range d.TrustedProxies {
		if prefix.Contains(addr.Unmap()) {
			values := r.Header.Values("X-Forwarded-For")
			if len(values) != 1 {
				return false
			}
			forwarded, err := netip.ParseAddr(strings.TrimSpace(values[0]))
			if err != nil || forwarded.Zone() != "" {
				return false
			}
			addr = forwarded.Unmap()
			break
		}
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

func (d Dependencies) signInPeer(r *http.Request) string {
	if d.trustedProxy(r) {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) == 1 {
			if addr, err := netip.ParseAddr(strings.TrimSpace(values[0])); err == nil && addr.Zone() == "" {
				return addr.String()
			}
		}
	}
	return r.RemoteAddr
}

// The unauthenticated token convenience is only for a direct loopback browser.
// Proxy operators and every other machine must read the local one-time code.
func (d Dependencies) sameMachineSetup(r *http.Request) bool {
	if len(d.TrustedProxies) != 0 || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host).IsLoopback()
}

// Networking claim adapters that have no proxy configuration must fail closed.
func privateSetupPeer(r *http.Request) bool {
	if r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "" {
		return false
	}
	return (Dependencies{}).privateSetupPeer(r)
}

// Only local browser origins may bootstrap first-run setup. Host validation
// prevents a public DNS name rebound to a private peer from acquiring the token.
func localSetupHost(host string) bool {
	parsed, err := url.Parse("http://" + host)
	if err != nil || parsed.User != nil {
		return false
	}
	name := strings.ToLower(parsed.Hostname())
	if name == "localhost" {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}
func sameRequestOrigin(r *http.Request, origin string) bool {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}
func setupPeerSubject(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return "unknown"
	}
	return host
}
func setupFailure(w http.ResponseWriter, e error) {
	var protocol *identity.SetupProtocolError
	if errors.As(e, &protocol) {
		status := 400
		retryable := false
		if protocol.Code == "rate_limited" {
			status = 429
			retryable = true
		}
		if protocol.Code == "authorization_pending" || protocol.Code == "slow_down" {
			retryable = true
		}
		if protocol.Interval > 0 {
			w.Header().Set("Retry-After", fmtSetupInterval(protocol.Interval))
		}
		write(w, status, map[string]any{"error": map[string]any{"code": protocol.Code, "message": protocol.Code, "retryable": retryable}})
		return
	}
	failure(w, e)
}
func fmtSetupInterval(n int) string {
	if n < 5 {
		n = 5
	}
	return strconv.Itoa(n)
}
func (d Dependencies) onboardingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/setup/browser", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		parsed, err := url.Parse(origin)
		allowed := sameRequestOrigin(r, origin)
		for _, configured := range d.Origins {
			if origin == configured {
				allowed = true
			}
		}
		// Any browser may start first-run setup (Justin, 23 Sep: no setup code, even
		// remotely). The origin check stays: another site can't drive a visitor's
		// browser through it. The secret only lets this browser resume an
		// interrupted setup; the first owner created wins.
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !allowed || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			failure(w, identity.ErrUnauthorized)
			return
		}
		var body struct{}
		if err := decode(w, r, &body); err != nil {
			failure(w, err)
			return
		}
		if err := d.Identity.SetupLimit(r.Context(), setupPeerSubject(r), "browser-setup", 10); err != nil {
			setupFailure(w, err)
			return
		}
		token, err := d.Identity.BrowserSetupToken(r.Context())
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 200, map[string]string{"serverId": d.Identity.ID(), "setupToken": token})
	})

	mux.HandleFunc("POST /v1/setup/resume", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var q struct {
			RequestID  string `json:"requestId"`
			SetupToken string `json:"setupToken"`
		}
		if e := decode(w, r, &q); e != nil {
			failure(w, e)
			return
		}
		if e := d.Identity.SetupLimit(r.Context(), setupPeerSubject(r), "resume", 20); e != nil {
			setupFailure(w, e)
			return
		}
		out, e := d.Identity.ResumeSetup(r.Context(), q.RequestID, q.SetupToken, d.privateSetupPeer(r))
		if e != nil {
			failure(w, e)
			return
		}
		writeAuth(w, r, 200, out)
	})
	mux.HandleFunc("GET /v1/setup/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		p, e := d.localOwner(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Identity.Onboarding(r.Context(), p)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	for _, action := range []string{"finish", "remote-recovery"} {
		action := action
		mux.HandleFunc("POST /v1/setup/"+action, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			p, e := d.localOwner(r)
			if e != nil {
				failure(w, e)
				return
			}
			var q struct {
				Revision         int64 `json:"revision,string"`
				Enabled          bool  `json:"enabled"`
				WarningConfirmed bool  `json:"warningConfirmed"`
			}
			if e = decode(w, r, &q); e != nil {
				failure(w, e)
				return
			}
			var out identity.OnboardingState
			if action == "finish" {
				out, e = d.Identity.FinishSetup(r.Context(), p, q.Revision)
			} else {
				out, e = d.Identity.SetRemoteRecovery(r.Context(), p, q.Revision, q.Enabled, q.WarningConfirmed)
			}
			if e != nil {
				failure(w, e)
				return
			}
			write(w, 200, out)
		})
	}
	mux.HandleFunc("POST /v1/quick-connect", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if e := d.Identity.SetupLimit(ctx, setupPeerSubject(r), "quick-create", 10); e != nil {
			setupFailure(w, e)
			return
		}
		var q identity.QuickStart
		if e := decode(w, r, &q); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Identity.StartQuick(ctx, q, d.privateSetupPeer(r))
		if e != nil {
			setupFailure(w, e)
			return
		}
		// A code for this server is approved on this server (its /device page, or
		// any app signed in to it), never on the Portico Account site.
		if origin := d.quickApprovalOrigin(r); origin != "" {
			out.VerificationURI = origin + "/device"
			out.VerificationURIComplete = origin + "/device#code=" + out.UserCode
		}
		write(w, 201, out)
	})
	mux.HandleFunc("POST /v1/quick-connect/review", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		for _, subject := range []string{"ip:" + setupPeerSubject(r), "account:" + p.AccountID} {
			if e = d.Identity.SetupLimit(r.Context(), subject, "quick-review", 10); e != nil {
				setupFailure(w, e)
				return
			}
		}
		var q struct {
			UserCode string `json:"userCode"`
		}
		if e = decode(w, r, &q); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Identity.ReviewQuick(r.Context(), q.UserCode)
		if e != nil {
			setupFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/quick-connect/decision", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		for _, subject := range []string{"ip:" + setupPeerSubject(r), "account:" + p.AccountID} {
			if e = d.Identity.SetupLimit(r.Context(), subject, "quick-decision", 10); e != nil {
				setupFailure(w, e)
				return
			}
		}
		var q struct {
			RequestID string `json:"requestId"`
			UserCode  string `json:"userCode"`
			Decision  string `json:"decision"`
		}
		if e = decode(w, r, &q); e != nil {
			failure(w, e)
			return
		}
		if e = d.Identity.DecideQuick(r.Context(), p, q.RequestID, q.UserCode, q.Decision); e != nil {
			setupFailure(w, e)
			return
		}
		w.WriteHeader(204)
	})
	for _, action := range []string{"token", "cancel"} {
		action := action
		mux.HandleFunc("POST /v1/quick-connect/"+action, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			var q struct {
				DeviceCode string `json:"deviceCode"`
			}
			if e := decode(w, r, &q); e != nil {
				failure(w, e)
				return
			}
			if action == "cancel" {
				if e := d.Identity.CancelQuick(ctx, q.DeviceCode); e != nil {
					setupFailure(w, e)
					return
				}
				w.WriteHeader(204)
				return
			}
			out, e := d.Identity.PollQuick(ctx, q.DeviceCode)
			if e != nil {
				setupFailure(w, e)
				return
			}
			if e = d.Identity.CheckRecoveryRoute(ctx, identity.Principal{Viewer: out.Viewer}, d.privateSetupPeer(r)); e != nil {
				failure(w, e)
				return
			}
			writeAuth(w, r, 200, out)
		})
	}
}

// quickApprovalOrigin is the origin the new device reached this server on. The
// answer goes only to that device, so it cannot redirect anyone else.
func (d Dependencies) quickApprovalOrigin(r *http.Request) string {
	host := strings.ToLower(r.Host)
	if host == "" || len(host) > 255 || strings.ContainsAny(host, "/\\@?#% ") {
		return ""
	}
	if d.secureTransport(r) {
		return "https://" + host
	}
	return "http://" + host
}
