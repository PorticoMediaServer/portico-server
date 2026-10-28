package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diskspace"
)

// ErrRecoveryNoRoom is a damaged candidate that could be salvaged, but not
// with the free space there is: salvage writes a second copy beside it.
var ErrRecoveryNoRoom = errors.New("there is not enough free space to recover this damaged database")

// Salvage bounds: rows read per window, consecutive unreadable steps in one
// table before the rest of it is given up (each step doubles, so 64 cover any
// rowid range), and passes removing rows whose references were lost.
const (
	salvageWindow       = 64
	salvageTableFaults  = 64
	salvageOrphanPasses = 16
)

// salvage rebuilds dir/name from every row that can still be read: a fresh
// copy of this baseline is installed beside it, the damaged file is attached
// and copied table by table in key order, an unreadable stretch of a table is
// skipped, rows left pointing at something lost are removed, and the result
// must pass a full integrity check before it replaces the original. Triggers
// are set aside while rows are copied, so copying reproduces the damaged
// file's rows and adds nothing of its own; full-text indexes are rebuilt from
// what was recovered.
func salvage(ctx context.Context, dir, name string) (*Recovery, error) {
	path := filepath.Join(dir, filepath.FromSlash(name))
	info, err := os.Stat(path)
	if err != nil {
		return nil, ErrCandidateIntegrity
	}
	if free, err := diskspace.Free(dir); err != nil || free < info.Size()+info.Size()/10+(64<<20) {
		return nil, ErrRecoveryNoRoom
	}
	freshName := name + ".salvage"
	fresh := path + ".salvage"
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err = os.Remove(fresh + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	file, err := os.OpenFile(fresh, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	file.Close()
	done := false
	defer func() {
		if !done {
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				_ = os.Remove(fresh + suffix)
			}
		}
	}()
	out, err := salvageInto(ctx, dir, freshName, path)
	if err != nil {
		return nil, err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err = os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err = os.Rename(fresh, path); err != nil {
		return nil, err
	}
	done = true
	return out, nil
}

func salvageInto(ctx context.Context, dir, freshName, damaged string) (*Recovery, error) {
	out := &Recovery{Salvaged: true}
	db, err := dbwork.OpenCandidate(dir, freshName, dbwork.CandidateOptions{ForeignKeysOff: true})
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		return nil, err
	}
	if err = db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, baselineSQL); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES(?,?),(?,?)`, schemaVersionKey, "1", baselineDigestKey, BaselineDigest())
		return err
	}); err != nil {
		return nil, fmt.Errorf("install baseline: %w", err)
	}
	triggers, err := setTriggersAside(ctx, db)
	if err != nil {
		return nil, err
	}
	source := (&url.URL{Scheme: "file", Path: filepath.ToSlash(damaged)}).String()
	if _, err = db.ExecContext(ctx, `ATTACH DATABASE ? AS damaged`, source); err != nil {
		return nil, ErrCandidateIntegrity
	}
	tables, err := salvageTables(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		whole, err := copyTable(ctx, db, table)
		if err != nil {
			return nil, err
		}
		if !whole {
			out.DamagedTables = append(out.DamagedTables, table.name)
		}
	}
	if _, err = db.ExecContext(ctx, `DETACH DATABASE damaged`); err != nil {
		return nil, err
	}
	if out.RemovedRows, err = removeLostReferences(ctx, db); err != nil {
		return nil, err
	}
	if err = rebuildTextIndexes(ctx, db); err != nil {
		return nil, err
	}
	for _, trigger := range triggers {
		if _, err = db.ExecContext(ctx, trigger); err != nil {
			return nil, err
		}
	}
	lines, err := db.Check(ctx, false, 1)
	if err != nil || !dbwork.Healthy(lines) {
		return nil, ErrCandidateIntegrity
	}
	if err = db.Fold(ctx); err != nil {
		return nil, err
	}
	sort.Strings(out.DamagedTables)
	return out, db.Close()
}

// setTriggersAside drops every trigger and returns the statements that
// recreate them, in their original order.
func setTriggersAside(ctx context.Context, db *dbwork.Candidate) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name,sql FROM sqlite_schema WHERE type='trigger' ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	var names, statements []string
	for rows.Next() {
		var name, statement string
		if err = rows.Scan(&name, &statement); err != nil {
			rows.Close()
			return nil, err
		}
		names, statements = append(names, name), append(statements, statement)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	for _, name := range names {
		if _, err = db.ExecContext(ctx, `DROP TRIGGER `+quoteIdentifier(name)); err != nil {
			return nil, err
		}
	}
	return statements, nil
}

type salvageTable struct {
	name    string
	columns []string
	keys    []string // nil for a rowid table
}

// salvageTables lists the fresh baseline's ordinary tables, full-text shadow
// tables included (their rows are the index), with the columns a copy writes.
func salvageTables(ctx context.Context, db *dbwork.Candidate) ([]salvageTable, error) {
	rows, err := db.QueryContext(ctx, `SELECT name,sql FROM main.sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	type entry struct{ name, sql string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.name, &e.sql); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, e)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	out := make([]salvageTable, 0, len(entries))
	for _, e := range entries {
		table := salvageTable{name: e.name}
		withoutRowid := strings.Contains(strings.ToUpper(e.sql), "WITHOUT ROWID")
		info, err := db.QueryContext(ctx, `SELECT name,hidden,pk FROM pragma_table_xinfo(?,'main') ORDER BY cid`, e.name)
		if err != nil {
			return nil, err
		}
		type key struct {
			name  string
			order int
		}
		var keys []key
		for info.Next() {
			var column string
			var hidden, pk int
			if err = info.Scan(&column, &hidden, &pk); err != nil {
				info.Close()
				return nil, err
			}
			if hidden == 2 || hidden == 3 {
				continue // generated
			}
			table.columns = append(table.columns, column)
			if pk > 0 {
				keys = append(keys, key{column, pk})
			}
		}
		if err = errors.Join(info.Err(), info.Close()); err != nil {
			return nil, err
		}
		if withoutRowid {
			sort.Slice(keys, func(i, j int) bool { return keys[i].order < keys[j].order })
			for _, k := range keys {
				table.keys = append(table.keys, k.name)
			}
		}
		out = append(out, table)
	}
	return out, nil
}

