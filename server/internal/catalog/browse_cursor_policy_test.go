package catalog

import (
	"errors"
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"strings"
	"testing"
	"time"
)

func cursorPolicyFixture(t *testing.T) (*catalogtest.Catalog, *Service, Viewer, BrowseRequest) {
	t.Helper()
	c := catalogtest.Open(t)
	lib := c.Library("a", "Movies", "movie", "/a")
	c.Library("b", "Other", "movie", "/b")
	for i := 0; i < 5; i++ {
		c.Movie(lib, fmt.Sprintf("/a/%d.mkv", i), fmt.Sprintf("Movie %d", i), 2000)
	}
	c.Drain()
	return c, New(c.DB), Viewer{Profile: "p", Fence: "principal", Libraries: []string{"b", "a"}}, BrowseRequest{ServerID: "server", Library: "a", Pivot: "movies", Limit: 2}
}

// Even a correctly signed old issuer must not accidentally satisfy the new
// source policy. Forged totals, expiry and soft revisions cannot move the
// signed traversal or supply a stale cached count.
func TestBrowseCursorSeparatesSourceAndCachedTotals(t *testing.T) {
	c, s, v, r := cursorPolicyFixture(t)
	first, e := s.BrowseEntities(v, r)
	if e != nil || first.PageInfo.NextCursor == "" {
		t.Fatal(first, e)
	}
	r.Cursor = first.PageInfo.NextCursor
	payload, _, _ := strings.Cut(r.Cursor, ".")
	value, e := decodeCursorPayload(payload)
	if e != nil {
		t.Fatal(e)
	}
	old := value
	old.Fingerprint = old.CacheRevision
	old.CacheRevision = ""
	r.Cursor, e = s.encodeRevisionCursor(old, ContentRevision{Catalog: old.CatalogRevision, Viewer: old.ViewerRevision})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BrowseEntities(v, r); !errors.Is(e, ErrCursor) {
		t.Fatalf("signed old format admitted: %v", e)
	}
	// Remove the anchor, add two titles and publish personal decoration. The
	// opaque sort key still resumes at Movie 2; the count must become six.
	var anchor int64
	if e := c.DB.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, first.Entries[1].ID).Scan(&anchor); e != nil {
		t.Fatal(e)
	}
	c.Delete(anchor)
	var lib int64
	if e := c.DB.QueryRow(`SELECT id FROM catalog_libraries WHERE library_id='a'`).Scan(&lib); e != nil {
		t.Fatal(e)
	}
	c.Movie(lib, "/a/5.mkv", "Movie 5", 2000)
	c.Movie(lib, "/a/6.mkv", "Movie 6", 2000)
	c.Exec(`INSERT INTO viewer_revisions(profile_id,library_id,revision) VALUES('p','a',1)`)
	c.Drain()
	value.Total = 9999
	value.Expires = time.Now().Add(-time.Hour).Unix()
	r.Cursor, e = s.encodeRevisionCursor(value, ContentRevision{Catalog: value.CatalogRevision, Viewer: value.ViewerRevision})
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.BrowseEntities(v, r)
	if e != nil || len(next.Entries) != 2 || next.Entries[0].Title != "Movie 2" || next.PageInfo.Total != 6 {
		t.Fatalf("lost signed position or reused stale count: %+v %v", next, e)
	}
	freshRequest := r
	freshRequest.Cursor = ""
	fresh, e := s.BrowseEntities(v, freshRequest)
	if e != nil {
		t.Fatal(e)
	}
	freshPayload, _, _ := strings.Cut(fresh.PageInfo.NextCursor, ".")
	current, e := decodeCursorPayload(freshPayload)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"expiry", "cache", "content"} {
		t.Run(kind, func(t *testing.T) {
			altered := current
			altered.Total = 9999
			switch kind {
			case "expiry":
				altered.Expires = time.Now().Add(-time.Hour).Unix()
			case "cache":
				altered.CacheRevision = "another-full-revision"
			case "content":
				altered.CatalogRevision--
			}
			freshRequest.Cursor, e = s.encodeRevisionCursor(altered, ContentRevision{Catalog: altered.CatalogRevision, Viewer: altered.ViewerRevision})
			if e != nil {
				t.Fatal(e)
			}
			page, e := s.BrowseEntities(v, freshRequest)
			if e != nil || page.PageInfo.Total != 6 || len(page.Entries) == 0 || page.Entries[0].Title != "Movie 3" {
				t.Fatalf("invalid count reuse or traversal for %s: %+v %v", kind, page, e)
			}
		})
	}
	missing := current
	missing.CacheRevision = ""
	freshRequest.Cursor, e = s.encodeRevisionCursor(missing, ContentRevision{Catalog: missing.CatalogRevision, Viewer: missing.ViewerRevision})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BrowseEntities(v, freshRequest); !errors.Is(e, ErrCursor) {
		t.Fatalf("signed missing cache policy admitted: %v", e)
	}
	// A library-set permutation has the identical authority, not a new source.
	reordered := v
	reordered.Libraries = []string{"a", "b"}
	if _, e = s.BrowseEntities(reordered, r); e != nil {
		t.Fatalf("library ordering changed authority: %v", e)
	}
}

