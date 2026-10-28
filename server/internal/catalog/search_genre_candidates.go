package catalog

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
)

const genrePositionGap = int64(1 << 48)

type genreTarget struct {
	Entity           int64
	Public           []byte
	Kind             int
	Rating           float64
	Order            string
	Backdrop, Poster []byte
}

func addGenreCandidates(ctx context.Context, tx *sql.Tx, base, value, target int64, kind int) error {
	t := genreTarget{Entity: target, Kind: kind}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(b.rating_max,0),b.sort_key,e.public_id FROM catalog_browse_rows b JOIN catalog_entities e ON e.id=b.entity_id WHERE b.entity_id=?`, target).Scan(&t.Rating, &t.Order, &t.Public); err != nil {
		return err
	}
	artKind := "item"
	if kind == 2 {
		artKind = "show"
	}
	for role, roleName := range []string{"backdrop", "poster"} {
		var digest string
		err := tx.QueryRowContext(ctx, `SELECT a.digest FROM artwork_selections a JOIN artwork_sources s ON s.digest=a.digest JOIN artwork_blobs sb ON sb.digest=s.small_blob AND sb.status='ready' JOIN artwork_blobs lb ON lb.digest=s.large_blob AND lb.status='ready' WHERE a.kind=? AND a.entity_id=? AND a.role=? AND a.subject=''`, artKind, target, roleName).Scan(&digest)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		raw, err := hex.DecodeString(digest)
		if err != nil || len(raw) != 32 {
			return ErrFacetBaseInput
		}
		if role == 0 {
			t.Backdrop = raw
		} else {
			t.Poster = raw
		}
	}
	for role, source := range [][]byte{t.Backdrop, t.Poster} {
		if source != nil {
			if err := insertGenreCandidate(ctx, tx, base, value, role+1, t); err != nil {
				return err
			}
		}
	}
	return nil
}
func insertGenreCandidate(ctx context.Context, tx *sql.Tx, base, value int64, role int, t genreTarget) error {
	// Only the maintained <=100 list is inspected, through its compact PK. Rank
	// keys are shared once per retained top target, never duplicated in an index.
	rows, err := tx.QueryContext(ctx, `SELECT c.position,c.entity_id,t.rating,t.sort_key FROM catalog_search_genre_candidates c JOIN catalog_search_genre_targets t ON t.base_id=c.base_id AND t.entity_id=c.entity_id WHERE c.base_id=? AND c.value_id=? AND c.role=? ORDER BY c.position`, base, value, role)
	if err != nil {
		return err
	}
	positions := make([]int64, 0, 100)
	before := int64(0)
	after := int64(0)
	count := 0
	last := int64(0)
	lastEntity := int64(0)
	found := false
	for rows.Next() {
		var position, entity int64
		var r float64
		var key string
		if err = rows.Scan(&position, &entity, &r, &key); err != nil {
			rows.Close()
			return err
		}
		positions = append(positions, position)
		count++
		last, lastEntity = position, entity
		if entity == t.Entity {
			rows.Close()
			return nil
		}
		better := t.Rating > r || t.Rating == r && (t.Order < key || t.Order == key && t.Entity < entity)
		if !found {
			if better {
				after = position
				found = true
			} else {
				before = position
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count == 100 && !found {
		return nil
	}
	if count == 100 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_search_genre_candidates WHERE base_id=? AND value_id=? AND role=? AND position=?`, base, value, role, last); err != nil {
			return err
		}
		positions = positions[:99]
		if _, err = tx.ExecContext(ctx, `DELETE FROM catalog_search_genre_targets WHERE base_id=? AND entity_id=? AND NOT EXISTS(SELECT 1 FROM catalog_search_genre_candidates WHERE base_id=? AND entity_id=?)`, base, lastEntity, base, lastEntity); err != nil {
			return err
		}
	}
	position := before + genrePositionGap
	if after != 0 {
		position = before + (after-before)/2
	}
	if position == before || position == after {
		// Rare gap exhaustion touches <=100 compact rows, never catalogue holders.
		for _, p := range positions {
			if _, err = tx.ExecContext(ctx, `UPDATE catalog_search_genre_candidates SET position=? WHERE base_id=? AND value_id=? AND role=? AND position=?`, -p, base, value, role, p); err != nil {
				return err
			}
		}
		for i, p := range positions {
			next := int64(i+1) * genrePositionGap
			if _, err = tx.ExecContext(ctx, `UPDATE catalog_search_genre_candidates SET position=? WHERE base_id=? AND value_id=? AND role=? AND position=?`, next, base, value, role, -p); err != nil {
				return err
			}
		}
		return insertGenreCandidate(ctx, tx, base, value, role, t)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_search_genre_targets(base_id,entity_id,public_id,kind,rating,sort_key,backdrop,poster) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(base_id,entity_id) DO NOTHING`, base, t.Entity, t.Public, t.Kind, t.Rating, t.Order, t.Backdrop, t.Poster); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO catalog_search_genre_candidates(base_id,value_id,role,position,entity_id) VALUES(?,?,?,?,?)`, base, value, role, position, t.Entity)
	return err
}
