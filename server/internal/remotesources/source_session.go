package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"sync"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/storage"
)

type SourceReadPurpose string

const (
	SourceReading          SourceReadPurpose = "reading"
	SourcePlayback         SourceReadPurpose = "playback"
	SourceInventory        SourceReadPurpose = "inventory"
	SourceEpisodeAutomatic SourceReadPurpose = "automatic_episode_thumbnail"
	SourceEpisodeExplicit  SourceReadPurpose = "explicit_episode_thumbnail"
)

// SourceReadBinding is the exact source object passed back to the real caller's
// purpose-specific authority resolver. A purpose/pool label grants nothing.
type SourceReadBinding struct {
	Purpose                  SourceReadPurpose
	Observation              ObservationRef
	Path, Root, ScanRevision string
	Size                     int64
	Profile                  SourceReadProfile
	LeaseID                  int64
}

type SourceSessionAdmission struct {
	Purpose     SourceReadPurpose
	Observation ObservationRef
	// CheckTx must resolve the actual actor/session or job, library, target and
	// operation/cost against THIS binding. No default or generic Allow exists.
	CheckTx   func(context.Context, *sql.Tx, SourceReadBinding) error
	OnChanged func()
}

// SourceReadLease belongs to one actual caller session/job. It holds no public
// bearer ID and cannot be retargeted after capture or a physical-process change.
// Capture is DB-only; all provider I/O is lazy and outside caller transactions.
type SourceReadLease struct {
	service           *Service
	binding           SourceReadBinding
	admission         SourceSessionAdmission
	life              context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	terminal          error
	processGeneration string
	notified          bool
}

