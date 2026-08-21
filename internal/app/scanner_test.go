package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestLibraryScannerIndexesFilesInsideConfiguredRoots(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Moonrise.S01E02.Signal.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Scanned Shows", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 1 {
		t.Fatalf("indexed files = %d, expected 1", result.FilesIndexed)
	}

	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'episode'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query scanned media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("scanned episode count = %d, expected 1", len(items))
	}
	if items[0].SourceURL == "" || items[0].SeasonNumber != 1 || items[0].EpisodeNumber != 2 {
		t.Fatalf("scanned episode metadata was not populated: %+v", items[0])
	}
}

func TestLibraryScannerDegradedRootCannotProveAbsence(t *testing.T) {
	server := newScannerTestServer(t)
	healthyRoot := t.TempDir()
	vanishingParent := t.TempDir()
	vanishingRoot := filepath.Join(vanishingParent, "remote")
	if err := os.Mkdir(vanishingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(healthyRoot, "Healthy.mp4"), []byte("healthy"), 0o600); err != nil {
		t.Fatal(err)
	}
	vanishingPath := filepath.Join(vanishingRoot, "Temporarily Offline.mp4")
	if err := os.WriteFile(vanishingPath, []byte("remote"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Mixed Storage", Type: "movie", Paths: []string{healthyRoot, vanishingRoot}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := server.performLibraryScan(library, "")
	if err != nil || !initial.AbsenceAuthoritative {
		t.Fatalf("initial scan result=%+v err=%v", initial, err)
	}
	if err := os.RemoveAll(vanishingRoot); err != nil {
		t.Fatal(err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("degraded mixed-root scan: %v", err)
	}
	if result.DegradedRoots != 1 || result.AbsenceAuthoritative || result.CleanupAllowed || result.MissingMarked != 0 {
		t.Fatalf("unsafe degraded result: %+v", result)
	}
	var available int
	if err := server.db.QueryRow(`
		SELECT mf.available FROM media_files mf
		JOIN media_items mi ON mi.id = mf.media_id
		WHERE mi.library_id = ? AND mi.title = 'Temporarily Offline'`, library.ID).Scan(&available); err != nil {
		t.Fatalf("load temporarily unavailable file: %v", err)
	}
	if available != 1 {
		t.Fatal("temporarily unavailable media was marked missing")
	}
	var runStatus string
	var authoritative, cleanup int
	if err := server.db.QueryRow(`
		SELECT status, absence_authoritative, cleanup_allowed
		FROM library_scan_runs WHERE library_id = ? ORDER BY started_at DESC, id DESC LIMIT 1`, library.ID).Scan(&runStatus, &authoritative, &cleanup); err != nil {
		t.Fatalf("load scan run evidence: %v", err)
	}
	if runStatus != "degraded" || authoritative != 0 || cleanup != 0 {
		t.Fatalf("scan run status=%q authoritative=%d cleanup=%d", runStatus, authoritative, cleanup)
	}
}

func TestLibraryScannerMidTraversalErrorCannotMarkMediaMissing(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Stale Handle.mp4")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Faulty Storage", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	originalWalk := scannerWalkDir
	scannerWalkDir = func(path string, fn fs.WalkDirFunc) error {
		if err := fn(path, nil, errors.New("stale file handle")); err != nil && !errors.Is(err, filepath.SkipDir) {
			return err
		}
		return nil
	}
	t.Cleanup(func() { scannerWalkDir = originalWalk })
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("degraded scan: %v", err)
	}
	if result.DegradedRoots != 1 || result.AbsenceAuthoritative || result.MissingMarked != 0 {
		t.Fatalf("unsafe traversal result: %+v", result)
	}
	var available int
	if err := server.db.QueryRow(`
		SELECT mf.available FROM media_files mf
		JOIN media_items mi ON mi.id = mf.media_id
		WHERE mi.library_id = ? AND mi.title = 'Stale Handle'`, library.ID).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != 1 {
		t.Fatal("mid-traversal error marked media missing")
	}
}

func TestBlockedStorageSourceDoesNotHoldOtherLibraryCapacity(t *testing.T) {
	server := newScannerTestServer(t)
	blockedRoot := t.TempDir()
	healthyRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(blockedRoot, "Blocked.mp4"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(healthyRoot, "Healthy.mp4"), []byte("healthy"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockedLibrary, err := server.createLibrary(CreateLibraryRequest{Name: "Blocked", Type: "movie", Paths: []string{blockedRoot}})
	if err != nil {
		t.Fatal(err)
	}
	healthyLibrary, err := server.createLibrary(CreateLibraryRequest{Name: "Healthy", Type: "movie", Paths: []string{healthyRoot}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScan(blockedLibrary, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScan(healthyLibrary, ""); err != nil {
		t.Fatal(err)
	}
	blockedResolved, err := filepath.EvalSymlinks(blockedRoot)
	if err != nil {
		t.Fatal(err)
	}
	originalWalk := scannerWalkDir
	originalTimeout := scannerRootIdleTimeout
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	scannerRootIdleTimeout = func(storageSourceClass) time.Duration { return 50 * time.Millisecond }
	scannerWalkDir = func(path string, fn fs.WalkDirFunc) error {
		if filepath.Clean(path) == filepath.Clean(blockedResolved) {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}
		return originalWalk(path, fn)
	}
	t.Cleanup(func() {
		close(release)
		scannerWalkDir = originalWalk
		scannerRootIdleTimeout = originalTimeout
	})
	blockedDone := make(chan libraryScanResult, 1)
	go func() {
		result, _ := server.performLibraryScan(blockedLibrary, "")
		blockedDone <- result
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("blocked traversal did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	healthyResult, err := server.performLibraryScanWithContext(ctx, healthyLibrary, "")
	if err != nil || healthyResult.DegradedRoots != 0 {
		t.Fatalf("healthy library was blocked: result=%+v err=%v", healthyResult, err)
	}
	select {
	case result := <-blockedDone:
		if result.DegradedRoots != 1 || result.AbsenceAuthoritative {
			t.Fatalf("blocked source result=%+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked source did not release scanner capacity after idle timeout")
	}
}

func TestScannerMetadataBacklogDrainsWithoutCandidateTruncation(t *testing.T) {
	server := newScannerTestServer(t)
	const candidateCount = 2001
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < candidateCount; index++ {
		mediaID := fmt.Sprintf("movie_scanner_backlog_metadata_%04d", index)
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
			VALUES (?, 'lib_movies', 'movie', ?, ?, ?)`, mediaID, mediaID, mediaID, now); err != nil {
			tx.Rollback()
			t.Fatalf("insert media %d: %v", index, err)
		}
		if queued, err := enqueueScannerBacklogTx(tx, "lib_movies", mediaID, scannerBacklogMetadata, "revision-1", now); err != nil || queued != 1 {
			tx.Rollback()
			t.Fatalf("enqueue metadata %d: queued=%d err=%v", index, queued, err)
		}
		if queued, err := enqueueScannerBacklogTx(tx, "lib_movies", mediaID, scannerBacklogMetadata, "revision-1", now); err != nil || queued != 0 {
			tx.Rollback()
			t.Fatalf("deduplicate metadata %d: queued=%d err=%v", index, queued, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	transferred := map[string]bool{}
	for iteration := 0; iteration < candidateCount; iteration++ {
		dispatched, err := server.dispatchScannerBacklog(context.Background())
		if err != nil {
			t.Fatalf("dispatch iteration %d: %v", iteration, err)
		}
		if dispatched == 0 {
			break
		}
		var jobID, metadataJSON string
		if err := server.db.QueryRow(`
			SELECT id, metadata_json FROM jobs
			WHERE type = 'metadata_refresh_library' AND resource_id = 'lib_movies' AND status = 'queued'
			ORDER BY created_at, id LIMIT 1`).Scan(&jobID, &metadataJSON); err != nil {
			t.Fatalf("load dispatched metadata job: %v", err)
		}
		metadata := decodeJobMetadata(metadataJSON)
		if len(metadata["mediaIds"]) > jobMaxMetadataValueBytes {
			t.Fatalf("mediaIds metadata exceeded byte budget: %d", len(metadata["mediaIds"]))
		}
		ids := jobMetadataMediaIDs(metadata["mediaIds"], scannerBacklogMetadataMaxIDs)
		if len(ids) == 0 {
			t.Fatalf("metadata job contained no exact ids: %#v", metadata)
		}
		for _, mediaID := range ids {
			if transferred[mediaID] {
				t.Fatalf("media id transferred twice: %s", mediaID)
			}
			transferred[mediaID] = true
		}
		if _, err := server.db.Exec(`UPDATE jobs SET status = 'complete', active_key = '', updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), jobID); err != nil {
			t.Fatalf("complete dispatched metadata job: %v", err)
		}
	}
	if len(transferred) != candidateCount {
		t.Fatalf("transferred metadata ids = %d, expected %d", len(transferred), candidateCount)
	}
	var queued, complete int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM scanner_backlog WHERE kind = 'metadata' AND status = 'queued'`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM scanner_backlog WHERE kind = 'metadata' AND status = 'complete'`).Scan(&complete); err != nil {
		t.Fatal(err)
	}
	if queued != 0 || complete != candidateCount {
		t.Fatalf("metadata backlog queued/complete = %d/%d, expected 0/%d", queued, complete, candidateCount)
	}
}

func TestScannerAnalysisBacklogDrainsWithoutCandidateTruncation(t *testing.T) {
	server := newScannerTestServer(t)
	server.cfg.FFprobePath = "true"
	const candidateCount = 1001
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < candidateCount; index++ {
		mediaID := fmt.Sprintf("movie_scanner_backlog_analysis_%04d", index)
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
			VALUES (?, 'lib_movies', 'movie', ?, ?, ?)`, mediaID, mediaID, mediaID, now); err != nil {
			tx.Rollback()
			t.Fatalf("insert media %d: %v", index, err)
		}
		if queued, err := enqueueScannerBacklogTx(tx, "lib_movies", mediaID, scannerBacklogAnalysis, "revision-1", now); err != nil || queued != 1 {
			tx.Rollback()
			t.Fatalf("enqueue analysis %d: queued=%d err=%v", index, queued, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 20; iteration++ {
		dispatched, err := server.dispatchScannerBacklog(context.Background())
		if err != nil {
			t.Fatalf("dispatch iteration %d: %v", iteration, err)
		}
		if dispatched == 0 {
			break
		}
	}
	var jobs, queued, complete int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_id LIKE 'movie_scanner_backlog_analysis_%'`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM scanner_backlog WHERE kind = 'analysis' AND status = 'queued'`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM scanner_backlog WHERE kind = 'analysis' AND status = 'complete'`).Scan(&complete); err != nil {
		t.Fatal(err)
	}
	if jobs != candidateCount || queued != 0 || complete != candidateCount {
		t.Fatalf("analysis jobs/queued/complete = %d/%d/%d, expected %d/0/%d", jobs, queued, complete, candidateCount, candidateCount)
	}
}

func TestLibraryScannerDirectoryCheckpointAvoidsUnchangedRowWritesAndDetectsSidecars(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Checkpoint Movie.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	const sentinel = "2001-02-03T04:05:06Z"
	if _, err := server.db.Exec(`UPDATE media_files SET last_seen_at = ? WHERE library_id = ?`, sentinel, library.ID); err != nil {
		t.Fatalf("set last-seen sentinel: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("unchanged scan: %v", err)
	}
	if result.FilesUnchanged != 1 || result.FilesIndexed != 1 {
		t.Fatalf("unchanged/indexed = %d/%d, expected 1/1", result.FilesUnchanged, result.FilesIndexed)
	}
	var lastSeen string
	if err := server.db.QueryRow(`SELECT last_seen_at FROM media_files WHERE library_id = ?`, library.ID).Scan(&lastSeen); err != nil {
		t.Fatalf("read last seen: %v", err)
	}
	if lastSeen != sentinel {
		t.Fatalf("unchanged checkpoint rewrote media row: last_seen_at = %q", lastSeen)
	}
	forceResult, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full")
	if err != nil {
		t.Fatalf("force-full scan: %v", err)
	}
	if forceResult.FilesIndexed != 1 {
		t.Fatalf("force-full indexed = %d, expected 1", forceResult.FilesIndexed)
	}
	if err := server.db.QueryRow(`SELECT last_seen_at FROM media_files WHERE library_id = ?`, library.ID).Scan(&lastSeen); err != nil {
		t.Fatalf("read force-full last seen: %v", err)
	}
	if lastSeen == sentinel {
		t.Fatal("force-full scan incorrectly reused the unchanged directory checkpoint")
	}
	if err := os.WriteFile(filepath.Join(root, "Checkpoint Movie.nfo"), []byte(`<movie><title>Checkpoint Override</title></movie>`), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	result, err = server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("sidecar scan: %v", err)
	}
	if result.FilesUnchanged != 0 {
		t.Fatalf("sidecar change incorrectly skipped %d file(s)", result.FilesUnchanged)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{library.ID})
	if err != nil || len(items) != 1 || items[0].Title != "Checkpoint Override" {
		t.Fatalf("sidecar metadata was not refreshed: items=%#v err=%v", items, err)
	}
}

func TestScannerDirectoryCheckpointDoesNotInvalidateSiblingForChildMutation(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	if err := os.Mkdir(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(right, 0o700); err != nil {
		t.Fatal(err)
	}
	leftMedia := filepath.Join(left, "Left.mp4")
	rightMedia := filepath.Join(right, "Right.mp4")
	if err := os.WriteFile(leftMedia, []byte("left"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rightMedia, []byte("right"), 0o600); err != nil {
		t.Fatal(err)
	}
	initialCache := map[string]string{}
	leftBefore, _ := scannerDirectoryCheckpointState(left, "movie", initialCache)
	rightBefore, _ := scannerDirectoryCheckpointState(right, "movie", initialCache)
	if err := os.WriteFile(leftMedia, []byte("left changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterCache := map[string]string{}
	leftAfter, _ := scannerDirectoryCheckpointState(left, "movie", afterCache)
	rightAfter, _ := scannerDirectoryCheckpointState(right, "movie", afterCache)
	if leftBefore == leftAfter {
		t.Fatal("changed directory retained its checkpoint signature")
	}
	if rightBefore != rightAfter {
		t.Fatal("child mutation invalidated an unchanged sibling checkpoint")
	}
}

func TestLibraryScannerPreservesStableIdentityAcrossRename(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	originalPath := filepath.Join(root, "Original Name.mp4")
	renamedPath := filepath.Join(root, "Renamed Film.mp4")
	if err := os.WriteFile(originalPath, []byte("durable media identity payload"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{library.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("initial media = %#v, err = %v", items, err)
	}
	originalID := items[0].ID
	if strings.HasPrefix(originalID, "scan_") {
		t.Fatalf("new media ID %q is derived from scanner metadata", originalID)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	userID := "user_identity_test"
	if _, err := server.db.Exec(`
		INSERT INTO users (id, email, display_name, role, permissions_json, created_at, updated_at)
		VALUES (?, 'identity@example.test', 'Identity Test', 'user', '{}', ?, ?)`, userID, now, now); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, favorite, progress_seconds, updated_at)
		VALUES (?, ?, ?, 1, 42, ?)
	`, userID, userID, originalID, now); err != nil {
		t.Fatalf("insert user state: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playlists (id, user_id, profile_id, title, created_at, updated_at)
		VALUES ('playlist_identity', ?, ?, 'Keep Me', ?, ?)`, userID, userID, now, now); err != nil {
		t.Fatalf("insert playlist: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playlist_items (playlist_id, media_id, added_at)
		VALUES ('playlist_identity', ?, ?)`, originalID, now); err != nil {
		t.Fatalf("insert playlist membership: %v", err)
	}
	if err := os.Rename(originalPath, renamedPath); err != nil {
		t.Fatalf("rename media: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan after rename: %v", err)
	}
	items, err = server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{library.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("renamed media = %#v, err = %v", items, err)
	}
	if items[0].ID != originalID {
		t.Fatalf("media ID changed across rename: %q -> %q", originalID, items[0].ID)
	}
	var path string
	if err := server.db.QueryRow(`SELECT path FROM media_files WHERE media_id = ?`, originalID).Scan(&path); err != nil {
		t.Fatalf("load reconciled locator: %v", err)
	}
	expectedPath, err := filepath.EvalSymlinks(renamedPath)
	if err != nil {
		t.Fatalf("resolve renamed path: %v", err)
	}
	if path != expectedPath {
		t.Fatalf("locator path = %q, expected %q", path, expectedPath)
	}
	var favorite, progress, memberships int
	if err := server.db.QueryRow(`SELECT favorite, progress_seconds FROM user_media_state WHERE user_id = ? AND media_id = ?`, userID, originalID).Scan(&favorite, &progress); err != nil {
		t.Fatalf("load preserved user state: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playlist_items WHERE playlist_id = 'playlist_identity' AND media_id = ?`, originalID).Scan(&memberships); err != nil {
		t.Fatalf("load preserved playlist membership: %v", err)
	}
	if favorite != 1 || progress != 42 || memberships != 1 {
		t.Fatalf("state after rename favorite=%d progress=%d memberships=%d", favorite, progress, memberships)
	}
}

func TestLibraryScannerCreatesReviewForAmbiguousFingerprintMove(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	payload := []byte("identical duplicate payload")
	for _, name := range []string{"Duplicate A.mp4", "Duplicate B.mp4"} {
		if err := os.WriteFile(filepath.Join(root, name), payload, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Duplicates", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	if err := os.Rename(filepath.Join(root, "Duplicate A.mp4"), filepath.Join(root, "Moved Duplicate.mp4")); err != nil {
		t.Fatalf("rename duplicate: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "Duplicate B.mp4")); err != nil {
		t.Fatalf("remove second duplicate: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan ambiguous move: %v", err)
	}
	var reviews int
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM identity_reconciliation_reviews
		WHERE domain = 'media' AND library_or_source_id = ? AND status = 'open'`, library.ID).Scan(&reviews); err != nil {
		t.Fatalf("count reconciliation reviews: %v", err)
	}
	if reviews != 1 {
		t.Fatalf("open reconciliation reviews = %d, expected 1", reviews)
	}
}

func TestListLibrariesIncludesScanSummary(t *testing.T) {
	server := newScannerTestServer(t)
	movies, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create movies library: %v", err)
	}
	shows, err := server.createLibrary(CreateLibraryRequest{Name: "Shows", Type: "show", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create shows library: %v", err)
	}

	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES
			('job_movies_completed', 'library_scan', 'completed', 100, 'Movies scan completed.', 'library', ?, '{}', '2026-06-19T12:00:00Z', '2026-06-19T12:02:00Z'),
			('job_movies_queued', 'library_scan', 'queued', 0, 'Scan queued for Movies.', 'library', ?, '{}', '2026-06-19T11:00:00Z', '2026-06-19T11:00:00Z'),
			('job_shows_failed', 'library_scan', 'failed', 100, 'Shows scan failed.', 'library', ?, '{}', '2026-06-19T10:00:00Z', '2026-06-19T10:05:00Z')
	`, movies.ID, movies.ID, shows.ID); err != nil {
		t.Fatalf("insert scan jobs: %v", err)
	}

	libraries, err := server.listLibrariesContext(context.Background())
	if err != nil {
		t.Fatalf("list libraries: %v", err)
	}
	byID := map[string]Library{}
	for _, library := range libraries {
		byID[library.ID] = library
	}
	movieSummary := byID[movies.ID].ScanSummary
	if movieSummary == nil {
		t.Fatalf("movies scan summary was nil")
	}
	if movieSummary.JobID != "job_movies_queued" || movieSummary.Status != "queued" || movieSummary.Message != "Scan queued for Movies." {
		t.Fatalf("movies scan summary = %+v", movieSummary)
	}
	showSummary := byID[shows.ID].ScanSummary
	if showSummary == nil {
		t.Fatalf("shows scan summary was nil")
	}
	if showSummary.JobID != "job_shows_failed" || showSummary.Status != "failed" || showSummary.UpdatedAt != "2026-06-19T10:05:00Z" {
		t.Fatalf("shows scan summary = %+v", showSummary)
	}
}

func TestLibraryScannerHonorsCancelledContext(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cancelled.Movie.2024.mp4"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Cancelled Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.performLibraryScanWithContext(ctx, library, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan cancelled error = %v, expected context.Canceled", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ?`, []any{library.ID})
	if err != nil {
		t.Fatalf("query scanned media: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("cancelled scan indexed media: %#v", items)
	}
}

func TestLibraryScannerPersistsCompletedDirectoryProgressAcrossRetry(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	for index := range 40 {
		directory := filepath.Join(root, fmt.Sprintf("Artist %02d", index), "Album")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create music directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(directory, "01 Track.mp3"), []byte("track"), 0o600); err != nil {
			t.Fatalf("write music file: %v", err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Resumable Music", Type: "music", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	originalWalk := scannerWalkDir
	t.Cleanup(func() { scannerWalkDir = originalWalk })
	scannerWalkDir = func(path string, callback fs.WalkDirFunc) error {
		directories := 0
		return filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
			if walkErr == nil && entry != nil && entry.IsDir() {
				directories++
				if directories > 72 {
					return context.Canceled
				}
			}
			return callback(candidate, entry, walkErr)
		})
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); !errors.Is(err, context.Canceled) {
		t.Fatalf("partial scan error = %v, expected cancellation", err)
	}
	var checkpoints int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM library_scan_continuation_directories WHERE library_id = ?`, library.ID).Scan(&checkpoints); err != nil {
		t.Fatalf("count durable checkpoints: %v", err)
	}
	if checkpoints < scannerCheckpointCommitBatch {
		t.Fatalf("durable checkpoints = %d, expected at least %d", checkpoints, scannerCheckpointCommitBatch)
	}

	scannerWalkDir = originalWalk
	result, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile")
	if err != nil {
		t.Fatalf("resume scan: %v", err)
	}
	if result.FilesUnchanged < scannerCheckpointCommitBatch {
		t.Fatalf("resumed unchanged files = %d, expected completed directory work to be reused", result.FilesUnchanged)
	}
}

func TestLibraryScannerTentativeContinuationCannotHideDeletion(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	paths := make([]string, 0, 40)
	for index := range 40 {
		directory := filepath.Join(root, fmt.Sprintf("Movie %02d", index))
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, fmt.Sprintf("Movie %02d.mkv", index))
		if err := os.WriteFile(path, []byte("movie"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Continuation Safety", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	deletedPath := paths[0]
	deletedRealPath, err := filepath.EvalSymlinks(deletedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(deletedPath); err != nil {
		t.Fatal(err)
	}

	originalWalk := scannerWalkDir
	t.Cleanup(func() { scannerWalkDir = originalWalk })
	scannerWalkDir = func(path string, callback fs.WalkDirFunc) error {
		directories := 0
		return filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
			if walkErr == nil && entry != nil && entry.IsDir() {
				directories++
				if directories > 36 {
					return context.Canceled
				}
			}
			return callback(candidate, entry, walkErr)
		})
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); !errors.Is(err, context.Canceled) {
		t.Fatalf("partial deletion scan error = %v", err)
	}
	var available int
	if err := server.db.QueryRow(`SELECT available FROM media_files WHERE path = ?`, deletedRealPath).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != 1 {
		t.Fatal("non-authoritative partial scan marked deletion missing")
	}
	var tentative int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM library_scan_continuation_directories WHERE library_id = ? AND path = ?`, library.ID, filepath.Dir(deletedRealPath)).Scan(&tentative); err != nil {
		t.Fatal(err)
	}
	if tentative != 1 {
		t.Fatal("changed directory did not retain tentative retry evidence")
	}

	scannerWalkDir = originalWalk
	result, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile")
	if err != nil {
		t.Fatalf("authoritative retry: %v", err)
	}
	if !result.AbsenceAuthoritative || result.MissingMarked != 1 {
		t.Fatalf("retry result = %#v", result)
	}
	if err := server.db.QueryRow(`SELECT available FROM media_files WHERE path = ?`, deletedRealPath).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != 0 {
		t.Fatal("healthy retry skipped deletion hidden behind tentative progress")
	}
}

func TestLibraryScannerPerFileStorageFailureCannotAuthorizeAbsence(t *testing.T) {
	for _, fault := range []string{"resolve", "stat"} {
		t.Run(fault, func(t *testing.T) {
			server := newScannerTestServer(t)
			root := t.TempDir()
			path := filepath.Join(root, "Still Here.mkv")
			if err := os.WriteFile(path, []byte("movie"), 0o600); err != nil {
				t.Fatal(err)
			}
			realPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				t.Fatal(err)
			}
			library, err := server.createLibrary(CreateLibraryRequest{Name: "Fault Safety", Type: "movie", Paths: []string{root}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
				t.Fatal(err)
			}

			originalResolve := scannerResolveMediaPath
			originalStat := scannerStatMediaPath
			t.Cleanup(func() {
				scannerResolveMediaPath = originalResolve
				scannerStatMediaPath = originalStat
			})
			if fault == "resolve" {
				scannerResolveMediaPath = func(*Server, context.Context, storageIORequest, string) (string, error) {
					return "", os.ErrPermission
				}
			} else {
				scannerStatMediaPath = func(*Server, context.Context, storageIORequest, string) (os.FileInfo, error) {
					return nil, os.ErrPermission
				}
			}
			result, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full")
			if err != nil {
				t.Fatalf("degraded scan returned fatal error: %v", err)
			}
			if result.AbsenceAuthoritative || result.CleanupAllowed || result.DegradedRoots == 0 {
				t.Fatalf("fault result = %#v", result)
			}
			var available int
			if err := server.db.QueryRow(`SELECT available FROM media_files WHERE path = ?`, realPath).Scan(&available); err != nil {
				t.Fatal(err)
			}
			if available != 1 {
				t.Fatal("per-file storage fault marked existing media unavailable")
			}
		})
	}
}

func TestLibraryScannerIncompleteSidecarReadPreservesCatalogRows(t *testing.T) {
	t.Run("subtitle", func(t *testing.T) {
		server := newScannerTestServer(t)
		root := t.TempDir()
		mediaPath := filepath.Join(root, "Signal.mkv")
		sidecarPath := filepath.Join(root, "Signal.en.srt")
		if err := os.WriteFile(mediaPath, []byte("movie"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		library, err := server.createLibrary(CreateLibraryRequest{Name: "Subtitle Safety", Type: "movie", Paths: []string{root}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
			t.Fatal(err)
		}
		realSidecarPath, err := filepath.EvalSymlinks(sidecarPath)
		if err != nil {
			t.Fatal(err)
		}
		var mediaID, originalStreamID, originalEvidence string
		if err := server.db.QueryRow(`SELECT media_id, identity_evidence FROM media_files WHERE library_id = ?`, library.ID).Scan(&mediaID, &originalEvidence); err != nil {
			t.Fatal(err)
		}
		if err := server.db.QueryRow(`SELECT id FROM media_streams WHERE media_id = ? AND source_kind = 'sidecar'`, mediaID).Scan(&originalStreamID); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nChanged\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mediaPath, []byte("movie changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		originalReadFile := scannerSidecarReadFile
		t.Cleanup(func() { scannerSidecarReadFile = originalReadFile })
		scannerSidecarReadFile = func(path string) ([]byte, error) {
			if filepath.Clean(path) == filepath.Clean(realSidecarPath) {
				return nil, fs.ErrPermission
			}
			return originalReadFile(path)
		}
		result, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full")
		if err != nil || result.AbsenceAuthoritative || result.DegradedRoots == 0 {
			t.Fatalf("incomplete preflight result = %#v error = %v", result, err)
		}
		var streamID, evidence string
		if err := server.db.QueryRow(`SELECT id FROM media_streams WHERE media_id = ? AND source_kind = 'sidecar'`, mediaID).Scan(&streamID); err != nil {
			t.Fatal(err)
		}
		if err := server.db.QueryRow(`SELECT identity_evidence FROM media_files WHERE media_id = ?`, mediaID).Scan(&evidence); err != nil {
			t.Fatal(err)
		}
		if streamID != originalStreamID || evidence != originalEvidence {
			t.Fatalf("failed preflight changed catalog: stream %q -> %q, evidence %q -> %q", originalStreamID, streamID, originalEvidence, evidence)
		}
	})

	t.Run("lyrics", func(t *testing.T) {
		server := newScannerTestServer(t)
		root := t.TempDir()
		mediaPath := filepath.Join(root, "01 - Signal.flac")
		sidecarPath := filepath.Join(root, "01 - Signal.en.lrc")
		if err := os.WriteFile(mediaPath, []byte("track"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sidecarPath, []byte("[00:01.00]Original"), 0o600); err != nil {
			t.Fatal(err)
		}
		library, err := server.createLibrary(CreateLibraryRequest{Name: "Lyric Safety", Type: "music", Paths: []string{root}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
			t.Fatal(err)
		}
		realSidecarPath, err := filepath.EvalSymlinks(sidecarPath)
		if err != nil {
			t.Fatal(err)
		}
		var mediaID, originalText string
		if err := server.db.QueryRow(`SELECT media_id FROM media_files WHERE library_id = ?`, library.ID).Scan(&mediaID); err != nil {
			t.Fatal(err)
		}
		if err := server.db.QueryRow(`SELECT text FROM media_lyrics WHERE media_id = ? AND source = 'local'`, mediaID).Scan(&originalText); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mediaPath, []byte("track changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		originalReadFile := scannerSidecarReadFile
		t.Cleanup(func() { scannerSidecarReadFile = originalReadFile })
		scannerSidecarReadFile = func(path string) ([]byte, error) {
			if filepath.Clean(path) == filepath.Clean(realSidecarPath) {
				return nil, fs.ErrPermission
			}
			return originalReadFile(path)
		}
		result, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full")
		if err != nil || result.AbsenceAuthoritative || result.DegradedRoots == 0 {
			t.Fatalf("incomplete preflight result = %#v error = %v", result, err)
		}
		var text string
		if err := server.db.QueryRow(`SELECT text FROM media_lyrics WHERE media_id = ? AND source = 'local'`, mediaID).Scan(&text); err != nil {
			t.Fatal(err)
		}
		if text != originalText {
			t.Fatalf("failed lyric preflight changed catalog: %q -> %q", originalText, text)
		}
	})
}

func TestLibraryScannerSubtitlePublicationFailureRemainsRetryable(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Retry.mkv")
	sidecarPath := filepath.Join(root, "Retry.en.srt")
	if err := os.WriteFile(mediaPath, []byte("movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Publication Retry", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
		t.Fatal(err)
	}
	var mediaID, originalStreamID, originalEvidence string
	if err := server.db.QueryRow(`SELECT media_id, identity_evidence FROM media_files WHERE library_id = ?`, library.ID).Scan(&mediaID, &originalEvidence); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT id FROM media_streams WHERE media_id = ? AND source_kind = 'sidecar'`, mediaID).Scan(&originalStreamID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("movie changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nChanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalPublish := scannerPublishArtifact
	t.Cleanup(func() { scannerPublishArtifact = originalPublish })
	scannerPublishArtifact = func(string, string, []byte) error { return fs.ErrPermission }
	result, err := server.performLibraryScanWithMode(context.Background(), library, "", "force_full")
	if !errors.Is(err, fs.ErrPermission) || result.AbsenceAuthoritative || result.DegradedRoots != 0 {
		t.Fatalf("publication failure result = %#v error = %v", result, err)
	}
	var failedStreamID, failedEvidence string
	if err := server.db.QueryRow(`SELECT id FROM media_streams WHERE media_id = ? AND source_kind = 'sidecar'`, mediaID).Scan(&failedStreamID); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT identity_evidence FROM media_files WHERE media_id = ?`, mediaID).Scan(&failedEvidence); err != nil {
		t.Fatal(err)
	}
	if failedStreamID != originalStreamID || failedEvidence != originalEvidence {
		t.Fatalf("failed publication advanced catalog: stream %q -> %q, evidence %q -> %q", originalStreamID, failedStreamID, originalEvidence, failedEvidence)
	}

	scannerPublishArtifact = originalPublish
	result, err = server.performLibraryScanWithMode(context.Background(), library, "", "force_full")
	if err != nil || !result.AbsenceAuthoritative {
		t.Fatalf("retry result = %#v error = %v", result, err)
	}
	var retryStreamID, retryEvidence string
	if err := server.db.QueryRow(`SELECT id FROM media_streams WHERE media_id = ? AND source_kind = 'sidecar'`, mediaID).Scan(&retryStreamID); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT identity_evidence FROM media_files WHERE media_id = ?`, mediaID).Scan(&retryEvidence); err != nil {
		t.Fatal(err)
	}
	if retryStreamID != originalStreamID || retryEvidence == originalEvidence {
		t.Fatalf("retry did not converge: stream %q -> %q, evidence %q -> %q", originalStreamID, retryStreamID, originalEvidence, retryEvidence)
	}
	artifact, err := os.ReadFile(filepath.Join(server.cfg.AppDataDir, "subtitles", safePathComponent(mediaID), safePathComponent(retryStreamID)+".vtt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(artifact), "Changed") {
		t.Fatalf("retry artifact = %q", string(artifact))
	}
}

func TestLibraryScannerPrunesDeletedLocalArtworkWithinAuthoritativeScope(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	showDir := filepath.Join(root, "Scope Show")
	seasonOne := filepath.Join(showDir, "Season 01")
	seasonTwo := filepath.Join(showDir, "Season 02")
	for _, dir := range []string{seasonOne, seasonTwo} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	showPoster := filepath.Join(showDir, "poster.jpg")
	seasonOnePoster := filepath.Join(seasonOne, "season.jpg")
	seasonTwoPoster := filepath.Join(seasonTwo, "season.jpg")
	for path, contents := range map[string]string{
		filepath.Join(seasonOne, "Scope Show S01E01.mkv"): "one",
		filepath.Join(seasonTwo, "Scope Show S02E01.mkv"): "two",
		showPoster:      "show",
		seasonOnePoster: "season one",
		seasonTwoPoster: "season two",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Artwork Scope", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
		t.Fatal(err)
	}
	realShowPoster, err := filepath.EvalSymlinks(showPoster)
	if err != nil {
		t.Fatal(err)
	}
	realSeasonOnePoster, err := filepath.EvalSymlinks(seasonOnePoster)
	if err != nil {
		t.Fatal(err)
	}
	realSeasonTwoPoster, err := filepath.EvalSymlinks(seasonTwoPoster)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(seasonOnePoster); err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
		t.Fatal(err)
	}
	assertLocalImageCount := func(path string, expected int) {
		t.Helper()
		var count int
		if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_images WHERE source = 'local' AND path = ?`, path).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != expected {
			t.Fatalf("local image %q count = %d, expected %d", path, count, expected)
		}
	}
	assertLocalImageCount(realSeasonOnePoster, 0)
	assertLocalImageCount(realShowPoster, 1)
	assertLocalImageCount(realSeasonTwoPoster, 1)
}

func TestLibraryScannerExpandsMultiEpisodeFiles(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Moonrise.S01E02-E03.Signal.mkv")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write multi-episode media file: %v", err)
	}
	realMediaPath, err := filepath.EvalSymlinks(mediaPath)
	if err != nil {
		t.Fatalf("resolve media path: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Multi Shows", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}

	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'episode' ORDER BY m.episode_number`, []any{library.ID})
	if err != nil {
		t.Fatalf("query scanned episodes: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("episode count = %d, expected 2: %#v", len(items), items)
	}
	if items[0].SeasonNumber != 1 || items[0].EpisodeNumber != 2 || items[1].SeasonNumber != 1 || items[1].EpisodeNumber != 3 {
		t.Fatalf("multi-episode numbers = %+v / %+v", items[0], items[1])
	}
	if items[0].SourceURL != realMediaPath || items[1].SourceURL != realMediaPath {
		t.Fatalf("multi-episode source paths = %q / %q, expected %q", items[0].SourceURL, items[1].SourceURL, realMediaPath)
	}
	var mediaFileRows int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE path = ? AND available = 1`, realMediaPath).Scan(&mediaFileRows); err != nil {
		t.Fatalf("count media file rows: %v", err)
	}
	if mediaFileRows != 2 {
		t.Fatalf("media file rows = %d, expected 2", mediaFileRows)
	}
}

func TestLibraryScannerSkipsMetadataRefreshForLocalProvider(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Local.Only.2026.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:     "Local Metadata Movies",
		Type:     "movie",
		Paths:    []string{root},
		Settings: map[string]any{"metadataProvider": "Local Media Assets"},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.MetadataRefreshQueued != 0 {
		t.Fatalf("metadata refresh queued = %d, expected 0", result.MetadataRefreshQueued)
	}
}

func TestLibraryScannerHonorsAnalyzeOnScanSetting(t *testing.T) {
	server := newScannerTestServer(t)
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe-stub")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\nprintf '{}'\n"), 0o700); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	server.cfg.FFprobePath = ffprobePath
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Quiet.Analysis.2026.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:     "Quiet Analysis",
		Type:     "movie",
		Paths:    []string{root},
		Settings: map[string]any{"analyzeOnScan": false},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 1 {
		t.Fatalf("indexed files = %d, expected 1", result.FilesIndexed)
	}
	if result.AnalysisQueued != 0 {
		t.Fatalf("analysis queued = %d, expected 0", result.AnalysisQueued)
	}
	var analysisJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze'`).Scan(&analysisJobs); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if analysisJobs != 0 {
		t.Fatalf("expected no media analysis jobs, got %d", analysisJobs)
	}
}

func TestLibraryScannerHonorsProbeStreamsSetting(t *testing.T) {
	server := newScannerTestServer(t)
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe-stub")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\nprintf '{}'\n"), 0o700); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	server.cfg.FFprobePath = ffprobePath
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "No.Probe.2026.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:     "No Probe",
		Type:     "movie",
		Paths:    []string{root},
		Settings: map[string]any{"probeStreams": false},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 1 {
		t.Fatalf("indexed files = %d, expected 1", result.FilesIndexed)
	}
	if result.AnalysisQueued != 0 {
		t.Fatalf("analysis queued = %d, expected 0", result.AnalysisQueued)
	}
	var analysisJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze'`).Scan(&analysisJobs); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if analysisJobs != 0 {
		t.Fatalf("expected no media analysis jobs, got %d", analysisJobs)
	}
}

func TestLibraryScannerStoresTechnicalMovieVersions(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	fullHDPath := filepath.Join(root, "F1.2025.1080p.WEB-DL.H264.AAC.mkv")
	ultraHDPath := filepath.Join(root, "F1.2025.Extended.3D.2160p.BluRay.HEVC.Atmos.HDR-Kitsune.mkv")
	if err := os.WriteFile(fullHDPath, []byte(strings.Repeat("a", 4096)), 0o600); err != nil {
		t.Fatalf("write 1080p movie: %v", err)
	}
	if err := os.WriteFile(ultraHDPath, []byte("small but better"), 0o600); err != nil {
		t.Fatalf("write 2160p movie: %v", err)
	}
	realUltraHDPath, err := filepath.EvalSymlinks(ultraHDPath)
	if err != nil {
		t.Fatalf("resolve 2160p movie: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Versioned Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}

	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query movie: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("movie count = %d, expected one versioned item: %#v", len(items), items)
	}
	if items[0].SourceURL != realUltraHDPath {
		t.Fatalf("selected source = %q, expected highest ranked version %q", items[0].SourceURL, realUltraHDPath)
	}
	if items[0].Year != 2025 || items[0].Edition != "Extended" {
		t.Fatalf("movie filename metadata year=%d edition=%q", items[0].Year, items[0].Edition)
	}
	detail, err := server.getMediaDetail("", items[0].ID)
	if err != nil {
		t.Fatalf("load movie detail: %v", err)
	}
	if len(detail.MediaFiles) != 2 {
		t.Fatalf("media versions = %d, expected 2: %#v", len(detail.MediaFiles), detail.MediaFiles)
	}
	if !detail.MediaFiles[0].Selected || detail.MediaFiles[0].Resolution != "2160p" || detail.MediaFiles[0].Source != "Blu-ray" || detail.MediaFiles[0].VideoCodec != "hevc" || detail.MediaFiles[0].DynamicRange != "HDR" {
		t.Fatalf("top version metadata = %#v", detail.MediaFiles[0])
	}
	if !detail.MediaFiles[0].ThreeD || detail.MediaFiles[0].ReleaseGroup != "Kitsune" || detail.MediaFiles[0].VersionGroup != items[0].ID {
		t.Fatalf("top version release/group metadata = %#v", detail.MediaFiles[0])
	}
	if detail.MediaFiles[1].Resolution != "1080p" || detail.MediaFiles[1].Source != "WEB-DL" || detail.MediaFiles[1].VideoCodec != "h264" {
		t.Fatalf("second version metadata = %#v", detail.MediaFiles[1])
	}
}

func TestLibraryScannerIngestsSidecarNFO(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "F1.2025.1080p.mkv")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "F1.2025.1080p.nfo"), []byte(`
<movie>
  <title>F1</title>
  <originaltitle>F1: The Movie</originaltitle>
  <year>2025</year>
  <plot>Racing drama from a local sidecar.</plot>
  <tagline>Drive faster.</tagline>
  <mpaa>PG-13</mpaa>
  <rating>7.8</rating>
  <studio>Apple Original Films</studio>
  <genre>Drama</genre>
  <genre>Action</genre>
  <director>Joseph Kosinski</director>
  <writer>Ehren Kruger</writer>
  <actor><name>Brad Pitt</name><role>Sonny Hayes</role><order>1</order></actor>
  <uniqueid type="tmdb" default="true">911430</uniqueid>
</movie>`), 0o600); err != nil {
		t.Fatalf("write nfo: %v", err)
	}
	for name, contents := range map[string]string{
		"logo.png":     "logo",
		"banner.jpg":   "banner",
		"disc.png":     "disc",
		"clearart.png": "clearart",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write local artwork %s: %v", name, err)
		}
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("movie count = %d", len(items))
	}
	if items[0].Title != "F1" || items[0].Summary != "Racing drama from a local sidecar." || items[0].Year != 2025 {
		t.Fatalf("nfo metadata was not applied: %+v", items[0])
	}
	if len(items[0].Genres) != 2 || items[0].Genres[0] != "Drama" || items[0].Genres[1] != "Action" {
		t.Fatalf("nfo genres = %#v", items[0].Genres)
	}
	var externalID string
	if err := server.db.QueryRow(`SELECT external_id FROM media_provider_ids WHERE media_id = ? AND provider = 'tmdb'`, items[0].ID).Scan(&externalID); err != nil {
		t.Fatalf("load provider id: %v", err)
	}
	if externalID != "911430" {
		t.Fatalf("provider id = %q", externalID)
	}
	var rawRating, country, ratingSystem string
	var normalizedRank int
	if err := server.db.QueryRow(`
		SELECT raw_rating, country, rating_system, normalized_rank
		FROM media_rating_evidence
		WHERE media_id = ? AND provider = 'local' AND source = 'nfo'`, items[0].ID).Scan(&rawRating, &country, &ratingSystem, &normalizedRank); err != nil {
		t.Fatalf("load rating evidence: %v", err)
	}
	if rawRating != "PG-13" || country != "US" || ratingSystem != "MPA" || normalizedRank != 4 {
		t.Fatalf("rating evidence = %q %q %q %d", rawRating, country, ratingSystem, normalizedRank)
	}
	detail, err := server.getMediaDetail("", items[0].ID)
	if err != nil {
		t.Fatalf("load detail: %v", err)
	}
	if len(detail.People) != 3 {
		t.Fatalf("people = %#v", detail.People)
	}
	for kind, name := range map[string]string{"logo": "logo.png", "banner": "banner.jpg", "disc": "disc.png", "clearart": "clearart.png"} {
		if !mediaImagesContainLocal(detail.MediaImages, kind, filepath.Join(root, name)) {
			t.Fatalf("missing local %s image: %#v", kind, detail.MediaImages)
		}
	}
}

func TestLibraryScannerLocalMetadataModes(t *testing.T) {
	server := newScannerTestServer(t)
	supplementRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(supplementRoot, "Local.Title.2026.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write supplement media: %v", err)
	}
	if err := os.WriteFile(filepath.Join(supplementRoot, "Local.Title.2026.nfo"), []byte(`<movie><title>NFO Override</title><plot>Local supplement summary.</plot><uniqueid type="tmdb">12345</uniqueid></movie>`), 0o600); err != nil {
		t.Fatalf("write supplement nfo: %v", err)
	}
	supplementLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:     "Supplement Movies",
		Type:     "movie",
		Paths:    []string{supplementRoot},
		Settings: map[string]any{"localMetadataMode": "supplement", "metadataProvider": "TMDB"},
	})
	if err != nil {
		t.Fatalf("create supplement library: %v", err)
	}
	if _, err := server.performLibraryScan(supplementLibrary, ""); err != nil {
		t.Fatalf("scan supplement library: %v", err)
	}
	supplementItems, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{supplementLibrary.ID})
	if err != nil {
		t.Fatalf("query supplement media: %v", err)
	}
	if len(supplementItems) != 1 {
		t.Fatalf("supplement movie count = %d", len(supplementItems))
	}
	if supplementItems[0].Title != "Local Title" || supplementItems[0].Summary != "" {
		t.Fatalf("supplement mode should not override fields: %+v", supplementItems[0])
	}
	if id, ok := server.mediaProviderID(supplementItems[0].ID, "tmdb", "movie"); !ok || id != "12345" {
		t.Fatalf("supplement provider id = %q ok=%v", id, ok)
	}

	offRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(offRoot, "Ignored.Local.2026.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write off media: %v", err)
	}
	if err := os.WriteFile(filepath.Join(offRoot, "Ignored.Local.2026.nfo"), []byte(`<movie><title>NFO Ignored</title><uniqueid type="tmdb">99999</uniqueid></movie>`), 0o600); err != nil {
		t.Fatalf("write off nfo: %v", err)
	}
	offLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:     "Local Off Movies",
		Type:     "movie",
		Paths:    []string{offRoot},
		Settings: map[string]any{"localMetadataMode": "off", "metadataProvider": "TMDB"},
	})
	if err != nil {
		t.Fatalf("create off library: %v", err)
	}
	if _, err := server.performLibraryScan(offLibrary, ""); err != nil {
		t.Fatalf("scan off library: %v", err)
	}
	offItems, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{offLibrary.ID})
	if err != nil {
		t.Fatalf("query off media: %v", err)
	}
	if len(offItems) != 1 {
		t.Fatalf("off movie count = %d", len(offItems))
	}
	if offItems[0].Title != "Ignored Local" {
		t.Fatalf("off mode title = %q", offItems[0].Title)
	}
	if id, ok := server.mediaProviderID(offItems[0].ID, "tmdb", "movie"); ok {
		t.Fatalf("off mode should ignore provider id, got %q", id)
	}
}

func TestLibraryScannerHonorsGlobalLocalNFODisabled(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'metadataAgents'`, `{"movies":"TMDB","tv":"TMDB","anime":"AniList","music":"MusicBrainz","localNFO":false,"embeddedTags":true,"refreshDays":7}`); err != nil {
		t.Fatalf("save metadata settings: %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Ignored.Local.2026.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Ignored.Local.2026.nfo"), []byte(`<movie><title>NFO Disabled</title><plot>Should not appear.</plot><uniqueid type="tmdb">99999</uniqueid></movie>`), 0o600); err != nil {
		t.Fatalf("write nfo: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("movie count = %d", len(items))
	}
	if items[0].Title != "Ignored Local" || items[0].Summary != "" {
		t.Fatalf("global localNFO disabled should ignore NFO fields: %+v", items[0])
	}
	if id, ok := server.mediaProviderID(items[0].ID, "tmdb", "movie"); ok {
		t.Fatalf("global localNFO disabled should ignore provider id, got %q", id)
	}
}

func TestLibraryScannerIngestsTextSidecarSubtitles(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Northbridge.S01E02.mkv")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Northbridge.S01E02.en.srt"), []byte("1\n00:00:01,000 --> 00:00:03,000\nHello\n"), 0o600); err != nil {
		t.Fatalf("write srt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Northbridge.S01E02.fr.forced.vtt"), []byte("WEBVTT\n\n00:00:01.000 --> 00:00:03.000\nBonjour\n"), 0o600); err != nil {
		t.Fatalf("write vtt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Northbridge.S01E02.es.ass"), []byte("[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:03.00,Default,Hola\n"), 0o600); err != nil {
		t.Fatalf("write ass: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Northbridge.S01E02.de.sbv"), []byte("0:00:01.000,0:00:03.000\nHallo\n"), 0o600); err != nil {
		t.Fatalf("write sbv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Northbridge.S01E02.it.ttml"), []byte(`<tt><body><div><p begin="00:00:01.000" end="00:00:03.000">Ciao</p></div></body></tt>`), 0o600); err != nil {
		t.Fatalf("write ttml: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "TV", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'episode'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query episode: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("episode count = %d", len(items))
	}
	detail, err := server.getMediaDetail("", items[0].ID)
	if err != nil {
		t.Fatalf("load detail: %v", err)
	}
	var subtitleCount int
	var languages []string
	for _, stream := range detail.Streams {
		if stream.Kind != "subtitle" {
			continue
		}
		subtitleCount++
		languages = append(languages, stream.Language)
		if stream.Codec != "webvtt" || !strings.HasPrefix(stream.SourceURL, "/api/media/") {
			t.Fatalf("bad subtitle stream: %#v", stream)
		}
	}
	if subtitleCount != 5 {
		t.Fatalf("subtitle count = %d streams=%#v", subtitleCount, detail.Streams)
	}
	for _, expected := range []string{"en", "fr", "es", "de", "it"} {
		if !containsTestString(languages, expected) {
			t.Fatalf("subtitle languages = %#v", languages)
		}
	}
}

func containsTestString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func mediaImagesContainLocal(images []MediaImage, imageType, path string) bool {
	expected, err := filepath.EvalSymlinks(path)
	if err != nil {
		expected = path
	}
	for _, image := range images {
		actual, err := filepath.EvalSymlinks(image.Path)
		if err != nil {
			actual = image.Path
		}
		if image.Type == imageType && image.Source == "local" && actual == expected {
			return true
		}
	}
	return false
}

func TestLibraryScannerBuildsNestedTVHierarchy(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	seasonDir := filepath.Join(root, "The Rookie", "Season 08")
	specialsDir := filepath.Join(root, "The Rookie", "Specials")
	if err := os.MkdirAll(seasonDir, 0o700); err != nil {
		t.Fatalf("create season dir: %v", err)
	}
	if err := os.MkdirAll(specialsDir, 0o700); err != nil {
		t.Fatalf("create specials dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "The Rookie", "poster.jpg"), []byte("show poster"), 0o600); err != nil {
		t.Fatalf("write show poster: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "The Rookie", "tvshow.nfo"), []byte(`<tvshow><title>The Rookie</title><plot>Local show summary.</plot><genre>Crime</genre><uniqueid type="tvdb">350665</uniqueid></tvshow>`), 0o600); err != nil {
		t.Fatalf("write show nfo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "season.jpg"), []byte("season poster"), 0o600); err != nil {
		t.Fatalf("write season poster: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "season.nfo"), []byte(`<season><title>Season Eight</title><plot>Local season summary.</plot><year>2026</year></season>`), 0o600); err != nil {
		t.Fatalf("write season nfo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "The Rookie S08E15 Survive the Streets 1080p AMZN WEB-DL H264-Kitsune.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write episode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "The Rookie S08E15 Survive the Streets 1080p AMZN WEB-DL H264-Kitsune.jpg"), []byte("episode still"), 0o600); err != nil {
		t.Fatalf("write episode still: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specialsDir, "The Rookie - S00E01 - Behind the Badge.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write special: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "TV Shows", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 2 {
		t.Fatalf("indexed files = %d, expected 2", result.FilesIndexed)
	}

	shows, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'show' AND m.title = 'The Rookie'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query show: %v", err)
	}
	if len(shows) != 1 {
		t.Fatalf("show count = %d, expected 1", len(shows))
	}
	seasons, err := server.childrenFor("", shows[0])
	if err != nil {
		t.Fatalf("load seasons: %v", err)
	}
	if len(seasons) != 2 {
		t.Fatalf("season count = %d, expected 2: %#v", len(seasons), seasons)
	}

	episodes, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'episode' ORDER BY m.season_number, m.episode_number`, []any{library.ID})
	if err != nil {
		t.Fatalf("query episodes: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episode count = %d, expected 2", len(episodes))
	}
	if episodes[0].SeasonNumber != 0 || episodes[0].EpisodeNumber != 1 || episodes[0].GrandparentID != shows[0].ID {
		t.Fatalf("special episode hierarchy was not populated: %+v show=%+v", episodes[0], shows[0])
	}
	if episodes[1].SeasonNumber != 8 || episodes[1].EpisodeNumber != 15 || episodes[1].GrandparentTitle != "The Rookie" {
		t.Fatalf("numbered episode hierarchy was not populated: %+v", episodes[1])
	}
	if episodes[1].Title != "Survive The Streets" {
		t.Fatalf("episode title = %q", episodes[1].Title)
	}
	showDetail, err := server.getMediaDetail("", shows[0].ID)
	if err != nil {
		t.Fatalf("load show detail: %v", err)
	}
	if !mediaImagesContainLocal(showDetail.MediaImages, "poster", filepath.Join(root, "The Rookie", "poster.jpg")) {
		t.Fatalf("show local images = %#v", showDetail.MediaImages)
	}
	if showDetail.Summary != "Local show summary." || len(showDetail.Genres) != 1 || showDetail.Genres[0] != "Crime" {
		t.Fatalf("show local metadata = %+v", showDetail)
	}
	if id, ok := server.mediaProviderID(showDetail.ID, "tvdb", ""); !ok || id != "350665" {
		t.Fatalf("show tvdb provider id = %q ok=%v", id, ok)
	}
	seasonDetail, err := server.getMediaDetail("", episodes[1].ParentID)
	if err != nil {
		t.Fatalf("load season detail: %v", err)
	}
	if !mediaImagesContainLocal(seasonDetail.MediaImages, "poster", filepath.Join(seasonDir, "season.jpg")) {
		t.Fatalf("season local images = %#v", seasonDetail.MediaImages)
	}
	if seasonDetail.Title != "Season Eight" || seasonDetail.Summary != "Local season summary." || seasonDetail.Year != 2026 {
		t.Fatalf("season local metadata = %+v", seasonDetail)
	}
	if seasonDetail.SeasonNumber != 8 || seasonDetail.IndexNumber != 8 {
		t.Fatalf("season numbering = %+v", seasonDetail)
	}
	episodeDetail, err := server.getMediaDetail("", episodes[1].ID)
	if err != nil {
		t.Fatalf("load episode detail: %v", err)
	}
	if !mediaImagesContainLocal(episodeDetail.MediaImages, "thumb", filepath.Join(seasonDir, "The Rookie S08E15 Survive the Streets 1080p AMZN WEB-DL H264-Kitsune.jpg")) {
		t.Fatalf("episode local images = %#v", episodeDetail.MediaImages)
	}
	if !strings.Contains(episodeDetail.Images.Thumb, "/api/artwork/"+episodes[1].ID+"/thumb.svg") {
		t.Fatalf("episode thumb image was not exposed: %+v", episodeDetail.Images)
	}

	providerTitle := "Survive the Streets"
	if _, err := server.db.Exec(
		`UPDATE media_items SET title = ?, sort_title = ?, summary = ?, metadata_refreshed_at = ? WHERE id = ?`,
		providerTitle, providerTitle, "The team works together after a possible suicide spirals into conspiracy.", time.Now().UTC().Format(time.RFC3339), episodes[1].ID,
	); err != nil {
		t.Fatalf("seed provider episode metadata: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("rescan library: %v", err)
	}
	refreshedEpisode, err := server.getMediaDetail("", episodes[1].ID)
	if err != nil {
		t.Fatalf("load refreshed episode detail: %v", err)
	}
	if refreshedEpisode.Title != providerTitle {
		t.Fatalf("episode title after rescan = %q, expected provider title %q", refreshedEpisode.Title, providerTitle)
	}
}

func TestLibraryScannerParsesAnimeAbsoluteEpisodes(t *testing.T) {
	server := newScannerTestServer(t)
	animeRoot := t.TempDir()
	animeDir := filepath.Join(animeRoot, "One Piece")
	if err := os.MkdirAll(animeDir, 0o700); err != nil {
		t.Fatalf("create anime dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(animeDir, "1089 - Entering a New Chapter.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write anime episode: %v", err)
	}
	frierenDir := filepath.Join(animeRoot, "Frieren")
	if err := os.MkdirAll(frierenDir, 0o700); err != nil {
		t.Fatalf("create frieren dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frierenDir, "Frieren - 01 - Journey's End.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write one-digit anime episode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frierenDir, "Frieren - Episode 02-03 - Magic.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write anime absolute range: %v", err)
	}
	seasonDir := filepath.Join(animeRoot, "Demon Slayer", "Season 02")
	if err := os.MkdirAll(seasonDir, 0o700); err != nil {
		t.Fatalf("create anime season dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "01 - Training Begins.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write season-folder anime episode: %v", err)
	}
	animeLibrary, err := server.createLibrary(CreateLibraryRequest{Name: "Anime", Type: "anime", Paths: []string{animeRoot}})
	if err != nil {
		t.Fatalf("create anime library: %v", err)
	}
	if _, err := server.performLibraryScan(animeLibrary, ""); err != nil {
		t.Fatalf("scan anime library: %v", err)
	}
	animeEpisodes, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'episode' ORDER BY m.parent_id, m.season_number, m.episode_number`, []any{animeLibrary.ID})
	if err != nil {
		t.Fatalf("query anime episodes: %v", err)
	}
	if len(animeEpisodes) != 5 {
		t.Fatalf("anime episode count = %d", len(animeEpisodes))
	}
	expected := map[string]struct {
		season  int
		episode int
	}{
		"One Piece":    {season: 1, episode: 1089},
		"Frieren/1":    {season: 1, episode: 1},
		"Frieren/2":    {season: 1, episode: 2},
		"Frieren/3":    {season: 1, episode: 3},
		"Demon Slayer": {season: 2, episode: 1},
	}
	for _, episode := range animeEpisodes {
		key := episode.GrandparentTitle
		if episode.GrandparentTitle == "Frieren" {
			key = fmt.Sprintf("Frieren/%d", episode.EpisodeNumber)
		}
		want, ok := expected[key]
		if !ok || episode.SeasonNumber != want.season || episode.EpisodeNumber != want.episode {
			t.Fatalf("anime absolute episode parsed incorrectly: %+v expected=%#v", episode, expected)
		}
	}
}

func TestLibraryScannerParsesDateEpisodes(t *testing.T) {
	server := newScannerTestServer(t)
	showRoot := t.TempDir()
	showDir := filepath.Join(showRoot, "Late News")
	if err := os.MkdirAll(showDir, 0o700); err != nil {
		t.Fatalf("create show dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "2024-05-01 Headlines.mkv"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write date episode: %v", err)
	}
	showLibrary, err := server.createLibrary(CreateLibraryRequest{Name: "Date Shows", Type: "show", Paths: []string{showRoot}})
	if err != nil {
		t.Fatalf("create date show library: %v", err)
	}
	if _, err := server.performLibraryScan(showLibrary, ""); err != nil {
		t.Fatalf("scan date show library: %v", err)
	}
	dateEpisodes, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'episode'`, []any{showLibrary.ID})
	if err != nil {
		t.Fatalf("query date episodes: %v", err)
	}
	if len(dateEpisodes) != 1 {
		t.Fatalf("date episode count = %d", len(dateEpisodes))
	}
	if dateEpisodes[0].SeasonNumber != 2024 || dateEpisodes[0].EpisodeNumber != 501 || dateEpisodes[0].Year != 2024 || dateEpisodes[0].GrandparentTitle != "Late News" || dateEpisodes[0].Title != "Headlines" {
		t.Fatalf("date episode parsed incorrectly: %+v", dateEpisodes[0])
	}
}

func TestLibraryScannerBuildsMusicArtistAlbumTrackHierarchy(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	albumDir := filepath.Join(root, "Daft Punk", "Random Access Memories")
	if err := os.MkdirAll(albumDir, 0o700); err != nil {
		t.Fatalf("create album dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Daft Punk", "artist.nfo"), []byte(`<artist><title>Daft Punk</title><plot>French electronic duo.</plot><genre>House</genre><uniqueid type="musicbrainz">artist-mbid</uniqueid></artist>`), 0o600); err != nil {
		t.Fatalf("write artist nfo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Daft Punk", "folder.jpg"), []byte("artist photo"), 0o600); err != nil {
		t.Fatalf("write artist image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01 - Give Life Back to Music.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write track one: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01 - Give Life Back to Music.en.lrc"), []byte("[00:01.00]Give life back to music"), 0o600); err != nil {
		t.Fatalf("write lyrics: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "02 - The Game of Love.flac"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write track two: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "cover.jpg"), []byte("fake cover"), 0o600); err != nil {
		t.Fatalf("write album cover: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "album.nfo"), []byte(`<album><title>Random Access Memories</title><year>2013</year><plot>Local album notes.</plot><genre>Electronic</genre></album>`), 0o600); err != nil {
		t.Fatalf("write album nfo: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Music", Type: "music", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 2 {
		t.Fatalf("indexed files = %d, expected 2", result.FilesIndexed)
	}
	if result.MetadataRefreshQueued != 4 {
		t.Fatalf("metadata refresh queued = %d, expected 4", result.MetadataRefreshQueued)
	}

	artists, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'artist' AND m.title = 'Daft Punk'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query artists: %v", err)
	}
	if len(artists) != 1 {
		t.Fatalf("artist count = %d, expected 1", len(artists))
	}
	if artists[0].Summary != "French electronic duo." || len(artists[0].Genres) != 1 || artists[0].Genres[0] != "House" {
		t.Fatalf("artist local metadata = %+v", artists[0])
	}
	if id, ok := server.mediaProviderID(artists[0].ID, "musicbrainz", "artist"); !ok || id != "artist-mbid" {
		t.Fatalf("artist provider id = %q ok=%v", id, ok)
	}
	artistDetail, err := server.getMediaDetail("", artists[0].ID)
	if err != nil {
		t.Fatalf("load artist detail: %v", err)
	}
	if !mediaImagesContainLocal(artistDetail.MediaImages, "poster", filepath.Join(root, "Daft Punk", "folder.jpg")) {
		t.Fatalf("artist media images = %#v", artistDetail.MediaImages)
	}
	albums, err := server.childrenFor("", artists[0])
	if err != nil {
		t.Fatalf("load albums: %v", err)
	}
	if len(albums) != 1 || albums[0].Type != "album" || albums[0].Title != "Random Access Memories" {
		t.Fatalf("albums = %#v", albums)
	}
	tracks, err := server.childrenFor("", albums[0])
	if err != nil {
		t.Fatalf("load tracks: %v", err)
	}
	if len(tracks) != 2 || tracks[0].IndexNumber != 1 || tracks[1].IndexNumber != 2 || tracks[0].GrandparentTitle != "Daft Punk" {
		t.Fatalf("tracks = %#v", tracks)
	}
	if tracks[0].TypedMetadata["trackArtist"] != "Daft Punk" || tracks[0].TypedMetadata["albumArtist"] != "Daft Punk" || tracks[0].TypedMetadata["albumTitle"] != "Random Access Memories" {
		t.Fatalf("track typed metadata = %#v", tracks[0].TypedMetadata)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.waitForLibraryReadModelRepair(waitCtx, library.ID); err != nil {
		t.Fatalf("wait for category facet repair: %v", err)
	}
	var facetCount int
	if err := server.db.QueryRow(`
		SELECT COUNT(*)
		FROM media_category_facets
		WHERE library_id = ? AND facet_type = 'albumArtist' AND value = 'Daft Punk'`, library.ID).Scan(&facetCount); err != nil {
		t.Fatalf("count album artist facets: %v", err)
	}
	if facetCount == 0 {
		t.Fatalf("scan did not refresh album artist category facets")
	}
	trackDetail, err := server.getMediaDetail("", tracks[0].ID)
	if err != nil {
		t.Fatalf("load track detail: %v", err)
	}
	if len(trackDetail.Lyrics) != 1 || !trackDetail.Lyrics[0].Synced || trackDetail.Lyrics[0].Language != "en" || !strings.Contains(trackDetail.Lyrics[0].Text, "Give life back to music") {
		t.Fatalf("track lyrics = %#v", trackDetail.Lyrics)
	}
	albumDetail, err := server.getMediaDetail("", albums[0].ID)
	if err != nil {
		t.Fatalf("load album detail: %v", err)
	}
	if len(albumDetail.MediaImages) == 0 || albumDetail.MediaImages[0].Type != "poster" || albumDetail.MediaImages[0].Source != "local" {
		t.Fatalf("album media images = %#v", albumDetail.MediaImages)
	}
	if albumDetail.Year != 2013 || albumDetail.Summary != "Local album notes." || len(albumDetail.Genres) != 1 || albumDetail.Genres[0] != "Electronic" {
		t.Fatalf("album local metadata = %+v", albumDetail)
	}
	albumPlayable, err := server.resolvePlayablePlaybackItem("", albumDetail)
	if err != nil {
		t.Fatalf("resolve album playback item: %v", err)
	}
	if albumPlayable.ID != tracks[0].ID {
		t.Fatalf("album playback item = %s, expected %s", albumPlayable.ID, tracks[0].ID)
	}
	artistPlayable, err := server.resolvePlayablePlaybackItem("", artistDetail)
	if err != nil {
		t.Fatalf("resolve artist playback item: %v", err)
	}
	if artistPlayable.ID != tracks[0].ID {
		t.Fatalf("artist playback item = %s, expected %s", artistPlayable.ID, tracks[0].ID)
	}
	albumQueue := server.playbackQueue("", albumDetail, albumPlayable, nil)
	if len(albumQueue) != 1 || albumQueue[0].ID != tracks[1].ID {
		t.Fatalf("album playback queue = %#v", albumQueue)
	}
	rowQueue := server.playbackQueue("", tracks[0], tracks[0], []string{tracks[0].ID, tracks[1].ID})
	if len(rowQueue) != 1 || rowQueue[0].ID != tracks[1].ID {
		t.Fatalf("row playback queue = %#v", rowQueue)
	}
	inheritedArtwork, inheritedKind, ok := server.inheritedArtworkItem("", trackDetail, "thumb")
	if !ok || inheritedArtwork.ID != albumDetail.ID || inheritedKind != "poster" {
		t.Fatalf("track inherited artwork item = %#v kind=%q ok=%v", inheritedArtwork, inheritedKind, ok)
	}
	if localPath, ok := server.localArtworkPath(inheritedArtwork, inheritedKind); !ok || filepath.Base(localPath) != "cover.jpg" || !strings.Contains(localPath, filepath.Join("Daft Punk", "Random Access Memories")) {
		t.Fatalf("track inherited artwork path = %q ok=%v", localPath, ok)
	}
	targetedItems, err := server.libraryMetadataRefreshItems(context.Background(), library.ID, map[string]string{"mediaIds": strings.Join([]string{tracks[0].ID, tracks[1].ID}, ",")})
	if err != nil {
		t.Fatalf("load targeted metadata refresh items: %v", err)
	}
	if len(targetedItems) != 1 || targetedItems[0].Type != "album" || targetedItems[0].ID != albums[0].ID {
		t.Fatalf("targeted metadata refresh items = %#v", targetedItems)
	}
	var metadataJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library' AND resource_type = 'library' AND resource_id = ?`, library.ID).Scan(&metadataJobs); err != nil {
		t.Fatalf("count library metadata jobs: %v", err)
	}
	if metadataJobs != 1 {
		t.Fatalf("library metadata jobs = %d, expected 1", metadataJobs)
	}
	var metadataJSON string
	if err := server.db.QueryRow(`SELECT metadata_json FROM jobs WHERE type = 'metadata_refresh_library' AND resource_type = 'library' AND resource_id = ?`, library.ID).Scan(&metadataJSON); err != nil {
		t.Fatalf("load library metadata job metadata: %v", err)
	}
	var jobMetadata map[string]string
	if err := json.Unmarshal([]byte(metadataJSON), &jobMetadata); err != nil {
		t.Fatalf("decode library metadata job metadata: %v", err)
	}
	if !strings.Contains(jobMetadata["mediaIds"], tracks[0].ID) || !strings.Contains(jobMetadata["mediaIds"], tracks[1].ID) {
		t.Fatalf("library metadata job did not retain discovered track ids: %#v", jobMetadata)
	}
	var itemMetadataJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh' AND resource_type = 'media'`).Scan(&itemMetadataJobs); err != nil {
		t.Fatalf("count item metadata jobs: %v", err)
	}
	if itemMetadataJobs != 0 {
		t.Fatalf("item metadata jobs = %d, expected 0", itemMetadataJobs)
	}
}

func TestLibraryScannerIngestsAudiobookOPF(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	bookDir := filepath.Join(root, "Andy Weir", "Project Hail Mary")
	if err := os.MkdirAll(bookDir, 0o700); err != nil {
		t.Fatalf("create book dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "01 - Project Hail Mary.m4b"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write audiobook: %v", err)
	}
	coverDir := filepath.Join(bookDir, "Images")
	if err := os.MkdirAll(coverDir, 0o700); err != nil {
		t.Fatalf("create cover dir: %v", err)
	}
	coverPath := filepath.Join(coverDir, "cover.png")
	if err := os.WriteFile(coverPath, []byte("cover image"), 0o600); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "metadata.opf"), []byte(`
<package>
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Project Hail Mary</dc:title>
    <dc:creator opf:role="aut">Andy Weir</dc:creator>
    <dc:subject>Science Fiction</dc:subject>
    <dc:subject>Adventure</dc:subject>
    <dc:description>A lone astronaut must save Earth.</dc:description>
    <dc:date>2021-05-04</dc:date>
    <dc:identifier id="isbn">9780593135204</dc:identifier>
    <meta name="calibre:series" content="Project Hail Mary"/>
    <meta name="calibre:series_index" content="1"/>
    <meta name="portico:author_provider" content="openlibrary"/>
    <meta name="portico:author_id" content="OL23919A"/>
    <meta name="portico:series_provider" content="openlibrary"/>
    <meta name="portico:series_id" content="OL-series-42"/>
    <meta name="narrator" content="Ray Porter"/>
    <meta name="cover" content="cover-image"/>
  </metadata>
  <manifest>
    <item id="cover-image" href="Images/cover.png" media-type="image/png" properties="cover-image"/>
  </manifest>
</package>`), 0o600); err != nil {
		t.Fatalf("write opf: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Audiobooks", Type: "audiobook", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'audiobook'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query audiobooks: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("audiobook count = %d", len(items))
	}
	item := items[0]
	if item.Title != "Project Hail Mary" || item.Year != 2021 || item.Summary != "A lone astronaut must save Earth." {
		t.Fatalf("opf metadata was not applied: %+v", item)
	}
	if len(item.Genres) != 2 || item.Genres[0] != "Science Fiction" || item.Genres[1] != "Adventure" {
		t.Fatalf("opf genres = %#v", item.Genres)
	}
	if item.TypedMetadata["author"] != "Andy Weir" || item.TypedMetadata["narrator"] != "Ray Porter" || item.TypedMetadata["series"] != "Project Hail Mary" || item.TypedMetadata["seriesIndex"] != "1" {
		t.Fatalf("audiobook typed metadata = %#v", item.TypedMetadata)
	}
	if item.TypedMetadata["authorProvider"] != "openlibrary" || item.TypedMetadata["authorId"] != "OL23919A" ||
		item.TypedMetadata["seriesProvider"] != "openlibrary" || item.TypedMetadata["seriesId"] != "OL-series-42" {
		t.Fatalf("explicit OPF audiobook identities did not survive sanitation: %#v", item.TypedMetadata)
	}
	if id, ok := server.mediaProviderID(item.ID, "isbn", "isbn"); !ok || id != "9780593135204" {
		t.Fatalf("isbn provider id = %q ok=%v", id, ok)
	}
	detail, err := server.getMediaDetail("", item.ID)
	if err != nil {
		t.Fatalf("load audiobook detail: %v", err)
	}
	if !mediaImagesContainLocal(detail.MediaImages, "poster", coverPath) {
		t.Fatalf("audiobook opf cover images = %#v", detail.MediaImages)
	}
}

func TestLibraryScannerGroupsAudiobookPartsByBookFolder(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	bookDir := filepath.Join(root, "Martha Wells", "All Systems Red")
	if err := os.MkdirAll(bookDir, 0o700); err != nil {
		t.Fatalf("create book dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "01 - Part One.m4b"), []byte("part one"), 0o600); err != nil {
		t.Fatalf("write part one: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "02 - Part Two.m4b"), []byte("part two"), 0o600); err != nil {
		t.Fatalf("write part two: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Audiobooks", Type: "audiobook", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 2 {
		t.Fatalf("indexed files = %d", result.FilesIndexed)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'audiobook'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query audiobooks: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("audiobook item count = %d items=%#v", len(items), items)
	}
	if items[0].Title != "All Systems Red" || items[0].TypedMetadata["author"] != "Martha Wells" || items[0].FileCount != 2 {
		t.Fatalf("grouped audiobook = %+v", items[0])
	}
}

func TestLibraryScannerRunsFFprobeAnalysisWhenAvailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell ffprobe stub uses POSIX sh")
	}
	server := newScannerTestServer(t)
	root := t.TempDir()
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe-stub")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\ncat <<'JSON'\n{\"format\":{\"format_name\":\"mp3\",\"duration\":\"123.4\",\"tags\":{\"title\":\"Tagged Track\",\"artist\":\"Tagged Artist\",\"album\":\"Tagged Album\",\"track\":\"07/12\",\"date\":\"2001-05-01\",\"genre\":\"Electronic;Dance\",\"composer\":\"Tagged Composer\",\"producer\":\"Tagged Producer\"}},\"streams\":[{\"index\":0,\"codec_type\":\"audio\",\"codec_name\":\"mp3\",\"channels\":2,\"tags\":{\"language\":\"eng\"}}]}\nJSON\n"), 0o700); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	server.cfg.FFprobePath = ffprobePath
	albumDir := filepath.Join(root, "Artist", "Album")
	if err := os.MkdirAll(albumDir, 0o700); err != nil {
		t.Fatalf("create album dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01 - Track.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write track: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Music", Type: "music", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	tracks, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'track'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("track count = %d, expected 1", len(tracks))
	}
	// Scans now hand analysis off through the durable job queue. The scanner
	// test server does not run background workers, so execute the dispatched
	// job explicitly before asserting the analysis projection.
	if err := server.analyzeMediaForItem(context.Background(), tracks[0], server.mediaAnalysisOptions(tracks[0], mediaAnalysisModeProbe)); err != nil {
		t.Fatalf("run dispatched scanner analysis: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		item, err := server.getMediaDetail("", tracks[0].ID)
		if err != nil {
			t.Fatalf("load track: %v", err)
		}
		if item.DurationSeconds == 123 && len(item.Streams) == 1 && item.Streams[0].Codec == "mp3" && item.Title == "Tagged Track" && item.Studio == "Tagged Artist" && item.IndexNumber == 7 && item.Year == 2001 && len(item.Genres) == 2 && len(item.People) == 2 {
			albums, err := server.queryMedia("", `WHERE m.id = ?`, []any{item.ParentID})
			if err != nil {
				t.Fatalf("load album: %v", err)
			}
			if len(albums) != 1 || albums[0].Title != "Tagged Album" || albums[0].Studio != "Tagged Artist" {
				t.Fatalf("album tags were not applied: %#v", albums)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("analysis did not complete with ffprobe data: %+v", item)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestMusicScanCanKeepFilenameTitleOverEmbeddedTitle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell ffprobe stub uses POSIX sh")
	}
	server := newScannerTestServer(t)
	root := t.TempDir()
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe-stub")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\ncat <<'JSON'\n{\"format\":{\"format_name\":\"mp3\",\"duration\":\"123.4\",\"tags\":{\"title\":\"Tagged Track\",\"artist\":\"Tagged Artist\",\"album\":\"Tagged Album\",\"track\":\"07/12\"}},\"streams\":[{\"index\":0,\"codec_type\":\"audio\",\"codec_name\":\"mp3\",\"channels\":2}]}\nJSON\n"), 0o700); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	server.cfg.FFprobePath = ffprobePath
	albumDir := filepath.Join(root, "Artist", "Album")
	if err := os.MkdirAll(albumDir, 0o700); err != nil {
		t.Fatalf("create album dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "Track.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write track: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Music", Type: "music", Paths: []string{root}, Settings: map[string]any{
		"preferEmbeddedTitles": false,
		// This test invokes analysis directly below; do not race that call with
		// the scanner's automatic media_analyze dispatch.
		"analyzeOnScan": false,
	}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	tracks, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'track'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("track count = %d, expected 1", len(tracks))
	}
	if err := server.analyzeMediaForItem(context.Background(), tracks[0], server.mediaAnalysisOptions(tracks[0], mediaAnalysisModeProbe)); err != nil {
		t.Fatalf("run dispatched scanner analysis: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		item, err := server.getMediaDetail("", tracks[0].ID)
		if err != nil {
			t.Fatalf("load track: %v", err)
		}
		if item.DurationSeconds == 123 {
			if item.Title != "Track" || item.Studio != "Tagged Artist" || item.IndexNumber != 7 || item.ParentTitle != "Tagged Album" {
				t.Fatalf("embedded title preference was not respected: %+v", item)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("analysis did not complete: %+v", item)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMusicScanHonorsGlobalEmbeddedTagsDisabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell ffprobe stub uses POSIX sh")
	}
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'metadataAgents'`, `{"movies":"TMDB","tv":"TMDB","anime":"AniList","music":"MusicBrainz","localNFO":true,"embeddedTags":false,"refreshDays":7}`); err != nil {
		t.Fatalf("save metadata settings: %v", err)
	}
	root := t.TempDir()
	ffprobePath := filepath.Join(t.TempDir(), "ffprobe-stub")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\ncat <<'JSON'\n{\"format\":{\"format_name\":\"mp3\",\"duration\":\"123.4\",\"tags\":{\"title\":\"Tagged Track\",\"artist\":\"Tagged Artist\",\"album\":\"Tagged Album\",\"track\":\"07/12\",\"date\":\"2001-05-01\",\"genre\":\"Electronic\"}},\"streams\":[{\"index\":0,\"codec_type\":\"audio\",\"codec_name\":\"mp3\",\"channels\":2}]}\nJSON\n"), 0o700); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	server.cfg.FFprobePath = ffprobePath
	albumDir := filepath.Join(root, "Folder Artist", "Folder Album")
	if err := os.MkdirAll(albumDir, 0o700); err != nil {
		t.Fatalf("create album dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01 - Filename Track.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write track: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Music", Type: "music", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	tracks, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'track'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("track count = %d, expected 1", len(tracks))
	}
	if err := server.analyzeMediaForItem(context.Background(), tracks[0], server.mediaAnalysisOptions(tracks[0], mediaAnalysisModeProbe)); err != nil {
		t.Fatalf("run dispatched scanner analysis: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		item, err := server.getMediaDetail("", tracks[0].ID)
		if err != nil {
			t.Fatalf("load track: %v", err)
		}
		if item.DurationSeconds == 123 {
			if item.Title == "Tagged Track" || item.Studio == "Tagged Artist" || item.ParentTitle == "Tagged Album" || item.Year != 0 || len(item.Genres) != 0 {
				t.Fatalf("global embeddedTags disabled should ignore embedded metadata: %+v", item)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("analysis did not complete: %+v", item)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAcoustIDFingerprintRefreshUsesMusicBrainzRecording(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fpcalc stub uses POSIX sh")
	}
	server := newScannerTestServer(t)
	root := t.TempDir()
	albumDir := filepath.Join(root, "Sweatshop Union", "United We Fall")
	if err := os.MkdirAll(albumDir, 0o700); err != nil {
		t.Fatalf("create album dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01 - Broken Record.mp3"), []byte("not real audio"), 0o600); err != nil {
		t.Fatalf("write track: %v", err)
	}
	fpcalcPath := filepath.Join(t.TempDir(), "fpcalc-stub")
	if err := os.WriteFile(fpcalcPath, []byte("#!/bin/sh\ncat <<'JSON'\n{\"duration\":201.0,\"fingerprint\":\"fingerprint-test\"}\nJSON\n"), 0o700); err != nil {
		t.Fatalf("write fpcalc stub: %v", err)
	}
	acoustid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lookup" {
			t.Fatalf("unexpected AcoustID path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("client"); got != "test-acoustid" {
			t.Fatalf("client = %q", got)
		}
		if got := r.Form.Get("fingerprint"); got != "fingerprint-test" {
			t.Fatalf("fingerprint = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","results":[{"id":"acoustid-123","score":0.94,"recordings":[{"id":"rec-fp"}]}]}`))
	}))
	defer acoustid.Close()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/recording/rec-fp":
			_, _ = w.Write([]byte(`{
				"id":"rec-fp",
				"title":"Better Days",
				"length":201000,
				"artist-credit":[{"name":"Sweatshop Union","artist":{"id":"artist-fp","name":"Sweatshop Union"}}],
				"releases":[{"id":"release-fp","title":"United We Fall","date":"2004-01-01","release-group":{"id":"rg-fp","title":"United We Fall"}}],
				"genres":[{"name":"Hip-Hop","count":5}]
			}`))
		case "/artist":
			_, _ = w.Write([]byte(`{"artists":[]}`))
		case "/release-group":
			_, _ = w.Write([]byte(`{"release-groups":[]}`))
		case "/recording":
			// The scan may perform a filename-derived provider search before the
			// fingerprint analysis has established the exact recording identity.
			_, _ = w.Write([]byte(`{"recordings":[]}`))
		default:
			t.Fatalf("unexpected MusicBrainz path: %s", r.URL.Path)
		}
	}))
	defer mb.Close()
	server.cfg.FPcalcPath = fpcalcPath
	server.cfg.AcoustIDAPIKey = "test-acoustid"
	server.cfg.AcoustIDBaseURL = acoustid.URL
	server.cfg.MusicBrainzBaseURL = mb.URL

	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Music",
		Type:  "music",
		Paths: []string{root},
		Settings: map[string]any{
			"sonicFingerprinting": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	tracks, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'track'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("track count = %d, expected 1", len(tracks))
	}
	updated, err := server.refreshTrackMetadataFromAcoustID(context.Background(), tracks[0])
	if err != nil {
		t.Fatalf("refresh acoustid: %v", err)
	}
	if updated.Title != "Better Days" || updated.Studio != "Sweatshop Union" || updated.DurationSeconds != 201 {
		t.Fatalf("updated track = %+v", updated)
	}
	if id, ok := server.mediaProviderID(updated.ID, "acoustid", "fingerprint"); !ok || id != "acoustid-123" {
		t.Fatalf("acoustid provider id = %q ok=%v", id, ok)
	}
	if id, ok := server.mediaProviderID(updated.ID, "musicbrainz", "recording"); !ok || id != "rec-fp" {
		t.Fatalf("recording provider id = %q ok=%v", id, ok)
	}
	var stored string
	if err := server.db.QueryRow(`SELECT recording_id FROM audio_fingerprints WHERE media_id = ?`, updated.ID).Scan(&stored); err != nil || stored != "rec-fp" {
		t.Fatalf("stored recording id = %q err=%v", stored, err)
	}
}

