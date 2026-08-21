package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func TestRestoreMaintenanceHandlerServesWebShellAssetsAndStatusCapability(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(filepath.Join(web, "assets"), 0o755); err != nil {
		t.Fatalf("create web directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>restore shell</html>"), 0o644); err != nil {
		t.Fatalf("write web shell: %v", err)
	}
	if err := os.WriteFile(filepath.Join(web, "assets", "restore.js"), []byte("restore asset"), 0o644); err != nil {
		t.Fatalf("write web asset: %v", err)
	}
	server := &Server{cfg: config.Config{AppDataDir: root, WebDistDir: web}}
	if err := database.PreparePrivateDataPaths(server.cfg); err != nil {
		t.Fatalf("prepare restore paths: %v", err)
	}
	operation := database.RestoreOperation{
		Version:         database.RestoreOperationVersion,
		OperationID:     "restore-maintenance-test",
		StatusTokenHash: hashToken("status-secret"),
		Phase:           database.RestorePhaseInstalling,
		State:           database.RestorePhaseInstalling,
		Progress:        60,
	}
	if err := database.WriteRestoreOperation(root, operation); err != nil {
		t.Fatalf("write restore marker: %v", err)
	}
	handler := server.RestoreMaintenanceHandler()

	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if rootResponse.Code != http.StatusOK || !strings.Contains(rootResponse.Body.String(), "restore shell") {
		t.Fatalf("maintenance root response = %d %q", rootResponse.Code, rootResponse.Body.String())
	}
	for _, header := range []string{"X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options", "Content-Security-Policy", "Cache-Control", "Pragma", "X-Request-ID"} {
		if rootResponse.Header().Get(header) == "" {
			t.Fatalf("maintenance root missing security/transport header %s", header)
		}
	}
	if rootResponse.Header().Get("X-Content-Type-Options") != "nosniff" || rootResponse.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(rootResponse.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("maintenance root security headers are not equivalent to normal chain: %#v", rootResponse.Header())
	}

	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, httptest.NewRequest(http.MethodGet, "/assets/restore.js", nil))
	if assetResponse.Code != http.StatusOK || assetResponse.Body.String() != "restore asset" {
		t.Fatalf("maintenance asset response = %d %q", assetResponse.Code, assetResponse.Body.String())
	}
	if assetResponse.Header().Get("X-Content-Type-Options") != "nosniff" || assetResponse.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("maintenance asset lost security headers: %#v", assetResponse.Header())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/backups/restore/restore-maintenance-test", nil)
	statusRequest.Header.Set(restoreStatusHeader, "status-secret")
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), operation.OperationID) {
		t.Fatalf("maintenance status response = %d %q", statusResponse.Code, statusResponse.Body.String())
	}
	if statusResponse.Header().Get("X-Content-Type-Options") != "nosniff" || statusResponse.Header().Get("X-Frame-Options") != "DENY" || statusResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("maintenance status lost security/cache headers: %#v", statusResponse.Header())
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/backups/restore/restore-maintenance-test", nil)
	preflight.Host = "server.test"
	preflight.Header.Set("Origin", "http://server.test")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflight.Header.Set("Access-Control-Request-Headers", "X-Portico-Restore-Status")
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || preflightResponse.Header().Get("Access-Control-Allow-Origin") != "http://server.test" || !strings.Contains(preflightResponse.Header().Get("Access-Control-Allow-Headers"), restoreStatusHeader) {
		t.Fatalf("allowed status preflight = %d headers=%#v", preflightResponse.Code, preflightResponse.Header())
	}
	disallowed := httptest.NewRequest(http.MethodOptions, "/api/backups/restore/restore-maintenance-test", nil)
	disallowed.Host = "server.test"
	disallowed.Header.Set("Origin", "https://untrusted.example")
	disallowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(disallowedResponse, disallowed)
	if disallowedResponse.Code != http.StatusForbidden {
		t.Fatalf("disallowed status preflight = %d, want 403", disallowedResponse.Code)
	}

	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/system/diagnostics", nil))
	if apiResponse.Code != http.StatusServiceUnavailable || !strings.Contains(apiResponse.Body.String(), "restore_in_progress") {
		t.Fatalf("maintenance normal API response = %d %q", apiResponse.Code, apiResponse.Body.String())
	}
}

func TestStartBackgroundRejectsRegistrationAfterBeginShutdown(t *testing.T) {
	background, cancel := context.WithCancel(context.Background())
	server := &Server{
		backgroundCtx:    background,
		backgroundCancel: cancel,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	started := make(chan struct{})
	release := make(chan struct{})
	if !server.startBackground("shutdown-race-test", func(context.Context) {
		close(started)
		<-release
	}) {
		t.Fatal("initial background worker was rejected")
	}
	<-started
	beginDone := make(chan struct{})
	go func() {
		server.BeginShutdown()
		close(beginDone)
	}()
	select {
	case <-beginDone:
	case <-time.After(time.Second):
		t.Fatal("BeginShutdown did not close background admission")
	}
	if server.startBackground("late-worker", func(context.Context) {}) {
		t.Fatal("late background registration crossed the shutdown join boundary")
	}
	close(release)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown after guarded join: %v", err)
	}
}

func TestUploadedRestorePublishesMarkerOnlyAfterOwnerLock(t *testing.T) {
	root := t.TempDir()
	server := &Server{cfg: config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}}
	if err := database.PreparePrivateDataPaths(server.cfg); err != nil {
		t.Fatalf("prepare restore paths: %v", err)
	}
	entered := make(chan struct{})
	continueReservation := make(chan struct{})
	restoreHook := setRestoreUploadReservationAfterOwnerLockForTest(func() {
		close(entered)
		<-continueReservation
	})
	defer restoreHook()
	type result struct {
		operation database.RestoreOperation
		release   func()
		err       error
	}
	resultCh := make(chan result, 1)
	go func() {
		operation, _, release, err := server.reserveUploadedRestore(User{ID: "owner", AccountID: "owner"}, "session")
		resultCh <- result{operation: operation, release: release, err: err}
	}()
	<-entered
	markerPath := database.RestoreOperationPath(root)
	if _, err := os.Lstat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("reservation marker became visible before the owner lock boundary: %v", err)
	}
	if err := database.RecoverInterruptedRestoreBeforeOpen(server.cfg); err != nil {
		t.Fatalf("recovery raced owner-lock boundary: %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("recovery created or observed a marker before publication: %v", err)
	}
	close(continueReservation)
	resultValue := <-resultCh
	if resultValue.err != nil {
		t.Fatalf("reserve uploaded restore: %v", resultValue.err)
	}
	if resultValue.release == nil || resultValue.operation.UploadOwnerLockPath == "" {
		t.Fatal("reservation did not return its held owner lock")
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("published marker missing after owner lock: %v", err)
	}
	resultValue.release()
	_ = os.Remove(resultValue.operation.UploadOwnerLockPath)
	_ = os.Remove(markerPath)
}

func TestDatabaseBackupCreatesRestrictedBackupFile(t *testing.T) {
	server := newScannerTestServer(t)
	path, err := server.createDatabaseBackup()
	if err != nil {
		t.Fatalf("create database backup: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Fatalf("backup is not a non-empty file: %s", path)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions = %o, expected 0600", info.Mode().Perm())
	}
	if filepath.Dir(path) != filepath.Join(server.cfg.AppDataDir, "backups") {
		t.Fatalf("backup path %s is outside app-data backups directory", path)
	}
	manifestPath := backupManifestPath(path)
	manifestInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat backup manifest: %v", err)
	}
	if manifestInfo.Mode().Perm() != 0o600 {
		t.Fatalf("backup manifest permissions = %o, expected 0600", manifestInfo.Mode().Perm())
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read backup manifest: %v", err)
	}
	var manifest databaseBackupManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("parse backup manifest: %v", err)
	}
	if manifest.BackupName != filepath.Base(path) || manifest.ChecksumSHA256 == "" || manifest.DatabaseFormatVersion == 0 || manifest.MigrationHead == "" {
		t.Fatalf("backup manifest missing recovery evidence: %#v", manifest)
	}
}

