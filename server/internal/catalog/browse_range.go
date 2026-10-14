package catalog

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
	query := `WITH candidates AS MATERIALIZED (SELECT e.entity_id,e.sort_key,` + expression + ` AS order_key FROM catalog_browse_rows e WHERE ` + p.where + `),
 ranked AS MATERIALIZED (SELECT entity_id,order_key,row_number() OVER(ORDER BY ` + order + `)-1 AS position FROM candidates)
 SELECT ` + browseRowColumns + `,page.order_key FROM ranked page CROSS JOIN catalog_browse_rows e ON e.entity_id=page.entity_id
 CROSS JOIN catalog_entities ce ON ce.id=e.entity_id WHERE page.position>=? AND page.position<? ORDER BY page.position`
	args = append(args, p.args...)
	if primary.ID == "random" {
		args = append(args, int64(p.sorts[0].Seed))
	}
	args = append(args, p.start, p.start+p.limit)
	rows, err := s.queryBrowseKeyRows(query, primary.ID, args...)
	if err != nil {
		return nil, err
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
