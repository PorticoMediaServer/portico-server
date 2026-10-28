package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"portico.local/server/internal/dbwork"
)

// Use the real baseline, observed driver and ordinary background-media plan.
func sourceAssetCost(t *testing.T, db *sql.DB, name string, fn func(context.Context, *sql.Tx) error) (dbwork.Cost, dbwork.WriteCost) {
	t.Helper()
	ctx, measure := dbwork.Measure(context.Background())
	dbwork.ResetPlans()
	if err := dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error { return fn(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	stats := dbwork.Plans()
	if stats.Measured != 1 {
		t.Fatalf("write not measured: %+v", stats)
	}
	if stats.Largest.Rows > 10 || stats.Overruns != 0 {
		t.Fatalf("asset acceptance exceeded constant trigger-inclusive bound: %+v", stats)
	}
	t.Logf("%s: statements=%d rows=%d pages=%d vmops=%d", name, measure().Statements, stats.Largest.Rows, stats.Largest.Pages, stats.Largest.VMOps)
	if name != "unlinked-insert" && measure().Statements != 3 {
		t.Fatalf("%s repeated source acceptance statements: %+v", name, measure())
	}
	if name == "availability-noop" && stats.Largest.Rows != 0 {
		t.Fatal("availability no-op wrote rows", stats.Largest)
	}
	if name == "unchanged-row" && stats.Largest.Rows != 1 {
		t.Fatal("unchanged UPDATE ran source writes", stats.Largest)
	}
	return measure(), stats.Largest
}

func TestSourceAssetAcceptanceCost(t *testing.T) {
	for _, owners := range []int{0, 1, 65} {
		t.Run(fmt.Sprintf("owners-%d", owners), func(t *testing.T) {
			db := cc1OpenDB(t, "asset-cost.sqlite")
			lib := cc1Library(t, db, "asset-cost", "Asset cost", "movie", "/asset-cost")
			items := make([]int64, owners)
			for i := range items {
				items[i] = facetTestEntity(t, db, lib, fmt.Sprintf("cost:%d", i))
			}
			asset := sourceTestAsset(t, db, items, "/asset-cost/shared.mkv")
			cc1Drain(t, db)
			sourceAssetCost(t, db, "availability", func(ctx context.Context, tx *sql.Tx) error { return SetAssetAvailableTx(ctx, tx, asset, false) })
			sourceAssetCost(t, db, "availability-noop", func(ctx context.Context, tx *sql.Tx) error { return SetAssetAvailableTx(ctx, tx, asset, false) })
			sourceAssetCost(t, db, "unchanged-row", func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `UPDATE catalog_assets SET id=id,token=token,path=path,size=size,modified_ns=modified_ns,container=container,video_codec=video_codec,audio_codec=audio_codec,width=width,height=height,duration=duration,available=available WHERE id=?`, asset)
				return err
			})
			sourceAssetCost(t, db, "codec", func(ctx context.Context, tx *sql.Tx) error {
				return SetAssetTx(ctx, tx, asset, map[string]any{"video_codec": "h264"})
			})
			sourceAssetCost(t, db, "duration", func(ctx context.Context, tx *sql.Tx) error {
				return SetAssetTx(ctx, tx, asset, map[string]any{"duration": 120})
			})
			sourceAssetCost(t, db, "size", func(ctx context.Context, tx *sql.Tx) error {
				return SetAssetTx(ctx, tx, asset, map[string]any{"size": int64(200)})
			})
		})
	}
}

func TestSourceAssetInsertCost(t *testing.T) {
	db := cc1OpenDB(t, "asset-insert-cost.sqlite")
	cc1Drain(t, db)
	sourceAssetCost(t, db, "unlinked-insert", func(ctx context.Context, tx *sql.Tx) error {
		_, err := CreateAssetTx(ctx, tx, "insert-cost", Asset{Path: "/asset-cost/new.mkv", Size: 100, Duration: 60}, true)
		return err
	})
}

