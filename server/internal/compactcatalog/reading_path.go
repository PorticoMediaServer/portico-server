package compactcatalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Private publication rule; independent of file metadata and ready projections.
const readingPathRule = 1
const readingPathBatch = 128
const pathColumns = `object_id,source_revision,author,author_source,issue_date,series,series_source,series_number`

type pathBundle struct {
	Object, Revision                                 string
	Author, AuthorSource, Date, Series, SeriesSource sql.NullString
	Number                                           sql.NullFloat64
}

func (v pathBundle) args() []any {
	return []any{v.Object, v.Revision, v.Author, v.AuthorSource, v.Date, v.Series, v.SeriesSource, v.Number}
}
func (v *pathBundle) scan() []any {
	return []any{&v.Object, &v.Revision, &v.Author, &v.AuthorSource, &v.Date, &v.Series, &v.SeriesSource, &v.Number}
}
func bundle(object, revision string, v ReadingValues) (pathBundle, error) {
	out := pathBundle{Object: object, Revision: revision}
	for _, f := range []struct {
		v            *ReadingValue
		text, source *sql.NullString
	}{{v.Author, &out.Author, &out.AuthorSource}, {v.Series, &out.Series, &out.SeriesSource}} {
		if f.v == nil {
			continue
		}
		if f.v.Source != ReadingFileName && f.v.Source != ReadingFolder || strings.TrimSpace(f.v.Value) == "" || len(f.v.Value) > 4096 || !utf8.ValidString(f.v.Value) || strings.ContainsAny(f.v.Value, "\x00\r\n") {
			return out, ErrReadingValue
		}
		*f.text = sql.NullString{String: f.v.Value, Valid: true}
		*f.source = sql.NullString{String: string(f.v.Source), Valid: true}
	}
	if v.IssueDate != nil {
		if v.IssueDate.Source != ReadingFileName || !ValidReadingIssueDate(v.IssueDate.Value) {
			return out, ErrReadingValue
		}
		out.Date = sql.NullString{String: v.IssueDate.Value, Valid: true}
	}
	if v.SeriesNumber != nil {
		n := v.SeriesNumber.Value
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return out, ErrReadingValue
		}
		out.Number = sql.NullFloat64{Float64: n, Valid: true}
	}
	return out, nil
}
func (v pathBundle) values() ReadingValues {
	out := ReadingValues{}
	if v.Author.Valid {
		out.Author = &ReadingValue{v.Author.String, ReadingValueSource(v.AuthorSource.String)}
	}
	if v.Series.Valid {
		out.Series = &ReadingValue{v.Series.String, ReadingValueSource(v.SeriesSource.String)}
	}
	if v.Date.Valid {
		out.IssueDate = &ReadingValue{v.Date.String, ReadingFileName}
	}
	if v.Number.Valid {
		out.SeriesNumber = &ReadingNumber{v.Number.Float64, ReadingFileName}
	}
	return out
}
func pathSequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var n int64
	err := tx.QueryRowContext(ctx, `UPDATE reading_path_clock SET sequence=sequence+1 WHERE singleton=1 AND sequence<9223372036854775807 RETURNING sequence`).Scan(&n)
	return n, err
}
func pathTurn(ctx context.Context, tx *sql.Tx) (int64, error) {
	var n int64
	err := tx.QueryRowContext(ctx, `UPDATE reading_path_clock SET turn=turn+1 WHERE singleton=1 AND turn<9223372036854775807 RETURNING turn`).Scan(&n)
	return n, err
}

// Queue one semantic invalidation per library/transaction. Flatten never calls it.
func pathSemantic(ctx context.Context, tx *sql.Tx, library int64) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO reading_path_semantic_pending VALUES(?)`, library)
	return err
}
func FlushReadingPathSemanticTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT library_id FROM reading_path_semantic_pending ORDER BY library_id LIMIT 128`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		for _, q := range []string{`INSERT INTO library_revisions(library_id,revision) SELECT library_id,1 FROM catalog_libraries WHERE id=? ON CONFLICT(library_id) DO UPDATE SET revision=revision+1`, `UPDATE reading_attribution_state SET input_generation=input_generation+1,phase='scan' WHERE library_id=?`, `DELETE FROM reading_path_semantic_pending WHERE library_id=?`} {
			if _, err = tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// The winner is selected BEFORE acceptance. Invalid newer evidence is an absence,
// never permission to resurrect an overwritten source. No witness FK is used.
func pathWinner(ctx context.Context, tx *sql.Tx, item, asset int64) (pathBundle, bool, error) {
	var v pathBundle
	err := tx.QueryRowContext(ctx, `SELECT `+pathColumns+` FROM (
 SELECT h.publisher_sequence,`+pathColumns+` FROM reading_path_head h WHERE item_id=?1 AND asset_id=?2
 UNION ALL SELECT b.publish_sequence,`+prefixColumns("v")+` FROM reading_path_slot z JOIN reading_path_build b ON b.id=z.overlay_id JOIN reading_path_stage v ON v.build_id=b.id AND v.item_id=?1 AND v.asset_id=?2 WHERE z.singleton=1
 ) ORDER BY publisher_sequence DESC LIMIT 1`, item, asset).Scan(v.scan()...)
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}
func prefixColumns(alias string) string {
	parts := strings.Split(pathColumns, ",")
	for i := range parts {
		parts[i] = alias + "." + parts[i]
	}
	return strings.Join(parts, ",")
}
func pathAccepted(ctx context.Context, tx *sql.Tx, item, asset int64, v pathBundle) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reading_publications p JOIN catalog_asset_links link ON link.entity_id=p.item_id AND link.asset_id=p.asset_id JOIN catalog_assets a ON a.id=p.asset_id JOIN catalog_entities e ON e.id=p.item_id JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries lib ON lib.id=cl.library_id JOIN inventory_objects o ON o.id=?3 AND o.revision=?4 AND o.asset_id=a.token AND o.retired=0 JOIN library_sources s ON s.id=o.source_id AND s.library_id=cl.library_id AND s.incarnation=o.root_incarnation WHERE p.item_id=?1 AND p.asset_id=?2 AND e.retired=0 AND cl.retired=0 AND ((e.kind=20 AND lib.kind='book' AND p.format IN('epub','pdf','mobi','azw3')) OR(e.kind=21 AND lib.kind='comic' AND p.format IN('cbz','pdf','cb7','cbr'))))`, item, asset, v.Object, v.Revision).Scan(&ok)
	return ok, err
}
func writePathHead(ctx context.Context, tx *sql.Tx, item, asset, seq int64, v pathBundle) error {
	args := append([]any{item, asset, seq}, v.args()...)
	_, err := tx.ExecContext(ctx, `INSERT INTO reading_path_head(item_id,asset_id,publisher_sequence,`+pathColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(item_id,asset_id) DO UPDATE SET publisher_sequence=excluded.publisher_sequence,object_id=excluded.object_id,source_revision=excluded.source_revision,author=excluded.author,author_source=excluded.author_source,issue_date=excluded.issue_date,series=excluded.series,series_source=excluded.series_source,series_number=excluded.series_number WHERE reading_path_head.publisher_sequence<=excluded.publisher_sequence`, args...)
	return err
}

