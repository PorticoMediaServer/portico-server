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

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

// porticoQAF046BrowserTuples is the authenticated 37-tuple Chromium evidence
// captured by PORTICO-QA-F046: 30 H.264/AAC/silent HTTP+HLS tuples, five
// VP9/Opus/silent WebM tuples, and two MP3 audio-only tuples.
func porticoQAF046BrowserTuples() []PlaybackCapabilityTuple {
	var tuples []PlaybackCapabilityTuple
	noSubtitle := PlaybackCapabilitySubtitle{Mode: "none"}
	textSubtitle := PlaybackCapabilitySubtitle{Codec: "webvtt", Kind: "text", Mode: "native"}
	audioLayouts := []PlaybackCapabilityAudio{
		{Codec: "aac", Profile: "lc", Layout: "mono", Route: "decode", MaxChannels: 1},
		{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
	}
	for _, profile := range []string{"baseline", "main", "high"} {
		video := PlaybackCapabilityVideo{Codec: "h264", Profile: profile, PixelFormat: "yuv420p", Chroma: "4:2:0", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 60}
		for _, route := range []struct{ protocol, container string }{{"http", "mp4"}, {"hls", "mpegts"}} {
			tuples = append(tuples, PlaybackCapabilityTuple{MediaKind: "audiovisual", Protocol: route.protocol, Container: route.container, Video: video, Subtitle: noSubtitle})
			for _, audio := range audioLayouts {
				tuples = append(tuples,
					PlaybackCapabilityTuple{MediaKind: "audiovisual", Protocol: route.protocol, Container: route.container, Video: video, Audio: audio, Subtitle: noSubtitle},
					PlaybackCapabilityTuple{MediaKind: "audiovisual", Protocol: route.protocol, Container: route.container, Video: video, Audio: audio, Subtitle: textSubtitle},
				)
			}
		}
	}
	webmVideo := PlaybackCapabilityVideo{Codec: "vp9", PixelFormat: "yuv420p", Chroma: "4:2:0", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 60}
	tuples = append(tuples, PlaybackCapabilityTuple{MediaKind: "audiovisual", Protocol: "http", Container: "webm", Video: webmVideo, Subtitle: noSubtitle})
	for _, audio := range []PlaybackCapabilityAudio{
		{Codec: "opus", Layout: "mono", Route: "decode", MaxChannels: 1},
		{Codec: "opus", Layout: "stereo", Route: "decode", MaxChannels: 2},
	} {
		tuples = append(tuples,
			PlaybackCapabilityTuple{MediaKind: "audiovisual", Protocol: "http", Container: "webm", Video: webmVideo, Audio: audio, Subtitle: noSubtitle},
			PlaybackCapabilityTuple{MediaKind: "audiovisual", Protocol: "http", Container: "webm", Video: webmVideo, Audio: audio, Subtitle: textSubtitle},
		)
	}
	for _, layout := range []struct {
		name     string
		channels int
	}{{"mono", 1}, {"stereo", 2}} {
		tuples = append(tuples, PlaybackCapabilityTuple{
			MediaKind: "audio", Protocol: "http", Container: "mp3",
			Audio: PlaybackCapabilityAudio{Codec: "mp3", Layout: layout.name, Route: "decode", MaxChannels: layout.channels}, Subtitle: noSubtitle,
		})
	}
	return tuples
}

func TestF046AuthenticatedBrowserExecutionPlanReachesSessionAndCompilerWithoutUpmix(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	user.AccountID = accountIDForUser(user)
	if user.ProfileID == "" {
		if err := server.db.QueryRow(`SELECT id FROM profiles WHERE account_id=? ORDER BY is_primary DESC, sort_order, id LIMIT 1`, user.AccountID).Scan(&user.ProfileID); err != nil {
			t.Fatalf("load F046 profile: %v", err)
		}
	}

	now := time.Now().UTC()
	deviceID, sessionID := "device-f046-chromium", "sess_f046_chromium"
	if _, err := server.db.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, app, platform, trusted, created_at, last_seen_at)
		VALUES (?, ?, 'install-f046-chromium', 'Chrome browser', 'Chrome', 'MacIntel', 1, ?, ?)`,
		deviceID, user.AccountID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed F046 trusted browser: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'hash-f046-browser', ?, ?, ?)`, sessionID, user.AccountID, viewerProfileID(user), deviceID,
		now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed F046 browser session: %v", err)
	}
	user.AuthSessionID, user.DeviceID = sessionID, deviceID

	mediaRoot := t.TempDir()
	sourcePath := filepath.Join(mediaRoot, "Portico Broadcast Loop.ts")
	const sourceSize int64 = 4_239_776
	if err := os.WriteFile(sourcePath, nil, 0o600); err != nil {
		t.Fatalf("create F046 source: %v", err)
	}
	if err := os.Truncate(sourcePath, sourceSize); err != nil {
		t.Fatalf("size F046 source: %v", err)
	}
	modTime := now.Format(time.RFC3339Nano)
	identity := canonicalAnalysisFileIdentity("file-f046-broadcast", "sha256:f046-broadcast-loop", sourceSize, modTime)
	facts := mediafacts.Facts{
		Version: mediafacts.SchemaVersion, Source: mediafacts.Source{Fingerprint: identity.Fingerprint, Revision: identity.revision(), SizeBytes: sourceSize},
		Container: "mpegts", DurationUS: 60_000_000, DurationConfidence: mediafacts.ConfidenceExact,
		Video: []mediafacts.Video{{
			Index: 0, Codec: "mpeg2video", Profile: "main", CodedWidth: 320, CodedHeight: 180,
			SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9},
			PixelFormat: "yuv420p", BitDepth: 8, ChromaSubsampling: "4:2:0", FieldOrder: "progressive", FrameRate: mediafacts.Rational{Num: 25, Den: 1},
		}},
		Audio: []mediafacts.Audio{{Index: 1, Codec: "mp2", Layout: "mono", Channels: 1, SampleRate: 48000, Bitrate: 128000}},
	}
	canonicalFacts, err := facts.Canonical()
	if err != nil {
		t.Fatalf("canonicalize F046 facts: %v", err)
	}
	factsJSON, err := canonicalFacts.StableJSON()
	if err != nil {
		t.Fatalf("encode F046 facts: %v", err)
	}
	factsDigest, err := canonicalFacts.Digest()
	if err != nil {
		t.Fatalf("digest F046 facts: %v", err)
	}
	user.LibraryIDs = append(user.LibraryIDs, "library-f046")
	mustExec := func(label, query string, args ...any) {
		t.Helper()
		if _, err := server.db.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
	}
	mustExec("seed F046 library", `INSERT INTO libraries (id, name, type, path, created_at) VALUES ('library-f046', 'Codec Lab', 'movie', ?, ?)`, mediaRoot, now.Format(time.RFC3339Nano))
	mustExec("seed F046 library path", `INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('path-f046', 'library-f046', ?, 0, ?)`, mediaRoot, now.Format(time.RFC3339Nano))
	mustExec("seed F046 media", `INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds) VALUES ('media-f046-broadcast', 'library-f046', 'movie', 'Portico Broadcast Loop', 'Portico Broadcast Loop', ?, ?, 60)`, now.Format(time.RFC3339Nano), sourcePath)
	mustExec("seed F046 file", `INSERT INTO media_files (id, media_id, library_id, path, container, source_type, size_bytes, mod_time, content_fingerprint, available, first_seen_at, last_seen_at) VALUES ('file-f046-broadcast', 'media-f046-broadcast', 'library-f046', ?, 'mpegts', 'local', ?, ?, ?, 1, ?, ?)`, sourcePath, sourceSize, modTime, identity.Fingerprint, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	mustExec("seed F046 video", `INSERT INTO media_streams (id, media_id, file_id, stream_index, source_kind, kind, codec, profile, width, height, pixel_format, bit_depth, frame_rate, field_order, display_title) VALUES ('video-f046-broadcast', 'media-f046-broadcast', 'file-f046-broadcast', 0, 'ffprobe', 'video', 'mpeg2video', 'main', 320, 180, 'yuv420p', 8, 25, 'progressive', 'MPEG-2 Main 320x180')`)
	mustExec("seed F046 audio", `INSERT INTO media_streams (id, media_id, file_id, stream_index, source_kind, kind, codec, channels, channel_layout, sample_rate, bitrate, display_title) VALUES ('audio-f046-broadcast', 'media-f046-broadcast', 'file-f046-broadcast', 1, 'ffprobe', 'audio', 'mp2', 1, 'mono', 48000, 128000, 'MP2 Mono')`)
	if _, err := server.db.Exec(`INSERT INTO media_analysis_facts
		(media_id, media_file_id, schema_version, source_revision, source_fingerprint, facts_digest, facts_json, analyzed_at)
		VALUES ('media-f046-broadcast', 'file-f046-broadcast', ?, ?, ?, ?, ?, ?)`,
		mediafacts.SchemaVersion, identity.revision(), identity.Fingerprint, factsDigest, string(factsJSON), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed exact F046 facts: %v", err)
	}

	tuples := porticoQAF046BrowserTuples()
	if len(tuples) != 37 {
		t.Fatalf("F046 fixture tuple count=%d, want 37", len(tuples))
	}
	profile := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "chromium", ClientVersion: "152.0.0.0", Platform: "web", Device: "Chrome browser",
		CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID: "runtime-f046", Source: "authenticated_runtime", Confidence: "medium", Producer: "portico-web-runtime", ProducerVersion: "browser-runtime-v2",
			ReviewedAt: now.Format(time.RFC3339), Tuples: tuples,
		}},
	}

	item, err := server.getMediaPlaybackDetailForUser(context.Background(), user, "media-f046-broadcast")
	if err != nil {
		t.Fatalf("load exact F046 item: %v", err)
	}
	if item.LibraryID != "library-f046" || item.SourceURL != sourcePath {
		t.Fatalf("load exact F046 source binding: library=%q source=%q", item.LibraryID, item.SourceURL)
	}
	if _, err := server.localSourcePathForTranscode(item); err != nil {
		library, libraryErr := server.getLibrary("library-f046")
		t.Fatalf("validate exact F046 source: %v (library=%#v loadErr=%v)", err, library, libraryErr)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", nil)
	policy, authorizedProfile := server.resolvePlaybackPolicyForRequest(context.Background(), request, user, item, PlaybackIntent{}, profile)
	withoutMono := authorizedProfile
	withoutMono.CapabilityEvidence = append([]PlaybackCapabilityEvidence(nil), authorizedProfile.CapabilityEvidence...)
	withoutMono.CapabilityEvidence[0].Tuples = nil
	for _, tuple := range tuples {
		if strings.EqualFold(tuple.Audio.Layout, "mono") {
			continue
		}
		withoutMono.CapabilityEvidence[0].Tuples = append(withoutMono.CapabilityEvidence[0].Tuples, tuple)
	}
	unsupported, err := server.planMediaPlayback(context.Background(), item, withoutMono, policy, "", "", "off")
	if err != nil {
		t.Fatalf("plan F046 negative profile: %v", err)
	}
	if unsupported.Mode != "unavailable" || !containsString(unsupported.ReasonCodes, string(playbackplan.ReasonNoCompatibleTuple)) {
		t.Fatalf("planner widened mono into the retained stereo tuples: %#v", unsupported)
	}

	body, err := json.Marshal(PlaybackSessionCreateRequest{MediaID: item.ID, SkipPreroll: true, ClientProfile: profile, Intent: automaticPlaybackIntent()})
	if err != nil {
		t.Fatalf("encode F046 app request: %v", err)
	}
	createRequest := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(body))
	createResponse := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(createResponse, createRequest, user)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("F046 app boundary status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(createResponse.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode F046 playback response: %v", err)
	}
	if playback.SessionID == "" || playback.MediaGrant.Token == "" || playback.Decision.Mode != "transcode_required" || playback.Decision.Protocol != "hls" || playback.Decision.Container != "mpegts" || playback.SelectedAudioStreamID != "audio-f046-broadcast" {
		t.Fatalf("F046 response did not reach the exact session boundary: %#v", playback)
	}
	if !strings.Contains(playback.SourceURL, "/hls/master.m3u8") || !strings.Contains(playback.SourceURL, "audio=transcode") || !strings.Contains(playback.SourceURL, "audioStream=audio-f046-broadcast") {
		t.Fatalf("F046 URL was not derived from the selected execution plan: %s", playback.SourceURL)
	}

	execution, err := server.playbackPlanForSession(context.Background(), playback.SessionID, item.ID)
	if err != nil {
		t.Fatalf("load F046 persisted execution plan: %v", err)
	}
	if execution.Plan.Mode != playbackplan.VideoTranscode || execution.Plan.Protocol != "hls" || execution.Plan.Container != "mpegts" || execution.AudioStreamID != "audio-f046-broadcast" || execution.audioMode() != "transcode" {
		t.Fatalf("F046 persisted execution plan mismatch: %#v", execution)
	}
	if execution.Plan.Audio.Codec != "aac" || execution.Plan.Audio.Layout != "mono" || execution.Plan.Audio.Channels != 1 || execution.Plan.Audio.Downmixed {
		t.Fatalf("F046 audio selection widened layout: %#v", execution.Plan.Audio)
	}
	var videoAction, audioAction playbackplan.StreamAction
	for _, action := range execution.Plan.Streams {
		switch action.Kind {
		case "video":
			videoAction = action
		case "audio":
			audioAction = action
		}
	}
	if videoAction.Action != playbackplan.Convert || videoAction.InputCodec != "mpeg2video" || videoAction.OutputCodec != "h264" ||
		audioAction.Action != playbackplan.Convert || audioAction.InputCodec != "mp2" || audioAction.OutputCodec != "aac" || audioAction.InputLayout != "mono" || audioAction.OutputLayout != "mono" {
		t.Fatalf("F046 stream graph mismatch: video=%#v audio=%#v", videoAction, audioAction)
	}
	var grantDigest, grantPlanJSON string
	if err := server.db.QueryRow(`SELECT plan_digest, plan_json FROM playback_media_grants WHERE playback_session_id=? AND revoked_at=''`, playback.SessionID).Scan(&grantDigest, &grantPlanJSON); err != nil {
		t.Fatalf("load F046 grant plan: %v", err)
	}
	grantPlan, err := decodePlaybackExecutionPlan(grantPlanJSON)
	if err != nil || grantDigest != execution.Digest || grantPlan.Digest != execution.Digest {
		t.Fatalf("F046 grant did not bind the session execution plan: grant=%q session=%q err=%v", grantDigest, execution.Digest, err)
	}

	fakeFFmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write F046 compiler executable: %v", err)
	}
	server.cfg.FFmpegPath = fakeFFmpeg
	command, err := server.compilePlannedVODHLS(context.Background(), plannedVODHLSRequest{
		Item: item, Binding: execution,
		Identity:       plannedTranscodeIdentity{UserID: user.AccountID, ProfileID: viewerProfileID(user), PlaybackSessionID: playback.SessionID, AuthorizationRevision: "f046-authorization", PlaybackGeneration: execution.generation(), GrantTokenHash: hashToken(playback.MediaGrant.Token)},
		GenerationRoot: t.TempDir(), SegmentSeconds: hlsSegmentSeconds,
	})
	if err != nil {
		t.Fatalf("compile F046 execution plan: %v", err)
	}
	args := strings.Join(command.Args, " ")
	for _, required := range []string{"-i " + sourcePath, "-map 0:0", "-map 0:1", "-c:v libx264", "-c:a aac", "-ac 1", "-channel_layout mono", "-ar 48000"} {
		if !strings.Contains(args, required) {
			t.Fatalf("F046 compiler omitted %q: %s", required, args)
		}
	}
	if strings.Contains(args, "-ac 2") || strings.Contains(args, "-channel_layout stereo") {
		t.Fatalf("F046 compiler implicitly upmixed mono: %s", args)
	}
}
