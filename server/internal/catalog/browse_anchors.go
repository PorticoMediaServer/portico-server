package catalog

// Position anchors for every sort: a scrub rail labels the list by
// what it is sorted on — letters for titles, years, months for dates, whole
// points for ratings, half hours for durations — and each anchor says where
// that run begins, so any run is one range request away.
//
// The compact browse projector maintains the sort buckets used by the rail.

// maxBrowseAnchors bounds the rail. A month rail longer than this becomes a
// year rail.
const maxBrowseAnchors = 240

// anchorCacheEntries bounds the cache's memory; the key carries the revision,
// so a stale entry is never served and clearing is always safe.
const anchorCacheEntries = 512

func (s *Service) cachedAnchors(revision string) ([]BrowsePositionAnchor, bool) {
	if revision == "" {
		return nil, false
	}
	s.state.anchorMu.Lock()
	defer s.state.anchorMu.Unlock()
	anchors, ok := s.state.anchorCache[revision]
	return anchors, ok
}

func (s *Service) storeAnchors(revision string, anchors []BrowsePositionAnchor) {
	if revision == "" {
		return
	}
	s.state.anchorMu.Lock()
	defer s.state.anchorMu.Unlock()
	if s.state.anchorCache == nil || len(s.state.anchorCache) >= anchorCacheEntries {
		s.state.anchorCache = map[string][]BrowsePositionAnchor{}
	}
	s.state.anchorCache[revision] = anchors
}

// cachedBrowseTotal and storeBrowseTotal keep a posted query's exact total by browse
// revision (which names the catalogue revision, the query, the sort and the
// viewer's fence), so a grid's range requests count once, not per range.
func (s *Service) cachedBrowseTotal(revision string) (int, bool) {
	s.state.anchorMu.Lock()
	defer s.state.anchorMu.Unlock()
	total, ok := s.state.browseTotals[revision]
	return total, ok
}

func (s *Service) storeBrowseTotal(revision string, total int) {
	s.state.anchorMu.Lock()
	defer s.state.anchorMu.Unlock()
	if s.state.browseTotals == nil || len(s.state.browseTotals) >= anchorCacheEntries {
		s.state.browseTotals = map[string]int{}
	}
	s.state.browseTotals[revision] = total
}