func TestDatabaseBackupRejectsExternalStorageBeforeCreatingExposedArtifact(t *testing.T) {
	server := newScannerTestServer(t)
	external := t.TempDir()
	server.cfg.BackupDir = external
	if _, err := server.createDatabaseBackup(); err == nil || !strings.Contains(err.Error(), "external storage") {
		t.Fatalf("external backup policy error=%v", err)
	}
	entries, err := os.ReadDir(external)
	if err != nil {
		t.Fatalf("read external backup root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("external backup policy left artifacts behind: %v", entries)
	}
}

func TestDatabaseBackupPublicationWarningKeepsCatalogIdentityTruthful(t *testing.T) {
	server := newScannerTestServer(t)
	backupDir := server.backupDir()
	calls := 0
	restoreSync := setBackupSyncDirectoryForTest(func(path string) error {
		if filepath.Clean(path) == filepath.Clean(backupDir) {
			calls++
			if calls == 3 {
				return errors.New("injected final directory sync failure")
			}
		}
		return database.SyncDirectory(path)
	})
	defer restoreSync()
	postResponse := httptest.NewRecorder()
	server.handleBackups(postResponse, httptest.NewRequest(http.MethodPost, "/api/backups", nil), restoreOwnerUser())
	if postResponse.Code != http.StatusCreated {
		t.Fatalf("backup POST status=%d body=%s", postResponse.Code, postResponse.Body.String())
	}
	var posted BackupInfo
	if err := json.Unmarshal(postResponse.Body.Bytes(), &posted); err != nil {
		t.Fatalf("decode backup POST: %v", err)
	}
	backupPath := filepath.Join(backupDir, posted.Name)
	if posted.Name == "" || posted.PublicationState != "degraded" || posted.WarningCode != "backup_directory_sync_pending" || !posted.RestoreReady {
		t.Fatalf("backup POST lost degraded publication truth: %#v", posted)
	}
	// Reconstruct the catalog through a fresh server instance to prove that
	// publication truth survives the POST response and is not just an in-memory
	// overlay on the creating generation.
	reloaded := &Server{cfg: server.cfg}
	getResponse := httptest.NewRecorder()
	reloaded.handleBackups(getResponse, httptest.NewRequest(http.MethodGet, "/api/backups", nil), restoreOwnerUser())
	if getResponse.Code != http.StatusOK {
		t.Fatalf("reloaded backup GET status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	var listed ListResponse[BackupInfo]
	if err := json.Unmarshal(getResponse.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode reloaded backup GET: %v", err)
	}
	backups := listed.Items
	found := false
	for _, candidate := range backups {
		if candidate.Name == filepath.Base(backupPath) && candidate.RestoreReady && candidate.PublicationState == "degraded" && candidate.WarningCode == posted.WarningCode {
			found = true
		}
	}
	if !found {
		t.Fatalf("degraded published backup was not catalogued: %#v", backups)
	}
}

func TestBackupDebrisRetentionPrunesQuarantinedArtifact(t *testing.T) {
	server := newScannerTestServer(t)
	backupPath := filepath.Join(server.backupDir(), "portico-debris-test.db.partial")
	if err := os.WriteFile(backupPath, []byte("partial backup debris"), 0o600); err != nil {
		t.Fatalf("write partial backup: %v", err)
	}
	removeErr := errors.New("injected remove failure")
	restoreRemove := setBackupRemoveForTest(func(path string) error {
		if filepath.Clean(path) == filepath.Clean(backupPath) {
			return removeErr
		}
		return os.Remove(path)
	})
	cleanupErr := cleanupFailedBackupPair(backupPath)
	restoreRemove()
	if !errors.Is(cleanupErr, removeErr) {
		t.Fatalf("cleanup error=%v, want injected remove failure", cleanupErr)
	}
	entries, err := os.ReadDir(server.backupDir())
	if err != nil {
		t.Fatalf("read backup debris directory: %v", err)
	}
	var debrisPath string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "portico-debris-test.db.partial.restore-debris-") {
			debrisPath = filepath.Join(server.backupDir(), entry.Name())
			break
		}
	}
	if debrisPath == "" {
		t.Fatalf("failed cleanup did not quarantine the partial artifact; entries=%v", entries)
	}
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(debrisPath, old, old); err != nil {
		t.Fatalf("age quarantined debris: %v", err)
	}
	if _, err := server.pruneDatabaseBackups(30); err != nil {
		t.Fatalf("prune quarantined debris: %v", err)
	}
	if _, err := os.Lstat(debrisPath); !os.IsNotExist(err) {
		t.Fatalf("quarantined debris remains after bounded prune: %v", err)
	}
}

func TestDatabaseBackupPublicationEvidenceClearFailureRemainsDegradedAfterReload(t *testing.T) {
	server := newScannerTestServer(t)
	restoreRemove := setBackupRemoveForTest(func(path string) error {
		if strings.HasSuffix(path, ".publication.json") {
			return errors.New("injected publication evidence clear failure")
		}
		return os.Remove(path)
	})
	defer restoreRemove()
	backupPath, err := server.createDatabaseBackup()
	var warning *BackupPublicationWarning
	if !errors.As(err, &warning) {
		t.Fatalf("create backup error=%v, want publication warning", err)
	}
	if backupPath == "" {
		t.Fatal("publication warning lost backup identity")
	}
	if _, err := os.Stat(backupPublicationEvidencePath(backupPath)); err != nil {
		t.Fatalf("failed evidence clear removed the conservative marker: %v", err)
	}
	reloaded := &Server{cfg: server.cfg}
	info, err := reloaded.backupInfo(backupPath)
	if err != nil {
		t.Fatalf("reload degraded backup info: %v", err)
	}
	if !info.RestoreReady || info.PublicationState != "degraded" || info.WarningCode != warning.Code {
		t.Fatalf("reload lost conservative publication state: %#v", info)
	}
}

func TestDatabaseBackupPublicationEvidencePersistenceFailureDoesNotPublishUnmarkedDatabase(t *testing.T) {
	server := newScannerTestServer(t)
	restoreEvidence := setBackupPublicationEvidenceForTest(func(string, []byte) error {
		return errors.New("injected publication evidence persistence failure")
	})
	defer restoreEvidence()
	if _, err := server.createDatabaseBackup(); err == nil {
		t.Fatal("evidence persistence failure unexpectedly published a backup")
	}
	entries, err := os.ReadDir(server.backupDir())
	if err != nil {
		t.Fatalf("read backup directory after evidence failure: %v", err)
	}
	for _, entry := range entries {
		if isSafeBackupName(entry.Name()) {
			t.Fatalf("evidence persistence failure left enumerable database %q", entry.Name())
		}
	}
	backups, err := server.listBackups()
	if err != nil {
		t.Fatalf("list backups after evidence failure: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("evidence persistence failure left catalog entries: %#v", backups)
	}
}

func TestDatabaseBackupVacuumENOSPCCleansPartialArtifact(t *testing.T) {
	server := newScannerTestServer(t)
	restoreVacuum := setBackupVacuumForTest(func(path string) error {
		if err := os.WriteFile(path, []byte(strings.Repeat("partial", 1024)), 0o600); err != nil {
			return err
		}
		return errors.New("injected ENOSPC during VACUUM INTO")
	})
	defer restoreVacuum()
	if _, err := server.createDatabaseBackup(); err == nil {
		t.Fatal("injected VACUUM ENOSPC unexpectedly succeeded")
	}
	entries, err := os.ReadDir(server.backupDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".partial") || strings.Contains(entry.Name(), ".restore-debris-") {
			t.Fatalf("failed VACUUM left unbounded backup debris %q", entry.Name())
		}
	}
}

func TestPruneDatabaseBackupsBoundsRecentInvalidCandidatesAndCleansPairs(t *testing.T) {
	server := newScannerTestServer(t)
	backupDir := server.backupDir()
	now := time.Now().UTC()
	for index := 0; index < 40; index++ {
		path := filepath.Join(backupDir, fmt.Sprintf("portico-invalid-%02d.db", index))
		if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := server.pruneDatabaseBackups(30)
	if err != nil {
		t.Fatalf("prune recent invalid candidates: %v", err)
	}
	if removed != 8 {
		t.Fatalf("recent invalid candidates removed=%d, want count bound of 8", removed)
	}
	remaining, err := server.listBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 32 {
		t.Fatalf("recent invalid candidates remaining=%d, want 32", len(remaining))
	}
	for _, info := range remaining {
		if info.RestoreReady || info.ValidationCode == "" {
			t.Fatalf("invalid candidate lost truthful catalog state: %#v", info)
		}
	}

	oldPath := filepath.Join(backupDir, "portico-old-invalid.db")
	if err := os.WriteFile(oldPath, []byte("old invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}
	manifestOnly := backupManifestPath(filepath.Join(backupDir, "portico-manifest-only.db"))
	if err := os.WriteFile(manifestOnly, []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(manifestOnly, old, old); err != nil {
		t.Fatal(err)
	}
	removed, err = server.pruneDatabaseBackups(30)
	if err != nil {
		t.Fatalf("prune expired invalid/orphan candidates: %v", err)
	}
	if removed < 2 {
		t.Fatalf("expired invalid/orphan candidates removed=%d, want at least 2", removed)
	}
	for _, path := range []string{oldPath, manifestOnly} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expired debris remains %s: %v", path, statErr)
		}
	}
}

func TestPruneDatabaseBackupsPreservesLiveReservationAndCleansStaleReservation(t *testing.T) {
	server := newScannerTestServer(t)
	backupDir := server.backupDir()
	reservation := filepath.Join(backupDir, "portico-live.reserve")
	if err := os.MkdirAll(reservation, 0o700); err != nil {
		t.Fatal(err)
	}
	ownerLock := filepath.Join(reservation, "owner.lock")
	release, err := database.AcquireRestoreArtifactLock(ownerLock)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := os.Chtimes(reservation, old, old); err != nil {
		release()
		t.Fatal(err)
	}
	if _, err := server.pruneDatabaseBackups(1); err != nil {
		release()
		t.Fatalf("prune live reservation: %v", err)
	}
	if _, err := os.Stat(reservation); err != nil {
		release()
		t.Fatalf("live reservation was removed: %v", err)
	}
	release()
	if _, err := server.pruneDatabaseBackups(1); err != nil {
		t.Fatalf("prune stale reservation: %v", err)
	}
	if _, err := os.Stat(reservation); !os.IsNotExist(err) {
		t.Fatalf("stale reservation remains: %v", err)
	}
}

func TestBackupInfoRejectsManifestChecksumMismatch(t *testing.T) {
	server := newScannerTestServer(t)
	path, err := server.createDatabaseBackup()
	if err != nil {
		t.Fatalf("create database backup: %v", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("open backup for tamper: %v", err)
	}
	if _, err := file.WriteString("tamper"); err != nil {
		_ = file.Close()
		t.Fatalf("tamper backup: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close tampered backup: %v", err)
	}
	info, err := server.backupInfo(path)
	if err != nil {
		t.Fatalf("backup info: %v", err)
	}
	if info.Integrity != "invalid" || info.RestoreReady {
		t.Fatalf("tampered backup should be invalid: %#v", info)
	}
	manifest, err := database.ReadBackupManifest(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if _, err := database.ValidateRestoreCandidate(context.Background(), path, &manifest); err == nil {
		t.Fatalf("expected tampered backup restore staging to fail")
	}
}

func TestSupervisedRestoreResponseIsPathFreeAndMarkerIsPrivate(t *testing.T) {
	server := newScannerTestServer(t)
	backupPath, err := server.createDatabaseBackup()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	manifest, err := database.ReadBackupManifest(backupPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	response, ok := server.createRestoreOperation(&httptest.ResponseRecorder{}, httptest.NewRequest(http.MethodPost, "/api/backups/restore", nil), User{
		ID: "restore-test", AccountID: "restore-test", Role: "owner", AuthOrigin: "local", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true},
	}, "session-test", filepath.Base(backupPath), backupPath, manifest)
	if !ok {
		t.Fatalf("stage backup failed")
	}
	if !response.OK || response.OperationID == "" || response.State != "validating" {
		t.Fatalf("unexpected path-free restore response: %#v", response)
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal restore response: %v", err)
	}
	serialized := string(body)
	for _, forbidden := range []string{server.cfg.AppDataDir, "stagedPath", "restore-pending.db", "Stop Portico"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("restore response leaked forbidden value %q: %s", forbidden, serialized)
		}
	}
	markerPath := database.RestoreOperationPath(server.cfg.AppDataDir)
	markerInfo, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("stat private restore marker: %v", err)
	}
	if markerInfo.Mode().Perm() != 0o600 {
		t.Fatalf("restore marker mode=%#o want 0600", markerInfo.Mode().Perm())
	}
	markerBody, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read restore marker: %v", err)
	}
	if !strings.Contains(string(markerBody), filepath.Join(server.cfg.AppDataDir, "restore", response.OperationID+".db")) {
		t.Fatalf("private restore marker did not retain its internal staged path: %s", markerBody)
	}
}

func TestScheduledWindowHandlesOvernightRanges(t *testing.T) {
	inWindow := time.Date(2026, 5, 3, 1, 0, 0, 0, time.UTC)
	outsideWindow := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	if !withinScheduledWindow(inWindow, 22, 5) {
		t.Fatalf("expected overnight window to include 01:00 UTC")
	}
	if withinScheduledWindow(outsideWindow, 22, 5) {
		t.Fatalf("expected overnight window to exclude 12:00 UTC")
	}
}

func TestScheduledDaysLimitWeekdayAndWeekendWindows(t *testing.T) {
	monday := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 6, 20, 2, 0, 0, 0, time.UTC)
	if !withinScheduledDays(monday, "weekdays") || withinScheduledDays(saturday, "weekdays") {
		t.Fatalf("weekday maintenance should include Monday and exclude Saturday")
	}
	if withinScheduledDays(monday, "weekends") || !withinScheduledDays(saturday, "weekends") {
		t.Fatalf("weekend maintenance should exclude Monday and include Saturday")
	}
	if !withinScheduledDays(monday, "every-day") || !withinScheduledDays(saturday, "every-day") {
		t.Fatalf("every-day maintenance should include weekdays and weekends")
	}
	if normalizeMaintenanceDays("unsupported") != "every-day" {
		t.Fatalf("unsupported maintenance day policy should normalize to every-day")
	}
}

func TestScheduledTaskCatalogAndManualRun(t *testing.T) {
	server := newScannerTestServer(t)
	tasks, err := server.listScheduledTasks()
	if err != nil {
		t.Fatalf("list scheduled tasks: %v", err)
	}
	if len(tasks) < 4 {
		t.Fatalf("expected built-in tasks, got %#v", tasks)
	}
	foundBackup := false
	for _, task := range tasks {
		if task.ID == "database-backup" && task.JobType == "database_backup" {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Fatalf("database backup task missing: %#v", tasks)
	}
	updated, err := server.updateScheduledTaskTrigger("database-backup", ScheduledTaskUpdateRequest{Enabled: boolPtr(false), IntervalHours: intPtr(12)})
	if err != nil {
		t.Fatalf("update database backup trigger: %v", err)
	}
	if updated.Trigger.Enabled || updated.Trigger.IntervalHours != 12 || updated.Enabled {
		t.Fatalf("updated database backup trigger = %#v", updated)
	}

	jobs, err := server.runScheduledTask("database-backup")
	if err != nil {
		t.Fatalf("run database backup task: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Type != "database_backup" || jobs[0].ResourceType != "database" {
		t.Fatalf("unexpected backup jobs: %#v", jobs)
	}
	if _, err := server.runScheduledTask("missing-task"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing task error, got %v", err)
	}
	if _, err := server.updateScheduledTaskTrigger("missing-task", ScheduledTaskUpdateRequest{Enabled: boolPtr(true)}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing task update error, got %v", err)
	}
}

func TestCancelQueuedJobAndClaimSkipsCancelled(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES ('job_cancel_test', 'database_backup', 'queued', 0, 'Queued backup.', 'database', 'manual', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert queued job: %v", err)
	}
	job, err := server.cancelQueuedJob("job_cancel_test")
	if err != nil {
		t.Fatalf("cancel queued job: %v", err)
	}
	if job.Status != "cancelled" || job.Progress != 100 {
		t.Fatalf("unexpected cancelled job: %#v", job)
	}
	if server.claimJobForRun("job_cancel_test") {
		t.Fatalf("cancelled job should not be claimable")
	}
	if _, err := server.cancelQueuedJob("job_cancel_test"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected not cancellable error, got %v", err)
	}

	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES ('job_running_cancel_test', 'metadata_refresh', 'running', 12, 'Refreshing metadata.', 'media', 'movie_meridian', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert running job: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET leased_by = ?, lease_expires_at = ? WHERE id = 'job_running_cancel_test'`, server.jobLeaseOwner("job_running_cancel_test"), time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("lease running job: %v", err)
	}
	ctx, done := server.registerRunningJob("job_running_cancel_test")
	defer done()
	runningJob, err := server.cancelJob("job_running_cancel_test")
	if err != nil {
		t.Fatalf("cancel running job: %v", err)
	}
	if runningJob.Status != "running" || runningJob.Phase != "cancelling" || runningJob.CancellationRequestedAt == "" {
		t.Fatalf("unexpected running cancellation job: %#v", runningJob)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("running job context was not cancelled")
	}
	if err := server.setJobMessage("job_running_cancel_test", "complete", 100, "Should not overwrite cancellation."); err != nil {
		t.Fatalf("set cancelled job message: %v", err)
	}
	runningJob, err = server.getJob("job_running_cancel_test")
	if err != nil {
		t.Fatalf("reload running cancellation job: %v", err)
	}
	if runningJob.Status != "running" || runningJob.Phase != "cancelling" || strings.Contains(runningJob.Message, "overwrite") {
		t.Fatalf("cancelling job was terminalized or overwritten: %#v", runningJob)
	}
	server.finalizeRequestedJobCancellation("job_running_cancel_test")
	runningJob, err = server.getJob("job_running_cancel_test")
	if err != nil {
		t.Fatalf("reload acknowledged cancellation job: %v", err)
	}
	if runningJob.Status != "cancelled" || runningJob.WorkerAcknowledgedAt == "" {
		t.Fatalf("worker acknowledgement did not terminalize cancellation: %#v", runningJob)
	}
}

func TestCancelJobsByTypeCancelsQueuedAndRunningAnalysis(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES
			('job_analysis_cancel_queued', 'media_analyze', 'queued', 0, 'Queued analysis.', 'media', 'movie_meridian', ?, ?),
			('job_analysis_cancel_running', 'media_analyze', 'running', 23, 'Running analysis.', 'media', 'movie_cosmos', ?, ?),
			('job_backup_not_cancelled', 'database_backup', 'queued', 0, 'Queued backup.', 'database', 'manual', ?, ?)`,
		now, now, now, now, now, now); err != nil {
		t.Fatalf("insert jobs: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET leased_by = ?, lease_expires_at = ? WHERE id = 'job_analysis_cancel_running'`, server.jobLeaseOwner("job_analysis_cancel_running"), time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("lease running analysis job: %v", err)
	}
	ctx, done := server.registerRunningJob("job_analysis_cancel_running")
	defer done()

	response, err := server.cancelJobsByType("media_analyze")
	if err != nil {
		t.Fatalf("cancel analysis jobs: %v", err)
	}
	if response.Type != "media_analyze" || response.Cancelled != 2 || response.Total != 2 || len(response.Items) != 2 {
		t.Fatalf("unexpected cancel response: %#v", response)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("running analysis job context was not cancelled")
	}

	var cancelled int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND status = 'cancelled' AND progress = 100`).Scan(&cancelled); err != nil {
		t.Fatalf("count cancelled analysis jobs: %v", err)
	}
	if cancelled != 1 {
		t.Fatalf("immediately terminalized analysis jobs = %d, expected only queued job", cancelled)
	}
	var phase string
	if err := server.db.QueryRow(`SELECT phase FROM jobs WHERE id = 'job_analysis_cancel_running'`).Scan(&phase); err != nil {
		t.Fatalf("load running cancellation phase: %v", err)
	}
	if phase != "cancelling" {
		t.Fatalf("running analysis phase = %q, expected cancelling", phase)
	}
	server.finalizeRequestedJobCancellation("job_analysis_cancel_running")
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND status = 'cancelled' AND progress = 100`).Scan(&cancelled); err != nil {
		t.Fatalf("count acknowledged cancelled analysis jobs: %v", err)
	}
	if cancelled != 2 {
		t.Fatalf("acknowledged cancelled analysis jobs = %d, expected 2", cancelled)
	}
	var backupStatus string
	if err := server.db.QueryRow(`SELECT status FROM jobs WHERE id = 'job_backup_not_cancelled'`).Scan(&backupStatus); err != nil {
		t.Fatalf("load backup job: %v", err)
	}
	if backupStatus != "queued" {
		t.Fatalf("unrelated backup status = %q, expected queued", backupStatus)
	}
}

func TestLibraryScanJobDuplicateSuppression(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, active_key, created_at, updated_at)
		VALUES ('job_scan_existing', 'library_scan', 'queued', 0, 'Existing scan.', 'library', 'lib_movies', 'library_scan|library|lib_movies', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert active scan job: %v", err)
	}
	job, err := server.createJobFor("library_scan", "Duplicate scan.", "library", "lib_movies")
	if err != nil {
		t.Fatalf("create duplicate scan: %v", err)
	}
	if job.ID != "job_scan_existing" {
		t.Fatalf("expected existing scan job, got %#v", job)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_type = 'library' AND resource_id = 'lib_movies'`).Scan(&count); err != nil {
		t.Fatalf("count scan jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate scan suppression inserted %d jobs", count)
	}
}

func TestRunDueJobsRecoversStaleRunningJobWithoutLease(t *testing.T) {
	server := newScannerTestServer(t)
	stale := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, next_run_at, leased_by, lease_expires_at, last_error, created_at, updated_at)
		VALUES ('job_stale_no_lease', 'metadata_refresh', 'running', 42, 'Stale running job.', 'media', 'movie_meridian', '', '', '', '', ?, ?)`, stale, stale); err != nil {
		t.Fatalf("insert stale running job: %v", err)
	}
	server.runDueJobsOnce()
	var status string
	var lastError string
	if err := server.db.QueryRow(`SELECT status, last_error FROM jobs WHERE id = 'job_stale_no_lease'`).Scan(&status, &lastError); err != nil {
		t.Fatalf("query recovered job: %v", err)
	}
	if status != "queued" {
		t.Fatalf("stale running job status = %q, expected queued", status)
	}
	if !strings.Contains(lastError, "without a lease") {
		t.Fatalf("stale running job last_error = %q", lastError)
	}
}

func TestJobLaneAdmissionSerializesWithinWorkloadClass(t *testing.T) {
	server := newScannerTestServer(t)
	releaseScan, ok := server.tryAcquireJobLane("library_scan")
	if !ok {
		t.Fatalf("expected first write-heavy lane acquisition")
	}
	defer releaseScan()
	for _, jobType := range []string{"library_change_check", "live_tv_refresh"} {
		if _, ok := server.tryAcquireJobLane(jobType); ok {
			t.Fatalf("expected %s to wait while write-heavy lane is occupied", jobType)
		}
	}

	releaseMetadata, ok := server.tryAcquireJobLane("metadata_refresh")
	if !ok {
		t.Fatalf("expected metadata lane acquisition")
	}
	defer releaseMetadata()
	if _, ok := server.tryAcquireJobLane("lyrics_fetch_missing"); ok {
		t.Fatalf("expected lyrics fetch to wait while metadata lane is occupied")
	}

	releaseAnalysis, ok := server.tryAcquireJobLane("media_analyze")
	if !ok {
		t.Fatalf("expected analysis lane acquisition")
	}
	defer releaseAnalysis()
	releaseOptimized, ok := server.tryAcquireJobLane("optimize_version")
	if !ok {
		t.Fatalf("expected optimized version lane acquisition independent from analysis")
	}
	defer releaseOptimized()

	releaseMaintenance, ok := server.tryAcquireJobLane("database_backup")
	if !ok {
		t.Fatalf("expected maintenance lane acquisition")
	}
	defer releaseMaintenance()
	if _, ok := server.tryAcquireJobLane("dvr_retention_cleanup"); ok {
		t.Fatalf("expected dvr retention cleanup to wait while maintenance lane is occupied")
	}
	if _, ok := server.tryAcquireJobLane("library_read_model_repair"); ok {
		t.Fatalf("expected read-model repair to wait while maintenance lane is occupied")
	}
}

func TestJobLaneAdmissionAllowsIndependentMaintenanceWorkloads(t *testing.T) {
	server := newScannerTestServer(t)
	for _, jobType := range []string{"library_scan", "metadata_refresh", "media_analyze", "optimize_version", "database_backup"} {
		release, ok := server.tryAcquireJobLane(jobType)
		if !ok {
			t.Fatalf("expected independent lane acquisition for %s", jobType)
		}
		defer release()
	}
}

func TestJobLaneMaintenanceReleasesNextJob(t *testing.T) {
	server := newScannerTestServer(t)
	releaseFirst, ok := server.tryAcquireJobLane("database_backup")
	if !ok {
		t.Fatalf("expected first maintenance lane acquisition")
	}
	if _, ok := server.tryAcquireJobLane("library_trash_cleanup"); ok {
		t.Fatalf("expected second maintenance job to wait")
	}
	releaseFirst()
	releaseSecond, ok := server.tryAcquireJobLane("library_trash_cleanup")
	if !ok {
		t.Fatalf("expected maintenance lane acquisition after release")
	}
	defer releaseSecond()
	if _, ok := server.tryAcquireJobLane("dvr_retention_cleanup"); ok {
		t.Fatalf("expected maintenance lane to enforce capacity")
	}
}

func TestScheduledTasksQueueCleanupThroughMaintenanceJobs(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{"enabled":true,"startHour":0,"endHour":0,"backupDatabase":false,"scanLibraries":false,"refreshMetadata":false,"analyzeMedia":false,"emptyTrash":true}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	server.queueScheduledTasks(time.Now().UTC())

	for _, jobType := range []string{"library_trash_cleanup", "optimized_version_prune", "trickplay_prune"} {
		var count int
		if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = ? AND resource_type = 'maintenance' AND resource_id = 'scheduled'`, jobType).Scan(&count); err != nil {
			t.Fatalf("count %s jobs: %v", jobType, err)
		}
		if count != 1 {
			t.Fatalf("%s jobs = %d, expected 1", jobType, count)
		}
		if jobLaneForType(jobType) != jobLaneMaintenance {
			t.Fatalf("%s lane = %s, expected maintenance", jobType, jobLaneForType(jobType))
		}
	}

	server.queueScheduledTasks(time.Now().UTC())
	for _, jobType := range []string{"library_trash_cleanup", "optimized_version_prune", "trickplay_prune"} {
		var count int
		if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = ? AND resource_type = 'maintenance' AND resource_id = 'scheduled'`, jobType).Scan(&count); err != nil {
			t.Fatalf("count repeated %s jobs: %v", jobType, err)
		}
		if count != 1 {
			t.Fatalf("%s jobs after repeat = %d, expected 1", jobType, count)
		}
	}
}

func TestDVRRetentionCleanupQueuesThroughMaintenanceJobs(t *testing.T) {
	server := newScannerTestServer(t)

	server.queueDVRRetentionCleanup()
	server.queueDVRRetentionCleanup()

	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'dvr_retention_cleanup' AND resource_type = 'maintenance' AND resource_id = 'dvr'`).Scan(&count); err != nil {
		t.Fatalf("count dvr retention jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("dvr retention cleanup jobs = %d, expected 1", count)
	}
	if jobLaneForType("dvr_retention_cleanup") != jobLaneMaintenance {
		t.Fatalf("dvr retention lane = %s, expected maintenance", jobLaneForType("dvr_retention_cleanup"))
	}
}

func TestMaintenanceJobDefersTransientFailureAndSkipsUntilDue(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, attempt_count, next_run_at, deferred_until, leased_by, lease_expires_at, last_error, failure_kind, created_at, updated_at)
		VALUES ('job_retry_timeout', 'metadata_refresh_library', 'running', 36, 'Refreshing metadata.', 'library', 'lib_movies', 1, '', '', ?, ?, '', '', ?, ?)`,
		server.jobLeaseOwner("job_retry_timeout"), time.Now().UTC().Add(20*time.Minute).Format(time.RFC3339), now, now); err != nil {
		t.Fatalf("insert retry job: %v", err)
	}

	if !server.deferMaintenanceJob("job_retry_timeout", context.DeadlineExceeded) {
		t.Fatalf("expected transient timeout to defer maintenance job")
	}

	var status, nextRunAt, deferredUntil, lastError, failureKind string
	var progress int
	if err := server.db.QueryRow(`
		SELECT status, progress, next_run_at, deferred_until, last_error, failure_kind
		FROM jobs
		WHERE id = 'job_retry_timeout'`).Scan(&status, &progress, &nextRunAt, &deferredUntil, &lastError, &failureKind); err != nil {
		t.Fatalf("load deferred job: %v", err)
	}
	if status != "queued" || progress != 36 || nextRunAt == "" || deferredUntil == "" || lastError == "" || failureKind != "timeout" {
		t.Fatalf("deferred job = status=%q progress=%d next=%q deferred=%q lastError=%q kind=%q", status, progress, nextRunAt, deferredUntil, lastError, failureKind)
	}
	jobs, err := server.dueQueuedJobs(time.Now().UTC().Format(time.RFC3339), 10)
	if err != nil {
		t.Fatalf("load due jobs: %v", err)
	}
	for _, job := range jobs {
		if job.ID == "job_retry_timeout" {
			t.Fatalf("deferred job was claimable before retry time: %#v", job)
		}
	}
}

