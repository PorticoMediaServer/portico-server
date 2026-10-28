package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/readprep"
)

func memoryPreviewReservation(t *testing.T) func(int64) error {
	t.Helper()
	var previous int64
	return func(bytes int64) error {
		t.Helper()
		if bytes < previous || bytes > bulkPreviewMemoryBytes {
			t.Fatalf("invalid memory reservation %d after %d", bytes, previous)
		}
		previous = bytes
		return nil
	}
}

func TestBulkPreviewMemoryLegacyOracle200AndNoScratch(t *testing.T) {
	s, db := editorFixture(t)
	c := catalogtest.New(t, db)
	request := catalog.BulkMetadataPreviewRequest{}
	for i := 0; i < 200; i++ {
		item := c.Movie(c.Handle("movies"), fmt.Sprintf("/movies/memory-%d.mkv", i), fmt.Sprintf("Memory %d", i), 1900+i%70)
		c.Fields(item.ID, map[string]any{"tags": fmt.Sprintf(`["Shared","Tag %02d","Shared"]`, i%70)})
		request.Targets = append(request.Targets, catalog.BulkMetadataTarget{Kind: "item", ContentKind: "movie", ID: item.Public, LibraryID: "movies"})
	}
	c.Drain()
	// A real owner edit exercises locked fields and the snapshot/CAS digest.
	value := `["Shared","Tag 00"]`
	editField(t, s, RepairTarget{Kind: "item", ID: request.Targets[0].ID}, "tags", RepairFieldEdit{Value: &value})
	base := previewRequestSource(s, request)
	want, err := s.BulkPreview(context.Background(), base, previewAuthorize)
	if err != nil {
		t.Fatal(err)
	}
	assertPreviewScratchClean(t, s)
	// The new path must succeed with a nonexistent cache parent and must not
	// execute the selection SQL/grouped vector: only verified explicit IDs.
	s.cacheRoot = filepath.Join(t.TempDir(), "never-created", "preview")
	source := func(ctx context.Context, tx *sql.Tx, after int64) (catalog.BulkMetadataSelectionPage, error) {
		page, err := base(ctx, tx, after)
		if len(page.Selection.Targets) > 0 {
			page.Selection.SQL = "invalid SQL: the explicit memory reader must never execute this"
		}
		return page, err
	}
	got, err := s.BulkPreviewMemory(context.Background(), source, previewAuthorize, memoryPreviewReservation(t))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("memory differs from legacy: got=%+v want=%+v", got, want)
	}
	if got.Total != 200 || len(got.Targets) != 200 || got.Fields["year"].DistinctCount != 70 || len(got.Fields["year"].Values) != 64 || !got.Fields["year"].Truncated || got.Fields["tags"].Locked != 1 {
		t.Fatalf("inexact bounded preview: total=%d year=%+v tags=%+v", got.Total, got.Fields["year"], got.Fields["tags"])
	}
	if _, err := os.Stat(s.cacheRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("memory preview touched cache directory: %v", err)
	}
	// A 201st accepted target cannot enter even if the source page cap is met.
	request.Targets = append(request.Targets, catalog.BulkMetadataTarget{Kind: "item", ContentKind: "movie", ID: "unresolved-201st", LibraryID: "movies"})
	// Use the verified 200-ID pages followed by a synthetic final page, so the
	// reducer guard (rather than the catalog's invalid-ID guard) is exercised.
	overflow := func(ctx context.Context, tx *sql.Tx, after int64) (catalog.BulkMetadataSelectionPage, error) {
		page, err := base(ctx, tx, after)
		if after == 192 {
			page.Selection.Targets = append(page.Selection.Targets, request.Targets[200])
		}
		return page, err
	}
	if p, err := s.BulkPreviewMemory(context.Background(), overflow, previewAuthorize, memoryPreviewReservation(t)); !errors.Is(err, ErrRepairInput) || p.Total != 0 {
		t.Fatalf("201 targets accepted: total=%d err=%v", p.Total, err)
	}
}

