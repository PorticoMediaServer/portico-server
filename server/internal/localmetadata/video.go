package localmetadata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"portico.local/server/internal/assets"
	"strings"
	"time"
)

// VideoNFO uses the scanner's admitted root/read guard and never resolves URLs
// inside XML. A failed/unavailable sidecar is not evidence to erase old metadata.
func (s *Service) VideoNFO(ctx context.Context, library, root, path, kind string, f assets.Facts) assets.Facts {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	f.VideoMetadataStatus = "not_observed"
	if s.storage == nil {
		return f
	}
	candidates := []string{strings.TrimSuffix(path, filepath.Ext(path)) + ".nfo"}
	if kind == "movie" {
		candidates = append(candidates, filepath.Join(filepath.Dir(path), "movie.nfo"))
	} else {
		dir := filepath.Dir(path)
		candidates = append(candidates, filepath.Join(dir, "tvshow.nfo"))
		parent := filepath.Dir(dir)
		if rel, e := filepath.Rel(root, parent); e == nil && (rel == "." || filepath.IsLocal(rel)) {
			candidates = append(candidates, filepath.Join(parent, "tvshow.nfo"))
		}
	}
	seen := map[string]bool{}
	for _, side := range candidates {
		if seen[side] {
			continue
		}
		seen[side] = true
		rel, e := filepath.Rel(root, side)
		if e != nil || !filepath.IsLocal(rel) {
			continue
		}
		before, e := s.storage.Stat(ctx, library, side)
		if e != nil {
			continue
		}
		if before.Directory || before.Size > 256<<10 {
			f.LocalMetadataIssue = "invalid_video_nfo"
			continue
		}
		raw, e := s.storage.ReadSmall(ctx, library, side, 256<<10)
		if e != nil {
			f.LocalMetadataIssue = "video_nfo_unavailable"
			continue
		}
		after, e := s.storage.Stat(ctx, library, side)
		if e != nil || before.Size != after.Size || before.ModifiedNS != after.ModifiedNS || before.Revision != after.Revision || before.ObjectIdentity != after.ObjectIdentity {
			f.LocalMetadataIssue = "video_nfo_changed"
			continue
		}
		records, e := ParseVideoNFO(raw, kind == "anime")
		if e != nil {
			f.LocalMetadataIssue = "invalid_video_nfo"
			continue
		}
		locator := sha256.Sum256([]byte(rel))
		for i := range records {
			v := &records[i]
			v.Locator = hex.EncodeToString(locator[:])
			v.Size = after.Size
			v.ModifiedNS = after.ModifiedNS
			v.Revision = after.Revision
			if kind == "movie" && v.Kind != "movie" || kind != "movie" && v.Kind == "movie" {
				f.LocalMetadataIssue = "video_nfo_kind_conflict"
				continue
			}
			f.VideoMetadata = append(f.VideoMetadata, *v)
		}
		f.VideoMetadataStatus = "observed"
	}
	return f
}
