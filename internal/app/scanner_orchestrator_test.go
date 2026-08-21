package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryScanRequestsCoalesceAndPromoteMode(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	first, err := server.queueLibraryScan(library, "targeted", "watcher", "Watcher scan queued.")
	if err != nil {
		t.Fatalf("queue watcher scan: %v", err)
	}
	second, err := server.queueLibraryScan(library, "force_full", "api", "Force scan queued.")
	if err != nil {
		t.Fatalf("queue force scan: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("coalesced job id = %q, expected %q", second.ID, first.ID)
	}
	stored, err := server.getJob(first.ID)
	if err != nil {
		t.Fatalf("get scan job: %v", err)
	}
	if stored.Metadata["mode"] != "force_full" || stored.Metadata[scanTriggerMetadataKey] != "api" {
		t.Fatalf("promoted metadata = %#v", stored.Metadata)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_id = ? AND status IN ('queued', 'running')`, library.ID).Scan(&count); err != nil {
		t.Fatalf("count active scans: %v", err)
	}
	if count != 1 {
		t.Fatalf("active scan count = %d, expected 1", count)
	}
}

func TestRunningLibraryScanClaimsPromotedContinuationBeforeCompletion(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	job, err := server.queueLibraryScan(library, "reconcile", "schedule", "Scheduled scan queued.")
	if err != nil {
		t.Fatalf("queue scan: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status = 'running', leased_by = ? WHERE id = ?`, server.jobLeaseOwner(job.ID), job.ID); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if _, err := server.queueLibraryScan(library, "force_full", "api", "Force scan queued."); err != nil {
		t.Fatalf("promote running scan: %v", err)
	}
	next, complete, err := server.completeLibraryScanOrContinue(job.ID, "reconcile", "Reconcile complete.")
	if err != nil {
		t.Fatalf("claim continuation: %v", err)
	}
	if complete || next != "force_full" {
		t.Fatalf("continuation = %q complete=%v, expected force_full", next, complete)
	}
	next, complete, err = server.completeLibraryScanOrContinue(job.ID, "force_full", "Force scan complete.")
	if err != nil {
		t.Fatalf("complete force scan: %v", err)
	}
	if !complete || next != "" {
		t.Fatalf("final continuation = %q complete=%v", next, complete)
	}
	stored, err := server.getJob(job.ID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if stored.Status != "complete" || stored.Progress != 100 {
		t.Fatalf("completed job = %+v", stored)
	}
}

func TestForceFullScanBypassesUnchangedDirectoryCheckpoint(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	leaf := filepath.Join(root, "Collection")
	if err := os.MkdirAll(leaf, 0o700); err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leaf, "Movie.mp4"), []byte("media"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	originalWalk := scannerWalkDir
	t.Cleanup(func() { scannerWalkDir = originalWalk })
	visitedFile := false
	scannerWalkDir = func(path string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if entry != nil && !entry.IsDir() && filepath.Base(path) == "Movie.mp4" {
				visitedFile = true
			}
			return fn(path, entry, err)
		})
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full"); err != nil {
		t.Fatalf("force-full scan: %v", err)
	}
	if !visitedFile {
		t.Fatal("force-full scan did not traverse the file beneath an unchanged checkpoint")
	}
}
