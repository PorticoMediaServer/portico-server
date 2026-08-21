package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackExecutionProjectionIsAccurateAndPrivate(t *testing.T) {
	plan := playbackplan.Plan{
		SchemaRevision: playbackplan.SchemaRevision, SourceFingerprint: "sha256:private-source",
		SourceRevision: "private-revision", CapabilityEvidenceID: "private-capability",
		Mode: playbackplan.VideoTranscode, Protocol: "hls", Container: "mpegts",
		Streams: []playbackplan.StreamAction{
			{Index: 0, Kind: "video", Action: playbackplan.Convert, InputCodec: "hevc", OutputCodec: "h264"},
			{Index: 1, Kind: "audio", Action: playbackplan.Convert, InputCodec: "truehd", OutputCodec: "aac"},
		},
		Timeline: playbackplan.Timeline{Mode: "vod", Generation: 2},
		Hardware: playbackplan.HardwareRoute{Verified: true, Backend: playbackhw.VideoToolbox, Stages: []playbackplan.Stage{{Kind: "video", Operation: "encode", Execution: "hardware"}}},
		Reasons:  []playbackplan.ReasonCode{playbackplan.ReasonVideoConversion, playbackplan.ReasonAudioConversion},
	}
	plan.Digest, _ = plan.ComputeDigest()
	planJSON, _ := json.Marshal(plan)
	binding := playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: plan.SourceRevision, MediaFactsDigest: "private-facts",
		CapabilityEvidenceID: plan.CapabilityEvidenceID, Generation: 2, Mode: string(plan.Mode),
		Protocol: plan.Protocol, Container: plan.Container, Plan: planJSON,
		X264Preset: "veryfast",
		HardwarePlan: &playbackhw.Plan{Backend: playbackhw.VideoToolbox, RuntimeIdentity: playbackhw.RuntimeIdentity{
			ExecutablePath: "/private/bin/ffmpeg", BinaryFingerprint: "private-binary", DeviceIdentity: "private-device",
			DriverIdentity: "private-driver", DriverVersion: "private-version",
		}},
	}
	if err := binding.seal(); err != nil {
		t.Fatal(err)
	}
	projection, _, ok := playbackExecutionProjection(binding)
	if !ok {
		t.Fatal("expected valid projection")
	}
	if projection.Mode != "video_transcode" || projection.Protocol != "hls" || projection.Container != "mpegts" {
		t.Fatalf("projection = %#v", projection)
	}
	if len(projection.Streams) != 2 || projection.Streams[0].InputCodec != "hevc" || projection.Streams[0].OutputCodec != "h264" {
		t.Fatalf("streams = %#v", projection.Streams)
	}
	if projection.Hardware.Backend != string(playbackhw.VideoToolbox) || len(projection.Hardware.Stages) != 1 {
		t.Fatalf("hardware = %#v", projection.Hardware)
	}
	encoded, _ := json.Marshal(projection)
	for _, secret := range []string{"/private/bin/ffmpeg", "private-source", "private-revision", "private-capability", "private-device", "private-driver", "private-binary"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("diagnostics leaked %q: %s", secret, encoded)
		}
	}
}

func TestPlaybackResourceDiagnosticsAggregatesWithoutFilesystemIdentity(t *testing.T) {
	server := &Server{mediaResources: newMediaResourceGovernor()}
	governor := server.mediaResources
	governor.cpuUsed, governor.diskUsed, governor.networkUsed, governor.backgroundCPUUsed = 1, 2, 3, 1
	governor.diskReservedBytes = map[string]int64{"private-volume-a": 100, "private-volume-b": 250}
	got := server.playbackResourceDiagnostics()
	if got.CPUUsed != 1 || got.DiskUsed != 2 || got.NetworkUsed != 3 || got.ReservedDiskBytes != 350 || got.ReservedFilesystems != 2 {
		t.Fatalf("resources = %#v", got)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "private-volume") {
		t.Fatalf("resource diagnostics leaked filesystem identity: %s", encoded)
	}
}

func TestSystemDiagnosticsRejectsNonOwner(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/system/diagnostics", nil)
	(&Server{}).handleSystemDiagnostics(recorder, request, User{Role: "user", ProfileIsPrimary: true})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}
