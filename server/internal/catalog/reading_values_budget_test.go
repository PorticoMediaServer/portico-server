package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// refreshReadingDirectoryTx runs inside the inventory writer gate, so its read
// shape is the gate hold for every folder page of a book or comic scan. Its two
// properties are pinned here: an unchanged folder costs a constant number of
// statements no matter how many publications it holds, and the streamed
// producer still publishes exactly the folder context, duplicate-publication and
// path-guard outcomes the whole-folder producer did.

// readingRefreshStatementBudget is the constant query ceiling: kind, fence,
// sibling context and the single published stream. It is deliberately loose
// enough for a couple of helpers but far below one statement per publication.
const readingRefreshStatementBudget = 10

type readingFile struct {
	key      string // entity identity; rows sharing a key are assets of one publication
	relative string // slash relative path, as the walk reports it
	format   string
	path     string // asset path override for the path guard; empty means root+relative
	revision string
	missing  bool   // observed but not available: never context, never published
	// duplicate, when set, adds a retired observation of the same asset at the
	// same relative path. Only the live object may own publication.
	duplicate string
}

type readingSeed struct {
	Item, Asset      int64
	Object, Revision string
	Relative         string
}

type readingFixture struct {
	*catalogtest.Catalog
	root    string
	library int64
	source  LibrarySource
	job     string
}

func newReadingFixture(t *testing.T) *readingFixture {
	t.Helper()
	c := catalogtest.Open(t)
	root := t.TempDir()
	f := &readingFixture{Catalog: c, root: root, library: c.Library("reading", "Reading", "book", root)}
	sources, err := New(c.DB).LibrarySources(context.Background(), "reading")
	if err != nil || len(sources) != 1 {
		t.Fatalf("reading source: %+v %v", sources, err)
	}
	f.source = sources[0]
	f.job = "reading-budget-job"
	c.Exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,'reading','running','2026-10-27T00:00:00Z')`, f.job)
	c.Exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision) VALUES(?,?,?,?,?,1)`,
		f.job, f.source.ID, f.source.Generation, f.source.Incarnation, f.source.RootIdentity)
	return f
}

// seed writes publications, assets, links and inventory observations the way a
// scan would, in one fixture transaction. Only inventory page replay normally
// writes inventory_objects, so those rows are inserted directly here.
func (f *readingFixture) seed(t *testing.T, files []readingFile) []readingSeed {
	t.Helper()
	seeds := make([]readingSeed, len(files))
	tokens := make([]string, len(files))
	f.Write(func(ctx context.Context, tx *sql.Tx) error {
		for i, spec := range files {
			item, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{
				Library: f.library, Kind: compactcatalog.Ebook, Key: spec.key, Title: filepath.Base(spec.relative),
			})
			if err != nil {
				return err
			}
			path := spec.path
			if path == "" {
				path = filepath.Join(f.root, filepath.FromSlash(spec.relative))
			}
			asset, token, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{
				Path: path, Size: 1000, ModifiedNS: 1, Container: strings.TrimPrefix(filepath.Ext(spec.relative), "."),
			})
			if err != nil {
				return err
			}
			if err = compactcatalog.LinkAssetTx(ctx, tx, item, asset, compactcatalog.Link{}); err != nil {
				return err
			}
			revision := spec.revision
			if revision == "" {
				revision = fmt.Sprintf("revision-%05d", i)
			}
			seeds[i] = readingSeed{Item: item, Asset: asset, Object: fmt.Sprintf("object-%05d", i), Revision: revision, Relative: spec.relative}
			tokens[i] = token
		}
		return insertReadingInventory(ctx, tx, f.source, seeds, tokens, files)
	})
	return seeds
}

