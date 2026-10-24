package ingestion

import (
	"context"
	"path/filepath"
	"sort"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/localmetadata"
	"portico.local/server/internal/naming"
)

// Naming evidence only, never an I/O grant. An incomplete, changed, remote or
// oversized directory cannot establish an album-wide packed-number pattern.
func (s *Service) scanMusicTracksListed(ctx context.Context, job string, source catalog.LibrarySource) localmetadata.MusicTrackListing {
	current := s.scanDirectoryListed(ctx, job, source)
	return func(directory string) ([]string, bool) {
		relative, err := filepath.Rel(source.ResolvedPath, directory)
		if err != nil || relative != "." && !filepath.IsLocal(relative) {
			return nil, false
		}
		dir, known := current(relative)
		if !known || len(dir.names) > naming.MaxPackedTrackCohort {
			return nil, false
		}
		names := []string{}
		for name := range dir.names {
			if assets.SupportedForKind(name, "music") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		return names, true
	}
}
