// Package optimizedartifact implements durable publication of optimized media
// artifacts. It deliberately owns no concrete filesystem or database adapter;
// callers inject both boundaries so publication can be fault-tested precisely.
package optimizedartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type Stage string

const (
	StageReserved  Stage = "reserved"
	StageCreated   Stage = "temp_created"
	StageSynced    Stage = "temp_synced"
	StageValidated Stage = "validated"
	StageRenamed   Stage = "renamed"
	StageDirSynced Stage = "directory_synced"
	StageCommitted Stage = "committed"
)

type IdentityInput struct {
	Root, MediaID, PresetVersion, SourceFingerprint, PlanDigest, Extension string
}

type Identity struct {
	GenerationID, Directory, FinalPath, TempPath, MarkerID string
}

// DeriveIdentity never places owner media identifiers or fingerprints in a path.
func DeriveIdentity(in IdentityInput) (Identity, error) {
	if strings.TrimSpace(in.Root) == "" || strings.TrimSpace(in.MediaID) == "" ||
		strings.TrimSpace(in.PresetVersion) == "" || strings.TrimSpace(in.SourceFingerprint) == "" ||
		strings.TrimSpace(in.PlanDigest) == "" {
		return Identity{}, publicError("invalid_identity", nil)
	}
	mediaKey := digest("media\x00" + in.MediaID)[:24]
	generation := digest(strings.Join([]string{in.MediaID, in.PresetVersion, in.SourceFingerprint, in.PlanDigest}, "\x00"))
	ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(in.Extension)), ".")
	if ext == "" {
		ext = "bin"
	}
	for _, r := range ext {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return Identity{}, publicError("invalid_identity", nil)
		}
	}
	dir := filepath.Join(filepath.Clean(in.Root), mediaKey)
	final := filepath.Join(dir, generation+"."+ext)
	return Identity{GenerationID: generation, Directory: dir, FinalPath: final,
		TempPath: final + ".partial", MarkerID: generation + ".publication"}, nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type Reservation interface{ Release() }

type PrivateFile interface {
	io.Writer
	Sync() error
	Close() error
}

// Filesystem operations must preserve private permissions for files and markers.
type Filesystem interface {
	Reserve(context.Context, string, int64) (Reservation, error)
	CreatePrivate(string) (PrivateFile, error)
	Rename(string, string) error
	SyncDirectory(string) error
	Remove(string) error
	Exists(string) (bool, error)
	PutMarker(context.Context, Marker) error
	DeleteMarker(context.Context, string) error
}

type Metadata struct {
	GenerationID, MediaID, PresetVersion, SourceFingerprint, PlanDigest, Path string
	SizeBytes                                                                 int64
	PublishedAt                                                               time.Time
}

// Store.Publish must atomically install next and return the previously published
// generation. The previous artifact must not be removed inside that transaction.
type Store interface {
	Publish(context.Context, Metadata) (*Metadata, error)
	Current(context.Context, string, string) (*Metadata, error)
}

type Marker struct {
	ID        string
	Stage     Stage
	Metadata  Metadata
	TempPath  string
	FinalPath string
}

type Request struct {
	Identity       IdentityInput
	PredictedBytes int64
	Produce        func(context.Context, io.Writer) error
	Validate       func(context.Context, string) (int64, error)
	Now            func() time.Time
}

type Result struct {
	Metadata Metadata
	Previous *Metadata
}

type Publisher struct {
	FS    Filesystem
	Store Store
}

