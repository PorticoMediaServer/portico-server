package mediafacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 2

type Confidence string

const (
	ConfidenceUnknown   Confidence = "unknown"
	ConfidenceEstimated Confidence = "estimated"
	ConfidenceExact     Confidence = "exact"
)

type Rational struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}
type Source struct {
	Fingerprint string    `json:"fingerprint"`
	Revision    string    `json:"revision"`
	SizeBytes   int64     `json:"sizeBytes,omitempty"`
	StartTime   *Rational `json:"startTime,omitempty"`
	TimeBase    *Rational `json:"timeBase,omitempty"`
}
type Timing struct {
	StartTime          *Rational  `json:"startTime,omitempty"`
	Duration           *Rational  `json:"duration,omitempty"`
	TimeBase           *Rational  `json:"timeBase,omitempty"`
	DurationConfidence Confidence `json:"durationConfidence"`
}
type Facts struct {
	Version            int        `json:"version"`
	Source             Source     `json:"source"`
	Container          string     `json:"container"`
	DurationUS         int64      `json:"durationUs,omitempty"`
	DurationConfidence Confidence `json:"durationConfidence"`
	// VariableFrameRate is nil when the probe could not establish the mode.
	VariableFrameRate           *bool        `json:"variableFrameRate,omitempty"`
	VariableFrameRateConfidence Confidence   `json:"variableFrameRateConfidence"`
	Video                       []Video      `json:"video,omitempty"`
	Audio                       []Audio      `json:"audio,omitempty"`
	Subtitles                   []Subtitle   `json:"subtitles,omitempty"`
	Attachments                 []Attachment `json:"attachments,omitempty"`
	Chapters                    []Chapter    `json:"chapters,omitempty"`
}
type Disposition struct {
	Default         bool `json:"default,omitempty"`
	Forced          bool `json:"forced,omitempty"`
	HearingImpaired bool `json:"hearingImpaired,omitempty"`
	VisualImpaired  bool `json:"visualImpaired,omitempty"`
	Original        bool `json:"original,omitempty"`
	Commentary      bool `json:"commentary,omitempty"`
}
type Video struct {
	Index                       int               `json:"index"`
	Codec                       string            `json:"codec"`
	Bitrate                     int64             `json:"bitrate,omitempty"`
	Profile                     string            `json:"profile,omitempty"`
	Level                       string            `json:"level,omitempty"`
	CodecTag                    string            `json:"codecTag,omitempty"`
	CodedWidth                  int               `json:"codedWidth"`
	CodedHeight                 int               `json:"codedHeight"`
	SampleAspectRatio           Rational          `json:"sampleAspectRatio"`
	DisplayAspectRatio          Rational          `json:"displayAspectRatio"`
	Rotation                    int               `json:"rotation,omitempty"`
	DisplayMatrix               []int64           `json:"displayMatrix,omitempty"`
	PixelFormat                 string            `json:"pixelFormat"`
	BitDepth                    int               `json:"bitDepth,omitempty"`
	ChromaSubsampling           string            `json:"chromaSubsampling,omitempty"`
	ColorRange                  string            `json:"colorRange,omitempty"`
	ColorPrimaries              string            `json:"colorPrimaries,omitempty"`
	ColorTransfer               string            `json:"colorTransfer,omitempty"`
	ColorMatrix                 string            `json:"colorMatrix,omitempty"`
	MasteringDisplay            *MasteringDisplay `json:"masteringDisplay,omitempty"`
	MaxCLL                      int               `json:"maxCll,omitempty"`
	MaxFALL                     int               `json:"maxFall,omitempty"`
	HDR10Plus                   bool              `json:"hdr10Plus,omitempty"`
	HDR10PlusKnown              bool              `json:"hdr10PlusKnown"`
	DolbyVision                 *DolbyVision      `json:"dolbyVision,omitempty"`
	FieldOrder                  string            `json:"fieldOrder,omitempty"`
	FrameRate                   Rational          `json:"frameRate"` // canonical average rate retained for planner compatibility
	AverageFrameRate            *Rational         `json:"averageFrameRate,omitempty"`
	NominalFrameRate            *Rational         `json:"nominalFrameRate,omitempty"`
	VariableFrameRate           *bool             `json:"variableFrameRate,omitempty"`
	VariableFrameRateConfidence Confidence        `json:"variableFrameRateConfidence"`
	// ExactSeekSafe is nil until an authoritative keyframe-grid analysis has
	// completed. A non-nil false value is an observed unsafe grid, not missing
	// evidence. The evidence revision binds the observation to Source.Revision.
	ExactSeekSafe            *bool       `json:"exactSeekSafe,omitempty"`
	KeyframeEvidenceAt       string      `json:"keyframeEvidenceAt,omitempty"`
	KeyframeEvidenceRevision string      `json:"keyframeEvidenceRevision,omitempty"`
	Timing                   Timing      `json:"timing"`
	Disposition              Disposition `json:"disposition"`
}
type DolbyVision struct {
	Profile               int    `json:"profile"`
	Level                 int    `json:"level,omitempty"`
	RPU                   bool   `json:"rpu"`
	RPUKnown              bool   `json:"rpuKnown"`
	EnhancementLayer      bool   `json:"enhancementLayer"`
	EnhancementLayerKnown bool   `json:"enhancementLayerKnown"`
	BaseLayerPresent      bool   `json:"baseLayerPresent"`
	BaseLayerPresentKnown bool   `json:"baseLayerPresentKnown"`
	BaseLayerCodec        string `json:"baseLayerCodec,omitempty"`
	Fallback              string `json:"fallback,omitempty"`
	Evidence              string `json:"evidence"`
}
type Chromaticity struct {
	X Rational `json:"x"`
	Y Rational `json:"y"`
}
type MasteringDisplay struct {
	Red          Chromaticity `json:"red"`
	Green        Chromaticity `json:"green"`
	Blue         Chromaticity `json:"blue"`
	WhitePoint   Chromaticity `json:"whitePoint"`
	MinLuminance Rational     `json:"minLuminance"`
	MaxLuminance Rational     `json:"maxLuminance"`
}
type Audio struct {
	Index                 int    `json:"index"`
	Codec                 string `json:"codec"`
	Bitrate               int64  `json:"bitrate,omitempty"`
	Profile               string `json:"profile,omitempty"`
	Service               string `json:"service,omitempty"`
	Layout                string `json:"layout,omitempty"`
	Channels              int    `json:"channels"`
	SampleRate            int    `json:"sampleRate,omitempty"`
	SampleFormat          string `json:"sampleFormat,omitempty"`
	BitDepth              int    `json:"bitDepth,omitempty"`
	ObjectAudio           string `json:"objectAudio,omitempty"`
	ObjectAudioEvidence   string `json:"objectAudioEvidence,omitempty"`
	EncoderDelaySamples   int64  `json:"encoderDelaySamples,omitempty"`
	EncoderPaddingSamples int64  `json:"encoderPaddingSamples,omitempty"`
	// GaplessConfidence distinguishes an observed zero delay/padding from probe
	// output that simply did not expose the information. GaplessEvidence names
	// the packet/container field or probe that produced the values.
	GaplessConfidence Confidence  `json:"gaplessConfidence"`
	GaplessEvidence   string      `json:"gaplessEvidence,omitempty"`
	Language          string      `json:"language,omitempty"`
	Disposition       Disposition `json:"disposition"`
	Timing            Timing      `json:"timing"`
}
type Subtitle struct {
	Index              int         `json:"index"`
	Codec              string      `json:"codec"`
	Kind               string      `json:"kind"`
	ClosedCaption      bool        `json:"closedCaption,omitempty"`
	ClosedCaptionKnown bool        `json:"closedCaptionKnown"`
	SDH                *bool       `json:"sdh,omitempty"`
	Signs              *bool       `json:"signs,omitempty"`
	Language           string      `json:"language,omitempty"`
	Disposition        Disposition `json:"disposition"`
	Timing             Timing      `json:"timing"`
}
type Attachment struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	MIMEType string `json:"mimeType,omitempty"`
	Filename string `json:"filename,omitempty"`
	Title    string `json:"title,omitempty"`
	Timing   Timing `json:"timing"`
}
type Chapter struct {
	Index   int    `json:"index"`
	StartUS int64  `json:"startUs"`
	EndUS   int64  `json:"endUs,omitempty"`
	Title   string `json:"title,omitempty"`
}

