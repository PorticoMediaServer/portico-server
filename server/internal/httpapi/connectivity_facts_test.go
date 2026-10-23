package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestConnectivityIdentityUsesCanonicalLocalSnapshot(t *testing.T) {
	f := administrationFixtureNew(t)
	ctx := context.Background()
	code, err := f.deps.localIdentityCode(ctx)
	if err != nil || code != "JJWY-C8J8-J26F" {
		t.Fatal("canonical local code mismatch", err)
	}
	// The historical configuration ID and own-address settings are not inputs.
	if _, err = f.deps.DB.Exec(`UPDATE configuration SET value='historical-id' WHERE key='id'`); err != nil {
		t.Fatal(err)
	}
	if code, err = f.deps.localIdentityCode(ctx); err != nil || code != "JJWY-C8J8-J26F" {
		t.Fatal("historical configuration replaced current key", err)
	}
	if _, err = f.deps.DB.Exec(`UPDATE identity_state SET sign_in_kind='portico' WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if code, err = f.deps.localIdentityCode(ctx); err != nil || code != "" {
		t.Fatal("Portico exposed a Local code", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = f.deps.localIdentityCode(cancelled); err == nil {
		t.Fatal("cancelled read accepted")
	}
}

func TestConnectivitySnapshotFactsAreNotAdvertisedOrSettingsValues(t *testing.T) {
	f := administrationFixtureNew(t)
	w := f.call("GET", "/v1/admin/connectivity/status", f.owner, "")
	var status ConnectivityStatusDocument
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.IdentityCode != "JJWY-C8J8-J26F" || status.LocalURL != "" {
		t.Fatalf("current snapshot failed: %d", w.Code)
	}
	d := f.deps
	d.LocalHTTPURL = "http://localhost:41234" // Actual-listener composition is checked by networking's producer tests.
	d.connectionPolicy = &connectionPolicyCache{}
	d.connectionPolicy.setting.Store(&connectionSettingValue{revision: 1, accessURLs: []string{"https://advertised.example"}, certificateDomain: "manual.example"})
	d.ListeningPort = 41234
	connection := d.connectionDocument(httptest.NewRequest("GET", "http://host/v1/networking/connection", nil))
	if connection.ManagedHTTPSOrigin != "" || len(connection.AccessURLs) != 2 {
		t.Fatal("advertised/manual addresses became checked managed HTTPS")
	}
	handler := New(d)
	r := httptest.NewRequest("GET", "http://host/v1/admin/connectivity/status", nil)
	r.Header.Set("Authorization", "Bearer "+f.owner)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, r)
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &status) != nil || status.LocalURL != d.LocalHTTPURL {
		t.Fatalf("listener fact missing: %d", out.Code)
	}
}
