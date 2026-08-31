package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/optimized"
	"github.com/PorticoMediaServer/portico-server/internal/optimizedartifact"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestOptimizedV2PlanIdentityIsStableAndPresetBound(t *testing.T) {
	source := optimizedV2SourceIdentity{Revision: "media-revision-7", Fingerprint: "source-fingerprint",
		FactsDigest: "mediafacts-v2:sha256:facts", Facts: optimized.SourceFacts{Width: 3840, Height: 2160,
			SARNumerator: 1, SARDenominator: 1, DynamicRange: optimized.RangeHDR10,
			AudioCodec: "eac3", AudioChannels: 6, AudioLayout: "5.1"}}
	server := &Server{}
	a, err := newOptimizedV2Publication(server, t.TempDir(), "media-1", "efficient-1080p", optimized.RouteSoftwareHEVC, source)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newOptimizedV2Publication(server, a.root, "media-1", "efficient-1080p", optimized.RouteSoftwareHEVC, source)
	if err != nil {
		t.Fatal(err)
	}
	if a.planDigest != b.planDigest || a.identity.GenerationID != b.identity.GenerationID {
		t.Fatal("identical optimized request did not produce a stable immutable identity")
	}
	if a.plan.PresetID != "efficient-1080p" || a.plan.PresetVersion != 1 || a.plan.EncoderRoute != optimized.RouteSoftwareHEVC {
		t.Fatalf("wrong sealed plan: %+v", a.plan)
	}
	if len(optimized.List()) != 8 {
		t.Fatal("production adapter must expose exactly eight presets")
	}
}

func TestOptimizedV2ReadyArtifactRequiresExactSourceAndReprobedFacts(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	root := filepath.Join(server.cfg.AppDataDir, "optimized", "media-ready")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "artifact.mp4")
	if err := os.WriteFile(path, []byte("artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	probeJSON := []byte(`{"format":{"format_name":"mp4","duration":"120"},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":1280,"height":720}]}`)
	source := optimizedV2SourceIdentity{Revision: "source-r1", Fingerprint: "source-fp", FactsDigest: "facts-digest"}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib-ready', 'Ready', 'movie', ?); INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES ('media-ready', 'lib-ready', 'movie', 'Ready', 'Ready', ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO optimized_versions
		(id, media_id, profile, path, size_bytes, artifact_sha256, created_at, updated_at, state, preset_version,
		 planner_revision, source_revision, source_fingerprint, source_facts_digest, plan_digest,
		 plan_json, output_facts_digest, output_facts_json, compatibility_tags_json,
		 container, video_codec, audio_codec, width, height, bitrate, duration_seconds)
		VALUES ('artifact-ready', 'media-ready', 'universal-720p', ?, ?, 'c7c5c1d70c5dec4416ab6158afd0b223ef40c29b1dc1f97ed9428b94d4cadb1c', ?, ?, 'ready', 1,
		 ?, ?, ?, ?, 'plan-digest', '{"presetId":"universal-720p"}', ?, ?, '["universal"]',
		 'mp4', 'h264', 'aac', 1280, 720, 1000000, 120)`, path, len("artifact"), now, now,
		optimized.PlannerRevision, source.Revision, source.Fingerprint, source.FactsDigest,
		optimizedV2Digest(probeJSON), string(probeJSON)); err != nil {
		t.Fatal(err)
	}
	ready, err := server.optimizedV2ReadyForSource(context.Background(), "media-ready", "universal-720p", source, func(path string, size int64) bool {
		return server.optimizedArtifactUsable(context.Background(), path, size)
	})
	if err != nil || ready == nil || ready.ID != "artifact-ready" || ready.OutputFactsDigest == "" {
		t.Fatalf("current artifact = %+v, err=%v", ready, err)
	}
	stale := source
	stale.Revision = "source-r2"
	if ready, err = server.optimizedV2ReadyForSource(context.Background(), "media-ready", "universal-720p", stale, func(string, int64) bool { return true }); err != nil || ready != nil {
		t.Fatalf("stale source resolved artifact = %+v, err=%v", ready, err)
	}
	if _, err := db.Exec(`UPDATE optimized_versions SET output_facts_json = '{"tampered":true}' WHERE id = 'artifact-ready'`); err != nil {
		t.Fatal(err)
	}
	if ready, err = server.optimizedV2ReadyForSource(context.Background(), "media-ready", "universal-720p", source, func(string, int64) bool { return true }); err != nil || ready != nil {
		t.Fatalf("tampered output facts resolved artifact = %+v, err=%v", ready, err)
	}
}

