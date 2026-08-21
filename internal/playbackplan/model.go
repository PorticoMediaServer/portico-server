// Package playbackplan produces immutable, deterministic playback decisions.
// It is deliberately free of persistence, process execution, and filesystem
// concerns: handlers execute a stored Plan and never reinterpret client flags.
package playbackplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
)

const SchemaRevision = "playback-plan-v2"

type OwnerPolicy string

const (
	MaximumFidelity      OwnerPolicy = "maximum_fidelity"
	MaximumCompatibility OwnerPolicy = "maximum_compatibility"
	MinimizeServerWork   OwnerPolicy = "minimize_server_work"
)

type Mode string

const (
	DirectPlay     Mode = "direct_play"
	Remux          Mode = "remux"
	DirectStream   Mode = "video_copy_audio_convert"
	VideoTranscode Mode = "video_transcode"
	Unsupported    Mode = "unsupported"
)

type Action string

const (
	Copy         Action = "copy"
	Convert      Action = "convert"
	Drop         Action = "drop"
	ExternalText Action = "external_text"
	BurnIn       Action = "burn_in"
)

type ReasonCode string

const (
	ReasonExactTuple           ReasonCode = "exact_tuple"
	ReasonContainerChange      ReasonCode = "container_change"
	ReasonAudioConversion      ReasonCode = "audio_conversion"
	ReasonAudioLayoutReduced   ReasonCode = "audio_layout_reduced"
	ReasonGaplessPreserved     ReasonCode = "gapless_packet_timing_preserved"
	ReasonGaplessUnverified    ReasonCode = "gapless_output_unverified"
	ReasonGaplessFactsUnknown  ReasonCode = "gapless_source_facts_unknown"
	ReasonObjectAudioLost      ReasonCode = "object_audio_not_preserved"
	ReasonVideoConversion      ReasonCode = "video_conversion"
	ReasonVideoConstraint      ReasonCode = "video_constraint_exceeded"
	ReasonAudioConstraint      ReasonCode = "audio_constraint_exceeded"
	ReasonExactSeekUnavailable ReasonCode = "exact_seek_evidence_unavailable"
	ReasonSubtitleExternal     ReasonCode = "subtitle_external"
	ReasonSubtitleBurn         ReasonCode = "subtitle_burn_in"
	ReasonHDRPreserved         ReasonCode = "hdr_preserved"
	ReasonHDRToneMapped        ReasonCode = "hdr_tone_mapped_to_sdr"
	ReasonHDRToneMapDisabled   ReasonCode = "hdr_tone_mapping_disabled"
	ReasonHDR10PlusDowngraded  ReasonCode = "hdr10plus_downgraded_to_hdr10"
	ReasonDVPreserved          ReasonCode = "dolby_vision_preserved"
	ReasonDVVerifiedBase       ReasonCode = "dolby_vision_verified_base_used"
	ReasonNoCompatibleTuple    ReasonCode = "no_compatible_delivery_tuple"
	ReasonDVUnsupported        ReasonCode = "dolby_vision_conversion_unsupported"
	ReasonHardwareUnverified   ReasonCode = "hardware_route_unverified"
	ReasonInvalidInput         ReasonCode = "invalid_input"
)

