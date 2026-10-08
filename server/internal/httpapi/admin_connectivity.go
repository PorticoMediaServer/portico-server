package httpapi

import (
	"context"
	"net/http"

	"portico.local/server/internal/connectivity"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/operations"
)

// connectivityRoutes registers the connectivity policy and the status report.
//
// Tier: the policy is server security — who may sign in remotely, whether TLS is
// required, which networks count as the LAN — so reading and writing it is
// owner-only. The status report is admin-tier, because an administrator
// diagnosing "nobody can reach the server" needs to see it.
func (d Dependencies) connectivityRoutes(mux *http.ServeMux) {
	if d.Console == nil {
		return
	}
	mux.HandleFunc("GET /v1/admin/connectivity/policy", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			accessFailure(w, e)
			return
		}
		document, e := d.settingsDocument(r.Context())
		if e != nil {
			accessFailure(w, e)
			return
		}
		write(w, 200, connectivity.ProjectPolicy(document))
	})
	mux.HandleFunc("PATCH /v1/admin/connectivity/policy", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			accessFailure(w, e)
			return
		}
		var body struct {
			ExpectedRevision int64     `json:"expectedRevision"`
			OperationID      string    `json:"operationId"`
			RemoteSignIn     *string   `json:"remoteSignInPolicy"`
			RemoteBitrate    *int      `json:"remoteBitrateLimitKbps"`
			Secure           *string   `json:"secureConnectionsPolicy"`
			LANNetworks      *[]string `json:"lanNetworks"`
			AccessURLs       *[]string `json:"accessUrls"`
			LANDiscovery     *bool     `json:"lanDiscoveryEnabled"`
			TrustedProxies   *[]string `json:"trustedProxies"`
			TreatWANAsLAN    *bool     `json:"treatWanAsLan"`
			UploadCapacity   *int      `json:"uploadCapacityKbps"`
			PausedTimeout    *int      `json:"pausedSessionTimeoutMinutes"`
			Interface        *string   `json:"advertisedInterface"`
			CertificatePath  *string   `json:"customCertificatePath"`
			CertificateKey   *string   `json:"customCertificateKeyPath"`
			CertificateHost  *string   `json:"customCertificateDomain"`
		}
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		document, e := d.applySettings(r, p, body.ExpectedRevision, body.OperationID, func(v *operations.Settings) {
			if body.RemoteSignIn != nil {
				v.RemoteSignInPolicy = *body.RemoteSignIn
			}
			if body.RemoteBitrate != nil {
				v.RemoteBitrateLimitKbps = *body.RemoteBitrate
			}
			if body.Secure != nil {
				v.SecureConnectionsPolicy = *body.Secure
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
			if body.TreatWANAsLAN != nil {
				v.TreatWANAsLAN = *body.TreatWANAsLAN
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
		})
		if e != nil {
			accessFailure(w, e)
			return
		}
		// Discovery is a hot setting: the advertisement follows the save without
		// a restart, which is what the registry's application mode promises.
		d.Access.Advertiser.Apply(document.Effective.LANDiscoveryEnabled)
		// Evaluate the policy now so a claimed server with no working HTTPS
		// route raises the owner alert as soon as Required is saved.
		_, _, _ = d.requiredHTTPS(r.Context())
		write(w, 200, connectivity.ProjectPolicy(document))
	})
	mux.HandleFunc("GET /v1/admin/connectivity/status", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.administrator(r); e != nil {
			accessFailure(w, e)
			return
		}
		document, e := d.settingsDocument(r.Context())
		if e != nil {
			accessFailure(w, e)
			return
		}
		reporter := connectivity.Reporter{Advertiser: d.Access.Advertiser, Certificates: d.certificateStatus(), Custom: d.customCertificateStatus()}
		write(w, 200, reporter.Report(r.Context(), connectivity.ProjectPolicy(document)))
	})
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
