package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/ffmpegsupervisor"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

type f062PlaybackFixture struct {
	server       *Server
	user         User
	item         MediaItem
	binding      playbackExecutionPlan
	grant        MediaGrant
	sessionID    string
	transcodeDir string
}

type f062Problem struct {
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail"`
	MessageID string `json:"messageId"`
}

func newF062PlaybackFixture(t *testing.T, executable string) f062PlaybackFixture {
	t.Helper()
	server := newScannerTestServer(t)
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	server.backgroundCtx = backgroundCtx
	server.ffmpegSupervisor = newTranscodeSupervisorV2(backgroundCtx, ffmpegsupervisor.Config{InterruptGrace: 50 * time.Millisecond, VODAbandonment: time.Minute})
	t.Cleanup(func() {
		cancelBackground()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
		defer cancelShutdown()
		_ = server.ffmpegSupervisor.Shutdown(shutdownCtx)
	})
	server.cfg.FFmpegPath = executable
	server.cfg.TranscodeDir = filepath.Join(t.TempDir(), "transcodes")
	if err := os.MkdirAll(server.cfg.TranscodeDir, 0o700); err != nil {
		t.Fatalf("create F062 transcode root: %v", err)
	}

	mediaRoot := t.TempDir()
	sourcePath := filepath.Join(mediaRoot, "codec-lab-av1-opus.mkv")
	if err := os.WriteFile(sourcePath, []byte("sealed source identity for a fake producer"), 0o600); err != nil {
		t.Fatalf("write F062 source: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "F062 Codec Lab", Type: "movie", Paths: []string{mediaRoot}})
	if err != nil {
		t.Fatalf("create F062 library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const mediaID = "movie_f062_av1_opus_mono"
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, duration_seconds, added_at)
		VALUES (?, ?, 'movie', 'Codec Lab AV1 Opus Mono', 'Codec Lab AV1 Opus Mono', ?, 60, ?)`, mediaID, library.ID, sourcePath, now); err != nil {
		t.Fatalf("insert F062 media: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_streams (
			id, media_id, kind, codec, channels, bitrate, width, height, display_title,
			profile, pixel_format, bit_depth, dynamic_range, channel_layout, frame_rate,
			sample_rate, stream_index
		) VALUES
			('stream_f062_video', ?, 'video', 'av1', 0, 1800000, 320, 180, 'AV1 Main',
			 'main', 'yuv420p', 8, 'sdr', '', 24, 0, 0),
			('stream_f062_audio', ?, 'audio', 'opus', 1, 96000, 0, 0, 'Opus Mono',
			 '', '', 0, '', 'mono', 0, 48000, 1)`, mediaID, mediaID); err != nil {
		t.Fatalf("insert F062 streams: %v", err)
	}

	user := dvrTestUser(t, server)
	if _, err := server.db.Exec(`
		INSERT INTO profiles (id, account_id, display_name, role, permissions_json, preferences_json, created_at, updated_at, is_primary)
		VALUES (?, ?, ?, 'owner', '{}', '{}', ?, ?, 1)
		ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id, is_primary=1`,
		viewerProfileID(user), accountIDForUser(user), user.DisplayName, now, now); err != nil {
		t.Fatalf("ensure F062 profile: %v", err)
	}
	item, err := server.getMediaPlaybackDetailForUser(context.Background(), user, mediaID)
	if err != nil {
		t.Fatalf("load F062 media: %v", err)
	}
	facts, factsDigest, err := server.mediaFactsForPlayback(context.Background(), item)
	if err != nil || len(facts.Video) != 1 || len(facts.Audio) != 1 {
		t.Fatalf("resolve exact F062 facts: video=%d audio=%d err=%v", len(facts.Video), len(facts.Audio), err)
	}
	videoIndex, audioIndex := facts.Video[0].Index, facts.Audio[0].Index
	plan := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: facts.Source.Fingerprint,
		SourceRevision: facts.Source.Revision, CapabilityEvidenceID: "codec-lab-av1-opus-mono-v1",
		Policy: playbackplan.MaximumFidelity, Mode: playbackplan.VideoTranscode,
		MediaKind: playbackcap.MediaAudiovisual, Protocol: "hls", Container: "mpegts", SegmentFormat: "mpegts",
		Selection: playbackplan.Selection{VideoIndex: &videoIndex, AudioIndex: &audioIndex},
		Streams: []playbackplan.StreamAction{
			{Index: videoIndex, Kind: "video", Action: playbackplan.Convert, InputCodec: "av1", OutputCodec: "h264"},
			{Index: audioIndex, Kind: "audio", Action: playbackplan.Convert, InputCodec: "opus", OutputCodec: "aac", InputLayout: "mono", OutputLayout: "mono"},
		},
		Stages: []playbackplan.Stage{
			{Kind: "video", Operation: "decode", Execution: "software"},
			{Kind: "video", Operation: "encode", Execution: "software"},
			{Kind: "audio", Operation: "encode", Execution: "software"},
			{Kind: "mux", Operation: "package", Execution: "stream"},
		},
		Audio:    playbackplan.AudioDecision{Codec: "aac", Layout: "mono", Channels: 1},
		Subtitle: playbackplan.SubtitleDecision{Action: playbackplan.Drop},
		Timeline: playbackplan.Timeline{Mode: "vod", DurationUS: facts.DurationUS, Generation: 1},
		Reasons:  []playbackplan.ReasonCode{playbackplan.ReasonVideoConversion, playbackplan.ReasonAudioConversion},
	}
	binding := testPlaybackExecutionPlan(t, func(execution *playbackExecutionPlan) {
		execution.Plan = plan
		execution.MediaFactsDigest = factsDigest
		execution.Quality = "720p-standard"
		execution.X264Preset = "veryfast"
	})
	bindingJSON, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("encode F062 binding: %v", err)
	}
	sessionID := randomID("play_f062")
	if _, err := server.db.Exec(`
		INSERT INTO playback_sessions (
			id, user_id, profile_id, current_entry_id, media_id, media_type, title,
			started_at, last_seen_at, state, plan_schema_version, plan_digest, plan_json,
			source_revision, media_facts_digest, capability_evidence_id, playback_generation
		) VALUES (?, ?, ?, ?, ?, 'movie', ?, ?, ?, 'playing', ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, accountIDForUser(user), viewerProfileID(user), "qentry_"+sessionID, item.ID, item.Title,
		now, now, binding.SchemaVersion, binding.Digest, string(bindingJSON), binding.Plan.SourceRevision,
		binding.MediaFactsDigest, binding.Plan.CapabilityEvidenceID, binding.generation()); err != nil {
		t.Fatalf("insert F062 playback session: %v", err)
	}
	grant, err := server.issueMediaGrant(context.Background(), user, sessionID, "media", item.ID)
	if err != nil {
		t.Fatalf("issue F062 media grant: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO playback_session_continuation_credentials (
			playback_session_id, token_hash, user_id, profile_id, client_instance_id, origin,
			issued_at, expires_at, absolute_expires_at
		) VALUES (?, ?, ?, ?, 'client_f062', 'https://app.example.test', ?, ?, ?)`,
		sessionID, hashToken("continuation_"+sessionID), accountIDForUser(user), viewerProfileID(user), now,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert F062 continuation: %v", err)
	}
	return f062PlaybackFixture{server: server, user: user, item: item, binding: binding, grant: grant, sessionID: sessionID, transcodeDir: server.cfg.TranscodeDir}
}

