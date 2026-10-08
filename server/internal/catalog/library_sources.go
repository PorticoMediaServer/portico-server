package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrSourceBusy = errors.New("source has active playback or scan work")
var ErrSourceChanged = errors.New("source root changed; explicitly accept the replacement root before scanning")

// Source settings are owner-only. Viewer projections never include these paths.
type LibrarySource struct {
	IdentityConfirmed   bool   `json:"identityConfirmed"`
	ID                  string `json:"id"`
	LibraryID           string `json:"libraryId"`
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	Path                string `json:"path"`
	ResolvedPath        string `json:"resolvedPath"`
	Classification      string `json:"classification"`
	Generation          int64  `json:"generation"`
	Health              string `json:"health"`
	Enabled             bool   `json:"enabled"`
	FollowSymlinks      bool   `json:"followSymlinks"`
	MissingGraceSeconds int64  `json:"missingGraceSeconds"`
	IntervalSeconds     int64  `json:"intervalSeconds"`
	NextScanAt          string `json:"nextScanAt"`
	LastCompleteAt      string `json:"lastCompleteAt"`
	LastProgressAt      string `json:"lastProgressAt"`
	RootIdentity        string `json:"-"`
	Incarnation         string `json:"-"`
}
type SourceSettings struct {
	Name                string `json:"name"`
	Path                string `json:"path"`
	Classification      string `json:"classification"`
	FollowSymlinks      bool   `json:"followSymlinks"`
	MissingGraceSeconds int64  `json:"missingGraceSeconds"`
	IntervalSeconds     int64  `json:"intervalSeconds"`
	ExpectedRevision    int64  `json:"expectedRevision"`
	AcceptReplacement   bool   `json:"acceptReplacement"`
}
type ScanPolicy struct {
	Tier       string            `json:"tier"`
	Revision   int64             `json:"revision"`
	Operations []string          `json:"operations"`
	Trickplay  TrickplaySettings `json:"trickplay"`
}

// TrickplaySettings is owner-configured production geometry. Zero means the
// documented default; the stored values are clamped once here so no producer,
// delivery route or client re-derives a different bound.
type TrickplaySettings struct {
	IntervalSeconds int `json:"intervalSeconds"`
	TileWidth       int `json:"tileWidth"`
	MaxTiles        int `json:"maxTiles"`
}

const (
	DefaultTrickplayIntervalSeconds = 10
	DefaultTrickplayTileWidth       = 320
	DefaultTrickplayMaxTiles        = 3600
)

// Normalized returns the effective production geometry. Callers never see a
// zero, a negative, or an unbounded frame budget.
func (t TrickplaySettings) Normalized() TrickplaySettings {
	if t.IntervalSeconds == 0 {
		t.IntervalSeconds = DefaultTrickplayIntervalSeconds
	}
	if t.TileWidth == 0 {
		t.TileWidth = DefaultTrickplayTileWidth
	}
	if t.MaxTiles == 0 {
		t.MaxTiles = DefaultTrickplayMaxTiles
	}
	t.IntervalSeconds = min(600, max(1, t.IntervalSeconds))
	t.TileWidth = min(640, max(64, t.TileWidth/2*2))
	t.MaxTiles = min(20000, max(16, t.MaxTiles))
	return t
}

// A submitted value is accepted only inside the published bounds. Silently
// clamping an owner's choice would publish geometry they never selected.
func validTrickplay(t TrickplaySettings) bool {
	if t.IntervalSeconds != 0 && (t.IntervalSeconds < 1 || t.IntervalSeconds > 600) {
		return false
	}
	if t.TileWidth != 0 && (t.TileWidth < 64 || t.TileWidth > 640 || t.TileWidth%2 != 0) {
		return false
	}
	return t.MaxTiles == 0 || t.MaxTiles >= 16 && t.MaxTiles <= 20000
}

var BasicScanOperations = []string{"probe", "local_metadata", "subtitles"}
var DeepScanOperations = []string{"checksum", "chapter_images", "trickplay", "waveform", "loudness", "fingerprint", "segment_detection"}

