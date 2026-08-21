package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecideMediaPlaybackRequiresTranscodeForUnsupportedCodecs(t *testing.T) {
	item := MediaItem{
		ID: "movie_4k",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "hevc", Width: 3840, Height: 2160, Bitrate: 18_000},
			{ID: "a1", Kind: "audio", Codec: "truehd", Bitrate: 4_000},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mp4", PlaybackClientProfile{
		SupportedContainers:  []string{"mp4"},
		SupportedVideoCodecs: []string{"h264"},
		SupportedAudioCodecs: []string{"aac"},
		MaxWidth:             1920,
		MaxHeight:            1080,
	})
	if !decision.RequiresTranscode || decision.Mode != "transcode_required" {
		t.Fatalf("expected transcode requirement, got %+v", decision)
	}
	if !decision.IsProxied {
		t.Fatalf("expected media playback to remain server proxied")
	}
}

func TestDecideMediaPlaybackSeparatesContainerRemuxFromCodecTranscode(t *testing.T) {
	item := MediaItem{
		ID: "movie_mkv",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 8_000},
			{ID: "a1", Kind: "audio", Codec: "eac3", Channels: 6, Bitrate: 640},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mkv", PlaybackClientProfile{
		SupportedContainers:  []string{"mp4", "hls"},
		SupportedVideoCodecs: []string{"h264", "hevc"},
		SupportedAudioCodecs: []string{"aac", "ac3", "eac3"},
		MaxWidth:             3840,
		MaxHeight:            2160,
		MaxAudioChannels:     8,
	})
	if !decision.RequiresTranscode || decision.VideoTranscode || decision.AudioTranscode || !strings.Contains(decision.Reason, "container") {
		t.Fatalf("expected only the container to require server handling, got %+v", decision)
	}
	next := newScannerTestServer(t).applyDirectStreamRemuxPolicy(decision, item, PlaybackClientProfile{SupportsHLS: true})
	if next.Mode != "direct_stream" || next.RequiresTranscode || !next.RequiresRemux {
		t.Fatalf("expected direct stream remux without codec transcode, got %+v", next)
	}
}

func TestWebProfileAudioChannelLimitUsesAudioOnlyTranscode(t *testing.T) {
	item := MediaItem{
		ID: "movie_mkv",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 8_000},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 6, Bitrate: 640},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mkv", PlaybackClientProfile{
		SupportedContainers:  []string{"mp4", "hls"},
		SupportedVideoCodecs: []string{"h264"},
		SupportedAudioCodecs: []string{"aac", "mp3"},
		MaxWidth:             3840,
		MaxHeight:            2160,
		MaxAudioChannels:     2,
	})
	if !decision.RequiresTranscode || !decision.AudioTranscode || !strings.Contains(decision.Reason, "audio channels") {
		t.Fatalf("expected audio channel transcode requirement, got %+v", decision)
	}
	next := newScannerTestServer(t).applyDirectStreamRemuxPolicy(decision, item, PlaybackClientProfile{SupportsHLS: true})
	if next.Mode != "direct_stream" || !next.RequiresTranscode || !next.RequiresRemux || !next.AudioTranscode || next.VideoTranscode {
		t.Fatalf("expected video copy with audio-only transcode when audio channels exceed profile, got %+v", next)
	}
	playbackURL := mediaPlaybackHLSURLWithOptions(item.ID, "original", "", transcodeAudioModeForDecision(next), "", next.Mode == "direct_stream")
	if !strings.Contains(playbackURL, "audio=transcode") || !strings.Contains(playbackURL, "directStream=1") {
		t.Fatalf("expected HLS URL to preserve direct-stream audio-only transcode mode, got %q", playbackURL)
	}
}

func TestMSEBrowserDoesNotDirectStreamHEVCOverHLS(t *testing.T) {
	server := newScannerTestServer(t)
	decision := PlaybackDecision{
		Mode:              "transcode_required",
		Reason:            "container is not in the client profile",
		RequiresTranscode: true,
	}
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "hevc", Width: 1920, Height: 1080}, {Kind: "audio", Codec: "aac", Channels: 2}}}
	profile := PlaybackClientProfile{
		Device:               "Mozilla/5.0 Chrome/148.0.0.0 Safari/537.36",
		Platform:             "MacIntel",
		SupportsHLS:          true,
		SupportsMSE:          true,
		SupportedVideoCodecs: []string{"h264", "hevc"},
		SupportedAudioCodecs: []string{"aac"},
	}
	next := server.applyDirectStreamRemuxPolicy(decision, item, profile)
	if next.Mode == "direct_stream" || !next.VideoTranscode || !strings.Contains(next.Reason, "client HLS path") {
		t.Fatalf("expected HEVC MSE/HLS path to force full video transcode, got %+v", next)
	}
}

func TestHEVCMP4DirectPlaysWhenNativeClientReportsSupport(t *testing.T) {
	item := MediaItem{
		ID: "movie_hevc_mp4",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "hevc", Width: 1920, Height: 1080, Bitrate: 2_000_000},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 6, Bitrate: 224_000},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mp4", PlaybackClientProfile{
		Device:               "Portico Native",
		Platform:             "tvos",
		SupportsHLS:          true,
		SupportedContainers:  []string{"mp4", "m4v", "mov", "hls"},
		SupportedVideoCodecs: []string{"h264", "hevc"},
		SupportedAudioCodecs: []string{"aac", "mp3"},
		MaxWidth:             3840,
		MaxHeight:            2160,
	})
	if decision.Mode != "direct_play" || decision.RequiresTranscode {
		t.Fatalf("expected supported HEVC MP4 native playback to direct play, got %+v", decision)
	}
}

func TestAppleClientProfileRejectsUnsupportedVideoCharacteristics(t *testing.T) {
	profile := PlaybackClientProfile{
		Device:                       "Portico Apple",
		Platform:                     "tvOS",
		SupportsHLS:                  true,
		SupportsHEVC:                 true,
		SupportsHDR:                  true,
		SupportedContainers:          []string{"mp4", "hls"},
		SupportedVideoCodecs:         []string{"h264", "hevc"},
		SupportedAudioCodecs:         []string{"aac"},
		SupportedVideoProfiles:       []string{"h264:baseline", "h264:main", "h264:high", "hevc:main", "hevc:main 10"},
		SupportedPixelFormats:        []string{"yuv420p", "yuvj420p", "yuv420p10le", "p010le"},
		SupportedHDRFormats:          []string{"hdr10", "hlg"},
		SupportedDolbyVisionProfiles: []string{"5", "8"},
		MaxVideoBitDepth:             10,
		MaxWidth:                     3840,
		MaxHeight:                    2160,
	}
	cases := []struct {
		name   string
		video  Stream
		reason string
	}{
		{
			name:   "h264 high 10",
			video:  Stream{ID: "v1", Kind: "video", Codec: "h264", Profile: "High 10", PixelFormat: "yuv420p10le", BitDepth: 10, Width: 1920, Height: 1080},
			reason: "video profile",
		},
		{
			name:   "422 pixel format",
			video:  Stream{ID: "v1", Kind: "video", Codec: "hevc", Profile: "Main 10", PixelFormat: "yuv422p10le", BitDepth: 10, Width: 1920, Height: 1080},
			reason: "pixel format",
		},
		{
			name:   "12 bit",
			video:  Stream{ID: "v1", Kind: "video", Codec: "hevc", Profile: "Main 10", PixelFormat: "yuv420p12le", BitDepth: 12, Width: 1920, Height: 1080},
			reason: "bit depth",
		},
		{
			name:   "dolby vision profile 7",
			video:  Stream{ID: "v1", Kind: "video", Codec: "hevc", Profile: "Main 10", PixelFormat: "yuv420p10le", BitDepth: 10, DynamicRange: "dolby_vision_profile_7", DolbyVisionProfile: "7", Width: 1920, Height: 1080},
			reason: "HDR format",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideMediaPlayback(MediaItem{ID: "movie", Streams: []Stream{
				tc.video,
				{ID: "a1", Kind: "audio", Codec: "aac", Channels: 2},
			}}, "https://media.example.com/movie.mp4", profile)
			if decision.Mode != "transcode_required" || !decision.VideoTranscode || !strings.Contains(decision.Reason, tc.reason) {
				t.Fatalf("expected %s to require video transcode, got %+v", tc.name, decision)
			}
		})
	}
}

