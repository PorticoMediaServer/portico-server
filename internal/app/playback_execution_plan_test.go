package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackExecutionPlanHasOneVODPlannerAndNoLegacyAuthority(t *testing.T) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate app source directory")
	}
	dir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, removed := range []string{
			"type playbackExecutionBinding struct",
			"func decideMediaPlayback(",
			"func applyDirectStreamRemuxPolicy(",
			"type mediaPlaybackContract struct",
			"func mediaPlaybackCanUseDirectStream(",
		} {
			if strings.Contains(source, removed) {
				t.Fatalf("legacy VOD authority %q survived in %s", removed, name)
			}
		}
		if strings.Contains(source, "playbackplan.Build(") && name != "playback_planner_v2.go" {
			t.Fatalf("VOD playback plan is built outside its canonical owner: %s", name)
		}
		if strings.Contains(source, "applyResolvedDeliveryMode(") && name != "delivery_policy.go" && name != "live_tv.go" {
			t.Fatalf("legacy resolved-delivery adapter escaped its Live-only boundary: %s", name)
		}
	}
	planStore, err := os.ReadFile(filepath.Join(dir, "playback_plan_store.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planStore), "json.RawMessage") {
		t.Fatal("sealed execution plan reparses its canonical playback plan from raw JSON")
	}
}

func testPlaybackExecutionPlan(t *testing.T, configure func(*playbackExecutionPlan)) playbackExecutionPlan {
	t.Helper()
	plan := playbackExecutionPlan{
		SchemaVersion: playbackExecutionPlanSchemaVersion,
		Quality:       "original",
		Plan: playbackplan.Plan{
			SchemaRevision:       playbackplan.SchemaRevision,
			SourceFingerprint:    "test-source-fingerprint",
			SourceRevision:       "test-source-revision",
			CapabilityEvidenceID: "test-capability-evidence",
			Mode:                 playbackplan.Remux,
			Protocol:             "hls",
			Container:            "mpegts",
			Streams:              []playbackplan.StreamAction{{Index: 0, Kind: "video", Action: playbackplan.Copy, InputCodec: "h264", OutputCodec: "h264"}},
			Subtitle:             playbackplan.SubtitleDecision{Action: playbackplan.Drop},
			Timeline:             playbackplan.Timeline{Mode: "vod", Generation: 1},
		},
	}
	if configure != nil {
		configure(&plan)
	}
	if plan.requiresX264Preset() && plan.X264Preset == "" {
		plan.X264Preset = "veryfast"
	}
	plan.Plan.Digest, _ = plan.Plan.ComputeDigest()
	if err := plan.seal(); err != nil {
		t.Fatalf("seal test playback execution plan: %v", err)
	}
	return plan
}

func resealTestPlaybackExecutionPlan(t *testing.T, plan playbackExecutionPlan) playbackExecutionPlan {
	t.Helper()
	plan.Plan.Digest, _ = plan.Plan.ComputeDigest()
	if err := plan.seal(); err != nil {
		t.Fatalf("reseal test playback execution plan: %v", err)
	}
	return plan
}
