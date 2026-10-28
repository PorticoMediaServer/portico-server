package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

func mbRetireTestMark(t *testing.T, db *sql.DB, kind, rev, state, owner, parent string) {
	t.Helper()
	zero := strings.Repeat("0", 64)
	_, err := db.Exec(`INSERT INTO mb_evidence_builds(build_id,kind,revision_id,provider_id,digest,state,phase,expected_shape_digest,expected_rows,materialized_rows,materialized_digest,parent_build,owner_operation,expires_at) SELECT revision_id,?,revision_id,provider_id,digest,?,'cleanup',?,1,1,?,?,?,'2000-01-01T00:00:00Z' FROM mb_`+kind+`_evidence WHERE revision_id=?`, kind, state, zero, zero, parent, owner, rev)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMBRetireNormalizedWindowsAndParentGuard(t *testing.T) {
	db, _ := mbLaneDB(t)
	defer db.Close()
	value := mbLaneReleaseFixture()
	value.Media[0].Tracks = nil
	for i := 0; i < 97; i++ {
		r := mbLaneRecordingFixture()
		r.ID = fmt.Sprintf("%08x-0000-0000-0000-000000000000", i+1000)
		value.Media[0].Tracks = append(value.Media[0].Tracks, metadataprovider.ReleaseTrack{ID: fmt.Sprintf("%08x-0000-0000-0000-000000000000", i+2000), Position: i + 1, Recording: r})
	}
	stage, err := stageMBRelease(metadataprovider.ReleaseLookup{RequestedID: mbLaneRelease, Release: value})
	if err != nil {
		t.Fatal(err)
	}
	var rev string
	if err = dbwork.WithWriteTx(context.Background(), db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		var e error
		rev, e = saveMBReleaseEvidence(context.Background(), tx, stage, "2000-01-01T00:00:00Z")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM mb_release_evidence WHERE revision_id=?`, rev); err == nil {
		t.Fatal("parent cascade bypassed normalized retirement")
	}
	s := New(db, "")
	for turn := 0; turn < 150; turn++ {
		if readPickCount(t, db, `SELECT count(*) FROM mb_release_evidence WHERE revision_id=?`, rev) == 0 {
			break
		}
		dbwork.ResetPlans()
		if err = s.mbRetireEvidence(context.Background()); err != nil {
			t.Fatal(turn, err)
		}
		assertMBAccounting(t, db)
		if got := dbwork.Plans().Largest; got.Rows > 160 {
			t.Fatalf("turn%d exceeded transitive plan: %+v", turn, got)
		}
	}
	if n := readPickCount(t, db, `SELECT count(*) FROM mb_release_evidence WHERE revision_id=?`, rev); n != 0 {
		t.Fatal("release did not finish", n)
	}
	if n := readPickCount(t, db, `SELECT count(*) FROM mb_evidence_builds WHERE kind='release' AND revision_id=?`, rev); n != 0 {
		t.Fatal("finished build retained", n)
	}
}

func TestMBRetireLiveOwnerAndAbortedOriginChildren(t *testing.T) {
	db, _ := mbLaneDB(t)
	defer db.Close()
	s := New(db, "")
	j := mbStagingJob(t, s, "album")
	r := mbLaneRecordingFixture()
	stage, err := stageMBRecording(metadataprovider.RecordingLookup{RequestedID: r.ID, Recording: r})
	if err != nil {
		t.Fatal(err)
	}
	var child string
	if err = dbwork.WithWriteTx(context.Background(), db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		var e error
		child, e = saveMBRecordingEvidence(context.Background(), tx, stage, "2000-01-01T00:00:00Z")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	mbRetireTestMark(t, db, "recording", child, "ready", j.operation, "")
	if err = s.mbRetireEvidence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := readPickCount(t, db, `SELECT count(*) FROM mb_recording_evidence WHERE revision_id=?`, child); n != 1 {
		t.Fatal("live inter-quantum child reservation retired")
	}
	// Expired owner is no authority. Its ready child is drained without waiting
	// thirty days, even though its parent never got as far as a track insert.
	mbCurrentExec(t, db, `UPDATE mb_jobs SET lease_until='2000-01-01T00:00:00Z'`)
	for i := 0; i < 8; i++ {
		if err = s.mbRetireEvidence(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertMBAccounting(t, db)
	}
	if n := readPickCount(t, db, `SELECT count(*) FROM mb_recording_evidence WHERE revision_id=?`, child); n != 0 {
		t.Fatal("expired ready reservation leaked")
	}
	// Newly made child provenance survives owner clearing when its track is
	// attached; aborting the parent drains/detaches that origin before its row.
	release := readPickRelease(t, db, "Aborted origin")
	var origin string
	if err = db.QueryRow(`SELECT recording_revision FROM mb_release_tracks WHERE release_revision=? LIMIT 1`, release).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	mbRetireTestMark(t, db, "release", release, "aborting", "", "")
	mbRetireTestMark(t, db, "recording", origin, "ready", "", release)
	for i := 0; i < 25; i++ {
		if err = s.mbRetireEvidence(context.Background()); err != nil {
			t.Fatal(i, err)
		}
		assertMBAccounting(t, db)
	}
	if n := readPickCount(t, db, `SELECT count(*) FROM mb_evidence_builds WHERE revision_id IN(?,?)`, release, origin); n != 0 {
		t.Fatal("abort ancestry leaked", n)
	}
}

func TestMBRetireRollbackRetainsRowsAndCharge(t *testing.T) {
	db, _ := mbLaneDB(t)
	defer db.Close()
	rev := readPickRelease(t, db, "Rollback")
	mbRetireTestMark(t, db, "release", rev, "aborting", "", "")
	before := readPickCount(t, db, `SELECT charge FROM mb_evidence_totals`)
	w, err := dbwork.Begin(context.Background(), db, dbwork.ClassBackgroundMedia)
	if err != nil {
		t.Fatal(err)
	}
	target, err := mbRetirePick(context.Background(), w.Tx(), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	if err != nil || target == nil {
		t.Fatal(target, err)
	}
	if _, err = mbRetireRowsTx(context.Background(), w.Tx(), target); err != nil {
		t.Fatal(err)
	}
	if err = w.Rollback(); err != nil {
		t.Fatal(err)
	}
	if n := readPickCount(t, db, `SELECT charge FROM mb_evidence_totals`); n != before {
		t.Fatal("rollback lost charge", n, before)
	}
	if n := readPickCount(t, db, `SELECT count(*) FROM mb_release_tracks WHERE release_revision=?`, rev); n != 1 {
		t.Fatal("rollback lost track", n)
	}
	assertMBAccounting(t, db)
}
