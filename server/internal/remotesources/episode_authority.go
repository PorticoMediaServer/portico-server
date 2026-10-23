package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

var ErrEpisodeAuthority = errors.New("episode thumbnail authority is unavailable or refused")

type EpisodePurpose string

const (
	EpisodeAutomatic          EpisodePurpose = "automatic"
	EpisodeExplicitOneEpisode EpisodePurpose = "explicit_one_episode"
)

type EpisodeTarget struct {
	LibraryID, LibrarySourceID, ObjectID, SourceRevision, EpisodeID string
}

type EpisodeAuthorityRequest struct {
	Target               EpisodeTarget
	Purpose              EpisodePurpose
	Operation, CostClass string
	JobID                string
	Owner                identity.Principal
}

// EpisodeState is read from the single library episode-mode writer and current
// episode artwork facts. A cost label, picker policy or default cannot supply
// these facts. Explicit work does not depend on provider-still presence.
type EpisodeState struct {
	PolicyRevision, ArtworkRevision   int64
	Mode                              string
	ProviderStillKnown, ProviderStill bool
}

type EpisodeStateReader interface {
	ReadEpisodeStateTx(context.Context, *sql.Tx, EpisodeTarget) (EpisodeState, error)
}

// ReadCeiling is an explicitly known operational ceiling. Zero means unbounded
// only when Known is true; absent authority must never become an unlimited cap.
type ReadCeiling struct {
	Known  bool
	Limits ReadLimits
}

type EpisodeWorkState struct {
	ServerRevision, LibraryRevision, ProviderRevision int64
	HeavyWindowRevision, AnalysisScheduleRevision     int64
	HeavyWindowOpen, AnalysisScheduleOpen             bool
	Server, Library, Provider                         ReadCeiling
	// State is observed, normalized provider/process state. A response from
	// rclone is not by itself proof of a provider's quota or retry time.
	ProviderState string
}

// EpisodeWorkReader must be backed by the checked scheduler and source-wide
// enforcing resource lease. There is no permissive implementation/default.
// CheckEpisodeReadLease performs a nonblocking local lease check; it must not
// do remote I/O or wait for capacity while a database transaction is held.
type EpisodeWorkReader interface {
	ReadEpisodeWorkTx(context.Context, *sql.Tx, EpisodeTarget, EpisodePurpose) (EpisodeWorkState, error)
	CheckEpisodeReadLease(context.Context, EpisodeTarget, EpisodePurpose, ReadLimits) error
}

type EpisodeAuthorityResolver struct {
	Sources    *Service
	Identities *identity.Service
	Episodes   EpisodeStateReader
	Work       EpisodeWorkReader
}

type episodeAuthoritySnapshot struct {
	Source           catalog.LibrarySource
	File             string
	PolicyRevision   int64
	Episode          EpisodeState
	Profile          SourceReadProfile
	RemoteGeneration string
	Observation      ObservationRef
	Work             EpisodeWorkState
	Limits           ReadLimits
}

// EpisodeAuthority is a captured recheck fence, not a bearer grant. Failure is
// terminal, including a canceled job/actor, changed source or contributing CAS
// revision. No reconnect may resolve it to the current replacement object.
type EpisodeAuthority struct {
	mu       sync.Mutex
	resolver EpisodeAuthorityResolver
	request  EpisodeAuthorityRequest
	captured episodeAuthoritySnapshot
	life     context.Context
	terminal error
}

func validEpisodeRequest(r EpisodeAuthorityRequest) bool {
	if r.Operation != "episode_thumbnail" || r.CostClass != "decode" || r.Target.LibraryID == "" || r.Target.LibrarySourceID == "" || r.Target.ObjectID == "" || r.Target.SourceRevision == "" || r.Target.EpisodeID == "" {
		return false
	}
	switch r.Purpose {
	case EpisodeAutomatic:
		return r.JobID != "" && r.Owner.Hash == ""
	case EpisodeExplicitOneEpisode:
		return r.JobID == "" && r.Owner.Hash != ""
	}
	return false
}

