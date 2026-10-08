package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/catalog"
	"sort"
)

// AnalysisOperation is one thing the server can do to a file after it has been
// discovered. Cost classes group the matrix so an owner can reason about what a
// library will make the machine do, not about individual feature names.
type AnalysisOperation struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	CostClass      string   `json:"costClass"`
	Description    string   `json:"description"`
	MediaKinds     []string `json:"mediaKinds"`
	DefaultEnabled bool     `json:"defaultEnabled"`
	Requires       []string `json:"requires"`
	// ProducesArtifacts says the operation writes bytes into server storage, so
	// the storage care page has a category for it.
	ProducesArtifacts bool `json:"producesArtifacts"`
	// Network says the operation contacts something outside this machine.
	Network bool `json:"network"`
}

// CostClass is the published vocabulary the matrix groups by.
type CostClass struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Order lets a client render the groups cheapest first without inventing an
	// order of its own.
	Order int `json:"order"`
}

// CostClasses is the whole vocabulary, cheapest first.
func CostClasses() []CostClass {
	return []CostClass{
		{"metadata-only", "Metadata only", "Reads container headers and sidecar files. Costs one small read per file.", 1},
		{"single-pass-read", "One pass over the file", "Reads the media once end to end without decoding video.", 2},
		{"decode", "Decodes the video", "Decodes frames. This is the expensive class: expect sustained CPU or a busy hardware encoder for the length of the file.", 3},
		{"network", "Contacts a provider", "Sends a request off this machine. Rate limits and provider availability apply.", 4},
	}
}

// AnalysisOperations is every analysis operation this server knows about. The
// library configuration page enables them per library; the scan policy carries
// the enabled set into the ingestion pipeline.
func AnalysisOperations() []AnalysisOperation {
	video := []string{"movie", "show", "episode"}
	all := []string{"movie", "show", "episode", "music", "book"}
	operations := []AnalysisOperation{
		{"probe", "Media probe", "metadata-only", "Reads container, stream, codec, resolution and duration facts. Everything else depends on this.", all, true, nil, false, false},
		{"local_metadata", "Local metadata and sidecars", "metadata-only", "Reads embedded tags, NFO files and sidecar images that sit next to the media.", all, true, []string{"probe"}, false, false},
		{"artwork_extraction", "Embedded artwork", "metadata-only", "Extracts cover art already embedded in the file.", all, true, []string{"probe"}, true, false},
		{"subtitle_discovery", "Subtitle discovery", "metadata-only", "Lists the subtitle tracks inside the file and the subtitle files beside it.", video, true, []string{"probe"}, false, false},
		{"chapter_read", "Chapter markers", "metadata-only", "Reads chapter markers the file already carries.", append([]string{"book"}, video...), true, []string{"probe"}, false, false},
		{"deep_stream_analysis", "Deep stream analysis", "single-pass-read", "Reads the whole file to confirm frame rate, bit depth, HDR format and stream health when headers are incomplete.", video, false, []string{"probe"}, false, false},
		{"subtitle_extraction", "Subtitle extraction", "single-pass-read", "Writes embedded text subtitles out as separate tracks so they can be delivered without a conversion.", video, false, []string{"subtitle_discovery"}, true, false},
		{"loudness", "Loudness measurement", "single-pass-read", "Measures programme loudness so playback can normalise between items.", all, false, []string{"probe"}, false, false},
		{"audio_fingerprint", "Audio fingerprint", "single-pass-read", "Computes an acoustic fingerprint used to match recordings that tags cannot identify.", []string{"music"}, false, []string{"probe"}, false, false},
		{"trickplay", "Trickplay tiles", "decode", "Decodes evenly spaced frames into scrubbing tiles. Storage grows with interval and tile width.", video, false, []string{"probe"}, true, false},
		{"chapter_images", "Chapter images", "decode", "Decodes one frame per chapter for the chapter list.", video, false, []string{"probe"}, true, false},
		{"video_preview", "Video preview", "decode", "Encodes a short silent preview clip used when browsing.", video, false, []string{"probe"}, true, false},
		{"segment_detection", "Intro and recap detection", "decode", "Compares episodes within a season to find repeated openings so they can be skipped.", []string{"show", "episode"}, false, []string{"probe"}, true, false},
		{"credits_detection", "Credits detection", "decode", "Finds where the end credits begin so playback can offer the next episode.", video, false, []string{"probe"}, true, false},
		{"subtitle_download", "Subtitle download", "network", "Asks the configured subtitle provider for tracks the file does not carry.", video, false, []string{"subtitle_discovery"}, true, true},
		{"lyrics", "Lyrics", "network", "Fetches timed lyrics for tracks that have none.", []string{"music"}, false, []string{"probe"}, true, true},
	}
	operations = append(operations, AnalysisOperation{ID: "checksum", Name: "File checksum", CostClass: "single-pass-read", MediaKinds: all}, AnalysisOperation{ID: "waveform", Name: "Audio waveform", CostClass: "decode", MediaKinds: all, Requires: []string{"probe"}, ProducesArtifacts: true})
	supported := []AnalysisOperation{}
	for _, op := range operations {
		if _, ok := analysisRuntime[op.ID]; ok {
			supported = append(supported, op)
		}
	}
	return supported

}

