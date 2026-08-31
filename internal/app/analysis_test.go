package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaAnalysisFromFFprobeMapsDurationAndStreams(t *testing.T) {
	duration, streams, chapters, attachments := mediaAnalysisFromFFprobe("movie", ffprobePayload{
		Format: ffprobeFormat{Duration: "123.6"},
		Streams: []ffprobeStream{
			{Index: 0, CodecType: "video", CodecName: "h264", Width: 1920, Height: 1080, AverageFrameRate: "24000/1001", AspectRatio: "16:9", BitRate: "6000000", Disposition: map[string]int{"default": 1}},
			{Index: 1, CodecType: "audio", CodecName: "aac", Channels: 2, ChannelLayout: "stereo", SampleRate: "48000", BitRate: "192000", Tags: map[string]string{"language": "eng"}},
			{Index: 2, CodecType: "subtitle", CodecName: "subrip", Tags: map[string]string{"language": "spa"}, Disposition: map[string]int{"forced": 1, "hearing_impaired": 1}},
			{Index: 3, CodecType: "attachment", CodecName: "ttf", Tags: map[string]string{"filename": "../Fancy Font.ttf", "mimetype": "application/x-truetype-font"}},
		},
		Chapters: []ffprobeChapter{
			{StartTime: "0.000000", EndTime: "60.000000", Tags: map[string]string{"title": "Cold Open"}},
			{StartTime: "60.000000", EndTime: "123.600000"},
		},
	})
	if duration != 124 {
		t.Fatalf("duration = %d, expected 124", duration)
	}
	if len(streams) != 3 {
		t.Fatalf("stream count = %d, expected 3", len(streams))
	}
	if streams[0].Kind != "video" || streams[0].Width != 1920 || streams[0].Codec != "h264" {
		t.Fatalf("video stream not mapped: %+v", streams[0])
	}
	if streams[0].Index != 0 || streams[0].FrameRate != 23.976 || streams[0].AspectRatio != "16:9" || !streams[0].Default {
		t.Fatalf("video technical stream data not mapped: %+v", streams[0])
	}
	if streams[1].Kind != "audio" || streams[1].Language != "eng" || streams[1].Channels != 2 {
		t.Fatalf("audio stream not mapped: %+v", streams[1])
	}
	if streams[1].Index != 1 || streams[1].ChannelLayout != "stereo" || streams[1].SampleRate != 48000 {
		t.Fatalf("audio technical stream data not mapped: %+v", streams[1])
	}
	if streams[2].Kind != "subtitle" || streams[2].Language != "spa" {
		t.Fatalf("subtitle stream not mapped: %+v", streams[2])
	}
	if streams[2].Index != 2 || !streams[2].Forced || !streams[2].HearingImpaired {
		t.Fatalf("subtitle dispositions not mapped: %+v", streams[2])
	}
	if len(chapters) != 2 {
		t.Fatalf("chapter count = %d, expected 2", len(chapters))
	}
	if chapters[0].Title != "Cold Open" || chapters[0].StartSeconds != 0 || chapters[0].EndSeconds != 60 {
		t.Fatalf("chapter not mapped: %+v", chapters[0])
	}
	if chapters[1].Title != "Chapter 2" || chapters[1].StartSeconds != 60 {
		t.Fatalf("fallback chapter not mapped: %+v", chapters[1])
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v", attachments)
	}
	attachmentStreamBytes, attachmentStreamErr := hex.DecodeString(attachments[0].StreamID)
	if attachments[0].Filename != "Fancy Font.ttf" || attachments[0].MimeType != "application/x-truetype-font" || attachmentStreamErr != nil || len(attachmentStreamBytes) != 20 {
		t.Fatalf("attachments = %#v", attachments)
	}
}