func TestSourceAssetUnlinkedChangesThenLinkConsumers(t *testing.T) {
	db := cc1OpenDB(t, "asset-future-link.sqlite")
	lib := cc1Library(t, db, "future-link", "Future link", "movie", "/future-link")
	item := facetTestEntity(t, db, lib, "future:movie")
	cc1Drain(t, db)
	epoch := cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`)
	// CreateAssetTx retains one generic DomainAssetMetrics dirty row. Its
	// existing upstream trigger stamps the source epoch independently; no
	// owner-specific source marker is admitted for this unlinked asset.
	asset := sourceTestAsset(t, db, nil, "/future-link/movie.mkv")
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_upstream_sources WHERE domain=41`) != 0 || cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`) != epoch+1 {
		t.Fatalf("unlinked insert queued/fenced nonexistent owners: sources=%d epoch=%d before=%d", cc1Scalar(t, db, `SELECT count(*) FROM catalog_upstream_sources WHERE domain=41`), cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`), epoch)
	}
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_dirty WHERE domain=26 AND entity_id=?`, asset) != 1 {
		t.Fatal("insert lost generic asset metrics work")
	}
	cc1Drain(t, db)
	epoch = cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`)
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := SetAssetTx(ctx, tx, asset, map[string]any{"size": int64(200), "duration": 120, "available": false}); err != nil {
			return err
		}
		return SettleAssetMissingTx(ctx, tx, asset)
	})
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_upstream_sources WHERE domain=41`) != 0 || cc1Scalar(t, db, `SELECT count(*) FROM catalog_dirty WHERE domain IN(26,41)`) != 0 || cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`) != epoch {
		t.Fatal("unlinked update queued/fenced nonexistent owners")
	}
	before := cc1Scalar(t, db, `SELECT source_revision FROM metadata_publication_heads WHERE item_id=?`, item)
	revision := cc1Scalar(t, db, `SELECT revision FROM library_revisions WHERE library_id='future-link'`)
	cc1Exec(t, db, `UPDATE screen_metadata_work SET status='matched',lease='old',lease_until='future' WHERE target_kind='item' AND target_id=?`, item)
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error { return LinkAssetTx(ctx, tx, item, asset, Link{}) })
	if cc1Scalar(t, db, `SELECT available FROM catalog_asset_links WHERE entity_id=? AND asset_id=?`, item, asset) != 0 {
		t.Fatal("later link lost current raw availability")
	}
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_dirty WHERE entity_id=? AND domain IN(18,20,25)`, item) != 3 || cc1Scalar(t, db, `SELECT count(*) FROM catalog_upstream_sources WHERE domain=44 AND source_id=?`, item) != 1 {
		t.Fatal("later link lost browse/availability/metrics/missing consumers")
	}
	if cc1Scalar(t, db, `SELECT source_revision FROM metadata_publication_heads WHERE item_id=?`, item) <= before || cc1Scalar(t, db, `SELECT revision FROM library_revisions WHERE library_id='future-link'`) <= revision || cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`) <= epoch {
		t.Fatal("later link lost metadata/library/source invalidation")
	}
	if cc1Scalar(t, db, `SELECT count(*) FROM screen_metadata_work WHERE target_kind='item' AND target_id=? AND status='pending' AND lease=''`, item) != 1 {
		t.Fatal("later link lost screen wake")
	}
	cc1Drain(t, db)
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id=?`, item) != 1 {
		t.Fatal("later link did not settle missing title")
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error { return SetAssetAvailableTx(ctx, tx, asset, true) })
	cc1Drain(t, db)
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id=?`, item) != 0 || cc1Scalar(t, db, `SELECT available FROM catalog_item_availability WHERE entity_id=?`, item) != 1 || cc1Scalar(t, db, `SELECT duration FROM catalog_browse_item_metrics WHERE item_id=?`, item) != 120 {
		t.Fatal("future linked change lost availability/missing/metrics consumers")
	}
}

