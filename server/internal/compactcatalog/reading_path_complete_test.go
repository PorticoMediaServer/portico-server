package compactcatalog_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
)

type completeFixture struct {
	t                               *testing.T
	c                               *catalogtest.Catalog
	root, source, inc, library, job string
	handle                          int64
	serial                          int
}
type completePublication struct {
	item, asset              int64
	object, relative, format string
	kind                     compactcatalog.Kind
}

func newCompleteFixture(t *testing.T, c *catalogtest.Catalog, name string) *completeFixture {
	t.Helper()
	if c == nil {
		c = catalogtest.Open(t)
	}
	f := &completeFixture{t: t, c: c, root: t.TempDir(), library: name, job: "job-" + name}
	f.handle = c.Library(name, name, "book", f.root)
	if err := c.DB.QueryRow(`SELECT id,incarnation FROM library_sources WHERE library_id=?`, name).Scan(&f.source, &f.inc); err != nil {
		t.Fatal(err)
	}
	c.Exec(`UPDATE library_sources SET identity_confirmed=1,root_identity='root' WHERE id=?`, f.source)
	c.Exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,?,'running','now')`, f.job, name)
	c.Exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision) VALUES(?,?,1,?,'root',1)`, f.job, f.source, f.inc)
	return f
}
func (f *completeFixture) add(relative, format string, item int64) completePublication {
	f.t.Helper()
	f.serial++
	if item == 0 {
		item = f.c.Entity(compactcatalog.Entity{Library: f.handle, Kind: compactcatalog.Ebook, Key: fmt.Sprint(f.serial), Title: relative}, nil).ID
	}
	asset, token := f.c.File(item, filepath.Join(f.root, filepath.FromSlash(relative)), 0)
	object := fmt.Sprintf("%s-%06d", f.library, f.serial)
	f.c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,directory_path,revision,evidence_json,size,modified_ns,seen_job) VALUES(?,?,?,?,?,?,'r1','{}',1,1,?)`, object, f.source, token, f.inc, filepath.FromSlash(relative), filepath.Dir(filepath.FromSlash(relative)), f.job)
	f.c.Exec(`INSERT INTO reading_publications VALUES(?,?,?)`, item, asset, format)
	return completePublication{item, asset, object, relative, format, compactcatalog.Ebook}
}
func (f *completeFixture) settle() {
	f.c.Exec(`DELETE FROM catalog_dirty WHERE domain IN(20,28)`)
	f.c.Exec(`DELETE FROM catalog_derivation_failures WHERE domain IN(20,28)`)
	f.c.Exec(`DELETE FROM catalog_fanouts WHERE domain=28`)
}
func (f *completeFixture) seal() {
	f.c.Exec(`UPDATE inventory_runs SET phase='reconciling',authoritative=1 WHERE job_id=?`, f.job)
	f.c.Exec(`UPDATE inventory_runs SET phase='sidecars' WHERE job_id=?`, f.job)
}
func completeStep(t *testing.T, c *catalogtest.Catalog, limit int) int {
	t.Helper()
	n := 0
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		n, err = compactcatalog.StepReadingPaths(ctx, tx, limit)
		return err
	})
	if n > min(limit, 128) {
		t.Fatalf("raw work %d > budget %d", n, limit)
	}
	return n
}
func scalar(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func completeUntil(t *testing.T, c *catalogtest.Catalog, predicate func() bool) {
	t.Helper()
	for i := 0; i < 5000; i++ {
		if predicate() {
			return
		}
		completeStep(t, c, 7)
	}
	t.Fatal("bounded worker did not reach target")
}
func completeDrain(t *testing.T, c *catalogtest.Catalog) {
	t.Helper()
	completeUntil(t, c, func() bool {
		return scalar(t, c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 && scalar(t, c.DB, `SELECT count(*) FROM reading_path_build`) == 0 && scalar(t, c.DB, `SELECT count(*) FROM reading_path_scope WHERE queued=1`) == 0
	})
}
func completeValues(t *testing.T, c *catalogtest.Catalog, item int64) compactcatalog.ReadingValues {
	t.Helper()
	var v compactcatalog.ReadingValues
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		v, err = compactcatalog.ReadingAutomaticFieldsTx(ctx, tx, item)
		return err
	})
	return v
}
func onlyPath(v compactcatalog.ReadingValues) compactcatalog.ReadingValues {
	v.Title = ""
	v.PublicationType = nil
	return v
}
func assertCompleteValue(t *testing.T, c *catalogtest.Catalog, p completePublication, want compactcatalog.ReadingValues) {
	t.Helper()
	got := onlyPath(completeValues(t, c, p.item))
	if !reflect.DeepEqual(got, onlyPath(want)) {
		t.Fatalf("%s\ngot  %+v\nwant %+v", p.relative, got, want)
	}
}

func TestReadingCompletePureRuleCorpusAndUnchangedRescan(t *testing.T) {
	f := newCompleteFixture(t, nil, "corpus")
	var pubs []completePublication
	for _, r := range []string{"Frank Herbert/Dune/Dune 01 - Dune.epub", "Frank Herbert/Dune/Dune 02 - Dune Messiah.epub", "Frank Herbert/Dune/Books/Dune 03 - Children.epub", "Écrivain/Étoiles/Étoiles 01 - Été.epub", "Écrivain/Étoiles/Étoiles 02 - Lune.epub", "Solo/Only.epub", "Mixed/Number 01.epub", "Mixed/Unnumbered.epub", "Explicit Writer - Root 03.epub", "Books/Generic 04.epub"} {
		pubs = append(pubs, f.add(r, "epub", 0))
	}
	// Lowest asset representative wins even if raw object order is reversed.
	alt := f.add("Frank Herbert/Dune/Books/Unnumbered.pdf", "pdf", pubs[0].item)
	f.c.Exec(`UPDATE inventory_objects SET id='000-first-raw' WHERE id=?`, alt.object)
	// Unsupported/no-publication raw rows and MOBI never enter the path corpus.
	mobi := f.add("Frank Herbert/Dune/Dune 99.mobi", "mobi", 0)
	f.settle()
	f.seal()
	completeDrain(t, f.c)
	byKey := map[string][]compactcatalog.ReadingRuleInput{}
	for _, p := range pubs {
		key, _, err := compactcatalog.ReadingPathFolderEvidence(compactcatalog.ReadingRuleInput{Item: p.item, SourceRoot: f.source, RelativePath: p.relative, Kind: p.kind})
		if err != nil {
			t.Fatal(err)
		}
		byKey[key] = append(byKey[key], compactcatalog.ReadingRuleInput{Item: p.item, SourceRoot: f.source, RelativePath: p.relative, Kind: p.kind})
	}
	expected := map[int64]compactcatalog.ReadingValues{}
	for _, inputs := range byKey {
		got, err := compactcatalog.ReadingPathValues(inputs)
		if err != nil {
			t.Fatal(err)
		}
		for id, v := range got {
			expected[id] = v
		}
	}
	for _, p := range pubs {
		assertCompleteValue(t, f.c, p, expected[p.item])
	}
	assertCompleteValue(t, f.c, mobi, compactcatalog.ReadingValues{})
	beforeSeq := scalar(t, f.c.DB, `SELECT sequence FROM reading_path_clock`)
	beforeInput := scalar(t, f.c.DB, `SELECT revision FROM reading_path_inputs WHERE library_id=?`, f.handle)
	beforeDesired := scalar(t, f.c.DB, `SELECT sum(desired) FROM reading_path_scope`)
	f.c.Exec(`UPDATE inventory_objects SET seen_job='unchanged',analysis_state='done' WHERE source_id=?`, f.source)
	if got := scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`); got != 0 {
		t.Fatalf("unchanged scan queued %d fanouts", got)
	}
	if scalar(t, f.c.DB, `SELECT revision FROM reading_path_inputs WHERE library_id=?`, f.handle) != beforeInput || scalar(t, f.c.DB, `SELECT sum(desired) FROM reading_path_scope`) != beforeDesired || scalar(t, f.c.DB, `SELECT sequence FROM reading_path_clock`) != beforeSeq {
		t.Fatal("unchanged bookkeeping changed path work/publication")
	}
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE directory_key='' AND queued=1`) != 0 {
		t.Fatal("root/generic path queued whole-root worker")
	}
}

func TestReadingCompleteIndependentCoverageAndAvailabilityFailure(t *testing.T) {
	f := newCompleteFixture(t, nil, "isolation")
	good := f.add("Good/Series/Series 01.epub", "epub", 0)
	f.add("Good/Series/Books/Series 02.epub", "epub", 0)
	blocked := f.add("Unreadable/Book.epub", "epub", 0)
	generic := f.add("Generic/Series 01.epub", "epub", 0)
	f.add("Generic/Books/Series 02.epub", "epub", 0)
	failed := f.add("Failed/Book.epub", "epub", 0)
	f.settle()
	f.c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := compactcatalog.ReadingPathBlockTx(ctx, tx, f.job, "Unreadable", "directory_unreadable"); err != nil {
			return err
		}
		return compactcatalog.ReadingPathBlockTx(ctx, tx, f.job, "Generic/Books", "directory_unreadable")
	})
	f.c.Exec(`INSERT INTO catalog_derivation_failures(domain,entity_id,reason,failed_ms) VALUES(20,?,'permanent',1)`, failed.item)
	f.seal()
	f.c.Exec(`UPDATE jobs SET status='complete' WHERE id=?`, f.job)
	completeUntil(t, f.c, func() bool {
		return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE directory_key='Good/Series' AND desired=completed`) == 1 && scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 && scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build`) == 0
	})
	v := completeValues(t, f.c, good.item)
	if v.Author == nil || v.Author.Value != "Good" || v.Series == nil || v.Series.Value != "Series" {
		t.Fatalf("healthy folder held back: %+v", v)
	}
	for _, p := range []completePublication{blocked, generic, failed} {
		if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE item_id=? AND author_source='folder'`, p.item) != 0 {
			t.Fatalf("untrusted folder published: %s", p.relative)
		}
	}
	for _, folder := range []string{"Unreadable", "Generic", "Failed"} {
		if scalar(t, f.c.DB, `SELECT count(*) FROM inventory_scan_issues WHERE job_id=? AND relative_path=? AND reason LIKE 'reading_path_%'`, f.job, folder) == 0 {
			t.Fatalf("unpublished folder missing from job history: %s", folder)
		}
	}
	// Clearing dirty alone must never clear a failure marker.
	f.c.Exec(`INSERT INTO catalog_dirty VALUES(20,?,1)`, failed.item)
	f.c.Exec(`DELETE FROM catalog_dirty WHERE domain=20 AND entity_id=?`, failed.item)
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_availability_pending WHERE entity_id=?`, failed.item) != 1 {
		t.Fatal("failure disappeared when dirty cleared")
	}
	f.c.Exec(`DELETE FROM catalog_derivation_failures WHERE domain=20 AND entity_id=?`, failed.item)
	completeUntil(t, f.c, func() bool {
		return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE directory_key='Failed' AND completed=desired`) == 1
	})
}

