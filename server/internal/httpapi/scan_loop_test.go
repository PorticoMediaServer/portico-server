package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/ingestion"
	"strings"
	"testing"
)

func TestTypedScanSettingsDiagnosticsAndChangeExpiry(t *testing.T) {
	d, owner := logoutHTTPFixture(t)
	d.Administration = administration.New(d.DB)
	d.Ingestion = ingestion.New(d.DB, d.Catalog, assets.Probe{})
	if _, err := d.DB.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('films','Films','movie','/films')`); err != nil {
		t.Fatal(err)
	}
	job, err := d.Ingestion.Queue("films")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.DB.Exec(`INSERT INTO inventory_scan_issues(job_id,relative_path,reason,line,warning) VALUES(?,'Folder/file.mkv','entry_unreadable',0,1)`, job.ID); err != nil {
		t.Fatal(err)
	}
	h := New(d)
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		out := httptest.NewRecorder()
		h.ServeHTTP(out, request)
		return out
	}
	path := "/v1/admin/libraries/films/scans/" + job.ID + "/issues"
	if response := call("GET", path, "", ""); response.Code != 401 {
		t.Fatalf("anonymous diagnostics: %d", response.Code)
	}
	if response := call("GET", path, "", owner.AccessToken); response.Code != 200 || !strings.Contains(response.Body.String(), `"messageKey":"scan.issue.entry_unreadable"`) {
		t.Fatalf("diagnostics: %d %s", response.Code, response.Body.String())
	}
	if response := call("GET", path+"?limit=129", "", owner.AccessToken); response.Code != 400 {
		t.Fatalf("unbounded issue list: %d %s", response.Code, response.Body.String())
	}
	response := call("GET", "/v1/admin/libraries/films/settings", "", owner.AccessToken)
	var document administration.LibraryDocument
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &document) != nil || !document.Settings.MonitorChanges {
		t.Fatalf("settings: %d %s", response.Code, response.Body.String())
	}
	document.Settings.MonitorChanges = false
	body, _ := json.Marshal(administration.Change[administration.LibrarySettings]{ExpectedRevision: document.Revision, OperationID: "monitor-setting-test", Settings: document.Settings})
	response = call("PUT", "/v1/admin/libraries/films/settings", string(body), owner.AccessToken)
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &document) != nil || document.Settings.MonitorChanges {
		t.Fatalf("save monitor: %d %s", response.Code, response.Body.String())
	}
	var stored string
	if err = d.DB.QueryRow(`SELECT body FROM admin_documents WHERE scope='library:films'`).Scan(&stored); err != nil || !strings.Contains(stored, `"monitorChanges":false`) {
		t.Fatal("monitor setting not persisted", err, stored)
	}
	if _, err = d.DB.Exec(`UPDATE library_sources SET changes_pruned_sequence=50 WHERE library_id='films'`); err != nil {
		t.Fatal(err)
	}
	var source string
	if err = d.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id='films'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	response = call("GET", fmt.Sprintf("/v1/admin/libraries/films/sources/%s/changes?after=1", source), "", owner.AccessToken)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"resetRequired":true`) {
		t.Fatalf("expired changes: %d %s", response.Code, response.Body.String())
	}
}
