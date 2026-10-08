package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyClientAddress(t *testing.T) {
	trusted, e := ParseTrustedProxies("127.0.0.1/32,::1/128")
	if e != nil {
		t.Fatal(e)
	}
	h := &Handler{TrustedProxies: trusted}
	for _, test := range []struct {
		name, remote string
		headers      []string
		want         string
		fail         bool
	}{
		{"trusted", "127.0.0.1:1234", []string{"203.0.113.7"}, "203.0.113.7", false},
		{"untrusted spoof", "192.0.2.1:1234", []string{"203.0.113.7"}, "192.0.2.1", false},
		{"chain", "127.0.0.1:1234", []string{"203.0.113.7, 192.0.2.3"}, "", true},
		{"duplicates", "127.0.0.1:1234", []string{"203.0.113.7", "203.0.113.7"}, "", true},
		{"invalid peer", "bad", nil, "", true},
		{"forwarded port", "127.0.0.1:1234", []string{"203.0.113.7:1234"}, "", true},
		{"v6", "[::1]:1234", []string{"2001:db8::7"}, "2001:db8::7", false},
		{"missing header", "127.0.0.1:1234", nil, "127.0.0.1", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/system", nil)
			r.RemoteAddr = test.remote
			for _, v := range test.headers {
				r.Header.Add("X-Forwarded-For", v)
			}
			got, e := h.clientAddress(r)
			if (e != nil) != test.fail || got != test.want {
				t.Fatalf("got %q %v", got, e)
			}
		})
	}
	r := httptest.NewRequest("GET", "/v1/system", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	got, e := (&Handler{}).clientAddress(r)
	if e != nil || got != "127.0.0.1" {
		t.Fatal("proxy trusted by default")
	}
}
func TestTrustedProxyConfigurationAndMiddleware(t *testing.T) {
	for _, value := range []string{"0.0.0.0/0", "::/0", "10.0.0.0/8", "127.0.0.1/7", "localhost", "::ffff:127.0.0.1/128"} {
		if _, e := ParseTrustedProxies(value); e == nil {
			t.Errorf("accepted %s", value)
		}
	}
	prefixes, _ := ParseTrustedProxies("127.0.0.1/32")
	h := (&Handler{TrustedProxies: prefixes}).Routes()
	r := httptest.NewRequest("GET", "/v1/system", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Add("X-Forwarded-For", "203.0.113.1")
	r.Header.Add("X-Forwarded-For", "203.0.113.2")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestAuthLimiterUsesResolvedClientAddress(t *testing.T) {
	prefixes, _ := ParseTrustedProxies("127.0.0.1/32")
	h := &Handler{TrustedProxies: prefixes}
	request := func(ip string) *http.Request {
		r := httptest.NewRequest("POST", "/v1/sessions", nil)
		r.RemoteAddr = "127.0.0.1:2222"
		r.Header.Set("X-Forwarded-For", ip)
		resolved, e := h.clientAddress(r)
		if e != nil {
			t.Fatal(e)
		}
		return withClientAddress(r, resolved)
	}
	first := request("203.0.113.1")
	for range 30 {
		if !h.allowAuth(first) {
			t.Fatal("quota early")
		}
	}
	if h.allowAuth(first) {
		t.Fatal("quota missing")
	}
	if !h.allowAuth(request("203.0.113.2")) {
		t.Fatal("different forwarded client shares quota")
	}
}
