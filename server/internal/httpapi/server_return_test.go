package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisteredReturnHTTPAuthorityAndPrivateFailures(t *testing.T) {
	f := administrationFixtureNew(t)
	const code = "src_private-disposable-fixture-code-01234567890123456789"
	for _, kind := range []string{"missing-installation", "mismatched-installation", "untrusted-setup-host"} {
		t.Run(kind, func(t *testing.T) {
			body := map[string]any{"code": code, "transactionId": "srt_fixture", "verifier": strings.Repeat("v", 43), "registrationId": "ret_fixture", "serverChallenge": strings.Repeat("c", 32), "installationId": strings.Repeat("i", 43), "purpose": "setup", "requestId": "33333333-3333-4333-8333-333333333333", "setupToken": "private-setup-fixture", "expectedRevision": "0"}
			raw, _ := json.Marshal(body)
			request := httptest.NewRequest("POST", "http://localhost:32500/v1/auth/portico-return/exchange", strings.NewReader(string(raw)))
			request.RemoteAddr = "192.168.1.20:1000"
			request.Header.Set("Content-Type", "application/json")
			if kind == "mismatched-installation" {
				request.Header.Set("X-Portico-Installation-Id", strings.Repeat("j", 43))
			}
			if kind == "untrusted-setup-host" {
				request.Host = "rebinding.attacker.test:32500"
				request.Header.Set("Origin", "http://rebinding.attacker.test:32500")
				request.Header.Set("X-Portico-Installation-Id", strings.Repeat("i", 43))
			}
			response := httptest.NewRecorder()
			f.handler.ServeHTTP(response, request)
			if response.Code >= 200 && response.Code < 300 || response.Code >= 500 {
				t.Fatalf("authority failure status %d", response.Code)
			}
			if strings.Contains(response.Body.String(), code) || strings.Contains(response.Body.String(), "private-setup-fixture") {
				t.Fatal("private failure disclosed continuation")
			}
			assertSpecResponse(t, "POST", "/v1/auth/portico-return/exchange", response)
			var owners int
			if e := f.deps.DB.QueryRow(`SELECT count(*) FROM direct_memberships WHERE role='owner' AND disabled=0`).Scan(&owners); e != nil || owners != 1 {
				t.Fatal("failed return changed accounts", e)
			}
		})
	}
}
