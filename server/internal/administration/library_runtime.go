package administration

import (
	"context"
	"database/sql"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/metadata"
	"sort"
)

var analysisRuntime = map[string]string{"probe": "probe", "local_metadata": "local_metadata", "subtitle_discovery": "subtitles", "loudness": "loudness", "audio_fingerprint": "fingerprint", "trickplay": "trickplay", "chapter_images": "chapter_images", "segment_detection": "segment_detection", "checksum": "checksum", "waveform": "waveform"}

func libraryMediaKind(kind string) string {
	switch kind {
	case "tv", "anime":
		return "show"
	case "audiobook":
		return "book"
	}
	return kind
}
func effectiveLibraryPolicy(ctx context.Context, tx *sql.Tx, id, kind string, v *LibrarySettings, revision int64) (int64, error) {
	p, err := catalog.ScanPolicyTx(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	provider, err := metadata.AdministrationPolicyTx(ctx, tx, id, kind)
	if err != nil {
		return 0, err
	}
	v.Analysis = []string{}
	for ui, op := range analysisRuntime {
		if p.Allows(op) {
			v.Analysis = append(v.Analysis, ui)
		}
	}
	sort.Strings(v.Analysis)
	geometry := p.Trickplay.Normalized()
	v.Navigation = GeneratedNavigation{TrickplayIntervalSeconds: geometry.IntervalSeconds, TrickplayTileWidth: geometry.TileWidth, TrickplayMaxTiles: geometry.MaxTiles, ChapterThumbnailMode: "embedded", VideoPreviewSeconds: 30}
	if p.Allows("chapter_images") {
		v.Navigation.ChapterThumbnailMode = "generated"
	}
	v.Providers = []ProviderSelection{}
	for _, providerID := range provider.Providers {
		v.Providers = append(v.Providers, ProviderSelection{MediaKind: libraryMediaKind(kind), Provider: providerID, Language: provider.Language, Region: provider.Region})
	}
	if len(v.Providers) == 0 {
		v.Providers = append(v.Providers, ProviderSelection{MediaKind: libraryMediaKind(kind), Provider: "local"})
	}
	// Every contributing revision only increases, so all administration and
	// domain writers invalidate this single optimistic revision without hashes.
	return revision + p.Revision - 1 + provider.Revision, nil
}
func applyLibraryPolicy(ctx context.Context, tx *sql.Tx, id, kind string, v LibrarySettings) error {
	ops := []string{}
	seen := map[string]bool{}
	for _, ui := range v.Analysis {
		op, ok := analysisRuntime[ui]
		if !ok {
			return invalid("settings.analysis")
		}
		if !seen[op] {
			ops = append(ops, op)
			seen[op] = true
		}
	}
	if v.Navigation.VideoPreviewEnabled || v.Navigation.VideoPreviewSeconds != 30 {
		return invalid("settings.navigation.videoPreviewEnabled")
	}
	if (v.Navigation.ChapterThumbnailMode == "generated") != seen["chapter_images"] || v.Navigation.ChapterThumbnailMode == "none" {
		return invalid("settings.navigation.chapterThumbnailMode")
	}
	p := catalog.ScanPolicy{Tier: "custom", Operations: ops, Trickplay: catalog.TrickplaySettings{IntervalSeconds: v.Navigation.TrickplayIntervalSeconds, TileWidth: v.Navigation.TrickplayTileWidth, MaxTiles: v.Navigation.TrickplayMaxTiles}}
	if err := catalog.ApplyScanPolicyTx(ctx, tx, id, p); err != nil {
		return invalid("settings.analysis", "settings.navigation")
	}
	provider := metadata.LibraryProviderPolicy{Providers: []string{}}
	selected := map[string]bool{}
	for i, sel := range v.Providers {
		if sel.APIKey != "" {
			return invalid("settings.providers.apiKey")
		}
		if sel.MediaKind != libraryMediaKind(kind) {
			return invalid("settings.providers.mediaKind")
		}
		if i == 0 {
			provider.Language, provider.Region = sel.Language, sel.Region
		} else if provider.Language != sel.Language || provider.Region != sel.Region {
			return invalid("settings.providers")
		}
		if sel.Provider == "local" {
			if len(v.Providers) > 1 {
				return invalid("settings.providers")
			}
			continue
		}
		if !selected[sel.Provider] {
			provider.Providers = append(provider.Providers, sel.Provider)
			selected[sel.Provider] = true
		}
	}
	if err := metadata.ApplyAdministrationPolicyTx(ctx, tx, id, kind, provider); err != nil {
		return invalid("settings.providers")
	}
	return nil
}