func TestLibraryScannerRejectsSymlinkEscape(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "Outside.mp4")
	if err := os.WriteFile(outsideFile, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "Outside.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if result.FilesIndexed != 0 {
		t.Fatalf("indexed escaped symlink files = %d, expected 0", result.FilesIndexed)
	}
}

func TestAutomaticScannerQueuesEnabledLibraries(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Auto Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"scanAutomatically": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	server.queueAutomaticLibraryScans()

	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_id = ?`, library.ID).Scan(&count); err != nil {
		t.Fatalf("count scan jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("automatic scan jobs = %d, expected 1", count)
	}
}

func TestLibraryChangeWatcherQueuesScanWhenFingerprintChanges(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Watched Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"scanAutomatically": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	server.runLibraryChangeCheck(context.Background(), Job{
		ID:           "change_baseline",
		Type:         "library_change_check",
		ResourceType: "library",
		ResourceID:   library.ID,
		Metadata:     map[string]string{"queueChanges": "false"},
	})
	if err := os.WriteFile(filepath.Join(root, "New Movie.mp4"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	server.runLibraryChangeCheck(context.Background(), Job{
		ID:           "change_detect",
		Type:         "library_change_check",
		ResourceType: "library",
		ResourceID:   library.ID,
		Metadata:     map[string]string{"queueChanges": "true"},
	})

	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_id = ? AND message LIKE 'Filesystem change%'`, library.ID).Scan(&count); err != nil {
		t.Fatalf("count scan jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("filesystem change scan jobs = %d, expected 1", count)
	}
}

