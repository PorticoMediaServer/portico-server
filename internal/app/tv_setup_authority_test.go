package app

import (
	"net/http"
	"testing"
)

func TestLocalServerDoesNotExposeTVSetupCodeAuthority(t *testing.T) {
	serverURL := newAuthTestServer(t)
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/auth/tv-setup/sessions"},
		{http.MethodGet, "/api/auth/tv-setup/sessions/setup-1"},
		{http.MethodPost, "/api/auth/tv-setup/grants"},
		{http.MethodPost, "/api/auth/tv-setup/redeem"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			status, body := doJSON(t, http.DefaultClient, test.method, serverURL+test.path, map[string]string{}, nil)
			if status != http.StatusNotFound {
				t.Fatalf("removed local TV setup route status=%d body=%s", status, body)
			}
		})
	}
}
