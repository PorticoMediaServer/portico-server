package ingestion

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/storage"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// A scan lists every directory before it analyses a file in it, and analysis
// then asks storage for each sidecar name a file might have beside it: about
// thirty names for an episode, each one a helper process, nearly all of them
// for files that are not there. What the listing already showed answers most of
// those questions for the price of one look at the directory.
//
// The memory is per scan and per directory, and it stands only while the
// directory is as it was listed: before it is used for a file, the directory's
// identity and revision are read again (once per analysed file), and a
// directory that has changed since is forgotten, so a sidecar added after the
// listing is read like any other. Only a listing this process saw from its
// first page to its last is kept at all. Anything else — a directory an
// incremental scan skipped, a listing resumed after a restart, a remote source,
// a kind of file not tracked here, more directories than the bound, a
// directory that cannot be re-read just now — is simply not known, and storage
// is asked for each name as before.
const (
	maxListedDirectories = 100000
	maxListedNames       = 400000
)

// listedDirectory is one directory as a scan saw it.
type listedDirectory struct {
	names              map[string]struct{}
	identity, revision string
}

type scanListing struct {
	open  map[string]map[string]struct{}
	done  map[string]listedDirectory
	names int
}

// sidecarLike reports whether a name has one of the extensions local metadata
// looks for beside a media file. Only those names are remembered, so only
// those can be reported absent.
func sidecarLike(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".tbn", ".webp", ".nfo", ".opf", ".json":
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
		l = &scanListing{open: map[string]map[string]struct{}{}, done: map[string]listedDirectory{}}
		s.listings[job] = l
	}
	if cursor == "" {
		l.names -= len(l.open[relative]) + len(l.done[relative].names)
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
		if page.DirectoryIdentity != "" && page.DirectoryRevision != "" {
			l.done[relative] = listedDirectory{names: names, identity: page.DirectoryIdentity, revision: page.DirectoryRevision}
		} else {
			l.names -= len(names)
		}
	}
}

// forgetListing drops what a scan remembered, when the scan has ended.
func (s *Service) forgetListing(job string) {
	s.listingMu.Lock()
	delete(s.listings, job)
	delete(s.ignoreScopes, job)
	s.listingMu.Unlock()
}

// scanListed answers, for the analysis of one file, whether this scan saw a
// sidecar at a path. known is false unless the scan listed the whole directory,
// the name is of a kind it remembers, and the directory is still exactly as it
// was listed. That last check reads the directory once per call of scanListed,
// so each analysed file looks again; a directory found changed is forgotten.
func (s *Service) scanListed(ctx context.Context, job string, source catalog.LibrarySource) func(path string) (present, known bool) {
	var mu sync.Mutex
	unchanged := map[string]bool{}
	return func(path string) (bool, bool) {
		if !sidecarLike(path) {
			return false, false
		}
		relative, err := filepath.Rel(source.ResolvedPath, filepath.Dir(path))
		if err != nil || relative != "." && !filepath.IsLocal(relative) {
			return false, false
		}
		s.listingMu.Lock()
		var listed listedDirectory
		known := false
		if l := s.listings[job]; l != nil {
			listed, known = l.done[relative]
		}
		s.listingMu.Unlock()
		if !known {
			return false, false
		}
		mu.Lock()
		defer mu.Unlock()
		same, checked := unchanged[relative]
		if !checked {
			same = s.directoryAsListed(ctx, source, relative, listed)
			unchanged[relative] = same
			if !same {
				s.listingMu.Lock()
				if l := s.listings[job]; l != nil && l.done[relative].revision == listed.revision {
					l.names -= len(l.done[relative].names)
					delete(l.done, relative)
				}
				s.listingMu.Unlock()
			}
		}
		if !same {
			return false, false
		}
		_, present := listed.names[listedName(filepath.Base(path))]
		return present, true
	}
}

// directoryAsListed reads a directory's identity and revision the way the
// scan's own verification does and compares them with the listing's. A
// directory that cannot be read just now is treated as changed.
func (s *Service) directoryAsListed(ctx context.Context, source catalog.LibrarySource, relative string, listed listedDirectory) bool {
	req := storage.InventoryRequest{Root: source.ResolvedPath, RootIdentity: source.RootIdentity, RelativePath: relative, FollowSymlinks: source.FollowSymlinks}
	var v storage.Snapshot
	var err error
	if s.storage != nil {
		v, err = s.storage.InventoryDirectory(ctx, source.ID, req)
	} else {
		v, err = storage.LocalInventoryDirectory(req)
	}
	return err == nil && v.ObjectIdentity == listed.identity && storage.InventoryRevision(v) == listed.revision
}
