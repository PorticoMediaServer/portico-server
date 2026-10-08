package catalog

import (
	"encoding/json"
	"sort"
)

// Profile sorts (my rating, last played) order a pivot by a value only the
// viewer's history holds. Descending, the rows the profile has a value for
// come first (highest, newest), then every other row in title order;
// ascending is the reverse split: the rest in title order, then the valued
// rows lowest first.
//
// The valued rows are read from the profile's own history (personal_items by
// its rating or last-played index, then the memberships), so they cost what
// the history holds, never what the library holds. The rest is a title
// position shifted past the valued rows that sort before it: with title
// blocks that is a few block reads; under a posted query it is the query's
// bounded offset.

type personalRow struct {
	entity int64
	title  titleKey
}

func (s *Service) personalValued(p pageShape, definition browseSort, desc bool) ([]personalRow, error) {
	if definition.ID == "forYou" {
		return s.recForYouValued(p)
	}
	value, valued, index := "max(pi.rating)", "pi.rating>0", "personal_rating_range"
	if definition.ID == "lastPlayed" {
		value, valued, index = "max(pi.last_played_at)", "pi.last_played_at>''", "personal_last_played"
	}
	direction := ""
	if desc {
		direction = " DESC"
	}
	args := append([]any{p.request.Profile}, p.args...)
	rows, err := s.read().Query(`SELECT e.entity_id,lower(e.sort_key),`+value+` AS value
		FROM personal_items pi INDEXED BY `+index+`
		CROSS JOIN catalog_browse_memberships bei ON bei.item_id=pi.item_id
		CROSS JOIN catalog_browse_rows e ON e.entity_id=bei.entity_id
		WHERE pi.profile_id=? AND `+valued+` AND `+p.where+`
		GROUP BY e.entity_id ORDER BY value`+direction+`,e.entity_id`+direction, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []personalRow{}
	for rows.Next() {
		var row personalRow
		var ignored any
		if err = rows.Scan(&row.entity, &row.title.key, &ignored); err != nil {
			return nil, err
		}
		row.title.entity = row.entity
		out = append(out, row)
	}
	return out, rows.Err()
}

// personalPage reads the page of a profile sort at p.start.
func (s *Service) personalPage(p pageShape, definition browseSort, desc bool) ([]browseRow, error) {
	valued, err := s.personalValued(p, definition, desc)
	if err != nil {
		return nil, err
	}
	keys := make([]titleKey, 0, len(valued))
	ids := make(map[int64]bool, len(valued))
	for _, row := range valued {
		keys = append(keys, row.title)
		ids[row.entity] = true
	}
	sortTitleKeys(keys)
	takeValued := func(from, n int) ([]browseRow, error) {
		if from >= len(valued) || n <= 0 {
			return nil, nil
		}
		slice := valued[from:min(len(valued), from+n)]
		want := make([]int64, 0, len(slice))
		for _, row := range slice {
			want = append(want, row.entity)
		}
		raw, _ := json.Marshal(want)
		rows, err := s.queryBrowseRows(`SELECT `+browseRowColumns+` FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE e.entity_id IN (SELECT value FROM json_each(?))`, string(raw))
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]browseRow, len(rows))
		for _, row := range rows {
			byID[row.entity] = row
		}
		out := make([]browseRow, 0, len(want))
		for _, id := range want {
			if row, ok := byID[id]; ok {
				out = append(out, row)
			}
		}
		return out, nil
	}
	if desc {
		out, err := takeValued(p.start, p.limit)
		if err != nil || len(out) == p.limit {
			return out, err
		}
		rest, err := s.personalRest(p, keys, ids, max(0, p.start-len(valued)), p.limit-len(out))
		return append(out, rest...), err
	}
	out, err := s.personalRest(p, keys, ids, p.start, p.limit)
	if err != nil || len(out) == p.limit {
		return out, err
	}
	// The rest ran out inside this page (the valued rows start at its end) or
	// before it (count where it ended, bounded by the page's own start).
	from := 0
	if len(out) == 0 {
		size, err := s.personalRestSize(p, ids, p.start)
		if err != nil {
			return nil, err
		}
		from = p.start - size
	}
	more, err := takeValued(from, p.limit-len(out))
	return append(out, more...), err
}

// personalRest reads n of the rows without a value, in title order, from the
// rest's own index r.
func (s *Service) personalRest(p pageShape, keys []titleKey, valued map[int64]bool, r, n int) ([]browseRow, error) {
	if !p.positional() {
		raw, _ := json.Marshal(mapKeys(valued))
		args := append(append([]any{}, p.args...), string(raw), n, r)
		return s.queryBrowseRows(`SELECT `+browseRowColumns+` FROM catalog_browse_rows e JOIN catalog_entities ce ON ce.id=e.entity_id WHERE `+p.where+
			` AND e.entity_id NOT IN (SELECT value FROM json_each(?)) ORDER BY e.sort_key COLLATE NOCASE,e.entity_id LIMIT ? OFFSET ?`, args...)
	}
	// The rest's index r is the title position t with r rest rows before it:
	// t = r + (valued rows before t). Each step can only move t forward, and it
	// settles as soon as no valued row lies in the newly covered span.
	t := r
	for {
		rows, err := s.ascendingFrom(p, 0, t, 1)
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		at := titleKey{asciiLower(rows[0].sortKey), rows[0].entity}
		before := sort.Search(len(keys), func(i int) bool { return !keys[i].less(at) })
		if r+before == t {
			break
		}
		t = r + before
	}
	out := make([]browseRow, 0, n)
	for len(out) < n {
		want := n - len(out) + min(len(keys), 256)
		rows, err := s.ascendingFrom(p, 0, t, want)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if !valued[row.entity] && len(out) < n {
				out = append(out, row)
			}
		}
		if len(rows) < want {
			break
		}
		t += len(rows)
	}
	return out, nil
}

// personalRestSize counts the rows without a value, up to limit.
func (s *Service) personalRestSize(p pageShape, valued map[int64]bool, limit int) (int, error) {
	if p.positional() {
		return max(0, p.total-len(valued)), nil
	}
	raw, _ := json.Marshal(mapKeys(valued))
	args := append(append([]any{}, p.args...), string(raw), limit)
	var size int
	err := s.read().QueryRow(`SELECT count(*) FROM (SELECT 1 FROM catalog_browse_rows e WHERE `+p.where+` AND e.entity_id NOT IN (SELECT value FROM json_each(?)) LIMIT ?)`, args...).Scan(&size)
	return size, err
}

func mapKeys(set map[int64]bool) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}
