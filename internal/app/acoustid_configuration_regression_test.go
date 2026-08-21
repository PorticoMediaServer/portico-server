package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAcoustIDLookupFailsClosedWithoutAPIKey(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(endpoint.Close)

	server := newScannerTestServer(t)
	server.cfg.AcoustIDAPIKey = ""
	server.cfg.AcoustIDBaseURL = endpoint.URL
	_, err := server.lookupAcoustID(context.Background(), fpcalcResult{Duration: 180, Fingerprint: "fingerprint"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("lookup error = %v, expected missing-key failure", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("lookup made %d provider request(s) without an API key", got)
	}
}