func TestMaintenanceJobFailsAfterRetryBudget(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, attempt_count, next_run_at, deferred_until, leased_by, lease_expires_at, last_error, failure_kind, created_at, updated_at)
		VALUES ('job_retry_exhausted', 'library_scan', 'running', 72, 'Scanning.', 'library', 'lib_movies', ?, '', '', ?, ?, '', '', ?, ?)`,
		maintenanceMaxAttempts, server.jobLeaseOwner("job_retry_exhausted"), time.Now().UTC().Add(20*time.Minute).Format(time.RFC3339), now, now); err != nil {
		t.Fatalf("insert exhausted job: %v", err)
	}

	if !server.deferMaintenanceJob("job_retry_exhausted", context.DeadlineExceeded) {
		t.Fatalf("expected timeout to be handled as terminal maintenance failure")
	}

	var remaining int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = 'job_retry_exhausted'`).Scan(&remaining); err != nil {
		t.Fatalf("count exhausted job: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("terminally failed maintenance evidence was discarded")
	}
	var status, errorCode string
	if err := server.db.QueryRow(`SELECT status, error_code FROM jobs WHERE id = 'job_retry_exhausted'`).Scan(&status, &errorCode); err != nil {
		t.Fatalf("load retained failed maintenance job: %v", err)
	}
	if status != "failed" || errorCode != "timeout" {
		t.Fatalf("retained failed maintenance job = status=%q errorCode=%q", status, errorCode)
	}
}

