package persistence

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryRetentionIndexDoesNotRewriteCheckpointKeys(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	importers, err := catalogFixtureRuntimeImports(root)
	if err != nil || len(importers) != 0 {
		t.Fatalf("cannot classify catalogue-only fixture writes: importers=%v err=%v", importers, err)
	}
	statements, _ := productionStatements(t)
	checkpoints, inserts, moves := 0, 0, 0
	for _, statement := range statements {
		if strings.HasPrefix(filepath.ToSlash(statement.where), "internal/catalogtest/") {
			continue // The import-closure assertion above proves this is fixture code.
		}
		text := strings.Join(strings.Fields(statement.text), " ")
		upper := strings.ToUpper(text)
		if strings.Contains(upper, "UPDATE PERSONAL_HISTORY SET") {
			// This is a deliberate title merge, not a playback checkpoint.
			// Moving the item updates the new index's entry, once per moved row.
			if !strings.Contains(statement.where, "title_merge.go:") || text != "UPDATE personal_history SET item_id=?2 WHERE item_id=?1" {
				t.Fatalf("uncensused history rewrite at %s: %s", statement.where, text)
			}
			moves++
		}
		if !strings.Contains(upper, "INSERT INTO PERSONAL_HISTORY(") {
			continue
		}
		if !strings.Contains(statement.where, "personal_activity.go:") {
			t.Fatalf("uncensused production history insert at %s", statement.where)
		}
		inserts++
		matches := ownerWalkConflict.FindAllStringIndex(text, -1)
		for _, match := range matches {
			columns := ownerWalkAssignedColumns(text[match[1]:])
			if len(columns) == 0 {
				t.Fatal("history checkpoint SET list disappeared")
			}
			for _, column := range columns {
				if column == "item_id" || column == "profile_id" {
					t.Fatalf("checkpoint rewrites retention index key %s at %s", column, statement.where)
				}
			}
			checkpoints++
		}
	}
	if inserts != 1 || checkpoints != 1 || moves != 1 {
		t.Fatalf("history producer census changed: inserts=%d checkpoints=%d explicit title moves=%d", inserts, checkpoints, moves)
	}
	// The added index has one entry per inserted history row. The key census
	// proves zero added index writes for the ordinary checkpoint UPDATE path;
	// it makes no physical page or WAL-frame assertion.
}
