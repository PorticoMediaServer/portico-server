package httpapi

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/server/internal/dbwork"
)

// A write the writer turned away, or one that met a lock after admission,
// saved nothing. Until the typed writer_capacity answer lands it keeps the
// existing retryable busy refusal, never "the request could not be saved".
func TestWriterRefusalKeepsTheRetryableBusyAnswer(t *testing.T) {
	for _, err := range []error{
		dbwork.ErrWriterCapacity,
		fmt.Errorf("save playlist: %w", dbwork.ErrWriterCapacity),
		errors.New("database is locked (5) (SQLITE_BUSY)"),
	} {
		w := httptest.NewRecorder()
		failure(w, err)
		body := w.Body.String()
		if w.Code != 503 || !strings.Contains(body, `"code":"server_busy"`) || !strings.Contains(body, `"retryable":true`) || w.Header().Get("Retry-After") != "1" {
			t.Fatalf("%v answered %d %q Retry-After=%q", err, w.Code, body, w.Header().Get("Retry-After"))
		}
	}
}
