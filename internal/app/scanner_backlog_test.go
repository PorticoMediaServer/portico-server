package app

import (
	"context"
	"testing"
	"time"
)

func TestScannerBacklogWaitsForActiveMetadataJobWithoutLosingRows(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const mediaID = "movie_scanner_backlog_active_metadata"
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES (?, 'lib_movies', 'movie', 'Backlog Active', 'Backlog Active', ?)`, mediaID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO scanner_backlog (id, library_id, media_id, kind, source_revision, status, created_at, updated_at)
		VALUES ('scanq_active_metadata', 'lib_movies', ?, 'metadata', 'revision-1', 'queued', ?, ?)`, mediaID, now, now); err != nil {
		t.Fatal(err)
	}
	active, err := server.createJobForWithMetadata("metadata_refresh_library", "Existing metadata refresh.", "library", "lib_movies", map[string]string{"limit": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if dispatched, err := server.dispatchScannerBacklog(context.Background()); err != nil || dispatched != 0 {
		t.Fatalf("dispatch with active owner = %d, err=%v", dispatched, err)
	}
	var status string
	if err := server.db.QueryRow(`SELECT status FROM scanner_backlog WHERE id = 'scanq_active_metadata'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("backlog status with active owner = %q, expected queued", status)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status = 'complete', active_key = '', updated_at = ? WHERE id = ?`, now, active.ID); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := server.dispatchScannerBacklog(context.Background()); err != nil || dispatched != 1 {
		t.Fatalf("dispatch after active owner = %d, err=%v", dispatched, err)
	}
	if err := server.db.QueryRow(`SELECT status FROM scanner_backlog WHERE id = 'scanq_active_metadata'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "complete" {
		t.Fatalf("backlog status after transfer = %q, expected complete", status)
	}
}

func TestScannerBacklogRecoveryRequeuesClaimAndDeduplicatesActiveAnalysis(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFprobePath = "true"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const mediaID = "movie_scanner_backlog_claimed_analysis"
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES (?, 'lib_movies', 'movie', 'Claimed Analysis', 'Claimed Analysis', ?)`, mediaID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO scanner_backlog (id, library_id, media_id, kind, source_revision, status, created_at, updated_at)
		VALUES ('scanq_claimed_analysis', 'lib_movies', ?, 'analysis', 'revision-1', 'claimed', ?, ?)`, mediaID, now, now); err != nil {
		t.Fatal(err)
	}
	metadata := representativeFrameAnalysisMetadata()
	metadata["sourceRevision"] = "revision-1"
	if _, err := server.createJobForWithMetadata("media_analyze", "Existing analysis.", "media", mediaID, metadata); err != nil {
		t.Fatal(err)
	}
	if err := server.recoverScannerBacklogClaims(); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := server.dispatchScannerBacklog(context.Background()); err != nil || dispatched != 1 {
		t.Fatalf("dispatch recovered claim = %d, err=%v", dispatched, err)
	}
	var backlogStatus string
	if err := server.db.QueryRow(`SELECT status FROM scanner_backlog WHERE id = 'scanq_claimed_analysis'`).Scan(&backlogStatus); err != nil {
		t.Fatal(err)
	}
	if backlogStatus != "complete" {
		t.Fatalf("recovered backlog status = %q, expected complete", backlogStatus)
	}
	var jobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_id = ?`, mediaID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("analysis jobs = %d, expected active-job deduplication", jobs)
	}
}

func TestScannerBacklogKeepsNewerAnalysisRevisionQueuedUntilActiveJobCompletes(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFprobePath = "true"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const mediaID = "movie_scanner_backlog_newer_analysis"
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES (?, 'lib_movies', 'movie', 'Newer Analysis', 'Newer Analysis', ?)`, mediaID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO scanner_backlog (id, library_id, media_id, kind, source_revision, status, created_at, updated_at)
		VALUES ('scanq_newer_analysis', 'lib_movies', ?, 'analysis', 'revision-2', 'queued', ?, ?)`, mediaID, now, now); err != nil {
		t.Fatal(err)
	}
	metadata := representativeFrameAnalysisMetadata()
	metadata["sourceRevision"] = "revision-1"
	active, err := server.createJobForWithMetadata("media_analyze", "Existing older analysis.", "media", mediaID, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched, err := server.dispatchScannerBacklog(context.Background()); err != nil || dispatched != 0 {
		t.Fatalf("dispatch behind older revision = %d, err=%v", dispatched, err)
	}
	var status string
	if err := server.db.QueryRow(`SELECT status FROM scanner_backlog WHERE id = 'scanq_newer_analysis'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("newer backlog status = %q, expected queued", status)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status = 'complete', active_key = '', updated_at = ? WHERE id = ?`, now, active.ID); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := server.dispatchScannerBacklog(context.Background()); err != nil || dispatched != 1 {
		t.Fatalf("dispatch newer revision after active completion = %d, err=%v", dispatched, err)
	}
	var encoded string
	if err := server.db.QueryRow(`
		SELECT metadata_json FROM jobs
		WHERE type = 'media_analyze' AND resource_id = ? AND status = 'queued'`, mediaID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	if got := decodeJobMetadata(encoded)["sourceRevision"]; got != "revision-2" {
		t.Fatalf("queued analysis sourceRevision = %q, expected revision-2", got)
	}
}
