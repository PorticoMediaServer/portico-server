package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

func MergeTitlesStepTx(ctx context.Context, tx *sql.Tx, ref compactcatalog.MutationRef) (compactcatalog.MutationStep, error) {
	m, step, err := checkedMutationTx(ctx, tx, ref)
	if err != nil || m.Status != "active" || step.Complete || step.Outcome == "blocked" {
		return step, err
	}
	ctx = compactcatalog.WithMutationFamily(ctx, m.Family)
	if m.Child != "" {
		return advanceMutationChildTx(ctx, tx, m)
	}
	if m.Phase == "admitted" {
		return compactcatalog.PendingMutationStep(), compactcatalog.SetMutationPhaseTx(ctx, tx, ref, "children")
	}
	if m.Phase == "children" {
		switch m.Kind {
		case compactcatalog.Album:
			more, e := mergeOneAlbumChildTx(ctx, tx, m)
			if e != nil || more {
				return compactcatalog.PendingMutationStep(), e
			}
		case compactcatalog.Artist:
			more, e := mergeOneArtistChildTx(ctx, tx, m)
			if e != nil || more {
				return compactcatalog.PendingMutationStep(), e
			}
		case compactcatalog.Show:
			more, e := mergeShowChildrenTx(ctx, tx, m.Entity, m.Target, showMergeBatch)
			if e != nil || more {
				return compactcatalog.PendingMutationStep(), e
			}
		case compactcatalog.Season:
			return compactcatalog.MutationStep{}, ErrNotMergeable
		case compactcatalog.Book, compactcatalog.Author:
			// These domains have no MB pins; retain their complete-only existing
			// state/membership semantics in finalization below.
		case compactcatalog.Movie, compactcatalog.Episode, compactcatalog.Track, compactcatalog.Part, compactcatalog.Extra:
		default:
			return compactcatalog.MutationStep{}, ErrNotMergeable
		}
		return compactcatalog.PendingMutationStep(), compactcatalog.SetMutationPhaseTx(ctx, tx, ref, "drain")
	}
	if m.Phase == "drain" {
		if err = dbwork.UsePlan(tx, entityPinDrainPlan); err != nil {
			return compactcatalog.MutationStep{}, err
		}
		return compactcatalog.DrainEntityMBPinTx(ctx, tx, ref)
	}
	if m.Phase != "finalize" {
		return compactcatalog.MutationStep{}, fmt.Errorf("unexpected merge phase %s", m.Phase)
	}
	if err = compactcatalog.RequireNoEntityMBPinsTx(ctx, tx, m.Entity); err != nil {
		return compactcatalog.MutationStep{}, err
	}
	step, err = compactcatalog.FinishEntityMutationTx(ctx, tx, ref, "merged")
	if err != nil {
		return step, err
	}
	// Additive personal-state transfer and source deletion are indivisible.
	// Neither is executed by an earlier child/drain transaction.
	if m.Kind == compactcatalog.Book || m.Kind == compactcatalog.Author {
		err = mergeContainerTx(ctx, tx, m.Entity, m.Target, m.Kind)
	} else {
		err = absorbTx(ctx, tx, m.Entity, m.Target, m.Kind)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE library_revisions SET revision=revision+1 WHERE library_id=(SELECT library_id FROM catalog_libraries WHERE id=?)`, m.Library)
	}
	return step, err
}
func mergeOneAlbumChildTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) (bool, error) {
	var c containerChild
	err := tx.QueryRowContext(ctx, `SELECT x.entity_id,x.disc_number,x.track_number,e.title FROM catalog_songs x INDEXED BY catalog_songs_album_order JOIN catalog_entities e ON e.id=x.entity_id WHERE x.album_id=? ORDER BY x.disc_number,x.track_number,x.entity_id LIMIT 1`, m.Entity).Scan(&c.id, &c.disc, &c.number, &c.title)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	same, err := samePlaceTx(ctx, tx, `SELECT x.entity_id,e.title FROM catalog_songs x INDEXED BY catalog_songs_listening_order JOIN catalog_entities e ON e.id=x.entity_id WHERE x.album_id=? AND COALESCE(x.disc_number,1)=COALESCE(?,1) AND COALESCE(x.track_number,0)=COALESCE(?,0) ORDER BY x.entity_id LIMIT 16`, m.Target, c)
	if err != nil {
		return false, err
	}
	if same != 0 && same != c.id {
		return true, mutationChildTx(ctx, tx, m, c.id, same, "merge")
	}
	if err = compactcatalog.RequireEntityAcquirableTx(ctx, tx, c.id); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=? WHERE id=?`, m.Target, c.id); err != nil {
		return false, err
	}
	return true, compactcatalog.SetFactsTx(ctx, tx, c.id, map[string]any{"album_id": m.Target})
}
func mergeOneArtistChildTx(ctx context.Context, tx *sql.Tx, m compactcatalog.EntityMutation) (bool, error) {
	var id int64
	var title, edition string
	var year int
	err := tx.QueryRowContext(ctx, `SELECT a.entity_id,e.title,e.year,a.edition_key FROM catalog_albums a INDEXED BY catalog_albums_artist JOIN catalog_entities e ON e.id=a.entity_id WHERE a.artist_id=? ORDER BY a.entity_id LIMIT 1`, m.Entity).Scan(&id, &title, &year, &edition)
	if err == nil {
		rows, e := tx.QueryContext(ctx, `SELECT a.entity_id,e.title,a.edition_key FROM catalog_albums a INDEXED BY catalog_albums_artist JOIN catalog_entities e ON e.id=a.entity_id WHERE a.artist_id=? AND e.year=? ORDER BY a.entity_id LIMIT 64`, m.Target, year)
		if e != nil {
			return false, e
		}
		var same int64
		for rows.Next() {
			var candidate int64
			var name, ed string
			if e = rows.Scan(&candidate, &name, &ed); e != nil {
				rows.Close()
				return false, e
			}
			if _, ok := mergeLocalEdition(ed, edition); ok && same == 0 && compactcatalog.IdentityText(name) == compactcatalog.IdentityText(title) {
				same = candidate
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return false, e
		}
		if same != 0 {
			return true, mutationChildTx(ctx, tx, m, id, same, "merge")
		}
		if e = compactcatalog.RequireEntityAcquirableTx(ctx, tx, id); e != nil {
			return false, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=? WHERE id=?`, m.Target, id); e != nil {
			return false, e
		}
		return true, compactcatalog.SetFactsTx(ctx, tx, id, map[string]any{"artist_id": m.Target})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var song int64
	err = tx.QueryRowContext(ctx, `SELECT song_id FROM catalog_song_artists INDEXED BY catalog_song_artists_artist WHERE artist_id=? ORDER BY song_id LIMIT 1`, m.Entity).Scan(&song)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	artists, err := scanInt64s(tx.QueryContext(ctx, `SELECT artist_id FROM catalog_song_artists WHERE song_id=? ORDER BY artist_id`, song))
	if err != nil {
		return false, err
	}
	var next []int64
	seen := map[int64]bool{}
	for _, a := range artists {
		if a == m.Entity {
			a = m.Target
		}
		if !seen[a] {
			next = append(next, a)
			seen[a] = true
		}
	}
	return true, compactcatalog.SetSongArtistsTx(ctx, tx, song, next)
}