func TestBrowseCursorBindsEveryAuthorityAndQueryScope(t *testing.T) {
	_, s, v, r := cursorPolicyFixture(t)
	r.Sort = []BrowseSortSelection{{Field: "random", Direction: "asc", Seed: 123}}
	first, e := s.BrowseEntities(v, r)
	if e != nil || first.PageInfo.NextCursor == "" {
		t.Fatal(first, e)
	}
	r.Cursor = first.PageInfo.NextCursor
	cases := []struct {
		name   string
		change func(*Viewer, *BrowseRequest)
	}{
		{"profile", func(v *Viewer, r *BrowseRequest) { v.Profile = "another" }},
		{"principal", func(v *Viewer, r *BrowseRequest) { v.Fence = "another" }},
		{"libraries", func(v *Viewer, r *BrowseRequest) { v.Libraries = []string{"a"} }},
		{"server", func(v *Viewer, r *BrowseRequest) { r.ServerID = "another" }},
		{"library", func(v *Viewer, r *BrowseRequest) { r.Library = "b" }},
		{"query", func(v *Viewer, r *BrowseRequest) {
			r.Query = &BrowseNode{Field: "title", Operator: "contains", Value: "Movie"}
		}},
		{"sort", func(v *Viewer, r *BrowseRequest) { r.Sort = []BrowseSortSelection{{Field: "title", Direction: "asc"}} }},
		{"seed", func(v *Viewer, r *BrowseRequest) {
			r.Sort = []BrowseSortSelection{{Field: "random", Direction: "asc", Seed: 124}}
		}},
		{"limit", func(v *Viewer, r *BrowseRequest) { r.Limit = 3 }},
		{"visibility", func(v *Viewer, r *BrowseRequest) {
			age := 13
			v.Restrictions = identity.ContentRestrictions{MaximumAge: &age}
		}},
	}
	titleRequest := r
	titleRequest.Cursor = ""
	titleRequest.Sort = []BrowseSortSelection{{Field: "title", Direction: "asc"}}
	titleFirst, e := s.BrowseEntities(v, titleRequest)
	if e != nil {
		t.Fatal(e)
	}
	titleRequest.Cursor = titleFirst.PageInfo.NextCursor
	titleRequest.Seek = &BrowseSeek{Prefix: "M"}
	if _, e = s.BrowseEntities(v, titleRequest); e == nil {
		t.Fatal("crossed signed title seek scope")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changedViewer, changedRequest := v, r
			tc.change(&changedViewer, &changedRequest)
			if _, e := s.BrowseEntities(changedViewer, changedRequest); e == nil {
				t.Fatal("crossed signed authority or query scope")
			}
		})
	}
}

