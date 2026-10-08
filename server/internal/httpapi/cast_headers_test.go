package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCastReceiverSecurityHeaders(t *testing.T) {
	d, _ := logoutHTTPFixture(t)
	h := New(d)
	for _, scheme := range []string{"http", "https"} {
		for _, path := range []string{"/receiver/cast/", "/receiver/cast/receiver.js"} {
			t.Run(scheme+path, func(t *testing.T) {
				r := httptest.NewRequest("GET", scheme+"://portico.test"+path, nil)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				if w.Header().Get("Content-Security-Policy") != castReceiverCSP {
					t.Fatal(w.Header())
				}
				if w.Header().Get("X-Frame-Options") != "" {
					t.Fatal("DENY overrides Cast exception")
				}
				if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal(w.Header())
				}
				if (w.Header().Get("Strict-Transport-Security") != "") != (scheme == "https") {
					t.Fatal(w.Header())
				}
			})
		}
	}
	for _, directive := range strings.Split(castReceiverCSP, ";") {
		if strings.Contains(directive, "script-src") && (strings.Contains(directive, "unsafe-") || strings.Contains(directive, "*")) {
			t.Fatal(directive)
		}
	}
}