// ReadingPublishPathTx is also used by explicit changed-input publication tests.
// It writes one typed bundle, including a real all-null empty bundle.
func ReadingPublishPathTx(ctx context.Context, tx *sql.Tx, item, asset int64, object, revision string, values ReadingValues) (bool, error) {
	next, err := bundle(object, revision, values)
	if err != nil {
		return false, err
	}
	accepted, err := pathAccepted(ctx, tx, item, asset, next)
	if err != nil {
		return false, err
	}
	if !accepted {
		return false, context.Canceled
	}
	old, exists, err := pathWinner(ctx, tx, item, asset)
	if err != nil {
		return false, err
	}
	if exists && reflect.DeepEqual(old, next) {
		return false, nil
	}
	seq, err := pathSequence(ctx, tx)
	if err != nil {
		return false, err
	}
	if err = writePathHead(ctx, tx, item, asset, seq, next); err != nil {
		return false, err
	}
	var lib int64
	if err = tx.QueryRowContext(ctx, `SELECT library_id FROM catalog_entities WHERE id=?`, item).Scan(&lib); err != nil {
		return false, err
	}
	return true, pathSemantic(ctx, tx, lib)
}

type pathObject struct {
	ID, Source, Incarnation, Relative, Revision, Root, RootIdentity, Actual, LibraryKind, Format string
	Library, Item, Asset, Generation                                                             int64
	Kind                                                                                         Kind
	Eligible, Context                                                                            bool
}

func readPathObject(ctx context.Context, tx *sql.Tx, id string) (pathObject, error) {
	v := pathObject{ID: id}
	err := tx.QueryRowContext(ctx, `SELECT o.source_id,o.root_incarnation,o.relative_path,o.revision,s.root,s.root_identity,a.path,lib.kind,COALESCE(p.format,''),cl.id,COALESCE(e.id,0),a.id,s.generation,COALESCE(e.kind,20),COALESCE(link.available,0),
 o.retired=0 AND o.state='available' AND o.unsupported_reason='' AND s.incarnation=o.root_incarnation AND COALESCE(e.retired=0 AND e.library_id=cl.id AND cl.retired=0,0) AND a.available=1
 AND ((e.kind=20 AND lib.kind='book' AND p.format IN('epub','pdf')) OR(e.kind=21 AND lib.kind='comic' AND p.format IN('cbz','pdf','cb7','cbr'))) AND (p.format NOT IN('cb7','cbr') OR s.kind='local')
 AND o.id=(SELECT first.id FROM inventory_objects first INDEXED BY inventory_objects_reading_first WHERE first.asset_id=o.asset_id AND first.source_id=o.source_id AND first.root_incarnation=o.root_incarnation AND first.relative_path=o.relative_path AND first.retired=0 ORDER BY first.id LIMIT 1)
 FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id JOIN libraries lib ON lib.id=s.library_id AND lib.kind IN('book','comic') JOIN catalog_libraries cl ON cl.library_id=lib.id JOIN catalog_assets a ON a.token=o.asset_id LEFT JOIN reading_publications p ON p.asset_id=a.id LEFT JOIN catalog_entities e ON e.id=p.item_id LEFT JOIN catalog_asset_links link ON link.entity_id=p.item_id AND link.asset_id=p.asset_id WHERE o.id=? AND o.retired=0`, id).Scan(&v.Source, &v.Incarnation, &v.Relative, &v.Revision, &v.Root, &v.RootIdentity, &v.Actual, &v.LibraryKind, &v.Format, &v.Library, &v.Item, &v.Asset, &v.Generation, &v.Kind, &v.Eligible, &v.Context)
	if err == nil {
		v.Context = v.Context && v.Actual == rootPrefix(v.Root)+v.Relative && filepath.Clean(filepath.Join(v.Root, v.Relative)) == filepath.Clean(v.Actual)
		v.Eligible = v.Eligible && v.Context
	}
	return v, err
}
func rootPrefix(root string) string {
	root = filepath.Clean(root)
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	return root
}