func TestBrowseCursorRefusesCurrentLibraryRetirement(t *testing.T) {
	c, s, v, r := cursorPolicyFixture(t)
	first, e := s.BrowseEntities(v, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Cursor = first.PageInfo.NextCursor
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE library_id='b'`)
	if _, e = s.BrowseEntities(v, r); !errors.Is(e, ErrStaleContinuation) {
		t.Fatalf("retired authorized library retained cursor: %v", e)
	}
}

func TestAcrossCursorKeepsPositionAndRecountsOrdinaryRevisions(t *testing.T) {
	c, s, v, _ := cursorPolicyFixture(t)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,revision) SELECT 'p',id,1,1 FROM catalog_entities WHERE kind=1`)
	request := AcrossRequest{ServerID: "server", Pivot: "items", Limit: 2, Query: &BrowseNode{Field: "favorite", Operator: "equals", Value: true}}
	first, e := s.BrowseAcross(v, request)
	if e != nil || first.PageInfo.NextCursor == "" {
		t.Fatal(first, e)
	}
	request.Cursor = first.PageInfo.NextCursor
	payload, _, _ := strings.Cut(request.Cursor, ".")
	original, e := decodeCursorPayload(payload)
	if e != nil {
		t.Fatal(e)
	}
	old := original
	old.Fingerprint = old.CacheRevision
	old.CacheRevision = ""
	request.Cursor, e = s.encodeRevisionCursor(old, ContentRevision{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BrowseAcross(v, request); !errors.Is(e, ErrCursor) {
		t.Fatalf("across old signed format admitted: %v", e)
	}
	var lib int64
	if e = c.DB.QueryRow(`SELECT id FROM catalog_libraries WHERE library_id='a'`).Scan(&lib); e != nil {
		t.Fatal(e)
	}
	added := c.Movie(lib, "/a/5.mkv", "Movie 5", 2000)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,revision) VALUES('p',?,1,1)`, added.ID)
	c.Exec(`INSERT INTO viewer_revisions(profile_id,library_id,revision) VALUES('p','a',1) ON CONFLICT(profile_id,library_id) DO UPDATE SET revision=revision+1`)
	c.Drain()
	original.Total = 9999
	request.Cursor, e = s.encodeRevisionCursor(original, ContentRevision{})
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.BrowseAcross(v, request)
	if e != nil || next.PageInfo.Total != 6 || len(next.Entries) != 2 || next.Entries[0].Title != "Movie 2" {
		t.Fatalf("across lost position or cached count: %+v %v", next, e)
	}
	changed := v
	changed.Libraries = []string{"a"}
	if _, e = s.BrowseAcross(changed, request); !errors.Is(e, ErrStaleContinuation) {
		t.Fatalf("across library authority changed: %v", e)
	}
	c.Exec(`UPDATE library_metadata_agents SET agent='local',revision=revision+1 WHERE library_id='a'`)
	if _, e = s.BrowseAcross(v, request); !errors.Is(e, ErrStaleContinuation) {
		t.Fatalf("across metadata source changed: %v", e)
	}
}

func TestAggregateCursorBindsDisplayAndKeepsSoftTraversal(t *testing.T) {
	c, s, v, r := cursorPolicyFixture(t)
	ids := []int64{}
	rows, e := c.DB.Query(`SELECT id FROM catalog_entities WHERE kind=1 ORDER BY title`)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	for i, id := range ids {
		c.Fields(id, map[string]any{"year": 1990 + i*10})
	}
	c.Drain()
	r.Pivot = "categories"
	r.Limit = 1
	r.Query = &BrowseNode{Field: "homeRow", Operator: "equals", Value: "new_on_server"}
	r.Rows = map[string][]int64{"new_on_server": ids}
	r.HomeRowDisplay = "new_on_server"
	if e = ValidateHomeRowDisplay(r.Query, r.HomeRowDisplay); e != nil {
		t.Fatal(e)
	}
	first, e := s.BrowseEntities(v, r)
	if e != nil || first.PageInfo.NextCursor == "" {
		t.Fatal(first, e)
	}
	r.Cursor = first.PageInfo.NextCursor
	changed := r
	changed.HomeRowDisplay = ""
	if e = ValidateHomeRowDisplay(changed.Query, changed.HomeRowDisplay); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BrowseEntities(v, changed); !errors.Is(e, ErrCursor) {
		t.Fatalf("aggregate changed validated display: %v", e)
	}
	// Remove the decade anchor while preserving the canonical Home member set.
	// This is an ordinary ordering change, not a new source or authority.
	c.Fields(ids[0], map[string]any{"year": 2000})
	c.Exec(`INSERT INTO viewer_revisions(profile_id,library_id,revision) VALUES('p','a',1) ON CONFLICT(profile_id,library_id) DO UPDATE SET revision=revision+1`)
	c.Drain()
	next, e := s.BrowseEntities(v, r)
	if e != nil || next.PageInfo.Total != 4 || len(next.Entries) != 1 || next.Entries[0].ID != "decade:2000" {
		t.Fatalf("aggregate lost soft traversal/count: %+v %v", next, e)
	}
}

func TestAcrossHistoryCursorPolicySurvivesSoftChangesAndExpiry(t *testing.T) {
	c, s, v, _ := cursorPolicyFixture(t)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,last_played_at,revision) SELECT 'p',id,'2026-10-0'||(row_number() OVER(ORDER BY title))||'T00:00:00Z',1 FROM catalog_entities WHERE kind=1`)
	c.Drain()
	r := AcrossRequest{ServerID: "server", Pivot: "items", Limit: 2, Query: &BrowseNode{Field: "lastPlayedAt", Operator: "is-present"}, Sort: []BrowseSortSelection{{Field: "groupOrder", Direction: "asc"}}}
	first, e := s.BrowseAcross(v, r)
	if e != nil || first.PageInfo.NextCursor == "" {
		t.Fatal(first, e)
	}
	payload, _, _ := strings.Cut(first.PageInfo.NextCursor, ".")
	value, e := decodeCursorPayload(payload)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"old", "missing"} {
		t.Run(kind, func(t *testing.T) {
			bad := value
			if kind == "old" {
				bad.Fingerprint = bad.CacheRevision
			} else {
				bad.CacheRevision = ""
			}
			r.Cursor, e = s.encodeRevisionCursor(bad, ContentRevision{})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.BrowseAcross(v, r); !errors.Is(e, ErrCursor) {
				t.Fatalf("history signed %s policy admitted: %v", kind, e)
			}
		})
	}
	var lib int64
	if e = c.DB.QueryRow(`SELECT id FROM catalog_libraries WHERE library_id='a'`).Scan(&lib); e != nil {
		t.Fatal(e)
	}
	older := c.Movie(lib, "/a/older.mkv", "Older", 2000)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,last_played_at,revision) VALUES('p',?,'2026-09-01T00:00:00Z',1)`, older.ID)
	c.Exec(`INSERT INTO viewer_revisions(profile_id,library_id,revision) VALUES('p','a',1) ON CONFLICT(profile_id,library_id) DO UPDATE SET revision=revision+1`)
	c.Drain()
	value.Total = 9999
	r.Cursor, e = s.encodeRevisionCursor(value, ContentRevision{})
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.BrowseAcross(v, r)
	if e != nil || next.PageInfo.Total != 6 || len(next.Entries) != 2 || next.Entries[0].Title != "Movie 2" {
		t.Fatalf("history soft position/count: %+v %v", next, e)
	}
	// Expiry alone must invalidate a correctly source/cache-bound signed total.
	r.Cursor = ""
	fresh, e := s.BrowseAcross(v, r)
	if e != nil {
		t.Fatal(e)
	}
	payload, _, _ = strings.Cut(fresh.PageInfo.NextCursor, ".")
	current, e := decodeCursorPayload(payload)
	if e != nil {
		t.Fatal(e)
	}
	current.Total = 9999
	current.Expires = time.Now().Add(-time.Hour).Unix()
	r.Cursor, e = s.encodeRevisionCursor(current, ContentRevision{})
	if e != nil {
		t.Fatal(e)
	}
	next, e = s.BrowseAcross(v, r)
	if e != nil || next.PageInfo.Total != 6 || len(next.Entries) != 2 || next.Entries[0].Title != "Movie 2" {
		t.Fatalf("history expiry position/count: %+v %v", next, e)
	}
	c.Exec(`UPDATE library_metadata_agents SET agent='local',revision=revision+1 WHERE library_id='a'`)
	if _, e = s.BrowseAcross(v, r); !errors.Is(e, ErrStaleContinuation) {
		t.Fatalf("history crossed metadata source: %v", e)
	}
}
