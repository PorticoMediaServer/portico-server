package localmetadata

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/assets"
	"strings"
)

func (s *Service) prepareAudioEvidence(ctx context.Context, library, kind, path string, f assets.Facts) assets.Facts {
	if f.Tags == nil {
		f.Tags = map[string]string{}
	}
	if f.TagSources == nil {
		f.TagSources = map[string]string{}
	}
	// Older in-process producers may provide just Tags. Preserve them as evidence.
	if len(f.AudioEvidence) == 0 {
		for k, v := range f.Tags {
			source := f.TagSources[k]
			if source == "" {
				source = "embedded"
			}
			f.AudioEvidence = append(f.AudioEvidence, assets.AudioTagEvidence{Field: k, Value: v, Source: source})
		}
	}
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	sides := []string{filepath.Join(filepath.Dir(path), "album.nfo"), stem + ".nfo", path + ".portico.json"}
	if kind == "audiobook" {
		sides = []string{filepath.Join(filepath.Dir(path), "metadata.opf"), stem + ".opf", filepath.Join(filepath.Dir(path), "book.nfo"), stem + ".nfo", path + ".portico.json"}
	}
	for _, side := range sides {
		raw, e := s.storage.ReadSmall(ctx, library, side, 64<<10)
		if e != nil {
			continue
		}
		var values map[string]string
		source := "nfo"
		switch filepath.Ext(side) {
		case ".opf":
			source = "opf"
			values, e = parseOPF(raw)
		case ".json":
			source = "portico_json"
			values, e = parseAudioJSON(raw)
		default:
			values, e = parseMusicNFO(raw, kind)
		}
		if e != nil {
			f.LocalMetadataIssue = "invalid_" + source + "_sidecar"
			continue
		}
		for k, v := range values {
			v = assets.CleanAudioTag(k, v)
			if !assets.ValidAudioTag(k, v) || len(f.AudioEvidence) >= 512 {
				continue
			}
			f.AudioEvidence = append(f.AudioEvidence, assets.AudioTagEvidence{Field: k, Value: v, Source: source})
			f.Tags[k] = v
			f.TagSources[k] = source
		}
	}
	return f
}