func TestBulkPreviewMemoryCountSetsBinaryOrderingAndDelivery(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	spool, err := newBulkPreviewSpool(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if spool.db != nil {
			if err := spool.close(); err != nil {
				t.Error(err)
			}
		}
	})
	m := bulkPreviewMemory{reserve: memoryPreviewReservation(t)}
	if err := m.charge(64 << 10); err != nil {
		t.Fatal(err)
	}
	m.counts = make(map[string]*bulkPreviewMemoryField)
	observations := []bulkPreviewObservation{}
	for i := 0; i < 200; i++ {
		value := fmt.Sprintf("%02d", i%70)
		observations = append(observations, bulkPreviewObservation{field: "list", value: value, entries: []string{"Shared", value, "Shared", "é", "Z", "a", "\x00", "<&>\n"}, count: 1, locked: i%3 == 0})
	}
	entries := make([]string, 70)
	for i := range entries {
		entries[i] = fmt.Sprintf("entry %02d", i)
	}
	observations = append(observations, bulkPreviewObservation{field: "entries-only", value: encodeList(entries), entries: entries, count: 1})
	page := bulkPreviewPage{observations: observations}
	if err := m.add(ctx, &BulkMetadataPreview{}, page); err != nil {
		t.Fatal(err)
	}
	if err := spool.add(ctx, observations); err != nil {
		t.Fatal(err)
	}
	schema := []RepairFieldSpec{{Field: "list"}, {Field: "entries-only"}}
	want, err := spool.fields(ctx, schema)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.fields(ctx, schema)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("memory count sets differ: got=%+v want=%+v", got, want)
	}
	if got["entries-only"].DistinctCount != 1 || !got["entries-only"].Truncated || len(got["entries-only"].Entries) != 64 || got["entries-only"].Value == nil {
		t.Fatalf("entry truncation changed shared value semantics: %+v", got["entries-only"])
	}
	// A second oracle with one shared large escapable scalar must retain it
	// losslessly, including enough charged memory for JSON output and copies.
	text := strings.Repeat("<&>\n", 1000)
	if err := m.add(ctx, &BulkMetadataPreview{}, bulkPreviewPage{observations: []bulkPreviewObservation{{field: "scalar", value: text, count: 1}}}); err != nil {
		t.Fatal(err)
	}
	fields, err := m.fields(ctx, []RepairFieldSpec{{Field: "scalar"}})
	if err != nil || fields["scalar"].Value == nil || *fields["scalar"].Value != text {
		t.Fatalf("scalar truncated: %v", err)
	}
	raw, err := json.Marshal(fields)
	if err != nil || int64(2*len(raw)) > m.used {
		t.Fatalf("delivery not charged: json=%d reservation=%d err=%v", len(raw), m.used, err)
	}
	if err := spool.close(); err != nil {
		t.Fatal(err)
	}
	spool.db = nil
	scratchEntries, err := os.ReadDir(root)
	if err != nil || len(scratchEntries) != 0 {
		t.Fatalf("oracle scratch survived: %v %v", scratchEntries, err)
	}
}

func TestBulkPreviewMemoryCapacityBeforeRetention(t *testing.T) {
	denied := errors.New("reservation denied")
	m := bulkPreviewMemory{reserve: func(int64) error { return denied }}
	set := make(map[string]*int64)
	if err := m.increment(set, "value", 1); !errors.Is(err, denied) || len(set) != 0 {
		t.Fatalf("allocated before reservation: set=%v err=%v", set, err)
	}
	m.reserve = memoryPreviewReservation(t)
	if err := m.increment(set, strings.Repeat("x", int(bulkPreviewMemoryBytes/32)), 1); !errors.Is(err, readprep.ErrCapacity) || len(set) != 0 {
		t.Fatalf("oversized scalar entered reducer: set=%d err=%v", len(set), err)
	}
	if err := m.preflight(RepairSnapshot{Fields: map[string]RepairField{"title": {Value: strings.Repeat("x", int(bulkPreviewMemoryBytes/32))}}}, repairEntity{}); !errors.Is(err, readprep.ErrCapacity) {
		t.Fatalf("oversized source snapshot accepted: %v", err)
	}
}