func (s *Service) CaptureReadSourceTx(ctx context.Context, tx *sql.Tx, admission SourceSessionAdmission) (*SourceReadLease, error) {
	if s == nil || s.db == nil || tx == nil || admission.CheckTx == nil || admission.OnChanged == nil {
		return nil, storage.ErrRemoteConfig
	}
	switch admission.Purpose {
	case SourceReading, SourcePlayback, SourceInventory, SourceEpisodeAutomatic, SourceEpisodeExplicit:
	default:
		return nil, storage.ErrRemoteConfig
	}
	ref := admission.Observation
	if !validObservationObject(ref.Object) || ref.Revision < 1 {
		return nil, storage.ErrRemoteConfig
	}
	generation, err := observationGenerationTx(ctx, tx, ref.SourceID)
	if err != nil {
		return nil, err
	}
	if generation != ref.Generation {
		return nil, storage.ErrRemoteChanged
	}
	profile, err := SourceReadProfileTx(ctx, tx, ref.SourceID)
	if err != nil {
		return nil, err
	}
	scan, err := readScanObservationTx(ctx, tx, ref.SourceID, ref.Object)
	if err != nil {
		return nil, err
	}
	if scan.Ref != ref {
		return nil, storage.ErrRemoteChanged
	}
	var root string
	if err = tx.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, ref.SourceID).Scan(&root); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, storage.ErrRemoteConfig
	}
	id := s.readLeaseSequence.Add(1)
	if id < 1 {
		return nil, storage.ErrRemoteLimit
	}
	binding := SourceReadBinding{Purpose: admission.Purpose, Observation: ref, Path: filepath.Join(root, filepath.FromSlash(ref.Object)), Root: root, ScanRevision: scan.Observation.ScanRevision, Size: scan.Observation.Facts.Size, Profile: profile, LeaseID: id}
	if err = admission.CheckTx(ctx, tx, binding); err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(ctx)
	return &SourceReadLease{service: s, binding: binding, admission: admission, life: life, cancel: cancel}, nil
}
func (l *SourceReadLease) Binding() SourceReadBinding { return l.binding }
func (l *SourceReadLease) CheckTx(ctx context.Context, tx *sql.Tx) error {
	if l == nil || tx == nil {
		return storage.ErrRemoteConfig
	}
	l.mu.Lock()
	terminal := l.terminal
	l.mu.Unlock()
	if terminal != nil {
		return terminal
	}
	if err := ctx.Err(); err != nil {
		return l.fail(err)
	}
	if err := l.life.Err(); err != nil {
		return l.fail(err)
	}
	if err := l.admission.CheckTx(ctx, tx, l.binding); err != nil {
		return l.fail(err)
	}
	generation, err := observationGenerationTx(ctx, tx, l.binding.Observation.SourceID)
	if err != nil {
		return l.fail(err)
	}
	profile, err := SourceReadProfileTx(ctx, tx, l.binding.Observation.SourceID)
	if err != nil {
		return l.fail(err)
	}
	scan, err := readScanObservationTx(ctx, tx, l.binding.Observation.SourceID, l.binding.Observation.Object)
	if err != nil {
		return l.fail(err)
	}
	var root string
	err = tx.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, l.binding.Observation.SourceID).Scan(&root)
	if err != nil {
		return l.fail(err)
	}
	if generation != l.binding.Observation.Generation || root != l.binding.Root || profile != l.binding.Profile || scan.Ref != l.binding.Observation || scan.Observation.ScanRevision != l.binding.ScanRevision || scan.Observation.Facts.Size != l.binding.Size {
		return l.fail(storage.ErrRemoteChanged)
	}
	l.mu.Lock()
	epoch := l.processGeneration
	l.mu.Unlock()
	if epoch != "" {
		l.service.serveMu.Lock()
		slot := l.service.serveLeases[l.binding.Observation.SourceID]
		l.service.serveMu.Unlock()
		if slot == nil || slot.CheckLocal() != nil {
			return l.fail(storage.ErrRemoteOffline)
		}
		l.service.serveMu.Lock()
		process := slot.process
		l.service.serveMu.Unlock()
		if process == nil || process.Generation() != epoch || process.CheckLocal(ctx) != nil {
			return l.fail(storage.ErrRemoteOffline)
		}
		network, e := l.service.sourceNetworkTx(ctx, tx, slot.source, slot.root)
		sequential, se := sourceSequentialTx(ctx, tx, slot.source, slot.root)
		if e != nil {
			return l.fail(e)
		}
		if se != nil {
			return l.fail(se)
		}
		if network.Fingerprint != slot.networkFingerprint || sequential != slot.sequential {
			slot.cancel()
			return l.fail(storage.ErrRemoteChanged)
		}
	}
	return nil
}
func (l *SourceReadLease) Check(ctx context.Context) error {
	if l == nil {
		return storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, l.service.db)
	if err != nil {
		return l.fail(err)
	}
	err = l.CheckTx(ctx, snapshot.Tx())
	snapshot.Rollback()
	l.notifyChanged()
	return err
}
func (l *SourceReadLease) fail(err error) error {
	l.mu.Lock()
	first := l.terminal == nil
	if first {
		l.terminal = err
		l.cancel()
	}
	out := l.terminal
	l.mu.Unlock()
	return out
}
func (l *SourceReadLease) notifyChanged() {
	l.mu.Lock()
	notify := !l.notified && errors.Is(l.terminal, storage.ErrRemoteChanged)
	if notify {
		l.notified = true
	}
	l.mu.Unlock()
	if notify {
		l.admission.OnChanged()
	}
}
func (l *SourceReadLease) changed() {
	// Retire this source's old VFS cache only after its physical process Wait.
	l.service.stopServeProcess(l.binding.Observation.SourceID)
	l.fail(storage.ErrRemoteChanged)
	l.notifyChanged()
}
func (l *SourceReadLease) Close() error { l.fail(storage.ErrRemoteOffline); return nil }
func (l *SourceReadLease) bound(ctx context.Context) (context.Context, func()) {
	call, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(l.life, cancel)
	return call, func() { stop(); cancel() }
}
func (l *SourceReadLease) process(ctx context.Context) (*rcloneprogram.ServeProcess, error) {
	if err := l.Check(ctx); err != nil {
		return nil, err
	}
	l.mu.Lock()
	expected := l.processGeneration
	l.mu.Unlock()
	process, err := l.service.ensureSourceServe(ctx, l.binding, expected)
	if err != nil {
		if errors.Is(err, storage.ErrBusy) {
			return nil, err
		}
		return nil, l.fail(err)
	}
	l.mu.Lock()
	if l.processGeneration == "" {
		l.processGeneration = process.Generation()
	}
	changed := l.processGeneration != process.Generation()
	l.mu.Unlock()
	if changed {
		return nil, l.fail(storage.ErrRemoteChanged)
	}
	if err = l.Check(ctx); err != nil {
		return nil, err
	}
	return process, nil
}
func (l *SourceReadLease) OpenRange(ctx context.Context) (*SourceRange, error) {
	call, release := l.bound(ctx)
	defer release()
	process, err := l.process(call)
	if err != nil {
		return nil, err
	}
	var unsupported bool
	err = l.service.db.QueryRowContext(call, `SELECT EXISTS(SELECT 1 FROM remote_sources WHERE id=? AND (range_support='unsupported' OR error_code='range_unsupported')) OR EXISTS(SELECT 1 FROM remote_analysis_status WHERE path=? AND code='range_unsupported')`, l.binding.Observation.SourceID, l.binding.Path).Scan(&unsupported)
	if err != nil {
		return nil, l.fail(err)
	}
	if unsupported {
		return nil, storage.ErrRemoteRange
	}
	session, err := l.service.OpenFreshRange(call, process, FreshReadAdmission{Observation: l.binding.Observation, ProcessGeneration: process.Generation(), Check: l.Check, OnChanged: l.changed})
	if err != nil {
		return nil, l.fail(err)
	}
	return &SourceRange{lease: l, session: session}, nil
}
func (l *SourceReadLease) OpenSequential(ctx context.Context) (*SourceSequential, error) {
	if l.binding.Purpose != SourceReading && l.binding.Purpose != SourcePlayback {
		return nil, storage.ErrRemoteRange
	}
	call, release := l.bound(ctx)
	process, err := l.process(call)
	if err != nil {
		release()
		return nil, err
	}
	session, err := l.service.OpenFreshSequential(call, process, FreshReadAdmission{Observation: l.binding.Observation, ProcessGeneration: process.Generation(), Check: l.Check, OnChanged: l.changed})
	if err != nil {
		release()
		return nil, l.fail(err)
	}
	return &SourceSequential{lease: l, session: session, ctx: call, release: release}, nil
}

