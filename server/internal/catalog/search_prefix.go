package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"portico.local/server/internal/facettext"
	"strconv"
	"strings"
)

// Each authorized library supplies one bounded stream. The final top-k merges
// at most libraries*(limit+1) narrow rows. Exact title, title word and context
// are maintained ranking tiers; their tie order is the stable internal identity.
func (s *Service) prefixSearchCandidates(ctx context.Context, r SearchRequest, kind int, terms []string, where string, filterArgs []any, limit int, scope cursorScope, rev ContentRevision) (string, []any, error) {
	ids := make([][]int64, len(terms))
	lead, total := 0, int64(^uint64(0)>>1)
	raw, _ := json.Marshal(r.Libraries)
	for i, term := range terms {
		var err error
		ids[i], err = s.prefixTermIDs(ctx, term)
		if err != nil {
			return "", nil, err
		}
		if r.Direction == "asc" && len(ids[i]) > 1 {
			return "", nil, errPrefixFallback
		}
		list, _ := json.Marshal(ids[i])
		var count int64
		if err = s.read().QueryRowContext(ctx, `SELECT COALESCE(sum(total),0) FROM catalog_search_prefix_counts WHERE term_id IN(SELECT value FROM json_each(?)) AND kind=? AND library_id IN(SELECT id FROM catalog_libraries WHERE retired=0 AND library_id IN(SELECT value FROM json_each(?)))`, string(list), kind, string(raw)).Scan(&count); err != nil {
			return "", nil, err
		}
		if count < total {
			lead, total = i, count
		}
	}
	extra := ""
	extraArgs := []any{}
	for i, list := range ids {
		if i != lead {
			raw, _ := json.Marshal(list)
			extra += ` AND EXISTS(SELECT 1 FROM catalog_search_prefix_rows other INDEXED BY catalog_search_prefix_rows_entity WHERE other.entity_id=p.entity_id AND other.term_id IN(SELECT value FROM json_each(?)))`
			extraArgs = append(extraArgs, string(raw))
		}
	}
	if len(ids[lead]) > 1 {
		raw, _ := json.Marshal(ids[lead])
		extra += ` AND p.tier=(SELECT max(best.tier) FROM catalog_search_prefix_rows best INDEXED BY catalog_search_prefix_rows_entity WHERE best.entity_id=p.entity_id AND best.term_id IN(SELECT value FROM json_each(?)))`
		extraArgs = append(extraArgs, string(raw))
	}
	if len(ids[lead])*len(r.Libraries) > 128 || len(r.Libraries) > 64 {
		return "", nil, errPrefixFallback
	}
	direction, op := r.Direction, ">"
	if direction == "desc" {
		op = "<"
	}
	var cursorTier float64
	var cursorID int64
	if r.Cursor != "" {
		cursor, err := s.decodeRevisionCursor(r.Cursor, scope, rev)
		if err != nil {
			return "", nil, err
		}
		cursorTier, err = strconv.ParseFloat(cursor.Value, 64)
		if err != nil {
			return "", nil, ErrCursor
		}
		cursorID, err = strconv.ParseInt(cursor.ID, 10, 64)
		if err != nil {
			return "", nil, ErrCursor
		}
	}
	streams := []string{}
	args := []any{}
	fold := facettext.SearchFold(r.Q)
	for _, library := range r.Libraries {
		for _, termID := range ids[lead] {
			addStream := func(bound string, boundArgs []any) {
				streams = append(streams, `SELECT entity_id,entity_pid,ord FROM(SELECT p.entity_id,d.entity_pid entity_pid,p.tier AS ord FROM catalog_search_prefix_rows p
 CROSS JOIN catalog_search_documents d ON d.entity_id=p.entity_id CROSS JOIN catalog_entities e ON e.id=p.entity_id JOIN catalog_libraries l ON l.id=e.library_id
 WHERE p.term_id=? AND p.kind=? AND p.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?)`+extra+` AND d.title_fold<>? AND 1`+where+bound+` ORDER BY p.tier `+direction+`,p.entity_id `+direction+` LIMIT ?)`)
				args = append(args, termID, kind, library)
				args = append(args, extraArgs...)
				args = append(args, fold)
				args = append(args, filterArgs...)
				args = append(args, boundArgs...)
				args = append(args, limit)
			}
			if r.Cursor == "" {
				addStream("", nil)
			} else {
				addStream(` AND p.tier=? AND p.entity_id`+op+`?`, []any{cursorTier, cursorID})
				addStream(` AND p.tier`+op+`?`, []any{cursorTier})
			}
		}
		// Exact titles have a separate covering seek, including multi-word
		// titles, so they cannot be crowded out by a broad prefix stream.
		exactBound := ""
		exactArgs := []any{}
		include := r.Cursor == "" || direction == "desc" && cursorTier >= 3 || direction == "asc" && cursorTier <= 3
		if r.Cursor != "" && cursorTier == 3 {
			exactBound = ` AND e.id` + op + `?`
			exactArgs = []any{cursorID}
		}
		if include {
			streams = append(streams, `SELECT entity_id,entity_pid,ord FROM(SELECT d.entity_id,d.entity_pid entity_pid,3 AS ord FROM catalog_search_documents d
 CROSS JOIN catalog_entities e ON e.id=d.entity_id JOIN catalog_libraries l ON l.id=e.library_id
 WHERE d.title_fold=? AND d.entity_kind=? AND d.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND 1`+where+exactBound+` ORDER BY d.entity_id `+direction+` LIMIT ?)`)
			args = append(args, fold, kind, library)
			args = append(args, filterArgs...)
			args = append(args, exactArgs...)
			args = append(args, limit)
		}
	}
	if len(streams) == 0 {
		return `SELECT 0 entity_id,'' entity_pid,0 ord WHERE 0`, nil, nil
	}
	return strings.Join(streams, " UNION ALL "), args, nil
}