// copyTable copies every readable row of one table, in key order, a window at
// a time. Past a window that cannot be read, a rowid table is read a row at a
// time up to the damage and then stepped over it, the step doubling until a
// row reads again; a table keyed otherwise gives up the rest of it. It reports whether the whole
// table was copied.
func copyTable(ctx context.Context, db *dbwork.Candidate, table salvageTable) (bool, error) {
	columns := quoteIdentifiers(table.columns)
	keyed := table.keys != nil
	keys := []string{"rowid"}
	if keyed {
		keys = table.keys
	}
	keyList := strings.Join(quoteIdentifiers(keys), ",")
	selected := keyList + "," + strings.Join(columns, ",")
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	where := ""
	if keyed {
		where = "(" + keyList + ")>(" + placeholders + ")"
	} else {
		where = "rowid>?"
	}
	name := quoteIdentifier(table.name)
	read := `SELECT ` + selected + ` FROM damaged.` + name + ` WHERE ` + where + ` ORDER BY ` + keyList + ` LIMIT ` + fmt.Sprint(salvageWindow)
	first := `SELECT ` + selected + ` FROM damaged.` + name + ` ORDER BY ` + keyList + ` LIMIT ` + fmt.Sprint(salvageWindow)
	insertColumns := strings.Join(columns, ",")
	values := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	if !keyed {
		insertColumns, values = "rowid,"+insertColumns, "?,"+values
	}
	// A copied row replaces the fresh baseline's seeded one (settings
	// singletons keep the owner's values). A row the schema itself refuses (a
	// CHECK, or NOT NULL) is left out and reported.
	insert := `INSERT OR REPLACE INTO main.` + name + `(` + insertColumns + `) VALUES(` + values + `)`

	var last []any
	whole := true
	window := salvageWindow
	faults, skip, steady := 0, int64(0), 0
	for {
		query := read
		args := last
		if last == nil {
			query, args = first, nil
		}
		if window != salvageWindow {
			query = strings.Replace(query, " LIMIT "+fmt.Sprint(salvageWindow), " LIMIT 1", 1)
		}
		rows, err := readWindow(ctx, db, query, args, len(keys)+len(table.columns))
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			whole = false
			if keyed {
				return false, nil
			}
			// Narrow to single rows to keep everything before the damage,
			// then step past it, doubling the step until a row reads again.
			if window != 1 {
				window = 1
				continue
			}
			if faults++; faults > salvageTableFaults {
				return false, nil
			}
			skip = max(1, skip*2)
			from := int64(0)
			if last != nil {
				from, _ = last[0].(int64)
			}
			last = []any{from + skip}
			continue
		}
		if len(rows) == 0 {
			return whole, nil
		}
		if window == 1 {
			faults, skip = 0, 0
			if steady++; steady >= salvageWindow {
				window, steady = salvageWindow, 0
			}
		}
		err = db.WithTx(ctx, func(tx *sql.Tx) error {
			statement, err := tx.PrepareContext(ctx, insert)
			if err != nil {
				return err
			}
			defer statement.Close()
			for _, row := range rows {
				args := row[len(keys):]
				if !keyed {
					args = row
				}
				if _, err = statement.ExecContext(ctx, args...); err != nil {
					if !constraintFailure(err) {
						return err
					}
					whole = false
				}
			}
			return nil
		})
		if err != nil {
			return false, err
		}
		last = rows[len(rows)-1][:len(keys)]
	}
}

