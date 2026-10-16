package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"portico.local/server/internal/requestinput"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
)

// Two catalogue rows that turn out to be one title become one. The files of
// both are versions of the title that stays; everyone's state for both is
// combined on it; every name either was known by now finds it. Nothing a
// person did to either is lost, and nothing is asked of them.

// ErrNotMergeable: the two entities are not the same kind of thing in the
// same library, so they cannot be one title.
var ErrNotMergeable = requestinput.New("titles of different kinds or libraries cannot be merged")

// MergeTitlesTx makes from part of into and removes from. Both must be of one
// kind in one library. A show takes the other show's seasons and episodes
// (same-numbered ones merge in turn); a film, episode, track or book part
// takes the other's files as versions.
func MergeTitlesTx(ctx context.Context, tx *sql.Tx, from, into int64) error {
	if from == into {
		return nil
	}
	var fromLibrary, intoLibrary int64
	var fromKind, intoKind compactcatalog.Kind
	if err := tx.QueryRowContext(ctx, `SELECT library_id,kind FROM catalog_entities WHERE id=?`, from).Scan(&fromLibrary, &fromKind); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT library_id,kind FROM catalog_entities WHERE id=?`, into).Scan(&intoLibrary, &intoKind); err != nil {
		return err
	}
	if fromLibrary != intoLibrary || fromKind != intoKind {
		return ErrNotMergeable
	}
	switch fromKind {
	case compactcatalog.Show:
		if _, err := mergeShowChildrenTx(ctx, tx, from, into, 0); err != nil {
			return err
		}
	case compactcatalog.Season:
		return fmt.Errorf("%w: seasons merge with their shows", ErrNotMergeable)
	case compactcatalog.Movie, compactcatalog.Episode, compactcatalog.Track, compactcatalog.Part, compactcatalog.Extra:
	default:
		return fmt.Errorf("%w: kind %d", ErrNotMergeable, fromKind)
	}
	return absorbTx(ctx, tx, from, into, fromKind)
}

// absorbTx moves everything that hangs off one entity onto another of the
// same kind and deletes the first. Children must already have been moved.
func absorbTx(ctx context.Context, tx *sql.Tx, from, into int64, kind compactcatalog.Kind) error {
	rows, err := tx.QueryContext(ctx, `SELECT provider,provider_kind,provider_id FROM catalog_external_ids WHERE entity_id=? ORDER BY provider,provider_kind,provider_id`, from)
	if err != nil {
		return err
	}
	var identities []compactcatalog.ProviderIdentity
	for rows.Next() {
		var p compactcatalog.ProviderIdentity
		if err = rows.Scan(&p.Provider, &p.Kind, &p.ID); err != nil {
			rows.Close()
			return err
		}
		identities = append(identities, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range identities {
		if err := compactcatalog.RecordProviderIdentityTx(ctx, tx, into, p); err != nil {
			if errors.Is(err, compactcatalog.ErrIdentityConflict) {
				return fmt.Errorf("%w: provider identities differ", ErrNotMergeable)
			}
			return err
		}
	}
	if err := moveDownloadClaimsTx(ctx, tx, from, into, ""); err != nil {
		return err
	}
	if err := moveFilesTx(ctx, tx, from, into); err != nil {
		return err
	}
	if err := movePersonalTx(ctx, tx, from, into); err != nil {
		return err
	}
	if err := moveContainerStateTx(ctx, tx, from, into); err != nil {
		return err
	}
	for _, q := range []string{
		// Sessions in flight keep playing: the file they hold is a version of
		// the surviving title now.
		`UPDATE playback_sessions SET item_id=?2 WHERE item_id=?1`,
		// Lists point at the title that stays.
		`UPDATE catalog_playlist_entries SET item_id=?2 WHERE item_id=?1`,
		`INSERT OR IGNORE INTO catalog_collection_members(collection_id,item_id,order_key) SELECT collection_id,?2,order_key FROM catalog_collection_members INDEXED BY catalog_collection_members_item WHERE item_id=?1`,
		`DELETE FROM catalog_collection_members WHERE item_id=?1`,
		`UPDATE OR IGNORE saved_resource_entries SET item_id=?2 WHERE item_id=?1`,
		`DELETE FROM saved_resource_entries WHERE item_id=?1`,
		// Subtitles and lyrics people added belong to the title.
		`INSERT OR IGNORE INTO subtitle_collections(item_id,revision) SELECT ?2,revision FROM subtitle_collections WHERE item_id=?1`,
		`UPDATE subtitle_resources SET item_id=?2 WHERE item_id=?1`,
		`UPDATE lyric_resources SET item_id=?2 WHERE item_id=?1`,
		// The ledger: every name of the absorbed title is a name of this one.
		`UPDATE catalog_identities SET public_id=(SELECT public_id FROM catalog_entities WHERE id=?2) WHERE public_id=(SELECT public_id FROM catalog_entities WHERE id=?1)`,
		// Its public id, and any it had absorbed before, now lead here.
		`UPDATE catalog_merged_ids SET entity_id=?2 WHERE entity_id=?1`,
		`INSERT OR REPLACE INTO catalog_merged_ids(public_id,entity_id) SELECT public_id,?2 FROM catalog_entities WHERE id=?1`,
	} {
		if _, err := tx.ExecContext(ctx, q, from, into); err != nil {
			return err
		}
	}
	// The surviving collections recount their members.
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_dirty(domain,entity_id,revision) SELECT ?,collection_id,1 FROM catalog_collection_members INDEXED BY catalog_collection_members_item WHERE item_id=?
 ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, compactcatalog.DomainBrowseRows, into); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM lyric_selections WHERE resource_id IN(SELECT id FROM lyric_resources WHERE item_id=?)`, from); err != nil {
		return err
	}
	if err := compactcatalog.DeleteEntityTx(ctx, tx, from); err != nil {
		return err
	}
	if err := compactcatalog.TouchEntityTx(ctx, tx, into); err != nil {
		return err
	}
	if err := compactcatalog.TouchTx(ctx, tx, compactcatalog.DomainAvailability, into); err != nil {
		return err
	}
	return compactcatalog.SettleMissingTx(ctx, tx, into)
}

