package httpapi

import (
	"net/http/httptest"
	"portico.local/server/internal/compactcatalog"
	"testing"
)

func TestPreparedMutationBusyUsesRegisteredRequestInProgress(t *testing.T) {
	w := httptest.NewRecorder()
	preparedFailure(w, compactcatalog.ErrEntityMutating)
	if w.Code != 409 || bodyErrorCode(w.Body.String()) != "request_in_progress" {
		t.Fatalf("custody was not a retryable domain refusal: %d %s", w.Code, w.Body.String())
	}
}