// ReadingNormalizeObjectTx normalizes ONE raw object, never its siblings. This
// seam is used on observation and by the durable feed for all other writers.
func ReadingNormalizeObjectTx(ctx context.Context, tx *sql.Tx, id string) error {
	v, err := readPathObject(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `DELETE FROM reading_path_object_keys WHERE object_id=?`, id)
		return err
	}
	if err != nil {
		return err
	}
	key, err := ReadingDirectoryKey(filepath.ToSlash(filepath.Dir(v.Relative)))
	if err != nil {
		return err
	}
	var scope int64
	err = tx.QueryRowContext(ctx, `INSERT INTO reading_path_scope(library_id,source_id,incarnation,root_prefix,directory_key,rule_version) VALUES(?,?,?,?,?,?) ON CONFLICT(source_id,incarnation,root_prefix,directory_key,rule_version) DO UPDATE SET library_id=excluded.library_id RETURNING id`, v.Library, v.Source, v.Incarnation, rootPrefix(v.Root), key, readingPathRule).Scan(&scope)
	if err != nil {
		return err
	}
	var pending bool
	if v.Item != 0 && v.Context {
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reading_path_availability_pending WHERE entity_id=?)`, v.Item).Scan(&pending); err != nil {
			return err
		}
	}
	var oldScope, oldItem int64
	var oldPending, oldContext bool
	err = tx.QueryRowContext(ctx, `SELECT scope_id,item_id,pending,context_active FROM reading_path_object_keys WHERE object_id=?`, id).Scan(&oldScope, &oldItem, &oldPending, &oldContext)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if oldScope != scope || oldItem != v.Item || oldPending != pending || oldContext != v.Context {
		if _, err = tx.ExecContext(ctx, `DELETE FROM reading_path_object_keys WHERE object_id=?`, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO reading_path_object_keys VALUES(?,?,?,?,?)`, id, scope, v.Item, pending, v.Context); err != nil {
			return err
		}
	}
	// Relevant object changes already dirtied the old scope synchronously. A new
	// scope has desired=1. Fanouts dirty retained keys even when membership agrees.
	if key == "" {
		if _, err = tx.ExecContext(ctx, `UPDATE reading_path_scope SET completed=desired,queued=0 WHERE id=?`, scope); err != nil {
			return err
		}
	}
	if !v.Eligible {
		return nil
	}
	old, exists, err := pathWinner(ctx, tx, v.Item, v.Asset)
	if err != nil {
		return err
	}
	if exists && key != "" {
		ok, e := pathAccepted(ctx, tx, v.Item, v.Asset, old)
		if e != nil {
			return e
		}
		if ok {
			return nil
		}
	}
	input := ReadingRuleInput{Item: v.Item, SourceRoot: v.Source, RelativePath: filepath.ToSlash(v.Relative), Kind: v.Kind}
	values, err := ReadingPathValue(input, 1, false)
	if err != nil {
		return err
	}
	if key != "" {
		if values.Author != nil && values.Author.Source != ReadingFileName {
			values.Author = nil
		}
		values.Series = nil
		values.SeriesNumber = nil
	}
	_, err = ReadingPublishPathTx(ctx, tx, v.Item, v.Asset, v.ID, v.Revision, values)
	return err
}

