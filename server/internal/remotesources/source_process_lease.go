package remotesources

import (
	"context"
	"path/filepath"
	"sync/atomic"

	"os"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// sourceProcessLease owns the one physical serve-process slot for a source.
// It is capacity/custody, not actor, target, operation or cost authorization.
// The command supervisor consumes it and releases only after physical Wait.
type sourceProcessLease struct {
	service            *Service
	source             string
	generation         string
	root               string
	profile            SourceReadProfile
	life               context.Context
	cancel             context.CancelFunc
	released           atomic.Bool
	process            *rcloneprogram.ServeProcess
	proxy              *remotemedia.PolicyProxy
	cache              string
	sequential         bool
	networkFingerprint string
	capabilities       backendCapabilities
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
	if s.serveLeases[source] != nil || s.sourceTests[source] != nil {
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
	var native *nativeSourceAuthority
	var imported *importedSourceConfiguration
	if l.networkFingerprint != "" {
		network, e := l.service.sourceNetworkTx(ctx, snapshot.Tx(), l.source, l.root)
		sequential, se := sourceSequentialTx(ctx, snapshot.Tx(), l.source, l.root)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e != nil || se != nil || network.Fingerprint != l.networkFingerprint || sequential != l.sequential {
			l.cancel()
			return storage.ErrRemoteChanged
		}
		native = network.Native
		imported = network.Imported
	}
	// DNS/provider checks happen outside the snapshot. A reconnect is admitted
	// only against this source's saved native approval; the hidden rclone dial
	// still has the explicitly accepted check/connect gap for hostnames.
	snapshot.Rollback()
	if native != nil {
		_, err = (remotemedia.Policy{Resolver: l.service.resolver}).CheckNativeEndpoint(ctx, native.endpoint, native.config.Approval)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			l.cancel()
			return err
		}
	}
	if imported != nil {
		if err = l.service.checkImportedApprovals(ctx, *imported); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			l.cancel()
			return err
		}
	}
	return l.CheckLocal()
}

func (l *sourceProcessLease) Release() {
	if !l.released.CompareAndSwap(false, true) {
		return
	}
	l.cancel()
	if l.proxy != nil {
		l.proxy.Close()
		_ = l.proxy.Wait(context.Background())
	}
	if l.cache != "" {
		_ = os.RemoveAll(l.cache)
	}
	l.service.serveMu.Lock()
	process := l.process
	if l.service.serveLeases[l.source] == l {
		delete(l.service.serveLeases, l.source)
	}
	var readers []*SourceReadLease
	for _, reader := range l.service.readLeases[l.source] {
		readers = append(readers, reader)
	}
	l.service.serveMu.Unlock()
	// A physical exit ends only readers bound to that exact process epoch.
	// A provisional reader which never opened bytes has no retired epoch.
	if process != nil {
		for _, reader := range readers {
			reader.mu.Lock()
			same := reader.processGeneration == process.Generation()
			reader.mu.Unlock()
			if same {
				reader.fail(storage.ErrRemoteOffline)
			}
		}
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
	s.serveClosed = true
	for _, scope := range s.sourceTests {
		scope.cancel()
	}
	for _, lease := range s.serveLeases {
		lease.cancel()
	}
	s.serveMu.Unlock()
	s.stopReadLeases("", storage.ErrRemoteOffline, nil)
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
	snapshot.Rollback()
	if err != nil {
		return err
	}
	if profile.Revision != revision {
		return storage.ErrRemoteChanged
	}
	s.retireReadProfile(source, profile)
	return nil
}

func (s *Service) retireReadProfile(source string, profile SourceReadProfile) {
	s.serveMu.Lock()
	if lease := s.serveLeases[source]; lease != nil && lease.profile != profile {
		lease.cancel()
	}
	s.serveMu.Unlock()
	s.stopReadLeases(source, storage.ErrRemoteChanged, &profile)
}
