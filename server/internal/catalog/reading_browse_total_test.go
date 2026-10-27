package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// browseRowCountStatements classifies the two whole-set count shapes an
// unfiltered browse can issue: the exact count of the eligible set, and the
// capped count a posted filter uses. An unrestricted Reading continuation
// must carry the exact total instead of running either.
func browseRowCountStatements(queries []string) (whole, capped int) {
	for _, query := range queries {
		query = strings.Join(strings.Fields(query), " ")
		switch {
		case strings.Contains(query, "SELECT count(*) FROM (SELECT 1 FROM catalog_browse_rows"):
			capped++
		case strings.Contains(query, "SELECT count(*) FROM catalog_browse_rows"):
			whole++
		}
	}
	return whole, capped
}

func readingBrowsePageIDs(entries []ContentEntry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}

func TestReadingBrowseContinuationReusesTheExactTotal(t *testing.T) {
	c, s, viewer, items := readingBrowseFixture(t, "book")
	c.Drain()
	want := make([]string, len(items))
	for i, item := range items {
		want[i] = item.Public
	}

	// Walk the whole unfiltered list twice: the first page of each walk counts
	// the eligible set exactly once, every continuation in between counts
	// nothing while still returning the same exact total and order.
	for _, limit := range []int{2, 1} {
		var ids []string
		cursor := ""
		pages := 0
		for {
			pages++
			if pages > len(items)+1 {
				t.Fatalf("limit %d: walk did not terminate", limit)
			}
			ctx, trace := dbwork.TraceStatements(context.Background())
			page, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: limit, Cursor: cursor})
			if err != nil {
				t.Fatalf("limit %d page %d: %v", limit, pages, err)
			}
			whole, capped := browseRowCountStatements(trace())
			if pages == 1 {
				if whole != 1 || capped != 0 {
					t.Fatalf("limit %d first page counts: whole=%d capped=%d", limit, whole, capped)
				}
			} else if whole != 0 || capped != 0 {
				t.Fatalf("limit %d page %d recounted: whole=%d capped=%d", limit, pages, whole, capped)
			}
			if page.PageInfo.Total != len(items) || page.PageInfo.TotalIsLowerBound {
				t.Fatalf("limit %d page %d total: %+v", limit, pages, page.PageInfo)
			}
			if page.Pivot != "books" {
				t.Fatalf("limit %d page %d pivot: %q", limit, pages, page.Pivot)
			}
			ids = append(ids, readingBrowsePageIDs(page.Entries)...)
			if !page.PageInfo.HasMore {
				if page.PageInfo.NextCursor != "" {
					t.Fatalf("limit %d last page published a cursor", limit)
				}
				break
			}
			if page.PageInfo.NextCursor == "" {
				t.Fatalf("limit %d page %d has more but no cursor", limit, pages)
			}
			cursor = page.PageInfo.NextCursor
		}
		if strings.Join(ids, ",") != strings.Join(want, ",") {
			t.Fatalf("limit %d order: %v want %v", limit, ids, want)
		}
	}

	// A legacy continuation with no carried total still owes the exact
	// unfiltered count: it must not fall back to the capped filter count.
	first, err := s.BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := s.readSignedBrowseCursor(first.PageInfo.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Total = 0
	legacy.Expires = time.Now().Add(time.Minute).Unix()
	raw, err := s.encodeCursor(legacy)
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace := dbwork.TraceStatements(context.Background())
	page, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2, Cursor: raw})
	if err != nil {
		t.Fatal(err)
	}
	whole, capped := browseRowCountStatements(trace())
	if whole != 1 || capped != 0 {
		t.Fatalf("legacy total fallback counts: whole=%d capped=%d", whole, capped)
	}
	if page.PageInfo.Total != len(items) || page.PageInfo.TotalIsLowerBound {
		t.Fatalf("legacy total fallback: %+v", page.PageInfo)
	}
	if got := readingBrowsePageIDs(page.Entries); strings.Join(got, ",") != strings.Join(want[2:], ",") {
		t.Fatalf("legacy total fallback order: %v want %v", got, want[2:])
	}
}