// ReadingPathBlockTx stores one normalized coverage dependency. Generic
// descendants map to their parent logical scope; non-generic subtrees do not
// hold back the parent's independent direct/generic publications.
func ReadingPathBlockTx(ctx context.Context, tx *sql.Tx, job, relative, reason string) error {
	ext := strings.ToLower(filepath.Ext(relative))
	if (strings.HasPrefix(reason, "entry_") || strings.HasPrefix(reason, "symlink_")) && (ext == ".epub" || ext == ".pdf" || ext == ".cbz" || ext == ".cb7" || ext == ".cbr") {
		parent := filepath.Dir(relative)
		if parent != relative {
			if err := ReadingPathBlockTx(ctx, tx, job, parent, "unknown_entry_"+reason); err != nil {
				return err
			}
		}
	}
	key, err := ReadingDirectoryKey(filepath.ToSlash(filepath.Clean(relative)))
	if err != nil {
		key = filepath.ToSlash(filepath.Clean(relative))
		reason = "untrusted_coverage"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO reading_path_block(job_id,source_id,directory_key,reason) SELECT ?,source_id,?,? FROM inventory_runs WHERE job_id=? ON CONFLICT(source_id,job_id,directory_key) DO UPDATE SET reason=excluded.reason`, job, key, reason, job)
	return err
}

type pathWork struct {
	Library, Requested, Captured, Asset, Scope, Phase int64
	Kind, Key, Object                                 string
}

func pathWorkStep(ctx context.Context, tx *sql.Tx, limit int, turn int64) (int, error) {
	var w pathWork
	err := tx.QueryRowContext(ctx, `SELECT library_id,kind,work_key,requested,captured,after_object,after_asset,after_scope,phase FROM reading_path_dirty_work ORDER BY served,kind,work_key LIMIT 1`).Scan(&w.Library, &w.Kind, &w.Key, &w.Requested, &w.Captured, &w.Object, &w.Asset, &w.Scope, &w.Phase)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if w.Requested != w.Captured {
		w.Captured = w.Requested
		w.Object = ""
		w.Asset = 0
		w.Scope = 0
		w.Phase = 0
	}
	done := false
	n := 0
	normalize := func(ids []string) error {
		for _, id := range ids {
			if e := ReadingNormalizeObjectTx(ctx, tx, id); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, `UPDATE reading_path_scope SET desired=desired+1,queued=1 WHERE id=(SELECT scope_id FROM reading_path_object_keys WHERE object_id=?) AND directory_key<>''`, id); e != nil {
				return e
			}
		}
		return nil
	}
	switch w.Kind {
	case "object":
		err = ReadingNormalizeObjectTx(ctx, tx, w.Key)
		done = true
		n = 1
	case "asset", "source":
		if w.Phase == 0 {
			q := `SELECT id FROM inventory_objects INDEXED BY inventory_objects_reading_asset_object WHERE asset_id=? AND id>? ORDER BY id LIMIT ?`
			if w.Kind == "source" {
				q = `SELECT id FROM inventory_objects INDEXED BY inventory_objects_reading_source_object WHERE source_id=? AND id>? ORDER BY id LIMIT ?`
			}
			var ids []string
			ids, err = pathStrings(ctx, tx, q, w.Key, w.Object, limit)
			n = len(ids)
			if err == nil {
				err = normalize(ids)
			}
			if n > 0 {
				w.Object = ids[n-1]
			} else if w.Kind == "source" {
				w.Phase = 1
			} else {
				done = true
			}
		} else {
			var ids []int64
			ids, err = scanIDs(ctx, tx, `SELECT id FROM reading_path_scope WHERE source_id=? AND id>? ORDER BY id LIMIT ?`, w.Key, w.Scope, limit)
			n = len(ids)
			for _, id := range ids {
				if err != nil {
					break
				}
				_, err = tx.ExecContext(ctx, `UPDATE reading_path_scope SET desired=desired+1,queued=1,sealed=0,seal_job='',seal_cursor=0,reason='' WHERE id=?`, id)
				w.Scope = id
			}
			done = n == 0
		}
	case "item":
		if w.Phase == 0 {
			var ids []string
			ids, err = pathStrings(ctx, tx, `SELECT object_id FROM reading_path_object_keys WHERE item_id=? AND object_id>? ORDER BY object_id LIMIT ?`, w.Key, w.Object, limit)
			n = len(ids)
			if err == nil {
				err = normalize(ids)
			}
			if n > 0 {
				w.Object = ids[n-1]
			} else {
				w.Phase = 1
				w.Object = ""
			}
		} else if w.Phase == 1 {
			var next int64
			err = tx.QueryRowContext(ctx, `SELECT asset_id FROM reading_publications WHERE item_id=? AND asset_id>? ORDER BY asset_id LIMIT 1`, w.Key, w.Asset).Scan(&next)
			if errors.Is(err, sql.ErrNoRows) {
				err = nil
				done = true
			} else if err == nil {
				w.Asset = next
				w.Phase = 2
				w.Object = ""
			}
			n = 1
		} else {
			var ids []string
			ids, err = pathStrings(ctx, tx, `SELECT o.id FROM catalog_assets a CROSS JOIN inventory_objects o INDEXED BY inventory_objects_reading_asset_object ON o.asset_id=a.token WHERE a.id=? AND o.id>? ORDER BY o.id LIMIT ?`, w.Asset, w.Object, limit)
			n = len(ids)
			if err == nil {
				err = normalize(ids)
			}
			if n > 0 {
				w.Object = ids[n-1]
			} else {
				w.Phase = 1
				w.Object = ""
			}
		}
	case "pending":
		var ids []string
		ids, err = pathStrings(ctx, tx, `SELECT object_id FROM reading_path_object_keys WHERE item_id=? AND object_id>? ORDER BY object_id LIMIT ?`, w.Key, w.Object, limit)
		n = len(ids)
		for _, id := range ids {
			if err != nil {
				break
			}
			_, err = tx.ExecContext(ctx, `UPDATE reading_path_object_keys SET pending=context_active AND EXISTS(SELECT 1 FROM reading_path_availability_pending WHERE entity_id=item_id) WHERE object_id=?`, id)
			w.Object = id
		}
		done = n == 0
	case "report":
		var source string
		err = tx.QueryRowContext(ctx, `SELECT source_id FROM inventory_runs WHERE job_id=?`, w.Key).Scan(&source)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
			done = true
			break
		}
		if err != nil {
			break
		}
		var ids []int64
		ids, err = scanIDs(ctx, tx, `SELECT id FROM reading_path_scope WHERE source_id=? AND id>? ORDER BY id LIMIT ?`, source, w.Scope, limit)
		n = len(ids)
		for _, id := range ids {
			if err != nil {
				break
			}
			var s pathScope
			s, err = pathScopeRead(ctx, tx, id)
			if err != nil {
				break
			}
			if s.Desired != s.Completed {
				var reason string
				reason, err = scopeReady(ctx, tx, s)
				if reason == "" {
					reason = "publication_queued"
				}
				if s.Job != w.Key {
					reason = "next_scan_started"
				}
				s.Job = w.Key
				if err == nil {
					err = pathReport(ctx, tx, s, reason)
				}
			}
			w.Scope = id
		}
		done = n == 0
	case "coverage":
		rows, e := tx.QueryContext(ctx, `SELECT id,job_id FROM reading_path_block WHERE source_id=? AND id>? ORDER BY id LIMIT ?`, w.Key, w.Scope, limit)
		if e != nil {
			return 0, e
		}
		type block struct {
			id  int64
			job string
		}
		var batch []block
		for rows.Next() {
			var v block
			if e = rows.Scan(&v.id, &v.job); e != nil {
				rows.Close()
				return 0, e
			}
			batch = append(batch, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return 0, e
		}
		var job string
		err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT job_id FROM reading_path_source_barrier WHERE source_id=?),'')`, w.Key).Scan(&job)
		for _, v := range batch {
			if err != nil {
				break
			}
			if v.job != job {
				_, err = tx.ExecContext(ctx, `DELETE FROM reading_path_block WHERE id=?`, v.id)
			}
			w.Scope = v.id
		}
		n = len(batch)
		done = n == 0
	case "library":
		if w.Phase == 0 {
			var ids []string
			ids, err = pathStrings(ctx, tx, `SELECT s.id FROM catalog_libraries cl CROSS JOIN library_sources s ON s.library_id=cl.library_id WHERE cl.id=? AND s.id>? ORDER BY s.id LIMIT ?`, w.Library, w.Object, limit)
			n = len(ids)
			for _, id := range ids {
				if err != nil {
					break
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO reading_path_dirty_work(library_id,kind,work_key) VALUES(?,'source',?) ON CONFLICT(kind,work_key) DO UPDATE SET requested=requested+1`, w.Library, id)
				w.Object = id
			}
			if n == 0 {
				w.Phase = 1
			}
		} else {
			var ids []int64
			ids, err = scanIDs(ctx, tx, `SELECT id FROM reading_path_scope WHERE library_id=? AND id>? ORDER BY id LIMIT ?`, w.Library, w.Scope, limit)
			n = len(ids)
			for _, id := range ids {
				if err != nil {
					break
				}
				_, err = tx.ExecContext(ctx, `UPDATE reading_path_scope SET desired=desired+1,queued=1 WHERE id=?`, id)
				w.Scope = id
			}
			done = n == 0
		}

	}
	if err != nil {
		return 0, err
	}
	if done {
		_, err = tx.ExecContext(ctx, `DELETE FROM reading_path_dirty_work WHERE kind=? AND work_key=? AND requested=?`, w.Kind, w.Key, w.Captured)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE reading_path_dirty_work SET captured=?,after_object=?,after_asset=?,after_scope=?,phase=?,served=? WHERE kind=? AND work_key=?`, w.Captured, w.Object, w.Asset, w.Scope, w.Phase, turn, w.Kind, w.Key)
	}
	return max(1, n), err
}

// A quantum may finish several tiny coalesced feed keys. The aggregate raw
// budget, including empty subcursor/EOF probes, remains <=128. Only builder
// transitions require a dedicated commit; a one-object feed is not a batch.
func pathWorkQuantum(ctx context.Context, tx *sql.Tx, limit int, turn int64) (int, error) {
	total := 0
	for total < limit {
		n, err := pathWorkStep(ctx, tx, limit-total, turn)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			break
		}
		total += n
	}
	if err := FlushReadingPathSemanticTx(ctx, tx); err != nil {
		return 0, err
	}
	return total, nil
}

