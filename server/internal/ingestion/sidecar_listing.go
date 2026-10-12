package ingestion

import (
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/storage"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// A scan lists every directory before it analyses a file in it, and analysis
// then asks storage for each sidecar name a file might have beside it: about
// thirty names for an episode, each one a helper process, nearly all of them
// for files that are not there. What the listing already showed answers most of
// those questions for nothing.
//
// The memory is per scan and per directory, and only a listing this process saw
// from its first page to its last is trusted. Anything else — a directory an
// incremental scan skipped, a listing resumed after a restart, a remote source,
// a kind of file not tracked here, more directories than the bound — is simply
// not known, and storage is asked as before. A sidecar added after its
// directory was listed is picked up by a later scan, not this one.
const (
	maxListedDirectories = 100000
	maxListedNames       = 400000
)

type scanListing struct {
	open  map[string]map[string]struct{}
	done  map[string]map[string]struct{}
	names int
}

// sidecarLike reports whether a name has one of the extensions local metadata
// looks for beside a media file. Only those names are remembered, so only
// those can be reported absent.
func sidecarLike(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".png", ".nfo", ".opf", ".json":
		return true
	}
	return false
}

// listedName is how names are compared: a file system that ignores letter case
// or Unicode form would open the file either way, so neither may make a
// present file look absent.
func listedName(name string) string { return strings.ToLower(norm.NFC.String(name)) }

func (s *Service) noteDirectoryPage(job, relative, cursor string, page storage.InventoryPage) {
	s.listingMu.Lock()
	defer s.listingMu.Unlock()
	l := s.listings[job]
	if l == nil {
		// A new scan is a good moment to forget the ones that have ended.
		for other := range s.listings {
			var status string
			if s.db == nil || s.db.QueryRow(`SELECT status FROM jobs WHERE id=?`, other).Scan(&status) != nil || status != "queued" && status != "running" && status != "paused" {
				delete(s.listings, other)
			}
		}
		if s.listings == nil {
			s.listings = map[string]*scanListing{}
		}
		l = &scanListing{open: map[string]map[string]struct{}{}, done: map[string]map[string]struct{}{}}
		s.listings[job] = l
	}
	if cursor == "" {
		l.names -= len(l.open[relative]) + len(l.done[relative])
		delete(l.open, relative)
		delete(l.done, relative)
		if len(l.open)+len(l.done) >= maxListedDirectories {
			return
		}
		l.open[relative] = map[string]struct{}{}
	}
	names, started := l.open[relative]
	if !started {
		return
	}
	for _, v := range page.Entries {
		if v.Directory || !sidecarLike(v.Name) {
			continue
		}
		if l.names >= maxListedNames {
			l.names -= len(names)
			delete(l.open, relative)
			return
		}
		if _, seen := names[listedName(v.Name)]; !seen {
			names[listedName(v.Name)] = struct{}{}
			l.names++
		}
	}
	if page.Complete {
		delete(l.open, relative)
		l.done[relative] = names
	}
}

// sidecarListed reports whether this scan saw a file at path, and whether it
// knows: known is false unless the scan listed the whole directory and the
// name is of a kind it remembers.
func (s *Service) sidecarListed(job string, source catalog.LibrarySource, path string) (present, known bool) {
	if !sidecarLike(path) {
		return false, false
	}
	relative, err := filepath.Rel(source.ResolvedPath, filepath.Dir(path))
	if err != nil || relative != "." && !filepath.IsLocal(relative) {
		return false, false
	}
	s.listingMu.Lock()
	defer s.listingMu.Unlock()
	l := s.listings[job]
	if l == nil {
		return false, false
	}
	names, known := l.done[relative]
	if !known {
		return false, false
	}
	_, present = names[listedName(filepath.Base(path))]
	return present, true
}