func TestExactSeekEvidenceRequiresEveryHLSGridBoundary(t *testing.T) {
	if !keyframesCoverExactSeekGrid([]float64{0, 4, 8, 12, 16}, 18, 4, 0.12) {
		t.Fatal("aligned keyframe evidence was rejected")
	}
	if keyframesCoverExactSeekGrid([]float64{0, 3, 8, 12, 16}, 18, 4, 0.12) {
		t.Fatal("irregular GOP evidence was accepted")
	}
	item := MediaItem{Streams: []Stream{{Kind: "video", ExactSeekSafe: true, KeyframeEvidenceAt: "2026-07-16T00:00:00Z"}}}
	if !directStreamExactSeekEvidence(item) {
		t.Fatal("persisted positive exact-seek evidence was ignored")
	}
	item.MediaFiles = []MediaFileVersion{{ID: "source", Selected: true, ModTime: "2026-07-16T00:00:01Z"}}
	if directStreamExactSeekEvidence(item) {
		t.Fatal("evidence older than a replaced source file was accepted")
	}
}

func TestMediaAnalysisSkipsAttachedPictureStreams(t *testing.T) {
	duration, streams, _, _ := mediaAnalysisFromFFprobe("track", ffprobePayload{
		Format: ffprobeFormat{Duration: "60"},
		Streams: []ffprobeStream{
			{Index: 0, CodecType: "video", CodecName: "mjpeg", Disposition: map[string]int{"attached_pic": 1}},
			{Index: 1, CodecType: "audio", CodecName: "mp3", Channels: 2},
		},
	})
	if duration != 60 {
		t.Fatalf("duration = %d", duration)
	}
	if len(streams) != 1 || streams[0].Kind != "audio" {
		t.Fatalf("streams = %#v", streams)
	}
	if index, ok := embeddedCoverStreamIndex(ffprobePayload{Streams: []ffprobeStream{{Index: 4, CodecType: "video", Disposition: map[string]int{"attached_pic": 1}}}}); !ok || index != 4 {
		t.Fatalf("embedded cover index = %d ok=%v", index, ok)
	}
}

func TestAudioNormalizationFromFFprobeIsPersistedOnMediaDetail(t *testing.T) {
	server := newScannerTestServer(t)
	now := "2026-05-04T00:00:00Z"
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at) VALUES ('track_normalized', 'track', 'Normalized Track', 'Normalized Track', '[]', ?)`, now); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	payload := ffprobePayload{
		Format: ffprobeFormat{Tags: map[string]string{
			"replaygain_track_gain": "-7.50 dB",
			"replaygain_track_peak": "0.987654",
			"replaygain_album_gain": "-6.25 dB",
			"replaygain_album_peak": "0.998",
			"integrated_lufs":       "-16.1 LUFS",
		}},
	}
	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := server.upsertAudioNormalizationFromFFprobe(tx, "track_normalized", payload, now); err != nil {
		_ = tx.Rollback()
		t.Fatalf("upsert normalization: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit normalization: %v", err)
	}

	item, err := server.getMediaDetail("", "track_normalized")
	if err != nil {
		t.Fatalf("load media detail: %v", err)
	}
	if item.AudioNormalization == nil {
		t.Fatalf("expected normalization on detail")
	}
	if item.AudioNormalization.TrackGainDB != -7.5 || item.AudioNormalization.TrackPeak != 0.987654 || item.AudioNormalization.AlbumGainDB != -6.25 || item.AudioNormalization.AlbumPeak != 0.998 || item.AudioNormalization.IntegratedLUFS != -16.1 {
		t.Fatalf("normalization = %+v", item.AudioNormalization)
	}
	if item.AudioNormalization.Source != "replaygain" || item.AudioNormalization.UpdatedAt != now {
		t.Fatalf("normalization source/time = %+v", item.AudioNormalization)
	}

	tx, err = server.db.Begin()
	if err != nil {
		t.Fatalf("begin cleanup tx: %v", err)
	}
	if err := server.upsertAudioNormalizationFromFFprobe(tx, "track_normalized", ffprobePayload{}, now); err != nil {
		_ = tx.Rollback()
		t.Fatalf("clear normalization: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit cleanup: %v", err)
	}
	cleared, err := server.getMediaDetail("", "track_normalized")
	if err != nil {
		t.Fatalf("load cleared detail: %v", err)
	}
	if cleared.AudioNormalization != nil {
		t.Fatalf("expected normalization to clear, got %+v", cleared.AudioNormalization)
	}
}

func TestFFprobeTagsUseAtomicEmbeddedMetadataApply(t *testing.T) {
	server := newScannerTestServer(t)
	now := "2026-08-05T00:00:00Z"
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, studio, genres_json, typed_metadata_json, added_at) VALUES (?, 'track', 'Filename Title', 'Filename Title', 'Manual Artist', '[]', '{}', ?)`, "track_ffprobe_metadata", now); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_metadata_locks (media_id, field, user_id, metadata_revision, updated_at) VALUES (?, 'studio', 'owner', 0, ?)`, "track_ffprobe_metadata", now); err != nil {
		t.Fatalf("insert studio lock: %v", err)
	}
	item, err := server.getMedia("", "track_ffprobe_metadata")
	if err != nil {
		t.Fatal(err)
	}
	payload := ffprobePayload{Format: ffprobeFormat{Tags: map[string]string{
		"title": "Embedded Title", "artist": "Embedded Artist", "album": "Embedded Album",
		"track": "4/12", "date": "2024", "genre": "Jazz; Soul",
	}}}
	if err := server.updateMediaTagsFromFFprobe(context.Background(), item, payload); err != nil {
		t.Fatal(err)
	}
	after, err := server.getMedia("", item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title != "Embedded Title" || after.IndexNumber != 4 || after.Year != 2024 {
		t.Fatalf("embedded canonical fields not applied: %+v", after)
	}
	if after.Studio != "Manual Artist" {
		t.Fatalf("locked studio overwritten: %q", after.Studio)
	}
	if after.MetadataRevision != item.MetadataRevision+1 {
		t.Fatalf("metadata revision=%d, want %d", after.MetadataRevision, item.MetadataRevision+1)
	}
	var evidence, genreFacets, searchMatches int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_metadata_field_values WHERE media_id = ? AND source_kind = 'embedded'`, item.ID).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_category_facets WHERE media_id = ? AND facet_type = 'genre'`, item.ID).Scan(&genreFacets); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_search WHERE media_id = ? AND media_search MATCH 'Embedded'`, item.ID).Scan(&searchMatches); err != nil {
		t.Fatal(err)
	}
	if evidence == 0 || genreFacets != 2 || searchMatches != 1 {
		t.Fatalf("evidence/projections missing: evidence=%d genres=%d search=%d", evidence, genreFacets, searchMatches)
	}
}

