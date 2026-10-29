package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"portico.local/server/internal/dbwork"
)

func TestReadingBrowseImportingPagesSkipExactCountAndAnchors(t *testing.T) {
	c, s, viewer, items := readingBrowseFixture(t, "book")
	c.Drain()
	c.Exec(`UPDATE metadata_owner_fields SET value='next pass' WHERE entity_id=? AND field='author'`, items[0].ID)
	request := BrowseRequest{Library: "reading", Limit: 1}
	var seen []string
	for turn := 0; turn < 3; turn++ {
		ctx, trace := dbwork.TraceStatements(context.Background())
		page, err := s.WithContext(ctx).BrowseEntities(viewer, request)
		if err != nil || !page.Importing || !page.PageInfo.TotalIsLowerBound || len(page.Entries) != 1 || len(page.PositionIndex) != 0 {
			t.Fatalf("turn %d: %+v %v", turn, page, err)
		}
		for _, statement := range trace() {
			sql := strings.ToUpper(statement)
			if strings.Contains(sql, "COUNT(*)") || strings.Contains(sql, "ROW_NUMBER()") {
				t.Fatal("importing page computed whole-view count/anchors", statement)
			}
		}
		seen = append(seen, page.Entries[0].ID)
		for _, prior := range seen[:len(seen)-1] {
			if prior == seen[len(seen)-1] {
				t.Fatal("duplicate", seen)
			}
		}
		request.Cursor = page.PageInfo.NextCursor
		if request.Cursor == "" {
			t.Fatal("lost importing continuation")
		}
		cursor, err := s.readSignedBrowseCursor(request.Cursor)
		if err != nil || cursor.TotalPresent || !strings.HasPrefix(cursor.Fingerprint, readingImportingCursorPrefix) {
			t.Fatalf("not the existing lower-bound cursor envelope: %+v %v", cursor, err)
		}
		c.Exec(`UPDATE metadata_owner_fields SET value=value||'x' WHERE entity_id=? AND field='author'`, items[0].ID)
	}
	// Once a new image is published, the old keyset cannot cross it.
	c.Drain()
	if _, err := s.BrowseEntities(viewer, request); !errors.Is(err, ErrReadingProjectionChanged) {
		t.Fatalf("cursor crossed publication: %v", err)
	}
}

func TestReadingBrowseClassifierChangeIsBuildingUntilNewImage(t *testing.T) {
	c, s, viewer, _ := readingBrowseFixture(t, "book")
	c.Drain()
	c.Exec(`UPDATE reading_attribution_state SET publication_rule_version=publication_rule_version+1 WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id='reading')`)
	caps, err := s.BrowseCapabilitiesForViewer(viewer, "reading", "")
	if err != nil || !caps.Importing {
		t.Fatalf("classifier-change capabilities: %+v %v", caps, err)
	}
	page, err := s.BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 1})
	if err != nil || !page.Importing || len(page.Entries) != 0 || !page.PageInfo.TotalIsLowerBound {
		t.Fatalf("classifier-change page: %+v %v", page, err)
	}
	c.Drain()
	page, err = s.BrowseEntities(viewer, BrowseRequest{Library: "reading", Limit: 1})
	if err != nil || page.Importing || len(page.Entries) != 1 {
		t.Fatalf("new classifier image: %+v %v", page, err)
	}
}