func TestCancelLibraryMetadataRefreshCancelsChildRefreshJobs(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES
			('job_parent_refresh', 'metadata_refresh_library', 'queued', 0, 'Queued.', 'library', 'lib_movies', '{"libraryId":"lib_movies","libraryName":"Movies"}', ?, ?),
			('job_child_refresh', 'metadata_refresh', 'queued', 0, 'Queued.', 'media', 'movie_meridian', '{"libraryId":"lib_movies","libraryName":"Movies"}', ?, ?),
			('job_other_refresh', 'metadata_refresh', 'queued', 0, 'Queued.', 'media', 'show_fargo', '{"libraryId":"lib_tv","libraryName":"TV"}', ?, ?)`,
		now, now, now, now, now, now); err != nil {
		t.Fatalf("insert metadata jobs: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET parent_operation_id = 'job_parent_refresh' WHERE id = 'job_child_refresh'`); err != nil {
		t.Fatalf("link child refresh: %v", err)
	}
	if _, err := server.cancelJob("job_parent_refresh"); err != nil {
		t.Fatalf("cancel parent refresh: %v", err)
	}
	statuses := map[string]string{}
	rows, err := server.db.Query(`SELECT id, status FROM jobs WHERE id IN ('job_parent_refresh', 'job_child_refresh', 'job_other_refresh')`)
	if err != nil {
		t.Fatalf("load job statuses: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatalf("scan job status: %v", err)
		}
		statuses[id] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("job status rows: %v", err)
	}
	if statuses["job_parent_refresh"] != "cancelled" || statuses["job_child_refresh"] != "cancelled" {
		t.Fatalf("parent and child refresh jobs should be cancelled together: %#v", statuses)
	}
	if statuses["job_other_refresh"] != "queued" {
		t.Fatalf("unrelated library refresh should remain queued: %#v", statuses)
	}
}

