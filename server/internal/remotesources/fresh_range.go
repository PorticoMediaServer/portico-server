package remotesources

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// FreshRangeAdmission binds the caller's real captured operation/actor/job
// authority to a persisted observation and one actual source-process epoch.
// Check must perform authority-only checks, without range/metadata I/O, so it
// can safely be re-entered from a transport dial. There is no default checker.
type FreshRangeAdmission struct {
	Observation       ObservationRef
	ProcessGeneration string
	Check             func(context.Context) error
	OnChanged         func()
}

// OpenFreshRange composes normalized scan facts with the live upstream-rclone
// metadata and range paths. It never copies old provider facts into a fresh
// observation or returns an immutable Version. Undetectable origin changes
// retain the explicitly accepted section15 limitation.
func (s *Service) OpenFreshRange(ctx context.Context, process *rcloneprogram.ServeProcess, admission FreshRangeAdmission) (session *RangeSession, err error) {
	if s == nil || s.db == nil || process == nil || admission.Check == nil || admission.OnChanged == nil || admission.ProcessGeneration == "" {
		return nil, storage.ErrRemoteConfig
	}
	notify := sync.OnceFunc(admission.OnChanged)
	defer func() {
		if errors.Is(err, storage.ErrRemoteChanged) || errors.Is(err, remotemedia.ErrObservationChanged) {
			notify()
		}
	}()
	if process.Generation() != admission.ProcessGeneration {
		return nil, storage.ErrRemoteChanged
	}
	ref := admission.Observation
	id, generation := process.Source()
	if ref.SourceID != id || ref.Generation != generation || !validObservationObject(ref.Object) || ref.Revision < 1 {
		return nil, storage.ErrRemoteChanged
	}
	var root string
	if err := s.db.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, id).Scan(&root); err != nil {
		return nil, storage.ErrRemoteOffline
	}
	if !filepath.IsAbs(root) {
		return nil, storage.ErrRemoteConfig
	}
	scan, err := s.ScanObservation(ctx, filepath.Join(root, filepath.FromSlash(ref.Object)))
	if err != nil {
		return nil, err
	}
	if scan.Ref != ref {
		return nil, storage.ErrRemoteChanged
	}
	check := func(ctx context.Context) error {
		if err := admission.Check(ctx); err != nil {
			return err
		}
		if err := process.Check(ctx); err != nil {
			return err
		}
		var currentRoot string
		if err := s.db.QueryRowContext(ctx, `SELECT root FROM remote_sources WHERE id=? AND removed=0`, id).Scan(&currentRoot); err != nil || currentRoot != root {
			return storage.ErrRemoteChanged
		}
		return s.CheckScanObservation(ctx, ref)
	}
	if err = check(ctx); err != nil {
		return nil, err
	}
	endpoint, err := process.Endpoint(ctx)
	if err != nil {
		return nil, err
	}
	metadata, err := process.Metadata(ctx)
	if err != nil {
		return nil, err
	}
	observer, err := process.ObservationEndpoint(ctx)
	if err != nil {
		return nil, err
	}
	observe := func(ctx context.Context) (remotemedia.FileObservation, error) {
		fresh, err := metadata.Stat(ctx, ref.Object, check)
		if err != nil {
			return remotemedia.FileObservation{}, err
		}
		served, err := observer.Stat(ctx, ref.Object, check)
		if err != nil {
			return remotemedia.FileObservation{}, err
		}
		if fresh.Directory || served.Directory || fresh.Size != served.Size {
			return remotemedia.FileObservation{}, remotemedia.ErrObservationChanged
		}
		facts := remotemedia.FileObservation{SourceID: id, Incarnation: generation, Object: ref.Object, ObjectID: fresh.ID, Size: fresh.Size, ProviderHashes: fresh.Hashes, ServeETag: served.ETag}
		if !fresh.Modified.IsZero() && time.Unix(0, fresh.Modified.UnixNano()).Equal(fresh.Modified) {
			facts.ModifiedKnown, facts.ModifiedNS = true, fresh.Modified.UnixNano()
		}
		return facts.Canonical()
	}
	return OpenRangeSession(ctx, endpoint, RangeAdmission{Scan: scan.Observation.Facts, Check: check, Observe: observe, OnChanged: notify})
}