func TestReadingCompleteChurningLibraryCannotStarveOtherLibrary(t *testing.T) {
	a := newCompleteFixture(t, nil, "churn")
	pa := a.add("Author/Series/Series 01.epub", "epub", 0)
	a.add("Author/Series/Series 02.epub", "epub", 0)
	b := newCompleteFixture(t, a.c, "steady")
	pb := b.add("Writer/Series/Series 01.epub", "epub", 0)
	b.add("Writer/Series/Series 02.epub", "epub", 0)
	a.settle()
	a.seal()
	b.seal()
	completeUntil(t, a.c, func() bool { return scalar(t, a.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 })
	completed := false
	for i := 0; i < 1000; i++ {
		// A genuine path input changes on EVERY quantum. The steady library still
		// captures, stages, activates and flattens through the one global slot.
		a.c.Exec(`UPDATE inventory_objects SET revision=? WHERE id=?`, fmt.Sprintf("churn-%d", i), pa.object)
		a.settle()
		completeStep(t, a.c, 3)
		if scalar(t, a.c.DB, `SELECT count(*) FROM reading_path_scope WHERE library_id=? AND desired=completed AND completed>0`, b.handle) == 1 {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatal("genuine churn starved the other library")
	}
	v := completeValues(t, a.c, pb.item)
	if v.Author == nil || v.Author.Value != "Writer" || v.Series == nil {
		t.Fatalf("steady publication missing: %+v", v)
	}
}

func TestReadingCompleteRestartEveryDurablePhase(t *testing.T) {
	for _, target := range []string{"pending", "seal", "capture", "capture-partial", "stage", "stage-partial", "ready", "flatten", "flatten-partial", "cleanup", "cleanup-partial", "abort", "abort-partial"} {
		t.Run(target, func(t *testing.T) {
			f := newCompleteFixture(t, nil, "restart")
			var pubs []completePublication
			for i := 0; i < 10; i++ {
				pubs = append(pubs, f.add(fmt.Sprintf("Author/Series/Series %02d.epub", i), "epub", 0))
			}
			f.settle()
			f.seal()
			reached := false
			for i := 0; i < 2000; i++ {
				var phase string
				var capture, stage, flat int64
				err := f.c.DB.QueryRow(`SELECT phase,(SELECT count(*) FROM reading_path_capture WHERE build_id=b.id),(SELECT count(*) FROM reading_path_stage WHERE build_id=b.id),after_flatten_item FROM reading_path_build b LIMIT 1`).Scan(&phase, &capture, &stage, &flat)
				if err != nil && err != sql.ErrNoRows {
					t.Fatal(err)
				}
				switch target {
				case "pending":
					reached = i == 0
				case "seal":
					reached = scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE sealed=1`) > 0 && phase == ""
				case "capture", "stage", "ready", "flatten", "cleanup":
					reached = phase == target
				case "capture-partial":
					reached = phase == "capture" && capture > 0 && capture < 10
				case "stage-partial":
					reached = phase == "stage" && stage > 0 && stage < 10
				case "flatten-partial":
					reached = phase == "flatten" && flat > 0 && scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE author_source='folder'`) < 10
				case "cleanup-partial":
					reached = phase == "cleanup" && capture > 0 && capture < 10
				case "abort", "abort-partial":
					if phase == "stage" && stage > 0 {
						f.c.Exec(`UPDATE inventory_objects SET revision='new' WHERE id=?`, pubs[0].object)
						f.settle()
					}
					reached = phase == "abort" && (target == "abort" || capture > 0 && capture < 10)
				}
				if reached {
					break
				}
				completeStep(t, f.c, 3)
			}
			if !reached {
				t.Fatal("never reached durable checkpoint")
			}
			// Close all handles; reopen the actual disk file with zero in-memory worker
			// progress. This also checks baseline compatibility after interrupted work.
			var dbfile string
			var seq int
			var dbname string
			if err := f.c.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &dbname, &dbfile); err != nil {
				t.Fatal(err)
			}
			if err := f.c.DB.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := persistence.Open(dbfile)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			f.c.DB = db
			completeDrain(t, f.c)
			for _, p := range pubs {
				v := completeValues(t, f.c, p.item)
				if v.Author == nil || v.Author.Value != "Author" || v.Series == nil || v.Series.Value != "Series" {
					t.Fatalf("restart %s lost COMPLETE: %+v", target, v)
				}
			}
			if scalar(t, db, `SELECT count(*) FROM reading_path_capture`)+scalar(t, db, `SELECT count(*) FROM reading_path_stage`)+scalar(t, db, `SELECT count(*) FROM reading_path_representative`) != 0 {
				t.Fatal("scratch leaked after restart")
			}
		})
	}
}

