package remotesources

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"portico.local/apikit"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/storage"
)

// SourceAdvanced is the drawn Advanced page for one source: how often its
// folders are checked, whether it watches for changes, scans at start or asks
// for a full re-check, the disk the read cache may use, and which analysis
// steps run for files found through it. Every field is consumed by a real
// scanner, scheduler or analysis path; nothing is a stored-only toggle.
type SourceAdvanced struct {
	ScanIntervalSeconds int   `json:"scanIntervalSeconds"`
	WatchForChanges     bool  `json:"watchForChanges"`
	ScanOnStart         bool  `json:"scanOnStart"`
	FullRecheckDays     int   `json:"fullRecheckDays"`
	CacheBytes          int64 `json:"cacheBytes"`
	CacheFloorBytes     int64 `json:"cacheFloorBytes"`
	SeekPreviews        bool  `json:"seekPreviews"`
	ChapterImages       bool  `json:"chapterImages"`
	Waveforms           bool  `json:"waveforms"`
	Loudness            bool  `json:"loudness"`
	Checksums           bool  `json:"checksums"`
}

// AnalysisSwitches is the analysis subset the scan pipeline consults for one
// run. It is the only part of the page the analysis path needs.
type AnalysisSwitches struct {
	SeekPreviews  bool
	ChapterImages bool
	Waveforms     bool
	Loudness      bool
	Checksums     bool
}

type SourceAdvancedSettings struct {
	SourceID       string `json:"sourceId"`
	Generation     string `json:"generation"`
	Revision       int64  `json:"revision"`
	Saved          bool   `json:"saved"`
	CacheSupported bool   `json:"cacheSupported"`
	// CacheApplied is the durable projection: the mount's applied cache equals
	// what this page asks for. A saved value whose mount change failed reads false.
	CacheApplied bool `json:"cacheApplied"`
	// CacheError names why the mount did not take the saved cache, when it did not.
	CacheError string `json:"cacheError,omitempty"`
	// ScanIntervalObserved is the shortest interval the folders really have,
	// even when it is not one the page offers. It is reported, never relabelled.
	ScanIntervalObserved int            `json:"scanIntervalObserved"`
	Values               SourceAdvanced `json:"values"`
}

type SourceAdvancedChange struct {
	ExpectedGeneration string         `json:"expectedGeneration"`
	ExpectedRevision   int64          `json:"expectedRevision"`
	Values             SourceAdvanced `json:"values"`
}

var advancedFieldNames = []string{"scanIntervalSeconds", "watchForChanges", "scanOnStart", "fullRecheckDays", "cacheBytes", "cacheFloorBytes", "seekPreviews", "chapterImages", "waveforms", "loudness", "checksums"}

// A settings value is complete by construction: omitted, null or extra fields
// are refused, so a partial document can never silently switch analysis off.
func (v *SourceAdvanced) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(advancedFieldNames) {
		return storage.ErrRemoteConfig
	}
	for _, name := range advancedFieldNames {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return storage.ErrRemoteConfig
		}
	}
	type plain SourceAdvanced
	var decoded plain
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return storage.ErrRemoteConfig
	}
	*v = SourceAdvanced(decoded)
	return nil
}

func (c *SourceAdvancedChange) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		return storage.ErrRemoteConfig
	}
	for _, name := range []string{"expectedGeneration", "expectedRevision", "values"} {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return storage.ErrRemoteConfig
		}
	}
	type plain SourceAdvancedChange
	var decoded plain
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return storage.ErrRemoteConfig
	}
	*c = SourceAdvancedChange(decoded)
	return nil
}

