package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// A stored browse with no filter must hydrate through the endpoint published
// by Home, under the same owner and in the saved order, including later pages.
func TestHomeSavedBookViewHydratesPublishedEndpoint(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	id, err := identity.New(c.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('owner','owner',x'00','profile',1)`)
	library := c.Library("books", "Audiobooks", "audiobook", root)
	books := []catalogtest.Item{}
	for i, title := range []string{"First (Unabridged)", "Second (Unabridged)", "Third"} {
		book := c.Book(library, title, "Author")
		part := c.BookFile(book, 1, filepath.Join(root, title+".flac"))
		c.Fields(part.ID, map[string]any{"added_text": []string{"2026-01-01T00:00:00.000Z", "2026-01-02T00:00:00.000Z", "2026-01-03T00:00:00.000Z"}[i]})
		books = append(books, book)
	}
	c.Drain()
	owner, err := id.Issue("owner", "profile", "local", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Dependencies{DB: c.DB, Identity: id, Catalog: catalog.New(c.DB), Console: operations.New(c.DB)})
	read := func(method, path string, body any, out any) {
		t.Helper()
		raw := ""
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			raw = string(encoded)
		}
		w := homeAPIRequest(t, h, owner.AccessToken, method, path, raw)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	var created struct{ Receipt catalog.SavedResourceReceipt }
	read("POST", "/v1/saved-resources", map[string]any{
		"operationId": "home-book-view-create", "expectedRevision": 0, "kind": "view", "name": "Recently added in Audiobooks",
		"definition": catalog.SavedViewDefinition{LibraryID: "books", Pivot: "books", Sort: []catalog.BrowseSortSelection{{Field: "added", Direction: "desc"}}, Presentation: "grid"},
	}, &created)
	resourceID := created.Receipt.ResourceID
	rowID := "view:" + resourceID
	var content catalog.SavedResourceContent
	read("GET", "/v1/saved-resources/"+resourceID+"/content?limit=2", nil, &content)
	if content.Resource.Status != "ready" || content.Browse == nil || len(content.Browse.Entries) != 2 {
		t.Fatalf("stored browse: %+v", content)
	}
	read("PUT", "/v1/home/layout", map[string]any{"expectedRevision": 1, "idempotencyKey": "home-book-view-layout", "rowOrder": []string{rowID}, "hiddenRowIds": []string{}}, nil)
	var manifest catalog.HomeDocument
	read("GET", "/v1/home", nil, &manifest)
	endpoint := ""
	for _, row := range manifest.Rows {
		if row.ID == rowID {
			endpoint = row.Endpoint
		}
	}
	if endpoint != "/v1/home/rows/"+rowID {
		t.Fatalf("published endpoint: %q", endpoint)
	}
	for _, path := range []string{endpoint, "/v1/home/rows/view%3A" + resourceID} {
		t.Run(path, func(t *testing.T) {
			var first catalog.HomeRow
			w := homeAPIRequest(t, h, owner.AccessToken, "GET", path+"?limit=2", "")
			if w.Code != http.StatusOK {
				t.Fatalf("hydrate: %d %s", w.Code, w.Body.String())
			}
			if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
				t.Fatal(err)
			}
			if first.Total != 3 || len(first.Entries) != 2 || first.NextCursor == "" || first.Entries[0].ID != books[2].Public || first.Entries[1].ID != books[1].Public {
				t.Fatalf("first page: %+v", first)
			}
			var next catalog.HomeRow
			w = homeAPIRequest(t, h, owner.AccessToken, "GET", path+"?limit=2&cursor="+url.QueryEscape(first.NextCursor), "")
			if w.Code != http.StatusOK {
				t.Fatalf("continue: %d %s", w.Code, w.Body.String())
			}
			if err := json.Unmarshal(w.Body.Bytes(), &next); err != nil {
				t.Fatal(err)
			}
			if next.Start != 2 || len(next.Entries) != 1 || next.Entries[0].ID != books[0].Public || next.HasMore || next.NextCursor != "" {
				t.Fatalf("last page: %+v", next)
			}
			if !reflect.DeepEqual([]string{first.Entries[0].ID, first.Entries[1].ID}, []string{content.Browse.Entries[0].ID, content.Browse.Entries[1].ID}) {
				t.Fatal("Home changed the saved browse order")
			}
		})
	}
}

func TestHomeRowPathGrammarPreservesOpaqueIDGuards(t *testing.T) {
	h, owner, _ := homeAPIFixture(t)
	for _, path := range []string{
		"/v1/home/rows/view%3A", "/v1/home/rows/view%3Aunknown%3Aextra",
		"/v1/home/rows/view%3Aunknown%2Fchild", "/v1/home/rows/view%3A..",
		"/v1/home/rows/view%3Aunknown%252Fchild", "/v1/home/rows/unknown%3Arow",
		"/v1/items/unknown%3Aid", "/v1/books/unknown%3Aid",
	} {
		t.Run(path, func(t *testing.T) {
			w := homeAPIRequest(t, h, owner.AccessToken, "GET", path, "")
			if w.Code != http.StatusNotFound {
				t.Fatalf("malformed or unknown ID reached content: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
