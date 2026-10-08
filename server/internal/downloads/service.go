package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
	"strconv"
	"sync"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
)

// SourceOpener reads an original catalog file. The server never opens a library
// path from this package directly: production wires storage.Client.OpenPlayback,
// which reads through the isolated media process, and a test wires a plain file
// opener. The signature matches OpenPlayback exactly so no adapter is needed.
type SourceOpener func(ctx context.Context, path string, size, modifiedNS int64) (io.ReadSeekCloser, error)

// ArtifactOpener reads a published prepared version's bytes while holding a
// deletion lease on them. internal/preparedmedia implements it.
type ArtifactOpener interface {
	OpenDownloadArtifact(ctx context.Context, versionID, digest string, size int64) (io.ReadSeekCloser, error)
}

// Optimizer asks the prepared-media producer for a version this package needs.
// Conversions are owner-authorized by design, so this is only reached for an
// owner viewer; a non-owner's rung preparation waits for an owner-prepared
// version instead of escalating the request.
type Optimizer interface {
	RequestOptimization(ctx context.Context, p identity.Principal, itemID, assetID, profileID, operationID string) error
}

type Options struct {
	SourceSnapshotRoot string
	DB                 *sql.DB
	Now                func() time.Time
	OpenSource         SourceOpener
	Artifacts          ArtifactOpener
	Optimizer          Optimizer
	Notifier           DownloadNotifier
	// BaseGrantPath is the route prefix a grant URL is published under. It is a
	// path, never an absolute URL: the client already knows which server it is
	// talking to, and a server behind a proxy must not invent an origin.
	BaseGrantPath string
}

type Service struct {
	access       RequestAccess
	accessMu     sync.RWMutex
	snapshotRoot string
	snapshotMu   sync.Mutex
	advanceMu    sync.Mutex
	db           *sql.DB
	now          func() time.Time
	openSource   SourceOpener
	artifacts    ArtifactOpener
	optimizer    Optimizer
	notifier     DownloadNotifier
	grantPath    string
	wake         *worker.Signal
	done         chan struct{}
	cancel       context.CancelFunc
}

func New(o Options) (*Service, error) {
	if o.DB == nil {
		return nil, ErrInput
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Notifier == nil {
		o.Notifier = noopNotifier{}
	}
	if o.BaseGrantPath == "" {
		o.BaseGrantPath = "/v1/downloads/artifacts/"
	}
	if o.SourceSnapshotRoot == "" {
		var sequence int
		var name, path string
		if err := o.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
			return nil, err
		}
		if path == "" {
			return nil, errors.New("download source snapshot directory required")
		}
		o.SourceSnapshotRoot = filepath.Join(filepath.Dir(path), "download-source-snapshots")
	}
	if err := os.MkdirAll(o.SourceSnapshotRoot, 0700); err != nil {
		return nil, err
	}
	return &Service{snapshotRoot: o.SourceSnapshotRoot, db: o.DB, now: o.Now, openSource: o.OpenSource, artifacts: o.Artifacts, optimizer: o.Optimizer, notifier: o.Notifier, grantPath: o.BaseGrantPath, wake: worker.NewSignal()}, nil
}

func (s *Service) millis() int64 { return s.now().UnixMilli() }
func (s *Service) signal()       { s.wake.Wake() }

// Settings is the owner policy for downloads.
type Settings struct {
	MaxPreparedBytes int64 `json:"maxPreparedBytes"`
	RetentionDays    int   `json:"retentionDays"`
	Revision         int64 `json:"revision"`
}

