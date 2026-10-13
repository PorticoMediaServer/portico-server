package catalog

// A Continue count depends on progress as well as semantic personal state.
// Combining these monotonic counters is local to this cache; it never changes
// the Home or command revision. The FIFO cap bounds memory across viewers.
func (s *Service) continueCountRevision(library, profile string) (ContentRevision, error) {
	var revision ContentRevision
	err := s.read().QueryRow(`SELECT r.revision,COALESCE(v.revision,0)+COALESCE(v.observation_revision,0)
 FROM library_revisions r LEFT JOIN viewer_revisions v ON v.library_id=r.library_id AND v.profile_id=?
 WHERE r.library_id=?`, profile, library).Scan(&revision.Catalog, &revision.Viewer)
	return revision, err
}

func (s *Service) continueCount(library, profile string) (int, error) {
	revision, e := s.continueCountRevision(library, profile)
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
	after, e := s.continueCountRevision(library, profile)
	if e != nil {
		return 0, e
	}
	if after != revision {
		return 0, ErrStaleContinuation
	}
	s.storeCount(key, count)
	return count, nil
}