// JSONSchema publishes the exact domains Release generates from: the offered
// intervals and re-check days as enums, and the cache sizes as bounded ranges.
func (SourceAdvanced) JSONSchema() map[string]any {
	type wire SourceAdvanced
	shape, _ := apikit.Schema(reflect.TypeFor[wire]())
	props, _ := shape["properties"].(map[string]any)
	if props == nil {
		return shape
	}
	if field, ok := props["scanIntervalSeconds"].(map[string]any); ok {
		field["enum"] = []int{0, 900, 3600, 21600, 86400, 604800}
	}
	if field, ok := props["fullRecheckDays"].(map[string]any); ok {
		field["enum"] = []int{0, 7, 14, 30}
	}
	if field, ok := props["cacheBytes"].(map[string]any); ok {
		field["description"] = "0 disables the read cache; otherwise 64 MiB through 64 GiB."
	}
	if field, ok := props["cacheFloorBytes"].(map[string]any); ok {
		field["description"] = "0 keeps the mount's current free-space floor; otherwise 1 GiB through 1 TiB."
	}
	return shape
}

// SourceScanTask is one due scan the scheduler should enqueue for a source's
// folder. Mode is the ingestion scan mode; Watch marks the five-minute pass.
type SourceScanTask struct {
	SourceID  string
	LibraryID string
	FolderID  string
	Mode      string
	Watch     bool
	Full      bool
}

var scanIntervals = map[int]bool{0: true, 900: true, 3600: true, 21600: true, 86400: true, 604800: true}
var fullRecheckDays = map[int]bool{0: true, 7: true, 14: true, 30: true}

func defaultAdvanced() SourceAdvanced {
	return SourceAdvanced{ScanIntervalSeconds: 0, FullRecheckDays: 0, SeekPreviews: true, ChapterImages: true, Waveforms: true, Loudness: true, Checksums: true}
}

func (v SourceAdvanced) valid() bool {
	if !scanIntervals[v.ScanIntervalSeconds] || !fullRecheckDays[v.FullRecheckDays] {
		return false
	}
	if v.CacheBytes != 0 && (v.CacheBytes < 64<<20 || v.CacheBytes > 64<<30) {
		return false
	}
	if v.CacheFloorBytes != 0 && (v.CacheFloorBytes < 1<<30 || v.CacheFloorBytes > 1<<40) {
		return false
	}
	return true
}

func (v SourceAdvanced) analysis() AnalysisSwitches {
	return AnalysisSwitches{SeekPreviews: v.SeekPreviews, ChapterImages: v.ChapterImages, Waveforms: v.Waveforms, Loudness: v.Loudness, Checksums: v.Checksums}
}

// readAdvancedTx reads the saved page, or the defaults a new source starts from.
// A source with folders reports the shortest interval its folders already have,
// so the page never claims "follow the library" when a real interval is set.
type advancedRow struct {
	values                 SourceAdvanced
	saved                  bool
	revision               int64
	appliedBytes           int64
	appliedFloor           int64
	observedIntervalSecond int
}

