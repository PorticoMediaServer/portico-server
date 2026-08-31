package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// mediaMergePolicies is intentionally a closed inventory. A schema change which
// adds a media_items foreign key must make an explicit merge decision here.
// "union" keeps all non-conflicting rows and deterministically keeps the target
// row when the table's unique key makes two rows equivalent. "retarget" is for
// tables whose key is independent of media_id.
var mediaMergePolicies = map[string]string{
	"audio_fingerprints": "union", "audio_normalization": "union",
	"audiobook_browse_entity_members": "union", "download_preparations": "retarget",
	"dvr_recording_media": "union", "library_channel_schedule_entries": "retarget",
	"media_access_labels": "union", "media_access_tags": "union", "media_attachments": "union",
	"media_analysis_facts": "union", "media_segment_analysis_runs": "union",
	"media_availability": "union", "media_category_facets": "union", "media_chapters": "retarget",
	"media_download_grants": "retarget", "media_files": "retarget", "media_identity_evidence": "union",
	"media_images": "retarget", "media_lyrics": "union", "media_match_candidates": "retarget",
	"media_metadata_locks": "union", "media_people": "union", "media_provider_ids": "provider_identity",
	"media_metadata_revisions": "revision", "media_provider_snapshots": "retarget",
	"media_metadata_field_values": "retarget", "media_metadata_relationships": "retarget",
	"media_metadata_refresh_outcomes": "retarget", "media_rating_evidence": "union",
	"media_scanner_hints": "union", "media_scanner_identity_aliases": "retarget",
	"media_segments": "union", "media_streams": "retarget", "media_trickplay_sets": "union",
	"media_waveform_artifacts": "union",
	"metadata_health_issues":   "retarget", "optimized_versions": "retarget", "playlist_items": "retarget",
	"scanner_backlog": "union", "user_media_state": "viewer_state", "user_recommendation_cache": "union",
}

// Logical references intentionally do not declare a foreign key because they
// are audit/history, active playback, FTS, or cross-device coordination rows.
// They must still follow the canonical identity.
var mediaLogicalReferencePolicies = map[string]string{
	"dashboard_playback_rollups": "retarget", "media_search": "retarget",
	"playback_prepared_handoffs": "retarget", "playback_session_history": "union",
	"playback_session_queue": "union", "playback_sessions": "retarget",
	"viewer_feedback": "retarget", "watch_with_friends_groups": "retarget",
	"watch_with_friends_queue": "union",
}

func inventoryMediaForeignKeysTx(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		fkRows, err := tx.Query(`PRAGMA foreign_key_list("` + table + `")`)
		if err != nil {
			return nil, err
		}
		found := false
		for fkRows.Next() {
			var id, seq int
			var referenced, from, to, onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &referenced, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				fkRows.Close()
				return nil, err
			}
			if referenced == "media_items" && from == "media_id" && to == "id" {
				found = true
			}
		}
		if err := fkRows.Close(); err != nil {
			return nil, err
		}
		if found {
			tables = append(tables, table)
		}
	}
	return tables, rows.Err()
}

func validateMediaMergePolicyTx(tx *sql.Tx) error {
	tables, err := inventoryMediaForeignKeysTx(tx)
	if err != nil {
		return err
	}
	var unknown, stale []string
	for _, table := range tables {
		if _, ok := mediaMergePolicies[table]; !ok {
			unknown = append(unknown, table)
		}
	}
	seen := make(map[string]bool, len(tables))
	for _, table := range tables {
		seen[table] = true
	}
	for table := range mediaMergePolicies {
		if !seen[table] {
			stale = append(stale, table)
		}
	}
	sort.Strings(unknown)
	sort.Strings(stale)
	if len(unknown) != 0 || len(stale) != 0 {
		return fmt.Errorf("media merge policy/schema mismatch: unknown=%v stale=%v", unknown, stale)
	}
	return nil
}

