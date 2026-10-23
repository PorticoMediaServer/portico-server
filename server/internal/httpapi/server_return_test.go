package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"strings"
	"testing"
)

func TestRegisteredReturnServicesUnavailableIsPublicRetryableRefusal(t *testing.T) {
	w := httptest.NewRecorder()
	failure(w, hosted.ErrServicesUnavailable)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"portico_services_unavailable"`) || !strings.Contains(w.Body.String(), `"retry":"same_request"`) {
		t.Fatalf("offline refusal %d %s", w.Code, w.Body)
	}
	assertSpecResponse(t, "POST", "/v1/auth/portico-return/exchange", w)
}

func TestRegisteredReturnPreparationRejectsUnboundOriginsAndLocal(t *testing.T) {
	f := administrationFixtureNew(t)
	for _, q := range []struct{ origin, nonce, installation string }{
		{"https://evil.example", identity.Token(), identity.Token()},
		{"http://server.example:32500", identity.Token(), identity.Token()},
		{"https://server.example:32500/path", identity.Token(), identity.Token()},
		{"https://server.example:32500", "bad", identity.Token()},
		{"https://server.example:32500", identity.Token(), ""},
	} {
		raw, _ := json.Marshal(porticoReturnPrepareRequest{Origin: q.origin, Nonce: q.nonce})
		r := httptest.NewRequest("POST", "https://server.example:32500/v1/auth/portico-return/prepare", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Portico-Installation-Id", q.installation)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("unbound prepare %d %s", w.Code, w.Body)
		}
		assertSpecResponse(t, "POST", "/v1/auth/portico-return/prepare", w)
	}
	if _, e := f.deps.DB.Exec(`INSERT INTO identity_state(singleton,sign_in_kind) VALUES(1,'local')`); e != nil {
		t.Fatal(e)
	}
	w := f.call("POST", "/v1/auth/portico-return/prepare", "", `{"origin":"https://server.example:32500","nonce":"`+identity.Token()+`"}`)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "sign_in_kind_local") {
		t.Fatalf("Local prepare %d %s", w.Code, w.Body)
	}
	var challenges int
	if e := f.deps.DB.QueryRow(`SELECT count(*) FROM hosted_identity_challenges`).Scan(&challenges); e != nil || challenges != 0 {
		t.Fatal("failed prepare created challenges", e)
	}
}

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
