package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"portico.local/apikit"
	"portico.local/server/internal/playback"
	"strings"
	"testing"
)

func TestConversionAdmissionIsTypedRetryable(t *testing.T) {
	w := httptest.NewRecorder()
	failure(w, playback.ErrConversionCapacity)
	if w.Code != 429 || w.Header().Get("Retry-After") != "2" || !strings.Contains(w.Body.String(), `"code":"conversion_capacity"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPlaybackV1ConversionCapacityUsesRetryableEnvelope(t *testing.T) {
	r := apikit.New(nil, v1Catalogue())
	v1Route(r, apikit.Metadata{ID: "capacity", Method: "POST", Path: "/v1/capacity", Summary: "Capacity", Access: apikit.Public, Requires: apikit.ScopeOpen, Lane: apikit.Media, Admission: lanePlayback, Cost: apikit.Constant, BodyLimit: 1024, Status: 201, Errors: []string{"conversion_capacity"}}, func(context.Context, *http.Request, noBody) (noBody, error) {
		return noBody{}, fmt.Errorf("conversion admission: %w", playback.ErrConversionCapacity)
	})
	request := httptest.NewRequest("POST", "/v1/capacity", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code != 429 || w.Header().Get("Retry-After") != "2" || !strings.Contains(w.Body.String(), `"code":"conversion_capacity"`) || !strings.Contains(w.Body.String(), `"retry":"same_request"`) {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}