type DynamicRange string

const (
	DynamicRangeSDR         DynamicRange = "sdr"
	DynamicRangePQ          DynamicRange = "pq"
	DynamicRangeHLG         DynamicRange = "hlg"
	DynamicRangeHDR10Plus   DynamicRange = "hdr10plus"
	DynamicRangeDolbyVision DynamicRange = "dolby_vision"
)

func (v Video) DynamicRange() DynamicRange {
	if v.DolbyVision != nil && v.DolbyVision.Profile > 0 && strings.TrimSpace(v.DolbyVision.Evidence) != "" {
		return DynamicRangeDolbyVision
	}
	if v.HDR10Plus {
		return DynamicRangeHDR10Plus
	}
	switch canonicalToken(v.ColorTransfer) {
	case "smpte2084", "pq":
		return DynamicRangePQ
	case "arib-std-b67", "hlg":
		return DynamicRangeHLG
	}
	return DynamicRangeSDR
}

func (f Facts) Clone() Facts {
	b, _ := json.Marshal(f)
	var out Facts
	_ = json.Unmarshal(b, &out)
	return out
}

func (f Facts) Canonical() (Facts, error) {
	c := f.Clone()
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	c.Container = canonicalToken(c.Container)
	c.Source.Fingerprint = strings.TrimSpace(c.Source.Fingerprint)
	c.Source.Revision = strings.TrimSpace(c.Source.Revision)
	if c.DurationConfidence == "" {
		c.DurationConfidence = ConfidenceUnknown
	}
	if c.VariableFrameRateConfidence == "" {
		c.VariableFrameRateConfidence = ConfidenceUnknown
	}
	canonicalOptionalRational(c.Source.StartTime)
	canonicalOptionalRational(c.Source.TimeBase)
	for i := range c.Video {
		canonicalVideo(&c.Video[i])
	}
	for i := range c.Audio {
		canonicalAudio(&c.Audio[i])
	}
	for i := range c.Subtitles {
		canonicalSubtitle(&c.Subtitles[i])
	}
	for i := range c.Attachments {
		c.Attachments[i].Codec = canonicalToken(c.Attachments[i].Codec)
		c.Attachments[i].MIMEType = strings.ToLower(strings.TrimSpace(c.Attachments[i].MIMEType))
		c.Attachments[i].Filename = strings.TrimSpace(c.Attachments[i].Filename)
		c.Attachments[i].Title = strings.TrimSpace(c.Attachments[i].Title)
		canonicalTiming(&c.Attachments[i].Timing)
	}
	for i := range c.Chapters {
		c.Chapters[i].Title = strings.TrimSpace(c.Chapters[i].Title)
	}
	sort.Slice(c.Video, func(i, j int) bool { return c.Video[i].Index < c.Video[j].Index })
	sort.Slice(c.Audio, func(i, j int) bool { return c.Audio[i].Index < c.Audio[j].Index })
	sort.Slice(c.Subtitles, func(i, j int) bool { return c.Subtitles[i].Index < c.Subtitles[j].Index })
	sort.Slice(c.Attachments, func(i, j int) bool { return c.Attachments[i].Index < c.Attachments[j].Index })
	sort.Slice(c.Chapters, func(i, j int) bool { return c.Chapters[i].Index < c.Chapters[j].Index })
	if len(c.Video) == 0 {
		c.Video = nil
	}
	if len(c.Audio) == 0 {
		c.Audio = nil
	}
	if len(c.Subtitles) == 0 {
		c.Subtitles = nil
	}
	if len(c.Attachments) == 0 {
		c.Attachments = nil
	}
	if len(c.Chapters) == 0 {
		c.Chapters = nil
	}
	if err := c.Validate(); err != nil {
		return Facts{}, err
	}
	return c, nil
}

