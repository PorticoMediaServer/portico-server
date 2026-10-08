package httpapi

import (
	"net/http/httptest"
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
