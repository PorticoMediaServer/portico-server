package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlaybackStorageClassifiesErrorsWithoutLeakingPaths(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "private-media.mkv")
	tests := []struct {
		cause  error
		target error
	}{
		{cause: errors.Join(os.ErrNotExist, errors.New(secret)), target: errPlaybackStorageOffline},
		{cause: errors.Join(errStorageIOStalled, errors.New(secret)), target: errPlaybackStorageStalled},
		{cause: errors.Join(errStorageIOBusy, errors.New(secret)), target: errPlaybackStorageTransient},
	}
	for _, test := range tests {
		err := classifyPlaybackStorageError(playbackStorageDirect, "open source", test.cause)
		if !errors.Is(err, test.target) {
			t.Fatalf("classify %v: got %v", test.cause, err)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("classified error leaked media path: %v", err)
		}
	}
	decoderErr := errors.New("invalid codec parameters")
	if got := classifyPlaybackStorageError(playbackStorageTranscode, "decode", decoderErr); got != decoderErr {
		t.Fatalf("non-storage error was replaced: %v", got)
	}
}

func TestWithPlaybackStorageIORetriesTransientRemoteFailure(t *testing.T) {
	server := &Server{}
	path := `/mnt/rclone/library/movie.mkv`
	attempts := 0
	err := server.withPlaybackStorageIO(context.Background(), path, playbackStorageDirect, "read range", func(ctx context.Context, _ func()) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("attempt context has no deadline")
		}
		attempts++
		if attempts == 1 {
			return errStorageIOBusy
		}
		return nil
	})
	if err != nil {
		t.Fatalf("remote retry failed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestPlaybackStorageLeaseProgressAndRelease(t *testing.T) {
	server := &Server{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := server.acquirePlaybackStorageLease(ctx, t.TempDir(), playbackStorageTranscode, "transcode source")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	lease.Progress()
	lease.Release(nil)
	lease.Release(errors.New("ignored duplicate release"))
	select {
	case err := <-lease.Done():
		if err != nil {
			t.Fatalf("lease result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease did not release")
	}
}

func TestPlaybackStorageLeaseHonorsCanceledContext(t *testing.T) {
	server := &Server{}
	ctx, cancel := context.WithCancel(context.Background())
	lease, err := server.acquirePlaybackStorageLease(ctx, t.TempDir(), playbackStorageAnalysis, "probe source")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	cancel()
	select {
	case err := <-lease.Done():
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("result = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease did not observe cancellation")
	}
}
