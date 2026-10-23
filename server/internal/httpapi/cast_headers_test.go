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
		for _, path := range []string{"/receiver/cast/", "/receiver/cast/index.html", "/receiver/cast/receiver.js", "/receiver/cast/receiver-text.js", "/receiver/cast/not-found"} {
			t.Run(scheme+path, func(t *testing.T) {
				r := httptest.NewRequest("GET", scheme+"://portico.test"+path, nil)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				wantStatus, wantPolicy := 200, castReceiverCSP
				if path == "/receiver/cast/" || path == "/receiver/cast/index.html" {
					wantPolicy = castReceiverPageCSP
				}
				if path == "/receiver/cast/index.html" {
					wantStatus = 301
				}
				if path == "/receiver/cast/not-found" {
					wantStatus = 404
				}
				if w.Code != wantStatus {
					t.Fatal(w.Code, w.Body.String())
				}
				if w.Header().Get("Content-Security-Policy") != wantPolicy {
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
	const shaka = " https://ajax.googleapis.com/ajax/libs/shaka-player/4.15.56/"
	if strings.Replace(castReceiverPageCSP, shaka, "", 1) != castReceiverCSP || strings.Contains(castReceiverCSP, "googleapis") {
		t.Fatal("only the receiver document gains the checked Google script path")
	}
	for _, directive := range strings.Split(castReceiverPageCSP, ";") {
		if strings.Contains(directive, "script-src") && (strings.Contains(directive, "unsafe-") || strings.Contains(directive, "*")) {
			t.Fatal(directive)
		}
	}
}