func TestRunningJobProgressSuppressesMessageOnlyChurn(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	updatedAt := now.Add(-time.Minute).Format(time.RFC3339)
	leaseExpiresAt := now.Add(10 * time.Minute).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, leased_by, lease_expires_at, created_at, updated_at)
		VALUES ('job_progress_churn', 'library_scan', 'running', 50, 'Writing media index (100/1000).', 'library', 'lib_movies', ?, ?, ?, ?)`,
		server.jobLeaseOwner("job_progress_churn"), leaseExpiresAt, updatedAt, updatedAt); err != nil {
		t.Fatalf("insert running job: %v", err)
	}

	if err := server.setJobProgress("job_progress_churn", "running", 50, "Writing media index (150/1000)."); err != nil {
		t.Fatalf("set same progress message: %v", err)
	}
	var message, unchangedAt string
	if err := server.db.QueryRow(`SELECT message, updated_at FROM jobs WHERE id = 'job_progress_churn'`).Scan(&message, &unchangedAt); err != nil {
		t.Fatalf("load suppressed progress job: %v", err)
	}
	if message != "Writing media index (100/1000)." || unchangedAt != updatedAt {
		t.Fatalf("message-only running progress was persisted: message=%q updated_at=%q", message, unchangedAt)
	}

	if err := server.setJobProgress("job_progress_churn", "running", 51, "Writing media index (200/1000)."); err != nil {
		t.Fatalf("set advanced progress message: %v", err)
	}
	var progress int
	if err := server.db.QueryRow(`SELECT progress, message, updated_at FROM jobs WHERE id = 'job_progress_churn'`).Scan(&progress, &message, &unchangedAt); err != nil {
		t.Fatalf("load advanced progress job: %v", err)
	}
	if progress != 51 || message != "Writing media index (200/1000)." || unchangedAt == updatedAt {
		t.Fatalf("advanced progress was not persisted: progress=%d message=%q updated_at=%q", progress, message, unchangedAt)
	}
}

func TestPresenceTouchThrottleCoalescesRepeatedRequests(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	if !server.shouldTouchPresence("session:test", now) {
		t.Fatalf("expected first session touch to be allowed")
	}
	if server.shouldTouchPresence("session:test", now.Add(time.Minute)) {
		t.Fatalf("expected repeated session touch inside throttle window to be suppressed")
	}
	if !server.shouldTouchPresence("session:test", now.Add(presenceTouchInterval+time.Second)) {
		t.Fatalf("expected session touch after throttle window to be allowed")
	}
}

func TestPresenceTouchThrottleExpiresAndBoundsIdentifiers(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	server.presenceTouchMu.Lock()
	server.presenceTouches = map[string]time.Time{
		"session:expired": now.Add(-presenceTouchRetention - time.Second),
		"session:recent":  now.Add(-time.Minute),
	}
	server.presenceLastPrune = now.Add(-presenceTouchInterval - time.Second)
	server.presenceTouchMu.Unlock()

	if !server.shouldTouchPresence("session:new", now) {
		t.Fatal("new presence identifier should be admitted")
	}
	server.presenceTouchMu.Lock()
	_, expiredExists := server.presenceTouches["session:expired"]
	_, recentExists := server.presenceTouches["session:recent"]
	server.presenceTouchMu.Unlock()
	if expiredExists || !recentExists {
		t.Fatalf("presence expiry mismatch: expired=%v recent=%v", expiredExists, recentExists)
	}

	server.presenceTouchMu.Lock()
	server.presenceTouches = make(map[string]time.Time, presenceTouchMaximum)
	for index := 0; index < presenceTouchMaximum; index++ {
		server.presenceTouches[fmt.Sprintf("device:%05d", index)] = now.Add(time.Duration(index) * time.Millisecond)
	}
	server.presenceLastPrune = now
	server.presenceTouchMu.Unlock()
	if !server.shouldTouchPresence("device:replacement", now.Add(time.Second)) {
		t.Fatal("replacement presence identifier should be admitted")
	}
	server.presenceTouchMu.Lock()
	count := len(server.presenceTouches)
	_, replacementExists := server.presenceTouches["device:replacement"]
	server.presenceTouchMu.Unlock()
	if count != presenceTouchMaximum || !replacementExists {
		t.Fatalf("presence map count=%d replacement=%v, want %d/true", count, replacementExists, presenceTouchMaximum)
	}
}

func intPtr(value int) *int {
	return &value
}

func TestScheduledTasksQueueMetadataAndAnalysisJobs(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Queued Movie.mp4")
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
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{"enabled":true,"startHour":0,"endHour":0,"backupDatabase":false,"scanLibraries":false,"refreshMetadata":true,"analyzeMedia":true,"taskTriggers":{"media-analysis":{"enabled":false,"intervalHours":24},"metadata-refresh":{"enabled":true,"intervalHours":24}}}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	now := time.Now().UTC()
	server.queueScheduledTasks(now)

	var metadataJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library'`).Scan(&metadataJobs); err != nil {
		t.Fatalf("count metadata jobs: %v", err)
	}
	if metadataJobs == 0 {
		t.Fatalf("expected scheduled library metadata refresh jobs")
	}
	var itemMetadataJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh'`).Scan(&itemMetadataJobs); err != nil {
		t.Fatalf("count item metadata jobs: %v", err)
	}
	if itemMetadataJobs != 0 {
		t.Fatalf("expected no scheduled item metadata jobs, got %d", itemMetadataJobs)
	}
	var analysisJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze'`).Scan(&analysisJobs); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if analysisJobs != 0 {
		t.Fatalf("expected disabled scheduled analysis to queue no jobs, got %d", analysisJobs)
	}

	server.queueScheduledTasks(now)

	var metadataJobsAfterRepeat int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library'`).Scan(&metadataJobsAfterRepeat); err != nil {
		t.Fatalf("count repeated metadata jobs: %v", err)
	}
	if metadataJobsAfterRepeat != metadataJobs {
		t.Fatalf("metadata jobs grew from %d to %d on repeated scheduler tick", metadataJobs, metadataJobsAfterRepeat)
	}
	var analysisJobsAfterRepeat int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze'`).Scan(&analysisJobsAfterRepeat); err != nil {
		t.Fatalf("count repeated analysis jobs: %v", err)
	}
	if analysisJobsAfterRepeat != analysisJobs {
		t.Fatalf("analysis jobs grew from %d to %d on repeated scheduler tick", analysisJobs, analysisJobsAfterRepeat)
	}
}

