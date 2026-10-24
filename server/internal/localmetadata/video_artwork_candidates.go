package localmetadata

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/storage"
)

// ReadArtworkCandidates reads only recognized image members of actual complete
// listings. An incomplete/changed/unreadable set never replaces a prior set.
func (s *Service) ReadArtworkCandidates(ctx context.Context, library, kind, root, media string, season int) ([]assets.LocalArtworkCandidate, bool) {
	listed, ok := ctx.Value(artworkListingKey{}).(func(string) (ArtworkDirectoryListing, bool))
	if !ok || listed == nil || s.storage == nil || s.readSmall == nil || (kind != "movie" && kind != "tv" && kind != "anime") {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dirs := []string{filepath.Dir(media)}
	if (kind == "tv" || kind == "anime") && seasonFolderName.MatchString(filepath.Base(dirs[0])) && underRoot(root, filepath.Dir(dirs[0])) {
		dirs = append(dirs, filepath.Dir(dirs[0]))
	}
	result := []assets.LocalArtworkCandidate{}
	for _, dir := range dirs {
		listing, known := listed(dir)
		if !known || listing.SourceID == "" || listing.RootIdentity == "" {
			return nil, false
		}
		entries := append([]ArtworkListingEntry(nil), listing.Entries...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		for _, entry := range entries {
			if filepath.Base(entry.Name) != entry.Name || entry.Name == "." || entry.Name == ".." || strings.ContainsAny(entry.Name, "/\\\x00") || entry.Revision == "" {
				return nil, false
			}
			path := filepath.Join(dir, entry.Name)
			slot := videoArtworkSlot(kind, root, media, path, season)
			if slot == "" {
				continue
			}
			relative, err := filepath.Rel(root, path)
			if err != nil || !filepath.IsLocal(relative) {
				return nil, false
			}
			request := storage.InventoryRequest{Root: root, RootIdentity: listing.RootIdentity, RelativePath: relative, FollowSymlinks: listing.FollowSymlinks}
			check := func() bool {
				if storage.CheckScanRead(ctx, path) != nil {
					return false
				}
				current, err := s.storage.InventoryStat(ctx, listing.SourceID, request)
				return err == nil && !current.Directory && current.Size >= 0 && current.Size <= 4<<20 && storage.InventoryRevision(current) == entry.Revision
			}
			if !check() {
				artworkReadFailed(ctx)
				return nil, false
			}
			stamp := listing.SourceID + "\x00" + listing.RootIdentity + "\x00" + entry.Revision
			s.mu.Lock()
			if s.artworkCandidateObservations == nil || len(s.artworkCandidateObservations) > 1024 {
				s.artworkCandidateObservations = map[string]string{}
			}
			if s.artworkCandidateObservations[path] != stamp {
				delete(s.sidecars, path)
				s.artworkCandidateObservations[path] = stamp
			}
			s.mu.Unlock()
			key, valid := s.sidecarKey(ctx, library, path)
			if !valid || !check() {
				artworkReadFailed(ctx)
				return nil, false
			}
			result = append(result, assets.LocalArtworkCandidate{Slot: slot, Key: key})
		}
	}
	return result, true
}

// videoArtworkSlot classifies only known image conventions in the file/show's
// own allowed directories. A match does not authorize reading that path.
func videoArtworkSlot(kind, root, media, candidate string, season int) string {
	ext := strings.ToLower(filepath.Ext(candidate))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".tbn", ".webp":
	default:
		return ""
	}
	dir := filepath.Dir(media)
	candidateDir := filepath.Dir(candidate)
	stem := strings.TrimSuffix(filepath.Base(media), filepath.Ext(media))
	name := strings.TrimSuffix(filepath.Base(candidate), filepath.Ext(candidate))
	matches := func(bases ...string) bool {
		for _, base := range bases {
			if numberedArtName(name, base, false) {
				return true
			}
		}
		return false
	}
	if kind == "movie" {
		if candidateDir != dir {
			return ""
		}
		if matches(stem+"-poster", stem, "poster", "folder", "cover", "default", "movie") {
			return "item/poster"
		}
		if matches(stem+"-fanart", "fanart", "backdrop", "art", "background") {
			return "item/backdrop"
		}
		if matches(stem+"-clearlogo", "logo", "clearlogo") {
			return "item/logo"
		}
		return ""
	}
	if kind != "tv" && kind != "anime" {
		return ""
	}
	showDir := dir
	inSeason := seasonFolderName.MatchString(filepath.Base(dir))
	if inSeason {
		showDir = filepath.Dir(dir)
		if !underRoot(root, showDir) {
			showDir = ""
		}
	}
	if candidateDir == showDir {
		if matches("poster", "folder", "show") {
			return "show/poster"
		}
		if matches("fanart", "backdrop", "art", "background") {
			return "show/backdrop"
		}
		if matches("logo", "clearlogo") {
			return "show/logo"
		}
	}
	if season >= 0 && season <= 9999 && (candidateDir == showDir || inSeason && candidateDir == dir) {
		for _, base := range []string{"season" + strconv.Itoa(season), "season" + twoDigits(season), "season" + twoDigits(season) + "-poster"} {
			if numberedArtName(name, base, true) {
				return "season/" + strconv.Itoa(season) + "/poster"
			}
		}
		if season == 0 && numberedArtName(name, "season-specials-poster", false) {
			return "season/0/poster"
		}
		if inSeason && candidateDir == dir && matches("poster", "folder") {
			return "season/" + strconv.Itoa(season) + "/poster"
		}
	}
	return ""
}

func numberedArtName(name, base string, letter bool) bool {
	name, base = strings.ToLower(name), strings.ToLower(base)
	if name == base {
		return true
	}
	if !strings.HasPrefix(name, base) {
		return false
	}
	tail := strings.TrimPrefix(name, base)
	if letter && len(tail) == 1 && tail[0] >= 'a' && tail[0] <= 'z' {
		return true
	}
	if !strings.HasPrefix(tail, "-") || len(tail) == 1 {
		return false
	}
	for _, r := range tail[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
