package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

// browseRowCountStatements classifies the two whole-set count shapes an
// unfiltered browse can issue: the exact count of the eligible set, and the
// capped count a posted filter uses. A Reading continuation must carry the
// exact total instead of running either.
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
