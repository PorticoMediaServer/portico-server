package catalog

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
	"testing"
)

func TestBulkMetadataPreviewKeepsDisplayedShowAndRefusesWrongScope(t *testing.T) {
	service, _, _, viewer, _, fixture, names := bulkFixture(t, 3)
	ctx := context.Background()
	gated, err := dbwork.BeginSnapshot(ctx, fixture.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer gated.Rollback()
	target := BulkMetadataTarget{Kind: "show", ContentKind: "show", ID: names["show"].Public, LibraryID: "lib"}
	selected, err := service.BulkMetadataSelectionTx(ctx, gated.Tx(), viewer, BulkMetadataPreviewRequest{Targets: []BulkMetadataTarget{target}})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var id int64
	if err = gated.Tx().QueryRowContext(ctx, `SELECT count(*),min(id) FROM (`+selected.SQL+`)`, selected.Args...).Scan(&count, &id); err != nil {
		t.Fatal(err)
	}
	if count != 1 || id != names["show"].ID {
		t.Fatalf("show preview expanded into descendants: count=%d id=%d", count, id)
	}
	viewer.Libraries = []string{}
	if _, err = service.BulkMetadataSelectionTx(ctx, gated.Tx(), viewer, BulkMetadataPreviewRequest{Targets: []BulkMetadataTarget{target}}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("wrong scope disclosed metadata: %v", err)
	}
}
func TestBulkMetadataPreviewQueryBindsRevisionAndDoesNotExpandShows(t *testing.T) {
	service, _, _, viewer, _, fixture, names := bulkFixture(t, 3)
	ctx := context.Background()
	sorts := []BrowseSortSelection{{Field: "title", Direction: "asc"}}
	browse, err := service.BrowseEntities(viewer, BrowseRequest{Library: "lib", Pivot: "shows", Limit: 1, Sort: sorts})
	if err != nil {
		t.Fatal(err)
	}
	gated, err := dbwork.BeginSnapshot(ctx, fixture.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer gated.Rollback()
	request := BulkMetadataPreviewRequest{Selector: &JobSelector{Query: &JobQuery{LibraryID: "lib", Pivot: "shows", Sort: sorts}}, Expected: &JobExpected{CatalogRevision: browse.PageInfo.Revision}}
	selected, err := service.BulkMetadataSelectionTx(ctx, gated.Tx(), viewer, request)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var id int64
	if err = gated.Tx().QueryRowContext(ctx, `SELECT count(*),min(id) FROM (`+selected.SQL+`)`, selected.Args...).Scan(&count, &id); err != nil {
		t.Fatal(err)
	}
	if count != 1 || id != names["show"].ID {
		t.Fatalf("query preview changed entity kind: %d %d", count, id)
	}
	request.Expected = &JobExpected{CatalogRevision: "stale"}
	if _, err = service.BulkMetadataSelectionTx(ctx, gated.Tx(), viewer, request); !errors.Is(err, ErrStaleContinuation) {
		t.Fatalf("stale query accepted: %v", err)
	}
}
