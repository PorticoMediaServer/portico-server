package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestRewriteMediaHLSManifestUsesAuthenticatedSegmentRoutes(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-MAP:URI=\"init.mp4\"\nsegment_00000.m4s\n#EXTINF:4.000,\nsegment_00001.m4s\n"
	rewritten := rewriteMediaHLSManifest("media id", "720p-high", "movie_probe_2", 0, "transcode", "audio_commentary", true, "ptc_clt_test", manifest)
	if !strings.Contains(rewritten, "/api/media/media%20id/hls/segment?quality=720p-high") || !strings.Contains(rewritten, "name=segment_00000.m4s") {
		t.Fatalf("first segment was not rewritten:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, `#EXT-X-MAP:URI="/api/media/media%20id/hls/segment?quality=720p-high`) || !strings.Contains(rewritten, "name=init.mp4") {
		t.Fatalf("init segment was not rewritten:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "subtitle=movie_probe_2") {
		t.Fatalf("segment routes did not preserve burn-in subtitle selection:\n%s", rewritten)
	}
	if strings.Contains(rewritten, "media_grant=") {
		t.Fatalf("segment routes exposed the playback media grant:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "audio=transcode") {
		t.Fatalf("segment routes did not preserve audio transcode mode:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "audioStream=audio_commentary") {
		t.Fatalf("segment routes did not preserve selected audio stream:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "directStream=1") {
		t.Fatalf("segment routes did not preserve direct stream remux mode:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "#EXT-X-VERSION:7") {
		t.Fatalf("manifest tags were not preserved:\n%s", rewritten)
	}
}

func TestRewriteMediaHLSManifestDiscardsLegacySourceStart(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.000,\nsegment_00001.m4s\n"
	rewritten := rewriteMediaHLSManifest("media id", "original", "", 2100, "", "", false, "", manifest)
	if strings.Contains(rewritten, "start=") || strings.Contains(rewritten, "startSeconds=") {
		t.Fatalf("full-timeline segment routes retained a legacy source offset:\n%s", rewritten)
	}
}

func TestRewriteUnknownDurationEventManifestPreservesTruthfulLifecycle(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:EVENT\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:4.000,\nsegment_00000.ts\n"
	rewritten := rewriteMediaHLSManifest("movie", "original", "", 0, "", "", false, "", manifest)
	if !strings.Contains(rewritten, "#EXT-X-PLAYLIST-TYPE:EVENT") || strings.Contains(rewritten, "#EXT-X-ENDLIST") {
		t.Fatalf("in-progress event lifecycle was fabricated:\n%s", rewritten)
	}
	finished := rewriteMediaHLSManifest("movie", "original", "", 0, "", "", false, "", manifest+"#EXT-X-ENDLIST\n")
	if !strings.Contains(finished, "#EXT-X-ENDLIST") {
		t.Fatalf("finite-source completion was lost:\n%s", finished)
	}
}

func TestReadTranscodeManifestContextHonorsRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session := &transcodeSession{manifest: filepath.Join(t.TempDir(), "not-ready.m3u8"), done: make(chan struct{}), updateCh: make(chan struct{})}
	_, err := (&Server{}).readTranscodeManifestContext(ctx, session, "movie", "original", "", 0, "", "", false, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("manifest read cancellation error = %v", err)
	}
}

func TestGeneratedHLSManifestValidationFencesStructureAndFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), []byte("init"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "segment_00000.m4s"), []byte("segment"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := &transcodeSession{dir: dir}
	valid := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.000,\nsegment_00000.m4s\n"
	if err := validateGeneratedHLSManifest(session, valid); err != nil {
		t.Fatalf("valid generated manifest rejected: %v", err)
	}
	for name, manifest := range map[string]string{
		"duration over target": strings.Replace(valid, "#EXTINF:4.000", "#EXTINF:5.000", 1),
		"missing segment":      strings.Replace(valid, "segment_00000.m4s", "segment_00001.m4s", 1),
		"traversal":            strings.Replace(valid, "segment_00000.m4s", "../segment_00000.m4s", 1),
		"missing extinf":       strings.Replace(valid, "#EXTINF:4.000,\n", "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateGeneratedHLSManifest(session, manifest); err == nil {
				t.Fatal("invalid generated manifest was accepted")
			}
		})
	}
}

func TestGeneratedHLSBandwidthUsesProducedSegmentBytes(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "index.m3u8")
	text := "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:4.0,\nsegment_00000.ts\n#EXTINF:2.0,\nsegment_00001.ts\n"
	if err := os.WriteFile(manifest, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "segment_00000.ts"), make([]byte, 1_000), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "segment_00001.ts"), make([]byte, 1_000), 0o600); err != nil {
		t.Fatal(err)
	}
	measured := measureGeneratedHLSBandwidth(&transcodeSession{dir: dir, manifest: manifest})
	if measured.PeakBitsPerSecond != 4_000 || measured.AverageBitsPerSecond != 2_667 {
		t.Fatalf("measured bandwidth = %#v", measured)
	}
	item := MediaItem{ID: "movie", Streams: []Stream{{ID: "v", Kind: "video", Codec: "h264", Width: 1280, Height: 720, Bitrate: 9_000_000}}}
	master := buildMediaHLSMasterManifestWithBandwidth(item, item.ID, "original", "", 0, "", "", true, "", "", text, measured)
	if !strings.Contains(master, "BANDWIDTH=4000,AVERAGE-BANDWIDTH=2667") {
		t.Fatalf("master did not use measured generated output bandwidth:\n%s", master)
	}
}

func TestBuildMediaHLSMasterManifestAdvertisesSidecarTextSubtitle(t *testing.T) {
	item := MediaItem{
		ID:              "movie",
		DurationSeconds: 120,
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "h264", Profile: "high", Width: 1920, Height: 1080, FrameRate: 23.976, DynamicRange: "sdr", Bitrate: 4_000_000},
			{ID: "a1", Kind: "audio", Codec: "aac", Bitrate: 160_000},
			{ID: "sub1", Kind: "subtitle", Codec: "webvtt", Language: "eng", DisplayTitle: "English", SourceURL: "/api/media/movie/subtitles/sub1"},
		},
	}
	manifest := buildMediaHLSMasterManifest(item, "movie", "original", "sub1", 42, "transcode", "a1", true, "ptc_clt_test", "subtitle-refresh", "#EXTM3U\nsegment_00000.m4s\n")
	if !strings.Contains(manifest, "#EXT-X-MEDIA:TYPE=SUBTITLES") || !strings.Contains(manifest, `GROUP-ID="subs"`) {
		t.Fatalf("master manifest did not advertise subtitle group:\n%s", manifest)
	}
	if !strings.Contains(manifest, "/api/media/movie/hls/subtitles.m3u8?textSubtitle=sub1") {
		t.Fatalf("master manifest did not include subtitle playlist route:\n%s", manifest)
	}
	if !strings.Contains(manifest, "/api/media/movie/hls/variant.m3u8?quality=original") || !strings.Contains(manifest, "directStream=1") {
		t.Fatalf("master manifest did not preserve variant options:\n%s", manifest)
	}
	if !strings.Contains(manifest, "audioStream=a1") {
		t.Fatalf("master manifest did not preserve selected audio stream:\n%s", manifest)
	}
	for _, want := range []string{`BANDWIDTH=`, `AVERAGE-BANDWIDTH=`, `CODECS="avc1.640028,mp4a.40.2"`, `RESOLUTION=1920x1080`, `FRAME-RATE=23.976`, `VIDEO-RANGE=SDR`, `FORCED=NO`, `CLOSED-CAPTIONS=NONE`} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("master manifest missing required attribute %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "media_grant=") {
		t.Fatalf("master manifest exposed a media grant:\n%s", manifest)
	}
	if strings.Contains(manifest, "start=") || strings.Contains(manifest, "_porticoSeek=") {
		t.Fatalf("master manifest retained legacy seek-offset state:\n%s", manifest)
	}
}

func TestHLSSegmentContentTypesDistinguishInitializationMedia(t *testing.T) {
	for name, want := range map[string]string{
		"segment_00001.ts":  "video/mp2t",
		"segment_00001.m4s": "video/iso.segment",
		"init.mp4":          "video/mp4",
	} {
		if got := hlsSegmentContentType(name); got != want {
			t.Fatalf("hlsSegmentContentType(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestBuildMediaHLSMasterManifestWithoutSubtitleStillPublishesVariant(t *testing.T) {
	item := MediaItem{ID: "silent", Streams: []Stream{{ID: "v1", Kind: "video", Codec: "h264", Width: 1280, Height: 720, FrameRate: 30, Bitrate: 2_000_000}}}
	manifest := buildMediaHLSMasterManifest(item, item.ID, "original", "", 0, "", "", true, "", "", "#EXTM3U\n#EXT-X-ENDLIST\n")
	if !strings.Contains(manifest, "#EXT-X-STREAM-INF:") || !strings.Contains(manifest, "/api/media/silent/hls/variant.m3u8?quality=original") {
		t.Fatalf("master manifest did not publish its sole variant:\n%s", manifest)
	}
	if strings.Contains(manifest, "TYPE=SUBTITLES") || strings.Contains(manifest, `SUBTITLES="subs"`) {
		t.Fatalf("master manifest invented a subtitle group:\n%s", manifest)
	}
	if strings.Contains(manifest, "mp4a") {
		t.Fatalf("silent-video master manifest invented an audio codec:\n%s", manifest)
	}
}

func TestBuildMediaHLSMasterManifestAdvertisesEncodedGeometry(t *testing.T) {
	item := MediaItem{ID: "movie", Streams: []Stream{
		{ID: "v1", Kind: "video", Codec: "hevc", Profile: "main10", Width: 1920, Height: 1080, FrameRate: 60, DynamicRange: "pq", Bitrate: 8_000_000},
		{ID: "a1", Kind: "audio", Codec: "dts", Bitrate: 768_000},
	}}
	manifest := buildMediaHLSMasterManifest(item, item.ID, "720p-medium", "", 0, "transcode", "a1", false, "", "", "#EXTM3U\n#EXT-X-ENDLIST\n")
	for _, want := range []string{`RESOLUTION=1280x720`, `CODECS="avc1.640028,mp4a.40.2"`, `VIDEO-RANGE=SDR`} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("encoded master manifest missing %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "hvc1") || strings.Contains(manifest, "VIDEO-RANGE=PQ") {
		t.Fatalf("encoded master manifest advertised source encoding facts:\n%s", manifest)
	}
}

func TestBuildStaticMediaHLSManifestUsesAbsoluteSegmentTimeline(t *testing.T) {
	server := newScannerTestServer(t)
	item := MediaItem{
		ID:              "movie_static_hls",
		DurationSeconds: 10,
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 4_000_000},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 2, Bitrate: 160_000},
		},
	}
	manifest, err := server.buildStaticMediaHLSManifest(item, item.ID, "original", "", 0, "", "", true, "ptc_clt_test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXTINF:4.000,\n/api/media/movie_static_hls/hls/segment?quality=original&directStream=1&name=segment_00000.ts",
		"#EXTINF:4.000,\n/api/media/movie_static_hls/hls/segment?quality=original&directStream=1&name=segment_00001.ts",
		"#EXTINF:2.000,\n/api/media/movie_static_hls/hls/segment?quality=original&directStream=1&name=segment_00002.ts",
		"#EXT-X-ENDLIST",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("static manifest missing %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "media_grant=") {
		t.Fatalf("static manifest exposed a media grant:\n%s", manifest)
	}
}

func TestBuildStaticMediaHLSManifestIgnoresResumeOffsetAndPublishesFullTimeline(t *testing.T) {
	server := newScannerTestServer(t)
	item := MediaItem{
		ID:              "movie_seek_hls",
		DurationSeconds: 130,
		Streams: []Stream{
			{ID: "v1", Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 4_000_000},
			{ID: "a1", Kind: "audio", Codec: "aac", Channels: 2, Bitrate: 160_000},
		},
	}
	manifest, err := server.buildStaticMediaHLSManifest(item, item.ID, "720p-high", "", 42, "", "", false, "ptc_clt_seek")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXTINF:4.000,\n/api/media/movie_seek_hls/hls/segment?quality=720p-high&name=segment_00000.ts",
		"#EXTINF:2.000,\n/api/media/movie_seek_hls/hls/segment?quality=720p-high&name=segment_00032.ts",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("seek static manifest missing %q:\n%s", want, manifest)
		}
	}
	if strings.Contains(manifest, "media_grant=") {
		t.Fatalf("seek manifest exposed a media grant:\n%s", manifest)
	}
	if strings.Contains(manifest, "start=42") {
		t.Fatalf("full-timeline manifest should not carry a resume offset:\n%s", manifest)
	}
}

func TestBuildStaticMediaHLSManifestRejectsUnboundedDuration(t *testing.T) {
	server := &Server{}
	item := MediaItem{
		ID:              "movie_unbounded_duration",
		DurationSeconds: maximumStaticHLSDurationSeconds + 1,
	}
	manifest, err := server.buildStaticMediaHLSManifest(item, item.ID, "original", "", 0, "", "", true, "ptc_clt_test")
	if err == nil {
		t.Fatal("expected oversized media duration to be rejected")
	}
	if manifest != "" {
		t.Fatalf("expected no manifest for oversized duration, got %d bytes", len(manifest))
	}
}

func TestMediaHLSSegmentStartSecondsTracksRequestedSegment(t *testing.T) {
	if got := mediaHLSSegmentStartSeconds(912, "segment_00228.m4s", 2885); got != 912 {
		t.Fatalf("initial segment start = %d, expected 912", got)
	}
	if got := mediaHLSSegmentStartSeconds(912, "segment_00382.m4s", 2885); got != 1528 {
		t.Fatalf("later segment start = %d, expected 1528", got)
	}
	if got := mediaHLSSegmentStartSeconds(0, "segment_00157.ts", 2885); got != 628 {
		t.Fatalf("zero manifest start = %d, expected 628", got)
	}
	if got := mediaHLSSegmentStartSeconds(912, "init.mp4", 2885); got != 912 {
		t.Fatalf("init segment start = %d, expected 912", got)
	}
	if got := mediaHLSSegmentStartSeconds(912, "segment_00999.m4s", 2885); got != 2884 {
		t.Fatalf("bounded segment start = %d, expected 2884", got)
	}
}

func TestPublicHLSCannotOptIntoKeyframeSnappedRemux(t *testing.T) {
	query := make(url.Values)
	query.Set("directStream", "1")
	if hlsDirectStreamRemuxRequested(query) {
		t.Fatalf("public HLS query unexpectedly enabled copy-remux")
	}
}

func TestMediaHLSSubtitleSegmentAppliesStoredOffset(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, duration_seconds)
		VALUES ('movie_hls_subtitle_offset', ?, 'movie', 'Movie', 'Movie', ?, 120)`,
		library.ID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url, subtitle_offset_ms)
		VALUES ('sub_offset', 'movie_hls_subtitle_offset', 'subtitle', 'webvtt', 'English', '/api/media/movie_hls_subtitle_offset/subtitles/sub_offset', 1500)`); err != nil {
		t.Fatalf("insert subtitle stream: %v", err)
	}
	dir := filepath.Join(server.cfg.AppDataDir, "subtitles", safePathComponent("movie_hls_subtitle_offset"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create subtitle dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub_offset.vtt"), []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n"), 0o600); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}
	user := dvrTestUser(t, server)
	req := httptest.NewRequest(http.MethodGet, "/api/media/movie_hls_subtitle_offset/hls/subtitle.vtt?textSubtitle=sub_offset", nil)
	authorizeTextSubtitleRequestForTest(t, server, user, MediaItem{ID: "movie_hls_subtitle_offset", Type: "movie", Title: "Movie"}, "sub_offset", req)
	rec := httptest.NewRecorder()
	server.handleMediaHLSSubtitleSegment(rec, req, user, "movie_hls_subtitle_offset")
	if rec.Code != http.StatusOK {
		t.Fatalf("subtitle segment status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "00:00:02.500 --> 00:00:03.500") {
		t.Fatalf("HLS subtitle segment did not apply offset:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "X-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:0") {
		t.Fatalf("first HLS subtitle segment did not map its local timeline:\n%s", rec.Body.String())
	}
}

func TestHLSWebVTTSegmentAcceptsHourlessTimestampsAndMapsPresentationTime(t *testing.T) {
	input := []byte("WEBVTT\n\n00:09.000 --> 00:11.000 line:90%\nHourless cue\n")
	shifted := shiftWebVTTCues(input, 500)
	if !strings.Contains(string(shifted), "00:00:09.500 --> 00:00:11.500 line:90%") {
		t.Fatalf("hourless WebVTT cue was not shifted:\n%s", shifted)
	}

	segment := string(buildHLSWebVTTMediaSegment(input, 500, 8, 4))
	for _, want := range []string{
		"X-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:720000",
		"00:00:01.500 --> 00:00:03.500 line:90%",
		"Hourless cue",
	} {
		if !strings.Contains(segment, want) {
			t.Fatalf("hourless HLS WebVTT segment missing %q:\n%s", want, segment)
		}
	}
}

func TestMediaHLSSubtitlePlaylistUsesSegmentTimeline(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, duration_seconds)
		VALUES ('movie_hls_subtitle_timeline', ?, 'movie', 'Movie', 'Movie', ?, 13)`,
		library.ID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url)
		VALUES ('sub_timeline', 'movie_hls_subtitle_timeline', 'subtitle', 'webvtt', 'English', '/api/media/movie_hls_subtitle_timeline/subtitles/sub_timeline')`); err != nil {
		t.Fatalf("insert subtitle stream: %v", err)
	}
	dir := filepath.Join(server.cfg.AppDataDir, "subtitles", safePathComponent("movie_hls_subtitle_timeline"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create subtitle dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub_timeline.vtt"), []byte("WEBVTT\n\n00:00:11.000 --> 00:00:13.000\nLate cue\n"), 0o600); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}
	user := dvrTestUser(t, server)
	req := httptest.NewRequest(http.MethodGet, "/api/media/movie_hls_subtitle_timeline/hls/subtitles.m3u8?textSubtitle=sub_timeline&start=10&_porticoSeek=refresh-1", nil)
	grant := authorizeTextSubtitleRequestForTest(t, server, user, MediaItem{ID: "movie_hls_subtitle_timeline", Type: "movie", Title: "Movie"}, "sub_timeline", req)
	rec := httptest.NewRecorder()
	server.handleMediaHLSSubtitlePlaylist(rec, req, user, "movie_hls_subtitle_timeline")
	if rec.Code != http.StatusOK {
		t.Fatalf("subtitle playlist status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"#EXT-X-TARGETDURATION:4",
		"#EXT-X-MEDIA-SEQUENCE:0",
		"/api/media/movie_hls_subtitle_timeline/hls/subtitle.vtt?textSubtitle=sub_timeline&segment=2",
		"/api/media/movie_hls_subtitle_timeline/hls/subtitle.vtt?textSubtitle=sub_timeline&segment=3",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("subtitle playlist missing %q:\n%s", want, body)
		}
	}

	segmentReq := httptest.NewRequest(http.MethodGet, "/api/media/movie_hls_subtitle_timeline/hls/subtitle.vtt?textSubtitle=sub_timeline&segment=2&start=10", nil)
	segmentReq.Header.Set("Authorization", "PorticoMedia "+grant.Token)
	segmentRec := httptest.NewRecorder()
	server.handleMediaHLSSubtitleSegment(segmentRec, segmentReq, user, "movie_hls_subtitle_timeline")
	if segmentRec.Code != http.StatusOK {
		t.Fatalf("subtitle segment status=%d body=%s", segmentRec.Code, segmentRec.Body.String())
	}
	if !strings.Contains(segmentRec.Body.String(), "00:00:03.000 --> 00:00:04.000") {
		t.Fatalf("subtitle segment did not localize cue time:\n%s", segmentRec.Body.String())
	}
	if !strings.Contains(segmentRec.Body.String(), "X-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:720000") {
		t.Fatalf("subtitle segment did not map local time to its presentation offset:\n%s", segmentRec.Body.String())
	}
}

func authorizeTextSubtitleRequestForTest(t *testing.T, server *Server, user User, item MediaItem, subtitleID string, request *http.Request) MediaGrant {
	t.Helper()
	decision := playbackDecisionWithTestPlan(t, PlaybackDecision{Mode: "transcode_required", RequiresTranscode: true}, item.ID, "text", subtitleID)
	sessionID := randomID("play_test_subtitle")
	if err := server.createPlaybackSession(httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil), user, item, sessionID, decision, PlaybackClientProfile{SupportsHLS: true}, PlaybackIntent{}, "", "", false, "", PlaybackSourceContext{}, "off"); err != nil {
		t.Fatalf("create planned subtitle session: %v", err)
	}
	grant, err := server.issueMediaGrantForPlayback(context.Background(), user, sessionID, item, decision, ResolvedPlaybackPolicy{}, true, true)
	if err != nil {
		t.Fatalf("issue planned subtitle grant: %v", err)
	}
	request.Header.Set("Authorization", "PorticoMedia "+grant.Token)
	return grant
}

func TestLocalSourcePathForTranscodeRequiresLibraryRoot(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mp4")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	item := MediaItem{ID: "movie", LibraryID: library.ID, SourceURL: mediaPath}
	validated, err := server.localSourcePathForTranscode(item)
	if err != nil {
		t.Fatalf("validate local source: %v", err)
	}
	realMediaPath, err := filepath.EvalSymlinks(mediaPath)
	if err != nil {
		t.Fatalf("resolve media path: %v", err)
	}
	if validated != realMediaPath {
		t.Fatalf("validated path = %s, expected %s", validated, realMediaPath)
	}

	item.SourceURL = "https://media.example.test/movie.mp4"
	if _, err := server.localSourcePathForTranscode(item); err == nil {
		t.Fatalf("expected remote source transcode to be rejected")
	}
}

func TestSourcePathForHLSTranscodeAllowsValidatedRemoteSources(t *testing.T) {
	server := newScannerTestServer(t)
	item := MediaItem{ID: "movie_remote", SourceURL: "https://media.example.test/movie.mp4?token=abc"}
	sourcePath, err := server.sourcePathForHLSTranscode(item)
	if err != nil {
		t.Fatalf("validate remote HLS source: %v", err)
	}
	if sourcePath != item.SourceURL {
		t.Fatalf("remote HLS source path = %q, expected %q", sourcePath, item.SourceURL)
	}

	item.SourceURL = "http://127.0.0.1/movie.mp4"
	if _, err := server.sourcePathForHLSTranscode(item); err == nil {
		t.Fatalf("expected unsafe loopback remote HLS source to be rejected")
	}
}

func TestSourcePathForHLSTranscodeAllowsCatalogBoundRemoteStorage(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Cloud", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO storage_sources(id,library_id,configured_path,classification,classification_source,backend_kind,display_name,created_at,updated_at) VALUES('storage-cloud',?,'webdav://cloud','network','owner','webdav','Cloud',?,?)`, library.ID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO storage_remote_objects(source_id,object_path,revision,size_bytes,first_seen_generation,last_seen_generation,updated_at) VALUES('storage-cloud','Movies/Film.mkv','rev-1',42,'gen-1','gen-1',?)`, now); err != nil {
		t.Fatal(err)
	}
	item := MediaItem{ID: "movie-cloud", LibraryID: library.ID, SourceURL: "portico-storage://storage-cloud/Movies/Film.mkv"}
	if got, err := server.sourcePathForHLSTranscode(item); err != nil || got != item.SourceURL {
		t.Fatalf("source=%q err=%v", got, err)
	}
	item.LibraryID = "another-library"
	if _, err := server.sourcePathForHLSTranscode(item); err == nil {
		t.Fatal("cross-library remote storage locator was accepted")
	}
}

func TestSourcePathForHLSTranscodeAllowsHDHomeRunLiveSources(t *testing.T) {
	server := newScannerTestServer(t)
	item := MediaItem{ID: "live_chan", Type: "live_channel", Labels: []string{"hdhomerun"}, SourceURL: "http://192.168.1.50:5004/auto/v1"}
	sourcePath, err := server.sourcePathForHLSTranscode(item)
	if err != nil {
		t.Fatalf("validate hdhomerun HLS source: %v", err)
	}
	if sourcePath != item.SourceURL {
		t.Fatalf("hdhomerun source path = %q, expected %q", sourcePath, item.SourceURL)
	}
}

func TestOriginalQualityUsesSourceEquivalentPreset(t *testing.T) {
	item := MediaItem{
		DurationSeconds: 120,
		Streams: []Stream{
			{Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 9_800_000},
			{Kind: "audio", Codec: "eac3", Channels: 6, Bitrate: 640_000},
		},
	}
	preset := sourceEquivalentTranscodePreset(item)
	if preset.height != 1080 || preset.videoK != 9800 || preset.audioK != 640 || preset.crf != 18 {
		t.Fatalf("source equivalent preset = %+v", preset)
	}
	item.Streams[0].Codec = "hevc"
	item.Streams[0].Bitrate = 1_999_113
	preset = sourceEquivalentTranscodePreset(item)
	if preset.videoK != 8000 {
		t.Fatalf("expected HEVC source to reserve enough H.264 bitrate, got %+v", preset)
	}
}

func TestDirectStreamRemuxAvailableForOriginalH264WithoutBurnIn(t *testing.T) {
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "h264", Width: 1920, Height: 1080}}}
	settings := transcodeSettings{DirectStreamRemux: true}
	if !directStreamRemuxAvailable(item, "original", settings, "") {
		t.Fatalf("expected original H.264 playback to use direct stream remux")
	}
	if directStreamRemuxAvailable(item, "720p-medium", settings, "") {
		t.Fatalf("scaled quality should not use direct stream remux")
	}
	if directStreamRemuxAvailable(item, "original", settings, "sub_1") {
		t.Fatalf("subtitle burn-in should force video transcode")
	}
	if directStreamRemuxUsesFragmentedMP4(item) {
		t.Fatalf("expected H.264 remux to use MPEG-TS HLS for streaming startup stability")
	}
	if directStreamRemuxNeedsHVC1Tag(item) {
		t.Fatalf("H.264 remux should not be tagged as HEVC")
	}
}

func TestDirectStreamRemuxAvailableForOriginalHEVC(t *testing.T) {
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "hevc", Width: 1920, Height: 1080}}}
	settings := transcodeSettings{DirectStreamRemux: true}
	if !directStreamRemuxAvailable(item, "original", settings, "") {
		t.Fatalf("expected original HEVC playback to use direct stream remux")
	}
	if !directStreamRemuxUsesFragmentedMP4(item) {
		t.Fatalf("expected HEVC remux to use fragmented MP4 HLS")
	}
	if !directStreamRemuxNeedsHVC1Tag(item) {
		t.Fatalf("expected HEVC remux to use hvc1 MP4 tagging")
	}
}

func TestDolbyVisionHLSUsesExplicitDVH1SampleEntryAndCodec(t *testing.T) {
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "hevc", DolbyVisionProfile: "8", DolbyVisionLevel: 6, HLSSampleEntry: "dvh1", Width: 3840, Height: 2160}}}
	if got := directStreamRemuxVideoTag(item); got != "dvh1" {
		t.Fatalf("Dolby Vision remux sample entry = %q", got)
	}
	video := item.Streams[0]
	if got := hlsCodecListForStream(video, video.Codec, "eac3"); got != "dvh1.08.06,ec-3" {
		t.Fatalf("Dolby Vision HLS CODECS = %q", got)
	}
	video.HLSSampleEntry = ""
	if got := hlsCodecListForStream(video, video.Codec, "eac3"); got != "" {
		t.Fatalf("unverified Dolby Vision sample entry advertised as %q", got)
	}
}

func TestDirectStreamRemuxUsesMpegTSForH264(t *testing.T) {
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "h264", Width: 1920, Height: 1080}}}
	settings := transcodeSettings{DirectStreamRemux: true}
	if !directStreamRemuxAvailable(item, "original", settings, "") {
		t.Fatalf("expected original H264 playback to use direct stream remux")
	}
	if directStreamRemuxUsesFragmentedMP4(item) {
		t.Fatalf("expected H264 remux to use MPEG-TS HLS for streaming startup stability")
	}
}

func TestSoftwareTranscodeNormalizesHLSTimestamps(t *testing.T) {
	server := newScannerTestServer(t)
	tempDir, argsPath := configureFakeFFmpeg(t, server)
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: tempDir,
		DirectStreamRemux:  true,
		X264Preset:         "veryfast",
	}
	item := MediaItem{
		ID:              "movie_software_hls_offsets",
		DurationSeconds: 600,
		Streams: []Stream{
			{Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
			{Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	session, err := server.startTranscodeLocked("user", item, "/tmp/source.mp4", "720p-medium", settings, "", 106, "", "", false, false)
	if err != nil {
		t.Fatalf("start software transcode: %v", err)
	}
	defer session.stop(0)
	args := readFakeFFmpegArgs(t, session, argsPath)
	assertFFmpegArgsContain(t, args,
		"-fflags\n+genpts\n",
		"-ss\n106\n",
		"-muxdelay\n0\n",
		"-muxpreload\n0\n",
		"-output_ts_offset\n106\n",
		"-start_number\n26\n",
		"-hls_playlist_type\nvod\n",
		"-hls_segment_options\nmpegts_copyts=0\n",
	)
}

func TestFFmpegAudioMapForSelection(t *testing.T) {
	item := MediaItem{
		ID: "movie_audio_select",
		Streams: []Stream{
			{ID: "movie_audio_select_probe_0", SourceKind: "ffprobe", Index: 0, Kind: "video", Codec: "h264"},
			{ID: "movie_audio_select_probe_1", SourceKind: "ffprobe", Index: 1, Kind: "audio", Codec: "aac", Channels: 2},
			{ID: "movie_audio_select_probe_2", SourceKind: "ffprobe", Index: 2, Kind: "audio", Codec: "eac3", Channels: 6},
			{ID: "audio_commentary", Kind: "audio", Codec: "aac", Channels: 2},
			{ID: "movie_audio_select_probe_4", Kind: "subtitle", Codec: "srt"},
		},
	}
	if got, err := ffmpegAudioMapForSelection(item, ""); err != nil || got != "0:a:0?" {
		t.Fatalf("default audio map = %q, %v; expected first audio", got, err)
	}
	if got, err := ffmpegAudioMapForSelection(item, "movie_audio_select_probe_2"); err != nil || got != "0:2?" {
		t.Fatalf("probe-index audio map = %q, %v; expected 0:2?", got, err)
	}
	if got, err := ffmpegAudioMapForSelection(item, "audio_commentary"); err != nil || got != "0:a:2?" {
		t.Fatalf("ordinal fallback audio map = %q, %v; expected 0:a:2?", got, err)
	}
	if _, err := ffmpegAudioMapForSelection(item, "movie_audio_select_probe_4"); err == nil {
		t.Fatalf("subtitle stream should not be accepted as an audio selection")
	}
	if _, err := ffmpegAudioMapForSelection(item, "missing_audio"); err == nil {
		t.Fatalf("missing audio stream should be rejected")
	}
}

func TestStartTranscodeMapsSelectedAudioStream(t *testing.T) {
	server := newScannerTestServer(t)
	tempDir, argsPath := configureFakeFFmpeg(t, server)
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: tempDir,
		X264Preset:         "veryfast",
	}
	item := MediaItem{
		ID:              "movie_audio_select",
		DurationSeconds: 120,
		Streams: []Stream{
			{ID: "movie_audio_select_probe_0", SourceKind: "ffprobe", Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
			{ID: "movie_audio_select_probe_1", SourceKind: "ffprobe", Index: 1, Kind: "audio", Codec: "aac", Channels: 2},
			{ID: "movie_audio_select_probe_2", SourceKind: "ffprobe", Index: 2, Kind: "audio", Codec: "eac3", Channels: 6},
		},
	}
	session, err := server.startTranscodeLocked("user", item, "/tmp/source.mp4", "720p-medium", settings, "", 0, "", "movie_audio_select_probe_2", false, false)
	if err != nil {
		t.Fatalf("start selected-audio transcode: %v", err)
	}
	defer session.stop(0)
	args := readFakeFFmpegArgs(t, session, argsPath)
	assertFFmpegArgsContain(t, args, "-map\n0:2?\n")
	if strings.Contains(args, "-map\n0:a:0?\n") {
		t.Fatalf("selected audio transcode unexpectedly mapped default audio:\n%s", args)
	}
	if !strings.Contains(filepath.ToSlash(session.dir), "/stream_movie_audio_select_probe_2") {
		t.Fatalf("selected audio stream was not isolated in the session path: %s", session.dir)
	}
}

func TestFFmpegAudioMapRejectsStreamFromUnselectedMediaVersion(t *testing.T) {
	item := MediaItem{
		ID: "movie_multiversion_audio",
		MediaFiles: []MediaFileVersion{
			{ID: "file_primary", Selected: true},
			{ID: "file_alternate"},
		},
		Streams: []Stream{
			{ID: "primary_video", FileID: "file_primary", SourceKind: "ffprobe", Index: 0, Kind: "video", Codec: "h264"},
			{ID: "primary_audio", FileID: "file_primary", SourceKind: "ffprobe", Index: 1, Kind: "audio", Codec: "aac"},
			{ID: "alternate_video", FileID: "file_alternate", SourceKind: "ffprobe", Index: 0, Kind: "video", Codec: "hevc"},
			{ID: "alternate_audio", FileID: "file_alternate", SourceKind: "ffprobe", Index: 2, Kind: "audio", Codec: "eac3"},
		},
	}

	if got, err := ffmpegAudioMapForSelection(item, "primary_audio"); err != nil || got != "0:1?" {
		t.Fatalf("selected-version audio map = %q, %v; expected 0:1?", got, err)
	}
	if _, err := ffmpegAudioMapForSelection(item, "alternate_audio"); err == nil {
		t.Fatalf("audio stream from the unselected media version should be rejected")
	}
	if got := playbackStreamsForSelectedVersion(item); len(got) != 2 {
		t.Fatalf("selected-version streams = %+v; expected only the primary file streams", got)
	}
}

func TestDirectStreamMpegTSRemuxNormalizesHLSTimestamps(t *testing.T) {
	server := newScannerTestServer(t)
	tempDir, argsPath := configureFakeFFmpeg(t, server)
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: tempDir,
		DirectStreamRemux:  true,
		X264Preset:         "veryfast",
	}
	item := MediaItem{
		ID:              "movie_direct_ts_offsets",
		DurationSeconds: 600,
		Streams: []Stream{
			{Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
			{Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	session, err := server.startTranscodeLocked("user", item, "/tmp/source.mp4", "original", settings, "", 106, "", "", true, false)
	if err != nil {
		t.Fatalf("start direct stream remux: %v", err)
	}
	defer session.stop(0)
	args := readFakeFFmpegArgs(t, session, argsPath)
	assertFFmpegArgsContain(t, args,
		"-fflags\n+genpts\n",
		"-ss\n106\n",
		"-muxdelay\n0\n",
		"-muxpreload\n0\n",
		"-output_ts_offset\n106\n",
		"-start_number\n26\n",
		"-hls_playlist_type\nvod\n",
		"-hls_segment_options\nmpegts_copyts=0\n",
	)
}

func TestDirectStreamFMP4RemuxNormalizesHLSTimestamps(t *testing.T) {
	server := newScannerTestServer(t)
	tempDir, argsPath := configureFakeFFmpeg(t, server)
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: tempDir,
		DirectStreamRemux:  true,
		X264Preset:         "veryfast",
	}
	item := MediaItem{
		ID:              "movie_direct_fmp4_offsets",
		DurationSeconds: 600,
		Streams: []Stream{
			{Kind: "video", Codec: "hevc", Width: 1920, Height: 1080},
			{Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	session, err := server.startTranscodeLocked("user", item, "/tmp/source.mp4", "original", settings, "", 106, "", "", true, false)
	if err != nil {
		t.Fatalf("start fmp4 direct stream remux: %v", err)
	}
	defer session.stop(0)
	if session.throttleBufferSeconds != 0 {
		t.Fatalf("fMP4 direct remux throttle = %d, expected disabled", session.throttleBufferSeconds)
	}
	args := readFakeFFmpegArgs(t, session, argsPath)
	assertFFmpegArgsContain(t, args,
		"-fflags\n+genpts\n",
		"-ss\n106\n",
		"-muxdelay\n0\n",
		"-muxpreload\n0\n",
		"-output_ts_offset\n106\n",
		"-start_number\n26\n",
		"-hls_playlist_type\nvod\n",
		"-hls_segment_options\nmovflags=+frag_keyframe+empty_moov+default_base_moof:use_editlist=0\n",
		"-hls_segment_type\nfmp4\n",
	)
}

func TestGeneratedHLSManifestsPreserveMediaTimelineAtProducerStart(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not available")
	}
	tempDir := t.TempDir()
	h264Source := filepath.Join(tempDir, "source_h264.mp4")
	runCommand(t, "generate h264 fixture", ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=8",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac",
		h264Source,
	)
	offsetH264Source := filepath.Join(tempDir, "source_h264_pts_offset.mkv")
	runCommand(t, "generate shifted-PTS h264 fixture", ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=8",
		"-vf", "setpts=PTS+300/TB",
		"-af", "asetpts=PTS+300/TB",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac",
		offsetH264Source,
	)
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: filepath.Join(tempDir, "hls"),
		DirectStreamRemux:  true,
		X264Preset:         "ultrafast",
	}
	t.Run("software transcode", func(t *testing.T) {
		server := newScannerTestServer(t)
		server.cfg.FFmpegPath = ffmpegPath
		item := MediaItem{
			ID:              "runtime_software_hls",
			DurationSeconds: 8,
			Streams: []Stream{
				{Kind: "video", Codec: "h264", Width: 320, Height: 180},
				{Kind: "audio", Codec: "aac", Channels: 2},
			},
		}
		session, err := server.startTranscodeLocked("user", item, h264Source, "720p-medium", settings, "", 2, "", "", false, false)
		if err != nil {
			t.Fatalf("start software transcode: %v", err)
		}
		defer session.stop(0)
		waitForTranscodeDone(t, session)
		assertHLSManifestStartsNear(t, ffprobePath, session.manifest, 2)
	})
	t.Run("mpegts direct remux", func(t *testing.T) {
		server := newScannerTestServer(t)
		server.cfg.FFmpegPath = ffmpegPath
		item := MediaItem{
			ID:              "runtime_mpegts_hls",
			DurationSeconds: 8,
			Streams: []Stream{
				{Kind: "video", Codec: "h264", Width: 320, Height: 180},
				{Kind: "audio", Codec: "aac", Channels: 2},
			},
		}
		session, err := server.startTranscodeLocked("user", item, h264Source, "original", settings, "", 2, "", "", true, false)
		if err != nil {
			t.Fatalf("start mpegts direct remux: %v", err)
		}
		defer session.stop(0)
		waitForTranscodeDone(t, session)
		assertHLSManifestStartsNear(t, ffprobePath, session.manifest, 2)
	})
	t.Run("mpegts direct remux with shifted source timestamps", func(t *testing.T) {
		server := newScannerTestServer(t)
		server.cfg.FFmpegPath = ffmpegPath
		item := MediaItem{
			ID:              "runtime_mpegts_shifted_pts_hls",
			DurationSeconds: 8,
			Streams: []Stream{
				{Kind: "video", Codec: "h264", Width: 320, Height: 180},
				{Kind: "audio", Codec: "aac", Channels: 2},
			},
		}
		session, err := server.startTranscodeLocked("user", item, offsetH264Source, "original", settings, "", 2, "", "", true, false)
		if err != nil {
			t.Fatalf("start shifted-PTS mpegts direct remux: %v", err)
		}
		defer session.stop(0)
		waitForTranscodeDone(t, session)
		assertHLSManifestStartsNear(t, ffprobePath, session.manifest, 2)
	})
	t.Run("mpegts direct remux with audio transcode", func(t *testing.T) {
		server := newScannerTestServer(t)
		server.cfg.FFmpegPath = ffmpegPath
		item := MediaItem{
			ID:              "runtime_mpegts_audio_transcode_hls",
			DurationSeconds: 8,
			Streams: []Stream{
				{Kind: "video", Codec: "h264", Width: 320, Height: 180},
				{Kind: "audio", Codec: "eac3", Channels: 6},
			},
		}
		session, err := server.startTranscodeLocked("user", item, h264Source, "original", settings, "", 2, "transcode", "", true, false)
		if err != nil {
			t.Fatalf("start mpegts direct remux with audio transcode: %v", err)
		}
		defer session.stop(0)
		waitForTranscodeDone(t, session)
		assertHLSManifestStartsNear(t, ffprobePath, session.manifest, 2)
	})
	t.Run("fmp4 direct remux", func(t *testing.T) {
		hevcSource := filepath.Join(tempDir, "source_hevc.mp4")
		cmd := exec.Command(ffmpegPath,
			"-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8",
			"-f", "lavfi", "-i", "sine=frequency=1000:duration=8",
			"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:keyint=48:min-keyint=48",
			"-tag:v", "hvc1", "-pix_fmt", "yuv420p",
			"-c:a", "aac",
			hevcSource,
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("libx265 fixture generation is not available: %v\n%s", err, string(output))
		}
		server := newScannerTestServer(t)
		server.cfg.FFmpegPath = ffmpegPath
		item := MediaItem{
			ID:              "runtime_fmp4_hls",
			DurationSeconds: 8,
			Streams: []Stream{
				{Kind: "video", Codec: "hevc", Width: 320, Height: 180},
				{Kind: "audio", Codec: "aac", Channels: 2},
			},
		}
		session, err := server.startTranscodeLocked("user", item, hevcSource, "original", settings, "", 2, "", "", true, false)
		if err != nil {
			t.Fatalf("start fmp4 direct remux: %v", err)
		}
		defer session.stop(0)
		waitForTranscodeDone(t, session)
		// Fragmented-MP4 copy-remux is retained only as an internal legacy
		// primitive; the public full-timeline route rejects query-forced remux.
		assertHLSManifestStartsNear(t, ffprobePath, session.manifest, 0)
	})
}

func TestMediaHLSRouteServesNormalizedDirectStreamManifest(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not available")
	}
	server := newScannerTestServer(t)
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	server.backgroundCtx = backgroundCtx
	server.ffmpegSupervisor = newTranscodeSupervisorV2(backgroundCtx, ffmpegsupervisor.Config{})
	t.Cleanup(func() {
		cancelBackground()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
		defer cancelShutdown()
		_ = server.ffmpegSupervisor.Shutdown(shutdownCtx)
	})
	server.cfg.FFmpegPath = ffmpegPath
	server.cfg.TranscodeDir = filepath.Join(t.TempDir(), "transcodes")
	if err := os.MkdirAll(server.cfg.TranscodeDir, 0o700); err != nil {
		t.Fatalf("create transcode fixture root: %v", err)
	}
	root := t.TempDir()
	mediaPath := filepath.Join(root, "shifted-pts.mkv")
	runCommand(t, "generate shifted-PTS route fixture", ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=8",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=8",
		"-vf", "setpts=PTS+300/TB",
		"-af", "asetpts=PTS+300/TB",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-pix_fmt", "yuv420p",
		"-c:a", "aac",
		mediaPath,
	)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('movie_route_shifted_pts', ?, 'movie', 'Shifted PTS', 'Shifted PTS', ?, ?, 8)`,
		library.ID, now, mediaPath); err != nil {
		t.Fatalf("insert media item: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, stream_index, kind, codec, channels, width, height, display_title)
		VALUES
			('movie_route_shifted_pts_video', 'movie_route_shifted_pts', 0, 'video', 'h264', 0, 320, 180, 'H.264'),
			('movie_route_shifted_pts_audio', 'movie_route_shifted_pts', 1, 'audio', 'aac', 2, 0, 0, 'AAC')`); err != nil {
		t.Fatalf("insert media streams: %v", err)
	}
	user := dvrTestUser(t, server)
	seedExactPlaybackFactsForFixture(t, server, "movie_route_shifted_pts")
	item, err := server.getMediaPlaybackDetail(user.ID, "movie_route_shifted_pts")
	if err != nil {
		t.Fatalf("load playback facts fixture: %v", err)
	}
	facts, factsDigest, err := server.mediaFactsForPlayback(context.Background(), item)
	if err != nil || len(facts.Video) == 0 || len(facts.Audio) == 0 {
		t.Fatalf("resolve playback facts fixture: facts=%#v err=%v", facts, err)
	}
	videoIndex, audioIndex := facts.Video[0].Index, facts.Audio[0].Index
	plan := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: facts.Source.Fingerprint,
		SourceRevision: facts.Source.Revision, CapabilityEvidenceID: "test-capability-v1",
		Mode: playbackplan.Remux, Protocol: "hls", Container: "mpegts", SegmentFormat: "mpegts",
		Selection: playbackplan.Selection{VideoIndex: &videoIndex, AudioIndex: &audioIndex},
		Streams: []playbackplan.StreamAction{
			{Index: facts.Video[0].Index, Kind: "video", Action: playbackplan.Copy, InputCodec: facts.Video[0].Codec, OutputCodec: facts.Video[0].Codec},
			{Index: facts.Audio[0].Index, Kind: "audio", Action: playbackplan.Copy, InputCodec: facts.Audio[0].Codec, OutputCodec: facts.Audio[0].Codec, InputLayout: facts.Audio[0].Layout, OutputLayout: facts.Audio[0].Layout},
		},
		Stages: []playbackplan.Stage{
			{Kind: "video", Operation: "copy", Execution: "stream"},
			{Kind: "audio", Operation: "copy", Execution: "stream"},
			{Kind: "mux", Operation: "package", Execution: "stream"},
		},
		Audio:    playbackplan.AudioDecision{Codec: facts.Audio[0].Codec, Layout: facts.Audio[0].Layout, Channels: facts.Audio[0].Channels, Passthrough: true, ObjectsPreserved: true},
		Timeline: playbackplan.Timeline{Mode: "vod", DurationUS: facts.DurationUS, Generation: 1},
		Subtitle: playbackplan.SubtitleDecision{Action: playbackplan.Drop},
	}
	plan.Digest, _ = plan.ComputeDigest()
	planJSON, _ := json.Marshal(plan)
	binding := playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: facts.Source.Revision, MediaFactsDigest: factsDigest,
		CapabilityEvidenceID: plan.CapabilityEvidenceID, Generation: 1, Mode: string(plan.Mode),
		Protocol: "hls", Container: "mpegts", Quality: "original", AudioMode: "auto",
		SubtitleMode: "off", DirectStream: true, X264Preset: "veryfast", Plan: planJSON,
	}
	if err := binding.seal(); err != nil {
		t.Fatalf("seal playback facts fixture: %v", err)
	}
	bindingJSON, _ := json.Marshal(binding)
	const playbackSessionID = "play_route_shifted_pts"
	if _, err := server.db.Exec(`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state,
			plan_schema_version, plan_digest, plan_json, source_revision, capability_evidence_id, playback_generation
		) VALUES (?, ?, ?, 'movie_route_shifted_pts', 'movie', 'Shifted PTS', ?, ?, 'playing', ?, ?, ?, ?, ?, ?)`,
		playbackSessionID, accountIDForUser(user), viewerProfileID(user), now, now,
		binding.SchemaVersion, binding.Digest, string(bindingJSON), binding.SourceRevision, binding.CapabilityEvidenceID, binding.Generation); err != nil {
		t.Fatalf("insert planned playback session: %v", err)
	}
	grant, err := server.issueMediaGrant(context.Background(), user, playbackSessionID, "media", "movie_route_shifted_pts")
	if err != nil {
		t.Fatalf("issue planned playback grant: %v", err)
	}
	req := mediaGrantRequest(http.MethodGet, "/api/media/movie_route_shifted_pts/hls/master.m3u8?quality=original&directStream=1&start=2", grant.Token)
	rec := httptest.NewRecorder()
	server.handleMediaHLSManifest(rec, req, user, "movie_route_shifted_pts", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("HLS manifest status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/api/media/movie_route_shifted_pts/hls/variant.m3u8?quality=original&directStream=1") || !strings.Contains(body, "#EXT-X-STREAM-INF:") || strings.Contains(body, "start=") {
		t.Fatalf("master did not retain the plan-bound remux variant or retained a caller-forced offset:\n%s", body)
	}
	if strings.Contains(body, server.cfg.TranscodeDir) || strings.Contains(body, "shifted-pts.mkv") {
		t.Fatalf("manifest leaked local paths:\n%s", body)
	}
	variantReq := mediaGrantRequest(http.MethodGet, "/api/media/movie_route_shifted_pts/hls/variant.m3u8?quality=original&directStream=1&start=2", grant.Token)
	variantRec := httptest.NewRecorder()
	server.handleMediaHLSManifest(variantRec, variantReq, user, "movie_route_shifted_pts", false)
	if variantRec.Code != http.StatusOK || !strings.Contains(variantRec.Body.String(), "/api/media/movie_route_shifted_pts/hls/segment?quality=original&directStream=1") || strings.Contains(variantRec.Body.String(), "start=") {
		t.Fatalf("variant did not retain the plan-bound remux segments: status=%d body=%s", variantRec.Code, variantRec.Body.String())
	}

	segmentReq := mediaGrantRequest(http.MethodGet, "/api/media/movie_route_shifted_pts/hls/segment?quality=original&directStream=1&start=2&name=segment_00000.ts", grant.Token)
	segmentRec := httptest.NewRecorder()
	server.handleMediaHLSSegment(segmentRec, segmentReq, user, "movie_route_shifted_pts")
	if segmentRec.Code != http.StatusOK {
		t.Fatalf("HLS segment status=%d body=%s", segmentRec.Code, segmentRec.Body.String())
	}
	if got := segmentRec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("segment cache control = %q, expected recovery-safe no-store", got)
	}

	server.transcodeMu.Lock()
	var session *transcodeSession
	for _, candidate := range server.transcodes {
		session = candidate
		break
	}
	server.transcodeMu.Unlock()
	if session == nil {
		t.Fatalf("expected route to create a transcode session")
	}
	if !strings.Contains(filepath.ToSlash(session.dir), "/planned-v2/") {
		t.Fatalf("session dir did not use the canonical planned-transcode generation root: %s", session.dir)
	}
	defer session.stop(0)
	waitForTranscodeDone(t, session)
	assertHLSManifestStartsNearZero(t, ffprobePath, session.manifest)
}

func configureFakeFFmpeg(t *testing.T, server *Server) (string, string) {
	t.Helper()
	tempDir := t.TempDir()
	argsPath := filepath.Join(tempDir, "ffmpeg.args")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    -encoders|-filters|-hwaccels|-version) exit 0 ;;\n  esac\ndone\nprintf '%s\\n' \"$@\" > \"$PORTICO_FFMPEG_ARGS\"\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	t.Setenv("PORTICO_FFMPEG_ARGS", argsPath)
	server.cfg.FFmpegPath = ffmpegPath
	return tempDir, argsPath
}

func TestLiveTVTranscodeUsesRollingHLSAndProviderUserAgent(t *testing.T) {
	server := newScannerTestServer(t)
	tempDir, argsPath := configureFakeFFmpeg(t, server)
	settings := transcodeSettings{
		Enabled:               true,
		TemporaryDirectory:    tempDir,
		MaxConcurrentSessions: 2,
		X264Preset:            "veryfast",
		ThrottleBufferSeconds: 60,
	}
	item := MediaItem{
		ID:              "channel_live_transcode_contract",
		LibraryID:       "source_live_transcode_contract",
		Type:            "live_channel",
		Title:           "Live channel",
		SourceURL:       "https://provider.example.test/live.ts",
		SourceUserAgent: "Portico Provider\r\nIgnored: header",
	}
	server.transcodeMu.Lock()
	session, err := server.startTranscodeLocked("viewer_live", item, item.SourceURL, "720p-medium", settings, "", 0, "transcode", "", false, false)
	server.transcodeMu.Unlock()
	if err != nil {
		t.Fatalf("start live transcode: %v", err)
	}
	args := readFakeFFmpegArgs(t, session, argsPath)
	assertFFmpegArgsContain(t, args,
		"-user_agent\nPortico ProviderIgnored: header",
		"-hls_list_size\n12",
		"-hls_delete_threshold\n4",
		"delete_segments+omit_endlist",
		"program_date_time",
		"-c:a\naac",
	)
	if strings.Contains(args, "-hls_playlist_type") {
		t.Fatalf("live transcode incorrectly requested a finite VOD playlist:\n%s", args)
	}
	select {
	case <-session.done:
	case <-time.After(15 * time.Second):
		t.Fatal("live transcode did not report provider EOF")
	}
	if err := session.transcodeError(); err == nil || !strings.Contains(err.Error(), "provider stream ended") || !session.recoverableFailure() {
		t.Fatalf("clean Live TV provider EOF was not made recoverable: %v", err)
	}
	if session.throttleBufferSeconds != 0 {
		t.Fatalf("live transcode must remain continuous instead of VOD-throttling, got %d seconds", session.throttleBufferSeconds)
	}
	if !session.live || session.lastProducedSegment <= 0 {
		t.Fatalf("live transcode did not use a monotonic live media sequence: live=%v sequence=%d", session.live, session.lastProducedSegment)
	}
	redacted := redactedFFmpegContext(strings.Split(strings.TrimSpace(args), "\n"))
	if strings.Contains(redacted, "Portico Provider") || !strings.Contains(redacted, "<provider-user-agent>") {
		t.Fatalf("provider user agent was not redacted from diagnostics: %s", redacted)
	}
}

func configureRecoveryFakeFFmpeg(t *testing.T, server *Server, mode string) string {
	t.Helper()
	tempDir := t.TempDir()
	countPath := filepath.Join(tempDir, "ffmpeg.count")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg")
	script := `#!/bin/sh
count_file='` + shellSingleQuote(countPath) + `'
mode='` + shellSingleQuote(mode) + `'
manifest=""
segment_pattern=""
prev=""
for arg in "$@"; do
	if [ "$prev" = "-hls_segment_filename" ]; then segment_pattern="$arg"; fi
	prev="$arg"
	manifest="$arg"
done

if [ "$segment_pattern" = "" ]; then
	exit 0
fi

lock_dir="$count_file.lock"
while ! mkdir "$lock_dir" 2>/dev/null; do sleep 0.01; done
count=0
if [ -f "$count_file" ]; then count=$(cat "$count_file"); fi
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
rmdir "$lock_dir"

write_hls_output() {
	segment=$(printf "$segment_pattern" 0)
	mkdir -p "$(dirname "$manifest")" "$(dirname "$segment")"
	printf '#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:4.000,\nsegment_00000.ts\n#EXT-X-ENDLIST\n' > "$manifest"
	printf 'segment-%s\n' "$count" > "$segment"
}

case "$mode" in
	crash-then-success)
		if [ "$count" -eq 1 ]; then
			echo "simulated ffmpeg crash" >&2
			exit 42
		fi
		write_hls_output
		exit 0
		;;
	always-crash)
		echo "simulated ffmpeg crash" >&2
		exit 42
		;;
	slow)
		sleep 30
		exit 0
		;;
	*)
		write_hls_output
		exit 0
		;;
