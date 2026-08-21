package app

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

type fixedLiveTVResolver struct {
	addresses []netip.Addr
	err       error
}

func TestLiveTVSourceSecretIsAuthenticatedAndSourceBound(t *testing.T) {
	server := &Server{cfg: config.Config{AppDataDir: t.TempDir()}}
	envelope, err := server.sealLiveTVSourceSecret("source-a", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(envelope, liveTVSecretPrefix) || strings.Contains(envelope, "correct horse") {
		t.Fatalf("credential was not sealed: %q", envelope)
	}
	plaintext, err := server.openLiveTVSourceSecret("source-a", envelope)
	if err != nil || plaintext != "correct horse battery staple" {
		t.Fatalf("open secret: plaintext=%q err=%v", plaintext, err)
	}
	if _, err := server.openLiveTVSourceSecret("source-b", envelope); err == nil {
		t.Fatal("credential envelope was reusable for another source")
	}
}

func (r fixedLiveTVResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addresses, r.err
}

func TestLiveTVEndpointApprovalSupportsScopedLANAuthorities(t *testing.T) {
	cases := []string{"http://192.168.1.50:8080/playlist.m3u", "http://threadfin:34400/xmltv.xml", "http://[fd12:3456::9]:8080/guide.xml", "http://[fe80::9%25en0]:5004/stream"}
	for _, raw := range cases {
		approval, _, err := approveLiveTVEndpoint(raw, "playlist")
		if err != nil {
			t.Fatalf("approve %q: %v", raw, err)
		}
		if approval.Purpose != "playlist" {
			t.Fatalf("purpose=%q", approval.Purpose)
		}
	}
	if _, _, err := approveLiveTVEndpoint("http://[fe80::9]:5004/stream", "stream"); err == nil {
		t.Fatal("zone-less IPv6 link-local source accepted")
	}
}

func TestLiveTVEndpointApprovalRejectsEscapeMixedAnswersAndMetadata(t *testing.T) {
	approval, _, err := approveLiveTVEndpoint("http://threadfin:34400/playlist.m3u", "playlist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := approval.validateURL("http://evil:34400/child.m3u8"); err == nil {
		t.Fatal("authority escape accepted")
	}
	if _, err := approval.validateURL("http://user:pass@threadfin:34400/child.m3u8"); err == nil {
		t.Fatal("credential-bearing child accepted")
	}
	if _, err := resolveLiveTVApproval(context.Background(), approval, fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("172.18.0.4"), netip.MustParseAddr("203.0.113.7")}}); err == nil {
		t.Fatal("mixed public/private answers accepted")
	}
	if _, err := resolveLiveTVApproval(context.Background(), approval, fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("169.254.169.254")}}); err == nil {
		t.Fatal("metadata address accepted")
	}
}

func TestLiveTVRedirectKeepsApprovedIncidentAuthority(t *testing.T) {
	approval, _, err := approveLiveTVEndpoint("http://xteve:34400/playlist.m3u", "playlist")
	if err != nil {
		t.Fatal(err)
	}
	client, err := newApprovedLiveTVHTTPClient(context.Background(), approval, fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("172.18.0.5")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(mustLiveTVRequest(t, "http://xteve:34400/child.m3u8"), nil); err != nil {
		t.Fatalf("same authority redirect rejected: %v", err)
	}
	if err := client.CheckRedirect(mustLiveTVRequest(t, "http://169.254.169.254/latest/meta-data"), nil); err == nil {
		t.Fatal("redirect escape accepted")
	}
}

func mustLiveTVRequest(t *testing.T, raw string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
