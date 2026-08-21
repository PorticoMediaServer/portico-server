package app

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	_ "modernc.org/sqlite"
)

func TestPlaybackFactsFromFFprobePreservesVideoEvidence(t *testing.T) {
	one, zero := 1, 0
	tests := []struct {
		name, transfer, sideType string
		dvProfile                int
		wantRange                string
	}{
		{"sdr", "bt709", "", 0, "sdr"},
		{"hdr10", "smpte2084", "Mastering display metadata", 0, "pq"},
		{"hlg", "arib-std-b67", "", 0, "hlg"},
		{"hdr10plus", "smpte2084", "HDR Dynamic Metadata SMPTE2094-40", 0, "hdr10plus"},
		{"dv5", "smpte2084", "DOVI configuration record", 5, "dolby_vision"},
		{"dv7", "smpte2084", "DOVI configuration record", 7, "dolby_vision"},
		{"dv8", "smpte2084", "DOVI configuration record", 8, "dolby_vision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			side := ffprobeSideData{SideDataType: tc.sideType, DVProfile: tc.dvProfile, DVLevel: 6}
			if tc.dvProfile > 0 {
				side.RPUPresent, side.ELPresent, side.BLPresent = &one, &zero, &one
			}
			payload := ffprobePayload{Format: ffprobeFormat{FormatName: "matroska", Duration: "12.5"}, Streams: []ffprobeStream{{Index: 0, CodecType: "video", CodecName: "hevc", Width: 3840, Height: 2160, PixelFormat: "yuv420p10le", AverageFrameRate: "24000/1001", FrameRate: "24/1", SampleAspectRatio: "1:1", AspectRatio: "16:9", ColorTransfer: tc.transfer, Disposition: map[string]int{}, SideDataList: []ffprobeSideData{side}}}}
			facts, err := playbackFactsFromFFprobe(canonicalAnalysisFileIdentity("file-1", "fp-1", 99, "2026-01-01T00:00:00Z"), payload)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(facts.Video[0].DynamicRange()); got != tc.wantRange {
				t.Fatalf("dynamic range=%q want %q", got, tc.wantRange)
			}
			if facts.Video[0].VariableFrameRate == nil || !*facts.Video[0].VariableFrameRate {
				t.Fatal("expected explicit VFR evidence")
			}
		})
	}
}

func TestPlaybackFactsFromFFprobeMapsAudioSubtitleAndAttachment(t *testing.T) {
	payload := ffprobePayload{Format: ffprobeFormat{FormatName: "matroska", Duration: "1"}, Streams: []ffprobeStream{
		{Index: 1, CodecType: "audio", CodecName: "truehd", Profile: "Dolby TrueHD + Dolby Atmos", Channels: 8, ChannelLayout: "7.1", SampleRate: "48000", SampleFormat: "s32p", BitsPerRawSample: "24", Disposition: map[string]int{"default": 1}},
		{Index: 2, CodecType: "audio", CodecName: "dts", Profile: "DTS-HD MA", Channels: 6, SampleRate: "48000", SampleFormat: "s32p", Disposition: map[string]int{}},
		{Index: 3, CodecType: "audio", CodecName: "pcm_s24le", Channels: 2, SampleRate: "96000", SampleFormat: "s32", BitsPerRawSample: "24", Disposition: map[string]int{}},
		{Index: 4, CodecType: "subtitle", CodecName: "subrip", Disposition: map[string]int{}},
		{Index: 5, CodecType: "subtitle", CodecName: "hdmv_pgs_subtitle", Disposition: map[string]int{"hearing_impaired": 1}},
		{Index: 6, CodecType: "subtitle", CodecName: "dvd_subtitle", Disposition: map[string]int{}},
		{Index: 7, CodecType: "attachment", CodecName: "ttf", Tags: map[string]string{"filename": "font.ttf", "mimetype": "font/ttf"}},
	}}
	facts, err := playbackFactsFromFFprobe(canonicalAnalysisFileIdentity("file-2", "fp-2", 100, "now"), payload)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Audio[0].ObjectAudio != "dolby_atmos" || facts.Audio[0].BitDepth != 24 {
		t.Fatalf("unexpected TrueHD facts: %+v", facts.Audio[0])
	}
	if facts.Subtitles[0].Kind != "text" || facts.Subtitles[1].Kind != "bitmap" || facts.Subtitles[1].SDH == nil || !*facts.Subtitles[1].SDH {
		t.Fatalf("unexpected subtitles: %+v", facts.Subtitles)
	}
	if len(facts.Attachments) != 1 || facts.Attachments[0].Filename != "font.ttf" {
		t.Fatalf("unexpected attachments: %+v", facts.Attachments)
	}
}

