package metadata

import (
	"context"
	"database/sql"
	"errors"
	"image"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/imagework"
)

// floodedArtworkSource returns a service whose selected poster is an installed
// 2700x1200 source with no 1920 mapping, so a 1920 request has to generate and
// publish one. An explicit source is installed because an upload already
// publishes its 400, 800, 1920 and 2560 buckets.
func floodedArtworkSource(t *testing.T) (*Service, *sql.DB, RepairTarget, string) {
	t.Helper()
	s, db, target := repairFixture(t)
	ctx := context.Background()
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2700, 1080), editorActor(), allowAll); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.installArtworkFile(editorJPEG(t, 2700, 1200), 2700, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if err = dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		if err := recordArtworkObject(ctx, tx, legacy, "2024-01-01T00:00:00Z"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE artwork_selections SET digest=?,thumbnail_digest=?,revision=revision+1 WHERE role='poster'`, legacy.digest, legacy.digest)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return s, db, target, legacy.digest
}

// waitForWriterWaiter blocks until the write gate has queued a writer of class,
// so ordering is established by the gate itself rather than by a sleep.
func waitForWriterWaiter(t *testing.T, class dbwork.Class) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if dbwork.WriteGate().Waiting()[class.String()] > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("write gate never queued a %s writer: %#v", class, dbwork.WriteGate().Waiting())
}

func artworkVariantMapping(t *testing.T, db *sql.DB, source string, width int) string {
	t.Helper()
	var digest string
	if err := db.QueryRow(`SELECT digest FROM artwork_variants WHERE source_digest=? AND width=?`, source, width).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}

// A cold wave for one card size must produce one variant. Every caller is
// parked on the decoder gate before any publication can start, and the first
// publisher is held inside its mapping transaction, so the rest are queued
// behind it when the publication lands. Each of them must then re-check the
// published variant and open those bytes instead of decoding the source again;
// any duplicate encode installs and commits, so the publication count is exact.
func TestArtworkVariantStampedeEncodesAndPublishesOncePerSourceWidth(t *testing.T) {
	s, db, target, source := floodedArtworkSource(t)
	ctx := context.Background()
	const width = 1920
	const callers = 4

	slot, err := imagework.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hold, err := dbwork.WriteGate().Acquire(ctx, dbwork.ClassMaintenance)
	if err != nil {
		slot()
		t.Fatal(err)
	}
	var once sync.Once
	releaseWriter := func() { once.Do(hold) }
	defer releaseWriter()
	defer slot()

	published := dbwork.ChangingCommits()
	type result struct {
		name string
		err  error
	}
	results := make([]result, callers)
	started := make(chan struct{}, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started <- struct{}{}
			f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", width, source)
			if err != nil {
				results[i].err = err
				return
			}
			defer f.Close()
			cfg, _, err := image.DecodeConfig(f)
			if err != nil {
				results[i].err = err
				return
			}
			if cfg.Width != width {
				results[i].err = errors.New("served variant is not the requested width")
				return
			}
			results[i].name = filepath.Base(f.Name())
		}(i)
	}
	for i := 0; i < callers; i++ {
		<-started
	}
	// Let the wave through: one caller wins the decoder slot, and the writer
	// gate catches it inside the mapping transaction that publishes the variant.
	slot()
	waitForWriterWaiter(t, dbwork.ClassBackgroundMedia)
	releaseWriter()
	wg.Wait()

	mapped := artworkVariantMapping(t, db, source, width)
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("caller %d failed: %v", i, r.err)
		}
		if r.name != mapped+".img" {
			t.Fatalf("caller %d opened %q, want the published variant %q", i, r.name, mapped+".img")
		}
	}
	if got := dbwork.ChangingCommits() - published; got != 1 {
		t.Fatalf("cold wave committed %d mapping publications, want 1", got)
	}
	if _, err = os.Stat(filepath.Join(s.cacheRoot, mapped+".img")); err != nil {
		t.Fatalf("published variant file is missing: %v", err)
	}
	var mappings, objects int
	if err = db.QueryRow(`SELECT COUNT(*) FROM artwork_variants WHERE source_digest=?`, source).Scan(&mappings); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM artwork_objects WHERE digest=?`, mapped).Scan(&objects); err != nil {
		t.Fatal(err)
	}
	if mappings != 1 || objects != 1 {
		t.Fatalf("published %d mappings and %d objects, want 1 and 1", mappings, objects)
	}
}

// The re-check must not report a mapped variant whose bytes are gone: a deleted
// cache file still self-repairs through the same generate path.
func TestArtworkVariantRecheckRepairsMissingPublishedFile(t *testing.T) {
	s, db, target, source := floodedArtworkSource(t)
	ctx := context.Background()
	const width = 1920

	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", width, source)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	mapped := artworkVariantMapping(t, db, source, width)
	path := filepath.Join(s.cacheRoot, mapped+".img")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	f, _, err = s.EntityArtworkVariant(ctx, target, "poster", "", width, source)
	if err != nil {
		t.Fatalf("missing cached variant was not repaired: %v", err)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != width {
		t.Fatalf("repaired variant is not the requested width: %#v %v", cfg, err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("repaired variant file is missing: %v", err)
	}
	if again := artworkVariantMapping(t, db, source, width); again != mapped {
		t.Fatalf("repair changed the mapping: %q -> %q", mapped, again)
	}
}

// A request cancelled while it holds the decoder slot and waits to publish must
// return its context error and release the slot, so the next request still runs.
func TestArtworkVariantCancelledPublicationLeavesDecoderGateFree(t *testing.T) {
	s, _, target, source := floodedArtworkSource(t)
	ctx := context.Background()
	const width = 1920

	hold, err := dbwork.WriteGate().Acquire(ctx, dbwork.ClassMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	releaseWriter := func() { once.Do(hold) }
	defer releaseWriter()

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		f, _, err := s.EntityArtworkVariant(reqCtx, target, "poster", "", width, source)
		if f != nil {
			f.Close()
		}
		done <- err
	}()
	waitForWriterWaiter(t, dbwork.ClassBackgroundMedia)
	cancel()
	select {
	case err = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("cancelled publication never returned")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publication returned %v, want context.Canceled", err)
	}
	// A slot leaked by the cancelled request would block this until the probe
	// deadline instead of failing for a reason worth reporting.
	probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer probeCancel()
	probe, err := imagework.Acquire(probeCtx)
	if err != nil {
		t.Fatalf("decoder gate was not released after cancellation: %v", err)
	}
	probe()
	releaseWriter()

	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", width, source)
	if err != nil {
		t.Fatalf("artwork did not recover after cancellation: %v", err)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != width {
		t.Fatalf("recovered variant is not the requested width: %#v %v", cfg, err)
	}
}
