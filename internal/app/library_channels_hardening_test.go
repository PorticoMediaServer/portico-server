package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/librarychannels"
	_ "modernc.org/sqlite"
)

func TestLibraryChannelReplacementSeedRequiresExplicitReshuffle(t *testing.T) {
	const existing = "stable-programming-seed"
	if actual := libraryChannelReplacementSeed(existing, false); actual != existing {
		t.Fatalf("ordinary replacement changed seed: %q", actual)
	}
	if actual := libraryChannelReplacementSeed(existing, true); actual == existing || !strings.HasPrefix(actual, "seed_") {
		t.Fatalf("explicit reshuffle did not mint a new seed: %q", actual)
	}
}

func TestLibraryChannelSegmentQueryRejectsClientControlledPlayoutFields(t *testing.T) {
	for _, rawURL := range []string{
		"/api/library-channels/channel/hls/segment?at=1700000000&quality=original",
		"/api/library-channels/channel/hls/segment?at=1700000000&index=999999",
		"/api/library-channels/channel/hls/segment?at=1700000000&entry=other",
		"/api/library-channels/channel/hls/segment?at=1700000000&start=999999",
		"/api/library-channels/channel/hls/segment?at=1700000000&at=1700000008",
	} {
		if validLibraryChannelSegmentQuery(httptest.NewRequest(http.MethodGet, rawURL, nil)) {
			t.Fatalf("accepted client-controlled Library Channel playout query %q", rawURL)
		}
	}
	if !validLibraryChannelSegmentQuery(httptest.NewRequest(http.MethodGet, "/api/library-channels/channel/hls/segment?at=1700000000", nil)) {
		t.Fatal("rejected server-issued segment timeline")
	}
	if validLibraryChannelSegmentQuery(httptest.NewRequest(http.MethodGet, "/api/library-channels/channel/hls/segment?at=1700000000&media_grant=ptc_mg_example", nil)) {
		t.Fatal("accepted a media grant in the query string")
	}
}

func TestLibraryChannelQualityOnlyNarrowsResolvedPlaybackPolicy(t *testing.T) {
	if quality := resolvedLibraryChannelQuality("1080p-high", ResolvedPlaybackPolicy{DeliveryProfile: "720p-high"}); quality != "720p-high" {
		t.Fatalf("remote delivery profile was widened to %q", quality)
	}
	if quality := resolvedLibraryChannelQuality("original", ResolvedPlaybackPolicy{DeliveryProfile: "original", MaxVideoBitrateMbps: 4}); quality != "720p-medium" {
		t.Fatalf("account bitrate clamp resolved to %q", quality)
	}
	if quality := resolvedLibraryChannelQuality("480p", ResolvedPlaybackPolicy{DeliveryProfile: "1080p-high"}); quality != "480p" {
		t.Fatalf("channel quality clamp was widened to %q", quality)
	}
}

func TestLibraryChannelProblemsCarrySharedProductLanguageMessageID(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeLibraryChannelError(recorder, librarychannels.ErrGenerationInProgress)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"messageId":"library-channel.generation-in-progress"`) {
		t.Fatalf("problem response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestLibraryChannelConsumerSummaryOmitsAdministrationDocument(t *testing.T) {
	channel := librarychannels.Channel{
		ID: "channel-private-config", Name: "Safe Channel", Description: "Viewer description", Enabled: true,
		Timezone: "America/Halifax", DefaultRuleID: "rule-private", QualityProfile: "720p-medium",
		TemplateKey: "classic-cinema", TemplateVersion: 3, ConfigRevision: 9, Seed: "private-seed",
		Logo: librarychannels.LogoConfig{BugEnabled: true, BugOverheadAccepted: true},
	}
	encoded, err := json.Marshal(libraryChannelSummaryDocumentFrom(channel, User{Permissions: map[string]bool{"playLiveTV": true}}))
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"rule-private", "private-seed", "templateKey", "templateVersion", "configRevision", "qualityProfile", "bugEnabled", "bugOverheadAccepted", "timezone", "healthState"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("consumer Library Channel summary exposed %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"id":"channel-private-config"`) || !strings.Contains(body, `"name":"Safe Channel"`) {
		t.Fatalf("consumer summary omitted identity: %s", body)
	}
}

