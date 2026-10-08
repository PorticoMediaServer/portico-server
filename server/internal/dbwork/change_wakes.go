package dbwork

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"sync"
)

type changeKey struct{}
type changeSet struct {
	mu      sync.Mutex
	tables  map[string]bool
	unknown bool
}

func (s *changeSet) note(statement string) {
	if s == nil {
		return
	}
	text := strings.ToLower(strings.TrimSpace(statement))
	// A CTE or multi-statement command may write several roots. Preserve safety
	// by broadcasting whenever the bounded head parser cannot prove its scope.
	unknown := strings.HasPrefix(text, "with ") || strings.Contains(strings.TrimSuffix(text, ";"), ";")
	table, ok := writtenTable(text)
	if !ok && !unknown {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unknown = s.unknown || unknown || table == ""
	if table != "" {
		if s.tables == nil {
			s.tables = map[string]bool{}
		}
		s.tables[table] = true
	}
}

// known is every table a write in the set named, even when the set also holds
// a write whose scope couldn't be proven.
func (s *changeSet) known() map[string]bool {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]bool{}
	for table := range s.tables {
		out[table] = true
	}
	return out
}

func (s *changeSet) roots() map[string]bool {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unknown || len(s.tables) == 0 {
		return nil
	}
	out := map[string]bool{}
	for table := range s.tables {
		out[table] = true
	}
	return out
}

var wakeDependencies = struct {
	sync.RWMutex
	edges map[string]map[string]bool
}{edges: map[string]map[string]bool{}}
var mutationTarget = regexp.MustCompile(`(?i)\b(?:INSERT\s+(?:OR\s+\w+\s+)?INTO|REPLACE\s+INTO|UPDATE\s+(?:OR\s+\w+\s+)?|DELETE\s+FROM)\s*["\x60\[]?(\w+)`)
var referenceTarget = regexp.MustCompile(`(?i)\bREFERENCES\s+["\x60\[]?(\w+)`)

// InstallWakeDependencies follows trigger writes and foreign-key edges so a
// worker subscribed to a derived job table hears the original catalog commit.
// This is one schema read on open, never a read on commit or a request path.
func InstallWakeDependencies(ctx context.Context, db *sql.DB) error {
	rows, e := db.QueryContext(ctx, `SELECT type,name,tbl_name,COALESCE(sql,'') FROM sqlite_schema WHERE type IN('table','trigger') AND name NOT LIKE 'sqlite_%'`)
	if e != nil {
		return e
	}
	defer rows.Close()
	type object struct{ kind, name, table, sql string }
	var objects []object
	tables := map[string]bool{}
	for rows.Next() {
		var o object
		if e = rows.Scan(&o.kind, &o.name, &o.table, &o.sql); e != nil {
			return e
		}
		objects = append(objects, o)
		if o.kind == "table" {
			tables[strings.ToLower(o.name)] = true
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	edges := map[string]map[string]bool{}
	add := func(from, to string) {
		from, to = strings.ToLower(from), strings.ToLower(to)
		if !tables[from] || !tables[to] {
			return
		}
		if edges[from] == nil {
			edges[from] = map[string]bool{}
		}
		edges[from][to] = true
	}
	for _, o := range objects {
		if o.kind == "trigger" {
			for _, m := range mutationTarget.FindAllStringSubmatch(o.sql, -1) {
				add(o.table, m[1])
			}
		}
		if o.kind == "table" {
			for _, m := range referenceTarget.FindAllStringSubmatch(o.sql, -1) {
				add(m[1], o.name)
			}
		}
	}
	wakeDependencies.Lock()
	defer wakeDependencies.Unlock()
	// Handles in one process can be test databases with different schemas. Union
	// edges is conservative: it may wake extra workers but never hides a cascade.
	for from, targets := range edges {
		if wakeDependencies.edges[from] == nil {
			wakeDependencies.edges[from] = map[string]bool{}
		}
		for to := range targets {
			wakeDependencies.edges[from][to] = true
		}
	}
	return nil
}
func expandedChanges(roots map[string]bool) map[string]bool {
	if roots == nil {
		return nil
	}
	wakeDependencies.RLock()
	defer wakeDependencies.RUnlock()
	queue := make([]string, 0, len(roots))
	for table := range roots {
		queue = append(queue, table)
	}
	for len(queue) > 0 {
		table := queue[0]
		queue = queue[1:]
		for target := range wakeDependencies.edges[table] {
			if !roots[target] {
				roots[target] = true
				queue = append(queue, target)
			}
		}
	}
	return roots
}
