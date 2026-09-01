package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

func TestResolvePlaybackCapabilitiesUsesOneAuthenticatedTupleSet(t *testing.T) {
	profile := PlaybackClientProfile{CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS", Device: "Apple TV", CapabilityEvidence: []PlaybackCapabilityEvidence{{
		ID: "native:apple-tv", Source: "native_runtime", Confidence: "high", Producer: "portico-apple", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts", Video: PlaybackCapabilityVideo{Codec: "hevc", Profile: "main10", PixelFormat: "yuv420p10le", Chroma: "4:2:0", DynamicRange: "pq", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}, Audio: PlaybackCapabilityAudio{Codec: "eac3", Layout: "5.1", Route: "passthrough", MaxChannels: 6}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}}},
	}}}
	profile.capabilityAuthority = playbackCapabilityAuthority{Source: playbackcap.SourceNativeRuntime, Family: "avkit", Platform: "tvos", DeviceID: "verified-apple-tv", Producer: "portico-native/avkit/tvos", ProducerVersion: playbackCapabilitySchemaV2 + "/18"}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resolution.EvidenceID, "native_runtime:verified-apple-tv:") || resolution.Source != playbackcap.SourceNativeRuntime || len(resolution.Tuples) != 1 {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
}

func TestTrustedHostedWebAttachmentAuthorizesExactFLACRuntimeEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	user.AccountID = user.ID
	if err := server.db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? ORDER BY is_primary DESC, sort_order, id LIMIT 1`, accountIDForUser(user)).Scan(&user.ProfileID); err != nil {
		t.Fatalf("load browser capability profile: %v", err)
	}
	now := time.Now().UTC()
	deviceID := "dev_browser_capability_authority"
	refreshID := "refresh_browser_capability_authority"
	sessionID := "nativesess_" + refreshID
	if _, err := server.db.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, app, platform, trusted, created_at, last_seen_at)
		VALUES (?, ?, 'install-browser-capability-authority', 'Web browser', 'portico-web', 'MacIntel', 1, ?, ?)`,
		deviceID, accountIDForUser(user), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed trusted browser device: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO native_refresh_tokens (id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, created_at, expires_at)
		VALUES (?, 'family-browser-capability-authority', ?, ?, ?, 'portico', 'hash-refresh-browser-capability-authority', ?, ?)`,
		refreshID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Format(time.RFC3339Nano), now.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed Hosted Web attachment refresh credential: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'hash-browser-capability-authority', ?, ?, ?)`,
		sessionID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed browser session: %v", err)
	}
	user.AuthSessionID, user.DeviceID = sessionID, deviceID

	tuples := make([]PlaybackCapabilityTuple, 0, 51)
	for width := 1872; width <= 1920; width++ {
		tuples = append(tuples, PlaybackCapabilityTuple{
			MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts",
			Video:    PlaybackCapabilityVideo{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", DynamicRange: "sdr", BitDepth: 8, MaxWidth: width, MaxHeight: 1080, MaxFrameRate: 60},
			Audio:    PlaybackCapabilityAudio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
			Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
		})
	}
	tuples = append(tuples,
		PlaybackCapabilityTuple{MediaKind: "audio", Protocol: "http", Container: "flac", Audio: PlaybackCapabilityAudio{Codec: "flac", Layout: "mono", Route: "decode", MaxChannels: 1}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}},
		PlaybackCapabilityTuple{MediaKind: "audio", Protocol: "http", Container: "flac", Audio: PlaybackCapabilityAudio{Codec: "flac", Layout: "stereo", Route: "decode", MaxChannels: 2}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}},
	)
	if len(tuples) != 51 {
		t.Fatalf("runtime fixture must match the live 51-tuple profile, got %d", len(tuples))
	}
	profile := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "chromium", ClientVersion: "152.0.0.0", Platform: "web", Device: "Web browser",
		CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID: "browser-runtime", Source: "authenticated_runtime", Confidence: "medium", Producer: "portico-web-runtime", ProducerVersion: "browser-runtime-v2", ReviewedAt: now.Format(time.RFC3339), Tuples: tuples,
		}},
	}
	request := httptest.NewRequest("POST", "/api/playback-sessions", nil)
	request.RemoteAddr = "203.0.113.10:443"
	item := MediaItem{
		ID: "signal-one", Type: "track", Title: "Signal One", SourceURL: "/media/Signal One.flac", DurationSeconds: 15,
		Streams: []Stream{{ID: "signal-one-audio", Index: 0, Kind: "audio", Codec: "flac", Channels: 1, ChannelLayout: "mono", SampleRate: 48000, Default: true}},
	}
	policy, authorizedProfile := server.resolvePlaybackPolicyForRequest(context.Background(), request, user, item, PlaybackIntent{}, profile)
	resolution, err := resolvePlaybackCapabilities(authorizedProfile)
	if err != nil {
		t.Fatalf("resolve authenticated browser evidence: %v", err)
	}
	if resolution.Source != playbackcap.SourceAuthenticatedRuntime || len(resolution.Tuples) != 51 {
		t.Fatalf("trusted browser runtime evidence fell back: source=%s tuples=%d evidence=%s", resolution.Source, len(resolution.Tuples), resolution.EvidenceID)
	}
	decision, err := server.planMediaPlayback(context.Background(), item, authorizedProfile, policy, "", "", "off")
	if err != nil {
		t.Fatalf("plan exact FLAC mono browser playback: %v", err)
	}
	if decision.Mode != "direct_play" || decision.Protocol != "http" || !strings.EqualFold(decision.AudioCodec, "flac") {
		t.Fatalf("unexpected FLAC plan: %#v", decision)
	}
	rotatedRefreshID := "refresh_browser_capability_rotated"
	rotatedSessionID := "nativesess_" + rotatedRefreshID
	if _, err := server.db.Exec(`
		INSERT INTO native_refresh_tokens (id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, created_at, expires_at)
		VALUES (?, 'family-browser-capability-authority', ?, ?, ?, 'portico', 'hash-refresh-browser-capability-rotated', ?, ?)`,
		rotatedRefreshID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Add(time.Minute).Format(time.RFC3339Nano), now.Add(time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("rotate Hosted Web attachment refresh credential: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'hash-session-browser-capability-rotated', ?, ?, ?)`,
		rotatedSessionID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Add(time.Hour).Format(time.RFC3339Nano), now.Add(time.Minute).Format(time.RFC3339Nano), now.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("rotate Hosted Web attachment access session: %v", err)
	}
	user.AuthSessionID = rotatedSessionID
	_, rotatedProfile := server.resolvePlaybackPolicyForRequest(context.Background(), request, user, item, PlaybackIntent{}, profile)
	rotatedResolution, err := resolvePlaybackCapabilities(rotatedProfile)
	if err != nil || rotatedResolution.Source != playbackcap.SourceAuthenticatedRuntime || len(rotatedResolution.Tuples) != 51 {
		t.Fatalf("attachment refresh rotation lost browser runtime authority: resolution=%#v err=%v", rotatedResolution, err)
	}

	mediaPath := filepath.Join(t.TempDir(), "Signal One.flac")
	if err := os.WriteFile(mediaPath, []byte("flac-fixture"), 0o600); err != nil {
		t.Fatalf("write exact FLAC source fixture: %v", err)
	}
	user.LibraryIDs = append(user.LibraryIDs, "lib_signal_one")
	if _, err := server.db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib_signal_one', 'Codec Lab Audio', 'music', ?)`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed FLAC library: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('lib_signal_one_path', 'lib_signal_one', ?, 0, ?)`, filepath.Dir(mediaPath), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed FLAC library path: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('signal-one', 'lib_signal_one', 'track', 'Signal One', 'Signal One', ?, ?, 15)`, now.Format(time.RFC3339Nano), mediaPath); err != nil {
		t.Fatalf("seed FLAC media item: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, channel_layout, sample_rate, display_title, stream_index)
		VALUES ('signal-one-audio', 'signal-one', 'audio', 'flac', 1, 'mono', 48000, 'FLAC Mono', 0)`); err != nil {
		t.Fatalf("seed FLAC media stream: %v", err)
	}
	body, err := json.Marshal(PlaybackSessionCreateRequest{MediaID: "signal-one", SkipPreroll: true, ClientProfile: profile, Intent: automaticPlaybackIntent()})
	if err != nil {
		t.Fatalf("encode app-layer FLAC playback request: %v", err)
	}
	createRequest := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	createRequest.RemoteAddr = "203.0.113.10:443"
	createResponse := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(createResponse, createRequest, user)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("app-layer FLAC playback status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode app-layer FLAC playback response: %v", err)
	}
	if playback.SessionID == "" || playback.Decision.Mode != "direct_play" || !strings.EqualFold(playback.Decision.AudioCodec, "flac") {
		t.Fatalf("unexpected app-layer FLAC playback response: %#v", playback)
	}
	forgedNative := profile
	forgedNative.CapabilityEvidence = append([]PlaybackCapabilityEvidence(nil), profile.CapabilityEvidence...)
	forgedNative.CapabilityEvidence[0].Source = "native_runtime"
	_, forgedNative = server.resolvePlaybackPolicyForRequest(context.Background(), request, user, item, PlaybackIntent{}, forgedNative)
	forgedResolution, err := resolvePlaybackCapabilities(forgedNative)
	if err != nil {
		t.Fatalf("resolve self-promoted native browser evidence: %v", err)
	}
	if forgedResolution.Source != playbackcap.SourceStaticFallback {
		t.Fatalf("Hosted Web attachment self-promoted native evidence: %#v", forgedResolution)
	}

	if _, err := server.db.Exec(`UPDATE devices SET trusted=0 WHERE id=?`, deviceID); err != nil {
		t.Fatalf("mark browser untrusted: %v", err)
	}
	if _, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, profile); ok {
		t.Fatal("untrusted browser device retained runtime capability authority")
	}
	if _, err := server.db.Exec(`UPDATE devices SET trusted=1, revoked_at=? WHERE id=?`, now.Format(time.RFC3339Nano), deviceID); err != nil {
		t.Fatalf("revoke browser device: %v", err)
	}
	if _, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, profile); ok {
		t.Fatal("revoked browser device retained runtime capability authority")
	}
}

func TestServerBurnInDerivesOnlyFromProvenAudiovisualOutputTuples(t *testing.T) {
	base := playbackcap.Resolution{EvidenceID: "browser", Source: playbackcap.SourceAuthenticatedRuntime, Tuples: []playbackcap.DeliveryTuple{
		{
			Kind: playbackcap.MediaAudiovisual, Protocol: "hls", Container: "mpegts",
			Video:    playbackcap.Video{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", HDR: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 60},
			Audio:    playbackcap.Audio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
			Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone},
		},
		{Kind: playbackcap.MediaAudio, Protocol: "http", Container: "mp3", Audio: playbackcap.Audio{Codec: "mp3", Layout: "stereo", Route: "decode", MaxChannels: 2}, Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone}},
	}}
	got := playbackCapabilitiesWithServerBurnIn(base, "hdmv_pgs_subtitle", "bitmap")
	if len(base.Tuples) != 2 || len(got.Tuples) != 3 {
		t.Fatalf("burn-in derivation mutated source or widened wrong tuples: base=%d derived=%d", len(base.Tuples), len(got.Tuples))
	}
	burn := got.Tuples[2]
	if burn.Kind != playbackcap.MediaAudiovisual || burn.Video != base.Tuples[0].Video || burn.Audio != base.Tuples[0].Audio || burn.Subtitle != (playbackcap.Subtitle{Codec: "hdmv_pgs_subtitle", Kind: "bitmap", Mode: playbackcap.SubtitleBurn}) {
		t.Fatalf("unexpected server burn-in tuple: %#v", burn)
	}
}

func TestNativeCapabilityEvidencePreservesExactSilentVideoTuple(t *testing.T) {
	client := playbackcap.Client{Family: "avkit", Version: "18", Platform: "tvos", Device: "Apple TV"}
	authority := playbackCapabilityAuthority{Source: playbackcap.SourceNativeRuntime, Family: "avkit", Platform: "tvos", DeviceID: "verified-apple-tv", Producer: "portico-native/avkit/tvos", ProducerVersion: playbackCapabilitySchemaV2 + "/18"}
	raw := PlaybackCapabilityEvidence{
		ID: "reported-id", Source: "native_runtime", Confidence: "high", Producer: "reported-producer", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{
			MediaKind: "audiovisual", Protocol: "hls", Container: "mp4",
			Video:    PlaybackCapabilityVideo{Codec: "hevc", Profile: "main10", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60},
			Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
		}},
	}
	normalized, ok := normalizePlaybackCapabilityEvidence(client, playbackCapabilitySchemaV2, authority, raw)
	if !ok || len(normalized.Tuples) != 1 {
		t.Fatalf("exact silent-video evidence failed normalization: %#v ok=%v", normalized, ok)
	}
	if normalized.Tuples[0].Audio != (playbackcap.Audio{}) {
		t.Fatalf("normalization invented an audio capability: %#v", normalized.Tuples[0].Audio)
	}
}

func TestEstimatedPlaybackFactsAcceptSilentVideo(t *testing.T) {
	item := MediaItem{
		ID: "silent-video", SourceURL: "/media/silent-video.mp4", DurationSeconds: 12,
		Streams: []Stream{{Index: 0, Kind: "video", Codec: "h264", Width: 1280, Height: 720, FrameRate: 30, PixelFormat: "yuv420p"}},
	}
	facts, _, err := (&Server{}).estimatedPlaybackFacts(context.Background(), item, "")
	if err != nil {
		t.Fatalf("silent-video facts were rejected: %v", err)
	}
	if len(facts.Video) != 1 || len(facts.Audio) != 0 {
		t.Fatalf("estimated facts invented or lost streams: %#v", facts)
	}
}

func TestNativeCapabilityAuthorityComesFromVerifiedSessionAndDevice(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	user.AccountID = user.ID
	if err := server.db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? ORDER BY is_primary DESC, sort_order, id LIMIT 1`, accountIDForUser(user)).Scan(&user.ProfileID); err != nil {
		t.Fatalf("load native capability profile: %v", err)
	}
	now := time.Now().UTC()
	refreshID := "refresh_capability_authority"
	sessionID := "nativesess_" + refreshID
	deviceID := "device_capability_authority"
	if _, err := server.db.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, app, platform, trusted, created_at, last_seen_at)
		VALUES (?, ?, 'install-capability-authority', 'Living Room Apple TV', 'Portico', 'tvOS', 1, ?, ?)`,
		deviceID, accountIDForUser(user), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed capability device: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO native_refresh_tokens (id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, created_at, expires_at)
		VALUES (?, 'family-capability-authority', ?, ?, ?, 'local', 'hash-refresh-capability-authority', ?, ?)`,
		refreshID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("seed capability refresh credential: account=%s profile=%s: %v", accountIDForUser(user), viewerProfileID(user), err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'hash-session-capability-authority', ?, ?, ?)`,
		sessionID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed capability access session: %v", err)
	}
	user.AuthSessionID, user.DeviceID = sessionID, deviceID
	authority, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS",
	})
	if !ok || authority.Source != playbackcap.SourceNativeRuntime || authority.Family != "avkit" || authority.Platform != "tvos" || authority.DeviceID == "" || authority.ProducerVersion == "" {
		t.Fatalf("unexpected verified capability authority: %#v ok=%v", authority, ok)
	}
	if _, err := server.db.Exec(`UPDATE devices SET platform = 'MacIntel', app = 'portico-web' WHERE id = ?`, deviceID); err != nil {
		t.Fatalf("convert attachment credential to current Web device shape: %v", err)
	}
	webAuthority, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "chromium", ClientVersion: "152.0.0.0", Platform: "web",
	})
	if !ok || webAuthority.Source != playbackcap.SourceAuthenticatedRuntime || webAuthority.Family != "chromium" || webAuthority.Platform != "web" {
		t.Fatalf("current Hosted Web attachment did not receive browser runtime authority: %#v ok=%v", webAuthority, ok)
	}
	if _, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS",
	}); ok {
		t.Fatal("Hosted Web attachment self-promoted to native runtime authority")
	}
	user.AuthSessionID = ""
	if _, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, PlaybackClientProfile{CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18"}); ok {
		t.Fatal("ordinary authenticated request was promoted to native capability authority")
	}
}

func TestResolvePlaybackCapabilitiesCannotSelfPromoteRuntimeEvidence(t *testing.T) {
	profile := PlaybackClientProfile{CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS", Device: "Forged Apple TV", CapabilityEvidence: []PlaybackCapabilityEvidence{{
		ID: "forged-native", Source: "native_runtime", Confidence: "high", Producer: "portico-apple", ProducerVersion: "999", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts", Video: PlaybackCapabilityVideo{Codec: "hevc", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}, Audio: PlaybackCapabilityAudio{Codec: "eac3", MaxChannels: 6}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}}},
	}}}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Source != playbackcap.SourceStaticFallback || resolution.EvidenceID == "forged-native" {
		t.Fatalf("client self-promotion became authoritative: %#v", resolution)
	}
}

func TestResolvePlaybackCapabilitiesRejectsClientSuppliedStaticPromotion(t *testing.T) {
	profile := PlaybackClientProfile{ClientFamily: "roku", Platform: "roku", CapabilityEvidence: []PlaybackCapabilityEvidence{{
		ID: "forged", Source: "static_fallback", Confidence: "high", Producer: "client", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{MediaKind: "audiovisual", Protocol: "http", Container: "mp4", Video: PlaybackCapabilityVideo{Codec: "av1", BitDepth: 10, MaxWidth: 7680, MaxHeight: 4320, MaxFrameRate: 120}, Audio: PlaybackCapabilityAudio{Codec: "truehd", MaxChannels: 8}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}}},
	}}}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.EvidenceID == "forged" || resolution.Source != playbackcap.SourceStaticFallback {
		t.Fatalf("untrusted evidence was promoted: %#v", resolution)
	}
}

func TestChromiumUnauthenticatedProbeFallsBackToReviewedManagedHLSBand(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	profile := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "chromium", ClientVersion: "147.0.7727.15", Platform: "web", Device: "Chrome on macOS",
		CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID: "portico-web-runtime-chromium-147", Source: "unauthenticated_probe", Confidence: "medium", Producer: "portico-web-runtime", ReviewedAt: now,
			Tuples: []PlaybackCapabilityTuple{{
				MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts",
				Video:    PlaybackCapabilityVideo{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 30},
				Audio:    PlaybackCapabilityAudio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
				Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
			}},
		}},
	}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	want := playbackcap.DeliveryTuple{
		Kind: playbackcap.MediaAudiovisual, Protocol: "hls", Container: "mpegts",
		Video:    playbackcap.Video{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", HDR: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 30},
		Audio:    playbackcap.Audio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
		Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone},
	}
	if resolution.Source != playbackcap.SourceStaticFallback || resolution.Band != "chromium-120-159" || !resolution.Supports(want) {
		t.Fatalf("Chromium did not resolve to the reviewed managed-HLS baseline: %#v", resolution)
	}
}

func TestChromiumStaticFallbackDirectPlaysExactMonoAACBaseline(t *testing.T) {
	server := newScannerTestServer(t)
	profile := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "chromium", ClientVersion: "load-harness-1", Platform: "web", Device: "Portico acceptance load harness",
		CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID: "untrusted-acceptance-probe", Source: "unauthenticated_probe", Confidence: "high", Producer: "portico-load-harness", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
			Tuples: []PlaybackCapabilityTuple{{
				MediaKind: "audiovisual", Protocol: "http", Container: "mp4",
				Video:    PlaybackCapabilityVideo{Codec: "h264", Profile: "baseline", PixelFormat: "yuv420p", Chroma: "4:2:0", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 60},
				Audio:    PlaybackCapabilityAudio{Codec: "aac", Profile: "lc", Layout: "mono", Route: "decode", MaxChannels: 1},
				Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
			}},
		}},
	}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Source != playbackcap.SourceStaticFallback {
		t.Fatalf("untrusted probe became playback authority: %#v", resolution)
	}
	item := MediaItem{
		ID: "acceptance-mono-mp4", Type: "movie", SourceURL: "/media/acceptance-mono.mp4", DurationSeconds: 2,
		Streams: []Stream{
			{ID: "acceptance-video", Index: 0, Kind: "video", Codec: "h264", Profile: "Constrained Baseline", Width: 320, Height: 180, FrameRate: 12, PixelFormat: "yuv420p", BitDepth: 8, Default: true},
			{ID: "acceptance-audio", Index: 1, Kind: "audio", Codec: "aac", Profile: "LC", Channels: 1, ChannelLayout: "mono", SampleRate: 48000, Default: true},
		},
	}
	policy := defaultResolvedPlaybackPolicy(item.Type, playbackNetworkRemote)
	decision, err := server.planMediaPlayback(context.Background(), item, profile, policy, "acceptance-audio", "", "off")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Mode != "direct_play" || decision.Protocol != "http" || decision.executionPlan == nil || decision.executionPlan.Plan.Audio.Channels != 1 {
		t.Fatalf("exact conservative mono baseline did not direct play: %#v", decision)
	}
}
