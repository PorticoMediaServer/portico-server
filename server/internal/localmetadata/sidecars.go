package localmetadata

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Sidecar image discovery for every agent (Spec — Page Content §0.5): the
// local files each library kind honours live next to the media, so they are
// read here, at analysis, never inside a catalog write transaction. The keys
// travel in Facts and the catalog commit writes them as provider='local'
// artwork candidates (migration 0263); local art wins over online art unless
// the owner picks otherwise in the editor.
//
// Layout rules, per kind:
//   - movie: logo.png beside the file.
//   - tv/anime: poster.jpg/folder.jpg (.png too), fanart.jpg/backdrop.jpg and
//     logo.png in the file's directory, then its parent (never above the
//     library root); season posters seasonNN.jpg and season-specials-poster.jpg
//     for the episode's own season number.
//   - music: artist.jpg and fanart.jpg/backdrop.jpg in the file's directory,
//     then its parent (never above the root); the album's parent-folder cover
//     is reported as album/cover for CD1/-style layouts.
//
// Only positive hits are cached by path (a show's hundred episodes share one
// read); misses are re-read, so art added later is picked up on rescan.
func (s *Service) ReadSidecars(ctx context.Context, library, kind, root, path string, season int) map[string]string {
	out := map[string]string{}
	if s.readSmall == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	dir := filepath.Dir(path)
	dirs := []string{dir}
	if parent := filepath.Dir(dir); parent != dir && underRoot(root, parent) && (kind == "tv" || kind == "anime" || kind == "music" || kind == "audiobook") {
		dirs = append(dirs, parent)
	}
	first := func(dirs []string, names ...string) string {
		for _, dir := range dirs {
			for _, name := range names {
				if key, ok := s.sidecarKey(ctx, library, filepath.Join(dir, name)); ok {
					return key
				}
			}
		}
		return ""
	}
	switch kind {
	case "movie":
		if key := first(dirs, "logo.png"); key != "" {
			out["item/logo"] = key
		}
	case "tv", "anime":
		if key := first(dirs, "poster.jpg", "poster.png", "folder.jpg", "folder.png"); key != "" {
			out["show/poster"] = key
		}
		if key := first(dirs, "fanart.jpg", "fanart.png", "backdrop.jpg", "backdrop.png"); key != "" {
			out["show/backdrop"] = key
		}
		if key := first(dirs, "logo.png"); key != "" {
			out["show/logo"] = key
		}
		if season >= 0 && season <= 9999 {
			names := []string{"season" + strconv.Itoa(season) + ".jpg", "season" + strconv.Itoa(season) + ".png", "season" + twoDigits(season) + ".jpg", "season" + twoDigits(season) + ".png"}
			if season == 0 {
				names = append(names, "season-specials-poster.jpg")
			}
			if key := first(dirs, names...); key != "" {
				out["season/"+strconv.Itoa(season)+"/poster"] = key
			}
		}
	case "music", "audiobook":
		if key := first(dirs, "artist.jpg", "artist.png"); key != "" {
			out["artist/portrait"] = key
		}
		if key := first(dirs, "fanart.jpg", "fanart.png", "backdrop.jpg", "backdrop.png"); key != "" {
			out["artist/backdrop"] = key
		}
		// The album's own parent-folder cover (CD1/-style layouts); the
		// file's own folder is already covered by PrepareKind.
		if len(dirs) > 1 {
			if key := first(dirs[1:], "cover.jpg", "cover.png", "folder.jpg", "folder.png"); key != "" {
				out["album/cover"] = key
			}
		}
	}
	return out
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// underRoot reports whether dir is strictly under the library root (both
// absolute and cleaned): sidecar discovery never reads at or above the root,
// so a stray file at the root can never tattoo every entity beneath it.
func underRoot(root, dir string) bool {
	root, dir = filepath.Clean(root), filepath.Clean(dir)
	return dir != root && strings.HasPrefix(dir, root+string(filepath.Separator))
}

// sidecarKey reads one sidecar file into the local artwork cache, returning
// its key. Positive hits are cached by path within a bounded map; misses are
// re-read so later-added art is picked up on rescan.
func (s *Service) sidecarKey(ctx context.Context, library, full string) (string, bool) {
	s.mu.Lock()
	if key, ok := s.sidecars[full]; ok {
		s.mu.Unlock()
		return key, true
	}
	s.mu.Unlock()
	raw, err := s.readSmall(ctx, library, full, 4<<20)
	if err != nil {
		return "", false
	}
	key, err := s.store(raw)
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	if s.sidecars == nil {
		s.sidecars = map[string]string{}
	}
	if len(s.sidecars) > 1024 {
		s.sidecars = map[string]string{}
	}
	s.sidecars[full] = key
	s.mu.Unlock()
	return key, true
}