func TestScheduledLibraryScanUsesPerLibraryCadence(t *testing.T) {
	server := newScannerTestServer(t)
	hourlyLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Hourly Scan",
		Type:  "movie",
		Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"scheduledScanCadence": "hourly",
		},
	})
	if err != nil {
		t.Fatalf("create hourly library: %v", err)
	}
	defaultLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Default Scan",
		Type:  "movie",
		Paths: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatalf("create default library: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{
		"enabled": true,
		"startHour": 0,
		"endHour": 0,
		"backupDatabase": false,
		"scanLibraries": true,
		"libraryScanCadence": "weekly",
		"libraryScanIntervalHours": 168,
		"refreshMetadata": false,
		"analyzeMedia": false,
		"emptyTrash": false
	}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}
	previousScan := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES
			('job_hourly_previous_scan', 'library_scan', 'completed', 100, 'Previous hourly scan.', 'library', ?, '{}', ?, ?),
			('job_default_previous_scan', 'library_scan', 'completed', 100, 'Previous default scan.', 'library', ?, '{}', ?, ?)`,
		hourlyLibrary.ID, previousScan, previousScan, defaultLibrary.ID, previousScan, previousScan); err != nil {
		t.Fatalf("insert previous scan jobs: %v", err)
	}

	server.queueScheduledTasks(time.Now().UTC())

	var hourlyJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_id = ?`, hourlyLibrary.ID).Scan(&hourlyJobs); err != nil {
		t.Fatalf("count hourly scan jobs: %v", err)
	}
	if hourlyJobs != 2 {
		t.Fatalf("hourly library scan jobs = %d, expected previous plus newly queued", hourlyJobs)
	}
	var defaultJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan' AND resource_id = ?`, defaultLibrary.ID).Scan(&defaultJobs); err != nil {
		t.Fatalf("count default scan jobs: %v", err)
	}
	if defaultJobs != 1 {
		t.Fatalf("default library scan jobs = %d, expected weekly cadence to suppress new scan", defaultJobs)
	}
}