func TestF062CodecLabProducerExitHasOneTerminalOutcomeAndZeroResidue(t *testing.T) {
	producerDir := t.TempDir()
	argumentsPath := filepath.Join(producerDir, "arguments.txt")
	launchesPath := filepath.Join(producerDir, "launches.txt")
	executable := filepath.Join(producerDir, "fake-ffmpeg")
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" -filters \"*) exit 1 ;; esac\n" +
		"printf '%s\\n' \"$@\" > " + f062ShellQuote(argumentsPath) + "\n" +
		"printf 'launch\\n' >> " + f062ShellQuote(launchesPath) + "\n" +
		"printf 'PRIVATE_FFMPEG_STDERR_DO_NOT_EXPOSE\\n' >&2\n" +
		"exit 42\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := newF062PlaybackFixture(t, executable)
	preflightRequest := mediaGrantRequest(http.MethodGet, "/api/media/"+fixture.item.ID+"/hls/variant.m3u8", fixture.grant.Token)
	identity, err := fixture.server.plannedTranscodeIdentityForRequest(context.Background(), preflightRequest, fixture.user, fixture.item.ID, fixture.binding)
	if err != nil {
		t.Fatalf("resolve Codec Lab producer identity: %v", err)
	}
	preflight, err := fixture.server.compilePlannedVODHLS(context.Background(), plannedVODHLSRequest{
		Item: fixture.item, Binding: fixture.binding, Identity: identity,
		GenerationRoot: filepath.Join(fixture.transcodeDir, "planned-v2"), SegmentSeconds: hlsSegmentSeconds,
	})
	if err != nil {
		t.Fatalf("compile exact Codec Lab AV1/Opus producer: %v", err)
	}
	_ = os.RemoveAll(preflight.GenerationDir)

	first := f062ManifestRequest(fixture)
	assertF062TerminalProblem(t, first)
	if strings.Contains(first.Body.String(), "PRIVATE_FFMPEG_STDERR_DO_NOT_EXPOSE") || strings.Contains(first.Body.String(), fixture.transcodeDir) {
		t.Fatalf("terminal problem leaked private producer diagnostics: %s", first.Body.String())
	}
	second := f062ManifestRequest(fixture)
	assertF062TerminalProblem(t, second)
	var firstProblem, secondProblem f062Problem
	if err := json.Unmarshal(first.Body.Bytes(), &firstProblem); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondProblem); err != nil {
		t.Fatal(err)
	}
	if firstProblem != secondProblem {
		t.Fatalf("terminal problem changed across retries: first=%#v second=%#v", firstProblem, secondProblem)
	}
	launches, err := os.ReadFile(launchesPath)
	if err != nil {
		t.Fatalf("read fake FFmpeg launches: %v", err)
	}
	if got := len(strings.Fields(string(launches))); got != 1 {
		t.Fatalf("FFmpeg launch count=%d, want one", got)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatalf("read compiled FFmpeg arguments: %v", err)
	}
	argv := strings.Fields(string(arguments))
	for _, pair := range [][2]string{{"-c:v", "libx264"}, {"-c:a", "aac"}, {"-ac", "1"}} {
		if !f062ContainsAdjacent(argv, pair[0], pair[1]) {
			t.Fatalf("Codec Lab command lacks %q %q: %q", pair[0], pair[1], string(arguments))
		}
	}
	assertF062TerminalResidue(t, fixture)
}

