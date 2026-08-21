package playbackcap

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, time.August, 7, 0, 0, 0, 0, time.UTC)

func TestTupleMatchingDoesNotInventCrossProduct(t *testing.T) {
	h264AAC := baselineTuple()
	hevcEAC3 := hevcTuple()
	hevcEAC3.Audio = Audio{Codec: "eac3", Layout: "5.1", Route: "passthrough", MaxChannels: 6}
	r := Resolution{Tuples: []DeliveryTuple{h264AAC, hevcEAC3}}

	want := h264AAC
	want.Audio = hevcEAC3.Audio
	if r.Supports(want) {
		t.Fatal("combined video and audio from different tuples")
	}
	if !r.Supports(hevcEAC3) {
		t.Fatal("rejected an exact compatible tuple")
	}
}

func TestAudioOnlyFallbackTuplesMatchWithoutDummyVideo(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	got, err := r.Resolve(Client{Family: "roku", Version: "13", Platform: "roku"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]DeliveryTuple{"mp3": audioMP3Tuple(), "aac": audioAACTuple()} {
		t.Run(name, func(t *testing.T) {
			if want.Video != (Video{}) || want.Subtitle.Mode != SubtitleNone {
				t.Fatal("audio-only tuple contains dummy audiovisual fields")
			}
			if !got.Supports(want) {
				t.Fatalf("fallback does not support %s audio tuple", name)
			}
		})
	}
}

func TestMediaKindValidationKeepsAudiovisualStrict(t *testing.T) {
	audio := audioMP3Tuple()
	audio.Video.Codec = "h264"
	if err := audio.Validate(); err == nil {
		t.Fatal("audio-only tuple accepted video fields")
	}
	av := baselineTuple()
	av.Video.MaxWidth = 0
	if err := av.Validate(); err == nil {
		t.Fatal("audiovisual tuple accepted missing dimensions")
	}
	av = baselineTuple()
	av.Kind = MediaAudio
	if err := av.Validate(); err == nil {
		t.Fatal("audiovisual tuple was relabeled as audio-only")
	}
}

func TestTupleMatchingFailsClosedForUnassertedProperties(t *testing.T) {
	have := baselineTuple()
	want := have
	want.Video.HDR = "hdr10"
	if (Resolution{Tuples: []DeliveryTuple{have}}).Supports(want) {
		t.Fatal("unspecified HDR was treated as supported")
	}
	want = have
	want.Video.DolbyVisionProfile = 5
	if (Resolution{Tuples: []DeliveryTuple{have}}).Supports(want) {
		t.Fatal("unasserted Dolby Vision profile was treated as supported")
	}
}

func TestMalformedRuntimeEvidenceFailsClosedToFallback(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	bad := Evidence{
		ID: "bad", Client: Client{Family: "roku", Platform: "roku"},
		Provenance: Provenance{Source: SourceNativeRuntime, Confidence: ConfidenceHigh, Producer: "test-native", Authenticated: false, ReviewedAt: testNow},
		Tuples:     []DeliveryTuple{baselineTuple()},
	}
	got, err := r.Resolve(Client{Family: " Roku ", Version: "13", Platform: "ROKU"}, []Evidence{bad})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceStaticFallback || got.Band != "roku-os-11-14" {
		t.Fatalf("unexpected fallback: %+v", got)
	}
}

func TestAuthenticatedNativeRuntimePrecedence(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	makeEvidence := func(id string, source EvidenceSource, tuple DeliveryTuple) Evidence {
		return Evidence{ID: id, Client: Client{Family: "avkit", Platform: "tvos"}, Provenance: Provenance{
			Source: source, Confidence: ConfidenceHigh, Producer: "test-client", ProducerVersion: "1", SchemaVersion: SchemaVersion, Authenticated: true, ReviewedAt: testNow,
		}, Tuples: []DeliveryTuple{tuple}}
	}
	auth := makeEvidence("authenticated", SourceAuthenticatedRuntime, hevcTuple())
	nativeTuple := av1Tuple()
	native := makeEvidence("native", SourceNativeRuntime, nativeTuple)
	got, err := r.Resolve(Client{Family: "avkit", Version: "18", Platform: "tvos"}, []Evidence{auth, native})
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceID != "native" || got.Source != SourceNativeRuntime {
		t.Fatalf("native evidence did not win: %+v", got)
	}
	if !got.Supports(nativeTuple) || got.Supports(hevcTuple()) {
		t.Fatal("resolution unioned lower-priority tuples")
	}
}

func TestRuntimeEvidenceHonorsClientAndVersionBounds(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	e := Evidence{ID: "future-only", Client: Client{Family: "safari", Platform: "web"}, Provenance: Provenance{
		Source: SourceAuthenticatedRuntime, Confidence: ConfidenceHigh, Producer: "test-client", ProducerVersion: "1", SchemaVersion: SchemaVersion, Authenticated: true, MinVersion: "20", MaxVersion: "21", ReviewedAt: testNow,
	}, Tuples: []DeliveryTuple{av1Tuple()}}
	got, err := r.Resolve(Client{Family: "safari", Version: "18", Platform: "web"}, []Evidence{e})
	if err != nil {
		t.Fatal(err)
	}
	if got.Band != "safari-17-19" {
		t.Fatalf("out-of-range runtime evidence won: %+v", got)
	}
}

func TestRuntimeEvidenceExpiresBySource(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	stale := Evidence{ID: "stale-native", Client: Client{Family: "roku", Platform: "roku"}, Provenance: Provenance{
		Source: SourceNativeRuntime, Confidence: ConfidenceHigh, Producer: "portico-roku", ProducerVersion: "13", SchemaVersion: SchemaVersion,
		Authenticated: true, ReviewedAt: testNow.Add(-24*time.Hour - time.Second),
	}, Tuples: []DeliveryTuple{av1Tuple()}}
	got, err := r.Resolve(Client{Family: "roku", Version: "13", Platform: "roku"}, []Evidence{stale})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceStaticFallback || got.EvidenceID == stale.ID {
		t.Fatalf("stale runtime evidence remained authoritative: %#v", got)
	}
}

func TestUnauthenticatedProbeCannotAuthorizeDelivery(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	probe := Evidence{ID: "browser-claim", Client: Client{Family: "firefox", Platform: "web"}, Provenance: Provenance{
		Source: SourceUnauthenticatedProbe, Confidence: ConfidenceHigh, Producer: "request-header", ReviewedAt: testNow,
	}, Tuples: []DeliveryTuple{av1Tuple()}}
	got, err := r.Resolve(Client{Family: "firefox", Version: "130", Platform: "web"}, []Evidence{probe})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceStaticFallback || got.Supports(av1Tuple()) {
		t.Fatalf("unauthenticated probe authorized delivery: %+v", got)
	}
}

func TestBoundedPorticoWebBandSupportsManagedHLSBaseline(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	got, err := r.Resolve(Client{Family: "chromium", Version: "147.0.7727.15", Platform: "web"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceStaticFallback || got.Band != "chromium-120-159" || !got.Supports(hlsTuple()) {
		t.Fatalf("bounded Portico Web band lacks its reviewed managed-HLS baseline: %+v", got)
	}
	if got.Supports(av1Tuple()) {
		t.Fatalf("bounded Portico Web band invented an unreviewed AV1 capability: %+v", got)
	}
}

func TestFallbackFamilyVersionGolden(t *testing.T) {
	type golden struct {
		Family, Version, Platform, Band string
		Tuples                          int
	}
	raw, err := os.ReadFile("testdata/family_version_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []golden
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	for _, tc := range cases {
		t.Run(tc.Family+"/"+tc.Platform+"/"+tc.Version, func(t *testing.T) {
			got, err := r.Resolve(Client{Family: tc.Family, Version: tc.Version, Platform: tc.Platform}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.Band != tc.Band || len(got.Tuples) != tc.Tuples {
				t.Fatalf("got band=%s tuples=%d", got.Band, len(got.Tuples))
			}
		})
	}
}

func TestPrimaryEvidenceCatalogOwnsEveryStaticFamilyAndRuntimeGap(t *testing.T) {
	type reference struct {
		ID, VersionBounds, Claim, Limitations string
		Families                              []string
		RuntimeGaps                           []string
		URLs                                  []string
	}
	type catalog struct {
		SchemaVersion int
		Owner         string
		ReviewedAt    string
		Policy        string
		References    []reference
	}
	raw, err := os.ReadFile("testdata/evidence_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var got catalog
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.Owner == "" || got.Policy == "" {
		t.Fatal("evidence catalog lacks schema ownership or policy")
	}
	if _, err := time.Parse("2006-01-02", got.ReviewedAt); err != nil {
		t.Fatalf("invalid catalog review date: %v", err)
	}
	covered := map[string]bool{}
	ids := map[string]bool{}
	for _, ref := range got.References {
		if ref.ID == "" || ids[ref.ID] || ref.VersionBounds == "" || ref.Claim == "" || ref.Limitations == "" || len(ref.RuntimeGaps) == 0 || len(ref.URLs) == 0 {
			t.Fatalf("incomplete or duplicate evidence reference: %+v", ref)
		}
		ids[ref.ID] = true
		for _, rawURL := range ref.URLs {
			if !strings.HasPrefix(rawURL, "https://") {
				t.Fatalf("non-HTTPS primary source in %s: %s", ref.ID, rawURL)
			}
		}
		for _, family := range ref.Families {
			covered[family] = true
		}
	}
	for _, band := range DefaultFallbackBands() {
		family := normalize(band.Evidence.Client.Family)
		if !covered[family] {
			t.Fatalf("static family %q has no owned evidence reference", family)
		}
	}
}

func TestFutureBrowserVersionGetsProgressiveBaselineInsteadOfInventedHLS(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	got, err := r.Resolve(Client{Family: "chromium", Version: "999", Platform: "web"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Band != "chromium-unknown-conservative" || len(got.Tuples) != 6 {
		t.Fatalf("future version inherited rich band: %+v", got)
	}
	if got.Supports(hlsTuple()) {
		t.Fatal("browser version alone authorized native HLS")
	}
	if got.Supports(av1Tuple()) {
		t.Fatal("future version inherited unreviewed AV1 tuple")
	}
}

func TestChromiumStaticBandDoesNotClaimAV1(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	for _, family := range []string{"chromium", "edge"} {
		got, err := r.Resolve(Client{Family: family, Version: "125", Platform: "web"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Supports(av1Tuple()) {
			t.Fatalf("%s static fallback claimed AV1", family)
		}
	}
}

func TestEqualRuntimeEvidenceTieBreakIsStable(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	makeEvidence := func(id string, tuple DeliveryTuple) Evidence {
		return Evidence{ID: id, Client: Client{Family: "roku", Platform: "roku"}, Provenance: Provenance{
			Source: SourceNativeRuntime, Confidence: ConfidenceHigh, Producer: "native-client", ProducerVersion: "13", SchemaVersion: SchemaVersion, Authenticated: true, ReviewedAt: testNow,
		}, Tuples: []DeliveryTuple{tuple}}
	}
	a := makeEvidence("a-stable", audioAACTuple())
	z := makeEvidence("z-stable", audioMP3Tuple())
	for _, evidence := range [][]Evidence{{z, a}, {a, z}} {
		got, err := r.Resolve(Client{Family: "roku", Version: "13", Platform: "roku"}, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if got.EvidenceID != "a-stable" || !got.Supports(audioAACTuple()) || got.Supports(audioMP3Tuple()) {
			t.Fatalf("input order changed equal-rank resolution: %+v", got)
		}
	}
}

func TestEvidenceValidationRejectsMalformedTuplesAndBounds(t *testing.T) {
	e := Evidence{ID: "x", Client: Client{Family: "roku"}, Provenance: Provenance{
		Source: SourceStaticFallback, Confidence: ConfidenceLow, Producer: "test", MinVersion: "14", MaxVersion: "11", ReviewedAt: testNow,
	}, Tuples: []DeliveryTuple{baselineTuple()}}
	if err := e.Validate(testNow); err == nil {
		t.Fatal("accepted reversed version bounds")
	}
	e.Provenance.MinVersion, e.Provenance.MaxVersion = "", ""
	e.Tuples[0].Subtitle = Subtitle{Mode: SubtitleNative}
	if err := e.Validate(testNow); err == nil {
		t.Fatal("accepted malformed subtitle declaration")
	}
}

func TestDefaultFallbackCatalogValidates(t *testing.T) {
	seen := map[string]bool{}
	for _, band := range DefaultFallbackBands() {
		if seen[band.Name] {
			t.Fatalf("duplicate fallback band %q", band.Name)
		}
		seen[band.Name] = true
		if err := band.Evidence.Validate(testNow); err != nil {
			t.Fatalf("fallback %q is invalid: %v", band.Name, err)
		}
	}
	if err := ValidateFallbackBands(DefaultFallbackBands(), testNow); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackValidationRejectsOverlapAndRichCatchAll(t *testing.T) {
	base := staticBand("a", "roku", "roku", "11", "14", baselineTuple())
	overlap := staticBand("b", "roku", "roku", "13", "15", baselineTuple())
	if err := ValidateFallbackBands([]FallbackBand{base, overlap}, testNow); err == nil {
		t.Fatal("accepted overlapping bounded bands")
	}
	richCatchAll := staticBand("unknown", "roku", "roku", "", "", baselineTuple(), hevcTuple())
	if err := ValidateFallbackBands([]FallbackBand{base, richCatchAll}, testNow); err == nil {
		t.Fatal("accepted rich catch-all inheritance")
	}
	r := NewResolver([]FallbackBand{base, overlap})
	r.now = func() time.Time { return testNow }
	if _, err := r.Resolve(Client{Family: "roku", Version: "13", Platform: "roku"}, nil); err == nil {
		t.Fatal("resolver accepted invalid catalog")
	}
}

func TestUnknownFallbackAllowsOnlySafeHLSBaseline(t *testing.T) {
	base := staticBand("bounded", "roku", "roku", "11", "14", baselineTuple())
	safe := staticBand("unknown", "roku", "roku", "", "", unknownHLSTuples()...)
	if err := ValidateFallbackBands([]FallbackBand{base, safe}, testNow); err != nil {
		t.Fatalf("rejected conservative HLS catch-all: %v", err)
	}

	unsafeVideo := hlsTuple()
	unsafeVideo.Video = hevcTuple().Video
	unsafeAudio := hlsTuple()
	unsafeAudio.Audio.Layout = "5.1"
	unsafeAudio.Audio.MaxChannels = 6
	unsafeAudio.Audio.ObjectPassthrough = true
	for name, tuple := range map[string]DeliveryTuple{
		"rich video":         unsafeVideo,
		"multichannel audio": unsafeAudio,
	} {
		t.Run(name, func(t *testing.T) {
			candidate := staticBand("unknown", "roku", "roku", "", "", baselineTuple(), tuple, audioMP3Tuple(), audioAACTuple())
			if err := ValidateFallbackBands([]FallbackBand{base, candidate}, testNow); err == nil {
				t.Fatalf("accepted unsafe catch-all tuple: %#v", tuple)
			}
		})
	}
}

func TestDLNACatchAllRemainsProgressiveOnly(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	got, err := r.Resolve(Client{Family: "dlna", Version: "unknown", Platform: "dlna"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Supports(hlsTuple()) || !got.Supports(baselineTuple()) {
		t.Fatalf("DLNA fallback must remain progressive-only: %+v", got)
	}
	withHLS := staticBand("dlna-unsafe", "dlna", "dlna", "", "", unknownHLSTuples()...)
	if err := ValidateFallbackBands([]FallbackBand{withHLS}, testNow); err == nil {
		t.Fatal("accepted HLS in DLNA catch-all")
	}
}

func TestStaticCatalogHasExactSubtitleAndManagedDeliveryModes(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	got, err := r.Resolve(Client{Family: "chromium", Version: "140", Platform: "web"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wants := []DeliveryTuple{baselineTuple(), textSubtitleTuple(), convertedTextTuple(), bitmapBurnTuple(), audioMP3Tuple(), audioAACTuple()}
	for _, want := range wants {
		if !got.Supports(want) {
			t.Fatalf("missing exact fallback tuple: %#v", want)
		}
	}
	if got.Supports(hevcTuple()) || got.Supports(av1Tuple()) {
		t.Fatal("static catalog made an unauthenticated rich codec claim")
	}
	if !got.Supports(hlsTuple()) {
		t.Fatal("bounded Portico Web band lost its managed HLS baseline")
	}
}

func TestNativeHLSFamiliesRetainConservativeHLSFallback(t *testing.T) {
	r := DefaultResolver()
	r.now = func() time.Time { return testNow }
	for _, client := range []Client{
		{Family: "safari", Version: "19", Platform: "web"},
		{Family: "avkit", Version: "19", Platform: "tvos"},
		{Family: "media3", Version: "16", Platform: "android"},
		{Family: "roku", Version: "14", Platform: "roku"},
		{Family: "tizen", Version: "2026", Platform: "tizen"},
		{Family: "webos", Version: "26", Platform: "webos"},
	} {
		got, err := r.Resolve(client, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Supports(hlsTuple()) {
			t.Fatalf("%s/%s lost documented HLS baseline", client.Family, client.Platform)
		}
	}
}
