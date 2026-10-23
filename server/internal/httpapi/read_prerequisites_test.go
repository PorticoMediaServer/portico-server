package httpapi

import (
	"portico.local/server/internal/catalog"
	"testing"
)

func TestReadPrerequisitesCurrentLibraryHTTPContract(t *testing.T) {
	f := newV1Fixture(t, 0)
	for _, kind := range []string{"audiobook", "book", "comic"} {
		var created catalog.Library
		w := f.call("POST", "/v1/libraries", nil, map[string]any{"name": kind, "kind": kind, "path": t.TempDir()}, 201, &created)
		assertSpecResponse(t, "POST", "/v1/libraries", w)
		if created.Kind != kind || created.Capabilities == nil || created.Capabilities.Reading {
			t.Fatalf("creation promises a reader: %+v", created)
		}
		if kind == "audiobook" {
			if !created.Capabilities.TV || created.Capabilities.CardShape != "square" {
				t.Fatalf("audiobook lost identity: %+v", created)
			}
		} else if created.Capabilities.TV || created.Capabilities.CardShape != "portrait" {
			t.Fatalf("Read media capabilities: %+v", created)
		}
	}
	var directory struct {
		Items []catalog.Library `json:"items"`
	}
	w := f.call("GET", "/v1/libraries", nil, nil, 200, &directory)
	assertSpecResponse(t, "GET", "/v1/libraries", w)
	var startup BootstrapDocument
	w = f.call("GET", "/v1/bootstrap?deviceClass=television", nil, nil, 200, &startup)
	assertSpecResponse(t, "GET", "/v1/bootstrap", w)
	if len(directory.Items) != 4 || len(startup.Libraries) != 4 {
		t.Fatalf("directory/bootstrap: %d/%d", len(directory.Items), len(startup.Libraries))
	}
	for _, items := range [][]catalog.Library{directory.Items, startup.Libraries} {
		for _, library := range items {
			if library.Capabilities == nil || library.Capabilities.Reading {
				t.Fatalf("missing or invented support: %+v", library)
			}
			if (library.Kind == "book" || library.Kind == "comic") && library.Capabilities.TV {
				t.Fatal("Read advertised on TV")
			}
		}
	}
	if w := f.raw("GET", "/v1/libraries", "", nil, nil); w.Code != 401 {
		t.Fatalf("anonymous directory: %d", w.Code)
	}
}