func TestReadingBrowseContinuationRefusesBeforeCounting(t *testing.T) {
	c, s, viewer, items := readingBrowseFixture(t, "book")
	c.Drain()
	first, err := s.BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := s.readSignedBrowseCursor(first.PageInfo.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	expired := valid
	expired.Expires = time.Now().Add(-time.Second).Unix()
	expiredRaw, err := s.encodeCursor(expired)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name    string
		cursor  string
		request BrowseRequest
		viewer  Viewer
		mutate  func()
	}{
		{name: "malformed", cursor: "not-a-cursor"},
		{name: "expired", cursor: expiredRaw},
		{name: "wrong viewer", cursor: first.PageInfo.NextCursor, viewer: Viewer{Profile: viewer.Profile, Fence: "other-viewer", Libraries: viewer.Libraries}},
		{name: "wrong sort", cursor: first.PageInfo.NextCursor, request: BrowseRequest{Library: "reading", Limit: 2, Sort: []BrowseSortSelection{{Field: "added"}}}},
		{name: "wrong limit", cursor: first.PageInfo.NextCursor, request: BrowseRequest{Library: "reading", Limit: 1}},
		{
			name:   "stale generation",
			cursor: first.PageInfo.NextCursor,
			mutate: func() {
				c.Exec(`UPDATE metadata_owner_fields SET value='Changed Author' WHERE entity_id=? AND field='author'`, items[0].ID)
				c.Drain()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.mutate != nil {
				test.mutate()
			}
			request := test.request
			if request.Library == "" {
				request = BrowseRequest{Library: "reading", Limit: 2}
			}
			request.Cursor = test.cursor
			requestViewer := viewer
			if test.viewer.Fence != "" {
				requestViewer = test.viewer
			}
			ctx, trace := dbwork.TraceStatements(context.Background())
			_, err := s.WithContext(ctx).BrowseEntities(requestViewer, request)
			if !errors.Is(err, ErrStaleContinuation) && !errors.Is(err, ErrCursor) {
				t.Fatalf("refused cursor error: %v", err)
			}
			whole, capped := browseRowCountStatements(trace())
			if whole != 0 || capped != 0 {
				t.Fatalf("cursor refused after counting: whole=%d capped=%d", whole, capped)
			}
		})
	}
}

func TestReadingBrowseEmptyTotalIsExact(t *testing.T) {
	c, s, viewer, _ := readingBrowseFixture(t, "book")
	// No readable publication remains, so the ready projection is genuinely
	// empty rather than merely unbuilt.
	c.Exec(`DELETE FROM reading_publications`)
	c.Drain()
	ctx, trace := dbwork.TraceStatements(context.Background())
	page, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	whole, capped := browseRowCountStatements(trace())
	if whole != 1 || capped != 0 {
		t.Fatalf("empty total counts: whole=%d capped=%d", whole, capped)
	}
	if page.PageInfo.Total != 0 || page.PageInfo.TotalIsLowerBound || page.PageInfo.HasMore || len(page.Entries) != 0 || page.PageInfo.NextCursor != "" {
		t.Fatalf("empty page: %+v", page)
	}
}

// readingBrowseRevisions reads the split library counters: order_revision is
// what the browse fingerprint digests, while revision also moves on a rating
// classification change that does not reorder the library.
func readingBrowseRevisions(t *testing.T, c *catalogtest.Catalog) (revision, orderRevision int64) {
	t.Helper()
	if err := c.DB.QueryRow(`SELECT revision,order_revision FROM library_revisions WHERE library_id='reading'`).Scan(&revision, &orderRevision); err != nil {
		t.Fatal(err)
	}
	return revision, orderRevision
}

