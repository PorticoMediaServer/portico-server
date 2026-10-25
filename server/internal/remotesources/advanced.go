package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

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
	SourceID       string         `json:"sourceId"`
	Generation     string         `json:"generation"`
	Revision       int64          `json:"revision"`
	Saved          bool           `json:"saved"`
	CacheSupported bool           `json:"cacheSupported"`
	Values         SourceAdvanced `json:"values"`
}

type SourceAdvancedChange struct {
	ExpectedGeneration string         `json:"expectedGeneration"`
	ExpectedRevision   int64          `json:"expectedRevision"`
	Values             SourceAdvanced `json:"values"`
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
func readAdvancedTx(ctx context.Context, tx *sql.Tx, source, root string) (SourceAdvanced, bool, error) {
	var values SourceAdvanced
	err := tx.QueryRowContext(ctx, `SELECT scan_interval_seconds,watch_for_changes,scan_on_start,full_recheck_days,cache_bytes,cache_floor_bytes,seek_previews,chapter_images,waveforms,loudness,checksums FROM remote_source_settings WHERE source_id=?`, source).
		Scan(&values.ScanIntervalSeconds, &values.WatchForChanges, &values.ScanOnStart, &values.FullRecheckDays, &values.CacheBytes, &values.CacheFloorBytes, &values.SeekPreviews, &values.ChapterImages, &values.Waveforms, &values.Loudness, &values.Checksums)
	if err == nil {
		return values, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SourceAdvanced{}, false, err
	}
	values = defaultAdvanced()
	var shortest int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(NULLIF(interval_seconds,0)),0) FROM library_sources WHERE configured_root=? OR substr(configured_root,1,length(?)+1)=?||'/'`, root, root, root).Scan(&shortest); err != nil {
		return SourceAdvanced{}, false, err
	}
	if !scanIntervals[shortest] {
		shortest = 21600
	}
	values.ScanIntervalSeconds = shortest
	return values, false, nil
}

func advancedRevisionTx(ctx context.Context, tx *sql.Tx, source string) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM remote_source_settings WHERE source_id=?`, source).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, err
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
	values, saved, err := readAdvancedTx(ctx, snapshot.Tx(), source, row.RootPath)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	revision, err := advancedRevisionTx(ctx, snapshot.Tx(), source)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
	return SourceAdvancedSettings{SourceID: source, Generation: strconv.FormatInt(row.Generation, 10), Revision: revision, Saved: saved, CacheSupported: row.MountID != "", Values: values}, nil
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
	revision, err := advancedRevisionTx(ctx, tx, source)
	if err != nil {
		return SourceAdvancedSettings{}, err
	}
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
		if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET interval_seconds=?,next_scan_at=CASE WHEN next_scan_at='' OR next_scan_at>? THEN ? ELSE next_scan_at END WHERE interval_seconds IS NOT ? AND (configured_root=? OR substr(configured_root,1,length(?)+1)=?||'/')`, v.ScanIntervalSeconds, next, next, v.ScanIntervalSeconds, row.RootPath, row.RootPath, row.RootPath); err != nil {
			return SourceAdvancedSettings{}, err
		}
	}
	if err = gated.Commit(); err != nil {
		return SourceAdvancedSettings{}, err
	}
	if row.MountID != "" {
		if err = s.applyCache(row.MountID, v.CacheBytes, v.CacheFloorBytes); err != nil {
			return SourceAdvancedSettings{}, err
		}
	}
	return SourceAdvancedSettings{SourceID: source, Generation: generation, Revision: revision + 1, Saved: true, CacheSupported: row.MountID != "", Values: v}, nil
}

// applyCache changes a mounted source's real read cache through the existing
// mount command: rclone restarts with the new size, and nothing pretends a
// cache exists for a source the server reads directly.
func (s *Service) applyCache(mountID string, cacheBytes, floorBytes int64) error {
	if s.mounts == nil || mountID == "" {
		return nil
	}
	list, err := s.mounts.List()
	if err != nil {
		return err
	}
	for _, mount := range list {
		if mount.ID != mountID {
			continue
		}
		currentCache, currentFloor := mount.CacheBytes, mount.CacheFloor
		if currentCache == cacheBytes && currentFloor == floorBytes {
			return nil
		}
		floor := floorBytes
		if floor == 0 {
			floor = currentFloor
		}
		_, err = s.mounts.Command(context.Background(), "source-advanced", mount.ID, mounts.Command{
			OperationID:      scanOperationID(),
			Action:           "configure",
			ExpectedRevision: mount.ControlRevision,
			CacheBytes:       &cacheBytes,
			CacheFloor:       &floor,
		}, nil)
		return err
	}
	return nil
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
		folders, err := folderSources(ctx, s.db, d.root)
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

func folderSources(ctx context.Context, db *sql.DB, root string) ([]folderSource, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,library_id FROM library_sources WHERE enabled=1 AND (configured_root=? OR substr(configured_root,1,length(?)+1)=?||'/') ORDER BY id`, root, root, root)
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
		folders, err := folderSources(ctx, s.db, one.root)
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
func AnalysisSwitchesTx(ctx context.Context, tx *sql.Tx, root string) (AnalysisSwitches, bool, error) {
	var s AnalysisSwitches
	var scanInterval int
	err := tx.QueryRowContext(ctx, `SELECT st.scan_interval_seconds,st.seek_previews,st.chapter_images,st.waveforms,st.loudness,st.checksums FROM remote_source_settings st JOIN remote_sources r ON r.id=st.source_id WHERE r.removed=0 AND (r.root=? OR substr(?,1,length(r.root)+1)=r.root||'/') LIMIT 1`, root, root).Scan(&scanInterval, &s.SeekPreviews, &s.ChapterImages, &s.Waveforms, &s.Loudness, &s.Checksums)
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
		case "checksum", "fingerprint":
			if !s.Checksums {
				continue
			}
		}
		kept = append(kept, op)
	}
	return kept
}
