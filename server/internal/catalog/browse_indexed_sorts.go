package catalog

import (
	"strconv"
)

// Exact keys travel with the selected rows so a changed or removed anchor does
// not change the meaning of a continuation. SQL selects only a bounded page.
func (s *Service) queryBrowseKeyRows(query, field string, args ...any) ([]browseRow, error) {
	rows, err := s.read().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []browseRow{}
	for rows.Next() {
		var r browseRow
		var key any
		if err = rows.Scan(&r.id, &r.entity, &r.kind, &r.title, &r.year, &r.added, &r.sortKey, &r.duration, &r.rating, &key); err != nil {
			return nil, err
		}
		text := formatOrderValue(key)
		if n, ok := key.(int64); ok {
			text = strconv.FormatInt(n, 10)
		}
		r.keys = map[string]string{field: text}
		out = append(out, r)
	}
	return out, rows.Err()
}

func indexedSortValue(field, key string) (any, error) {
	switch field {
	case "criticRating", "personalRating":
		return strconv.ParseFloat(key, 64)
	case "episodeCount", "trackNumber", "fileSize", "resolution", "random", "playCount":
		return strconv.ParseInt(key, 10, 64)
	}
	return key, nil
}

func (s *Service) indexedFactPage(p pageShape, definition browseSort, desc bool) ([]browseRow, error) {
	// Direct ranges on orders without counted blocks walk bounded keysets. This
	// also keeps filtered ranges correct without an unbounded OFFSET.
	if p.start > 0 && len(p.seekKeys) == 0 {
		return s.walkIndexedRange(p, func(p pageShape) ([]browseRow, error) { return s.indexedFactPage(p, definition, desc) })
	}
	if definition.ID == "random" {
		return s.seededRandomPage(p, definition, desc)
	}
	where, args := p.where, append([]any{}, p.args...)
	direction, op := "ASC", ">"
	if desc {
		direction, op = "DESC", "<"
	}
	if len(p.seekKeys) > 0 {
		key, err := indexedSortValue(definition.ID, p.seekKeys[0])
		if err != nil {
			return nil, ErrCursor
		}
		id, err := strconv.ParseInt(p.seekAnchor, 10, 64)
		if err != nil {
			return nil, ErrCursor
		}
		where += ` AND (` + definition.Expression + op + `? OR ` + definition.Expression + `=? AND e.entity_id` + op + `?)`
		args = append(args, key, key, id)
	}
	args = append(args, p.limit)
	return s.queryBrowseKeyRows(`SELECT `+browseRowColumns+`,`+definition.Expression+` FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE `+where+` ORDER BY `+definition.Expression+` `+direction+`,e.entity_id `+direction+` LIMIT ?`, definition.ID, args...)
}

func (s *Service) walkIndexedRange(p pageShape, page func(pageShape) ([]browseRow, error)) ([]browseRow, error) {
	skip, want := p.start, p.limit
	p.start = 0
	p.limit = BrowseMaximumLimit
	for skip > 0 {
		p.limit = min(skip, BrowseMaximumLimit)
		rows, err := page(p)
		if err != nil || len(rows) == 0 {
			return rows, err
		}
		skip -= len(rows)
		last := rows[len(rows)-1]
		p.seekAnchor = strconv.FormatInt(last.entity, 10)
		p.seekKeys = []string{browseSortValue(last, p.sorts[0].Field)}
		if p.sorts[0].Field == "personalRating" || p.sorts[0].Field == "lastPlayed" || p.sorts[0].Field == "playCount" {
			p.seekKeys = append(p.seekKeys, last.keys["phase"], last.keys["restTitle"])
		}
	}
	p.limit = want
	return page(p)
}

// A seed rotates a maintained pseudorandom index. Both halves are index walks;
// hashing or sorting the whole library per request is unnecessary.
func (s *Service) seededRandomPage(p pageShape, definition browseSort, desc bool) ([]browseRow, error) {
	seed := int64(p.sorts[0].Seed)
	phase := 0
	direction, op, firstBound, secondBound := "ASC", ">", `e.random_key>=?`, `e.random_key<?`
	if desc {
		direction, op, firstBound, secondBound = "DESC", "<", `e.random_key<=?`, `e.random_key>?`
	}
	var last any
	var anchor int64
	if len(p.seekKeys) > 0 {
		value, err := strconv.ParseInt(p.seekKeys[0], 10, 64)
		if err != nil {
			return nil, ErrCursor
		}
		last = value
		anchor, err = strconv.ParseInt(p.seekAnchor, 10, 64)
		if err != nil {
			return nil, ErrCursor
		}
		if !desc && value < seed || desc && value > seed {
			phase = 1
		}
	}
	out := []browseRow{}
	for ; phase < 2 && len(out) < p.limit; phase++ {
		bound := firstBound
		if phase == 1 {
			bound = secondBound
		}
		args := append(append([]any{}, p.args...), seed)
		if last != nil {
			bound += ` AND (e.random_key` + op + `? OR e.random_key=? AND e.entity_id` + op + `?)`
			args = append(args, last, last, anchor)
		}
		args = append(args, p.limit-len(out))
		rows, err := s.queryBrowseKeyRows(`SELECT `+browseRowColumns+`,e.random_key FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE `+p.where+` AND `+bound+` ORDER BY e.random_key `+direction+`,e.entity_id `+direction+` LIMIT ?`, definition.ID, args...)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		last = nil
	}
	return out, nil
}

