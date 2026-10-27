package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
)

// Real Reading items keep their own identities and use personal Saved
// membership. Opening a source or receiving an AV/job capability is unnecessary.
func TestSavedCollectionAdmitsReadingAndProjectsReadingFacts(t *testing.T) {
	for _, format := range []string{"epub", "cbz"} {
		t.Run(format, func(t *testing.T) {
			kind := "book"
			if format == "cbz" {
				kind = "comic"
			}
			r := newReadingFixture(t, kind, format)
			// A completed scan still needs the real catalog worker to publish ready Reading facts.
			readingSeriesDrain(t, r)
			h := New(r.d)
			token := r.f.owner.AccessToken
			var created struct{ Receipt catalog.SavedResourceReceipt }
			w := readingHTTP(t, h, "POST", "/v1/saved-resources", token, "", map[string]any{"operationId": "reading-collection-create", "expectedRevision": 0, "kind": "collection", "name": "Reading picks"})
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &created) != nil || created.Receipt.ResourceID == "" {
				t.Fatal("create personal collection", w.Code)
			}
			path := "/v1/saved-resources/" + created.Receipt.ResourceID
			body := map[string]any{"operationId": "reading-collection-add", "expectedRevision": created.Receipt.Revision, "addItemIds": []string{r.item}}
			var added struct{ Receipt catalog.SavedResourceReceipt }
			w = readingHTTP(t, h, "PUT", path+"/entries", token, "", body)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &added) != nil || added.Receipt.Entries == nil || len(added.Receipt.Entries.Added) != 1 || len(added.Receipt.Entries.Failed) != 0 {
				t.Fatal("Reading membership refused", w.Code)
			}
			var replay struct{ Receipt catalog.SavedResourceReceipt }
			w = readingHTTP(t, h, "PUT", path+"/entries", token, "", body)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &replay) != nil || replay.Receipt.Revision != added.Receipt.Revision {
				t.Fatal("membership retry did not resolve the committed receipt", w.Code)
			}
			var page catalog.SavedResourceContent
			w = readingHTTP(t, h, "GET", path+"/content", token, "", nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Entries) != 1 || page.Entries[0].Hidden || page.Entries[0].Media == nil {
				t.Fatal("collection did not project its real Reading member", w.Code)
			}
			entry := page.Entries[0].Media
			if entry.ID != r.item || entry.LibraryID != r.library || entry.Author != r.fileAuthor || entry.Playback != nil || entry.Duration != nil || entry.ProgressSeconds != nil {
				t.Fatal("Reading identity/facts replaced by timed-media facts")
			}
			actions := entry.HomeActions
			if actions == nil || !actions.Collection || !actions.Selection.Collection || actions.Play || actions.Queue || actions.Playlist || actions.Selection.Play || actions.Selection.Queue || actions.Selection.Playlist || actions.Selection.Edit || actions.Selection.Refresh {
				t.Fatal("Reading received an unsupported action")
			}
			var detail catalog.Detail
			w = readingHTTP(t, h, "GET", "/v1/items/"+r.item+"/detail", token, "", nil)
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.Item.ID != r.item || detail.Item.Kind != entry.Kind || detail.Scope.LibraryID != r.library || len(detail.Actions) != 1 || detail.Actions[0].ID != "add_to_collection" || !detail.Actions[0].Enabled {
				t.Fatal("Reading detail did not publish its exact Collection action", w.Code)
			}
			// Admission was successful above; retirement must be observed again
			// by the callback inside the accepting write, using the old principal.
			ctx := context.Background()
			if err := r.d.savedResourceItemAccess(ctx, r.p, r.item); err != nil {
				t.Fatal("initial admission", err)
			}
			if _, err := dbwork.ExecWrite(ctx, r.f.db, dbwork.ClassInteractive, `UPDATE catalog_entities SET retired=1 WHERE public_id=pid_blob(?)`, r.item); err != nil {
				t.Fatal(err)
			}
			gate, err := dbwork.Begin(ctx, r.f.db, dbwork.ClassInteractive)
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Rollback()
			if err := r.d.savedResourceItemAuthorityTx(ctx, gate.Tx(), r.p, r.item); err == nil {
				t.Fatal("retired Reading target retained a saved-entry grant")
			}
		})
	}
}