func TestAppleClientProfileAllowsHDR10Main10WithinLimits(t *testing.T) {
	item := MediaItem{
		ID: "movie_hdr10",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "hevc", Profile: "Main 10", PixelFormat: "yuv420p10le", BitDepth: 10, DynamicRange: "hdr10", Width: 3840, Height: 2160},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mp4", PlaybackClientProfile{
		Device:                 "Portico Apple",
		Platform:               "tvOS",
		SupportsHEVC:           true,
		SupportsHDR:            true,
		SupportedContainers:    []string{"mp4", "hls"},
		SupportedVideoCodecs:   []string{"h264", "hevc"},
		SupportedAudioCodecs:   []string{"aac"},
		SupportedVideoProfiles: []string{"hevc:main", "hevc:main 10"},
		SupportedPixelFormats:  []string{"yuv420p", "yuv420p10le", "p010le"},
		SupportedHDRFormats:    []string{"hdr10", "hlg"},
		MaxVideoBitDepth:       10,
		MaxWidth:               3840,
		MaxHeight:              2160,
	})
	if decision.Mode != "direct_play" || decision.RequiresTranscode {
		t.Fatalf("expected HDR10 Main10 within profile to direct play, got %+v", decision)
	}
}

func TestAppleClientProfileRejectsHDRWhenClientOptsOut(t *testing.T) {
	item := MediaItem{
		ID: "movie_hdr10",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "hevc", Profile: "Main 10", PixelFormat: "yuv420p10le", BitDepth: 10, DynamicRange: "hdr10", Width: 1920, Height: 1080},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mp4", PlaybackClientProfile{
		Device:                 "Portico Apple",
		Platform:               "tvOS",
		SupportsHEVC:           true,
		SupportsHDR:            false,
		SupportedContainers:    []string{"mp4", "hls"},
		SupportedVideoCodecs:   []string{"h264", "hevc"},
		SupportedAudioCodecs:   []string{"aac"},
		SupportedVideoProfiles: []string{"hevc:main", "hevc:main 10"},
		SupportedPixelFormats:  []string{"yuv420p", "yuv420p10le", "p010le"},
		MaxVideoBitDepth:       10,
	})
	if decision.Mode != "transcode_required" || !decision.VideoTranscode || !strings.Contains(decision.Reason, "HDR format") {
		t.Fatalf("expected HDR source to transcode when client opts out, got %+v", decision)
	}
}

func TestPlaybackDecisionReportsHDRToneMappingRuntimeLimitation(t *testing.T) {
	server := newScannerTestServer(t)
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\ncase \"$arg\" in\n-version) echo 'ffmpeg test build'; exit 0;;\n-filters) echo ' ... scale test filter'; exit 0;;\nesac\ndone\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	if _, err := server.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('transcoder', ?, ?)`, `{
		"enabled": true,
		"hdrToneMapping": true
	}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("save transcode settings: %v", err)
	}

	item := MediaItem{
		ID: "movie_hdr_transcode",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "hevc", DisplayTitle: "HEVC HDR10", Width: 3840, Height: 2160},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mp4", PlaybackClientProfile{
		SupportedContainers:  []string{"mp4", "hls"},
		SupportedVideoCodecs: []string{"h264"},
		SupportedAudioCodecs: []string{"aac"},
	})
	decision = server.applyTranscodeCapabilityNotes(decision, item)
	if !decision.VideoTranscode || !strings.Contains(decision.Reason, "zscale/tonemap filters are unavailable") {
		t.Fatalf("expected HDR tone-map limitation in playback reason, got %+v", decision)
	}
}

func TestHEVCMP4WebBrowserRequiresVideoTranscode(t *testing.T) {
	item := MediaItem{
		ID: "movie_hevc_mp4",
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "hevc", Width: 1920, Height: 1080, Bitrate: 2_000_000},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 6, Bitrate: 224_000},
		},
	}
	decision := decideMediaPlayback(item, "https://media.example.com/movie.mp4", PlaybackClientProfile{
		Device:               "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
		Platform:             "MacIntel",
		SupportsHLS:          true,
		SupportsMSE:          true,
		SupportsHEVC:         true,
		SupportedContainers:  []string{"mp4", "m4v", "mov", "hls"},
		SupportedVideoCodecs: []string{"h264", "hevc"},
		SupportedAudioCodecs: []string{"aac", "mp3"},
		MaxWidth:             3840,
		MaxHeight:            2160,
	})
	if decision.Mode != "transcode_required" || !decision.RequiresTranscode || !decision.VideoTranscode {
		t.Fatalf("expected browser HEVC MP4 playback to require video transcode, got %+v", decision)
	}
	if !strings.Contains(decision.Reason, "browser HEVC") {
		t.Fatalf("expected browser HEVC reason, got %q", decision.Reason)
	}
}

func TestDecideLiveTVPlaybackMarksHLSAsCachedProxy(t *testing.T) {
	decision := decideLiveTVPlayback("https://provider.example.com/live/master.m3u8", "hls", liveTVSourceRecord{
		LiveTVSource: LiveTVSource{StreamBufferSeconds: 12},
	}, PlaybackClientProfile{SupportsHLS: true})
	if decision.RequiresTranscode || decision.Mode != "direct_play" {
		t.Fatalf("expected direct HLS playback, got %+v", decision)
	}
	if !decision.IsProxied || !decision.IsServerCached || decision.BufferSeconds != 12 {
		t.Fatalf("expected cached proxy decision, got %+v", decision)
	}
}

func TestNonHLSLiveTVUsesServerTranscodeHLSDelivery(t *testing.T) {
	decision := decideLiveTVHLSDelivery("https://provider.example.com/live/channel.ts", liveTVSourceRecord{
		LiveTVSource: LiveTVSource{StreamBufferSeconds: 8},
	}, PlaybackClientProfile{SupportsHLS: true})
	if !decision.RequiresTranscode || !decision.VideoTranscode || !decision.AudioTranscode || decision.Mode != "transcode_required" {
		t.Fatalf("non-HLS provider did not use transcode delivery: %+v", decision)
	}
	if decision.Protocol != "hls" || decision.Container != "mpegts" || !decision.IsProxied || !decision.IsServerCached {
		t.Fatalf("non-HLS provider did not resolve to server HLS: %+v", decision)
	}
}

func TestPlaybackStartAcceptsCurrentAppleClientProfile(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	user.LibraryIDs = append(user.LibraryIDs, "lib_music_handoff")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_playback', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_playback_profile', 'lib_playback', 'movie', 'Movie', 'Movie', ?, 'https://media.example.com/Movie.mp4', 120)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (
			id, media_id, kind, codec, channels, bitrate, width, height, display_title,
			profile, pixel_format, bit_depth, frame_rate, channel_layout, sample_rate, stream_index
		) VALUES
			('movie_playback_profile_video', 'movie_playback_profile', 'video', 'h264', 0, 4000000, 1920, 1080, 'H264 Main - 1080p', 'main', 'yuv420p', 8, 24, '', 0, 0),
			('movie_playback_profile_audio', 'movie_playback_profile', 'audio', 'aac', 2, 160000, 0, 0, 'AAC LC - Stereo', 'lc', '', 0, 0, 'stereo', 48000, 1)`); err != nil {
		t.Fatalf("insert exact playback facts: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "movie_playback_profile",
		ClientProfile: PlaybackClientProfile{
			Device:               "Apple",
			Platform:             "tvOS",
			SupportsHLS:          true,
			SupportsMPEGTS:       true,
			SupportedContainers:  []string{"mp4", "mov", "m4v", "mpegts", "hls"},
			SupportedVideoCodecs: []string{"h264", "hevc"},
			SupportedAudioCodecs: []string{"aac", "ac3", "eac3", "mp3", "alac"},
			MaxWidth:             3840,
			MaxHeight:            2160,
			SupportsHEVC:         true,
			SupportsHDR:          true,
			SupportsEAC3:         true,
			SupportsAC3:          true,
			PrefersServerProxy:   true,
		},
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code == http.StatusBadRequest && strings.Contains(rec.Body.String(), "bad_json") {
		t.Fatalf("current Apple playback profile was rejected as bad JSON: %s", rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlaybackStartReportsMissingLocalMediaFile(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	mediaRoot := t.TempDir()
	missingPath := filepath.Join(mediaRoot, "Missing Movie.mp4")
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_missing_playback', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('lib_missing_playback_path', 'lib_missing_playback', ?, 0, ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert library path: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('movie_missing_playback', 'lib_missing_playback', 'movie', 'Missing Movie', 'Missing Movie', ?, ?, 120)`, now, missingPath); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{MediaID: "movie_missing_playback", ClientProfile: PlaybackClientProfile{Device: "Apple", Platform: "tvOS", SupportsHLS: true}})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "media_file_missing") || !strings.Contains(body, "This file is missing on the server") || strings.Contains(body, missingPath) {
		t.Fatalf("expected friendly missing-file error without leaking path, body=%s", body)
	}
}