func TestScheduledTaskGlobalScanAndAnalysisDisableOverridesTriggers(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Disabled Automation.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	if _, err := server.createLibrary(CreateLibraryRequest{Name: "Disabled Automation", Type: "movie", Paths: []string{root}}); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, type, title, sort_title, source_url, duration_seconds, genres_json, added_at)
		VALUES ('movie_disabled_automation', 'movie', 'Disabled Automation', 'Disabled Automation', ?, 0, '[]', ?)`,
		mediaPath, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{
		"enabled": true,
		"startHour": 0,
		"endHour": 0,
		"backupDatabase": false,
		"scanLibraries": false,
		"refreshMetadata": false,
		"analyzeMedia": false,
		"emptyTrash": false,
		"taskTriggers": {
			"library-scan": {"enabled": true, "intervalHours": 1},
			"media-analysis": {"enabled": true, "intervalHours": 1}
		}
	}`); err != nil {
		t.Fatalf("save inconsistent scheduled settings: %v", err)
	}

	server.queueScheduledTasks(time.Now().UTC())

	var scanJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'library_scan'`).Scan(&scanJobs); err != nil {
		t.Fatalf("count scan jobs: %v", err)
	}
	if scanJobs != 0 {
		t.Fatalf("expected global scan disable to suppress trigger-enabled scans, got %d", scanJobs)
	}
	var analysisJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze'`).Scan(&analysisJobs); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if analysisJobs != 0 {
		t.Fatalf("expected global analysis disable to suppress trigger-enabled analysis, got %d", analysisJobs)
	}

	tasks, err := server.listScheduledTasks()
	if err != nil {
		t.Fatalf("list scheduled tasks: %v", err)
	}
	for _, task := range tasks {
		if (task.ID == "library-scan" || task.ID == "media-analysis") && (task.Enabled || task.Trigger.Enabled) {
			t.Fatalf("task %s should report disabled under global override: %#v", task.ID, task)
		}
	}
}

func TestScheduledTaskUpdateKeepsGlobalEnabledSettingInSync(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{"enabled":true,"scanLibraries":false,"analyzeMedia":false}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}
	task, err := server.updateScheduledTaskTrigger("media-analysis", ScheduledTaskUpdateRequest{Enabled: boolPtr(true), IntervalHours: intPtr(6)})
	if err != nil {
		t.Fatalf("enable media analysis task: %v", err)
	}
	if !task.Enabled || !task.Trigger.Enabled || task.Trigger.IntervalHours != 6 {
		t.Fatalf("updated task = %#v", task)
	}
	var stored string
	if err := server.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'scheduledTasks'`).Scan(&stored); err != nil {
		t.Fatalf("load scheduled settings: %v", err)
	}
	if !strings.Contains(stored, `"analyzeMedia":true`) {
		t.Fatalf("expected top-level analyzeMedia to be synced true, got %s", stored)
	}
}

func TestScheduledMediaAnalysisHonorsLibraryProbeSetting(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE media_items SET source_url = '', duration_seconds = 1`); err != nil {
		t.Fatalf("neutralize seeded media: %v", err)
	}
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Skipped Analysis.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:     "No Scheduled Analysis",
		Type:     "movie",
		Paths:    []string{root},
		Settings: map[string]any{"probeStreams": false},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, duration_seconds, genres_json, added_at)
		VALUES ('movie_scheduled_analysis_disabled', ?, 'movie', 'Skipped Analysis', 'Skipped Analysis', ?, 0, '[]', ?)`,
		library.ID, mediaPath, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media: %v", err)
	}

	server.queueScheduledMediaAnalysis(time.Now().UTC(), "test-run", 24)

	var analysisJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze'`).Scan(&analysisJobs); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if analysisJobs != 0 {
		t.Fatalf("expected scheduled analysis to skip disabled library, got %d jobs", analysisJobs)
	}
}

func TestScheduledMediaAnalysisUsesPerLibraryCadence(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE media_items SET source_url = '', duration_seconds = 1`); err != nil {
		t.Fatalf("neutralize seeded media: %v", err)
	}
	hourlyLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Hourly Analysis",
		Type:  "movie",
		Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"scheduledAnalysisEnabled": true,
			"scheduledAnalysisCadence": "hourly",
		},
	})
	if err != nil {
		t.Fatalf("create hourly library: %v", err)
	}
	defaultLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Default Analysis",
		Type:  "movie",
		Paths: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatalf("create default library: %v", err)
	}
	neverLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Never Analysis",
		Type:  "movie",
		Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"scheduledAnalysisEnabled": false,
		},
	})
	if err != nil {
		t.Fatalf("create never library: %v", err)
	}
	now := time.Now().UTC()
	mediaRows := []struct {
		id        string
		libraryID string
		title     string
	}{
		{id: "movie_hourly_analysis", libraryID: hourlyLibrary.ID, title: "Hourly Analysis"},
		{id: "movie_default_analysis", libraryID: defaultLibrary.ID, title: "Default Analysis"},
		{id: "movie_never_analysis", libraryID: neverLibrary.ID, title: "Never Analysis"},
	}
	for _, row := range mediaRows {
		if _, err := server.db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, duration_seconds, genres_json, added_at)
			VALUES (?, ?, 'movie', ?, ?, ?, 0, '[]', ?)`,
			row.id, row.libraryID, row.title, row.title, filepath.Join(t.TempDir(), row.id+".mp4"), now.Format(time.RFC3339)); err != nil {
			t.Fatalf("insert media %s: %v", row.id, err)
		}
	}
	previous := now.Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES
			('job_hourly_previous_analysis', 'media_analyze', 'complete', 100, 'Previous hourly analysis.', 'media', 'movie_hourly_analysis', '{}', ?, ?),
			('job_default_previous_analysis', 'media_analyze', 'complete', 100, 'Previous default analysis.', 'media', 'movie_default_analysis', '{}', ?, ?)`,
		previous, previous, previous, previous); err != nil {
		t.Fatalf("insert previous analysis jobs: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{
		"enabled": true,
		"startHour": 0,
		"endHour": 0,
		"backupDatabase": false,
		"scanLibraries": false,
		"refreshMetadata": false,
		"analyzeMedia": true,
		"analysisCadence": "weekly",
		"emptyTrash": false
	}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	server.queueScheduledTasks(now)

	var hourlyJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_id = 'movie_hourly_analysis'`).Scan(&hourlyJobs); err != nil {
		t.Fatalf("count hourly analysis jobs: %v", err)
	}
	if hourlyJobs != 2 {
		t.Fatalf("hourly analysis jobs = %d, expected previous plus newly queued", hourlyJobs)
	}
	var defaultJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_id = 'movie_default_analysis'`).Scan(&defaultJobs); err != nil {
		t.Fatalf("count default analysis jobs: %v", err)
	}
	if defaultJobs != 1 {
		t.Fatalf("default analysis jobs = %d, expected weekly cadence to suppress new job", defaultJobs)
	}
	var neverJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_id = 'movie_never_analysis'`).Scan(&neverJobs); err != nil {
		t.Fatalf("count never analysis jobs: %v", err)
	}
	if neverJobs != 0 {
		t.Fatalf("never analysis jobs = %d, expected no scheduled analysis", neverJobs)
	}
}