// SettingsChange fences an owner write the same way every other console write
// is fenced.
type SettingsChange struct {
	OperationID      string `json:"operationId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	MaxPreparedBytes int64  `json:"maxPreparedBytes"`
	RetentionDays    int    `json:"retentionDays"`
}

func readSettings(tx *sql.Tx) (Settings, error) {
	var out Settings
	e := tx.QueryRow(`SELECT max_prepared_bytes,retention_days,revision FROM download_settings WHERE singleton=1`).Scan(&out.MaxPreparedBytes, &out.RetentionDays, &out.Revision)
	return out, e
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return Settings{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	out, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	return out, gated.Commit()
}

func (s *Service) ApplySettings(ctx context.Context, p identity.Principal, c SettingsChange) (Settings, error) {
	var out Settings
	if !validID.MatchString(c.OperationID) || c.MaxPreparedBytes < 0 || c.RetentionDays < 1 || c.RetentionDays > 365 {
		return out, ErrInput
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	now := s.millis()
	scope := "downloads-settings:" + operations.AccountKey(p)
	raw, digest, e := operations.Receipt(tx, scope, c.OperationID, c, now)
	if e != nil {
		return out, e
	}
	if raw != "" {
		return out, json.Unmarshal([]byte(raw), &out)
	}
	current, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	if current.Revision != c.ExpectedRevision {
		return current, ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `UPDATE download_settings SET max_prepared_bytes=?,retention_days=?,revision=revision+1 WHERE singleton=1`, c.MaxPreparedBytes, c.RetentionDays); e != nil {
		return out, e
	}
	if out, e = readSettings(tx); e != nil {
		return out, e
	}
	if e = operations.Audit(tx, now, operations.AccountKey(p), "downloads.settings", "downloads", out.Revision); e != nil {
		return out, e
	}
	if e = operations.SaveReceipt(tx, scope, c.OperationID, digest, out, now); e != nil {
		return out, e
	}
	if e = gated2.Commit(); e == nil {
		s.signal()
	}
	return out, e
}

// Usage is the storage a viewer and the whole server have committed to prepared
// downloads. It counts claims, not files: two viewers holding the same prepared
// version each account for its bytes, because either of them removing the claim
// must free nothing while the other still holds it.
type Usage struct {
	ProfileID        string      `json:"profileId"`
	ProfileBytes     int64       `json:"profileBytes"`
	ProfileCount     int         `json:"profileCount"`
	ServerBytes      int64       `json:"serverBytes"`
	ServerCount      int         `json:"serverCount"`
	DistinctBytes    int64       `json:"distinctArtifactBytes"`
	MaxPreparedBytes int64       `json:"maxPreparedBytes"`
	RemainingBytes   *int64      `json:"remainingBytes"`
	RetentionDays    int         `json:"retentionDays"`
	Profiles         []UsageLine `json:"profiles"`
}

// UsageLine is one profile's share of the server total. Only an owner reads the
// whole list; a viewer sees their own line.
type UsageLine struct {
	ProfileID string `json:"profileId"`
	Bytes     int64  `json:"bytes"`
	Count     int    `json:"count"`
}

// committedStates are the preparations the server has promised storage to. A
// queued claim counts against the ceiling from the moment it is admitted;
// admitting work the store cannot hold and discovering it later would leave a
// viewer watching a progress bar that can only end in storage_full.
const committedStates = `('queued','running','ready','paused')`

func (s *Service) Usage(ctx context.Context, p identity.Principal, all bool) (Usage, error) {
	out := Usage{ProfileID: p.ProfileID, Profiles: []UsageLine{}}
	gated3, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	out.MaxPreparedBytes, out.RetentionDays = settings.MaxPreparedBytes, settings.RetentionDays
	key := operations.ViewerKey(p)
	var bytes sql.NullInt64
	if e = tx.QueryRowContext(ctx, `SELECT sum(bytes_total),count(*) FROM download_preparations WHERE profile_key=? AND state IN `+committedStates, key).Scan(&bytes, &out.ProfileCount); e != nil {
		return out, e
	}
	out.ProfileBytes = bytes.Int64
	if e = tx.QueryRowContext(ctx, `SELECT sum(bytes_total),count(*) FROM download_preparations WHERE state IN `+committedStates).Scan(&bytes, &out.ServerCount); e != nil {
		return out, e
	}
	out.ServerBytes = bytes.Int64
	// Distinct artifact bytes is what the disk actually holds: two viewers
	// claiming one prepared version share its file.
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(bytes),0) FROM (SELECT max(bytes_total) AS bytes FROM download_preparations WHERE state='ready' AND artifact_digest!='' GROUP BY artifact_digest)`).Scan(&out.DistinctBytes); e != nil {
		return out, e
	}
	if settings.MaxPreparedBytes > 0 {
		remaining := settings.MaxPreparedBytes - out.ServerBytes
		if remaining < 0 {
			remaining = 0
		}
		out.RemainingBytes = &remaining
	}
	if all {
		rows, err := tx.QueryContext(ctx, `SELECT profile_id,COALESCE(sum(bytes_total),0),count(*) FROM download_preparations WHERE state IN `+committedStates+` GROUP BY profile_id ORDER BY profile_id LIMIT 256`)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var line UsageLine
			if err = rows.Scan(&line.ProfileID, &line.Bytes, &line.Count); err != nil {
				rows.Close()
				return out, err
			}
			out.Profiles = append(out.Profiles, line)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	} else {
		out.Profiles = append(out.Profiles, UsageLine{p.ProfileID, out.ProfileBytes, out.ProfileCount})
	}
	return out, gated3.Commit()
}

