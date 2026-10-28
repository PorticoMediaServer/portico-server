package metadata

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
)

// The accumulator is a private, disposable database with a bounded page cache.
// It never attaches to the catalogue and takes neither its writer gate nor a
// catalogue snapshot. Exact DISTINCT sets (including individual list entries)
// may grow on disk; no selection-size or distinct-value ceiling is imposed.
type bulkPreviewSpool struct {
	db        *sql.DB
	directory string
	locked    map[string]int64
}

func newBulkPreviewSpool(ctx context.Context, root string) (*bulkPreviewSpool, error) {
	if root != "" {
		if err := os.MkdirAll(root, 0700); err != nil {
			return nil, err
		}
	}
	directory, err := os.MkdirTemp(root, "bulk-preview-")
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "counts.sqlite"))
	if err != nil {
		return nil, errors.Join(err, os.RemoveAll(directory))
	}
	db.SetMaxOpenConns(1)
	s := &bulkPreviewSpool{db: db, directory: directory, locked: map[string]int64{}}
	for _, q := range []string{`PRAGMA cache_size=-4096`, `PRAGMA temp_store=FILE`, `PRAGMA journal_mode=DELETE`, `CREATE TABLE counts(field TEXT NOT NULL,entry INTEGER NOT NULL,value TEXT NOT NULL,n INTEGER NOT NULL,PRIMARY KEY(field,entry,value)) WITHOUT ROWID, STRICT`} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			return nil, errors.Join(err, s.close())
		}
	}
	return s, nil
}

func (s *bulkPreviewSpool) close() error {
	return errors.Join(s.db.Close(), os.RemoveAll(s.directory))
}

func (s *bulkPreviewSpool) add(ctx context.Context, observations []bulkPreviewObservation) error {
	// This transaction belongs exclusively to the disposable spool. In particular
	// it must not take dbwork's gate for the persistent catalogue file.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO counts(field,entry,value,n) VALUES(?,?,?,?) ON CONFLICT(field,entry,value) DO UPDATE SET n=n+excluded.n`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, o := range observations {
		if _, err = stmt.ExecContext(ctx, o.field, 0, o.value, o.count); err != nil {
			return err
		}
		// List interpretation is the editor's; repeated entries count once per
		// selected entity. This set is one entity/vector, never a whole selection.
		seen := map[string]bool{}
		for _, entry := range o.entries {
			if !seen[entry] {
				seen[entry] = true
				if _, err = stmt.ExecContext(ctx, o.field, 1, entry, o.count); err != nil {
					return err
				}
			}
		}
		if o.locked {
			s.locked[o.field] += o.count
		}
	}
	return tx.Commit()
}

func (s *bulkPreviewSpool) fields(ctx context.Context, schema []RepairFieldSpec) (map[string]BulkPreviewField, error) {
	out := map[string]BulkPreviewField{}
	for _, spec := range schema {
		field := BulkPreviewField{State: "shared", Values: []BulkPreviewValue{}, Entries: []BulkPreviewValue{}, Locked: s.locked[spec.Field]}
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM counts WHERE field=? AND entry=0`, spec.Field).Scan(&field.DistinctCount); err != nil {
			return nil, err
		}
		if field.DistinctCount > 1 {
			field.State = "mixed"
		}
		for entry := 0; entry <= 1; entry++ {
			rows, err := s.db.QueryContext(ctx, `SELECT value,n FROM counts WHERE field=? AND entry=? ORDER BY value LIMIT ?`, spec.Field, entry, bulkPreviewValues+1)
			if err != nil {
				return nil, err
			}
			values := []BulkPreviewValue{}
			for rows.Next() {
				var v BulkPreviewValue
				if err = rows.Scan(&v.Value, &v.Count); err != nil {
					break
				}
				values = append(values, v)
			}
			if err == nil {
				err = rows.Err()
			}
			closeErr := rows.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return nil, err
			}
			if len(values) > bulkPreviewValues {
				field.Truncated = true
				values = values[:bulkPreviewValues]
			}
			if entry == 0 {
				field.Values = values
			} else {
				field.Entries = values
			}
		}
		if field.State == "shared" && len(field.Values) > 0 {
			value := field.Values[0].Value
			field.Value = &value
		}
		out[spec.Field] = field
	}
	return out, nil
}