func (p Publisher) Publish(ctx context.Context, req Request) (Result, error) {
	id, err := DeriveIdentity(req.Identity)
	if err != nil {
		return Result{}, err
	}
	if p.FS == nil || p.Store == nil || req.Produce == nil || req.Validate == nil || req.PredictedBytes < 0 {
		return Result{}, publicError("invalid_request", nil)
	}
	reservation, err := p.FS.Reserve(ctx, id.Directory, req.PredictedBytes)
	if err != nil {
		return Result{}, classify("space_reservation_failed", err)
	}
	if reservation == nil {
		return Result{}, publicError("space_reservation_failed", nil)
	}
	defer reservation.Release()
	meta := Metadata{GenerationID: id.GenerationID, MediaID: req.Identity.MediaID, PresetVersion: req.Identity.PresetVersion,
		SourceFingerprint: req.Identity.SourceFingerprint, PlanDigest: req.Identity.PlanDigest, Path: id.FinalPath}
	marker := Marker{ID: id.MarkerID, Stage: StageReserved, Metadata: meta, TempPath: id.TempPath, FinalPath: id.FinalPath}
	if err = p.FS.PutMarker(ctx, marker); err != nil {
		return Result{}, classify("marker_write_failed", err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = p.FS.Remove(id.TempPath)
		}
	}()
	f, err := p.FS.CreatePrivate(id.TempPath)
	if err != nil {
		return Result{}, classify("temp_create_failed", err)
	}
	marker.Stage = StageCreated
	if err = p.FS.PutMarker(ctx, marker); err != nil {
		_ = f.Close()
		return Result{}, classify("marker_write_failed", err)
	}
	if err = req.Produce(ctx, f); err != nil {
		_ = f.Close()
		return Result{}, classify("produce_failed", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return Result{}, classify("file_sync_failed", err)
	}
	if err = f.Close(); err != nil {
		return Result{}, classify("file_close_failed", err)
	}
	marker.Stage = StageSynced
	if err = p.FS.PutMarker(ctx, marker); err != nil {
		return Result{}, classify("marker_write_failed", err)
	}
	meta.SizeBytes, err = req.Validate(ctx, id.TempPath)
	if err != nil {
		return Result{}, classify("validation_failed", err)
	}
	if meta.SizeBytes < 0 {
		return Result{}, publicError("validation_failed", nil)
	}
	marker.Metadata = meta
	marker.Stage = StageValidated
	if err = p.FS.PutMarker(ctx, marker); err != nil {
		return Result{}, classify("marker_write_failed", err)
	}
	if err = p.FS.Rename(id.TempPath, id.FinalPath); err != nil {
		return Result{}, classify("rename_failed", err)
	}
	cleanupTemp = false
	marker.Stage = StageRenamed
	if err = p.FS.PutMarker(ctx, marker); err != nil {
		return Result{}, classify("marker_write_failed", err)
	}
	if err = p.FS.SyncDirectory(id.Directory); err != nil {
		return Result{}, classify("directory_sync_failed", err)
	}
	now := time.Now().UTC()
	if req.Now != nil {
		now = req.Now().UTC()
	}
	meta.PublishedAt = now
	marker.Metadata = meta
	marker.Stage = StageDirSynced
	if err = p.FS.PutMarker(ctx, marker); err != nil {
		return Result{}, classify("marker_write_failed", err)
	}
	previous, err := p.Store.Publish(ctx, meta)
	if err != nil {
		return Result{}, classify("metadata_publish_failed", err)
	}
	marker.Stage = StageCommitted
	_ = p.FS.PutMarker(ctx, marker)
	_ = p.FS.DeleteMarker(ctx, marker.ID)
	return Result{Metadata: meta, Previous: previous}, nil
}

type ReconcileOutcome string

const (
	ReconcileCommitted ReconcileOutcome = "committed"
	ReconcileCleaned   ReconcileOutcome = "cleaned"
	ReconcileNoop      ReconcileOutcome = "noop"
)

// Reconcile either completes a durably renamed publication or removes an
// incomplete private artifact. A currently published generation is never removed.
func (p Publisher) Reconcile(ctx context.Context, marker Marker) (ReconcileOutcome, error) {
	if p.FS == nil || p.Store == nil || marker.ID == "" {
		return ReconcileNoop, publicError("invalid_request", nil)
	}
	current, err := p.Store.Current(ctx, marker.Metadata.MediaID, marker.Metadata.PresetVersion)
	if err != nil {
		return ReconcileNoop, classify("metadata_read_failed", err)
	}
	if current != nil && current.GenerationID == marker.Metadata.GenerationID {
		_ = p.FS.Remove(marker.TempPath)
		_ = p.FS.DeleteMarker(ctx, marker.ID)
		return ReconcileCommitted, nil
	}
	finalExists, err := p.FS.Exists(marker.FinalPath)
	if err != nil {
		return ReconcileNoop, classify("artifact_read_failed", err)
	}
	if (marker.Stage == StageDirSynced || marker.Stage == StageCommitted) && finalExists {
		if _, err = p.Store.Publish(ctx, marker.Metadata); err != nil {
			return ReconcileNoop, classify("metadata_publish_failed", err)
		}
		_ = p.FS.Remove(marker.TempPath)
		_ = p.FS.DeleteMarker(ctx, marker.ID)
		return ReconcileCommitted, nil
	}
	_ = p.FS.Remove(marker.TempPath)
	if finalExists {
		_ = p.FS.Remove(marker.FinalPath)
		_ = p.FS.SyncDirectory(filepath.Dir(marker.FinalPath))
	}
	_ = p.FS.DeleteMarker(ctx, marker.ID)
	return ReconcileCleaned, nil
}

type Superseded struct {
	PublishedAt  time.Time
	SizeBytes    int64
	ActiveLeases int
}

type RetentionPolicy struct {
	MinimumAge time.Duration
	MaximumAge time.Duration
}

type RetentionDecision string

const (
	RetentionKeep        RetentionDecision = "keep"
	RetentionDelete      RetentionDecision = "delete"
	RetentionDeferLeased RetentionDecision = "defer_leased"
)

// DecideSupersededRetention provides an explicit bounded decision. MaximumAge
// is the hard collection bound once readers release their leases; MinimumAge
// prevents a newly superseded generation from being removed early.
func DecideSupersededRetention(item Superseded, now time.Time, policy RetentionPolicy) RetentionDecision {
	if item.ActiveLeases > 0 {
		return RetentionDeferLeased
	}
	age := now.Sub(item.PublishedAt)
	if policy.MaximumAge > 0 && age >= policy.MaximumAge {
		return RetentionDelete
	}
	if policy.MinimumAge < 0 || age < policy.MinimumAge {
		return RetentionKeep
	}
	return RetentionDelete
}

// RetainSuperseded makes garbage collection bounded and lease-safe.
func RetainSuperseded(item Superseded, now time.Time, minimumRetention time.Duration) bool {
	return DecideSupersededRetention(item, now, RetentionPolicy{MinimumAge: minimumRetention}) != RetentionDelete
}

type Error struct {
	Code  string
	cause error
}

func (e *Error) Error() string                   { return e.Code }
func (e *Error) Unwrap() error                   { return e.cause }
func publicError(code string, cause error) error { return &Error{Code: code, cause: cause} }
func classify(code string, err error) error {
	if errors.Is(err, ErrNoSpace) {
		code = "insufficient_space"
	}
	return publicError(code, err)
}

var ErrNoSpace = errors.New("optimized artifact capacity exhausted")
