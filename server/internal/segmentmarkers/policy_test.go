package segmentmarkers

import "testing"

func TestConfidencePolicyCannotGrantMeasurementOrResolveAmbiguity(t *testing.T) {
	for _, viewer := range []int{30, 50, 70} {
		for _, automatic := range []int{70, 85, 95} {
			p := ConfidencePolicy{viewer, automatic}
			if !p.Valid() {
				t.Fatal(p)
			}
			for _, provenance := range []string{"embedded_chapter_label:v1", "visual_credits:v1", "owner:manual", "season_audio_matcher:v1", "season_audio_match:intro:semantic_ambiguous"} {
				if p.AutomaticSafe(false, 1, provenance) {
					t.Fatalf("threshold conferred trust: %+v %s", p, provenance)
				}
				if !p.AutomaticSafe(true, 0, provenance) || !p.ViewerVisible(true, 0, provenance) {
					t.Fatal("approval lost")
				}
			}
			if p.ViewerVisible(false, 1, "visual_credits:semantic_ambiguous") {
				t.Fatal("ambiguity became visible")
			}
			floor := float64(automatic) / 100
			if !p.AutomaticSafe(false, floor, "season_audio_match:intro:v1") || p.AutomaticSafe(false, floor-.001, "season_audio_match:intro:v1") {
				t.Fatal("automatic boundary", p)
			}
			floor = float64(viewer) / 100
			if !p.ViewerVisible(false, floor, "visual_credits:v1") || p.ViewerVisible(false, floor-.001, "visual_credits:v1") {
				t.Fatal("viewer boundary", p)
			}
		}
	}
}
