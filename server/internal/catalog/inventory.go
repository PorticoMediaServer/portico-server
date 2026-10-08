package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/localmetadata"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"strings"
	"time"
)

// InventoryFenceTx is the last publication check for every page, status and
// analysis result. Cancellation and configuration mutations use the same DB.
func InventoryFenceTx(ctx context.Context, tx *sql.Tx, job string) (LibrarySource, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT r.source_id FROM inventory_runs r JOIN jobs j ON j.id=r.job_id JOIN library_sources s ON s.id=r.source_id WHERE r.job_id=? AND j.status='running' AND s.enabled=1 AND r.source_generation=s.generation AND r.root_incarnation=s.incarnation AND r.root_identity=s.root_identity`, job).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return LibrarySource{}, context.Canceled
	}
	if err != nil {
		return LibrarySource{}, err
	}
	source, err := scanLibrarySource(tx.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM library_sources WHERE id=?`, id))
	if err == nil && source.Kind == "remote" {
		err = remoteInventoryFenceTx(ctx, tx, job)
	}
	return source, err
}
func inventoryEvent(ctx context.Context, tx *sql.Tx, source, job, object, asset, revision, kind string) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_changes(source_id,job_id,object_id,asset_id,revision,kind,created_at) VALUES(?,?,?,?,?,?,?)`, source, job, object, asset, revision, kind, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func InventoryMedia(path, kind string) (bool, string) {
	ext := strings.ToLower(filepath.Ext(path))
	if (ext == ".iso" || ext == ".img") && kind != "music" && kind != "audiobook" {
		return true, "unsupported_disc_image"
	}
	return assets.SupportedForKind(path, kind), ""
}
func observationRevision(source LibrarySource, v storage.Snapshot) (string, string, error) {
	object := v.ObjectIdentity
	if object == "" {
		object = "path:" + v.Path
	}
	e := mediasource.InventoryEvidence{Kind: source.Kind, Scope: source.ID + ":" + source.Incarnation, Object: object, Size: v.Size, ModifiedNS: v.ModifiedNS, ChangeToken: v.ChangeToken}
	revision, err := e.Revision()
	raw, _ := json.Marshal(e)
	return revision, string(raw), err
}
func (s *Service) CommitInventoryPage(ctx context.Context, job, relative, cursor string, page storage.InventoryPage) error {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	source, err := InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	if page.RootIdentity != source.RootIdentity || page.DirectoryIdentity == "" || page.DirectoryRevision == "" || len(page.Entries) > 128 || page.Complete && page.NextCursor != "" || !page.Complete && (page.NextCursor == "" || page.NextCursor == cursor && !page.Pending) {
		return storage.ErrInventoryChanged
	}
	if page.Pending && (page.Remote == nil || len(page.Entries) != 0 || page.Complete) {
		return storage.ErrInventoryChanged
	}
	if page.Remote != nil {
		if s.storage == nil || !s.storage.IsRemote(source.ResolvedPath) || page.Remote.Directory != filepath.Join(source.ResolvedPath, relative) {
			return storage.ErrRemoteConfig
		}
		if err = s.storage.Remote.ValidateScan(ctx, tx, job, source.ResolvedPath); err != nil {
			return err
		}
		if err = s.storage.Remote.CommitPage(ctx, tx, *page.Remote); err != nil {
			return err
		}
	}
	if page.Remote != nil {
		if err = subtitles.CommitManagedPage(ctx, tx, source.ID, source.Incarnation, job, filepath.Join(source.ResolvedPath, relative), page.Entries); err != nil {
			return err
		}
	}
	var oldCursor, state, oldRevision string
	if err = tx.QueryRowContext(ctx, `SELECT cursor,state,revision FROM inventory_directories WHERE job_id=? AND relative_path=?`, job, relative).Scan(&oldCursor, &state, &oldRevision); err != nil {
		return err
	}
	if state == "done" || oldCursor != cursor {
		if oldCursor == page.NextCursor && oldRevision == page.DirectoryRevision {
			return nil
		}
		return ErrStaleContinuation
	}
	if oldRevision != "" && oldRevision != page.DirectoryRevision {
		return storage.ErrInventoryChanged
	}
	var libraryKind string
	if err = tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&libraryKind); err != nil {
		return err
	}
	var policy ScanPolicy
	if policy, err = ScanPolicyTx(ctx, tx, source.LibraryID); err != nil {
		return err
	}
	// Register the current directory before its children so a link back to the
	// root is recognized as an already visited identity rather than a conflict.
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_directories SET identity=?,revision=? WHERE job_id=? AND relative_path=?`, page.DirectoryIdentity, page.DirectoryRevision, job, relative); err != nil {
		return err
	}
	discovered := 0
	for _, v := range page.Entries {
		child := filepath.Join(relative, v.Name)
		expected := filepath.Join(source.ResolvedPath, child)
		if v.Path != expected || !filepath.IsLocal(child) || filepath.Base(v.Name) != v.Name || v.Size < 0 {
			return ErrAdminQuery
		}
		if v.Directory {
			if v.ObjectIdentity == "" {
				return storage.ErrInventoryChanged
			}
			childRevision := storage.InventoryRevision(v)
			if page.Remote != nil {
				childRevision = ""
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_directory_cache(source_id,incarnation,relative_path,parent_path,parent_revision) VALUES(?,?,?,?,?) ON CONFLICT(source_id,incarnation,relative_path) DO UPDATE SET parent_path=excluded.parent_path,parent_revision=excluded.parent_revision`, source.ID, source.Incarnation, child, relative, page.DirectoryRevision); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_directories(job_id,relative_path,identity,revision) VALUES(?,?,?,?)`, job, child, v.ObjectIdentity, childRevision); err != nil {
				return err
			}
			continue
		}
		supported, reason := InventoryMedia(v.Path, libraryKind)
		if !supported {
			continue
		}
		added, e := s.observeInventoryTx(ctx, tx, job, source, child, v, reason, policy)
		if e != nil {
			return e
		}
		if added {
			discovered++
		}
	}
	state = "pending"
	if page.Complete {
		state = "done"
		if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_directory_cache(source_id,incarnation,relative_path,parent_path,identity,revision) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,incarnation,relative_path) DO UPDATE SET identity=excluded.identity,revision=excluded.revision`, source.ID, source.Incarnation, relative, filepath.Dir(relative), page.DirectoryIdentity, page.DirectoryRevision); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_directories SET identity=?,revision=?,cursor=?,state=?,pages=pages+1 WHERE job_id=? AND relative_path=? AND cursor=?`, page.DirectoryIdentity, page.DirectoryRevision, page.NextCursor, state, job, relative, cursor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET discovered=discovered+? WHERE job_id=?`, discovered, job); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET processed=processed+? WHERE id=?`, discovered, job); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE library_sources SET last_progress_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), source.ID); err != nil {
		return err
	}
	return gated.Commit()
}
func (s *Service) observeInventoryTx(ctx context.Context, tx *sql.Tx, job string, source LibrarySource, relative string, v storage.Snapshot, reason string, policy ScanPolicy) (bool, error) {
	if source.Kind == "remote" {
		if s.storage == nil || !s.storage.IsRemote(v.Path) {
			return false, storage.ErrRemoteConfig
		}
		if err := s.storage.Remote.RecordReference(ctx, tx, v.Path, v.Revision); err != nil {
			return false, err
		}
	}
	revision, evidence, err := observationRevision(source, v)
	if err != nil {
		return false, err
	}
	var object, asset, previousRevision, previousSeen, previousIdentity, incarnation, previousState string
	relocated := false
	err = tx.QueryRowContext(ctx, `SELECT id,asset_id,revision,seen_job,object_identity,root_incarnation,state FROM inventory_objects WHERE source_id=? AND relative_path=? AND retired=0`, source.ID, relative).Scan(&object, &asset, &previousRevision, &previousSeen, &previousIdentity, &incarnation, &previousState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	replaced := object != "" && (incarnation != source.Incarnation || previousIdentity != "" && v.ObjectIdentity != "" && previousIdentity != v.ObjectIdentity)
	if replaced {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET retired=1,state='unavailable' WHERE id=?`, object); err != nil {
			return false, err
		}
		if err = inventoryEvent(ctx, tx, source.ID, job, object, asset, previousRevision, "replaced"); err != nil {
			return false, err
		}
		object, asset, previousRevision, previousSeen, previousState = "", "", "", "", ""
	}
	if object == "" {
		// Exact storage identity plus unchanged size/mtime, never filename similarity.
		// LIMIT 2 deliberately refuses ambiguous identity candidates.
		if v.ObjectIdentity != "" {
			rows, e := tx.QueryContext(ctx, `SELECT DISTINCT o.asset_id FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id WHERE o.object_identity=? AND o.size=? AND o.modified_ns=? AND s.kind=? AND o.root_incarnation=s.incarnation LIMIT 2`, v.ObjectIdentity, v.Size, v.ModifiedNS, source.Kind)
			if e != nil {
				return false, e
			}
			ids := []string{}
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return false, e
				}
				ids = append(ids, id)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return false, e
			}
			if len(ids) == 1 {
				asset = ids[0]
			}
		}
		if asset == "" {
			// Adopt legacy assets only once and only with the retained observation. A
			// migrated pathname is not evidence for a future replacement or rename.
			_ = tx.QueryRowContext(ctx, `SELECT a.token FROM catalog_assets a WHERE a.path=? AND a.size=? AND a.modified_ns=? AND NOT EXISTS(SELECT 1 FROM inventory_objects o WHERE o.asset_id=a.token)`, v.Path, v.Size, v.ModifiedNS).Scan(&asset)
		}
		// A single known location of this exact physical object can move without
		// replacing its location ID. Multiple hardlink aliases stay separate.
		if asset != "" && v.ObjectIdentity != "" {
			var oldID, oldRev, oldSeen, oldState string
			e := tx.QueryRowContext(ctx, `SELECT id,revision,seen_job,state FROM inventory_objects WHERE source_id=? AND root_incarnation=? AND asset_id=? AND object_identity=? AND retired=0 AND seen_job!=? AND (SELECT count(*) FROM inventory_objects x WHERE x.source_id=? AND x.asset_id=? AND x.retired=0)=1 LIMIT 1`, source.ID, source.Incarnation, asset, v.ObjectIdentity, job, source.ID, asset).Scan(&oldID, &oldRev, &oldSeen, &oldState)
			if e == nil {
				object, previousRevision, previousSeen, previousState = oldID, oldRev, oldSeen, oldState
				relocated = true
			} else if !errors.Is(e, sql.ErrNoRows) {
				return false, e
			}
		}
		if object == "" {
			object = identity.Token()
		}
	}
	newAsset := asset == ""
	if newAsset {
		asset = identity.Token()
	}
	// The baseline asset path is a locator projection. Preserve the old asset ID
	// when that pathname is replaced, but never silently point it at new bytes.
	var collision string
	var collisionID int64
	err = tx.QueryRowContext(ctx, `SELECT id,token FROM catalog_assets WHERE path=? AND token!=?`, v.Path, asset).Scan(&collisionID, &collision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if collision != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET state='unavailable' WHERE asset_id=? AND source_id=? AND relative_path=?`, collision, source.ID, relative); err != nil {
			return false, err
		}
		var available bool
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT available FROM inventory_asset_availability WHERE asset_id=?),0)`, collision).Scan(&available); err != nil {
			return false, err
		}
		if err = compactcatalog.SetAssetTx(ctx, tx, collisionID, map[string]any{"path": "unavailable:" + collision, "available": available}); err != nil {
			return false, err
		}
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(v.Path)), ".")
	if newAsset {
		_, err = compactcatalog.CreateAssetTx(ctx, tx, asset, compactcatalog.Asset{Path: v.Path, Size: v.Size, ModifiedNS: v.ModifiedNS, Container: ext}, reason == "")
	} else {
		var size, modified, assetID int64
		if err = tx.QueryRowContext(ctx, `SELECT id,size,modified_ns FROM catalog_assets WHERE token=?`, asset).Scan(&assetID, &size, &modified); err != nil {
			return false, err
		}
		changed := size != v.Size || modified != v.ModifiedNS || previousRevision != "" && previousRevision != revision
		if changed {
			if err = invalidateInventoryFacts(ctx, tx, asset); err != nil {
				return false, err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_revision='',analysis_policy=0,analysis_operations_json='[]',analysis_state='pending' WHERE asset_id=?`, asset); err != nil {
				return false, err
			}
		}
		// Do not replace verified format facts with a filename extension on an
		// unchanged scan. Avoid no-op asset writes/revisions for every observation.
		values := map[string]any{"path": v.Path, "size": v.Size, "modified_ns": v.ModifiedNS, "available": reason == ""}
		if changed {
			values["container"] = ext
		}
		err = compactcatalog.SetAssetTx(ctx, tx, assetID, values)
	}
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,object_identity,revision,evidence_json,size,modified_ns,seen_job,unsupported_reason,directory_path) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET directory_path=excluded.directory_path,relative_path=excluded.relative_path,revision=excluded.revision,evidence_json=excluded.evidence_json,size=excluded.size,modified_ns=excluded.modified_ns,object_identity=excluded.object_identity,seen_job=excluded.seen_job,state='available',missing_since='',missing_observations=0,last_missing_job='',trashed_at='',unsupported_reason=excluded.unsupported_reason`, object, source.ID, asset, source.Incarnation, relative, v.ObjectIdentity, revision, evidence, v.Size, v.ModifiedNS, job, reason, filepath.Dir(relative))
	if err != nil {
		return false, err
	}
	var associated int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_assets a JOIN catalog_asset_links l INDEXED BY catalog_asset_links_asset ON l.asset_id=a.id JOIN catalog_entities i ON i.id=l.entity_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE a.token=? AND cl.library_id=?)`, asset, source.LibraryID).Scan(&associated); err != nil {
		return false, err
	}
	if associated == 0 {
		if err = s.associateInventoryTx(ctx, tx, source, asset, v.Path); err != nil {
			return false, err
		}
	}
	kind := ""
	if previousRevision == "" {
		kind = "created"
	} else if previousRevision != revision {
		kind = "revision_changed"
	} else if previousState != "available" {
		kind = "reappeared"
	} else if relocated {
		kind = "relocated"
	}
	if kind != "" {
		if err = inventoryEvent(ctx, tx, source.ID, job, object, asset, revision, kind); err != nil {
			return false, err
		}
	}
	if reason != "" && previousSeen != job {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET warnings=warnings+1 WHERE job_id=?`, job); err != nil {
			return false, err
		}
	}
	// A revision/policy-specific queue is separate from inventory visibility.
	if policy.Tier != "file_list_only" && reason == "" && ext != "strm" {
		_, err = tx.ExecContext(ctx, `INSERT INTO inventory_analysis_queue(job_id,object_id,revision,policy_revision) SELECT ?,id,revision,? FROM inventory_objects WHERE id=? AND (analysis_revision!=revision OR analysis_policy!=?) ON CONFLICT(job_id,object_id) DO UPDATE SET revision=excluded.revision,policy_revision=excluded.policy_revision,state=CASE WHEN inventory_analysis_queue.revision=excluded.revision AND inventory_analysis_queue.policy_revision=excluded.policy_revision THEN inventory_analysis_queue.state ELSE 'pending' END`, job, policy.Revision, object, policy.Revision)
	}
	return previousSeen != job, err
}
func invalidateInventoryFacts(ctx context.Context, tx *sql.Tx, asset string) error {
	for _, table := range []string{"asset_stream_facts", "asset_subtitle_facts", "asset_chapter_facts"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE asset_id=?`, asset); err != nil {
			return err
		}
	}
	id, err := compactcatalog.AssetByTokenTx(ctx, tx, asset)
	if err != nil || id == 0 {
		return err
	}
	return compactcatalog.SetAssetTx(ctx, tx, id, map[string]any{"video_codec": "", "audio_codec": "", "width": 0, "height": 0, "duration": 0})
}
func (s *Service) associateInventoryTx(ctx context.Context, tx *sql.Tx, source LibrarySource, asset, path string) error {
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind); err != nil {
		return err
	}
	// Sidecar extras are published before any library-kind association so an
	// Extras/ folder never becomes a movie, episode, track or book part.
	if extra, ok := ClassifyExtraFor(kind, source.ResolvedPath, path); ok {
		return s.publishExtraTx(ctx, tx, source, asset, path, extra)
	}
	switch kind {
	case "music", "audiobook":
		return s.commitAudio(ctx, tx, source.LibraryID, kind, source.ResolvedPath, asset, path, assets.Facts{InventoryOnly: true})
	case "tv", "anime":
		relative, err := filepath.Rel(source.ResolvedPath, path)
		if err != nil {
			return err
		}
		return s.commitEpisodes(tx, source.LibraryID, asset, filepath.Base(path), ParseEpisode(relative, kind), false)
	default:
		assetID, err := compactcatalog.AssetByTokenTx(ctx, tx, asset)
		if err != nil {
			return err
		}
		_, err = commitMovieItem(ctx, tx, source.LibraryID, assetID, path)
		return err
	}
}

