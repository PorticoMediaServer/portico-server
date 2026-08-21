package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func scannerOperationsOwner() User {
	return User{ID: "owner", AccountID: "owner", ProfileID: "owner", ProfileIsPrimary: true, Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()}
}

func TestLibraryScanOperationsProjectsOneOperationSourcesAndHistory(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	job, err := server.queueLibraryScan(library, "force_full", "api", "Force scan queued.")
	if err != nil {
		t.Fatalf("queue scan: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO storage_sources (
			id, library_id, configured_path, resolved_path, classification, classification_source,
			health_state, circuit_state, error_class, error_message, latency_ms,
			consecutive_failures, last_progress_at, last_success_at, last_failure_at, created_at, updated_at
		) VALUES ('source-1', ?, '/media', '/media', 'fuse', 'owner', 'degraded', 'open',
			'stale_file_handle', 'stale file handle', 85, 2, ?, '', ?, ?, ?)`, library.ID, now, now, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO library_scan_runs (
			id, library_id, job_id, mode, status, phase, files_indexed, files_unchanged,
			files_skipped, missing_marked, metadata_queued, analysis_queued,
			absence_authoritative, cleanup_allowed, warnings_json, started_at, completed_at, updated_at
		) VALUES ('run-1', ?, '', 'reconcile', 'degraded', 'complete', 7, 3, 1, 0, 4, 2,
			0, 0, '["Missing-item cleanup was held because storage was unavailable."]', ?, ?, ?)`, library.ID, now, now, now); err != nil {
		t.Fatalf("insert scan run: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO library_scan_run_roots (
			run_id, source_id, configured_path, resolved_path, status, error_class,
			error_message, directories_seen, files_seen, started_at, completed_at, last_progress_at
		) VALUES ('run-1', 'source-1', '/media', '/media', 'degraded', 'stale_file_handle',
			'stale file handle', 2, 10, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert scan root: %v", err)
	}

	projection, err := server.libraryScanOperations(t.Context(), library)
	if err != nil {
		t.Fatalf("project scan operations: %v", err)
	}
	if projection.Operation == nil || projection.Operation.JobID != job.ID || projection.Operation.Mode != "force_full" {
		t.Fatalf("operation = %#v", projection.Operation)
	}
	if len(projection.RecentRuns) != 1 || projection.LastRun == nil || projection.LastRun.ID != "run-1" {
		t.Fatalf("runs = %#v last=%#v", projection.RecentRuns, projection.LastRun)
	}
	if projection.LastRun.AbsenceAuthoritative || projection.LastRun.CleanupAllowed || len(projection.LastRun.Warnings) < 2 {
		t.Fatalf("unsafe/degraded run projection = %#v", projection.LastRun)
	}
	if len(projection.Sources) != 1 || projection.Sources[0].Classification != "fuse" || projection.Sources[0].ClassificationSource != "owner" {
		t.Fatalf("sources = %#v", projection.Sources)
	}
	if !projection.Actions.CanCancel || projection.Actions.CanRetry {
		t.Fatalf("actions = %#v", projection.Actions)
	}
}

func TestRemoveMissingRequiresLatestAuthoritativeRunAcknowledgement(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	owner := scannerOperationsOwner()
	request := func(body map[string]any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/libraries/"+library.ID+"/scan", bytes.NewReader(payload))
		recorder := httptest.NewRecorder()
		server.handleLibraryRoute(recorder, req, owner)
		return recorder
	}

	denied := request(map[string]any{"mode": "remove_missing"})
	if denied.Code != http.StatusConflict {
		t.Fatalf("unconfirmed remove status=%d body=%s", denied.Code, denied.Body.String())
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO library_scan_runs (
			id, library_id, mode, status, phase, absence_authoritative, cleanup_allowed,
			warnings_json, started_at, completed_at, updated_at
		) VALUES ('reviewed-run', ?, 'reconcile', 'healthy', 'complete', 1, 1, '[]', ?, ?, ?)`, library.ID, now, now, now); err != nil {
		t.Fatalf("insert reviewed run: %v", err)
	}
	wrong := request(map[string]any{"mode": "remove_missing", "confirmedRunId": "another-run"})
	if wrong.Code != http.StatusConflict {
		t.Fatalf("wrong acknowledgement status=%d body=%s", wrong.Code, wrong.Body.String())
	}
	accepted := request(map[string]any{"mode": "remove_missing", "confirmedRunId": "reviewed-run"})
	if accepted.Code != http.StatusCreated {
		t.Fatalf("confirmed remove status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	job, err := server.getJob(decodeResponse[Job](t, accepted).ID)
	if err != nil || job.Metadata["mode"] != "remove_missing" {
		t.Fatalf("remove job=%#v err=%v", job, err)
	}
}

func TestStorageClassificationOverrideIsOwnerOnlyAndPersistent(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO storage_sources (id, library_id, configured_path, classification, classification_source,
			health_state, circuit_state, created_at, updated_at)
		VALUES ('source-override', ?, '/media', 'local', 'detected', 'healthy', 'closed', ?, ?)`, library.ID, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	body := bytes.NewBufferString(`{"classification":"network"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/libraries/"+library.ID+"/storage-sources/source-override", body)
	denied := httptest.NewRecorder()
	server.handleLibraryRoute(denied, req, User{Role: "user", AuthProvider: "local"})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-owner status=%d body=%s", denied.Code, denied.Body.String())
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/libraries/"+library.ID+"/storage-sources/source-override", bytes.NewBufferString(`{"classification":"network"}`))
	accepted := httptest.NewRecorder()
	server.handleLibraryRoute(accepted, req, scannerOperationsOwner())
	if accepted.Code != http.StatusOK {
		t.Fatalf("owner status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	var classification, source string
	if err := server.db.QueryRow(`SELECT classification, classification_source FROM storage_sources WHERE id = 'source-override'`).Scan(&classification, &source); err != nil {
		t.Fatalf("read override: %v", err)
	}
	if classification != "network" || source != "owner" {
		t.Fatalf("override = %q/%q", classification, source)
	}
}

func TestLibraryScanReviewExposesBoundedMissingAndIdentityEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{"/media"}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO storage_sources (id, library_id, configured_path, resolved_path, classification,
			classification_source, health_state, circuit_state, created_at, updated_at)
		VALUES ('review-source', ?, '/media', '/media', 'network', 'owner', 'offline', 'open', ?, ?)`, library.ID, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES ('missing-media', ?, 'movie', 'Missing Movie', 'Missing Movie', ?)`, library.ID, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, available, missing_since, first_seen_at, last_seen_at)
		VALUES ('missing-file', 'missing-media', ?, '/media/Missing Movie.mkv', 0, ?, ?, ?)`, library.ID, now, now, now); err != nil {
		t.Fatalf("insert missing file: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO identity_reconciliation_reviews (
			id, domain, library_or_source_id, subject_id, candidate_locator, evidence_kind,
			evidence_value, candidate_ids_json, status, created_at)
		VALUES ('review-open', 'media', ?, 'missing-media', '/media/ambiguous.mkv',
			'content_fingerprint_ambiguous', 'fingerprint', '["candidate-a","candidate-b"]', 'open', ?)`, library.ID, now); err != nil {
		t.Fatalf("insert identity review: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO library_scan_runs (
			id, library_id, mode, status, phase, absence_authoritative, cleanup_allowed,
			warnings_json, started_at, completed_at, updated_at)
		VALUES ('confirmation-run', ?, 'reconcile', 'healthy', 'complete', 1, 1, '[]', ?, ?, ?)`, library.ID, now, now, now); err != nil {
		t.Fatalf("insert confirmation run: %v", err)
	}

	review, err := server.libraryScanReview(t.Context(), library.ID, 1, "", "")
	if err != nil {
		t.Fatalf("load scan review: %v", err)
	}
	if review.ConfirmationRunID != "confirmation-run" || review.CanConfirmRemoval {
		t.Fatalf("confirmation = %#v", review)
	}
	if review.MissingTotal != 1 || len(review.MissingItems) != 1 {
		t.Fatalf("missing evidence = %#v", review.MissingItems)
	}
	missing := review.MissingItems[0]
	if missing.MediaID != "missing-media" || missing.Title != "Missing Movie" || missing.Path != "/media/Missing Movie.mkv" || missing.SourceID != "review-source" || missing.SourceHealth != "offline" {
		t.Fatalf("missing item = %#v", missing)
	}
	if review.OpenIdentityTotal != 1 || len(review.IdentityReviews) != 1 || review.IdentityReviews[0].ID != "review-open" {
		t.Fatalf("identity evidence = %#v", review.IdentityReviews)
	}
	operations, err := server.libraryScanOperations(t.Context(), library)
	if err != nil {
		t.Fatalf("load operations: %v", err)
	}
	if operations.Actions.CanRemoveMissing {
		t.Fatalf("open identity review did not block remove-missing: %#v", operations.Actions)
	}
	if _, err := server.db.Exec(`UPDATE identity_reconciliation_reviews SET status = 'ignored' WHERE id = 'review-open'`); err != nil {
		t.Fatalf("resolve identity review: %v", err)
	}
	review, err = server.libraryScanReview(t.Context(), library.ID, 1, "", "")
	if err != nil || !review.CanConfirmRemoval {
		t.Fatalf("resolved review confirmation = %#v err=%v", review, err)
	}
}

func TestLibraryScanCancelAndRetryStayScopedToLibraryOperation(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	queued, err := server.queueLibraryScan(library, "quick", "api", "Quick scan queued.")
	if err != nil {
		t.Fatalf("queue scan: %v", err)
	}
	cancelled, err := server.cancelLibraryScan(t.Context(), library.ID)
	if err != nil {
		t.Fatalf("cancel scan: %v", err)
	}
	if cancelled.ID != queued.ID || cancelled.Status != "cancelled" {
		t.Fatalf("cancelled job = %#v", cancelled)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO library_scan_runs (id, library_id, job_id, mode, status, phase,
			warnings_json, started_at, completed_at, updated_at)
		VALUES ('retry-run', ?, ?, 'quick', 'failed', 'complete', '[]', ?, ?, ?)`, library.ID, queued.ID, now, now, now); err != nil {
		t.Fatalf("insert failed run: %v", err)
	}
	retry, err := server.retryLibraryScan(t.Context(), library, "retry-run")
	if err != nil {
		t.Fatalf("retry scan: %v", err)
	}
	if retry.ID == queued.ID || retry.ResourceID != library.ID || retry.Metadata["mode"] != "quick" || retry.Metadata[scanTriggerMetadataKey] != "retry" {
		t.Fatalf("retry job = %#v", retry)
	}
}

func decodeResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	return value
}