func (s *Service) prefixSearchTotal(ctx context.Context, r SearchRequest, kind int, term string) (int, bool, error) {
	if len(r.Restrictions.MemberDeniedLabels) > 0 {
		return 0, false, nil
	}
	var pending, retired bool
	if err := s.read().QueryRowContext(ctx, `SELECT (EXISTS(SELECT 1 FROM catalog_dirty WHERE domain=17) OR COALESCE((SELECT rebuilding FROM catalog_derivations WHERE domain=17),0)),EXISTS(SELECT 1 FROM dvr_catalog_retirements)`).Scan(&pending, &retired); err != nil {
		return 0, false, err
	}
	if pending || retired {
		return 0, false, nil
	}
	terms, err := s.prefixTermIDs(ctx, term)
	if err != nil {
		return 0, false, err
	}
	if len(terms) > 1 {
		return 0, false, nil
	}
	termID := int64(0)
	if len(terms) == 1 {
		termID = terms[0]
	}
	raw, _ := json.Marshal(r.Libraries)
	var ages ratingAges
	if r.Restrictions.Active() {
		ages, err = s.ratingAges()
		if err != nil {
			return 0, false, err
		}
	}
	blocked := map[string]bool{}
	for _, label := range r.Restrictions.BlockedLabels {
		blocked[strings.ToLower(label)] = true
	}
	rows, err := s.read().QueryContext(ctx, `SELECT container,rating_key,label_key,total FROM catalog_search_prefix_counts WHERE term_id=? AND kind=? AND library_id IN(SELECT id FROM catalog_libraries WHERE retired=0 AND library_id IN(SELECT value FROM json_each(?)))`, termID, kind, string(raw))
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var b browseBucket
		if err = rows.Scan(&b.container, &b.ratingKey, &b.labelKey, &b.total); err != nil {
			return 0, false, err
		}
		if b.container && r.Restrictions.Active() {
			return 0, false, nil
		}
		if !r.Restrictions.Active() || bucketVisible(b, ages, r.Restrictions, blocked) {
			total += b.total
		}
	}
	if err = rows.Err(); err != nil {
		return 0, false, err
	}
	return total, true, nil
}

var errPrefixFallback = errors.New("prefix dictionary requires the full-text selector")

// Numeric ranges enumerate at most 128 terminal keys. A larger range keeps
// the full-text selector; no matches are silently truncated.
func (s *Service) prefixTermIDs(ctx context.Context, term string) ([]int64, error) {
	query := `SELECT id FROM catalog_search_prefix_terms WHERE terminal=0 AND prefix=?`
	args := []any{term}
	if facettext.NumericSearchTerm(term) {
		query = `SELECT id FROM catalog_search_prefix_terms WHERE terminal=0 AND prefix=? UNION ALL SELECT id FROM(SELECT id FROM catalog_search_prefix_terms WHERE terminal=1 AND prefix>=? AND prefix<? ORDER BY prefix LIMIT 129)`
		args = []any{term, term, term + "\U0010ffff"}
	}
	rows, err := s.read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > 128 {
		return nil, errPrefixFallback
	}
	return ids, nil
}
