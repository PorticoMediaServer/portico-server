package catalog

import (
	"context"
	"errors"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/localmetadata"
	"portico.local/server/internal/subtitles"
)

// CommitInventoryLocalRefresh publishes a video file's re-read local metadata
// and subtitle inventory after only its sidecars changed (S6). The file's
// technical facts, streams, chapters and naming are this revision's already
// and are left exactly as they are; this writes what CommitInventoryAnalysis
// writes from local metadata and subtitle discovery, under the same fences,
// and settles the analysis queue row the same way. f.AnalysisOperations is the
// object's complete operation list after the refresh.
func (s *Service) CommitInventoryLocalRefresh(ctx context.Context, job, object, revision string, policyRevision int64, f assets.Facts) error {
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
	policy, err := ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return err
	}
	if policy.Revision != policyRevision || !policy.Allows("probe") {
		return context.Canceled
	}
	// The stored probe evidence is reused only for the revision it was taken
	// from: the object must still be that revision and its analysis must be of it.
	var asset string
	var size, modified int64
	err = tx.QueryRowContext(ctx, `SELECT asset_id,size,modified_ns FROM inventory_objects WHERE id=? AND source_id=? AND revision=? AND analysis_revision=? AND retired=0 AND state='available'`, object, source.ID, revision, revision).Scan(&asset, &size, &modified)
	if err != nil {
		return err
	}
	var kind string
	if err = tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind); err != nil {
		return err
	}
	if kind != "movie" && kind != "tv" && kind != "anime" {
		return errors.New("local refresh is for video libraries")
	}
	if policy.Allows("subtitles") && f.SubtitleInventory != nil {
		if err = subtitles.Persist(tx, asset, size, modified, f.SubtitleInventory); err != nil {
			return err
		}
	}
	var sidecarObservation *sidecarRead
	if f.ArtworkObserved {
		ctx, sidecarObservation = withSidecarRead(ctx, asset)
	}
	f.ObservedRevision = revision
	if policy.Allows("local_metadata") {
		if err = localmetadata.PersistVideo(ctx, tx, source.LibraryID, asset, f); err != nil {
			return err
		}
	}
	if err = s.persistScanArtworkTx(ctx, tx, source, object, revision, asset, kind, policy, f, sidecarObservation); err != nil {
		return err
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
	analyzed := revision
	if f.SubtitleTextPending {
		analyzed = ""
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_revision=?,analysis_policy=?,analysis_operations_json=?,analysis_state=?,analysis_error='',analysis_generation=? WHERE id=?`, analyzed, policyRevision, actualScanOperations(policy, f.AnalysisOperations), analysisState, source.Generation, object); err != nil {
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
	return gated.Commit()
}
