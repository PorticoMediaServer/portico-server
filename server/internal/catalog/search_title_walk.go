package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"portico.local/server/internal/facettext"
)

// searchWalk is an ordered index a sorted search reads in its own order: the
// key it orders by (as the ranked path computes it), the row identity (the
// entity id, the tie break of both paths) and the library and kind scope.
type searchWalk struct {
	from, join, scope, kind, key, order, id string
	numeric                                 bool
}

// searchWalks are the sorts a search can answer from an ordered index rather
// than by ranking every match: title from the title index, release year and
// date added from the browse rows' own indexes.
var searchWalks = map[string]searchWalk{
	"title": {from: `catalog_entities e INDEXED BY catalog_entities_title`, scope: "e.library_id", kind: "e.kind", key: "e.sort_key", order: "e.sort_key", id: "e.id"},
	"releaseYear": {from: `catalog_browse_rows wb INDEXED BY catalog_browse_year`, join: ` CROSS JOIN catalog_entities e ON e.id=wb.entity_id`, scope: "wb.library_id", kind: "wb.kind",
		key: "CAST(wb.year AS REAL)", order: "wb.year", id: "wb.entity_id", numeric: true},
	"dateAdded": {from: `catalog_browse_rows wb INDEXED BY catalog_browse_added`, join: ` CROSS JOIN catalog_entities e ON e.id=wb.entity_id`, scope: "wb.library_id", kind: "wb.kind",
		key: "COALESCE(wb.added_text,'')", order: "COALESCE(wb.added_text,'')", id: "wb.entity_id"},
}

// titleWalkWindow is how many titles of one library a title-sorted search
// reads in title order before it gives up and ranks the full-text matches.
var titleWalkWindow = 4096

