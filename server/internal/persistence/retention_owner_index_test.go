package persistence

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// P2-59 O022 adds owner-walk indexes so a title/container lookup is a bounded
// seek instead of a profile-first scan:
//
//	personal_items(item_id,profile_id)
//	reading_progress(item_id,owner_key,asset_id)
//	personal_watched_intents(item_id,profile_id)
//	container_personal_state(container_id,profile_id,kind)
//
// The cost of that seek is one index entry maintained per write. A position or
// progress write is the hot path, so it must not assign any column the index
// keys on. SQLite maintains the index for an assignment even when the new value
// equals the old one, so the check is the presence of the column in the SET
// list, never a comparison of values. The insert path is different: it names
// the columns it writes, and every key is written there by definition.
type ownerWalkIndex struct {
	table   string
	name    string
	columns []string
}

var ownerWalkIndexes = []ownerWalkIndex{
	{table: "personal_items", name: "personal_items_retention_owner", columns: []string{"item_id", "profile_id"}},
	{table: "reading_progress", name: "reading_progress_retention_owner", columns: []string{"item_id", "owner_key", "asset_id"}},
	{table: "personal_watched_intents", name: "personal_watched_intents_retention_owner", columns: []string{"item_id", "profile_id"}},
	{table: "container_personal_state", name: "container_personal_state_retention_owner", columns: []string{"container_id", "profile_id", "kind"}},
}

// progress is not proposed here: progress_item_state already has exactly the
// owner-walk shape. It is censused as the reference for the position update
// path, which is the write this ruling exists to protect.
var ownerWalkReferenceKeys = map[string][]string{
	"progress": {"item_id", "profile_id"},
}

var ownerWalkTables = func() map[string]map[string]bool {
	tables := map[string]map[string]bool{}
	add := func(table string, columns []string) {
		if tables[table] == nil {
			tables[table] = map[string]bool{}
		}
		for _, column := range columns {
			tables[table][column] = true
		}
	}
	for _, index := range ownerWalkIndexes {
		add(index.table, index.columns)
	}
	for table, columns := range ownerWalkReferenceKeys {
		add(table, columns)
	}
	return tables
}()

var (
	ownerWalkInsert   = regexp.MustCompile(`(?is)\bINSERT\s+(?:OR\s+(?:ABORT|FAIL|IGNORE|REPLACE|ROLLBACK)\s+)?INTO\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(([^()]*)\))?`)
	ownerWalkUpdate   = regexp.MustCompile(`(?is)\bUPDATE\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s+(?:AS\s+)?[A-Za-z_][A-Za-z0-9_]*)?\s+SET\s+`)
	ownerWalkConflict = regexp.MustCompile(`(?is)\bON\s+CONFLICT\s*(?:\([^()]*\))?\s+DO\s+UPDATE\s+SET\s+`)
)

type ownerWalkWrite struct {
	kind    string
	table   string
	where   string
	text    string
	columns []string
}