func (f Facts) StableJSON() ([]byte, error) {
	c, err := f.Canonical()
	if err != nil {
		return nil, err
	}
	return json.Marshal(c)
}
func (f Facts) Digest() (string, error) {
	b, err := f.StableJSON()
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return "mediafacts-v2:sha256:" + hex.EncodeToString(s[:]), nil
}

func (f Facts) Validate() error {
	var es []error
	if f.Version != SchemaVersion {
		es = append(es, fmt.Errorf("version must be %d", SchemaVersion))
	}
	if strings.TrimSpace(f.Source.Fingerprint) == "" {
		es = append(es, errors.New("source fingerprint is required"))
	}
	if strings.TrimSpace(f.Source.Revision) == "" {
		es = append(es, errors.New("source revision is required"))
	}
	if f.Source.SizeBytes < 0 {
		es = append(es, errors.New("source size cannot be negative"))
	}
	checkOptionalRational(&es, f.Source.StartTime, "source start time", true)
	checkOptionalRational(&es, f.Source.TimeBase, "source time base", false)
	if strings.TrimSpace(f.Container) == "" {
		es = append(es, errors.New("container is required"))
	}
	if f.DurationUS < 0 {
		es = append(es, errors.New("duration cannot be negative"))
	}
	if !oneOf(string(f.DurationConfidence), "unknown", "estimated", "exact") {
		es = append(es, errors.New("invalid duration confidence"))
	}
	if !oneOf(string(f.VariableFrameRateConfidence), "unknown", "estimated", "exact") {
		es = append(es, errors.New("invalid variable frame rate confidence"))
	}
	if f.VariableFrameRate == nil && f.VariableFrameRateConfidence != ConfidenceUnknown || f.VariableFrameRate != nil && f.VariableFrameRateConfidence == ConfidenceUnknown {
		es = append(es, errors.New("variable frame rate value and confidence disagree"))
	}
	seen := map[int]string{}
	add := func(idx int, kind string) {
		if idx < 0 {
			es = append(es, fmt.Errorf("%s index cannot be negative", kind))
			return
		}
		if prior, ok := seen[idx]; ok {
			es = append(es, fmt.Errorf("stream index %d duplicated by %s and %s", idx, prior, kind))
		}
		seen[idx] = kind
	}
	for _, v := range f.Video {
		add(v.Index, "video")
		if v.Codec == "" || v.PixelFormat == "" || v.CodedWidth <= 0 || v.CodedHeight <= 0 {
			es = append(es, fmt.Errorf("video %d lacks codec, pixel format, or dimensions", v.Index))
		}
		checkRational(&es, v.SampleAspectRatio, "sample aspect ratio")
		checkRational(&es, v.DisplayAspectRatio, "display aspect ratio")
		checkRational(&es, v.FrameRate, "frame rate")
		checkOptionalRational(&es, v.AverageFrameRate, "average frame rate", false)
		checkOptionalRational(&es, v.NominalFrameRate, "nominal frame rate", false)
		validateTiming(&es, v.Timing, fmt.Sprintf("video %d", v.Index))
		validatePresenceConfidence(&es, v.VariableFrameRate, v.VariableFrameRateConfidence, fmt.Sprintf("video %d variable frame rate", v.Index))
		if v.ExactSeekSafe == nil {
			if v.KeyframeEvidenceAt != "" || v.KeyframeEvidenceRevision != "" {
				es = append(es, fmt.Errorf("video %d keyframe evidence metadata requires an exact-seek result", v.Index))
			}
		} else {
			if strings.TrimSpace(v.KeyframeEvidenceAt) == "" || strings.TrimSpace(v.KeyframeEvidenceRevision) == "" {
				es = append(es, fmt.Errorf("video %d exact-seek result requires keyframe evidence timestamp and revision", v.Index))
			} else if _, err := time.Parse(time.RFC3339Nano, v.KeyframeEvidenceAt); err != nil {
				es = append(es, fmt.Errorf("video %d keyframe evidence timestamp must be RFC3339", v.Index))
			}
		}
		if !v.HDR10PlusKnown && v.HDR10Plus {
			es = append(es, fmt.Errorf("video %d HDR10+ cannot be true when unknown", v.Index))
		}
		if v.Bitrate < 0 || v.BitDepth < 0 || v.MaxCLL < 0 || v.MaxFALL < 0 {
			es = append(es, fmt.Errorf("video %d has negative numeric facts", v.Index))
		}
		if v.Rotation%90 != 0 || v.Rotation < -270 || v.Rotation > 270 {
			es = append(es, fmt.Errorf("video %d has invalid rotation", v.Index))
		}
		if len(v.DisplayMatrix) != 0 && len(v.DisplayMatrix) != 9 {
			es = append(es, fmt.Errorf("video %d display matrix must contain 9 entries", v.Index))
		}
		if v.DolbyVision != nil {
			if v.DolbyVision.Profile <= 0 || strings.TrimSpace(v.DolbyVision.Evidence) == "" {
				es = append(es, fmt.Errorf("video %d Dolby Vision requires profile and evidence", v.Index))
			}
			if !v.DolbyVision.RPUKnown && v.DolbyVision.RPU || !v.DolbyVision.EnhancementLayerKnown && v.DolbyVision.EnhancementLayer || !v.DolbyVision.BaseLayerPresentKnown && v.DolbyVision.BaseLayerPresent {
				es = append(es, fmt.Errorf("video %d Dolby Vision presence value is marked unknown", v.Index))
			}
		}
		if v.MasteringDisplay != nil {
			validateMasteringDisplay(&es, v.Index, v.MasteringDisplay)
		}
	}
	for _, a := range f.Audio {
		add(a.Index, "audio")
		if a.Codec == "" || a.Channels <= 0 {
			es = append(es, fmt.Errorf("audio %d requires codec and channels", a.Index))
		}
		if a.Bitrate < 0 || a.SampleRate < 0 || a.BitDepth < 0 || a.EncoderDelaySamples < 0 || a.EncoderPaddingSamples < 0 {
			es = append(es, fmt.Errorf("audio %d has negative numeric facts", a.Index))
		}
		if !oneOf(string(a.GaplessConfidence), string(ConfidenceUnknown), string(ConfidenceEstimated), string(ConfidenceExact)) {
			es = append(es, fmt.Errorf("audio %d has invalid gapless confidence", a.Index))
		}
		if a.GaplessConfidence == ConfidenceExact && strings.TrimSpace(a.GaplessEvidence) == "" {
			es = append(es, fmt.Errorf("audio %d exact gapless facts require evidence", a.Index))
		}
		if a.ObjectAudio != "" && strings.TrimSpace(a.ObjectAudioEvidence) == "" {
			es = append(es, fmt.Errorf("audio %d object audio requires evidence", a.Index))
		}
		validateTiming(&es, a.Timing, fmt.Sprintf("audio %d", a.Index))
	}
	for _, s := range f.Subtitles {
		add(s.Index, "subtitle")
		if s.Codec == "" || !oneOf(s.Kind, "text", "bitmap") {
			es = append(es, fmt.Errorf("subtitle %d requires codec and text or bitmap kind", s.Index))
		}
		if !s.ClosedCaptionKnown && s.ClosedCaption {
			es = append(es, fmt.Errorf("subtitle %d closed-caption value is marked unknown", s.Index))
		}
		validateTiming(&es, s.Timing, fmt.Sprintf("subtitle %d", s.Index))
	}
	for _, a := range f.Attachments {
		add(a.Index, "attachment")
		if a.Codec == "" {
			es = append(es, fmt.Errorf("attachment %d requires codec", a.Index))
		}
		validateTiming(&es, a.Timing, fmt.Sprintf("attachment %d", a.Index))
	}
	lastEnd := int64(0)
	for i, c := range f.Chapters {
		if c.Index < 0 || c.StartUS < 0 || c.EndUS < 0 || c.EndUS > 0 && c.EndUS <= c.StartUS || i > 0 && c.StartUS < lastEnd {
			es = append(es, fmt.Errorf("chapter %d has invalid or overlapping bounds", c.Index))
		}
		if c.EndUS > 0 {
			lastEnd = c.EndUS
		} else {
			lastEnd = c.StartUS
		}
	}
	return errors.Join(es...)
}

