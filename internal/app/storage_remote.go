package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"
)

// storageObject is the durable identity returned by remote object stores. A
// scanner compares Revision before doing any metadata or media probing; Path
// is not assumed to be stable across provider-side moves.
type storageObject struct {
	Path        string
	ObjectID    string
	Revision    string
	ETag        string
	Hash        string
	Size        int64
	ModTime     time.Time
	ContentType string
}

type storageInventoryPage struct {
	Objects       []storageObject
	DeletedPaths  []string
	NextCursor    string
	SyncToken     string
	Authoritative bool
}

// remoteStorageBackend deliberately exposes inventory and ranged reads rather
// than filesystem calls. Large remote libraries must never be scanned through
// FUSE WalkDir/stat loops.
type remoteStorageBackend interface {
	Kind() string
	Inventory(context.Context, string, int) (storageInventoryPage, error)
	OpenRange(context.Context, string, int64, int64) (io.ReadCloser, error)
}

// remoteStorageObjectStatter is required for Complete analysis. Inventory is
// intentionally durable and asynchronous, so it cannot prove that an object
// stayed unchanged while Portico staged it. Provider-backed Stat calls bracket
// the ranged download and make that guarantee without rescanning a library.
type remoteStorageObjectStatter interface {
	Stat(context.Context, string) (storageObject, error)
}

type remoteStorageBackgroundReadKey struct{}

func withRemoteStorageBackgroundRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, remoteStorageBackgroundReadKey{}, true)
}

func remoteStorageReadIsPlayback(ctx context.Context) bool {
	background, _ := ctx.Value(remoteStorageBackgroundReadKey{}).(bool)
	return !background
}

var (
	errRemoteStorageBusy            = errors.New("remote storage admission capacity exhausted")
	errRemoteStoragePreempted       = errors.New("remote background I/O preempted by playback")
	errInvalidObjectPath            = errors.New("invalid remote object path")
	errRemoteInventoryCursorInvalid = errors.New("remote inventory cursor is no longer valid")
	errRemoteStorageSourceRemoved   = errors.New("remote storage source was removed during scanning")
	errRemoteStorageSourceInUse     = errors.New("remote storage source is in use")
)

const (
	remoteStoragePathMaxBytes      = 4 << 10
	remoteStorageMetadataMaxBytes  = 8 << 10
	remoteStorageCursorMaxBytes    = 1 << 20
	remoteStoragePageMaxObjects    = 10_000
	remoteStorageDirectoryQueueMax = 100_000
)

func normalizeRemoteObjectPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if len(value) > remoteStoragePathMaxBytes {
		return "", errInvalidObjectPath
	}
	value = strings.TrimPrefix(value, "/")
	clean := path.Clean(value)
	if value == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsRune(clean, '\x00') {
		return "", errInvalidObjectPath
	}
	return clean, nil
}

