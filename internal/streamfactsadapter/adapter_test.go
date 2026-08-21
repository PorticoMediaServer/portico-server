package streamfactsadapter

import (
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
)

func truth(v bool) *bool { return &v }
func dispositions() DispositionRecord {
	return DispositionRecord{Default: truth(true), Forced: truth(false), HearingImpaired: truth(false), VisualImpaired: truth(false), Original: truth(true), Commentary: truth(false)}
}

func TestAdaptExhaustiveAuthoritativeRecords(t *testing.T) {
	start, timeBase := mediafacts.Rational{Num: -1, Den: 2}, mediafacts.Rational{Num: 1, Den: 90000}
	duration, average, nominal := mediafacts.Rational{Num: 180000, Den: 90000}, mediafacts.Rational{Num: 24000, Den: 1001}, mediafacts.Rational{Num: 24, Den: 1}
	source := SourceRecord{Fingerprint: " sha256:abc ", Revision: " rev-2 ", SizeBytes: 9, Container: " Matroska ", DurationUS: 2_000_000, DurationConfidence: mediafacts.ConfidenceExact, StartTime: &start, TimeBase: &timeBase, VariableFrameRate: truth(true), VariableFrameRateConfidence: mediafacts.ConfidenceExact, Streams: []StreamRecord{
		{Index: 2, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", SubtitleKind: "bitmap", Language: "EN", ClosedCaption: truth(false), SDH: truth(true), Signs: truth(false), Disposition: dispositions()},
		{Index: 0, Kind: "video", Codec: "HEVC", Bitrate: 12000000, Profile: "Main 10", Level: "5.1", CodecTag: "hvc1", Duration: &duration, DurationConfidence: mediafacts.ConfidenceExact, StartTime: &start, TimeBase: &timeBase, CodedWidth: 3840, CodedHeight: 2160, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, Rotation: 90, DisplayMatrix: []int64{0, -1, 0, 1, 0, 0, 0, 0, 1}, PixelFormat: "yuv420p10le", BitDepth: 10, ChromaSubsampling: "4:2:0", ColorRange: "tv", ColorPrimaries: "bt2020", ColorTransfer: "smpte2084", ColorMatrix: "bt2020nc", MaxCLL: 1000, MaxFALL: 400, HDR10Plus: truth(true), DolbyVision: &DolbyVisionRecord{Profile: 8, Level: 6, RPU: truth(true), EnhancementLayer: truth(false), BaseLayerPresent: truth(true), BaseLayerCodec: "hevc", Fallback: "hdr10", Evidence: "configuration record and RPU"}, FieldOrder: "progressive", FrameRate: average, AverageFrameRate: &average, NominalFrameRate: &nominal, VariableFrameRate: truth(true), VariableFrameRateConfidence: mediafacts.ConfidenceExact, ExactSeekSafe: truth(false), KeyframeEvidenceAt: "2026-08-06T10:00:00Z", KeyframeEvidenceRevision: "rev-2", Disposition: dispositions()},
		{Index: 1, Kind: "audio", Codec: "truehd", Bitrate: 640000, Profile: "Atmos", Service: "main", Layout: "7.1", Channels: 8, SampleRate: 48000, SampleFormat: "s32", BitDepth: 24, ObjectAudio: "dolby_atmos", ObjectAudioEvidence: "dependent substream", Language: "eng", Disposition: dispositions()},
	}}
	got, err := Adapt(source)
	if err != nil {
		t.Fatal(err)
	}
	if got.Container != "matroska" || got.Video[0].CodecTag != "hvc1" || got.Video[0].DolbyVision.Profile != 8 || !got.Video[0].HDR10Plus || got.Audio[0].ObjectAudio != "dolby_atmos" || got.Subtitles[0].Kind != "bitmap" || got.VariableFrameRate == nil || !*got.VariableFrameRate {
		t.Fatalf("facts lost: %#v", got)
	}
	if got.Video[0].DisplayMatrix[1] != -1 || got.Video[0].ColorRange != "tv" || got.Video[0].MaxFALL != 400 {
		t.Fatalf("video facts lost: %#v", got.Video[0])
	}
	if got.Video[0].Bitrate != 12000000 || got.Audio[0].Bitrate != 640000 {
		t.Fatalf("stream bitrates lost: video=%d audio=%d", got.Video[0].Bitrate, got.Audio[0].Bitrate)
	}
	if got.Video[0].ExactSeekSafe == nil || *got.Video[0].ExactSeekSafe || got.Video[0].KeyframeEvidenceAt != "2026-08-06T10:00:00Z" || got.Video[0].KeyframeEvidenceRevision != "rev-2" {
		t.Fatalf("exact-seek evidence lost: %#v", got.Video[0])
	}
	if got.Source.StartTime == nil || got.Video[0].Timing.Duration == nil || got.Video[0].AverageFrameRate == nil || got.Video[0].NominalFrameRate == nil || got.Video[0].VariableFrameRate == nil || !got.Video[0].DolbyVision.BaseLayerPresent || !got.Video[0].DolbyVision.BaseLayerPresentKnown || got.Audio[0].ObjectAudioEvidence == "" || got.Subtitles[0].SDH == nil || got.Subtitles[0].Signs == nil {
		t.Fatalf("v2 evidence facts lost: %#v", got)
	}
	if !got.Subtitles[0].Disposition.HearingImpaired || got.Subtitles[0].Disposition.Forced {
		t.Fatalf("subtitle dispositions lost: %#v", got.Subtitles[0])
	}
}

func TestAdaptPreservesUnknownExactSeekEvidence(t *testing.T) {
	source := SourceRecord{Fingerprint: "x", Revision: "r", Container: "mkv", DurationConfidence: mediafacts.ConfidenceUnknown, Streams: []StreamRecord{{Index: 0, Kind: "video", Codec: "h264", CodedWidth: 1920, CodedHeight: 1080, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, HDR10Plus: truth(false), Disposition: dispositions()}}}
	got, err := Adapt(source)
	if err != nil {
		t.Fatal(err)
	}
	if got.Video[0].ExactSeekSafe != nil || got.Video[0].KeyframeEvidenceAt != "" || got.Video[0].KeyframeEvidenceRevision != "" {
		t.Fatalf("unknown evidence collapsed into a result: %#v", got.Video[0])
	}
}

func TestAdaptRejectsUnknownInsteadOfCollapsingToFalse(t *testing.T) {
	s := SourceRecord{Fingerprint: "x", Revision: "r", Container: "mkv", DurationConfidence: mediafacts.ConfidenceUnknown, Streams: []StreamRecord{{Index: 4, Kind: "subtitle", Codec: "subrip", SubtitleKind: "text", ClosedCaption: truth(false), SDH: nil, Signs: truth(false), Disposition: dispositions()}}}
	_, err := Adapt(s)
	if err == nil || !strings.Contains(err.Error(), "unknown SDH status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdaptErrorsAreDeterministicAndNeverContainPaths(t *testing.T) {
	s := SourceRecord{Fingerprint: "x", Revision: "r", Container: "mkv", DurationConfidence: mediafacts.ConfidenceUnknown, Streams: []StreamRecord{{Index: 9, Kind: "video", HDR10Plus: nil, Disposition: DispositionRecord{}}}}
	_, a := Adapt(s)
	_, b := Adapt(s)
	if a == nil || b == nil || a.Error() != b.Error() {
		t.Fatalf("non-deterministic errors: %v / %v", a, b)
	}
	if strings.Contains(a.Error(), "/Users/") || strings.Contains(a.Error(), "\\") {
		t.Fatalf("path leaked: %v", a)
	}
}