// ownerWalkCensus reads every production statement that writes one of the
// censused tables. inserts carries the INSERT column list; updates carries the
// columns assigned by UPDATE ... SET and by ON CONFLICT ... DO UPDATE SET.
func ownerWalkCensus(statements []productionStatement) (inserts, updates []ownerWalkWrite, problems []string) {
	for _, statement := range statements {
		text := strings.Join(strings.Fields(statement.text), " ")
		for _, match := range ownerWalkInsert.FindAllStringSubmatchIndex(text, -1) {
			table := strings.ToLower(text[match[2]:match[3]])
			if _, hot := ownerWalkTables[table]; !hot {
				continue
			}
			write := ownerWalkWrite{kind: "insert", table: table, where: statement.where, text: text}
			if match[4] < 0 {
				problems = append(problems, fmt.Sprintf("%s: INSERT INTO %s writes without a column list, so the insert path cannot be censused: %s", statement.where, table, text))
			} else {
				write.columns = ownerWalkColumnList(text[match[4]:match[5]])
			}
			inserts = append(inserts, write)
		}
		for _, match := range ownerWalkUpdate.FindAllStringSubmatchIndex(text, -1) {
			table := strings.ToLower(text[match[2]:match[3]])
			if _, hot := ownerWalkTables[table]; !hot {
				continue
			}
			write := ownerWalkWrite{kind: "update", table: table, where: statement.where, text: text}
			write.columns = ownerWalkAssignedColumns(text[match[1]:])
			if len(write.columns) == 0 {
				problems = append(problems, fmt.Sprintf("%s: UPDATE %s has no readable SET assignment: %s", statement.where, table, text))
			}
			updates = append(updates, write)
		}
		for _, match := range ownerWalkConflict.FindAllStringSubmatchIndex(text, -1) {
			table := ownerWalkInsertTableBefore(text, match[0])
			if table == "" {
				// Not attributable inside this literal (for example a fragment
				// assembled at run time); a hot upsert always carries its own
				// INSERT, which the insert census above would have seen.
				continue
			}
			if _, hot := ownerWalkTables[table]; !hot {
				continue
			}
			write := ownerWalkWrite{kind: "conflict update", table: table, where: statement.where, text: text}
			write.columns = ownerWalkAssignedColumns(text[match[1]:])
			if len(write.columns) == 0 {
				problems = append(problems, fmt.Sprintf("%s: ON CONFLICT DO UPDATE on %s has no readable SET assignment: %s", statement.where, table, text))
			}
			updates = append(updates, write)
		}
	}
	return inserts, updates, problems
}

func ownerWalkInsertTableBefore(text string, before int) string {
	table := ""
	for _, match := range ownerWalkInsert.FindAllStringSubmatchIndex(text, -1) {
		if match[0] >= before {
			break
		}
		table = strings.ToLower(text[match[2]:match[3]])
	}
	return table
}

func ownerWalkColumnList(list string) []string {
	var columns []string
	for _, column := range strings.Split(list, ",") {
		column = strings.ToLower(strings.Trim(strings.TrimSpace(column), "\"`[]"))
		if column != "" {
			columns = append(columns, column)
		}
	}
	return columns
}

// ownerWalkAssignedColumns reads the left-hand sides of a SET list up to the
// first top-level WHERE or RETURNING. It deliberately discards the right-hand
// side, so item_id=item_id and item_id=excluded.item_id both count.
func ownerWalkAssignedColumns(text string) []string {
	text = ownerWalkUntil(text, "WHERE", "RETURNING")
	var columns []string
	for _, part := range ownerWalkSplit(text) {
		part = strings.TrimSpace(part)
		equals := strings.Index(part, "=")
		if equals < 0 {
			continue
		}
		left := strings.TrimSpace(part[:equals])
		if dot := strings.LastIndex(left, "."); dot >= 0 {
			left = left[dot+1:]
		}
		left = strings.ToLower(strings.Trim(strings.TrimSpace(left), "\"`[]"))
		if left != "" {
			columns = append(columns, left)
		}
	}
	return columns
}

// ownerWalkUntil cuts at the first keyword outside parentheses and quoted
// literals, so a subquery's WHERE cannot end the assignment list early.
func ownerWalkUntil(text string, keywords ...string) string {
	depth := 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			continue
		case '(':
			depth++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 {
			continue
		}
		for _, keyword := range keywords {
			end := i + len(keyword)
			if end > len(text) || !strings.EqualFold(text[i:end], keyword) {
				continue
			}
			if i > 0 && ownerWalkWordByte(text[i-1]) {
				continue
			}
			if end < len(text) && ownerWalkWordByte(text[end]) {
				continue
			}
			return text[:i]
		}
	}
	return text
}

func ownerWalkWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func ownerWalkSplit(text string) []string {
	var parts []string
	start, depth := 0, 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, text[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, text[start:])
}

func ownerWalkKeys(table string) []string {
	for _, index := range ownerWalkIndexes {
		if index.table == table {
			return append([]string{}, index.columns...)
		}
	}
	return append([]string{}, ownerWalkReferenceKeys[table]...)
}

