package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// A filtered list's total is exact at any size, and a range may start
// anywhere (under a filter, past the matches it is empty; unfiltered, it
// clamps to the last row). Nothing is refused for being deep.
func TestBrowseAdHocCountsAreExactAndRangesUnbounded(t *testing.T) {
	_, s, _ := browseFixture(t)
	query, err := ParseBrowseQuery(json.RawMessage(`{"field":"year","operator":"at-least","value":0}`), "query")
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	for _, start := range []int{0, 20000} {
		page, err := s.BrowseEntities(viewer, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Query: query, Limit: 2, Range: &BrowseRange{Start: start}})
		if err != nil || page.PageInfo.Total != 5 {
			t.Fatalf("start %d: %+v %v", start, page.PageInfo, err)
		}
	}
	page, err := s.BrowseEntities(viewer, BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Limit: 2, Sort: []BrowseSortSelection{{Field: "year", Direction: "desc"}, {Field: "title", Direction: "asc"}}, Range: &BrowseRange{Start: 20000}})
	// An unfiltered range past the end clamps to the last row, as it always has.
	if err != nil || page.PageInfo.Start != 4 || len(page.Entries) != 1 {
		t.Fatalf("a deep range under two sorts: %+v %v", page.PageInfo, err)
	}
}

func TestBrowseAdHocPagesWithoutAnchors(t *testing.T) {
	_, s, names := browseFixture(t)
	query, err := ParseBrowseQuery(json.RawMessage(`{"field":"year","operator":"at-least","value":0}`), "query")
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	request := BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Query: query, Limit: 2}
	seen := []string{}
	for {
		page, err := s.BrowseEntities(viewer, request)
		if err != nil {
			t.Fatal(err)
		}
		if page.PageInfo.Total != 5 || len(page.PositionIndex) != 0 {
			t.Fatalf("filtered page metadata %+v, anchors %+v", page.PageInfo, page.PositionIndex)
		}
		for _, entry := range page.Entries {
			seen = append(seen, entry.ID)
		}
		if page.PageInfo.NextCursor == "" {
			if page.PageInfo.HasMore {
				t.Fatalf("terminal page claims more: %+v", page.PageInfo)
			}
			break
		}
		if !page.PageInfo.HasMore {
			t.Fatalf("continuation lacks hasMore: %+v", page.PageInfo)
		}
		request.Cursor = page.PageInfo.NextCursor
	}
	want := []string{}
	for _, name := range []string{"m5", "m1", "m2", "m4", "m3"} {
		want = append(want, names[name].Public)
	}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("cursor order %v", seen)
	}
	request.Cursor = ""
	request.Range = &BrowseRange{Start: 3}
	ranged, err := s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	if ranged.PageInfo.Start != 3 || len(ranged.Entries) != 2 || ranged.Entries[0].ID != names["m4"].Public || ranged.PageInfo.HasMore {
		t.Fatalf("filtered range %+v, entries %+v", ranged.PageInfo, ranged.Entries)
	}
	request.Range.Start = 20000
	ranged, err = s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	if ranged.PageInfo.Start != 20000 || len(ranged.Entries) != 0 {
		t.Fatalf("out-of-result filtered range was clamped: %+v, entries %+v", ranged.PageInfo, ranged.Entries)
	}
	request.Range = nil
	request.Sort = []BrowseSortSelection{{Field: "year", Direction: "desc"}}
	sorted, err := s.BrowseEntities(viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	// Gamma (m3) and Delta (m4) share 2015; ties break in the sort's direction.
	if len(sorted.PositionIndex) != 0 || len(sorted.Entries) != 2 || sorted.Entries[0].ID != names["m4"].Public {
		t.Fatalf("filtered sorted page has anchors or wrong order: %+v, entries %+v", sorted.PositionIndex, sorted.Entries)
	}
}

func TestBrowseAdHocRejectsAnchorNavigation(t *testing.T) {
	_, s, _ := browseFixture(t)
	query, err := ParseBrowseQuery(json.RawMessage(`{"field":"year","operator":"at-least","value":0}`), "query")
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"a"}}
	request := BrowseRequest{Library: "a", Profile: "p", ViewerFence: "f", Pivot: "movies", Query: query, Limit: 2}
	for _, tc := range []struct {
		name, path   string
		seek         *BrowseSeek
		rangeRequest *BrowseRange
	}{
		{"seek", "seek", &BrowseSeek{Prefix: "A"}, nil},
		{"anchor", "range.anchorId", nil, &BrowseRange{Start: 1, AnchorID: "m1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request.Seek, request.Range = tc.seek, tc.rangeRequest
			_, err := s.BrowseEntities(viewer, request)
			var issue *BrowseValidationError
			if !errors.As(err, &issue) || issue.Path != tc.path {
				t.Fatalf("wanted %s validation, got %v", tc.path, err)
			}
		})
	}
}
