package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

const mediaWriteMinimumFreeBytes = int64(512 << 20)

var errMediaResourcesBusy = errors.New("media processing resources are busy")
var errMediaStoragePressure = errors.New("media storage does not have enough free space")

type mediaResourceRequest struct {
	class   foundationcontract.WorkClass
	cpu     int
	disk    int
	network int
}

func mediaProcessingWorkClass(background bool) foundationcontract.WorkClass {
	if background {
		return foundationcontract.WorkClassBackgroundMedia
	}
	return foundationcontract.WorkClassPlaybackStart
}

type mediaResourceGovernor struct {
	mu                sync.Mutex
	cpuCapacity       int
	diskCapacity      int
	networkCapacity   int
	cpuUsed           int
	diskUsed          int
	networkUsed       int
	backgroundCPUUsed int
	diskReservedBytes map[string]int64
	nextBackgroundID  uint64
	backgroundCancels map[uint64]context.CancelCauseFunc
}

func newMediaResourceGovernor() *mediaResourceGovernor {
	return &mediaResourceGovernor{
		cpuCapacity:       max(2, runtime.NumCPU()/2),
		diskCapacity:      16,
		networkCapacity:   32,
		backgroundCancels: map[uint64]context.CancelCauseFunc{},
	}
}

func (governor *mediaResourceGovernor) registerBackgroundContext(ctx context.Context) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	operationCtx, cancel := context.WithCancelCause(ctx)
	governor.mu.Lock()
	governor.nextBackgroundID++
	id := governor.nextBackgroundID
	governor.backgroundCancels[id] = cancel
	governor.mu.Unlock()
	var once sync.Once
	return operationCtx, func() {
		once.Do(func() {
			governor.mu.Lock()
			delete(governor.backgroundCancels, id)
			governor.mu.Unlock()
			cancel(context.Canceled)
		})
	}
}

func (governor *mediaResourceGovernor) preemptBackgroundForPlayback() {
	governor.mu.Lock()
	cancels := make([]context.CancelCauseFunc, 0, len(governor.backgroundCancels))
	for _, cancel := range governor.backgroundCancels {
		cancels = append(cancels, cancel)
	}
	governor.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errRemoteStoragePreempted)
	}
}

func (s *Server) mediaResourceGovernor() *mediaResourceGovernor {
	s.mediaResourceMu.Lock()
	defer s.mediaResourceMu.Unlock()
	if s.mediaResources == nil {
		s.mediaResources = newMediaResourceGovernor()
	}
	return s.mediaResources
}

func (governor *mediaResourceGovernor) tryAcquire(request mediaResourceRequest) (func(), bool) {
	governor.mu.Lock()
	defer governor.mu.Unlock()
	if !request.class.Valid() {
		return nil, false
	}
	request.cpu = max(0, request.cpu)
	request.disk = max(0, request.disk)
	request.network = max(0, request.network)
	backgroundCPUCapacity := governor.cpuCapacity
	if backgroundCPUCapacity > 1 {
		backgroundCPUCapacity--
	}
	background := request.class.Priority() >= foundationcontract.WorkClassBackgroundMedia.Priority()
	if governor.cpuUsed+request.cpu > governor.cpuCapacity ||
		governor.diskUsed+request.disk > governor.diskCapacity ||
		governor.networkUsed+request.network > governor.networkCapacity ||
		(background && governor.backgroundCPUUsed+request.cpu > backgroundCPUCapacity) {
		return nil, false
	}
	governor.cpuUsed += request.cpu
	governor.diskUsed += request.disk
	governor.networkUsed += request.network
	if background {
		governor.backgroundCPUUsed += request.cpu
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			governor.mu.Lock()
			governor.cpuUsed = max(0, governor.cpuUsed-request.cpu)
			governor.diskUsed = max(0, governor.diskUsed-request.disk)
			governor.networkUsed = max(0, governor.networkUsed-request.network)
			if background {
				governor.backgroundCPUUsed = max(0, governor.backgroundCPUUsed-request.cpu)
			}
			governor.mu.Unlock()
		})
	}, true
}

func (governor *mediaResourceGovernor) acquireContext(ctx context.Context, request mediaResourceRequest) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if release, ok := governor.tryAcquire(request); ok {
			return release, nil
		}
		select {
		case <-ctx.Done():
			if cause := context.Cause(ctx); cause != nil {
				return nil, cause
			}
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func ensureMediaWriteCapacity(path string, minimumFreeBytes int64) error {
	path = filepath.Clean(path)
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			available, _, spaceErr := filesystemSpace(path)
			if spaceErr != nil {
				return spaceErr
			}
			if available < minimumFreeBytes {
				return errMediaStoragePressure
			}
			return nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return os.ErrNotExist
		}
		path = parent
	}
}
