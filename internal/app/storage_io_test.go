package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBoundedStorageIOQuarantinesOneBlockedCallPerSource(t *testing.T) {
	server := &Server{}
	release := make(chan struct{})
	request := storageIORequest{SourceID: "source-a", Classification: storageSourceNetwork, Operation: "fault probe", Timeout: 15 * time.Millisecond}
	err := server.boundedStorageIO(context.Background(), request, func() error {
		<-release
		return nil
	})
	if !errors.Is(err, errStorageIOStalled) {
		t.Fatalf("first operation error = %v, expected stalled", err)
	}
	if err := server.boundedStorageIO(context.Background(), request, func() error { return nil }); !errors.Is(err, errStorageIOBusy) {
		t.Fatalf("second operation error = %v, expected busy quarantine", err)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		err = server.boundedStorageIO(context.Background(), request, func() error { return nil })
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("source admission did not recover: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBoundedStorageIOKeepsHealthyLocalFastPathParallel(t *testing.T) {
	server := &Server{}
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	request := storageIORequest{SourceID: "local-a", Classification: storageSourceLocal, Operation: "local read", Timeout: time.Second}
	done := make(chan error, 4)
	for range 4 {
		go func() {
			done <- server.boundedStorageIO(context.Background(), request, func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for range 4 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("local operation was spuriously serialized")
		}
	}
	if err := server.boundedStorageIO(context.Background(), request, func() error { return nil }); !errors.Is(err, errStorageIOBusy) {
		t.Fatalf("fifth local operation error = %v, expected bounded admission", err)
	}
	close(release)
	for range 4 {
		if err := <-done; err != nil {
			t.Fatalf("local operation failed: %v", err)
		}
	}
}

func TestBoundedStorageIOAppliesClassificationOverrideToExistingAdmission(t *testing.T) {
	server := &Server{}
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	local := storageIORequest{SourceID: "reclassified", Classification: storageSourceLocal, Operation: "local read", Timeout: time.Second}
	done := make(chan error, 1)
	go func() {
		done <- server.boundedStorageIO(context.Background(), local, func() error {
			started <- struct{}{}
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("local operation did not start")
	}
	network := storageIORequest{SourceID: local.SourceID, Classification: storageSourceNetwork, Operation: "network read", Timeout: time.Second}
	if err := server.boundedStorageIO(context.Background(), network, func() error { return nil }); !errors.Is(err, errStorageIOBusy) {
		t.Fatalf("reclassified operation error = %v, expected tightened admission", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("original operation failed: %v", err)
	}
}

func TestBoundedStorageIOEnforcesGlobalQuarantineBudget(t *testing.T) {
	originalLimit := storageIOGlobalAdmissionLimit
	storageIOGlobalAdmissionLimit = 2
	t.Cleanup(func() { storageIOGlobalAdmissionLimit = originalLimit })
	server := &Server{}
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	done := make(chan error, 2)
	for index := range 2 {
		request := storageIORequest{SourceID: fmt.Sprintf("source-%d", index), Classification: storageSourceNetwork, Operation: "blocked read", Timeout: time.Second}
		go func() {
			done <- server.boundedStorageIO(context.Background(), request, func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("global-budget operation did not start")
		}
	}
	third := storageIORequest{SourceID: "source-3", Classification: storageSourceNetwork, Operation: "third read", Timeout: time.Second}
	if err := server.boundedStorageIO(context.Background(), third, func() error { return nil }); !errors.Is(err, errStorageIOCapacity) {
		t.Fatalf("third operation error = %v, expected global capacity", err)
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("admitted operation failed: %v", err)
		}
	}
}

func TestBoundedStorageIORejectsOpenCircuitOutsideRecoveryProbe(t *testing.T) {
	server := &Server{}
	request := storageIORequest{SourceID: "open-source", Classification: storageSourceNetwork, CircuitState: "open", Operation: "read"}
	if err := server.boundedStorageIO(context.Background(), request, func() error { return nil }); !errors.Is(err, errStorageCircuitOpen) {
		t.Fatalf("open-circuit error = %v", err)
	}
	request.RecoveryProbe = true
	if err := server.boundedStorageIO(context.Background(), request, func() error { return nil }); err != nil {
		t.Fatalf("recovery probe failed: %v", err)
	}
}

func TestStorageReadRangeContainsOpenSeekAndRead(t *testing.T) {
	server := &Server{}
	path := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	request := storageIORequest{SourceID: "read-source", Classification: storageSourceLocal, Operation: "sample range", Timeout: time.Second}
	result, err := server.storageReadRange(context.Background(), request, path, 3, 4)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if string(result) != "3456" {
		t.Fatalf("range = %q", result)
	}
	if _, err := server.storageReadRange(context.Background(), request, path, 0, storageReadBufferLimit+1); err == nil {
		t.Fatal("oversized supervised read was accepted")
	}
}

func TestStorageErrorClassificationUsesWrappedErrno(t *testing.T) {
	for _, test := range []struct {
		err       error
		class     string
		transient bool
	}{
		{err: errors.New("stale file handle"), class: "stale_handle", transient: true},
		{err: errors.New("input/output error"), class: "io", transient: true},
		{err: os.ErrPermission, class: "permission", transient: true},
	} {
		wrapped := errors.Join(errors.New("storage failure"), test.err)
		if got := storageErrorClass(wrapped); got != test.class {
			t.Errorf("storageErrorClass(%v) = %q, expected %q", test.err, got, test.class)
		}
		if got := storageErrorTransient(wrapped); got != test.transient {
			t.Errorf("storageErrorTransient(%v) = %v", test.err, got)
		}
	}
}

func TestBoundedStorageProgressIOResetsOnlyOnProgress(t *testing.T) {
	server := &Server{}
	request := storageIORequest{SourceID: "source-progress", Operation: "copy", Timeout: 20 * time.Millisecond}
	err := server.boundedStorageProgressIO(context.Background(), request, func(progress func()) error {
		for range 3 {
			time.Sleep(10 * time.Millisecond)
			progress()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("progressing operation timed out: %v", err)
	}
}

func TestStoredCircuitAdmissionDoesNotExtendFailureWindow(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Circuit", Type: "movie", Paths: []string{root}})
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
	failureAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`UPDATE storage_sources SET health_state = 'stalled', circuit_state = 'open', error_class = 'stalled', error_message = 'hung', consecutive_failures = 4, last_failure_at = ? WHERE id = ?`, failureAt, evidence[0].SourceID); err != nil {
		t.Fatalf("open circuit: %v", err)
	}
	held := server.inspectLibraryRootsWithContext(context.Background(), library)
	if len(held) != 1 || !held[0].CircuitHeld {
		t.Fatalf("expected held circuit evidence: %#v", held)
	}
	if _, err := server.beginLibraryScanRun(context.Background(), library, "", "reconcile", held); err != nil {
		t.Fatalf("record held admission: %v", err)
	}
	var failures int
	var actualFailureAt string
	if err := server.db.QueryRow(`SELECT consecutive_failures, last_failure_at FROM storage_sources WHERE id = ?`, evidence[0].SourceID).Scan(&failures, &actualFailureAt); err != nil {
		t.Fatalf("read circuit: %v", err)
	}
	if failures != 4 || actualFailureAt != failureAt {
		t.Fatalf("held admission extended circuit: failures=%d failureAt=%q", failures, actualFailureAt)
	}
}

func TestLibraryChangeWatchPolicyHonorsStoredOwnerClassification(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Owner network", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	evidence := server.inspectLibraryRootsWithContext(context.Background(), library)
	if _, err := server.beginLibraryScanRun(context.Background(), library, "", "reconcile", evidence); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE storage_sources SET classification = 'network', classification_source = 'owner' WHERE id = ?`, evidence[0].SourceID); err != nil {
		t.Fatalf("set owner classification: %v", err)
	}
	interval, strategy := server.libraryChangeWatchPolicy(context.Background(), library)
	if interval != 2*time.Minute || strategy != "adaptive_poll_network" {
		t.Fatalf("watch policy = %s %q, expected owner network policy", interval, strategy)
	}
}