func TestReadingCompleteRawRejectedPrefixAndManyVersionsAreBounded(t *testing.T) {
	f := newCompleteFixture(t, nil, "raw")
	p := f.add("Shelf/Series 01.epub", "epub", 0)
	for i := 0; i < 135; i++ {
		v := f.add(fmt.Sprintf("Shelf/Books/Series %03d.pdf", i), "pdf", p.item)
		if i < 129 {
			f.c.Exec(`UPDATE inventory_objects SET unsupported_reason='reject' WHERE id=?`, v.object)
		}
	}
	f.settle()
	f.seal()
	completeDrain(t, f.c)
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE author='Shelf'`) != 7 {
		t.Fatal("raw prefix skipped eligible versions or counted versions as titles")
	}
}

func TestReadingCompleteMissingRetainsLastCompleteAndEmptyClears(t *testing.T) {
	f := newCompleteFixture(t, nil, "missing")
	one := f.add("Frank Herbert/Dune/Dune 01 - Dune.epub", "epub", 0)
	two := f.add("Frank Herbert/Dune/Dune 02 - Dune Messiah.epub", "epub", 0)
	f.settle()
	f.seal()
	completeDrain(t, f.c)
	sequence := scalar(t, f.c.DB, `SELECT max(publisher_sequence) FROM reading_path_head`)
	f.c.Exec(`UPDATE inventory_objects SET state='missing' WHERE source_id=?`, f.source)
	f.c.Exec(`UPDATE catalog_assets SET available=0 WHERE id IN(?,?)`, one.asset, two.asset)
	f.c.Exec(`UPDATE catalog_asset_links SET available=0 WHERE entity_id IN(?,?)`, one.item, two.item)
	f.settle()
	completeDrain(t, f.c)
	for _, p := range []completePublication{one, two} {
		v := completeValues(t, f.c, p.item)
		if v.Author == nil || v.Author.Value != "Frank Herbert" || v.Series == nil || v.Series.Value != "Dune" {
			t.Fatalf("missing last COMPLETE changed: %+v", v)
		}
	}
	if scalar(t, f.c.DB, `SELECT min(publisher_sequence) FROM reading_path_head`) != sequence {
		t.Fatal("inherited missing head was retimestamped")
	}
	f.c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, err := compactcatalog.ReadingPublishPathTx(ctx, tx, one.item, one.asset, one.object, "r1", compactcatalog.ReadingValues{})
		if err != nil {
			return err
		}
		return compactcatalog.FlushReadingPathSemanticTx(ctx, tx)
	})
	assertCompleteValue(t, f.c, one, compactcatalog.ReadingValues{})
}

func TestReadingCompleteLatestWitnessCannotResurrectAndFlattenIsInvisible(t *testing.T) {
	f := newCompleteFixture(t, nil, "latest")
	p := f.add("Author/Series/Series 01.epub", "epub", 0)
	f.add("Author/Series/Series 02.epub", "epub", 0)
	f.settle()
	f.seal()
	completeUntil(t, f.c, func() bool {
		return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='flatten'`) == 1
	})
	before := completeValues(t, f.c, p.item)
	revision := scalar(t, f.c.DB, `SELECT input_generation FROM reading_attribution_state WHERE library_id=?`, f.handle)
	// A newer direct head must survive flatten and win over the older overlay.
	f.c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, err := compactcatalog.ReadingPublishPathTx(ctx, tx, p.item, p.asset, p.object, "r1", compactcatalog.ReadingValues{Author: &compactcatalog.ReadingValue{Value: "Latest", Source: compactcatalog.ReadingFileName}})
		if err != nil {
			return err
		}
		return compactcatalog.FlushReadingPathSemanticTx(ctx, tx)
	})
	semantic := scalar(t, f.c.DB, `SELECT input_generation FROM reading_attribution_state WHERE library_id=?`, f.handle)
	if semantic != revision+1 {
		t.Fatal("point publication missing semantic invalidation")
	}
	completeDrain(t, f.c)
	if scalar(t, f.c.DB, `SELECT input_generation FROM reading_attribution_state WHERE library_id=?`, f.handle) != semantic {
		t.Fatal("flatten/cleanup invalidated semantic revision")
	}
	v := completeValues(t, f.c, p.item)
	if before.Series == nil || v.Author == nil || v.Author.Value != "Latest" || v.Series != nil {
		t.Fatalf("new head lost to overlay: %+v", v)
	}
	f.c.Exec(`DELETE FROM inventory_objects WHERE id=?`, p.object)
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE item_id=?`, p.item) != 1 {
		t.Fatal("object deletion erased supersession watermark")
	}
	assertCompleteValue(t, f.c, p, compactcatalog.ReadingValues{})
	f.c.Exec(`DELETE FROM reading_publications WHERE item_id=? AND asset_id=?`, p.item, p.asset)
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE item_id=?`, p.item) != 0 {
		t.Fatal("publication deletion retained head")
	}
}

