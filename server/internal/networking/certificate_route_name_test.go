package networking

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A4: Hosted names the route with the member label, c-<label>.<namespace>, and
// leaves port 443 implicit. The certificate manager refused both, so remote
// certificates never got a route on a current Hosted.
func TestCertificateRouteAcceptsTheMemberLabelName(t *testing.T) {
	for _, port := range []int{443, 32500} {
		f, m, _, _, _, ctx := routeRaceFixture(t)
		cert := m.h.certificates
		if _, e := f.db.Exec(`UPDATE networking_certificate_settings SET public_port=?`, port); e != nil {
			t.Fatal(e)
		}
		c, q, _, e := cert.snapshot(ctx)
		if e != nil {
			t.Fatal(e)
		}
		host := "c-0123456789abcdef0123456789abcdef." + q.Namespace + ".direct.getportico.tv"
		base := "https://" + host
		if port != 443 {
			base += ":32500"
		}
		cert.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/direct-route") {
				raw, _ := json.Marshal(certificateRoute{Namespace: q.Namespace, Hostname: host, BaseURL: base, CertificateDNSName: "*." + q.Namespace + ".direct.getportico.tv"})
				return reply(200, string(raw)), nil
			}
			return reply(503, `{"code":"unavailable"}`), nil
		})
		cert.updateRoute(ctx, q, c)
		var stored, errorCode string
		if e := f.db.QueryRow(`SELECT route_hostname,route_error_code FROM networking_certificate_state WHERE scope_id=?`, q.Scope.ID).Scan(&stored, &errorCode); e != nil {
			t.Fatal(e)
		}
		if stored != host || errorCode != "" {
			t.Fatalf("port %d: route %q error %q", port, stored, errorCode)
		}
	}
}
