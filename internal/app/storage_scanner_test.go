package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestBlockedRootAdmissionIsBoundedAndDoesNotPoisonHealthySource(t *testing.T) {
	server := newScannerTestServer(t)
	blockedPath := t.TempDir()
	healthyPath := t.TempDir()
	blockedLibrary := Library{ID: "lib_probe_blocked", Type: "movie", Paths: []string{blockedPath}}
	healthyLibrary := Library{ID: "lib_probe_healthy", Type: "movie", Paths: []string{healthyPath}}
	originalProbe := scannerProbeRootPath
	originalTimeout := scannerRootProbeTimeout
	release := make(chan struct{})
	scannerRootProbeTimeout = func(storageSourceClass) time.Duration { return 20 * time.Millisecond }
	scannerProbeRootPath = func(path string) (string, error) {
		if filepath.Clean(path) == filepath.Clean(blockedPath) {
			<-release
		}
		return originalProbe(path)
	}
	t.Cleanup(func() {
		close(release)
		scannerProbeRootPath = originalProbe
		scannerRootProbeTimeout = originalTimeout
	})

	started := time.Now()
	blocked := server.inspectLibraryRootsWithContext(context.Background(), blockedLibrary)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("blocked root admission took %s", elapsed)
	}
	if len(blocked) != 1 || blocked[0].Health != "stalled" || blocked[0].ErrorClass != "stalled" {
		t.Fatalf("blocked evidence = %#v", blocked)
	}
	healthy := server.inspectLibraryRootsWithContext(context.Background(), healthyLibrary)
	if len(healthy) != 1 || healthy[0].Health != "healthy" || healthy[0].ResolvedPath == "" {
		t.Fatalf("healthy evidence after blocked source = %#v", healthy)
	}
}

func TestStoredStallCircuitSkipsFilesystemProbeUntilBackoffExpires(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Circuit", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	configured, _ := filepath.Abs(root)
	sourceID := storageSourceID(library.ID, filepath.Clean(configured))
	if _, err := server.db.Exec(`
		INSERT INTO storage_sources (
			id, library_id, configured_path, classification, health_state, circuit_state,
			error_class, error_message, consecutive_failures, last_failure_at, created_at, updated_at
		) VALUES (?, ?, ?, 'local', 'stalled', 'open', 'stalled', 'blocked probe', 1, ?, ?, ?)`,
		sourceID, library.ID, filepath.Clean(configured), now, now, now); err != nil {
		t.Fatal(err)
	}
	originalProbe := scannerProbeRootPath
	called := false
	scannerProbeRootPath = func(path string) (string, error) {
		called = true
		return originalProbe(path)
	}
	t.Cleanup(func() { scannerProbeRootPath = originalProbe })
	evidence := server.inspectLibraryRootsWithContext(context.Background(), library)
	if called {
		t.Fatal("open stalled circuit touched the filesystem during backoff")
	}
	if len(evidence) != 1 || !evidence[0].CircuitHeld || evidence[0].Health != "stalled" {
		t.Fatalf("circuit evidence = %#v", evidence)
	}
}

func TestStoredOwnerClassificationOverridesPathHeuristic(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Owner Class", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	configured, _ := filepath.Abs(root)
	sourceID := storageSourceID(library.ID, filepath.Clean(configured))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO storage_sources (
			id, library_id, configured_path, classification, classification_source,
			health_state, circuit_state, created_at, updated_at
		) VALUES (?, ?, ?, 'network', 'owner', 'healthy', 'closed', ?, ?)`,
		sourceID, library.ID, filepath.Clean(configured), now, now); err != nil {
		t.Fatal(err)
	}
	evidence := server.inspectLibraryRootsWithContext(context.Background(), library)
	if len(evidence) != 1 || evidence[0].Classification != storageSourceNetwork || evidence[0].ClassificationSource != "owner" {
		t.Fatalf("owner classification evidence = %#v", evidence)
	}
}
