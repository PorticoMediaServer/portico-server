package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"syscall"
	"time"
)

// playbackStorageConsumer names the server subsystem using owner-managed
// media. Keeping this explicit makes every eventual handler wiring auditable
// and prevents a long analysis job from silently inheriting playback policy.
type playbackStorageConsumer string

const (
	playbackStorageDirect       playbackStorageConsumer = "playback-direct"
	playbackStorageTranscode    playbackStorageConsumer = "playback-transcode"
	playbackStorageAnalysis     playbackStorageConsumer = "analysis"
	playbackStorageOptimization playbackStorageConsumer = "optimization"
)

type playbackStorageErrorKind string

const (
	playbackStorageErrorTransient playbackStorageErrorKind = "transient"
	playbackStorageErrorOffline   playbackStorageErrorKind = "offline"
	playbackStorageErrorStalled   playbackStorageErrorKind = "stalled"
)

var (
	errPlaybackStorageTransient = errors.New("media storage is temporarily unavailable")
	errPlaybackStorageOffline   = errors.New("media storage is offline")
	errPlaybackStorageStalled   = errors.New("media storage stopped responding")
)

// playbackStorageError deliberately excludes the media path. It can safely
// cross an HTTP/job boundary without disclosing an owner's filesystem layout.
type playbackStorageError struct {
	Kind      playbackStorageErrorKind
	Consumer  playbackStorageConsumer
	Operation string
	Cause     error
}

func (e *playbackStorageError) Error() string {
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "media I/O"
	}
	return fmt.Sprintf("%s %s failed: %s", e.Consumer, operation, playbackStoragePublicMessage(e.Kind))
}

func (e *playbackStorageError) Unwrap() error { return e.Cause }

func (e *playbackStorageError) Is(target error) bool {
	switch target {
	case errPlaybackStorageTransient:
		return e.Kind == playbackStorageErrorTransient
	case errPlaybackStorageOffline:
		return e.Kind == playbackStorageErrorOffline
	case errPlaybackStorageStalled:
		return e.Kind == playbackStorageErrorStalled
	default:
		return false
	}
}

func playbackStoragePublicMessage(kind playbackStorageErrorKind) string {
	switch kind {
	case playbackStorageErrorOffline:
		return "source is offline"
	case playbackStorageErrorStalled:
		return "source stopped responding"
	default:
		return "source is temporarily unavailable"
	}
}

func classifyPlaybackStorageError(consumer playbackStorageConsumer, operation string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	kind := playbackStorageErrorTransient
	switch {
	case errors.Is(err, errStorageIOStalled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, io.ErrNoProgress):
		kind = playbackStorageErrorStalled
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENODEV), errors.Is(err, syscall.ENXIO), errors.Is(err, errStorageCircuitOpen):
		kind = playbackStorageErrorOffline
	case storageErrorTransient(err):
		kind = playbackStorageErrorTransient
	default:
		// Non-storage failures from the supplied operation retain their original
		// identity. Callers must not turn decoder or validation failures into a
		// misleading storage-health response.
		return err
	}
	return &playbackStorageError{Kind: kind, Consumer: consumer, Operation: operation, Cause: err}
}

type playbackStorageAttemptPolicy struct {
	Timeout  time.Duration
	Attempts int
	Backoff  time.Duration
}

func playbackStoragePolicy(class storageSourceClass, consumer playbackStorageConsumer) playbackStorageAttemptPolicy {
	policy := playbackStorageAttemptPolicy{Timeout: storageIOOperationTimeout(class), Attempts: 1}
	if policy.Timeout <= 0 {
		policy.Timeout = 30 * time.Second
	}
	if class == storageSourceNetwork || class == storageSourceFUSE || class == storageSourceUnknown {
		policy.Attempts = 2
		policy.Backoff = 150 * time.Millisecond
	}
	// Process-backed work must regularly report progress, but gets a longer
	// quiet window than an HTTP startup/read operation.
	if consumer == playbackStorageAnalysis || consumer == playbackStorageOptimization {
		if policy.Timeout < 45*time.Second {
			policy.Timeout = 45 * time.Second
		}
	}
	return policy
}

