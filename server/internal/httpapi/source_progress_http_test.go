package httpapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
)

// TestInventoryConfigShowsCommittedProgressWithoutAuthorityChange is the wire
// boundary for SRV002: the existing admin DTO's lastProgressAt reflects a
// committed page, the authority generation does not move, and a real disable
// plus a revoked credential are still refused.
func TestInventoryConfigShowsCommittedProgressWithoutAuthorityChange(t *testing.T) {
	f := newV1Fixture(t, 0)
	ctx := context.Background()
	library, source := "movies", "movies"
	page, err := storage.LocalInventoryPage(storage.InventoryRequest{Root: f.root, RelativePath: ".", Limit: 64})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = dbwork.ExecWrite(ctx, f.db, dbwork.ClassSecurityFence, `UPDATE library_sources SET root_identity=?,identity_confirmed=1 WHERE id=?`, page.RootIdentity, source); err != nil {
		t.Fatal(err)
	}
	src, err := f.cat.LibrarySource(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	job := "http-progress"
	at := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = dbwork.ExecWrite(ctx, f.db, dbwork.ClassBackgroundMedia, `INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,?,'running',?)`, job, library, at); err != nil {
		t.Fatal(err)
	}
	if _, err = dbwork.ExecWrite(ctx, f.db, dbwork.ClassBackgroundMedia, `INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,scan_mode) VALUES(?,?,?,?,?,1,'inventory','full')`, job, source, src.Generation, src.Incarnation, src.RootIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err = dbwork.ExecWrite(ctx, f.db, dbwork.ClassBackgroundMedia, `INSERT INTO inventory_directories(job_id,relative_path) VALUES(?,'.')`, job); err != nil {
		t.Fatal(err)
	}
	if _, err = dbwork.ExecWrite(ctx, f.db, dbwork.ClassBackgroundMedia, `INSERT INTO inventory_source_active(source_id,job_id) VALUES(?,?)`, source, job); err != nil {
		t.Fatal(err)
	}
	read := func() catalog.LibrarySource {
		t.Helper()
		w := f.raw("GET", "/v1/admin/libraries/"+library+"/inventory-config", f.owner.AccessToken, nil, nil)
		if w.Code != 200 {
			t.Fatalf("inventory config: %d %s", w.Code, w.Body.String())
		}
		var document struct {
			Sources []catalog.LibrarySource `json:"sources"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Sources) != 1 {
			t.Fatalf("sources: %+v", document.Sources)
		}
		return document.Sources[0]
	}
	if before := read(); before.LastProgressAt != "" {
		t.Fatalf("a new source projected progress %q", before.LastProgressAt)
	}
	generation := dbwork.AuthorityGeneration()
	if err = f.cat.CommitInventoryPage(ctx, job, ".", "", page); err != nil {
		t.Fatal(err)
	}
	after := read()
	if after.LastProgressAt == "" {
		t.Fatal("the wire field did not carry the committed progress")
	}
	if _, err = time.Parse(time.RFC3339Nano, after.LastProgressAt); err != nil {
		t.Fatalf("wire progress is not the plain string: %q %v", after.LastProgressAt, err)
	}
	if after.ID != source || after.LibraryID != library || !after.Enabled || after.Path != filepath.Clean(f.root) {
		t.Fatalf("progress changed the existing source DTO: %+v", after)
	}
	if got := dbwork.AuthorityGeneration(); got != generation {
		t.Fatalf("the progress page moved authority across the wire boundary: %d → %d", generation, got)
	}
	if _, err = dbwork.ExecWrite(ctx, f.db, dbwork.ClassSecurityFence, `UPDATE library_sources SET enabled=0 WHERE id=?`, source); err != nil {
		t.Fatal(err)
	}
	if disabled := read(); disabled.Enabled {
		t.Fatal("a real source disable was not visible on the existing DTO")
	}
	if got := dbwork.AuthorityGeneration(); got <= generation {
		t.Fatalf("a real source disable did not move AuthorityGeneration: %d", got)
	}
	if err = f.id.LogoutToken(ctx, f.owner.AccessToken); err != nil {
		t.Fatal(err)
	}
	if w := f.raw("GET", "/v1/admin/libraries/"+library+"/inventory-config", f.owner.AccessToken, nil, nil); w.Code != 401 {
		t.Fatalf("a revoked credential read inventory config: %d %s", w.Code, w.Body.String())
	}
	if w := f.raw("GET", "/v1/events?waitMs=0", f.owner.AccessToken, nil, nil); w.Code != 401 {
		t.Fatalf("a revoked credential opened the event feed: %d %s", w.Code, w.Body.String())
	}
}