// AnalysisMatrix is the published document behind the library configuration
// page's analysis section.
type AnalysisMatrix struct {
	CostClasses []CostClass         `json:"costClasses"`
	Operations  []AnalysisOperation `json:"operations"`
}

// Matrix publishes the operation vocabulary. It is static server knowledge and
// needs no database read.
func Matrix() AnalysisMatrix { return AnalysisMatrix{CostClasses(), AnalysisOperations()} }

func analysisOperationIDs() map[string]AnalysisOperation {
	out := map[string]AnalysisOperation{}
	for _, op := range AnalysisOperations() {
		out[op.ID] = op
	}
	return out
}

// MetadataProvider is one provider the server can ask about one media kind.
type MetadataProvider struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	MediaKinds      []string `json:"mediaKinds"`
	SupportsAPIKey  bool     `json:"supportsApiKey"`
	SupportsLocale  bool     `json:"supportsLocale"`
	AttributionNote string   `json:"attributionNote"`
}

// MetadataProviders publishes provider choices so a client never hard-codes the
// list or guesses which provider can serve which kind.
func MetadataProviders() []MetadataProvider {
	return []MetadataProvider{
		{"tmdb", "TMDB", []string{"movie", "show", "episode"}, false, true, "Metadata provided by TMDB."},
		{"tvdb", "TheTVDB", []string{"movie", "show", "episode"}, false, true, "Metadata provided by TheTVDB."},
		{"musicbrainz", "MusicBrainz", []string{"music", "book"}, false, false, "Music metadata provided by MusicBrainz."},
		{"anilist", "AniList", []string{"show", "episode", "movie"}, false, false, "Anime metadata provided by AniList."},
		{"local", "Local files only", []string{"movie", "show", "episode", "music", "book"}, false, false, ""},
	}
}

// ProviderSelection is one media kind's provider choice for one library.
type ProviderSelection struct {
	MediaKind string `json:"mediaKind"`
	Provider  string `json:"provider"`
	// APIKey is retained for wire compatibility; nonempty writes are rejected.
	// Credentials are managed by the provider runtime, never this document.
	APIKey    string `json:"apiKey"`
	APIKeySet bool   `json:"apiKeySet"`
	Language  string `json:"language"`
	Region    string `json:"region"`
}

// GeneratedNavigation controls the artifacts the player navigates with.
type GeneratedNavigation struct {
	TrickplayIntervalSeconds int    `json:"trickplayIntervalSeconds"`
	TrickplayTileWidth       int    `json:"trickplayTileWidth"`
	TrickplayMaxTiles        int    `json:"trickplayMaxTiles"`
	ChapterThumbnailMode     string `json:"chapterThumbnailMode"`
	VideoPreviewEnabled      bool   `json:"videoPreviewEnabled"`
	VideoPreviewSeconds      int    `json:"videoPreviewSeconds"`
}

// LibrarySettings is one library's whole administration document.
type LibrarySettings struct {
	AllowMediaDeletion bool                             `json:"allowMediaDeletion"`
	TrashRetentionDays int                              `json:"trashRetentionDays"`
	ContinueWatching   catalog.ContinueWatchingSettings `json:"continueWatching"`
	Providers          []ProviderSelection              `json:"providers"`
	Analysis           []string                         `json:"analysis"`
	Navigation         GeneratedNavigation              `json:"navigation"`
}

// ChapterThumbnailModes is the published choice list for chapter images.
var ChapterThumbnailModes = []string{"embedded", "generated"}