func (p ScanPolicy) Allows(operation string) bool {
	if p.Tier == "file_list_only" {
		return false
	}
	for _, v := range p.Operations {
		if v == operation {
			return true
		}
	}
	return false
}
func ValidateScanPolicy(p ScanPolicy) (ScanPolicy, error) {
	if !validTrickplay(p.Trickplay) {
		return p, ErrAdminQuery
	}
	switch p.Tier {
	case "file_list_only":
		p.Operations = []string{}
	case "basic":
		p.Operations = append([]string{}, BasicScanOperations...)
	case "complete":
		p.Operations = append(append([]string{}, BasicScanOperations...), DeepScanOperations...)
	case "custom":
		seen := map[string]bool{}
		for _, v := range p.Operations {
			valid := false
			for _, allowed := range append(append([]string{}, BasicScanOperations...), DeepScanOperations...) {
				valid = valid || v == allowed
			}
			if !valid || seen[v] {
				return p, ErrAdminQuery
			}
			seen[v] = true
		}
		// Local extraction/subtitle timing requires a bounded probe. Do not silently
		// broaden an owner's Custom content-read permission to satisfy dependencies.
		if (seen["local_metadata"] || seen["subtitles"] || seen["chapter_images"] || seen["trickplay"] || seen["waveform"] || seen["loudness"] || seen["fingerprint"] || seen["segment_detection"]) && !seen["probe"] {
			return p, ErrAdminQuery
		}
		if p.Operations == nil {
			p.Operations = []string{}
		}
	default:
		return p, ErrAdminQuery
	}
	return p, nil
}

const scanPolicyColumns = `tier,revision,operations_json,trickplay_interval_seconds,trickplay_tile_width,trickplay_max_tiles`

func scanScanPolicy(row interface{ Scan(...any) error }) (ScanPolicy, error) {
	var p ScanPolicy
	var raw string
	err := row.Scan(&p.Tier, &p.Revision, &raw, &p.Trickplay.IntervalSeconds, &p.Trickplay.TileWidth, &p.Trickplay.MaxTiles)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &p.Operations)
	}
	return p, err
}
func (s *Service) ScanPolicy(ctx context.Context, library string) (ScanPolicy, error) {
	return scanScanPolicy(s.read().QueryRowContext(ctx, `SELECT `+scanPolicyColumns+` FROM library_scan_policies WHERE library_id=?`, library))
}
func ScanPolicyTx(ctx context.Context, tx *sql.Tx, library string) (ScanPolicy, error) {
	return scanScanPolicy(tx.QueryRowContext(ctx, `SELECT `+scanPolicyColumns+` FROM library_scan_policies WHERE library_id=?`, library))
}

const sourceColumns = `id,library_id,kind,name,configured_root,root,classification,generation,health,enabled,follow_symlinks,missing_grace_seconds,interval_seconds,next_scan_at,last_complete_at,last_progress_at,root_identity,incarnation,identity_confirmed`

