package app

import (
	"context"
	"strings"
)

func catalogueAuthorizationScope(user User) string {
	// The effective authorization fingerprint intentionally excludes profile ID
	// while including every content-visibility policy. This lets empty-state Home
	// and search responses share work only across equivalent catalogue access.
	return strings.TrimSpace(effectiveAuthorizationCacheFingerprint(user))
}

func emptyMediaStateCacheScope(user User, hasState bool) string {
	if hasState {
		return homeCacheKey(user)
	}
	return "empty-state\x00" + catalogueAuthorizationScope(user)
}

func (s *Server) buildHomeCatalogueProjection(ctx context.Context, user User) ([]HomeRow, error) {
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
	return rows, nil
}
