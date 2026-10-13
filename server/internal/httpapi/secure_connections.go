package httpapi

import (
	"context"
	"net/http"
	"portico.local/server/internal/networkpolicy"
	"portico.local/server/internal/operations"
	"strings"
	"sync"
	"sync/atomic"
)

// The settings document is read once per router and replaced synchronously on
// every settings save. Warm request decisions do no settings SQL.
type securePolicyCache struct {
	mu      sync.Mutex
	setting atomic.Pointer[secureSetting]
	// uploadAlert mirrors the upload-budget owner alert (upload_budget.go).
	uploadAlert atomic.Bool
}

type secureSetting struct {
	locality   localityPolicy
	accessURLs []string
}

func (c *securePolicyCache) set(document operations.SettingsDocument) {
	urls := append([]string{}, document.Effective.AccessURLs...)
	if document.Effective.AdvertiseURL != "" {
		urls = append(urls, document.Effective.AdvertiseURL)
	}
	c.setting.Store(&secureSetting{locality: newLocalityPolicy(document.Effective), accessURLs: urls})
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

// secureTransport shares the trusted scheme parser with origin admission.
func (d Dependencies) secureTransport(r *http.Request) bool {
	return r.TLS != nil || strings.HasPrefix(networkpolicy.Origin(r, d.trustedProxyPrefixes(r.Context())), "https://")
}
func (d Dependencies) loopbackClient(r *http.Request) bool {
	addr, ok := networkpolicy.Client(r, d.trustedProxyPrefixes(r.Context()))
	return ok && addr.IsLoopback()
}
