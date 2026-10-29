package catalog

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/phasedpurge"
)

// Accepted removals prepare existing normalized removed-title facts before
// draining their inputs. The live entity is the public visibility barrier;
// its eventual DELETE publishes the complete facts. A vanished/dead local
// binding is refused by the owner's same-writer guard before any viewer copy.
const retainedTitleTombstoneHeaderSQL = `INSERT INTO catalog_title_tombstones(public_id,library_id,title,kind,poster_url,backdrop_url,recording,removed_at,expires_at)
 SELECT pid(e.public_id),COALESCE(cl.library_id,''),e.title,e.kind,
 COALESCE((SELECT poster_url FROM catalog_item_details WHERE entity_id=e.id),''),
 COALESCE((SELECT backdrop_url FROM catalog_item_details WHERE entity_id=e.id),''),
 EXISTS(SELECT 1 FROM dvr_catalog_provenance WHERE item_id=e.id),
 strftime('%Y-%m-%dT%H:%M:%fZ','now'),strftime('%Y-%m-%dT%H:%M:%fZ','now','+30 days')
 FROM catalog_entities e LEFT JOIN catalog_libraries cl ON cl.id=e.library_id
 WHERE e.id=? AND e.kind IN(1,4,7,9) ON CONFLICT(public_id) DO NOTHING`

func prepareRetainedTitleTombstoneTx(ctx context.Context, tx *sql.Tx, item int64) error {
	_, err := tx.ExecContext(ctx, retainedTitleTombstoneHeaderSQL, item)
	return err
}

// Rebase only the accepted O022 build, immediately before releasing the
// mutation's restrictive entity FK in the final deletion writer. The actual
// DELETE must follow in the same transaction; a failed deletion rolls this
// timestamp change back. Legacy removal has no matching job and writes zero.
func publishRetainedTitleTombstoneTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) error {
	_, err := tx.ExecContext(ctx, `UPDATE catalog_title_tombstones SET
 removed_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),expires_at=strftime('%Y-%m-%dT%H:%M:%fZ','now','+30 days')
 WHERE public_id=?1 AND EXISTS(SELECT 1 FROM catalog_purge_jobs j
 JOIN catalog_entity_mutations mt ON mt.mutation_id=j.mutation_id AND mt.generation=j.generation
 JOIN catalog_entities e ON e.id=mt.entity_id
 WHERE j.mutation_id=?2 AND j.generation=?3 AND mt.status='active'
 AND mt.entity_id=?4 AND mt.public_id=?1 AND e.public_id=pid_blob(?1))`, m.Public, m.Ref.ID, m.Ref.Generation, m.Entity)
	return err
}

// Precedence is the existing trigger's order: continuing progress, then the
// first raw history row, then a favourite/watchlist reference. Each source is
// an exact item/owner prefix; a viewer INSERT costs at most two relational
// effects, including catalog_removed_title_reference_insert. The read-only
// missing probes are also the shared engine's verification/rewind witnesses.
const retainedTitleTombstoneProgressSource = ` FROM catalog_entities e
 JOIN progress_activity a ON a.item_id=e.id AND a.profile_id=?1
 JOIN progress p ON p.profile_id=a.profile_id AND p.item_id=a.item_id
 WHERE e.id=?2 AND e.kind IN(1,4,7,9)`

const retainedTitleTombstoneProgressSQL = `INSERT OR IGNORE INTO catalog_title_tombstone_viewers(public_id,profile_id,activity_at,continuing)
 SELECT pid(e.public_id),a.profile_id,a.updated_at,a.state<>'ended' AND p.position>0 AND p.completed=0` + retainedTitleTombstoneProgressSource

const retainedTitleTombstoneProgressMissingSQL = `SELECT EXISTS(SELECT 1` + retainedTitleTombstoneProgressSource + ` AND NOT EXISTS(SELECT 1 FROM catalog_title_tombstone_viewers v WHERE v.public_id=pid(e.public_id) AND v.profile_id=?1))`

const retainedTitleTombstoneHistorySource = ` FROM catalog_entities e
 JOIN personal_history h INDEXED BY personal_history_retention_owner ON h.item_id=e.id AND h.profile_id=?1
 WHERE e.id=?2 AND e.kind IN(1,4,7,9)`