func ownerWalkHasColumn(columns []string, want string) bool {
	for _, column := range columns {
		if column == want {
			return true
		}
	}
	return false
}

func ownerWalkIdentityAssignments(write ownerWalkWrite) []string {
	var touched []string
	seen := map[string]bool{}
	for _, column := range write.columns {
		if ownerWalkTables[write.table][column] && !seen[column] {
			seen[column] = true
			touched = append(touched, column)
		}
	}
	return touched
}

func TestOwnerWalkIndexesStayOffTheHotWritePath(t *testing.T) {
	statements, _ := productionStatements(t)
	inserts, updates, problems := ownerWalkCensus(statements)
	if len(problems) != 0 {
		t.Fatalf("owner-walk census could not read %d hot write(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
	if len(inserts) == 0 || len(updates) == 0 {
		t.Fatalf("owner-walk census is blind: %d hot INSERT and %d hot UPDATE/conflict writes", len(inserts), len(updates))
	}

	// Insert path: every key must be named, because a row insert writes one
	// entry per index whether or not the statement mentions the column.
	for _, insert := range inserts {
		for _, column := range ownerWalkKeys(insert.table) {
			if !ownerWalkHasColumn(insert.columns, column) {
				t.Errorf("%s: INSERT INTO %s does not name indexed identity column %s, so the insert path cannot be reported exactly: %s", insert.where, insert.table, column, insert.text)
			}
		}
	}

	// Update path: the ruling. Presence of the column is the whole test; a
	// same-value assignment is still an index rewrite.
	for _, update := range updates {
		if touched := ownerWalkIdentityAssignments(update); len(touched) != 0 {
			t.Errorf("%s: %s assigns indexed identity column(s) %s. SQLite maintains the owner-walk index for that assignment even when the value is unchanged; keep identity columns out of SET, or reject/drop the affected index instead of blessing a hot identity rewrite: %s", update.where, update.table, strings.Join(touched, ", "), update.text)
		}
	}

	for _, index := range ownerWalkIndexes {
		insertPath := 0
		for _, insert := range inserts {
			if insert.table != index.table {
				continue
			}
			named := true
			for _, column := range index.columns {
				named = named && ownerWalkHasColumn(insert.columns, column)
			}
			if named {
				insertPath++
			}
		}
		updatePath := 0
		for _, update := range updates {
			if update.table == index.table {
				updatePath++
			}
		}
		if insertPath == 0 {
			t.Errorf("%s(%s): no production INSERT names every key, so the insert-path cost cannot be reported", index.table, strings.Join(index.columns, ","))
		}
		if updatePath == 0 {
			t.Errorf("%s(%s): no production UPDATE or upsert conflict path was censused; the hot write shape is unverified", index.table, strings.Join(index.columns, ","))
		}
		t.Logf("%s(%s): %d INSERT statements write every key (one index entry per inserted row after install); %d UPDATE/DO UPDATE statements assign none of the keys", index.table, strings.Join(index.columns, ","), insertPath, updatePath)
	}
	positionUpdates := 0
	for _, update := range updates {
		if update.table == "progress" && ownerWalkHasColumn(update.columns, "position") {
			positionUpdates++
		}
	}
	if positionUpdates == 0 {
		t.Error("census found no progress position update, so the reference path was not checked")
	}
	t.Logf("reference progress(item_id,profile_id) is progress_item_state: %d position UPDATE statements assign neither key", positionUpdates)

	// Pin the exact production statements, not a mirror: the census above is
	// only meaningful while it sees the canonical writers.
	for _, want := range []struct{ what, where, contains string }{
		{"canonical personal state upsert", "internal/catalog/personal.go:", "INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,not_interested,rating,revision,watched,last_played_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(profile_id,item_id) DO UPDATE SET watchlisted=excluded.watchlisted"},
		{"watch position upsert", "internal/catalog/personal.go:", "INSERT INTO progress(profile_id,item_id,position,playback_id) VALUES(?,?,?,'') ON CONFLICT(profile_id,item_id) DO UPDATE SET position=excluded.position"},
		{"accepted progress position update", "internal/catalog/personal_activity.go:", "UPDATE progress SET position=? WHERE profile_id=? AND item_id=? AND (playback_id=? OR ?=1)"},
		{"container member position reset", "internal/catalog/personal_batch_read.go:", "UPDATE progress SET position=0 WHERE profile_id=? AND item_id IN(SELECT CAST(value AS INTEGER) FROM json_each(?))"},
		{"batch member watched intents", "internal/catalog/personal_batch_read.go:", "INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) SELECT ?,CAST(value AS INTEGER),?,? FROM json_each(?)"},
		{"batch member personal state", "internal/catalog/personal_batch_read.go:", "INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,not_interested,rating,revision,watched,last_played_at) SELECT ?"},
		{"terminal activity personal_items update", "internal/catalog/personal_playback_boundary.go:", "UPDATE personal_items SET last_played_at=? WHERE profile_id=? AND item_id=?"},
		{"accepted reading progress upsert", "internal/reading/progress.go:", "INSERT INTO reading_progress(owner_key,item_id,asset_id,source_revision,format,cfi,section,page_index,progression,revision,observed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(owner_key,item_id,asset_id) DO UPDATE SET source_revision=excluded.source_revision"},
		{"restore reading progress insert", "internal/catalog/reading_retention.go:", "INSERT INTO reading_progress(owner_key,item_id,asset_id,source_revision,format,cfi,section,page_index,progression,revision,observed_at) SELECT ?,?,p.asset_id"},
		{"watch intent write", "internal/personalstate/watched.go:", "INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES(?,?,?,?) ON CONFLICT(profile_id,item_id) DO UPDATE SET watched=excluded.watched,authored_at=excluded.authored_at"},
		{"container watched state upsert", "internal/catalog/container_personal.go:", "INSERT INTO container_personal_state(profile_id,kind,container_id,watched,watermark,revision,cleared_through) VALUES(?,?,?,?,?,?,?) ON CONFLICT(profile_id,kind,container_id) DO UPDATE SET watched=excluded.watched"},
	} {
		if !ownerWalkSees(statements, want.where, want.contains) {
			t.Errorf("census no longer sees the %s writer under %s; the hot write shape changed or the statement moved", want.what, want.where)
		}
	}
}

func ownerWalkSees(statements []productionStatement, where, contains string) bool {
	for _, statement := range statements {
		if !strings.Contains(filepath.ToSlash(statement.where), where) {
			continue
		}
		if strings.Contains(strings.Join(strings.Fields(statement.text), " "), contains) {
			return true
		}
	}
	return false
}

// The detector must fail on a same-value identity assignment and on an
// excluded.* assignment. This is a guard on the guard; the evidence above is
// the production census.
func TestOwnerWalkCensusRejectsIdentityRewrites(t *testing.T) {
	sources := []string{
		`UPDATE personal_items SET item_id=item_id WHERE profile_id=? AND item_id=?`,
		`INSERT INTO reading_progress(owner_key,item_id,asset_id,revision) VALUES(?,?,?,1) ON CONFLICT(owner_key,item_id,asset_id) DO UPDATE SET owner_key=excluded.owner_key`,
	}
	var statements []productionStatement
	for i, source := range sources {
		statements = append(statements, productionStatement{where: fmt.Sprintf("fixture.go:%d", i+1), text: source})
	}
	_, updates, problems := ownerWalkCensus(statements)
	if len(problems) != 0 {
		t.Fatalf("census rejected its own fixture: %v", problems)
	}
	if len(updates) != len(sources) {
		t.Fatalf("census read %d of %d fixture updates", len(updates), len(sources))
	}
	want := [][]string{{"item_id"}, {"owner_key"}}
	for i, update := range updates {
		touched := ownerWalkIdentityAssignments(update)
		if strings.Join(touched, ",") != strings.Join(want[i], ",") {
			t.Errorf("fixture %d assigned identity columns %v; want %v (columns read: %v)", i, touched, want[i], update.columns)
		}
	}
}

func ownerWalkIndexMatches(t *testing.T, db *sql.DB, proposed ownerWalkIndex) []string {
	t.Helper()
	var names []string
	for _, existing := range sqliteIndexes(t, db, proposed.table) {
		if existing.partial || len(existing.columns) != len(proposed.columns) {
			continue
		}
		if sameStrings(existing.columns, proposed.columns) {
			names = append(names, existing.name)
		}
	}
	return names
}

func ensureOwnerWalkIndex(t *testing.T, db *sql.DB, proposed ownerWalkIndex) {
	t.Helper()
	if len(ownerWalkIndexMatches(t, db, proposed)) != 0 {
		return
	}
	statement := "CREATE INDEX " + quoteSQLite(proposed.name) + " ON " + quoteSQLite(proposed.table) + "(" + strings.Join(proposed.columns, ",") + ")"
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("create proposed owner-walk index %s: %v", proposed.name, err)
	}
}

var ownerWalkMissingTable = regexp.MustCompile(`(?i)no such table: (?:\w+\.)?(\w+)`)

// ownerWalkPrepares prepares one hot statement. Only a table the fixture
// statements create at run time excuses a failure; any other error is returned
// instead of being folded into "covered elsewhere".
func ownerWalkPrepares(db *sql.DB, text string, transient map[string]bool) (bool, error) {
	statement, err := db.Prepare(text)
	if err == nil {
		statement.Close()
		return true, nil
	}
	if match := ownerWalkMissingTable.FindStringSubmatch(err.Error()); match != nil && transient["table:"+strings.ToLower(match[1])] {
		return false, nil
	}
	return false, err
}

// The test database is the real baseline. The indexes are created only when no
// equivalent key exists, so this runs before and after the DDL step that
// installs them: if the DDL has landed under any name, this guard uses it and
// does not add a second copy.
func TestOwnerWalkIndexesInstallAndKeepHotWritesPreparable(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "owner-walk-index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	statements, transient := productionStatements(t)
	inserts, updates, problems := ownerWalkCensus(statements)
	if len(problems) != 0 {
		t.Fatalf("owner-walk census could not read %d hot write(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
	for _, proposed := range ownerWalkIndexes {
		ensureOwnerWalkIndex(t, db, proposed)
	}
	for _, proposed := range ownerWalkIndexes {
		matches := ownerWalkIndexMatches(t, db, proposed)
		switch {
		case len(matches) == 0:
			t.Errorf("%s(%s) is not installed in the test database", proposed.table, strings.Join(proposed.columns, ","))
		case len(matches) > 1:
			t.Errorf("%s has %d indexes with key (%s): %s; the DDL must not leave a second copy", proposed.table, len(matches), strings.Join(proposed.columns, ","), strings.Join(matches, ", "))
		}
	}

	// The position writes only compile here: an EXPLAIN opcode count cannot
	// isolate the statement, because triggers on UPDATE OF position
	// (rec_profile_progress_update, selection_predicate_progress_update) add
	// their own index writes to the same program.
	writes := append(append([]ownerWalkWrite{}, inserts...), updates...)
	checked := 0
	for _, write := range writes {
		if strings.Contains(strings.ToUpper(write.text), " INDEXED BY ") {
			// Merge statements name today's single-column indexes. Whether the
			// DDL retires one of those is a separate decision; their
			// assignments are still censused above.
			continue
		}
		// Statements for a temporary fixture table are skipped only when that
		// table is the obstacle; a genuine schema or conflict-target error
		// still fails here (unlike the general guard, which ignores them).
		prepared, err := ownerWalkPrepares(db, write.text, transient)
		if err != nil {
			t.Errorf("%s: hot %s write for %s no longer prepares with the owner-walk indexes installed: %v\n\t%s", write.where, write.kind, write.table, err, write.text)
			continue
		}
		if prepared {
			checked++
		}
	}
	if checked < 8 {
		t.Fatalf("only %d hot writes prepared; the census is broken", checked)
	}
	t.Logf("%d hot production writes prepare unchanged with the four owner-walk indexes installed", checked)
}
