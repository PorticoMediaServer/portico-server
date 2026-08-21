package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"

	"github.com/PorticoMediaServer/portico-server/internal/redaction"
)

func (s *Server) analysisSourceStat(ctx context.Context, sourcePath, operation string) (os.FileInfo, error) {
	var info os.FileInfo
	err := s.withPlaybackStorageIO(ctx, sourcePath, playbackStorageAnalysis, operation, func(_ context.Context, progress func()) error {
		var statErr error
		info, statErr = os.Stat(sourcePath)
		if statErr == nil {
			progress()
		}
		return statErr
	})
	return info, err
}

// analysisCommandOutput is the bounded result of a source-reading analysis
// process. Stderr is kept separate where stdout is a machine-readable payload.
type analysisCommandOutput struct {
	Stdout []byte
	Stderr []byte
}

type analysisProgressBuffer struct {
	mu       sync.Mutex
	buffer   limitedCommandBuffer
	progress func()
}

func newAnalysisProgressBuffer(limit int, progress func()) *analysisProgressBuffer {
	return &analysisProgressBuffer{buffer: limitedCommandBuffer{limit: limit}, progress: progress}
}

func (buffer *analysisProgressBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	written, err := buffer.buffer.Write(value)
	buffer.mu.Unlock()
	if written > 0 && buffer.progress != nil {
		// This is deliberately tied to bytes actually emitted by the process.
		// A timer heartbeat would hide a wedged source from the W3 watchdog.
		buffer.progress()
	}
	return written, err
}

func (buffer *analysisProgressBuffer) bytes() ([]byte, bool) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.buffer.Bytes()...), buffer.buffer.overflow
}

// runAnalysisSourceCommand holds W3 admission for the complete external
// process lifetime. The lease watchdog cancels the entire managed process
// group if genuine output progress stops, and cleanup waits for both the
// process and lease before returning.
func (s *Server) runAnalysisSourceCommand(
	ctx context.Context,
	sourcePath string,
	operation string,
	executable string,
	args []string,
	dir string,
	stdoutLimit int,
	stderrLimit int,
) (analysisCommandOutput, error) {
	if err := ctx.Err(); err != nil {
		return analysisCommandOutput{}, err
	}
	info, err := s.analysisSourceStat(ctx, sourcePath, operation+" preflight")
	if err != nil {
		return analysisCommandOutput{}, err
	}
	if info.IsDir() {
		return analysisCommandOutput{}, &playbackStorageError{
			Kind: playbackStorageErrorOffline, Consumer: playbackStorageAnalysis, Operation: operation,
			Cause: errors.New("media source is not a regular file"),
		}
	}
	lease, err := s.acquirePlaybackStorageLease(ctx, sourcePath, playbackStorageAnalysis, operation)
	if err != nil {
		return analysisCommandOutput{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaseResult := make(chan error, 1)
	go func() {
		storageErr := <-lease.Done()
		leaseResult <- storageErr
		if storageErr != nil {
			cancel()
		}
	}()

	stdout := newAnalysisProgressBuffer(stdoutLimit, lease.Progress)
	stderr := newAnalysisProgressBuffer(stderrLimit, lease.Progress)
	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := managedCommandRun(runCtx, cmd)
	// A decoder/process exit is not storage-health evidence. Releasing the
	// source lease with runErr would open the durable storage circuit before
	// the failure check below can prove that the source is still readable.
	// The lease reports only its own watchdog/storage outcome; process errors
	// are classified separately after a supervised source re-check.
	lease.Release(nil)
	storageErr := <-leaseResult
	stdoutBytes, stdoutOverflow := stdout.bytes()
	stderrBytes, stderrOverflow := stderr.bytes()
	result := analysisCommandOutput{Stdout: stdoutBytes, Stderr: stderrBytes}
	if stdoutOverflow || stderrOverflow || errors.Is(runErr, errManagedCommandOutputLimit) {
		return result, errManagedCommandOutputLimit
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if storageErr != nil {
		return result, redaction.Error(storageErr, sourcePath)
	}
	if runErr != nil {
		// A decoder exit is not itself proof of a storage failure. Re-check the
		// admitted source so disappearance or a wedged mount becomes a typed W3
		// outcome while malformed media retains its decoder identity.
		if _, sourceErr := s.analysisSourceStat(ctx, sourcePath, operation+" failure check"); sourceErr != nil {
			return result, sourceErr
		}
		return result, redaction.Error(runErr, sourcePath)
	}
	return result, nil
}

func analysisProgressArgs() []string {
	return []string{"-progress", "pipe:2", "-stats_period", "2"}
}

func redactedAnalysisOutput(output []byte, sourcePath string) string {
	return redaction.RedactString(string(output), sourcePath)
}
