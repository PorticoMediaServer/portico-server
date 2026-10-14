package catalog

import "strings"

// An arbitrary filtered range has no counted library positions. Resolve its
// ordering once in SQL and hydrate just the page. Continuations keep seeking
// their signed keys, including the personal-valued and random-rotation phase.
func (s *Service) filteredRangePage(p pageShape) ([]browseRow, error) {
	primary, desc := p.primary()
	if primary.ID == "forYou" {
		return s.personalPage(p, primary, desc)
	}
	direction := "ASC"
	if desc {
		direction = "DESC"
	}
	expression := primary.Expression
	args := []any{}
	order := `order_key ` + direction + `,entity_id ` + direction
	phase, rest := "", ""
	if primary.Profile {
		column, zero, _ := personalSortColumn(primary.ID)
		expression = `COALESCE((SELECT personal.` + column + ` FROM catalog_personal_sort_rows personal WHERE personal.profile_id=? AND personal.entity_id=e.entity_id),` + zero + `)`
		args = append(args, p.request.Profile)
		phase = `CASE WHEN order_key>` + zero + ` THEN 1 ELSE 0 END`
		if desc {
			phase = `CASE WHEN order_key>` + zero + ` THEN 0 ELSE 1 END`
		}
		rest = `CASE WHEN order_key<=` + zero + ` THEN sort_key END COLLATE NOCASE`
		order = phase + ` ASC,order_key ` + direction + `,` + rest + ` ASC,CASE WHEN order_key>` + zero + ` THEN entity_id END ` + direction + `,CASE WHEN order_key<=` + zero + ` THEN entity_id END ASC`
	} else if primary.ID == "random" {
		op := ">="
		if desc {
			op = "<="
		}
		order = `CASE WHEN order_key` + op + `? THEN 0 ELSE 1 END ASC,` + order
	}
	from, where, scopeArgs := "catalog_browse_rows e", p.where, p.args
	live, liveArgs := "1", []any(nil)
	class := p.request.Query == nil && p.summary.classKey != "" && !primary.Profile
	if class {
		from = `compact_visibility_rows v CROSS JOIN catalog_browse_rows e ON e.entity_id=v.entity_id`
		if p.unrestricted != "" {
			// The published class is the restricted viewer's row set and its
			// positions; the class count is the total. The live restriction is
			// checked on the selected rows only, so a deep position does not
			// test every row before it.
			where, scopeArgs = p.unrestricted, p.unrestrictedArgs
			live, liveArgs = p.live, p.liveArgs
		}
		where = `v.class_id=? AND v.generation=? AND v.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND v.kind=? AND (` + where + `)`
		scopeArgs = append([]any{p.summary.classID, p.summary.generation, p.request.Library, p.kind}, scopeArgs...)
	}
	query := `WITH page AS MATERIALIZED (SELECT e.entity_id AS entity_id,e.sort_key AS sort_key,` + expression + ` AS order_key FROM ` + from + ` WHERE ` + where + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?)
 SELECT ` + browseRowColumns + `,page.order_key FROM page CROSS JOIN catalog_browse_rows e ON e.entity_id=page.entity_id
 CROSS JOIN catalog_entities ce ON ce.id=e.entity_id WHERE ` + live + ` ORDER BY ` + strings.NewReplacer("order_key", "page.order_key", "entity_id", "page.entity_id", "sort_key", "page.sort_key").Replace(order)
	selectRows := func(limit, offset int) ([]browseRow, error) {
		bound := append(append([]any{}, args...), scopeArgs...)
		if primary.ID == "random" {
			bound = append(bound, int64(p.sorts[0].Seed))
		}
		bound = append(bound, limit, offset)
		bound = append(bound, liveArgs...)
		if primary.ID == "random" {
			bound = append(bound, int64(p.sorts[0].Seed))
		}
		return s.queryBrowseKeyRows(query, primary.ID, bound...)
	}
	rows, err := selectRows(p.limit, p.start)
	if err != nil {
		return nil, err
	}
	// A title restricted since the class published is dropped from its page;
	// the page is refilled from the rows after it, in growing steps.
	for chunk, offset, attempt := p.limit, p.start+p.limit, 0; live != "1" && len(rows) < p.limit && offset < p.total && attempt < 8; chunk, offset, attempt = chunk*2, offset+chunk*2, attempt+1 {
		more, err := selectRows(chunk*2, offset)
		if err != nil {
			return nil, err
		}
		rows = append(rows, more...)
	}
	if len(rows) > p.limit {
		rows = rows[:p.limit]
	}
	if primary.Profile {
		_, zero, _ := personalSortColumn(primary.ID)
		zeroValue := "0"
		if zero == "''" {
			zeroValue = ""
		}
		for i := range rows {
			state := "valued"
			if rows[i].keys[primary.ID] == zeroValue {
				state = "rest"
			}
			rows[i].keys["phase"] = state
			rows[i].keys["restTitle"] = rows[i].sortKey
		}
	}
	return rows, nil
}