esac
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write recovery fake ffmpeg: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	return countPath
}

func shellSingleQuote(value string) string {
	return strings.ReplaceAll(value, `'`, `'\''`)
}

func recoveryTranscodeRequest(t *testing.T, mediaID string) (transcodeSettings, transcodeStartRequest) {
	t.Helper()
	settings := transcodeSettings{
		Enabled:               true,
		TemporaryDirectory:    filepath.Join(t.TempDir(), "hls"),
		X264Preset:            "veryfast",
		MaxConcurrentSessions: 8,
	}
	item := MediaItem{
		ID:              mediaID,
		Type:            "movie",
		Title:           mediaID,
		DurationSeconds: 120,
		SourceURL:       "https://media.example.com/source.mp4",
		Streams: []Stream{
			{Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
			{Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	request := transcodeStartRequest{
		userID:       "user_recovery",
		item:         item,
		sourcePath:   "https://media.example.com/source.mp4",
		quality:      "720p-medium",
		startSeconds: 0,
	}
	return settings, request
}

func startRecoveryTestTranscode(t *testing.T, server *Server, settings transcodeSettings, request transcodeStartRequest) *transcodeSession {
	t.Helper()
	session, err := server.startTranscodeLocked(request.userID, request.item, request.sourcePath, request.quality, settings, request.subtitleID, request.startSeconds, request.audioMode, request.audioStreamID, request.directStream, request.background)
	if err != nil {
		t.Fatalf("start recovery test transcode: %v", err)
	}
	server.transcodeMu.Lock()
	if server.transcodes == nil {
		server.transcodes = map[string]*transcodeSession{}
	}
	server.transcodes[session.key] = session
	server.transcodeMu.Unlock()
	return session
}

func waitForTranscodeError(t *testing.T, session *transcodeSession) {
	t.Helper()
	select {
	case <-session.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("transcode did not fail")
	}
	if err := session.transcodeError(); err == nil {
		exit := "unknown"
		if session.cmd != nil && session.cmd.ProcessState != nil {
			exit = strconv.Itoa(session.cmd.ProcessState.ExitCode())
		}
		t.Fatalf("expected transcode error, exit=%s stopped=%t stderr=%q", exit, session.stopped, session.stderr)
	}
}

func readRecoveryFakeFFmpegCount(t *testing.T, countPath string) int {
	t.Helper()
	raw, err := os.ReadFile(countPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("read recovery fake ffmpeg count: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse recovery fake ffmpeg count: %v", err)
	}
	return count
}

func waitForRecoveryFakeFFmpegCount(t *testing.T, countPath string, expected int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if count := readRecoveryFakeFFmpegCount(t, countPath); count >= expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake ffmpeg count did not reach %d, got %d", expected, readRecoveryFakeFFmpegCount(t, countPath))
}

func runCommand(t *testing.T, label string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s failed: %v\n%s", label, err, string(output))
	}
}

func waitForTranscodeDone(t *testing.T, session *transcodeSession) {
	t.Helper()
	select {
	case <-session.done:
	case <-time.After(15 * time.Second):
		t.Fatalf("transcode did not finish")
	}
	if err := session.transcodeError(); err != nil {
		t.Fatalf("transcode failed: %v", err)
	}
}

func assertHLSManifestStartsNear(t *testing.T, ffprobePath string, manifest string, expected float64) {
	t.Helper()
	output, err := exec.Command(ffprobePath, "-v", "error", "-show_entries", "format=start_time", "-of", "default=nw=1:nk=1", manifest).Output()
	if err != nil {
		t.Fatalf("probe HLS manifest start time: %v", err)
	}
	value := strings.TrimSpace(string(output))
	startTime, err := strconv.ParseFloat(value, 64)
	if err != nil {
		t.Fatalf("parse HLS start time %q: %v", value, err)
	}
	if math.Abs(startTime-expected) > 0.1 {
		t.Fatalf("HLS manifest %s starts at %.3fs, expected %.3fs", manifest, startTime, expected)
	}
}

func assertHLSManifestStartsNearZero(t *testing.T, ffprobePath string, manifest string) {
	t.Helper()
	assertHLSManifestStartsNear(t, ffprobePath, manifest, 0)
}

func readFakeFFmpegArgs(t *testing.T, session *transcodeSession, argsPath string) string {
	t.Helper()
	select {
	case <-session.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("fake ffmpeg did not exit")
	}
	rawArgs, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake ffmpeg args: %v", err)
	}
	return string(rawArgs)
}

func assertFFmpegArgsContain(t *testing.T, args string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(args, value) {
			t.Fatalf("ffmpeg args did not contain %q:\n%s", value, args)
		}
	}
}

func TestOriginalTranscodeRequiresExplicitDirectStreamFlagForRemux(t *testing.T) {
	server := newScannerTestServer(t)
	ffmpegPath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("locate true command: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: t.TempDir(),
		DirectStreamRemux:  true,
		X264Preset:         "veryfast",
	}
	item := MediaItem{
		ID:              "movie_direct_gate",
		DurationSeconds: 120,
		Streams: []Stream{
			{Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
			{Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	session, err := server.startTranscodeLocked("user", item, "/tmp/source.mp4", "original", settings, "", 0, "", "", false, false)
	if err != nil {
		t.Fatalf("start original transcode: %v", err)
	}
	session.stop(0)
	if session.method == "direct-stream-remux" || session.filter == "video copy" {
		t.Fatalf("plain original HLS transcode should not remux without directStream=1, got method=%s filter=%s", session.method, session.filter)
	}
	if !strings.Contains(session.dir, "video_transcode") {
		t.Fatalf("expected full transcode output directory, got %s", session.dir)
	}

	remuxSession, err := server.startTranscodeLocked("user", item, "/tmp/source.mp4", "original", settings, "", 0, "", "", true, false)
	if err != nil {
		t.Fatalf("start direct stream remux: %v", err)
	}
	remuxSession.stop(0)
	if remuxSession.method != "direct-stream-remux" || remuxSession.filter != "video copy" {
		t.Fatalf("directStream=1 should allow remux, got method=%s filter=%s", remuxSession.method, remuxSession.filter)
	}
	if !strings.Contains(remuxSession.dir, "direct_stream") {
		t.Fatalf("expected direct stream output directory, got %s", remuxSession.dir)
	}
}

func TestCompletedTranscodeSessionRemainsReusable(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "index.m3u8")
	if err := os.WriteFile(manifest, []byte("#EXTM3U\n#EXTINF:4.0,\nsegment_00000.ts\n#EXT-X-ENDLIST\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	done := make(chan struct{})
	close(done)
	session := &transcodeSession{manifest: manifest, done: done}
	if !session.completedSuccessfully() {
		t.Fatalf("expected completed session with manifest to be reusable")
	}
	session.stopped = true
	if session.completedSuccessfully() {
		t.Fatalf("stopped session should not be reusable")
	}
}

func TestWaitForHLSSegmentFileAllowsCompletedSessionArtifacts(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "index.m3u8")
	segment := filepath.Join(dir, "segment_00000.m4s")
	if err := os.WriteFile(manifest, []byte("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\nsegment_00000.m4s\n#EXT-X-ENDLIST\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(segment, []byte("segment"), 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}
	done := make(chan struct{})
	close(done)
	session := &transcodeSession{manifest: manifest, done: done}
	if err := waitForHLSSegmentFile(session, segment); err != nil {
		t.Fatalf("expected completed session segment to be served: %v", err)
	}
}

func TestRecoverTranscodeSessionRestartsCrashedProcessOnDemand(t *testing.T) {
	server := newScannerTestServer(t)
	countPath := configureRecoveryFakeFFmpeg(t, server, "crash-then-success")
	settings, request := recoveryTranscodeRequest(t, "movie_recover_crash")
	first := startRecoveryTestTranscode(t, server, settings, request)
	firstDir := first.dir
	waitForTranscodeError(t, first)

	recovered, err := server.recoverTranscodeSessionForDemand(context.Background(), settings, request, first, first.err)
	if err != nil {
		t.Fatalf("recover transcode session: %v", err)
	}
	if recovered == nil || recovered == first {
		t.Fatalf("expected recovery to replace the failed session")
	}
	if err := waitForHLSSegmentFile(recovered, filepath.Join(recovered.dir, "segment_00000.ts")); err != nil {
		t.Fatalf("expected recovered segment to become available: %v", err)
	}
	waitForTranscodeDone(t, recovered)
	if count := readRecoveryFakeFFmpegCount(t, countPath); count != 2 {
		t.Fatalf("expected initial crash plus one recovery start, got %d starts", count)
	}
	if !first.stopped {
		t.Fatalf("expected failed generation to be stopped before replacement")
	}
	if recovered.recoveryAttempts != 1 {
		t.Fatalf("expected recovered session to carry one recovery attempt, got %d", recovered.recoveryAttempts)
	}
	if recovered.generation != 1 {
		t.Fatalf("expected recovered session generation 1, got %d", recovered.generation)
	}
	archives, err := filepath.Glob(firstDir + ".generation-0-*")
	if err != nil || len(archives) != 1 {
		t.Fatalf("expected one isolated failed-generation directory, got %v (err=%v)", archives, err)
	}
}

func TestRunningTranscodeSessionIsReusedWithoutDuplicateRecovery(t *testing.T) {
	server := newScannerTestServer(t)
	countPath := configureRecoveryFakeFFmpeg(t, server, "slow")
	settings, request := recoveryTranscodeRequest(t, "movie_recover_slow")
	session := startRecoveryTestTranscode(t, server, settings, request)
	defer session.stop(0)
	waitForRecoveryFakeFFmpegCount(t, countPath, 1)

	reused := server.findReusableTranscodeSession(request.item.ID, request.quality, request.subtitleID, request.startSeconds, request.audioMode, request.audioStreamID, request.directStream, "segment_00000.ts")
	if reused != session {
		t.Fatalf("expected slow running session to be reused instead of replaced")
	}
	if count := readRecoveryFakeFFmpegCount(t, countPath); count != 1 {
		t.Fatalf("expected no duplicate FFmpeg start for slow running session, got %d starts", count)
	}
}

func TestConcurrentTranscodeRecoveryRequestsCoalesce(t *testing.T) {
	server := newScannerTestServer(t)
	countPath := configureRecoveryFakeFFmpeg(t, server, "crash-then-success")
	settings, request := recoveryTranscodeRequest(t, "movie_recover_concurrent")
	failed := startRecoveryTestTranscode(t, server, settings, request)
	waitForTranscodeError(t, failed)

	const callers = 5
	var wg sync.WaitGroup
	results := make([]*transcodeSession, callers)
	errs := make([]error, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = server.recoverTranscodeSessionForDemand(context.Background(), settings, request, failed, failed.err)
		}(i)
	}
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("recovery caller %d failed: %v", index, err)
		}
	}
	recovered := results[0]
	if recovered == nil || recovered == failed {
		t.Fatalf("expected concurrent recovery to return replacement session")
	}
	for index, result := range results {
		if result != recovered {
			t.Fatalf("recovery caller %d received a different session", index)
		}
	}
	waitForRecoveryFakeFFmpegCount(t, countPath, 2)
	if count := readRecoveryFakeFFmpegCount(t, countPath); count != 2 {
		t.Fatalf("expected one recovery FFmpeg start despite concurrent callers, got %d starts", count)
	}
	recovered.stop(0)
}

func TestTranscodeRecoveryAttemptsAreBounded(t *testing.T) {
	server := newScannerTestServer(t)
	countPath := configureRecoveryFakeFFmpeg(t, server, "always-crash")
	settings, request := recoveryTranscodeRequest(t, "movie_recover_terminal")
	current := startRecoveryTestTranscode(t, server, settings, request)
	waitForTranscodeError(t, current)

	for attempt := 0; attempt < maxTranscodeRecoveryAttempts; attempt++ {
		recovered, err := server.recoverTranscodeSessionForDemand(context.Background(), settings, request, current, current.err)
		if err != nil {
			t.Fatalf("attempt %d should start a bounded recovery generation: %v", attempt+1, err)
		}
		current = recovered
		waitForTranscodeError(t, current)
	}
	_, err := server.recoverTranscodeSessionForDemand(context.Background(), settings, request, current, current.err)
	if err == nil {
		t.Fatalf("expected terminal recovery failure after bounded attempts")
	}
	if !strings.Contains(err.Error(), "recovery attempts exhausted") {
		t.Fatalf("terminal recovery error did not explain exhausted attempts: %v", err)
	}
	if current.terminalErr == nil {
		t.Fatalf("expected failed session to be marked terminal")
	}
	if count := readRecoveryFakeFFmpegCount(t, countPath); count != maxTranscodeRecoveryAttempts+1 {
		t.Fatalf("expected initial start plus bounded recovery starts, got %d starts", count)
	}
	current.errAt = time.Now().Add(-time.Minute)
	_, err = server.ensureTranscodeSessionForSegmentWithIntent(request.userID, request.item, request.quality, request.subtitleID, request.startSeconds, request.audioMode, request.audioStreamID, request.directStream, "segment_00000.ts", false)
	if err == nil {
		t.Fatalf("expected terminal session to remain failed after the transient error window")
	}
	if !strings.Contains(err.Error(), "recovery attempts exhausted") {
		t.Fatalf("terminal ensure error did not preserve exhausted state: %v", err)
	}
	if count := readRecoveryFakeFFmpegCount(t, countPath); count != maxTranscodeRecoveryAttempts+1 {
		t.Fatalf("terminal session should not start a new FFmpeg after aging out, got %d starts", count)
	}
}

func TestFindReusableTranscodeSessionCanServeStoppedBufferedSegment(t *testing.T) {
	server := newScannerTestServer(t)
	dir := t.TempDir()
	segment := filepath.Join(dir, "segment_00142.ts")
	if err := os.WriteFile(segment, []byte("segment"), 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}
	done := make(chan struct{})
	close(done)
	session := &transcodeSession{
		mediaID:        "movie_buffered",
		quality:        "720p-medium",
		subtitleID:     "",
		audioMode:      "",
		directStream:   false,
		start:          560,
		dir:            dir,
		done:           done,
		stopped:        true,
		segmentSeconds: hlsSegmentSeconds,
	}
	server.transcodes = map[string]*transcodeSession{"buffered": session}
	if found := server.findReusableTranscodeSession("movie_buffered", "720p-medium", "", 568, "", "", false, "segment_00142.ts"); found != session {
		t.Fatalf("expected stopped session with buffered segment to be reusable")
	}
	if found := server.findReusableTranscodeSession("movie_buffered", "720p-medium", "", 572, "", "", false, "segment_00143.ts"); found != nil {
		t.Fatalf("missing stopped segment should not be reusable")
	}
}

func TestTranscodeBufferedAheadSecondsTracksLastServedSegment(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "index.m3u8")
	if err := os.WriteFile(manifest, []byte("#EXTM3U\n#EXTINF:4,\nsegment_00141.ts\n#EXTINF:4,\nsegment_00142.ts\n#EXTINF:4,\nsegment_00143.ts\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	session := &transcodeSession{manifest: manifest, segmentSeconds: hlsSegmentSeconds, lastServedSegment: -1}
	if got := transcodeBufferedAheadSeconds(session); got != 12 {
		t.Fatalf("unserved buffer = %d, expected 12", got)
	}
	session.lastServedSegment = 141
	if got := transcodeBufferedAheadSeconds(session); got != 8 {
		t.Fatalf("served buffer = %d, expected 8", got)
	}
	session.lastServedSegment = 143
	if got := transcodeBufferedAheadSeconds(session); got != 0 {
		t.Fatalf("fully consumed buffer = %d, expected 0", got)
	}
}

func TestCleanupPlayedTranscodeSegmentsKeepsFiveMinutePlayedBuffer(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"segment_00000.ts", "segment_00001.ts", "segment_00074.ts", "segment_00075.ts", "segment_00076.ts", "init.mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("segment"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	session := &transcodeSession{dir: dir, start: 120, segmentSeconds: 4, playedRetentionSeconds: 300, lastServedSegment: -1}
	removed, err := cleanupPlayedTranscodeSegments(session, "segment_00076.ts")
	if err != nil {
		t.Fatalf("cleanup played segments: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, expected 1", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "segment_00000.ts")); !os.IsNotExist(err) {
		t.Fatalf("old segment should be removed, stat err=%v", err)
	}
	for _, name := range []string{"segment_00001.ts", "segment_00074.ts", "segment_00075.ts", "segment_00076.ts", "init.mp4"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s should be retained: %v", name, err)
		}
	}
}

func TestCleanupPlayedTranscodeSegmentsCleansStartZeroSegmentsAfterRetention(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"segment_00000.ts", "segment_00001.ts", "segment_00074.ts", "segment_00075.ts", "segment_00076.ts"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("segment"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	session := &transcodeSession{dir: dir, start: 0, segmentSeconds: 4, playedRetentionSeconds: 8, lastServedSegment: -1}
	removed, err := cleanupPlayedTranscodeSegments(session, "segment_00076.ts")
	if err != nil {
		t.Fatalf("cleanup played segments: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, expected 2", removed)
	}
	for _, name := range []string{"segment_00000.ts", "segment_00001.ts"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed after retention window: %v", name, err)
		}
	}
	for _, name := range []string{"segment_00074.ts", "segment_00075.ts", "segment_00076.ts"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s should be retained: %v", name, err)
		}
	}
}

func TestCleanupPlayedTranscodeSegmentsHonorsConfigurableRetention(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"segment_00000.m4s", "segment_00001.m4s", "segment_00002.m4s", "segment_00004.m4s"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("segment"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	session := &transcodeSession{dir: dir, start: 120, segmentSeconds: 4, playedRetentionSeconds: 8, lastServedSegment: -1}
	removed, err := cleanupPlayedTranscodeSegments(session, "segment_00004.m4s")
	if err != nil {
		t.Fatalf("cleanup played segments: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, expected 2", removed)
	}
	for _, name := range []string{"segment_00000.m4s", "segment_00001.m4s"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed, stat err=%v", name, err)
		}
	}
	for _, name := range []string{"segment_00002.m4s", "segment_00004.m4s"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s should be retained: %v", name, err)
		}
	}
}

func TestTranscodeVideoFilterAppliesToneMappingOnlyForHDR(t *testing.T) {
	preset := transcodePresets["720p-medium"]
	settings := transcodeSettings{HDRToneMapping: true, HDRToneMappingFilters: true}
	hdr := MediaItem{Streams: []Stream{{Kind: "video", DisplayTitle: "HEVC 3840x2160 HDR"}}}
	filter := transcodeVideoFilter(preset, hdr, settings)
	if !strings.Contains(filter, "zscale=") || !strings.Contains(filter, "smpte2084") || !strings.Contains(filter, "bt709") {
		t.Fatalf("expected HDR filter to include explicit zscale HDR to SDR conversion, got %s", filter)
	}
	filter = transcodeVideoFilter(preset, hdr, transcodeSettings{HDRToneMapping: true})
	if strings.Contains(filter, "zscale") || strings.Contains(filter, "tonemap") || !strings.Contains(filter, "format=yuv420p") {
		t.Fatalf("expected HDR fallback filter without zscale/tonemap, got %s", filter)
	}
	sdr := MediaItem{Streams: []Stream{{Kind: "video", DisplayTitle: "H264 1920x1080"}}}
	filter = transcodeVideoFilter(preset, sdr, settings)
	if strings.Contains(filter, "tonemap") {
		t.Fatalf("expected SDR filter to skip tonemap, got %s", filter)
	}
}

func TestSubtitleBurnInMapsEmbeddedAndExternalStreams(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("not real video"), 0o600); err != nil {
		t.Fatalf("write media: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	item := MediaItem{
		ID:        "movie",
		LibraryID: library.ID,
		SourceURL: mediaPath,
		Streams: []Stream{
			{ID: "movie_probe_3", SourceKind: "ffprobe", Index: 3, Kind: "subtitle", Codec: "ass", DisplayTitle: "ASS"},
			{ID: "movie_probe_4", SourceKind: "ffprobe", Index: 4, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", DisplayTitle: "PGS"},
			{ID: "sidecar_test", Kind: "subtitle", Codec: "webvtt", DisplayTitle: "Sidecar", SourceURL: "/api/media/movie/subtitles/sidecar_test"},
		},
	}
	if _, err := server.db.Exec(`INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url) VALUES (?, ?, 'movie', 'Movie', 'Movie', ?, ?)`, item.ID, library.ID, time.Now().UTC().Format(time.RFC3339), mediaPath); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_streams (id, media_id, kind, codec, display_title, source_url) VALUES (?, ?, 'subtitle', 'webvtt', 'Sidecar', ?)`, "sidecar_test", item.ID, "/api/media/movie/subtitles/sidecar_test"); err != nil {
		t.Fatalf("insert stream: %v", err)
	}
	dir := filepath.Join(server.cfg.AppDataDir, "subtitles", safePathComponent(item.ID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create subtitles dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sidecar_test.vtt"), []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n"), 0o600); err != nil {
		t.Fatalf("write vtt: %v", err)
	}

	embedded, err := server.subtitleBurnInForTranscode(item, mediaPath, "movie_probe_3")
	if err != nil {
		t.Fatalf("embedded burn-in: %v", err)
	}
	if embedded.streamIndex != 3 || embedded.subtitleOrdinal != 0 || embedded.external || embedded.imageBased {
		t.Fatalf("embedded burn-in spec = %#v", embedded)
	}
	if !strings.Contains(embedded.videoFilter(mediaPath), "stream_index=0") {
		t.Fatalf("embedded filter did not target subtitle ordinal: %s", embedded.videoFilter(mediaPath))
	}
	image, err := server.subtitleBurnInForTranscode(item, mediaPath, "movie_probe_4")
	if err != nil {
		t.Fatalf("image burn-in: %v", err)
	}
	if !image.imageBased || image.streamIndex != 4 {
		t.Fatalf("image burn-in spec = %#v", image)
	}
	external, err := server.subtitleBurnInForTranscode(item, mediaPath, "sidecar_test")
	if err != nil {
		t.Fatalf("external burn-in: %v", err)
	}
	if !external.external || !strings.Contains(external.videoFilter(mediaPath), "sidecar_test.vtt") {
		t.Fatalf("external burn-in spec/filter = %#v %s", external, external.videoFilter(mediaPath))
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET subtitle_offset_ms = 750 WHERE id = 'sidecar_test'`); err != nil {
		t.Fatalf("set subtitle offset: %v", err)
	}
	offsetExternal, err := server.subtitleBurnInForTranscode(item, mediaPath, "sidecar_test")
	if err != nil {
		t.Fatalf("offset external burn-in: %v", err)
	}
	if !offsetExternal.external || !strings.Contains(offsetExternal.path, "sidecar_test_offset_750.vtt") {
		t.Fatalf("offset external burn-in spec = %#v", offsetExternal)
	}
	offsetBytes, err := os.ReadFile(offsetExternal.path)
	if err != nil {
		t.Fatalf("read offset vtt: %v", err)
	}
	if !strings.Contains(string(offsetBytes), "00:00:01.750 --> 00:00:02.750") {
		t.Fatalf("offset vtt was not shifted: %s", offsetBytes)
	}
}

func TestHardwareVideoEncoderSelection(t *testing.T) {
	cases := map[string]string{
		"videotoolbox": "h264_videotoolbox",
		"qsv":          "h264_qsv",
		"cuda":         "h264_nvenc",
		"vaapi":        "h264_vaapi",
		"auto":         "",
	}
	for input, expected := range cases {
		if actual := hardwareVideoEncoder(input); actual != expected {
			t.Fatalf("hardwareVideoEncoder(%q) = %q, expected %q", input, actual, expected)
		}
	}
}

func TestAutoHardwareDeviceSelectsAvailableEncoder(t *testing.T) {
	tempDir := t.TempDir()
	ffmpegPath := filepath.Join(tempDir, "ffmpeg")
	script := "#!/bin/sh\nfor arg in \"$@\"; do if [ \"$arg\" = \"-encoders\" ]; then printf '%s\\n' ' V....D h264_qsv Intel Quick Sync Video H.264 Encoder'; fi; done\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	if actual := resolveHardwareDevice("auto", ffmpegPath); actual != "qsv" {
		t.Fatalf("resolveHardwareDevice(auto) = %q, expected qsv", actual)
	}
	if actual := resolveHardwareDevice("nvidia", ffmpegPath); actual != "cuda" {
		t.Fatalf("resolveHardwareDevice(nvidia) = %q, expected cuda", actual)
	}
}

func TestHardwareEncodingArgsAvoidX264OnlyOptions(t *testing.T) {
	preset := transcodePresets["720p-medium"]
	settings := transcodeSettings{X264Preset: "veryfast"}
	software := strings.Join(videoEncodingArgs("libx264", preset, settings), "\n")
	if !strings.Contains(software, "-preset\nveryfast") || !strings.Contains(software, "-crf\n") {
		t.Fatalf("software args should include x264 quality options:\n%s", software)
	}
	hardware := strings.Join(videoEncodingArgs("h264_videotoolbox", preset, settings), "\n")
	if strings.Contains(hardware, "-preset") || strings.Contains(hardware, "-crf") {
		t.Fatalf("hardware args should not include x264-only options:\n%s", hardware)
	}
	if !strings.Contains(hardware, "-b:v\n") || strings.Contains(hardware, "-realtime") {
		t.Fatalf("hardware args should include bitrate without realtime throttling:\n%s", hardware)
	}
}

func TestDirectStreamRemuxPolicyMarksCompatibleVideoForDirectStream(t *testing.T) {
	server := newScannerTestServer(t)
	decision := PlaybackDecision{
		Mode:              "transcode_required",
		Reason:            "container is not in the client profile",
		RequiresTranscode: true,
	}
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "h264", Width: 1920, Height: 1080}, {Kind: "audio", Codec: "eac3"}}}
	next := server.applyDirectStreamRemuxPolicy(decision, item, PlaybackClientProfile{SupportsHLS: true})
	if next.RequiresTranscode || !next.RequiresRemux || next.Mode != "direct_stream" || !strings.Contains(next.Reason, "compatible video") {
		t.Fatalf("expected direct stream decision, got %+v", next)
	}
}