func TestLibraryFilesystemFingerprintHonorsMediaFileBudget(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	for _, name := range []string{"One.mp4", "Two.mp4", "Three.mp4"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("not real video"), 0o600); err != nil {
			t.Fatalf("write media file: %v", err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Budget", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	if _, err := server.libraryFilesystemFingerprintWithLimit(context.Background(), library, 2); !errors.Is(err, errLibraryFingerprintBudgetExceeded) {
		t.Fatalf("fingerprint error = %v, expected budget exceeded", err)
	}
	if _, err := server.libraryFilesystemFingerprintWithLimit(context.Background(), library, 3); err != nil {
		t.Fatalf("fingerprint at budget: %v", err)
	}
}

func TestLibraryChangeCheckQueuesScanWhenUnindexedFilesystemExceedsProbeBudget(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	originalLimit := scannerFingerprintMediaFileLimit
	scannerFingerprintMediaFileLimit = 2
	t.Cleanup(func() {
		scannerFingerprintMediaFileLimit = originalLimit
	})
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Huge Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"scanAutomatically": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	jobID := "change_budget"
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, leased_by, lease_expires_at, created_at, updated_at)
		VALUES (?, 'library_change_check', 'running', 20, '', 'library', ?, ?, ?, ?, ?, ?)`,
		jobID, library.ID, `{"queueChanges":"true"}`, server.jobLeaseOwner(jobID), time.Now().UTC().Add(30*time.Minute).Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	for i := 0; i <= scannerFingerprintMediaFileLimit; i++ {
		name := fmt.Sprintf("Movie %05d.mp4", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("not real video"), 0o600); err != nil {
			t.Fatalf("write media file: %v", err)
		}
	}

	server.runLibraryChangeCheck(context.Background(), Job{
		ID:           jobID,
		Type:         "library_change_check",
		ResourceType: "library",
		ResourceID:   library.ID,
		Metadata:     map[string]string{"queueChanges": "true"},
	})

	var status, message string
	var attemptCount int
	if err := server.db.QueryRow(`SELECT status, message, attempt_count FROM jobs WHERE id = ?`, jobID).Scan(&status, &message, &attemptCount); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "complete" {
		t.Fatalf("status = %q, expected complete", status)
	}
	if !strings.Contains(message, "scan queued") {
		t.Fatalf("message = %q, expected a scan to establish the durable large-library index", message)
	}
	if attemptCount != 0 {
		t.Fatalf("attempt_count = %d, expected no retry scheduling", attemptCount)
	}
}

func TestLibraryChangeCheckProbesKnownLargeLibraryWithoutFilesystemWalk(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	originalLimit := scannerFingerprintMediaFileLimit
	scannerFingerprintMediaFileLimit = 1
	t.Cleanup(func() {
		scannerFingerprintMediaFileLimit = originalLimit
	})
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Known Huge Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"scanAutomatically": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 2; i++ {
		mediaID := fmt.Sprintf("known_huge_%d", i)
		if _, err := server.db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at)
			VALUES (?, ?, 'movie', ?, ?, ?, ?)`,
			mediaID, library.ID, mediaID, mediaID, "file:///missing/"+mediaID+".mp4", now); err != nil {
			t.Fatalf("insert media %s: %v", mediaID, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at)
			VALUES (?, ?, ?, ?, 'local', 1, 1024, ?, ?)`,
			mediaID+"_file", mediaID, library.ID, "/missing/"+mediaID+".mp4", now, now); err != nil {
			t.Fatalf("insert media file %s: %v", mediaID, err)
		}
	}
	jobID := "change_known_large"
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, leased_by, lease_expires_at, created_at, updated_at)
		VALUES (?, 'library_change_check', 'running', 20, '', 'library', ?, ?, ?, ?, ?, ?)`,
		jobID, library.ID, `{"queueChanges":"true"}`, server.jobLeaseOwner(jobID), time.Now().UTC().Add(30*time.Minute).Format(time.RFC3339), now, now); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	server.runLibraryChangeCheck(context.Background(), Job{
		ID:           jobID,
		Type:         "library_change_check",
		ResourceType: "library",
		ResourceID:   library.ID,
		Metadata:     map[string]string{"queueChanges": "true"},
	})

	var status, message string
	if err := server.db.QueryRow(`SELECT status, message FROM jobs WHERE id = ?`, jobID).Scan(&status, &message); err != nil {
		t.Fatalf("query job: %v", err)
	}
	if status != "complete" {
		t.Fatalf("status = %q, expected complete", status)
	}
	if !strings.Contains(message, "scan queued") {
		t.Fatalf("message = %q, expected indexed-path probe to detect missing files", message)
	}
	var scanJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_id = ?`, library.ID).Scan(&scanJobs); err != nil {
		t.Fatalf("count scan jobs: %v", err)
	}
	if scanJobs != 1 {
		t.Fatalf("library_scan jobs = %d, expected one bounded recovery scan", scanJobs)
	}
}

func TestLibraryChangeWatcherRespectsGlobalSetting(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Watched Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"scanAutomatically": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'library'`, `{"scanOnFilesystemChanges":false,"emptyTrashAfterScan":false,"allowMediaDeletion":false}`); err != nil {
		t.Fatalf("save library settings: %v", err)
	}
	server.refreshLibraryWatchFingerprints(context.Background(), false)
	if err := os.WriteFile(filepath.Join(root, "New Movie.mp4"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	server.refreshLibraryWatchFingerprints(context.Background(), true)

	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_change_check' AND resource_id = ?`, library.ID).Scan(&count); err != nil {
		t.Fatalf("count change check jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("filesystem change check jobs = %d, expected disabled watcher to queue none", count)
	}
}

func TestLibraryChangeWatcherThrottlesRecentlyQueuedChecks(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Watched Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"scanAutomatically": true,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	server.refreshLibraryWatchFingerprints(context.Background(), false)
	if _, err := server.db.Exec(`UPDATE jobs SET status = 'complete' WHERE type = 'library_change_check' AND resource_id = ?`, library.ID); err != nil {
		t.Fatalf("complete first change check: %v", err)
	}
	server.refreshLibraryWatchFingerprints(context.Background(), true)

	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_change_check' AND resource_id = ?`, library.ID).Scan(&count); err != nil {
		t.Fatalf("count change checks: %v", err)
	}
	if count != 1 {
		t.Fatalf("filesystem change check jobs = %d, expected recently queued check to be throttled", count)
	}
}

