package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnalysisProgressBufferReportsOnlyWrittenBytes(t *testing.T) {
	var progress atomic.Int32
	buffer := newAnalysisProgressBuffer(16, func() { progress.Add(1) })
	if _, err := buffer.Write(nil); err != nil {
		t.Fatalf("empty write: %v", err)
	}
	if got := progress.Load(); got != 0 {
		t.Fatalf("empty write reported progress: %d", got)
	}
	if _, err := buffer.Write([]byte("frame=1\n")); err != nil {
		t.Fatalf("process output write: %v", err)
	}
	if got := progress.Load(); got != 1 {
		t.Fatalf("process output progress = %d, want 1", got)
	}
}

func TestAnalysisSourceCommandClassifiesMissingSourceWithoutPath(t *testing.T) {
	server := &Server{}
	secret := filepath.Join(t.TempDir(), "owner-private", "missing.mkv")
	_, err := server.runAnalysisSourceCommand(context.Background(), secret, "probe media facts", "/bin/sh", []string{"-c", "exit 0"}, "", 1024, 1024)
	if !errors.Is(err, errPlaybackStorageOffline) {
		t.Fatalf("error = %v, want offline storage error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked source path: %v", err)
	}
}

func TestAnalysisSourceCommandCancelsAndReleasesAdmission(t *testing.T) {
	server := &Server{}
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, []byte("media"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := server.runAnalysisSourceCommand(ctx, source, "probe media facts", "/bin/sh", []string{"-c", "sleep 30"}, "", 1024, 1024)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	server.storageIOMu.Lock()
	inFlight := server.storageIOInFlight
	admissions := len(server.storageIOAdmissions)
	server.storageIOMu.Unlock()
	if inFlight != 0 || admissions != 0 {
		t.Fatalf("analysis admission leaked: inFlight=%d admissions=%d", inFlight, admissions)
	}
}

func TestAnalysisSourceCommandClassifiesSourceDisappearance(t *testing.T) {
	server := &Server{}
	source := filepath.Join(t.TempDir(), "private-movie.mkv")
	if err := os.WriteFile(source, []byte("media"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	_, err := server.runAnalysisSourceCommand(context.Background(), source, "probe media facts", "/bin/sh", []string{"-c", `rm "$1"; exit 9`, "analysis", source}, "", 1024, 1024)
	if !errors.Is(err, errPlaybackStorageOffline) {
		t.Fatalf("error = %v, want offline storage error", err)
	}
	if strings.Contains(err.Error(), source) {
		t.Fatalf("error leaked source path: %v", err)
	}
}

func TestAnalysisSourceCommandDoesNotPoisonHealthyStorageOnDecoderExit(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	source := filepath.Join(root, "valid-storage-invalid-media.mkv")
	if err := os.WriteFile(source, []byte("not valid media"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Decoder failure", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	evidence := server.inspectLibraryRootsWithContext(context.Background(), library)
	if len(evidence) != 1 {
		t.Fatalf("root evidence count = %d", len(evidence))
	}
	if _, err := server.beginLibraryScanRun(context.Background(), library, "", "reconcile", evidence); err != nil {
		t.Fatalf("seed source evidence: %v", err)
	}

	_, err = server.runAnalysisSourceCommand(context.Background(), source, "probe media facts", "/bin/sh", []string{"-c", "exit 234"}, "", 1024, 1024)
	if err == nil || errors.Is(err, errPlaybackStorageOffline) || errors.Is(err, errPlaybackStorageStalled) {
		t.Fatalf("decoder exit classification = %v, want non-storage process error", err)
	}
	var health, circuit, errorClass string
	if err := server.db.QueryRow(`SELECT health_state, circuit_state, error_class FROM storage_sources WHERE id = ?`, evidence[0].SourceID).Scan(&health, &circuit, &errorClass); err != nil {
		t.Fatalf("read source health: %v", err)
	}
	if health != "healthy" || circuit != "closed" || errorClass != "" {
		t.Fatalf("decoder exit poisoned storage health: health=%q circuit=%q errorClass=%q", health, circuit, errorClass)
	}
}
