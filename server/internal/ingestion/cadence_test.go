package ingestion

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/persistence"
	workerloop "portico.local/server/internal/worker"
	"sync/atomic"
	"testing"
	"time"
)

// withoutTimers leaves the scan loop nothing to run on but its own
// continuation: the in-flight poll and the shared commit floor are both pushed
// far past the life of a test.
func withoutTimers(t *testing.T) {
	t.Helper()
	poll, floor := quantumPoll, workerloop.MinSignalInterval
	quantumPoll, workerloop.MinSignalInterval = time.Hour, time.Hour
	t.Cleanup(func() { quantumPoll, workerloop.MinSignalInterval = poll, floor })
}

func TestScanLoopContinuesAsSoonAsAQuantumAdvances(t *testing.T) {
	withoutTimers(t)
	root := t.TempDir()
	for i := 0; i < 40; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprint(i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat := catalog.New(db)
	lib, err := cat.Create("Movies", "movie", root)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, cat, assets.Probe{})
	job, err := s.Queue(lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.After(20 * time.Second)
	for {
		current, err := s.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "complete" {
			return
		}
		if current.Status == "failed" {
			t.Fatalf("scan failed: %+v", current)
		}
		select {
		case <-deadline:
			t.Fatalf("scan waited on a timer between steps: %+v", current)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestScanLoopDoesNotRespinAQuantumThatDidNotAdvance(t *testing.T) {
	withoutTimers(t)
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat := catalog.New(db)
	lib, err := cat.Create("Films", "movie", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, cat, assets.Probe{})
	job, err := s.Queue(lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The loop admits the job; the quantum itself is then refused, so it ends
	// without moving the run.
	var calls atomic.Int64
	s.Admission = func(context.Context, *sql.Tx, string) (bool, error) { return calls.Add(1)%2 == 1, nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done
	if n := calls.Load(); n != 2 {
		t.Fatalf("a quantum that made no progress was started %d times", n/2)
	}
	if j, err := s.Get(job.ID); err != nil || j.Status != "queued" {
		t.Fatalf("refused quantum changed the job: %+v %v", j, err)
	}
}

func TestQuantumReportsWhetherTheRunAdvanced(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat := catalog.New(db)
	lib, err := cat.Create("Films", "movie", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, cat, assets.Probe{})
	job, err := s.Queue(lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !s.processQuantum(context.Background(), job.ID, lib.ID) {
		t.Fatal("a committed inventory page was not reported as progress")
	}
	if err = s.Control(context.Background(), job.ID, "pause", nil); err != nil {
		t.Fatal(err)
	}
	if s.processQuantum(context.Background(), job.ID, lib.ID) {
		t.Fatal("a paused job was reported as progress")
	}
}
