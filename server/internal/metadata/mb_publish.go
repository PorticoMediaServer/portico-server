package metadata

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/metadataprovider"
)

// resolveMBEntity maps a job's public id to its integer entity id. The worker
// holds a lease on live catalogue state; a vanished entity reads as a stale
// base, which the worker already releases cleanly.
func resolveMBEntity(ctx context.Context, tx *sql.Tx, public string) (int64, error) {
	id, err := entityid.Resolve(ctx, tx, public)
	if err != nil {
		if errors.Is(err, entityid.ErrNotFound) {
			return 0, errTVDBStale
		}
		return 0, err
	}
	return id, nil
}

func mbIdentityAllowed(ctx context.Context, tx *sql.Tx, j mbJob, canonical string) error {
	current, revision := j.base.RecordingID, j.base.RecordingRevision
	if j.kind == "album" {
		current, revision = j.base.ReleaseID, j.base.ReleaseRevision
	}
	if canonical == current {
		return nil
	}
	if !j.manual {
		if current == "" {
			return nil
		}
		return ErrMBConflict
	}
	var n int
	entity, err := resolveMBEntity(ctx, tx, j.id)
	if err != nil {
		return err
	}
	e := tx.QueryRowContext(ctx, `SELECT count(*) FROM mb_selection_receipts WHERE kind=? AND entity_id=? AND job_generation<=? AND candidate_id=? AND prior_revision_id=? AND status='pending' AND actor_authority<>'' AND actor_account_id<>'' AND actor_profile_id<>''`, j.kind, entity, j.generation, j.selected, revision).Scan(&n)
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrMBConflict
	}
	return nil
}
func (s *Service) mbPublishSong(ctx context.Context, j mbJob, result metadataprovider.RecordingLookup, observed string) error {
	stage, e := stageMBRecording(result)
	if e != nil {
		return errPublicationInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx := gated.Tx()
	if e = s.activeMB(ctx, tx, j); e != nil {
		gated.Rollback()
		return e
	}
	revision, e := saveMBRecordingEvidence(ctx, tx, stage, observed)
	if e != nil {
		gated.Rollback()
		return e
	}
	if e = checkMBEvidenceBudget(ctx, tx); e != nil {
		gated.Rollback()
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE mb_publication_operations SET result_recording_revision=?,result_digest=?,status='staged' WHERE id=?`, revision, stage.digest, j.operation); e != nil {
		gated.Rollback()
		return e
	}
	if e = s.commitMBCurrent(ctx, gated, j); e != nil {
		return e
	}
	var gated2 *dbwork.Write
	gated2, e = dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx = gated2.Tx()
	defer gated2.Rollback()
	if e = s.activeMB(ctx, tx, j); e != nil {
		return e
	}
	r, digest, e := loadMBRecordingEvidence(ctx, tx, revision, false)
	if e != nil || digest != stage.digest {
		if e != nil {
			return e
		}
		return errPublicationInput
	}
	if e = mbIdentityAllowed(ctx, tx, j, r.ID); e != nil {
		gated2.Rollback()
		if errors.Is(e, ErrMBConflict) {
			return s.mbStatus(ctx, j, "needs_selection", "accepted_identity_conflict")
		}
		return e
	}
	track, e := readMBTrack(ctx, tx, j, r.ID)
	if e != nil {
		return e
	}
	entity, e := resolveMBEntity(ctx, tx, j.id)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO mb_recording_aliases(requested_id,canonical_id,evidence_revision,observed_at) VALUES(?,?,?,?) ON CONFLICT(requested_id,evidence_revision) DO UPDATE SET observed_at=excluded.observed_at`, stage.requested, r.ID, revision, observed); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO mb_song_links(item_id,recording_revision,recording_id,observed_at) VALUES(?,?,?,?) ON CONFLICT(item_id) DO UPDATE SET recording_revision=excluded.recording_revision,recording_id=excluded.recording_id,observed_at=excluded.observed_at`, entity, revision, r.ID, observed); e != nil {
		return e
	}
	// The song's release columns are catalogue facts: write them through the
	// catalogue API, mirroring the link row above (matched links carry the
	// release identity; anything else carries the status only).
	fields := map[string]any{"recording_id": r.ID, "release_status": track.status, "provider_observed_at": observed, "provider_title": r.Title, "provider_artist": mbArtist(r.ArtistCredit)}
	if track.status == "matched" {
		if _, e = tx.ExecContext(ctx, `UPDATE mb_song_links SET release_revision=?,release_id=?,track_id=?,release_status='matched' WHERE item_id=?`, track.releaseRevision, track.releaseID, track.trackID, entity); e != nil {
			return e
		}
		var group string
		if e = tx.QueryRowContext(ctx, `SELECT release_group_id FROM mb_release_evidence WHERE revision_id=?`, track.releaseRevision).Scan(&group); e != nil {
			return e
		}
		fields["release_id"], fields["track_id"], fields["release_group_id"] = track.releaseID, track.trackID, group
	} else if _, e = tx.ExecContext(ctx, `UPDATE mb_song_links SET release_status=? WHERE item_id=?`, track.status, entity); e != nil {
		return e
	}
	if e = compactcatalog.SetFieldsTx(ctx, tx, entity, compactcatalog.Automatic, fields); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO metadata_details VALUES(?,'musicbrainz',?,?,?) ON CONFLICT(item_id,provider) DO UPDATE SET provider_id=excluded.provider_id,source_url=excluded.source_url,observed_at=excluded.observed_at`, entity, r.ID, "https://musicbrainz.org/recording/"+r.ID, observed); e != nil {
		return e
	}
	if e = s.finishMBPublication(ctx, tx, j, stage.requested, revision); e != nil {
		return e
	}
	// Switching the pin can turn the former accepted closure into cache.
	// Check that prospective final state before committing either pin/receipt.
	if e = checkMBEvidenceBudget(ctx, tx); e != nil {
		return e
	}
	return s.commitMBCurrent(ctx, gated2, j)
}
func (s *Service) mbPublishAlbum(ctx context.Context, j mbJob, result metadataprovider.ReleaseLookup, observed string) error {
	stage, e := stageMBRelease(result)
	if e != nil {
		return errPublicationInput
	}
	gated3, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx := gated3.Tx()
	if e = s.activeMB(ctx, tx, j); e != nil {
		gated3.Rollback()
		return e
	}
	revision, e := saveMBReleaseEvidence(ctx, tx, stage, observed)
	if e != nil {
		gated3.Rollback()
		return e
	}
	if e = checkMBEvidenceBudget(ctx, tx); e != nil {
		gated3.Rollback()
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE mb_publication_operations SET result_release_revision=?,result_digest=?,status='staged' WHERE id=?`, revision, stage.digest, j.operation); e != nil {
		gated3.Rollback()
		return e
	}
	if e = s.commitMBCurrent(ctx, gated3, j); e != nil {
		return e
	}
	var gated4 *dbwork.Write
	gated4, e = dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx = gated4.Tx()
	defer gated4.Rollback()
	if e = s.activeMB(ctx, tx, j); e != nil {
		return e
	}
	r, digest, e := loadMBReleaseEvidence(ctx, tx, revision, false)
	if e != nil || digest != stage.digest {
		if e != nil {
			return e
		}
		return errPublicationInput
	}
	if e = mbIdentityAllowed(ctx, tx, j, r.ID); e != nil {
		gated4.Rollback()
		if errors.Is(e, ErrMBConflict) {
			return s.mbStatus(ctx, j, "needs_selection", "accepted_identity_conflict")
		}
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO mb_release_aliases(requested_id,canonical_id,evidence_revision,observed_at) VALUES(?,?,?,?) ON CONFLICT(requested_id,evidence_revision) DO UPDATE SET observed_at=excluded.observed_at`, stage.requested, r.ID, revision, observed); e != nil {
		return e
	}
	entity, e := resolveMBEntity(ctx, tx, j.id)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO mb_album_links(album_id,release_revision,release_id,requested_id,observed_at) VALUES(?,?,?,?,?) ON CONFLICT(album_id) DO UPDATE SET release_revision=excluded.release_revision,release_id=excluded.release_id,requested_id=excluded.requested_id,observed_at=excluded.observed_at`, entity, revision, r.ID, stage.requested, observed); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO mb_album_reconciliations(album_id,release_revision,membership_revision,status) VALUES(?,?,?,'pending') ON CONFLICT(album_id) DO UPDATE SET release_revision=excluded.release_revision,membership_revision=excluded.membership_revision,cursor='',status='pending',revision=revision+1`, entity, revision, j.base.MembershipRevision); e != nil {
		return e
	}
	// The album with its link, status and label: catalogue facts go through
	// the catalogue API. The release's label fills an empty owner field; a
	// locked or written label stays.
	fields := map[string]any{"provider_match_status": "matched", "release_id": r.ID, "release_group_id": r.ReleaseGroup.ID, "release_date": r.Date, "release_country": r.Country, "provider_observed_at": observed}
	if label := mbReleaseLabel(r); label != "" {
		var current string
		err := tx.QueryRowContext(ctx, `SELECT label FROM catalog_albums WHERE entity_id=?`, entity).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			current = ""
		} else if err != nil {
			return err
		}
		if current == "" {
			fields["label"] = label
		}
	}
	if e = compactcatalog.SetFieldsTx(ctx, tx, entity, compactcatalog.Automatic, fields); e != nil {
		return e
	}
	if e = s.finishMBPublication(ctx, tx, j, stage.requested, revision); e != nil {
		return e
	}
	// Switching the pin can turn the former accepted closure into cache.
	// Check that prospective final state before committing either pin/receipt.
	if e = checkMBEvidenceBudget(ctx, tx); e != nil {
		return e
	}
	return s.commitMBCurrent(ctx, gated4, j)
}
func (s *Service) finishMBPublication(ctx context.Context, tx *sql.Tx, j mbJob, requested, revision string) error {
	entity, e := resolveMBEntity(ctx, tx, j.id)
	if e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE mb_jobs SET status='matched',selected_id=?,attempts=0,error='',revision=revision+1 WHERE kind=? AND entity_id=?`, requested, j.kind, entity); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE mb_candidates SET decision=CASE WHEN provider_id=? THEN 'accepted' ELSE 'superseded' END WHERE kind=? AND entity_id=?`, requested, j.kind, entity); e != nil {
		return e
	}
	if _, e := tx.ExecContext(ctx, `UPDATE mb_selection_receipts SET accepted_revision_id=?,status='applied',finished_at=? WHERE kind=? AND entity_id=? AND job_generation<=? AND candidate_id=? AND status='pending'`, revision, s.publicationTime().Format(publicationClockFormat), j.kind, entity, j.generation, j.selected); e != nil {
		return e
	}
	return finishMBOperation(ctx, tx, j, "applied", "published", s.publicationTime())
}
func trackArtist(t metadataprovider.ReleaseTrack) string {
	if len(t.ArtistCredit) > 0 {
		return mbArtist(t.ArtistCredit)
	}
	return mbArtist(t.Recording.ArtistCredit)
}

// mbReleaseLabel is the release's first named label, for the album's owner
// label field. Empty when the payload carries none.
func mbReleaseLabel(r metadataprovider.Release) string {
	for _, info := range r.LabelInfo {
		if info.Label != nil && strings.TrimSpace(info.Label.Name) != "" {
			return strings.TrimSpace(info.Label.Name)
		}
	}
	return ""
}
