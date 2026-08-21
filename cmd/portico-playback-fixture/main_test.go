package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteManifestIsPrivateAndMachineReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "fixture.json")
	want := manifest{Schema: fixtureSchema, BaseURL: "http://127.0.0.1:1234", Owner: credential{Login: "owner", Password: "secret"}, Media: map[string]string{"direct": "fixture-direct"}}
	if err := writeManifest(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode=%o", info.Mode().Perm())
	}
}

func TestFaultGateIsSecretExactPathAndOneShot(t *testing.T) {
	gate := &faultGate{secret: "secret"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := gate.handler(next)
	arm := httptest.NewRequest(http.MethodPost, fixtureControlPath, bytes.NewBufferString(`{"path":"/api/media/item/stream","status":410}`))
	arm.RemoteAddr = "127.0.0.1:1234"
	arm.Header.Set("Authorization", "Bearer secret")
	armRec := httptest.NewRecorder()
	h.ServeHTTP(armRec, arm)
	if armRec.Code != http.StatusOK {
		t.Fatalf("arm status=%d", armRec.Code)
	}
	for index, want := range []int{http.StatusNoContent, http.StatusGone, http.StatusNoContent} {
		path := "/api/media/other/stream"
		if index > 0 {
			path = "/api/media/item/stream"
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Fatalf("request %d status=%d want=%d", index, rec.Code, want)
		}
		if want == http.StatusGone && rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("one-shot fault was cacheable: %q", rec.Header().Get("Cache-Control"))
		}
	}
}

func TestFaultGateRejectsNonLoopbackAndInvalidScope(t *testing.T) {
	gate := &faultGate{secret: "secret"}
	for _, tc := range []struct{ remote, body string }{{"192.0.2.4:99", `{"path":"/api/media/x/stream","status":404}`}, {"127.0.0.1:99", `{"path":"/not-api","status":404}`}} {
		req := httptest.NewRequest(http.MethodPost, fixtureControlPath, bytes.NewBufferString(tc.body))
		req.RemoteAddr = tc.remote
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		gate.handler(http.NotFoundHandler()).ServeHTTP(rec, req)
		if rec.Code < 400 {
			t.Fatalf("remote=%s unexpectedly accepted", tc.remote)
		}
	}
}

func TestRandomPasswordIsNotStableCredential(t *testing.T) {
	a, err := randomPassword()
	if err != nil {
		t.Fatal(err)
	}
	b, err := randomPassword()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 20 {
		t.Fatalf("weak fixture credentials")
	}
}
