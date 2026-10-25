package metadata

import (
	"context"
	"errors"
	"testing"
)

func TestMetadataLibraryChangesRollbackWithTheGlobalDocument(t *testing.T) {
	svc, _, _ := screenFixture(t)
	ctx := context.Background()
	before, err := svc.MetadataSettings(ctx)
	if err != nil || len(before.Libraries) == 0 {
		t.Fatal(before, err)
	}
	row := before.Libraries[0]
	local := AgentLocal
	change := MetadataLibraryChange{ID: row.ID, ExpectedRevision: row.Revision, Agent: &local}
	_, err = svc.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: before.Revision, LocalArtworkLimitBytes: before.LocalArtworkLimitBytes, IncludeAdult: !before.IncludeAdult, Libraries: []MetadataLibraryChange{change, change}}, nil)
	if !errors.Is(err, ErrRepairInput) {
		t.Fatal("duplicate row accepted", err)
	}
	after, err := svc.MetadataSettings(ctx)
	if err != nil || after.Revision != before.Revision || after.IncludeAdult != before.IncludeAdult || after.Libraries[0].Agent != row.Agent || after.Libraries[0].Revision != row.Revision {
		t.Fatal("partial library/global write escaped rollback", after, err)
	}
	change.ExpectedRevision = "0000000000000000000000000000000000000000000000000000000000000000"
	_, err = svc.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: before.Revision, LocalArtworkLimitBytes: before.LocalArtworkLimitBytes, Libraries: []MetadataLibraryChange{change}}, nil)
	if !errors.Is(err, ErrScreenConflict) {
		t.Fatal("stale library row accepted", err)
	}
}

func TestOptionalMetadataLibraryProjectionCannotDisableLegacyGlobalSettings(t *testing.T) {
	svc, db, _ := screenFixture(t)
	publicationExec(t, db, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<513) INSERT INTO libraries(id,name,kind,root) SELECT 'projection-'||i,'Projection '||i,'movie','/synthetic/projection-'||i FROM n`)
	ctx := context.Background()
	before, err := svc.MetadataSettings(ctx)
	if err != nil || before.LibrariesComplete == nil || *before.LibrariesComplete || len(before.Libraries) != 0 {
		t.Fatal("oversized projection blocked or misrepresented global read", before, err)
	}
	after, err := svc.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: before.Revision, LocalArtworkLimitBytes: before.LocalArtworkLimitBytes, IncludeAdult: !before.IncludeAdult}, nil)
	if err != nil || after.IncludeAdult == before.IncludeAdult || after.LibrariesComplete == nil || *after.LibrariesComplete {
		t.Fatal("legacy global write blocked", after, err)
	}
}
