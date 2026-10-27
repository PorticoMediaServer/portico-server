package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"portico.local/server/internal/compactcatalog"
)

// The physical directory scope and the root path guard are one predicate on
// every platform. inventory_objects.directory_path and .relative_path hold
// native separators, and catalog_assets.path is filepath.Join(root, relative),
// so these bindings use the platform separator instead of a hard-coded slash.
// ReadingPathFolderEvidence keeps comparing the logical slash key; stored paths
// are never normalized.
const (
	readingDirectoryScopeSQL = `(o.directory_path=?3 OR (?4<>'' AND o.directory_path>=?4 AND o.directory_path<?5))`
	readingDirectoryPathSQL  = `a.path=?6||o.relative_path`
)

// readingDirectoryBindings returns the exact-directory match, the prefix range
// covering descendants, and the exact root prefix for the path guard. logical
// is the slash-normalized ReadingDirectoryKey and directory is the native
// folder the walk reported for it.
func readingDirectoryBindings(logical, directory, resolvedRoot, separator string) (base, lower, upper, rootPrefix string) {
	base = directory
	if logical != "" {
		// The logical key is slash-normalized; the stored folder is native.
		base = strings.ReplaceAll(logical, "/", separator)
		lower = base + separator
		// The next byte after the separator bounds only descendants: a sibling
		// whose name continues the same text sorts below the separator.
		upper = base + string(rune(separator[0]+1))
	}
	rootPrefix = filepath.Clean(resolvedRoot)
	if !strings.HasSuffix(rootPrefix, separator) {
		rootPrefix += separator
	}
	return base, lower, upper, rootPrefix
}