func readAdvancedTx(ctx context.Context, tx *sql.Tx, source string) (advancedRow, error) {
	var row advancedRow
	err := tx.QueryRowContext(ctx, `SELECT revision,scan_interval_seconds,watch_for_changes,scan_on_start,full_recheck_days,cache_bytes,cache_floor_bytes,cache_applied_bytes,cache_applied_floor_bytes,seek_previews,chapter_images,waveforms,loudness,checksums FROM remote_source_settings WHERE source_id=?`, source).
		Scan(&row.revision, &row.values.ScanIntervalSeconds, &row.values.WatchForChanges, &row.values.ScanOnStart, &row.values.FullRecheckDays, &row.values.CacheBytes, &row.values.CacheFloorBytes, &row.appliedBytes, &row.appliedFloor, &row.values.SeekPreviews, &row.values.ChapterImages, &row.values.Waveforms, &row.values.Loudness, &row.values.Checksums)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return advancedRow{}, err
	}
	row.saved = err == nil
	if !row.saved {
		row.values = defaultAdvanced()
	}
	// The folders' real interval is always reported as observed, whether or not
	// it is one this page offers; an unsupported one is never relabelled.
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(NULLIF(interval_seconds,0)),0) FROM library_sources WHERE remote_source_id=?`, source).Scan(&row.observedIntervalSecond); err != nil {
		return advancedRow{}, err
	}
	if !row.saved {
		if scanIntervals[row.observedIntervalSecond] {
			row.values.ScanIntervalSeconds = row.observedIntervalSecond
		}
	}
	return row, nil
}

// ReadSourceAdvanced is the owner's bounded read of one source's Advanced page.
func (s *Service) ReadSourceAdvanced(ctx context.Context, identities *identity.Service, owner identity.Principal, source string) (SourceAdvancedSettings, error) {
	if s == nil || s.db == nil || source == "" {
		return SourceAdvancedSettings{}, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	defer snapshot.Rollback()
	if err = currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner); err != nil {
		return SourceAdvancedSettings{}, err
	}
	row, err := sourceRowTx(ctx, snapshot.Tx(), source)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceAdvancedSettings{}, storage.ErrRemoteOffline
		}
		return SourceAdvancedSettings{}, err
	}
	settings, err := readAdvancedTx(ctx, snapshot.Tx(), source)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	return advancedSettings(row, settings, ""), nil
}

func advancedSettings(row Source, settings advancedRow, cacheError string) SourceAdvancedSettings {
	// Applied is read from the stored projection alone: asking to disable the
	// cache is only applied once the mount really reports no cache.
	applied := settings.appliedBytes == settings.values.CacheBytes &&
		(settings.values.CacheFloorBytes == 0 || settings.appliedFloor == settings.values.CacheFloorBytes)
	return SourceAdvancedSettings{
		SourceID:             row.ID,
		Generation:           strconv.FormatInt(row.Generation, 10),
		Revision:             settings.revision,
		Saved:                settings.saved,
		CacheSupported:       row.MountID != "",
		CacheApplied:         applied,
		CacheError:           cacheError,
		ScanIntervalObserved: settings.observedIntervalSecond,
		Values:               settings.values,
	}
}

// SaveSourceAdvanced stores one CAS write and applies it: the folders' scan
// interval and next check, and the mount's real read cache. The folder update
// is what the scanner already reads; no second scheduler is invented.
func (s *Service) SaveSourceAdvanced(ctx context.Context, identities *identity.Service, owner identity.Principal, source string, change SourceAdvancedChange) (SourceAdvancedSettings, error) {
	if s == nil || s.db == nil || source == "" || !change.Values.valid() {
		return SourceAdvancedSettings{}, storage.ErrRemoteConfig
	}
	s.commands.Lock()
	defer s.commands.Unlock()
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassInteractive)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if err = currentLocalOwnerTx(ctx, tx, identities, owner); err != nil {
		return SourceAdvancedSettings{}, err
	}
	row, err := sourceRowTx(ctx, tx, source)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceAdvancedSettings{}, storage.ErrRemoteOffline
		}
		return SourceAdvancedSettings{}, err
	}
	generation := strconv.FormatInt(row.Generation, 10)
	if change.ExpectedGeneration != generation {
		return SourceAdvancedSettings{}, storage.ErrRemoteChanged
	}
	current, err := readAdvancedTx(ctx, tx, source)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	revision := current.revision
	if change.ExpectedRevision != revision {
		return SourceAdvancedSettings{}, storage.ErrRemoteChanged
	}
	v := change.Values
	if v.CacheBytes != 0 || v.CacheFloorBytes != 0 {
		if row.MountID == "" {
			return SourceAdvancedSettings{}, storage.ErrRemoteConfig
		}
	}
	if revision == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO remote_source_settings(source_id,revision,scan_interval_seconds,watch_for_changes,scan_on_start,full_recheck_days,cache_bytes,cache_floor_bytes,seek_previews,chapter_images,waveforms,loudness,checksums) VALUES(?,1,?,?,?,?,?,?,?,?,?,?,?)`,
			source, v.ScanIntervalSeconds, v.WatchForChanges, v.ScanOnStart, v.FullRecheckDays, v.CacheBytes, v.CacheFloorBytes, v.SeekPreviews, v.ChapterImages, v.Waveforms, v.Loudness, v.Checksums)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE remote_source_settings SET revision=revision+1,scan_interval_seconds=?,watch_for_changes=?,scan_on_start=?,full_recheck_days=?,cache_bytes=?,cache_floor_bytes=?,seek_previews=?,chapter_images=?,waveforms=?,loudness=?,checksums=? WHERE source_id=? AND revision=?`,
			v.ScanIntervalSeconds, v.WatchForChanges, v.ScanOnStart, v.FullRecheckDays, v.CacheBytes, v.CacheFloorBytes, v.SeekPreviews, v.ChapterImages, v.Waveforms, v.Loudness, v.Checksums, source, revision)
	}
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	if v.ScanIntervalSeconds > 0 {
		// The drawn page owns these folders' check interval; the scanner reads
		// library_sources, so the same rows are updated here, once.
		next := time.Now().UTC().Add(time.Duration(v.ScanIntervalSeconds) * time.Second).Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET interval_seconds=?,next_scan_at=CASE WHEN next_scan_at='' OR next_scan_at>? THEN ? ELSE next_scan_at END WHERE remote_source_id=? AND interval_seconds IS NOT ?`, v.ScanIntervalSeconds, next, next, source, v.ScanIntervalSeconds); err != nil {
			return SourceAdvancedSettings{}, err
		}
	}
	if err = gated.Commit(); err != nil {
		return SourceAdvancedSettings{}, err
	}
	settings := advancedRow{values: v, saved: true, revision: revision + 1, appliedBytes: current.appliedBytes, appliedFloor: current.appliedFloor, observedIntervalSecond: current.observedIntervalSecond}
	cacheError := ""
	if row.MountID != "" {
		apply := cacheApply{admitted: row, revision: revision + 1, values: v}
		var applied bool
		applied, cacheError = s.applyCache(ctx, identities, owner, apply)
		if applied {
			settings.appliedBytes, settings.appliedFloor = v.CacheBytes, effectiveFloor(v.CacheFloorBytes, current.appliedFloor)
			// The projection belongs to the exact saved revision it describes.
			result, e := dbwork.ExecWrite(ctx, s.db, dbwork.ClassInteractive, `UPDATE remote_source_settings SET cache_applied_bytes=?,cache_applied_floor_bytes=? WHERE source_id=? AND revision=?`, settings.appliedBytes, settings.appliedFloor, source, revision+1)
			if e != nil {
				applied = false
				cacheError = cacheReasonCacheLost
			} else if n, _ := result.RowsAffected(); n != 1 {
				applied = false
				cacheError = cacheReasonChanged
			}
		}
	}
	if v.ScanIntervalSeconds > 0 {
		settings.observedIntervalSecond = v.ScanIntervalSeconds
	}
	return advancedSettings(row, settings, cacheError), nil
}