func TestDirectStreamRemuxPolicyAllowsAudioOnlyTranscode(t *testing.T) {
	server := newScannerTestServer(t)
	decision := PlaybackDecision{
		Mode:              "transcode_required",
		Reason:            "audio channels exceed the client profile",
		RequiresTranscode: true,
		AudioTranscode:    true,
	}
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "h264", Width: 1920, Height: 1080}, {Kind: "audio", Codec: "aac", Channels: 6}}}
	next := server.applyDirectStreamRemuxPolicy(decision, item, PlaybackClientProfile{SupportsHLS: true})
	if !next.RequiresTranscode || !next.RequiresRemux || next.Mode != "direct_stream" || !next.AudioTranscode || next.VideoTranscode {
		t.Fatalf("expected direct stream with audio-only transcode, got %+v", next)
	}
}

func TestDirectStreamRemuxPolicyKeepsFullTranscodeForVideoConversion(t *testing.T) {
	server := newScannerTestServer(t)
	decision := PlaybackDecision{
		Mode:              "transcode_required",
		Reason:            "video codec requires conversion",
		RequiresTranscode: true,
		VideoTranscode:    true,
	}
	item := MediaItem{Streams: []Stream{{Kind: "video", Codec: "hevc", Width: 1920, Height: 1080}}}
	next := server.applyDirectStreamRemuxPolicy(decision, item, PlaybackClientProfile{SupportsHLS: true})
	if !next.RequiresTranscode || next.Mode != "transcode_required" {
		t.Fatalf("expected full transcode decision, got %+v", next)
	}
}

