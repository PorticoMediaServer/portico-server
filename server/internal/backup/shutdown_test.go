package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

func TestBackupShutdownCancelsAndJoinsDetachedWorker(t *testing.T) {
	s, state, db := testService(t)
	entered := make(chan struct{})
	released := make(chan struct{})
	s.SetCopyScrubber(func(ctx context.Context, _ *dbwork.Candidate) error {
		close(entered)
		<-ctx.Done()
		<-released // model a native operation still unwinding after cancellation
		return ctx.Err()
	})
	id, err := s.Start(context.Background(), KindManual, "shutdown-test")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	stop, cancel := context.WithCancel(context.Background())
	cancel()
	if err = s.Shutdown(stop); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if n, _ := holdRows(t, db); n != 1 {
		t.Fatal("shutdown released the running native copy's hold")
	}
	if _, err = s.Start(context.Background(), KindManual, "shutdown-reject"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = s.Create(context.Background(), KindManual); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(released)
	wait, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	if err = s.Shutdown(wait); err != nil {
		t.Fatal(err)
	}
	job, ok := s.Job(id)
	if !ok || !errors.Is(job.Result(), context.Canceled) {
		t.Fatal("detached job was not canceled and joined")
	}
	if n, _ := holdRows(t, db); n != 0 {
		t.Fatal("finished worker retained its hold")
	}
	assertNoScratch(t, state, s.backupRoot())
}

func TestBackupShutdownJoinsSynchronousCreate(t *testing.T) {
	s, _, _ := testService(t)
	entered := make(chan struct{})
	s.SetCopyScrubber(func(ctx context.Context, _ *dbwork.Candidate) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	result := make(chan error, 1)
	go func() { _, err := s.Create(context.Background(), KindManual); result <- err }()
	<-entered
	wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(wait); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
