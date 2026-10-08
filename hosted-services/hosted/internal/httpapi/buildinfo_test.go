package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"portico.local/hosted/internal/buildinfo"
	"testing"
)

func TestPublicBuildIdentityDoesNotRequireApplicationState(t *testing.T) {
	// A nil database, identity, directory and key store are deliberate: this route
	// must never touch application state or expose credentials/readiness details.
	h := (&Handler{}).Routes()
	r := httptest.NewRequest("GET", "/v1/system", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var info map[string]string
	if e := json.Unmarshal(w.Body.Bytes(), &info); e != nil {
		t.Fatal(e)
	}
	expected := buildinfo.Info()
	if len(info) != len(expected) {
		t.Fatalf("unexpected public fields: %#v", info)
	}
	for k, v := range expected {
		if info[k] != v {
			t.Errorf("%s differs", k)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("build identity cached")
	}
}
