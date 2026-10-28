package compactcatalog_test

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/compactcatalog"
)

// O-101 parity: the batched capture window and the batched normalize sweeps must
// leave exactly the state that one-object steps leave. The same fixture is
// drained twice: with a raw budget of one (every window is a single point read)
// and with a larger budget (one set-based read per window). Capture, the
// representative choice, the durable cursor, the per-scope counters and the
// published heads must all be identical.

func readingBatchRows(t *testing.T, db *sql.DB, q string) []string {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		values := make([]any, len(cols))
		pointers := make([]any, len(cols))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		parts := make([]string, len(values))
		for i, value := range values {
			switch v := value.(type) {
			case nil:
				parts[i] = "null"
			case []byte:
				parts[i] = string(v)
			default:
				parts[i] = fmt.Sprint(v)
			}
		}
		out = append(out, strings.Join(parts, "|"))
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

type readingBatchState struct {
	capture, representative, build, keys, scopes, heads []string
	values                                              map[int64]compactcatalog.ReadingValues
}

func runReadingBatchFixture(t *testing.T, limit int) readingBatchState {
	t.Helper()
	f := newCompleteFixture(t, nil, "batch")
	// One item, two publications in one folder. The lower asset is captured
	// last (its object id sorts after the other), so the representative must be
	// replaced downwards exactly once.
	low := f.add("Author/Series/Series 01.epub", "epub", 0)
	high := f.add("Author/Series/Series 02.pdf", "pdf", low.item)
	f.c.Exec(`UPDATE inventory_objects SET id='zzz-low' WHERE id=?`, low.object)
	// An unsupported row and a publication-less row: hydrated but ineligible.
	// The publication-less object also takes the highest raw id, so the window's
	// last member is a rejected object and the durable cursor must still advance
	// past it. (An active canonical duplicate is not representable: the baseline
	// keeps inventory_objects(source_id,relative_path) unique while retired=0.)
	unsupported := f.add("Author/Series/Series 03.epub", "epub", 0)
	f.c.Exec(`UPDATE inventory_objects SET unsupported_reason='reject' WHERE id=?`, unsupported.object)
	bare := f.add("Author/Series/Series 04.epub", "epub", 0)
	f.c.Exec(`DELETE FROM reading_publications WHERE item_id=? AND asset_id=?`, bare.item, bare.asset)
	f.c.Exec(`UPDATE inventory_objects SET id='zzz-zzz' WHERE id=?`, bare.object)

	f.settle()
	f.seal()
	for i := 0; i < 5000; i++ {
		if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='stage'`) > 0 {
			break
		}
		completeStep(t, f.c, limit)
	}
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build WHERE phase='stage'`) != 1 {
		t.Fatal("fixture never reached the capture/stage boundary")
	}
	state := readingBatchState{
		capture:        readingBatchRows(t, f.c.DB, `SELECT item_id,asset_id,object_id,source_revision,relative_path,item_kind,series_evidence FROM reading_path_capture ORDER BY item_id,asset_id,object_id`),
		representative: readingBatchRows(t, f.c.DB, `SELECT item_id,asset_id,series_evidence FROM reading_path_representative ORDER BY item_id`),
		build:          readingBatchRows(t, f.c.DB, `SELECT after_object,title_count,non_series_count FROM reading_path_build ORDER BY id`),
	}
	for i := 0; i < 5000; i++ {
		if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) == 0 &&
			scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build`) == 0 &&
			scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE queued=1`) == 0 {
			break
		}
		completeStep(t, f.c, limit)
	}
	if scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_dirty_work`) != 0 ||
		scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_build`) != 0 ||
		scalar(t, f.c.DB, `SELECT count(*) FROM reading_path_scope WHERE queued=1`) != 0 {
		t.Fatalf("fixture did not drain at raw budget %d", limit)
	}
	state.keys = readingBatchRows(t, f.c.DB, `SELECT object_id,scope_id,item_id,context_active,asset_id,source_revision,relative_path,eligible FROM reading_path_object_keys ORDER BY object_id`)
	state.scopes = readingBatchRows(t, f.c.DB, `SELECT id,library_id,source_id,directory_key,desired,completed,queued,pending,sealed FROM reading_path_scope ORDER BY id`)
	state.heads = readingBatchRows(t, f.c.DB, `SELECT item_id,asset_id,object_id,source_revision,author,author_source,issue_date,series,series_source,series_number FROM reading_path_head ORDER BY item_id,asset_id`)
	state.values = map[int64]compactcatalog.ReadingValues{low.item: completeValues(t, f.c, low.item)}
	if high.item != low.item {
		state.values[high.item] = completeValues(t, f.c, high.item)
	}
	return state
}

func TestReadingCompleteBatchHydrationMatchesSingleObjectSteps(t *testing.T) {
	point := runReadingBatchFixture(t, 1)
	batch := runReadingBatchFixture(t, 7)
	if !reflect.DeepEqual(point, batch) {
		t.Fatalf("batched Reading steps diverged from one-object steps\npoint %+v\nbatch %+v", point, batch)
	}
	// Independent expectations on the batched run, so the oracle cannot pass by
	// both paths making the same mistake.
	if len(batch.capture) != 2 {
		t.Fatalf("capture has %d rows, want the item's two publications: %v", len(batch.capture), batch.capture)
	}
	if len(batch.representative) != 1 || !strings.HasSuffix(batch.representative[0], "|1") {
		t.Fatalf("representative is %v, want the item's lowest asset with series evidence", batch.representative)
	}
	if len(batch.build) != 1 || !strings.HasPrefix(batch.build[0], "zzz-zzz|1|0") {
		t.Fatalf("capture cursor/counters are %v, want the full raw window consumed (zzz-zzz|1|0)", batch.build)
	}
	if len(batch.keys) != 4 {
		t.Fatalf("object keys are %v, want the scope's four raw objects", batch.keys)
	}
	if len(batch.heads) != 2 {
		t.Fatalf("heads are %v, want one head per publication of the item", batch.heads)
	}
	if v := batch.values[point.valuesKey()]; v.Series == nil || v.Series.Value != "Series" {
		t.Fatalf("published series is %+v", v)
	}
}

// valuesKey is the item id of the single fixture value, kept out of the state
// comparison so both runs are compared by content only.
func (s readingBatchState) valuesKey() int64 {
	for item := range s.values {
		return item
	}
	return 0
}