func TestAlbumPlaybackQueueOrderingSkipsCurrentTrack(t *testing.T) {
	album := MediaItem{
		ID:    "album_queue_order",
		Type:  "album",
		Title: "Album Queue Order",
		Children: []MediaItem{
			{ID: "track_queue_01", Type: "track", Title: "One", IndexNumber: 1},
			{ID: "track_queue_02", Type: "track", Title: "Two", IndexNumber: 2},
			{ID: "track_queue_03", Type: "track", Title: "Three", IndexNumber: 3},
		},
	}
	queue := newScannerTestServer(t).playbackQueue("", album, album.Children[0], nil)
	if got := mediaIDsForTest(queue); strings.Join(got, ",") != "track_queue_02,track_queue_03" {
		t.Fatalf("album playback queue = %v, expected ordered tracks after current", got)
	}
}

func TestPlaybackStartFromAlbumUsesBoundedDescendantQueue(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_album_start_queue', 'Album Start Queue', 'music', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES ('album_start_queue', 'lib_album_start_queue', 'album', 'Start Album', 'Start Album', ?)`, now); err != nil {
		t.Fatalf("insert album: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at, source_url, duration_seconds, index_number)
		VALUES
			('track_start_queue_01', 'lib_album_start_queue', 'album_start_queue', 'track', 'One', 'One', ?, 'https://media.example.com/one.mp3', 120, 1),
			('track_start_queue_02', 'lib_album_start_queue', 'album_start_queue', 'track', 'Two', 'Two', ?, 'https://media.example.com/two.mp3', 120, 2),
			('track_start_queue_03', 'lib_album_start_queue', 'album_start_queue', 'track', 'Three', 'Three', ?, 'https://media.example.com/three.mp3', 120, 3)`,
		now, now, now); err != nil {
		t.Fatalf("insert tracks: %v", err)
	}

	playback := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{MediaID: "album_start_queue", SkipPreroll: true})
	if playback.Media.ID != "track_start_queue_01" {
		t.Fatalf("playback media = %q, expected first track", playback.Media.ID)
	}
	if got := mediaIDsForTest(playback.Queue); strings.Join(got, ",") != "track_start_queue_02,track_start_queue_03" {
		t.Fatalf("album start queue = %v, expected remaining tracks", got)
	}
}

func TestPlaybackStartUsesSourceContextOrderingWhenQueueOmitsCurrent(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_middle_track_queue', 'Middle Track Queue', 'music', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at)
		VALUES ('album_middle_track_queue', 'lib_middle_track_queue', 'album', 'Ordered Album', 'Ordered Album', ?)`, now); err != nil {
		t.Fatalf("insert album: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at, source_url, duration_seconds, index_number)
		VALUES
			('track_middle_01', 'lib_middle_track_queue', 'album_middle_track_queue', 'track', 'One', 'One', ?, 'https://media.example.com/one.mp3', 120, 1),
			('track_middle_02', 'lib_middle_track_queue', 'album_middle_track_queue', 'track', 'Two', 'Two', ?, 'https://media.example.com/two.mp3', 120, 2),
			('track_middle_03', 'lib_middle_track_queue', 'album_middle_track_queue', 'track', 'Three', 'Three', ?, 'https://media.example.com/three.mp3', 120, 3),
			('track_middle_04', 'lib_middle_track_queue', 'album_middle_track_queue', 'track', 'Four', 'Four', ?, 'https://media.example.com/four.mp3', 120, 4)`,
		now, now, now, now); err != nil {
		t.Fatalf("insert tracks: %v", err)
	}
	sourceIDs := []string{"track_middle_01", "track_middle_02", "track_middle_03", "track_middle_04"}
	playback := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID:       "track_middle_03",
		QueueMediaIDs: []string{"track_middle_01", "track_middle_02", "track_middle_04"},
		SourceContext: PlaybackSourceContext{Type: "album", ID: "album_middle_track_queue", Title: "Ordered Album", MediaIDs: sourceIDs},
	})
	if got := mediaIDsForTest(playback.Queue); strings.Join(got, ",") != "track_middle_04" {
		t.Fatalf("middle-track queue = %v, expected only tracks after current", got)
	}
	if got := strings.Join(playback.SourceContext.MediaIDs, ","); got != strings.Join(sourceIDs, ",") {
		t.Fatalf("source context media ids = %q, expected ordered album context", got)
	}
}

func TestInstantMixQueueOrderingFollowsRankThenAlbumOrder(t *testing.T) {
	seed := MediaItem{ID: "track_seed", Type: "track", ParentTitle: "Seed Album", Genres: []string{"Electronic"}, TypedMetadata: map[string]string{"trackArtist": "Mara Vale", "albumTitle": "Seed Album"}}
	candidates := []MediaItem{
		{ID: "track_other_artist", Type: "track", ParentTitle: "Other Album", IndexNumber: 1, Genres: []string{"Electronic"}, TypedMetadata: map[string]string{"trackArtist": "Other Artist", "albumTitle": "Other Album"}},
		{ID: "track_seed_album_02", Type: "track", ParentTitle: "Seed Album", IndexNumber: 2, Genres: []string{"Electronic"}, TypedMetadata: map[string]string{"trackArtist": "Mara Vale", "albumTitle": "Seed Album"}},
		{ID: "track_seed_album_01", Type: "track", ParentTitle: "Seed Album", IndexNumber: 1, Genres: []string{"Electronic"}, TypedMetadata: map[string]string{"trackArtist": "Mara Vale", "albumTitle": "Seed Album"}},
		{ID: "track_artist_album_a", Type: "track", ParentTitle: "Another Album", IndexNumber: 1, Genres: []string{"Electronic"}, TypedMetadata: map[string]string{"trackArtist": "Mara Vale", "albumTitle": "Another Album"}},
	}
	got := mediaIDsForTest(instantMixRankedTracks(candidates, []MediaItem{seed}, 10))
	want := []string{"track_seed_album_01", "track_seed_album_02", "track_artist_album_a", "track_other_artist"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("instant mix order = %v, want %v", got, want)
	}
}

func TestMusicHandoffCarriesQueueHistoryToNewSession(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_music_handoff', 'Music', 'music', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, typed_metadata_json)
		VALUES ('album_handoff', 'lib_music_handoff', 'album', 'Album', 'Album', ?, '{}')`, now); err != nil {
		t.Fatalf("insert album: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, added_at, source_url, duration_seconds, typed_metadata_json)
		VALUES
			('track_handoff_a', 'lib_music_handoff', 'album_handoff', 'track', 'Track A', 'Track A', ?, 'https://media.example.com/a.mp3', 120, '{"trackArtist":"Artist","albumTitle":"Album","trackNumber":1}'),
			('track_handoff_b', 'lib_music_handoff', 'album_handoff', 'track', 'Track B', 'Track B', ?, 'https://media.example.com/b.mp3', 130, '{"trackArtist":"Artist","albumTitle":"Album","trackNumber":2}'),
			('track_handoff_c', 'lib_music_handoff', 'album_handoff', 'track', 'Track C', 'Track C', ?, 'https://media.example.com/c.mp3', 140, '{"trackArtist":"Artist","albumTitle":"Album","trackNumber":3}')`,
		now, now, now); err != nil {
		t.Fatalf("insert tracks: %v", err)
	}
	seedExactPlaybackFactsForFixture(t, server, "track_handoff_a")
	seedExactPlaybackFactsForFixture(t, server, "track_handoff_b")
	seedExactPlaybackFactsForFixture(t, server, "track_handoff_c")

	sourceContext := PlaybackSourceContext{Type: "instant_mix", ID: "track_handoff_a", Title: "Instant Mix: Track A", MediaIDs: []string{"track_handoff_a", "track_handoff_b", "track_handoff_c"}}
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{MediaID: "track_handoff_a", QueueMediaIDs: []string{"track_handoff_b", "track_handoff_c"}, RepeatMode: "all", SourceContext: sourceContext})
	if len(started.Queue) != 2 || started.Queue[0].ID != "track_handoff_b" {
		t.Fatalf("start queue = %#v", started.Queue)
	}
	if started.RepeatMode != "all" || started.QueueRevision != 0 {
		t.Fatalf("start queue state = repeat %q revision %d", started.RepeatMode, started.QueueRevision)
	}
	if started.SourceContext.Type != "instant_mix" || started.SourceContext.Title != "Instant Mix: Track A" || len(started.SourceContext.MediaIDs) != 3 {
		t.Fatalf("start source context = %#v", started.SourceContext)
	}

	prepareBody, err := json.Marshal(PlaybackPrepareNextRequest{MediaID: "track_handoff_b", QueueMediaIDs: []string{"track_handoff_c"}, PreferredHandoff: "gapless", SourceContext: sourceContext})
	if err != nil {
		t.Fatalf("marshal prepare request: %v", err)
	}
	prepareReq := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+started.SessionID+"/prepare-next", bytes.NewReader(prepareBody))
	prepareRec := httptest.NewRecorder()
	server.handlePlaybackPrepareNext(prepareRec, prepareReq, user, started.SessionID)
	if prepareRec.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", prepareRec.Code, prepareRec.Body.String())
	}
	var prepared PlaybackPreparedResponse
	if err := json.Unmarshal(prepareRec.Body.Bytes(), &prepared); err != nil {
		t.Fatalf("decode prepare response: %v", err)
	}
	if prepared.PreparedSessionID == "" || prepared.Playback.Media.ID != "track_handoff_b" || prepared.ExpiresAt == "" {
		t.Fatalf("prepared response = %#v", prepared)
	}

	handoffBody, err := json.Marshal(PlaybackHandoffRequest{PreparedSessionID: prepared.PreparedSessionID, MediaID: "track_handoff_b", QueueMediaIDs: []string{"track_handoff_c"}, ProgressSeconds: 118, Intent: PlaybackIntent{QualityProfile: "standard", PreferredSubtitleMode: "off"}})
	if err != nil {
		t.Fatalf("marshal handoff request: %v", err)
	}
	handoffReq := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+started.SessionID+"/handoff", bytes.NewReader(handoffBody))
	handoffRec := httptest.NewRecorder()
	server.handlePlaybackHandoff(handoffRec, handoffReq, user, started.SessionID)
	if handoffRec.Code != http.StatusOK {
		t.Fatalf("handoff status=%d body=%s", handoffRec.Code, handoffRec.Body.String())
	}
	var handedOff PlaybackResponse
	if err := json.Unmarshal(handoffRec.Body.Bytes(), &handedOff); err != nil {
		t.Fatalf("decode handoff response: %v", err)
	}
	if handedOff.Media.ID != "track_handoff_b" {
		t.Fatalf("handoff media = %q", handedOff.Media.ID)
	}
	retryReq := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+started.SessionID+"/handoff", bytes.NewReader(handoffBody))
	retryRec := httptest.NewRecorder()
	server.handlePlaybackHandoff(retryRec, retryReq, user, started.SessionID)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("idempotent handoff retry status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
	var retried PlaybackResponse
	if err := json.Unmarshal(retryRec.Body.Bytes(), &retried); err != nil {
		t.Fatalf("decode retried handoff: %v", err)
	}
	if retried.SessionID != handedOff.SessionID || retried.MediaGrant.Token != handedOff.MediaGrant.Token || retried.ContinuationCredential == nil || handedOff.ContinuationCredential == nil || retried.ContinuationCredential.Token != handedOff.ContinuationCredential.Token {
		t.Fatalf("idempotent handoff did not recover exact response: first=%#v retry=%#v", handedOff, retried)
	}
	if handedOff.Policy.QualityProfile != "standard" {
		t.Fatalf("handoff dropped portable playback intent: policy=%#v", handedOff.Policy)
	}
	if handedOff.RepeatMode != "all" || handedOff.QueueRevision != 0 {
		t.Fatalf("handoff queue state = repeat %q revision %d", handedOff.RepeatMode, handedOff.QueueRevision)
	}
	if handedOff.SourceContext.Type != "instant_mix" || handedOff.SourceContext.Title != "Instant Mix: Track A" || len(handedOff.SourceContext.MediaIDs) != 3 {
		t.Fatalf("handoff source context = %#v", handedOff.SourceContext)
	}
	persistedContext := server.playbackSessionSourceContext(handedOff.SessionID)
	if persistedContext.Type != "instant_mix" || persistedContext.ID != "track_handoff_a" {
		t.Fatalf("persisted source context = %#v", persistedContext)
	}
	history := server.loadPlaybackSessionHistory(user.ID, handedOff.SessionID)
	if len(history) != 1 || history[0].ID != "track_handoff_a" {
		t.Fatalf("handoff history = %#v, expected previous track on new session", history)
	}
	queue := server.loadPlaybackSessionQueue(user.ID, handedOff.SessionID)
	if len(queue) != 1 || queue[0].ID != "track_handoff_c" {
		t.Fatalf("handoff queue = %#v, expected remaining next track", queue)
	}
	var oldEndedAt string
	if err := server.db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = ?`, started.SessionID).Scan(&oldEndedAt); err != nil {
		t.Fatalf("read old session: %v", err)
	}
	if oldEndedAt == "" {
		t.Fatalf("old playback session was not ended during handoff")
	}
}

func TestPlaybackHandoffFailureLeavesPreviousSessionActive(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_atomic_handoff', 'Music', 'music', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('track_atomic_handoff', 'lib_atomic_handoff', 'track', 'Current Track', 'Current Track', ?, 'https://media.example.com/current.mp3', 120)`, now); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID:          "track_atomic_handoff",
		ClientInstanceID: "apple-player-atomic",
		SkipPreroll:      true,
	})

	body, err := json.Marshal(PlaybackHandoffRequest{MediaID: "missing_replacement"})
	if err != nil {
		t.Fatalf("marshal handoff: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions/"+started.SessionID+"/handoff", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackHandoff(rec, req, user, started.SessionID)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "media_not_found") {
		t.Fatalf("failed handoff status=%d body=%s", rec.Code, rec.Body.String())
	}

	var endedAt, state string
	if err := server.db.QueryRow(`SELECT ended_at, state FROM playback_sessions WHERE id = ?`, started.SessionID).Scan(&endedAt, &state); err != nil {
		t.Fatalf("read previous session: %v", err)
	}
	if endedAt != "" || state == "stopped" {
		t.Fatalf("failed replacement destroyed previous session: ended_at=%q state=%q", endedAt, state)
	}
}

func TestPlaybackSessionQueueMutationCommands(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_queue_mutation', 'Music', 'music', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES
			('track_queue_a', 'lib_queue_mutation', 'track', 'Track A', 'Track A', ?, 'https://media.example.com/a.mp3', 120),
			('track_queue_b', 'lib_queue_mutation', 'track', 'Track B', 'Track B', ?, 'https://media.example.com/b.mp3', 130),
			('track_queue_c', 'lib_queue_mutation', 'track', 'Track C', 'Track C', ?, 'https://media.example.com/c.mp3', 140),
			('track_queue_d', 'lib_queue_mutation', 'track', 'Track D', 'Track D', ?, 'https://media.example.com/d.mp3', 150)`,
		now, now, now, now); err != nil {
		t.Fatalf("insert tracks: %v", err)
	}
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{MediaID: "track_queue_a", ClientInstanceID: "queue-client", QueueMediaIDs: []string{"track_queue_b"}, RepeatMode: "off", SourceContext: PlaybackSourceContext{Type: "queue", Title: "Queue", MediaIDs: []string{"track_queue_a", "track_queue_b"}}})
	revision := started.QueueRevision

	assertQueueMutation := func(name string, req PlaybackSessionQueueRequest, want []string) PlaybackSessionQueueResponse {
		t.Helper()
		req.ExpectedRevision = &revision
		body, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		httpReq := httptest.NewRequest(http.MethodPatch, "/api/playback-sessions/"+started.SessionID+"/queue", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		server.handlePlaybackSessionQueue(rec, httpReq, user, started.SessionID)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", name, rec.Code, rec.Body.String())
		}
		var response PlaybackSessionQueueResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		got := mediaIDsForTest(response.Items)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s queue = %v, want %v", name, got, want)
		}
		if len(response.SourceContext.MediaIDs) != len(want)+1 || response.SourceContext.MediaIDs[0] != "track_queue_a" {
			t.Fatalf("%s source context ids = %#v", name, response.SourceContext.MediaIDs)
		}
		if response.Revision != revision+1 {
			t.Fatalf("%s revision = %d, want %d", name, response.Revision, revision+1)
		}
		revision = response.Revision
		return response
	}

	assertQueueMutation("append", PlaybackSessionQueueRequest{Action: "append", MediaIDs: []string{"track_queue_c"}}, []string{"track_queue_b", "track_queue_c"})
	assertQueueMutation("play next", PlaybackSessionQueueRequest{Action: "play_next", MediaID: "track_queue_d"}, []string{"track_queue_d", "track_queue_b", "track_queue_c"})
	from, to := 0, 2
	assertQueueMutation("reorder", PlaybackSessionQueueRequest{Action: "reorder", FromIndex: &from, ToIndex: &to}, []string{"track_queue_b", "track_queue_c", "track_queue_d"})
	index := 1
	assertQueueMutation("remove", PlaybackSessionQueueRequest{Action: "remove", Index: &index}, []string{"track_queue_b", "track_queue_d"})
	assertQueueMutation("clear", PlaybackSessionQueueRequest{Action: "clear"}, []string{})
	repeated := assertQueueMutation("set repeat", PlaybackSessionQueueRequest{Action: "set_repeat", RepeatMode: "all"}, []string{})
	if repeated.RepeatMode != "all" {
		t.Fatalf("repeat mode = %q, want all", repeated.RepeatMode)
	}

	staleRevision := revision - 1
	staleBody, _ := json.Marshal(PlaybackSessionQueueRequest{ExpectedRevision: &staleRevision, Action: "set_repeat", RepeatMode: "off"})
	staleReq := httptest.NewRequest(http.MethodPatch, "/api/playback-sessions/"+started.SessionID+"/queue", bytes.NewReader(staleBody))
	staleRec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(staleRec, staleReq, user, started.SessionID)
	if staleRec.Code != http.StatusConflict || !strings.Contains(staleRec.Body.String(), "queue_revision_conflict") {
		t.Fatalf("stale mutation status=%d body=%s", staleRec.Code, staleRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/playback-sessions/"+started.SessionID+"/queue", nil)
	getRec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(getRec, getReq, user, started.SessionID)
	if getRec.Code != http.StatusOK {
		t.Fatalf("queue read status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var afterConflict PlaybackSessionQueueResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &afterConflict); err != nil {
		t.Fatalf("decode queue read: %v", err)
	}
	if afterConflict.Revision != revision || afterConflict.RepeatMode != "all" {
		t.Fatalf("state changed after conflict: %#v", afterConflict)
	}

	restoreBody, _ := json.Marshal(PlaybackRestoreRequest{ClientInstanceID: "queue-client", ClientProfile: PlaybackClientProfile{SupportsHLS: true}})
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/playback/active", bytes.NewReader(restoreBody))
	restoreRec := httptest.NewRecorder()
	server.handlePlaybackActive(restoreRec, restoreReq, user)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	var restored PlaybackRestoreResponse
	if err := json.Unmarshal(restoreRec.Body.Bytes(), &restored); err != nil {
		t.Fatalf("decode restore: %v", err)
	}
	if !restored.Active || restored.Playback == nil || restored.Playback.RepeatMode != "all" || restored.Playback.QueueRevision != revision {
		t.Fatalf("restored queue state = %#v", restored)
	}

	replaceBody, _ := json.Marshal(PlaybackSessionQueueReplaceRequest{ExpectedRevision: &revision, MediaIDs: []string{"track_queue_b", "track_queue_c"}, RepeatMode: "one"})
	replaceReq := httptest.NewRequest(http.MethodPut, "/api/playback-sessions/"+started.SessionID+"/queue", bytes.NewReader(replaceBody))
	replaceRec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(replaceRec, replaceReq, user, started.SessionID)
	if replaceRec.Code != http.StatusOK {
		t.Fatalf("replace status=%d body=%s", replaceRec.Code, replaceRec.Body.String())
	}
	var replaced PlaybackSessionQueueResponse
	if err := json.Unmarshal(replaceRec.Body.Bytes(), &replaced); err != nil {
		t.Fatalf("decode replacement: %v", err)
	}
	if replaced.Revision != revision+1 || replaced.RepeatMode != "one" || strings.Join(mediaIDsForTest(replaced.Items), ",") != "track_queue_b,track_queue_c" {
		t.Fatalf("replacement state = %#v", replaced)
	}

	invalidRevision := replaced.Revision
	invalidRepeatBody, _ := json.Marshal(PlaybackSessionQueueRequest{ExpectedRevision: &invalidRevision, Action: "set_repeat", RepeatMode: "none"})
	invalidRepeatReq := httptest.NewRequest(http.MethodPatch, "/api/playback-sessions/"+started.SessionID+"/queue", bytes.NewReader(invalidRepeatBody))
	invalidRepeatRec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(invalidRepeatRec, invalidRepeatReq, user, started.SessionID)
	if invalidRepeatRec.Code != http.StatusBadRequest || !strings.Contains(invalidRepeatRec.Body.String(), "queue_invalid") {
		t.Fatalf("invalid repeat status=%d body=%s", invalidRepeatRec.Code, invalidRepeatRec.Body.String())
	}

	missingRevisionBody, _ := json.Marshal(PlaybackSessionQueueRequest{Action: "clear"})
	missingRevisionReq := httptest.NewRequest(http.MethodPatch, "/api/playback-sessions/"+started.SessionID+"/queue", bytes.NewReader(missingRevisionBody))
	missingRevisionRec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(missingRevisionRec, missingRevisionReq, user, started.SessionID)
	if missingRevisionRec.Code != http.StatusBadRequest || !strings.Contains(missingRevisionRec.Body.String(), "queue_revision_required") {
		t.Fatalf("missing revision status=%d body=%s", missingRevisionRec.Code, missingRevisionRec.Body.String())
	}

	otherUser := User{ID: "usr_queue_other", Permissions: map[string]bool{"playMedia": true}}
	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/api/playback-sessions/"+started.SessionID+"/queue", nil)
	unauthorizedRec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(unauthorizedRec, unauthorizedReq, otherUser, started.SessionID)
	if unauthorizedRec.Code != http.StatusNotFound {
		t.Fatalf("cross-user queue read status=%d body=%s", unauthorizedRec.Code, unauthorizedRec.Body.String())
	}

	preferenceUser := user
	preferenceUser.Preferences.MusicPlayback.RepeatDefault = "all"
	preferred := startPlaybackForTest(t, server, preferenceUser, PlaybackSessionCreateRequest{MediaID: "track_queue_d"})
	if preferred.RepeatMode != "all" || preferred.QueueRevision != 0 {
		t.Fatalf("music repeat preference was not applied: repeat=%q revision=%d", preferred.RepeatMode, preferred.QueueRevision)
	}
}

func TestPlaybackQueueNormalizationCapsIDs(t *testing.T) {
	rawIDs := make([]string, maxPlaybackQueueItems+25)
	for index := range rawIDs {
		rawIDs[index] = fmt.Sprintf("queue_cap_%03d", index)
	}
	if normalized := normalizeMediaIDs(rawIDs); len(normalized) != maxPlaybackQueueItems {
		t.Fatalf("normalized queue id count = %d, want %d", len(normalized), maxPlaybackQueueItems)
	}
	if parsed := splitCSV(strings.Join(rawIDs, ",")); len(parsed) != maxPlaybackQueueItems {
		t.Fatalf("query string queue id count = %d, want %d", len(parsed), maxPlaybackQueueItems)
	}
	context := normalizePlaybackSourceContext(PlaybackSourceContext{Type: "queue", MediaIDs: rawIDs})
	if len(context.MediaIDs) != maxPlaybackQueueItems {
		t.Fatalf("source context queue id count = %d, want %d", len(context.MediaIDs), maxPlaybackQueueItems)
	}
	items := make([]MediaItem, maxPlaybackQueueItems+25)
	for index := range items {
		items[index] = MediaItem{ID: rawIDs[index]}
	}
	if capped := capPlaybackQueueItems(items); len(capped) != maxPlaybackQueueItems {
		t.Fatalf("capped queue item count = %d, want %d", len(capped), maxPlaybackQueueItems)
	}
}

func TestPlaybackSessionQueueMutationFiltersUnavailableMedia(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	perms, _ := json.Marshal(map[string]bool{"playMedia": true, "transcode": true})
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_queue_limited', 'limited', 'limited@example.test', 'Limited', 'hash', 'user', ?, '{}', ?, ?)`, string(perms), now, now); err != nil {
		t.Fatalf("insert limited user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, created_at)
		VALUES ('lib_queue_allowed', 'Allowed Music', 'music', ?), ('lib_queue_blocked', 'Blocked Music', 'music', ?)`, now, now); err != nil {
		t.Fatalf("insert libraries: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_queue_limited', 'lib_queue_allowed', ?)`, now); err != nil {
		t.Fatalf("grant library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES
			('track_queue_allowed', 'lib_queue_allowed', 'track', 'Allowed', 'Allowed', ?, 'https://media.example.com/allowed.mp3', 120),
			('track_queue_blocked', 'lib_queue_blocked', 'track', 'Blocked', 'Blocked', ?, 'https://media.example.com/blocked.mp3', 120)`, now, now); err != nil {
		t.Fatalf("insert tracks: %v", err)
	}
	user := User{ID: "usr_queue_limited", Username: "limited", Email: "limited@example.test", DisplayName: "Limited", Role: "user", Permissions: map[string]bool{"playMedia": true}}
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{MediaID: "track_queue_allowed", Intent: PlaybackIntent{QualityProfile: "original"}})

	revision := started.QueueRevision
	body, err := json.Marshal(PlaybackSessionQueueRequest{ExpectedRevision: &revision, Action: "append", MediaID: "track_queue_blocked"})
	if err != nil {
		t.Fatalf("marshal mutation: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/playback-sessions/"+started.SessionID+"/queue", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionQueue(rec, req, user, started.SessionID)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Every queue item must be accessible and playable") {
		t.Fatalf("restricted mutation status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlaybackStartFiltersRestrictedSourceContextMediaIDs(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	perms, _ := json.Marshal(map[string]bool{"playMedia": true})
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_context_limited', 'context', 'context@example.test', 'Context', 'hash', 'user', ?, '{}', ?, ?)`, string(perms), now, now); err != nil {
		t.Fatalf("insert limited user: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO libraries (id, name, type, created_at)
		VALUES ('lib_context_allowed', 'Allowed Music', 'music', ?), ('lib_context_blocked', 'Blocked Music', 'music', ?)`, now, now); err != nil {
		t.Fatalf("insert libraries: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_context_limited', 'lib_context_allowed', ?)`, now); err != nil {
		t.Fatalf("grant library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES
			('track_context_allowed', 'lib_context_allowed', 'track', 'Allowed', 'Allowed', ?, 'https://media.example.com/allowed.mp3', 120),
			('track_context_blocked', 'lib_context_blocked', 'track', 'Blocked', 'Blocked', ?, 'https://media.example.com/blocked.mp3', 120)`, now, now); err != nil {
		t.Fatalf("insert tracks: %v", err)
	}
	user := User{ID: "usr_context_limited", Username: "context", Email: "context@example.test", DisplayName: "Context", Role: "user", Permissions: map[string]bool{"playMedia": true, "transcode": true}}
	playback := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID:       "track_context_allowed",
		QueueMediaIDs: []string{"track_context_blocked"},
		SourceContext: PlaybackSourceContext{Type: "queue", Title: "Mixed Queue", MediaIDs: []string{"track_context_allowed", "track_context_blocked"}},
	})
	if got := mediaIDsForTest(playback.Queue); len(got) != 0 {
		t.Fatalf("restricted queue = %v, expected inaccessible track to be filtered", got)
	}
	if got := strings.Join(playback.SourceContext.MediaIDs, ","); got != "track_context_allowed" {
		t.Fatalf("source context leaked restricted ids: %q", got)
	}
}

func startPlaybackForTest(t *testing.T, server *Server, user User, playbackReq PlaybackSessionCreateRequest) PlaybackResponse {
	t.Helper()
	seedExactPlaybackFactsForFixture(t, server, playbackReq.MediaID)
	if len(playbackReq.ClientProfile.CapabilityEvidence) == 0 {
		playbackReq.ClientProfile = attachAuthenticatedPlaybackRuntime(playbackReq.ClientProfile)
	}
	body, err := json.Marshal(playbackReq)
	if err != nil {
		t.Fatalf("marshal start request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	return playback
}

func TestPlaybackRestorePreservesSelectedQualityAudioSubtitleAndVersion(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	mediaRoot := t.TempDir()
	primaryPath := filepath.Join(mediaRoot, "primary.mp4")
	alternatePath := filepath.Join(mediaRoot, "alternate.mp4")
	if err := os.WriteFile(primaryPath, []byte("primary"), 0o600); err != nil {
		t.Fatalf("write primary version: %v", err)
	}
	if err := os.WriteFile(alternatePath, []byte("alternate"), 0o600); err != nil {
		t.Fatalf("write alternate version: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, path, created_at) VALUES ('lib_restore_selection', 'Movies', 'movie', ?, ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('movie_restore_selection', 'lib_restore_selection', 'movie', 'Restore Selection', 'Restore Selection', ?, ?, 600)`, now, primaryPath); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	for _, row := range []struct{ id, path, label string }{
		{"version_primary", primaryPath, "Primary"},
		{"version_alternate", alternatePath, "Alternate"},
	} {
		if _, err := server.db.Exec(`INSERT INTO media_files (id, media_id, library_id, path, container, version_label, source_type, available, first_seen_at, last_seen_at) VALUES (?, 'movie_restore_selection', 'lib_restore_selection', ?, 'mp4', ?, 'local', 1, ?, ?)`, row.id, row.path, row.label, now, now); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, language, channels, bitrate, width, height, display_title)
		VALUES
			('video_main', 'movie_restore_selection', 'video', 'h264', '', 0, 4000000, 1920, 1080, 'H264'),
			('audio_main', 'movie_restore_selection', 'audio', 'aac', 'eng', 2, 160000, 0, 0, 'English'),
			('audio_alt', 'movie_restore_selection', 'audio', 'aac', 'fra', 2, 160000, 0, 0, 'French'),
			('sub_text', 'movie_restore_selection', 'subtitle', 'webvtt', 'eng', 0, 0, 0, 0, 'English')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET source_url = '/api/media/movie_restore_selection/subtitles/sub_text' WHERE id = 'sub_text'`); err != nil {
		t.Fatalf("set subtitle source: %v", err)
	}
	profile := PlaybackClientProfile{SupportedContainers: []string{"mp4", "hls"}, SupportedVideoCodecs: []string{"h264"}, SupportedAudioCodecs: []string{"aac"}, SupportsHLS: true, MaxWidth: 3840, MaxHeight: 2160, MaxAudioChannels: 8}
	started := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{MediaID: "movie_restore_selection", VersionID: "version_alternate", ClientProfile: profile, SkipPreroll: true})
	if started.SelectedVersionID != "version_alternate" {
		t.Fatalf("started selected version = %q, expected version_alternate", started.SelectedVersionID)
	}
	selectedPath := ""
	for _, version := range started.Media.MediaFiles {
		if version.Selected {
			selectedPath = version.Path
		}
	}
	if selectedPath != alternatePath {
		t.Fatalf("started selected path = %q, expected %q", selectedPath, alternatePath)
	}
	if _, err := server.db.Exec(`UPDATE playback_sessions SET selected_quality_id = 'video-standard', selected_audio_stream_id = 'audio_alt', selected_subtitle_stream_id = 'sub_text', selected_subtitle_mode = 'text' WHERE id = ?`, started.SessionID); err != nil {
		t.Fatalf("persist selected tuple: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/playback-sessions/"+started.SessionID+"/restore", nil)
	restored, err := server.mediaPlaybackResponseForSession(request, user, started.SessionID, started.SessionID, "movie_restore_selection", 37, profile, PlaybackIntent{})
	if err != nil {
		t.Fatalf("restore playback: %v", err)
	}
	if restored.SelectedQualityID != "video-standard" || restored.SelectedAudioStreamID != "audio_alt" || restored.SelectedSubtitleID != "sub_text" || restored.SelectedSubtitleMode != "text" || restored.SelectedVersionID != "version_alternate" {
		t.Fatalf("restored selected tuple = quality %q audio %q subtitle %q mode %q version %q", restored.SelectedQualityID, restored.SelectedAudioStreamID, restored.SelectedSubtitleID, restored.SelectedSubtitleMode, restored.SelectedVersionID)
	}
}

func TestPlaybackStartRejectsUnavailableOrCrossMediaVersion(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	var libraryID string
	if err := server.db.QueryRow(`SELECT library_id FROM media_items WHERE id = 'movie_meridian'`).Scan(&libraryID); err != nil {
		t.Fatalf("resolve fixture library: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, added_at) VALUES ('movie_version_other', ?, 'movie', 'Other Version Owner', 'Other Version Owner', '/tmp/other-version.mp4', ?)`, libraryID, now); err != nil {
		t.Fatalf("insert other media: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_files (id, media_id, library_id, path, source_type, available, first_seen_at, last_seen_at) VALUES ('version-from-another-item', 'movie_version_other', ?, '/tmp/other-version.mp4', 'local', 1, ?, ?)`, libraryID, now, now); err != nil {
		t.Fatalf("insert cross-media version: %v", err)
	}
	for _, versionID := range []string{"missing-version", "version-from-another-item"} {
		recorder := httptest.NewRecorder()
		body, err := json.Marshal(PlaybackSessionCreateRequest{MediaID: "movie_meridian", VersionID: versionID, SkipPreroll: true})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
		server.handlePlaybackSessionCreate(recorder, request, user)
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("version %q status = %d body=%s", versionID, recorder.Code, recorder.Body.String())
		}
		var problem map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
			t.Fatalf("decode problem: %v", err)
		}
		if problem["code"] != "media_version_unavailable" {
			t.Fatalf("version %q problem = %#v", versionID, problem)
		}
	}
}

func mediaIDsForTest(items []MediaItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestPlaybackStartServesDirectPlayAsByteRangeStream(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_direct_play_hls', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_direct_play_hls', 'lib_direct_play_hls', 'movie', 'Direct Play Movie', 'Direct Play Movie', ?, 'https://media.example.com/Direct%20Play%20Movie.mp4', 7200)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title, profile, pixel_format, bit_depth, frame_rate, channel_layout, sample_rate, stream_index)
		VALUES
			('movie_direct_play_hls_video', 'movie_direct_play_hls', 'video', 'h264', 0, 4000000, 1920, 1080, 'H264 Main - 1920x1080', 'main', 'yuv420p', 8, 24, '', 0, 0),
			('movie_direct_play_hls_audio', 'movie_direct_play_hls', 'audio', 'aac', 2, 160000, 0, 0, 'eng - AAC LC - 2 ch', 'lc', '', 0, 0, 'stereo', 48000, 1)`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "movie_direct_play_hls",
		ClientProfile: PlaybackClientProfile{
			Device:               "Apple",
			Platform:             "tvOS",
			SupportsHLS:          true,
			SupportsMPEGTS:       true,
			SupportedContainers:  []string{"mp4", "mov", "m4v", "mpegts", "hls"},
			SupportedVideoCodecs: []string{"h264", "hevc"},
			SupportedAudioCodecs: []string{"aac", "ac3", "eac3", "mp3", "alac"},
			MaxWidth:             3840,
			MaxHeight:            2160,
			MaxAudioChannels:     8,
			SupportsHEVC:         true,
			SupportsHDR:          true,
			SupportsEAC3:         true,
			SupportsAC3:          true,
			PrefersServerProxy:   true,
		},
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.Decision.Mode != "direct_play" || !playback.DirectPlay {
		t.Fatalf("expected direct play decision semantics to be preserved, got directPlay=%v decision=%+v", playback.DirectPlay, playback.Decision)
	}
	if playback.StreamFormat != "http" || playback.Decision.Protocol != "http" || playback.Decision.Container != "mp4" {
		t.Fatalf("expected direct byte-range playback contract, got streamFormat=%q decision=%+v", playback.StreamFormat, playback.Decision)
	}
	if !strings.Contains(playback.SourceURL, "/stream") || strings.Contains(playback.SourceURL, "/hls/") {
		t.Fatalf("expected raw stream URL instead of HLS source URL, got %q", playback.SourceURL)
	}
	if playback.Decision.RequiresRemux {
		t.Fatalf("direct byte-range playback should not require remux, got %+v", playback.Decision)
	}
}

func TestPlaybackStartValidatesAndPropagatesSelectedAudioStream(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_audio_select_hls', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_audio_select_hls', 'lib_audio_select_hls', 'movie', 'Audio Select Movie', 'Audio Select Movie', ?, 'https://media.example.com/Audio%20Select%20Movie.mp4', 7200)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('movie_audio_select_hls_video', 'movie_audio_select_hls', 'video', 'h264', 0, 0, 1920, 1080, 'H264 - 1920x1080'),
			('movie_audio_select_hls_audio_main', 'movie_audio_select_hls', 'audio', 'aac', 2, 160000, 0, 0, 'English Stereo'),
			('movie_audio_select_hls_audio_commentary', 'movie_audio_select_hls', 'audio', 'aac', 2, 128000, 0, 0, 'Commentary')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET stream_index = CASE id WHEN 'movie_audio_select_hls_video' THEN 0 WHEN 'movie_audio_select_hls_audio_main' THEN 1 ELSE 2 END WHERE media_id = 'movie_audio_select_hls'`); err != nil {
		t.Fatalf("seed exact stream indexes: %v", err)
	}
	profile := PlaybackClientProfile{
		Device:               "Browser",
		Platform:             "macOS Chrome",
		SupportsHLS:          true,
		SupportsMSE:          true,
		SupportedContainers:  []string{"mp4", "hls"},
		SupportedVideoCodecs: []string{"h264"},
		SupportedAudioCodecs: []string{"aac"},
		MaxWidth:             1920,
		MaxHeight:            1080,
		MaxAudioChannels:     2,
		PrefersServerProxy:   true,
	}
	playback := startPlaybackForTest(t, server, user, PlaybackSessionCreateRequest{
		MediaID:       "movie_audio_select_hls",
		ClientProfile: profile,
		AudioStreamID: "movie_audio_select_hls_audio_commentary",
	})
	if playback.SelectedAudioStreamID != "movie_audio_select_hls_audio_commentary" {
		t.Fatalf("selected audio stream was not echoed in playback response: %+v", playback)
	}
	if playback.Decision.Mode != "direct_play" || !strings.Contains(playback.SourceURL, "/stream") || strings.Contains(playback.SourceURL, "/hls/") {
		t.Fatalf("compatible MP4 selection should retain canonical direct byte-range resource: decision=%#v source=%q", playback.Decision, playback.SourceURL)
	}

	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID:       "movie_audio_select_hls",
		ClientProfile: profile,
		AudioStreamID: "movie_audio_select_hls_missing",
	})
	if err != nil {
		t.Fatalf("marshal invalid audio request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "audio_stream_not_found") {
		t.Fatalf("invalid audio stream status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlaybackStartServesBrowserDirectPlayAsByteRangeStream(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_browser_hls', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_browser_hls', 'lib_browser_hls', 'movie', 'Browser Movie', 'Browser Movie', ?, 'https://media.example.com/Browser%20Movie.mp4', 5400)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, profile, channels, bitrate, width, height, display_title)
		VALUES
			('movie_browser_hls_video', 'movie_browser_hls', 'video', 'h264', 'Constrained Baseline', 0, 3000000, 1280, 720, 'H264 - 1280x720'),
			('movie_browser_hls_audio', 'movie_browser_hls', 'audio', 'aac', '', 2, 128000, 0, 0, 'eng - AAC - 2 ch')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	seedExactPlaybackFactsForFixture(t, server, "movie_browser_hls")
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "movie_browser_hls",
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			Device:                 "Browser",
			Platform:               "macOS Chrome",
			SupportsHLS:            true,
			SupportsMSE:            true,
			SupportedContainers:    []string{"mp4", "hls"},
			SupportedVideoCodecs:   []string{"h264"},
			SupportedVideoProfiles: []string{"h264:baseline", "h264:main", "h264:high"},
			SupportedAudioCodecs:   []string{"aac"},
			MaxWidth:               1920,
			MaxHeight:              1080,
			MaxAudioChannels:       2,
			PrefersServerProxy:     true,
		}),
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.StreamFormat != "http" || playback.Decision.Protocol != "http" {
		t.Fatalf("expected browser playback response to be direct HTTP, got streamFormat=%q decision=%+v", playback.StreamFormat, playback.Decision)
	}
	if !strings.Contains(playback.SourceURL, "/stream") || strings.Contains(playback.SourceURL, "/hls/") {
		t.Fatalf("expected browser playback response to use raw stream URL, got %q", playback.SourceURL)
	}
}