type SourceRange struct {
	lease   *SourceReadLease
	session *RangeSession
}

func (r *SourceRange) ReadAt(ctx context.Context, p []byte, offset int64) (int, error) {
	call, release := r.lease.bound(ctx)
	defer release()
	n, err := r.session.ReadAt(call, p, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		r.lease.fail(err)
		r.lease.notifyChanged()
	}
	return n, err
}
func (r *SourceRange) Check(ctx context.Context) error {
	call, release := r.lease.bound(ctx)
	defer release()
	if err := r.session.Check(call); err != nil {
		return r.lease.fail(err)
	}
	return nil
}
func (r *SourceRange) Close() error { return r.session.Close() }

type SourceSequential struct {
	lease   *SourceReadLease
	session *SequentialSession
	ctx     context.Context
	release func()
	once    sync.Once
}

func (r *SourceSequential) Read(p []byte) (int, error) {
	n, err := r.session.Read(r.ctx, p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.lease.fail(err)
		r.lease.notifyChanged()
	}
	return n, err
}
func (r *SourceSequential) Check(ctx context.Context) error {
	call, release := r.lease.bound(ctx)
	defer release()
	if err := r.session.Check(call); err != nil {
		return r.lease.fail(err)
	}
	return nil
}
func (r *SourceSequential) StopAndRecheck(ctx context.Context) error {
	err := r.session.StopAndRecheck(ctx)
	r.lease.fail(storage.ErrRemoteOffline)
	return err
}
func (r *SourceSequential) Close() error {
	r.once.Do(func() { r.session.Close(); r.release() })
	return nil
}
