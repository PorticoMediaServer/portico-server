package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portico.local/server/internal/operations"
)

// The settings document is read once per router and replaced synchronously on
// every settings save. A normal Preferred request does no settings SQL.
type securePolicyCache struct {
	mu      sync.Mutex
	setting atomic.Pointer[secureSetting]
	route   atomic.Pointer[secureRoute]
	routeMu sync.Mutex
	alert   atomic.Bool
	// uploadAlert mirrors the upload-budget owner alert (upload_budget.go).
	uploadAlert atomic.Bool
}

type secureSetting struct {
	required bool
	locality localityPolicy
}
type secureRoute struct {
	checked  time.Time
	required bool
	url      string
}

func (c *securePolicyCache) set(document operations.SettingsDocument) {
	c.routeMu.Lock()
	c.setting.Store(&secureSetting{required: document.Effective.SecureConnectionsPolicy == "required", locality: newLocalityPolicy(document.Effective)})
	c.route.Store(nil)
	c.routeMu.Unlock()
}

func (d Dependencies) securePolicyRequired(ctx context.Context) (bool, error) {
	setting, err := d.connectionSetting(ctx)
	if err != nil || setting == nil {
		return false, err
	}
	return setting.required, nil
}

// connectionSetting is the cached projection of the connection rows (secure
// connections and request locality), read once and replaced on every save, so
// per-request decisions do no settings SQL. Nil without a console.
func (d Dependencies) connectionSetting(ctx context.Context) (*secureSetting, error) {
	if d.securePolicy == nil || d.Console == nil {
		return nil, nil
	}
	if setting := d.securePolicy.setting.Load(); setting != nil {
		return setting, nil
	}
	d.securePolicy.mu.Lock()
	defer d.securePolicy.mu.Unlock()
	if setting := d.securePolicy.setting.Load(); setting != nil {
		return setting, nil
	}
	document, err := d.settingsDocument(ctx)
	if err != nil {
		return nil, err
	}
	d.securePolicy.set(document)
	return d.securePolicy.setting.Load(), nil
}

// secureTransport recognizes direct TLS and HTTPS asserted by an explicitly
// trusted reverse proxy. An untrusted client cannot set its own scheme.
func (d Dependencies) secureTransport(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	values := r.Header.Values("X-Forwarded-Proto")
	return d.trustedProxy(r) && len(values) == 1 && values[0] == "https"
}

func loopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}

// An explicitly trusted proxy speaks for the client address. A malformed or
// ambiguous forwarded address is never treated as a local recovery request.
func (d Dependencies) loopbackClient(r *http.Request) bool {
	if !d.trustedProxy(r) {
		return loopbackPeer(r)
	}
	addr, ok := forwardedClient(r, d.trustedProxyPrefixes(r.Context()))
	return ok && addr.IsLoopback()
}

func (d Dependencies) alertHTTPSUnavailable(ctx context.Context, active bool) {
	if d.securePolicy == nil || d.Console == nil {
		return
	}
	if d.securePolicy.alert.Load() == active {
		return
	}
	d.securePolicy.mu.Lock()
	defer d.securePolicy.mu.Unlock()
	if d.securePolicy.alert.Load() == active {
		return
	}
	if err := d.Console.Alert(ctx, "secure-connections-required-unavailable", "warning", active); err == nil {
		d.securePolicy.alert.Store(active)
	}
}

func (d Dependencies) requiredHTTPS(ctx context.Context) (bool, string, error) {
	required, err := d.securePolicyRequired(ctx)
	if err != nil || !required || d.DB == nil {
		if !required {
			d.alertHTTPSUnavailable(ctx, false)
		}
		return false, "", err
	}
	if cached := d.securePolicy.route.Load(); cached != nil && time.Since(cached.checked) < time.Second {
		return cached.required, cached.url, nil
	}
	d.securePolicy.routeMu.Lock()
	if cached := d.securePolicy.route.Load(); cached != nil && time.Since(cached.checked) < time.Second {
		d.securePolicy.routeMu.Unlock()
		return cached.required, cached.url, nil
	}
	ready, route, claimed, err := d.probeRequiredHTTPS(ctx)
	if err == nil {
		d.securePolicy.route.Store(&secureRoute{checked: time.Now(), required: ready, url: route})
	}
	d.securePolicy.routeMu.Unlock()
	d.alertHTTPSUnavailable(ctx, claimed && !ready)
	return ready, route, err
}

func (d Dependencies) probeRequiredHTTPS(ctx context.Context) (bool, string, bool, error) {
	var err error
	// Direct Sign-In never loses HTTP access just because a stale settings
	// document says Required. A claim is necessary, but never sufficient.
	var schema bool
	if err = d.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='networking_claim_identity')`).Scan(&schema); err != nil || !schema {
		return false, "", false, err
	}
	var claimed bool
	if err = d.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM networking_claim_identity WHERE singleton=1 AND installed_operation_id IS NOT NULL)`).Scan(&claimed); err != nil || !claimed {
		return false, "", false, err
	}
	var route string
	var ready bool
	if d.HTTPSRoute != nil {
		route, ready, err = d.HTTPSRoute(ctx)
	} else if d.Networking != nil && d.Networking.Certificates() != nil {
		status, statusErr := d.Networking.Certificates().Status(ctx)
		err, route, ready = statusErr, status.RouteURL, status.TLSReady
	}
	if err != nil {
		// A failed certificate check must not lock the owner out.
		ready = false
	}
	ready = ready && strings.HasPrefix(route, "https://")
	if !ready {
		return false, "", true, nil
	}
	return true, route, true, nil
}

func (d Dependencies) enforceSecureConnections(w http.ResponseWriter, r *http.Request) bool {
	if d.secureTransport(r) || d.loopbackClient(r) || !(r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/")) {
		return false
	}
	required, route, err := d.requiredHTTPS(r.Context())
	if err != nil {
		write(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]any{"code": "transport_policy_unavailable", "message": "The server connection policy is temporarily unavailable.", "retryable": true}})
		return true
	}
	if !required {
		return false
	}
	write(w, http.StatusForbidden, map[string]any{"error": map[string]any{"code": "secure_connection_required", "message": "This server requires HTTPS for API requests.", "retryable": false, "httpsUrl": route}})
	return true
}