// searchTitleWalk answers a broad title-sorted search from the title index:
// each authorized library's titles are read in title order, at most
// titleWalkWindow of them, and each is tested against the typed words'
// prefix postings. A word that matches a large share of the library fills the
// page within a few dozen titles, where ranking every full-text match cost
// the whole match set. ok is false when a library's window held neither a
// full page nor its last title, or when the words are rare enough that the
// full-text matches are cheaper; the caller then ranks the matches.
//
// The result is a JSON list of [entity id, public id, sort key] rows, the
// union of each library's first `limit` matches after the cursor, which holds
// the overall first `limit`.
func (s *Service) searchTitleWalk(ctx context.Context, r SearchRequest, kind int, sortID, where string, filterArgs []any, limit int, scopeHash string, g searchGroupDefinition, rev ContentRevision) (string, bool, error) {
	walk, ok := searchWalks[sortID]
	if !ok {
		return "", false, nil
	}
	terms, indexed := facettext.IndexedSearchTerms(r.Q)
	if !indexed || len(terms) == 0 || len(r.Libraries) == 0 || len(r.Libraries) > 64 {
		return "", false, nil
	}
	// Rare words: their matches are fewer than the window would read.
	raw, _ := json.Marshal(r.Libraries)
	for _, term := range terms {
		ids, err := s.prefixTermIDs(ctx, term, kind, r.Libraries)
		if err != nil {
			continue // a wide numeric word: its share is unknown, so try the walk
		}
		list, _ := json.Marshal(ids)
		var count int
		if err = s.read().QueryRowContext(ctx, `SELECT COALESCE(sum(total),0) FROM catalog_search_prefix_counts WHERE term_id IN(SELECT value FROM json_each(?)) AND kind=? AND container=-1 AND library_id IN(SELECT id FROM catalog_libraries WHERE retired=0 AND library_id IN(SELECT value FROM json_each(?)))`, string(list), kind, string(raw)).Scan(&count); err != nil {
			return "", false, err
		}
		if count < titleWalkWindow/4 {
			return "", false, nil
		}
	}
	direction, op := "ASC", ">"
	if r.Direction == "desc" {
		direction, op = "DESC", "<"
	}
	bound, boundArgs := "", []any{}
	if r.Cursor != "" {
		c, err := s.decodeRevisionCursor(r.Cursor, s.groupCursorScope(r, g, scopeHash, sortID), rev)
		if err != nil {
			return "", false, nil // the ranked path reports the cursor
		}
		id, err := strconv.ParseInt(c.ID, 10, 64)
		if err != nil {
			return "", false, nil
		}
		value, err := cursorOrderValue(c.Value, walk.numeric)
		if err != nil {
			return "", false, nil
		}
		bound, boundArgs = ` AND (`+walk.key+`,`+walk.id+`)`+op+`(?,?)`, []any{value, id}
	}
	probes, probeArgs := []string{}, []any{}
	for _, term := range terms {
		probeArgs = append(probeArgs, term)
		probes = append(probes, `EXISTS(SELECT 1 FROM catalog_search_prefix_rows p INDEXED BY catalog_search_prefix_rows_entity WHERE p.entity_id=e.id AND p.term_id=(SELECT id FROM catalog_search_prefix_terms WHERE prefix=?))`)
	}
	// The window's last title bounds the walk; the walk itself reads the
	// title index in order and stops at the page, so a broad word reads a
	// few dozen titles.
	scope := walk.from + ` WHERE ` + walk.scope + `=(SELECT id FROM catalog_libraries WHERE library_id=?) AND ` + walk.kind + `=?`
	last := `SELECT ` + walk.key + `,` + walk.id + ` FROM ` + scope + bound + ` ORDER BY ` + walk.order + ` ` + direction + `,` + walk.id + ` ` + direction + ` LIMIT 1 OFFSET ` + strconv.Itoa(titleWalkWindow-1)
	query := `SELECT e.id,d.entity_pid,` + walk.key + ` FROM ` + walk.from + walk.join + `
 CROSS JOIN catalog_search_documents d ON d.entity_id=e.id JOIN catalog_libraries l ON l.id=e.library_id
 WHERE ` + walk.scope + `=(SELECT id FROM catalog_libraries WHERE library_id=?) AND ` + walk.kind + `=?` + bound + `{window} AND ` + strings.Join(probes, " AND ") + ` AND 1` + where + ` ORDER BY ` + walk.order + ` ` + direction + `,` + walk.id + ` ` + direction + ` LIMIT ?`
	inside := "<="
	if direction == "DESC" {
		inside = ">="
	}
	rows := [][3]any{}
	for _, library := range r.Libraries {
		var endKey any
		var endID int64
		capped := true
		err := s.read().QueryRowContext(ctx, last, append([]any{library, kind}, boundArgs...)...).Scan(&endKey, &endID)
		if errors.Is(err, sql.ErrNoRows) {
			capped = false // the library has fewer titles left than the window
		} else if err != nil {
			return "", false, err
		}
		cap, capArgs := "", []any{}
		if capped {
			cap, capArgs = ` AND (`+walk.key+`,`+walk.id+`)`+inside+`(?,?)`, []any{endKey, endID}
		}
		args := append([]any{library, kind}, boundArgs...)
		args = append(args, capArgs...)
		args = append(args, probeArgs...)
		args = append(args, filterArgs...)
		args = append(args, limit)
		found, err := s.titleWalkRows(ctx, strings.Replace(query, "{window}", cap, 1), args)
		if err != nil {
			return "", false, err
		}
		if len(found) < limit && capped {
			return "", false, nil // the window held neither a page nor the end
		}
		rows = append(rows, found...)
	}
	out, _ := json.Marshal(rows)
	return string(out), true, nil
}

func (s *Service) titleWalkRows(ctx context.Context, query string, args []any) ([][3]any, error) {
	rows, err := s.read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := [][3]any{}
	for rows.Next() {
		var id int64
		var public string
		var key any
		if err = rows.Scan(&id, &public, &key); err != nil {
			return nil, err
		}
		out = append(out, [3]any{id, public, key})
	}
	return out, rows.Err()
}