// cacheApply is the exact accepted settings a cache change belongs to.
type cacheApply struct {
	admitted Source
	revision int64
	values   SourceAdvanced
}

const (
	cacheReasonChanged     = "source_changed"
	cacheReasonUnavailable = "mount_unavailable"
	cacheReasonRefused     = "cache_refused"
	cacheReasonCacheLost   = "cache_record_failed"
)

// applyCache changes a mounted source's real read cache through the existing
// mount command. Its authorizer re-checks the captured owner, source
// generation, root, mount and saved settings revision in the mount writer's
// own transaction, so a change accepted after the settings moved on is refused
// rather than applied to the wrong source. Reasons are closed codes, never
// provider or database text.
func (s *Service) applyCache(ctx context.Context, identities *identity.Service, owner identity.Principal, accept cacheApply) (bool, string) {
	if s.mounts == nil || accept.admitted.MountID == "" {
		return false, cacheReasonUnavailable
	}
	list, err := s.mounts.List()
	if err != nil {
		return false, cacheReasonUnavailable
	}
	controlRevision := int64(-1)
	for _, mount := range list {
		if mount.ID == accept.admitted.MountID {
			controlRevision = mount.ControlRevision
			break
		}
	}
	if controlRevision < 1 {
		return false, cacheReasonUnavailable
	}
	floor := effectiveFloor(accept.values.CacheFloorBytes, 0)
	cacheBytes := accept.values.CacheBytes
	authorize := func(tx *sql.Tx) error {
		if err := currentLocalOwnerTx(ctx, tx, identities, owner); err != nil {
			return err
		}
		current, err := sourceRowTx(ctx, tx, accept.admitted.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return storage.ErrRemoteChanged
			}
			return err
		}
		if current.Generation != accept.admitted.Generation || current.RootPath != accept.admitted.RootPath || current.MountID != accept.admitted.MountID || current.Kind != accept.admitted.Kind {
			return storage.ErrRemoteChanged
		}
		var revision, savedBytes, savedFloor int64
		if err = tx.QueryRowContext(ctx, `SELECT revision,cache_bytes,cache_floor_bytes FROM remote_source_settings WHERE source_id=?`, accept.admitted.ID).Scan(&revision, &savedBytes, &savedFloor); err != nil {
			return storage.ErrRemoteChanged
		}
		if revision != accept.revision || savedBytes != accept.values.CacheBytes || savedFloor != accept.values.CacheFloorBytes {
			return storage.ErrRemoteChanged
		}
		return nil
	}
	if floor == 0 {
		// No floor requested means keep the mount's current floor, read under
		// the same association check before the command runs.
		for _, mount := range list {
			if mount.ID == accept.admitted.MountID {
				floor = effectiveFloor(accept.values.CacheFloorBytes, mount.CacheFloor)
			}
		}
	}
	_, err = s.mounts.Command(ctx, owner.Hash, accept.admitted.MountID, mounts.Command{
		OperationID:      scanOperationID(),
		Action:           "configure",
		ExpectedRevision: controlRevision,
		CacheBytes:       &cacheBytes,
		CacheFloor:       &floor,
	}, authorize)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrRemoteChanged), errors.Is(err, mounts.ErrCommandConflict), errors.Is(err, mounts.ErrCommandExpired):
			return false, cacheReasonChanged
		case errors.Is(err, mounts.ErrUnavailable):
			return false, cacheReasonUnavailable
		default:
			return false, cacheReasonRefused
		}
	}
	return true, ""
}

