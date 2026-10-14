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
func (s *Service) searchTitleWalk(ctx context.Context, r SearchRequest, kind int, where string, filterArgs []any, limit int, scopeHash string, g searchGroupDefinition, rev ContentRevision) (string, bool, error) {
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
		c, err := s.decodeRevisionCursor(r.Cursor, s.groupCursorScope(r, g, scopeHash, "title"), rev)
		if err != nil {
			return "", false, nil // the ranked path reports the cursor
		}
		id, err := strconv.ParseInt(c.ID, 10, 64)
		if err != nil {
			return "", false, nil
		}
		bound, boundArgs = ` AND (sort_key,id)`+op+`(?,?)`, []any{c.Value, id}
	}
	probes, probeArgs := []string{}, []any{}
	for _, term := range terms {
		predicate := `t.terminal=0 AND t.prefix=?`
		probeArgs = append(probeArgs, term)
		if facettext.NumericSearchTerm(term) {
			predicate = `(` + predicate + ` OR t.terminal=1 AND t.prefix>=? AND t.prefix<?)`
			probeArgs = append(probeArgs, term, term+"\U0010ffff")
		}
		probes = append(probes, `EXISTS(SELECT 1 FROM catalog_search_prefix_rows p INDEXED BY catalog_search_prefix_rows_entity JOIN catalog_search_prefix_terms t ON t.id=p.term_id WHERE p.entity_id=e.id AND `+predicate+`)`)
	}
	// The window's last title bounds the walk; the walk itself reads the
	// title index in order and stops at the page, so a broad word reads a
	// few dozen titles.
	last := `SELECT sort_key,id FROM catalog_entities INDEXED BY catalog_entities_title WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND kind=?` + bound + ` ORDER BY sort_key ` + direction + `,id ` + direction + ` LIMIT 1 OFFSET ` + strconv.Itoa(titleWalkWindow-1)
	query := `SELECT e.id,d.entity_pid,e.sort_key FROM catalog_entities e INDEXED BY catalog_entities_title
 CROSS JOIN catalog_search_documents d ON d.entity_id=e.id JOIN catalog_libraries l ON l.id=e.library_id
 WHERE e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.kind=?` + strings.ReplaceAll(bound, "(sort_key,id)", "(e.sort_key,e.id)") + `{window} AND ` + strings.Join(probes, " AND ") + ` AND 1` + where + ` ORDER BY e.sort_key ` + direction + `,e.id ` + direction + ` LIMIT ?`
	inside := "<="
	if direction == "DESC" {
		inside = ">="
	}
	rows := [][3]any{}
	for _, library := range r.Libraries {
		var endKey string
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
			cap, capArgs = ` AND (e.sort_key,e.id)`+inside+`(?,?)`, []any{endKey, endID}
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
		var public, key string
		if err = rows.Scan(&id, &public, &key); err != nil {
			return nil, err
		}
		out = append(out, [3]any{id, public, key})
	}
	return out, rows.Err()
}
