package reading

// Source formats describe original physical publications, never engine formats.
// SQL fragments contain only fixed literals; no caller text participates.
func (s *Service) bookFormatsSQL() string {
	if s.Mobi != nil {
		return "'epub','pdf','mobi','azw3'"
	}
	return "'epub','pdf'"
}
func (s *Service) sourceFormatsSQL() string {
	if s.Mobi != nil {
		return "'epub','pdf','cbz','mobi','azw3'"
	}
	return "'epub','pdf','cbz'"
}

// Apply conversion readiness BEFORE choosing a primary version and LIMIT, so an
// unsupported conversion cannot conceal an otherwise eligible native version.
func (s *Service) primaryPublicationSQL() string {
	return `(SELECT candidate.asset_id FROM reading_publications candidate WHERE candidate.item_id=e.id AND candidate.format IN(` + s.sourceFormatsSQL() + `)
 ORDER BY EXISTS(SELECT 1 FROM inventory_item_availability eligible WHERE eligible.item_id=candidate.item_id AND eligible.asset_id=(SELECT token FROM catalog_assets WHERE id=candidate.asset_id) AND eligible.available=1) DESC,candidate.asset_id LIMIT 1)`
}