// moveFilesTx makes every file of from a file of into, keeping its part,
// boundaries and edition, and any owner decision that tied the file to a title.
func moveFilesTx(ctx context.Context, tx *sql.Tx, from, into int64) error {
	type link struct {
		asset   int64
		token   string
		part    int
		start   float64
		end     sql.NullFloat64
		edition string
	}
	rows, err := tx.QueryContext(ctx, `SELECT l.asset_id,a.token,l.part_index,l.start_seconds,l.end_seconds,l.edition FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE l.entity_id=? ORDER BY l.part_index,l.asset_id`, from)
	if err != nil {
		return err
	}
	var links []link
	for rows.Next() {
		var l link
		if err = rows.Scan(&l.asset, &l.token, &l.part, &l.start, &l.end, &l.edition); err != nil {
			rows.Close()
			return err
		}
		links = append(links, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, l := range links {
		v := compactcatalog.Link{Part: l.part, Start: l.start, Edition: l.edition}
		if l.end.Valid {
			end := l.end.Float64
			v.End = &end
		}
		if err = compactcatalog.LinkAssetTx(ctx, tx, into, l.asset, v); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO episode_asset_boundaries(item_id,asset_id,status) SELECT ?2,asset_id,status FROM episode_asset_boundaries WHERE item_id=?1 AND asset_id=?3`, from, into, l.token); err != nil {
			return err
		}
	}
	// Before the links go: the cascade removes only decisions still naming from.
	_, err = tx.ExecContext(ctx, `UPDATE inventory_version_links SET item_id=?2 WHERE item_id=?1`, from, into)
	return err
}

// movePersonalTx combines each profile's state for from into its state for
// into. Where both have a value the kinder one wins: watched, saved, favourite
// and rated stay so; the later progress and the later explicit watched mark
// win; plays add up. Rows are inserted and deleted rather than re-pointed so
// every trigger that follows personal state sees the change.
func movePersonalTx(ctx context.Context, tx *sql.Tx, from, into int64) error {
	for _, q := range []string{
		`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at,not_interested)
 SELECT profile_id,?2,watchlisted,favorite,rating,revision,watched,last_played_at,not_interested FROM personal_items INDEXED BY personal_items_item_id_fk WHERE item_id=?1
 ON CONFLICT(profile_id,item_id) DO UPDATE SET watchlisted=max(personal_items.watchlisted,excluded.watchlisted),favorite=max(personal_items.favorite,excluded.favorite),
 rating=COALESCE(personal_items.rating,excluded.rating),revision=max(personal_items.revision,excluded.revision)+1,watched=max(personal_items.watched,excluded.watched),
 last_played_at=max(personal_items.last_played_at,excluded.last_played_at),not_interested=max(personal_items.not_interested,excluded.not_interested)`,
		`DELETE FROM personal_items WHERE item_id=?1`,

		`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) SELECT profile_id,?2,watched,authored_at FROM personal_watched_intents WHERE item_id=?1
 ON CONFLICT(profile_id,item_id) DO UPDATE SET watched=excluded.watched,authored_at=excluded.authored_at WHERE excluded.authored_at>personal_watched_intents.authored_at`,
		`DELETE FROM personal_watched_intents WHERE item_id=?1`,

		// Progress: the surviving title keeps its own position unless the other
		// was played more recently.
		`DELETE FROM progress WHERE item_id=?2 AND EXISTS(SELECT 1 FROM progress_activity theirs WHERE theirs.profile_id=progress.profile_id AND theirs.item_id=?1
 AND theirs.updated_at>COALESCE((SELECT ours.updated_at FROM progress_activity ours WHERE ours.profile_id=progress.profile_id AND ours.item_id=?2),''))`,
		`INSERT OR IGNORE INTO progress(profile_id,item_id,position,unit,completed,playback_id) SELECT profile_id,?2,position,unit,completed,playback_id FROM progress INDEXED BY progress_item_state WHERE item_id=?1`,
		`INSERT OR IGNORE INTO progress_activity(profile_id,library_id,item_id,updated_at,state) SELECT a.profile_id,a.library_id,?2,a.updated_at,a.state FROM progress p INDEXED BY progress_item_state
 JOIN progress_activity a ON a.profile_id=p.profile_id AND a.item_id=p.item_id WHERE p.item_id=?1`,
		`DELETE FROM progress WHERE item_id=?1`,
		`UPDATE book_resume SET item_id=?2 WHERE item_id=?1`,

		`UPDATE personal_history SET item_id=?2 WHERE item_id=?1`,
		`UPDATE personal_completed_plays SET item_id=?2 WHERE item_id=?1`,
		`INSERT INTO personal_play_counts(profile_id,item_id,plays) SELECT profile_id,?2,plays FROM personal_play_counts INDEXED BY personal_play_counts_item WHERE item_id=?1
 ON CONFLICT(profile_id,item_id) DO UPDATE SET plays=personal_play_counts.plays+excluded.plays`,
		`DELETE FROM personal_play_counts WHERE item_id=?1`,
		`INSERT INTO play_counts(item_id,plays,last_played_ms) SELECT ?2,plays,last_played_ms FROM play_counts WHERE item_id=?1
 ON CONFLICT(item_id) DO UPDATE SET plays=play_counts.plays+excluded.plays,last_played_ms=max(play_counts.last_played_ms,excluded.last_played_ms)`,
		`DELETE FROM play_counts WHERE item_id=?1`,

		`INSERT OR IGNORE INTO continue_dismissals(profile_id,item_id,playback_id) SELECT profile_id,?2,playback_id FROM continue_dismissals WHERE item_id=?1`,
		`DELETE FROM continue_dismissals WHERE item_id=?1`,

		// Sync and idempotency ledgers follow the title so a device that replays
		// an old change still finds its answer.
		`INSERT OR IGNORE INTO personal_evidence_fences(profile_id,item_id,playback_id) SELECT profile_id,?2,playback_id FROM personal_evidence_fences WHERE item_id=?1`,
		`INSERT OR IGNORE INTO playback_personal_fences(profile_key,item_id,through_ordinal) SELECT profile_key,?2,through_ordinal FROM playback_personal_fences WHERE item_id=?1`,
		`INSERT OR IGNORE INTO personal_snapshots(profile_id,item_id,revision,state,created_at) SELECT profile_id,?2,revision,state,created_at FROM personal_snapshots WHERE item_id=?1`,
		`UPDATE personal_explicit_events SET item_id=?2 WHERE item_id=?1`,
		`UPDATE OR IGNORE personal_conflicts SET item_id=?2 WHERE item_id=?1`,
		`UPDATE OR IGNORE personal_receipts SET item_id=?2 WHERE item_id=?1`,
	} {
		if _, err := tx.ExecContext(ctx, q, from, into); err != nil {
			return err
		}
	}
	// A show's "what was I watching" pointer names an episode by number.
	_, err := tx.ExecContext(ctx, `UPDATE profile_show_activity SET last_item_id=CAST(?2 AS TEXT) WHERE last_item_id=CAST(?1 AS TEXT)
 AND show_id IN(SELECT show_id FROM catalog_episodes WHERE entity_id IN(?1,?2))`, from, into)
	return err
}

// moveContainerStateTx carries the watched marks set on a whole show, season,
// album or book. The surviving container keeps its own mark where it has one.
func moveContainerStateTx(ctx context.Context, tx *sql.Tx, from, into int64) error {
	for _, q := range []string{
		`INSERT OR IGNORE INTO container_personal_state(profile_id,kind,container_id,watched,watermark,revision,cleared_through)
 SELECT profile_id,kind,?2,watched,watermark,revision,cleared_through FROM container_personal_state WHERE container_id=?1`,
		`DELETE FROM container_personal_state WHERE container_id=?1`,
		`DELETE FROM container_personal_undo WHERE container_id=?1`,
		`INSERT INTO profile_show_activity(profile_id,show_id,library_id,last_activity,last_item_id) SELECT profile_id,?2,library_id,last_activity,last_item_id FROM profile_show_activity INDEXED BY profile_show_activity_show_id_fk WHERE show_id=?1
 ON CONFLICT(profile_id,show_id) DO UPDATE SET last_activity=excluded.last_activity,last_item_id=excluded.last_item_id WHERE excluded.last_activity>profile_show_activity.last_activity`,
		`DELETE FROM profile_show_activity WHERE show_id=?1`,
		`INSERT OR IGNORE INTO continue_show_dismissals(profile_id,show_id,item_id,playback_id,last_activity) SELECT profile_id,?2,item_id,playback_id,last_activity FROM continue_show_dismissals INDEXED BY continue_show_dismissals_show_id_fk WHERE show_id=?1`,
		`DELETE FROM continue_show_dismissals WHERE show_id=?1`,
	} {
		if _, err := tx.ExecContext(ctx, q, from, into); err != nil {
			return err
		}
	}
	return nil
}

// showMergeBatch is how many of a show's episodes one write transaction of
// the owner's merge moves, so merging a very large show never holds the write
// gate for the whole show.
const showMergeBatch = 100

// mergeShowChildrenTx moves a show's seasons and episodes under another show.
// A season or episode the other show already has at the same number absorbs
// the incoming one; the rest are re-parented as they are. With limit > 0 it
// moves at most limit episodes and reports more=true while episodes remain;
// the seasons and the show itself follow once none do. Each step leaves both
// shows whole, so a merge interrupted between steps simply continues.
func mergeShowChildrenTx(ctx context.Context, tx *sql.Tx, from, into int64, limit int) (more bool, err error) {
	var intoKey string
	if err = tx.QueryRowContext(ctx, `SELECT local_key FROM catalog_shows WHERE entity_id=?`, into).Scan(&intoKey); err != nil {
		return false, err
	}
	// Where a season of from ends up: the other show's season of that number,
	// or the season itself, re-parented, when the other show has none.
	type placed struct {
		id     int64
		number int
	}
	targets := map[int64]placed{}
	seasonTarget := func(season int64) (placed, error) {
		if t, ok := targets[season]; ok {
			return t, nil
		}
		var number int
		if err := tx.QueryRowContext(ctx, `SELECT number FROM catalog_seasons WHERE entity_id=?`, season).Scan(&number); err != nil {
			return placed{}, err
		}
		var existing int64
		err := tx.QueryRowContext(ctx, `SELECT entity_id FROM catalog_seasons WHERE show_id=? AND number=?`, into, number).Scan(&existing)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if err = reparentSeasonTx(ctx, tx, season, into, intoKey, number); err != nil {
				return placed{}, err
			}
			existing = season
		case err != nil:
			return placed{}, err
		}
		targets[season] = placed{existing, number}
		return targets[season], nil
	}
	type episode struct {
		id        int64
		season    sql.NullInt64
		numbering string
		number    int
	}
	query := `SELECT entity_id,season_id,numbering,number FROM catalog_episodes INDEXED BY catalog_episodes_show WHERE show_id=? ORDER BY entity_id`
	args := []any{from}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	var episodes []episode
	for rows.Next() {
		var e episode
		if err = rows.Scan(&e.id, &e.season, &e.numbering, &e.number); err != nil {
			rows.Close()
			return false, err
		}
		episodes = append(episodes, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, e := range episodes {
		var seasonID int64
		var seasonNumber int
		if e.season.Valid {
			t, err := seasonTarget(e.season.Int64)
			if err != nil {
				return false, err
			}
			seasonID, seasonNumber = t.id, t.number
		}
		var existing int64
		err = tx.QueryRowContext(ctx, `SELECT entity_id FROM catalog_episodes INDEXED BY catalog_episodes_position WHERE show_id=? AND COALESCE(season_id,0)=? AND numbering=? AND number=?`, into, seasonID, e.numbering, e.number).Scan(&existing)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			parent := into
			var seasonRef any
			if seasonID != 0 {
				parent, seasonRef = seasonID, seasonID
			}
			if _, err = tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=? WHERE id=?`, parent, e.id); err != nil {
				return false, err
			}
			if err = compactcatalog.SetFactsTx(ctx, tx, e.id, map[string]any{"show_id": into, "season_id": seasonRef}); err != nil {
				return false, err
			}
			if _, err = compactcatalog.AddIdentityTx(ctx, tx, e.id, compactcatalog.EpisodeKey(intoKey, e.numbering, seasonNumber, e.number)); err != nil {
				return false, err
			}
			if err = compactcatalog.TouchEntityTx(ctx, tx, e.id); err != nil {
				return false, err
			}
		case err != nil:
			return false, err
		default:
			if err = absorbTx(ctx, tx, e.id, existing, compactcatalog.Episode); err != nil {
				return false, err
			}
		}
	}
	if limit > 0 && len(episodes) == limit {
		return true, nil
	}
	// No episodes are left under from: its seasons follow, now empty or
	// already re-parented above.
	seasons, err := scanInt64s(tx.QueryContext(ctx, `SELECT entity_id FROM catalog_seasons WHERE show_id=? ORDER BY number`, from))
	if err != nil {
		return false, err
	}
	for _, season := range seasons {
		t, err := seasonTarget(season)
		if err != nil {
			return false, err
		}
		if t.id != season {
			if err = absorbTx(ctx, tx, season, t.id, compactcatalog.Season); err != nil {
				return false, err
			}
		}
	}
	// What the show's own facts were derived from has changed under it.
	_, err = tx.ExecContext(ctx, `INSERT INTO catalog_dirty(domain,entity_id,revision) SELECT ?,entity_id,1 FROM catalog_episodes INDEXED BY catalog_episodes_show WHERE show_id=?
 ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, compactcatalog.DomainBrowseEdges, into)
	return false, err
}

// reparentSeasonTx moves a season, with whatever episodes it still holds,
// under another show as that show's season of its number.
func reparentSeasonTx(ctx context.Context, tx *sql.Tx, season, into int64, intoKey string, number int) error {
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=? WHERE id=?`, into, season); err != nil {
		return err
	}
	if err := compactcatalog.SetFactsTx(ctx, tx, season, map[string]any{"show_id": into}); err != nil {
		return err
	}
	if _, err := compactcatalog.AddIdentityTx(ctx, tx, season, compactcatalog.SeasonKey(intoKey, number)); err != nil {
		return err
	}
	return compactcatalog.TouchEntityTx(ctx, tx, season)
}

