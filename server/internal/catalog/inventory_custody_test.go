package catalog

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
)

func TestInventoryCustodyQueuesRerunAndAlternatesAdmissions(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("custody", "Custody", "movie", root)
	a := c.Movie(lib, filepath.Join(root, "a.mkv"), "A", 2026)
	b := c.Movie(lib, filepath.Join(root, "b.mkv"), "B", 2026)
	d := c.Movie(lib, filepath.Join(root, "d.mkv"), "D", 2026)
	var source string
	if err := c.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id='custody'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	begin := func(item int64, owner string) (compactcatalog.MutationRef, error) {
		var ref compactcatalog.MutationRef
		err := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
			var e error
			ref, e = compactcatalog.BeginEntityMutationTx(context.Background(), tx, compactcatalog.MutationIntent{Entity: item, OwnerKind: "title", OwnerID: owner, OwnerGeneration: 1, Mode: "remove"})
			return e
		})
		return ref, err
	}
	finish := func(ref compactcatalog.MutationRef) {
		t.Helper()
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			_, e := compactcatalog.FinishEntityMutationTx(ctx, tx, ref, "retained")
			return e
		})
	}
	ref, err := begin(a.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	s := New(c.DB)
	c.Exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES('custody-scan','custody','running','unit')`)
	c.Exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision) SELECT 'custody-scan',id,generation,incarnation,root_identity,0 FROM library_sources WHERE id=?`, source)
	c.Exec(`INSERT INTO inventory_directories(job_id,relative_path) VALUES('custody-scan','.')`)
	var sourceRoot string
	if err := c.DB.QueryRow(`SELECT root_identity FROM library_sources WHERE id=?`, source).Scan(&sourceRoot); err != nil {
		t.Fatal(err)
	}
	page := storage.InventoryPage{Complete: true, RootIdentity: sourceRoot, DirectoryIdentity: "fixture-directory", DirectoryRevision: "fixture-page"}
	err = s.CommitInventoryPages(context.Background(), "custody-scan", []InventoryPageWrite{{Relative: ".", Page: page}})
	if !errors.Is(err, ErrInventoryItemOperation) {
		t.Fatal("scan applied behind item custody", err)
	}
	if rows(t, c, `SELECT count(*) FROM inventory_strict_intents WHERE source_id=? AND relative_path='.'`, source) != 1 {
		t.Fatal("no durable source rerun")
	}
	var state string
	if err = c.DB.QueryRow(`SELECT state FROM inventory_directories WHERE job_id='custody-scan' AND relative_path='.'`).Scan(&state); err != nil || state != "pending" {
		t.Fatal("deferred scan advanced", state, err)
	}
	if _, err = begin(b.ID, "second-too-early"); !errors.Is(err, compactcatalog.ErrEntityMutating) {
		t.Fatal("new operations starved queued ingest", err)
	}
	finish(ref)
	// One real normal publication, not an admission/status read, hands off.
	if err = s.CommitInventoryPages(context.Background(), "custody-scan", []InventoryPageWrite{{Relative: ".", Page: page}}); err != nil {
		t.Fatal(err)
	}
	ref, err = begin(b.ID, "second")
	if err != nil {
		t.Fatal("continuous ingest starved item admission", err)
	}
	if _, err = begin(d.ID, "third-too-early"); !errors.Is(err, compactcatalog.ErrEntityMutating) {
		t.Fatal("one ingest turn admitted an endless operation stream", err)
	}
	finish(ref)
	// The next source turn buys the next operation without waiting for a whole
	// library pass. The still-pending source-wide intent is preserved.
	c.Exec(`INSERT INTO inventory_directories(job_id,relative_path) VALUES('custody-scan','next')`)
	if err = s.CommitInventoryPages(context.Background(), "custody-scan", []InventoryPageWrite{{Relative: "next", Page: page}}); err != nil {
		t.Fatal(err)
	}
	ref, err = begin(d.ID, "third")
	if err != nil {
		t.Fatal("second alternation failed", err)
	}
	finish(ref)
	found := false
	for _, v := range dbwork.CatalogItemIngestWaits() {
		if v.Library == "custody" {
			found = true
			if v.LastMillis < 0 || v.MaxMillis < v.LastMillis {
				t.Fatal("bad defer telemetry", v)
			}
		}
	}
	if !found {
		t.Fatal("defer duration absent from diagnostics")
	}
}

func TestInventoryCustodyHealthPauseKeepsRootAndHasNoItemWrites(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("health-pause", "Health", "movie", root)
	item := c.Movie(lib, filepath.Join(root, "a.mkv"), "A", 2026)
	var source string
	if err := c.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id='health-pause'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	inventoryFile(t, c, source, item, "a.mkv", ".")
	var ref compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: item.ID, OwnerKind: "title", OwnerID: "health-pause", OwnerGeneration: 1, Mode: "remove"})
		return e
	})
	var before, after string
	c.DB.QueryRow(`SELECT source_digest FROM catalog_entity_mutations WHERE mutation_id=?`, ref.ID).Scan(&before)
	// This source-level flap must not rewrite the inventory, links or assets.
	// Persistent diagnostic triggers share the real writer connection and fail
	// any accidental item fanout, rather than relying on a fixture row count.
	for _, table := range []string{"inventory_objects", "catalog_assets", "catalog_asset_links"} {
		c.Exec(`CREATE TRIGGER health_pause_` + table + ` BEFORE UPDATE ON ` + table + ` BEGIN SELECT RAISE(ABORT,'health wrote item evidence');END`)
	}
	if err := New(c.DB).SetSourceHealth(context.Background(), source, "offline"); err != nil {
		t.Fatal(err)
	}
	var paused compactcatalog.MutationStep
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		paused, e = RemoveTitleStepTx(ctx, tx, ref)
		return e
	})
	if !paused.SourcePaused || paused.Complete || paused.Progressed {
		t.Fatal("offline did not pause", paused)
	}
	c.DB.QueryRow(`SELECT source_digest FROM catalog_entity_mutations WHERE mutation_id=? AND status='active'`, ref.ID).Scan(&after)
	if after != before {
		t.Fatal("pause changed accepted root digest")
	}
	if err := New(c.DB).SetSourceHealth(context.Background(), source, "ready"); err != nil {
		t.Fatal(err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		m, e := compactcatalog.CheckEntityMutationTx(ctx, tx, ref)
		if e == nil && m.Entity != item.ID {
			t.Fatal("resume lost exact custody")
		}
		return e
	})
	if rows(t, c, `SELECT count(*) FROM catalog_entity_mutations WHERE mutation_id=? AND status='active' AND entity_id=?`, ref.ID, item.ID) != 1 {
		t.Fatal("health flap released custody")
	}
}

func TestInventoryCustodyProbeUsesLiteralPartialPrefix(t *testing.T) {
	c := catalogtest.Open(t)
	plan, err := c.DB.Query(`EXPLAIN QUERY PLAN `+compactcatalog.InventoryCustodySQL, "library")
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	var lines []string
	for plan.Next() {
		var a, b, d int
		var s string
		if err = plan.Scan(&a, &b, &d, &s); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, s)
	}
	if err = plan.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "catalog_entity_mutations_inventory_custody (library_id=?)") || strings.Contains(joined, "SCAN catalog_entity_mutations") || strings.Contains(joined, "TEMP B-TREE") {
		t.Fatal(joined)
	}
	if !strings.Contains(compactcatalog.InventoryCustodySQL, `status<>'terminal'`) {
		t.Fatal("partial predicate was parameterized")
	}
}
