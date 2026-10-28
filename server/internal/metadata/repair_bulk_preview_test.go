package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"testing"
	"time"
)

func TestBulkMetadataPreviewUsesActualSnapshotsAndCASDigests(t *testing.T) {
	service, targets, _ := bulkFixture(t)
	ctx := context.Background()
	request := catalog.BulkMetadataPreviewRequest{}
	for _, target := range targets {
		request.Targets = append(request.Targets, catalog.BulkMetadataTarget{Kind: target.Kind, ContentKind: "movie", ID: target.ID, LibraryID: "movies"})
	}
	gated, err := dbwork.BeginSnapshot(ctx, service.db)
	if err != nil {
		t.Fatal(err)
	}
	defer gated.Rollback()
	selection, err := catalog.New(service.db).BulkMetadataSelectionTx(ctx, gated.Tx(), catalog.Viewer{Profile: "profile", Libraries: []string{"movies"}}, request)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.BulkPreviewTx(ctx, gated.Tx(), selection, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Total != 3 || len(preview.Targets) != 3 {
		t.Fatal(preview)
	}
	for index, target := range targets {
		if preview.Targets[index].ID != target.ID || preview.Targets[index].ExpectedRevision != target.ExpectedRevision {
			t.Fatalf("CAS digest/order changed: %+v %+v", preview.Targets[index], target)
		}
	}
	year := preview.Fields["year"]
	if year.State != "mixed" || year.Value != nil || year.DistinctCount != 3 {
		t.Fatalf("Mixed fabricated a blank: %+v", year)
	}
	if _, ok := preview.Fields["title"]; ok {
		t.Fatal("non-bulk field offered")
	}
	foundTitle := false
	for _, field := range preview.ExcludedFields {
		if field.Field == "title" && field.Label == "Title" && !field.Bulk {
			foundTitle = true
		}
	}
	if !foundTitle || len(preview.SingleItemFields) != len(repairSchema("item", "movie")) {
		t.Fatal("single-item exclusions were guessed or lost", preview.ExcludedFields)
	}
	if preview.Fields["tags"].Entries[0].Value != "Keep" || preview.Fields["tags"].Entries[0].Count != 1 {
		t.Fatal(preview.Fields["tags"])
	}
}
func TestBulkMetadataPreviewQueryReportsMoreThan64DistinctValuesWithoutEnumeratingTargets(t *testing.T) {
	service, db := editorFixture(t)
	fixture := catalogtest.New(t, db)
	ids := []string{}
	for index := 0; index < 70; index++ {
		item := fixture.Movie(fixture.Handle("movies"), fmt.Sprintf("/movies/preview-%d.mkv", index), fmt.Sprintf("Preview %d", index), 1900+index)
		ids = append(ids, item.Public)
	}
	fixture.Drain()
	raw, _ := json.Marshal(ids)
	selection := catalog.BulkMetadataSelection{SQL: `SELECT id FROM catalog_entities WHERE public_id IN(SELECT pid_blob(value) FROM json_each(?))`, Args: []any{string(raw)}, Revision: "query-revision"}
	gated, err := dbwork.BeginSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer gated.Rollback()
	preview, err := service.BulkPreviewTx(context.Background(), gated.Tx(), selection, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	year := preview.Fields["year"]
	if preview.Total != 70 || len(preview.Targets) != 0 || year.DistinctCount != 70 || len(year.Values) != 64 || !year.Truncated || year.State != "mixed" {
		t.Fatalf("untruthful bounded aggregate: total=%d targets=%d field=%+v", preview.Total, len(preview.Targets), year)
	}
}
func TestBulkMetadataPreviewRechecksAuthorityInsideSnapshot(t *testing.T) {
	service, targets, _ := bulkFixture(t)
	gated, err := dbwork.BeginSnapshot(context.Background(), service.db)
	if err != nil {
		t.Fatal(err)
	}
	defer gated.Rollback()
	raw, _ := json.Marshal([]string{targets[0].ID})
	selection := catalog.BulkMetadataSelection{SQL: `SELECT id FROM catalog_entities WHERE public_id IN(SELECT pid_blob(value) FROM json_each(?))`, Args: []any{string(raw)}}
	denied := errors.New("revoked")
	calls := 0
	_, err = service.BulkPreviewTx(context.Background(), gated.Tx(), selection, func(*sql.Tx) error {
		calls++
		if calls > 1 {
			return denied
		}
		return nil
	})
	if !errors.Is(err, denied) || calls != 2 {
		t.Fatalf("authority not rechecked: %d %v", calls, err)
	}
}

func TestBulkMetadataPreviewWholeView48000(t *testing.T) {
	service, db := editorFixture(t)
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, err := tx.Exec(`WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<48000) INSERT INTO catalog_entities(public_id,library_id,kind,title,sort_key,sort_head,year) SELECT randomblob(16),(SELECT id FROM catalog_libraries WHERE library_id='movies'),1,'Whole view '||x,'whole view '||x,'W',2020+(x%2) FROM n`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO catalog_item_details(entity_id) SELECT id FROM catalog_entities WHERE title LIKE 'Whole view %'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// A whole-view preview reads its selection in one snapshot, so it holds
	// the owner-long read permit, as the HTTP route does.
	ctx, release, err := dbwork.AcquireLongRead(ctx, dbwork.LongReadBulkMetadataPreview)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	snapshot, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Rollback()
	preview, err := service.BulkPreviewTx(ctx, snapshot.Tx(), catalog.BulkMetadataSelection{SQL: `SELECT id FROM catalog_entities WHERE title LIKE 'Whole view %'`, Revision: "whole-view-48000"}, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Total != 48000 || len(preview.Targets) != 0 || preview.SelectionRevision != "whole-view-48000" {
		t.Fatalf("whole-view selection was enumerated/capped %+v", preview)
	}
	year := preview.Fields["year"]
	if year.State != "mixed" || year.DistinctCount != 2 || len(year.Values) != 2 || year.Values[0].Count != 24000 || year.Values[1].Count != 24000 {
		t.Fatalf("inexact whole-view aggregate %+v", year)
	}
	if len(preview.Tabs) == 0 {
		t.Fatal("bulk schema tabs absent")
	}
}