func TestF062ProducerStartFailureIsTerminalAndStable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "invalid-ffmpeg")
	if err := os.WriteFile(executable, []byte("this is not an executable image\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := newF062PlaybackFixture(t, executable)
	first := f062ManifestRequest(fixture)
	assertF062TerminalProblem(t, first)
	second := f062ManifestRequest(fixture)
	assertF062TerminalProblem(t, second)
	assertF062TerminalResidue(t, fixture)
}

func TestF062ConcurrentManifestRequestsCoalesceOnOneFailedProducer(t *testing.T) {
	producerDir := t.TempDir()
	launchesPath := filepath.Join(producerDir, "launches.txt")
	executable := filepath.Join(producerDir, "fake-ffmpeg")
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" -filters \"*) exit 1 ;; esac\n" +
		"printf 'launch\\n' >> " + f062ShellQuote(launchesPath) + "\n" +
		"sleep 0.05\n" +
		"exit 42\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := newF062PlaybackFixture(t, executable)
	recorders := make([]*httptest.ResponseRecorder, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range recorders {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			recorders[index] = f062ManifestRequest(fixture)
		}(index)
	}
	close(start)
	wait.Wait()
	for _, recorder := range recorders {
		assertF062TerminalProblem(t, recorder)
	}
	launches, err := os.ReadFile(launchesPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(launches))); got != 1 {
		t.Fatalf("concurrent requests launched %d producers, want one", got)
	}
	assertF062TerminalResidue(t, fixture)
}

