//go:build devtrust && !release

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// In a devtrust build the route exists but answers nothing without the
// development cookie mode and a store.
func TestDevE2ESessionRefusesWithoutDevelopmentMode(t *testing.T) {
	if len(devRoutes) != 1 {
		t.Fatalf("devRoutes has %d entries, want 1", len(devRoutes))
	}
	w := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dev/e2e/session?account=bob", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("answered %d, want 404", w.Code)
	}
}