func TestHLSAudioEncodingCopiesAppleCompatibleAudio(t *testing.T) {
	preset := transcodePresets["1080p-medium"]
	item := MediaItem{Streams: []Stream{{Kind: "audio", Codec: "eac3", Channels: 6, Bitrate: 640_000}}}
	args := strings.Join(hlsAudioEncodingArgs(item, preset, false, ""), " ")
	if args != "-c:a copy" {
		t.Fatalf("expected compatible Apple audio to be copied, got %s", args)
	}
	args = strings.Join(hlsAudioEncodingArgs(item, preset, true, ""), " ")
	if !strings.Contains(args, "-c:a aac") || !strings.Contains(args, "-ac 2") {
		t.Fatalf("expected forced audio transcode to stereo AAC, got %s", args)
	}
	item.Streams[0].Codec = "truehd"
	args = strings.Join(hlsAudioEncodingArgs(item, preset, false, ""), " ")
	if !strings.Contains(args, "-c:a aac") || !strings.Contains(args, "-ac 2") {
		t.Fatalf("expected incompatible audio to be transcoded to stereo AAC, got %s", args)
	}
}

func TestEndPlaybackSessionStopsForcedTranscodeForDirectPlaySession(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, added_at) VALUES ('movie_transcode', 'movie', 'Movie', 'Movie', ?)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, decision)
		VALUES ('play_direct_forced', ?, ?, 'movie_transcode', 'movie', 'Movie', ?, ?, 'Direct Play')`,
		accountIDForUser(user), viewerProfileID(user), now, now); err != nil {
		t.Fatalf("insert playback: %v", err)
	}
	session := &transcodeSession{mediaID: "movie_transcode", quality: "720p-high"}
	server.transcodes["movie_transcode:720p-high:"] = session

	if err := server.endPlaybackSession(user, "play_direct_forced"); err != nil {
		t.Fatalf("end playback: %v", err)
	}
	if !session.stopped {
		t.Fatalf("expected forced transcode session to be stopped")
	}
	if len(server.transcodes) != 0 {
		t.Fatalf("expected transcode registry to be empty, got %#v", server.transcodes)
	}
}

func TestEndPlaybackSessionRemovesTranscodeTempFiles(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, added_at) VALUES ('movie_cleanup', 'movie', 'Movie', 'Movie', ?)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, decision)
		VALUES ('play_cleanup', ?, ?, 'movie_cleanup', 'movie', 'Movie', ?, ?, 'Transcode')`,
		accountIDForUser(user), viewerProfileID(user), now, now); err != nil {
		t.Fatalf("insert playback: %v", err)
	}
	root := t.TempDir()
	sessionDir := filepath.Join(root, "movie_cleanup", "720p-medium", "start_000000", "video_transcode", "audio_copy")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("create transcode dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "segment_00000.ts"), []byte("segment"), 0o600); err != nil {
		t.Fatalf("write transcode segment: %v", err)
	}
	session := &transcodeSession{mediaID: "movie_cleanup", quality: "720p-medium", root: root, dir: sessionDir}
	server.transcodes["movie_cleanup:720p-medium:"] = session

	if err := server.endPlaybackSession(user, "play_cleanup"); err != nil {
		t.Fatalf("end playback: %v", err)
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("expected transcode session dir to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "movie_cleanup")); !os.IsNotExist(err) {
		t.Fatalf("expected empty media transcode parent to be removed, stat err=%v", err)
	}
}