func insertReadingInventory(ctx context.Context, tx *sql.Tx, source LibrarySource, seeds []readingSeed, tokens []string, files []readingFile) error {
	const chunk = 128
	for start := 0; start < len(seeds); start += chunk {
		end := start + chunk
		if end > len(seeds) {
			end = len(seeds)
		}
		var objects, publications strings.Builder
		objectArgs := make([]any, 0, (end-start)*9)
		publicationArgs := make([]any, 0, (end-start)*3)
		objects.WriteString(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,directory_path,state) VALUES`)
		publications.WriteString(`INSERT INTO reading_publications(item_id,asset_id,format) VALUES`)
		for i := start; i < end; i++ {
			if i > start {
				objects.WriteString(",")
				publications.WriteString(",")
			}
			relative := filepath.FromSlash(files[i].relative)
			state := "available"
			if files[i].missing {
				state = "missing"
			}
			objects.WriteString("(?,?,?,?,?,?,'{}',1,1,?,?)")
			objectArgs = append(objectArgs, seeds[i].Object, source.ID, tokens[i], source.Incarnation, relative, seeds[i].Revision, filepath.Dir(relative), state)
			publications.WriteString("(?,?,?)")
			publicationArgs = append(publicationArgs, seeds[i].Item, seeds[i].Asset, files[i].format)
		}
		if _, err := tx.ExecContext(ctx, objects.String(), objectArgs...); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, publications.String(), publicationArgs...); err != nil {
			return err
		}
	}
	for i, spec := range files {
		if spec.duplicate == "" {
			continue
		}
		relative := filepath.FromSlash(files[i].relative)
		if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,directory_path,retired)
 VALUES(?,?,?,?,?,?,'{}',1,1,?,1)`, fmt.Sprintf("zz-duplicate-%05d", i), source.ID, tokens[i], source.Incarnation, relative, spec.duplicate, filepath.Dir(relative)); err != nil {
			return err
		}
	}
	return nil
}

// refresh runs the real producer transaction under an instrumented context.
func (f *readingFixture) refresh(t *testing.T, directory string) (dbwork.Cost, []string, time.Duration) {
	t.Helper()
	ctx, plans := dbwork.TraceQueryPlans(context.Background())
	ctx, cost := dbwork.Measure(ctx)
	start := time.Now()
	err := dbwork.WithWriteTx(ctx, f.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		return refreshReadingDirectoryTx(ctx, tx, f.job, f.source, directory)
	})
	if err != nil {
		t.Fatalf("refresh %q: %v", directory, err)
	}
	return cost(), plans(), time.Since(start)
}

func readingRefreshBudget(t *testing.T, label string, cost dbwork.Cost, plans []string) {
	t.Helper()
	if cost.Statements > readingRefreshStatementBudget {
		t.Fatalf("%s: unchanged refresh issued %d statements, want <= %d; one statement per publication is the regression: %v",
			label, cost.Statements, readingRefreshStatementBudget, plans)
	}
	for _, q := range plans {
		// The old producer dispatched this exact per-publication read inside the loop.
		if strings.Contains(q, "reading_source_values WHERE item_id=? AND asset_id=? AND source IN") {
			t.Fatalf("%s: per-publication source read remains: %s", label, q)
		}
	}
}