func TestReadingCompleteInterimFilenameRulesAndCanonicalOwnerPrecedence(t *testing.T) {
	f := newCompleteFixture(t, nil, "interim")
	explicit := f.add("Author/Series/Writer - Issue 01 (2026-01).epub", "epub", 0)
	nofacts := f.add("Author/Series/Plain.epub", "epub", 0)
	root := f.add("Books/Writer - Root #03.epub", "epub", 0)
	// The source is still OPEN. Only the approved immediate fields are visible.
	f.settle()
	completeUntil(t, f.c, func() bool { return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 })
	v := completeValues(t, f.c, explicit.item)
	if v.Author == nil || v.Author.Value != "Writer" || v.IssueDate == nil || v.IssueDate.Value != "2026-01" || v.Series != nil || v.SeriesNumber != nil {
		t.Fatalf("interim rule: %+v", v)
	}
	assertCompleteValue(t, f.c, nofacts, compactcatalog.ReadingValues{})
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE item_id=? AND author IS NULL AND series IS NULL`, nofacts.item) != 1 {
		t.Fatal("missing explicit empty interim bundle")
	}
	if v := completeValues(t, f.c, root.item); v.SeriesNumber == nil || v.SeriesNumber.Value != 3 {
		t.Fatalf("root/generic independent number lost: %+v", v)
	}
	f.seal()
	completeDrain(t, f.c)
	previous := completeValues(t, f.c, explicit.item)
	// Same accepted object revision on a rename retains the entire last bundle.
	f.c.Exec(`UPDATE catalog_assets SET path=? WHERE id=?`, filepath.Join(f.root, "Author", "Series", "Different Writer - Issue 04 (2026-02).epub"), explicit.asset)
	f.c.Exec(`UPDATE inventory_objects SET relative_path='Author/Series/Different Writer - Issue 04 (2026-02).epub' WHERE id=?`, explicit.object)
	f.settle()
	completeUntil(t, f.c, func() bool { return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 })
	if got := completeValues(t, f.c, explicit.item); !reflect.DeepEqual(got, previous) {
		t.Fatal("accepted interim bundle was spliced/replaced during rename")
	}
	// File metadata and owner values remain independent and higher priority.
	f.c.Exec(`INSERT INTO reading_source_values(item_id,asset_id,field,text_value,source,inventory_object_id,source_revision) VALUES(?,?,'author','Embedded','file-metadata',?,'r1')`, explicit.item, explicit.asset, explicit.object)
	if got := completeValues(t, f.c, explicit.item); got.Author == nil || got.Author.Value != "Embedded" {
		t.Fatal("file metadata lost precedence")
	}
	f.c.Exec(`INSERT INTO metadata_owner_fields(kind,entity_id,field,value,automatic_value,locked,actor,observed_at) VALUES('item',?,'author','Owner','Embedded',1,'owner','now')`, explicit.item)
	f.c.Write(func(ctx context.Context, tx *sql.Tx) error {
		got, err := compactcatalog.ReadingAcceptedValuesTx(ctx, tx, []int64{explicit.item})
		if err == nil && (got[explicit.item].Author == nil || got[explicit.item].Author.Value != "Owner") {
			t.Fatal("owner lost precedence")
		}
		return err
	})
	// Canonical selection includes MOBI metadata but the path publisher excludes it.
	alt := f.add("Author/Series/Alternative.mobi", "mobi", explicit.item)
	f.c.Exec(`INSERT INTO reading_source_values(item_id,asset_id,field,text_value,source,inventory_object_id,source_revision) VALUES(?,?,'author','MOBI metadata','file-metadata',?,'r1')`, explicit.item, alt.asset, alt.object)
	f.c.Exec(`UPDATE inventory_objects SET state='missing' WHERE id=?`, explicit.object)
	if got := completeValues(t, f.c, explicit.item); got.Author == nil || got.Author.Value != "MOBI metadata" {
		t.Fatalf("canonical available MOBI metadata lost: %+v", got)
	}
}

func TestReadingCompleteOverlappingSourceSupersessionBeforeAcceptance(t *testing.T) {
	f := newCompleteFixture(t, nil, "overlap")
	p := f.add("Author/Series/Series 01.epub", "epub", 0)
	q := f.add("Author/Series/Series 02.epub", "epub", 0)
	f.settle()
	f.seal()
	completeDrain(t, f.c)
	old := completeValues(t, f.c, p.item)
	if old.Author == nil || old.Author.Value != "Author" {
		t.Fatal("first publisher absent")
	}
	otherRoot := filepath.Join(f.root, "Author")
	f.c.Exec(`INSERT INTO library_sources(id,library_id,configured_root,root,incarnation,root_identity,identity_confirmed) VALUES('overlapping-source',?,?,?,'other-inc','other-root',1)`, f.library, otherRoot, otherRoot)
	f.c.Exec(`INSERT INTO jobs VALUES('other-job',?,'running',0,'','now')`, f.library)
	f.c.Exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision) VALUES('other-job','overlapping-source',1,'other-inc','other-root',1)`)
	for _, v := range []completePublication{p, q} {
		f.c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,directory_path,revision,evidence_json,size,modified_ns) SELECT 'other-'||o.id,'overlapping-source',o.asset_id,'other-inc',substr(o.relative_path,8),'Series','other-revision','{}',1,1 FROM inventory_objects o WHERE id=?`, v.object)
	}
	f.settle()
	f.c.Exec(`UPDATE inventory_runs SET phase='reconciling',authoritative=1 WHERE job_id='other-job'`)
	f.c.Exec(`UPDATE inventory_runs SET phase='sidecars' WHERE job_id='other-job'`)
	completeUntil(t, f.c, func() bool {
		return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build b JOIN reading_path_scope s ON s.id=b.scope_id WHERE s.source_id='overlapping-source' AND b.phase='flatten'`) == 1
	})
	got := completeValues(t, f.c, p.item)
	if got.Author != nil || got.Series == nil {
		t.Fatalf("second publisher did not replace the entire bundle: %+v", got)
	}
	// A still-valid A head exists, but B wins even AFTER B's witness disappears.
	f.c.Exec(`DELETE FROM inventory_objects WHERE id=?`, "other-"+p.object)
	assertCompleteValue(t, f.c, p, compactcatalog.ReadingValues{})
	// Complete only the already activated build, without starting a new A build.
	completeUntil(t, f.c, func() bool { return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build`) == 0 })
	assertCompleteValue(t, f.c, p, compactcatalog.ReadingValues{})
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_head WHERE item_id=? AND object_id=?`, p.item, "other-"+p.object) != 1 {
		t.Fatal("invalid newest witness was not retained as watermark")
	}
}