func TestBulkPreviewMemoryRejectsQueryAndReservationFailureBeforeSource(t *testing.T) {
	s, targets, _ := bulkFixture(t)
	called := false
	denied := readprep.ErrCapacity
	source := func(context.Context, *sql.Tx, int64) (catalog.BulkMetadataSelectionPage, error) {
		called = true
		return catalog.BulkMetadataSelectionPage{}, nil
	}
	if p, err := s.BulkPreviewMemory(context.Background(), source, previewAuthorize, func(int64) error { return denied }); !errors.Is(err, denied) || p.Total != 0 || called {
		t.Fatalf("source ran before initial reservation: total=%d called=%v err=%v", p.Total, called, err)
	}
	query := previewSQLSource(s, catalog.BulkMetadataSelection{SQL: "SELECT id FROM catalog_entities"})
	if p, err := s.BulkPreviewMemory(context.Background(), query, previewAuthorize, memoryPreviewReservation(t)); !errors.Is(err, ErrRepairInput) || p.Total != 0 {
		t.Fatalf("query source accepted: total=%d err=%v", p.Total, err)
	}
	request := catalog.BulkMetadataPreviewRequest{Targets: []catalog.BulkMetadataTarget{{Kind: "item", ContentKind: "movie", ID: targets[0].ID, LibraryID: "movies"}}}
	base := previewRequestSource(s, request)
	for _, tc := range []string{"duplicate", "kind", "library", "oversized-fence"} {
		t.Run(tc, func(t *testing.T) {
			source := func(ctx context.Context, tx *sql.Tx, after int64) (catalog.BulkMetadataSelectionPage, error) {
				page, err := base(ctx, tx, after)
				if err != nil {
					return page, err
				}
				page.Selection.Targets = append([]catalog.BulkMetadataTarget(nil), page.Selection.Targets...)
				switch tc {
				case "duplicate":
					page.Selection.Targets = append(page.Selection.Targets, page.Selection.Targets[0])
				case "kind":
					page.Selection.Targets[0].Kind = "show"
				case "library":
					page.Selection.Targets[0].LibraryID = "wrong"
				case "oversized-fence":
					page.Fence = strings.Repeat("x", int(bulkPreviewMemoryBytes/32)+1)
				}
				return page, err
			}
			_, err := s.BulkPreviewMemory(context.Background(), source, previewAuthorize, memoryPreviewReservation(t))
			if tc == "oversized-fence" && !errors.Is(err, readprep.ErrCapacity) || tc != "oversized-fence" && !errors.Is(err, ErrRepairInput) {
				t.Fatal(err)
			}
		})
	}
}

func TestBulkPreviewMemoryFinalAndInterPageFences(t *testing.T) {
	for _, stage := range []string{"page", "final", "authority", "cancel", "revoke"} {
		t.Run(stage, func(t *testing.T) {
			s, targets, _ := bulkFixture(t)
			request := catalog.BulkMetadataPreviewRequest{Targets: []catalog.BulkMetadataTarget{{Kind: "item", ContentKind: "movie", ID: targets[0].ID, LibraryID: "movies"}}}
			base := previewRequestSource(s, request)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			source := func(ctx context.Context, tx *sql.Tx, after int64) (catalog.BulkMetadataSelectionPage, error) {
				calls++
				if calls == 3 {
					switch stage {
					case "page", "final":
						if _, err := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassInteractive, `UPDATE catalog_entities SET year=year+1 WHERE title='Before'`); err != nil {
							t.Fatal(err)
						}
					case "authority":
						dbwork.BumpAuthority()
					case "cancel":
						cancel()
					}
				}
				page, err := base(ctx, tx, after)
				if stage == "page" && calls <= 2 {
					page.Done = false // next snapshot reaches EOF after one target
				}
				return page, err
			}
			want := error(catalog.ErrStaleContinuation)
			if stage == "cancel" {
				want = context.Canceled
			}
			denied := errors.New("revoked")
			if stage == "revoke" {
				want = denied
			}
			p, err := s.BulkPreviewMemory(ctx, source, func(context.Context, *sql.Tx) error {
				if stage == "revoke" && calls >= 3 {
					return denied
				}
				return nil
			}, memoryPreviewReservation(t))
			if !errors.Is(err, want) || p.Total != 0 {
				t.Fatalf("late change leaked aggregate: total=%d err=%v", p.Total, err)
			}
		})
	}
}