func TestOptimizedPlaybackBindingSealsExactArtifactRoute(t *testing.T) {
	var canonical playbackplan.Plan
	if err := json.Unmarshal([]byte(`{"schemaRevision":"playback-plan-v2","sourceFingerprint":"fp","sourceRevision":"optimized-r1","capabilityEvidenceId":"cap","policy":"maximum-fidelity","mode":"direct_play","mediaKind":"video","protocol":"http","container":"mp4","selection":{},"streams":[{"index":0,"kind":"video","action":"copy","inputCodec":"h264","outputCodec":"h264"}],"audio":{"codec":"aac","channels":2,"passthrough":true,"objectsPreserved":false,"downmixed":false},"subtitle":{"action":"drop"},"timeline":{"mode":"vod","generation":1,"dynamic":false},"hardware":{"verified":false,"softwareFallbackVerified":false},"constraints":{},"reasons":[]}`), &canonical); err != nil {
		t.Fatal(err)
	}
	binding := testPlaybackExecutionPlan(t, func(plan *playbackExecutionPlan) {
		plan.Plan = canonical
		plan.OptimizedArtifactID = "artifact-exact"
		plan.OptimizedPresetID = "universal-720p"
	})
	if got := mediaPlaybackURLForDecision("media-1", PlaybackDecision{executionPlan: &binding}); got != "/api/media/media-1/optimized/universal-720p" {
		t.Fatalf("optimized playback URL = %q", got)
	}
}