func TestScheduledMediaAnalysisSkipsActiveItemJob(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE media_items SET source_url = '', duration_seconds = 1`); err != nil {
		t.Fatalf("neutralize seeded media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Active Analysis",
		Type:  "movie",
		Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"scheduledAnalysisEnabled": true,
			"scheduledAnalysisCadence": "hourly",
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC()
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, duration_seconds, genres_json, added_at)
		VALUES ('movie_active_analysis', ?, 'movie', 'Active Analysis', 'Active Analysis', ?, 0, '[]', ?)`,
		library.ID, filepath.Join(t.TempDir(), "Active Analysis.mp4"), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	activeAt := now.Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES ('job_active_analysis', 'media_analyze', 'queued', 0, 'Already queued.', 'media', 'movie_active_analysis', '{}', ?, ?)`,
		activeAt, activeAt); err != nil {
		t.Fatalf("insert active analysis job: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{
		"enabled": true,
		"startHour": 0,
		"endHour": 0,
		"backupDatabase": false,
		"scanLibraries": false,
		"refreshMetadata": false,
		"analyzeMedia": true,
		"analysisCadence": "weekly",
		"emptyTrash": false
	}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	server.queueScheduledTasks(now)

	var jobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'media_analyze' AND resource_id = 'movie_active_analysis'`).Scan(&jobs); err != nil {
		t.Fatalf("count analysis jobs: %v", err)
	}
	if jobs != 1 {
		t.Fatalf("active item analysis jobs = %d, expected existing job only", jobs)
	}
}

func TestScheduledMetadataRefreshUsesPerLibraryAgeThreshold(t *testing.T) {
	server := newScannerTestServer(t)
	customLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Ninety Day Refresh",
		Type:  "movie",
		Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"scheduledMetadataRefreshEnabled": true,
			"scheduledMetadataRefreshDays":    90,
		},
	})
	if err != nil {
		t.Fatalf("create custom refresh library: %v", err)
	}
	inheritedLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Inherited Refresh",
		Type:  "movie",
		Paths: []string{t.TempDir()},
	})
	if err != nil {
		t.Fatalf("create inherited refresh library: %v", err)
	}
	neverLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Never Refresh",
		Type:  "movie",
		Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"scheduledMetadataRefreshEnabled": false,
		},
	})
	if err != nil {
		t.Fatalf("create never refresh library: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{
		"enabled": true,
		"startHour": 0,
		"endHour": 0,
		"backupDatabase": false,
		"scanLibraries": false,
		"refreshMetadata": true,
		"metadataRefreshDays": 14,
		"analyzeMedia": false,
		"emptyTrash": false
	}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	server.queueScheduledTasks(time.Now().UTC())

	assertMetadataRefreshDays := func(libraryID string, expected string) {
		t.Helper()
		var metadataJSON string
		if err := server.db.QueryRow(`SELECT metadata_json FROM jobs WHERE type = 'metadata_refresh_library' AND resource_type = 'library' AND resource_id = ?`, libraryID).Scan(&metadataJSON); err != nil {
			t.Fatalf("load metadata refresh job for %s: %v", libraryID, err)
		}
		metadata := map[string]string{}
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			t.Fatalf("decode metadata refresh job for %s: %v", libraryID, err)
		}
		if metadata["refreshDays"] != expected {
			t.Fatalf("metadata refresh days for %s = %q, expected %q", libraryID, metadata["refreshDays"], expected)
		}
	}
	assertMetadataRefreshDays(customLibrary.ID, "90")
	assertMetadataRefreshDays(inheritedLibrary.ID, "14")

	var neverJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library' AND resource_type = 'library' AND resource_id = ?`, neverLibrary.ID).Scan(&neverJobs); err != nil {
		t.Fatalf("count never refresh jobs: %v", err)
	}
	if neverJobs != 0 {
		t.Fatalf("never refresh jobs = %d, expected no scheduled metadata refresh", neverJobs)
	}
}

func TestScheduledMetadataRefreshUsesLibraryScopedInterval(t *testing.T) {
	server := newScannerTestServer(t)
	recentLibrary, err := server.createLibrary(CreateLibraryRequest{Name: "Recent Refresh", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create recent refresh library: %v", err)
	}
	dueLibrary, err := server.createLibrary(CreateLibraryRequest{Name: "Due Refresh", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create due refresh library: %v", err)
	}
	now := time.Now().UTC()
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, metadata_json, created_at, updated_at)
		VALUES ('job_recent_metadata_refresh', 'metadata_refresh_library', 'complete', 100, 'Previous metadata refresh.', 'library', ?, '{"refreshDays":"14"}', ?, ?)`,
		recentLibrary.ID, recent, recent); err != nil {
		t.Fatalf("insert recent metadata refresh job: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{
		"enabled": true,
		"startHour": 0,
		"endHour": 0,
		"backupDatabase": false,
		"scanLibraries": false,
		"refreshMetadata": true,
		"metadataRefreshCadence": "weekly",
		"metadataRefreshDays": 14,
		"analyzeMedia": false,
		"emptyTrash": false
	}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	server.queueScheduledTasks(now)

	var recentJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library' AND resource_id = ?`, recentLibrary.ID).Scan(&recentJobs); err != nil {
		t.Fatalf("count recent refresh jobs: %v", err)
	}
	if recentJobs != 1 {
		t.Fatalf("recent refresh jobs = %d, expected existing job only", recentJobs)
	}
	var dueJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library' AND resource_id = ?`, dueLibrary.ID).Scan(&dueJobs); err != nil {
		t.Fatalf("count due refresh jobs: %v", err)
	}
	if dueJobs != 1 {
		t.Fatalf("due refresh jobs = %d, expected newly queued job", dueJobs)
	}
}

func TestScheduledMetadataRefreshHonorsCadence(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Fresh Movie.mp4")
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
	if _, err := server.db.Exec(`DELETE FROM jobs WHERE type IN ('metadata_refresh', 'metadata_refresh_library')`); err != nil {
		t.Fatalf("clear scan metadata jobs: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET metadata_refreshed_at = ?`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("mark metadata fresh: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{"enabled":true,"startHour":0,"endHour":0,"backupDatabase":false,"scanLibraries":false,"refreshMetadata":true,"metadataRefreshDays":14,"analyzeMedia":false}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}

	server.queueScheduledTasks(time.Now().UTC())

	var metadataJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type = 'metadata_refresh_library'`).Scan(&metadataJobs); err != nil {
		t.Fatalf("count metadata jobs: %v", err)
	}
	if metadataJobs == 0 {
		t.Fatalf("expected one auditable library metadata refresh job even when no items need refresh")
	}
}

func TestCreateJobForRejectsUnsupportedType(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.createJobFor("unsupported_job", "Unsupported job.", "media", "movie_meridian"); err == nil {
		t.Fatalf("expected unsupported job type to be rejected")
	}
}
