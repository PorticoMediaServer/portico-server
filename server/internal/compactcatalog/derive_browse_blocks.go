package compactcatalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Counted order blocks (catalog_browse_blocks over catalog_browse_rows, and
// compact_visibility_blocks over a visibility class's rows; both kept by the
// triggers blocks.py generates) index each scope's rows in every sort order
// for positions: a block starts at one row and counts the rows up to the next
// block's start. Every order is (key, entity_id) ascending; a descending page
// is the ascending run read backwards.

// The orderings a block set keeps, by the id stored in its ordering column.
const (
	OrderTitle = iota
	OrderAdded
	OrderYear
	OrderDuration
	OrderRating
	orderCount
)

// BlockOrderKeys are the browse rows' key expressions for each ordering, over
// the alias e, exactly as the blocks store them. A title key is lowered, which
// orders as sort_key COLLATE NOCASE does.
var BlockOrderKeys = [orderCount]string{
	OrderTitle:    "lower(e.sort_key)",
	OrderAdded:    "COALESCE(e.added_text,'')",
	OrderYear:     "e.year",
	OrderDuration: "COALESCE(e.duration_max,0)",
	OrderRating:   "COALESCE(e.rating_max,0)",
}

// VisibilityOrderKeys are the same keys as a visibility class row (alias v)
// carries them.
var VisibilityOrderKeys = [orderCount]string{
	OrderTitle:    "lower(v.sort_key)",
	OrderAdded:    "v.added",
	OrderYear:     "v.year",
	OrderDuration: "v.duration",
	OrderRating:   "v.rating",
}

// blockKey is a row's place in one scope's ordering.
type blockKey struct {
	scope  string
	value  any
	entity int64
}

// CheckBrowseBlocks verifies every block set against its rows: each block
// starts at a row, its count is the rows up to the next start, and together
// they count every row. It reads everything, for tests and diagnostics only.
func CheckBrowseBlocks(ctx context.Context, db *sql.DB) error {
	for ordering := 0; ordering < orderCount; ordering++ {
		if err := checkBlocks(ctx, db, "catalog_browse_blocks", []string{"library_id", "kind"},
			`SELECT e.library_id,e.kind,`+BlockOrderKeys[ordering]+`,e.entity_id FROM catalog_browse_rows e`, ordering); err != nil {
			return err
		}
		if err := checkBlocks(ctx, db, "compact_visibility_blocks", []string{"class_id", "generation", "library_id", "kind"},
			`SELECT v.class_id,v.generation,v.library_id,v.kind,`+VisibilityOrderKeys[ordering]+`,v.entity_id FROM compact_visibility_rows v`, ordering); err != nil {
			return err
		}
	}
	return nil
}

func checkBlocks(ctx context.Context, db *sql.DB, table string, scope []string, rowsSQL string, ordering int) error {
	scan := func(rows *sql.Rows, extra ...any) (blockKey, error) {
		parts := make([]int64, len(scope))
		targets := make([]any, 0, len(scope)+2+len(extra))
		for i := range parts {
			targets = append(targets, &parts[i])
		}
		var k blockKey
		targets = append(targets, &k.value, &k.entity)
		targets = append(targets, extra...)
		if err := rows.Scan(targets...); err != nil {
			return k, err
		}
		k.scope = fmt.Sprint(parts)
		if b, ok := k.value.([]byte); ok {
			k.value = string(b)
		}
		return k, nil
	}
	blocks := map[blockKey]int{}
	rows, err := db.QueryContext(ctx, `SELECT `+strings.Join(scope, ",")+`,sort_value,entity_id,total FROM `+table+` WHERE ordering=?`, ordering)
	if err != nil {
		return err
	}
	for rows.Next() {
		var total int
		k, scanErr := scan(rows, &total)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		blocks[k] = total
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	// SQLite orders the keys; ORDER BY position sorts by the scope, the key and
	// the entity as the blocks do.
	order := make([]string, 0, len(scope)+2)
	for i := range scope {
		order = append(order, fmt.Sprint(i+1))
	}
	order = append(order, fmt.Sprint(len(scope)+1), fmt.Sprint(len(scope)+2))
	rows, err = db.QueryContext(ctx, rowsSQL+` ORDER BY `+strings.Join(order, ","))
	if err != nil {
		return err
	}
	defer rows.Close()
	var current blockKey
	var counted, want int
	started := false
	finish := func() error {
		if started && counted != want {
			return fmt.Errorf("%s ordering %d: block %v counts %d rows, holds %d", table, ordering, current, want, counted)
		}
		return nil
	}
	for rows.Next() {
		k, scanErr := scan(rows)
		if scanErr != nil {
			return scanErr
		}
		if total, ok := blocks[k]; ok {
			if err = finish(); err != nil {
				return err
			}
			current, counted, want, started = k, 0, total, true
			delete(blocks, k)
		} else if !started || current.scope != k.scope {
			return fmt.Errorf("%s ordering %d: row %v precedes its scope's first block", table, ordering, k)
		}
		counted++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = finish(); err != nil {
		return err
	}
	for k := range blocks {
		return fmt.Errorf("%s ordering %d: block %v starts at no row", table, ordering, k)
	}
	return nil
}
