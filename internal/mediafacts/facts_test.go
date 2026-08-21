package mediafacts

import (
	"encoding/json"
	"strings"
	"testing"
)

func rationalPtr(num, den int64) *Rational { return &Rational{Num: num, Den: den} }
func boolPtr(value bool) *bool             { return &value }

func completeFacts() Facts {
	vfr := true
	return Facts{
		Version: SchemaVersion, Source: Source{Fingerprint: "sha256:abc", Revision: "etag:1", SizeBytes: 42, StartTime: rationalPtr(-1, 2), TimeBase: rationalPtr(1, 90000)}, Container: "matroska", DurationUS: 2_000_000, DurationConfidence: ConfidenceExact, VariableFrameRate: &vfr, VariableFrameRateConfidence: ConfidenceExact,
		Video:       []Video{{Index: 2, Codec: "hevc", Profile: "Main 10", Level: "5.1", CodecTag: "hvc1", CodedWidth: 3840, CodedHeight: 2160, SampleAspectRatio: Rational{1, 1}, DisplayAspectRatio: Rational{16, 9}, Rotation: 90, DisplayMatrix: []int64{0, -1, 0, 1, 0, 0, 0, 0, 1}, PixelFormat: "yuv420p10le", BitDepth: 10, ChromaSubsampling: "4:2:0", ColorRange: "tv", ColorPrimaries: "bt2020", ColorTransfer: "smpte2084", ColorMatrix: "bt2020nc", MasteringDisplay: &MasteringDisplay{Red: Chromaticity{Rational{17, 25}, Rational{8, 25}}, Green: Chromaticity{Rational{17, 100}, Rational{797, 1000}}, Blue: Chromaticity{Rational{13, 100}, Rational{23, 500}}, WhitePoint: Chromaticity{Rational{3127, 10000}, Rational{329, 1000}}, MinLuminance: Rational{1, 200}, MaxLuminance: Rational{1000, 1}}, MaxCLL: 1000, MaxFALL: 400, HDR10PlusKnown: true, DolbyVision: &DolbyVision{Profile: 8, Level: 6, RPU: true, RPUKnown: true, EnhancementLayerKnown: true, BaseLayerPresent: true, BaseLayerPresentKnown: true, BaseLayerCodec: "hevc", Evidence: "probe side data"}, FieldOrder: "progressive", FrameRate: Rational{60000, 1001}, AverageFrameRate: rationalPtr(120000, 2002), NominalFrameRate: rationalPtr(60, 1), VariableFrameRate: boolPtr(true), VariableFrameRateConfidence: ConfidenceExact, ExactSeekSafe: boolPtr(false), KeyframeEvidenceAt: "2026-08-06T10:00:00Z", KeyframeEvidenceRevision: "etag:1", Timing: Timing{StartTime: rationalPtr(-1, 2), Duration: rationalPtr(2, 1), TimeBase: rationalPtr(1, 90000), DurationConfidence: ConfidenceExact}, Disposition: Disposition{Default: true}}},
		Audio:       []Audio{{Index: 3, Codec: "truehd", Profile: "Atmos", Service: "main", Layout: "7.1", Channels: 8, SampleRate: 48000, SampleFormat: "s32", BitDepth: 24, ObjectAudio: "dolby_atmos", ObjectAudioEvidence: "probe side data", EncoderDelaySamples: 12, EncoderPaddingSamples: 24, Language: "EN_us", Disposition: Disposition{Original: true}}},
		Subtitles:   []Subtitle{{Index: 4, Codec: "hdmv_pgs_subtitle", Kind: "bitmap", ClosedCaptionKnown: true, SDH: boolPtr(true), Signs: boolPtr(false), Language: "EN", Disposition: Disposition{Forced: true}}},
		Attachments: []Attachment{{Index: 5, Codec: "ttf", MIMEType: "Font/TTF", Filename: " font.ttf ", Title: " Font "}},
		Chapters:    []Chapter{{Index: 0, StartUS: 0, EndUS: 1_000_000, Title: " One "}, {Index: 1, StartUS: 1_000_000, EndUS: 2_000_000, Title: "Two"}},
	}
}