// storageRoom refuses admission once the owner's ceiling is reached. The check
// is inside the admitting transaction, so two concurrent batches cannot both
// squeeze past the same remaining allowance.
func storageRoom(tx *sql.Tx, want int64) error {
	settings, e := readSettings(tx)
	if e != nil {
		return e
	}
	if settings.MaxPreparedBytes <= 0 {
		return nil
	}
	var committed sql.NullInt64
	if e = tx.QueryRow(`SELECT sum(bytes_total) FROM download_preparations WHERE state IN ` + committedStates).Scan(&committed); e != nil {
		return e
	}
	if committed.Int64+want > settings.MaxPreparedBytes {
		return ErrStorageFull
	}
	return nil
}

const preparationColumns = `id,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_preparations.item_id),''),library_id,profile_id,quality,origin,batch_id,state,reason,artifact_kind,artifact_ref,artifact_digest,artifact_container,bytes_done,bytes_total,estimated,revision,created_ms,updated_ms,started_ms,ready_ms`

type row struct {
	Preparation
	StartedMS int64
	CreatedMS int64
}

func readPreparation(scan interface{ Scan(...any) error }) (row, error) {
	var r row
	var estimated int
	var updatedMS, readyMS int64
	e := scan.Scan(&r.ID, &r.ItemID, &r.LibraryID, &r.ProfileID, &r.Quality, &r.Origin, &r.BatchID, &r.State, &r.Reason,
		&r.Artifact.Kind, &r.Artifact.Ref, &r.Artifact.SHA256, &r.Artifact.Container,
		&r.Progress.BytesDone, &r.Progress.BytesTotal, &estimated, &r.Revision, &r.CreatedMS, &updatedMS, &r.StartedMS, &readyMS)
	if e != nil {
		return r, e
	}
	// An estimate is a moving lower bound once the worker has produced more.
	// Exact ready sizes remain authoritative.
	if estimated != 0 && r.State != StateReady && r.Progress.BytesDone > r.Progress.BytesTotal {
		r.Progress.BytesTotal = r.Progress.BytesDone
	}
	r.Artifact.Estimated = estimated != 0
	r.Artifact.Bytes = r.Progress.BytesTotal
	r.Artifact.ContentType, r.Artifact.FileName = artifactDelivery(r.Artifact.Container, r.ID)
	r.Progress.Estimated = estimated != 0
	r.CreatedAt, r.UpdatedAt, r.ReadyAt = stamp(r.CreatedMS), stamp(updatedMS), stamp(readyMS)
	r.Actions = actionsFor(r.State)
	if r.Progress.BytesTotal > 0 {
		percent := float64(r.Progress.BytesDone) / float64(r.Progress.BytesTotal) * 100
		if percent > 100 {
			percent = 100
		}
		r.Progress.Percent = percent
	}
	if r.State == StateReady {
		r.Progress.Percent = 100
		r.Progress.BytesDone = r.Progress.BytesTotal
	}
	return r, nil
}

