package remotesources

import (
	"context"
	"path/filepath"
	"sync/atomic"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
)

// sourceProcessLease owns the one physical serve-process slot for a source.
// It is capacity/custody, not actor, target, operation or cost authorization.
// The command supervisor consumes it and releases only after physical Wait.
type sourceProcessLease struct {
	service    *Service
	source     string
	generation string
	root       string
	profile    SourceReadProfile
	life       context.Context
	cancel     context.CancelFunc
	released   atomic.Bool
}

func (s *Service) reserveSourceProcess(ctx context.Context, source, generation string, profileRevision int64) (*sourceProcessLease, error) {
	if s == nil || s.db == nil || source == "" || generation == "" || profileRevision < 0 {
		return nil, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer snapshot.Rollback()
	current, err := observationGenerationTx(ctx, snapshot.Tx(), source)
	if err != nil {
		return nil, err
	}
	if current != generation {
		return nil, storage.ErrRemoteChanged
	}
	profile, err := SourceReadProfileTx(ctx, snapshot.Tx(), source)
	if err != nil {
		return nil, err
	}
	if profile.Revision != profileRevision {
		return nil, storage.ErrRemoteChanged
	}
	var root string
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, source).Scan(&root); err != nil || !filepath.IsAbs(root) {
		return nil, storage.ErrRemoteConfig
	}
	s.serveMu.Lock()
	defer s.serveMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.serveClosed {
		return nil, storage.ErrRemoteOffline
	}
	if s.serveLeases == nil {
		s.serveLeases = make(map[string]*sourceProcessLease)
	}
	if s.serveLeases[source] != nil {
		return nil, storage.ErrBusy
	}
	life, cancel := context.WithCancel(ctx)
	lease := &sourceProcessLease{service: s, source: source, generation: generation, root: root, profile: profile, life: life, cancel: cancel}
	s.serveLeases[source] = lease
	return lease, nil
}

func (l *sourceProcessLease) Context() context.Context { return l.life }

// CheckLocal is safe inside a caller-owned transaction: it never opens another
// DB snapshot, performs remote I/O, or waits for capacity. The caller's actual
// authority reader must compare its captured generation/profile/root there.
func (l *sourceProcessLease) CheckLocal() error {
	if l == nil || l.released.Load() {
		return storage.ErrRemoteOffline
	}
	return l.life.Err()
}

func (l *sourceProcessLease) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := l.CheckLocal(); err != nil {
		return err
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, l.service.db)
	if err != nil {
		return err
	}
	defer snapshot.Rollback()
	current, err := observationGenerationTx(ctx, snapshot.Tx(), l.source)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		l.cancel()
		return err
	}
	profile, err := SourceReadProfileTx(ctx, snapshot.Tx(), l.source)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		l.cancel()
		return err
	}
	var root string
	err = snapshot.Tx().QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, l.source).Scan(&root)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || root != l.root || current != l.generation || profile != l.profile {
		l.cancel()
		return storage.ErrRemoteChanged
	}
	return l.CheckLocal()
}

func (l *sourceProcessLease) Release() {
	if !l.released.CompareAndSwap(false, true) {
		return
	}
	l.cancel()
	l.service.serveMu.Lock()
	defer l.service.serveMu.Unlock()
	if l.service.serveLeases[l.source] == l {
		delete(l.service.serveLeases, l.source)
	}
}

func (s *Service) stopServeProcess(source string) {
	s.serveMu.Lock()
	defer s.serveMu.Unlock()
	if lease := s.serveLeases[source]; lease != nil {
		lease.cancel()
	}
}

func (s *Service) stopServeProcesses() {
	s.serveMu.Lock()
	defer s.serveMu.Unlock()
	s.serveClosed = true
	for _, lease := range s.serveLeases {
		lease.cancel()
	}
}

// ReadProfileCommitted applies a committed profile change to process custody.
// It never changes root/source identity and never releases a physical slot.
// A missed notification is also caught by Check's actual DB/profile recheck.
func (s *Service) ReadProfileCommitted(ctx context.Context, source string, revision int64) error {
	if s == nil || s.db == nil || source == "" || revision < 0 {
		return storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return err
	}
	defer snapshot.Rollback()
	profile, err := SourceReadProfileTx(ctx, snapshot.Tx(), source)
	if err != nil {
		return err
	}
	if profile.Revision != revision {
		return storage.ErrRemoteChanged
	}
	s.serveMu.Lock()
	defer s.serveMu.Unlock()
	if lease := s.serveLeases[source]; lease != nil && lease.profile != profile {
		lease.cancel()
	}
	return nil
}