func (f *readingFixture) libraryRevision(t *testing.T) int64 {
	t.Helper()
	var revision int64
	if err := f.DB.QueryRow(`SELECT COALESCE((SELECT revision FROM library_revisions WHERE library_id='reading'),0)`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

type storedPathValue struct {
	source, text, object, revision string
	number                         sql.NullFloat64
}

// storedPathValues reads the accepted path facts exactly as publication left
// them: one row per field and source for each (item, asset).
func storedPathValues(t *testing.T, c *catalogtest.Catalog) map[[2]int64]map[string]storedPathValue {
	t.Helper()
	rows, err := c.DB.Query(`SELECT item_id,asset_id,field,source,COALESCE(text_value,''),number_value,inventory_object_id,source_revision FROM reading_source_values ORDER BY item_id,asset_id,field,source`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[[2]int64]map[string]storedPathValue{}
	for rows.Next() {
		var item, asset int64
		var field string
		var v storedPathValue
		if err = rows.Scan(&item, &asset, &field, &v.source, &v.text, &v.number, &v.object, &v.revision); err != nil {
			t.Fatal(err)
		}
		key := [2]int64{item, asset}
		if out[key] == nil {
			out[key] = map[string]storedPathValue{}
		}
		out[key][field] = v
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// referenceValues runs the pure whole-folder producer over the same admitted
// rows, so the streamed transaction is compared with the semantics it must keep.
func (f *readingFixture) referenceValues(t *testing.T, seeds []readingSeed, files []readingFile) map[int64]compactcatalog.ReadingValues {
	t.Helper()
	var inputs []compactcatalog.ReadingRuleInput
	for i, spec := range files {
		if spec.missing {
			continue
		}
		inputs = append(inputs, compactcatalog.ReadingRuleInput{
			Item: seeds[i].Item, SourceRoot: f.source.ID, RelativePath: filepath.ToSlash(spec.relative), Kind: compactcatalog.Ebook,
		})
	}
	values, err := compactcatalog.ReadingPathValues(inputs)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func expectedPathFields(v compactcatalog.ReadingValues) map[string]string {
	out := map[string]string{}
	add := func(field string, value *compactcatalog.ReadingValue) {
		if value != nil {
			out[field] = string(value.Source) + "|" + value.Value
		}
	}
	add("author", v.Author)
	add("issueDate", v.IssueDate)
	add("series", v.Series)
	if v.SeriesNumber != nil {
		out["seriesNumber"] = string(v.SeriesNumber.Source) + "|" + strconv.FormatFloat(v.SeriesNumber.Value, 'g', -1, 64)
	}
	return out
}

func storedPathField(v storedPathValue) string {
	if v.number.Valid {
		return v.source + "|" + strconv.FormatFloat(v.number.Float64, 'g', -1, 64)
	}
	return v.source + "|" + v.text
}

// assertPublished compares the stored facts with the pure producer's values for
// the rows the transaction admitted: available, in scope, path-guard passing.
func assertPublished(t *testing.T, c *catalogtest.Catalog, expected map[int64]compactcatalog.ReadingValues, admitted map[int64][]int64) {
	t.Helper()
	stored := storedPathValues(t, c)
	want := 0
	for _, assets := range admitted {
		want += len(assets)
	}
	if len(stored) != want {
		t.Fatalf("published %d (item, asset) pairs, want %d: %v", len(stored), want, stored)
	}
	for item, assets := range admitted {
		wantFields := expectedPathFields(expected[item])
		for _, asset := range assets {
			fields := stored[[2]int64{item, asset}]
			if fields == nil {
				t.Fatalf("item %d asset %d not published", item, asset)
			}
			got := map[string]string{}
			for field, value := range fields {
				got[field] = storedPathField(value)
			}
			if !reflect.DeepEqual(got, wantFields) {
				t.Fatalf("item %d asset %d published %v, whole-folder producer gives %v", item, asset, got, wantFields)
			}
		}
	}
	allowed := map[[2]int64]bool{}
	for item, assets := range admitted {
		for _, asset := range assets {
			allowed[[2]int64{item, asset}] = true
		}
	}
	for key := range stored {
		if !allowed[key] {
			t.Fatalf("published unexpected pair %v", key)
		}
	}
}

func TestReadingDirectoryRefreshUnchangedFolderIsConstantWork(t *testing.T) {
	f := newReadingFixture(t)
	const count = 4096
	files := make([]readingFile, count)
	for i := range files {
		files[i] = readingFile{
			key:      fmt.Sprintf("shelf:%05d", i),
			relative: fmt.Sprintf("Shelf/Shelf %05d.epub", i+1),
			format:   "epub",
		}
	}
	seeds := f.seed(t, files)

	first, firstPlans, firstElapsed := f.refresh(t, "Shelf")
	revision := f.libraryRevision(t)
	published := storedPathValues(t, f.Catalog)
	if len(published) != count {
		t.Fatalf("first refresh published %d pairs, want %d", len(published), count)
	}
	before := f.libraryRevision(t)

	second, secondPlans, secondElapsed := f.refresh(t, "Shelf")
	t.Logf("%d-publication folder: first refresh %d statements in %s (%d plans traced), unchanged refresh %d statements in %s, vmSteps=%d, library revision %d -> %d",
		count, first.Statements, firstElapsed, len(firstPlans), second.Statements, secondElapsed, second.VMSteps, revision, before)
	readingRefreshBudget(t, "unchanged 4096-publication folder", second, secondPlans)
	if after := f.libraryRevision(t); after != before {
		t.Fatalf("unchanged refresh churned library revision %d -> %d", before, after)
	}
	if now := storedPathValues(t, f.Catalog); !reflect.DeepEqual(now, published) {
		t.Fatal("unchanged refresh rewrote accepted path values")
	}
	admitted := map[int64][]int64{}
	for _, seed := range seeds {
		admitted[seed.Item] = append(admitted[seed.Item], seed.Asset)
	}
	assertPublished(t, f.Catalog, f.referenceValues(t, seeds, files), admitted)
}

func TestReadingRootFolderRefreshUsesFileNameContextOnly(t *testing.T) {
	f := newReadingFixture(t)
	const count = 1024
	files := make([]readingFile, count)
	for i := range files {
		// Root/generic folders carry no sibling-derived context: a single title in
		// the root must be published from the file name, and the root refresh must
		// not pay for a sibling-context pass at all.
		files[i] = readingFile{
			key:      fmt.Sprintf("root:%05d", i),
			relative: fmt.Sprintf("%05d Author - Book 2021-07.epub", i+1),
			format:   "epub",
		}
	}
	seeds := f.seed(t, files)
	first, _, firstElapsed := f.refresh(t, ".")
	revision := f.libraryRevision(t)
	published := storedPathValues(t, f.Catalog)
	if len(published) != count {
		t.Fatalf("root refresh published %d pairs, want %d", len(published), count)
	}

	second, secondPlans, secondElapsed := f.refresh(t, ".")
	t.Logf("%d root publications: first refresh %d statements in %s, unchanged refresh %d statements in %s, vmSteps=%d, library revision %d",
		count, first.Statements, firstElapsed, second.Statements, secondElapsed, second.VMSteps, revision)
	readingRefreshBudget(t, "unchanged root folder", second, secondPlans)
	if after := f.libraryRevision(t); after != revision {
		t.Fatalf("unchanged root refresh churned library revision %d -> %d", revision, after)
	}
	if now := storedPathValues(t, f.Catalog); !reflect.DeepEqual(now, published) {
		t.Fatal("unchanged root refresh rewrote accepted path values")
	}
	admitted := map[int64][]int64{}
	for _, seed := range seeds {
		admitted[seed.Item] = append(admitted[seed.Item], seed.Asset)
	}
	expected := f.referenceValues(t, seeds, files)
	for item := range admitted {
		if expected[item].Series != nil {
			t.Fatalf("root item %d inferred series %q from root siblings", item, expected[item].Series.Value)
		}
	}
	assertPublished(t, f.Catalog, expected, admitted)
	for _, fields := range storedPathValues(t, f.Catalog) {
		for field, value := range fields {
			if value.source != "file-name" && value.source != "folder" {
				t.Fatalf("%s published from %q", field, value.source)
			}
			if value.source == "folder" {
				t.Fatalf("root publication %q took folder context", field)
			}
		}
	}
}

func TestReadingFolderContextMatchesWholeFolderProducer(t *testing.T) {
	f := newReadingFixture(t)
	nested := []readingFile{
		{key: "series:01", relative: "Author One/Series One/Series One 01.epub", format: "epub", duplicate: "duplicate-revision"},
		{key: "series:02", relative: "Author One/Series One/Series One 02.epub", format: "epub"},
		{key: "series:03", relative: "Author One/Series One/Series One 03.epub", format: "epub"},
		// One publication, two assets: each asset owns a row set, counted once.
		{key: "series:shared", relative: "Author One/Series One/Shared Volume.epub", format: "epub"},
		{key: "series:shared", relative: "Author One/Series One/Shared Volume.pdf", format: "pdf"},
	}
	nestedSeeds := f.seed(t, nested)

	flat := []readingFile{
		{key: "flat:01", relative: "Series One/Series One 01.epub", format: "epub"},
		{key: "flat:02", relative: "Series One/Series One 02.epub", format: "epub"},
		{key: "flat:03", relative: "Series One/Unrelated Tale.epub", format: "epub"},
		{key: "flat:04", relative: "Series One/Other Author - Other Book.epub", format: "epub"},
		// Not available: never context, never published.
		{key: "flat:05", relative: "Series One/Gone.epub", format: "epub", missing: true},
		// Asset path outside the source root: the path guard keeps it out.
		{key: "flat:06", relative: "Series One/Guarded.epub", format: "epub", path: filepath.Join(f.root, "Elsewhere", "Guarded.epub")},
	}
	flatSeeds := f.seed(t, flat)

	_, nestedPlans, _ := f.refresh(t, "Author One/Series One")
	for _, q := range nestedPlans {
		if strings.Contains(q, "reading_source_values WHERE item_id=? AND asset_id=? AND source IN") {
			t.Fatalf("nested refresh kept the per-publication read: %s", q)
		}
	}
	nestedAdmitted := map[int64][]int64{
		nestedSeeds[0].Item: {nestedSeeds[0].Asset},
		nestedSeeds[1].Item: {nestedSeeds[1].Asset},
		nestedSeeds[2].Item: {nestedSeeds[2].Asset},
		nestedSeeds[3].Item: {nestedSeeds[3].Asset, nestedSeeds[4].Asset},
	}
	assertPublished(t, f.Catalog, f.referenceValues(t, nestedSeeds, nested), nestedAdmitted)
	stored := storedPathValues(t, f.Catalog)
	for field, value := range stored[[2]int64{nestedSeeds[0].Item, nestedSeeds[0].Asset}] {
		if value.object != nestedSeeds[0].Object || value.revision != nestedSeeds[0].Revision {
			t.Fatalf("%s published from duplicate object %q/%q, want %q/%q", field, value.object, value.revision, nestedSeeds[0].Object, nestedSeeds[0].Revision)
		}
	}
	if got := stored[[2]int64{nestedSeeds[0].Item, nestedSeeds[0].Asset}]["series"]; got.source != "folder" || got.text != "Series One" {
		t.Fatalf("nested series context = %q/%q, want folder/Series One", got.source, got.text)
	}
	for _, seed := range nestedSeeds[:3] {
		author := stored[[2]int64{seed.Item, seed.Asset}]["author"]
		if author.source != "folder" || author.text != "Author One" {
			t.Fatalf("nested author context = %q/%q, want folder/Author One", author.source, author.text)
		}
	}

	_, flatPlans, _ := f.refresh(t, "Series One")
	for _, q := range flatPlans {
		if strings.Contains(q, "reading_source_values WHERE item_id=? AND asset_id=? AND source IN") {
			t.Fatalf("flat refresh kept the per-publication read: %s", q)
		}
	}
	flatAdmitted := map[int64][]int64{}
	for i := 0; i < 4; i++ {
		flatAdmitted[flatSeeds[i].Item] = []int64{flatSeeds[i].Asset}
	}
	assertPublished(t, f.Catalog, f.referenceValues(t, flatSeeds[:5], flat[:5]), flatAdmitted)
	stored = storedPathValues(t, f.Catalog)
	// Mixed siblings at a non-root folder: every title carries the folder author,
	// while a file-name author still wins. The stream can stop counting once the
	// sibling context is final without changing either outcome.
	for i := 0; i < 3; i++ {
		author := stored[[2]int64{flatSeeds[i].Item, flatSeeds[i].Asset}]["author"]
		if author.source != "folder" || author.text != "Series One" {
			t.Fatalf("flat mixed-folder author = %q/%q, want folder/Series One", author.source, author.text)
		}
	}
	fileName := stored[[2]int64{flatSeeds[3].Item, flatSeeds[3].Asset}]["author"]
	if fileName.source != "file-name" || fileName.text != "Other Author" {
		t.Fatalf("file-name author = %q/%q, want file-name/Other Author", fileName.source, fileName.text)
	}
	for _, seed := range flatSeeds[4:] {
		if fields := stored[[2]int64{seed.Item, seed.Asset}]; len(fields) != 0 {
			t.Fatalf("unadmitted row %d published %v", seed.Item, fields)
		}
	}
}