func TestCanonicalPlannerPrefersOnlyDirectPlayableCurrentOptimizedArtifact(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mediaRoot := filepath.Join(t.TempDir(), "media")
	if err := os.MkdirAll(mediaRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(mediaRoot, "source.mkv")
	if err := os.WriteFile(sourcePath, []byte("source-media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, path, created_at) VALUES ('lib-plan-opt', 'Movies', 'movie', ?, ?);
		INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('lp-plan-opt', 'lib-plan-opt', ?, 0, ?);
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('media-plan-opt', 'lib-plan-opt', 'movie', 'Plan Optimized', 'Plan Optimized', ?, ?, 120);
		INSERT INTO media_streams (id, media_id, stream_index, kind, codec, channels, channel_layout, width, height, pixel_format, display_title)
		VALUES ('v-plan-opt', 'media-plan-opt', 0, 'video', 'hevc', 0, '', 3840, 2160, 'yuv420p10le', 'HEVC'),
		       ('a-plan-opt', 'media-plan-opt', 1, 'audio', 'truehd', 8, '7.1', 0, 0, '', 'TrueHD 7.1')`,
		mediaRoot, now, mediaRoot, now, now, sourcePath); err != nil {
		t.Fatal(err)
	}
	item, err := server.getMediaBackgroundSourceSeedContext(context.Background(), "media-plan-opt")
	if err != nil {
		t.Fatal(err)
	}
	item.Streams, _ = server.listStreamsContext(context.Background(), item.ID)
	item.MediaFiles = server.primaryMediaFileForPlaybackContext(context.Background(), item.ID, item.SourceURL)
	originalFacts, originalDigest, err := server.mediaFactsForPlayback(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	sourceIdentity, err := optimizedSourceIdentityFromFacts(originalFacts, originalDigest)
	if err != nil {
		t.Fatal(err)
	}
	artifactRoot := filepath.Join(server.cfg.AppDataDir, "optimized", item.ID)
	if err := os.MkdirAll(artifactRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(artifactRoot, "universal-720p.mp4")
	if err := os.WriteFile(artifactPath, []byte("optimized-media"), 0o600); err != nil {
		t.Fatal(err)
	}
	probeJSON := []byte(`{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"120","bit_rate":"2500000"},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","profile":"Main","width":1280,"height":720,"pix_fmt":"yuv420p","sample_aspect_ratio":"1:1","avg_frame_rate":"24/1"},{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"channel_layout":"stereo","sample_rate":"48000"}]}`)
	if _, err := db.Exec(`INSERT INTO optimized_versions
		(id, media_id, profile, path, size_bytes, artifact_sha256, created_at, updated_at, state, preset_version,
		 planner_revision, source_revision, source_fingerprint, source_facts_digest, plan_digest,
		 plan_json, output_facts_digest, output_facts_json, compatibility_tags_json,
		 container, video_codec, audio_codec, width, height, bitrate, duration_seconds)
		VALUES ('artifact-plan-opt', ?, 'universal-720p', ?, ?, '23b1494108427eb7802b6055b04f0f4828eb66b86e4d1172c65447b09395a9ef', ?, ?, 'ready', 1, ?, ?, ?, ?,
		 'plan-current', '{"presetId":"universal-720p"}', ?, ?, '["universal"]',
		 'mp4', 'h264', 'aac', 1280, 720, 2500000, 120)`, item.ID, artifactPath, len("optimized-media"), now, now,
		optimized.PlannerRevision, sourceIdentity.Revision, sourceIdentity.Fingerprint, sourceIdentity.FactsDigest,
		optimizedV2Digest(probeJSON), string(probeJSON)); err != nil {
		t.Fatal(err)
	}
	saveOptimizedSettings(t, db, `{"defaultProfile":"universal-720p","preferOptimizedPlayback":true}`)
	decision, err := server.planMediaPlayback(context.Background(), item, PlaybackClientProfile{
		Device: "Browser", Platform: "macOS Chrome", SupportsHLS: true, SupportsMSE: true,
		SupportedContainers: []string{"mp4", "hls"}, SupportedVideoCodecs: []string{"h264"},
		SupportedVideoProfiles: []string{"h264:main"}, SupportedAudioCodecs: []string{"aac"},
		MaxWidth: 1920, MaxHeight: 1080, MaxAudioChannels: 2,
	}, ResolvedPlaybackPolicy{}, "", "", "off")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Mode != "optimized_version" || decision.executionPlan == nil || decision.executionPlan.OptimizedArtifactID != "artifact-plan-opt" {
		t.Fatalf("optimized decision = %+v, plan=%+v", decision, decision.executionPlan)
	}
}

func TestOptimizedLocalFilesystemPersistsPrivateDurableMarkers(t *testing.T) {
	root := t.TempDir()
	fs := optimizedLocalFilesystem{root: root}
	marker := optimizedartifact.Marker{ID: "generation.publication", Stage: optimizedartifact.StageSynced,
		Metadata: optimizedartifact.Metadata{GenerationID: "generation", MediaID: "media", PresetVersion: "universal-720p:v1"},
		TempPath: filepath.Join(root, "artifact.partial"), FinalPath: filepath.Join(root, "artifact.mp4")}
	if err := fs.PutMarker(context.Background(), marker); err != nil {
		t.Fatal(err)
	}
	markers, err := fs.markers()
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 || markers[0].ID != marker.ID || markers[0].Stage != marker.Stage {
		t.Fatalf("marker round trip = %+v", markers)
	}
	info, err := os.Stat(fs.markerPath(marker.ID))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("marker permissions = %o", info.Mode().Perm())
	}
	if err := fs.DeleteMarker(context.Background(), marker.ID); err != nil {
		t.Fatal(err)
	}
	markers, err = fs.markers()
	if err != nil || len(markers) != 0 {
		t.Fatalf("markers after delete = %+v, %v", markers, err)
	}
}

func TestOptimizedV2ReconciliationRejectsNoncanonicalMarkerPathsWithoutTouchingArtifact(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	root := server.optimizedVersionStorageDir()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO libraries (id, name, type, created_at) VALUES ('lib-reconcile', 'Reconcile', 'movie', ?);
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at) VALUES ('media-reconcile', 'lib-reconcile', 'movie', 'Reconcile', 'Reconcile', ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	source := optimizedV2SourceIdentity{Revision: "source-r1", Fingerprint: "source-fp", FactsDigest: "facts-digest",
		Facts: optimized.SourceFacts{Width: 1280, Height: 720, SARNumerator: 1, SARDenominator: 1,
			DynamicRange: optimized.RangeSDR, AudioCodec: "aac", AudioChannels: 2}}
	publication, err := newOptimizedV2Publication(server, root, "media-reconcile", "universal-720p", optimized.RouteSoftwareH264, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(publication.identity.FinalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	arbitrary := filepath.Join(filepath.Dir(publication.identity.FinalPath), "unrelated-user-artifact.mp4")
	if err := os.WriteFile(arbitrary, []byte("do-not-touch"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical := optimizedartifact.Marker{ID: publication.identity.MarkerID, Stage: optimizedartifact.StageSynced,
		Metadata: optimizedartifact.Metadata{GenerationID: publication.identity.GenerationID, MediaID: publication.mediaID,
			PresetVersion: optimizedV2PresetVersion(publication.preset), SourceFingerprint: source.Fingerprint,
			PlanDigest: publication.planDigest, Path: publication.identity.FinalPath},
		TempPath: publication.identity.TempPath, FinalPath: publication.identity.FinalPath}

	tests := []struct {
		name   string
		mutate func(*optimizedartifact.Marker)
	}{
		{"media", func(m *optimizedartifact.Marker) { m.Metadata.MediaID = "other-media" }},
		{"metadata path", func(m *optimizedartifact.Marker) { m.Metadata.Path = arbitrary }},
		{"temporary path", func(m *optimizedartifact.Marker) { m.TempPath = arbitrary }},
		{"final path", func(m *optimizedartifact.Marker) { m.FinalPath = arbitrary }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			marker := canonical
			test.mutate(&marker)
			if _, err := server.reconcileOptimizedV2Marker(context.Background(), optimizedLocalFilesystem{root: root, server: server}, marker); err == nil {
				t.Fatal("noncanonical marker was accepted")
			}
			if body, err := os.ReadFile(arbitrary); err != nil || string(body) != "do-not-touch" {
				t.Fatalf("arbitrary in-root artifact was touched: body=%q err=%v", body, err)
			}
		})
	}
}

func TestOptimizedLocalFilesystemRejectsSymlinkComponents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	outsideFile := filepath.Join(outside, "artifact.mp4")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := optimizedLocalFilesystem{root: root}
	linkedFile := filepath.Join(link, "artifact.mp4")
	if file, err := fs.CreatePrivate(filepath.Join(link, "new.partial")); err == nil {
		_ = file.Close()
		t.Fatal("create followed a symlink component")
	}
	if exists, err := fs.Exists(linkedFile); err == nil || exists {
		t.Fatalf("exists followed symlink component: exists=%v err=%v", exists, err)
	}
	if err := fs.Remove(linkedFile); err == nil {
		t.Fatal("remove followed a symlink component")
	}
	from := filepath.Join(root, "rename-source.partial")
	if err := os.WriteFile(from, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fs.Rename(from, filepath.Join(link, "renamed.mp4")); err == nil {
		t.Fatal("rename followed a symlink component")
	}
	if body, err := os.ReadFile(outsideFile); err != nil || string(body) != "outside" {
		t.Fatalf("outside artifact was touched: body=%q err=%v", body, err)
	}
}

func TestOptimizedV2RejectsUnknownPresetAndIncompleteOutputFacts(t *testing.T) {
	source := optimizedV2SourceIdentity{Revision: "r", Fingerprint: "f", FactsDigest: "d",
		Facts: optimized.SourceFacts{Width: 1920, Height: 1080, AudioChannels: 2}}
	if _, err := newOptimizedV2Publication(&Server{}, t.TempDir(), "media", "legacy-720p", optimized.RouteSoftwareH264, source); err == nil {
		t.Fatal("unknown legacy preset accepted")
	}
	if err := validateOptimizedV2OutputFacts(optimizedV2OutputFacts{SizeBytes: 1, Container: "mp4", VideoCodec: "h264",
		FactsDigest: "digest", FactsJSON: json.RawMessage(`[]`)}); err == nil {
		t.Fatal("non-object output facts accepted")
	}
	factsJSON := json.RawMessage(`{"streams":[]}`)
	if err := validateOptimizedV2OutputFacts(optimizedV2OutputFacts{SizeBytes: 1, ArtifactSHA256: "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881", Container: "mp4", VideoCodec: "h264",
		FactsDigest: optimizedV2Digest(factsJSON), FactsJSON: factsJSON}); err != nil {
		t.Fatalf("valid output facts rejected: %v", err)
	}
}

func TestOptimizedProfilesExposeExactOrderedRegistry(t *testing.T) {
	server := &Server{}
	profiles := server.optimizedVersionProfiles()
	want := []string{"universal-1080p", "universal-720p", "universal-480p", "efficient-4k", "efficient-1080p", "efficient-720p", "maximum-compression-source", "maximum-compression-1080p"}
	if len(profiles) != len(want) {
		t.Fatalf("profile count = %d, want %d", len(profiles), len(want))
	}
	for i := range want {
		if profiles[i].ID != want[i] {
			t.Fatalf("profile %d = %q, want %q", i, profiles[i].ID, want[i])
		}
	}
	if got := server.requestedOptimizedProfile("720p-medium"); got != "720p-medium" {
		t.Fatalf("request normalization silently aliased legacy preset to %q", got)
	}
}

func TestOptimizedDownloadGrantAcceptsOnlyRegistryProfiles(t *testing.T) {
	server := &Server{}
	// A real Server reads the same normalized registry value from settings; use
	// an explicit profile here so this unit remains independent of a database.
	if got, err := server.normalizeMediaDownloadGrantProfile("efficient-1080p"); err != nil || got != "efficient-1080p" {
		t.Fatalf("exact optimized profile = %q, err=%v", got, err)
	}
	if _, err := server.normalizeMediaDownloadGrantProfile("720p-medium"); err == nil {
		t.Fatal("removed legacy optimized profile was accepted")
	}
}

func TestOptimizedOutputValidationIsSealedPlanExact(t *testing.T) {
	preset, _ := optimized.Lookup("universal-720p")
	plan, err := optimized.PlanForRoute(preset, optimized.SourceFacts{Width: 1920, Height: 1080, SARNumerator: 1,
		SARDenominator: 1, DynamicRange: optimized.RangeSDR, AudioCodec: "aac", AudioChannels: 2}, optimized.RouteSoftwareH264)
	if err != nil {
		t.Fatal(err)
	}
	good := optimizedV2OutputFacts{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: plan.Geometry.Width, Height: plan.Geometry.Height,
		SampleAspectRatio: "1:1", FieldOrder: "progressive", PixelFormat: "yuv420p", ColorPrimaries: "bt709", ColorTransfer: "bt709", ColorMatrix: "bt709",
		AudioChannels: plan.Audio.Channels, AudioLayout: plan.Audio.Layout}
	if err := validateOptimizedOutputAgainstPlan(good, plan); err != nil {
		t.Fatalf("matching output rejected: %v", err)
	}
	wrongCodec := good
	wrongCodec.VideoCodec = "hevc"
	if err := validateOptimizedOutputAgainstPlan(wrongCodec, plan); err == nil {
		t.Fatal("wrong video codec accepted")
	}
	wrongGeometry := good
	wrongGeometry.Width += 2
	if err := validateOptimizedOutputAgainstPlan(wrongGeometry, plan); err == nil {
		t.Fatal("wrong geometry accepted")
	}
}

func TestOptimizedHardwareArgsRequireVerifiedDeviceAndRepresentUpload(t *testing.T) {
	preset, _ := optimized.Lookup("efficient-1080p")
	plan, err := optimized.PlanForRoute(preset, optimized.SourceFacts{Width: 3840, Height: 2160, SARNumerator: 1,
		SARDenominator: 1, DynamicRange: optimized.RangeHDR10, AudioCodec: "eac3", AudioChannels: 6}, optimized.RouteVAAPI)
	if err != nil {
		t.Fatal(err)
	}
	audio := mediafacts.Audio{Codec: "eac3", Layout: "5.1", Channels: 6, SampleRate: 48000}
	if _, err := optimizedFFmpegArgs(plan, "/media/source.mkv", nil, audio); err == nil {
		t.Fatal("VAAPI command accepted without exact verified runtime identity")
	}
	hardware := &playbackhw.Plan{Backend: playbackhw.VAAPI, RuntimeIdentity: playbackhw.RuntimeIdentity{DevicePath: "/dev/dri/renderD128"}}
	args, err := optimizedFFmpegArgs(plan, "/media/source.mkv", hardware, audio)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{"-vaapi_device /dev/dri/renderD128", "format=p010le", "hwupload=extra_hw_frames=64", "hevc_vaapi"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("hardware command missing %q: %s", required, joined)
		}
	}
}

func TestOptimizedAudioUsesSharedExplicitDownmixGraph(t *testing.T) {
	preset, _ := optimized.Lookup("universal-720p")
	plan, err := optimized.PlanForRoute(preset, optimized.SourceFacts{Width: 1920, Height: 1080, SARNumerator: 1,
		SARDenominator: 1, DynamicRange: optimized.RangeSDR, AudioCodec: "truehd", AudioChannels: 8, AudioLayout: "7.1"}, optimized.RouteSoftwareH264)
	if err != nil {
		t.Fatal(err)
	}
	args, err := optimizedFFmpegArgs(plan, "/media/source.mkv", nil, mediafacts.Audio{Codec: "truehd", Layout: "7.1", Channels: 8, SampleRate: 96000})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-af pan=stereo", "0.25*LFE", "alimiter=limit=0.95", "-c:a aac", "-channel_layout stereo", "-ar 48000"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("optimized audio graph lacks %q: %s", want, joined)
		}
	}
}

func TestOptimizedVideoGraphSealsRotationInterlaceAndHLGMetadata(t *testing.T) {
	preset, _ := optimized.Lookup("efficient-1080p")
	plan, err := optimized.PlanForRoute(preset, optimized.SourceFacts{Width: 1920, Height: 1080, SARNumerator: 4,
		SARDenominator: 3, Rotation: 90, Interlaced: true, DynamicRange: optimized.RangeHLG,
		AudioCodec: "aac", AudioChannels: 2, AudioLayout: "stereo"}, optimized.RouteSoftwareHEVC)
	if err != nil {
		t.Fatal(err)
	}
	args, err := optimizedFFmpegArgs(plan, "/media/source.mkv", nil, mediafacts.Audio{Codec: "aac", Layout: "stereo", Channels: 2, SampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-noautorotate", "bwdif=", "transpose=clock", "setsar=1", "-color_primaries bt2020", "-color_trc arib-std-b67", "transfer=arib-std-b67"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("optimized graph lacks %q: %s", want, joined)
		}
	}
	if strings.Index(joined, "bwdif=") > strings.Index(joined, "transpose=clock") || strings.Index(joined, "transpose=clock") > strings.Index(joined, "scale=") {
		t.Fatalf("optimized transform order is not deinterlace -> rotate -> scale: %s", joined)
	}
}
