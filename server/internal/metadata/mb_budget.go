package metadata

import (
	"context"
	"database/sql"
	"errors"
)

var errMBBudget = errors.New("MusicBrainz evidence retention budget exhausted")

// Admission charges payload bytes conservatively plus every normalized row.
// Accepted catalog evidence is protected authority, outside this cache budget.
// Aliases and live stages remain in the budget and cannot be evicted to admit
// another request. This check runs after construction in the same transaction;
// refusal rolls back the entire prospective revision, including its children.
func checkMBEvidenceBudget(ctx context.Context, tx *sql.Tx) error {
	var releases, recordings, charge int64
	e := tx.QueryRowContext(ctx, `WITH unaccepted_releases AS (
 SELECT r.revision_id,length(CAST(r.payload AS BLOB))*8+512*(1+
 (SELECT count(*) FROM mb_release_credits c WHERE c.revision_id=r.revision_id)+
 (SELECT count(*) FROM mb_release_group_types c WHERE c.release_revision=r.revision_id)+
 (SELECT count(*) FROM mb_release_media c WHERE c.release_revision=r.revision_id)+
 (SELECT count(*) FROM mb_release_tracks c WHERE c.release_revision=r.revision_id)+
 (SELECT count(*) FROM mb_track_credits c WHERE c.release_revision=r.revision_id)) AS charge
 FROM mb_release_evidence r WHERE NOT EXISTS(SELECT 1 FROM mb_album_links l WHERE l.release_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_song_links l WHERE l.release_revision=r.revision_id)
 ),unaccepted_recordings AS (
 SELECT r.revision_id,length(CAST(r.payload AS BLOB))*8+512*(1+(SELECT count(*) FROM mb_recording_credits c WHERE c.revision_id=r.revision_id)) AS charge
 FROM mb_recording_evidence r WHERE NOT EXISTS(SELECT 1 FROM mb_song_links l WHERE l.recording_revision=r.revision_id)
 AND NOT EXISTS(SELECT 1 FROM mb_release_tracks t WHERE t.recording_revision=r.revision_id AND (EXISTS(SELECT 1 FROM mb_album_links l WHERE l.release_revision=t.release_revision) OR EXISTS(SELECT 1 FROM mb_song_links l WHERE l.release_revision=t.release_revision)))
 ) SELECT (SELECT count(*) FROM unaccepted_releases),(SELECT count(*) FROM unaccepted_recordings),COALESCE((SELECT sum(charge) FROM unaccepted_releases),0)+COALESCE((SELECT sum(charge) FROM unaccepted_recordings),0)`).Scan(&releases, &recordings, &charge)
	if e != nil {
		return e
	}
	if releases > 512 || recordings > 20000 || charge > 256<<20 {
		return errMBBudget
	}
	return nil
}
