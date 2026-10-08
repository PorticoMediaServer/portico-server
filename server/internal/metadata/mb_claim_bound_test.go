package metadata

// The fixture songs are compact catalogue tracks and their MusicBrainz jobs
// are minted by the catalogue writer before synthetic leases are applied.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// The MusicBrainz claim once scanned mb_jobs under the write gate; with ~1M
// pending jobs every write waited behind it. The claim must stay O(batch):
// selection runs outside the gate as a LIMIT 1 walk of mb_jobs_due, the gate
// sees one job by key, and the live-lease probe reads the bounded operations
// table through its status index. This test builds a large synthetic pending
// set and asserts all three: every claim query plan uses an index with no full
// scan, the claim still finds the due job, and a concurrent write completes
// within a small bound while the claim runs.
func TestMBClaimStaysBoundedOverLargePendingSet(t *testing.T) {
	db, names := mbLaneDB(t)
	defer db.Close()
	// The query plans are the guard at any size; the release and deep tiers also
	// time the claim and a concurrent write over 200,000 pending jobs.
	synthJobs := 5000
	if mbClaimScaleTier() {
		synthJobs = 200000
	}
	seedMBClaimJobs(t, db, synthJobs)
	// After the seed: inserting songs reset the fixture album's job to pending
	// through the song-created trigger, so exclude it again to exercise the
	// song path deterministically.
	mbClaimExec(t, db, `UPDATE mb_jobs SET status='needs_selection' WHERE kind='album'`)
	var jobs int
	if e := db.QueryRow(`SELECT count(*) FROM mb_jobs`).Scan(&jobs); e != nil {
		t.Fatal(e)
	}
	if jobs != synthJobs+2 {
		t.Fatalf("synthetic pending set has %d jobs, not %d", jobs, synthJobs+2)
	}
	stamp := "2026-09-24T12:00:00Z"
	mbClaimPlan(t, db, "live-lease probe", `SELECT count(*) FROM mb_publication_operations WHERE status IN('claimed','staged') AND lease_until=?`, "mb_operations_status", stamp)
	mbClaimPlan(t, db, "due-job search", `SELECT kind,entity_id FROM mb_jobs WHERE `+mbClaimable+` ORDER BY next_attempt,lease_until,kind,entity_id LIMIT 1`, "mb_jobs_due", stamp, stamp, stamp)
	mbClaimPlan(t, db, "gated re-check", `SELECT kind,entity_id,selected_id,manual,review_search,generation,attempts FROM mb_jobs WHERE kind=? AND entity_id=? AND `+mbClaimable, "sqlite_autoindex_mb_jobs_1", "song", names["song"].ID, stamp, stamp, stamp)

	s := New(db, "")
	claimCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	j, e := s.claimMB(claimCtx)
	elapsed := time.Since(start)
	if e != nil || j == nil {
		t.Fatal("claim over the synthetic set found nothing", j, e)
	}
	if j.kind != "song" || j.id != names["song"].Public || j.lease == "" || j.until == "" {
		t.Fatalf("claim returned the wrong job: %+v", j)
	}
	t.Logf("claim over %d pending jobs took %s", jobs, elapsed)

	// The claim leased the song; release it so the concurrency pass claims again.
	mbClaimExec(t, db, `UPDATE mb_jobs SET lease='',lease_until='' WHERE kind='song' AND entity_id=?;DELETE FROM mb_publication_operations`, names["song"].ID)
	done := make(chan error, 1)
	go func() {
		_, e := s.claimMB(claimCtx)
		done <- e
	}()
	writeStart := time.Now()
	_, e = dbwork.ExecWrite(claimCtx, db, dbwork.ClassBackgroundMedia, `UPDATE catalog_entities SET title='Concurrent write',sort_key=portico_sort_title('Concurrent write',sort_title,metadata_language),sort_head=substr(portico_sort_title('Concurrent write',sort_title,metadata_language),1,1) WHERE id=?`, names["song"].ID)
	writeElapsed := time.Since(writeStart)
	if e != nil {
		t.Fatal("concurrent write failed while the claim ran", e)
	}
	if e = <-done; e != nil {
		t.Fatal("concurrent claim failed", e)
	}
	t.Logf("concurrent write during the claim took %s", writeElapsed)
	if writeElapsed > 10*time.Second {
		t.Fatalf("concurrent write blocked %s behind the claim", writeElapsed)
	}
}

// seedMBClaimJobs adds n pending song jobs that sort before the fixture's own
// song in claim order but can never be claimed: their leases are held to 2099,
// so the claim's index walk must step over every one of them through
// mb_jobs_due without touching the write gate. Inserts go through the real
// catalog tables so the triggers mint the jobs exactly as a scan would.
func seedMBClaimJobs(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	c := catalogtest.New(t, db)
	var album int64
	if err := db.QueryRow(`SELECT id FROM catalog_entities WHERE kind=? AND title='Local Album'`, int(compactcatalog.Album)).Scan(&album); err != nil {
		t.Fatal(err)
	}
	library := c.Handle("lib")
	start := time.Now()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for i := 0; i < n; i++ {
			path := fmt.Sprintf("/music/aa%06d.flac", i)
			id, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{
				Library: library, Kind: compactcatalog.Track, Parent: album,
				Key: compactcatalog.ItemKey("/music", path, 0), Title: fmt.Sprintf("Synth %06d", i),
			})
			if err != nil {
				return err
			}
			if err = compactcatalog.SetFactsTx(ctx, tx, id, map[string]any{"album_id": album, "disc_number": 1, "track_number": i + 1}); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE mb_jobs SET lease='synthetic',lease_until='2099-01-01T00:00:00Z' WHERE kind='song' AND entity_id=?`, id); err != nil {
				return err
			}
		}
		return nil
	})
	t.Logf("seeded %d synthetic pending jobs in %s", n, time.Since(start))
}

// This local copy keeps the bounded-claim test independent of the unassigned
// performance_tier_test.go helper.
func mbClaimScaleTier() bool {
	switch os.Getenv("PORTICO_PERFORMANCE_TIER") {
	case "release", "deep":
		return true
	}
	return false
}

// mbClaimPlan asserts the query is served by the named index: every plan node
// is an index search, with no table scan and no sort spill. A scan of mb_jobs
// here would be O(jobs) per claim.
func mbClaimPlan(t *testing.T, db *sql.DB, name, query, index string, args ...any) {
	t.Helper()
	rows, e := db.Query(`EXPLAIN QUERY PLAN `+query, args...)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	seen := false
	for rows.Next() {
		var a, b, c int
		var detail string
		if e = rows.Scan(&a, &b, &c, &detail); e != nil {
			t.Fatal(e)
		}
		t.Logf("PLAN %s | %s", name, detail)
		if strings.Contains(detail, "SCAN") || strings.Contains(detail, "TEMP B-TREE") {
			t.Fatalf("claim query is not index-bounded: %s | %s", name, detail)
		}
		if strings.Contains(detail, "SEARCH") && strings.Contains(detail, index) {
			seen = true
		}
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	if !seen {
		t.Fatalf("claim query does not use %s: %s", index, name)
	}
}

func mbClaimExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, e := db.Exec(query, args...); e != nil {
		t.Fatal(query, e)
	}
}