func pathStrings(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []string
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}

type pathScope struct {
	ID, Library, Desired, Completed, Pending, SealCursor, Revision, Generation                 int64
	Source, Incarnation, Root, Key, SealJob, Reason, Job, Barrier, BarrierReason, RootIdentity string
	Sealed                                                                                     bool
}

func pathScopeRead(ctx context.Context, tx *sql.Tx, id int64) (pathScope, error) {
	var s pathScope
	err := tx.QueryRowContext(ctx, `SELECT q.id,q.library_id,q.desired,q.completed,q.pending,q.seal_cursor,q.source_id,q.incarnation,q.root_prefix,q.directory_key,q.seal_job,q.reason,q.sealed,COALESCE(i.revision,0),COALESCE(b.job_id,''),COALESCE(b.phase,'open'),COALESCE(b.reason,''),COALESCE(src.generation,0),COALESCE(src.root_identity,'')
 FROM reading_path_scope q LEFT JOIN reading_path_inputs i ON i.library_id=q.library_id LEFT JOIN reading_path_source_barrier b ON b.source_id=q.source_id LEFT JOIN library_sources src ON src.id=q.source_id WHERE q.id=?`, id).Scan(&s.ID, &s.Library, &s.Desired, &s.Completed, &s.Pending, &s.SealCursor, &s.Source, &s.Incarnation, &s.Root, &s.Key, &s.SealJob, &s.Reason, &s.Sealed, &s.Revision, &s.Job, &s.Barrier, &s.BarrierReason, &s.Generation, &s.RootIdentity)
	return s, err
}
func scopeLive(ctx context.Context, tx *sql.Tx, s pathScope) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_sources src JOIN catalog_libraries cl ON cl.library_id=src.library_id JOIN libraries l ON l.id=cl.library_id WHERE src.id=? AND src.incarnation=? AND cl.id=? AND cl.retired=0 AND l.kind IN('book','comic') AND src.root=?)`, s.Source, s.Incarnation, s.Library, strings.TrimSuffix(s.Root, string(filepath.Separator))).Scan(&ok)
	// '/' and a drive root retain their separator.
	if err == nil && !ok {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_sources src JOIN catalog_libraries cl ON cl.library_id=src.library_id JOIN libraries l ON l.id=cl.library_id WHERE src.id=? AND src.incarnation=? AND cl.id=? AND cl.retired=0 AND l.kind IN('book','comic') AND src.root=?)`, s.Source, s.Incarnation, s.Library, s.Root).Scan(&ok)
	}
	return ok, err
}
func scopeReady(ctx context.Context, tx *sql.Tx, s pathScope) (string, error) {
	var current bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reading_path_source_barrier WHERE source_id=? AND incarnation=? AND source_generation=?)`, s.Source, s.Incarnation, s.Generation).Scan(&current); err != nil {
		return "", err
	}
	if !current {
		return "source_changed", nil
	}
	if s.Barrier != "sealed" {
		return "scan_" + s.Barrier, nil
	}
	if s.BarrierReason != "" {
		return s.BarrierReason, nil
	}
	if !s.Sealed || s.SealJob != s.Job {
		return "coverage_pending", nil
	}
	if s.Reason != "" {
		return s.Reason, nil
	}
	if s.Pending != 0 {
		return "availability_pending", nil
	}
	var pending bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reading_path_dirty_work WHERE library_id=?1 AND kind NOT IN('report','coverage')) OR EXISTS(SELECT 1 FROM catalog_dirty WHERE domain=28 AND entity_id=?1) OR EXISTS(SELECT 1 FROM catalog_derivation_failures WHERE domain=28 AND entity_id=?1) OR EXISTS(SELECT 1 FROM catalog_fanouts WHERE domain=28 AND key=?1) OR EXISTS(SELECT 1 FROM catalog_derivations WHERE domain IN(20,28) AND (rebuilding=1 OR cleaning=1 OR first_build=1))`, s.Library).Scan(&pending)
	if pending {
		return "availability_or_input_fanout", err
	}
	return "", err
}
func pathReport(ctx context.Context, tx *sql.Tx, s pathScope, reason string) error {
	if s.Job == "" {
		return nil
	}
	var finished bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=? AND status IN('complete','complete_with_warnings'))`, s.Job).Scan(&finished); err != nil {
		return err
	}
	if !finished {
		return nil
	}
	folder := s.Key
	if folder == "" {
		folder = "."
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO inventory_scan_issues(job_id,relative_path,reason,warning) VALUES(?,?,?,1)`, s.Job, filepath.FromSlash(folder), "reading_path_"+reason)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		log.Printf("Reading COMPLETE unpublished: source=%s folder=%q reason=%s job=%s", s.Source, folder, reason, s.Job)
		for _, q := range []string{`UPDATE inventory_runs SET warnings=warnings+1 WHERE job_id=?`, `UPDATE jobs SET status='complete_with_warnings' WHERE id=? AND status='complete'`} {
			if _, err = tx.ExecContext(ctx, q, s.Job); err != nil {
				return err
			}
		}
	}
	return nil
}