func TestAudioNormalizationFromLoudnormOutput(t *testing.T) {
	normalization, err := audioNormalizationFromLoudnormOutput([]byte(`noise
{
	"input_i" : "-18.42",
	"input_tp" : "-1.63",
	"input_lra" : "7.20"
}`))
	if err != nil {
		t.Fatalf("parse loudnorm: %v", err)
	}
	if normalization.IntegratedLUFS != -18.42 || normalization.TrackPeak != -1.63 {
		t.Fatalf("normalization = %+v", normalization)
	}
	if _, err := audioNormalizationFromLoudnormOutput([]byte(`{"input_tp":"-1.0"}`)); err == nil {
		t.Fatalf("expected missing input_i error")
	}
}

func TestExtractEmbeddedCoverImageCreatesMediaImage(t *testing.T) {
	server := newScannerTestServer(t)
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg-stub")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nfor last do :; done\nprintf cover > \"$last\"\n"), 0o700); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	sourcePath := filepath.Join(t.TempDir(), "track.mp3")
	if err := os.WriteFile(sourcePath, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	item := MediaItem{ID: "track_cover_test", Type: "track"}
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at) VALUES ('lib_cover_test', 'Cover Test', 'music', 1, '/tmp/cover-test', '{}', ?)`, "2026-05-04T00:00:00Z"); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at) VALUES (?, 'lib_cover_test', 'track', 'Cover Test', 'Cover Test', '[]', ?)`, item.ID, "2026-05-04T00:00:00Z"); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	err := server.extractEmbeddedCoverImage(context.Background(), item, sourcePath, ffprobePayload{
		Streams: []ffprobeStream{{Index: 3, CodecType: "video", Disposition: map[string]int{"attached_pic": 1}}},
	})
	if err != nil {
		t.Fatalf("extract embedded cover: %v", err)
	}
	path, ok := server.mediaImagePath(item.ID, "poster")
	if !ok || !strings.HasSuffix(path, "embedded_cover.jpg") {
		t.Fatalf("embedded cover path = %q ok=%v", path, ok)
	}
}

