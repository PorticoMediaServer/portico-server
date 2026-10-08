package catalog

// Exact counts are cached only against both durable revisions. Progress and
// source changes invalidate them; the FIFO cap bounds memory across viewers.
func (s *Service) continueCount(library, profile string) (int, error) {
	revision, e := s.ContentRevision(library, profile)
	if e != nil {
		return 0, e
	}
	key := countKey{library, profile, revision, "movie"}
	if cached, ok := s.cachedCount(key); ok {
		return cached, nil
	}
	var count int
	e = s.read().QueryRow(`SELECT count(*) FROM progress_activity a JOIN progress p ON p.profile_id=a.profile_id AND p.item_id=a.item_id JOIN catalog_entities i ON i.id=p.item_id JOIN catalog_libraries cl ON cl.id=i.library_id WHERE a.profile_id=? AND a.library_id=? AND cl.library_id=a.library_id AND i.kind=1 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=i.id) AND a.state!='ended' AND NOT EXISTS(SELECT 1 FROM continue_dismissals d WHERE d.profile_id=p.profile_id AND d.item_id=p.item_id AND d.playback_id=p.playback_id) AND p.position>0 AND p.position<(SELECT max(s.duration)*1000-3000 FROM catalog_asset_links l JOIN catalog_assets s ON s.id=l.asset_id WHERE l.entity_id=i.id AND s.available=1)`, profile, library).Scan(&count)
	if e != nil {
		return 0, e
	}
	after, e := s.ContentRevision(library, profile)
	if e != nil {
		return 0, e
	}
	if after != revision {
		return 0, ErrStaleContinuation
	}
	s.storeCount(key, count)
	return count, nil
}
