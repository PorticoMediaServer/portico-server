package app

import (
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/optimized"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPredictedPlaybackOutputUsesPlanAndSourceFacts(t *testing.T) {
	copyPlan := playbackplan.Plan{Streams: []playbackplan.StreamAction{{Kind: "video", Action: playbackplan.Copy}}}
	facts := mediafacts.Facts{Source: mediafacts.Source{SizeBytes: 1 << 30}}
	if got := predictedPlaybackOutputBytes(copyPlan, facts); got <= facts.Source.SizeBytes {
		t.Fatalf("copy estimate %d did not include packaging headroom", got)
	}

	encode := playbackplan.Plan{Timeline: playbackplan.Timeline{DurationUS: 3_600_000_000}, Constraints: playbackplan.Constraints{MaxVideoBitrate: 5_000_000, MaxAudioBitrate: 384_000}}
	got := predictedPlaybackOutputBytes(encode, mediafacts.Facts{})
	if got < 2_800_000_000 || got > 3_000_000_000 {
		t.Fatalf("one-hour estimate = %d", got)
	}
	if got := predictedPlaybackOutputBytes(playbackplan.Plan{}, mediafacts.Facts{}); got != unknownMediaOutputReservation {
		t.Fatalf("unknown estimate = %d", got)
	}
}

func TestPredictedOptimizedOutputUsesConservativePlanBound(t *testing.T) {
	plan := optimized.OutputPlan{PresetID: "universal-1080p", EncoderRoute: optimized.RouteSoftwareH264,
		Geometry: optimized.Geometry{Width: 1920, Height: 1080}, Audio: optimized.AudioDecision{Channels: 2}}
	facts := mediafacts.Facts{Source: mediafacts.Source{SizeBytes: 500 << 20}}
	got := predictedOptimizedOutputBytes(plan, facts, 3600)
	if got <= facts.Source.SizeBytes || got < 8_000_000_000 {
		t.Fatalf("optimized estimate %d did not reserve a conservative quality-encode bound", got)
	}
	if got := predictedOptimizedOutputBytes(optimized.OutputPlan{}, mediafacts.Facts{}, 0); got != unknownMediaOutputReservation {
		t.Fatalf("unknown optimized estimate = %d", got)
	}
}