// AdoptProviderIdentityTx is called when a title has just been given a
// provider identity. Any other title of the same kind in the same library that
// carries the same identity is the same title: it is merged into this one,
// which keeps its id. It reports how many were merged.
func AdoptProviderIdentityTx(ctx context.Context, tx *sql.Tx, entity int64, p compactcatalog.ProviderIdentity) (int, error) {
	if p.Provider == "" || p.ID == "" {
		return 0, nil
	}
	others, err := scanInt64s(tx.QueryContext(ctx, `SELECT other.id FROM catalog_entities me CROSS JOIN catalog_external_ids x INDEXED BY catalog_external_ids_lookup ON x.provider=?2 AND x.provider_kind=?3 AND x.provider_id=?4
 JOIN catalog_entities other ON other.id=x.entity_id AND other.library_id=me.library_id AND other.kind=me.kind AND other.id<>me.id WHERE me.id=?1 ORDER BY other.id LIMIT 16`, entity, p.Provider, p.Kind, p.ID))
	if err != nil {
		return 0, err
	}
	for _, other := range others {
		if err = MergeTitlesTx(ctx, tx, other, entity); err != nil {
			return 0, err
		}
	}
	if err = restoreRetainedTx(ctx, tx, entity); err != nil {
		return 0, err
	}
	return len(others), nil
}