func TestDeleteMediaItemRemovesRecordAndMovesFilesToTrash(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Delete Me.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Delete Me'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("media count = %d, expected 1", len(items))
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'library'`, `{"scanOnFilesystemChanges":true,"emptyTrashAfterScan":false,"allowMediaDeletion":true}`); err != nil {
		t.Fatalf("save library settings: %v", err)
	}

	result, err := server.deleteMediaItem(items[0].ID, DeleteMediaRequest{DeleteFiles: true})
	if err != nil {
		t.Fatalf("delete media: %v", err)
	}
	if result.DeletedItems != 1 || result.TrashedFiles != 1 {
		t.Fatalf("delete result = %+v", result)
	}
	if _, err := os.Stat(mediaPath); !os.IsNotExist(err) {
		t.Fatalf("source file still exists or stat failed unexpectedly: %v", err)
	}
	trashed, err := filepath.Glob(filepath.Join(server.cfg.AppDataDir, "media-trash", "*", "*Delete Me.mp4"))
	if err != nil || len(trashed) != 1 {
		t.Fatalf("trashed files = %v, err = %v", trashed, err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id = ?`, items[0].ID).Scan(&count); err != nil {
		t.Fatalf("count media: %v", err)
	}
	if count != 0 {
		t.Fatalf("media rows remaining = %d, expected 0", count)
	}
}

