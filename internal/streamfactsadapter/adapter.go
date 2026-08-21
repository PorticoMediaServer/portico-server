// Package streamfactsadapter converts authoritative probe/persistence records
// into the canonical mediafacts contract. It deliberately has no database or
// ffprobe dependency; callers must preserve presence while decoding either.
package streamfactsadapter

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
)

// SourceRecord is the lossless boundary between persisted/ffprobe data and
// mediafacts. Pointer booleans distinguish an observed false value from a fact
// the source did not report. Adapt rejects unknown values where mediafacts only
// has a bool, rather than silently turning unknown into false.
//
// SourcePath is intentionally absent: adapter errors and summaries therefore
// cannot disclose a filesystem path.
type SourceRecord struct {
	Fingerprint                 string
	Revision                    string
	SizeBytes                   int64
	Container                   string
	DurationUS                  int64
	DurationConfidence          mediafacts.Confidence
	StartTime                   *mediafacts.Rational
	TimeBase                    *mediafacts.Rational
	VariableFrameRate           *bool
	VariableFrameRateConfidence mediafacts.Confidence
	Streams                     []StreamRecord
}

type StreamRecord struct {
	Index              int
	Kind               string
	Codec              string
	Bitrate            int64
	Profile            string
	Level              string
	CodecTag           string
	Duration           *mediafacts.Rational
	DurationConfidence mediafacts.Confidence
	StartTime          *mediafacts.Rational
	TimeBase           *mediafacts.Rational
	Disposition        DispositionRecord

	CodedWidth                  int
	CodedHeight                 int
	SampleAspectRatio           mediafacts.Rational
	DisplayAspectRatio          mediafacts.Rational
	Rotation                    int
	DisplayMatrix               []int64
	PixelFormat                 string
	BitDepth                    int
	ChromaSubsampling           string
	ColorRange                  string
	ColorPrimaries              string
	ColorTransfer               string
	ColorMatrix                 string
	MasteringDisplay            *mediafacts.MasteringDisplay
	MaxCLL                      int
	MaxFALL                     int
	HDR10Plus                   *bool
	DolbyVision                 *DolbyVisionRecord
	FieldOrder                  string
	FrameRate                   mediafacts.Rational
	AverageFrameRate            *mediafacts.Rational
	NominalFrameRate            *mediafacts.Rational
	VariableFrameRate           *bool
	VariableFrameRateConfidence mediafacts.Confidence
	ExactSeekSafe               *bool
	KeyframeEvidenceAt          string
	KeyframeEvidenceRevision    string

	Service               string
	Layout                string
	Channels              int
	SampleRate            int
	SampleFormat          string
	ObjectAudio           string
	ObjectAudioEvidence   string
	EncoderDelaySamples   int64
	EncoderPaddingSamples int64
	GaplessConfidence     mediafacts.Confidence
	GaplessEvidence       string
	Language              string

	SubtitleKind  string
	ClosedCaption *bool
	SDH           *bool
	Signs         *bool

	MIMEType string
	Filename string
	Title    string
}

type DispositionRecord struct {
	Default         *bool
	Forced          *bool
	HearingImpaired *bool
	VisualImpaired  *bool
	Original        *bool
	Commentary      *bool
}

type DolbyVisionRecord struct {
	Profile          int
	Level            int
	RPU              *bool
	EnhancementLayer *bool
	BaseLayerPresent *bool
	BaseLayerCodec   string
	Fallback         string
	Evidence         string
}