func TestGeneratedArtworkPathRequiresExistingThumb(t *testing.T) {
	server := newScannerTestServer(t)
	if _, ok := server.generatedArtworkPath("movie", "thumb"); ok {
		t.Fatalf("expected missing generated thumbnail to be ignored")
	}
}

func TestRepresentativeThumbnailUsesMediaMidpoint(t *testing.T) {
	if got := representativeThumbnailSecond(2_400); got != 1_200 {
		t.Fatalf("representative thumbnail second = %v", got)
	}
	if got := representativeThumbnailSecond(20); got != 10 {
		t.Fatalf("short media thumbnail second = %v", got)
	}
}

func TestChaptersForMediaIncludesGeneratedThumbnailURL(t *testing.T) {
	server := newScannerTestServer(t)
	mediaID := "movie_meridian"
	thumbDir := filepath.Join(server.cfg.AppDataDir, "artwork", mediaID)
	if err := os.MkdirAll(thumbDir, 0o700); err != nil {
		t.Fatalf("create thumb dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(thumbDir, mediaID+"_chapter_1.jpg"), []byte("jpg"), 0o600); err != nil {
		t.Fatalf("write chapter thumb: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_chapters (id, media_id, title, start_seconds, end_seconds, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
		mediaID+"_chapter_1", mediaID, "Opening", 0, 60, 0); err != nil {
		t.Fatalf("insert chapter: %v", err)
	}
	chapters := server.chaptersForMedia(mediaID)
	if len(chapters) != 1 || chapters[0].ThumbURL == "" {
		t.Fatalf("chapters = %#v, expected thumbnail URL", chapters)
	}
	detail, err := server.getMediaDetail("", mediaID)
	if err != nil {
		t.Fatalf("load media detail: %v", err)
	}
	if len(detail.Chapters) != 1 || detail.Chapters[0].Title != "Opening" {
		t.Fatalf("detail chapters = %#v", detail.Chapters)
	}
}

func TestGenerateMediaTrickplayCreatesManagedTiles(t *testing.T) {
	server := newScannerTestServer(t)
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg-stub")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nfor last do :; done\ndir=$(dirname \"$last\")\nmkdir -p \"$dir\"\nprintf one > \"$(printf \"$last\" 1)\"\nprintf two > \"$(printf \"$last\" 2)\"\n"), 0o700); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	sourcePath := filepath.Join(t.TempDir(), "movie.mp4")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	item := MediaItem{ID: "movie_trickplay_test", Type: "movie", Title: "Trickplay Test"}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at) VALUES (?, 'movie', 'Trickplay Test', 'Trickplay Test', '[]', ?)`, item.ID, "2026-05-04T00:00:00Z"); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	if err := server.generateMediaTrickplay(context.Background(), item, sourcePath, 120, []Stream{{Kind: "video", Width: 1920, Height: 1080}}); err != nil {
		t.Fatalf("generate trickplay: %v", err)
	}
	sets := server.trickplaySetsForMedia(item.ID)
	if len(sets) != 1 || sets[0].TileCount != 2 || sets[0].IntervalSeconds != 10 || sets[0].Width != 160 || sets[0].Height != 90 {
		t.Fatalf("trickplay sets = %#v", sets)
	}
	var tilePath string
	if err := server.db.QueryRow(`SELECT path FROM media_trickplay_tiles WHERE set_id = ? AND tile_index = 1`, sets[0].ID).Scan(&tilePath); err != nil {
		t.Fatalf("load tile path: %v", err)
	}
	if !strings.HasPrefix(tilePath, filepath.Join(server.cfg.AppDataDir, "trickplay", safePathComponent(item.ID), safePathComponent(sets[0].ID))) {
		t.Fatalf("tile path outside trickplay root: %s", tilePath)
	}
	if bytes, err := os.ReadFile(tilePath); err != nil || string(bytes) != "two" {
		t.Fatalf("tile bytes = %q err=%v", bytes, err)
	}
}

func TestGenerateMediaTrickplayUsesLibraryTuning(t *testing.T) {
	server := newScannerTestServer(t)
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg-stub")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nfor last do :; done\ndir=$(dirname \"$last\")\nmkdir -p \"$dir\"\nprintf one > \"$(printf \"$last\" 1)\"\n"), 0o700); err != nil {
		t.Fatalf("write ffmpeg stub: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	root := t.TempDir()
	sourcePath := filepath.Join(root, "movie.mp4")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"trickplayStorageLocation": "with_media",
			"trickplayIntervalSeconds": 4,
			"trickplayTileWidth":       320,
			"trickplayMaxTiles":        50,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	item := MediaItem{ID: "movie_trickplay_tuned", Type: "movie", Title: "Trickplay Tuned", LibraryID: library.ID}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at) VALUES (?, 'movie', 'Trickplay Tuned', 'Trickplay Tuned', '[]', ?)`, item.ID, "2026-05-04T00:00:00Z"); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	if err := server.generateMediaTrickplay(context.Background(), item, sourcePath, 3600, []Stream{{Kind: "video", Width: 1920, Height: 1080}}); err != nil {
		t.Fatalf("generate trickplay: %v", err)
	}
	sets := server.trickplaySetsForMedia(item.ID)
	if len(sets) != 1 || sets[0].IntervalSeconds != 4 || sets[0].Width != 320 || sets[0].Height != 180 {
		t.Fatalf("trickplay set did not use library tuning: %#v", sets)
	}
	var tilePath string
	if err := server.db.QueryRow(`SELECT path FROM media_trickplay_tiles WHERE set_id = ? AND tile_index = 0`, sets[0].ID).Scan(&tilePath); err != nil {
		t.Fatalf("load tile path: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(tilePath), "/.portico/trickplay/") {
		t.Fatalf("expected beside-media trickplay path, got %s", tilePath)
	}
}

