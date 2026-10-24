package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/persistence"
)

// The scan keeps numbered sidecar images beside the primary one. They reach the
// editor as ordinary provider='local' candidates, seeded after the primary, so
// they can never replace a slot that is already held.
func TestSeedLocalArtworkAddsScannedAlternativesAfterThePrimary(t *testing.T) {
	db, movie := openAlternativesFixture(t, "alt1")
	primary := strings.Repeat("c", 64)
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	mustExec(t, db, `INSERT INTO local_artwork(library_id,kind,entity_id,role,art_key,observed_at) VALUES('alt1','item',?,'poster',?,'now')`, movie.ID, primary)
	seedScannedAlternatives(t, db, "alt1", movie, "poster", first, second)
	fence := seedLocalArtworkAlternativesNow(t, db, "item", movie)
	origins, fences := localCandidates(t, db, movie.ID, "poster")
	want := []string{"local:" + first, "local:" + second, "local:" + primary}
	if len(origins) != len(want) {
		t.Fatalf("candidates %v", origins)
	}
	for n, origin := range want {
		if origins[n] != origin {
			t.Fatalf("candidates %v, want %v", origins, want)
		}
		if fences[n] != fence {
			t.Fatalf("%s kept fence %q, want %q", origin, fences[n], fence)
		}
	}
	jobs := localArtworkJobs(t, db, movie.ID, "poster")
	if len(jobs) != 1 || jobs[0] != "local:"+primary {
		t.Fatalf("alternatives changed the queued slot: %v", jobs)
	}
}

// A head with no primary sidecar for a slot still gets its numbered images;
// the first accepted one (the reader's deterministic key order) queues the
// ordinary local fetch.
func TestSeedLocalArtworkQueuesTheFirstScannedAlternativeWhenNothingHoldsTheSlot(t *testing.T) {
	db, movie := openAlternativesFixture(t, "alt2")
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	seedScannedAlternatives(t, db, "alt2", movie, "poster", second, first)
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	origins, _ := localCandidates(t, db, movie.ID, "poster")
	if len(origins) != 2 {
		t.Fatalf("candidates %v", origins)
	}
	jobs := localArtworkJobs(t, db, movie.ID, "poster")
	if len(jobs) != 1 || jobs[0] != "local:"+first {
		t.Fatalf("queued slot %v, want the first accepted alternative", jobs)
	}
}

// An owner's locked choice stays the slot's authority: the alternative is
// listed for a later manual pick but nothing is queued or replaced.
func TestSeedLocalArtworkKeepsALockedOwnerChoice(t *testing.T) {
	db, movie := openAlternativesFixture(t, "alt3")
	digest := strings.Repeat("d", 64)
	mustExec(t, db, `INSERT INTO artwork_objects(digest,mime,width,height,bytes,created_at) VALUES(?,'image/jpeg',1,1,1,'now')`, digest)
	mustExec(t, db, `INSERT INTO artwork_candidates(id,kind,entity_id,role,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('owner-pick','item',?,'poster','local','owner','local:owner-pick','Local file','older-fence','now')`, movie.ID)
	mustExec(t, db, `INSERT INTO artwork_selections(kind,entity_id,role,candidate_id,digest,thumbnail_digest,locked,actor,observed_at) VALUES('item',?,'poster','owner-pick',?,?,1,'owner','now')`, movie.ID, digest, digest)
	seedScannedAlternatives(t, db, "alt3", movie, "poster", strings.Repeat("a", 64))
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	origins, _ := localCandidates(t, db, movie.ID, "poster")
	if len(origins) != 2 {
		t.Fatalf("candidates %v", origins)
	}
	if jobs := localArtworkJobs(t, db, movie.ID, "poster"); len(jobs) != 0 {
		t.Fatalf("an owner choice was queued over: %v", jobs)
	}
	var selected string
	if err := db.QueryRow(`SELECT candidate_id FROM artwork_selections WHERE kind='item' AND entity_id=? AND role='poster'`, movie.ID).Scan(&selected); err != nil || selected != "owner-pick" {
		t.Fatalf("owner selection changed to %q (%v)", selected, err)
	}
}

