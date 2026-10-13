package httpapi

import (
	"context"
	"net/http"
	"portico.local/apikit"

	"portico.local/server/internal/connectivity"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/operations"
)

// Connectivity routes use the same typed registry as the first-party API.
type ConnectivityChange struct {
	ExpectedRevision int64     `json:"expectedRevision"`
	OperationID      string    `json:"operationId"`
	RemoteSignIn     *string   `json:"remoteSignInPolicy,omitempty"`
	RemoteBitrate    *int      `json:"remoteBitrateLimitKbps,omitempty"`
	LANNetworks      *[]string `json:"lanNetworks,omitempty"`
	AccessURLs       *[]string `json:"accessUrls,omitempty"`
	LANDiscovery     *bool     `json:"lanDiscoveryEnabled,omitempty"`
	TrustedProxies   *[]string `json:"trustedProxies,omitempty"`
	UploadCapacity   *int      `json:"uploadCapacityKbps,omitempty"`
	PausedTimeout    *int      `json:"pausedSessionTimeoutMinutes,omitempty"`
	Interface        *string   `json:"advertisedInterface,omitempty"`
	CertificatePath  *string   `json:"customCertificatePath,omitempty"`
	CertificateKey   *string   `json:"customCertificateKeyPath,omitempty"`
	CertificateHost  *string   `json:"customCertificateDomain,omitempty"`
	ConnectionLimit  *int      `json:"connectionLimit,omitempty"`
	AdvertiseURL     *string   `json:"advertiseUrl,omitempty"`
}

func (d Dependencies) changeConnectivity(r *http.Request, body ConnectivityChange) (connectivity.Policy, error) {
	p, err := d.owner(r)
	if err != nil {
		return connectivity.Policy{}, err
	}
	document, err := d.applySettings(r, p, body.ExpectedRevision, body.OperationID, func(v *operations.Settings) {
		if body.RemoteSignIn != nil {
			v.RemoteSignInPolicy = *body.RemoteSignIn
		}
		if body.RemoteBitrate != nil {
			v.RemoteBitrateLimitKbps = *body.RemoteBitrate
		}
		if body.LANNetworks != nil {
			v.LANNetworks = *body.LANNetworks
		}
		if body.AccessURLs != nil {
			v.AccessURLs = *body.AccessURLs
		}
		if body.LANDiscovery != nil {
			v.LANDiscoveryEnabled = *body.LANDiscovery
		}
		if body.TrustedProxies != nil {
			v.TrustedProxies = *body.TrustedProxies
		}
		if body.UploadCapacity != nil {
			v.UploadCapacityKbps = *body.UploadCapacity
		}
		if body.PausedTimeout != nil {
			v.PausedSessionTimeoutMinutes = *body.PausedTimeout
		}
		if body.Interface != nil {
			v.AdvertisedInterface = *body.Interface
		}
		if body.CertificatePath != nil {
			v.CustomCertificatePath = *body.CertificatePath
		}
		if body.CertificateKey != nil {
			v.CustomCertificateKeyPath = *body.CertificateKey
		}
		if body.CertificateHost != nil {
			v.CustomCertificateDomain = *body.CertificateHost
		}
		if body.ConnectionLimit != nil {
			v.ConnectionLimit = *body.ConnectionLimit
		}
		if body.AdvertiseURL != nil {
			v.AdvertiseURL = *body.AdvertiseURL
		}
	})
	if err != nil {
		return connectivity.Policy{}, err
	}
	if d.Access.Advertiser != nil {
		d.Access.Advertiser.Apply(document.Effective.LANDiscoveryEnabled)
	}
	return connectivity.ProjectPolicy(document), nil
}

// ConnectionDocument lets sign-in show a quiet HTTP note and lets owners see
// the address actually used by rate limits and the home-network policy.
type ConnectionDocument struct {
	Secure               bool     `json:"secure"`
	Remote               bool     `json:"remote"`
	PlainHTTPOutsideHome bool     `json:"plainHttpOutsideHome"`
	ClientAddress        string   `json:"clientAddress"`
	AccessURLs           []string `json:"accessUrls"`
}