func effectiveFloor(requested, current int64) int64 {
	if requested <= 0 {
		return current
	}
	return requested
}

func scanOperationID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), identity.Token())
}

// DueSourceScans lists the scans a source's own page asks for now: the
// five-minute watch pass, the scan at server start, and a full re-check whose
// interval has passed. The caller enqueues them through the normal pipeline.
func (s *Service) DueSourceScans(ctx context.Context) ([]SourceScanTask, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	rows, err := s.db.QueryContext(ctx, `SELECT st.source_id,r.root,st.watch_for_changes,st.scan_on_start,st.full_recheck_days,st.last_full_recheck_ms,st.last_watch_ms FROM remote_source_settings st JOIN remote_sources r ON r.id=st.source_id WHERE r.removed=0`)
	if err != nil {
		return nil, err
	}
	type due struct {
		source, root        string
		watch, start        bool
		days                int
		lastFull, lastWatch int64
	}
	pending := []due{}
	for rows.Next() {
		var d due
		if err = rows.Scan(&d.source, &d.root, &d.watch, &d.start, &d.days, &d.lastFull, &d.lastWatch); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, d)
	}
	readErr := rows.Err()
	rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	tasks := []SourceScanTask{}
	for _, d := range pending {
		watchDue := d.watch && (d.lastWatch == 0 || now-d.lastWatch >= 5*60*1000)
		fullDue := d.days > 0 && (d.lastFull == 0 || now-d.lastFull >= int64(d.days)*24*60*60*1000)
		if !watchDue && !fullDue {
			continue
		}
		folders, err := folderSources(ctx, s.db, d.source)
		if err != nil {
			return nil, err
		}
		for _, folder := range folders {
			mode := "incremental"
			if fullDue {
				mode = "full"
			}
			tasks = append(tasks, SourceScanTask{SourceID: d.source, LibraryID: folder.library, FolderID: folder.id, Mode: mode, Watch: watchDue, Full: fullDue})
		}
	}
	return tasks, nil
}

