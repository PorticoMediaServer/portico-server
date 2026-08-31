package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/optimized"
	"github.com/PorticoMediaServer/portico-server/internal/optimizedartifact"
)

// optimizedV2SourceIdentity is deliberately separate from filesystem metadata.
// The caller must derive these values from the canonical analyzed media facts.
type optimizedV2SourceIdentity struct {
	Revision    string
	Fingerprint string
	FactsDigest string
	Facts       optimized.SourceFacts
}

type optimizedV2OutputFacts struct {
	SizeBytes         int64
	ArtifactSHA256    string
	Container         string
	VideoCodec        string
	AudioCodec        string
	Width             int
	Height            int
	Bitrate           int
	DurationSeconds   int
	FactsDigest       string
	FactsJSON         json.RawMessage
	SampleAspectRatio string
	FieldOrder        string
	PixelFormat       string
	ColorPrimaries    string
	ColorTransfer     string
	ColorMatrix       string
	AudioLayout       string
	AudioChannels     int
}

type optimizedV2Publication struct {
	server            *Server
	root              string
	mediaID           string
	preset            optimized.Preset
	plan              optimized.OutputPlan
	planJSON          []byte
	planDigest        string
	source            optimizedV2SourceIdentity
	identity          optimizedartifact.Identity
	compatibilityJSON []byte
	output            optimizedV2OutputFacts
	now               func() time.Time
}

// optimizedLocalFilesystem is the production durable adapter for optimized
// artifacts. Marker replacement and directory sync make every state transition
// crash-observable; private modes prevent media paths or bytes leaking to other
// local users.
type optimizedLocalFilesystem struct {
	root   string
	server *Server
}
type optimizedLocalReservation struct{ release func() }

func (s *Server) acquireOptimizedArtifactLease(id string) (func(), bool) {
	id = strings.TrimSpace(id)
	if s == nil || id == "" {
		return nil, false
	}
	s.optimizedArtifactMu.Lock()
	defer s.optimizedArtifactMu.Unlock()
	if s.optimizedArtifactDeleting[id] {
		return nil, false
	}
	if s.optimizedArtifactLeases == nil {
		s.optimizedArtifactLeases = map[string]int{}
	}
	s.optimizedArtifactLeases[id]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.optimizedArtifactMu.Lock()
			defer s.optimizedArtifactMu.Unlock()
			if s.optimizedArtifactLeases[id] <= 1 {
				delete(s.optimizedArtifactLeases, id)
			} else {
				s.optimizedArtifactLeases[id]--
			}
		})
	}, true
}

func (s *Server) claimOptimizedArtifactDeletion(id string) (func(), bool) {
	id = strings.TrimSpace(id)
	if s == nil || id == "" {
		return nil, false
	}
	s.optimizedArtifactMu.Lock()
	defer s.optimizedArtifactMu.Unlock()
	if s.optimizedArtifactDeleting == nil {
		s.optimizedArtifactDeleting = map[string]bool{}
	}
	if s.optimizedArtifactDeleting[id] || s.optimizedArtifactLeases[id] > 0 {
		return nil, false
	}
	s.optimizedArtifactDeleting[id] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			s.optimizedArtifactMu.Lock()
			delete(s.optimizedArtifactDeleting, id)
			s.optimizedArtifactMu.Unlock()
		})
	}, true
}

func (r optimizedLocalReservation) Release() {
	if r.release != nil {
		r.release()
	}
}

func (f optimizedLocalFilesystem) Reserve(_ context.Context, directory string, predicted int64) (optimizedartifact.Reservation, error) {
	if !pathInsideRoot(filepath.Clean(directory), filepath.Clean(f.root)) {
		return nil, errors.New("optimized artifact directory is outside configured storage")
	}
	if err := validateOptimizedManagedPath(f.root, directory, true, true); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := validateOptimizedManagedPath(f.root, directory, false, true); err != nil {
		return nil, err
	}
	required := predicted
	if required < mediaWriteMinimumFreeBytes {
		required = mediaWriteMinimumFreeBytes
	}
	if err := ensureMediaWriteCapacity(directory, required); err != nil {
		return nil, err
	}
	if f.server == nil {
		return nil, errors.New("optimized artifact disk reservation is unavailable")
	}
	release, err := f.server.mediaResourceGovernor().reserveMediaDisk(directory, predicted, mediaDiskReservationMinimum)
	if err != nil {
		return nil, err
	}
	return optimizedLocalReservation{release: release}, nil
}
func (f optimizedLocalFilesystem) CreatePrivate(path string) (optimizedartifact.PrivateFile, error) {
	if err := validateOptimizedManagedPath(f.root, path, true, false); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}