func readWindow(ctx context.Context, db *dbwork.Candidate, query string, after []any, width int) ([][]any, error) {
	rows, err := db.QueryContext(ctx, query, after...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		row := make([]any, width)
		targets := make([]any, width)
		for i := range row {
			targets[i] = &row[i]
		}
		if err = rows.Scan(targets...); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// removeLostReferences deletes rows whose foreign keys name a row that was
// lost, repeating while a deletion leaves others pointing at nothing.
func removeLostReferences(ctx context.Context, db *dbwork.Candidate) (int64, error) {
	var removed int64
	for pass := 0; pass < salvageOrphanPasses; pass++ {
		rows, err := db.QueryContext(ctx, `SELECT DISTINCT "table",fkid FROM pragma_foreign_key_check`)
		if err != nil {
			return removed, err
		}
		type violation struct {
			table string
			fk    int
		}
		var found []violation
		for rows.Next() {
			var v violation
			if err = rows.Scan(&v.table, &v.fk); err != nil {
				rows.Close()
				return removed, err
			}
			found = append(found, v)
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return removed, err
		}
		if len(found) == 0 {
			return removed, nil
		}
		for _, v := range found {
			statement, err := orphanDelete(ctx, db, v.table, v.fk)
			if err != nil {
				return removed, err
			}
			result, err := db.ExecContext(ctx, statement)
			if err != nil {
				return removed, err
			}
			n, _ := result.RowsAffected()
			removed += n
		}
	}
	return removed, nil
}

// orphanDelete builds the statement deleting table's rows whose foreign key
// fk names no parent row.
func orphanDelete(ctx context.Context, db *dbwork.Candidate, table string, fk int) (string, error) {
	rows, err := db.QueryContext(ctx, `SELECT "table","from","to" FROM pragma_foreign_key_list(?) WHERE id=? ORDER BY seq`, table, fk)
	if err != nil {
		return "", err
	}
	var parent string
	var from, to []string
	for rows.Next() {
		var p, f string
		var t sql.NullString
		if err = rows.Scan(&p, &f, &t); err != nil {
			rows.Close()
			return "", err
		}
		parent = p
		from, to = append(from, f), append(to, t.String)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return "", err
	}
	if len(from) == 0 {
		return "", fmt.Errorf("salvage: no foreign key %d on %s", fk, table)
	}
	if to[0] == "" {
		// The key names the parent's primary key.
		keys, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) WHERE pk>0 ORDER BY pk`, parent)
		if err != nil {
			return "", err
		}
		to = to[:0]
		for keys.Next() {
			var name string
			if err = keys.Scan(&name); err != nil {
				keys.Close()
				return "", err
			}
			to = append(to, name)
		}
		if err = errors.Join(keys.Err(), keys.Close()); err != nil {
			return "", err
		}
		if len(to) == 0 {
			to = []string{"rowid"}
		}
	}
	var present, match []string
	for i := range from {
		present = append(present, "c."+quoteIdentifier(from[i])+" IS NOT NULL")
		match = append(match, "p."+quoteIdentifier(to[min(i, len(to)-1)])+"=c."+quoteIdentifier(from[i]))
	}
	return `DELETE FROM main.` + quoteIdentifier(table) + ` AS c WHERE ` + strings.Join(present, " AND ") +
		` AND NOT EXISTS(SELECT 1 FROM main.` + quoteIdentifier(parent) + ` AS p WHERE ` + strings.Join(match, " AND ") + `)`, nil
}

// rebuildTextIndexes makes every full-text index agree with the recovered
// rows: an index over another table is rebuilt from it; one that keeps its
// own copy is checked and rebuilt from that if needed; a contentless index
// that fails its check is emptied, and the catalogue's own work fills it again.
func rebuildTextIndexes(ctx context.Context, db *dbwork.Candidate) error {
	rows, err := db.QueryContext(ctx, `SELECT name,sql FROM sqlite_schema WHERE type='table' AND sql LIKE 'CREATE VIRTUAL TABLE%USING fts5%'`)
	if err != nil {
		return err
	}
	type index struct{ name, sql string }
	var indexes []index
	for rows.Next() {
		var i index
		if err = rows.Scan(&i.name, &i.sql); err != nil {
			rows.Close()
			return err
		}
		indexes = append(indexes, i)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, i := range indexes {
		name := quoteIdentifier(i.name)
		external := strings.Contains(i.sql, "content='") && !strings.Contains(i.sql, "content=''")
		contentless := strings.Contains(i.sql, "content=''")
		if external {
			if _, err = db.ExecContext(ctx, `INSERT INTO `+name+`(`+name+`) VALUES('rebuild')`); err != nil {
				return err
			}
			continue
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO `+name+`(`+name+`) VALUES('integrity-check')`); err == nil {
			continue
		}
		command := "rebuild"
		if contentless {
			command = "delete-all"
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO `+name+`(`+name+`) VALUES('`+command+`')`); err != nil {
			return err
		}
	}
	return nil
}

// constraintFailure is SQLite's SQLITE_CONSTRAINT, for any constraint.
func constraintFailure(err error) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code()&255 == 19
}

func quoteIdentifier(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func quoteIdentifiers(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = quoteIdentifier(name)
	}
	return out
}
