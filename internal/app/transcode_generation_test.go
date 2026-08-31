package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
)

func TestOrphanedTranscodeGenerationDirectoryStatesIncludeRetired(t *testing.T) {
	for _, name := range []string{".generation-123", "movie.failed-123", "movie.retired-123"} {
		if !isOrphanedTranscodeGenerationDirectory(name) {
			t.Fatalf("state %q was not recognized", name)
		}
	}
	for _, name := range []string{"movie-current", "movie.unknown-123"} {
		if isOrphanedTranscodeGenerationDirectory(name) {
			t.Fatalf("unknown/current state %q was recognized", name)
		}
	}
}

func TestPlannedVODGenerationDirectoryStatesAreStrictlyRecognized(t *testing.T) {
	for _, name := range []string{
		"movie-g2-s0-0123456789abcdef01234567-i0123456789abcdef01234567",
		"movie-g2-s8-0123456789abcdef01234567",
	} {
		if !isPlannedVODGenerationDirectory(name) {
			t.Fatalf("planned generation state %q was not recognized", name)
		}
	}
	for _, name := range []string{"movie-current", "movie-gx-s0-digest", "movie-g2-sx-digest", "movie-g2-s0"} {
		if isPlannedVODGenerationDirectory(name) {
			t.Fatalf("non-generation state %q was recognized", name)
		}
	}
}

func TestPlannedTranscodeSessionRejectsStaleGenerationNamespace(t *testing.T) {
	binding := testPlaybackExecutionPlan(t, func(plan *playbackExecutionPlan) { plan.Plan.Timeline.Generation = 2 })
	identity := plannedTranscodeIdentity{
		UserID: "user", ProfileID: "profile", PlaybackSessionID: "session",
		AuthorizationRevision: "authorization", PlaybackGeneration: 2, GrantTokenHash: "grant",
	}
	currentKey := plannedTranscodeSessionKey("movie", binding, identity, 0)
	staleIdentity := identity
	staleIdentity.PlaybackGeneration = 1
	staleKey := plannedTranscodeSessionKey("movie", binding, staleIdentity, 0)
	session := &transcodeSession{key: currentKey}
	server := &Server{transcodes: map[string]*transcodeSession{currentKey: session}}
	if !server.plannedTranscodeSessionIsCurrent(session, currentKey) {
		t.Fatal("current planned VOD generation was rejected")
	}
	if server.plannedTranscodeSessionIsCurrent(session, staleKey) {
		t.Fatal("stale playback generation was accepted for publication")
	}
	delete(server.transcodes, currentKey)
	if server.plannedTranscodeSessionIsCurrent(session, currentKey) {
		t.Fatal("unregistered planned VOD generation was accepted for publication")
	}
}

func TestReconcilePlannedVODGenerationNamespaceRetiresStaleAndProtectsActive(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "stale-generation")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(stale, "marker")
	if err := os.WriteFile(marker, []byte("old generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{transcodes: map[string]*transcodeSession{}}
	if err := server.reconcilePlannedVODGenerationDirectory(""); err == nil {
		t.Fatal("empty generation path was accepted for reconciliation")
	}
	if err := server.reconcilePlannedVODGenerationDirectory(stale); err != nil {
		t.Fatalf("retire stale namespace: %v", err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale namespace remained after retirement: %v", err)
	}
	archives, err := filepath.Glob(stale + ".retired-*")
	if err != nil || len(archives) != 1 {
		t.Fatalf("retired namespace count = %d, err = %v", len(archives), err)
	}
	if got, err := os.ReadFile(filepath.Join(archives[0], "marker")); err != nil || string(got) != "old generation" {
		t.Fatalf("retired generation contents = %q, err = %v", got, err)
	}

	active := filepath.Join(root, "active-generation")
	if err := os.MkdirAll(active, 0o700); err != nil {
		t.Fatal(err)
	}
	server.transcodes["active"] = &transcodeSession{dir: active}
	if err := server.reconcilePlannedVODGenerationDirectory(active); err == nil {
		t.Fatal("active namespace was reconciled while still registered")
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active namespace was removed after reconciliation failure: %v", err)
	}
}

func TestReconcilePlannedVODGenerationNamespacePropagatesRenameFailure(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "stale-generation")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected planned generation rename failure")
	originalRename := renamePlannedVODGeneration
	renamePlannedVODGeneration = func(_, _ string) error { return injected }
	t.Cleanup(func() { renamePlannedVODGeneration = originalRename })

	server := &Server{transcodes: map[string]*transcodeSession{}}
	err := server.reconcilePlannedVODGenerationDirectory(stale)
	if !errors.Is(err, injected) {
		t.Fatalf("rename failure = %v, want injected error", err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("stale namespace disappeared after failed rename: %v", err)
	}
}

func TestArchivePlannedTranscodeGenerationPropagatesRenameFailure(t *testing.T) {
	generation := filepath.Join(t.TempDir(), "generation")
	if err := os.MkdirAll(generation, 0o700); err != nil {
		t.Fatal(err)
	}
	session := &transcodeSession{dir: generation, done: make(chan struct{})}
	close(session.done)
	injected := errors.New("injected planned generation rename failure")
	originalRename := renamePlannedVODGeneration
	renamePlannedVODGeneration = func(_, _ string) error { return injected }
	t.Cleanup(func() { renamePlannedVODGeneration = originalRename })
	if err := archivePlannedTranscodeGeneration(session); !errors.Is(err, injected) {
		t.Fatalf("rename failure = %v, want injected error", err)
	}
	if _, err := os.Stat(generation); err != nil {
		t.Fatalf("generation disappeared after failed rename: %v", err)
	}
	if release, ok := session.acquireReader(); ok {
		release()
		t.Fatal("retiring generation remained readable after failed rename")
	}
}

func TestPublishPlannedTranscodeProgressPropagatesSupervisorErrors(t *testing.T) {
	supervisor := newTranscodeSupervisorV2(context.Background(), ffmpegsupervisor.Config{})
	t.Cleanup(func() { _ = supervisor.Shutdown(context.Background()) })
	session := &transcodeSession{
		supervisor: supervisor,
		supervised: &transcodeGenerationV2{Key: "missing-generation", Generation: 1},
		generation: 2,
	}
	if err := publishPlannedTranscodeProgress(session, 4, time.Now().UTC()); err == nil {
		t.Fatal("supervisor publication error was ignored")
	}
}

func TestPruneOrphanedTranscodeGenerationsIncludesPlannedAndRetiredStates(t *testing.T) {
	root := t.TempDir()
	plannedRoot := filepath.Join(root, "planned-v2")
	if err := os.MkdirAll(plannedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	stalePlanned := filepath.Join(plannedRoot, "movie-g2-s0-0123456789abcdef01234567-i0123456789abcdef01234567")
	activePlanned := filepath.Join(plannedRoot, "movie-g2-s4-0123456789abcdef01234567-i0123456789abcdef01234567")
	retired := filepath.Join(root, "legacy", "movie.retired-123")
	for _, path := range []string{stalePlanned, activePlanned, retired} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	server := &Server{cfg: config.Config{TranscodeDir: root}, transcodes: map[string]*transcodeSession{
		"active": {dir: activePlanned},
	}}
	removed, err := server.pruneOrphanedTranscodeGenerations(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed generation count = %d, want 2", removed)
	}
	if _, err := os.Stat(activePlanned); err != nil {
		t.Fatalf("active planned namespace was pruned: %v", err)
	}
	for _, path := range []string{stalePlanned, retired} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("orphaned generation %q remained: %v", path, err)
		}
	}
}