func scanLibrarySource(row interface{ Scan(...any) error }) (LibrarySource, error) {
	var v LibrarySource
	err := row.Scan(&v.ID, &v.LibraryID, &v.Kind, &v.Name, &v.Path, &v.ResolvedPath, &v.Classification, &v.Generation, &v.Health, &v.Enabled, &v.FollowSymlinks, &v.MissingGraceSeconds, &v.IntervalSeconds, &v.NextScanAt, &v.LastCompleteAt, &v.LastProgressAt, &v.RootIdentity, &v.Incarnation, &v.IdentityConfirmed)
	return v, err
}
func (s *Service) LibrarySources(ctx context.Context, library string) ([]LibrarySource, error) {
	rows, err := s.read().QueryContext(ctx, `SELECT `+sourceColumns+` FROM library_sources WHERE library_id=? ORDER BY id LIMIT 65`, library)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibrarySource{}
	for rows.Next() {
		v, e := scanLibrarySource(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) LibrarySource(ctx context.Context, id string) (LibrarySource, error) {
	return scanLibrarySource(s.read().QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM library_sources WHERE id=?`, id))
}
func (s *Service) inspectSource(ctx context.Context, path string) (storage.Snapshot, error) {
	if !filepath.IsAbs(path) || !utf8.ValidString(path) || len(path) > 8192 || strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return storage.Snapshot{}, ErrAdminQuery
	}
	if s.storage != nil {
		return s.storage.InspectRoot(ctx, path)
	}
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return storage.Snapshot{}, err
	}
	return storage.LocalInventoryDirectory(storage.InventoryRequest{Root: root, RelativePath: "."})
}
func configurationFence(ctx context.Context, tx *sql.Tx, library string, expected int64, authorize func(*sql.Tx) error) error {
	if authorize == nil {
		return ErrAdminQuery
	}
	if err := authorize(tx); err != nil {
		return err
	}
	var managed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dvr_private_libraries WHERE library_id=?)`, library).Scan(&managed); err != nil {
		return err
	}
	if managed {
		return ErrAdminQuery
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM library_configuration WHERE library_id=?`, library).Scan(&revision); err != nil {
		return err
	}
	if revision != expected {
		return ErrLibraryConfigurationConflict
	}
	return nil
}
func (s *Service) SaveSource(ctx context.Context, library, id string, input SourceSettings, authorize func(*sql.Tx) error) (LibrarySource, error) {
	var result LibrarySource
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 || !utf8.ValidString(input.Name) || strings.IndexFunc(input.Name, unicode.IsControl) >= 0 || input.Classification != "local" && input.Classification != "network" || input.MissingGraceSeconds < 0 || input.MissingGraceSeconds > 90*86400 || input.Classification == "network" && input.MissingGraceSeconds < 3600 || input.IntervalSeconds != 0 && (input.IntervalSeconds < 900 || input.IntervalSeconds > 365*86400) {
		return result, ErrAdminQuery
	}
	if s.storage.IsRemote(input.Path) {
		input.Classification = "network"
		if input.FollowSymlinks || input.MissingGraceSeconds < 3600 {
			return result, ErrAdminQuery
		}
	}
	if s.storage != nil && s.storage.SourceGuard != nil && !s.storage.IsRemote(input.Path) {
		var previous string
		if id != "" {
			e := s.db.QueryRowContext(ctx, `SELECT configured_root FROM library_sources WHERE id=? AND library_id=?`, id, library).Scan(&previous)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return result, e
			}
		}
		if previous != input.Path {
			if e := s.storage.SourceGuard(input.Path); e != nil {
				return result, e
			}
		}
	}
	observed, err := s.inspectSource(ctx, input.Path)
	if err != nil {
		return result, errors.New("source unavailable or storage operation timed out")
	}
	rootID := storage.RootIdentity(observed)
	if rootID == "" {
		return result, ErrSourceChanged
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return result, err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if err = configurationFence(ctx, tx, library, input.ExpectedRevision, authorize); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,root FROM library_sources WHERE library_id=? AND enabled=1`, library)
	if err != nil {
		return result, err
	}
	count := 0
	for rows.Next() {
		var existing, path string
		if err = rows.Scan(&existing, &path); err != nil {
			rows.Close()
			return result, err
		}
		count++
		if existing != id {
			a, _ := filepath.Rel(path, observed.Path)
			b, _ := filepath.Rel(observed.Path, path)
			if filepath.IsLocal(a) || filepath.IsLocal(b) {
				rows.Close()
				return result, ErrSourceAlreadyConfigured
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	next := ""
	if input.IntervalSeconds > 0 {
		next = now.Add(time.Duration(input.IntervalSeconds) * time.Second).Format(time.RFC3339Nano)
	}
	if id == "" {
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM library_sources WHERE library_id=?`, library).Scan(&count); err != nil {
			return result, err
		}
		if count >= 64 {
			return result, ErrAdminQuery
		}
		id = identity.Token()
		_, err = tx.ExecContext(ctx, `INSERT INTO library_sources(id,library_id,name,configured_root,root,classification,root_identity,follow_symlinks,missing_grace_seconds,interval_seconds,next_scan_at,identity_confirmed) VALUES(?,?,?,?,?,?,?,?,?,?,?,1)`, id, library, input.Name, input.Path, observed.Path, input.Classification, rootID, input.FollowSymlinks, input.MissingGraceSeconds, input.IntervalSeconds, next)
	} else {
		old, e := scanLibrarySource(tx.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM library_sources WHERE id=? AND library_id=?`, id, library))
		if e != nil {
			return result, e
		}
		if old.Kind != "local" && old.Kind != "remote" {
			return result, ErrAdminQuery
		}
		var busy int
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_source_active WHERE source_id=?)`, id).Scan(&busy); e != nil {
			return result, e
		}
		if busy != 0 {
			return result, ErrSourceBusy
		}
		replacement := old.RootIdentity != "" && old.RootIdentity != rootID
		if replacement && !input.AcceptReplacement {
			return result, ErrSourceChanged
		}
		if old.ResolvedPath != observed.Path || replacement || old.FollowSymlinks != input.FollowSymlinks {
			if e = sourcePlaybackBusy(ctx, tx, id); e != nil {
				return result, e
			}
		}
		incarnation := old.Incarnation
		if replacement {
			incarnation = identity.Token()

		}
		_, err = tx.ExecContext(ctx, `UPDATE library_sources SET name=?,configured_root=?,root=?,classification=?,generation=generation+1,last_complete_job='',last_complete_at='',incarnation=?,root_identity=?,identity_confirmed=1,health='ready',enabled=1,follow_symlinks=?,missing_grace_seconds=?,interval_seconds=?,next_scan_at=? WHERE id=?`, input.Name, input.Path, observed.Path, input.Classification, incarnation, rootID, input.FollowSymlinks, input.MissingGraceSeconds, input.IntervalSeconds, next, id)

		// Keep the original primary root as a compatibility projection. Preparation
		// selects the exact source below; it never widens to a common ancestor.
		if err == nil && id == library {
			_, err = tx.ExecContext(ctx, `UPDATE libraries SET root=? WHERE id=?`, observed.Path, library)
		}
	}
	if err != nil {
		return result, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET kind=? WHERE id=?`, s.sourceKind(observed.Path), id); err != nil {
		return result, err
	}
	if err = gated.Commit(); err != nil {
		return result, err
	}
	return s.LibrarySource(ctx, id)
}
func sourcePlaybackBusy(ctx context.Context, tx *sql.Tx, source string) error {
	var busy int
	// Legacy established sessions plus contract-32 preparation/live occurrences.
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_sessions p JOIN catalog_entities i ON i.id=p.item_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE (EXISTS(SELECT 1 FROM inventory_objects o WHERE o.asset_id=p.asset_id AND o.source_id=?) OR cl.library_id=? AND NOT EXISTS(SELECT 1 FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id WHERE o.asset_id=p.asset_id AND s.library_id=cl.library_id)) AND p.state NOT IN('ended','stopped','failed') AND p.expires_at>?)`, source, source, time.Now().UTC().Format(time.RFC3339)).Scan(&busy)
	if err != nil {
		return err
	}
	if busy != 0 {
		return ErrSourceBusy
	}
	return nil
}
func (s *Service) SetSourceHealth(ctx context.Context, id, health string) error {
	gated2, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET health=? WHERE id=? AND health!=?`, health, id, health); err != nil {
		return err
	}

	return gated2.Commit()
}
func (s *Service) RemoveSource(ctx context.Context, library, id string, expected int64, authorize func(*sql.Tx) error) error {
	gated3, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	if err = configurationFence(ctx, tx, library, expected, authorize); err != nil {
		return err
	}
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM library_sources WHERE id=? AND library_id=?`, id, library).Scan(&exists); err != nil {
		return err
	}
	if err = sourcePlaybackBusy(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='cancelled' WHERE id IN(SELECT job_id FROM inventory_source_active WHERE source_id=?)`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM inventory_source_active WHERE source_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET enabled=0,health='removing',generation=generation+1,next_scan_at='' WHERE id=?`, id); err != nil {
		return err
	}

	return gated3.Commit()
}
func (s *Service) UpdateScanPolicy(ctx context.Context, library string, p ScanPolicy, expected int64, authorize func(*sql.Tx) error) (ScanPolicy, error) {
	p, err := ValidateScanPolicy(p)
	if err != nil {
		return p, err
	}
	raw, _ := json.Marshal(p.Operations)
	gated4, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return p, err
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	if err = configurationFence(ctx, tx, library, expected, authorize); err != nil {
		return p, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_scan_policies SET tier=?,operations_json=?,trickplay_interval_seconds=?,trickplay_tile_width=?,trickplay_max_tiles=?,revision=revision+1 WHERE library_id=?`, p.Tier, string(raw), p.Trickplay.IntervalSeconds, p.Trickplay.TileWidth, p.Trickplay.MaxTiles, library); err != nil {
		return p, err
	}
	// Existing inventory remains valid; its pending analysis is read-fenced by
	// the new policy revision. The scanner schedules missing current stages.
	if err = gated4.Commit(); err != nil {
		return p, err
	}
	return s.ScanPolicy(ctx, library)
}