func TestLibraryPreviewGenerationSettings(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"generateChapterThumbnails": false,
			"generateTrickplay":         false,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	item := MediaItem{ID: "preview_settings_movie", Type: "movie", LibraryID: library.ID}
	if server.chapterThumbnailGenerationEnabled(item) {
		t.Fatalf("expected chapter thumbnail generation to be disabled")
	}
	if server.trickplayGenerationEnabled(item) {
		t.Fatalf("expected trickplay generation to be disabled")
	}
	if !server.chapterThumbnailGenerationEnabled(MediaItem{ID: "default_movie", Type: "movie"}) {
		t.Fatalf("expected chapter thumbnail generation to default on")
	}
	if !server.trickplayGenerationEnabled(MediaItem{ID: "default_movie", Type: "movie"}) {
		t.Fatalf("expected trickplay generation to default on")
	}
}

func TestMediaAnalysisOptionsHonorLibraryFeatureSwitches(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{root},
		Settings: map[string]any{
			"analysisTier":                    analysisTierCustom,
			"probeStreams":                    true,
			"generateRepresentativeThumbnail": false,
			"generateChapterThumbnails":       false,
			"generateTrickplay":               false,
			"analyzeLoudness":                 false,
			"sonicFingerprinting":             true,
			"extractAllEmbeddedAttachments":   false,
		},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	item := MediaItem{ID: "movie_analysis_options", Type: "movie", LibraryID: library.ID}

	full := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	if !full.ProbeStreams {
		t.Fatalf("expected stream probing to remain enabled")
	}
	if full.GenerateThumbnails || full.ChapterThumbnails || full.GenerateTrickplay || full.AnalyzeAudio || full.DetectSegments || full.ExtractEmbeddedCovers || full.ExtractEmbeddedAttachments {
		t.Fatalf("full analysis ignored disabled library switches: %#v", full)
	}
	if !full.SonicFingerprinting {
		t.Fatalf("explicit sonic fingerprinting switch should be preserved for full analysis")
	}

	probe := server.mediaAnalysisOptions(item, mediaAnalysisModeProbe)
	if !probe.ProbeStreams {
		t.Fatalf("expected probe mode to keep stream probing enabled")
	}
	if probe.GenerateThumbnails || probe.ChapterThumbnails || probe.GenerateTrickplay || probe.AnalyzeAudio || probe.SonicFingerprinting || probe.FullFileChecksum || probe.GenerateWaveforms || probe.ExtractEmbeddedCovers || probe.ExtractEmbeddedAttachments {
		t.Fatalf("probe analysis should not enable heavy stages: %#v", probe)
	}
	if probe.DetectSegments {
		t.Fatalf("probe analysis should honor disabled chapter segment detection")
	}
}