// Sparse maintained profile keys serve valued rows. Rows with no value use the
// maintained title order. A cursor says which stream it is in; neither history
// nor the library is collected and sorted in application memory.
func (s *Service) indexedPersonalPage(p pageShape, definition browseSort, desc bool) ([]browseRow, error) {
	if p.start > 0 && len(p.seekKeys) == 0 {
		return s.walkIndexedRange(p, func(p pageShape) ([]browseRow, error) { return s.indexedPersonalPage(p, definition, desc) })
	}
	column, zero, index := "rating", "0", "catalog_personal_order_rating"
	if definition.ID == "lastPlayed" {
		column, zero, index = "last_played_at", "''", "catalog_personal_order_last"
	}
	if definition.ID == "playCount" {
		column, zero, index = "play_count", "0", "catalog_personal_order_plays"
	}
	phases := []string{"rest", "valued"}
	if desc {
		phases = []string{"valued", "rest"}
	}
	at := 0
	var anchor int64
	if len(p.seekKeys) > 0 {
		if len(p.seekKeys) != 3 {
			return nil, ErrCursor
		}
		if p.seekKeys[1] == phases[1] {
			at = 1
		} else if p.seekKeys[1] != phases[0] {
			return nil, ErrCursor
		}
		var err error
		anchor, err = strconv.ParseInt(p.seekAnchor, 10, 64)
		if err != nil {
			return nil, ErrCursor
		}
	}
	out := []browseRow{}
	for ; at < 2 && len(out) < p.limit; at++ {
		phase := phases[at]
		where, args := p.where, []any{}
		from := `catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id`
		key, order, value := "e.sort_key COLLATE NOCASE", "e.sort_key COLLATE NOCASE ASC,e.entity_id ASC", zero
		if phase == "valued" {
			from = `catalog_personal_sort_rows personal INDEXED BY ` + index + ` CROSS JOIN catalog_browse_rows e ON e.entity_id=personal.entity_id CROSS JOIN catalog_entities ce ON ce.id=e.entity_id`
			where = `personal.profile_id=? AND personal.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND personal.kind=? AND personal.` + column + `>` + zero + ` AND ` + where
			args = []any{p.request.Profile, p.request.Library, p.kind}
			key, value = "personal."+column, "personal."+column
			direction := "ASC"
			if desc {
				direction = "DESC"
			}
			order = key + ` ` + direction + `,personal.entity_id ` + direction
		} else {
			where += ` AND NOT EXISTS(SELECT 1 FROM catalog_personal_sort_rows personal WHERE personal.profile_id=? AND personal.entity_id=e.entity_id AND personal.` + column + `>` + zero + `)`
		}
		args = append(args, p.args...)
		if phase == "rest" {
			args = append(args, p.request.Profile)
		}
		if len(p.seekKeys) > 0 {
			op := ">"
			var bound any = p.seekKeys[2]
			if phase == "valued" {
				if desc {
					op = "<"
				}
				var err error
				bound, err = indexedSortValue(definition.ID, p.seekKeys[0])
				if err != nil {
					return nil, ErrCursor
				}
			}
			where += ` AND (` + key + op + `? OR ` + key + `=? AND e.entity_id` + op + `?)`
			args = append(args, bound, bound, anchor)
		}
		args = append(args, p.limit-len(out))
		rows, err := s.queryBrowseKeyRows(`SELECT `+browseRowColumns+`,`+value+` FROM `+from+` WHERE `+where+` ORDER BY `+order+` LIMIT ?`, definition.ID, args...)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			rows[i].keys["phase"] = phase
			rows[i].keys["restTitle"] = rows[i].sortKey
		}
		out = append(out, rows...)
		p.seekKeys = nil
	}
	return out, nil
}
