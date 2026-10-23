package receiver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedCastCatalogueIsServedAsLocalScript(t *testing.T) {
	// Use the same embedded filesystem and mount as the production receiver route.
	handler := http.StripPrefix("/receiver/", http.FileServerFS(Files()))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/receiver/cast/receiver-text.js", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "javascript") || !strings.Contains(response.Body.String(), "window.porticoCastText") {
		t.Fatalf("catalogue asset: status=%d type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/receiver/generate-cast-text.mjs", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("build-time producer must not be embedded: %d", response.Code)
	}
}