// Coverage reduction consumes exact ancestor keys, at most limit per turn. A
// blocked generic descendant is the same key; a blocked non-generic subtree is
// an ancestor only of contexts that actually depend on that subtree.
func pathSealStep(ctx context.Context, tx *sql.Tx, s pathScope, limit int) (int, error) {
	if s.SealJob != s.Job {
		s.SealCursor = 0
		s.Reason = ""
	}
	keys := []string{""}
	if s.Key != "" {
		for i := range s.Key {
			if s.Key[i] == '/' {
				keys = append(keys, s.Key[:i])
			}
		}
		keys = append(keys, s.Key)
	}
	n := 0
	for int(s.SealCursor) < len(keys) && n < limit {
		var reason string
		err := tx.QueryRowContext(ctx, `SELECT reason FROM reading_path_block WHERE source_id=? AND job_id=? AND directory_key=?`, s.Source, s.Job, keys[s.SealCursor]).Scan(&reason)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if err == nil {
			s.Reason = reason
		}
		s.SealCursor++
		n++
	}
	sealed := int(s.SealCursor) == len(keys)
	_, err := tx.ExecContext(ctx, `UPDATE reading_path_scope SET seal_job=?,seal_cursor=?,sealed=?,reason=? WHERE id=?`, s.Job, s.SealCursor, sealed, s.Reason, s.ID)
	return max(n, 1), err
}

// StepReadingPaths is one durable quantum on the existing supervised catalogue
// worker. Dirty feeds and the builder alternate, so another library's constant
// genuine churn cannot monopolize the slot or prevent queue normalization.
func StepReadingPaths(ctx context.Context, tx *sql.Tx, limit int) (int, error) {
	limit = max(1, min(limit, readingPathBatch))
	turn, err := pathTurn(ctx, tx)
	if err != nil {
		return 0, err
	}
	if turn%2 == 1 {
		n, e := pathWorkQuantum(ctx, tx, limit, turn)
		if n != 0 || e != nil {
			return n, e
		}
	}
	var build sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT build_id FROM reading_path_slot WHERE singleton=1`).Scan(&build); err != nil {
		return 0, err
	}
	if build.Valid {
		return pathBuildStep(ctx, tx, build.Int64, limit)
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM reading_path_scope WHERE queued=1 ORDER BY served,id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return pathWorkQuantum(ctx, tx, limit, turn)
	}
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE reading_path_scope SET served=? WHERE id=?`, turn, id); err != nil {
		return 0, err
	}
	s, err := pathScopeRead(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	live, err := scopeLive(ctx, tx, s)
	if err != nil {
		return 0, err
	}
	if !live {
		return pathRetireScope(ctx, tx, s, limit)
	}
	if s.Key == "" {
		_, err = tx.ExecContext(ctx, `UPDATE reading_path_scope SET completed=desired,queued=0 WHERE id=?`, s.ID)
		return 1, err
	}
	if s.Barrier == "sealed" && (s.SealJob != s.Job || !s.Sealed) {
		return pathSealStep(ctx, tx, s, limit)
	}
	reason, err := scopeReady(ctx, tx, s)
	if err != nil {
		return 0, err
	}
	if reason != "" {
		if err = pathReport(ctx, tx, s, reason); err != nil {
			return 0, err
		}
		return pathWorkQuantum(ctx, tx, limit, turn)
	}
	if err = pathReport(ctx, tx, s, "publication_queued"); err != nil {
		return 0, err
	}
	var bid int64
	err = tx.QueryRowContext(ctx, `INSERT INTO reading_path_build(scope_id,desired,input_revision,source_generation,root_identity,job_id,phase) VALUES(?,?,?,?,?,?,'capture') RETURNING id`, s.ID, s.Desired, s.Revision, s.Generation, s.RootIdentity, s.Job).Scan(&bid)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE reading_path_slot SET build_id=? WHERE singleton=1`, bid)
	return 1, err
}
func pathRetireScope(ctx context.Context, tx *sql.Tx, s pathScope, limit int) (int, error) {
	// Public deletion already removed authority. Private cleanup never refuses it.
	r, err := tx.ExecContext(ctx, `DELETE FROM reading_path_object_keys WHERE object_id IN(SELECT object_id FROM reading_path_object_keys WHERE scope_id=? ORDER BY object_id LIMIT ?)`, s.ID, limit)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	if n > 0 {
		return int(n), nil
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM reading_path_scope WHERE id=?`, s.ID)
	return 1, err
}

