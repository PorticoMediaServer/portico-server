package metadata

import (
	"context"
	"database/sql"
)

// Automatic acquisition always yields to an explicit owner selection, upload,
// generated picture or lock. The preference only orders ordinary automatic art.
func automaticArtworkOccupied(ctx context.Context, tx *sql.Tx, t RepairTarget, entity int64, role, subject, provider, fence string, pending bool) (bool, error) {
	var preferred string
	if err := tx.QueryRowContext(ctx, `SELECT artwork_preference FROM metadata_runtime_settings WHERE singleton=1`).Scan(&preferred); err != nil {
		return false, err
	}
	local := provider == "local"
	var held bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id
 WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject=? AND
 (a.locked=1 OR a.actor<>'' OR c.provider IN('upload','generated')
 OR (c.provider='local' AND ((?=0 AND ?='folders') OR (?=1 AND c.source_fence=?)))
 OR (c.provider<>'local' AND c.source_fence=? AND (?=0 OR ?='online'))))`, t.Kind, entity, role, subject, local, preferred, local, fence, fence, local, preferred).Scan(&held)
	if err != nil || held || !pending {
		return held, err
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artwork_jobs WHERE kind=? AND entity_id=? AND role=? AND subject=? AND status IN('pending','running','retry')
 AND (actor<>'' OR provider IN('upload','generated') OR (source_fence=? AND
 ((provider='local' AND (?=1 OR ?='folders')) OR (provider<>'local' AND (?=0 OR ?='online'))))))`, t.Kind, entity, role, subject, fence, local, preferred, local, preferred).Scan(&held)
	return held, err
}