func applyMediaMergePoliciesTx(tx *sql.Tx, subjectID, targetID string) error {
	if err := validateMediaMergePolicyTx(tx); err != nil {
		return err
	}
	var lockConflicts int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM media_metadata_locks s JOIN media_metadata_locks t ON t.media_id=? AND t.field=s.field WHERE s.media_id=? AND (s.value_json<>t.value_json OR s.lock_kind<>t.lock_kind)`, targetID, subjectID).Scan(&lockConflicts); err != nil {
		return err
	}
	if lockConflicts != 0 {
		return errIdentityReviewUnsafeMerge
	}
	// An accepted identity records an explicit user decision when
	// accepted_by_user_id is populated. Never resolve a conflict involving one
	// of those decisions by ranking the identities or demoting either side to a
	// candidate: doing so would silently discard protected manual metadata.
	// Fully automated conflicts retain the deterministic target-preserving
	// behavior below.
	var protectedIdentityConflicts int
	if err := tx.QueryRow(`SELECT COUNT(*)
		FROM media_provider_ids s
		JOIN media_provider_ids t
		  ON t.media_id=?
		 AND t.provider=s.provider
		 AND t.external_type=s.external_type
		WHERE s.media_id=?
		  AND s.status='accepted'
		  AND t.status='accepted'
		  AND s.external_id<>t.external_id
		  AND (s.accepted_by_user_id<>'' OR t.accepted_by_user_id<>'')`, targetID, subjectID).Scan(&protectedIdentityConflicts); err != nil {
		return err
	}
	if protectedIdentityConflicts != 0 {
		return errIdentityReviewUnsafeMerge
	}
	if _, err := tx.Exec(`INSERT INTO media_provider_ids
		(media_id,provider,external_id,external_type,confidence,source,status,observed_at,provider_updated_at,snapshot_id,evidence_revision,accepted_at,accepted_by_user_id,created_at,updated_at)
		SELECT ?,s.provider,s.external_id,s.external_type,s.confidence,s.source,
			CASE WHEN s.status='accepted' AND EXISTS (
				SELECT 1 FROM media_provider_ids t
				WHERE t.media_id=? AND t.provider=s.provider AND t.external_type=s.external_type
					AND t.status='accepted' AND t.external_id<>s.external_id
			) THEN 'candidate' ELSE s.status END,
			s.observed_at,s.provider_updated_at,s.snapshot_id,s.evidence_revision,s.accepted_at,s.accepted_by_user_id,s.created_at,s.updated_at
		FROM media_provider_ids s WHERE s.media_id=?
		ON CONFLICT(media_id,provider,external_type,external_id) DO UPDATE SET
		status=CASE
			WHEN media_provider_ids.status='accepted' THEN 'accepted'
			WHEN excluded.status='accepted' THEN 'accepted'
			ELSE media_provider_ids.status
		END,
		confidence=MAX(media_provider_ids.confidence,excluded.confidence), updated_at=MAX(media_provider_ids.updated_at,excluded.updated_at)`, targetID, targetID, subjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_provider_ids WHERE media_id=?`, subjectID); err != nil {
		return err
	}
	// At most one identity per provider/type may remain canonical. Prefer the
	// highest-confidence, freshest value and use the external ID as a stable
	// final tie-breaker so replaying the same merge produces the same winner.
	if _, err := tx.Exec(`UPDATE media_provider_ids AS p SET status='candidate', accepted_at='', accepted_by_user_id=''
		WHERE p.media_id=? AND p.status='accepted' AND p.external_id<>(
			SELECT winner.external_id FROM media_provider_ids AS winner
			WHERE winner.media_id=p.media_id AND winner.provider=p.provider
				AND winner.external_type=p.external_type AND winner.status='accepted'
			ORDER BY winner.confidence DESC, winner.updated_at DESC, winner.external_id ASC LIMIT 1
		)`, targetID); err != nil {
		return err
	}

	// Viewer state is a true union, not target-wins deduplication.
	if _, err := tx.Exec(`INSERT INTO user_media_state
		(profile_id,user_id,media_id,watchlisted,favorite,liked,watched,progress_seconds,rating,last_played_at,updated_at,progress_session_id,progress_recorded_at)
		SELECT profile_id,user_id,?,watchlisted,favorite,liked,watched,progress_seconds,rating,last_played_at,updated_at,progress_session_id,progress_recorded_at
		FROM user_media_state WHERE media_id=?
		ON CONFLICT(profile_id,media_id) DO UPDATE SET
		watchlisted=MAX(user_media_state.watchlisted,excluded.watchlisted), favorite=MAX(user_media_state.favorite,excluded.favorite),
		liked=MAX(user_media_state.liked,excluded.liked), watched=MAX(user_media_state.watched,excluded.watched),
		progress_seconds=MAX(user_media_state.progress_seconds,excluded.progress_seconds), rating=MAX(user_media_state.rating,excluded.rating),
		last_played_at=MAX(COALESCE(user_media_state.last_played_at,''),COALESCE(excluded.last_played_at,'')),
		updated_at=MAX(user_media_state.updated_at,excluded.updated_at)`, targetID, subjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM user_media_state WHERE media_id=?`, subjectID); err != nil {
		return err
	}

	// Revision numbers are scoped to media. Move the entire subject evidence
	// namespace above the target range before retargeting dependent rows.
	var maxRevision int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(revision),0) FROM media_metadata_revisions WHERE media_id=?`, targetID).Scan(&maxRevision); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE media_metadata_revisions SET revision=revision+?, base_revision=CASE WHEN base_revision=0 THEN 0 ELSE base_revision+? END WHERE media_id=?`, maxRevision, maxRevision, subjectID); err != nil {
		return err
	}
	for _, tableColumn := range [][2]string{
		{"media_category_facets", "metadata_revision"}, {"media_images", "metadata_revision"},
		{"media_metadata_locks", "metadata_revision"}, {"media_people", "metadata_revision"},
	} {
		if _, err := tx.Exec(`UPDATE "`+tableColumn[0]+`" SET "`+tableColumn[1]+`"="`+tableColumn[1]+`"+? WHERE media_id=? AND "`+tableColumn[1]+`">0`, maxRevision, subjectID); err != nil {
			return fmt.Errorf("offset merge evidence %s: %w", tableColumn[0], err)
		}
	}

	tables := make([]string, 0, len(mediaMergePolicies))
	for table := range mediaMergePolicies {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		mode := mediaMergePolicies[table]
		if mode == "viewer_state" || mode == "provider_identity" || table == "media_category_facets" {
			continue
		}
		if table == "download_preparations" {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := tx.Exec(`UPDATE download_preparations
				SET media_id=?, state='unavailable', progress=100, media_version_id='', version_fingerprint='',
					artifact_sha256='', size_bytes=0, size_kind='unknown', artifact_expires_at='', job_id='',
					error_code='media_identity_changed', updated_at=?,
					removed_at=CASE WHEN EXISTS (
						SELECT 1 FROM download_preparations AS existing
						WHERE existing.server_id=download_preparations.server_id
							AND existing.account_id=download_preparations.account_id
							AND existing.profile_id=download_preparations.profile_id
							AND existing.media_id=?
							AND existing.quality_profile=download_preparations.quality_profile
							AND existing.removed_at='' AND existing.id<>download_preparations.id
					) THEN ? ELSE removed_at END
				WHERE media_id=?`, targetID, now, targetID, now, subjectID); err != nil {
				return fmt.Errorf("invalidate merged download preparations: %w", err)
			}
			continue
		}
		query := `UPDATE "` + table + `" SET media_id=? WHERE media_id=?`
		if mode == "union" {
			query = `UPDATE OR IGNORE "` + table + `" SET media_id=? WHERE media_id=?`
		}
		if _, err := tx.Exec(query, targetID, subjectID); err != nil {
			return fmt.Errorf("merge media reference %s: %w", table, err)
		}
		if mode == "union" {
			if _, err := tx.Exec(`DELETE FROM "`+table+`" WHERE media_id=?`, subjectID); err != nil {
				return fmt.Errorf("dedupe media reference %s: %w", table, err)
			}
		}
	}
	logicalTables := make([]string, 0, len(mediaLogicalReferencePolicies))
	for table := range mediaLogicalReferencePolicies {
		logicalTables = append(logicalTables, table)
	}
	sort.Strings(logicalTables)
	for _, table := range logicalTables {
		mode := mediaLogicalReferencePolicies[table]
		if table == "media_search" {
			continue
		}
		query := `UPDATE "` + table + `" SET media_id=? WHERE media_id=?`
		if mode == "union" {
			query = `UPDATE OR IGNORE "` + table + `" SET media_id=? WHERE media_id=?`
		}
		if _, err := tx.Exec(query, targetID, subjectID); err != nil {
			return fmt.Errorf("merge logical media reference %s: %w", table, err)
		}
		if mode == "union" {
			if _, err := tx.Exec(`DELETE FROM "`+table+`" WHERE media_id=?`, subjectID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(`UPDATE library_category_counts SET representative_media_id=? WHERE representative_media_id=?`, targetID, subjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE playback_prepared_handoffs
		SET queue_entries_json = (
			SELECT json_group_array(json(CASE
				WHEN json_extract(value, '$.mediaId') = ? THEN json_set(value, '$.mediaId', ?)
				ELSE value
			END))
			FROM json_each(queue_entries_json)
		)
		WHERE EXISTS (
			SELECT 1 FROM json_each(queue_entries_json)
			WHERE json_extract(value, '$.mediaId') = ?
		)`, subjectID, targetID, subjectID); err != nil {
		return err
	}

	var baseRevision int
	if err := tx.QueryRow(`SELECT metadata_revision FROM media_items WHERE id=?`, targetID).Scan(&baseRevision); err != nil {
		return err
	}
	var mergeRevision int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(revision),0)+1 FROM media_metadata_revisions WHERE media_id=?`, targetID).Scan(&mergeRevision); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mergeRevisionID := randomID("mrev")
	if _, err := tx.Exec(`INSERT INTO media_metadata_revisions
		(id,media_id,revision,base_revision,state,trigger_kind,provider,started_at,completed_at,detail)
		VALUES (?,?,?,?,'applied','system','identity-merge',?,?,?)`, mergeRevisionID, targetID, mergeRevision, baseRevision, now, now, "Canonical identity merge from "+subjectID); err != nil {
		return fmt.Errorf("record identity merge revision: %w", err)
	}
	// The merge revision is a complete projection snapshot. Target evidence wins
	// exact-key conflicts; non-conflicting subject evidence is then unioned.
	if err := carryForwardMetadataEvidenceTx(tx, targetID, maxRevision, mergeRevisionID, mergeRevision, now); err != nil {
		return err
	}
	if subjectHistoryHead := mergeRevision - 1; subjectHistoryHead > maxRevision {
		if err := carryForwardMetadataEvidenceTx(tx, targetID, subjectHistoryHead, mergeRevisionID, mergeRevision, now); err != nil {
			return err
		}
	}
	state, err := loadMetadataCanonicalStateTx(tx, targetID)
	if err != nil {
		return err
	}
	state.Revision = mergeRevision
	state.ETag, err = metadataCanonicalETag(state)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE media_items SET metadata_revision=?, metadata_etag=? WHERE id=?`, mergeRevision, state.ETag, targetID); err != nil {
		return err
	}
	// These are current-state evidence projections, rather than immutable
	// revision children. Stamp their merged target state coherently.
	for _, tableColumn := range [][2]string{
		{"media_images", "metadata_revision"}, {"media_metadata_locks", "metadata_revision"},
		{"media_people", "metadata_revision"}, {"media_provider_ids", "evidence_revision"},
	} {
		if _, err := tx.Exec(`UPDATE "`+tableColumn[0]+`" SET "`+tableColumn[1]+`"=? WHERE media_id=?`, mergeRevision, targetID); err != nil {
			return fmt.Errorf("stamp merge evidence %s: %w", tableColumn[0], err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM media_category_facets WHERE media_id IN (?,?)`, subjectID, targetID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM media_search WHERE media_id IN (?,?)`, subjectID, targetID); err != nil {
		return err
	}
	if err := replaceMediaCategoryFacetsTx(context.Background(), tx, targetID, mergeRevision, "identity-merge"); err != nil {
		return err
	}
	if err := replaceMediaSearchTx(context.Background(), tx, targetID, mergeRevision, "identity-merge"); err != nil {
		return err
	}
	return nil
}