type pathBuild struct {
	ID, Scope, Desired, Revision, Generation, Item, Asset, Titles, NonSeries, Sequence, FlatItem, FlatAsset int64
	RootIdentity, Job, Phase, Object                                                                        string
}

func pathBuildStep(ctx context.Context, tx *sql.Tx, id int64, limit int) (int, error) {
	var b pathBuild
	b.ID = id
	err := tx.QueryRowContext(ctx, `SELECT scope_id,desired,input_revision,source_generation,root_identity,job_id,phase,after_object,after_item,after_asset,title_count,non_series_count,COALESCE(publish_sequence,0),after_flatten_item,after_flatten_asset FROM reading_path_build WHERE id=?`, id).Scan(&b.Scope, &b.Desired, &b.Revision, &b.Generation, &b.RootIdentity, &b.Job, &b.Phase, &b.Object, &b.Item, &b.Asset, &b.Titles, &b.NonSeries, &b.Sequence, &b.FlatItem, &b.FlatAsset)
	if err != nil {
		return 0, err
	}
	s, err := pathScopeRead(ctx, tx, b.Scope)
	if err != nil {
		return 0, err
	}
	if b.Phase == "capture" || b.Phase == "stage" || b.Phase == "ready" {
		reason, e := scopeReady(ctx, tx, s)
		if e != nil {
			return 0, e
		}
		live, e := scopeLive(ctx, tx, s)
		if e != nil {
			return 0, e
		}
		if reason != "" || !live || b.Desired != s.Desired || b.Revision != s.Revision || b.Generation != s.Generation || b.RootIdentity != s.RootIdentity || b.Job != s.Job {
			_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET phase='abort' WHERE id=?`, id)
			return 1, err
		}
	}
	switch b.Phase {
	case "capture":
		return pathCaptureStep(ctx, tx, b, s, limit)
	case "stage":
		return pathStageStep(ctx, tx, b, s, limit)
	case "ready":
		seq, e := pathSequence(ctx, tx)
		if e != nil {
			return 0, e
		}
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`UPDATE reading_path_build SET phase='flatten',publish_sequence=? WHERE id=?`, []any{seq, id}},
			{`UPDATE reading_path_slot SET overlay_id=? WHERE singleton=1`, []any{id}},
			{`UPDATE reading_path_scope SET completed=?,queued=(desired<>?) WHERE id=?`, []any{b.Desired, b.Desired, b.Scope}},
		} {
			if _, err = tx.ExecContext(ctx, q.sql, q.args...); err != nil {
				return 0, err
			}
		}
		if err = pathSemantic(ctx, tx, s.Library); err == nil {
			err = FlushReadingPathSemanticTx(ctx, tx)
		}
		return 1, err
	case "flatten":
		return pathFlattenStep(ctx, tx, b, limit)
	case "cleanup", "abort":
		for _, table := range []string{"reading_path_capture", "reading_path_representative", "reading_path_stage"} {
			// Every table is WITHOUT ROWID with a build-leading primary key.
			key := "item_id,asset_id"
			if table == "reading_path_representative" {
				key = "item_id"
			}
			r, e := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE build_id=?1 AND (`+key+`) IN(SELECT `+key+` FROM `+table+` WHERE build_id=?1 ORDER BY `+key+` LIMIT ?2)`, id, limit)
			if e != nil {
				return 0, e
			}
			n, _ := r.RowsAffected()
			if n > 0 {
				return int(n), nil
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE reading_path_slot SET build_id=NULL WHERE singleton=1 AND overlay_id IS NULL`); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM reading_path_build WHERE id=?`, id)
		return 1, err
	}
	return 0, fmt.Errorf("unknown Reading path phase %q", b.Phase)
}
func pathCaptureStep(ctx context.Context, tx *sql.Tx, b pathBuild, s pathScope, limit int) (int, error) {
	// Materialize the RAW window before eligibility. Rejected prefixes still
	// advance the durable cursor and consume budget. Point joins never hide EOF.
	ids, err := pathStrings(ctx, tx, `WITH raw AS MATERIALIZED (SELECT object_id FROM reading_path_object_keys WHERE scope_id=? AND object_id>? ORDER BY object_id LIMIT ?) SELECT object_id FROM raw ORDER BY object_id`, s.ID, b.Object, limit)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET phase='stage' WHERE id=?`, b.ID)
		return 1, err
	}
	for _, id := range ids {
		v, e := readPathObject(ctx, tx, id)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return 0, e
		}
		if !v.Eligible {
			continue
		}
		in := ReadingRuleInput{Item: v.Item, SourceRoot: v.Source, RelativePath: filepath.ToSlash(v.Relative), Kind: v.Kind}
		key, evidence, e := ReadingPathFolderEvidence(in)
		if e != nil {
			return 0, e
		}
		if key != s.Key || v.Source != s.Source || v.Incarnation != s.Incarnation || rootPrefix(v.Root) != s.Root {
			continue
		}
		// Duplicate aliases can name the same publication; preserve the first object
		// in exact object-id driver order without changing its representative twice.
		r, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO reading_path_capture VALUES(?,?,?,?,?,?,?,?)`, b.ID, v.Item, v.Asset, v.ID, v.Revision, v.Relative, v.Kind, evidence)
		if e != nil {
			return 0, e
		}
		added, _ := r.RowsAffected()
		if added == 0 {
			continue
		}
		var asset int64
		var oldEvidence bool
		e = tx.QueryRowContext(ctx, `SELECT asset_id,series_evidence FROM reading_path_representative WHERE build_id=? AND item_id=?`, b.ID, v.Item).Scan(&asset, &oldEvidence)
		if errors.Is(e, sql.ErrNoRows) {
			b.Titles++
			if !evidence {
				b.NonSeries++
			}
			_, e = tx.ExecContext(ctx, `INSERT INTO reading_path_representative VALUES(?,?,?,?)`, b.ID, v.Item, v.Asset, evidence)
		} else if e == nil && v.Asset < asset {
			if !oldEvidence {
				b.NonSeries--
			}
			if !evidence {
				b.NonSeries++
			}
			_, e = tx.ExecContext(ctx, `UPDATE reading_path_representative SET asset_id=?,series_evidence=? WHERE build_id=? AND item_id=?`, v.Asset, evidence, b.ID, v.Item)
		}
		if e != nil {
			return 0, e
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET after_object=?,title_count=?,non_series_count=? WHERE id=?`, ids[len(ids)-1], b.Titles, b.NonSeries, b.ID)
	return len(ids), err
}
func pathStageStep(ctx context.Context, tx *sql.Tx, b pathBuild, s pathScope, limit int) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item_id,asset_id,object_id,source_revision,relative_path,item_kind FROM reading_path_capture WHERE build_id=? AND (item_id,asset_id)>(?,?) ORDER BY item_id,asset_id LIMIT ?`, b.ID, b.Item, b.Asset, limit)
	if err != nil {
		return 0, err
	}
	type row struct {
		item, asset                int64
		object, revision, relative string
		kind                       Kind
	}
	var batch []row
	for rows.Next() {
		var v row
		if err = rows.Scan(&v.item, &v.asset, &v.object, &v.revision, &v.relative, &v.kind); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET phase='ready' WHERE id=?`, b.ID)
		return 1, err
	}
	for _, v := range batch {
		values, e := ReadingPathValue(ReadingRuleInput{Item: v.item, SourceRoot: s.Source, RelativePath: filepath.ToSlash(v.relative), Kind: v.kind}, int(min(b.Titles, 2)), b.NonSeries == 0)
		if e != nil {
			return 0, e
		}
		value, e := bundle(v.object, v.revision, values)
		if e != nil {
			return 0, e
		}
		// Association deletion cascades staged rows. Never recreate a deleted one.
		_, e = tx.ExecContext(ctx, `INSERT INTO reading_path_stage(build_id,item_id,asset_id,`+pathColumns+`) SELECT ?,?,?,`+strings.TrimSuffix(strings.Repeat("?,", 8), ",")+` WHERE EXISTS(SELECT 1 FROM reading_publications WHERE item_id=? AND asset_id=?)`, append(append([]any{b.ID, v.item, v.asset}, value.args()...), v.item, v.asset)...)
		if e != nil {
			return 0, e
		}
	}
	last := batch[len(batch)-1]
	_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET after_item=?,after_asset=? WHERE id=?`, last.item, last.asset, b.ID)
	return len(batch), err
}
func pathFlattenStep(ctx context.Context, tx *sql.Tx, b pathBuild, limit int) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item_id,asset_id,`+pathColumns+` FROM reading_path_stage WHERE build_id=? AND (item_id,asset_id)>(?,?) ORDER BY item_id,asset_id LIMIT ?`, b.ID, b.FlatItem, b.FlatAsset, limit)
	if err != nil {
		return 0, err
	}
	type row struct {
		item, asset int64
		value       pathBundle
	}
	var batch []row
	for rows.Next() {
		var v row
		if err = rows.Scan(append([]any{&v.item, &v.asset}, v.value.scan()...)...); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE reading_path_slot SET overlay_id=NULL WHERE singleton=1`); err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET phase='cleanup' WHERE id=?`, b.ID)
		return 1, err
	}
	for _, v := range batch {
		if err = writePathHead(ctx, tx, v.item, v.asset, b.Sequence, v.value); err != nil {
			return 0, err
		}
	}
	last := batch[len(batch)-1]
	_, err = tx.ExecContext(ctx, `UPDATE reading_path_build SET after_flatten_item=?,after_flatten_asset=? WHERE id=?`, last.item, last.asset, b.ID)
	return len(batch), err
}

func readingPathAutomaticTx(ctx context.Context, tx *sql.Tx, items []int64, out map[int64]ReadingValues) error {
	for _, item := range items {
		var asset int64
		err := tx.QueryRowContext(ctx, `SELECT p.asset_id FROM catalog_entities e JOIN reading_publications p ON p.item_id=e.id WHERE e.id=? AND ((e.kind=20 AND p.format IN('epub','pdf','mobi','azw3')) OR(e.kind=21 AND p.format IN('cbz','pdf','cb7','cbr'))) ORDER BY EXISTS(SELECT 1 FROM inventory_item_availability av WHERE av.item_id=p.item_id AND av.asset_id=(SELECT token FROM catalog_assets WHERE id=p.asset_id) AND av.available=1) DESC,p.asset_id LIMIT 1`, item).Scan(&asset)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		winner, exists, err := pathWinner(ctx, tx, item, asset)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		accepted, err := pathAccepted(ctx, tx, item, asset, winner)
		if err != nil {
			return err
		}
		if accepted {
			out[item] = winner.values()
		}
	}
	return nil
}