// CommitInventoryAnalysis never stats/reads storage while holding a DB writer.
// The caller revalidates the observed revision after all bounded reads; this
// transaction additionally checks job cancellation, source and current policy.
func (s *Service) CommitInventoryAnalysis(ctx context.Context, job, object, revision string, policyRevision int64, f assets.Facts) error {
	gated2, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	source, err := InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	policy, err := ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	if policy.Revision != policyRevision || !policy.Allows("probe") {
		return context.Canceled
	}
	var asset, relative string
	var size, modified int64
	err = tx.QueryRowContext(ctx, `SELECT asset_id,relative_path,size,modified_ns FROM inventory_objects WHERE id=? AND source_id=? AND revision=? AND retired=0 AND state='available'`, object, source.ID, revision).Scan(&asset, &relative, &size, &modified)
	if err != nil {
		return err
	}
	var kind string
	if err = tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind); err != nil {
		return err
	}
	if f.InventoryOnly {
		return errors.New("technical metadata was not observed")
	}
	if source.Kind == "remote" {
		if s.storage == nil {
			return storage.ErrRemoteConfig
		}
		if err = s.storage.Remote.RecordReference(ctx, tx, filepath.Join(source.ResolvedPath, relative), f.ObservedRevision); err != nil {
			return err
		}
	}
	if (kind == "music" || kind == "audiobook") && f.AudioCodec == "" || kind != "music" && kind != "audiobook" && f.VideoCodec == "" {
		return errors.New("media stream unsupported for library kind")
	}
	var assetID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM catalog_assets WHERE token=? AND size=? AND modified_ns=?`, asset, size, modified).Scan(&assetID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if assetID != 0 {
		if err = compactcatalog.SetAssetTx(ctx, tx, assetID, map[string]any{"container": f.Container, "video_codec": f.VideoCodec, "audio_codec": f.AudioCodec, "width": f.Width, "height": f.Height, "duration": f.Duration}); err != nil {
			return err
		}
	}
	if err = assets.PersistStreams(tx, asset, size, modified, f.Streams); err != nil {
		return err
	}
	if err = assets.PersistChapters(tx, asset, size, modified, f); err != nil {
		return err
	}
	if policy.Allows("subtitles") && f.SubtitleInventory != nil {
		if err = subtitles.Persist(tx, asset, size, modified, f.SubtitleInventory); err != nil {
			return err
		}
	}
	// Bind local evidence to the same revision already authorized by this commit.
	f.ObservedRevision = revision
	var manuallyLinked int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inventory_version_links WHERE library_id=? AND asset_id=?)`, source.LibraryID, asset).Scan(&manuallyLinked); err != nil {
		return err
	}
	if manuallyLinked != 0 && (kind == "music" || kind == "audiobook") {
		if err = persistAudioEvidence(tx, source.LibraryID, asset, f); err != nil {
			return err
		}
		// A manually linked asset's selected tags are effective facts too: the
		// linked commit path skips commitAudio, so the genre projection runs
		// here on the same shared implementation, keeping the item's facets in
		// step with its selected evidence.
		var linkedItem int64
		if e := tx.QueryRowContext(ctx, `SELECT i.id FROM catalog_assets a JOIN catalog_asset_links l INDEXED BY catalog_asset_links_asset ON l.asset_id=a.id
 JOIN catalog_entities i ON i.id=l.entity_id JOIN catalog_kinds k ON k.id=i.kind AND k.playable=1 AND k.listening=1 JOIN catalog_libraries cl ON cl.id=i.library_id
 WHERE a.token=? AND cl.library_id=? LIMIT 1`, asset, source.LibraryID).Scan(&linkedItem); e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if linkedItem != 0 {
			if err = compactcatalog.ProjectAudioLocalGenresItem(ctx, tx, source.LibraryID, linkedItem); err != nil {
				return err
			}
		}
	}
	if manuallyLinked == 0 && (kind == "music" || kind == "audiobook") {
		if err = s.commitAudio(ctx, tx, source.LibraryID, kind, source.ResolvedPath, asset, filepath.Join(source.ResolvedPath, relative), f); err != nil {
			return err
		}
	}
	if policy.Allows("local_metadata") && (kind == "movie" || kind == "tv" || kind == "anime") {
		if err = localmetadata.PersistVideo(ctx, tx, source.LibraryID, asset, f); err != nil {
			return err
		}
	}
	if policy.Allows("local_metadata") && kind == "movie" {
		if f.ArtworkKey != "" {
			items, err := scanInt64s(tx.QueryContext(ctx, `SELECT l.entity_id FROM catalog_assets a JOIN catalog_asset_links l INDEXED BY catalog_asset_links_asset ON l.asset_id=a.id
 JOIN catalog_entities i ON i.id=l.entity_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE a.token=? AND cl.library_id=?`, asset, source.LibraryID))
			if err != nil {
				return err
			}
			for _, item := range items {
				if err = compactcatalog.SetFieldsTx(ctx, tx, item, compactcatalog.Automatic, map[string]any{"poster_url": "local:" + f.ArtworkKey}); err != nil {
					return err
				}
			}
		}
	}
	if policy.Allows("local_metadata") && len(f.ArtworkExtra) > 0 && (kind == "movie" || kind == "tv" || kind == "anime") {
		if err = s.commitAssetSidecars(ctx, tx, source.LibraryID, kind, asset, f.ArtworkExtra); err != nil {
			return err
		}
	}
	if f.Container == "strm" {
		if f.AnalysisSourceEvidence == "" {
			return errors.New("STRM analysis target evidence is missing")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO analysis_probe_bindings(object_id,source_revision,evidence) VALUES(?,?,?) ON CONFLICT(object_id) DO UPDATE SET source_revision=excluded.source_revision,evidence=excluded.evidence`, object, revision, f.AnalysisSourceEvidence); err != nil {
			return err
		}
	}
	queueState := "done"
	if f.SubtitleTextPending {
		queueState = "pending"
	}
	for _, op := range DeepScanOperations {
		if policy.Allows(op) {
			queueState = "pending"
		}
	}
	analysisState := "complete"
	if queueState == "pending" {
		analysisState = "pending"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_revision=?,analysis_policy=?,analysis_operations_json=?,analysis_state=?,analysis_error='' WHERE id=?`, func() string {
		if f.SubtitleTextPending {
			return ""
		}
		return revision
	}(), policyRevision, actualScanOperations(policy, f.AnalysisOperations), analysisState, object); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_analysis_queue SET state=? WHERE job_id=? AND object_id=? AND revision=? AND policy_revision=?`, queueState, job, object, revision, policyRevision); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_runs SET analyzed=analyzed+1 WHERE job_id=?`, job); err != nil {
		return err
	}
	if err = inventoryEvent(ctx, tx, source.ID, job, object, asset, revision, "analysis_completed"); err != nil {
		return err
	}
	return gated2.Commit()
}

