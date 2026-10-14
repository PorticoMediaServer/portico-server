package catalog

import (
	"context"
	"encoding/json"
	"portico.local/server/internal/identity"
	"testing"
)

func TestAvailabilityCountersMatchRestrictedPages(t *testing.T) {
	db, s, _ := browseFixture(t)
	age := 13
	viewers := []Viewer{{Profile: "p", Fence: "open", Libraries: []string{"a"}}, {Profile: "p", Fence: "pg", Libraries: []string{"a"}, Restrictions: identity.ContentRestrictions{MaximumAge: &age, BlockUnrated: true}}}
	if err := s.RebuildVisibilityClass(context.Background(), "a", viewers[1].Restrictions); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range viewers {
		for _, state := range []string{"available", "partial", "unavailable"} {
			node, err := ParseBrowseQuery(json.RawMessage(`{"field":"availability","operator":"equals","value":"`+state+`"}`), "query")
			if err != nil {
				t.Fatal(err)
			}
			compiler := browseCompiler{}
			clause, err := compiler.compile(node)
			if err != nil {
				t.Fatal(err)
			}
			restriction, restrictionArgs := EntityRestrictionSQL("e.entity_id", viewer.Restrictions)
			args := append([]any{"a"}, compiler.args...)
			args = append(args, restrictionArgs...)
			var expected int
			if err = db.QueryRow(`SELECT count(*) FROM catalog_browse_rows e WHERE e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.kind=1 AND `+clause+` AND `+restriction, args...).Scan(&expected); err != nil {
				t.Fatal(err)
			}
			out, err := s.BrowseEntities(viewer, BrowseRequest{Library: "a", Pivot: "movies", Query: node, Limit: 40})
			if err != nil || out.PageInfo.Total != expected || len(out.Entries) != expected {
				t.Fatalf("fence=%s state=%s expected=%d out=%+v err=%v", viewer.Fence, state, expected, out, err)
			}
		}
	}
	var rollup, rows int
	if err := db.QueryRow(`SELECT sum(total) FROM catalog_browse_availability_counts WHERE container=-1`).Scan(&rollup); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT count(*) FROM catalog_browse_rows`).Scan(&rows)
	if rollup != rows {
		t.Fatalf("rollup=%d rows=%d", rollup, rows)
	}
}

func TestColdRestrictedBrowseDoesNotBlankLargeLibraries(t *testing.T) {
	db, s, _ := browseFixture(t)
	// Crossing the former cold-class threshold does not require a full catalogue.
	// Give existing titles a maintained count of a large eligible title bucket.
	db.Exec(`UPDATE catalog_browse_buckets SET total=total+2000 WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id='a') AND kind=1 AND axis=0 AND rating_key='pg'`)
	age := 13
	viewer := Viewer{Profile: "cold", Fence: "cold", Libraries: []string{"a"}, Restrictions: identity.ContentRestrictions{MaximumAge: &age, BlockUnrated: true}}
	out, err := s.BrowseEntities(viewer, BrowseRequest{Library: "a", Pivot: "movies", Limit: 40})
	if err != nil || len(out.Entries) != 1 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
