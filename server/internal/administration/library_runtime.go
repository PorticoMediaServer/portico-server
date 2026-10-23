package administration

import (
	"context"
	"database/sql"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/metadata"
	"sort"
)

var analysisRuntime = map[string]string{"probe": "probe", "local_metadata": "local_metadata", "subtitle_discovery": "subtitles", "loudness": "loudness", "audio_fingerprint": "fingerprint", "trickplay": "trickplay", "chapter_images": "chapter_images", "checksum": "checksum", "waveform": "waveform"}

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
	prefs, err := catalog.LibraryPreferencesTx(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	v.GroupCollections = prefs.GroupCollections
	v.LocalTags = prefs.LocalTags
	v.LyricsSource = prefs.LyricsSource
	v.TitleSource = prefs.TitleSource
	v.EpisodeNumbering = prefs.EpisodeNumbering
	v.FindExtras = prefs.FindExtras
	v.SortIgnoreArticles = prefs.SortIgnoreArticles
	v.RefreshMode = "replace_unlocked"
	v.AcoustIDEnabled = false
	if kind == "music" || kind == "audiobook" {
		if err = tx.QueryRowContext(ctx, `SELECT local_mode,acoustid_enabled FROM audio_metadata_policies WHERE library_id=?`, id).Scan(&v.LocalTags, &v.AcoustIDEnabled); err != nil {
			return 0, err
		}
	}
	if kind == "movie" || kind == "tv" || kind == "anime" {
		if err = tx.QueryRowContext(ctx, `SELECT refresh_mode FROM screen_metadata_policies WHERE library_id=?`, id).Scan(&v.RefreshMode); err != nil {
			return 0, err
		}
	}
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
		if containsOperation(p.Operations, op) {
			v.Analysis = append(v.Analysis, ui)
		}
	}
	sort.Strings(v.Analysis)
	v.ScanDepth = scanDepthFor(p)
	geometry := p.Trickplay.Normalized()
	v.Navigation = GeneratedNavigation{TrickplayIntervalMs: geometry.IntervalSeconds * 1000, TrickplayTileWidth: geometry.TileWidth, TrickplayMaxTiles: geometry.MaxTiles, ChapterThumbnailMode: "embedded", VideoPreviewMs: 30000}
	if containsOperation(p.Operations, "chapter_images") {
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
	return revision + p.Revision - 1 + provider.Revision + prefs.Revision - 1, nil
}
func applyLibraryPolicy(ctx context.Context, tx *sql.Tx, id, kind string, v LibrarySettings) error {
	if kind != "movie" && (!v.GroupCollections) {
		return invalid("settings.groupCollections")
	}
	if kind != "movie" && kind != "tv" && kind != "anime" {
		if v.TitleSource != "tags" || !v.FindExtras || v.RefreshMode != "replace_unlocked" {
			return invalid("settings.titleSource", "settings.findExtras", "settings.refreshMode")
		}
	}
	if (kind == "book" || kind == "comic") && v.LocalTags != "prefer" {
		return invalid("settings.localTags")
	}
	if kind != "music" && kind != "audiobook" && v.LocalTags == "supplement" {
		return invalid("settings.localTags")
	}
	if kind != "music" && (v.LyricsSource != "files" || v.AcoustIDEnabled) {
		return invalid("settings.lyricsSource", "settings.acoustidEnabled")
	}
	if kind != "tv" && kind != "anime" && v.EpisodeNumbering != "season" {
		return invalid("settings.episodeNumbering")
	}
	prefs := catalog.LibraryPreferences{GroupCollections: v.GroupCollections, LocalTags: v.LocalTags, LyricsSource: v.LyricsSource, TitleSource: v.TitleSource, EpisodeNumbering: v.EpisodeNumbering, FindExtras: v.FindExtras, SortIgnoreArticles: v.SortIgnoreArticles}
	if kind == "music" || kind == "audiobook" {
		prefs.LocalTags = "prefer"
	}
	if err := catalog.ApplyLibraryPreferencesTx(ctx, tx, id, prefs); err != nil {
		return err
	}
	if kind == "music" || kind == "audiobook" {
		if _, err := tx.ExecContext(ctx, `UPDATE audio_metadata_policies SET local_mode=?,acoustid_enabled=?,revision=revision+1 WHERE library_id=? AND (local_mode<>? OR acoustid_enabled<>?)`, v.LocalTags, v.AcoustIDEnabled, id, v.LocalTags, v.AcoustIDEnabled); err != nil {
			return err
		}
	}
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
	if v.Navigation.VideoPreviewEnabled || v.Navigation.VideoPreviewMs != 30000 {
		return invalid("settings.navigation.videoPreviewEnabled")
	}
	if (v.Navigation.ChapterThumbnailMode == "generated") != seen["chapter_images"] || v.Navigation.ChapterThumbnailMode == "none" {
		return invalid("settings.navigation.chapterThumbnailMode")
	}
	p := catalog.ScanPolicy{Tier: "custom", Operations: ops, Trickplay: catalog.TrickplaySettings{IntervalSeconds: v.Navigation.TrickplayIntervalMs / 1000, TileWidth: v.Navigation.TrickplayTileWidth, MaxTiles: v.Navigation.TrickplayMaxTiles}}
	if v.ScanDepth == "file_list_only" {
		p.Tier = "file_list_only"
	}
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
	provider.RefreshMode = v.RefreshMode
	if err := metadata.ApplyAdministrationPolicyTx(ctx, tx, id, kind, provider); err != nil {
		return invalid("settings.providers")
	}
	return nil
}

func containsOperation(ops []string, op string) bool {
	for _, v := range ops {
		if v == op {
			return true
		}
	}
	return false
}
func scanDepthFor(p catalog.ScanPolicy) string {
	if p.Tier == "file_list_only" {
		return p.Tier
	}
	if !containsOperation(p.Operations, "probe") {
		return "custom"
	}
	basic := containsOperation(p.Operations, "local_metadata") && containsOperation(p.Operations, "subtitles") && !containsOperation(p.Operations, "checksum") && !containsOperation(p.Operations, "fingerprint")
	if basic && !containsOperation(p.Operations, "waveform") && !containsOperation(p.Operations, "loudness") {
		return "basic"
	}
	if basic && containsOperation(p.Operations, "waveform") && containsOperation(p.Operations, "loudness") {
		return "complete"
	}
	return "custom"
}
