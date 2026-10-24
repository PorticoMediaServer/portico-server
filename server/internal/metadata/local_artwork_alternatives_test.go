package metadata

import (
	"context"
	"database/sql"
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
	want := []string{"local:" + first, "local:" + primary, "local:" + second}
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
	c.Exec(`INSERT INTO library_sources(id,library_id,kind,name,configured_root,root) VALUES(?,?,'local','Movies',?,?)`, library, library, "/"+library, "/"+library)
	var incarnation string
	if err := db.QueryRow(`SELECT incarnation FROM library_sources WHERE id=?`, library).Scan(&incarnation); err != nil {
		t.Fatal(err)
	}
	movie := c.Movie(c.Handle(library), "/"+library+"/Film.mkv", "Film", 2000)
	mustExec(t, db, `INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) VALUES(?,?,?,?,'Film.mkv','r1','{}',1,1)`, "object-"+library, library, movie.Token, incarnation)
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