func actualScanOperations(p ScanPolicy, actual []string) string {
	if actual != nil {
		b, _ := json.Marshal(actual)
		return string(b)
	}
	// Compatibility for callers that supply a complete local Basic result. The
	// durable scanner always passes exact successfully executed operations.
	return completedScanOperations(p)
}
func completedScanOperations(p ScanPolicy) string {
	ops := []string{}
	for _, v := range BasicScanOperations {
		if p.Allows(v) {
			ops = append(ops, v)
		}
	}
	b, _ := json.Marshal(ops)
	return string(b)
}

// AdoptLegacyInventoryTx records path-only baseline evidence without claiming a
// fresh observation. It runs in bounded pages, never as a startup-wide rewrite.
// Two complete inventories and the source grace still govern missing status.
func (s *Service) AdoptLegacyInventoryTx(ctx context.Context, tx *sql.Tx, source LibrarySource, asset, relative string, v storage.Snapshot) error {
	revision, evidence, err := observationRevision(source, v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,state) VALUES(?,?,?,?,?,?,?,?,?,CASE WHEN (SELECT available FROM catalog_assets WHERE token=?)=1 THEN 'available' ELSE 'unavailable' END)`, identity.Token(), source.ID, asset, source.Incarnation, relative, revision, evidence, v.Size, v.ModifiedNS, asset)
	return err
}