func TestCanonicalStableJSONDigestAndDeepCopy(t *testing.T) {
	a := completeFacts()
	a.Container = " Matroska "
	a.Video[0].FrameRate = Rational{120000, 2002}
	a.Video[0].Rotation = 450
	b := a.Clone()
	b.Video[0].DisplayMatrix[0] = 99
	b.Source.StartTime.Num = 99
	*b.Subtitles[0].SDH = false
	if a.Video[0].DisplayMatrix[0] == 99 || a.Source.StartTime.Num == 99 || !*a.Subtitles[0].SDH {
		t.Fatal("Clone aliased nested slice")
	}
	c, err := a.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if c.Container != "matroska" || c.Video[0].FrameRate != (Rational{60000, 1001}) || c.Video[0].Rotation != 90 || c.Audio[0].Language != "en-us" || c.Attachments[0].MIMEType != "font/ttf" {
		t.Fatalf("not canonical: %#v", c)
	}
	d1, err := a.Digest()
	if err != nil {
		t.Fatal(err)
	}
	d2, _ := c.Digest()
	if d1 != d2 || !strings.HasPrefix(d1, "mediafacts-v2:sha256:") {
		t.Fatalf("unstable digest %q %q", d1, d2)
	}
	j1, _ := a.StableJSON()
	var decoded Facts
	if err := json.Unmarshal(j1, &decoded); err != nil {
		t.Fatal(err)
	}
	j2, _ := decoded.StableJSON()
	if string(j1) != string(j2) {
		t.Fatal("stable JSON changed after round trip")
	}
	if decoded.Source.TimeBase == nil || decoded.Video[0].Timing.Duration == nil || decoded.Video[0].AverageFrameRate == nil || decoded.Video[0].VariableFrameRate == nil || decoded.Video[0].ExactSeekSafe == nil || *decoded.Video[0].ExactSeekSafe || decoded.Video[0].KeyframeEvidenceRevision != "etag:1" || decoded.Subtitles[0].Signs == nil || decoded.Video[0].DolbyVision == nil || !decoded.Video[0].DolbyVision.BaseLayerPresentKnown {
		t.Fatalf("v2 facts did not survive stable JSON: %#v", decoded)
	}
}

func TestCanonicalSortsEveryIndexedCollection(t *testing.T) {
	f := completeFacts()
	f.Video = append(f.Video, Video{Index: 0, Codec: "h264", CodedWidth: 1, CodedHeight: 1, SampleAspectRatio: Rational{1, 1}, DisplayAspectRatio: Rational{1, 1}, PixelFormat: "gray", FrameRate: Rational{1, 1}})
	f.Audio = append(f.Audio, Audio{Index: 1, Codec: "aac", Channels: 2})
	f.Subtitles = append(f.Subtitles, Subtitle{Index: 6, Codec: "srt", Kind: "text"})
	f.Attachments = append(f.Attachments, Attachment{Index: 7, Codec: "mjpeg"})
	f.Chapters = []Chapter{{Index: 2, StartUS: 2}, {Index: 1, StartUS: 1}}
	c, err := f.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if c.Video[0].Index != 0 || c.Audio[0].Index != 1 || c.Subtitles[0].Index != 4 || c.Attachments[0].Index != 5 || c.Chapters[0].Index != 1 {
		t.Fatal("collections not sorted")
	}
}

func TestDigestDistinguishesUnknownUnsafeAndSafeSeekEvidence(t *testing.T) {
	unsafe := completeFacts()
	unsafeDigest, err := unsafe.Digest()
	if err != nil {
		t.Fatal(err)
	}
	safe := unsafe.Clone()
	*safe.Video[0].ExactSeekSafe = true
	safeDigest, err := safe.Digest()
	if err != nil {
		t.Fatal(err)
	}
	unknown := unsafe.Clone()
	unknown.Video[0].ExactSeekSafe = nil
	unknown.Video[0].KeyframeEvidenceAt = ""
	unknown.Video[0].KeyframeEvidenceRevision = ""
	unknownDigest, err := unknown.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if unsafeDigest == safeDigest || unsafeDigest == unknownDigest || safeDigest == unknownDigest {
		t.Fatalf("seek evidence states must have distinct digests: unsafe=%q safe=%q unknown=%q", unsafeDigest, safeDigest, unknownDigest)
	}
}