func TestCustomSelectedEmbeddedAssetUsesBoundedProbeWithoutFullAnalysis(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{
		Name: "Selected Cover", Type: "music", Paths: []string{t.TempDir()},
		Settings: map[string]any{
			"analysisTier":                  analysisTierCustom,
			"probeStreams":                  true,
			"readEmbeddedIndexes":           true,
			"extractSelectedEmbeddedAssets": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := MediaItem{ID: "track_selected_cover", Type: "track", LibraryID: library.ID}
	probe := server.mediaAnalysisOptions(item, mediaAnalysisModeProbe)
	if !probe.ExtractEmbeddedCovers || !probe.ReadEmbeddedIndexes {
		t.Fatalf("selected embedded asset did not compile into the bounded probe: %#v", probe)
	}
	if server.analysisTierWantsFull(item) {
		t.Fatal("selected embedded cover alone incorrectly authorized a full-file analysis pass")
	}
}

func TestFixedAnalysisTiersIgnoreCustomOperationDefaults(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	completeLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name: "Complete", Type: "movie", Paths: []string{root}, Settings: map[string]any{"analysisTier": analysisTierComplete},
	})
	if err != nil {
		t.Fatal(err)
	}
	complete := server.mediaAnalysisOptions(MediaItem{ID: "complete", Type: "movie", LibraryID: completeLibrary.ID}, mediaAnalysisModeFull)
	if !complete.ProbeStreams || !complete.ReadEmbeddedTags || !complete.ReadEmbeddedIndexes || !complete.GenerateThumbnails || !complete.ChapterThumbnails || !complete.GenerateTrickplay || !complete.AnalyzeAudio || !complete.SonicFingerprinting || !complete.FullFileChecksum || !complete.GenerateWaveforms || !complete.ExtractEmbeddedAttachments || !complete.ExtractEmbeddedCovers {
		t.Fatalf("Complete omitted fixed deep-analysis work because Custom defaults are false: %#v", complete)
	}
	basicLibrary, err := server.createLibrary(CreateLibraryRequest{
		Name: "Basic", Type: "movie", Paths: []string{root}, Settings: map[string]any{"analysisTier": analysisTierBasic},
	})
	if err != nil {
		t.Fatal(err)
	}
	basic := server.mediaAnalysisOptions(MediaItem{ID: "basic", Type: "movie", LibraryID: basicLibrary.ID}, mediaAnalysisModeFull)
	if !basic.ProbeStreams || !basic.ReadEmbeddedTags || !basic.ReadEmbeddedIndexes {
		t.Fatalf("Basic omitted bounded technical facts: %#v", basic)
	}
	if basic.GenerateThumbnails || basic.ChapterThumbnails || basic.GenerateTrickplay || basic.AnalyzeAudio || basic.SonicFingerprinting || basic.FullFileChecksum || basic.GenerateWaveforms || basic.ExtractEmbeddedAttachments || basic.ExtractEmbeddedCovers {
		t.Fatalf("Basic authorized Complete-only work: %#v", basic)
	}
}

func TestMediaAnalysisQueueCanBeDisabledPerLibrary(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:     "Movies",
		Type:     "movie",
		Paths:    []string{root},
		Settings: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": false},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	item := MediaItem{ID: "movie_analysis_disabled", Type: "movie", LibraryID: library.ID}

	if server.mediaAnalysisQueueEnabled(item) {
		t.Fatalf("expected probeStreams=false to disable queued stream analysis")
	}
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	if options.ProbeStreams || options.GenerateThumbnails || options.ChapterThumbnails || options.GenerateTrickplay || options.AnalyzeAudio || options.FullFileChecksum || options.GenerateWaveforms || options.ValidateSeekBehavior || options.DetectSegments || options.ExtractEmbeddedCovers || options.ExtractEmbeddedAttachments {
		t.Fatalf("probeStreams=false should disable all analysis work: %#v", options)
	}
}

