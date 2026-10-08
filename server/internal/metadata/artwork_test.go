package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// resolveArtworkTestEntity maps a fixture target's public id to its integer
// entity id for new-schema test assertions.
func resolveArtworkTestEntity(t *testing.T, db *sql.DB, target RepairTarget) int64 {
	t.Helper()
	id, err := entityid.Resolve(context.Background(), db, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// artworkFixtureTarget finds a converted fixture's entity by catalogue kind
// and library. TODO(phase34): editorFixture (repair_editor_test.go, another
// lane) still creates old-schema rows; confirm these keys when that lane
// converts it to catalogue entities.
func artworkFixtureTarget(t *testing.T, db *sql.DB, kind int, library string) RepairTarget {
	t.Helper()
	var public string
	if err := db.QueryRow(`SELECT pid(e.public_id) FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.kind=? AND cl.library_id=? LIMIT 1`, kind, library).Scan(&public); err != nil {
		t.Fatal(err)
	}
	return RepairTarget{"item", public}
}

// BE-SRV-14: there is no artwork lock. A removal sets the file aside and asks
// the database; an object referenced meanwhile keeps (gets back) its file, and
// a writer reinstalls a file that a concurrent removal took after it committed.
func TestArtworkLifecycleNeedsNoLock(t *testing.T) {
	s, db, _ := repairFixture(t)
	ctx := context.Background()
	raw := p08cPNG(t, 12, 12)
	o, err := s.installArtworkFile(raw, 12, 12)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.cacheRoot, o.digest+".img")
	// A publication lands between the rename and the database decision.
	kept, err := s.retireArtworkFile(ctx, o.digest, func(context.Context) (bool, error) {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("the file was not set aside before the decision")
		}
		return false, nil
	})
	if err != nil || kept {
		t.Fatal("retired a live object", kept, err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("a live object lost its file", err)
	}
	// The removal wins the race: the writer's post-commit check reinstalls.
	if gone, err := s.retireArtworkFile(ctx, o.digest, func(context.Context) (bool, error) { return true, nil }); err != nil || !gone {
		t.Fatal(gone, err)
	}
	if err = s.ensureArtworkFiles(o); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("the writer did not reinstall its file", err)
	}
	// Collection: an old unreferenced object goes; a tomb left by a crash is
	// restored while its object is known.
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err = db.Exec(`INSERT INTO artwork_objects VALUES(?,?,12,12,?,?,'ready')`, o.digest, o.mime, o.size, old); err != nil {
		t.Fatal(err)
	}
	tomb := o.digest + artworkTombMarker + "crash"
	if err = os.Rename(path, filepath.Join(s.cacheRoot, tomb)); err != nil {
		t.Fatal(err)
	}
	if err = s.settleArtworkTomb(ctx, tomb); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("a crash tomb of a known object was not restored", err)
	}
	if err = s.CleanupArtwork(ctx); err != nil {
		t.Fatal(err)
	}
	var left int
	if err = db.QueryRow(`SELECT count(*) FROM artwork_objects WHERE digest=?`, o.digest).Scan(&left); err != nil || left != 0 {
		t.Fatal("unreferenced object was not collected", left, err)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("collected object kept its file", err)
	}
}

// Reads never perform provider IO. The accepted-identity worker first installs
// immutable bytes; later reads keep last-good art through untrusted/stale URLs.
func TestArtworkIsFixedOriginCredentialFreeAndCached(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	body := p08cPNG(t, 20, 20)
	requests := 0
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "image.tmdb.org" || r.Header.Get("Authorization") != "" {
			t.Fatal("artwork credential or origin leak")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}
	if f, _, e := s.Artwork(ctx, target.ID, "poster"); e == nil {
		f.Close()
		t.Fatal("unexpected uninstalled bytes")
	} else if !errors.Is(e, ErrArtworkPending) {
		t.Fatal(e)
	}
	if requests != 0 {
		t.Fatal("read fetched provider bytes")
	}
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.ArtworkStep(ctx); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 2; n++ {
		f, mime, e := s.Artwork(ctx, target.ID, "poster")
		if e != nil {
			t.Fatal(e)
		}
		if mime != "image/png" {
			t.Fatal(mime)
		}
		f.Close()
	}
	if requests != 1 {
		t.Fatalf("cache made %d provider calls", requests)
	}
	integrationExec(t, db, `UPDATE catalog_item_details SET poster_url='https://evil.example/test.png' WHERE entity_id=?`, resolveArtworkTestEntity(t, db, target))
	f, _, e := s.Artwork(ctx, target.ID, "poster")
	if e != nil {
		t.Fatal("lost last-good selection", e)
	}
	f.Close()
	if _, e = s.fetchArtwork(ctx, "https://evil.example/test.png", "tmdb"); e == nil {
		t.Fatal("untrusted origin accepted")
	}
	if requests != 1 {
		t.Fatal("untrusted or read path contacted provider")
	}
}