func TestF062ExactPlaybackCleanupPreservesAnotherSessionForTheSameMedia(t *testing.T) {
	first := &transcodeSession{key: "first", playbackSessionID: "playback-first", mediaID: "shared-media"}
	second := &transcodeSession{key: "second", playbackSessionID: "playback-second", mediaID: "shared-media"}
	server := newScannerTestServer(t)
	server.transcodes = map[string]*transcodeSession{first.key: first, second.key: second}
	if !server.stopTranscodeSessionForPlaybackSession(first.playbackSessionID) {
		t.Fatal("exact playback cleanup did not find its generation")
	}
	server.transcodeMu.Lock()
	defer server.transcodeMu.Unlock()
	if server.transcodes[first.key] != nil || server.transcodes[second.key] != second {
		t.Fatalf("exact cleanup crossed playback ownership: %#v", server.transcodes)
	}
}

func TestF062ManifestAndSegmentPublicationHorizonsTerminalizePlayback(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		request func(f062PlaybackFixture) *httptest.ResponseRecorder
	}{
		{name: "manifest", request: f062ManifestRequest},
		{name: "segment", request: f062SegmentRequest},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "unused-ffmpeg")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			fixture := newF062PlaybackFixture(t, executable)
			request := mediaGrantRequest(http.MethodGet, "/api/media/"+fixture.item.ID+"/hls/variant.m3u8", fixture.grant.Token)
			identity, err := fixture.server.plannedTranscodeIdentityForRequest(context.Background(), request, fixture.user, fixture.item.ID, fixture.binding)
			if err != nil {
				t.Fatalf("resolve F062 timeout identity: %v", err)
			}
			root := filepath.Join(fixture.transcodeDir, "planned-v2")
			dir := filepath.Join(root, "expired-generation")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			session := &transcodeSession{
				key:    plannedTranscodeSessionKey(fixture.item.ID, fixture.binding, identity, 0),
				userID: identity.UserID, playbackSessionID: fixture.sessionID, mediaID: fixture.item.ID,
				quality: fixture.binding.Quality, root: root, dir: dir, manifest: filepath.Join(dir, "index.m3u8"),
				startedAt: time.Now().UTC().Add(-time.Minute), done: make(chan struct{}), updateCh: make(chan struct{}),
				segmentSeconds: hlsSegmentSeconds, lastServedSegment: -1, lastProducedSegment: -1,
				generation: fixture.binding.generation(), admissionActive: true,
			}
			fixture.server.transcodeMu.Lock()
			fixture.server.transcodes[session.key] = session
			fixture.server.transcodeMu.Unlock()

			recorder := testCase.request(fixture)
			assertF062TerminalProblem(t, recorder)
			assertF062TerminalResidue(t, fixture)
		})
	}
}

func TestF062PublicationWaitsUseProducerStartAsTheBoundedHorizon(t *testing.T) {
	dir := t.TempDir()
	session := &transcodeSession{
		dir: dir, manifest: filepath.Join(dir, "missing.m3u8"), method: "planned-v2-software",
		startedAt: time.Now().UTC().Add(-time.Minute), done: make(chan struct{}), updateCh: make(chan struct{}), admissionActive: true,
	}
	server := &Server{}
	if _, err := server.readTranscodeManifestContext(context.Background(), session, "media", "original", "", 0, "", "", false, "grant"); !errors.Is(err, errHLSManifestTimeout) {
		t.Fatalf("manifest wait error=%v, want producer-anchored timeout", err)
	}
	if err := waitForHLSSegmentFileContext(context.Background(), nil, session, filepath.Join(dir, "segment_00000.ts")); !errors.Is(err, errHLSSegmentTimeout) {
		t.Fatalf("segment wait error=%v, want producer-anchored timeout", err)
	}
}

func TestF062PreProducerAdmissionFailuresRemainRetryable(t *testing.T) {
	for _, err := range []error{
		errMediaResourcesBusy,
		errMediaStoragePressure,
		errPlannedTranscodeSessionLimit,
		errPlannedTranscodeHardwareLimit,
		errPlannedTranscodeSoftwareLimit,
		errPlannedTranscodeBackgroundLimit,
		errPlannedTranscodeBackgroundDeferred,
		errPlannedTranscodeAlreadyAdmitting,
		errPlannedTranscodeRestoreAdmission,
	} {
		if !plannedTranscodeFailureIsRetryableAdmission(err) {
			t.Errorf("pre-producer admission failure became terminal: %v", err)
		}
	}
	for _, err := range []error{
		errors.New("ffmpeg producer exited"),
		errors.New("media facts changed"),
		errors.New("the transcode session limit has been reached"),
		errors.New("restore admission is quiescing"),
		errHLSManifestTimeout,
		errHLSSegmentTimeout,
	} {
		if plannedTranscodeFailureIsRetryableAdmission(err) {
			t.Errorf("terminal producer outcome was classified as admission: %v", err)
		}
	}
	if got := plannedTranscodeFailureClass(errHLSManifestTimeout); got != "manifest_timeout" {
		t.Errorf("manifest timeout class=%q", got)
	}
	if got := plannedTranscodeFailureClass(errHLSSegmentTimeout); got != "segment_timeout" {
		t.Errorf("segment timeout class=%q", got)
	}
}