// MergeTitles is the owner's "these two are the same title": from becomes
// part of into. Both are public ids of titles in one library. A show's
// episodes move in bounded write transactions, each authorized and resolved
// afresh, and the show itself is absorbed in the last one.
func (s *Service) MergeTitles(ctx context.Context, from, into string, authorize func(*sql.Tx) error) error {
	if from == "" || into == "" || from == into || authorize == nil {
		return ErrAdminQuery
	}
	for {
		done := false
		err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive), func(tx *sql.Tx) error {
			done = false
			if err := authorize(tx); err != nil {
				return err
			}
			a, err := entityid.Resolve(ctx, tx, from)
			if err != nil {
				return err
			}
			b, err := entityid.Resolve(ctx, tx, into)
			if err != nil {
				return err
			}
			if a == b {
				done = true
				return nil
			}
			var library string
			if err = tx.QueryRowContext(ctx, `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=? AND cl.retired=0`, b).Scan(&library); err != nil {
				return err
			}
			var fromKind, intoKind compactcatalog.Kind
			var fromLibrary, intoLibrary int64
			if err = tx.QueryRowContext(ctx, `SELECT library_id,kind FROM catalog_entities WHERE id=?`, a).Scan(&fromLibrary, &fromKind); err != nil {
				return err
			}
			if err = tx.QueryRowContext(ctx, `SELECT library_id,kind FROM catalog_entities WHERE id=?`, b).Scan(&intoLibrary, &intoKind); err != nil {
				return err
			}
			if fromKind == compactcatalog.Show && intoKind == compactcatalog.Show && fromLibrary == intoLibrary {
				more, err := mergeShowChildrenTx(ctx, tx, a, b, showMergeBatch)
				if err != nil {
					return err
				}
				if more {
					return nil
				}
				err = absorbTx(ctx, tx, a, b, compactcatalog.Show)
			} else {
				err = MergeTitlesTx(ctx, tx, a, b)
			}
			if err != nil {
				return err
			}
			done = true
			_, err = tx.ExecContext(ctx, `UPDATE library_revisions SET revision=revision+1 WHERE library_id=?`, library)
			return err
		})
		if err != nil || done {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
	}
}

// SurvivingTitleID returns the public id a request naming public should use:
// the title that absorbed it when public names a merged-away title, otherwise
// public itself. A failed lookup leaves the id as it was.
func (s *Service) SurvivingTitleID(ctx context.Context, public string) string {
	if s == nil || s.db == nil {
		return public
	}
	survivor, err := entityid.Survivor(ctx, dbwork.ReadHandle(ctx, s.db), public)
	if err != nil || survivor == "" {
		return public
	}
	return survivor
}
