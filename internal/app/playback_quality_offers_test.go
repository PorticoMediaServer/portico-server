package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackRenegotiationHasExactlyOneQualityAuthority(t *testing.T) {
	selection := PlaybackQualitySelection{Mode: playbackQualityModeExplicit, SelectionID: "qsel_current", OfferRevision: "qrev_current"}
	if playbackRenegotiationHasNestedQuality(PlaybackRenegotiationRequest{Quality: &selection}) {
		t.Fatal("top-level renegotiation quality was incorrectly treated as a conflict")
	}
	if !playbackRenegotiationHasNestedQuality(PlaybackRenegotiationRequest{Quality: &selection, Intent: PlaybackIntent{Quality: selection}}) {
		t.Fatal("top-level and nested quality authorities were both accepted")
	}
}

func TestExplicitQualityFailureCarriesCompleteRefreshedOffers(t *testing.T) {
	authority := mustPlaybackQualityAuthority(t, "source-current", "version-current", "movie")
	recorder := httptest.NewRecorder()
	writePlaybackStartError(recorder, playbackQualityStartError(&ExplicitQualityUnavailableError{Offers: authority.set}))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var problem struct {
		Code    string `json:"code"`
		Details struct {
			Offers PlaybackQualityOfferSet `json:"qualityOffers"`
		} `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "explicit_quality_unavailable" || !reflect.DeepEqual(problem.Details.Offers, authority.set) {
		t.Fatalf("refreshed offer details are incomplete: %#v", problem)
	}
}

func TestPlaybackQualityIssuerOwnsOrderedOpaqueResolvedOffers(t *testing.T) {
	item := MediaItem{
		ID: "movie-quality-offers", Type: "movie",
		Streams: []Stream{
			{ID: "video", Kind: "video", Height: 2160, Bitrate: 32_000_000},
			{ID: "audio", Kind: "audio", Bitrate: 640_000},
		},
	}
	authority, err := issuePlaybackQualityOffers(playbackQualityOfferIssue{
		Item: item, VersionID: "version-source", SourceRevision: "source-revision-a",
		Policy: ResolvedPlaybackPolicy{TranscodePolicy: "allow"}, AllowTranscode: true,
	})
	if err != nil {
		t.Fatalf("issue offers: %v", err)
	}
	if authority.set.ContractID != playbackQualityOfferContractID || authority.set.SchemaVersion != playbackQualityOfferSchemaVersion || authority.set.OfferRevision == "" {
		t.Fatalf("offer-set identity is incomplete: %#v", authority.set)
	}
	if len(authority.set.Offers) < 4 || authority.set.Offers[0].Kind != playbackQualityKindAutomatic || authority.set.Offers[1].Kind != playbackQualityKindOriginal {
		t.Fatalf("ordered automatic/original/fixed offers missing: %#v", authority.set.Offers)
	}
	seen := map[string]bool{}
	for _, offer := range authority.set.Offers {
		if offer.SelectionID == "" || seen[offer.SelectionID] {
			t.Fatalf("selection identity is missing or duplicated: %#v", authority.set.Offers)
		}
		seen[offer.SelectionID] = true
		if strings.Contains(offer.SelectionID, "1080") || strings.Contains(offer.SelectionID, "video-") || strings.Contains(offer.SelectionID, "audio-") {
			t.Fatalf("selection identity leaked an implementation preset: %q", offer.SelectionID)
		}
		if offer.Kind == playbackQualityKindFixed && offer.MaxVideoBitrateBps == 0 && offer.MaxAudioBitrateBps == 0 && offer.TargetDisplayHeight == 0 {
			t.Fatalf("fixed offer has no concrete target: %#v", offer)
		}
	}
}

func TestPlaybackQualitySelectionRequiresExactCurrentRevisionAndReturnsRefresh(t *testing.T) {
	first := mustPlaybackQualityAuthority(t, "source-a", "version-a", "movie")
	second := mustPlaybackQualityAuthority(t, "source-b", "version-a", "movie")
	fixed := firstOfferOfKind(t, first.set.Offers, playbackQualityKindFixed)

	if fixed.SelectionID != firstOfferOfKind(t, second.set.Offers, playbackQualityKindFixed).SelectionID {
		t.Fatal("stable semantic offer changed its selectionId with the source revision")
	}
	if first.set.OfferRevision == second.set.OfferRevision {
		t.Fatal("source revision change did not advance offerRevision")
	}

	cases := []PlaybackQualitySelection{
		{Mode: playbackQualityModeExplicit, SelectionID: fixed.SelectionID},
		{Mode: playbackQualityModeExplicit, OfferRevision: second.set.OfferRevision},
		{Mode: playbackQualityModeExplicit, SelectionID: fixed.SelectionID, OfferRevision: first.set.OfferRevision},
		{Mode: playbackQualityModeExplicit, SelectionID: "qsel_unknown", OfferRevision: second.set.OfferRevision},
	}
	for _, selection := range cases {
		_, err := resolvePlaybackQualitySelection(second, selection)
		if !errors.Is(err, ErrExplicitQualityUnavailable) {
			t.Fatalf("selection %#v error = %v, want explicit_quality_unavailable", selection, err)
		}
		var unavailable *ExplicitQualityUnavailableError
		if !errors.As(err, &unavailable) || !reflect.DeepEqual(unavailable.Offers, second.set) {
			t.Fatalf("selection %#v did not carry the one refreshed set: %#v", selection, err)
		}
	}
}

func TestPlaybackQualityAutomaticIsSeparateFromExplicitOfferSelection(t *testing.T) {
	authority := mustPlaybackQualityAuthority(t, "source", "version", "movie")
	automatic := firstOfferOfKind(t, authority.set.Offers, playbackQualityKindAutomatic)

	resolved, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{Mode: playbackQualityModeAutomatic})
	if err != nil || resolved.Kind != playbackQualityKindAutomatic {
		t.Fatalf("automatic selection = %#v, %v", resolved, err)
	}
	if _, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{
		Mode: playbackQualityModeExplicit, SelectionID: automatic.SelectionID, OfferRevision: authority.set.OfferRevision,
	}); !errors.Is(err, ErrExplicitQualityUnavailable) {
		t.Fatalf("explicit selection admitted the automatic offer: %v", err)
	}
	if _, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{
		Mode: playbackQualityModeAutomatic, SelectionID: automatic.SelectionID,
	}); !errors.Is(err, errPlaybackQualitySelectionInvalid) {
		t.Fatalf("automatic accepted explicit-selection fields: %v", err)
	}
}

func TestPlaybackQualityExplicitTargetsApplyLiterallyWithoutFallback(t *testing.T) {
	authority := mustPlaybackQualityAuthority(t, "source", "version", "movie")
	original := firstOfferOfKind(t, authority.set.Offers, playbackQualityKindOriginal)
	fixed := firstOfferOfKind(t, authority.set.Offers, playbackQualityKindFixed)

	originalTarget, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{
		Mode: playbackQualityModeExplicit, SelectionID: original.SelectionID, OfferRevision: authority.set.OfferRevision,
	})
	if err != nil {
		t.Fatalf("resolve original: %v", err)
	}
	originalPolicy := applyResolvedPlaybackQuality(ResolvedPlaybackPolicy{QualityProfile: "standard", DeliveryProfile: "video-standard"}, originalTarget, "movie")
	if originalPolicy.QualityProfile != "original" || originalPolicy.DeliveryProfile != "video-original" {
		t.Fatalf("original was rewritten to a named fallback: %#v", originalPolicy)
	}
	if got := transcodeQualityForResolvedPolicy("movie", originalPolicy); got != "original" {
		t.Fatalf("original execution identity became %q", got)
	}

	fixedTarget, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{
		Mode: playbackQualityModeExplicit, SelectionID: fixed.SelectionID, OfferRevision: authority.set.OfferRevision,
	})
	if err != nil {
		t.Fatalf("resolve fixed: %v", err)
	}
	fixedPolicy := applyResolvedPlaybackQuality(ResolvedPlaybackPolicy{}, fixedTarget, "movie")
	if fixedPolicy.DeliveryProfile != fixedTarget.PresetID || resolvedMaxVideoBitrateBps(fixedPolicy) != fixed.MaxVideoBitrateBps || fixedPolicy.MaxVideoHeight != fixed.TargetDisplayHeight {
		t.Fatalf("fixed target was not applied literally: target=%#v policy=%#v", fixedTarget, fixedPolicy)
	}
}

func TestPlaybackQualityFixedOfferPreservesFractionalMbpsExactly(t *testing.T) {
	authority := mustPlaybackQualityAuthority(t, "source", "version", "movie")
	var selected PlaybackQualityOffer
	for _, offer := range authority.set.Offers {
		if offer.Kind == playbackQualityKindFixed && offer.MaxVideoBitrateBps == 1_500_000 {
			selected = offer
			break
		}
	}
	if selected.SelectionID == "" {
		t.Fatalf("1.5 Mbps offer missing from %#v", authority.set.Offers)
	}
	target, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{
		Mode: playbackQualityModeExplicit, SelectionID: selected.SelectionID, OfferRevision: authority.set.OfferRevision,
	})
	if err != nil {
		t.Fatalf("resolve 1.5 Mbps selection: %v", err)
	}
	policy := applyResolvedPlaybackQuality(ResolvedPlaybackPolicy{}, target, "movie")
	if got := resolvedMaxVideoBitrateBps(policy); got != 1_500_000 {
		t.Fatalf("exact offer target became %d Bps", got)
	}
	if got := resolvedDeliveryProfile("movie", policy); got != target.PresetID {
		t.Fatalf("exact named profile became %q, want %q", got, target.PresetID)
	}
	profile := applyResolvedPlaybackPolicy(PlaybackClientProfile{}, "movie", policy)
	if profile.MaxBitrate != 1_500_000 {
		t.Fatalf("client execution ceiling became %d Bps", profile.MaxBitrate)
	}
	decision := PlaybackDecision{executionPlan: &playbackExecutionPlan{
		Quality: target.PresetID,
		Plan: playbackplan.Plan{Constraints: playbackplan.Constraints{
			MaxVideoBitrate: target.MaxVideoBitrateBps,
			MaxAudioBitrate: target.MaxAudioBitrateBps,
			MaxHeight:       target.TargetDisplayHeight,
		}},
	}}
	if err := validateResolvedPlaybackQualityExecution(authority, target, decision); err != nil {
		t.Fatalf("literal execution plan was rejected: %v", err)
	}
	decision.executionPlan.Plan.Constraints.MaxVideoBitrate = 1_000_000
	if err := validateResolvedPlaybackQualityExecution(authority, target, decision); !errors.Is(err, ErrExplicitQualityUnavailable) {
		t.Fatalf("silently lowered execution plan was admitted: %v", err)
	}
}

func TestPlaybackQualityOriginalAllowsRepackAndAudioConversionButNotVideoFallback(t *testing.T) {
	authority := mustPlaybackQualityAuthority(t, "source", "version", "movie")
	offer := firstOfferOfKind(t, authority.set.Offers, playbackQualityKindOriginal)
	target, err := resolvePlaybackQualitySelection(authority, PlaybackQualitySelection{
		Mode: playbackQualityModeExplicit, SelectionID: offer.SelectionID, OfferRevision: authority.set.OfferRevision,
	})
	if err != nil {
		t.Fatalf("resolve original: %v", err)
	}
	originalPlan := &playbackExecutionPlan{Quality: "original"}
	if err := validateResolvedPlaybackQualityExecution(authority, target, PlaybackDecision{RequiresRemux: true, AudioTranscode: true, executionPlan: originalPlan}); err != nil {
		t.Fatalf("original-picture repack/audio conversion was rejected: %v", err)
	}
	if err := validateResolvedPlaybackQualityExecution(authority, target, PlaybackDecision{VideoTranscode: true, executionPlan: originalPlan}); !errors.Is(err, ErrExplicitQualityUnavailable) {
		t.Fatalf("original silently admitted video conversion: %v", err)
	}
}

func TestPlaybackQualityAudioOnlyFixedTargetIsValidAndEmptyFixedIsRejected(t *testing.T) {
	authority := mustPlaybackQualityAuthority(t, "audio-source", "audio-version", "track")
	fixed := firstOfferOfKind(t, authority.set.Offers, playbackQualityKindFixed)
	if fixed.MaxAudioBitrateBps <= 0 || fixed.MaxVideoBitrateBps != 0 || fixed.TargetDisplayHeight != 0 {
		t.Fatalf("audio-only fixed target is not exact: %#v", fixed)
	}

	probe := playbackQualityOfferAuthority{set: PlaybackQualityOfferSet{}, targets: map[string]playbackResolvedQuality{}}
	probe.add("Empty", playbackResolvedQuality{Kind: playbackQualityKindFixed, PresetID: "empty"})
	if len(probe.set.Offers) != 0 || len(probe.targets) != 0 {
		t.Fatalf("empty fixed target entered the authority: %#v", probe)
	}
}

func TestPlaybackQualityIssuerAppliesHardOfferCeilingsWithoutClientAuthoredGeometry(t *testing.T) {
	item := MediaItem{
		ID: "movie-hard-ceiling", Type: "movie",
		Streams: []Stream{{Kind: "video", Height: 2160, Bitrate: 40_000_000}, {Kind: "audio", Bitrate: 640_000}},
	}
	authority, err := issuePlaybackQualityOffers(playbackQualityOfferIssue{
		Item: item, VersionID: "version", SourceRevision: "source",
		Policy:         ResolvedPlaybackPolicy{TranscodePolicy: "allow", MaxVideoBitrateMbps: 8, MaxVideoHeight: 720, MaxAudioBitrateKbps: 192},
		AllowTranscode: true,
	})
	if err != nil {
		t.Fatalf("issue clamped offers: %v", err)
	}
	for _, offer := range authority.set.Offers {
		if offer.Kind == playbackQualityKindOriginal {
			t.Fatalf("original escaped a hard source ceiling: %#v", authority.set.Offers)
		}
		if offer.MaxVideoBitrateBps > 8_000_000 || offer.TargetDisplayHeight > 720 || offer.MaxAudioBitrateBps > 192_000 {
			t.Fatalf("offer escaped a hard ceiling: %#v", offer)
		}
	}
}

func TestContinuousPlaybackQualityNeverTranscodesDoesNotPublishFixedOffers(t *testing.T) {
	authority, err := issueContinuousPlaybackQualityOffers(
		"channel-no-transcode",
		"version-no-transcode",
		"source-no-transcode",
		ResolvedPlaybackPolicy{TranscodePolicy: "never"},
		[]string{"1080p-high", "720p-high"},
		true,
	)
	if err != nil {
		t.Fatalf("issue continuous offers: %v", err)
	}
	for _, offer := range authority.set.Offers {
		if offer.Kind == playbackQualityKindFixed {
			t.Fatalf("fixed transcode offer escaped a never-transcode policy: %#v", authority.set.Offers)
		}
	}
}

func TestPlaybackQualityOriginalUsesCanonicalBitsPerSecondAtLowBitrates(t *testing.T) {
	item := MediaItem{
		ID: "movie-low-bitrate", Type: "movie",
		Streams: []Stream{{Kind: "video", Height: 144, Bitrate: 9_000}, {Kind: "audio", Bitrate: 8_000}},
	}
	if !originalQualityWithinPolicy(item, ResolvedPlaybackPolicy{MaxVideoBitrateMbps: 1, MaxAudioBitrateKbps: 16}) {
		t.Fatal("canonical low-bitrate source was incorrectly reinterpreted as kilobits per second")
	}
}

func TestPlaybackQualityVideoOriginalIgnoresConvertibleAlternateAudioCeiling(t *testing.T) {
	item := MediaItem{
		ID: "movie-original-audio-conversion", Type: "movie",
		Streams: []Stream{
			{Kind: "video", Height: 1080, Bitrate: 8_000_000},
			{ID: "aac", Kind: "audio", Bitrate: 192_000},
			{ID: "truehd", Kind: "audio", Bitrate: 4_000_000},
		},
	}
	if !originalQualityWithinPolicy(item, ResolvedPlaybackPolicy{
		MaxVideoBitrateMbps: 20, MaxVideoHeight: 1080, MaxAudioBitrateKbps: 192,
	}) {
		t.Fatal("video Original was hidden because a convertible alternate audio stream exceeded its audio ceiling")
	}
}

func mustPlaybackQualityAuthority(t *testing.T, sourceRevision, versionID, mediaType string) playbackQualityOfferAuthority {
	t.Helper()
	streams := []Stream{{Kind: "video", Height: 1080, Bitrate: 8_000_000}, {Kind: "audio", Bitrate: 192_000}}
	if isAudioMediaType(mediaType) {
		streams = []Stream{{Kind: "audio", Bitrate: 192_000}}
	}
	authority, err := issuePlaybackQualityOffers(playbackQualityOfferIssue{
		Item:      MediaItem{ID: "media-" + mediaType, Type: mediaType, Streams: streams},
		VersionID: versionID, SourceRevision: sourceRevision,
		Policy: ResolvedPlaybackPolicy{TranscodePolicy: "allow"}, AllowTranscode: true,
	})
	if err != nil {
		t.Fatalf("issue %s offers: %v", mediaType, err)
	}
	return authority
}

func firstOfferOfKind(t *testing.T, offers []PlaybackQualityOffer, kind string) PlaybackQualityOffer {
	t.Helper()
	for _, offer := range offers {
		if offer.Kind == kind {
			return offer
		}
	}
	t.Fatalf("offer kind %q missing from %#v", kind, offers)
	return PlaybackQualityOffer{}
}
