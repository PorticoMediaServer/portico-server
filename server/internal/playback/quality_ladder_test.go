package playback

import (
	"strings"
	"testing"
)

func rungIDs(rungs []QualityRung) []string {
	out := make([]string, 0, len(rungs))
	for _, r := range rungs {
		out = append(out, r.ID)
	}
	return out
}

func TestQualityLadderCapsBySourceHeight(t *testing.T) {
	policy := openPolicy()
	cases := []struct {
		height int
		want   string
	}{
		{2160, "auto original 2160p 1440p 1080p 720p 480p 360p"},
		{1080, "auto original 1080p 720p 480p 360p"},
		{720, "auto original 720p 480p 360p"},
		{480, "auto original 480p 360p"},
		// Shorter than every rung: the smallest rung is kept, re-targeted at the
		// source, so a constrained lane still has something to convert to.
		{240, "auto original 360p"},
		// Unmeasured source: publish the whole ladder rather than guess.
		{0, "auto original 2160p 1440p 1080p 720p 480p 360p"},
	}
	for _, tt := range cases {
		got := strings.Join(rungIDs(QualityOffers(tt.height, true, true, policy)), " ")
		if got != tt.want {
			t.Fatalf("height %d published %q, want %q", tt.height, got, tt.want)
		}
	}
	// Exactly one automatic rung, and it is first.
	rungs := QualityOffers(1080, true, true, policy)
	automatic := 0
	for i, r := range rungs {
		if r.Kind == QualityAutomatic {
			automatic++
			if i != 0 {
				t.Fatal("automatic rung is not first")
			}
		}
	}
	if automatic != 1 {
		t.Fatal("published", automatic, "automatic rungs")
	}
	if rungs[1].Kind != QualityOriginal {
		t.Fatal("original rung is not second", rungs[1])
	}
	for _, r := range rungs[2:] {
		if r.Kind != QualityFixed || r.TargetDisplayHeight == 0 || r.MaxVideoBitrateBPS == 0 || r.MaxAudioBitrateBPS == 0 {
			t.Fatal("fixed rung is missing its ceilings", r)
		}
	}
	// The 240p source's kept rung is re-targeted rather than upscaling.
	short := QualityOffers(240, true, true, policy)
	if short[2].TargetDisplayHeight != 240 {
		t.Fatal("kept rung upscales the source", short[2])
	}
}

func TestQualityLadderRespectsOwnerAndPolicy(t *testing.T) {
	policy := openPolicy()
	// Owner-disabled transcoding leaves only what the source satisfies directly.
	off := QualityOffers(1080, true, false, policy)
	for _, r := range off {
		switch r.Kind {
		case QualityAutomatic, QualityOriginal:
			if !r.Enabled {
				t.Fatal("direct rung disabled with transcoding off", r)
			}
		default:
			if r.Enabled || r.Reason != RungReasonTranscodeOff {
				t.Fatal("fixed rung selectable with transcoding off", r)
			}
		}
	}

	// A lane ceiling disables the rungs above it but still publishes them, so a
	// client can say why the menu is short instead of inventing an explanation.
	constrained := ResolveDeliveryPolicy(stubPreferences{"quality.cellular.maxVideoHeight": 720, "quality.cellular.maxVideoBitrateMbps": 8}, NetworkCellular, LocalityRemote, "cellular", DefaultDeliveryConfiguration())
	rungs := QualityOffers(2160, true, true, constrained)
	for _, r := range rungs {
		if r.Kind != QualityFixed {
			continue
		}
		wantEnabled := r.TargetDisplayHeight <= 720 && r.MaxVideoBitrateBPS <= 8_000_000
		if r.Enabled != wantEnabled {
			t.Fatal("rung enablement disagrees with the lane ceiling", r)
		}
		if !r.Enabled && r.Reason != RungReasonExceedsPolicy {
			t.Fatal("disabled rung published no reason", r)
		}
	}

	// An unavailable source offers nothing selectable at all.
	for _, r := range QualityOffers(1080, false, true, policy) {
		if r.Enabled || r.Reason != RungReasonSourceUnusable {
			t.Fatal("unavailable source published a selectable rung", r)
		}
	}
}