type folderSource struct{ id, library string }

func folderSources(ctx context.Context, db *sql.DB, source string) ([]folderSource, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,library_id FROM library_sources WHERE enabled=1 AND remote_source_id=? ORDER BY id LIMIT 65`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []folderSource{}
	for rows.Next() {
		var f folderSource
		if err = rows.Scan(&f.id, &f.library); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// StartupSourceScans lists one scan per folder for sources that asked to be
// checked when the server starts. The runner calls this once per start.
func (s *Service) StartupSourceScans(ctx context.Context) ([]SourceScanTask, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT st.source_id,r.root FROM remote_source_settings st JOIN remote_sources r ON r.id=st.source_id WHERE r.removed=0 AND st.scan_on_start=1`)
	if err != nil {
		return nil, err
	}
	type started struct{ source, root string }
	all := []started{}
	for rows.Next() {
		var one started
		if err = rows.Scan(&one.source, &one.root); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, one)
	}
	readErr := rows.Err()
	rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	tasks := []SourceScanTask{}
	for _, one := range all {
		folders, err := folderSources(ctx, s.db, one.source)
		if err != nil {
			return nil, err
		}
		for _, folder := range folders {
			tasks = append(tasks, SourceScanTask{SourceID: one.source, LibraryID: folder.library, FolderID: folder.id, Mode: "incremental"})
		}
	}
	return tasks, nil
}

// MarkSourceScans records that the scheduler applied these scans, so a watch
// pass is not repeated within five minutes and a full re-check waits its days.
func (s *Service) MarkSourceScans(ctx context.Context, tasks []SourceScanTask) error {
	if s == nil || s.db == nil || len(tasks) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	seen := map[string]bool{}
	for _, task := range tasks {
		if seen[task.SourceID] {
			continue
		}
		seen[task.SourceID] = true
		if task.Full {
			if _, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE remote_source_settings SET last_full_recheck_ms=? WHERE source_id=?`, now, task.SourceID); err != nil {
				return err
			}
		}
		if task.Watch {
			if _, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE remote_source_settings SET last_watch_ms=? WHERE source_id=?`, now, task.SourceID); err != nil {
				return err
			}
		}
	}
	return nil
}

// AnalysisSwitchesTx reads the analysis switches for the remote source that
// holds a library source's root. `ok` is false for an ordinary local folder.
func AnalysisSwitchesTx(ctx context.Context, tx *sql.Tx, librarySource string) (AnalysisSwitches, bool, error) {
	var s AnalysisSwitches
	var scanInterval int
	err := tx.QueryRowContext(ctx, `SELECT st.scan_interval_seconds,st.seek_previews,st.chapter_images,st.waveforms,st.loudness,st.checksums FROM remote_source_settings st JOIN library_sources s ON s.remote_source_id=st.source_id WHERE s.id=? LIMIT 1`, librarySource).Scan(&scanInterval, &s.SeekPreviews, &s.ChapterImages, &s.Waveforms, &s.Loudness, &s.Checksums)
	if errors.Is(err, sql.ErrNoRows) {
		return AnalysisSwitches{}, false, nil
	}
	if err != nil {
		return AnalysisSwitches{}, false, err
	}
	return s, true, nil
}

// FilterAnalysisOperations removes the whole-file analysis stages this source
// switched off. Basic steps and playback probing are not whole-file analysis
// and are never removed here.
func (s AnalysisSwitches) Filter(operations []string) []string {
	kept := make([]string, 0, len(operations))
	for _, op := range operations {
		switch op {
		case "trickplay":
			if !s.SeekPreviews {
				continue
			}
		case "chapter_images":
			if !s.ChapterImages {
				continue
			}
		case "waveform":
			if !s.Waveforms {
				continue
			}
		case "loudness":
			if !s.Loudness {
				continue
			}
		case "checksum":
			if !s.Checksums {
				continue
			}
		}
		kept = append(kept, op)
	}
	return kept
}
