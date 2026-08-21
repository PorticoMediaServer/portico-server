package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestRequestBodyDeadlineUsesOperationSpecificBudgets(t *testing.T) {
	server := &Server{cfg: config.Config{RestoreIOTimeout: 3 * time.Minute}}
	assertBudget := func(path string, wantMinimum time.Duration) {
		t.Helper()
		observed := make(chan time.Duration, 1)
		handler := server.requestBodyDeadline(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			if !ok {
				t.Fatal("request body context has no deadline")
			}
			observed <- time.Until(deadline)
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodPost, path, &countingBody{remaining: 1})
		req.ContentLength = 1
		handler.ServeHTTP(httptest.NewRecorder(), req)
		budget := <-observed
		if budget < wantMinimum {
			t.Fatalf("%s body budget %s is below %s", path, budget, wantMinimum)
		}
	}
	assertBudget("/api/backups/restore/upload", 2*time.Minute)
	assertBudget("/api/settings", 20*time.Second)
}

type countingBody struct {
	remaining int
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, context.Canceled
	}
	b.remaining--
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = 'x'
	return 1, nil
}

func (b *countingBody) Close() error { return nil }