func TestPlaybackStartRejectsLegacyOptimizedVersionRow(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_optimized_hls', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_optimized_hls', 'lib_optimized_hls', 'movie', 'Optimized Movie', 'Optimized Movie', ?, 'https://media.example.com/Optimized%20Movie.mp4', 7200)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('movie_optimized_hls_video', 'movie_optimized_hls', 'video', 'h264', 0, 0, 1920, 1080, 'H264 - 1920x1080'),
			('movie_optimized_hls_audio', 'movie_optimized_hls', 'audio', 'aac', 2, 160000, 0, 0, 'eng - AAC - 2 ch')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	optimizedPath := filepath.Join(t.TempDir(), "720p.mp4")
	if err := os.WriteFile(optimizedPath, []byte("optimized"), 0o600); err != nil {
		t.Fatalf("write optimized file: %v", err)
	}
	saveOptimizedSettings(t, server.db, `{"defaultProfile":"720p-medium"}`)
	if _, err := server.db.Exec(`
		INSERT INTO optimized_versions (id, media_id, profile, path, size_bytes, created_at, updated_at)
		VALUES ('optimized_hls_version', 'movie_optimized_hls', '720p-medium', ?, 1234, ?, ?)`, optimizedPath, now, now); err != nil {
		t.Fatalf("insert optimized version: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "movie_optimized_hls",
		ClientProfile: PlaybackClientProfile{
			Device:               "Browser",
			Platform:             "macOS",
			SupportsHLS:          true,
			SupportsMSE:          true,
			SupportedContainers:  []string{"mp4", "hls"},
			SupportedVideoCodecs: []string{"h264"},
			SupportedAudioCodecs: []string{"aac"},
			MaxWidth:             1280,
			MaxHeight:            720,
			MaxAudioChannels:     2,
			PrefersServerProxy:   true,
		},
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("legacy optimized row must not satisfy playback: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlaybackStartDoesNotPromoteLegacyOptimizedVersionWhenEnabled(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_optimized_prefer', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_optimized_prefer', 'lib_optimized_prefer', 'movie', 'Optimized Preferred', 'Optimized Preferred', ?, 'https://media.example.com/Optimized%20Preferred.mp4', 7200)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('movie_optimized_prefer_video', 'movie_optimized_prefer', 'video', 'hevc', 0, 24000000, 3840, 2160, 'HEVC - 3840x2160'),
			('movie_optimized_prefer_audio', 'movie_optimized_prefer', 'audio', 'truehd', 8, 4000000, 0, 0, 'eng - TrueHD - 8 ch')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	optimizedDir := filepath.Join(server.cfg.AppDataDir, "optimized", "movie_optimized_prefer")
	if err := os.MkdirAll(optimizedDir, 0o700); err != nil {
		t.Fatalf("create optimized dir: %v", err)
	}
	optimizedPath := filepath.Join(optimizedDir, "720p-medium.mp4")
	if err := os.WriteFile(optimizedPath, []byte("optimized"), 0o600); err != nil {
		t.Fatalf("write optimized file: %v", err)
	}
	saveOptimizedSettings(t, server.db, `{"defaultProfile":"720p-medium","preferOptimizedPlayback":true}`)
	if _, err := server.db.Exec(`
		INSERT INTO optimized_versions (id, media_id, profile, path, size_bytes, created_at, updated_at)
		VALUES ('optimized_prefer_version', 'movie_optimized_prefer', '720p-medium', ?, 1234, ?, ?)`, optimizedPath, now, now); err != nil {
		t.Fatalf("insert optimized version: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "movie_optimized_prefer",
		ClientProfile: PlaybackClientProfile{
			Device:                 "Browser",
			Platform:               "macOS Chrome",
			SupportsHLS:            true,
			SupportsMSE:            true,
			SupportedContainers:    []string{"mp4", "hls"},
			SupportedVideoCodecs:   []string{"h264"},
			SupportedVideoProfiles: []string{"h264:baseline", "h264:main", "h264:high"},
			SupportedAudioCodecs:   []string{"aac"},
			MaxWidth:               1920,
			MaxHeight:              1080,
			MaxAudioChannels:       2,
			PrefersServerProxy:     true,
		},
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("legacy optimized row must not be promoted: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlaybackStartRemuxCandidateFailsClosedWithoutExactSeekEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_rookie_playback', 'TV', 'tv', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('episode_rookie_815', 'lib_rookie_playback', 'episode', 'Survive The Streets', 'Survive The Streets', ?, 'https://media.example.com/The%20Rookie%20S08E15.mkv', 2539)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('episode_rookie_815_video', 'episode_rookie_815', 'video', 'h264', 0, 0, 1920, 1080, 'H264 - 1920x1080'),
			('episode_rookie_815_audio', 'episode_rookie_815', 'audio', 'eac3', 6, 640000, 0, 0, 'eng - EAC3 - 6 ch')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET profile='main', pixel_format='yuv420p', bit_depth=8, frame_rate=24, dynamic_range='sdr', stream_index=0 WHERE id='episode_rookie_815_video'; UPDATE media_streams SET profile='', channel_layout='5.1', sample_rate=48000, stream_index=1 WHERE id='episode_rookie_815_audio'`); err != nil {
		t.Fatalf("complete remux source facts: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "episode_rookie_815",
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			Device:               "Apple",
			Platform:             "tvOS",
			SupportsHLS:          true,
			SupportsMPEGTS:       true,
			SupportedContainers:  []string{"mp4", "mov", "m4v", "mpegts", "hls"},
			SupportedVideoCodecs: []string{"h264", "hevc"},
			SupportedAudioCodecs: []string{"aac", "ac3", "eac3", "mp3", "alac"},
			MaxWidth:             3840,
			MaxHeight:            2160,
			MaxAudioChannels:     8,
			SupportsHEVC:         true,
			SupportsHDR:          true,
			SupportsEAC3:         true,
			SupportsAC3:          true,
			PrefersServerProxy:   true,
		}),
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.Decision.Mode != "transcode_required" || !playback.Decision.RequiresTranscode || !playback.Decision.VideoTranscode || playback.Decision.RequiresRemux {
		t.Fatalf("expected exact-seek-safe transcode fallback, got %+v", playback.Decision)
	}
	if !containsString(playback.Decision.ReasonCodes, "exact_seek_evidence_unavailable") {
		t.Fatalf("expected exact-seek rejection reason, got %+v", playback.Decision)
	}
	if strings.Contains(playback.SourceURL, "directStream=1") {
		t.Fatalf("unsafe direct stream remux escaped into source URL: %q", playback.SourceURL)
	}
	if playback.Timeline.Type != "vod" || playback.Timeline.DurationSeconds <= 0 || playback.Timeline.SegmentSeconds != hlsSegmentSeconds {
		t.Fatalf("unexpected playback timeline: %+v", playback.Timeline)
	}
}

func TestPlaybackStartHEVCRemuxCandidateFailsClosedWithoutExactSeekEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_fargo_playback', 'TV', 'tv', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('episode_fargo_203', 'lib_fargo_playback', 'episode', 'The Myth of Sisyphus', 'The Myth of Sisyphus', ?, 'https://media.example.com/Fargo%20S02E03.mkv', 2925)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('episode_fargo_203_video', 'episode_fargo_203', 'video', 'hevc', 0, 0, 1920, 1080, 'HEVC - 1920x1080'),
			('episode_fargo_203_audio', 'episode_fargo_203', 'audio', 'aac', 6, 0, 0, 0, 'eng - AAC - 6 ch')`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET profile='main', pixel_format='yuv420p', bit_depth=8, frame_rate=24, dynamic_range='sdr', stream_index=0 WHERE id='episode_fargo_203_video'; UPDATE media_streams SET profile='lc', channel_layout='5.1', sample_rate=48000, stream_index=1 WHERE id='episode_fargo_203_audio'`); err != nil {
		t.Fatalf("complete HEVC remux source facts: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "episode_fargo_203",
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			Device:               "Apple",
			Platform:             "tvOS",
			SupportsHLS:          true,
			SupportsMPEGTS:       true,
			SupportedContainers:  []string{"mp4", "mov", "m4v", "mpegts", "hls"},
			SupportedVideoCodecs: []string{"h264", "hevc"},
			SupportedAudioCodecs: []string{"aac", "ac3", "eac3", "mp3", "alac"},
			MaxWidth:             3840,
			MaxHeight:            2160,
			MaxAudioChannels:     8,
			SupportsHEVC:         true,
			SupportsHDR:          true,
			SupportsEAC3:         true,
			SupportsAC3:          true,
			PrefersServerProxy:   true,
		}),
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.Decision.Mode != "transcode_required" || !playback.Decision.RequiresTranscode || !playback.Decision.VideoTranscode || playback.Decision.RequiresRemux {
		t.Fatalf("expected exact-seek-safe HEVC transcode fallback, got %+v", playback.Decision)
	}
	if !containsString(playback.Decision.ReasonCodes, "video_conversion") {
		t.Fatalf("expected verified HEVC-to-H264 conversion reason, got %+v", playback.Decision)
	}
	if strings.Contains(playback.SourceURL, "directStream=1") {
		t.Fatalf("unsafe HEVC direct stream remux escaped into source URL: %q", playback.SourceURL)
	}
}

func TestPlaybackStartVideoTranscodeDoesNotRequestDirectStreamRemux(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_hevc_transcode_playback', 'Movies', 'movie', ?)`, now); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
			VALUES ('movie_hevc_transcode', 'lib_hevc_transcode_playback', 'movie', 'HEVC Source', 'HEVC Source', ?, 'https://media.example.com/HEVC%20Source.mp4', 7200)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title, profile, pixel_format, bit_depth, frame_rate, channel_layout, sample_rate, stream_index)
		VALUES
			('movie_hevc_transcode_video', 'movie_hevc_transcode', 'video', 'hevc', 0, 0, 1920, 1080, 'HEVC - 1920x1080', 'main', 'yuv420p', 8, 24, '', 0, 0),
			('movie_hevc_transcode_audio', 'movie_hevc_transcode', 'audio', 'aac', 2, 160000, 0, 0, 'eng - AAC - 2 ch', 'lc', '', 0, 0, 'stereo', 48000, 1)`); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{
		MediaID: "movie_hevc_transcode",
		ClientProfile: PlaybackClientProfile{
			Device:               "Safari",
			Platform:             "web",
			ClientVersion:        "18.0",
			SupportsHLS:          true,
			SupportsMSE:          true,
			SupportedContainers:  []string{"mp4", "hls"},
			SupportedVideoCodecs: []string{"h264"},
			SupportedAudioCodecs: []string{"aac"},
			MaxWidth:             3840,
			MaxHeight:            2160,
			MaxAudioChannels:     2,
			PrefersServerProxy:   true,
		},
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playback start status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v", err)
	}
	if playback.Decision.Mode == "direct_stream" || !playback.Decision.VideoTranscode {
		t.Fatalf("expected full video transcode decision, got %+v", playback.Decision)
	}
	if strings.Contains(playback.SourceURL, "directStream=1") {
		t.Fatalf("video transcode HLS URL must not request direct stream remux, got %q", playback.SourceURL)
	}
}