func canonicalVideo(v *Video) {
	v.Codec = canonicalToken(v.Codec)
	v.Profile = strings.TrimSpace(v.Profile)
	v.Level = strings.TrimSpace(v.Level)
	v.CodecTag = strings.ToLower(strings.TrimSpace(v.CodecTag))
	v.PixelFormat = canonicalToken(v.PixelFormat)
	v.ChromaSubsampling = canonicalToken(v.ChromaSubsampling)
	v.ColorRange = canonicalToken(v.ColorRange)
	v.ColorPrimaries = canonicalToken(v.ColorPrimaries)
	v.ColorTransfer = canonicalToken(v.ColorTransfer)
	v.ColorMatrix = canonicalToken(v.ColorMatrix)
	v.FieldOrder = canonicalToken(v.FieldOrder)
	v.KeyframeEvidenceAt = strings.TrimSpace(v.KeyframeEvidenceAt)
	v.KeyframeEvidenceRevision = strings.TrimSpace(v.KeyframeEvidenceRevision)
	v.Rotation = ((v.Rotation % 360) + 360) % 360
	if v.Rotation > 180 {
		v.Rotation -= 360
	}
	reduce(&v.SampleAspectRatio)
	reduce(&v.DisplayAspectRatio)
	reduce(&v.FrameRate)
	canonicalOptionalRational(v.AverageFrameRate)
	canonicalOptionalRational(v.NominalFrameRate)
	canonicalTiming(&v.Timing)
	if v.VariableFrameRateConfidence == "" {
		v.VariableFrameRateConfidence = ConfidenceUnknown
	}
	if v.MasteringDisplay != nil {
		canonicalMasteringDisplay(v.MasteringDisplay)
	}
	if v.DolbyVision != nil {
		v.DolbyVision.BaseLayerCodec = canonicalToken(v.DolbyVision.BaseLayerCodec)
		v.DolbyVision.Fallback = canonicalToken(v.DolbyVision.Fallback)
		v.DolbyVision.Evidence = strings.TrimSpace(v.DolbyVision.Evidence)
	}
}
func canonicalAudio(a *Audio) {
	a.Codec = canonicalToken(a.Codec)
	a.Profile = strings.TrimSpace(a.Profile)
	a.Service = canonicalToken(a.Service)
	a.Layout = canonicalToken(a.Layout)
	a.SampleFormat = canonicalToken(a.SampleFormat)
	a.ObjectAudio = canonicalToken(a.ObjectAudio)
	a.ObjectAudioEvidence = strings.TrimSpace(a.ObjectAudioEvidence)
	if a.GaplessConfidence == "" {
		a.GaplessConfidence = ConfidenceUnknown
	}
	a.GaplessEvidence = strings.TrimSpace(a.GaplessEvidence)
	a.Language = canonicalLanguage(a.Language)
	canonicalTiming(&a.Timing)
}
func canonicalSubtitle(s *Subtitle) {
	s.Codec = canonicalToken(s.Codec)
	s.Kind = canonicalToken(s.Kind)
	s.Language = canonicalLanguage(s.Language)
	canonicalTiming(&s.Timing)
}
func canonicalToken(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func canonicalLanguage(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", "-"))
}
func oneOf(v string, vals ...string) bool {
	for _, x := range vals {
		if v == x {
			return true
		}
	}
	return false
}
func checkRational(es *[]error, r Rational, name string) {
	if r.Den <= 0 || r.Num < 0 {
		*es = append(*es, fmt.Errorf("%s must be non-negative with positive denominator", name))
	}
}

func checkOptionalRational(es *[]error, r *Rational, name string, signed bool) {
	if r == nil {
		return
	}
	if r.Den <= 0 || !signed && r.Num < 0 {
		*es = append(*es, fmt.Errorf("%s must have a positive denominator%s", name, map[bool]string{true: "", false: " and non-negative numerator"}[signed]))
	}
}

func canonicalOptionalRational(r *Rational) {
	if r != nil {
		reduce(r)
	}
}

func canonicalTiming(t *Timing) {
	canonicalOptionalRational(t.StartTime)
	canonicalOptionalRational(t.Duration)
	canonicalOptionalRational(t.TimeBase)
	if t.DurationConfidence == "" {
		t.DurationConfidence = ConfidenceUnknown
	}
}

func validateTiming(es *[]error, t Timing, owner string) {
	checkOptionalRational(es, t.StartTime, owner+" start time", true)
	checkOptionalRational(es, t.Duration, owner+" duration", false)
	checkOptionalRational(es, t.TimeBase, owner+" time base", false)
	if !oneOf(string(t.DurationConfidence), "unknown", "estimated", "exact") {
		*es = append(*es, fmt.Errorf("%s has invalid duration confidence", owner))
	}
	if t.Duration == nil && t.DurationConfidence != ConfidenceUnknown || t.Duration != nil && t.DurationConfidence == ConfidenceUnknown {
		*es = append(*es, fmt.Errorf("%s duration value and confidence disagree", owner))
	}
}

func validatePresenceConfidence(es *[]error, value *bool, confidence Confidence, owner string) {
	if confidence == "" {
		confidence = ConfidenceUnknown
	}
	if !oneOf(string(confidence), "unknown", "estimated", "exact") {
		*es = append(*es, errors.New(owner+" has invalid confidence"))
		return
	}
	if value == nil && confidence != ConfidenceUnknown || value != nil && confidence == ConfidenceUnknown {
		*es = append(*es, errors.New(owner+" value and confidence disagree"))
	}
}

func canonicalMasteringDisplay(m *MasteringDisplay) {
	values := []*Rational{&m.Red.X, &m.Red.Y, &m.Green.X, &m.Green.Y, &m.Blue.X, &m.Blue.Y, &m.WhitePoint.X, &m.WhitePoint.Y, &m.MinLuminance, &m.MaxLuminance}
	for _, value := range values {
		reduce(value)
	}
}

func validateMasteringDisplay(es *[]error, index int, m *MasteringDisplay) {
	values := []struct {
		name  string
		value Rational
	}{{"red x", m.Red.X}, {"red y", m.Red.Y}, {"green x", m.Green.X}, {"green y", m.Green.Y}, {"blue x", m.Blue.X}, {"blue y", m.Blue.Y}, {"white x", m.WhitePoint.X}, {"white y", m.WhitePoint.Y}, {"minimum luminance", m.MinLuminance}, {"maximum luminance", m.MaxLuminance}}
	for _, item := range values {
		checkRational(es, item.value, fmt.Sprintf("video %d mastering %s", index, item.name))
	}
	if rationalCompare(m.MinLuminance, m.MaxLuminance) > 0 {
		*es = append(*es, fmt.Errorf("video %d mastering minimum luminance exceeds maximum", index))
	}
}

func rationalCompare(a, b Rational) int {
	left, right := a.Num*b.Den, b.Num*a.Den
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
func reduce(r *Rational) {
	if r.Den == 0 {
		return
	}
	if r.Den < 0 {
		r.Num = -r.Num
		r.Den = -r.Den
	}
	a, b := int64(math.Abs(float64(r.Num))), r.Den
	for b != 0 {
		a, b = b, a%b
	}
	if a > 1 {
		r.Num /= a
		r.Den /= a
	}
}