func episodePolicy(purpose EpisodePurpose, state EpisodeState, library catalog.ScanPolicy, remote bool, profile SourceReadProfile) error {
	if state.PolicyRevision < 1 || state.PolicyRevision != library.Revision {
		return ErrEpisodeAuthority
	}
	switch state.Mode {
	case "provider_then_made", "provider", "off":
	default:
		return ErrEpisodeAuthority
	}
	if purpose == EpisodeExplicitOneEpisode {
		return nil
	}
	if purpose != EpisodeAutomatic || state.Mode != "provider_then_made" || !state.ProviderStillKnown || state.ArtworkRevision < 1 || state.ProviderStill || !library.Allows("episode_thumbnail") || remote && (!profile.Saved || profile.ScanDepth != "complete") {
		return ErrEpisodeAuthority
	}
	// A bounded decode is not a whole-file read, irrespective of the profile's
	// whole-file I/O switch. No whole-file operation is authorized here.
	return nil
}

func minimumReadLimits(source ReadLimits, ceilings ...ReadCeiling) (ReadLimits, error) {
	if source.Connections < 0 || source.RequestsPerSecond < 0 || source.BytesPerSecond < 0 {
		return ReadLimits{}, ErrEpisodeAuthority
	}
	out := source
	for _, c := range ceilings {
		if !c.Known || c.Limits.Connections < 0 || c.Limits.RequestsPerSecond < 0 || c.Limits.BytesPerSecond < 0 {
			return ReadLimits{}, ErrEpisodeAuthority
		}
		if c.Limits.Connections > 0 && (out.Connections == 0 || c.Limits.Connections < out.Connections) {
			out.Connections = c.Limits.Connections
		}
		if c.Limits.RequestsPerSecond > 0 && (out.RequestsPerSecond == 0 || c.Limits.RequestsPerSecond < out.RequestsPerSecond) {
			out.RequestsPerSecond = c.Limits.RequestsPerSecond
		}
		if c.Limits.BytesPerSecond > 0 && (out.BytesPerSecond == 0 || c.Limits.BytesPerSecond < out.BytesPerSecond) {
			out.BytesPerSecond = c.Limits.BytesPerSecond
		}
	}
	return out, nil
}

func episodeWorkLimits(purpose EpisodePurpose, state EpisodeWorkState, source ReadLimits) (ReadLimits, error) {
	if state.ServerRevision < 1 || state.LibraryRevision < 1 || state.ProviderRevision < 1 || state.ProviderState != "ready" {
		return ReadLimits{}, ErrEpisodeAuthority
	}
	if purpose == EpisodeAutomatic && (state.HeavyWindowRevision < 1 || state.AnalysisScheduleRevision < 1 || !state.HeavyWindowOpen || !state.AnalysisScheduleOpen) {
		return ReadLimits{}, ErrEpisodeAuthority
	}
	return minimumReadLimits(source, state.Server, state.Library, state.Provider)
}