func TestArtworkReseedsAfterSourceFenceChanges(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	requests := 0
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(p08cPNG(t, 20, 20))), Header: http.Header{}}, nil
	})}
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.seedArtwork(ctx); err != nil {
		t.Fatal(err)
	}
	integrationExec(t, db, `UPDATE artwork_entity_heads SET revision=revision+1 WHERE kind='item' AND entity_id=?`, resolveArtworkTestEntity(t, db, target))
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatal("stale source downloaded")
	}
	var dirty bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM artwork_dirty WHERE kind='item' AND entity_id=?)`, resolveArtworkTestEntity(t, db, target)).Scan(&dirty); err != nil || !dirty {
		t.Fatal("stale artwork lost its repair intent", err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.Artwork(ctx, target.ID, "poster")
	if err != nil {
		t.Fatal("artwork never recovered after source changed", err)
	}
	f.Close()
}

// BE-SRV-14: compaction reads only its candidates through the partial index
// (whose literals must match the encoding thresholds), and one undecodable
// object is recorded and skipped instead of stalling the queue.
func TestArtworkCompactionSkipsABadObjectAndSeeksCandidates(t *testing.T) {
	if artworkDisplayBytes != 500000 || artworkLargeEdge != 1920 {
		t.Fatal("artwork thresholds changed: update migration 0103's partial index and CompactArtworkStep together")
	}
	s, db, _ := repairFixture(t)
	ctx := context.Background()
	bad := strings.Repeat("0", 64)
	if err := os.WriteFile(filepath.Join(s.cacheRoot, bad+".img"), []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO artwork_objects VALUES(?,'image/png',4000,4000,9000000,'2020-01-01T00:00:00Z','ready')`, bad); err != nil {
		t.Fatal(err)
	}
	if err := s.CompactArtworkStep(ctx); err != nil {
		t.Fatal("a recorded content failure must not fail the step", err)
	}
	var reason string
	if err := db.QueryRow(`SELECT reason FROM artwork_compaction_failures WHERE digest=?`, bad).Scan(&reason); err != nil || reason != "undecodable" {
		t.Fatal("failure not recorded", reason, err)
	}
	good, err := s.installArtworkFile(p08cPNG(t, 2000, 1), 2000, 1)
	if err != nil {
		t.Fatal(err)
	}
	integrationExec(t, db, `INSERT INTO artwork_objects VALUES(?,'image/png',2000,1,?,'2020-01-01T00:00:00Z','ready')`, good.digest, good.size)
	if err := s.CompactArtworkStep(ctx); err != nil {
		t.Fatal("the recorded failure still blocks the queue", err)
	}
	var remains bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM artwork_objects WHERE digest=?)`, good.digest).Scan(&remains); err != nil || remains {
		t.Fatal("valid object behind corrupt object did not compact", remains, err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT o.digest FROM artwork_objects o INDEXED BY artwork_objects_compaction WHERE o.status='ready' AND (o.bytes>500000 OR o.width>1920 OR o.height>1920) ORDER BY o.bytes DESC,o.digest LIMIT 1`)
	if err != nil {
		t.Fatal("partial index unusable for the step's predicate", err)
	}
	rows.Close()
}

// Hold the publication gate after the fetch starts, so the worker has installed
// bytes but cannot publish. Both read routes must remain independently usable.
func TestArtworkSlowPublicationDoesNotBlockReadsOrPreviews(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw := p08cPNG(t, 20, 20)
	s.artClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}, nil
	})}
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var candidate string
	if err := db.QueryRow(`SELECT candidate_id FROM artwork_selections WHERE entity_id=? AND role='poster'`, resolveArtworkTestEntity(t, db, target)).Scan(&candidate); err != nil {
		t.Fatal(err)
	}
	integrationExec(t, db, `INSERT INTO artwork_previews SELECT c.id,s.thumbnail_digest,c.source_fence,c.observed_at FROM artwork_candidates c JOIN artwork_selections s ON s.candidate_id=c.id WHERE c.id=?`, candidate)
	integrationExec(t, db, `UPDATE artwork_jobs SET status='pending',next_attempt='' WHERE entity_id=? AND role='poster'`, resolveArtworkTestEntity(t, db, target))
	fetching, resume := make(chan struct{}), make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(resume) })
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(fetching)
		select {
		case <-resume:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}, nil
	})}
	done := make(chan error, 1)
	go func() { done <- s.ArtworkStep(ctx) }()
	select {
	case <-fetching:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	held, err := dbwork.Begin(ctx, db, dbwork.ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback()
	release.Do(func() { close(resume) })
	for len(dbwork.WriteGate().Waiting()) == 0 {
		select {
		case err := <-done:
			t.Fatalf("publication did not wait: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	reads := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func(preview bool) {
			var file *os.File
			var err error
			if preview {
				file, _, err = s.PreviewArtwork(ctx, target, candidate, 400)
			} else {
				file, _, err = s.Artwork(ctx, target.ID, "poster")
			}
			if err == nil {
				var data []byte
				data, err = io.ReadAll(file)
				file.Close()
				if err == nil && len(data) == 0 {
					err = fmt.Errorf("empty artwork")
				}
			}
			reads <- err
		}(i%2 == 0)
	}
	for i := 0; i < 32; i++ {
		select {
		case err := <-reads:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("reads waited for publication", ctx.Err())
		}
	}
	held.Rollback()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