func (d Dependencies) connectionDocument(r *http.Request) ConnectionDocument {
	secure, remote := d.secureTransport(r), d.requestIsRemote(r)
	out := ConnectionDocument{Secure: secure, Remote: remote, PlainHTTPOutsideHome: !secure && remote, ClientAddress: d.setupPeerSubject(r), AccessURLs: []string{}}
	if setting, err := d.connectionSetting(r.Context()); err == nil && setting != nil {
		out.AccessURLs = append(out.AccessURLs, setting.accessURLs...)
	}
	return out
}

type ConnectivityStatusDocument struct {
	connectivity.Status
	Connection ConnectionDocument `json:"connection"`
}

func registerConnectivity(registry *apikit.Registry, d Dependencies) error {
	err := apikit.Register(registry, apikit.Route[struct{}, ConnectionDocument]{Metadata: apikit.Metadata{
		ID: "get_connection", Method: "GET", Path: "/v1/networking/connection", Summary: "Read the current connection and advertised addresses", Access: apikit.Public, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: []string{"internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (ConnectionDocument, error) {
		return d.connectionDocument(r), nil
	}})
	if err != nil {
		return err
	}
	err = apikit.Register(registry, apikit.Route[struct{}, connectivity.Policy]{Metadata: apikit.Metadata{
		ID: "get_connectivity_policy", Method: "GET", Path: "/v1/admin/connectivity/policy", Summary: "Read home networks and connection settings", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_permitted", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (connectivity.Policy, error) {
		document, err := d.settingsDocument(ctx)
		if err != nil {
			return connectivity.Policy{}, foundationAdministrationError(err)
		}
		return connectivity.ProjectPolicy(document), nil
	}})
	if err != nil {
		return err
	}
	err = apikit.Register(registry, apikit.Route[ConnectivityChange, connectivity.Policy]{Metadata: apikit.Metadata{
		ID: "change_connectivity_policy", Method: "PATCH", Path: "/v1/admin/connectivity/policy", Summary: "Set home networks and connection settings", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, BodyLimit: 32768, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "revision_mismatch", "request_in_progress", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, body ConnectivityChange) (connectivity.Policy, error) {
		out, err := d.changeConnectivity(r, body)
		if err != nil {
			return connectivity.Policy{}, foundationAdministrationError(err)
		}
		return out, nil
	}})
	if err != nil {
		return err
	}
	return apikit.Register(registry, apikit.Route[struct{}, ConnectivityStatusDocument]{Metadata: apikit.Metadata{
		ID: "get_connectivity_status", Method: "GET", Path: "/v1/admin/connectivity/status", Summary: "Read certificates and observed connection addresses", Access: apikit.Admin, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_permitted", "internal"},
	}, Handler: func(ctx context.Context, r *http.Request, _ struct{}) (ConnectivityStatusDocument, error) {
		document, err := d.settingsDocument(ctx)
		if err != nil {
			return ConnectivityStatusDocument{}, foundationAdministrationError(err)
		}
		reporter := connectivity.Reporter{Advertiser: d.Access.Advertiser, Certificates: d.certificateStatus(), Custom: d.customCertificateStatus()}
		return ConnectivityStatusDocument{Status: reporter.Report(ctx, connectivity.ProjectPolicy(document)), Connection: d.connectionDocument(r)}, nil
	}})
}

// certificateStatus adapts the networking claim handler's certificate manager
// into the connectivity report's seam. A server without networking configured
// reports no TLS rather than failing the whole report.
func (d Dependencies) certificateStatus() func(context.Context) (networking.CertificateStatus, error) {
	if d.Networking == nil {
		return nil
	}
	manager := d.Networking.Certificates()
	if manager == nil {
		return nil
	}
	return manager.Status
}

// customCertificateStatus adapts the owner-supplied certificate into the
// report's optional seam. Nil leaves the custom section out of the report.
func (d Dependencies) customCertificateStatus() func(context.Context) networking.CustomCertificateStatus {
	if d.CustomCertificate == nil {
		return nil
	}
	custom := d.CustomCertificate
	return func(ctx context.Context) networking.CustomCertificateStatus { return custom.Status(ctx) }
}