func TestQualityOffersRevisionFencesSelection(t *testing.T) {
	policy := openPolicy()
	rungs := QualityOffers(1080, true, true, policy)
	revision := OffersRevision("asset", rungs, policy)
	if len(revision) != 64 {
		t.Fatal("revision is not a full digest", revision)
	}
	if OffersRevision("asset", rungs, policy) != revision {
		t.Fatal("revision is not stable for an unchanged ladder")
	}
	// The same ladder under a different lane is a different ladder.
	other := ResolveDeliveryPolicy(stubPreferences{"quality.cellular.maxVideoHeight": 720}, NetworkCellular, LocalityRemote, "cellular", DefaultDeliveryConfiguration())
	if OffersRevision("asset", QualityOffers(1080, true, true, other), other) == revision {
		t.Fatal("revision ignored the resolved policy")
	}
	if OffersRevision("other-asset", rungs, policy) == revision {
		t.Fatal("revision ignored the source")
	}

	if _, ok := SelectedRung(rungs, "nonsense"); ok {
		t.Fatal("unknown rung accepted")
	}
	if _, ok := SelectedRung(QualityOffers(1080, true, false, policy), "720p"); ok {
		t.Fatal("disabled rung accepted")
	}
	rung, ok := SelectedRung(rungs, "")
	if !ok || rung.ID != "auto" {
		t.Fatal("empty selection did not default to automatic", rung)
	}
}

func TestQualityTargetNarrowsToTheStricterCeiling(t *testing.T) {
	lane := ResolveDeliveryPolicy(stubPreferences{"quality.cellular.maxVideoHeight": 720, "quality.cellular.maxVideoBitrateMbps": 3, "quality.cellular.maxAudioBitrateKbps": 128}, NetworkCellular, LocalityRemote, "cellular", DefaultDeliveryConfiguration())
	// A rung asking for more than the lane allows is narrowed to the lane.
	wide := QualityRung{ID: "1080p", Kind: QualityFixed, TargetDisplayHeight: 1080, MaxVideoBitrateBPS: 8_000_000, MaxAudioBitrateBPS: 192_000}.Target(lane, 2160)
	if wide.MaxVideoHeight != 720 || wide.MaxVideoBitrateBPS != 3_000_000 || wide.MaxAudioBitrateBPS != 128_000 {
		t.Fatal("rung was not narrowed by the lane", wide)
	}
	// A rung asking for less than the lane allows keeps its own smaller ceiling.
	narrow := QualityRung{ID: "480p", Kind: QualityFixed, TargetDisplayHeight: 480, MaxVideoBitrateBPS: 2_000_000, MaxAudioBitrateBPS: 96_000}.Target(lane, 2160)
	if narrow.MaxVideoHeight != 480 || narrow.MaxVideoBitrateBPS != 2_000_000 || narrow.MaxAudioBitrateBPS != 96_000 {
		t.Fatal("lane widened a narrower rung", narrow)
	}
	// Original carries no ceilings, and automatic carries the lane's.
	if original := (QualityRung{ID: "original", Kind: QualityOriginal}).Target(lane, 2160); original.MaxVideoBitrateBPS != 0 || original.MaxVideoHeight != 0 {
		t.Fatal("original rung acquired ceilings", original)
	}
	if auto := autoTarget(lane, 2160); auto.MaxVideoHeight != 720 || auto.MaxVideoBitrateBPS != 3_000_000 {
		t.Fatal("automatic rung did not take the lane ceiling", auto)
	}
	// No target ever exceeds the source it is cut from.
	if capped := autoTarget(openPolicy(), 480); capped.MaxVideoHeight != 0 {
		t.Fatal("unclamped lane invented a height", capped)
	}
	if capped := (QualityRung{ID: "1080p", Kind: QualityFixed, TargetDisplayHeight: 1080}).Target(openPolicy(), 480); capped.MaxVideoHeight != 480 {
		t.Fatal("target exceeded the source height", capped)
	}
}

func TestQualitySelectionDomain(t *testing.T) {
	if !validQualitySelection("auto") || !validQualitySelection("original") || !validQualitySelection("1080p") {
		t.Fatal("published rung rejected")
	}
	if validQualitySelection("") || validQualitySelection("4320p") || validQualitySelection("../auto") {
		t.Fatal("unpublished rung accepted")
	}
}