func TestReadingCompleteSameSnapshotOverlayAndHeadDuringFlatten(t *testing.T) {
	f := newCompleteFixture(t, nil, "snapshot")
	p := f.add("Author/Series/Series 01.epub", "epub", 0)
	f.add("Author/Series/Series 02.epub", "epub", 0)
	f.settle()
	f.seal()
	completeUntil(t, f.c, func() bool {
		return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='flatten'`) == 1
	})
	// A normal short read snapshot, not a worker-held WAL lease. Writer flatten,
	// pointer removal and cleanup must leave that snapshot's answer unchanged.
	tx, err := f.c.DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	first, err := compactcatalog.ReadingAutomaticFieldsTx(context.Background(), tx, p.item)
	if err != nil {
		t.Fatal(err)
	}
	completeDrain(t, f.c)
	second, err := compactcatalog.ReadingAutomaticFieldsTx(context.Background(), tx, p.item)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("flatten/cleanup changed an existing snapshot")
	}
}

func TestReadingCompleteMembershipMoveMissingReappearAndRootClear(t *testing.T) {
	f := newCompleteFixture(t, nil, "transitions")
	a := f.add("Author/Series/Series 01.epub", "epub", 0)
	b := f.add("Author/Series/Series 02.epub", "epub", 0)
	f.settle()
	f.seal()
	completeDrain(t, f.c)
	check := func(p completePublication, titles int, every bool, relative string) {
		t.Helper()
		want, err := compactcatalog.ReadingPathValue(compactcatalog.ReadingRuleInput{Item: p.item, SourceRoot: f.source, RelativePath: relative, Kind: p.kind}, titles, every)
		if err != nil {
			t.Fatal(err)
		}
		assertCompleteValue(t, f.c, p, want)
	}
	f.c.Exec(`UPDATE inventory_objects SET state='missing' WHERE id=?`, b.object)
	f.c.Exec(`UPDATE catalog_assets SET available=0 WHERE id=?`, b.asset)
	f.c.Exec(`UPDATE catalog_asset_links SET available=0 WHERE entity_id=?`, b.item)
	f.settle()
	completeDrain(t, f.c)
	check(a, 1, true, a.relative)
	check(b, 2, true, b.relative)
	// A failure on an entity whose object is missing cannot hold its live sibling.
	f.c.Exec(`INSERT INTO catalog_derivation_failures VALUES(20,?,'permanent',1,1)`, b.item)
	f.c.Exec(`UPDATE inventory_objects SET revision='new-a' WHERE id=?`, a.object)
	f.settle()
	f.c.Exec(`INSERT INTO catalog_derivation_failures VALUES(20,?,'permanent',1,1)`, b.item)
	completeDrain(t, f.c)
	check(a, 1, true, a.relative)
	f.c.Exec(`DELETE FROM catalog_derivation_failures WHERE domain=20 AND entity_id=?`, b.item)
	f.c.Exec(`UPDATE inventory_objects SET state='available' WHERE id=?`, b.object)
	f.c.Exec(`UPDATE catalog_assets SET available=1 WHERE id=?`, b.asset)
	f.c.Exec(`UPDATE catalog_asset_links SET available=1 WHERE entity_id=?`, b.item)
	f.settle()
	completeDrain(t, f.c)
	check(a, 2, true, a.relative)
	// Point changes reuse only an already trustworthy seal. Both old and new
	// normalized scopes are dirtied; no physical-folder refresh is invoked.
	moved := "Other/Zeta/Series 02.epub"
	f.c.Exec(`UPDATE catalog_assets SET path=? WHERE id=?`, filepath.Join(f.root, filepath.FromSlash(moved)), b.asset)
	f.c.Exec(`UPDATE inventory_objects SET relative_path=?,directory_path=? WHERE id=?`, filepath.FromSlash(moved), filepath.Dir(filepath.FromSlash(moved)), b.object)
	f.settle()
	completeDrain(t, f.c)
	check(a, 1, true, a.relative)
	check(b, 1, true, moved)
	root := "Root.epub"
	f.c.Exec(`UPDATE catalog_assets SET path=? WHERE id=?`, filepath.Join(f.root, root), b.asset)
	f.c.Exec(`UPDATE inventory_objects SET relative_path=?,directory_path='.' WHERE id=?`, root, b.object)
	f.settle()
	completeDrain(t, f.c)
	check(b, 1, false, root)
}

func TestReadingCompleteCancelledAndDriftedBuildCannotActivate(t *testing.T) {
	for _, change := range []string{"cancel", "generation", "incarnation", "root", "association", "retired"} {
		t.Run(change, func(t *testing.T) {
			f := newCompleteFixture(t, nil, "fences")
			p := f.add("Author/Series/Series 01.epub", "epub", 0)
			f.add("Author/Series/Series 02.epub", "epub", 0)
			f.settle()
			f.seal()
			completeUntil(t, f.c, func() bool {
				return scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='ready'`) == 1
			})
			seq := scalar(t, f.c.DB, `SELECT sequence FROM reading_path_clock`)
			switch change {
			case "cancel":
				f.c.Exec(`UPDATE jobs SET status='cancelled' WHERE id=?`, f.job)
			case "generation":
				f.c.Exec(`UPDATE library_sources SET generation=generation+1 WHERE id=?`, f.source)
			case "incarnation":
				f.c.Exec(`UPDATE library_sources SET incarnation='replacement' WHERE id=?`, f.source)
			case "root":
				f.c.Exec(`UPDATE library_sources SET root=? WHERE id=?`, filepath.Join(f.root, "new"), f.source)
			case "association":
				f.c.Exec(`DELETE FROM reading_publications WHERE item_id=?`, p.item)
			case "retired":
				f.c.Exec(`UPDATE catalog_entities SET retired=1 WHERE id=?`, p.item)
			}
			// Header fence validation runs before any newly queued normalization.
			for i := 0; i < 3; i++ {
				completeStep(t, f.c, 1)
				if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='abort'`) > 0 {
					break
				}
			}
			if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='abort'`) == 0 {
				t.Fatal("drift did not abort an unpublished build")
			}
			if scalar(t, f.c.DB, `SELECT sequence FROM reading_path_clock`) != seq || scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_slot WHERE overlay_id IS NOT NULL`) != 0 {
				t.Fatal("stale/cancelled stage activated")
			}
		})
	}
}

func TestReadingCompleteProducerAndSealSubcursorsRestart(t *testing.T) {
	// Deep logical paths force coverage reduction itself to persist a subcursor.
	f := newCompleteFixture(t, nil, "producer")
	p := f.add("A/B/C/D/E/F/Series/Series 01.epub", "epub", 0)
	f.add("A/B/C/D/E/F/Series/Series 02.epub", "epub", p.item)
	f.settle()
	f.seal()
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		completeStep(t, f.c, 1)
		rows, err := f.c.DB.Query(`SELECT kind,phase,after_object,after_asset,after_scope FROM reading_path_dirty_work WHERE captured>0`)
		if err != nil {
			t.Fatal(err)
		}
		var checkpoints []string
		for rows.Next() {
			var kind, object string
			var phase, asset, scope int64
			if err = rows.Scan(&kind, &phase, &object, &asset, &scope); err != nil {
				t.Fatal(err)
			}
			if object != "" || asset > 0 || scope > 0 || phase > 0 {
				checkpoints = append(checkpoints, fmt.Sprintf("%s/%d", kind, phase))
			}
		}
		rows.Close()
		if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE seal_cursor>0 AND sealed=0`) > 0 {
			checkpoints = append(checkpoints, "seal/partial")
		}
		reopened := false
		for _, key := range checkpoints {
			if !seen[key] {
				seen[key] = true
				reopened = true
			}
		}
		if reopened {
			var seq int
			var name, path string
			if err := f.c.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
				t.Fatal(err)
			}
			f.c.DB.Close()
			db, err := persistence.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			f.c.DB = db
			t.Cleanup(func() { db.Close() })
		}
		if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 && scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE queued=1`) == 0 && scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build`) == 0 {
			break
		}
	}
	for _, key := range []string{"item/0", "item/1", "item/2", "source/0", "source/1", "seal/partial"} {
		if !seen[key] {
			t.Fatalf("restart did not cover producer checkpoint %s: %v", key, seen)
		}
	}
	completeDrain(t, f.c)
}