const retainedTitleTombstoneHistorySQL = `INSERT OR IGNORE INTO catalog_title_tombstone_viewers(public_id,profile_id,activity_at,continuing)
 SELECT pid(e.public_id),h.profile_id,h.updated_at,0` + retainedTitleTombstoneHistorySource + ` LIMIT 1`

const retainedTitleTombstoneHistoryMissingSQL = `SELECT EXISTS(SELECT 1` + retainedTitleTombstoneHistorySource + ` AND NOT EXISTS(SELECT 1 FROM catalog_title_tombstone_viewers v WHERE v.public_id=pid(e.public_id) AND v.profile_id=?1))`

const retainedTitleTombstoneFavouriteSource = ` FROM catalog_entities e
 JOIN personal_items p ON p.item_id=e.id AND p.profile_id=?1
 WHERE e.id=?2 AND e.kind IN(1,4,7,9) AND (p.watchlisted=1 OR p.favorite=1)`

const retainedTitleTombstoneFavouriteSQL = `INSERT OR IGNORE INTO catalog_title_tombstone_viewers(public_id,profile_id,activity_at,continuing)
 SELECT pid(e.public_id),p.profile_id,COALESCE(p.last_played_at,''),0` + retainedTitleTombstoneFavouriteSource

const retainedTitleTombstoneFavouriteMissingSQL = `SELECT EXISTS(SELECT 1` + retainedTitleTombstoneFavouriteSource + ` AND NOT EXISTS(SELECT 1 FROM catalog_title_tombstone_viewers v WHERE v.public_id=pid(e.public_id) AND v.profile_id=?1))`

// This literal unit belongs before attribute-source retirement in the physical
// registry. The caller still owns its source/derivation custody fence and its
// durable compiled cursor. It is not wired into the legacy physical deletion.
// One raw edge is selected before field qualification; rejected fields also
// consume a turn. The exact term PK resolves its display/visibility facts.
// DELETE's six point trigger programs enqueue work and update singletons; the
// widest arm changes at most11 relational rows including the copied fact.
func retainedTitleTombstoneAttributeUnit(m compactcatalog.EntityMutation) phasedpurge.Unit {
	return phasedpurge.Unit{Phase: 0, Name: "removed-title-attribute", ReadBound: 32,
		Run: func(ctx context.Context, tx *sql.Tx, _ []phasedpurge.Key) (phasedpurge.Outcome, error) {
			root, err := readRetainedCaptureRootTx(ctx, tx, m)
			if err != nil {
				return phasedpurge.Outcome{}, err
			}
			if root == nil {
				return phasedpurge.Outcome{}, errRetainedTitleRoot
			}
			var term int64
			var source string
			err = tx.QueryRowContext(ctx, `SELECT term_id,source_value FROM catalog_item_attribute_edges WHERE item_id=? ORDER BY term_id LIMIT 1`, m.Entity).Scan(&term, &source)
			if errors.Is(err, sql.ErrNoRows) {
				return phasedpurge.Outcome{Drained: true}, nil
			}
			if err != nil {
				return phasedpurge.Outcome{}, err
			}
			var field int64
			var value string
			if err = tx.QueryRowContext(ctx, `SELECT field_id,value_key FROM catalog_attribute_terms WHERE id=?`, term).Scan(&field, &value); err != nil {
				return phasedpurge.Outcome{}, err
			}
			if (m.Kind == compactcatalog.Movie || m.Kind == compactcatalog.Episode || m.Kind == compactcatalog.Track || m.Kind == compactcatalog.Part) && field >= 1 && field <= 3 {
				if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_title_tombstone_attributes(public_id,field_id,value_key,source_value) VALUES(?,?,?,?)`, m.Public, field, value, source); err != nil {
					return phasedpurge.Outcome{}, err
				}
			}
			res, err := tx.ExecContext(ctx, `DELETE FROM catalog_item_attribute_edges WHERE item_id=? AND term_id=?`, m.Entity, term)
			if err != nil {
				return phasedpurge.Outcome{}, err
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				return phasedpurge.Outcome{}, errors.Join(errRetainedTitleState, err)
			}
			return phasedpurge.Outcome{Changed: true}, nil
		},
		Empty: func(ctx context.Context, tx *sql.Tx) (bool, error) {
			var present bool
			err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_item_attribute_edges WHERE item_id=?)`, m.Entity).Scan(&present)
			return !present, err
		},
	}
}