func validateStorageInventoryPage(page storageInventoryPage) error {
	if len(page.Objects)+len(page.DeletedPaths) > remoteStoragePageMaxObjects {
		return errors.New("remote inventory page exceeded the object limit")
	}
	if len(page.NextCursor) > remoteStorageCursorMaxBytes || len(page.SyncToken) > remoteStorageMetadataMaxBytes {
		return errors.New("remote inventory cursor exceeded the storage limit")
	}
	for _, object := range page.Objects {
		if _, err := normalizeRemoteObjectPath(object.Path); err != nil {
			return err
		}
		for _, value := range []string{object.ObjectID, object.Revision, object.ETag, object.Hash, object.ContentType} {
			if len(value) > remoteStorageMetadataMaxBytes {
				return errors.New("remote inventory object metadata exceeded the storage limit")
			}
		}
	}
	for _, path := range page.DeletedPaths {
		if _, err := normalizeRemoteObjectPath(path); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRemoteStorageRoot(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")), "/")
	if value == "" || value == "." {
		return "", nil
	}
	if strings.ContainsRune(value, '\x00') {
		return "", errors.New("invalid remote storage root")
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", errors.New("remote storage root must remain within the configured remote")
	}
	return clean, nil
}

// remoteStorageScheduler reserves independent foreground/background lanes.
// Playback never waits behind inventory, matching, artwork or analysis work.
type remoteStorageScheduler struct {
	playback          chan struct{}
	background        chan struct{}
	mu                sync.Mutex
	foreground        int
	removing          bool
	nextBackgroundID  uint64
	backgroundCancels map[uint64]context.CancelCauseFunc
}

func newRemoteStorageScheduler(playbackLimit, backgroundLimit int) *remoteStorageScheduler {
	if playbackLimit < 1 {
		playbackLimit = 1
	}
	if backgroundLimit < 1 {
		backgroundLimit = 1
	}
	return &remoteStorageScheduler{playback: make(chan struct{}, playbackLimit), background: make(chan struct{}, backgroundLimit), backgroundCancels: map[uint64]context.CancelCauseFunc{}}
}

func (q *remoteStorageScheduler) acquire(ctx context.Context, playback bool) (func(), error) {
	_, release, err := q.acquireOperation(ctx, playback)
	return release, err
}

func (q *remoteStorageScheduler) acquireOperation(ctx context.Context, playback bool) (context.Context, func(), error) {
	if q == nil {
		return ctx, func() {}, nil
	}
	if playback {
		q.mu.Lock()
		if q.removing {
			q.mu.Unlock()
			return nil, nil, errRemoteStorageSourceRemoved
		}
		q.foreground++
		for _, cancel := range q.backgroundCancels {
			cancel(errRemoteStoragePreempted)
		}
		q.mu.Unlock()
		select {
		case q.playback <- struct{}{}:
		case <-ctx.Done():
			q.mu.Lock()
			q.foreground--
			q.mu.Unlock()
			return nil, nil, ctx.Err()
		}
		return ctx, func() { q.mu.Lock(); q.foreground--; q.mu.Unlock(); <-q.playback }, nil
	}
	// Do not begin new background I/O while foreground work is active. Once
	// admitted, background operations remain bounded by their own lane.
	q.mu.Lock()
	if q.removing {
		q.mu.Unlock()
		return nil, nil, errRemoteStorageSourceRemoved
	}
	foreground := q.foreground
	q.mu.Unlock()
	if foreground > 0 {
		return nil, nil, fmt.Errorf("%w: playback active", errRemoteStorageBusy)
	}
	select {
	case q.background <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	q.mu.Lock()
	if q.removing {
		q.mu.Unlock()
		<-q.background
		return nil, nil, errRemoteStorageSourceRemoved
	}
	if q.foreground > 0 {
		q.mu.Unlock()
		<-q.background
		return nil, nil, fmt.Errorf("%w: playback active", errRemoteStorageBusy)
	}
	q.nextBackgroundID++
	id := q.nextBackgroundID
	operationCtx, cancel := context.WithCancelCause(ctx)
	q.backgroundCancels[id] = cancel
	q.mu.Unlock()
	var once sync.Once
	return operationCtx, func() {
		once.Do(func() {
			q.mu.Lock()
			delete(q.backgroundCancels, id)
			q.mu.Unlock()
			cancel(context.Canceled)
			<-q.background
		})
	}, nil
}

func (q *remoteStorageScheduler) cancelBackgroundOperations(cause error) {
	if q == nil {
		return
	}
	q.mu.Lock()
	cancels := make([]context.CancelCauseFunc, 0, len(q.backgroundCancels))
	for _, cancel := range q.backgroundCancels {
		cancels = append(cancels, cancel)
	}
	q.mu.Unlock()
	for _, cancel := range cancels {
		cancel(cause)
	}
}

func (q *remoteStorageScheduler) foregroundOperations() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.foreground
}

func (q *remoteStorageScheduler) beginRemoval(ctx context.Context) bool {
	if q == nil {
		return true
	}
	q.mu.Lock()
	if q.removing || q.foreground > 0 {
		q.mu.Unlock()
		return false
	}
	q.removing = true
	cancels := make([]context.CancelCauseFunc, 0, len(q.backgroundCancels))
	for _, cancel := range q.backgroundCancels {
		cancels = append(cancels, cancel)
	}
	q.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errRemoteStorageSourceRemoved)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		q.mu.Lock()
		drained := len(q.backgroundCancels) == 0
		q.mu.Unlock()
		if drained {
			return true
		}
		select {
		case <-ctx.Done():
			q.cancelRemoval()
			return false
		case <-ticker.C:
		}
	}
}

func (q *remoteStorageScheduler) cancelRemoval() {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.removing = false
	q.mu.Unlock()
}