func TestMediaSegmentsAreNeverGuessedFromChapterTitles(t *testing.T) {
	chapters := []Chapter{
		{ID: "ch_1", Title: "Previously On", StartSeconds: 0, EndSeconds: 42},
		{ID: "ch_2", Title: "Opening Credits", StartSeconds: 42, EndSeconds: 92},
		{ID: "ch_3", Title: "Main Feature", StartSeconds: 92, EndSeconds: 1200},
		{ID: "ch_4", Title: "Commercial Break", StartSeconds: 1200, EndSeconds: 1320},
		{ID: "ch_5", Title: "End Credits", StartSeconds: 3500, EndSeconds: 3600},
	}
	segments := mediaSegmentsFromChapters("episode_chapter_segments", chapters, 3600)
	if len(segments) != 0 {
		t.Fatalf("chapter titles are presentation metadata, not verified skip markers: %#v", segments)
	}
}

func TestPruneTrickplaySetsEnforcesStorageCap(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, genres_json, added_at) VALUES ('trickplay_cap_movie', 'movie', 'Trickplay Cap Movie', 'Trickplay Cap Movie', '[]', ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	oldDir := filepath.Join(server.cfg.AppDataDir, "trickplay", safePathComponent("trickplay_cap_movie"), "old_set")
	newDir := filepath.Join(server.cfg.AppDataDir, "trickplay", safePathComponent("trickplay_cap_movie"), "new_set")
	for _, dir := range []string{oldDir, newDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create trickplay dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tile_00000.jpg"), bytes.Repeat([]byte("x"), 800*1024), 0o600); err != nil {
			t.Fatalf("write trickplay tile: %v", err)
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_trickplay_sets (
			id, media_id, media_file_id, width, height, tile_width, tile_height,
			interval_seconds, duration_seconds, tile_count, path, stale, created_at
		) VALUES
			('old_set', 'trickplay_cap_movie', 'old_file', 160, 90, 160, 90, 10, 120, 1, ?, 0, ?),
			('new_set', 'trickplay_cap_movie', 'new_file', 160, 90, 160, 90, 10, 120, 1, ?, 0, ?)`,
		oldDir, now.Add(-2*time.Hour).Format(time.RFC3339), newDir, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert trickplay sets: %v", err)
	}
	removed, err := server.pruneTrickplaySets(30, 1)
	if err != nil {
		t.Fatalf("prune trickplay sets: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, expected 1", removed)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old trickplay dir to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("expected new trickplay dir to remain: %v", err)
	}
}

func TestLocalArtworkPathUsesFilesInsideLibraryRoot(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mp4")
	posterPath := filepath.Join(root, "poster.jpg")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	if err := os.WriteFile(posterPath, []byte("image"), 0o600); err != nil {
		t.Fatalf("write poster: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	path, ok := server.localArtworkPath(MediaItem{ID: "movie", LibraryID: library.ID, SourceURL: mediaPath}, "poster")
	if !ok {
		t.Fatalf("expected local poster to be discovered")
	}
	realPoster, err := filepath.EvalSymlinks(posterPath)
	if err != nil {
		t.Fatalf("resolve poster: %v", err)
	}
	if path != realPoster {
		t.Fatalf("poster path = %s, expected %s", path, realPoster)
	}
}

func TestLibraryArtworkPolicyOverridesServerDefault(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{
		Name:     "Movies",
		Type:     "movie",
		Paths:    []string{root},
		Settings: map[string]any{"imagePolicy": "provider_only"},
	})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	policy := server.artworkPolicyForItem(MediaItem{ID: "movie", LibraryID: library.ID})
	if policy != "provider_only" {
		t.Fatalf("artwork policy = %q", policy)
	}
	order := artworkSourceOrder(policy)
	if len(order) != 1 || order[0] != "provider" {
		t.Fatalf("artwork source order = %#v", order)
	}
}
