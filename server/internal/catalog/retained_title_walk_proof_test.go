package catalog

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// A new FK child or changed trigger in the point-delete closure requires a
// fresh source derivation even when a tiny fixture's total_changes still fits.
// The hash pins every trigger on the source and downstream tables, including
// non-delete arms conservatively. It is not a substitute for the literal cost
// derivation or a physical page/frame assertion.
func TestRetainedTitleWalkPointClosureRequiresSourceReview(t *testing.T) {
	c := catalogtest.Open(t)
	tables := []string{
		"personal_items", "progress", "progress_activity", "personal_watched_intents",
		"personal_play_counts", "personal_history", "reading_progress", "profile_show_partials",
		"viewer_revisions", "personal_activity_revisions", "rec_profile_jobs",
		"catalog_personal_sort_item_jobs", "catalog_title_tombstone_viewers",
		"catalog_removed_title_revisions", "catalog_selection_predicate_epoch",
		"api_change_pending", "maintenance_table_bounds",
	}
	args := make([]any, len(tables))
	covered := map[string]bool{}
	for i, table := range tables {
		args[i], covered[table] = table, true
	}
	h := sha256.New()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	args = append(args, args...)
	rows, err := c.DB.Query(`SELECT type,name,sql FROM sqlite_schema
 WHERE (type='table' AND name IN (`+placeholders+`)) OR (type='trigger' AND tbl_name IN (`+placeholders+`))
 ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END,name`, args...)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var kind, name, sql string
		if err := rows.Scan(&kind, &name, &sql); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%s|%s|%s\n", kind, name, sql)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	rows, err = c.DB.Query(`SELECT name FROM sqlite_schema WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		declared = append(declared, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	for _, table := range declared {
		rows, err := c.DB.Query(`SELECT id,seq,"table","from",COALESCE("to",''),on_update,on_delete,match FROM pragma_foreign_key_list(?) ORDER BY id,seq`, table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, seq int
			var parent, from, to, update, delete, match string
			if err := rows.Scan(&id, &seq, &parent, &from, &to, &update, &delete, &match); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if covered[parent] {
				fmt.Fprintf(h, "fk|%s|%d,%d,%s,%s,%s,%s,%s,%s\n", table, id, seq, parent, from, to, update, delete, match)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
	const reviewed = "ba06533734fe2ed4b40c296fe0e498ca31d5357a7c562e2e984e9b2f8e45fadb"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != reviewed {
		t.Fatalf("retained-input trigger/FK closure changed (%s); rederive effect/read bounds before updating this proof pin", got)
	}
}
