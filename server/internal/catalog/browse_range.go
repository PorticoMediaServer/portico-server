package catalog

// A filtered range has no library position blocks. Rank its SQL result once
// and hydrate only the requested page; repeatedly walking keyset pages before
// the range would evaluate the same membership predicate hundreds of times.
// Ordinary continuations still seek their signed keys.
func (s *Service) filteredRangePage(p pageShape) ([]browseRow, error) {
	primary, _ := p.primary()
	if primary.Profile || primary.ID == "random" || len(p.sorts) != 1 {
		return s.walkIndexedRange(p, func(next pageShape) ([]browseRow, error) { return s.browsePage(next) })
	}
	query := `WITH ranked AS MATERIALIZED (
 SELECT e.entity_id,` + primary.Expression + ` AS order_key,row_number() OVER(ORDER BY ` + p.order + `)-1 AS position
 FROM catalog_browse_rows e WHERE ` + p.where + `)
 SELECT ` + browseRowColumns + `,page.order_key FROM ranked page CROSS JOIN catalog_browse_rows e ON e.entity_id=page.entity_id
 CROSS JOIN catalog_entities ce ON ce.id=e.entity_id WHERE page.position>=? AND page.position<? ORDER BY page.position`
	args := append(append(append([]any{}, p.orderArgs...), p.args...), p.start, p.start+p.limit)
	return s.queryBrowseKeyRows(query, primary.ID, args...)
}