func TestDeleteMediaItemRejectsFileDeletionWhenDisabled(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Keep Me.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Keep Me'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media: %v", err)
	}

	if _, err := server.deleteMediaItem(items[0].ID, DeleteMediaRequest{DeleteFiles: true}); err == nil {
		t.Fatalf("expected file deletion to be rejected when disabled")
	}
	if _, err := os.Stat(mediaPath); err != nil {
		t.Fatalf("source file should remain: %v", err)
	}
}

func TestLibraryScannerMarksMissingFilesUntilTrashIsEmptied(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Gone Tomorrow.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if err := os.Remove(mediaPath); err != nil {
		t.Fatalf("remove media: %v", err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatalf("rescan library: %v", err)
	}
	if result.MissingMarked != 1 {
		t.Fatalf("missing marked = %d, expected 1", result.MissingMarked)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Gone Tomorrow'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("media count = %d, expected 1", len(items))
	}
	if !items[0].Missing || items[0].SourceURL != "" || items[0].FileCount != 1 || items[0].MissingFileCount != 1 {
		t.Fatalf("missing item = %+v", items[0])
	}
	removed, err := server.emptyMissingMediaTrash(0)
	if err != nil {
		t.Fatalf("empty trash: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, expected 1", removed)
	}
	items, err = server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Gone Tomorrow'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media after trash: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("media count after trash = %d, expected 0", len(items))
	}
}

