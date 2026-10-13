package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"portico.local/server/internal/operations"
	"strings"
	"testing"
)

func TestConversionSettingsTypedContractAndOwnerAccess(t *testing.T) {
	d, session, _ := hostedAdministrationFixture(t, "owner")
	mux := http.NewServeMux()
	d.foundationRoutes(mux)
	read := supportRequest(mux, session.AccessToken, "/v1/admin/console/settings")
	if read.Code != 200 {
		t.Fatal(read.Code, read.Body.String())
	}
	var envelope ConsoleReply[operations.SettingsDocument]
	if err := json.Unmarshal(read.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	values := envelope.Data.Requested
	values.SessionStorageMiB = 8192
	body, _ := json.Marshal(operations.SettingsChange{ExpectedRevision: envelope.Data.Revision, IdempotencyKey: "conversion-settings-1", Values: values})
	request := httptest.NewRequest("PATCH", "/v1/admin/console/settings", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	written := httptest.NewRecorder()
	mux.ServeHTTP(written, request)
	if written.Code != 200 || !strings.Contains(written.Body.String(), `"sessionStorageMiB":8192`) {
		t.Fatal(written.Code, written.Body.String())
	}
	registry, err := FoundationRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]bool{}
	for _, definition := range registry.Definitions() {
		routes[definition.Method+" "+definition.Path] = true
	}
	for _, route := range []string{"GET /v1/console/registry", "GET /v1/admin/console/settings", "PATCH /v1/admin/console/settings"} {
		if !routes[route] {
			t.Fatal("missing typed contract", route)
		}
	}
}
func TestConversionSettingsCompleteDocumentRejectsOmissionAndNull(t *testing.T) {
	values := operations.DefaultSettings()
	raw, _ := json.Marshal(operations.SettingsChange{ExpectedRevision: 1, IdempotencyKey: "conversion-fields-1", Values: values})
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	fields := body["values"].(map[string]any)
	delete(fields, "hdrToneMapping")
	incomplete, _ := json.Marshal(body)
	var request ConsoleSettingsChange
	if json.Unmarshal(incomplete, &request) == nil {
		t.Fatal("omitted boolean silently defaulted")
	}
	fields["hdrToneMapping"] = nil
	null, _ := json.Marshal(body)
	if json.Unmarshal(null, &request) == nil {
		t.Fatal("null boolean accepted")
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal("complete default settings refused", err)
	}
}
