package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"testing"
)

func TestBrowserSetupLocalOriginAndSingleUse(t *testing.T) {
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	host, err := hosted.New(db, id, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	h := New(Dependencies{DB: db, Identity: id, Catalog: catalog.New(db), Hosted: host, Origins: []string{"http://127.0.0.1:19412", "https://web.getportico.tv"}})
	call := func(address, origin, peer, path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", address+path, bytes.NewReader(raw))
		r.RemoteAddr = peer
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// Any peer and host may start setup (no setup code); only the browser origin
	// must be this server or a configured Portico origin.
	for _, tc := range []struct{ name, address, origin, peer string }{
		{"LAN peer", "http://127.0.0.1:32500", "http://127.0.0.1:32500", "192.168.1.2:5000"},
		{"public peer", "https://demo.example", "https://demo.example", "198.51.100.8:5000"},
		{"hosted origin", "http://127.0.0.1:32500", "https://web.getportico.tv", "127.0.0.1:5000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := call(tc.address, tc.origin, tc.peer, "/v1/setup/browser", map[string]any{}); w.Code != http.StatusOK {
				t.Fatalf("got status %d", w.Code)
			}
		})
	}
	for _, tc := range []struct{ name, address, origin, peer string }{
		{"cross-site origin", "http://127.0.0.1:32500", "http://evil.example", "127.0.0.1:5000"},
		{"missing origin", "http://127.0.0.1:32500", "", "127.0.0.1:5000"},
		{"unconfigured local origin", "http://127.0.0.1:32500", "http://localhost:9999", "127.0.0.1:5000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := call(tc.address, tc.origin, tc.peer, "/v1/setup/browser", map[string]any{}); w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
				t.Fatalf("got status %d", w.Code)
			}
		})
	}
	w := call("http://127.0.0.1:32500", "http://127.0.0.1:32500", "127.0.0.1:5000", "/v1/setup/browser", map[string]any{})
	if w.Code != 200 {
		t.Fatalf("bootstrap %d", w.Code)
	}
	var approval struct {
		ServerID string `json:"serverId"`
		Token    string `json:"setupToken"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &approval); err != nil || approval.ServerID != id.ID() || len(approval.Token) != 43 {
		t.Fatal("invalid bootstrap")
	}
	w = call("http://127.0.0.1:32500", "http://127.0.0.1:19412", "127.0.0.1:5000", "/v1/setup", map[string]any{"requestId": "33333333-3333-4333-8333-333333333333", "setupToken": approval.Token, "username": "owner", "password": "A-long-local-password", "name": "My media", "authMode": "local", "interactive": true})
	if w.Code != 201 {
		t.Fatalf("setup %d", w.Code)
	}
	var session identity.Envelope
	if err = json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:32500/v1/setup/state", nil)
	r.Header.Set("Authorization", "Bearer "+session.AccessToken)
	r.RemoteAddr = "127.0.0.1:5000"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var state identity.OnboardingState
	if err = json.Unmarshal(w.Body.Bytes(), &state); err != nil || w.Code != 200 || !state.Ready || state.Phase != "ready" || state.AuthMode != "local" || state.RecoveryOwner {
		t.Fatal("a local server is ready once its owner exists (no checklist), without a recovery-owner role")
	}
	w = call("http://127.0.0.1:32500", "http://127.0.0.1:32500", "127.0.0.1:5000", "/v1/setup/browser", map[string]any{})
	if w.Code != 409 {
		t.Fatalf("initialized server exposes bootstrap: %d", w.Code)
	}
}
