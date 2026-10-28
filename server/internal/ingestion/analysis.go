package ingestion

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/disc"
	"portico.local/server/internal/localmetadata"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"portico.local/server/internal/supervise"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// One analysis-enqueue page holds the single SQLite writer for its whole
// duration. Keep it small enough that a one-core server releases the writer
// often enough for playback control and navigation writes to win between pages.
const analysisEnqueuePageSize = "32"

func (s *Service) enqueueAnalysisPage(ctx context.Context, job string, source catalog.LibrarySource) error {
	if more, e := s.enqueueSidecarChanges(ctx, job, source); more || e != nil {
		return e
	}
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
		followUp, err := finishScanTx(ctx, tx, job, source.ID)
		if err != nil {
			return err
		}
		return s.commitWithFollowUp(ctx, gated.Commit, source, followUp)
	}
	var strmEnabled bool
	var strmRevision int64
	err = tx.QueryRowContext(ctx, `SELECT enabled,revision FROM source_strm_analysis WHERE library_id=?`, source.LibraryID).Scan(&strmEnabled, &strmRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	required := append([]string(nil), policy.Operations...)
	if !s.DeepAnalysis.SonicCanAttempt() {
		kept := required[:0]
		for _, op := range required {
			if op != catalog.SonicOperation {
				kept = append(kept, op)
			}
		}
		required = kept
	}
	if policy.Allows("probe") {
		required = append(required, assets.PlaybackAnalysisRevision)
	}
	// A remote source's own page can switch whole-file analysis stages off for
	// the files found through it. Local folders keep the library's policy.
	if switches, ok, e := remotesources.AnalysisSwitchesTx(ctx, tx, source.ID); e != nil {
		return e
	} else if ok {
		required = switches.Filter(required)
	}
	requiredJSON, _ := json.Marshal(required)
	// Bound evidence evaluation even when the entire source is already complete.
	// Unchanged candidates advance the cursor without creating queue rows.
	rows, err := tx.QueryContext(ctx, `WITH batch AS MATERIALIZED (
 SELECT o.* FROM inventory_objects o JOIN inventory_runs r ON r.source_id=o.source_id
 WHERE r.job_id=? AND o.id>r.reconcile_cursor AND o.retired=0 AND o.state='available' AND o.root_incarnation=r.root_incarnation AND o.unsupported_reason=''
 ORDER BY o.id LIMIT `+analysisEnqueuePageSize+`)
 SELECT o.id,o.revision,(lower(substr(o.relative_path,-5)) NOT IN('.epub','.mobi','.azw3') AND lower(substr(o.relative_path,-4)) NOT IN('.pdf','.cbz','.cb7','.cbr') AND (lower(substr(o.relative_path,-5))<>'.strm' OR ?=1) AND (o.analysis_revision<>o.revision OR o.analysis_policy<>? OR o.analysis_state='pending' OR o.analysis_generation<>r.source_generation OR (o.analysis_algorithm<>? AND EXISTS(SELECT 1 FROM json_each((SELECT operations_json FROM library_scan_policies WHERE library_id=?)) WHERE value IN('checksum','chapter_images','trickplay','waveform','loudness','fingerprint'))) OR
 (lower(substr(o.relative_path,-5))<>'.strm' AND EXISTS(SELECT 1 FROM json_each(?) wanted WHERE NOT EXISTS(SELECT 1 FROM json_each(o.analysis_operations_json) done WHERE done.value=wanted.value))) OR
 EXISTS(SELECT 1 FROM json_each(?) wanted WHERE wanted.value IN('checksum','chapter_images','trickplay','waveform','loudness','fingerprint') AND NOT EXISTS(SELECT 1 FROM analysis_heads h JOIN analysis_results a ON a.id=h.result_id WHERE h.object_id=o.id AND h.source_revision=o.revision AND h.stage=wanted.value AND a.algorithm=? AND a.root_incarnation=r.root_incarnation AND a.configuration_generation=r.source_generation AND a.retired_ms=0)) OR
 (lower(substr(o.relative_path,-5))='.strm' AND NOT EXISTS(SELECT 1 FROM json_each(o.analysis_operations_json) done WHERE done.value=?))))
 FROM batch o JOIN inventory_runs r ON r.source_id=o.source_id WHERE r.job_id=? ORDER BY o.id`, job, strmEnabled && policy.Allows("probe"), policy.Revision, mediaanalysis.Algorithm, source.LibraryID, string(requiredJSON), string(requiredJSON), mediaanalysis.Algorithm, "strm_target:"+strconv.FormatInt(strmRevision, 10), job)
	if err != nil {
		return err
	}
	type object struct {
		id, revision string
		needs        bool
	}
	batch := []object{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.id, &o.revision, &o.needs); err != nil {
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
	for _, o := range batch {
		if !o.needs && policy.Allows(catalog.SonicOperation) && s.DeepAnalysis.SonicCanAttempt() {
			current, e := s.DeepAnalysis.SonicCurrentTx(ctx, tx, mediaanalysis.Request{JobID: job, ObjectID: o.id, SourceRevision: o.revision, PolicyRevision: policy.Revision})
			if e != nil {
				return e
			}
			o.needs = !current
		}
		if !o.needs {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_analysis_queue(job_id,object_id,revision,policy_revision,state) VALUES(?,?,?,?,'pending') ON CONFLICT(job_id,object_id) DO UPDATE SET revision=excluded.revision,policy_revision=excluded.policy_revision,state=excluded.state`, job, o.id, o.revision, policy.Revision); err != nil {
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
	var object, revision, relative, analyzed, operations, analysisState string
	var queuedPolicy, analysisPolicy int64
	err = s.db.QueryRowContext(parent, `SELECT q.object_id,q.revision,q.policy_revision,o.relative_path,o.analysis_revision,o.analysis_operations_json,o.analysis_policy,o.analysis_state FROM inventory_analysis_queue q JOIN inventory_objects o ON o.id=q.object_id WHERE q.job_id=? AND q.state='pending' ORDER BY q.object_id LIMIT 1`, job).Scan(&object, &revision, &queuedPolicy, &relative, &analyzed, &operations, &analysisPolicy, &analysisState)
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
		followUp, e := finishScanTx(parent, tx, job, source.ID)
		if e != nil {
			return e
		}
		return s.commitWithFollowUp(parent, gated2.Commit, source, followUp)
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
		if !waitScanPace(parent, 250*time.Millisecond) {
			return parent.Err()
		}
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
	var discRequested bool
	if err = s.db.QueryRowContext(parent, `SELECT EXISTS(SELECT 1 FROM inventory_disc_title_requests WHERE job_id=? AND object_id=? AND source_revision=?)`, job, object, revision).Scan(&discRequested); err != nil {
		return err
	}
	basicNeeded = basicNeeded || discRequested
	deep := false
	for _, op := range catalog.DeepScanOperations {
		deep = deep || policy.Allows(op)
	}
	// Root64: a remote disc image is never probed, deep-analysed or read for a
	// scan, whatever historical operation tokens the row carries and whether or
	// not its basic bookkeeping looks complete. Naming is the only analysis it
	// gets, and this revision's naming being recorded is what completes it.
	// The source's own kind is the authority: a stale or missing storage registry
	// must never let a remote image fall into the local disc probe.
	remoteDisc := source.Kind == "remote" && assets.DiscImage(relative)
	if remoteDisc && analyzed == revision && analysisPolicy == queuedPolicy && analysisState == "deferred" {
		// Naming is already recorded for this exact revision and policy. Settle
		// this queue invocation through the ordinary outcome path, which reproves
		// the fence and policy revision, and reads nothing from the source.
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, false, "")
	}
	if !remoteDisc && (!policy.Allows("probe") || !basicNeeded) {
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
				if remoteDisc {
					// Naming has no probe prerequisite. Its sidecar reads separately
					// reprove local_metadata; retain the original running job/policy.
					var status string
					var currentPolicy int64
					e := s.db.QueryRowContext(ctx, `SELECT j.status,p.revision FROM jobs j JOIN library_scan_policies p ON p.library_id=j.library_id WHERE j.id=? AND j.library_id=?`, job, source.LibraryID).Scan(&status, &currentPolicy)
					if e != nil || status != "running" || currentPolicy != policy.Revision {
						cancel()
						return
					}
				} else if e := s.analysisAllowed(ctx, job, source.LibraryID, policy.Revision, "probe"); e != nil {
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
		// A folder disc's object path is a directory, which the regular-file
		// observation refuses; the directory observation is its exact stat.
		if s.storage != nil {
			if v, err := s.storage.InventoryStat(ctx, source.ID, r); err == nil {
				return v, nil
			}
			return s.storage.InventoryDirectory(ctx, source.ID, r)
		}
		if v, err := storage.LocalInventoryStat(r); err == nil {
			return v, nil
		}
		return storage.LocalInventoryDirectory(r)
	}
	// A folder disc carries its title bytes below the observed folder, so the
	// queued revision includes the last committed structure witness. The same
	// token is applied to every stat of this object while it is analysed.
	discToken := ""
	matches := func(v storage.Snapshot) bool {
		if v.Directory {
			v.ChangeToken = discToken
		}
		r, _, e2 := catalog.ObservationRevision(source, v)
		return e2 == nil && r == revision
	}
	clearActivityFile := s.activityFile(ctx, job, relative)
	defer clearActivityFile()
	before, err := stat()
	if err == nil && before.Directory {
		var fingerprint string
		if e := s.db.QueryRowContext(ctx, `SELECT COALESCE(fingerprint,'') FROM inventory_disc_facts WHERE object_id=?`, object).Scan(&fingerprint); e == nil {
			discToken = catalog.DiscStructureWitness(fingerprint)
		}
	}
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
	// One directory recheck is shared by every projection for this file, so a
	// folder is observed once per analysis instead of once per consumer. The
	// projections stay separate: each still answers only its own question, and
	// none of them is an I/O grant.
	listed, artworkListed, musicListed := s.analysisListings(ctx, job, source)
	artworkObserved := true
	readContext := func(operation string) context.Context {
		// The original media target is the invocation, not the observation of a
		// separately inventoried NFO, cover or subtitle candidate read below.
		admission := storage.InventoryBlobAdmission{Purpose: storage.InventoryBlobAnalysisSidecar,
			JobID: job, LibraryID: source.LibraryID, LibrarySourceID: source.ID,
			RootIncarnation: source.Incarnation, RootIdentity: source.RootIdentity, SourceGeneration: source.Generation,
			PolicyRevision: policy.Revision, ObjectID: object, SourceRevision: revision, Operation: operation}
		rooted := storage.WithScanInput(ctx, s.storage, source.ID, storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, FollowSymlinks: source.FollowSymlinks}, admission)
		// What this scan saw when it listed the directory spares a helper
		// process for every sidecar that is not there.
		rooted = localmetadata.WithListing(rooted, listed)
		rooted = localmetadata.WithMusicTrackListing(rooted, musicListed)
		rooted = localmetadata.WithArtworkListing(rooted, artworkListed)
		rooted = localmetadata.WithArtworkReadCheck(rooted, func() { artworkObserved = false })
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
	if remoteDisc {
		// Root64: a remote disc image is an ordinary title. A scan never reads
		// the image, so publish its name and local metadata only and leave
		// technical facts deferred; no probe, ranged read or disc read runs here.
		f := assets.Facts{}
		completed := []string{}
		if policy.Allows("local_metadata") && s.LocalMetadata != nil {
			if e := s.analysisAllowed(ctx, job, source.LibraryID, policy.Revision, "local_metadata"); e == nil {
				var kind string
				if e = s.db.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind); e == nil {
					f = s.LocalMetadata.VideoNFO(readContext("local_metadata"), source.LibraryID, source.ResolvedPath, path, kind, f)
					if kind == "movie" {
						f = s.LocalMetadata.MovieArtwork(readContext("local_metadata"), source.LibraryID, path, f)
					}
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
					f.ArtworkCandidates, f.ArtworkCandidatesObserved = s.LocalMetadata.ReadArtworkCandidates(readContext("local_metadata"), source.LibraryID, kind, source.ResolvedPath, path, season)
					f.ArtworkObserved = f.LocalMetadataIssue == ""
					completed = append(completed, "local_metadata")
				}
			}
		}
		after, e := stat()
		if e != nil || !matches(after) {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "source_revision_changed")
		}
		f.ObservedSize, f.ObservedModifiedNS, f.ObservedRevision = before.Size, before.ModifiedNS, before.Revision
		f.AnalysisOperations = completed
		if err = s.catalog.CommitInventoryNaming(ctx, job, object, revision, policy.Revision, f); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "analysis_publication_failed")
		}
		return nil
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
	} else if before.Directory {
		if !storage.DirectoryDiscScanSupported() {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "unsupported_disc_layout")
		}
		f, err = s.inspectDisc(ctx, job, object, revision, policy.Revision, source, relative, readContext("probe"), probe, path, true)
		if errors.Is(err, disc.ErrEncrypted) {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "unsupported_disc_image")
		}
	} else if assets.DiscImage(path) {
		f, err = s.inspectDisc(ctx, job, object, revision, policy.Revision, source, relative, readContext("probe"), probe, path, false)
		if errors.Is(err, disc.ErrEncrypted) {
			// Portico ships no disc decryption (VIDEO §3 VS-4). The image is
			// catalogued as unsupported rather than probed as a broken file.
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "unsupported_disc_image")
		}
	} else {
		f, err = probe.InspectScan(readContext("probe"), path)
		if err == nil {
			probe.AnalyzePlayback(readContext("probe"), path, &f)
		}
	}
	if err == nil && f.DiscAlternate {
		after, e := stat()
		if e != nil || !matches(after) {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "source_revision_changed")
		}
		if err = s.catalog.CommitDiscAlternate(ctx, job, object, revision, policy.Revision, f); err != nil {
			return s.analysisOutcome(parent, job, object, revision, queuedPolicy, true, "probe_or_sidecar_failed")
		}
		return s.analysisOutcome(parent, job, object, revision, queuedPolicy, false, "")
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
					f = s.LocalMetadata.PrepareKind(readContext("local_metadata"), source.LibraryID, kind, source.ResolvedPath, path, f)
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
					if kind == "movie" || kind == "tv" || kind == "anime" {
						f.ArtworkCandidates, f.ArtworkCandidatesObserved = s.LocalMetadata.ReadArtworkCandidates(readContext("local_metadata"), source.LibraryID, kind, source.ResolvedPath, path, season)
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
				tracks = append(tracks, subtitles.Track{Origin: "embedded", StreamIndex: st.Index, Format: format, Language: st.Language, Title: st.Title, Default: st.Default, Forced: st.Forced, HearingImpaired: st.HearingImpaired, Reason: reason})
			}
			relative, _ := filepath.Rel(source.ResolvedPath, filepath.Dir(path))
			f.SubtitleInventory, err = subtitles.DiscoverManaged(readContext("subtitles"), s.db, s.storage, source.ID, source.Incarnation, job, relative, path, tracks, f.OriginUS, f.TimingKnown)
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
	f.ArtworkObserved = policy.Allows("local_metadata") && s.LocalMetadata != nil && artworkObserved && f.LocalMetadataIssue == ""
	f.AnalysisOperations = completed
	f.AnalysisAlgorithm = mediaanalysis.Algorithm
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

// inspectDisc probes one disc image as a single title. The bounded helper
// observes the disc structure, the committed structure picks the main title
// (the longest of at least a minute, ties by disc order), and only that title's
// timed media is read through the closed disc input (VIDEO §3 VS-4a/VS-4b).
func (s *Service) inspectDisc(ctx context.Context, job, object, revision string, policyRevision int64, source catalog.LibrarySource, relative string, readCtx context.Context, probe assets.Probe, path string, directory bool) (assets.Facts, error) {
	tool := disc.Tool{}
	probeStructure := func() (disc.Probe, error) {
		if directory {
			return tool.ProbeDirectory(readCtx, path)
		}
		return tool.ProbeLocal(readCtx, path)
	}
	structure, err := probeStructure()
	if err != nil {
		return assets.Facts{}, err
	}
	if structure.Encryption != "none" {
		return assets.Facts{}, disc.ErrEncrypted
	}
	title, ok, err := s.catalog.InventoryDiscMainTitle(ctx, object, revision, structure)
	var requestedKey, requestedFingerprint string
	requestErr := s.db.QueryRowContext(ctx, `SELECT title_key,fingerprint FROM inventory_disc_title_requests WHERE job_id=? AND object_id=? AND source_revision=?`, job, object, revision).Scan(&requestedKey, &requestedFingerprint)
	if requestErr != nil && !errors.Is(requestErr, sql.ErrNoRows) {
		return assets.Facts{}, requestErr
	}
	if requestedKey != "" {
		raw, e := json.Marshal(structure)
		if e != nil {
			return assets.Facts{}, e
		}
		sum := sha256.Sum256(append([]byte("disc_structure_v1\x00"), raw...))
		if hex.EncodeToString(sum[:]) != requestedFingerprint {
			return assets.Facts{}, catalog.ErrDiscSelectionChanged
		}
		ok = false
		for _, candidate := range structure.Titles {
			if candidate.Key(structure.Kind) == requestedKey {
				title = candidate
				ok = true
				break
			}
		}
	}
	if err != nil {
		return assets.Facts{}, err
	}
	if !ok {
		return assets.Facts{}, errors.New("disc has no title of at least a minute")
	}
	manifest := func() ([]storage.DiscMember, error) { return nil, nil }
	if directory {
		consumed, e := disc.ConsumedMembers(structure)
		if e != nil {
			return assets.Facts{}, e
		}
		required, optional, directories := []string{}, []string{}, []string{}
		for _, member := range consumed {
			switch {
			case member.Directory:
				directories = append(directories, member.Path)
			case member.Required:
				required = append(required, member.Path)
			default:
				optional = append(optional, member.Path)
			}
		}
		req := storage.InventoryRequest{Root: source.ResolvedPath, RelativePath: relative, RootIdentity: source.RootIdentity, FollowSymlinks: source.FollowSymlinks}
		manifest = func() ([]storage.DiscMember, error) {
			if s.storage != nil {
				return s.storage.DiscMemberManifest(readCtx, source.ID, req, required, optional, directories)
			}
			return storage.LocalDiscMemberManifest(req, required, optional, directories)
		}
	}
	// The consumed files are witnessed before and after the reads, and the
	// structure is observed again between them, so a changed member can never
	// mix two source revisions into one publication.
	before, err := manifest()
	if err != nil {
		return assets.Facts{}, err
	}
	again, err := probeStructure()
	if err != nil {
		return assets.Facts{}, err
	}
	if !reflect.DeepEqual(structure, again) {
		return assets.Facts{}, errors.New("disc structure changed during analysis")
	}
	facts, err := probe.InspectDiscScan(readCtx, path, again, title)
	if err != nil {
		return assets.Facts{}, err
	}
	if directory {
		after, e := manifest()
		if e != nil {
			return assets.Facts{}, e
		}
		if len(before) == 0 || len(after) == 0 || !sameDiscMembers(before, after) {
			return assets.Facts{}, errors.New("disc members changed during analysis")
		}
	}
	if err = s.catalog.CommitInventoryDiscProbe(ctx, job, object, revision, policyRevision, again); err != nil {
		return assets.Facts{}, err
	}
	raw, err := json.Marshal(again)
	if err != nil {
		return assets.Facts{}, err
	}
	digest := sha256.Sum256(append([]byte("disc_structure_v1\x00"), raw...))
	facts.DiscAlternate = requestedKey != ""
	facts.DiscSelection = &assets.DiscMediaSelection{TitleKey: title.Key(again.Kind), StructureFingerprint: hex.EncodeToString(digest[:]), Members: before}
	return facts, nil
}

// sameDiscMembers compares the bounded identities and revisions of the files
// the title read actually consumed.
func sameDiscMembers(a, b []storage.DiscMember) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].ObjectIdentity != b[i].ObjectIdentity || a[i].Size != b[i].Size || a[i].ModifiedNS != b[i].ModifiedNS || a[i].Required != b[i].Required || a[i].Directory != b[i].Directory {
			return false
		}
	}
	return true
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
		var relative string
		if err = tx.QueryRowContext(ctx, `SELECT relative_path FROM inventory_objects WHERE id=?`, object).Scan(&relative); err != nil {
			return err
		}
		if code == "source_revision_changed" {
			if err = recordNoticeTx(ctx, tx, source.ID, filepath.Dir(relative), time.Now().UnixMilli()); err != nil {
				return err
			}
		}
		if err = catalog.InventoryIssueTx(ctx, tx, job, storage.InventoryIssue{Path: relative, Reason: code, Warning: false}); err != nil {
			return err
		}
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
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_state='complete',analysis_error='',analysis_algorithm=?,analysis_generation=? WHERE id=? AND revision=?`, mediaanalysis.Algorithm, source.Generation, object, revision); err != nil {
			return err
		}
	}
	return gated3.Commit()
}

// commitWithFollowUp commits the finishing run, then queues the one bounded
// second folder pass it asked for through the ordinary scan path. Queueing is
// deduplicated per folder, so an active scan is not stacked.
func (s *Service) commitWithFollowUp(ctx context.Context, commit func() error, source catalog.LibrarySource, followUp bool) error {
	if err := commit(); err != nil {
		return err
	}
	if !followUp {
		return nil
	}
	_, err := s.QueueMode(ctx, source.LibraryID, source.ID, "incremental", nil)
	return err
}

func finishScanTx(ctx context.Context, tx *sql.Tx, job, source string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// A file the library couldn't turn into an item (an episode name needing
	// assignment) keeps the scan from reading as clean, whichever scan last
	// looked at it: an incremental scan that skipped its folder still ends
	// "with warnings" and the source isn't "healthy" while any remain. One
	// seek on the issues-only index (0223).
	attention := `EXISTS(SELECT 1 FROM episodic_sources e JOIN inventory_objects o ON o.asset_id=e.asset_id JOIN library_sources s ON s.id=o.source_id WHERE s.id=? AND e.library_id=s.library_id AND e.issue<>'' AND o.retired=0 AND o.state='available' AND o.root_incarnation=s.incarnation)`
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status=CASE WHEN (SELECT warnings FROM inventory_runs WHERE job_id=?)>0 OR `+attention+` THEN 'complete_with_warnings' ELSE 'complete' END WHERE id=? AND status='running'`, job, source, job); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory_runs SET phase=(SELECT status FROM jobs WHERE id=?),finished_at=?,pause_reason='' WHERE job_id=?`, job, now, job); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_sources SET health=CASE WHEN (SELECT warnings FROM inventory_runs WHERE job_id=?)>0 OR `+attention+` THEN 'degraded' ELSE 'healthy' END WHERE id=?`, job, source, source); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO library_source_progress(source_id,last_progress_at) SELECT id,? FROM library_sources WHERE id=? ON CONFLICT(source_id) DO UPDATE SET last_progress_at=excluded.last_progress_at`, now, source); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM inventory_source_active WHERE job_id=?`, job); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_changes(source_id,job_id,kind,created_at) VALUES(?,?,'scan_completed',?)`, source, job, now); err != nil {
		return false, err
	}
	// One bounded second folder pass per initiating run: the run that asked for
	// it is remembered, so the follow-up pass itself never asks again, and the
	// caller queues exactly one job even when the folder's own interval is zero.
	doubleCheck, err := remotesources.DoubleCheckFoldersTx(ctx, tx, source)
	if err != nil {
		return false, err
	}
	var secondPass string
	if err = tx.QueryRowContext(ctx, `SELECT double_check_job FROM library_sources WHERE id=?`, source).Scan(&secondPass); err != nil {
		return false, err
	}
	if secondPass != "" {
		_, err = tx.ExecContext(ctx, `UPDATE library_sources SET double_check_job='' WHERE id=?`, source)
		return false, err
	}
	if !doubleCheck {
		return false, nil
	}
	var changed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_changes WHERE job_id=? AND kind<>'scan_completed')`, job).Scan(&changed); err != nil || !changed {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET double_check_job=? WHERE id=?`, job, source); err != nil {
		return false, err
	}
	return true, nil
}
