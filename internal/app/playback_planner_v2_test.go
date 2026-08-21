package app

import (
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackPlanModePolicyDisablesOnlyPackagingWhenRemuxIsOff(t *testing.T) {
	policy := ResolvedPlaybackPolicy{DirectPlayPolicy: "allow", DirectStreamPolicy: "allow", TranscodePolicy: "allow"}
	allowed, _ := playbackPlanModePolicy(policy, false)
	present := map[playbackplan.Mode]bool{}
	for _, mode := range allowed {
		present[mode] = true
	}
	if present[playbackplan.Remux] || present[playbackplan.DirectStream] {
		t.Fatalf("remux modes remained allowed: %v", allowed)
	}
	if !present[playbackplan.DirectPlay] || !present[playbackplan.VideoTranscode] {
		t.Fatalf("non-remux modes were removed: %v", allowed)
	}
}

func TestPlaybackPlanRequiresVerifiedSoftwareToneMapFilters(t *testing.T) {
	plan := playbackplan.Plan{Stages: []playbackplan.Stage{{Operation: "tone_map_sdr", Execution: "software"}}}
	if !playbackPlanRequiresSoftwareToneMapping(plan) {
		t.Fatal("software tone mapping was not detected")
	}
	plan.Stages[0].Execution = "hardware"
	if playbackPlanRequiresSoftwareToneMapping(plan) {
		t.Fatal("verified hardware tone mapping was treated as software")
	}
}
