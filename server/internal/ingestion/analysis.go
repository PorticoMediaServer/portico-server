package ingestion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"portico.local/server/internal/supervise"
	"strconv"
	"strings"
	"time"
)

// One analysis-enqueue page holds the single SQLite writer for its whole
// duration. Keep it small enough that a one-core server releases the writer
// often enough for playback control and navigation writes to win between pages.
const analysisEnqueuePageSize = "32"

func (s *Service) enqueueAnalysisPage(ctx context.Context, job string, source catalog.LibrarySource) error {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if _, err = catalog.InventoryFenceTx(ctx, tx, job); err != nil {
		return err
	}
	policy, err := catalog.ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	if policy.Tier == "file_list_only" {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_analysis_queue SET state='skipped' WHERE job_id=? AND state='pending'`, job); err != nil {
			return err
		}
		if err = finishScanTx(ctx, tx, job, source.ID); err != nil {
			return err
		}
		return gated.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT o.id,o.revision,o.analysis_revision,o.analysis_operations_json,o.relative_path FROM inventory_objects o JOIN inventory_runs r ON r.source_id=o.source_id WHERE r.job_id=? AND o.id>r.reconcile_cursor AND o.retired=0 AND o.state='available' AND o.root_incarnation=r.root_incarnation AND o.unsupported_reason='' ORDER BY o.id LIMIT `+analysisEnqueuePageSize+``, job)
	if err != nil {
		return err
	}
	type object struct{ id, revision, analyzed, ops, path string }
	batch := []object{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.id, &o.revision, &o.analyzed, &o.ops, &o.path); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var strmEnabled bool
	var strmRevision int64
	err = tx.QueryRowContext(ctx, `SELECT enabled,revision FROM source_strm_analysis WHERE library_id=?`, source.LibraryID).Scan(&strmEnabled, &strmRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	for _, o := range batch {
		descriptor := strings.EqualFold(filepath.Ext(o.path), ".strm")
		wanted := append([]string(nil), policy.Operations...)
		if !descriptor && policy.Allows("probe") {
			wanted = append(wanted, assets.PlaybackAnalysisRevision)
		}
		if descriptor {
			wanted = []string{"probe", "strm_target:" + strconv.FormatInt(strmRevision, 10)}
			for _, op := range catalog.DeepScanOperations {
				if policy.Allows(op) {
					wanted = append(wanted, op)
				}
			}
		}
		pending := o.analyzed != o.revision
		var completed []string
		if json.Unmarshal([]byte(o.ops), &completed) != nil {
			pending = true
		}
		for _, op := range wanted {
			found := false
			for _, done := range completed {
				found = found || done == op
			}
			for _, deep := range catalog.DeepScanOperations {
				if op == deep && found {
					if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_heads h JOIN analysis_results r ON r.id=h.result_id WHERE h.object_id=? AND h.source_revision=? AND h.stage=? AND r.algorithm=? AND r.root_incarnation=? AND r.configuration_generation=? AND r.retired_ms=0)`, o.id, o.revision, op, mediaanalysis.Algorithm, source.Incarnation, source.Generation).Scan(&found); err != nil {
						return err
					}
				}
			}
			pending = pending || !found
		}
		if descriptor {
			for _, op := range catalog.DeepScanOperations {
				pending = pending || policy.Allows(op)
			}
		}
		state := "done"
		if pending {
			state = "pending"
		}
		if descriptor && (!strmEnabled || !policy.Allows("probe")) {
			state = "skipped"
			if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_state='deferred' WHERE id=? AND analysis_revision!=revision`, o.id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_analysis_queue(job_id,object_id,revision,policy_revision,state) VALUES(?,?,?,?,?) ON CONFLICT(job_id,object_id) DO UPDATE SET revision=excluded.revision,policy_revision=excluded.policy_revision,state=excluded.state`, job, o.id, o.revision, policy.Revision, state); err != nil {
			return err
		}
	}
	if len(batch) > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET reconcile_cursor=?,policy_revision=? WHERE job_id=?`, batch[len(batch)-1].id, policy.Revision, job)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET phase='basic_pending',reconcile_cursor='',policy_revision=? WHERE job_id=?`, policy.Revision, job)
	}
	if err != nil {
		return err
	}
	return gated.Commit()
}
func (s *Service) playbackBusy(ctx context.Context, source string) (bool, error) {
	var busy bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_sessions p JOIN inventory_objects o ON o.asset_id=p.asset_id WHERE o.source_id=? AND p.state IN('playing','buffering') AND p.expires_at>?)`, source, time.Now().UTC().Format(time.RFC3339)).Scan(&busy)
	return busy, err
}
func (s *Service) analysisAllowed(ctx context.Context, job, library string, revision int64, op string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=?`, job).Scan(&status); err != nil {
		return err
	}
	if status != "running" {
		return context.Canceled
	}
	policy, err := s.catalog.ScanPolicy(ctx, library)
	if err != nil {
		return err
	}
	if policy.Revision != revision || !policy.Allows(op) {
		return context.Canceled
	}
	return nil
}
func (s *Service) analyzeOne(parent context.Context, job string, source catalog.LibrarySource) error {
	policy, err := s.catalog.ScanPolicy(parent, source.LibraryID)
	if err != nil {
		return err
	}
	var object, revision, relative, analyzed, operations string
	var queuedPolicy int64
	err = s.db.QueryRowContext(parent, `SELECT q.object_id,q.revision,q.policy_revision,o.relative_path,o.analysis_revision,o.analysis_operations_json FROM inventory_analysis_queue q JOIN inventory_objects o ON o.id=q.object_id WHERE q.job_id=? AND q.state='pending' ORDER BY q.object_id LIMIT 1`, job).Scan(&object, &revision, &queuedPolicy, &relative, &analyzed, &operations)
	if errors.Is(err, sql.ErrNoRows) {
		gated2, e := dbwork.Begin(parent, s.db, dbwork.ClassBackgroundMedia)
		if e != nil {
			return e
		}
		tx := gated2.Tx()
		defer gated2.Rollback()
		if _, e = catalog.InventoryFenceTx(parent, tx, job); e != nil {
			return e
		}
		if e = finishScanTx(parent, tx, job, source.ID); e != nil {
			return e
		}
		return gated2.Commit()
	}
	if err != nil {
		return err
	}
	if queuedPolicy != policy.Revision || policy.Tier == "file_list_only" {
		return s.phase(parent, job, "analysis_enqueue", true)
	}
	busy, err := s.playbackBusy(parent, source.ID)
	if err != nil {
		return err
	}
	if busy {
		_, err = dbwork.ExecWrite(parent, s.db, dbwork.ClassBackgroundMedia, `UPDATE inventory_runs SET pause_reason='playback' WHERE job_id=? AND pause_reason!='playback'`, job)
		return err
	}
	if _, err = dbwork.ExecWrite(parent, s.db, dbwork.ClassBackgroundMedia, `UPDATE inventory_runs SET pause_reason='',phase='basic_running' WHERE job_id=?`, job); err != nil {
		return err
	}
	// Completed basic stages are retained when Complete adds downstream stages.
	basicNeeded := analyzed != revision
	var done []string
	_ = json.Unmarshal([]byte(operations), &done)
	for _, op := range catalog.BasicScanOperations {
		if policy.Allows(op) {
			found := false
			for _, v := range done {
				found = found || v == op
			}
			basicNeeded = basicNeeded || !found
		}
	}
	descriptor := strings.EqualFold(filepath.Ext(relative), ".strm")
	if !descriptor && policy.Allows("probe") {
		found := false
		for _, op := range done {
			found = found || op == assets.PlaybackAnalysisRevision
		}
		basicNeeded = basicNeeded || !found
	}
	var strmRevision int64
	if descriptor {
		var enabled bool
		err = s.db.QueryRowContext(parent, `SELECT enabled,revision FROM source_strm_analysis WHERE library_id=?`, source.LibraryID).Scan(&enabled, &strmRevision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !enabled {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, false, "")
		}
		basicNeeded = analyzed != revision
		found := false
		for _, op := range done {
			found = found || op == "strm_target:"+strconv.FormatInt(strmRevision, 10)
		}
		basicNeeded = basicNeeded || !found
	}
	deep := false
	for _, op := range catalog.DeepScanOperations {
		deep = deep || policy.Allows(op)
	}
	if !policy.Allows("probe") || !basicNeeded {
		if deep {
			return s.analyzeDeep(parent, job, object, revision, policy, source)
		}
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, false, "")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	// Policy, pause/cancel and foreground activity preempt a running bounded read;
	// it stays pending and does not consume a failure attempt.
	watcherDone := make(chan struct{})
	defer close(watcherDone)
	supervise.Go("ingestion.probe-watch", func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-watcherDone:
				return
			case <-ticker.C:
				if e := s.analysisAllowed(ctx, job, source.LibraryID, policy.Revision, "probe"); e != nil {
					cancel()
					return
				}
				if b, e := s.playbackBusy(ctx, source.ID); e != nil || b {
					cancel()
					return
				}
			}
		}
	})
	path, err := relativeObjectPath(source, relative)
	if err != nil {
		return err
	}
	stat := func() (storage.Snapshot, error) {
		r := storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, RelativePath: relative, FollowSymlinks: source.FollowSymlinks}
		if s.storage != nil {
			return s.storage.InventoryStat(ctx, source.ID, r)
		}
		return storage.LocalInventoryStat(r)
	}
	matches := func(v storage.Snapshot) bool {
		objectID := v.ObjectIdentity
		if objectID == "" {
			objectID = "path:" + v.Path
		}
		e := mediasource.InventoryEvidence{Kind: source.Kind, Scope: source.ID + ":" + source.Incarnation, Object: objectID, Size: v.Size, ModifiedNS: v.ModifiedNS, ChangeToken: v.ChangeToken}
		r, e2 := e.Revision()
		return e2 == nil && r == revision
	}
	before, err := stat()
	if err != nil || !matches(before) {
		if ctx.Err() != nil {
			return nil
		}
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "source_revision_changed")
	}
	probe := s.probe
	probe.ReadGuard = func(check context.Context, _ string) error {
		return s.analysisAllowed(check, job, source.LibraryID, policy.Revision, "probe")
	}
	readContext := func(operation string) context.Context {
		rooted := storage.WithScanInput(ctx, s.storage, source.ID, storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, FollowSymlinks: source.FollowSymlinks})
		return storage.WithScanReadGuard(rooted, func(check context.Context, candidate string) error {
			if err := s.analysisAllowed(check, job, source.LibraryID, policy.Revision, operation); err != nil {
				return err
			}
			rel, err := filepath.Rel(source.ResolvedPath, candidate)
			if err != nil || !filepath.IsLocal(rel) {
				return catalog.ErrAdminQuery
			}
			r := storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, RelativePath: rel, FollowSymlinks: source.FollowSymlinks}
			if s.storage != nil {
				_, err = s.storage.InventoryStat(check, source.ID, r)
			} else {
				_, err = storage.LocalInventoryStat(r)
			}
			return err
		})
	}
	var f assets.Facts
	if err = s.analysisAllowed(ctx, job, source.LibraryID, policy.Revision, "probe"); err != nil {
		return nil
	}
	if descriptor {
		if s.AnalyzeSTRM == nil {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "strm_analysis_unavailable")
		}
		f, err = s.AnalyzeSTRM(ctx, source.LibraryID, path)
		s.recordDescriptorAnalysis(ctx, path, err)
	} else if s.storage.IsRemote(path) {
		f, err = s.inspectRemote(ctx, path, before)
	} else {
		f, err = probe.InspectScan(readContext("probe"), path)
		if err == nil {
			probe.AnalyzePlayback(readContext("probe"), path, &f)
		}
	}
	if err == nil && f.InventoryOnly {
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "remote_probe_unavailable")
	}
	completed := []string{"probe"}
	if !descriptor {
		completed = append(completed, assets.PlaybackAnalysisRevision)
	}
	if descriptor {
		completed = append(completed, "strm_target:"+strconv.FormatInt(strmRevision, 10))
	}
	if err == nil && !descriptor && policy.Allows("local_metadata") && s.LocalMetadata != nil {
		if err = s.analysisAllowed(ctx, job, source.LibraryID, policy.Revision, "local_metadata"); err == nil {
			var kind string
			err = s.db.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind)
			if err == nil {
				if kind == "music" || kind == "audiobook" {
					f = s.LocalMetadata.PrepareKind(readContext("local_metadata"), source.LibraryID, kind, path, f)
				} else if kind == "movie" || kind == "tv" || kind == "anime" {
					f = s.LocalMetadata.VideoNFO(readContext("local_metadata"), source.LibraryID, source.ResolvedPath, path, kind, f)
					if kind == "movie" {
						f = s.LocalMetadata.MovieArtwork(readContext("local_metadata"), source.LibraryID, path, f)
					}
				}
				// Sidecar images for every agent (Spec — Page Content §0.5):
				// show and season art, artist portraits, movie logos and
				// parent-folder album covers. Keys travel in Facts; the
				// catalog commit writes them as local_artwork rows.
				if kind == "movie" || kind == "tv" || kind == "anime" || kind == "music" {
					season := -1
					if kind == "tv" || kind == "anime" {
						if rel, rerr := filepath.Rel(source.ResolvedPath, path); rerr == nil {
							if plan := catalog.ParseEpisode(rel, kind); plan.Issue == "" && plan.Season >= 0 && plan.Season <= 9999 {
								season = plan.Season
							}
						}
					}
					extra := s.LocalMetadata.ReadSidecars(readContext("local_metadata"), source.LibraryID, kind, source.ResolvedPath, path, season)
					if key := extra["album/cover"]; key != "" && f.ArtworkKey == "" {
						f.ArtworkKey = key
						delete(extra, "album/cover")
					}
					if len(extra) > 0 {
						f.ArtworkExtra = extra
					}
				}
			}
		}
	}
	if err == nil && !descriptor && policy.Allows("local_metadata") && s.LocalMetadata != nil && f.LocalMetadataIssue == "" {
		completed = append(completed, "local_metadata")
	}
	if err == nil && !descriptor && policy.Allows("subtitles") && f.VideoCodec != "" {
		if err = s.analysisAllowed(ctx, job, source.LibraryID, policy.Revision, "subtitles"); err == nil {
			tracks := []subtitles.Track{}
			for _, st := range f.Streams {
				if st.Type != "subtitle" {
					continue
				}
				format, reason := subtitles.CodecFormat(st.Codec), ""
				if format == "" {
					format = st.Codec
					reason = "unsupported_format"
				}
				tracks = append(tracks, subtitles.Track{Origin: "embedded", StreamIndex: st.Index, Format: format, Language: st.Language, Title: st.Title, Default: st.Default, Forced: st.Forced, Reason: reason})
			}
			if s.storage.IsRemote(path) {
				relative, _ := filepath.Rel(source.ResolvedPath, filepath.Dir(path))
				if relative == "." {
					relative = ""
				}
				f.SubtitleInventory, err = subtitles.DiscoverManaged(readContext("subtitles"), s.db, s.storage, source.ID, source.Incarnation, job, relative, path, tracks, f.OriginUS, f.TimingKnown)
			} else {
				f.SubtitleInventory, err = subtitles.Discover(readContext("subtitles"), s.storage, source.LibraryID, path, tracks, f.OriginUS, f.TimingKnown)
			}
		}
	}
	if err == nil && !descriptor && policy.Allows("subtitles") && (f.VideoCodec == "" || f.SubtitleInventory != nil && f.SubtitleInventory.Status == "known") {
		completed = append(completed, "subtitles")
	}
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "probe_or_sidecar_failed")
	}
	after, err := stat()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil || !matches(after) {
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "source_revision_changed")
	}
	f.ObservedSize, f.ObservedModifiedNS, f.ObservedRevision = before.Size, before.ModifiedNS, before.Revision
	f.AnalysisOperations = completed
	f.SubtitleTextPending = !descriptor && policy.Allows("subtitles") && s.Subtitles != nil && f.VideoCodec != ""
	if err = s.catalog.CommitInventoryAnalysis(ctx, job, object, revision, policy.Revision, f); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "analysis_publication_failed")
	}
	if f.SubtitleTextPending {
		var item, asset string
		if err = s.db.QueryRowContext(ctx, `SELECT pid(e.public_id),a.token FROM inventory_objects o JOIN catalog_assets a ON a.token=o.asset_id JOIN catalog_asset_links l ON l.asset_id=a.id JOIN catalog_entities e ON e.id=l.entity_id JOIN catalog_libraries cl ON cl.id=e.library_id WHERE o.id=? AND o.revision=? AND cl.library_id=? ORDER BY e.id LIMIT 1`, object, revision, source.LibraryID).Scan(&item, &asset); errors.Is(err, sql.ErrNoRows) {
			// The file never became an item (for example a naming issue left
			// it unassigned): there is no subtitle text to import, so skip
			// without hiding the real issue behind subtitle_text_import_failed.
			err = nil
		} else if err == nil {
			err = s.Subtitles.ImportScanText(ctx, item, asset, func(check context.Context, tx *sql.Tx) error {
				live, e := catalog.InventoryFenceTx(check, tx, job)
				if e != nil {
					return e
				}
				if live.ID != source.ID || live.Incarnation != source.Incarnation {
					return catalog.ErrAdminQuery
				}
				current, e := catalog.ScanPolicyTx(check, tx, source.LibraryID)
				if e != nil {
					return e
				}
				if current.Revision != policy.Revision || !current.Allows("subtitles") {
					return catalog.ErrAdminQuery
				}
				var valid bool
				e = tx.QueryRowContext(check, `SELECT EXISTS(SELECT 1 FROM inventory_objects WHERE id=? AND revision=? AND asset_id=? AND state='available' AND retired=0)`, object, revision, asset).Scan(&valid)
				if e != nil {
					return e
				}
				if !valid {
					return catalog.ErrAdminQuery
				}
				return nil
			})
		}
		if err == nil {
			err = s.catalog.CompleteInventoryTextImports(ctx, job, object, revision, policy.Revision)
		}
		if err != nil {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "subtitle_text_import_failed")
		}
	}
	if !descriptor {
		for _, op := range catalog.BasicScanOperations {
			if !policy.Allows(op) {
				continue
			}
			found := false
			for _, done := range completed {
				found = found || done == op
			}
			if !found {
				return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "sidecar_analysis_incomplete")
			}
		}
	}
	if deep {
		return s.analyzeDeep(parent, job, object, revision, policy, source)
	}
	return nil
}
func (s *Service) analysisOutcome(ctx context.Context, job, object, revision string, policy int64, warning bool, code string) error {
	gated3, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	source, err := catalog.InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	current, err := catalog.ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	if current.Revision != policy {
		return nil
	}
	state := "done"
	if warning {
		state = "warning"
	} else {
		code = ""
	}
	result, err := tx.ExecContext(ctx, `UPDATE inventory_analysis_queue SET state=? WHERE job_id=? AND object_id=? AND revision=? AND policy_revision=? AND state!='warning'`, state, job, object, revision, policy)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n > 0 && warning {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_error=?,analysis_state='warning' WHERE id=? AND revision=?`, code, object, revision); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET warnings=warnings+1 WHERE job_id=?`, job); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_changes(source_id,job_id,object_id,asset_id,revision,kind,created_at) SELECT ?,?,id,asset_id,revision,?,? FROM inventory_objects WHERE id=? AND revision=?`, source.ID, job, code, time.Now().UTC().Format(time.RFC3339Nano), object, revision); err != nil {
			return err
		}
	}
	if !warning {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_state='complete',analysis_error='' WHERE id=? AND revision=?`, object, revision); err != nil {
			return err
		}
	}
	return gated3.Commit()
}
func finishScanTx(ctx context.Context, tx *sql.Tx, job, source string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// A file the library couldn't turn into an item (an episode name needing
	// assignment) keeps the scan from reading as clean, whichever scan last
	// looked at it: an incremental scan that skipped its folder still ends
	// "with warnings" and the source isn't "healthy" while any remain. One
	// seek on the issues-only index (0223).
	attention := `EXISTS(SELECT 1 FROM episodic_sources e WHERE e.library_id=(SELECT library_id FROM library_sources WHERE id=?) AND e.issue<>'')`
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN (SELECT warnings FROM inventory_runs WHERE job_id=?)>0 OR `+attention+` THEN 'complete_with_warnings' ELSE 'complete' END WHERE id=? AND status='running'`, job, source, job); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory_runs SET phase=(SELECT status FROM jobs WHERE id=?),finished_at=?,pause_reason='' WHERE job_id=?`, job, now, job); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_sources SET health=CASE WHEN (SELECT warnings FROM inventory_runs WHERE job_id=?)>0 OR `+attention+` THEN 'degraded' ELSE 'healthy' END,last_progress_at=? WHERE id=?`, job, source, now, source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM inventory_source_active WHERE job_id=?`, job); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_changes(source_id,job_id,kind,created_at) VALUES(?,?,'scan_completed',?)`, source, job, now)
	return err
}