type Selection struct {
	VideoIndex    *int `json:"videoIndex,omitempty"`
	AudioIndex    *int `json:"audioIndex,omitempty"`
	SubtitleIndex *int `json:"subtitleIndex,omitempty"`
}
type StreamAction struct {
	Index        int    `json:"index"`
	Kind         string `json:"kind"`
	Action       Action `json:"action"`
	InputCodec   string `json:"inputCodec"`
	OutputCodec  string `json:"outputCodec,omitempty"`
	InputLayout  string `json:"inputLayout,omitempty"`
	OutputLayout string `json:"outputLayout,omitempty"`
}
type Stage struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Execution string `json:"execution"`
}
type ColorDecision struct {
	Input              string `json:"input"`
	Output             string `json:"output"`
	Action             string `json:"action"`
	ToneMapAlgorithm   string `json:"toneMapAlgorithm,omitempty"`
	DolbyVisionProfile int    `json:"dolbyVisionProfile,omitempty"`
}
type AudioDecision struct {
	Codec            string          `json:"codec"`
	Layout           string          `json:"layout,omitempty"`
	Channels         int             `json:"channels"`
	Passthrough      bool            `json:"passthrough"`
	ObjectsPreserved bool            `json:"objectsPreserved"`
	Downmixed        bool            `json:"downmixed"`
	Gapless          GaplessDecision `json:"gapless"`
}
type GaplessDecision struct {
	Status                string                `json:"status"`
	Method                string                `json:"method,omitempty"`
	Reason                string                `json:"reason,omitempty"`
	SourceConfidence      mediafacts.Confidence `json:"sourceConfidence"`
	SourceEvidence        string                `json:"sourceEvidence,omitempty"`
	EncoderDelaySamples   int64                 `json:"encoderDelaySamples,omitempty"`
	EncoderPaddingSamples int64                 `json:"encoderPaddingSamples,omitempty"`
	SampleRate            int                   `json:"sampleRate,omitempty"`
	StartTime             *mediafacts.Rational  `json:"startTime,omitempty"`
	Duration              *mediafacts.Rational  `json:"duration,omitempty"`
	TimeBase              *mediafacts.Rational  `json:"timeBase,omitempty"`
}
type SubtitleDecision struct {
	Index           *int   `json:"index,omitempty"`
	Codec           string `json:"codec,omitempty"`
	Kind            string `json:"kind,omitempty"`
	Action          Action `json:"action"`
	Language        string `json:"language,omitempty"`
	Default         bool   `json:"default,omitempty"`
	Forced          bool   `json:"forced,omitempty"`
	HearingImpaired bool   `json:"hearingImpaired,omitempty"`
}
type Timeline struct {
	Mode       string `json:"mode"`
	DurationUS int64  `json:"durationUs,omitempty"`
	Dynamic    bool   `json:"dynamic"`
	Generation uint64 `json:"generation"`
}
type HardwareRoute struct {
	Verified                 bool               `json:"verified"`
	Backend                  playbackhw.Backend `json:"backend,omitempty"`
	Stages                   []Stage            `json:"stages,omitempty"`
	SoftwareFallbackVerified bool               `json:"softwareFallbackVerified"`
}
type Constraints struct {
	MaxVideoBitrate int64 `json:"maxVideoBitrate,omitempty"`
	MaxAudioBitrate int64 `json:"maxAudioBitrate,omitempty"`
	MaxWidth        int   `json:"maxWidth,omitempty"`
	MaxHeight       int   `json:"maxHeight,omitempty"`
}
type Request struct {
	Facts          mediafacts.Facts
	Capabilities   playbackcap.Resolution
	Policy         OwnerPolicy
	Protocol       string
	Selection      Selection
	Hardware       HardwareRoute
	Constraints    Constraints
	AllowedModes   []Mode
	PreferredModes []Mode
	// DisableToneMapping is owner policy, not a client claim. ToneMapAlgorithm
	// is sealed into the plan whenever conversion to SDR is required.
	DisableToneMapping bool
	ToneMapAlgorithm   string
}
type Plan struct {
	SchemaRevision       string                `json:"schemaRevision"`
	Digest               string                `json:"digest"`
	SourceFingerprint    string                `json:"sourceFingerprint"`
	SourceRevision       string                `json:"sourceRevision"`
	CapabilityEvidenceID string                `json:"capabilityEvidenceId"`
	Policy               OwnerPolicy           `json:"policy"`
	Mode                 Mode                  `json:"mode"`
	MediaKind            playbackcap.MediaKind `json:"mediaKind"`
	Protocol             string                `json:"protocol,omitempty"`
	Container            string                `json:"container,omitempty"`
	SegmentFormat        string                `json:"segmentFormat,omitempty"`
	Selection            Selection             `json:"selection"`
	Streams              []StreamAction        `json:"streams,omitempty"`
	Stages               []Stage               `json:"stages,omitempty"`
	Color                *ColorDecision        `json:"color,omitempty"`
	Audio                AudioDecision         `json:"audio"`
	Subtitle             SubtitleDecision      `json:"subtitle"`
	Timeline             Timeline              `json:"timeline"`
	Hardware             HardwareRoute         `json:"hardware"`
	Constraints          Constraints           `json:"constraints"`
	Reasons              []ReasonCode          `json:"reasons"`
}

func (p Plan) StableJSON() ([]byte, error) { q := p; q.Digest = ""; return json.Marshal(q) }
func (p Plan) ComputeDigest() (string, error) {
	b, e := p.StableJSON()
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return SchemaRevision + ":sha256:" + hex.EncodeToString(h[:]), nil
}
func (p Plan) Validate() error {
	if p.SchemaRevision != SchemaRevision {
		return fmt.Errorf("schema revision must be %s", SchemaRevision)
	}
	if p.Mode == Unsupported {
		if len(p.Reasons) == 0 {
			return fmt.Errorf("unsupported plan requires reason")
		}
		return nil
	}
	if p.SourceFingerprint == "" || p.SourceRevision == "" || p.CapabilityEvidenceID == "" {
		return fmt.Errorf("source and capability identity required")
	}
	if p.Protocol == "" || p.Container == "" || len(p.Streams) == 0 {
		return fmt.Errorf("playable plan requires delivery and stream actions")
	}
	d, err := p.ComputeDigest()
	if err != nil {
		return err
	}
	if p.Digest != "" && p.Digest != d {
		return fmt.Errorf("digest mismatch")
	}
	return nil
}
func (p Plan) Clone() Plan { b, _ := json.Marshal(p); var q Plan; _ = json.Unmarshal(b, &q); return q }

type PublicSummary struct {
	SchemaRevision string                `json:"schemaRevision"`
	Digest         string                `json:"digest"`
	Mode           Mode                  `json:"mode"`
	MediaKind      playbackcap.MediaKind `json:"mediaKind"`
	Protocol       string                `json:"protocol,omitempty"`
	Container      string                `json:"container,omitempty"`
	Streams        []StreamAction        `json:"streams,omitempty"`
	Color          *ColorDecision        `json:"color,omitempty"`
	Audio          AudioDecision         `json:"audio"`
	Subtitle       SubtitleDecision      `json:"subtitle"`
	Hardware       HardwareRoute         `json:"hardware"`
	Reasons        []ReasonCode          `json:"reasons"`
}

func (p Plan) PublicSummary() PublicSummary {
	q := p.Clone()
	return PublicSummary{q.SchemaRevision, q.Digest, q.Mode, q.MediaKind, q.Protocol, q.Container, q.Streams, q.Color, q.Audio, q.Subtitle, q.Hardware, q.Reasons}
}

func canonicalReasons(in []ReasonCode) []ReasonCode {
	seen := map[ReasonCode]bool{}
	out := make([]ReasonCode, 0, len(in))
	for _, r := range in {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func token(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