func TestLibraryScannerReconcilesMissingFilesInBatches(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	oldGeneration := "scan_old"
	currentGeneration := "scan_current"
	now := time.Now().UTC().Format(time.RFC3339)
	totalMissing := scannerReconcileBatchSize + 7
	for i := 0; i < totalMissing; i++ {
		mediaID := fmt.Sprintf("batch_missing_%03d", i)
		sourceURL := "file:///missing/" + mediaID + ".mp4"
		if _, err := server.db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at)
			VALUES (?, ?, 'movie', ?, ?, ?, ?)`,
			mediaID, library.ID, mediaID, mediaID, sourceURL, now); err != nil {
			t.Fatalf("insert media %s: %v", mediaID, err)
		}
		if _, err := server.db.Exec(`
			INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at, scan_generation)
			VALUES (?, ?, ?, ?, 'local', 1, 1024, ?, ?, ?)`,
			mediaID+"_file", mediaID, library.ID, sourceURL, now, now, oldGeneration); err != nil {
			t.Fatalf("insert media file %s: %v", mediaID, err)
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at)
		VALUES ('batch_current', ?, 'movie', 'Current', 'Current', 'file:///current.mp4', ?)`,
		library.ID, now); err != nil {
		t.Fatalf("insert current media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_files (id, media_id, library_id, path, source_type, available, size_bytes, first_seen_at, last_seen_at, scan_generation)
		VALUES ('batch_current_file', 'batch_current', ?, 'file:///current.mp4', 'local', 1, 1024, ?, ?, ?)`,
		library.ID, now, now, currentGeneration); err != nil {
		t.Fatalf("insert current media file: %v", err)
	}

	missing, err := server.reconcileScannedMedia(context.Background(), library.ID, now, currentGeneration)
	if err != nil {
		t.Fatalf("reconcile scanned media: %v", err)
	}
	if missing != totalMissing {
		t.Fatalf("missing marked = %d, expected %d", missing, totalMissing)
	}
	var staleAvailable int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_files WHERE library_id = ? AND scan_generation = ? AND available = 1`, library.ID, oldGeneration).Scan(&staleAvailable); err != nil {
		t.Fatalf("count stale available files: %v", err)
	}
	if staleAvailable != 0 {
		t.Fatalf("stale available files = %d, expected 0", staleAvailable)
	}
	var currentAvailable int
	if err := server.db.QueryRow(`SELECT available FROM media_files WHERE id = 'batch_current_file'`).Scan(&currentAvailable); err != nil {
		t.Fatalf("query current file: %v", err)
	}
	if currentAvailable != 1 {
		t.Fatalf("current file available = %d, expected 1", currentAvailable)
	}
}

