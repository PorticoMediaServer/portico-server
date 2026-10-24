package httpapi

import (
	"os"
	"path/filepath"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/catalog"
	"strings"
	"testing"
)

func TestLibrarySettingsHTTPUsesRealPolicyAndFolderCRUD(t *testing.T) {
	f := agentTestFixture(t)
	root := t.TempDir()
	var created catalog.Library
	f.call("POST", "/v1/libraries", nil, map[string]any{"name": "Films", "kind": "movie", "folders": []string{root}, "metadataAgent": "local"}, 201, &created)
	path := "/v1/admin/libraries/" + created.ID
	var doc administration.LibraryDocument
	response := f.call("GET", path+"/settings", nil, nil, 200, &doc)
	assertSpecResponse(t, "GET", "/v1/admin/libraries/{id}/settings", response)
	if len(doc.Sources) != 1 || doc.Sources[0].Path != root || !doc.Settings.GroupCollections {
		t.Fatal(doc)
	}
	doc.Settings.GroupCollections = false
	doc.Settings.LocalTags = "off"
	doc.Settings.AllowMediaDeletion = true
	response = f.call("PUT", path+"/settings", nil, administration.Change[administration.LibrarySettings]{ExpectedRevision: doc.Revision, OperationID: "http-library-settings-save", Settings: doc.Settings}, 200, &doc)
	assertSpecResponse(t, "PUT", "/v1/admin/libraries/{id}/settings", response)
	if doc.Settings.GroupCollections || doc.Settings.LocalTags != "off" || !doc.Settings.AllowMediaDeletion {
		t.Fatal(doc.Settings)
	}
	if w := f.raw("PUT", path+"/settings", f.owner.AccessToken, nil, map[string]any{"expectedRevision": doc.Revision, "operationId": "invalid-library-settings", "settings": map[string]any{"lyricsSource": "made_up"}}); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	other := t.TempDir()
	var cfg struct {
		Revision int64 `json:"revision"`
	}
	f.call("GET", path+"/inventory-config", nil, nil, 200, &cfg)
	var folder catalog.LibrarySource
	f.call("POST", path+"/sources", nil, catalog.SourceSettings{Name: "", Path: other, Classification: "local", IntervalSeconds: 3600, ExpectedRevision: cfg.Revision}, 200, &folder)
	if folder.Name != filepath.Base(other) || folder.Path != other || folder.IntervalSeconds != 3600 || folder.Kind != "local" {
		t.Fatal(folder)
	}
	f.call("GET", path+"/inventory-config", nil, nil, 200, &cfg)
	f.call("PATCH", path+"/sources/"+folder.ID, nil, catalog.SourceSettings{Name: "Archive", Path: other, Classification: "network", IntervalSeconds: 900, ExpectedRevision: cfg.Revision}, 200, &folder)
	if folder.Name != "Archive" || folder.IntervalSeconds != 900 || folder.Classification != "network" {
		t.Fatal(folder)
	}
	member, _ := f.member()
	for _, probe := range []struct {
		method, path string
		body         any
	}{
		{"GET", path + "/settings", nil}, {"PUT", path + "/settings", map[string]any{}}, {"GET", "/v1/admin/missing-titles", nil}, {"POST", "/v1/admin/missing-titles/empty", map[string]any{}},
	} {
		w := f.raw(probe.method, probe.path, member, nil, probe.body)
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("member reached management %d %s", w.Code, w.Body.String())
		}
	}
	listing := f.raw("GET", "/v1/libraries", f.owner.AccessToken, nil, nil)
	if listing.Code != 200 || strings.Contains(listing.Body.String(), root) || strings.Contains(listing.Body.String(), other) {
		t.Fatal("viewer paths", listing.Code, listing.Body.String())
	}
	entries, err := os.ReadDir(other)
	if err != nil || len(entries) != 0 {
		t.Fatal("folder CRUD touched media", entries, err)
	}
}