func (s *Server) playbackStorageRequest(ctx context.Context, path string, consumer playbackStorageConsumer, operation string) storageIORequest {
	var request storageIORequest
	if s != nil && s.db != nil {
		request = s.storageRequestForPath(ctx, path, operation)
	} else {
		class, _ := classifyStorageSource(path, "")
		request = storageIORequest{
			SourceID:       storageSourceID("unassigned", path),
			AdmissionKey:   storagePhysicalSourceKey(path, class),
			Classification: class,
			Operation:      operation,
		}
	}
	policy := playbackStoragePolicy(request.Classification, consumer)
	request.Timeout = policy.Timeout
	return request
}

// withPlaybackStorageIO admits one bounded operation through the W3 source
// circuit/quarantine. The operation receives an attempt context and progress
// callback. It must observe cancellation; a syscall that cannot be cancelled
// remains quarantined by W3 and consumes no additional retry slot.
func (s *Server) withPlaybackStorageIO(
	ctx context.Context,
	path string,
	consumer playbackStorageConsumer,
	operation string,
	fn func(context.Context, func()) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	request := s.playbackStorageRequest(ctx, path, consumer, operation)
	policy := playbackStoragePolicy(request.Classification, consumer)
	var last error
	for attempt := 0; attempt < policy.Attempts; attempt++ {
		// Short, bounded operations receive a real deadline so os/exec/network
		// implementations that honor Context can abort without waiting for the
		// W3 quarantine timer. Long-running work uses a lease instead.
		attemptCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
		err := s.boundedStorageProgressIO(attemptCtx, request, func(progress func()) error {
			return fn(attemptCtx, progress)
		})
		cancel()
		if err == nil {
			return nil
		}
		last = classifyPlaybackStorageError(consumer, operation, err)
		if !playbackStorageRetryable(last) || attempt+1 >= policy.Attempts {
			return last
		}
		timer := time.NewTimer(policy.Backoff * time.Duration(attempt+1))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

func playbackStorageRetryable(err error) bool {
	return errors.Is(err, errPlaybackStorageTransient) || errors.Is(err, errPlaybackStorageOffline) || errors.Is(err, errPlaybackStorageStalled)
}

// playbackStorageLease keeps W3 admission for a streaming/external-process
// lifetime. Progress resets the no-progress deadline; Release is idempotent
// and the returned result reports the classified terminal storage outcome.
type playbackStorageLease struct {
	progress func()
	release  func(error)
	done     <-chan error
	once     sync.Once
}

func (l *playbackStorageLease) Progress() {
	if l != nil && l.progress != nil {
		l.progress()
	}
}

func (l *playbackStorageLease) Release(outcome error) {
	if l == nil || l.release == nil {
		return
	}
	l.once.Do(func() { l.release(outcome) })
}

func (l *playbackStorageLease) Done() <-chan error {
	if l == nil {
		closed := make(chan error)
		close(closed)
		return closed
	}
	return l.done
}

func (s *Server) acquirePlaybackStorageLease(ctx context.Context, path string, consumer playbackStorageConsumer, operation string) (*playbackStorageLease, error) {
	request := s.playbackStorageRequest(ctx, path, consumer, operation)
	leaseCtx, cancel := context.WithCancel(ctx)
	started := make(chan func(), 1)
	release := make(chan error, 1)
	result := make(chan error, 1)
	go func() {
		defer cancel()
		err := s.boundedStorageProgressIO(leaseCtx, request, func(progress func()) error {
			started <- progress
			select {
			case outcome := <-release:
				return outcome
			case <-leaseCtx.Done():
				return leaseCtx.Err()
			}
		})
		result <- classifyPlaybackStorageError(consumer, operation, err)
		close(result)
	}()
	select {
	case progress := <-started:
		return &playbackStorageLease{
			progress: progress,
			release: func(err error) {
				release <- err
			},
			done: result,
		}, nil
	case err := <-result:
		cancel()
		return nil, err
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}
