package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"portico.local/server/internal/operations"
)

func TestSettingsConfigurationHTTPAuthorityAndSourceFacts(t *testing.T) {
	f := newTL11AdmissionFixture(t, "movie")
	handler := New(f.d)
	out := logoutHTTPRequest(handler, "GET", "/v1/admin/settings/configuration", f.token)
	if out.Code != http.StatusOK {
		t.Fatalf("owner configuration %d %s", out.Code, out.Body.String())
	}
	var doc ConsoleReply[operations.SettingsConfiguration]
	if err := json.Unmarshal(out.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Scope.ServerID == "" || doc.Data.Revision < 1 || len(doc.Data.Fields) < 30 {
		t.Fatal(doc)
	}
	for _, field := range doc.Data.Fields {
		if field.ID == "streaming.uploadSpeed" && (field.Current != nil || field.Default != nil || field.Source != "default") {
			t.Fatal("upload invented", field)
		}
		if field.ID == "remote.ownCertificate" && (field.Current == nil || *field.Current != false) {
			t.Fatal("own route starts enabled", field)
		}
	}
	if denied := logoutHTTPRequest(handler, "GET", "/v1/admin/settings/configuration", ""); denied.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous config %d %s", denied.Code, denied.Body.String())
	}
}
