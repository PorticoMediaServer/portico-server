package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSetupProxyBoundary(t *testing.T) {
	proxies, err := ParseTrustedProxyCIDRs("127.0.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	d := Dependencies{TrustedProxies: proxies}
	for _, tc := range []struct {
		peer, forwarded string
		private, local  bool
	}{
		{"127.0.0.1:1234", "", false, false},
		{"127.0.0.1:1234", "198.51.100.2", false, false},
		{"127.0.0.1:1234", "192.168.1.2", true, false},
		{"127.0.0.1:1234", "127.0.0.1, 198.51.100.2", false, false},
		{"198.51.100.2:1234", "127.0.0.1", false, false},
	} {
		r := httptest.NewRequest("POST", "http://localhost/v1/setup/browser", nil)
		r.RemoteAddr = tc.peer
		if tc.forwarded != "" {
			r.Header.Set("X-Forwarded-For", tc.forwarded)
		}
		if got := d.privateSetupPeer(r); got != tc.private {
			t.Errorf("%+v private=%v", tc, got)
		}
		if got := d.sameMachineSetup(r); got != tc.local {
			t.Errorf("%+v local=%v", tc, got)
		}
	}
	r := httptest.NewRequest("POST", "http://localhost/v1/setup/browser", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	if !(Dependencies{}).sameMachineSetup(r) {
		t.Fatal("direct loopback refused")
	}
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	if (Dependencies{}).sameMachineSetup(r) {
		t.Fatal("forwarded loopback treated as direct")
	}
	if (Dependencies{}).privateSetupPeer(r) {
		t.Fatal("untrusted forwarded header treated as private")
	}
	r.Header.Del("X-Forwarded-For")
	r.Header.Set("Forwarded", "for=127.0.0.1")
	if (Dependencies{}).privateSetupPeer(r) {
		t.Fatal("untrusted Forwarded header treated as private")
	}
}