// Adapt validates presence deterministically and returns canonical facts.
func Adapt(source SourceRecord) (mediafacts.Facts, error) {
	if err := validatePresence(source); err != nil {
		return mediafacts.Facts{}, err
	}
	facts := mediafacts.Facts{
		Version:                     mediafacts.SchemaVersion,
		Source:                      mediafacts.Source{Fingerprint: source.Fingerprint, Revision: source.Revision, SizeBytes: source.SizeBytes, StartTime: cloneRational(source.StartTime), TimeBase: cloneRational(source.TimeBase)},
		Container:                   source.Container,
		DurationUS:                  source.DurationUS,
		DurationConfidence:          source.DurationConfidence,
		VariableFrameRate:           cloneBool(source.VariableFrameRate),
		VariableFrameRateConfidence: source.VariableFrameRateConfidence,
	}
	streams := append([]StreamRecord(nil), source.Streams...)
	sort.SliceStable(streams, func(i, j int) bool { return streams[i].Index < streams[j].Index })
	for _, stream := range streams {
		d := disposition(stream.Disposition)
		switch canonical(stream.Kind) {
		case "video":
			v := mediafacts.Video{Index: stream.Index, Codec: stream.Codec, Bitrate: stream.Bitrate, Profile: stream.Profile, Level: stream.Level, CodecTag: stream.CodecTag, CodedWidth: stream.CodedWidth, CodedHeight: stream.CodedHeight, SampleAspectRatio: stream.SampleAspectRatio, DisplayAspectRatio: stream.DisplayAspectRatio, Rotation: stream.Rotation, DisplayMatrix: append([]int64(nil), stream.DisplayMatrix...), PixelFormat: stream.PixelFormat, BitDepth: stream.BitDepth, ChromaSubsampling: stream.ChromaSubsampling, ColorRange: stream.ColorRange, ColorPrimaries: stream.ColorPrimaries, ColorTransfer: stream.ColorTransfer, ColorMatrix: stream.ColorMatrix, MasteringDisplay: cloneMastering(stream.MasteringDisplay), MaxCLL: stream.MaxCLL, MaxFALL: stream.MaxFALL, HDR10Plus: *stream.HDR10Plus, HDR10PlusKnown: true, FieldOrder: stream.FieldOrder, FrameRate: stream.FrameRate, AverageFrameRate: cloneRational(stream.AverageFrameRate), NominalFrameRate: cloneRational(stream.NominalFrameRate), VariableFrameRate: cloneBool(stream.VariableFrameRate), VariableFrameRateConfidence: stream.VariableFrameRateConfidence, ExactSeekSafe: cloneBool(stream.ExactSeekSafe), KeyframeEvidenceAt: stream.KeyframeEvidenceAt, KeyframeEvidenceRevision: stream.KeyframeEvidenceRevision, Timing: timing(stream), Disposition: d}
			if stream.DolbyVision != nil {
				dv := stream.DolbyVision
				v.DolbyVision = &mediafacts.DolbyVision{Profile: dv.Profile, Level: dv.Level, RPU: *dv.RPU, RPUKnown: true, EnhancementLayer: *dv.EnhancementLayer, EnhancementLayerKnown: true, BaseLayerPresent: *dv.BaseLayerPresent, BaseLayerPresentKnown: true, BaseLayerCodec: dv.BaseLayerCodec, Fallback: dv.Fallback, Evidence: dv.Evidence}
			}
			facts.Video = append(facts.Video, v)
		case "audio":
			facts.Audio = append(facts.Audio, mediafacts.Audio{Index: stream.Index, Codec: stream.Codec, Bitrate: stream.Bitrate, Profile: stream.Profile, Service: stream.Service, Layout: stream.Layout, Channels: stream.Channels, SampleRate: stream.SampleRate, SampleFormat: stream.SampleFormat, BitDepth: stream.BitDepth, ObjectAudio: stream.ObjectAudio, ObjectAudioEvidence: stream.ObjectAudioEvidence, EncoderDelaySamples: stream.EncoderDelaySamples, EncoderPaddingSamples: stream.EncoderPaddingSamples, GaplessConfidence: stream.GaplessConfidence, GaplessEvidence: stream.GaplessEvidence, Language: stream.Language, Disposition: d, Timing: timing(stream)})
		case "subtitle":
			// SDH is the subtitle-specific expression of the canonical hearing-
			// impaired disposition. Keep the explicit input field so a caller
			// cannot accidentally derive it from a title such as "English SDH".
			d.HearingImpaired = *stream.SDH
			facts.Subtitles = append(facts.Subtitles, mediafacts.Subtitle{Index: stream.Index, Codec: stream.Codec, Kind: stream.SubtitleKind, ClosedCaption: *stream.ClosedCaption, ClosedCaptionKnown: true, SDH: cloneBool(stream.SDH), Signs: cloneBool(stream.Signs), Language: stream.Language, Disposition: d, Timing: timing(stream)})
		case "attachment":
			facts.Attachments = append(facts.Attachments, mediafacts.Attachment{Index: stream.Index, Codec: stream.Codec, MIMEType: stream.MIMEType, Filename: stream.Filename, Title: stream.Title, Timing: timing(stream)})
		}
	}
	return facts.Canonical()
}