// This is path-metadata publication inside the real inventory transaction. It
// does no provider/archive I/O and grants no interactive or background reads.
// Physical directory indexes bound its work to the affected folder subtree;
// context and publication stream without retaining a whole folder.
func refreshReadingDirectoryTx(ctx context.Context, tx *sql.Tx, job string, source LibrarySource, directory string) error {
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, source.LibraryID).Scan(&kind); err != nil {
		return err
	}
	if kind != "book" && kind != "comic" {
		return nil
	}
	current, err := InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return err
	}
	if current.ID != source.ID || current.Generation != source.Generation || current.Incarnation != source.Incarnation || current.RootIdentity != source.RootIdentity || current.LibraryID != source.LibraryID || filepath.Clean(current.ResolvedPath) != filepath.Clean(source.ResolvedPath) {
		return context.Canceled
	}
	key, err := compactcatalog.ReadingDirectoryKey(filepath.ToSlash(directory))
	if err != nil {
		return err
	}
	base, lower, upper, rootPrefix := readingDirectoryBindings(key, directory, source.ResolvedPath, string(filepath.Separator))
	args := []any{source.ID, source.Incarnation, base, lower, upper, rootPrefix}
	from := ` FROM inventory_objects o INDEXED BY inventory_objects_directory
 JOIN catalog_assets a ON a.token=o.asset_id
 JOIN reading_publications p ON p.asset_id=a.id
 JOIN catalog_asset_links link ON link.entity_id=p.item_id AND link.asset_id=p.asset_id
 JOIN catalog_entities e ON e.id=p.item_id
 JOIN catalog_libraries cl ON cl.id=e.library_id
 JOIN library_sources s ON s.id=o.source_id AND s.library_id=cl.library_id
 JOIN libraries lib ON lib.id=cl.library_id
 WHERE o.source_id=?1 AND o.root_incarnation=?2 AND s.incarnation=o.root_incarnation
 AND ` + readingDirectoryScopeSQL + `
 AND o.retired=0 AND o.state='available' AND o.unsupported_reason=''
 AND e.retired=0 AND cl.retired=0 AND link.available=1 AND a.available=1
 AND ` + readingDirectoryPathSQL + `
 AND ((e.kind=20 AND lib.kind='book' AND p.format IN('epub','pdf')) OR (e.kind=21 AND lib.kind='comic' AND p.format IN('cbz','pdf','cb7','cbr'))) AND (p.format NOT IN('cb7','cbr') OR s.kind='local')
 AND o.id=(SELECT first.id FROM inventory_objects first INDEXED BY inventory_objects_asset WHERE first.asset_id=a.token AND first.source_id=o.source_id AND first.root_incarnation=o.root_incarnation AND first.relative_path=o.relative_path AND first.retired=0 ORDER BY first.id LIMIT 1)`
	// Root/generic-folder filename values do not depend on sibling count.
	titles, every := 1, false
	if key != "" {
		rows, err := tx.QueryContext(ctx, `SELECT e.id,o.relative_path,e.kind,a.path`+from+` ORDER BY e.id,a.id`, args...)
		if err != nil {
			return err
		}
		titles, every = 0, true
		var last int64
		for rows.Next() {
			var id int64
			var relative, actualPath string
			var k compactcatalog.Kind
			if err = rows.Scan(&id, &relative, &k, &actualPath); err != nil {
				rows.Close()
				return err
			}
			if filepath.Clean(filepath.Join(source.ResolvedPath, relative)) != filepath.Clean(actualPath) {
				continue
			}
			found, evidence, e := compactcatalog.ReadingPathFolderEvidence(compactcatalog.ReadingRuleInput{Item: id, SourceRoot: source.ID, RelativePath: filepath.ToSlash(relative), Kind: k})
			if e != nil {
				rows.Close()
				return e
			}
			if found != key || id == last {
				continue
			}
			last = id
			if titles < 2 {
				titles++
			}
			every = every && evidence
			// Both aggregates are final; later siblings cannot change them.
			if titles == 2 && !every {
				break
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if titles == 0 {
			return nil
		}
	}
	// Stream this join once. Reissuing it for every 128 assets sorted the entire
	// folder repeatedly. Fetch prior path values by their exact indexed key in
	// the same cursor instead of dispatching one extra SELECT per publication.
	rows, err := tx.QueryContext(ctx, `SELECT p.item_id,a.id,o.id,o.revision,o.relative_path,e.kind,a.path,
 (SELECT json_group_array(json_array(field,COALESCE(text_value,''),number_value,source,inventory_object_id,source_revision))
 FROM (SELECT field,text_value,number_value,source,inventory_object_id,source_revision FROM reading_source_values
 WHERE item_id=p.item_id AND asset_id=a.id AND source IN('file-name','folder') ORDER BY field,source))`+from+` ORDER BY a.id,e.id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	changed := false
	for rows.Next() {
		var item, asset int64
		var object, revision, relative, actualPath, previous string
		var kind compactcatalog.Kind
		if err = rows.Scan(&item, &asset, &object, &revision, &relative, &kind, &actualPath, &previous); err != nil {
			return err
		}
		if filepath.Clean(filepath.Join(source.ResolvedPath, relative)) != filepath.Clean(actualPath) {
			continue
		}
		input := compactcatalog.ReadingRuleInput{Item: item, SourceRoot: source.ID, RelativePath: filepath.ToSlash(relative), Kind: kind}
		found, _, e := compactcatalog.ReadingPathFolderEvidence(input)
		if e != nil {
			return e
		}
		if found != key {
			continue
		}
		value, e := compactcatalog.ReadingPathValue(input, titles, every)
		if e != nil {
			return e
		}
		old, e := readingPathFieldsJSON(previous)
		if e != nil {
			return e
		}
		updated, e := publishReadingPathFieldsTx(ctx, tx, item, asset, object, revision, old, readingPathFields(object, revision, value))
		if e != nil {
			return e
		}
		changed = changed || updated
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if changed {
		_, err = tx.ExecContext(ctx, `INSERT INTO library_revisions(library_id,revision) VALUES(?,1) ON CONFLICT(library_id) DO UPDATE SET revision=revision+1`, source.LibraryID)
	}
	return err
}

type readingPathField struct {
	field, text, source, object, revision string
	number                                sql.NullFloat64
}

func readingPathFields(object, revision string, v compactcatalog.ReadingValues) []readingPathField {
	var next []readingPathField
	for _, field := range []struct {
		name  string
		value *compactcatalog.ReadingValue
	}{{"author", v.Author}, {"issueDate", v.IssueDate}, {"series", v.Series}} {
		if field.value != nil {
			next = append(next, readingPathField{field: field.name, text: field.value.Value, source: string(field.value.Source), object: object, revision: revision})
		}
	}
	if v.SeriesNumber != nil {
		next = append(next, readingPathField{field: "seriesNumber", source: string(v.SeriesNumber.Source), object: object, revision: revision, number: sql.NullFloat64{Float64: v.SeriesNumber.Value, Valid: true}})
	}
	return next
}

func publishReadingPathValuesTx(ctx context.Context, tx *sql.Tx, item, asset int64, object, revision string, v compactcatalog.ReadingValues) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT field,COALESCE(text_value,''),number_value,source,inventory_object_id,source_revision FROM reading_source_values WHERE item_id=? AND asset_id=? AND source IN('file-name','folder') ORDER BY field,source`, item, asset)
	if err != nil {
		return false, err
	}
	var old []readingPathField
	for rows.Next() {
		var field readingPathField
		if err = rows.Scan(&field.field, &field.text, &field.number, &field.source, &field.object, &field.revision); err != nil {
			rows.Close()
			return false, err
		}
		old = append(old, field)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	if err = rows.Close(); err != nil {
		return false, err
	}
	return publishReadingPathFieldsTx(ctx, tx, item, asset, object, revision, old, readingPathFields(object, revision, v))
}

// The open cursor reads immutable identity/path columns. These writes change
// only accepted values and entity revisions, never its membership or ordering.
func publishReadingPathFieldsTx(ctx context.Context, tx *sql.Tx, item, asset int64, object, revision string, old, next []readingPathField) (bool, error) {
	if reflect.DeepEqual(old, next) {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reading_source_values WHERE item_id=? AND asset_id=? AND source IN('file-name','folder')`, item, asset); err != nil {
		return false, err
	}
	for _, field := range next {
		var text, number any
		if field.number.Valid {
			number = field.number.Float64
		} else {
			text = field.text
		}
		// Recheck the exact association and revision at publication, not merely
		// a pathname or an equal size/mtime. The enclosing real job/root fence
		// remains in force for this writer transaction.
		result, e := tx.ExecContext(ctx, `INSERT INTO reading_source_values(item_id,asset_id,field,text_value,number_value,source,inventory_object_id,source_revision)
 SELECT ?1,?2,?3,?4,?5,?6,o.id,o.revision FROM inventory_objects o
 JOIN catalog_assets a ON a.token=o.asset_id AND a.id=?2
 JOIN reading_publications p ON p.item_id=?1 AND p.asset_id=a.id
 JOIN catalog_asset_links link ON link.entity_id=p.item_id AND link.asset_id=p.asset_id
 JOIN catalog_entities e ON e.id=p.item_id JOIN catalog_libraries cl ON cl.id=e.library_id
 JOIN library_sources s ON s.id=o.source_id AND s.library_id=cl.library_id
 WHERE o.id=?7 AND o.revision=?8 AND o.root_incarnation=s.incarnation AND o.retired=0`, item, asset, field.field, text, number, field.source, object, revision)
		if e != nil {
			return false, e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return false, e
		}
		if n != 1 {
			return false, context.Canceled
		}
	}
	automatic, err := compactcatalog.ReadingAutomaticFieldsTx(ctx, tx, item)
	if err != nil {
		return false, err
	}
	values := map[string]string{"author": "", "series": "", "seriesNumber": ""}
	if automatic.Author != nil {
		values["author"] = automatic.Author.Value
	}
	if automatic.Series != nil {
		values["series"] = automatic.Series.Value
	}
	if automatic.SeriesNumber != nil {
		values["seriesNumber"] = strconv.FormatFloat(automatic.SeriesNumber.Value, 'g', -1, 64)
	}
	for field, value := range values {
		if _, err = tx.ExecContext(ctx, `UPDATE metadata_owner_fields SET automatic_value=? WHERE kind='item' AND entity_id=? AND field=? AND automatic_value<>?`, value, item, field, value); err != nil {
			return false, err
		}
	}
	return true, compactcatalog.TouchEntityTx(ctx, tx, item)
}

// JSON preserves nullable numbers and exact Unicode strings without delimiters.
func readingPathFieldsJSON(raw string) ([]readingPathField, error) {
	var values [][]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	var fields []readingPathField
	for _, value := range values {
		if len(value) != 6 {
			return nil, compactcatalog.ErrReadingValue
		}
		var field readingPathField
		for i, target := range []any{&field.field, &field.text, nil, &field.source, &field.object, &field.revision} {
			if target != nil {
				if err := json.Unmarshal(value[i], target); err != nil {
					return nil, err
				}
			}
		}
		var number *float64
		if err := json.Unmarshal(value[2], &number); err != nil {
			return nil, err
		}
		if number != nil {
			field.number = sql.NullFloat64{Float64: *number, Valid: true}
		}
		fields = append(fields, field)
	}
	return fields, nil
}
