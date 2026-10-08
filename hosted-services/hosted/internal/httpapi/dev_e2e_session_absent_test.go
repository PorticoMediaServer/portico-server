//go:build !devtrust || release

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A build without the devtrust tag, and every release build, has no
// password-free development sign-in: /dev/e2e/session is not a route.
func TestDevE2ESessionAbsentOutsideDevtrust(t *testing.T) {
	if len(devRoutes) != 0 || len(devQueryPaths) != 0 {
		t.Fatalf("development routes in a non-devtrust build: %d routes, %d query paths", len(devRoutes), len(devQueryPaths))
	}
	h := &Handler{DevelopmentCookies: true, Origins: map[string]bool{"http://127.0.0.1:5173": true}}
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dev/e2e/session", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /dev/e2e/session answered %d, want 404", w.Code)
	}
}
