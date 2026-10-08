package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"portico.local/server/internal/storage"
)

// Page rows are retained only for the exact scan/configuration. Discovery refuses
// partial directories; these snapshots never prove absence on their own.
func CommitManagedPage(ctx context.Context, tx *sql.Tx, source, incarnation, job, directory string, entries []storage.Snapshot) error {
	for _, entry := range entries {
		if filepath.Dir(entry.Path) != directory {
			return ErrConflict
		}
		raw, e := json.Marshal(entry)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO subtitle_inventory_entries(source_id,incarnation,job_id,directory,path,snapshot) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,job_id,path) DO UPDATE SET snapshot=excluded.snapshot`, source, incarnation, job, directory, entry.Path, string(raw)); e != nil {
			return e
		}
	}
	return nil
}
func DiscoverManaged(ctx context.Context, db *sql.DB, c *storage.Client, source, incarnation, job, relative, path string, embedded []Track, origin int64, known bool) (*Inventory, error) {
	var done int
	e := db.QueryRowContext(ctx, `SELECT count(*) FROM inventory_directories d JOIN inventory_runs r ON r.job_id=d.job_id JOIN library_sources s ON s.id=r.source_id WHERE d.job_id=? AND d.relative_path=? AND d.state='done' AND s.id=? AND s.incarnation=? AND s.generation=r.source_generation AND s.enabled=1`, job, relative, source, incarnation).Scan(&done)
	if e != nil {
		return nil, e
	}
	if done != 1 {
		return &Inventory{Status: "unavailable"}, nil
	}
	rows, e := db.QueryContext(ctx, `SELECT snapshot FROM subtitle_inventory_entries WHERE source_id=? AND incarnation=? AND job_id=? AND directory=? ORDER BY path LIMIT 4097`, source, incarnation, job, filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	var entries []storage.Snapshot
	for rows.Next() {
		var raw string
		var v storage.Snapshot
		if e = rows.Scan(&raw); e != nil {
			break
		}
		if e = json.Unmarshal([]byte(raw), &v); e != nil {
			break
		}
		entries = append(entries, v)
	}
	rowError := rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if rowError != nil {
		return nil, rowError
	}
	return discover(ctx, c, source, path, embedded, origin, known, func(visit func(storage.Snapshot) error) error {
		for _, v := range entries {
			if e := visit(v); e != nil {
				return e
			}
		}
		return nil
	})
}