func TestDynamicRangeClassificationDoesNotInferFromPrimaries(t *testing.T) {
	cases := []struct {
		name string
		v    Video
		want DynamicRange
	}{{"bt2020 alone", Video{ColorPrimaries: "bt2020"}, DynamicRangeSDR}, {"pq", Video{ColorTransfer: "SMPTE2084"}, DynamicRangePQ}, {"hlg", Video{ColorTransfer: "arib-std-b67"}, DynamicRangeHLG}, {"hdr10plus", Video{HDR10Plus: true, ColorTransfer: "smpte2084"}, DynamicRangeHDR10Plus}, {"dolby vision", Video{HDR10Plus: true, DolbyVision: &DolbyVision{Profile: 8, RPU: true, Evidence: "RPU"}}, DynamicRangeDolbyVision}, {"dv label without evidence", Video{DolbyVision: &DolbyVision{Profile: 8}}, DynamicRangeSDR}, {"dv explicit absent rpu", Video{DolbyVision: &DolbyVision{Profile: 8, Evidence: "configuration record"}}, DynamicRangeDolbyVision}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.DynamicRange(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Facts)
	}{
		{"version", func(f *Facts) { f.Version = SchemaVersion + 1 }}, {"fingerprint", func(f *Facts) { f.Source.Fingerprint = "" }}, {"revision", func(f *Facts) { f.Source.Revision = "" }}, {"container", func(f *Facts) { f.Container = "" }}, {"negative duration", func(f *Facts) { f.DurationUS = -1 }}, {"confidence", func(f *Facts) { f.DurationConfidence = "likely" }}, {"duplicate stream", func(f *Facts) { f.Audio[0].Index = f.Video[0].Index }}, {"video codec", func(f *Facts) { f.Video[0].Codec = "" }}, {"video dimensions", func(f *Facts) { f.Video[0].CodedWidth = 0 }}, {"pixel format", func(f *Facts) { f.Video[0].PixelFormat = "" }}, {"rational", func(f *Facts) { f.Video[0].FrameRate.Den = 0 }}, {"rotation", func(f *Facts) { f.Video[0].Rotation = 45 }}, {"matrix", func(f *Facts) { f.Video[0].DisplayMatrix = []int64{1} }}, {"seek evidence timestamp", func(f *Facts) { f.Video[0].KeyframeEvidenceAt = "not-a-time" }}, {"seek evidence revision", func(f *Facts) { f.Video[0].KeyframeEvidenceRevision = "" }}, {"seek metadata without result", func(f *Facts) { f.Video[0].ExactSeekSafe = nil }}, {"dolby evidence", func(f *Facts) { f.Video[0].DolbyVision = &DolbyVision{Profile: 8, RPU: true} }}, {"mastering range", func(f *Facts) { f.Video[0].MasteringDisplay.MinLuminance = Rational{2000, 1} }}, {"audio channels", func(f *Facts) { f.Audio[0].Channels = 0 }}, {"negative gapless", func(f *Facts) { f.Audio[0].EncoderDelaySamples = -1 }}, {"subtitle kind", func(f *Facts) { f.Subtitles[0].Kind = "unknown" }}, {"attachment codec", func(f *Facts) { f.Attachments[0].Codec = "" }}, {"chapter bounds", func(f *Facts) { f.Chapters[0].EndUS = 0; f.Chapters[1].StartUS = -1 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := completeFacts()
			tc.mutate(&f)
			if err := f.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
			if _, err := f.StableJSON(); err == nil {
				t.Fatal("canonical serialization accepted invalid facts")
			}
		})
	}
}

func TestCanonicalRejectsDuplicateIndicesAfterSorting(t *testing.T) {
	f := completeFacts()
	f.Subtitles = append(f.Subtitles, f.Subtitles[0])
	if _, err := f.Canonical(); err == nil {
		t.Fatal("expected duplicate stream error")
	}
}
