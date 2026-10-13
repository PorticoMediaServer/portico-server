package httpapi

import (
	"context"
	"net/http"
	"portico.local/server/internal/identity"
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
	// remoteSignIn is the owner's choice for sign-in from outside the home
	// network: one of the identity.RemoteSignIn* values.
	remoteSignIn string
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
	c.setting.Store(&connectionSettingValue{revision: document.Revision, locality: newLocalityPolicy(document.Effective), accessURLs: urls, certificateDomain: document.Effective.CustomCertificateDomain, apiPerMinute: document.Effective.APIRequestsPerMinute, remoteSignIn: remoteSignInPolicy(document.Effective.RemoteSignInPolicy)})
}

// remoteSignInPolicy reads the stored choice. An empty value is the default,
// which allows; a value this build does not know closes remote sign-in rather
// than opening it.
func remoteSignInPolicy(stored string) string {
	switch stored {
	case "", identity.RemoteSignInAllow:
		return identity.RemoteSignInAllow
	case identity.RemoteSignInOwnerOnly:
		return identity.RemoteSignInOwnerOnly
	default:
		return identity.RemoteSignInOff
	}
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