func TestLibraryChannelGuideAccessIsOneSetQueryAtTwentyThousandEntries(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE library_channels(id TEXT PRIMARY KEY,enabled INTEGER,active_generation_id TEXT);
		CREATE TABLE library_channel_schedule_entries(channel_id TEXT,generation_id TEXT,media_id TEXT,starts_at INTEGER,ends_at INTEGER);
		CREATE TABLE media_items(id TEXT PRIMARY KEY,library_id TEXT,content_rating TEXT);
		CREATE TABLE media_availability(media_id TEXT PRIMARY KEY,file_count INTEGER,missing_file_count INTEGER);
		INSERT INTO library_channels VALUES('channel',1,'generation');
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mediaInsert, _ := tx.Prepare(`INSERT INTO media_items(id,library_id,content_rating) VALUES(?,'library','')`)
	availabilityInsert, _ := tx.Prepare(`INSERT INTO media_availability(media_id,file_count,missing_file_count) VALUES(?,?,?)`)
	entryInsert, _ := tx.Prepare(`INSERT INTO library_channel_schedule_entries(channel_id,generation_id,media_id,starts_at,ends_at) VALUES('channel','generation',?,0,4102444800)`)
	defer mediaInsert.Close()
	defer availabilityInsert.Close()
	defer entryInsert.Close()
	for index := 0; index < 20_000; index++ {
		mediaID := fmt.Sprintf("media-%05d", index)
		if _, err := mediaInsert.Exec(mediaID); err != nil {
			t.Fatal(err)
		}
		missing := 0
		if index%2 == 1 {
			missing = 1
		}
		if _, err := availabilityInsert.Exec(mediaID, 1, missing); err != nil {
			t.Fatal(err)
		}
		if _, err := entryInsert.Exec(mediaID); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	server := &Server{db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	decisions, err := server.libraryChannelGuideAccessDecisions(context.Background(), User{}, "channel", time.Unix(0, 0), time.Unix(4_102_444_800, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 20_000 || decisions["media-00000"] != librarychannels.AccessAllowed || decisions["media-00001"] != librarychannels.AccessUnavailable {
		t.Fatalf("unexpected decisions: count=%d first=%v second=%v", len(decisions), decisions["media-00000"], decisions["media-00001"])
	}
	if reads := server.sqliteMetrics.ReadOperations; reads != 1 {
		t.Fatalf("guide visibility used %d reads for 20,000 programs; expected one set query", reads)
	}
}

func TestLibraryChannelSegmentCachePrunesByAgeAndProtectsInUseFiles(t *testing.T) {
	server := newScannerTestServer(t)
	root := server.libraryChannelSegmentCacheRoot()
	stalePath := filepath.Join(root, "channel-cache", "1", "entry", "stale.ts")
	pinnedPath := filepath.Join(root, "channel-cache", "1", "entry", "pinned.ts")
	for _, path := range []string{stalePath, pinnedPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("segment"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().UTC().Add(-libraryChannelSegmentCacheMaxAge - time.Minute)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	release := server.pinLibraryChannelSegmentCachePath(pinnedPath)
	if err := server.maybePruneLibraryChannelSegmentCache(time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale cache file survived prune: %v", err)
	}
	if _, err := os.Stat(pinnedPath); err != nil {
		t.Fatalf("in-use cache file was pruned: %v", err)
	}
	status := server.libraryChannelSegmentCacheStatus()
	if status.PinnedFiles != 1 || status.Files != 1 || status.LimitBytes != libraryChannelSegmentCacheMaxBytes || status.LimitFiles != libraryChannelSegmentCacheMaxFiles {
		t.Fatalf("cache status = %#v", status)
	}
	server.removeLibraryChannelSegmentCache("channel-cache", 0)
	if _, err := os.Stat(pinnedPath); err != nil {
		t.Fatalf("in-use channel tree was removed: %v", err)
	}
	release()
	server.removeLibraryChannelSegmentCache("channel-cache", 0)
	if _, err := os.Stat(filepath.Join(root, "channel-cache")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete channel tree survived deletion: %v", err)
	}
}

func TestLibraryChannelOverlayUsesSharedTranscodeAdmissionAndOwnerCapacity(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO settings (key,value_json,updated_at) VALUES ('transcoder','{"enabled":true,"maxConcurrentSessions":2,"maxSoftwareSessions":2}',?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, now); err != nil {
		t.Fatal(err)
	}
	server.transcodes["vod-active"] = &transcodeSession{key: "vod-active", userID: "vod-user", mediaID: "movie", method: "software", done: make(chan struct{}), admissionActive: true}
	user := User{ID: "channel-user", AccountID: "channel-user"}
	channel := librarychannels.Channel{ID: "channel-overlay"}
	release, err := server.acquireLibraryChannelOverlayTranscode(user, channel, "720p-medium", "segment-one")
	if err != nil {
		t.Fatalf("admit overlay beside VOD: %v", err)
	}
	defer release()
	if active := server.activeTranscodeSessionCount(); active != 2 {
		t.Fatalf("owner capacity active sessions=%d, want 2", active)
	}
	if report := server.transcodeCapacityReport(); report.ActiveSessions != 2 || report.AvailableSlots != 0 {
		t.Fatalf("owner transcode report = %#v", report)
	}
	if _, err := server.acquireLibraryChannelOverlayTranscode(user, librarychannels.Channel{ID: "channel-overlay-two"}, "720p-medium", "segment-two"); !errors.Is(err, errLibraryChannelPlaybackCapacity) {
		t.Fatalf("third VOD/channel transcode admission error=%v", err)
	}
}

func TestLibraryChannelOverlayFFmpegDiagnosticsBoundNoisyStderr(t *testing.T) {
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
printf '%s\n' 'warning: https://user:password@example.test/live.m3u8?token=secret' >&2
i=0
while [ "$i" -lt 3000 ]; do
  printf '%s\n' 'decoder warning: invalid packet token=secret' >&2
  i=$((i + 1))
done
exit 23
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake FFmpeg: %v", err)
	}

	report, err := runLibraryChannelOverlayFFmpeg(context.Background(), ffmpegPath, []string{
		"-i", "https://user:password@example.test/live.m3u8?token=secret",
		"-filter_complex", "overlay",
	})
	if err == nil {
		t.Fatal("noisy fake FFmpeg unexpectedly succeeded")
	}
	if report.Bytes <= ffmpegDiagnosticMaxBytes || !report.Truncated {
		t.Fatalf("overlay diagnostics did not record bounded truncation: %+v", report)
	}
	// Sanitization can expand replacement markers slightly after the recorder
	// has bounded the captured bytes, but the returned report remains bounded.
	if len(report.Text) > int(ffmpegDiagnosticMaxBytes)*2 {
		t.Fatalf("overlay diagnostic text length=%d", len(report.Text))
	}
	if !strings.Contains(report.Text, "decoder warning") || !strings.Contains(report.Text, "stderr truncated") {
		t.Fatalf("overlay diagnostic context missing: %q", report.Text)
	}
	if strings.Contains(report.Text, "password") || strings.Contains(report.Text, "secret") {
		t.Fatalf("overlay diagnostic leaked credentials: %q", report.Text)
	}
	if !strings.Contains(report.CommandIdentity, "<media-source>") || strings.Contains(report.CommandIdentity, "secret") {
		t.Fatalf("overlay command identity was not redacted: %q", report.CommandIdentity)
	}
}

func TestLibraryChannelOverlayRestoreQuiescenceCancelsRegisteredSession(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO settings (key,value_json,updated_at) VALUES ('transcoder','{"enabled":true,"maxConcurrentSessions":1,"maxSoftwareSessions":1}',?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, now); err != nil {
		t.Fatal(err)
	}
	release, err := server.acquireLibraryChannelOverlayTranscode(User{ID: "overlay-user", AccountID: "overlay-user"}, librarychannels.Channel{ID: "overlay-quiesce"}, "720p-medium", "segment-quiesce")
	if err != nil {
		t.Fatalf("admit overlay: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.quiesceForRestore(ctx); err != nil {
		t.Fatalf("quiesce overlay: %v", err)
	}
	server.transcodeMu.Lock()
	_, present := server.transcodes["library-overlay:segment-quiesce"]
	server.transcodeMu.Unlock()
	if present {
		t.Fatal("quiescence left the canceled library overlay registered")
	}
	// The caller's normal cleanup remains safe after quiescence has already
	// invoked the same once-guarded function through session.cancel.
	release()
	server.restoreBarrier.unblock()
}