func f062ManifestRequest(fixture f062PlaybackFixture) *httptest.ResponseRecorder {
	request := mediaGrantRequest(http.MethodGet, "/api/media/"+fixture.item.ID+"/hls/variant.m3u8", fixture.grant.Token)
	recorder := httptest.NewRecorder()
	fixture.server.handleMediaHLSManifest(recorder, request, fixture.user, fixture.item.ID, false)
	return recorder
}

func f062SegmentRequest(fixture f062PlaybackFixture) *httptest.ResponseRecorder {
	request := mediaGrantRequest(http.MethodGet, "/api/media/"+fixture.item.ID+"/hls/segment?name=segment_00000.ts", fixture.grant.Token)
	recorder := httptest.NewRecorder()
	fixture.server.handleMediaHLSSegment(recorder, request, fixture.user, fixture.item.ID)
	return recorder
}

func assertF062TerminalProblem(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Header().Get("Retry-After") != "" {
		t.Fatalf("terminal playback response status=%d retry-after=%q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
	var problem f062Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode F062 problem: %v body=%s", err, recorder.Body.String())
	}
	if problem.Status != http.StatusUnprocessableEntity || problem.Code != "playback_failed" || problem.MessageID != "playback.failed" || problem.Detail != plannedPlaybackFailureDetail {
		t.Fatalf("unexpected F062 problem: %#v", problem)
	}
}

func assertF062TerminalResidue(t *testing.T, fixture f062PlaybackFixture) {
	t.Helper()
	var state, endedAt string
	if err := fixture.server.db.QueryRow(`SELECT state, ended_at FROM playback_sessions WHERE id = ?`, fixture.sessionID).Scan(&state, &endedAt); err != nil {
		t.Fatalf("load terminal F062 session: %v", err)
	}
	if state != "failed" || strings.TrimSpace(endedAt) == "" {
		t.Fatalf("F062 session state=%q endedAt=%q, want terminal failed", state, endedAt)
	}
	for name, query := range map[string]string{
		"grant":        `SELECT COUNT(*) FROM playback_media_grants WHERE playback_session_id = ? AND revoked_at = ''`,
		"continuation": `SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE playback_session_id = ? AND revoked_at = ''`,
	} {
		var count int
		if err := fixture.server.db.QueryRow(query, fixture.sessionID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("active %s residue count=%d err=%v", name, count, err)
		}
	}
	fixture.server.transcodeMu.Lock()
	for key, session := range fixture.server.transcodes {
		if session != nil && session.playbackSessionID == fixture.sessionID {
			fixture.server.transcodeMu.Unlock()
			t.Fatalf("terminal F062 generation remained registered: %s", key)
		}
	}
	fixture.server.transcodeMu.Unlock()
	if snapshots := fixture.server.ffmpegSupervisor.Snapshots(); len(snapshots) != 0 {
		t.Fatalf("terminal F062 supervisor residue: %#v", snapshots)
	}
	_ = filepath.WalkDir(fixture.transcodeDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			t.Errorf("walk F062 transcode residue: %v", err)
			return nil
		}
		if path != fixture.transcodeDir && !entry.IsDir() {
			t.Errorf("terminal F062 artifact remained: %s", path)
		}
		if path != fixture.transcodeDir && entry.IsDir() && entry.Name() != "planned-v2" {
			t.Errorf("terminal F062 generation directory remained: %s", path)
		}
		return nil
	})
}

func f062ContainsAdjacent(values []string, first, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if values[index] == first && values[index+1] == second {
			return true
		}
	}
	return false
}

func f062ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