func (r EpisodeAuthorityResolver) resolveTx(ctx context.Context, tx *sql.Tx, request EpisodeAuthorityRequest) (episodeAuthoritySnapshot, error) {
	var out episodeAuthoritySnapshot
	if tx == nil || r.Sources == nil || r.Sources.db == nil || r.Episodes == nil || r.Work == nil || !validEpisodeRequest(request) {
		return out, ErrEpisodeAuthority
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if request.Purpose == EpisodeExplicitOneEpisode {
		if err := currentLocalOwnerTx(ctx, tx, r.Identities, request.Owner); err != nil {
			return out, err
		}
		if err := identity.DirectAllowed(tx, request.Owner, request.Target.LibraryID); err != nil {
			return out, err
		}
	}
	t := request.Target
	s := &out.Source
	var relative string
	var size int64
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.library_id,s.kind,s.root,s.generation,s.incarnation,s.root_identity,s.enabled,s.identity_confirmed,o.relative_path,o.size FROM library_sources s JOIN inventory_objects o ON o.source_id=s.id JOIN catalog_assets a ON a.token=o.asset_id WHERE s.id=? AND s.library_id=? AND s.enabled=1 AND o.id=? AND o.revision=? AND o.root_incarnation=s.incarnation AND o.retired=0 AND o.state='available' AND a.available=1 AND a.video_codec!='' AND EXISTS(SELECT 1 FROM catalog_asset_links al JOIN catalog_entities e ON e.id=al.entity_id JOIN catalog_libraries l ON l.id=e.library_id JOIN catalog_kinds k ON k.id=e.kind WHERE al.asset_id=a.id AND al.available=1 AND pid(e.public_id)=? AND k.name='episode' AND e.retired=0 AND l.retired=0 AND l.library_id=s.library_id)`, t.LibrarySourceID, t.LibraryID, t.ObjectID, t.SourceRevision, t.EpisodeID).Scan(&s.ID, &s.LibraryID, &s.Kind, &s.ResolvedPath, &s.Generation, &s.Incarnation, &s.RootIdentity, &s.Enabled, &s.IdentityConfirmed, &relative, &size)
	if err != nil {
		return out, err
	}
	if !s.IdentityConfirmed || s.Generation < 1 || s.Incarnation == "" || s.RootIdentity == "" || !filepath.IsAbs(s.ResolvedPath) || filepath.Clean(s.ResolvedPath) != s.ResolvedPath || !validObservationObject(filepath.ToSlash(relative)) || size < 0 {
		return out, ErrEpisodeAuthority
	}
	out.File = filepath.Join(s.ResolvedPath, relative)
	if request.Purpose == EpisodeAutomatic {
		live, e := catalog.InventoryFenceTx(ctx, tx, request.JobID)
		if e != nil {
			return out, e
		}
		if live.ID != s.ID || live.LibraryID != s.LibraryID || live.Generation != s.Generation || live.Incarnation != s.Incarnation || live.RootIdentity != s.RootIdentity || live.ResolvedPath != s.ResolvedPath {
			return out, storage.ErrRemoteChanged
		}
	}
	policy, err := catalog.ScanPolicyTx(ctx, tx, s.LibraryID)
	if err != nil {
		return out, err
	}
	if request.Purpose == EpisodeAutomatic {
		var jobPolicyRevision int64
		if err = tx.QueryRowContext(ctx, `SELECT policy_revision FROM inventory_runs WHERE job_id=?`, request.JobID).Scan(&jobPolicyRevision); err != nil {
			return out, err
		}
		if jobPolicyRevision != policy.Revision {
			return out, ErrEpisodeAuthority
		}
	}
	out.PolicyRevision = policy.Revision
	out.Episode, err = r.Episodes.ReadEpisodeStateTx(ctx, tx, t)
	if err != nil {
		return out, err
	}
	switch s.Kind {
	case "remote":
		b, object, ok := r.Sources.find(out.File)
		if !ok || b.Removed || !validObservationObject(object) {
			return out, storage.ErrRemoteChanged
		}
		var root, support, code string
		if err = tx.QueryRowContext(ctx, `SELECT root,range_support,error_code FROM remote_sources WHERE id=? AND removed=0`, b.ID).Scan(&root, &support, &code); err != nil {
			return out, err
		}
		if root != b.Root {
			return out, storage.ErrRemoteChanged
		}
		if support == "unsupported" || code == "range_unsupported" {
			return out, storage.ErrRemoteRange
		}
		var itemCode string
		err = tx.QueryRowContext(ctx, `SELECT code FROM remote_analysis_status WHERE path=?`, out.File).Scan(&itemCode)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if itemCode == "range_unsupported" {
			return out, storage.ErrRemoteRange
		}
		out.RemoteGeneration, err = observationGenerationTx(ctx, tx, b.ID)
		if err != nil {
			return out, err
		}
		out.Profile, err = SourceReadProfileTx(ctx, tx, b.ID)
		if err != nil {
			return out, err
		}
		observation, e := readScanObservationTx(ctx, tx, b.ID, object)
		if e != nil {
			return out, e
		}
		if observation.Ref.Generation != out.RemoteGeneration || observation.Observation.ScanRevision != t.SourceRevision || observation.Observation.Facts.Size != size || observation.Observation.Facts.ValidateForRange() != nil {
			return out, storage.ErrRemoteChanged
		}
		out.Observation = observation.Ref
	case "local":
	default:
		return out, ErrEpisodeAuthority
	}
	if err = episodePolicy(request.Purpose, out.Episode, policy, s.Kind == "remote", out.Profile); err != nil {
		return out, err
	}
	out.Work, err = r.Work.ReadEpisodeWorkTx(ctx, tx, t, request.Purpose)
	if err != nil {
		return out, err
	}
	out.Limits, err = episodeWorkLimits(request.Purpose, out.Work, out.Profile.Limits)
	if err != nil {
		return out, err
	}
	if request.Purpose == EpisodeExplicitOneEpisode {
		// These facts have no permission meaning for the explicit command and
		// must not accidentally become automatic-tier/still/window fences.
		out.Episode.ArtworkRevision, out.Episode.ProviderStillKnown, out.Episode.ProviderStill = 0, false, false
		out.Work.HeavyWindowRevision, out.Work.AnalysisScheduleRevision = 0, 0
		out.Work.HeavyWindowOpen, out.Work.AnalysisScheduleOpen = false, false
	}
	return out, nil
}

func (r EpisodeAuthorityResolver) ResolveEpisodeAuthorityTx(ctx context.Context, tx *sql.Tx, request EpisodeAuthorityRequest) (*EpisodeAuthority, error) {
	captured, err := r.resolveTx(ctx, tx, request)
	if err != nil {
		return nil, err
	}
	return &EpisodeAuthority{resolver: r, request: request, captured: captured, life: ctx}, nil
}

func (a *EpisodeAuthority) checkTx(ctx context.Context, tx *sql.Tx) error {
	if a.terminal != nil {
		return a.terminal
	}
	if a.life == nil || a.resolver.Sources == nil || a.resolver.Work == nil {
		a.terminal = ErrEpisodeAuthority
		return a.terminal
	}
	err := a.life.Err()
	if err == nil {
		var current episodeAuthoritySnapshot
		current, err = a.resolver.resolveTx(ctx, tx, a.request)
		if err == nil && current != a.captured {
			err = storage.ErrRemoteChanged
		}
	}
	if err == nil {
		err = a.resolver.Work.CheckEpisodeReadLease(ctx, a.request.Target, a.request.Purpose, a.captured.Limits)
	}
	if err != nil {
		a.terminal = err
	}
	return err
}

// CheckTx must also run in the actual artwork publication transaction. A check
// before opening the transaction cannot close a concurrent revocation race.
func (a *EpisodeAuthority) CheckTx(ctx context.Context, tx *sql.Tx) error {
	if a == nil {
		return ErrEpisodeAuthority
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.checkTx(ctx, tx)
}

func (a *EpisodeAuthority) Check(ctx context.Context) error {
	if a == nil {
		return ErrEpisodeAuthority
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.terminal != nil {
		return a.terminal
	}
	if a.resolver.Sources == nil || a.resolver.Sources.db == nil {
		a.terminal = ErrEpisodeAuthority
		return a.terminal
	}
	gated, err := dbwork.BeginSnapshot(ctx, a.resolver.Sources.db)
	if err != nil {
		a.terminal = err
		return err
	}
	defer gated.Rollback()
	return a.checkTx(ctx, gated.Tx())
}

// Authorize has the Episode helper's exact adapter signature. The helper passes
// its actual invocation on every boundary; the captured purpose cannot change.
func (a *EpisodeAuthority) Authorize(ctx context.Context, operation, cost string, manual bool) error {
	if a == nil {
		return ErrEpisodeAuthority
	}
	if operation != "episode_thumbnail" || cost != "decode" || manual != (a.request.Purpose == EpisodeExplicitOneEpisode) {
		a.mu.Lock()
		a.terminal = ErrEpisodeAuthority
		a.mu.Unlock()
		return ErrEpisodeAuthority
	}
	return a.Check(ctx)
}
