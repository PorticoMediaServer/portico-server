package segmentmarkers

import "testing"

func TestAutomaticSafeAuthorizesApprovalAndMeasuredConfidenceOnly(t *testing.T) {
	for _, c := range []struct {
		name       string
		approved   bool
		confidence float64
		provenance string
		safe       bool
	}{
		{"owner approved low confidence detector", true, .2, "luminance_opening_boundary:v1", true},
		{"owner manual marker", true, 1, "owner:manual", true},
		{"unapproved detector at the floor", false, .85, "sustained_end_title_luminance:v1", true},
		{"unapproved detector above the floor", false, .97, "sustained_end_title_luminance:v1", true},
		{"unapproved detector below the floor", false, .84999, "sustained_end_title_luminance:v1", false},
		{"unapproved chapter title heuristic at the floor", false, .85, "embedded_chapter_label:v1", false},
		{"unapproved ambiguous detector at the floor", false, .95, "paired_black_boundaries_semantic_ambiguous:v1", false},
		{"unapproved owner provenance", false, 1, "owner:manual", false},
		{"unapproved without provenance", false, 1, "", false},
	} {
		if got := AutomaticSafe(c.approved, c.confidence, c.provenance); got != c.safe {
			t.Fatalf("%s: automaticSafe=%v want %v", c.name, got, c.safe)
		}
	}
}

func TestViewerKindExcludesOwnerOnlyKinds(t *testing.T) {
	for _, kind := range []string{"intro", "recap", "credits", "commercial", "outro"} {
		if !ViewerKind(kind) {
			t.Fatalf("viewer kind %q rejected", kind)
		}
	}
	for _, kind := range []string{"chapter", "", "Intro", "advert"} {
		if ViewerKind(kind) {
			t.Fatalf("non-viewer kind %q projected", kind)
		}
	}
}
