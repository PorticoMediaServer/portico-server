package postgres_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

func TestLifecycleHTTPRoutes(t *testing.T) {
	f := setup(t)
	sec := &postgres.AccountSecurity{Store: f.store, Keys: f.keys, Identity: f.ids, Lifetime: 15 * time.Minute}
	h := (&httpapi.Handler{Identity: f.ids, Directory: f.dir, Security: sec}).Routes()
	call := func(method, path, token, body string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	call("POST", "/v1/profiles", f.member.AccessToken, `{"name":"Guest"}`, 404)
	call("GET", "/v1/account/profiles", f.member.AccessToken, "", 404)
	call("POST", "/v1/account/name", f.member.AccessToken, `{"displayName":"Changed"}`, 204)
	intent := identity.SecurityIntent{Purpose: "password_set_or_change", OperationID: security.RandomID("")}
	proof, err := sec.PasswordProof(context.Background(), f.member.AccessToken, intent, "StrongPass9", "")
	if err != nil {
		t.Fatal(err)
	}
	change, err := json.Marshal(postgres.SecurityAction{SecurityIntent: intent, Proof: proof.Proof, NewPassword: "Replacement7"})
	if err != nil {
		t.Fatal(err)
	}
	call("POST", "/v1/account/password", f.member.AccessToken, string(change), 200)
	call("GET", "/v1/me", f.member.AccessToken, "", 401)
}