// DefaultLibrarySettings is the answer before any owner write. Deletion is off:
// a library only deletes media once its owner has said so on this page.
func DefaultLibrarySettings() LibrarySettings {
	enabled := []string{}
	for _, op := range AnalysisOperations() {
		if op.DefaultEnabled {
			enabled = append(enabled, op.ID)
		}
	}
	return LibrarySettings{
		AllowMediaDeletion: false,
		TrashRetentionDays: 30,
		ContinueWatching:   catalog.DefaultContinueWatchingSettings(),
		Providers:          []ProviderSelection{},
		Analysis:           enabled,
		Navigation:         GeneratedNavigation{TrickplayIntervalSeconds: 10, TrickplayTileWidth: 320, TrickplayMaxTiles: 400, ChapterThumbnailMode: "embedded", VideoPreviewEnabled: false, VideoPreviewSeconds: 30},
	}
}

func normalizeLibrarySettings(v *LibrarySettings) {
	catalog.NormalizeContinueWatchingSettings(&v.ContinueWatching)
	if v.Providers == nil {
		v.Providers = []ProviderSelection{}
	}
	if v.Analysis == nil {
		v.Analysis = []string{}
	}
	sort.Strings(v.Analysis)
	sort.Slice(v.Providers, func(i, j int) bool { return v.Providers[i].MediaKind < v.Providers[j].MediaKind })
}

// redactLibrarySettings strips provider keys on the way out. The stored document
// does not accept keys; no read hands legacy values back.
func redactLibrarySettings(v *LibrarySettings) {
	normalizeLibrarySettings(v)
	for i := range v.Providers {
		v.Providers[i].APIKeySet = v.Providers[i].APIKey != ""
		v.Providers[i].APIKey = ""
	}
}