func TestLibraryScannerGroupsMultipleFilesUnderOneItem(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	first := filepath.Join(root, "Arrival Window 1080p.mkv")
	second := filepath.Join(root, "Arrival Window 720p.mp4")
	if err := os.WriteFile(first, []byte("not real video with more bytes"), 0o600); err != nil {
		t.Fatalf("write first media: %v", err)
	}
	if err := os.WriteFile(second, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write second media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Arrival Window'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query media: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("media count = %d, expected 1", len(items))
	}
	if items[0].FileCount != 2 || items[0].MissingFileCount != 0 || items[0].Missing {
		t.Fatalf("grouped item = %+v", items[0])
	}
}

func TestLibraryScannerIndexesMovieExtrasAsChildren(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	trailerDir := filepath.Join(root, "Arrival Window", "Trailers")
	if err := os.MkdirAll(trailerDir, 0o700); err != nil {
		t.Fatalf("create trailer dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(trailerDir, "Official Trailer.mp4"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write trailer: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}

	parents, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Arrival Window'`, []any{library.ID})
	if err != nil {
		t.Fatalf("query parent: %v", err)
	}
	if len(parents) != 1 {
		t.Fatalf("movie parent count = %d, expected 1", len(parents))
	}
	children, err := server.childrenFor("", parents[0])
	if err != nil {
		t.Fatalf("load children: %v", err)
	}
	if len(children) != 1 || children[0].Type != "extra" || children[0].ArtSeed != "trailer" {
		t.Fatalf("extras children = %#v", children)
	}
}

func TestCinemaPrerollSelectsScannedTrailerForFreshMovie(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	movieDir := filepath.Join(root, "Arrival Window")
	trailerDir := filepath.Join(movieDir, "Trailers")
	if err := os.MkdirAll(trailerDir, 0o700); err != nil {
		t.Fatalf("create trailer dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "Arrival Window.mp4"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write movie: %v", err)
	}
	if err := os.WriteFile(filepath.Join(trailerDir, "Official Trailer.mp4"), []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write trailer: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan library: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'extras'`, `{"cinemaTrailers":1}`); err != nil {
		t.Fatalf("save extras settings: %v", err)
	}
	movies, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'movie' AND m.title = 'Arrival Window' AND m.source_url <> ''`, []any{library.ID})
	if err != nil {
		t.Fatalf("query movie: %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("movie count = %d, expected 1", len(movies))
	}
	trailer, ok := server.cinemaPrerollFor("", movies[0])
	if !ok || trailer.Type != "extra" || trailer.ArtSeed != "trailer" {
		t.Fatalf("preroll = %+v, ok=%v", trailer, ok)
	}
}

func TestScannerTestServerCleanupHasNoAutomaticJobWake(t *testing.T) {
	server := newScannerTestServer(t)
	server.testAfterDatabaseClose = func() {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, err := os.Stat(server.cfg.DatabasePath + suffix); err == nil {
				t.Errorf("scanner test database sidecar remains after cleanup: %s", suffix)
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat scanner test database sidecar %s: %v", suffix, err)
			}
		}
	}
	if _, err := server.createJobFor("tmdb_trending_refresh", "Queued test job.", "discovery", "all:day"); err != nil {
		t.Fatalf("queue test job: %v", err)
	}
}

func newScannerTestServer(t *testing.T) *Server {
	t.Helper()
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	backupDir := filepath.Join(appDataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("create scanner backup directory: %v", err)
	}
	cfg := config.Config{
		Addr:           "127.0.0.1:0",
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		BackupDir:      backupDir,
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	server := &Server{
		cfg:            cfg,
		db:             db,
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		logSubscribers: map[chan LogEvent]bool{},
		scannerWatch:   map[string]string{},
		transcodes:     map[string]*transcodeSession{},
	}
	// Scanner tests that need job execution call runDueJobsOnce explicitly. A
	// no-op hook keeps queued-job assertions from launching detached work before
	// the helper's database cleanup runs; production scheduling remains unchanged.
	server.jobWakeForTests = func() {}
	t.Cleanup(func() {
		// A completed scan queues a library read-model repair. In this inert test
		// server there is no long-lived job worker, so the repair uses the same
		// registered one-shot owned-async path used during generation transitions.
		// Seal admission and join that writer before closing SQLite; otherwise it
		// can recreate a WAL/SHM sidecar while testing.TempDir removes appDataDir.
		server.BeginShutdown()
		server.closeOwnedAsync()
		server.backgroundWG.Wait()
		if err := db.Close(); err != nil {
			t.Errorf("close scanner test database: %v", err)
		}
		if server.testAfterDatabaseClose != nil {
			server.testAfterDatabaseClose()
		}
	})
	return server
}