// publish finishes a stored row into its wire view: the retention deadline of a
// ready claim and the ETA of a running one are both computed, never stored, so
// a settings change or a stalled worker cannot leave a stale promise behind.
func (s *Service) publish(r row, retentionDays int) Preparation {
	out := r.Preparation
	if r.State == StateReady && r.ReadyAt != "" {
		if ready, e := time.Parse(time.RFC3339, r.ReadyAt); e == nil {
			out.ExpiresAt = ready.Add(time.Duration(retentionDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
		}
	}
	if r.State == StateRunning && r.StartedMS > 0 && r.Progress.BytesDone > 0 && r.Progress.BytesTotal > r.Progress.BytesDone {
		elapsed := float64(s.millis()-r.StartedMS) / 1000
		if elapsed > 0 {
			rate := float64(r.Progress.BytesDone) / elapsed
			if rate > 0 {
				eta := int64(float64(r.Progress.BytesTotal-r.Progress.BytesDone) / rate)
				out.Progress.ETASeconds = &eta
			}
		}
	}
	return out
}

// Get reads one preparation belonging to this viewer.
func (s *Service) Get(ctx context.Context, p identity.Principal, id string) (Preparation, error) {
	var out Preparation
	if !validID.MatchString(id) {
		return out, ErrInput
	}
	gated4, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	r, e := readPreparation(tx.QueryRowContext(ctx, `SELECT `+preparationColumns+` FROM download_preparations WHERE id=? AND profile_key=?`, id, operations.ViewerKey(p)))
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	return s.publish(r, settings.RetentionDays), gated4.Commit()
}

// ListQuery is one page request. ProfileID is only honoured for a caller the
// routes layer has established as the server owner; a viewer naming another
// profile reads their own, because a download list is personal state.
type ListQuery struct {
	State     string
	ProfileID string
	Cursor    string
	Limit     int
}

// List pages preparations newest first, keyset-ordered on (createdMs, id) so a
// page is stable while new claims are admitted above it.
func (s *Service) List(ctx context.Context, p identity.Principal, q ListQuery, anyProfile bool) (Page, error) {
	out := Page{Items: []Preparation{}}
	if q.Limit < 1 || q.Limit > 200 {
		q.Limit = 50
	}
	switch q.State {
	case "", StateQueued, StateRunning, StateReady, StatePaused, StateFailed, StateUnavailable, StateCancelled, StateExpired:
	default:
		return out, ErrInput
	}
	if q.ProfileID != "" && !validID.MatchString(q.ProfileID) {
		return out, ErrInput
	}
	before := int64(0)
	after := ""
	if q.Cursor != "" {
		ms, id, e := decodeCursor(q.Cursor)
		if e != nil {
			return out, e
		}
		before, after = ms, id
	}
	gated5, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	query := `SELECT ` + preparationColumns + ` FROM download_preparations WHERE `
	args := []any{}
	if anyProfile && q.ProfileID != "" {
		query += `profile_id=?`
		args = append(args, q.ProfileID)
	} else {
		query += `profile_key=?`
		args = append(args, operations.ViewerKey(p))
	}
	if q.State != "" {
		query += ` AND state=?`
		args = append(args, q.State)
	}
	if q.Cursor != "" {
		query += ` AND (created_ms<? OR (created_ms=? AND id<?))`
		args = append(args, before, before, after)
	}
	query += ` ORDER BY created_ms DESC,id DESC LIMIT ?`
	args = append(args, q.Limit+1)
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return out, e
	}
	collected := []row{}
	for rows.Next() {
		r, err := readPreparation(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		collected = append(collected, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(collected) > q.Limit {
		last := collected[q.Limit-1]
		out.NextCursor = encodeCursor(last.CreatedMS, last.ID)
		collected = collected[:q.Limit]
	}
	for _, r := range collected {
		out.Items = append(out.Items, s.publish(r, settings.RetentionDays))
	}
	return out, gated5.Commit()
}

func encodeCursor(ms int64, id string) string {
	return strconv.FormatInt(ms, 10) + ":" + id
}
func decodeCursor(v string) (int64, string, error) {
	for i := 0; i < len(v); i++ {
		if v[i] == ':' {
			ms, e := strconv.ParseInt(v[:i], 10, 64)
			id := v[i+1:]
			if e != nil || !validID.MatchString(id) {
				return 0, "", ErrInput
			}
			return ms, id, nil
		}
	}
	return 0, "", ErrInput
}

// preparationQuality validates a requested quality against the published ladder
// rather than accepting any string that later fails deep in the worker.
func preparationQuality(v string) error {
	if v == QualityOriginal {
		return nil
	}
	if _, ok := playback.LadderPreset(v); ok {
		return nil
	}
	return ErrInput
}