// openAlternativesFixture opens a movie library with one live inventory object
// for the movie's asset, so catalogue candidate reads see captured evidence.
func openAlternativesFixture(t *testing.T, library string) (*sql.DB, catalogtest.Item) {
	t.Helper()
	db, err := persistence.Open(filepath.Join(t.TempDir(), library+".sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := catalogtest.New(t, db)
	c.Library(library, "Movies", "movie", "/"+library)
	var source, incarnation string
	if err := db.QueryRow(`SELECT id,incarnation FROM library_sources WHERE library_id=?`, library).Scan(&source, &incarnation); err != nil {
		t.Fatal(err)
	}
	movie := c.Movie(c.Handle(library), "/"+library+"/Film.mkv", "Film", 2000)
	mustExec(t, db, `INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) VALUES(?,?,?,?,'Film.mkv','r1','{}',1,1)`, "object-"+library, source, movie.Token, incarnation)
	return db, movie
}

func seedScannedAlternatives(t *testing.T, db *sql.DB, library string, movie catalogtest.Item, role string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		mustExec(t, db, `INSERT INTO inventory_artwork_candidates(object_id,source_revision,asset_id,kind,entity_id,role,art_key) VALUES(?,?,?,'item',?,?,?)`, "object-"+library, "r1", movie.Token, movie.ID, role, key)
	}
}

// seedLocalArtworkAlternativesNow runs the real seeding path in one transaction
// and returns the fence the candidates were seeded under.
func seedLocalArtworkAlternativesNow(t *testing.T, db *sql.DB, kind string, target catalogtest.Item) string {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	repairTarget := RepairTarget{Kind: kind, ID: target.Public}
	fence, err := artworkFence(ctx, tx, repairTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err = seedLocalArtwork(ctx, tx, repairTarget, fence, "now"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return fence
}

func localCandidates(t *testing.T, db *sql.DB, entity int64, role string) (origins, fences []string) {
	t.Helper()
	rows, err := db.Query(`SELECT origin,source_fence FROM artwork_candidates WHERE kind='item' AND entity_id=? AND role=? AND provider='local' ORDER BY origin`, entity, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var origin, fence string
		if err = rows.Scan(&origin, &fence); err != nil {
			t.Fatal(err)
		}
		origins = append(origins, origin)
		fences = append(fences, fence)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return origins, fences
}

func localArtworkJobs(t *testing.T, db *sql.DB, entity int64, role string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT origin FROM artwork_jobs WHERE kind='item' AND entity_id=? AND role=? ORDER BY created_at,id`, entity, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	jobs := []string{}
	for rows.Next() {
		var origin string
		if err = rows.Scan(&origin); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, origin)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return jobs
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

// alternativesKeys returns n distinct accepted image keys in the reader's
// deterministic ascending order.
func alternativesKeys(n int) []string {
	keys := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		keys = append(keys, fmt.Sprintf("%064x", i))
	}
	return keys
}

func containsOrigin(origins []string, key string) bool {
	for _, origin := range origins {
		if origin == "local:"+key {
			return true
		}
	}
	return false
}

// Nine accepted images used to stop at the first eight-key page; every one of
// them must reach the editor and exactly the first queued pick stays.
func TestSeedLocalArtworkSeedsEveryAcceptedAlternativeAcrossPages(t *testing.T) {
	db, movie := openAlternativesFixture(t, "alt9")
	keys := alternativesKeys(9)
	seedScannedAlternatives(t, db, "alt9", movie, "poster", keys...)
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	origins, _ := localCandidates(t, db, movie.ID, "poster")
	if len(origins) != len(keys) {
		t.Fatalf("seeded %d of %d accepted images", len(origins), len(keys))
	}
	for _, key := range keys {
		if !containsOrigin(origins, key) {
			t.Fatalf("accepted image %s was skipped", key)
		}
	}
	if jobs := localArtworkJobs(t, db, movie.ID, "poster"); len(jobs) != 1 || jobs[0] != "local:"+keys[0] {
		t.Fatalf("queued slot %v, want the first accepted image", jobs)
	}
}

// The catalogue reader caps one call at 128; 130 accepted images must cross that
// bound through continuation instead of being capped.
func TestSeedLocalArtworkContinuationCrossesTheReaderBound(t *testing.T) {
	db, movie := openAlternativesFixture(t, "alt130")
	keys := alternativesKeys(130)
	seedScannedAlternatives(t, db, "alt130", movie, "poster", keys...)
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	origins, _ := localCandidates(t, db, movie.ID, "poster")
	if len(origins) != len(keys) {
		t.Fatalf("seeded %d of %d accepted images", len(origins), len(keys))
	}
	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM artwork_candidates WHERE kind='item' AND entity_id=? AND role='poster' AND provider='local'`, movie.ID).Scan(&rows); err != nil || rows != len(keys) {
		t.Fatalf("candidate rows %d of %d (%v)", rows, len(keys), err)
	}
	if jobs := localArtworkJobs(t, db, movie.ID, "poster"); len(jobs) != 1 || jobs[0] != "local:"+keys[0] {
		t.Fatalf("queued slot %v, want the first accepted image", jobs)
	}
}

// An interruption after one page must leave the following pass with the same
// final set as an uninterrupted pass, without duplicate or skipped candidates.
func TestSeedLocalArtworkRestartsAnInterruptedContinuation(t *testing.T) {
	cleanDB, cleanMovie := openAlternativesFixture(t, "altclean")
	cleanKeys := alternativesKeys(130)
	seedScannedAlternatives(t, cleanDB, "altclean", cleanMovie, "poster", cleanKeys...)
	seedLocalArtworkAlternativesNow(t, cleanDB, "item", cleanMovie)

	db, movie := openAlternativesFixture(t, "altrestart")
	keys := alternativesKeys(130)
	seedScannedAlternatives(t, db, "altrestart", movie, "poster", keys...)
	// Simulate a pass that stopped after its first page: the transaction rolls
	// back, so no partial progress is durable and the dirty row would stay.
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := RepairTarget{Kind: "item", ID: movie.Public}
	fence, err := artworkFence(ctx, tx, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, done, err := seedLocalArtworkCandidatePage(ctx, tx, target, movie.ID, "poster", "", fence, "now"); err != nil || done {
		t.Fatalf("first page done=%v err=%v", done, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	seedLocalArtworkAlternativesNow(t, db, "item", movie)

	restarted, _ := localCandidates(t, db, movie.ID, "poster")
	clean, _ := localCandidates(t, cleanDB, cleanMovie.ID, "poster")
	if len(restarted) != len(keys) || len(clean) != len(keys) {
		t.Fatalf("restarted %d, clean %d of %d", len(restarted), len(clean), len(keys))
	}
	for n := range clean {
		if restarted[n] != clean[n] {
			t.Fatalf("restarted set differs at %d: %q vs %q", n, restarted[n], clean[n])
		}
	}
	if got, want := localArtworkJobs(t, db, movie.ID, "poster"), localArtworkJobs(t, cleanDB, cleanMovie.ID, "poster"); len(got) != 1 || len(want) != 1 || got[0] != want[0] {
		t.Fatalf("restarted jobs %v, clean jobs %v", got, want)
	}
}

// A repeated dirty pass is stable: the same candidate rows and the same queued
// pick, with no duplicates.
func TestSeedLocalArtworkRepeatDirtyIsStable(t *testing.T) {
	db, movie := openAlternativesFixture(t, "altrepeat")
	keys := alternativesKeys(20)
	seedScannedAlternatives(t, db, "altrepeat", movie, "poster", keys...)
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	first, _ := localCandidates(t, db, movie.ID, "poster")
	firstJobs := localArtworkJobs(t, db, movie.ID, "poster")
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	second, _ := localCandidates(t, db, movie.ID, "poster")
	secondJobs := localArtworkJobs(t, db, movie.ID, "poster")
	if len(first) != len(second) {
		t.Fatalf("repeat dirty changed the set: %d then %d", len(first), len(second))
	}
	for n := range first {
		if first[n] != second[n] {
			t.Fatalf("repeat dirty differs at %d: %q vs %q", n, first[n], second[n])
		}
	}
	if len(firstJobs) != 1 || len(secondJobs) != 1 || firstJobs[0] != secondJobs[0] {
		t.Fatalf("repeat dirty changed the queued pick: %v then %v", firstJobs, secondJobs)
	}
}

// A source revision replaces the accepted evidence; the pass converges on the
// new keys, keeps candidate history, and leaves an owner choice and the slot's
// queued pick alone.
func TestSeedLocalArtworkFollowsASourceRevision(t *testing.T) {
	db, movie := openAlternativesFixture(t, "altrev")
	keys := alternativesKeys(3)
	seedScannedAlternatives(t, db, "altrev", movie, "poster", keys...)
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	jobs := localArtworkJobs(t, db, movie.ID, "poster")
	if len(jobs) != 1 {
		t.Fatalf("initial queued slot %v", jobs)
	}
	fresh := []string{fmt.Sprintf("%064x", 10), fmt.Sprintf("%064x", 11)}
	mustExec(t, db, `UPDATE inventory_objects SET revision='r2' WHERE id='object-altrev'`)
	mustExec(t, db, `DELETE FROM inventory_artwork_candidates WHERE object_id='object-altrev'`)
	for _, key := range fresh {
		mustExec(t, db, `INSERT INTO inventory_artwork_candidates(object_id,source_revision,asset_id,kind,entity_id,role,art_key) VALUES('object-altrev','r2',?,'item',?,'poster',?)`, movie.Token, movie.ID, key)
	}
	seedLocalArtworkAlternativesNow(t, db, "item", movie)
	origins, _ := localCandidates(t, db, movie.ID, "poster")
	for _, key := range fresh {
		if !containsOrigin(origins, key) {
			t.Fatalf("revised image %s was skipped", key)
		}
	}
	if got := localArtworkJobs(t, db, movie.ID, "poster"); len(got) != 1 || got[0] != jobs[0] {
		t.Fatalf("source revision changed the queued pick: %v then %v", jobs, got)
	}
}