// readingBrowseFingerprint recomputes the fingerprint a fresh page of this
// viewer and query would carry, without running one.
func readingBrowseFingerprint(t *testing.T, s *Service, viewer Viewer, query *BrowseNode) string {
	t.Helper()
	pivot, ok := pivotForKind("book", defaultPivot("book"))
	if !ok {
		t.Fatal("book browse pivot is missing")
	}
	sorts, err := resolveBrowseSorts(pivot, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := BrowseRequest{Library: "reading", Limit: 2, Profile: viewer.Profile, ViewerFence: viewer.Fence, Query: query, Restrictions: viewer.Restrictions}
	var out string
	err = dbwork.WithReadSnapshot(context.Background(), s.db, func(ctx context.Context) error {
		revision, err := s.WithContext(ctx).browseRevision(request, pivot, sorts)
		out = revision
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A restricted Reading continuation may not reuse the opening page's total: the
// live restriction reads content_rating_ages at query time, and a classification
// write moves library_revisions.revision without moving order_revision, which is
// all the cursor fingerprint digests. The continuation therefore reports the
// exact new total and remainder. (The legacy envelope's own raw-revision check
// also zeroes the carried total in this sequence; the reuse gate itself is
// isolated in TestReadingBrowseRestrictedContinuationIgnoresACarriedTotal.)
func TestReadingBrowseRestrictedContinuationRecountsTheExactTotal(t *testing.T) {
	c, s, restricted, items := readingBrowseFixture(t, "book")
	age := 12
	restricted.Restrictions = identity.ContentRestrictions{MaximumAge: &age, Revision: 1}
	// Classify the spelling before attaching the edge, so the drain has nothing
	// to classify and the opening page counts the title.
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('x',0)`)
	c.Attributes(items[3].ID, "contentRating", "X")
	c.Drain()

	// The same library without a live restriction still carries the total: the
	// gate is the compiled restriction clause, not Reading continuations.
	unrestricted := Viewer{Profile: restricted.Profile, Fence: restricted.Fence, Libraries: restricted.Libraries}
	open, err := s.BrowseEntities(unrestricted, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace := dbwork.TraceStatements(context.Background())
	next, err := s.WithContext(ctx).BrowseEntities(unrestricted, BrowseRequest{Library: "reading", Limit: 2, Cursor: open.PageInfo.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 0 || capped != 0 {
		t.Fatalf("unrestricted continuation recounted: whole=%d capped=%d", whole, capped)
	}
	if next.PageInfo.Total != len(items) {
		t.Fatalf("unrestricted continuation total: %+v", next.PageInfo)
	}

	revisionBefore, orderBefore := readingBrowseRevisions(t, c)
	ctx, trace = dbwork.TraceStatements(context.Background())
	first, err := s.WithContext(ctx).BrowseEntities(restricted, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 1 || capped != 0 {
		t.Fatalf("restricted first page counts: whole=%d capped=%d", whole, capped)
	}
	if len(first.Entries) != 2 || first.PageInfo.Total != 4 || first.PageInfo.TotalIsLowerBound {
		t.Fatalf("restricted first page: %+v", first.PageInfo)
	}
	fingerprint := first.PageInfo.Revision

	// Classification only: the visible anchor (Title 1) and its order are
	// untouched; Title 3's spelling moves above the ceiling.
	c.Exec(`UPDATE content_rating_ages SET minimum_age=17 WHERE value_key='x'`)
	revisionAfter, orderAfter := readingBrowseRevisions(t, c)
	if revisionAfter <= revisionBefore {
		t.Fatalf("classification write did not advance library revision: %d -> %d", revisionBefore, revisionAfter)
	}
	if orderAfter != orderBefore {
		t.Fatalf("classification write reordered the library: %d -> %d", orderBefore, orderAfter)
	}
	if got := readingBrowseFingerprint(t, s, restricted, nil); got != fingerprint {
		t.Fatalf("fingerprint changed across a classification write: %s -> %s", fingerprint, got)
	}

	ctx, trace = dbwork.TraceStatements(context.Background())
	second, err := s.WithContext(ctx).BrowseEntities(restricted, BrowseRequest{Library: "reading", Limit: 2, Cursor: first.PageInfo.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 1 || capped != 0 {
		t.Fatalf("restricted continuation counts: whole=%d capped=%d", whole, capped)
	}
	if second.PageInfo.Total != 3 || second.PageInfo.TotalIsLowerBound {
		t.Fatalf("restricted continuation total: %+v", second.PageInfo)
	}
	if got := readingBrowsePageIDs(second.Entries); strings.Join(got, ",") != items[2].Public {
		t.Fatalf("restricted continuation remainder: %v want [%s]", got, items[2].Public)
	}
	if second.PageInfo.HasMore || second.PageInfo.NextCursor != "" {
		t.Fatalf("restricted walk did not end: %+v", second.PageInfo)
	}
}

// The mirror direction: an unclassified spelling becomes classified, so a
// title that page one could not see becomes visible. The correct total grows
// while the fingerprint stays put, and the continuation must see both the new
// total and the new remainder.
func TestReadingBrowseRestrictedContinuationRecountsAnUnratedClassification(t *testing.T) {
	c, s, viewer, items := readingBrowseFixture(t, "book")
	viewer.Restrictions = identity.ContentRestrictions{BlockUnrated: true, Revision: 1}
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('g',0)`)
	for _, item := range items[:3] {
		c.Attributes(item.ID, "contentRating", "G")
	}
	// No age row: the drain classifies an unknown spelling as unrated, which
	// BlockUnrated hides until a real classification arrives.
	c.Attributes(items[3].ID, "contentRating", "Y")
	c.Drain()

	revisionBefore, orderBefore := readingBrowseRevisions(t, c)
	ctx, trace := dbwork.TraceStatements(context.Background())
	first, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 1 || capped != 0 {
		t.Fatalf("restricted first page counts: whole=%d capped=%d", whole, capped)
	}
	if first.PageInfo.Total != 3 || first.PageInfo.TotalIsLowerBound {
		t.Fatalf("restricted first page total: %+v", first.PageInfo)
	}
	fingerprint := first.PageInfo.Revision

	// Unknown spelling to a known age: the same one-row classification write the
	// drain performs, still without reordering the library.
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('y',0) ON CONFLICT(value_key) DO UPDATE SET minimum_age=0`)
	revisionAfter, orderAfter := readingBrowseRevisions(t, c)
	if revisionAfter <= revisionBefore || orderAfter != orderBefore {
		t.Fatalf("classification write counters: revision %d -> %d, order %d -> %d", revisionBefore, revisionAfter, orderBefore, orderAfter)
	}
	if got := readingBrowseFingerprint(t, s, viewer, nil); got != fingerprint {
		t.Fatalf("fingerprint changed across a classification write: %s -> %s", fingerprint, got)
	}

	ctx, trace = dbwork.TraceStatements(context.Background())
	second, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2, Cursor: first.PageInfo.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 1 || capped != 0 {
		t.Fatalf("restricted continuation counts: whole=%d capped=%d", whole, capped)
	}
	if second.PageInfo.Total != 4 || second.PageInfo.TotalIsLowerBound {
		t.Fatalf("restricted continuation total: %+v", second.PageInfo)
	}
	if got := readingBrowsePageIDs(second.Entries); strings.Join(got, ",") != strings.Join([]string{items[2].Public, items[3].Public}, ",") {
		t.Fatalf("restricted continuation remainder: %v want %v", got, []string{items[2].Public, items[3].Public})
	}
	if second.PageInfo.HasMore || second.PageInfo.NextCursor != "" {
		t.Fatalf("restricted walk did not end: %+v", second.PageInfo)
	}
}

// A filtered restricted continuation keeps the existing capped count and its
// lower-bound meaning, but must recount after validation and must not publish
// that snapshot to the revision-keyed cache.
func TestReadingBrowseRestrictedFilteredContinuationRecountsAfterClassification(t *testing.T) {
	previous := browseTotalThreshold
	browseTotalThreshold = 2
	defer func() { browseTotalThreshold = previous }()

	c, s, viewer, items := readingBrowseFixture(t, "book")
	age := 12
	viewer.Restrictions = identity.ContentRestrictions{MaximumAge: &age, Revision: 1}
	// Give the third match the same accepted author spelling as the first two,
	// and only the second title a distinct rating spelling: the classification
	// write below then removes a remainder title, not the anchor.
	c.Exec(`UPDATE metadata_owner_fields SET value='Café Author' WHERE entity_id=? AND field='author'`, items[2].ID)
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('y',0)`)
	c.Attributes(items[1].ID, "contentRating", "Y")
	c.Drain()

	query := &BrowseNode{All: []BrowseNode{{Field: "author", Operator: "contains", Value: "  café\tAUTHOR  "}}}
	ctx, trace := dbwork.TraceStatements(context.Background())
	first, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 1, Query: query})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 0 || capped != 1 {
		t.Fatalf("filtered first page counts: whole=%d capped=%d", whole, capped)
	}
	if first.PageInfo.Total != browseTotalThreshold || !first.PageInfo.TotalIsLowerBound || len(first.Entries) != 1 {
		t.Fatalf("filtered first page total: %+v entries %d", first.PageInfo, len(first.Entries))
	}
	if _, hit := s.cachedBrowseTotal(first.PageInfo.Revision); hit {
		t.Fatal("a restriction-bound total was cached under the browse revision")
	}
	fingerprint := first.PageInfo.Revision

	// Classification only; the anchor (Title 0) stays visible and matched.
	c.Exec(`UPDATE content_rating_ages SET minimum_age=17 WHERE value_key='y'`)
	if got := readingBrowseFingerprint(t, s, viewer, query); got != fingerprint {
		t.Fatalf("fingerprint changed across a classification write: %s -> %s", fingerprint, got)
	}

	ctx, trace = dbwork.TraceStatements(context.Background())
	second, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 1, Query: query, Cursor: first.PageInfo.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 0 || capped != 1 {
		t.Fatalf("filtered continuation counts: whole=%d capped=%d", whole, capped)
	}
	if second.PageInfo.Total != 2 || second.PageInfo.TotalIsLowerBound {
		t.Fatalf("filtered continuation total: %+v", second.PageInfo)
	}
	if got := readingBrowsePageIDs(second.Entries); strings.Join(got, ",") != items[2].Public {
		t.Fatalf("filtered continuation remainder: %v want [%s]", got, items[2].Public)
	}
	if second.PageInfo.HasMore || second.PageInfo.NextCursor != "" {
		t.Fatalf("filtered walk did not end: %+v", second.PageInfo)
	}
}