func TestSourceAssetRetouchResetsSameFlagsAndRollsBackAtomically(t *testing.T) {
	db := cc1OpenDB(t, "asset-retouch-cost.sqlite")
	lib := cc1Library(t, db, "asset-retouch", "Retouch", "movie", "/retouch")
	items := []int64{facetTestEntity(t, db, lib, "retouch:a"), facetTestEntity(t, db, lib, "retouch:b")}
	asset := sourceTestAsset(t, db, items, "/retouch/shared.mkv")
	cc1Drain(t, db)
	fence, err := CaptureFacetProducerFence(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		return SetAssetTx(ctx, tx, asset, map[string]any{"size": int64(200), "duration": 120, "available": false})
	})
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := ValidateFacetProducerFence(ctx, tx, fence); !errors.Is(err, ErrBuilding) {
			t.Fatalf("source drift admitted accepted fence: %v", err)
		}
		return nil
	})
	if cc1Scalar(t, db, `SELECT flags FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset) != 31 {
		t.Fatal("combined update lost required consumers")
	}
	facetTestTurn(t, db, 41, 1)
	if cc1Scalar(t, db, `SELECT has_after FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset) != 1 {
		t.Fatal("fixture did not partially progress")
	}
	before := cc1Scalar(t, db, `SELECT revision FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset)
	dirty := cc1Scalar(t, db, `SELECT revision FROM catalog_dirty WHERE domain=41 AND entity_id=?`, asset)
	epoch := cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`)
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		return SetAssetTx(ctx, tx, asset, map[string]any{"size": int64(300)})
	})
	// One source marker reset plus one independent asset-metrics dirty wake
	// each stamp the existing source fence; both source revisions advance once.
	if cc1Scalar(t, db, `SELECT revision FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset) != before+1 || cc1Scalar(t, db, `SELECT revision FROM catalog_dirty WHERE domain=41 AND entity_id=?`, asset) != dirty+1 || cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`) != epoch+2 {
		t.Fatalf("real same-flags change: source revision %d -> %d, dirty %d -> %d, epoch %d -> %d", before, cc1Scalar(t, db, `SELECT revision FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset), dirty, cc1Scalar(t, db, `SELECT revision FROM catalog_dirty WHERE domain=41 AND entity_id=?`, asset), epoch, cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`))
	}
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_upstream_sources WHERE domain=41 AND source_id=? AND flags=31 AND source_epoch=0 AND phase=0 AND has_after=0 AND after_id=0 AND active_id IS NULL AND inner_has=0`, asset) != 1 {
		t.Fatal("same-flags change retained stale progress")
	}
	source := groupRows(t, db, `SELECT * FROM catalog_upstream_sources WHERE domain=41`)
	dirt := groupRows(t, db, `SELECT * FROM catalog_dirty WHERE domain IN(26,41)`)
	epoch = cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// This rollback-only transaction defers the old playback token FK so the
	// source marker's captured lifetime can be inspected before rolling back.
	if _, err = tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE catalog_assets SET token='rollback-lifetime',size=400 WHERE id=?`, asset); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	var token string
	if err = tx.QueryRow(`SELECT token FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset).Scan(&token); err != nil || token != "rollback-lifetime" {
		tx.Rollback()
		t.Fatal("source did not capture changed input lifetime", token, err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if source != groupRows(t, db, `SELECT * FROM catalog_upstream_sources WHERE domain=41`) || dirt != groupRows(t, db, `SELECT * FROM catalog_dirty WHERE domain IN(26,41)`) || epoch != cc1Scalar(t, db, `SELECT source_epoch FROM catalog_facet_source_state`) {
		t.Fatal("rollback split source lifetime, cursor, dirty revision or epoch")
	}
	cc1Drain(t, db)
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id IN(?,?)`, items[0], items[1]) != 2 {
		t.Fatal("retouched work lost owners")
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := ValidateFacetProducerFence(ctx, tx, fence); !errors.Is(err, ErrFacetProducerChanged) {
			t.Fatalf("settled drift reused old accepted fence: %v", err)
		}
		return nil
	})
	// Physical deletion retains old link DELETE consumers; a reused integer ID
	// with a new token must never continue the old asset's pending source walk.
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		if err := SetAssetAvailableTx(ctx, tx, asset, true); err != nil {
			return err
		}
		if err := DeleteAssetTx(ctx, tx, asset); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO catalog_assets(id,token,path,size,modified_ns,container,video_codec,audio_codec,width,height,duration,available) VALUES(?,'reused-lifetime','/retouch/reused.mkv',500,0,'','','',0,0,180,1)`, asset)
		return err
	})
	facetTestTurn(t, db, 41, 1)
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset) != 0 {
		t.Fatal("reused integer continued old source lifetime")
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error { return LinkAssetTx(ctx, tx, items[0], asset, Link{}) })
	cc1Drain(t, db)
	if cc1Scalar(t, db, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id=?`, items[0]) != 0 || cc1Scalar(t, db, `SELECT duration FROM catalog_browse_item_metrics WHERE item_id=?`, items[0]) != 180 {
		t.Fatal("reused asset's new link lost current consumers")
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error { return SetAssetAvailableTx(ctx, tx, asset, false) })
	if err := db.QueryRow(`SELECT token FROM catalog_upstream_sources WHERE domain=41 AND source_id=?`, asset).Scan(&token); err != nil || token != "reused-lifetime" {
		t.Fatal("new source lost reused ID's new lifetime", token, err)
	}
}

func TestSourceAssetOwnerGuardUsesIndexedSeek(t *testing.T) {
	db := cc1OpenDB(t, "asset-owner-seek.sqlite")
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT a.token FROM catalog_assets a WHERE a.id=? AND EXISTS(SELECT 1 FROM catalog_asset_links INDEXED BY catalog_asset_links_asset WHERE asset_id=a.id)`, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
		if strings.Contains(strings.ReplaceAll(detail, " EXISTS USING ", " USING "), "SEARCH catalog_asset_links USING COVERING INDEX catalog_asset_links_asset (asset_id=?)") {
			found = true
		}
		if strings.Contains(detail, "SCAN catalog_asset_links") {
			t.Fatal("owner guard scanned links", detail)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("owner guard did not seek asset index", plan)
	}
}