func TestPlaybackFactsRejectsIncompleteDolbyVisionWithoutLeakingPaths(t *testing.T) {
	payload := ffprobePayload{Format: ffprobeFormat{FormatName: "matroska"}, Streams: []ffprobeStream{{Index: 0, CodecType: "video", CodecName: "hevc", Width: 1920, Height: 1080, PixelFormat: "yuv420p10le", SampleAspectRatio: "1:1", AspectRatio: "16:9", FrameRate: "24/1", Disposition: map[string]int{}, SideDataList: []ffprobeSideData{{SideDataType: "DOVI configuration record", DVProfile: 8}}}}}
	_, err := playbackFactsFromFFprobe(canonicalAnalysisFileIdentity("file-3", "", 1, "now"), payload)
	if err == nil {
		t.Fatal("expected incomplete Dolby Vision evidence to be rejected")
	}
	if strings.Contains(err.Error(), "/") || strings.Contains(err.Error(), "\\") {
		t.Fatalf("error may disclose a path: %v", err)
	}
}

func TestApplyPersistedKeyframeEvidencePreservesUnknownFalseAndTrue(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE media_streams (
		media_id TEXT, file_id TEXT, source_kind TEXT, kind TEXT,
		stream_index INTEGER, exact_seek_safe INTEGER, keyframe_evidence_at TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]any{
		{"media-1", "file-1", "ffprobe", "video", 0, 0, "2026-08-06T10:00:00Z"},
		{"media-1", "file-1", "ffprobe", "video", 1, 1, "2026-08-06T10:01:00Z"},
		{"media-1", "file-1", "ffprobe", "video", 2, 0, ""},
	} {
		if _, err := db.Exec(`INSERT INTO media_streams VALUES (?, ?, ?, ?, ?, ?, ?)`, args...); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	file := canonicalAnalysisFileIdentity("file-1", "fp-1", 99, "2026-08-06T09:00:00Z")
	facts := mediaFactsWithVideoIndices(0, 1, 2)
	if err := applyPersistedKeyframeEvidence(tx, "media-1", file, &facts); err != nil {
		t.Fatal(err)
	}
	if facts.Video[0].ExactSeekSafe == nil || *facts.Video[0].ExactSeekSafe {
		t.Fatalf("explicit unsafe result was lost: %#v", facts.Video[0])
	}
	if facts.Video[1].ExactSeekSafe == nil || !*facts.Video[1].ExactSeekSafe {
		t.Fatalf("explicit safe result was lost: %#v", facts.Video[1])
	}
	if facts.Video[2].ExactSeekSafe != nil || facts.Video[2].KeyframeEvidenceAt != "" || facts.Video[2].KeyframeEvidenceRevision != "" {
		t.Fatalf("unknown result collapsed to false: %#v", facts.Video[2])
	}
	if facts.Video[0].KeyframeEvidenceRevision != file.revision() || facts.Video[1].KeyframeEvidenceRevision != file.revision() {
		t.Fatalf("evidence was not bound to source revision: %#v", facts.Video)
	}
}

func mediaFactsWithVideoIndices(indices ...int) mediafacts.Facts {
	facts := mediafacts.Facts{Version: mediafacts.SchemaVersion, Source: mediafacts.Source{Fingerprint: "fp-1", Revision: "revision-1"}, Container: "matroska", DurationConfidence: mediafacts.ConfidenceUnknown, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown}
	for _, index := range indices {
		facts.Video = append(facts.Video, mediafacts.Video{Index: index, Codec: "h264", CodedWidth: 1920, CodedHeight: 1080, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, HDR10PlusKnown: true, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown})
	}
	return facts
}
