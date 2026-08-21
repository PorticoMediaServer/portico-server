package app

import (
	"context"
	"strings"
	"time"
)

const (
	homeProjectionCacheTTL = 45 * time.Second
	homeProjectionMaximum  = 128
)

// homeProjectionCacheEntry contains catalogue-only row descriptors. It never
// contains viewer state, progress, watch history, recommendations, or items.
// Those overlays continue to be assembled and cached per profile.
type homeProjectionCacheEntry struct {
	rows      []HomeRow
	expiresAt time.Time
}

func homeProjectionCacheKey(user User) string {
	// The effective authorization fingerprint intentionally excludes profile ID
	// while including every content-visibility policy. Equivalent profiles may
	// share catalogue structure without sharing personal state.
	return strings.TrimSpace(effectiveAuthorizationCacheFingerprint(user))
}

func emptyMediaStateCacheScope(user User, hasState bool) string {
	if hasState {
		return homeCacheKey(user)
	}
	return "empty-state\x00" + homeProjectionCacheKey(user)
}

func cloneHomeProjectionRows(rows []HomeRow) []HomeRow {
	result := make([]HomeRow, len(rows))
	copy(result, rows)
	for index := range result {
		result[index].Items = nil
		result[index].NextCursor = ""
	}
	return result
}

func (s *Server) cachedHomeProjection(user User) ([]HomeRow, bool) {
	key := homeProjectionCacheKey(user)
	if key == "" {
		return nil, false
	}
	now := time.Now()
	s.homeCacheMu.Lock()
	defer s.homeCacheMu.Unlock()
	entry, ok := s.homeProjectionCache[key]
	if !ok || !now.Before(entry.expiresAt) {
		delete(s.homeProjectionCache, key)
		return nil, false
	}
	return cloneHomeProjectionRows(entry.rows), true
}

func (s *Server) beginHomeProjectionBuild(user User) (chan struct{}, bool) {
	key := homeProjectionCacheKey(user)
	if key == "" {
		return nil, true
	}
	s.homeCacheMu.Lock()
	defer s.homeCacheMu.Unlock()
	if s.homeProjectionInFlight == nil {
		s.homeProjectionInFlight = map[string]chan struct{}{}
	}
	if wait, ok := s.homeProjectionInFlight[key]; ok {
		return wait, false
	}
	wait := make(chan struct{})
	s.homeProjectionInFlight[key] = wait
	return wait, true
}

func (s *Server) finishHomeProjectionBuild(user User) {
	key := homeProjectionCacheKey(user)
	if key == "" {
		return
	}
	s.homeCacheMu.Lock()
	defer s.homeCacheMu.Unlock()
	if wait, ok := s.homeProjectionInFlight[key]; ok {
		delete(s.homeProjectionInFlight, key)
		close(wait)
	}
}

func (s *Server) storeHomeProjection(user User, rows []HomeRow) {
	key := homeProjectionCacheKey(user)
	if key == "" {
		return
	}
	now := time.Now()
	s.homeCacheMu.Lock()
	defer s.homeCacheMu.Unlock()
	if s.homeProjectionCache == nil {
		s.homeProjectionCache = map[string]homeProjectionCacheEntry{}
	}
	for candidate, entry := range s.homeProjectionCache {
		if !now.Before(entry.expiresAt) {
			delete(s.homeProjectionCache, candidate)
		}
	}
	for len(s.homeProjectionCache) >= homeProjectionMaximum {
		oldestKey := ""
		var oldest time.Time
		for candidate, entry := range s.homeProjectionCache {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey, oldest = candidate, entry.expiresAt
			}
		}
		delete(s.homeProjectionCache, oldestKey)
	}
	s.homeProjectionCache[key] = homeProjectionCacheEntry{
		rows: cloneHomeProjectionRows(rows), expiresAt: now.Add(homeProjectionCacheTTL),
	}
}

func (s *Server) sharedHomeCatalogueProjection(ctx context.Context, user User) ([]HomeRow, error) {
	if rows, ok := s.cachedHomeProjection(user); ok {
		return rows, nil
	}
	wait, owner := s.beginHomeProjectionBuild(user)
	if !owner {
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if rows, ok := s.cachedHomeProjection(user); ok {
			return rows, nil
		}
		return s.sharedHomeCatalogueProjection(ctx, user)
	}
	defer s.finishHomeProjectionBuild(user)

	libraries, err := s.listLibrariesForUserContext(ctx, user)
	if err != nil {
		return nil, err
	}
	rows := make([]HomeRow, 0, len(libraries))
	priority := 40
	for _, library := range libraries {
		if libraryHiddenFrom(library, "home") {
			continue
		}
		row := homeRowDescriptor("recent_"+library.ID, "Recently Added in "+library.Name, "recently-added", "poster", "New top-level titles from "+library.Name+".", priority, priority == 40)
		row.LibraryID = library.ID
		rows = append(rows, row)
		priority += 10
	}
	s.storeHomeProjection(user, rows)
	return cloneHomeProjectionRows(rows), nil
}
