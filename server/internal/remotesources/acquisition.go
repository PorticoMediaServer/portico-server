package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"portico.local/server/internal/supervise"
	"sync"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

type object struct {
	service                    *Service
	binding                    binding
	relative, generation, lane string
	snapshot                   storage.Snapshot
	conditional                *remotemedia.VersionedClient
	native                     *mounts.NativeBackend
	life                       context.Context
	cancel                     context.CancelFunc
	release                    func()
	once                       sync.Once
	mu                         sync.Mutex
	stream                     *io.PipeReader
	finished                   chan error
	cache                      *os.File
	cached, ceiling, floor     int64
	terminal                   error
}

func (s *Service) Acquire(ctx context.Context, path, lane string) (storage.RemoteObject, error) {
	if lane != "playback" && lane != "analysis" {
		return nil, storage.ErrRemoteConfig
	}
	b, relative, ok := s.find(path)
	if !ok || b.Removed || relative == "" {
		return nil, storage.ErrRemoteOffline
	}
	pool := s.playback
	if lane == "analysis" {
		pool = s.inventory
	}
	select {
	case pool <- struct{}{}:
	default:
		return nil, storage.ErrBusy
	}
	life, cancel := context.WithCancel(context.Background())
	token := identity.Token()
	o := &object{service: s, binding: b, relative: relative, lane: lane, life: life, cancel: cancel, ceiling: 4 << 30, floor: 2 << 30}
	if lane == "analysis" {
		o.ceiling = 16 << 20
	}
	s.mu.Lock()
	if s.active[b.ID] == nil {
		s.active[b.ID] = map[string]activeRead{}
	}
	s.active[b.ID][token] = activeRead{cancel, lane}
	s.mu.Unlock()
	o.release = func() {
		s.mu.Lock()
		delete(s.active[b.ID], token)
		if len(s.active[b.ID]) == 0 {
			delete(s.active, b.ID)
		}
		s.mu.Unlock()
		<-pool
	}
	good := false
	defer func() {
		if !good {
			o.Close()
		}
	}()
	var err error
	if b.Kind == "webdav" {
		var d *remotemedia.DAV
		d, o.generation, err = s.dav(ctx, b.ID)
		if err != nil {
			return nil, err
		}
		entry, e := d.Stat(ctx, relative)
		if e != nil {
			return nil, mapError(e)
		}
		if entry.Directory || entry.Size <= 0 {
			return nil, storage.ErrRemoteRange
		}
		o.snapshot = davSnapshot(entry, b.Root)
		o.conditional, err = d.OpenVersion(ctx, relative, remoteVersionScope(b, o.generation), o.snapshot.ObjectID)
		if errors.Is(err, mediasource.ErrIdentityRequired) || errors.Is(err, remotemedia.ErrRepresentationRange) {
			return nil, storage.ErrRemoteRange
		}
		if err != nil {
			return nil, mapError(err)
		}
		ev := o.conditional.Version().Evidence()
		if ev.Size != entry.Size || ev.Revision != entry.ETag {
			return nil, storage.ErrRemoteChanged
		}
	} else {
		o.native, err = s.mounts.Native(ctx, b.ID)
		if err != nil {
			return nil, err
		}
		o.generation = o.native.Generation()
		entry, e := o.native.StatForLane(ctx, relative, lane)
		if e != nil {
			return nil, e
		}
		if entry.IsDir || entry.Size <= 0 {
			return nil, storage.ErrRemoteRange
		}
		o.snapshot = entry.Snapshot(b.Root, relative)
		_, _, cache, floor, e := s.mounts.BackendInfo(ctx, b.ID)
		if e == nil && lane == "playback" && cache > 0 {
			o.ceiling = cache
		}
		if e == nil {
			o.floor = floor
		}
		o.cache, err = os.CreateTemp(s.private, "acquisition-*")
		if err != nil {
			return nil, storage.ErrRemoteLimit
		}
		if err = o.cache.Chmod(0600); err != nil {
			return nil, err
		}
		// A single retained cat stream feeds an owned immutable-prefix cache. Seeking
		// never reopens a mutable provider object based solely on matching size/mtime.
		// This deliberately reports sequential access, not a fabricated strong ETag.
		var writer *io.PipeWriter
		o.stream, writer = io.Pipe()
		o.finished = make(chan error, 1)
		supervise.Go("remotesources.acquire.read", func() {
			err := o.native.Read(life, relative, lane, 0, 0, func(r io.Reader) error {
				n, e := io.Copy(writer, io.LimitReader(r, o.snapshot.Size+1))
				if n > o.snapshot.Size {
					return storage.ErrRemoteChanged
				}
				if e == nil && n != o.snapshot.Size {
					return io.ErrUnexpectedEOF
				}
				return e
			})
			_ = writer.CloseWithError(err)
			o.finished <- err
		})
	}
	if err = s.check(ctx, b, o.generation); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	o.snapshot = inventorySnapshot(b, o.generation, o.snapshot)
	if lane == "playback" {
		var expected string
		e := s.db.QueryRowContext(ctx, `SELECT revision FROM remote_object_refs WHERE path=?`, path).Scan(&expected)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		if expected != "" && expected != o.snapshot.Revision {
			return nil, storage.ErrRemoteChanged
		}
	}
	good = true
	return o, nil
}
func (o *object) Snapshot() storage.Snapshot { return o.snapshot }
func (o *object) Version() mediasource.Version {
	if o.conditional != nil {
		return o.conditional.Version()
	}
	return mediasource.Version{}
}
func (o *object) Access() string {
	if o.conditional != nil {
		return "finite_random"
	}
	return "finite_sequential"
}
func (o *object) bound(parent context.Context) (context.Context, func(), error) {
	if o.life.Err() != nil {
		return nil, nil, storage.ErrRemoteChanged
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	stop := context.AfterFunc(o.life, cancel)
	return ctx, func() { stop(); cancel() }, nil
}
func (o *object) Observe(parent context.Context) (storage.Snapshot, error) {
	ctx, release, err := o.bound(parent)
	if err != nil {
		return storage.Snapshot{}, err
	}
	defer release()
	if err = o.service.check(ctx, o.binding, o.generation); err != nil {
		return storage.Snapshot{}, err
	}
	if o.conditional != nil {
		response, e := o.conditional.Open(ctx, "GET", "bytes=0-0")
		if e != nil {
			return storage.Snapshot{}, mapError(e)
		}
		_, err = io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if err == nil {
			err = closeErr
		}
	} else {
		var entry mounts.NativeEntry
		entry, err = o.native.StatForLane(ctx, o.relative, o.lane)
		if err == nil {
			next := entry.Snapshot(o.binding.Root, o.relative)
			if o.generation+":"+next.Revision != o.snapshot.Revision || next.Size != o.snapshot.Size {
				err = storage.ErrRemoteChanged
			}
		}
	}
	if err != nil {
		o.cancel()
		return storage.Snapshot{}, mapError(err)
	}
	return o.snapshot, nil
}
func (o *object) OpenRange(parent context.Context, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length <= 0 || offset > o.snapshot.Size || length > o.snapshot.Size-offset {
		return nil, storage.ErrRemoteRange
	}
	ctx, release, err := o.bound(parent)
	if err != nil {
		return nil, err
	}
	if err = o.service.check(ctx, o.binding, o.generation); err != nil {
		release()
		return nil, err
	}
	if o.conditional != nil {
		response, e := o.conditional.Open(ctx, http.MethodGet, fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		if e != nil {
			release()
			return nil, mapError(e)
		}
		return &readCloser{Reader: response.Body, close: func() error { err := response.Body.Close(); release(); return err }}, nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.terminal != nil {
		release()
		return nil, o.terminal
	}
	end := offset + length
	if end > o.ceiling {
		release()
		return nil, storage.ErrRemoteLimit
	}
	if !mounts.CacheSpaceAvailable(o.service.private, o.floor) {
		release()
		return nil, storage.ErrRemoteLimit
	}
	// Losing a sequential read is terminal for this acquisition. It may never
	// reopen the source or continue under the old acquisition ID after timeout.
	stop := context.AfterFunc(ctx, func() { o.cancel(); _ = o.stream.CloseWithError(ctx.Err()) })
	defer stop()
	if end > o.cached {
		n, e := io.CopyN(o.cache, o.stream, end-o.cached)
		o.cached += n
		if e != nil {
			o.terminal = mapError(e)
			o.cancel()
			release()
			return nil, o.terminal
		}
	}
	if o.cached == o.snapshot.Size && o.finished != nil {
		// Require the physical stream's exact EOF, including the overrun check,
		// before returning the last bytes. Never silently ignore a late error.
		var extra [1]byte
		n, e := o.stream.Read(extra[:])
		if n != 0 || e != io.EOF {
			o.terminal = storage.ErrRemoteChanged
		}
		if e = <-o.finished; e != nil {
			o.terminal = mapError(e)
		}
		o.finished = nil
		if o.terminal != nil {
			o.cancel()
			release()
			return nil, o.terminal
		}
	}

	if ctx.Err() != nil {
		release()
		return nil, ctx.Err()
	}
	reader := io.NewSectionReader(o.cache, offset, length)
	return &readCloser{Reader: reader, close: func() error { release(); return nil }}, nil
}
func (o *object) Close() error {
	o.once.Do(func() {
		o.cancel()
		if o.conditional != nil {
			o.conditional.Close()
		}
		if o.stream != nil {
			_ = o.stream.Close()
		}
		o.mu.Lock()
		if o.cache != nil {
			name := o.cache.Name()
			_ = o.cache.Close()
			_ = os.Remove(name)
		}
		o.mu.Unlock()
		if o.release != nil {
			o.release()
		}
	})
	return nil
}

type readCloser struct {
	io.Reader
	close func() error
	once  sync.Once
	err   error
}

func (r *readCloser) Close() error { r.once.Do(func() { r.err = r.close() }); return r.err }