// The deferral must survive the gate: a restricted cursor that is malformed,
// expired, from another viewer or for another limit still refuses before any
// count is issued.
func TestReadingBrowseRestrictedContinuationRefusesBeforeCounting(t *testing.T) {
	c, s, viewer, items := readingBrowseFixture(t, "book")
	age := 12
	viewer.Restrictions = identity.ContentRestrictions{MaximumAge: &age, Revision: 1}
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('x',0)`)
	c.Attributes(items[3].ID, "contentRating", "X")
	c.Drain()

	first, err := s.BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := s.readSignedBrowseCursor(first.PageInfo.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	expired := valid
	expired.Expires = time.Now().Add(-time.Second).Unix()
	expiredRaw, err := s.encodeCursor(expired)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		cursor string
		limit  int
		viewer Viewer
	}{
		{name: "malformed", cursor: "not-a-cursor", limit: 2},
		{name: "expired", cursor: expiredRaw, limit: 2},
		{name: "wrong viewer", cursor: first.PageInfo.NextCursor, limit: 2, viewer: Viewer{Profile: viewer.Profile, Fence: "other-viewer", Libraries: viewer.Libraries}},
		{name: "wrong limit", cursor: first.PageInfo.NextCursor, limit: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestViewer := viewer
			if test.viewer.Fence != "" {
				requestViewer = test.viewer
			}
			ctx, trace := dbwork.TraceStatements(context.Background())
			_, err := s.WithContext(ctx).BrowseEntities(requestViewer, BrowseRequest{Library: "reading", Limit: test.limit, Cursor: test.cursor})
			if !errors.Is(err, ErrStaleContinuation) && !errors.Is(err, ErrCursor) {
				t.Fatalf("refused cursor error: %v", err)
			}
			if whole, capped := browseRowCountStatements(trace()); whole != 0 || capped != 0 {
				t.Fatalf("cursor refused after counting: whole=%d capped=%d", whole, capped)
			}
		})
	}
}

// The legacy envelope independently zeroes a carried total when
// library_revisions.revision moves, so the natural classification sequence
// above already recounts today. The reuse gate is a separate precondition: a
// restriction-bound continuation must never reuse a nonzero total, even one
// presented with a current envelope and a still-visible anchor. Re-signing the
// first page's stale total at the post-classification revision isolates exactly
// that decision; without the gate the page reports 4 instead of 3.
func TestReadingBrowseRestrictedContinuationIgnoresACarriedTotal(t *testing.T) {
	c, s, viewer, items := readingBrowseFixture(t, "book")
	age := 12
	viewer.Restrictions = identity.ContentRestrictions{MaximumAge: &age, Revision: 1}
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('x',0)`)
	c.Attributes(items[3].ID, "contentRating", "X")
	c.Drain()

	first, err := s.BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	carried, err := s.readSignedBrowseCursor(first.PageInfo.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if carried.Total != 4 {
		t.Fatalf("first page carried total: %d", carried.Total)
	}

	// Classification only: Title 3 drops out; the anchor (Title 1) stays.
	c.Exec(`UPDATE content_rating_ages SET minimum_age=17 WHERE value_key='x'`)
	carried.Expires = time.Now().Add(time.Minute).Unix()
	raw, err := s.encodeCursor(carried)
	if err != nil {
		t.Fatal(err)
	}

	ctx, trace := dbwork.TraceStatements(context.Background())
	second, err := s.WithContext(ctx).BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 2, Cursor: raw})
	if err != nil {
		t.Fatal(err)
	}
	if whole, capped := browseRowCountStatements(trace()); whole != 1 || capped != 0 {
		t.Fatalf("restricted continuation counts: whole=%d capped=%d", whole, capped)
	}
	if second.PageInfo.Total != 3 || second.PageInfo.TotalIsLowerBound {
		t.Fatalf("restricted continuation total: %+v", second.PageInfo)
	}
	if got := readingBrowsePageIDs(second.Entries); strings.Join(got, ",") != items[2].Public {
		t.Fatalf("restricted continuation remainder: %v want [%s]", got, items[2].Public)
	}
}