func validatePresence(source SourceRecord) error {
	var problems []string
	if source.VariableFrameRate != nil && source.VariableFrameRateConfidence == mediafacts.ConfidenceUnknown {
		problems = append(problems, "variable frame rate has a value without confidence")
	}
	for _, s := range source.Streams {
		kind := canonical(s.Kind)
		prefix := fmt.Sprintf("stream %d", s.Index)
		if kind != "video" && kind != "audio" && kind != "subtitle" && kind != "attachment" {
			problems = append(problems, prefix+" has unsupported kind")
			continue
		}
		if kind != "attachment" {
			for name, value := range map[string]*bool{"commentary": s.Disposition.Commentary, "default": s.Disposition.Default, "forced": s.Disposition.Forced, "hearing-impaired": s.Disposition.HearingImpaired, "original": s.Disposition.Original, "visual-impaired": s.Disposition.VisualImpaired} {
				if value == nil {
					problems = append(problems, prefix+" has unknown "+name+" disposition")
				}
			}
		}
		if kind == "video" {
			if s.HDR10Plus == nil {
				problems = append(problems, prefix+" has unknown HDR10+ presence")
			}
			if s.DolbyVision != nil {
				if s.DolbyVision.RPU == nil {
					problems = append(problems, prefix+" has unknown Dolby Vision RPU presence")
				}
				if s.DolbyVision.EnhancementLayer == nil {
					problems = append(problems, prefix+" has unknown Dolby Vision enhancement-layer presence")
				}
				if s.DolbyVision.BaseLayerPresent == nil {
					problems = append(problems, prefix+" has unknown Dolby Vision base-layer presence")
				}
			}
		}
		if kind == "subtitle" {
			if s.ClosedCaption == nil {
				problems = append(problems, prefix+" has unknown closed-caption status")
			}
			if s.SDH == nil {
				problems = append(problems, prefix+" has unknown SDH status")
			}
			if s.Signs == nil {
				problems = append(problems, prefix+" has unknown signs status")
			}
		}
		if kind == "audio" && s.ObjectAudio != "" && strings.TrimSpace(s.ObjectAudioEvidence) == "" {
			problems = append(problems, prefix+" claims object audio without evidence")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func disposition(d DispositionRecord) mediafacts.Disposition {
	return mediafacts.Disposition{Default: *d.Default, Forced: *d.Forced, HearingImpaired: *d.HearingImpaired, VisualImpaired: *d.VisualImpaired, Original: *d.Original, Commentary: *d.Commentary}
}
func cloneBool(v *bool) *bool {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
func cloneMastering(v *mediafacts.MasteringDisplay) *mediafacts.MasteringDisplay {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
func cloneRational(v *mediafacts.Rational) *mediafacts.Rational {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
func timing(s StreamRecord) mediafacts.Timing {
	return mediafacts.Timing{StartTime: cloneRational(s.StartTime), Duration: cloneRational(s.Duration), TimeBase: cloneRational(s.TimeBase), DurationConfidence: s.DurationConfidence}
}
func canonical(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
