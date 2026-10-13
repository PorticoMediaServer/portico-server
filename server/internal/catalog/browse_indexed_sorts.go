package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
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
		} else if n, ok := key.(float64); ok {
			text = strconv.FormatFloat(n, 'g', -1, 64)
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
	direction := "ASC"
	if desc {
		direction = "DESC"
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
		query, bound := browseBoundedSQL(browseRowColumns+",page.order_key", "catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id", p.where, p.args, definition.Expression, "e.entity_id", desc, false, key, id, p.limit, 0)
		return s.queryBrowseKeyRows(query, definition.ID, bound...)
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
func personalSortColumn(field string) (column, zero, index string) {
	switch field {
	case "lastPlayed":
		return "last_played_at", "''", "catalog_personal_order_last"
	case "playCount":
		return "play_count", "0", "catalog_personal_order_plays"
	}
	return "rating", "0", "catalog_personal_order_rating"
}

func personalKeyPageSQL(p pageShape, definition browseSort, desc bool) (string, []any, error) {
	column, zero, index := personalSortColumn(definition.ID)
	where := `personal.profile_id=? AND personal.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND personal.kind=? AND personal.` + column + `>` + zero + ` AND EXISTS(SELECT 1 FROM catalog_browse_rows e WHERE e.entity_id=personal.entity_id AND ` + p.where + `)`
	args := append([]any{p.request.Profile, p.request.Library, p.kind}, p.args...)
	direction, op := "ASC", ">"
	if desc {
		direction, op = "DESC", "<"
	}
	if len(p.seekKeys) > 0 {
		key, err := indexedSortValue(definition.ID, p.seekKeys[0])
		if err != nil {
			return "", nil, ErrCursor
		}
		id, err := strconv.ParseInt(p.seekAnchor, 10, 64)
		if err != nil {
			return "", nil, ErrCursor
		}
		where += ` AND (personal.` + column + op + `? OR personal.` + column + `=? AND personal.entity_id` + op + `?)`
		args = append(args, key, key, id)
	}
	args = append(args, p.limit)
	return `SELECT personal.entity_id,personal.` + column + ` FROM catalog_personal_sort_rows personal INDEXED BY ` + index + ` WHERE ` + where + ` ORDER BY personal.` + column + ` ` + direction + `,personal.entity_id ` + direction + ` LIMIT ?`, args, nil
}

func (s *Service) indexedPersonalValuedPage(p pageShape, definition browseSort, desc bool) ([]browseRow, error) {
	query, args, err := personalKeyPageSQL(p, definition, desc)
	if err != nil {
		return nil, err
	}
	rows, err := s.read().Query(query, args...)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	keys := map[int64]string{}
	for rows.Next() {
		var id int64
		var value any
		if err = rows.Scan(&id, &value); err != nil {
			break
		}
		ids = append(ids, id)
		keys[id] = formatOrderValue(value)
		if n, ok := value.(int64); ok {
			keys[id] = strconv.FormatInt(n, 10)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []browseRow{}, nil
	}
	raw, _ := json.Marshal(ids)
	projected, err := s.queryBrowseRows(`SELECT `+browseRowColumns+` FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE e.entity_id IN(SELECT value FROM json_each(?))`, string(raw))
	if err != nil {
		return nil, err
	}
	byID := map[int64]browseRow{}
	for _, row := range projected {
		byID[row.entity] = row
	}
	out := make([]browseRow, 0, len(ids))
	for _, id := range ids {
		row, ok := byID[id]
		if !ok {
			continue
		}
		row.keys = map[string]string{definition.ID: keys[id], "phase": "valued", "restTitle": row.sortKey}
		out = append(out, row)
	}
	return out, nil
}

func (s *Service) indexedPersonalPage(p pageShape, definition browseSort, desc bool) ([]browseRow, error) {
	// An untouched sort is the title stream alone. Use its maintained blocks
	// for a direct range instead of walking every earlier title.
	if p.start > 0 && len(p.seekKeys) == 0 && p.positional() {
		column, zero, _ := personalSortColumn(definition.ID)
		var valued bool
		err := s.read().QueryRow(`SELECT EXISTS(SELECT 1 FROM catalog_personal_sort_rows personal WHERE personal.profile_id=? AND personal.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND personal.kind=? AND personal.`+column+`>`+zero+`)`, p.request.Profile, p.request.Library, p.kind).Scan(&valued)
		if err != nil {
			return nil, err
		}
		if !valued {
			rows, err := s.positionPage(p, 0, false, p.start, p.limit)
			for i := range rows {
				rows[i].keys = map[string]string{definition.ID: strings.Trim(zero, "'"), "phase": "rest", "restTitle": rows[i].sortKey}
			}
			return rows, err
		}
	}
	if p.start > 0 && len(p.seekKeys) == 0 {
		return s.walkIndexedRange(p, func(p pageShape) ([]browseRow, error) { return s.indexedPersonalPage(p, definition, desc) })
	}
	column, zero, _ := personalSortColumn(definition.ID)
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
		if phase == "valued" {
			shape := p
			shape.limit = p.limit - len(out)
			rows, err := s.indexedPersonalValuedPage(shape, definition, desc)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
			p.seekKeys = nil
			continue
		}
		where := p.where + ` AND NOT EXISTS(SELECT 1 FROM catalog_personal_sort_rows personal WHERE personal.profile_id=? AND personal.entity_id=e.entity_id AND personal.` + column + `>` + zero + `)`
		args := append(append([]any{}, p.args...), p.request.Profile)
		if len(p.seekKeys) > 0 {
			query, bound := browseBoundedSQL(browseRowColumns+","+zero, "catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id", where, args, "e.sort_key COLLATE NOCASE", "e.entity_id", false, false, p.seekKeys[2], anchor, p.limit-len(out), 0)
			rows, err := s.queryBrowseKeyRows(query, definition.ID, bound...)
			if err != nil {
				return nil, err
			}
			for i := range rows {
				rows[i].keys["phase"] = "rest"
				rows[i].keys["restTitle"] = rows[i].sortKey
			}
			out = append(out, rows...)
			p.seekKeys = nil
			continue
		}
		args = append(args, p.limit-len(out))
		rows, err := s.queryBrowseKeyRows(`SELECT `+browseRowColumns+`,`+zero+` FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE `+where+` ORDER BY e.sort_key COLLATE NOCASE,e.entity_id LIMIT ?`, definition.ID, args...)
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

// indexedFactAnchorRank counts the indexed prefix before a surviving anchor,
// using the same scope, tie break and seeded rotation as indexedFactPage.
func (s *Service) indexedFactAnchorRank(p pageShape, anchor string) (int, bool, error) {
	definition, desc := p.primary()
	var value any
	var entity int64
	err := s.read().QueryRow(`SELECT `+definition.Expression+`,e.entity_id FROM catalog_browse_rows e WHERE `+p.where+
		` AND e.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, append(append([]any{}, p.args...), anchor)...).Scan(&value, &entity)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	op := "<"
	if desc {
		op = ">"
	}
	key := definition.Expression
	bound := `(` + key + op + `? OR ` + key + `=? AND e.entity_id` + op + `?)`
	args := append(append([]any{}, p.args...), value, value, entity)
	if definition.ID == "random" {
		randomKey, ok := value.(int64)
		if !ok {
			return 0, false, ErrCursor
		}
		seed := int64(p.sorts[0].Seed)
		first := key + `>=?`
		inFirst := randomKey >= seed
		if desc {
			first = key + `<=?`
			inFirst = randomKey <= seed
		}
		join := " OR "
		if inFirst {
			join = " AND "
		}
		bound = `(` + first + join + bound + `)`
		args = append(append([]any{}, p.args...), seed, value, value, entity)
	}
	var rank int
	err = s.read().QueryRow(`SELECT count(*) FROM catalog_browse_rows e WHERE `+p.where+` AND `+bound, args...).Scan(&rank)
	return rank, err == nil, err
}
