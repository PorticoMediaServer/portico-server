package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"

	"portico.local/server/internal/dbwork"
)

// stateDirectoryKey records the state folder a database was last opened from,
// so a copied state folder rebases its absolute paths on first start.
const stateDirectoryKey = "state_directory"

// rebasedColumn is one stored path that lives under the state folder.
type rebasedColumn struct {
	table, column string
}

// rebasedColumns are Portico's own files, referenced absolute to the state
// folder and rewritten when the state folder moves. Media library paths are
// absolute too, but they describe the owner's media rather than Portico's own
// files, so they keep pointing where they point until the owner changes them
// in the console.
var rebasedColumns = []rebasedColumn{
	{"libraries", "root"},
	{"library_sources", "root"},
	{"library_sources", "configured_root"},
	{"assets", "path"},
	{"managed_mounts", "mount_path"},
	{"mount_backend_configs", "config_file"},
	{"playback_artifacts", "directory"},
	{"playback_preparation_selections", "root_path"},
	{"playback_preparation_selections", "source_path"},
	{"playback_subtitle_tracks", "playlist_file"},
	{"subtitle_sidecars", "path"},
	{"subtitle_inventory_entries", "directory"},
	{"subtitle_inventory_entries", "path"},
	{"scan_queue", "path"},
	{"metadata_operation_sources", "path"},
	{"mb_operation_sources", "path"},
}

// RebaseStatePaths runs after Open: when the state folder moved since this
// database last started, it rewrites the old prefix to the new one in one
// gated transaction, only for values equal to the old folder or starting with
// the old folder plus a separator. Storage stays absolute; the move is what
// repairs it. A database that never recorded its folder only records this one.
func RebaseStatePaths(ctx context.Context, db *sql.DB, state string) error {
	state = filepath.Clean(state)
	var stored string
	err := db.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key=?`, stateDirectoryKey).Scan(&stored)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && filepath.Clean(stored) == state {
		return nil
	}
	if err == nil && stored != "" {
		old := filepath.Clean(stored)
		gated, err := dbwork.Begin(ctx, db, dbwork.ClassMaintenance)
		if err != nil {
			return err
		}
		tx := gated.Tx()
		defer gated.Rollback()
		for _, col := range rebasedColumns {
			if err = rebaseColumn(ctx, tx, old, state, col); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, stateDirectoryKey, state); err != nil {
			return err
		}
		if err = gated.Commit(); err != nil {
			return err
		}
		return nil
	}
	_, err = dbwork.ExecWrite(ctx, db, dbwork.ClassMaintenance, `INSERT INTO configuration(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, stateDirectoryKey, state)
	return err
}

// rebaseColumn rewrites one column's state-folder prefix. A missing table or
// column (a database from before it existed) has nothing to rewrite.
func rebaseColumn(ctx context.Context, tx *sql.Tx, old, state string, col rebasedColumn) error {
	var probe int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info(?) WHERE name=?`, col.table, col.column).Scan(&probe); err != nil || probe != 1 {
		return err
	}
	sep := string(filepath.Separator)
	pattern := escapeLike(old + sep)
	// The identifier comes from the fixed table above, never from a caller.
	// length(?)+1 keeps the cut on a character boundary whatever bytes the
	// old folder name holds.
	statement := `UPDATE "` + col.table + `" SET "` + col.column + `"=?||substr("` + col.column + `",length(?)+1) WHERE "` + col.column + `"=? OR "` + col.column + `" LIKE ? ESCAPE '\'`
	_, err := tx.ExecContext(ctx, statement, state, old, old, pattern+"%")
	return err
}

// escapeLike quotes a LIKE pattern's wildcards. State folders routinely hold
// spaces and brackets; a folder with % or _ must still match literally.
func escapeLike(raw string) string {
	out := strings.Builder{}
	for _, r := range raw {
		if r == '%' || r == '_' || r == '\\' {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}