func (f optimizedLocalFilesystem) Rename(from, to string) error {
	if err := validateOptimizedManagedPath(f.root, from, false, false); err != nil {
		return err
	}
	if err := validateOptimizedManagedPath(f.root, to, true, false); err != nil {
		return err
	}
	if filepath.Dir(filepath.Clean(from)) != filepath.Dir(filepath.Clean(to)) {
		return errors.New("optimized artifact rename must remain in one managed directory")
	}
	return os.Rename(from, to)
}
func (f optimizedLocalFilesystem) Remove(path string) error {
	if err := validateOptimizedManagedPath(f.root, path, true, false); err != nil {
		return err
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (f optimizedLocalFilesystem) Exists(path string) (bool, error) {
	if err := validateOptimizedManagedPath(f.root, path, true, false); err != nil {
		return false, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("optimized artifact is not a regular managed file")
	}
	return true, nil
}
func (f optimizedLocalFilesystem) SyncDirectory(path string) error {
	if err := validateOptimizedManagedPath(f.root, path, false, true); err != nil {
		return err
	}
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (f optimizedLocalFilesystem) markerDirectory() string {
	return filepath.Join(filepath.Clean(f.root), ".publications")
}
func (f optimizedLocalFilesystem) markerPath(id string) string {
	return filepath.Join(f.markerDirectory(), safePathComponent(id)+".json")
}
func (f optimizedLocalFilesystem) PutMarker(_ context.Context, marker optimizedartifact.Marker) error {
	if marker.ID == "" {
		return errors.New("optimized publication marker identity is empty")
	}
	if err := os.MkdirAll(f.markerDirectory(), 0o700); err != nil {
		return err
	}
	if err := validateOptimizedManagedPath(f.root, f.markerDirectory(), false, true); err != nil {
		return err
	}
	body, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	target, temporary := f.markerPath(marker.ID), f.markerPath(marker.ID)+".tmp"
	if err := validateOptimizedManagedPath(f.root, target, true, false); err != nil {
		return err
	}
	if err := validateOptimizedManagedPath(f.root, temporary, true, false); err != nil {
		return err
	}
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return f.SyncDirectory(f.markerDirectory())
}
func (f optimizedLocalFilesystem) DeleteMarker(_ context.Context, id string) error {
	if err := f.Remove(f.markerPath(id)); err != nil {
		return err
	}
	return f.SyncDirectory(f.markerDirectory())
}

func (f optimizedLocalFilesystem) markers() ([]optimizedartifact.Marker, error) {
	if err := validateOptimizedManagedPath(f.root, f.markerDirectory(), true, true); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(f.markerDirectory())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	markers := make([]optimizedartifact.Marker, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		markerPath := filepath.Join(f.markerDirectory(), entry.Name())
		if err := validateOptimizedManagedPath(f.root, markerPath, false, false); err != nil {
			if f.server != nil {
				f.server.log.Warn("ignored untrusted optimized publication marker", "marker", safePathComponent(entry.Name()))
			}
			continue
		}
		body, readErr := os.ReadFile(markerPath)
		if readErr != nil {
			if f.server != nil {
				f.server.log.Warn("ignored unreadable optimized publication marker", "marker", safePathComponent(entry.Name()))
			}
			continue
		}
		var marker optimizedartifact.Marker
		if json.Unmarshal(body, &marker) != nil || marker.ID == "" || entry.Name() != safePathComponent(marker.ID)+".json" {
			quarantine := strings.TrimSuffix(markerPath, ".json") + ".invalid"
			if validateOptimizedManagedPath(f.root, quarantine, true, false) == nil {
				_ = os.Rename(markerPath, quarantine)
				_ = f.SyncDirectory(f.markerDirectory())
			}
			if f.server != nil {
				f.server.log.Warn("quarantined invalid optimized publication marker", "marker", safePathComponent(entry.Name()))
			}
			continue
		}
		markers = append(markers, marker)
	}
	return markers, nil
}

// validateOptimizedManagedPath rejects lexical escapes and every existing
// symlink/reparse-like component before optimized lifecycle code touches a
// path. Platform-specific publication primitives still remain the final
// authority; this check also protects custom external roots and reconciliation
// inputs from marker-controlled paths.
func validateOptimizedManagedPath(root, candidate string, allowMissingLeaf, wantDirectory bool) error {
	root, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil || strings.TrimSpace(root) == "" {
		return errors.New("optimized storage root is invalid")
	}
	candidate, err = filepath.Abs(filepath.Clean(strings.TrimSpace(candidate)))
	if err != nil || !pathInsideRoot(candidate, root) && candidate != root {
		return errors.New("optimized path escaped configured storage")
	}
	rootInfo, rootErr := os.Lstat(root)
	if os.IsNotExist(rootErr) && allowMissingLeaf {
		return nil
	}
	if rootErr != nil {
		return rootErr
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("optimized storage root is not a trusted directory")
	}
	if candidate == root {
		if !wantDirectory {
			return errors.New("optimized file path resolved to its storage root")
		}
		return nil
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("optimized path escaped configured storage")
	}
	current := root
	parts := strings.FieldsFunc(relative, func(r rune) bool { return r == '/' || r == '\\' })
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) && allowMissingLeaf {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("optimized path contains a symlink or reparse point")
		}
		last := index == len(parts)-1
		if !last && !info.IsDir() {
			return errors.New("optimized path ancestor is not a directory")
		}
		if last {
			if wantDirectory && !info.IsDir() {
				return errors.New("optimized managed path is not a directory")
			}
			if !wantDirectory && !info.Mode().IsRegular() {
				return errors.New("optimized managed path is not a regular file")
			}
		}
	}
	return nil
}

func (s *Server) reconcileOptimizedV2Publications(ctx context.Context) error {
	fs := optimizedLocalFilesystem{root: s.optimizedVersionStorageDir(), server: s}
	if err := s.reconcileOptimizedV2Deletions(ctx); err != nil {
		return err
	}
	markers, err := fs.markers()
	if err != nil {
		return err
	}
	for _, marker := range markers {
		if _, err := s.reconcileOptimizedV2Marker(ctx, fs, marker); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) reconcileOptimizedV2Deletions(ctx context.Context) error {
	rows, err := s.queryBackgroundRead(ctx, `SELECT id, path FROM optimized_versions WHERE state = 'deleting' ORDER BY updated_at ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type deletion struct{ id, path string }
	var pending []deletion
	for rows.Next() {
		var item deletion
		if err := rows.Scan(&item.id, &item.path); err != nil {
			return err
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range pending {
		clean := filepath.Clean(item.path)
		root := ""
		for _, candidate := range s.optimizedVersionStorageRoots() {
			if pathInsideRoot(clean, candidate) {
				root = candidate
				break
			}
		}
		if root != "" && validateOptimizedManagedPath(root, clean, true, false) == nil {
			if err := (optimizedLocalFilesystem{root: root, server: s}).Remove(clean); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if _, err := s.execBackgroundWriteTagged(ctx, []string{"optimized_versions"}, `DELETE FROM optimized_versions WHERE id = ? AND state = 'deleting'`, item.id); err != nil {
			return err
		}
	}
	return nil
}

// newOptimizedV2Publication selects one of the registry's exactly eight stable
// presets and seals the source, preset, route, and graph identity before any
// encoder process is allowed to start.
func newOptimizedV2Publication(server *Server, root, mediaID, presetID string, route optimized.EncoderRoute, source optimizedV2SourceIdentity) (*optimizedV2Publication, error) {
	if server == nil || strings.TrimSpace(root) == "" || strings.TrimSpace(mediaID) == "" ||
		strings.TrimSpace(source.Revision) == "" || strings.TrimSpace(source.Fingerprint) == "" ||
		strings.TrimSpace(source.FactsDigest) == "" {
		return nil, errors.New("optimized publication identity is incomplete")
	}
	if err := optimized.ValidateRegistry(optimized.List()); err != nil {
		return nil, fmt.Errorf("optimized preset registry: %w", err)
	}
	preset, ok := optimized.Lookup(strings.TrimSpace(presetID))
	if !ok {
		return nil, fmt.Errorf("unknown optimized preset %q", presetID)
	}
	plan, err := optimized.PlanForRoute(preset, source.Facts, route)
	if err != nil {
		return nil, fmt.Errorf("plan optimized output: %w", err)
	}
	if plan.HDRAction == optimized.HDRToneMapSDR {
		settings := server.transcodeSettings()
		if !settings.HDRToneMapping {
			return nil, errors.New("HDR tone mapping is disabled by the server owner")
		}
		plan.ToneMapAlgorithm = safeToneMappingAlgorithm(settings.HDRToneMappingAlgorithm)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("encode optimized plan: %w", err)
	}
	planDigest := optimizedV2Digest(planJSON)
	presetVersion := optimizedV2PresetVersion(preset)
	identity, err := optimizedartifact.DeriveIdentity(optimizedartifact.IdentityInput{
		Root: root, MediaID: mediaID, PresetVersion: presetVersion,
		SourceFingerprint: source.Fingerprint, PlanDigest: planDigest,
		Extension: preset.Artifact.Extension,
	})
	if err != nil {
		return nil, err
	}
	tags, err := json.Marshal(plan.CompatibilityTags)
	if err != nil {
		return nil, fmt.Errorf("encode optimized compatibility tags: %w", err)
	}
	return &optimizedV2Publication{server: server, root: root, mediaID: mediaID, preset: preset,
		plan: plan, planJSON: planJSON, planDigest: planDigest, source: source,
		identity: identity, compatibilityJSON: tags, now: func() time.Time { return time.Now().UTC() }}, nil
}

func optimizedV2PresetVersion(p optimized.Preset) string {
	return p.ID + ":v" + strconv.Itoa(p.Version)
}

func optimizedV2Digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Publish owns the persisted state transitions and delegates filesystem
// durability to optimizedartifact. Validate must re-probe the staged artifact;
// its facts become immutable before the final rename.
func (p *optimizedV2Publication) Publish(ctx context.Context, fs optimizedartifact.Filesystem, predictedBytes int64,
	produce func(context.Context, io.Writer) error,
	validate func(context.Context, string) (optimizedV2OutputFacts, error),
) (optimizedartifact.Result, error) {
	if p == nil || p.server == nil || fs == nil || produce == nil || validate == nil || predictedBytes < 0 {
		return optimizedartifact.Result{}, errors.New("invalid optimized publication request")
	}
	if err := p.begin(ctx); err != nil {
		return optimizedartifact.Result{}, err
	}
	request := optimizedartifact.Request{
		Identity: optimizedartifact.IdentityInput{Root: p.root, MediaID: p.mediaID,
			PresetVersion: optimizedV2PresetVersion(p.preset), SourceFingerprint: p.source.Fingerprint,
			PlanDigest: p.planDigest, Extension: p.preset.Artifact.Extension},
		PredictedBytes: predictedBytes,
		Produce:        produce,
		Validate: func(ctx context.Context, path string) (int64, error) {
			if err := p.markValidating(ctx); err != nil {
				return 0, err
			}
			facts, err := validate(ctx, path)
			if err != nil {
				return 0, err
			}
			if err := validateOptimizedV2OutputFacts(facts); err != nil {
				return 0, err
			}
			p.output = facts
			if err := p.persistValidatedFacts(ctx); err != nil {
				return 0, err
			}
			return facts.SizeBytes, nil
		},
		Now: p.now,
	}
	result, err := (optimizedartifact.Publisher{FS: fs, Store: optimizedV2Store{publication: p}}).Publish(ctx, request)
	if err != nil {
		_ = p.markFailed(context.WithoutCancel(ctx))
		return optimizedartifact.Result{}, err
	}
	return result, nil
}

func validateOptimizedV2OutputFacts(f optimizedV2OutputFacts) error {
	if f.SizeBytes <= 0 || strings.TrimSpace(f.Container) == "" ||
		(strings.TrimSpace(f.VideoCodec) == "" && strings.TrimSpace(f.AudioCodec) == "") ||
		!validOfflineArtifact(f.ArtifactSHA256, f.SizeBytes) ||
		strings.TrimSpace(f.FactsDigest) == "" || len(f.FactsJSON) == 0 || !json.Valid(f.FactsJSON) {
		return errors.New("optimized output facts are incomplete")
	}
	var object map[string]any
	if json.Unmarshal(f.FactsJSON, &object) != nil || object == nil {
		return errors.New("optimized output facts must be a JSON object")
	}
	if f.FactsDigest != optimizedV2Digest(f.FactsJSON) {
		return errors.New("optimized output facts digest does not match its payload")
	}
	return nil
}

func (p *optimizedV2Publication) begin(ctx context.Context) error {
	now := p.now().UTC().Format(time.RFC3339Nano)
	_, err := p.server.execBackgroundWriteTagged(ctx, []string{"optimized_versions"}, `
		INSERT INTO optimized_versions
			(id, media_id, profile, path, size_bytes, created_at, updated_at, state,
			 preset_version, planner_revision, source_revision, source_fingerprint, source_facts_digest,
			 plan_digest, plan_json, output_facts_digest, output_facts_json, compatibility_tags_json)
		VALUES (?, ?, ?, '', 0, ?, ?, 'staging', ?, ?, ?, ?, ?, ?, ?, '', '{}', ?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at = excluded.updated_at,
			state = CASE WHEN optimized_versions.state = 'ready' THEN 'ready' ELSE 'staging' END`,
		p.identity.GenerationID, p.mediaID, p.preset.ID, now, now, p.preset.Version,
		optimized.PlannerRevision, p.source.Revision, p.source.Fingerprint, p.source.FactsDigest,
		p.planDigest, string(p.planJSON), string(p.compatibilityJSON))
	return err
}

func (p *optimizedV2Publication) markValidating(ctx context.Context) error {
	result, err := p.server.execBackgroundWriteTagged(ctx, []string{"optimized_versions"}, `
		UPDATE optimized_versions SET state = 'validating', updated_at = ?
		WHERE id = ? AND state IN ('staging', 'failed')`, p.now().UTC().Format(time.RFC3339Nano), p.identity.GenerationID)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return errors.New("optimized publication is not in a valid staging state")
	}
	return nil
}

func (p *optimizedV2Publication) persistValidatedFacts(ctx context.Context) error {
	_, err := p.server.execBackgroundWriteTagged(ctx, []string{"optimized_versions"}, `
		UPDATE optimized_versions SET size_bytes = ?, artifact_sha256 = ?, container = ?, video_codec = ?, audio_codec = ?,
			width = ?, height = ?, bitrate = ?, duration_seconds = ?, output_facts_digest = ?,
			output_facts_json = ?, updated_at = ?
		WHERE id = ? AND state = 'validating'`, p.output.SizeBytes, p.output.ArtifactSHA256, p.output.Container,
		p.output.VideoCodec, p.output.AudioCodec, p.output.Width, p.output.Height, p.output.Bitrate,
		p.output.DurationSeconds, p.output.FactsDigest, string(p.output.FactsJSON),
		p.now().UTC().Format(time.RFC3339Nano), p.identity.GenerationID)
	return err
}

func (p *optimizedV2Publication) markFailed(ctx context.Context) error {
	_, err := p.server.execBackgroundWriteTagged(ctx, []string{"optimized_versions"}, `
		UPDATE optimized_versions SET state = 'failed', updated_at = ?
		WHERE id = ? AND state IN ('staging', 'validating')`, p.now().UTC().Format(time.RFC3339Nano), p.identity.GenerationID)
	return err
}

type optimizedV2Store struct{ publication *optimizedV2Publication }

// reconcileOptimizedV2Marker is the startup hook for every durable publication
// marker. It reconstructs the sealed identity from persisted data, never from
// current defaults, then lets optimizedartifact complete or clean the crash.
func (s *Server) reconcileOptimizedV2Marker(ctx context.Context, fs optimizedartifact.Filesystem, marker optimizedartifact.Marker) (optimizedartifact.ReconcileOutcome, error) {
	if s == nil || fs == nil || strings.TrimSpace(marker.Metadata.GenerationID) == "" {
		return optimizedartifact.ReconcileNoop, errors.New("invalid optimized reconciliation request")
	}
	var mediaID, presetID, plannerRevision, sourceRevision, sourceFingerprint, sourceFactsDigest string
	var planDigest, planJSON, compatibilityJSON, artifactSHA256, outputFactsDigest, outputFactsJSON string
	var presetVersion int
	var outputSize int64
	var outputDuration int
	err := s.queryBackgroundRow(ctx, `
		SELECT media_id, profile, preset_version, planner_revision, source_revision, source_fingerprint,
			source_facts_digest, plan_digest, plan_json, compatibility_tags_json,
			size_bytes, artifact_sha256, duration_seconds, output_facts_digest, output_facts_json
		FROM optimized_versions WHERE id = ? AND state IN ('staging', 'validating', 'failed', 'ready')`,
		marker.Metadata.GenerationID).Scan(&mediaID, &presetID, &presetVersion, &plannerRevision, &sourceRevision,
		&sourceFingerprint, &sourceFactsDigest, &planDigest, &planJSON, &compatibilityJSON,
		&outputSize, &artifactSHA256, &outputDuration, &outputFactsDigest, &outputFactsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		// A marker without database authority cannot authorize deletion of either
		// pathname it contains. Remove only the independently derived marker file.
		if deleteErr := fs.DeleteMarker(ctx, marker.ID); deleteErr != nil {
			return optimizedartifact.ReconcileNoop, deleteErr
		}
		return optimizedartifact.ReconcileCleaned, nil
	}
	if err != nil {
		return optimizedartifact.ReconcileNoop, err
	}
	preset, ok := optimized.Lookup(presetID)
	if !ok || preset.Version != presetVersion || plannerRevision != optimized.PlannerRevision ||
		optimizedV2Digest([]byte(planJSON)) != planDigest {
		return optimizedartifact.ReconcileNoop, errors.New("persisted optimized publication identity is invalid")
	}
	var plan optimized.OutputPlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil || plan.PresetID != preset.ID || plan.PresetVersion != preset.Version {
		return optimizedartifact.ReconcileNoop, errors.New("persisted optimized plan is invalid")
	}
	derived, err := optimizedartifact.DeriveIdentity(optimizedartifact.IdentityInput{
		Root: s.optimizedVersionStorageDir(), MediaID: mediaID, PresetVersion: optimizedV2PresetVersion(preset),
		SourceFingerprint: sourceFingerprint, PlanDigest: planDigest, Extension: preset.Artifact.Extension,
	})
	if err != nil || marker.ID != derived.MarkerID || marker.Metadata.GenerationID != derived.GenerationID ||
		marker.Metadata.MediaID != mediaID || marker.Metadata.PresetVersion != optimizedV2PresetVersion(preset) ||
		marker.Metadata.SourceFingerprint != sourceFingerprint || marker.Metadata.PlanDigest != planDigest ||
		marker.Metadata.Path != derived.FinalPath || marker.TempPath != derived.TempPath || marker.FinalPath != derived.FinalPath {
		return optimizedartifact.ReconcileNoop, errors.New("optimized publication marker does not match its canonical identity")
	}
	p := &optimizedV2Publication{server: s, root: s.optimizedVersionStorageDir(), mediaID: mediaID, preset: preset, plan: plan,
		planJSON: []byte(planJSON), planDigest: planDigest,
		source:            optimizedV2SourceIdentity{Revision: sourceRevision, Fingerprint: sourceFingerprint, FactsDigest: sourceFactsDigest},
		identity:          derived,
		compatibilityJSON: []byte(compatibilityJSON), now: func() time.Time { return time.Now().UTC() }}
	if marker.Stage == optimizedartifact.StageDirSynced || marker.Stage == optimizedartifact.StageCommitted {
		exists, existsErr := fs.Exists(derived.FinalPath)
		if existsErr != nil {
			return optimizedartifact.ReconcileNoop, existsErr
		}
		if exists {
			probe, probeErr := s.validateOptimizedOutput(ctx, derived.FinalPath, outputDuration)
			facts, factsErr := optimizedOutputFactsFromProbe(derived.FinalPath, probe)
			if probeErr != nil || factsErr != nil || !validOfflineArtifact(artifactSHA256, outputSize) || validateOptimizedOutputAgainstPlan(facts, plan) != nil ||
				facts.SizeBytes != outputSize || facts.FactsDigest != outputFactsDigest || string(facts.FactsJSON) != outputFactsJSON {
				_ = fs.Remove(derived.FinalPath)
				_ = fs.SyncDirectory(derived.Directory)
				_ = fs.DeleteMarker(ctx, marker.ID)
				_ = p.markFailed(context.WithoutCancel(ctx))
				return optimizedartifact.ReconcileCleaned, nil
			}
			marker.Metadata.SizeBytes = facts.SizeBytes
		}
	}
	return (optimizedartifact.Publisher{FS: fs, Store: optimizedV2Store{publication: p}}).Reconcile(ctx, marker)
}

type optimizedV2MissingStore struct{}

func (optimizedV2MissingStore) Publish(context.Context, optimizedartifact.Metadata) (*optimizedartifact.Metadata, error) {
	return nil, errors.New("optimized publication record is missing")
}
func (optimizedV2MissingStore) Current(context.Context, string, string) (*optimizedartifact.Metadata, error) {
	return nil, nil
}

func (s optimizedV2Store) Publish(ctx context.Context, metadata optimizedartifact.Metadata) (*optimizedartifact.Metadata, error) {
	p := s.publication
	if p == nil || metadata.GenerationID != p.identity.GenerationID || metadata.MediaID != p.mediaID ||
		metadata.PresetVersion != optimizedV2PresetVersion(p.preset) || metadata.SourceFingerprint != p.source.Fingerprint ||
		metadata.PlanDigest != p.planDigest || metadata.Path != p.identity.FinalPath {
		return nil, errors.New("optimized artifact identity does not match sealed publication")
	}
	var previous *optimizedartifact.Metadata
	err := p.server.withBackgroundTxTagged(ctx, []string{"optimized_versions"}, func(tx *sql.Tx) error {
		var old optimizedartifact.Metadata
		var oldPublished string
		err := tx.QueryRowContext(ctx, `
			SELECT id, media_id, profile || ':v' || preset_version, source_fingerprint, plan_digest, path, size_bytes, updated_at
			FROM optimized_versions WHERE media_id = ? AND profile = ? AND state = 'ready' AND id <> ?
			ORDER BY updated_at DESC LIMIT 1`, p.mediaID, p.preset.ID, p.identity.GenerationID).
			Scan(&old.GenerationID, &old.MediaID, &old.PresetVersion, &old.SourceFingerprint,
				&old.PlanDigest, &old.Path, &old.SizeBytes, &oldPublished)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			old.PublishedAt, _ = time.Parse(time.RFC3339Nano, oldPublished)
			previous = &old
		}
		now := metadata.PublishedAt.UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `
			UPDATE optimized_versions SET state = 'superseded', superseded_at = ?, updated_at = ?
			WHERE media_id = ? AND profile = ? AND state = 'ready' AND id <> ?`,
			now, now, p.mediaID, p.preset.ID, p.identity.GenerationID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE optimized_versions SET state = 'ready', path = ?, size_bytes = ?, updated_at = ?, superseded_at = ''
			WHERE id = ? AND media_id = ? AND profile = ? AND state IN ('validating', 'failed')
			  AND preset_version = ? AND planner_revision = ? AND source_revision = ?
			  AND source_fingerprint = ? AND source_facts_digest = ? AND plan_digest = ?
			  AND plan_json = ? AND output_facts_digest <> ''`,
			metadata.Path, metadata.SizeBytes, now, p.identity.GenerationID, p.mediaID, p.preset.ID,
			p.preset.Version, optimized.PlannerRevision, p.source.Revision, p.source.Fingerprint,
			p.source.FactsDigest, p.planDigest, string(p.planJSON))
		if err != nil {
			return err
		}
		if rowsAffected(result) != 1 {
			return errors.New("optimized publication state changed before commit")
		}
		return nil
	})
	return previous, err
}

func (s optimizedV2Store) Current(ctx context.Context, mediaID, presetVersion string) (*optimizedartifact.Metadata, error) {
	p := s.publication
	if p == nil || mediaID != p.mediaID || presetVersion != optimizedV2PresetVersion(p.preset) {
		return nil, nil
	}
	var m optimizedartifact.Metadata
	var published string
	err := p.server.queryBackgroundRow(ctx, `
		SELECT id, media_id, profile || ':v' || preset_version, source_fingerprint, plan_digest,
			path, size_bytes, updated_at
		FROM optimized_versions WHERE id = ? AND state = 'ready'`, p.identity.GenerationID).
		Scan(&m.GenerationID, &m.MediaID, &m.PresetVersion, &m.SourceFingerprint,
			&m.PlanDigest, &m.Path, &m.SizeBytes, &published)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.PublishedAt, _ = time.Parse(time.RFC3339Nano, published)
	return &m, nil
}

type optimizedV2ReadyArtifact struct {
	ID, MediaID, PresetID, Path, SourceFingerprint, SourceFactsDigest, PlanDigest string
	ArtifactSHA256                                                                string
	PresetVersion                                                                 int
	SizeBytes                                                                     int64
	CompatibilityTags                                                             []string
	OutputFactsDigest                                                             string
	OutputFactsJSON                                                               json.RawMessage
	Container, VideoCodec, AudioCodec                                             string
	Width, Height, Bitrate, DurationSeconds                                       int
}

// optimizedV2ReadyForSource returns only a ready artifact that still matches
// the current source and planner. The path validator is the integration point
// for the W3 no-follow/trusted-root and availability policy.
func (s *Server) optimizedV2ReadyForSource(ctx context.Context, mediaID, presetID string,
	source optimizedV2SourceIdentity, pathUsable func(string, int64) bool) (*optimizedV2ReadyArtifact, error) {
	preset, ok := optimized.Lookup(strings.TrimSpace(presetID))
	if !ok || strings.TrimSpace(source.Revision) == "" || strings.TrimSpace(source.Fingerprint) == "" || strings.TrimSpace(source.FactsDigest) == "" {
		return nil, nil
	}
	var record optimizedV2ReadyArtifact
	var tagsJSON, outputFactsJSON string
	var size int64
	err := s.queryUserRow(ctx, `
		SELECT id, media_id, profile, path, source_fingerprint, source_facts_digest, plan_digest,
			preset_version, size_bytes, artifact_sha256, compatibility_tags_json, output_facts_digest, output_facts_json,
			container, video_codec, audio_codec, width, height, bitrate, duration_seconds
		FROM optimized_versions
		WHERE media_id = ? AND profile = ? AND state = 'ready' AND preset_version = ?
		  AND planner_revision = ? AND source_revision = ? AND source_fingerprint = ?
		  AND source_facts_digest = ? AND plan_digest <> '' AND plan_json <> '{}'
		  AND artifact_sha256 <> '' AND output_facts_digest <> ''
		ORDER BY updated_at DESC LIMIT 1`, mediaID, preset.ID, preset.Version, optimized.PlannerRevision,
		source.Revision, source.Fingerprint, source.FactsDigest).
		Scan(&record.ID, &record.MediaID, &record.PresetID, &record.Path, &record.SourceFingerprint,
			&record.SourceFactsDigest, &record.PlanDigest, &record.PresetVersion, &size, &record.ArtifactSHA256, &tagsJSON,
			&record.OutputFactsDigest, &outputFactsJSON, &record.Container, &record.VideoCodec,
			&record.AudioCodec, &record.Width, &record.Height, &record.Bitrate, &record.DurationSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pathUsable == nil || !pathUsable(record.Path, size) || !validOfflineArtifact(record.ArtifactSHA256, size) {
		return nil, nil
	}
	record.SizeBytes = size
	if err := json.Unmarshal([]byte(tagsJSON), &record.CompatibilityTags); err != nil {
		return nil, nil
	}
	record.OutputFactsJSON = json.RawMessage(outputFactsJSON)
	if !json.Valid(record.OutputFactsJSON) || optimizedV2Digest(record.OutputFactsJSON) != record.OutputFactsDigest ||
		strings.TrimSpace(record.Container) == "" || strings.TrimSpace(record.VideoCodec) == "" ||
		record.Width <= 0 || record.Height <= 0 || record.DurationSeconds <= 0 {
		return nil, nil
	}
	return &record, nil
}
