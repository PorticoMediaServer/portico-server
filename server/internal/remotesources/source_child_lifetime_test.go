package remotesources

import (
	"context"
	"errors"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

func TestSourceRequestCancellationDoesNotRetireParentAuthority(t *testing.T) {
	f := newObservationFixture(t)
	identities, owner, family := observationOwner(t, f)
	ref, err := f.record(t, scanFacts("child-life"), 0)
	if err != nil {
		t.Fatal(err)
	}
	lease := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
	defer lease.Close()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, check := range []func() error{
		func() error { return lease.Check(canceled) },
		func() error { _, err := lease.OpenRange(canceled); return err },
		func() error { _, err := lease.OpenSequential(canceled); return err },
	} {
		if err := check(); !errors.Is(err, context.Canceled) {
			t.Fatal("canceled request was not refused", err)
		}
		if err := lease.Check(context.Background()); err != nil {
			t.Fatal("child request retired retained owner authority", err)
		}
	}
	view, err := dbwork.BeginSnapshot(context.Background(), f.s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.CheckTx(canceled, view.Tx()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	view.Rollback()
	if err = lease.Check(context.Background()); err != nil {
		t.Fatal("canceled caller transaction poisoned the retained lease", err)
	}
	if _, err := f.s.db.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=?`, family); err != nil {
		t.Fatal(err)
	}
	if err = lease.childFailure(canceled, context.Canceled); errors.Is(err, context.Canceled) || err == nil {
		t.Fatal("cancellation shielded actual owner revocation", err)
	}
	if _, err := f.s.db.Exec(`UPDATE authorization_session_families SET revoked=0 WHERE id=?`, family); err != nil {
		t.Fatal(err)
	}
	if err = lease.Check(context.Background()); err == nil {
		t.Fatal("owner ABA revived the revoked retained lease")
	}
}

// Provider bytes are a blocked fixture; actual owner-family/source checks run
// through the retained lease. The controlled-rclone case covers real I/O.
func TestSourceClosingAnActiveChildLeavesParentLive(t *testing.T) {
	f := newObservationFixture(t)
	identities, owner, _ := observationOwner(t, f)
	facts := scanFacts("close-active")
	ref, err := f.record(t, facts, 0)
	if err != nil {
		t.Fatal(err)
	}
	lease := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
	defer lease.Close()
	life, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	child := &SourceRange{lease: lease, session: &RangeSession{life: life, cancel: cancel, admission: RangeAdmission{Scan: facts.Facts, Check: lease.Check, OnChanged: func() {}, Observe: func(context.Context) (remotemedia.FileObservation, error) { return facts.Facts, nil }}, readRange: func(ctx context.Context, _, _ string, _, _, _ int64) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}}
	if err = lease.keepChild(child); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	done := make(chan error, 1)
	go func() { _, err := child.ReadAt(ctx, make([]byte, 4), 0); done <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = child.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("closed child returned valid bytes")
	}
	if err = lease.Check(context.Background()); err != nil {
		t.Fatal("closing an active child retired its parent", err)
	}
	lease.mu.Lock()
	count := len(lease.children)
	lease.mu.Unlock()
	if count != 0 {
		t.Fatal("closed child remained retained", count)
	}
}

func TestSourceTerminalFailureIsNeverDowngradedToChildCancellation(t *testing.T) {
	f := newObservationFixture(t)
	identities, owner, _ := observationOwner(t, f)
	ref, err := f.record(t, scanFacts("child-terminal"), 0)
	if err != nil {
		t.Fatal(err)
	}
	lease := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
	defer lease.Close()
	lease.fail(storage.ErrRemoteChanged)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lease.childFailure(canceled, context.Canceled); !errors.Is(err, storage.ErrRemoteChanged) {
		t.Fatal("request cancellation concealed source retirement", err)
	}
}
