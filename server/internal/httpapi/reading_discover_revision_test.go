package httpapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
)

// A continuation is authorized by the same library, principal and series scope
// as the read that captured it. When a new ready image is published the stale
// continuation is refused with exactly the revision a fresh read of that same
// viewer publishes, so a client rebases from the answer rather than guessing or
// probing another read. Pending input alone does not retire the published image.
func TestReadingDiscoverSeriesContinuationRefusalCarriesTheFreshRevision(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("Saga %c", 'A'+i)
		path := readingTreeFile(t, r, fmt.Sprintf("Author/%s/Title %d.epub", name, i))
		readingScan(t, r.f, r.library)
		readingSeriesOwnerField(t, r, readingPathItem(t, r, path), "series", name)
	}
	readingSeriesDrain(t, r)
	h := New(r.d)
	first := readingDiscoverGet(t, r, url.Values{"libraryId": {r.library}, "limit": {"1"}, "seriesLimit": {"1"}})
	if first.Revision == nil || first.Importing || len(first.Rows) != 2 || first.Rows[1].NextCursor == "" {
		t.Fatalf("first page %+v", first)
	}
	// The captured continuation really continues this row in this viewer's scope.
	continuation := "/v1/reading/discover?libraryId=" + r.library + "&row=series&limit=1&cursor=" + url.QueryEscape(first.Rows[1].NextCursor)
	w := readingHTTP(t, h, "GET", continuation, r.f.owner.AccessToken, "", nil)
	var replay ReadingDiscoverResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &replay) != nil || len(replay.Rows) != 1 || replay.Rows[0].Key != "series" || len(replay.Rows[0].Series) != 1 {
		t.Fatalf("continuation replay %d %s", w.Code, w.Body.String())
	}
	if replay.Revision == nil || *replay.Revision != *first.Revision || replay.Importing {
		t.Fatalf("continuation revision %+v first %+v", replay.Revision, first.Revision)
	}
	assertSpecResponse(t, "GET", "/v1/reading/discover", w)
	// A pending owner edit is input only: the published image's continuation
	// still walks, now reporting that the input is ahead of the served image.
	readingSeriesOwnerField(t, r, r.item, "series", "Pending only")
	readingSeriesInputPending(t, r)
	w = readingHTTP(t, h, "GET", continuation, r.f.owner.AccessToken, "", nil)
	var importing ReadingDiscoverResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &importing) != nil || len(importing.Rows) != 1 || len(importing.Rows[0].Series) != 1 {
		t.Fatalf("continuation while importing %d %s", w.Code, w.Body.String())
	}
	if !importing.Importing || importing.Revision == nil || *importing.Revision != *first.Revision {
		t.Fatalf("continuation while importing %+v first %+v", importing, first.Revision)
	}
	// Publishing a new image (a real added title drained with that edit) retires
	// the captured continuation.
	readingTreeFile(t, r, "Author/Later/New.epub")
	readingScan(t, r.f, r.library)
	readingSeriesDrain(t, r)
	w = readingHTTP(t, h, "GET", continuation, r.f.owner.AccessToken, "", nil)
	if w.Code != 412 || bodyErrorCode(w.Body.String()) != "revision_mismatch" {
		t.Fatalf("stale continuation %d %s", w.Code, w.Body.String())
	}
	var refused struct {
		Error struct {
			Current catalog.ContentRevision `json:"current"`
		} `json:"error"`
	}
	if json.Unmarshal(w.Body.Bytes(), &refused) != nil {
		t.Fatalf("stale continuation body %s", w.Body.String())
	}
	assertSpecResponse(t, "GET", "/v1/reading/discover", w)
	fresh := readingDiscoverGet(t, r, url.Values{"libraryId": {r.library}, "seriesLimit": {"1"}})
	if fresh.Revision == nil || *fresh.Revision != refused.Error.Current {
		t.Fatalf("refusal current %+v fresh revision %+v", refused.Error.Current, fresh.Revision)
	}
	// The published revision is the live catalog/viewer content revision, not a
	// remembered token: the refusal body and a fresh read both report current.
	var liveCatalog, liveViewer int64
	if err := r.f.db.QueryRow(`SELECT r.revision,COALESCE(v.revision,0) FROM library_revisions r LEFT JOIN viewer_revisions v ON v.library_id=r.library_id AND v.profile_id=? WHERE r.library_id=?`, identity.PersonalKey(r.p.Viewer), r.library).Scan(&liveCatalog, &liveViewer); err != nil {
		t.Fatal(err)
	}
	if fresh.Revision.Catalog != liveCatalog || fresh.Revision.Viewer != liveViewer {
		t.Fatalf("fresh revision %+v current catalog=%d viewer=%d", fresh.Revision, liveCatalog, liveViewer)
	}
}
