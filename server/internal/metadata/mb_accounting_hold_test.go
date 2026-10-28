package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

func mbAccountingLargeRelease(side int) metadataprovider.ReleaseLookup {
	id := fmt.Sprintf("%08x-0000-0000-0000-000000000000", side)
	r := metadataprovider.ReleaseLookup{RequestedID: id, Release: metadataprovider.Release{
		ID: id, ReleaseGroup: metadataprovider.ReleaseGroup{ID: mbLaneGroup},
		Media: []metadataprovider.Medium{{Position: 1}},
	}}
	for n := 0; n < mbEvidenceTracks; n++ {
		r.Release.Media[0].Tracks = append(r.Release.Media[0].Tracks, metadataprovider.ReleaseTrack{
			ID: fmt.Sprintf("%08x-0000-0000-0000-%012x", side+100, n), Position: n + 1,
			Recording: metadataprovider.Recording{ID: fmt.Sprintf("%08x-0000-0000-0000-%012x", side+200, n)},
		})
	}
	return r
}

// A small historical comparison: same 10k distinct normalized input and
// unchanged save routine, with only candidate accounting DDL removed from this
// disposable test DB and the verbatim original budget query restored.
func TestMBAccountingOriginalStagingWriterHold(t *testing.T) {
	c := catalogtest.Open(t)
	rows, err := c.DB.Query(`SELECT name FROM sqlite_schema WHERE type='trigger' AND name LIKE 'mb_account_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var triggers []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		triggers = append(triggers, name)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 30 {
		t.Fatal("unexpected candidate trigger count", len(triggers))
	}
	for _, name := range triggers {
		if _, err = c.DB.Exec(`DROP TRIGGER "` + name + `"`); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"mb_release_accounting", "mb_recording_accounting", "mb_evidence_totals"} {
		if _, err = c.DB.Exec(`DROP TABLE ` + table); err != nil {
			t.Fatal(err)
		}
	}
	stage, err := stageMBRelease(mbAccountingLargeRelease(10))
	if err != nil {
		t.Fatal(err)
	}
	dbwork.ResetPlans()
	dbwork.WriteGate().ResetPeak()
	before := dbwork.WriteGate().Stats().HeldMilli
	w, err := dbwork.Begin(context.Background(), c.DB, dbwork.ClassBackgroundMedia)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Rollback()
	start := time.Now()
	if _, err = saveMBReleaseEvidence(context.Background(), w.Tx(), stage, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	var releases, recordings, charge int64
	if err = w.Tx().QueryRow(mbAccountingOracleSQL).Scan(&releases, &recordings, &charge); err != nil {
		t.Fatal(err)
	}
	if releases > 512 || recordings > 20000 || charge > 256<<20 {
		t.Fatal("historical exact budget unexpectedly refused fixture")
	}
	if err = w.Commit(); err != nil {
		t.Fatal(err)
	}
	cost := dbwork.Plans().Largest
	t.Logf("unchanged original staging + original aggregate: writer elapsed=%s gate-held=%dms rows/total_changes=%d pages=%d VM=%d", time.Since(start), dbwork.WriteGate().Stats().HeldMilli-before, cost.Rows, cost.Pages, cost.VMOps)
}

// This is the gate's explicitly required focused acceptance test, not a load
// benchmark. Real source triggers and FKs remain enabled throughout. A failure
// above 250ms requires the staged protocol; increasing a plan cannot pass it.
func TestMBAccountingTenThousandDistinctWriterHold(t *testing.T) {
	db, names := mbLaneDB(t)
	ctx := context.Background()
	type measurement struct {
		label string
		held  time.Duration
	}
	var measurements []measurement
	measure := func(label string, plan *dbwork.WritePlan, fn func(*sql.Tx) error) {
		t.Helper()
		dbwork.ResetPlans()
		dbwork.WriteGate().ResetPeak()
		before := dbwork.WriteGate().Stats().HeldMilli
		writeCtx := ctx
		if plan != nil {
			writeCtx = dbwork.WithPlan(ctx, *plan)
		}
		w, err := dbwork.Begin(writeCtx, db, dbwork.ClassBackgroundMedia)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Rollback()
		start := time.Now()
		var changesBefore, changesAfter int64
		if err = w.Tx().QueryRow(`SELECT total_changes()`).Scan(&changesBefore); err != nil {
			t.Fatal(err)
		}
		if err = fn(w.Tx()); err != nil {
			t.Fatal(label, err)
		}
		if err = w.Tx().QueryRow(`SELECT total_changes()`).Scan(&changesAfter); err != nil {
			t.Fatal(err)
		}
		if err = w.Commit(); err != nil {
			t.Fatal(label, err)
		}
		elapsed := time.Since(start)
		held := dbwork.WriteGate().Stats().HeldMilli - before
		cost := dbwork.Plans().Largest
		t.Logf("%s: writer elapsed=%s gate-held=%dms total_changes=%d measured rows=%d pages=%d VM=%d overruns=%d", label, elapsed, held, changesAfter-changesBefore, cost.Rows, cost.Pages, cost.VMOps, dbwork.Plans().Overruns)
		measurements = append(measurements, measurement{label, elapsed})
	}
	values := []metadataprovider.ReleaseLookup{mbAccountingLargeRelease(10), mbAccountingLargeRelease(20)}
	stages := make([]mbReleaseEvidence, 2)
	for n, v := range values {
		var err error
		stages[n], err = stageMBRelease(v)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("side %d: tracks=%d distinct recording inputs=%d encoded release bytes=%d", n, len(v.Release.Media[0].Tracks), mbEvidenceTracks, len(stages[n].payload))
	}
	revisions := make([]string, 2)
	for n := range stages {
		measure(fmt.Sprintf("original indivisible staging + maintained triggers side %d", n), nil, func(tx *sql.Tx) (err error) {
			revisions[n], err = saveMBReleaseEvidence(ctx, tx, stages[n], "2000-01-01T00:00:00Z")
			if err != nil {
				return err
			}
			return checkMBEvidenceBudget(ctx, tx)
		})
	}
	var distinct int
	if err := db.QueryRow(`SELECT count(*) FROM mb_recording_evidence`).Scan(&distinct); err != nil || distinct != 2*mbEvidenceTracks {
		t.Fatal("fixture did not create 20k disjoint recording revisions", distinct, err)
	}
	assertMBAccounting(t, db)
	// Exactly 20,000 unaccepted recordings fit; adding the 20,001st with its
	// real normalized child set must roll the complete prospective stage back.
	w, err := dbwork.Begin(ctx, db, dbwork.ClassBackgroundMedia)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := stageMBRecording(metadataprovider.RecordingLookup{RequestedID: mbLaneRecord, Recording: mbLaneRecordingFixture()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = saveMBRecordingEvidence(ctx, w.Tx(), stage, "now"); err != nil {
		t.Fatal(err)
	}
	if err = checkMBEvidenceBudget(ctx, w.Tx()); err != errMBBudget {
		t.Fatal("20,001st recording admitted", err)
	}
	if err = w.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertMBAccounting(t, db)

	var library int64
	if err = db.QueryRow(`SELECT library_id FROM catalog_entities WHERE id=?`, names["album"].ID).Scan(&library); err != nil {
		t.Fatal(err)
	}
	// Many direct album AND song pins do not multiply the accepted-release
	// closure. Only the final zero crossing walks the 10k track window.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n <= 128; n++ {
		kind := 6
		if n%2 == 1 {
			kind = 7
		}
		id := 1_000_000 + n
		if _, err = tx.Exec(`INSERT INTO catalog_entities(id,public_id,library_id,kind,title,sort_key,sort_head) VALUES(?,randomblob(16),?,?,'pin','pin','p')`, id, library, kind); err != nil {
			t.Fatal(err)
		}
		if kind == 6 {
			_, err = tx.Exec(`INSERT INTO mb_album_links VALUES(?,?,?,?,'now')`, id, revisions[0], values[0].Release.ID, values[0].RequestedID)
		} else {
			_, err = tx.Exec(`INSERT INTO mb_song_links(item_id,release_revision,release_id,observed_at) VALUES(?,?,?,'now')`, id, revisions[0], values[0].Release.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertMBAccounting(t, db)
	measure("128 non-last album/song pin cascades (no track-window walk)", &mbPublicationAccountingPlan, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM catalog_entities WHERE id>=1000000 AND id<1000128`)
		return err
	})
	assertMBAccounting(t, db)
	// Exercise a true old->new disjoint move, not a repeated-recording proxy.
	measure("old 10k -> disjoint new 10k pin move + exact budget", &mbPublicationAccountingPlan, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE mb_album_links SET release_revision=?,release_id=?,requested_id=? WHERE album_id=1000128`, revisions[1], values[1].Release.ID, values[1].RequestedID); err != nil {
			return err
		}
		return checkMBEvidenceBudget(ctx, tx)
	})
	assertMBAccounting(t, db)
	// Include current final-publication's reconstruction/validation inside the
	// writer, separately from pure accounting, so its existing cost is visible.
	measure("current final read/validate + reverse disjoint pin move", &mbPublicationAccountingPlan, func(tx *sql.Tx) error {
		if _, _, err := loadMBReleaseEvidence(ctx, tx, revisions[0], false); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE mb_album_links SET release_revision=?,release_id=?,requested_id=? WHERE album_id=1000128`, revisions[0], values[0].Release.ID, values[0].RequestedID); err != nil {
			return err
		}
		return checkMBEvidenceBudget(ctx, tx)
	})
	assertMBAccounting(t, db)
	measure("last album pin removed via real entity FK cascade (10k counters)", &mbPublicationAccountingPlan, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM catalog_entities WHERE id=1000128`)
		return err
	})
	assertMBAccounting(t, db)
	measure("one 10k-track release parent retention cascade", &mbPublicationAccountingPlan, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM mb_release_evidence WHERE revision_id=?`, revisions[0])
		return err
	})
	assertMBAccounting(t, db)
	// A physical plan assertion prevents a future recording-only probe plus
	// residual release filter from growing with other releases' shared tracks.
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT recording_revision,count(*) FROM mb_release_tracks INDEXED BY mb_release_track_release_recording WHERE release_revision=? GROUP BY recording_revision`, revisions[1])
	if err != nil {
		t.Fatal(err)
	}
	var details string
	for rows.Next() {
		var a, b, c int
		var detail string
		if err = rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		details += detail
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details, "SEARCH mb_release_tracks USING COVERING INDEX mb_release_track_release_recording (release_revision=?)") {
		t.Fatal(details)
	}
	for _, m := range measurements {
		if m.held > 250*time.Millisecond {
			t.Errorf("SRV023 STOP before merge: %s held writer %s >250ms; staged-publication protocol required", m.label, m.held)
		}
	}
}