func validLocale(v string, max int) bool {
	if v == "" {
		return true
	}
	if len(v) > max {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validateLibrarySettings(v *LibrarySettings) error {
	fields := []string{}
	if !catalog.ValidContinueWatchingSettings(v.ContinueWatching) {
		fields = append(fields, "settings.continueWatching")
	}
	if v.TrashRetentionDays < 0 || v.TrashRetentionDays > 3650 {
		fields = append(fields, "settings.trashRetentionDays")
	}
	providers, kinds := map[string]MetadataProvider{}, map[string]bool{}
	for _, p := range MetadataProviders() {
		providers[p.ID] = p
	}
	for _, sel := range v.Providers {
		p, ok := providers[sel.Provider]
		// One provider per media kind: providers are chosen, never chained.
		if !ok || kinds[sel.MediaKind] {
			fields = append(fields, "settings.providers")
			break
		}
		kinds[sel.MediaKind] = true
		serves := false
		for _, kind := range p.MediaKinds {
			serves = serves || kind == sel.MediaKind
		}
		if !serves || !safeText(sel.APIKey, 256) || sel.APIKey != "" && !p.SupportsAPIKey || !validLocale(sel.Language, 16) || !validLocale(sel.Region, 8) {
			fields = append(fields, "settings.providers")
			break
		}
	}
	if len(v.Providers) > 16 {
		fields = append(fields, "settings.providers")
	}
	known, seen := analysisOperationIDs(), map[string]bool{}
	for _, id := range v.Analysis {
		if _, ok := known[id]; !ok || seen[id] {
			fields = append(fields, "settings.analysis")
			break
		}
		seen[id] = true
	}
	// A dependency that is not enabled would make the operation silently never
	// run, so the page refuses the combination instead of running a half matrix.
	for _, id := range v.Analysis {
		for _, need := range known[id].Requires {
			if !seen[need] {
				fields = append(fields, "settings.analysis")
			}
		}
	}
	if !oneOf(v.Navigation.ChapterThumbnailMode, ChapterThumbnailModes...) {
		fields = append(fields, "settings.navigation.chapterThumbnailMode")
	}
	if len(fields) > 0 {
		return invalid(fields...)
	}
	clampInt(&v.Navigation.TrickplayIntervalSeconds, 1, 600)
	clampInt(&v.Navigation.TrickplayTileWidth, 96, 640)
	clampInt(&v.Navigation.TrickplayMaxTiles, 20, 20000)
	clampInt(&v.Navigation.VideoPreviewSeconds, 5, 120)
	return nil
}

// LibraryDocument is the library configuration page's response: the settings,
// the library it belongs to, and the vocabularies the page renders from.
type LibraryDocument struct {
	LibraryID   string                 `json:"libraryId"`
	LibraryName string                 `json:"libraryName"`
	LibraryKind string                 `json:"libraryKind"`
	Revision    int64                  `json:"revision"`
	Digest      string                 `json:"digest"`
	Settings    LibrarySettings        `json:"settings"`
	Matrix      AnalysisMatrix         `json:"analysisMatrix"`
	Providers   []MetadataProvider     `json:"availableProviders"`
	Modes       map[string][]string    `json:"enumerations"`
	Sources     []LibrarySourceSummary `json:"sources"`
}

// LibrarySourceSummary names each root the library scans, so the folder picker
// opens where the library already looks.
type LibrarySourceSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Root           string `json:"root"`
	Classification string `json:"classification"`
	Enabled        bool   `json:"enabled"`
}

func librarySources(ctx context.Context, tx *sql.Tx, library string) ([]LibrarySourceSummary, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,name,configured_root,classification,enabled FROM library_sources WHERE library_id=? ORDER BY id`, library)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibrarySourceSummary{}
	for rows.Next() {
		var s LibrarySourceSummary
		if err = rows.Scan(&s.ID, &s.Name, &s.Root, &s.Classification, &s.Enabled); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func libraryScope(id string) string { return scopeFor("library", id) }

// libraryDefaults is what a library holds before its own page is written. The
// trash retention comes from the owner settings registry's server-wide value, so
// that setting genuinely governs and this page only overrides it. The settings
// document is read, never written, from here.
func libraryDefaults(ctx context.Context, tx *sql.Tx) (LibrarySettings, error) {
	out := DefaultLibrarySettings()
	var body string
	err := tx.QueryRowContext(ctx, `SELECT body FROM console_documents WHERE scope='server'`).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var decoded struct {
		TrashRetentionDays *int `json:"library.trashRetentionDays"`
	}
	if json.Unmarshal([]byte(body), &decoded) == nil && decoded.TrashRetentionDays != nil && *decoded.TrashRetentionDays >= 0 && *decoded.TrashRetentionDays <= 3650 {
		out.TrashRetentionDays = *decoded.TrashRetentionDays
	}
	return out, nil
}

func (s *Service) libraryDocument(ctx context.Context, tx *sql.Tx, id string) (LibraryDocument, error) {
	out := LibraryDocument{LibraryID: id, Matrix: Matrix(), Modes: map[string][]string{"chapterThumbnailMode": ChapterThumbnailModes, "videoCompletion": {"earliest", "threshold", "credits"}}}
	name, kind, err := libraryExists(ctx, tx, id)
	if err != nil {
		return out, err
	}
	out.LibraryName, out.LibraryKind = name, kind
	// Only what this library can choose: AniList serves anime libraries alone.
	out.Providers = []MetadataProvider{}
	for _, p := range MetadataProviders() {
		if p.ID != "anilist" || kind == "anime" {
			out.Providers = append(out.Providers, p)
		}
	}
	defaults, err := libraryDefaults(ctx, tx)
	if err != nil {
		return out, err
	}
	settings, revision, err := readDocument(ctx, tx, libraryScope(id), defaults)
	if err != nil {
		return out, err
	}
	revision, err = effectiveLibraryPolicy(ctx, tx, id, kind, &settings, revision)
	if err != nil {
		return out, err
	}
	redactLibrarySettings(&settings)
	out.Settings, out.Revision, out.Digest = settings, revision, digestOf(settings)
	out.Sources, err = librarySources(ctx, tx, id)
	return out, err
}

// LibrarySettingsFor reads one library's configuration page.
func (s *Service) LibrarySettingsFor(ctx context.Context, auth Authorize, id string) (LibraryDocument, error) {
	var out LibraryDocument
	err := s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		var err error
		out, err = s.libraryDocument(ctx, tx, id)
		return err
	})
	return out, err
}

// SaveLibrarySettings applies one library configuration write. An empty apiKey
// keeps whatever key is already stored, so a client that never receives a key
// can still save the rest of the form.
func (s *Service) SaveLibrarySettings(ctx context.Context, auth Authorize, id string, change Change[LibrarySettings]) (LibraryDocument, error) {
	var out LibraryDocument
	if !validOperationID(change.OperationID) {
		return out, ErrInput
	}
	value := change.Settings
	normalizeLibrarySettings(&value)
	if err := validateLibrarySettings(&value); err != nil {
		return out, err
	}
	scope := libraryScope(id)
	digest := digestOf(struct {
		Expected int64           `json:"expected"`
		Settings LibrarySettings `json:"settings"`
	}{change.ExpectedRevision, value})
	err := s.transaction(ctx, auth, func(tx *sql.Tx) error {
		stored, replayed, err := receipt[LibraryDocument](ctx, tx, scope, change.OperationID, digest)
		if err != nil {
			return err
		}
		if replayed {
			out = stored
			return nil
		}
		_, kind, err := libraryExists(ctx, tx, id)
		if err != nil {
			return err
		}
		defaults, err := libraryDefaults(ctx, tx)
		if err != nil {
			return err
		}
		current, revision, err := readDocument(ctx, tx, scope, defaults)
		if err != nil {
			return err
		}
		effective, err := effectiveLibraryPolicy(ctx, tx, id, kind, &current, revision)
		if err != nil {
			return err
		}
		if effective != change.ExpectedRevision {
			return ErrConflict
		}
		if err = applyLibraryPolicy(ctx, tx, id, kind, value); err != nil {
			return err
		}
		// Runtime policy lives only in domain tables. Keep this document for
		// deletion settings; never persist provider credentials here.
		value.Providers = nil
		value.Analysis = nil
		value.Navigation = GeneratedNavigation{}
		at := s.milliseconds()
		if err = writeDocument(ctx, tx, scope, revision+1, value, at); err != nil {
			return err
		}
		if out, err = s.libraryDocument(ctx, tx, id); err != nil {
			return err
		}
		return saveReceipt(ctx, tx, scope, change.OperationID, digest, out, at)
	})
	return out, err
}

// deletionPolicy answers the two questions the delete path asks of a library.
func deletionPolicy(ctx context.Context, tx *sql.Tx, library string) (bool, int, error) {
	defaults, err := libraryDefaults(ctx, tx)
	if err != nil {
		return false, 0, err
	}
	settings, _, err := readDocument(ctx, tx, libraryScope(library), defaults)
	if err != nil {
		return false, 0, err
	}
	return settings.AllowMediaDeletion, settings.TrashRetentionDays, nil
}

// LibrarySettingsIndex is the libraries listing the administration pages open
// with: one row per library with just enough to render the list.
type LibrarySettingsIndex struct {
	Items      []LibraryIndexEntry `json:"items"`
	NextCursor string              `json:"nextCursor"`
}

// LibraryIndexEntry is one row of that listing.
type LibraryIndexEntry struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Kind               string `json:"kind"`
	Revision           int64  `json:"revision"`
	AllowMediaDeletion bool   `json:"allowMediaDeletion"`
	TrashRetentionDays int    `json:"trashRetentionDays"`
	AnalysisCount      int    `json:"analysisOperationCount"`
	SourceCount        int    `json:"sourceCount"`
}

// LibraryIndex pages the libraries by name.
func (s *Service) LibraryIndex(ctx context.Context, auth Authorize, token string, limit int) (LibrarySettingsIndex, error) {
	out := LibrarySettingsIndex{Items: []LibraryIndexEntry{}}
	size, err := pageSize(limit)
	if err != nil {
		return out, err
	}
	c, err := decodeCursor("library-index", token)
	if err != nil {
		return out, err
	}
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT l.id,l.name,l.kind,(SELECT COUNT(*) FROM library_sources s WHERE s.library_id=l.id) FROM libraries l WHERE (?='' OR l.id>?) ORDER BY l.id LIMIT ?`, c.ID, c.ID, size+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e LibraryIndexEntry
			if err = rows.Scan(&e.ID, &e.Name, &e.Kind, &e.SourceCount); err != nil {
				return err
			}
			out.Items = append(out.Items, e)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if len(out.Items) > size {
			out.Items = out.Items[:size]
			out.NextCursor = encodeCursor("library-index", 0, out.Items[size-1].ID)
		}
		defaults, err := libraryDefaults(ctx, tx)
		if err != nil {
			return err
		}
		for i := range out.Items {
			settings, revision, err := readDocument(ctx, tx, libraryScope(out.Items[i].ID), defaults)
			if err != nil {
				return err
			}
			out.Items[i].Revision = revision
			out.Items[i].AllowMediaDeletion = settings.AllowMediaDeletion
			out.Items[i].TrashRetentionDays = settings.TrashRetentionDays
			out.Items[i].AnalysisCount = len(settings.Analysis)
		}
		return nil
	})
	return out, err
}
