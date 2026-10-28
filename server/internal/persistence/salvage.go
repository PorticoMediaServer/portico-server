package persistence

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
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

// Salvage bounds: rows read per window, and midpoint probes into the
// damaged gaps of one table (a bisection needs about 64 per gap).
const (
	salvageWindow = 64
	salvageProbes = 1024
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
	// The fresh file is scratch until it passes its final check; it is
	// synced once, after that, rather than at every window's commit.
	for _, pragma := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=OFF`} {
		if _, err = db.ExecContext(ctx, pragma); err != nil {
			return nil, err
		}
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
	if err = db.Close(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, filepath.FromSlash(freshName)), os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	err = file.Sync()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return out, err
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

// copyTable copies every readable row of one table and reports whether that
// was all of it. It reads forward in key order a window at a time, narrowing
// to single rows where a window fails, so everything before damage is kept,
// then backwards from the end of the table down to the damage. What lies
// between is a gap of unknown size that may hold readable pages among damaged
// ones: it is bisected on the first key column (rowid, or the first column of
// the primary key). A readable row found at a midpoint is copied with every
// readable row on either side of it, and the parts of the gap still unread
// become new gaps. Re-reading a row is harmless (copies replace), and a probe
// budget bounds the work on a badly damaged table.
func copyTable(ctx context.Context, db *dbwork.Candidate, table salvageTable) (bool, error) {
	c := newTableCopy(db, table)
	_, last, faulted, err := c.run(ctx, nil, nil, false, "", nil)
	if err != nil || !faulted {
		return !c.rejected, err
	}
	_, low, faulted, err := c.run(ctx, nil, last, true, "", nil)
	if err != nil || !faulted {
		return false, err
	}
	type gap struct{ lo, hi any } // first-column values; nil lo is the table's start
	var lo, hi any
	if last != nil {
		lo = last[0]
	}
	if low == nil {
		return false, nil // damage at the very end: nothing was read backwards
	}
	hi = low[0]
	gaps := []gap{{lo, hi}}
	for budget := salvageProbes; len(gaps) > 0 && budget > 0; budget-- {
		g := gaps[len(gaps)-1]
		gaps = gaps[:len(gaps)-1]
		mid, ok := midpoint(g.lo, g.hi)
		if !ok {
			continue
		}
		// Forward from the midpoint, within the gap.
		first, end, faulted, err := c.run(ctx, nil, nil, false, c.column+`>=? AND `+c.column+`<=?`, []any{mid, g.hi})
		if err != nil {
			return false, err
		}
		if first == nil {
			if faulted {
				gaps = append(gaps, gap{mid, g.hi}) // damage at the midpoint itself
			}
			gaps = append(gaps, gap{g.lo, mid})
			continue
		}
		if faulted {
			gaps = append(gaps, gap{end[0], g.hi})
		}
		// Backward from the first row found, down to the bottom of the gap.
		extra, args := c.column+`>=?`, []any{g.lo}
		if g.lo == nil {
			extra, args = "", nil
		}
		_, bottom, faulted, err := c.run(ctx, first, nil, true, extra, args)
		if err != nil {
			return false, err
		}
		if faulted {
			gaps = append(gaps, gap{g.lo, bottom[0]})
		}
	}
	return false, nil
}

// midpoint is a value strictly between lo and hi of the same SQLite type (nil
// lo is unbounded below), or false when there is none.
func midpoint(lo, hi any) (any, bool) {
	switch h := hi.(type) {
	case int64:
		l, ok := lo.(int64)
		if lo == nil {
			l, ok = math.MinInt64, true
		}
		if !ok || h <= l {
			return nil, false
		}
		span := uint64(h) - uint64(l) // exact even when h-l overflows int64
		if span < 2 {
			return nil, false
		}
		return l + int64(span/2), true
	case float64:
		l, ok := lo.(float64)
		if lo == nil {
			l, ok = -math.MaxFloat64, true
		}
		m := l/2 + h/2
		return m, ok && m > l && m < h
	case string:
		l, ok := lo.(string)
		if lo == nil {
			ok = true
		}
		m, between := midBytes([]byte(l), []byte(h))
		return string(m), ok && between
	case []byte:
		l, ok := lo.([]byte)
		if lo == nil {
			ok = true
		}
		m, between := midBytes(l, h)
		return m, ok && between
	}
	return nil, false
}

// midBytes is a byte string strictly between a and b in memcmp order (the
// order SQLite uses for BLOBs and for TEXT under its default collation).
func midBytes(a, b []byte) ([]byte, bool) {
	if bytes.Compare(a, b) >= 0 {
		return nil, false
	}
	n := max(len(a), len(b)) + 1
	pad := func(v []byte) []int {
		out := make([]int, n)
		for i := range out {
			if i < len(v) {
				out[i] = int(v[i])
			}
		}
		return out
	}
	x, y := pad(a), pad(b)
	// (x+y)/2 in base 256, most significant byte first.
	sum := make([]int, n+1)
	for i := n - 1; i >= 0; i-- {
		sum[i+1] += x[i] + y[i]
		sum[i] += sum[i+1] / 256
		sum[i+1] %= 256
	}
	mid := make([]byte, n)
	carry := 0
	for i := 0; i <= n; i++ {
		v := carry*256 + sum[i]
		if i > 0 {
			mid[i-1] = byte(v / 2)
		}
		carry = v % 2
	}
	if trimmed := bytes.TrimRight(mid, "\x00"); bytes.Compare(trimmed, a) > 0 {
		mid = trimmed
	}
	return mid, bytes.Compare(mid, a) > 0 && bytes.Compare(mid, b) < 0
}

type tableCopy struct {
	db     *dbwork.Candidate
	keyed  bool
	keys   int
	width  int
	column string
	from   string
	order  string
	key    string
	slots  string
	insert string
	// rejected is set when the schema refused a readable row.
	rejected bool
}

func newTableCopy(db *dbwork.Candidate, table salvageTable) *tableCopy {
	columns := quoteIdentifiers(table.columns)
	keys := []string{"rowid"}
	if table.keys != nil {
		keys = table.keys
	}
	keyList := strings.Join(quoteIdentifiers(keys), ",")
	key, slots := keyList, "?"
	if len(keys) > 1 {
		// A row value for a composite key. A single key stays bare: SQLite
		// does not seek on a parenthesised one, and would rescan from the
		// start of the table (into the damage) on every read.
		key, slots = "("+keyList+")", "("+strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")+")"
	}
	insertColumns := strings.Join(columns, ",")
	values := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	if table.keys == nil {
		insertColumns, values = "rowid,"+insertColumns, "?,"+values
	}
	name := quoteIdentifier(table.name)
	return &tableCopy{
		db: db, keyed: table.keys != nil, keys: len(keys), width: len(keys) + len(columns), column: quoteIdentifier(keys[0]),
		from:  `SELECT ` + selectTyped(append(quoteIdentifiers(keys), columns...)) + ` FROM damaged.` + name,
		order: keyList, key: key, slots: slots,
		// A copied row replaces the fresh baseline's seeded one (settings
		// singletons keep the owner's values). A row the schema itself
		// refuses (a CHECK, or NOT NULL) is left out and reported.
		insert: `INSERT OR REPLACE INTO main.` + name + `(` + insertColumns + `) VALUES(` + values + `)`,
	}
}

// run copies rows strictly between lower and upper keys (either may be nil)
// that also satisfy extra, in ascending or descending key order, until the
// range ends or a single row cannot be read. It returns the first and last
// keys it copied (nil when none) and whether it stopped at damage. The
// returned last key is lower (ascending) or upper (descending) when nothing
// was copied.
func (c *tableCopy) run(ctx context.Context, upper, lower []any, descending bool, extra string, extraArgs []any) (first, last []any, faulted bool, err error) {
	window := salvageWindow
	at := lower
	if descending {
		at = upper
	}
	last = at
	direction := ""
	if descending {
		direction = " DESC"
	}
	order := ` ORDER BY ` + strings.ReplaceAll(c.order, ",", direction+",") + direction
	for {
		var where []string
		args := []any{}
		bound := func(op string, key []any) {
			if key != nil {
				where = append(where, c.key+op+c.slots)
				args = append(args, key...)
			}
		}
		if descending {
			bound("<", at)
			bound(">", lower)
		} else {
			bound(">", at)
			bound("<", upper)
		}
		if extra != "" {
			where = append(where, extra)
			args = append(args, extraArgs...)
		}
		query := c.from
		if len(where) > 0 {
			query += ` WHERE ` + strings.Join(where, " AND ")
		}
		query += order + ` LIMIT ` + fmt.Sprint(window)
		rows, err := c.read(ctx, query, args)
		if err != nil {
			if ctx.Err() != nil {
				return first, last, false, ctx.Err()
			}
			if window > 1 {
				window = 1
				continue
			}
			return first, last, true, nil
		}
		if len(rows) == 0 {
			return first, last, false, nil
		}
		if err = c.write(ctx, rows); err != nil {
			return first, last, false, err
		}
		if first == nil {
			first = rows[0][:c.keys]
		}
		at = rows[len(rows)-1][:c.keys]
		last = at
	}
}

// withEmptyBlobs is the insert with X” in place of each empty BLOB in args,
// and the remaining arguments, or false when there is none.
func (c *tableCopy) withEmptyBlobs(args []any) (string, []any, bool) {
	found := false
	for _, a := range args {
		if b, ok := a.([]byte); ok && len(b) == 0 {
			found = true
		}
	}
	if !found {
		return "", nil, false
	}
	head, _, _ := strings.Cut(c.insert, " VALUES(")
	slots := make([]string, len(args))
	kept := make([]any, 0, len(args))
	for i, a := range args {
		if b, ok := a.([]byte); ok && len(b) == 0 {
			slots[i] = "X''"
			continue
		}
		slots[i] = "?"
		kept = append(kept, a)
	}
	return head + " VALUES(" + strings.Join(slots, ",") + ")", kept, true
}

// selectTyped selects each expression followed, after all of them, by its
// SQLite type, so read can tell an empty BLOB or TEXT from NULL (the driver
// returns nil for all three).
func selectTyped(expressions []string) string {
	typed := make([]string, 0, 2*len(expressions))
	typed = append(typed, expressions...)
	for _, e := range expressions {
		typed = append(typed, "typeof("+e+")")
	}
	return strings.Join(typed, ",")
}

// read reads one window and restores the exact value of every empty BLOB and
// TEXT, so a copied row is the row that was read.
func (c *tableCopy) read(ctx context.Context, query string, args []any) ([][]any, error) {
	rows, err := readWindow(ctx, c.db, query, args, 2*c.width)
	for i, row := range rows {
		for j := 0; j < c.width; j++ {
			if row[j] == nil {
				switch fmt.Sprintf("%s", row[c.width+j]) {
				case "blob":
					row[j] = []byte{}
				case "text":
					row[j] = ""
				}
			}
		}
		rows[i] = row[:c.width]
	}
	return rows, err
}

func (c *tableCopy) write(ctx context.Context, rows [][]any) error {
	return c.db.WithTx(ctx, func(tx *sql.Tx) error {
		statement, err := tx.PrepareContext(ctx, c.insert)
		if err != nil {
			return err
		}
		defer statement.Close()
		for _, row := range rows {
			args := row
			if c.keyed {
				args = row[c.keys:]
			}
			run := statement.ExecContext
			if query, literal, ok := c.withEmptyBlobs(args); ok {
				// The driver binds an empty []byte as NULL; spell it out.
				run = func(ctx context.Context, _ ...any) (sql.Result, error) { return tx.ExecContext(ctx, query, literal...) }
			}
			if _, err = run(ctx, args...); err != nil {
				if !constraintFailure(err) {
					return err
				}
				c.rejected = true
			}
		}
		return nil
	})
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
// lost, repeating while a deletion leaves others pointing at nothing, until
// foreign_key_check is empty. Every pass removes at least one row, so it ends.
func removeLostReferences(ctx context.Context, db *dbwork.Candidate) (int64, error) {
	var removed int64
	for {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
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
		pass := int64(0)
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
			pass += n
		}
		if pass == 0 {
			// Violations that deleting orphans cannot clear: never publish
			// a database whose references dangle.
			return removed, ErrCandidateIntegrity
		}
		removed += pass
	}
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
