package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"portico.local/server/internal/apispec"
	"portico.local/server/internal/dbwork"
)

func TestHomeManifestDoesNotComposeShelves(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	req := homeRequestFixture("movies", "tv", "music")
	ctx, trace := dbwork.TraceQueryPlans(context.Background())
	doc, err := s.WithContext(ctx).HomeManifest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Rows) == 0 {
		t.Fatal("empty manifest")
	}
	for _, row := range doc.Rows {
		if !row.Deferred || len(row.Entries) != 0 || row.Endpoint == "" || row.ViewerFence != req.Viewer.Fence {
			t.Fatalf("row was composed or unfenced: %+v", row)
		}
	}
	for _, query := range trace() {
		if strings.Contains(query, "progress_activity") || strings.Contains(query, "catalog_browse_rows") || strings.Contains(query, "catalog_rec_") {
			t.Fatalf("manifest read shelf facts: %s", query)
		}
	}
	spec, schema, err := apispec.Schema("HomeDocument")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(doc)
	if issues := spec.ValidateJSON(schema, raw); len(issues) > 0 {
		t.Fatal(issues)
	}
}

func TestDeferredHomeFamilyIsBoundedAndFenced(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	req := homeRequestFixture("movies", "tv", "music")
	row, err := s.HomeSingleRow(req, "for_you", HomeRowPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if row.Deferred || row.ViewerFence != req.Viewer.Fence || len(row.Children) > recPersonalRows {
		t.Fatalf("family %+v", row)
	}
	for _, child := range row.Children {
		if child.Children != nil || child.Deferred || child.Family != "for_you" || child.Limit != 2 || child.ViewerFence != row.ViewerFence {
			t.Fatalf("child %+v", child)
		}
	}
	req.HiddenRowIDs = []string{"for_you"}
	if _, err = s.HomeSingleRow(req, "for_you", HomeRowPage{}); err != ErrHomeRowUnknown {
		t.Fatalf("hidden family reachable: %v", err)
	}
}

func TestContinuePreviewDoesNotReadSiblingRow(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	r := homeRequestFixture("movies", "tv", "music")
	ctx, trace := dbwork.TraceQueryPlans(context.Background())
	row, err := s.WithContext(ctx).HomeSingleRow(r, "continue", HomeRowPage{Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	if row.Hero != nil && (row.Hero.RowID != row.ID || row.Hero.SelectionKey == "") {
		t.Fatalf("unranked or sibling hero: %+v", row.Hero)
	}
	sources := 0
	for _, q := range trace() {
		if strings.Contains(q, "FROM progress_activity") {
			sources++
		}
	}
	if sources > 2 {
		t.Fatalf("preview composed its sibling: %d activity reads", sources)
	}
}
