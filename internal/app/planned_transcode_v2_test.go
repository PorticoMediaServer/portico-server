package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlannedTranscodeAdmissionEnforcesClassAndBackgroundLimits(t *testing.T) {
	server := &Server{transcodes: make(map[string]*transcodeSession)}
	settings := transcodeSettings{MaxConcurrentSessions: 4, MaxHardwareSessions: 1, MaxSoftwareSessions: 1, MaxBackgroundSessions: 1}
	hardwareRelease, err := server.acquirePlannedTranscodeAdmission("hardware", 1, settings, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.acquirePlannedTranscodeAdmission("hardware-blocked", 1, settings, true, false); err == nil {
		t.Fatal("second hardware generation admitted beyond the hardware limit")
	}
	softwareRelease, err := server.acquirePlannedTranscodeAdmission("software", 1, settings, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.acquirePlannedTranscodeAdmission("software-blocked", 1, settings, false, false); err == nil {
		t.Fatal("second software generation admitted beyond the software limit")
	}
	if _, err := server.acquirePlannedTranscodeAdmission("background-blocked", 1, settings, true, true); err == nil {
		t.Fatal("second background generation admitted beyond the background limit")
	}
	hardwareRelease()
	hardwareRelease() // release is generation-scoped and exactly-once
	if release, err := server.acquirePlannedTranscodeAdmission("hardware-after-release", 1, settings, true, false); err != nil {
		t.Fatalf("hardware capacity was not restored: %v", err)
	} else {
		release()
	}
	softwareRelease()
}

func TestPlannedTranscodeAdmissionZeroSemanticsAndRaceSafety(t *testing.T) {
	server := &Server{transcodes: make(map[string]*transcodeSession)}
	unlimited := transcodeSettings{MaxConcurrentSessions: 0, MaxHardwareSessions: 0, MaxSoftwareSessions: 0, MaxBackgroundSessions: 1}
	for i := 0; i < 8; i++ {
		if _, err := server.acquirePlannedTranscodeAdmission("unlimited-"+strconv.Itoa(i), 1, unlimited, i%2 == 0, false); err != nil {
			t.Fatalf("zero foreground limit should be unlimited: %v", err)
		}
	}
	if _, err := server.acquirePlannedTranscodeAdmission("disabled-background", 1, transcodeSettings{}, false, true); err == nil {
		t.Fatal("zero background limit should disable background admission")
	}

	raced := &Server{transcodes: make(map[string]*transcodeSession)}
	settings := transcodeSettings{MaxConcurrentSessions: 1, MaxHardwareSessions: 1, MaxBackgroundSessions: 1}
	var wg sync.WaitGroup
	var admitted int
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := raced.acquirePlannedTranscodeAdmission("race-"+strconv.Itoa(i), 1, settings, true, false); err == nil {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if admitted != 1 {
		t.Fatalf("race admitted %d generations, want 1", admitted)
	}
}

func TestPlannedTranscodeIdentitySeparatesPlaybackAndAuthorizationNamespaces(t *testing.T) {
	binding := playbackExecutionBinding{Digest: "plan-digest", Generation: 2}
	identity := plannedTranscodeIdentity{
		UserID: "user", ProfileID: "profile", PlaybackSessionID: "session-a",
		AuthorizationRevision: "authorization-a", PlaybackGeneration: 2, GrantTokenHash: "grant-a",
	}
	otherSession := identity
	otherSession.PlaybackSessionID = "session-b"
	otherProfile := identity
	otherProfile.ProfileID = "profile-b"
	otherAuthorization := identity
	otherAuthorization.AuthorizationRevision = "authorization-b"
	otherGeneration := identity
	otherGeneration.PlaybackGeneration = 3
	otherQuality := binding
	otherQuality.Quality = "720p-medium"
	otherTrack := binding
	otherTrack.AudioStreamID = "audio-2"
	otherTrack.SubtitleMode = "text"
	otherTrack.SubtitleStreamID = "subtitle-3"
	key := plannedTranscodeSessionKey("movie", binding, identity, 8)
	if key == plannedTranscodeSessionKey("movie", binding, otherSession, 8) {
		t.Fatal("playback sessions shared a planned transcode registry key")
	}
	if key == plannedTranscodeSessionKey("movie", binding, otherProfile, 8) {
		t.Fatal("playback profiles shared a planned transcode registry key")
	}
	if key == plannedTranscodeSessionKey("movie", binding, otherAuthorization, 8) {
		t.Fatal("authorization revisions shared a planned transcode registry key")
	}
	if key == plannedTranscodeSessionKey("movie", binding, otherGeneration, 8) {
		t.Fatal("playback generations shared a planned transcode registry key")
	}
	if key == plannedTranscodeSessionKey("movie", otherQuality, identity, 8) {
		t.Fatal("quality profiles shared a planned transcode registry key")
	}
	if key == plannedTranscodeSessionKey("movie", otherTrack, identity, 8) {
		t.Fatal("audio/subtitle track selections shared a planned transcode registry key")
	}
	if plannedTranscodeSessionKey("movie/unsafe", binding, identity, 8) == plannedTranscodeSessionKey("movie_unsafe", binding, identity, 8) {
		t.Fatal("media IDs that sanitize alike shared a planned transcode registry key")
	}
	root := t.TempDir()
	first, err := privatePlaybackGenerationPath(root, "movie", 2, binding.Digest, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	second, err := privatePlaybackGenerationPath(root, "movie", 2, binding.Digest, 2, otherSession)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("playback sessions shared a filesystem namespace: %q", first)
	}
	unsafeMediaPath, err := privatePlaybackGenerationPath(root, "movie/unsafe", 2, binding.Digest, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	mediaCollision, err := privatePlaybackGenerationPath(root, "movie_unsafe", 2, binding.Digest, 2, identity)
	if err != nil {
		t.Fatal(err)
	}
	if unsafeMediaPath == mediaCollision {
		t.Fatalf("media IDs that sanitize alike shared a filesystem namespace: %q", unsafeMediaPath)
	}
	if plannedTranscodeSessionKey("movie", binding, identity, 1) != plannedTranscodeSessionKey("movie", binding, identity, 3) {
		t.Fatal("off-grid seeks split one HLS generation into multiple registry namespaces")
	}
}

func TestCompilePlannedVODHLSUsesSealedFactsAndPrivateDeterministicLayout(t *testing.T) {
	_, _, server := newEmptyAuthTestServerWithInstance(t)
	source := filepath.Join(t.TempDir(), "source movie.mkv")
	item := MediaItem{
		ID: "movie/unsafe", Type: "movie", SourceURL: source, DurationSeconds: 120,
		Streams: []Stream{
			{ID: "video", Index: 0, Kind: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: 24, AspectRatio: "16:9", PixelFormat: "yuv420p10le", BitDepth: 10, FieldOrder: "progressive"},
			{ID: "audio", Index: 1, Kind: "audio", Codec: "aac", Channels: 2, ChannelLayout: "stereo", SampleRate: 48000},
		},
	}
	facts, factsDigest, err := server.mediaFactsForPlayback(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	videoIndex, audioIndex := 0, 1
	plan := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: facts.Source.Fingerprint,
		SourceRevision: facts.Source.Revision, CapabilityEvidenceID: "test-evidence",
		Policy: playbackplan.MaximumFidelity, Mode: playbackplan.VideoTranscode,
		MediaKind: playbackcap.MediaAudiovisual, Protocol: "hls", Container: "mpegts", SegmentFormat: "mpegts",
		Selection: playbackplan.Selection{VideoIndex: &videoIndex, AudioIndex: &audioIndex},
		Streams: []playbackplan.StreamAction{
			{Index: 0, Kind: "video", Action: playbackplan.Convert, InputCodec: "hevc", OutputCodec: "h264"},
			{Index: 1, Kind: "audio", Action: playbackplan.Copy, InputCodec: "aac", OutputCodec: "aac"},
		},
		Stages: []playbackplan.Stage{
			{Kind: "video", Operation: "decode", Execution: "software"},
			{Kind: "video", Operation: "encode", Execution: "software"},
			{Kind: "audio", Operation: "copy", Execution: "stream"},
			{Kind: "mux", Operation: "package", Execution: "stream"},
		},
		Audio:    playbackplan.AudioDecision{Codec: "aac", Layout: "stereo", Channels: 2, Passthrough: true},
		Subtitle: playbackplan.SubtitleDecision{Action: playbackplan.Drop},
		Timeline: playbackplan.Timeline{Mode: "vod", DurationUS: 120_000_000, Generation: 2},
		Reasons:  []playbackplan.ReasonCode{playbackplan.ReasonVideoConversion},
	}
	plan.Digest, err = plan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	planJSON, _ := json.Marshal(plan)
	binding := playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: facts.Source.Revision, MediaFactsDigest: factsDigest,
		CapabilityEvidenceID: "test-evidence", Generation: 2, Mode: string(plan.Mode),
		Protocol: "hls", Container: "mpegts", Quality: "1080p", AudioMode: "direct",
		Plan: planJSON, X264Preset: "slower",
	}
	if err := binding.seal(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "generations")
	identity := plannedTranscodeIdentity{
		UserID: "user-playback", ProfileID: "profile-playback", PlaybackSessionID: "session-playback",
		AuthorizationRevision: "auth-revision-1", PlaybackGeneration: 2, GrantTokenHash: "grant-hash-1",
	}
	req := plannedVODHLSRequest{Item: item, Binding: binding, Identity: identity, GenerationRoot: root, SegmentSeconds: 6, StartNumber: 7}
	first, err := server.compilePlannedVODHLS(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.compilePlannedVODHLS(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("compile is not deterministic:\n%#v\n%#v", first, second)
	}
	if strings.Contains(first.GenerationDir, "movie/unsafe") || !strings.HasPrefix(first.GenerationDir, root+string(os.PathSeparator)) {
		t.Fatalf("unsafe generation directory %q", first.GenerationDir)
	}
	info, err := os.Stat(first.GenerationDir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("generation mode = %v, err = %v", info.Mode().Perm(), err)
	}
	wantFragments := []string{"-i", source, "-map", "0:0", "-map", "0:1", "-start_number", "7", "-hls_segment_filename", first.SegmentPattern, first.ManifestPath}
	for _, fragment := range wantFragments {
		if !containsExact(first.Args, fragment) {
			t.Fatalf("argv lacks %q: %#v", fragment, first.Args)
		}
	}
	if first.UsesHardware || first.PlanDigest != binding.Digest || first.MediaFactsDigest != factsDigest {
		t.Fatalf("unexpected command identity: %#v", first)
	}
	seek := req
	seek.StartNumber = 8
	secondSeek, err := server.compilePlannedVODHLS(context.Background(), seek)
	if err != nil {
		t.Fatalf("compile seek-scoped generation: %v", err)
	}
	if first.GenerationDir == secondSeek.GenerationDir || first.ManifestPath == secondSeek.ManifestPath || first.SegmentPattern == secondSeek.SegmentPattern {
		t.Fatalf("seek positions shared publication namespace: first=%#v second=%#v", first, secondSeek)
	}
	unknown := req
	unknown.Item.DurationSeconds = 0
	unknownFacts, unknownFactsDigest, err := server.mediaFactsForPlayback(context.Background(), unknown.Item)
	if err != nil {
		t.Fatal(err)
	}
	unknownPlan := plan
	unknownPlan.SourceFingerprint = unknownFacts.Source.Fingerprint
	unknownPlan.SourceRevision = unknownFacts.Source.Revision
	unknownPlan.Timeline.DurationUS = 0
	unknownPlan.Timeline.Mode = "event"
	unknownPlan.Timeline.Dynamic = true
	unknownPlan.Digest = ""
	unknownPlan.Digest, err = unknownPlan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	unknown.Binding.SourceRevision = unknownFacts.Source.Revision
	unknown.Binding.MediaFactsDigest = unknownFactsDigest
	unknown.Binding.Plan, _ = json.Marshal(unknownPlan)
	if err := unknown.Binding.seal(); err != nil {
		t.Fatal(err)
	}
	unknown.StartNumber = 0
	unknownCommand, err := server.compilePlannedVODHLS(context.Background(), unknown)
	if err != nil {
		t.Fatalf("compile unknown-duration HLS: %v", err)
	}
	unknownArgs := strings.Join(unknownCommand.Args, " ")
	if !strings.Contains(unknownArgs, "-hls_playlist_type event") || !strings.Contains(unknownArgs, "-hls_list_size 0") {
		t.Fatalf("unknown-duration command is not append-only EVENT HLS: %s", unknownArgs)
	}
	if strings.Contains(unknownArgs, "delete_segments") || strings.Contains(unknownArgs, "omit_endlist") {
		t.Fatalf("unknown-duration command used rolling live semantics: %s", unknownArgs)
	}
	remote := req
	remote.RemoteSource = true
	remoteCommand, err := server.compilePlannedVODHLS(context.Background(), remote)
	if err != nil {
		t.Fatalf("compile remote pipe source: %v", err)
	}
	if !containsAdjacent(remoteCommand.Args, "-i", "pipe:0") || containsExact(remoteCommand.Args, source) {
		t.Fatalf("remote command exposed source instead of pipe:0: %#v", remoteCommand.Args)
	}
	storageRemote := remote
	storageRemote.RemoteObjectPath = "Movies/Film.mkv"
	storageRemote.SourcePath = "http://127.0.0.1:28473/probe_opaque_capability"
	storageRemoteCommand, err := server.compilePlannedVODHLS(context.Background(), storageRemote)
	if err != nil {
		t.Fatalf("compile range-capable storage source: %v", err)
	}
	if !containsAdjacent(storageRemoteCommand.Args, "-i", storageRemote.SourcePath) || containsExact(storageRemoteCommand.Args, "pipe:0") {
		t.Fatalf("storage remote command did not use loopback range transport: %#v", storageRemoteCommand.Args)
	}
	unsafeRemote := remote
	unsafeRemote.SourcePath = "http://198.51.100.10/provider-secret"
	if _, err := server.compilePlannedVODHLS(context.Background(), unsafeRemote); err == nil || !strings.Contains(err.Error(), "loopback capability") {
		t.Fatalf("non-loopback transcode transport accepted: %v", err)
	}

	changed := req
	changed.Binding.MediaFactsDigest = "mediafacts-v2:sha256:changed"
	if err := changed.Binding.seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.compilePlannedVODHLS(context.Background(), changed); err == nil || !strings.Contains(err.Error(), "media facts changed") {
		t.Fatalf("changed facts error = %v", err)
	}
}

func TestExactSidecarSubtitlePathRejectsEmbeddedOrDifferentSelection(t *testing.T) {
	index := 4
	plan := playbackplan.Plan{Selection: playbackplan.Selection{SubtitleIndex: &index}}
	item := MediaItem{Streams: []Stream{{Index: 4, Kind: "subtitle"}}}
	if _, err := exactSidecarSubtitlePath(item, plan); err == nil {
		t.Fatal("embedded subtitle was accepted without exact extraction resolver")
	}
	path := filepath.Join(t.TempDir(), "selected.srt")
	item.Streams = []Stream{{Index: 3, Kind: "subtitle", SourceURL: path}, {Index: 4, Kind: "subtitle", SourceURL: path}}
	got, err := exactSidecarSubtitlePath(item, plan)
	if err != nil || got != path {
		t.Fatalf("sidecar = %q, %v", got, err)
	}
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsAdjacent(values []string, first, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if values[index] == first && values[index+1] == second {
			return true
		}
	}
	return false
}
