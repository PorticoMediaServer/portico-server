package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivateArtifactPublicationIsAtomicAndReconcilesOrphans(t *testing.T) {
	server := newScannerTestServer(t)
	root := filepath.Join(server.cfg.AppDataDir, "subtitles")
	target := filepath.Join(root, "movie_atomic", "subtitle_atomic.vtt")
	body := []byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nComplete\n")
	if err := publishPrivateArtifact(root, target, body); err != nil {
		t.Fatalf("publish private artifact: %v", err)
	}
	actual, err := os.ReadFile(target)
	if err != nil || string(actual) != string(body) {
		t.Fatalf("published artifact = %q, err=%v", actual, err)
	}
	if matches, _ := filepath.Glob(target + ".tmp-*"); len(matches) != 0 {
		t.Fatalf("temporary artifacts remained after publication: %v", matches)
	}
	if _, err := os.Stat(target + ".reserve"); !os.IsNotExist(err) {
		t.Fatalf("reservation remained after publication: %v", err)
	}
	if err := server.reconcileSubtitleArtifacts(context.Background()); err != nil {
		t.Fatalf("reconcile fresh subtitle artifact: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("fresh uncommitted artifact was pruned: %v", err)
	}

	staleTemp := target + ".tmp-stale"
	staleReservation := target + ".reserve"
	if err := os.WriteFile(staleTemp, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staleReservation, 0o700); err != nil {
		t.Fatal(err)
	}
	staleAt := time.Now().UTC().Add(-2 * artifactOrphanGrace)
	for _, path := range []string{target, staleTemp, staleReservation} {
		if err := os.Chtimes(path, staleAt, staleAt); err != nil {
			t.Fatalf("age orphan %s: %v", path, err)
		}
	}
	if err := server.reconcileSubtitleArtifacts(context.Background()); err != nil {
		t.Fatalf("reconcile subtitle artifacts: %v", err)
	}
	for _, path := range []string{target, staleTemp, staleReservation} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unreferenced artifact %s survived reconciliation: %v", path, err)
		}
	}
}

func TestDurableTrashMoveRollsBackAndCrashReconciliationRestoresReferencedFile(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	source := filepath.Join(root, "recover.mp4")
	body := []byte("durable media payload")
	if err := os.WriteFile(source, body, 0o600); err != nil {
		t.Fatal(err)
	}
	move, err := server.stageMediaFileToTrash(source)
	if err != nil {
		t.Fatalf("stage media to trash: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source remained after staged move: %v", err)
	}
	if actual, err := os.ReadFile(move.journal.Target); err != nil || string(actual) != string(body) {
		t.Fatalf("trash target = %q, err=%v", actual, err)
	}
	if move.journal.SHA256 == "" || !move.journal.Published {
		t.Fatalf("trash journal lacks validation/publication evidence: %#v", move.journal)
	}
	if err := move.rollback(); err != nil {
		t.Fatalf("rollback trash move: %v", err)
	}
	if actual, err := os.ReadFile(source); err != nil || string(actual) != string(body) {
		t.Fatalf("restored source = %q, err=%v", actual, err)
	}

	move, err = server.stageMediaFileToTrash(source)
	if err != nil {
		t.Fatalf("restage media to trash: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('trash_recovery_library', 'Recovery', 'movie', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert recovery library: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at) VALUES ('trash_recovery_media', 'trash_recovery_library', 'movie', 'Recovery', 'Recovery', ?, CURRENT_TIMESTAMP)`, "file://"+source); err != nil {
		t.Fatalf("insert recovery media: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_files (id, media_id, library_id, path, available, first_seen_at, last_seen_at) VALUES ('trash_recovery_file', 'trash_recovery_media', 'trash_recovery_library', ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, source); err != nil {
		t.Fatalf("insert live media reference: %v", err)
	}
	if err := server.reconcileTrashMoves(context.Background()); err != nil {
		t.Fatalf("reconcile interrupted trash move: %v", err)
	}
	if actual, err := os.ReadFile(source); err != nil || string(actual) != string(body) {
		t.Fatalf("reconciled source = %q, err=%v", actual, err)
	}
	if _, err := os.Stat(move.journalPath); !os.IsNotExist(err) {
		t.Fatalf("completed recovery journal remains: %v", err)
	}
}

func TestSuccessfulRescanPrunesRemovedManagedSidecar(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Prune.S01E01.mkv")
	sidecarPath := filepath.Join(root, "Prune.S01E01.en.srt")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "TV", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full"); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	artifacts, err := filepath.Glob(filepath.Join(server.cfg.AppDataDir, "subtitles", "*", "*.vtt"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("managed sidecars = %v, err=%v", artifacts, err)
	}
	if err := os.Remove(sidecarPath); err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full"); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if _, err := os.Stat(artifacts[0]); !os.IsNotExist(err) {
		t.Fatalf("removed sidecar artifact survived successful catalog reconciliation: %v", err)
	}
}
