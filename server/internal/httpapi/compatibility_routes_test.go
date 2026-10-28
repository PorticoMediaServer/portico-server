package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func compatibilityCall(f administrationFixture, method, path, secret, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if secret != "" {
		r.Header.Set("Authorization", `MediaBrowser Client="Test", Device="Fixture", DeviceId="fixture", Version="1", Token="`+secret+`"`)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func TestCompatibilityDisabledPublicAndDeclaredAliasBoundary(t *testing.T) {
	f := administrationFixtureNew(t)
	for _, path := range []string{"/System/Info/Public", "/system/info/public", "/mediabrowser/System/Info/Public"} {
		w := compatibilityCall(f, "GET", path, "", "")
		if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("disabled compatibility %s: %d", path, w.Code)
		}
	}
	if _, err := f.deps.DB.Exec(`INSERT INTO server_settings(singleton,"integrations.compatibilityEnabled") VALUES(1,1) ON CONFLICT(singleton) DO UPDATE SET "integrations.compatibilityEnabled"=1`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/System/Info/Public", "/system/info/public", "/mediabrowser/System/Info/Public"} {
		w := compatibilityCall(f, "GET", path, "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"ProductName":"Portico"`) {
			t.Fatalf("public compatibility %s: %d %s", path, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/Videos/anything/stream", "/socket", "/Sessions/Playing/Progress"} {
		w := compatibilityCall(f, "POST", path, "", `{}`)
		if w.Code == 200 {
			t.Fatalf("undeclared player API unexpectedly succeeded: %s", path)
		}
	}
}
func TestCompatibilityTokenHeadersUseNativeScopesAndKeyRecovery(t *testing.T) {
	f := integrationFixture(t)
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"compat-key","name":"Tools","scopes":["read"]}`)
	for _, path := range []string{"/System/Info", "/system/info", "/mediabrowser/System/Info", "/System/Configuration"} {
		w := compatibilityCall(f, "GET", path, key.Secret, "")
		if w.Code != 200 {
			t.Fatalf("info alias: %s %d %s", path, w.Code, w.Body)
		}
	}
	r := httptest.NewRequest("GET", "/System/Info?api_key="+key.Secret, nil)
	r.Header.Set("X-Emby-Token", key.Secret)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("legacy proof: %d %s", w.Code, w.Body)
	}
	r = httptest.NewRequest("GET", "/System/Info", nil)
	r.Header.Set("Authorization", `MediaBrowser Token="`+key.Secret+`"`)
	r.Header.Set("X-MediaBrowser-Token", key.Secret)
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("identical modern+legacy proof refused: %d", w.Code)
	}
	w = compatibilityCall(f, "POST", "/Library/Media/Updated", key.Secret, `{"Updates":[{"Path":"/media/film","UpdateType":"Created"}]}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read key wrote library: %d %s", w.Code, w.Body)
	}
	w = compatibilityCall(f, "GET", "/Auth/Keys", key.Secret, "")
	if w.Code != 200 {
		t.Fatalf("key recovery: %d %s", w.Code, w.Body)
	}
	var keys struct {
		Items []struct{ AccessToken string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &keys); err != nil || len(keys.Items) != 1 || keys.Items[0].AccessToken != key.Secret {
		t.Fatal("foreign key recovery mismatch")
	}
}
