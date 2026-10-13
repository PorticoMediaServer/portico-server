package httpapi

import (
	"context"
	"net/http"
	"portico.local/server/internal/medialimits"
	"portico.local/server/internal/networkpolicy"
	"portico.local/server/internal/operations"
	"strings"
	"sync"
	"sync/atomic"
)

// The settings document is read once per router and replaced synchronously on
// every settings save. Warm request decisions do no settings SQL.
type connectionPolicyCache struct {
	mu      sync.Mutex
	applyMu sync.Mutex
	setting atomic.Pointer[connectionSettingValue]
	// uploadAlert mirrors the upload-budget owner alert (upload_budget.go).
	uploadAlert atomic.Bool
}

type connectionSettingValue struct {
	revision          int64
	locality          localityPolicy
	accessURLs        []string
	certificateDomain string
	// apiPerMinute is the owner's rate for one API key or access token.
	apiPerMinute int
}

func (c *connectionPolicyCache) set(document operations.SettingsDocument) {
	c.applyMu.Lock()
	defer c.applyMu.Unlock()
	if current := c.setting.Load(); current != nil && document.Revision < current.revision {
		return
	}
	medialimits.ConfigureSubtitles(document.Effective.SubtitleTrackLimit, document.Effective.SubtitleFileLimitMiB)
	urls := []string{}
	if document.Effective.AdvertiseURL != "" {
		urls = append(urls, document.Effective.AdvertiseURL)
	}
	urls = append(urls, document.Effective.AccessURLs...)
	c.setting.Store(&connectionSettingValue{revision: document.Revision, locality: newLocalityPolicy(document.Effective), accessURLs: urls, certificateDomain: document.Effective.CustomCertificateDomain, apiPerMinute: document.Effective.APIRequestsPerMinute})
}

// connectionSetting is the cached projection of the connection rows (secure
// connections and request locality), read once and replaced on every save, so
// per-request decisions do no settings SQL. Nil without a console.
func (d Dependencies) connectionSetting(ctx context.Context) (*connectionSettingValue, error) {
	if d.connectionPolicy == nil || d.Console == nil {
		return nil, nil
	}
	if setting := d.connectionPolicy.setting.Load(); setting != nil {
		return setting, nil
	}
	d.connectionPolicy.mu.Lock()
	defer d.connectionPolicy.mu.Unlock()
	if setting := d.connectionPolicy.setting.Load(); setting != nil {
		return setting, nil
	}
	document, err := d.settingsDocument(ctx)
	if err != nil {
		return nil, err
	}
	d.connectionPolicy.set(document)
	return d.connectionPolicy.setting.Load(), nil
}

// secureTransport shares the trusted scheme parser with origin admission.
func (d Dependencies) secureTransport(r *http.Request) bool {
	return r.TLS != nil || strings.HasPrefix(networkpolicy.Origin(r, d.trustedProxyPrefixes(r.Context())), "https://")
}
