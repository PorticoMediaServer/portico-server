package playbackv1

import (
	"context"
	"database/sql"
	"errors"
	"regexp"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/segmentmarkers"
)

var markerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// recordSkip records a timeline's skip report (spec §4.2) as marker evidence:
// one row per session and marker (a retried report writes no second row), and
// one diagnostics entry. An automatic skip of a marker the server never
// authorized for unattended skipping is not recorded: diagnostics must not
// report a policy the server did not make. A manual skip is recorded for any
// viewer marker of the session's item. Failures are dropped: evidence never
// fails the report that carried it.
func (s *Service) recordSkip(ctx context.Context, r row, k SkipReport) {
	var kind string
	recorded := false
	err := dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassBackgroundMedia, func(ctx context.Context, tx *sql.Tx) error {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM playback_marker_skips WHERE playback_id=? AND marker_id=?`, r.id, k.MarkerID).Scan(&one); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var provenance string
		var confidence float64
		var approved bool
		// Markers, inventory, links and entities are all synchronous facts.
		err := tx.QueryRowContext(ctx, `SELECT m.kind,m.confidence,m.provenance,m.approved FROM analysis_markers m JOIN inventory_objects o ON o.id=m.object_id JOIN catalog_assets asset ON asset.token=o.asset_id JOIN catalog_asset_links link ON link.asset_id=asset.id JOIN catalog_entities item ON item.id=link.entity_id WHERE m.id=? AND item.public_id=pid_blob(?) AND m.deleted=0 LIMIT 1`, k.MarkerID, r.item).Scan(&kind, &confidence, &provenance, &approved)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // not this item's marker
		}
		if err != nil {
			return err
		}
		if !segmentmarkers.ViewerKind(kind) || k.Mode == "automatic" && !segmentmarkers.AutomaticSafe(approved, confidence, provenance) {
			return nil
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO playback_marker_skips(playback_id,marker_id,mode,position_us,generation,created_ms) VALUES(?,?,?,?,?,?)`, r.id, k.MarkerID, k.Mode, k.PositionMs*1000, max(r.generation, 1), s.now().UnixMilli()); err != nil {
			return err
		}
		recorded = true
		return nil
	})
	if err == nil && recorded && s.SkipEvidence != nil {
		s.SkipEvidence(ctx, "marker_skip_"+k.Mode+"_"+kind, map[string]int64{"count": 1, "durationMs": k.PositionMs})
	}
}