func TestExpireStalePlaybackSessionsStopsOrphanedTranscode(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC()
	stale := now.Add(-2 * time.Minute).Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, added_at) VALUES ('movie_stale', 'movie', 'Movie', 'Movie', ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, decision)
		VALUES ('play_stale', ?, ?, 'movie_stale', 'movie', 'Movie', ?, ?, 'Direct Play')`,
		accountIDForUser(user), viewerProfileID(user), stale, stale); err != nil {
		t.Fatalf("insert playback: %v", err)
	}
	session := &transcodeSession{mediaID: "movie_stale", quality: "1080p-medium"}
	server.transcodes["movie_stale:1080p-medium:"] = session

	if err := server.expireStalePlaybackSessions(now); err != nil {
		t.Fatalf("expire stale playback: %v", err)
	}
	if !session.stopped {
		t.Fatalf("expected stale transcode session to be stopped")
	}
}

func TestUserTranscodeLimitPreventsOneUserFromMonopolizingCapacity(t *testing.T) {
	server := newScannerTestServer(t)
	settings := transcodeSettings{Enabled: true, MaxConcurrentSessions: 8}
	server.transcodes = map[string]*transcodeSession{
		"movie_a:720p-medium:": runningTranscodeForUser("user_a", "movie_a"),
		"movie_b:720p-medium:": runningTranscodeForUser("user_a", "movie_b"),
		"movie_c:720p-medium:": runningTranscodeForUser("user_b", "movie_c"),
	}
	server.transcodeMu.Lock()
	if server.userCanStartTranscodeLocked("user_a", settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("user_a should be at the per-user transcode cap")
	}
	if !server.userCanStartTranscodeLocked("user_b", settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("user_b should still have one transcode slot")
	}
	if !server.userCanStartTranscodeLocked("user_c", settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("user_c should be able to start a transcode")
	}
	server.transcodeMu.Unlock()
}

func TestUnlimitedTranscodeLimitDisablesGlobalAndUserAdmissionCaps(t *testing.T) {
	server := newScannerTestServer(t)
	settings := transcodeSettings{Enabled: true, MaxConcurrentSessions: 0}
	server.transcodes = map[string]*transcodeSession{
		"movie_a:720p-medium:": runningTranscodeForUser("user_a", "movie_a"),
		"movie_b:720p-medium:": runningTranscodeForUser("user_a", "movie_b"),
		"movie_c:720p-medium:": runningTranscodeForUser("user_a", "movie_c"),
	}
	server.transcodeMu.Lock()
	if !server.userCanStartTranscodeLocked("user_a", settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("unlimited transcode capacity should not enforce the per-user cap")
	}
	server.transcodeMu.Unlock()
	if maxConcurrentTranscodesPerUser(settings) != 0 {
		t.Fatalf("unlimited transcode capacity should report unlimited per-user capacity")
	}
}

func TestHardwareEncodeOverflowFallsBackToSoftwareSlots(t *testing.T) {
	server := newScannerTestServer(t)
	settings := transcodeSettings{Enabled: true, MaxConcurrentSessions: 10, HardwareEncoding: true, HardwareDevice: "videotoolbox", MaxHardwareSessions: 2}
	server.transcodes = map[string]*transcodeSession{
		"movie_a:720p-medium:": runningHardwareTranscodeForUser("user_a", "movie_a"),
		"movie_b:720p-medium:": runningHardwareTranscodeForUser("user_b", "movie_b"),
	}
	server.transcodeMu.Lock()
	if server.hardwareEncodeSlotAvailableLocked(settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("hardware encoder should be considered full before global transcode capacity is full")
	}
	if !server.userCanStartTranscodeLocked("user_c", settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("software transcode overflow should still be allowed")
	}
	server.transcodeMu.Unlock()
}

func TestBackgroundTranscodeSlotsAreBounded(t *testing.T) {
	server := newScannerTestServer(t)
	settings := transcodeSettings{Enabled: true, MaxConcurrentSessions: 10, MaxBackgroundSessions: 1}
	server.transcodes = map[string]*transcodeSession{
		"movie_background:720p-medium:": runningBackgroundTranscodeForUser("user_a", "movie_background"),
	}
	server.transcodeMu.Lock()
	if server.backgroundTranscodeSlotAvailableLocked(settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("background prewarm should be blocked when the background slot is full")
	}
	if !server.userCanStartTranscodeLocked("user_b", settings) {
		server.transcodeMu.Unlock()
		t.Fatalf("foreground playback transcode should not be blocked by another user's background prewarm slot")
	}
	server.transcodeMu.Unlock()
}

func TestServerShutdownStopsAndJoinsLegacyTranscodes(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	session := &transcodeSession{cmd: cmd, done: done, updateCh: make(chan struct{})}
	server := &Server{transcodes: map[string]*transcodeSession{"legacy": session}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.shutdownLegacyTranscodes(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("legacy transcode was not joined before shutdown returned")
	}
	if len(server.transcodes) != 0 {
		t.Fatal("legacy transcode remained published after shutdown")
	}
}

func TestForegroundSeekSupersedesStaleProducerWithoutTouchingOtherPlayback(t *testing.T) {
	server := newScannerTestServer(t)
	startRunning := func(userID, mediaID string, start int, lastProduced int) *transcodeSession {
		t.Helper()
		cmd := exec.Command("sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start seek fixture: %v", err)
		}
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		return &transcodeSession{
			userID:              userID,
			mediaID:             mediaID,
			quality:             "720p-medium",
			start:               start,
			cmd:                 cmd,
			done:                done,
			segmentSeconds:      hlsSegmentSeconds,
			lastProducedSegment: lastProduced,
		}
	}
	stale := startRunning("user_a", "movie_seek", 0, 8)
	otherUser := startRunning("user_b", "movie_seek", 0, 8)
	nearby := startRunning("user_a", "movie_seek", 400, 101)
	defer otherUser.stop(0)
	defer nearby.stop(0)
	server.transcodes = map[string]*transcodeSession{
		"stale":      stale,
		"other-user": otherUser,
		"nearby":     nearby,
	}

	server.transcodeMu.Lock()
	superseded := server.supersedeStaleSeekTranscodesLocked("user_a", "movie_seek", "720p-medium", "", "", "", false, "segment_00102.ts", "new-seek")
	server.transcodeMu.Unlock()

	if len(superseded) != 1 || superseded[0] != stale {
		t.Fatalf("superseded = %#v, expected only stale producer", superseded)
	}
	if !stale.stopped {
		t.Fatalf("stale seek producer was not interrupted")
	}
	if _, exists := server.transcodes["stale"]; exists {
		t.Fatalf("stale seek producer remained in admission registry")
	}
	if server.transcodes["other-user"] != otherUser || server.transcodes["nearby"] != nearby {
		t.Fatalf("unrelated or nearby playback was superseded")
	}
}

func TestExpectedBackgroundTranscodeRefusalsAreRecognized(t *testing.T) {
	expected := []error{
		errors.New("background transcode prewarm deferred while server is under load"),
		errors.New("the background transcode session limit has been reached"),
	}
	for _, err := range expected {
		if !isExpectedBackgroundTranscodeRefusal(err) {
			t.Fatalf("expected background refusal was not recognized: %v", err)
		}
	}
	unexpected := []error{
		nil,
		errors.New("FFmpeg is not available on PATH"),
		errors.New("the transcode session limit has been reached"),
		errors.New("the user transcode session limit has been reached"),
	}
	for _, err := range unexpected {
		if isExpectedBackgroundTranscodeRefusal(err) {
			t.Fatalf("unexpected background refusal classification for: %v", err)
		}
	}
}

func runningTranscodeForUser(userID, mediaID string) *transcodeSession {
	return &transcodeSession{
		userID:  userID,
		mediaID: mediaID,
		quality: "720p-medium",
		cmd:     &exec.Cmd{Process: &os.Process{Pid: 1}},
		done:    make(chan struct{}),
	}
}

func runningHardwareTranscodeForUser(userID, mediaID string) *transcodeSession {
	session := runningTranscodeForUser(userID, mediaID)
	session.method = "hardware-encode"
	return session
}

func runningBackgroundTranscodeForUser(userID, mediaID string) *transcodeSession {
	session := runningTranscodeForUser(userID, mediaID)
	session.background = true
	return session
}

func TestPlaybackDiagnosticsPersistProfileAndRedactedTranscodeContext(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO media_items (id, type, title, sort_title, added_at) VALUES ('movie_diag', 'movie', 'Movie', 'Movie', ?)`, now); err != nil {
		t.Fatalf("insert media: %v", err)
	}
	item := MediaItem{ID: "movie_diag", Type: "movie", Title: "Movie"}
	profile := PlaybackClientProfile{Device: "Chrome", Platform: "macOS", SupportsHLS: true, SupportsMSE: true, MaxBitrate: 8000}
	decision := PlaybackDecision{Mode: "transcode_required", Reason: "codec mismatch", Protocol: "hls", Container: "hls", VideoCodec: "hevc", AudioCodec: "aac", RequiresTranscode: true}
	decision = playbackDecisionWithTestPlan(t, decision, item.ID, "burn_in", "sub_1")
	req := httptest.NewRequest("GET", "/api/playback-sessions", nil)
	if err := server.createPlaybackSession(req, user, item, "play_diag", decision, profile, PlaybackIntent{}, "sub_1", "", false, "test-client", PlaybackSourceContext{}, "off"); err != nil {
		t.Fatalf("create playback session: %v", err)
	}
	rawContext := redactedFFmpegContext([]string{"-hide_banner", "-i", "/Users/justin/Movies/Movie.mkv", "-vf", "scale=w=1280:h=720,subtitles=filename='/Users/justin/Subs/Movie.srt':stream_index=0", "-hls_segment_filename", "/tmp/portico/segment_%05d.ts", "/tmp/portico/index.m3u8"})
	server.updatePlaybackTranscodeDiagnostics("movie_diag", PlaybackDiagnostics{TranscodeQuality: "720p-medium", TranscodeMethod: "software", TranscodeFilter: redactedTranscodeFilter("scale=w=1280:h=720,subtitles=filename='/Users/justin/Subs/Movie.srt':stream_index=0"), FFmpegContext: rawContext, SubtitleBurnIn: true, SubtitleBurnInReason: "External subtitle burn-in"})

	sessions, err := server.loadDashboardPlaybackSessions(User{ID: user.ID, Permissions: map[string]bool{"manageServer": true}}, dashboardFilters{Mode: "live", Since: time.Now().Add(-time.Hour)}, time.Now())
	if err != nil {
		t.Fatalf("load sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions))
	}
	diagnostics := sessions[0].Diagnostics
	if diagnostics.ClientProfile != "Chrome · macOS · HLS+MSE" || diagnostics.MaxBitrateMbps != 8 || diagnostics.DecisionReason != "codec mismatch" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if !diagnostics.SubtitleBurnIn || diagnostics.TranscodeQuality != "720p-medium" || diagnostics.TranscodeMethod != "software" {
		t.Fatalf("transcode diagnostics were not merged: %#v", diagnostics)
	}
	if strings.Contains(diagnostics.FFmpegContext, "/Users/justin") || strings.Contains(diagnostics.TranscodeFilter, "/Users/justin") {
		t.Fatalf("diagnostics leaked local paths: %#v", diagnostics)
	}
	if !strings.Contains(diagnostics.FFmpegContext, "<media-source>") || !strings.Contains(diagnostics.FFmpegContext, "<transcode-segment>") {
		t.Fatalf("ffmpeg context was not redacted usefully: %s", diagnostics.FFmpegContext)
	}
}

func TestTranscodeSessionSpeedMultiplierUsesProducedSegments(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	session := &transcodeSession{
		start:               40,
		startedAt:           now.Add(-6 * time.Second),
		segmentSeconds:      4,
		lastProducedSegment: 12,
	}

	if speed := transcodeSessionSpeedMultiplier(session, now); speed != 2 {
		t.Fatalf("speed multiplier = %.1f, expected 2.0", speed)
	}

	session.lastProducedSegment = 9
	if speed := transcodeSessionSpeedMultiplier(session, now); speed != 0 {
		t.Fatalf("speed multiplier before first produced segment = %.1f, expected 0", speed)
	}
}