func TestBulkPreviewMemoryOversizedEditorVectorAndSharedCapacity(t *testing.T) {
	s, targets, _ := bulkFixture(t)
	request := catalog.BulkMetadataPreviewRequest{Targets: []catalog.BulkMetadataTarget{{Kind: "item", ContentKind: "movie", ID: targets[0].ID, LibraryID: "movies"}}}
	source := previewRequestSource(s, request)
	calls := 0
	if p, err := s.BulkPreviewMemory(context.Background(), source, previewAuthorize, func(bytes int64) error {
		calls++
		if calls >= 8 {
			return readprep.ErrCapacity
		}
		return nil
	}); !errors.Is(err, readprep.ErrCapacity) || p.Total != 0 || calls != 8 {
		t.Fatalf("shared budget exhaustion leaked aggregate: total=%d calls=%d err=%v", p.Total, calls, err)
	}
	// Provider/scanner text is not constrained by the editor's input limits.
	// A legal large source must fail capacity, never truncate or omit the value.
	if _, err := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassInteractive, `UPDATE catalog_item_details SET studio=? WHERE entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, strings.Repeat("x", int(bulkPreviewMemoryBytes/32)), targets[0].ID); err != nil {
		t.Fatal(err)
	}
	if p, err := s.BulkPreviewMemory(context.Background(), source, previewAuthorize, memoryPreviewReservation(t)); !errors.Is(err, readprep.ErrCapacity) || p.Total != 0 {
		t.Fatalf("oversized editor vector leaked aggregate: total=%d err=%v", p.Total, err)
	}
	if _, err := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassInteractive, `UPDATE catalog_item_details SET studio='' WHERE entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, targets[0].ID); err != nil {
		t.Fatal(err)
	}
	// The CAS fence has source strings not present in the visible snapshot.
	if _, err := dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassInteractive, `INSERT INTO provider_evidence(item_id,provider,provider_id,payload,observed_at) SELECT id,'memory-test',1,'{}',? FROM catalog_entities WHERE public_id=pid_blob(?)`, strings.Repeat("x", int(bulkPreviewMemoryBytes/32)), targets[0].ID); err != nil {
		t.Fatal(err)
	}
	if p, err := s.BulkPreviewMemory(context.Background(), source, previewAuthorize, memoryPreviewReservation(t)); !errors.Is(err, readprep.ErrCapacity) || p.Total != 0 {
		t.Fatalf("oversized CAS vector leaked aggregate: total=%d err=%v", p.Total, err)
	}
}

func TestBulkPreviewMemoryReadOnlySingletonAndSnapshotLifetime(t *testing.T) {
	s, targets, _ := bulkFixture(t)
	_, release, err := dbwork.AcquireLongRead(context.Background(), dbwork.LongReadIntegrity)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, statements := dbwork.TraceQueryPlans(context.Background())
	request := catalog.BulkMetadataPreviewRequest{Targets: []catalog.BulkMetadataTarget{{Kind: "item", ContentKind: "movie", ID: targets[0].ID, LibraryID: "movies"}}}
	base := previewRequestSource(s, request)
	source := func(ctx context.Context, tx *sql.Tx, after int64) (catalog.BulkMetadataSelectionPage, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("source escaped ordinary bounded snapshot")
		}
		return base(ctx, tx, after)
	}
	if p, err := s.BulkPreviewMemory(ctx, source, previewAuthorize, memoryPreviewReservation(t)); err != nil || p.Total != 1 {
		t.Fatalf("preview consumed owner-long permit: total=%d err=%v", p.Total, err)
	}
	if len(statements()) == 0 {
		t.Fatal("missing statement trace")
	}
	for _, statement := range statements() {
		q := strings.ToUpper(strings.TrimSpace(statement))
		if !strings.HasPrefix(q, "SELECT") && !strings.HasPrefix(q, "WITH") {
			t.Fatalf("non-read statement: %s", statement)
		}
		if strings.Contains(q, "GROUP BY VECTOR") || strings.Contains(q, "CREATE TEMP") {
			t.Fatalf("grouped/temp work in explicit reader: %s", statement)
		}
	}
}
