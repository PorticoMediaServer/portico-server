package compactcatalog

import (
	"context"
	"database/sql"

	"portico.local/server/internal/dbwork"
)

// ClassifyPendingRatings publishes the minimum age of up to 5000 content-rating
// spellings a trigger queued in content_rating_pending (a rating arrived that
// content_rating_ages does not classify), in one gated write, and reports
// whether more are queued. An unrecognised spelling is classified as unrated
// (-1) rather than silently treated as suitable. Until its spelling is
// classified, visibility treats a title as unrated. Classification is
// idempotent, so overlapping callers only repeat work. age classifies one
// spelling (identity.RatingAge); it is passed in so this package does not
// depend on identity.
func ClassifyPendingRatings(ctx context.Context, db *sql.DB, age func(string) (int, bool)) (bool, error) {
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	type entry struct {
		key string
		age int
	}
	read := dbwork.ReadHandle(ctx, db)
	rows, err := read.QueryContext(ctx, `SELECT value_key,value FROM content_rating_pending LIMIT 5000`)
	if err != nil {
		return false, err
	}
	var pending []entry
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			rows.Close()
			return false, err
		}
		minimum := -1
		if resolved, ok := age(value); ok {
			minimum = resolved
		}
		pending = append(pending, entry{key, minimum})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(pending) > 0 {
		if err = dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			for _, row := range pending {
				if _, err := tx.ExecContext(ctx, `INSERT INTO content_rating_ages(value_key,minimum_age) VALUES(?,?) ON CONFLICT(value_key) DO UPDATE SET minimum_age=excluded.minimum_age`, row.key, row.age); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `DELETE FROM content_rating_pending WHERE value_key=?`, row.key); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return false, err
		}
	}
	var more bool
	err = read.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM content_rating_pending)`).Scan(&more)
	return more, err
}

// ReclassifyRatings judges every spelling already classified against the
// current rating table and corrects the ones whose age changed. It is run once
// after the table changes (the caller remembers which table it last applied),
// so a title rated in a spelling the table has since learned stops counting as
// unrated without a rescan. The table of spellings is small: one row per
// distinct rating in the library, not per title.
func ReclassifyRatings(ctx context.Context, db *sql.DB, age func(string) (int, bool)) error {
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	rows, err := dbwork.ReadHandle(ctx, db).QueryContext(ctx, `SELECT value_key,minimum_age FROM content_rating_ages`)
	if err != nil {
		return err
	}
	changed := map[string]int{}
	for rows.Next() {
		var key string
		var stored int
		if err = rows.Scan(&key, &stored); err != nil {
			rows.Close()
			return err
		}
		minimum := -1
		if resolved, ok := age(key); ok {
			minimum = resolved
		}
		if minimum != stored {
			changed[key] = minimum
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(changed) == 0 {
		return err
	}
	return dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		for key, minimum := range changed {
			if _, err := tx.ExecContext(ctx, `UPDATE content_rating_ages SET minimum_age=? WHERE value_key=?`, minimum, key); err != nil {
				return err
			}
		}
		return nil
	})
}
