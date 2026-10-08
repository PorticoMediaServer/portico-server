package metadata

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
)

func (s *Service) mbReconcileAlbums(ctx context.Context) (bool, error) {
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return false, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var albumID int64
	var release, cursor string
	var membership, currentMembership int64
	e = tx.QueryRowContext(ctx, `SELECT c.album_id,c.release_revision,c.membership_revision,c.cursor,h.membership_revision FROM mb_album_reconciliations c JOIN mb_album_heads h ON h.album_id=c.album_id JOIN mb_album_links l ON l.album_id=c.album_id AND l.release_revision=c.release_revision WHERE (c.status='pending' OR c.membership_revision<>h.membership_revision) ORDER BY c.album_id LIMIT 1`).Scan(&albumID, &release, &membership, &cursor, &currentMembership)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if membership != currentMembership {
		cursor = ""
		membership = currentMembership
	}
	// The cursor is opaque; it now holds an integer entity id as text.
	var cursorID int64
	if cursor != "" {
		cursorID, _ = strconv.ParseInt(cursor, 10, 64)
	}
	rows, e := tx.QueryContext(ctx, `SELECT cs.entity_id FROM catalog_songs cs WHERE cs.album_id=? AND cs.entity_id>? ORDER BY cs.entity_id LIMIT 101`, albumID, cursorID)
	if e != nil {
		return false, e
	}
	items := []int64{}
	for rows.Next() {
		var item int64
		if e = rows.Scan(&item); e != nil {
			rows.Close()
			return false, e
		}
		items = append(items, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	complete := len(items) <= 100
	if len(items) > 100 {
		items = items[:100]
	}
	for _, item := range items {
		if _, e = tx.ExecContext(ctx, `UPDATE mb_jobs SET status='pending',attempts=0,next_attempt='',error='',generation=generation+1,revision=revision+1,lease='',lease_until='' WHERE kind='song' AND entity_id=?`, item); e != nil {
			return false, e
		}
		cursor = strconv.FormatInt(item, 10)
	}
	status := "pending"
	if complete {
		status = "complete"
	}
	if _, e = tx.ExecContext(ctx, `UPDATE mb_album_reconciliations SET membership_revision=?,cursor=?,status=?,revision=revision+1 WHERE album_id=? AND release_revision=?`, membership, cursor, status, albumID, release); e != nil {
		return false, e
	}
	return true, gated.Commit()
}
func (s *Service) cleanupMusicBrainz(ctx context.Context) error {
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	now := s.publicationTime().Format(time.RFC3339)
	cutoff := s.publicationTime().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	// Expired leases no longer confer publication authority. Terminal compact
	// receipts and expired orphans are removed in bounded, deterministic batches.
	if _, e = tx.ExecContext(ctx, `DELETE FROM mb_publication_operations WHERE id IN(SELECT o.id FROM mb_publication_operations o WHERE (o.status IN('applied','stale','failed') AND (o.finished_at<? OR o.id IN(SELECT id FROM mb_publication_operations WHERE status IN('applied','stale','failed') ORDER BY created_at DESC,id DESC LIMIT 128 OFFSET 4000))) OR (o.status IN('claimed','staged') AND o.lease_until<=? AND NOT EXISTS(SELECT 1 FROM mb_jobs j WHERE j.kind=o.kind AND j.entity_id=o.entity_id AND j.generation=o.job_generation AND j.lease=o.lease AND j.lease_until>?)) ORDER BY o.created_at,o.id LIMIT 128)`, cutoff, now, now); e != nil {
		return e
	}
	for _, kind := range []string{"recording", "release"} {
		if _, e = tx.ExecContext(ctx, `DELETE FROM mb_`+kind+`_aliases WHERE rowid IN(SELECT rowid FROM mb_`+kind+`_aliases WHERE observed_at<? ORDER BY observed_at,requested_id,evidence_revision LIMIT 100)`, cutoff); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM mb_release_evidence WHERE revision_id IN(SELECT r.revision_id FROM mb_release_evidence r WHERE r.observed_at<? AND NOT EXISTS(SELECT 1 FROM mb_album_links l WHERE l.release_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_song_links l WHERE l.release_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_release_aliases a WHERE a.evidence_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_publication_operations o WHERE o.accepted_release_revision=r.revision_id OR o.result_release_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_album_reconciliations c WHERE c.release_revision=r.revision_id) ORDER BY r.observed_at,r.revision_id LIMIT 100)`, cutoff); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM mb_recording_evidence WHERE revision_id IN(SELECT r.revision_id FROM mb_recording_evidence r WHERE r.observed_at<? AND NOT EXISTS(SELECT 1 FROM mb_song_links l WHERE l.recording_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_release_tracks t WHERE t.recording_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_recording_aliases a WHERE a.evidence_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_publication_operations o WHERE o.accepted_recording_revision=r.revision_id OR o.result_recording_revision=r.revision_id) ORDER BY r.observed_at,r.revision_id LIMIT 100)`, cutoff); e != nil {
		return e
	}
	return gated2.Commit()
}
