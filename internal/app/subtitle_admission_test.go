package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func subtitleAdmissionFixture(t *testing.T) (*Server, User, MediaItem) {
	t.Helper()
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("test media"), 0o600); err != nil {
		t.Fatalf("write media fixture: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Subtitle Admission", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	user.LibraryIDs = append(user.LibraryIDs, library.ID)
	item := MediaItem{
		ID:              "movie_subtitle_admission",
		LibraryID:       library.ID,
		Type:            "movie",
		Title:           "Subtitle Admission",
		SourceURL:       mediaPath,
		DurationSeconds: 120,
		Streams: []Stream{
			{ID: "movie_subtitle_admission_probe_2", SourceKind: "ffprobe", Index: 2, Kind: "subtitle", Codec: "ass", Language: "eng", DisplayTitle: "English SDH"},
			{ID: "subtitle_text", Kind: "subtitle", Codec: "webvtt", Language: "eng", DisplayTitle: "English", SourceURL: "/api/media/movie_subtitle_admission/subtitles/subtitle_text"},
			{ID: "audio_main", Kind: "audio", Codec: "aac", Language: "eng", DisplayTitle: "English"},
		},
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.LibraryID, item.Type, item.Title, item.Title, now, item.SourceURL, item.DurationSeconds); err != nil {
		t.Fatalf("insert media fixture: %v", err)
	}
	for _, stream := range item.Streams {
		if _, err := server.db.Exec(`
			INSERT INTO media_streams (id, media_id, kind, codec, language, display_title, source_url)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, stream.ID, item.ID, stream.Kind, stream.Codec, stream.Language, stream.DisplayTitle, stream.SourceURL); err != nil {
			t.Fatalf("insert stream fixture %s: %v", stream.ID, err)
		}
	}
	return server, user, item
}

func TestPlaybackAdmissionRejectsUnknownBurnInSubtitleBeforeSessionCreation(t *testing.T) {
	server, user, item := subtitleAdmissionFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil)
	_, startErr := server.startPlaybackForRequest(req, user, PlaybackSessionCreateRequest{
		MediaID:          item.ID,
		SkipPreroll:      true,
		BurnInSubtitleID: "missing_subtitle",
		Intent:           automaticPlaybackIntent(),
	})
	if startErr == nil || startErr.status != http.StatusBadRequest || startErr.code != "subtitle_stream_not_found" {
		t.Fatalf("unexpected playback admission result: %#v", startErr)
	}
	var sessions int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE media_id = ?`, item.ID).Scan(&sessions); err != nil {
		t.Fatalf("count playback sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("invalid subtitle selection created %d playback sessions", sessions)
	}
}

func TestHLSMasterRejectsUnknownAndConflictingSubtitleSelections(t *testing.T) {
	server, user, item := subtitleAdmissionFixture(t)

	unknown := httptest.NewRecorder()
	server.handleMediaHLSManifest(unknown, httptest.NewRequest(http.MethodGet, "/api/media/"+item.ID+"/hls/master.m3u8?subtitle=missing_subtitle", nil), user, item.ID, true)
	if unknown.Code != http.StatusForbidden || !strings.Contains(unknown.Body.String(), "playback_plan_required") {
		t.Fatalf("unknown burn-in status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	conflict := httptest.NewRecorder()
	server.handleMediaHLSManifest(conflict, httptest.NewRequest(http.MethodGet, "/api/media/"+item.ID+"/hls/master.m3u8?subtitle=movie_subtitle_admission_probe_2&textSubtitle=subtitle_text", nil), user, item.ID, true)
	if conflict.Code != http.StatusForbidden || !strings.Contains(conflict.Body.String(), "playback_plan_required") {
		t.Fatalf("conflicting subtitle modes status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestPlaybackSessionPersistsBurnInSubtitleDiagnostics(t *testing.T) {
	server, user, item := subtitleAdmissionFixture(t)
	profile := PlaybackClientProfile{Device: "Chrome", Platform: "macOS", SupportsHLS: true, SupportsMSE: true}
	decision := PlaybackDecision{Mode: "transcode_required", Reason: "subtitle burn-in requested", Protocol: "hls", RequiresTranscode: true}
	decision = playbackDecisionWithTestPlan(t, decision, item.ID, "burn_in", "movie_subtitle_admission_probe_2")
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil)
	if err := server.createPlaybackSession(req, user, item, "qentry_subtitle_diag", "play_subtitle_diag", decision, profile, PlaybackIntent{}, "movie_subtitle_admission_probe_2", "", false, "subtitle-client", PlaybackSourceContext{}, "off"); err != nil {
		t.Fatalf("create playback session: %v", err)
	}
	var subtitleDecision string
	var diagnosticsJSON string
	if err := server.queryUserRow(context.Background(), `SELECT subtitle_decision, diagnostics_json FROM playback_sessions WHERE id = ?`, "play_subtitle_diag").Scan(&subtitleDecision, &diagnosticsJSON); err != nil {
		t.Fatalf("read playback subtitle diagnostics: %v", err)
	}
	if subtitleDecision != "Burn in: English SDH" {
		t.Fatalf("subtitle decision = %q", subtitleDecision)
	}
	var diagnostics PlaybackDiagnostics
	if err := json.Unmarshal([]byte(diagnosticsJSON), &diagnostics); err != nil {
		t.Fatalf("decode playback diagnostics: %v", err)
	}
	if !diagnostics.SubtitleBurnIn || diagnostics.SubtitleBurnInReason != subtitleDecision {
		t.Fatalf("subtitle diagnostics = %#v", diagnostics)
	}
}
