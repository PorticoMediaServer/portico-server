package playback

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/playback/vod"
	"portico.local/server/internal/preparedmedia"
)

// DeliveryProfileLegacy is a server policy, not a claim that every client can
// decode these formats. It preserves the existing qualified delivery choices.
const finiteDeliveryProfile = vod.Policy

const DeliveryProfileLegacy = "portico_legacy_baseline_v1"

type DeliveryStrategy string

const (
	DeliveryOriginal        DeliveryStrategy = "original"
	DeliveryCopyRemux       DeliveryStrategy = "copy_remux"
	DeliveryAudioConversion DeliveryStrategy = "audio_conversion"
	DeliveryVideoConversion DeliveryStrategy = "video_conversion"
)

// Delivery reason codes. Every plan publishes the codes that produced it, and
// every refusal names the code that caused it. These strings are part of the
// wire contract: clients key their explanations off them, so they are added to
// rather than reworded.
const (
	// Chosen-route codes.
	ReasonDirectPlayCompatible      = "direct_play_compatible"
	ReasonDirectAudioCompatible     = "direct_audio_compatible"
	ReasonRemotePreflightCompatible = "remote_preflight_compatible"
	ReasonPreparedVersionCompatible = "prepared_version_compatible"
	ReasonContainerRequiresRemux    = "container_requires_remux"
	ReasonAudioCodecRequiresConvert = "audio_codec_requires_conversion"
	ReasonVideoCodecRequiresConvert = "video_codec_requires_conversion"
	ReasonHeightExceedsPolicy       = "height_exceeds_policy"
	ReasonBitrateExceedsPolicy      = "bitrate_exceeds_policy"
	ReasonHDRNotAllowed             = "hdr_tone_mapping_required"
	ReasonQualityRungSelected       = "quality_rung_selected"
	ReasonSubtitleBurnIn            = "subtitle_burn_in"
	ReasonFiniteNormalization       = "finite_timeline_normalization"
	ReasonAlternateAudioPolicy      = "alternate_audio_fixed_output_policy"
	// ReasonDolbyVisionApproximate: a Dolby Vision Profile 5 picture (no HDR10
	// or SDR base layer) is converted by a tone mapper that can't apply its
	// reshaping, so its colors are approximate. Clients warn before playing.
	ReasonDolbyVisionApproximate = "dolby_vision_colors_approximate"

	// Preference and owner-policy codes.
	ReasonDirectPlayPreferred     = "direct_play_preferred"
	ReasonDirectStreamPreferred   = "direct_stream_preferred"
	ReasonTranscodePreferred      = "transcode_preferred"
	ReasonPlanningMaximumFidelity = "planning_maximum_fidelity"
	ReasonPlanningCompatibility   = "planning_maximum_compatibility"
	ReasonPlanningMinimizeWork    = "planning_minimize_server_work"
	ReasonHardwareBackend         = "hardware_backend_selected"
	ReasonSoftwareFallback        = "software_encode_fallback"

	// Refusal codes. A refusal always carries exactly one of these.
	ReasonDirectPlayRequired     = "direct_play_required_but_unavailable"
	ReasonDirectStreamRequired   = "direct_stream_required_but_unavailable"
	ReasonDirectPlayRefused      = "direct_play_refused_by_preference"
	ReasonDirectStreamRefused    = "direct_stream_refused_by_preference"
	ReasonTranscodeRefused       = "transcode_refused_by_preference"
	ReasonTranscodeDisabledOwner = "transcode_disabled_by_owner"
	ReasonNoAdmissibleRoute      = "no_admissible_delivery_route"
)

// ErrDeliveryRefused is a policy refusal, not a failure. Its reason code names
// the preference or clamp that closed every route.
type ErrDeliveryRefused struct{ Code string }

func (e ErrDeliveryRefused) Error() string {
	switch e.Code {
	case ReasonDirectPlayRequired:
		return "This source cannot play directly, and direct play is required for this network."
	case ReasonDirectStreamRequired:
		return "This source cannot be repackaged without conversion, and direct stream is required for this network."
	case ReasonTranscodeRefused, ReasonTranscodeDisabledOwner:
		return "This source needs conversion, which is not permitted for this network."
	}
	return "No permitted delivery route is available for this source on this network."
}

type BurnInSpec struct {
	ResourceID string `json:"resourceId"`
	Revision   int64  `json:"revision"`
	Format     string `json:"format"`
}

type DeliveryPlan struct {
	BurnIn           *BurnInSpec             `json:"burnIn,omitempty"`
	TextRenditions   []TextRenditionPlan     `json:"-"`
	Renditions       []AudioRenditionPlan    `json:"audioRenditions,omitempty"`
	Prepared         *PreparedDelivery       `json:"prepared,omitempty"`
	Policy           *ResolvedDeliveryPolicy `json:"policy,omitempty"`
	Trace            *DecisionTrace          `json:"trace,omitempty"`
	SourceSize       int64                   `json:"-"`
	SourceModifiedNS int64                   `json:"-"`
	Profile          string                  `json:"profile"`
	SourceID         string                  `json:"sourceId"`
	FactsRevision    int64                   `json:"factsRevision"`
	SourceContainer  string                  `json:"sourceContainer"`
	SourceVideoCodec string                  `json:"sourceVideoCodec"`
	SourceAudioCodec string                  `json:"sourceAudioCodec"`
	Duration         float64                 `json:"duration"`
	Mode             string                  `json:"mode"`
	Strategy         DeliveryStrategy        `json:"strategy"`
	Reason           string                  `json:"reason"`
	ReasonCodes      []string                `json:"reasonCodes"`
	OutputContainer  string                  `json:"outputContainer"`
	VideoAction      string                  `json:"videoAction"`
	AudioAction      string                  `json:"audioAction"`
	OutputVideoCodec string                  `json:"outputVideoCodec"`
	OutputAudioCodec string                  `json:"outputAudioCodec"`
	AudioChannels    int                     `json:"audioChannels"`
	QualityID        string                  `json:"qualityId"`
	TargetWidth      int                     `json:"targetDisplayWidth,omitempty"`
	TargetHeight     int                     `json:"targetDisplayHeight,omitempty"`
	VideoBitrateBPS  int                     `json:"videoBitrateBps,omitempty"`
	AudioBitrateBPS  int                     `json:"audioBitrateBps,omitempty"`
	ToneMap          bool                    `json:"toneMap"`
	ToneMapAlgorithm string                  `json:"toneMapAlgorithm,omitempty"`
	HardwareBackend  string                  `json:"hardwareBackend"`
	// AudioStream is the absolute source stream index the session plays, -1 when
	// the source has no audio or the plan predates track choice.
	AudioStream    int     `json:"audioStream"`
	Deinterlace    bool    `json:"deinterlace,omitempty"`
	MaxFrameRate   float64 `json:"maxFrameRate,omitempty"`
	Downmix        string  `json:"downmix,omitempty"`
	SoftwarePreset string  `json:"-"`
	HardwareDevice string  `json:"-"`
	HardwareNative bool    `json:"-"`
}

// DownmixITULimited names the one downmix the server performs: the ITU-R BS.775
// matrix, normalised so it cannot clip whatever sample format the decoder used,
// three decibels of make-up gain, and a limiter for the rare full-scale passage.
const DownmixITULimited = "itu_limited"

// DeliveryInput is everything planning needs about one source, one device and
// the viewer's resolved position. Planning is a pure function of it, which is
// what makes the decision matrix testable without a database.
type DeliveryInput struct {
	BurnIn bool
	Server *decoder.ToolchainFacts
	// Source and Client are the full facts. When Source is nil the flat fields
	// below describe a source whose detail was never observed, and a zero Client
	// is the built-in baseline; both are how older call sites and tests read.
	Source *DeliverySource
	Client ClientProfile

	SourceID       string
	Container      string
	VideoCodec     string
	AudioCodec     string
	Duration       float64
	Height         int
	ColorTransfer  string
	ColorPrimaries string
	Remote         bool

	// AudioStream is the viewer's explicit track (absolute stream index), or -1.
	// The zero value also means "no explicit choice" when HasAudioChoice is false.
	AudioStream             int
	HasAudioChoice          bool
	PreferredAudioLanguages []string
	// Excluded routes were reported broken by this device for this source.
	Excluded map[DeliveryStrategy]RouteRejection
	// FMP4Output says this server build can produce fragmented MP4 HLS.
	FMP4Output bool
	// NoHLS says this server has no converter (ffmpeg was not found), so only the
	// original route exists. The zero value keeps HLS available for callers and
	// tests that never think about it.
	NoHLS bool

	Policy             ResolvedDeliveryPolicy
	Target             QualityTarget
	Config             DeliveryConfiguration
	TranscodingEnabled bool
	Hardware           decoder.HardwareProbe
}

type routeCost int

const (
	costDirect routeCost = iota
	costRemux
	costAudioConversion
	costVideoConversion
)

type candidate struct {
	strategy  DeliveryStrategy
	cost      routeCost
	mode      string
	transport string
	codes     []string
	rank      int
}

func bitrateOver(actual int64, ceiling int) bool {
	// Ten percent of tolerance: an average rate is not a peak, and a file a hair
	// over a round-number ceiling is not worth a conversion.
	return ceiling > 0 && actual > 0 && actual > int64(ceiling)+int64(ceiling)/10
}

// planDelivery chooses one route for a source, for one device, under a resolved
// policy, and records why every cheaper route was not taken.
//
// The order of preference is the fewest conversions that satisfies the source,
// the device and the policy. Planning policy only breaks ties: maximum_fidelity
// and minimize_server_work both take the cheapest admissible route;
// maximum_compatibility deliberately takes the most converted admissible route,
// because the most normalized output is the most widely playable.
func planDelivery(in DeliveryInput) (p DeliveryPlan, err error) {
	cfg := in.Config.Normalized()
	source := legacySource(in)
	if in.Source != nil {
		source = *in.Source
	}
	client := in.Client
	if client.Version == 0 {
		client = BaselineClientProfile()
	}
	p = DeliveryPlan{
		Profile: DeliveryProfileLegacy, SourceID: source.ID, SourceContainer: source.Container, Duration: source.Duration,
		Mode: "direct", Strategy: DeliveryOriginal, Reason: ReasonDirectPlayCompatible,
		OutputContainer: source.Container, VideoAction: "original", AudioAction: "original",
		QualityID: in.Target.RungID, SoftwarePreset: cfg.SoftwarePreset,
		ToneMapAlgorithm: cfg.ToneMapAlgorithm, HardwareBackend: string(decoder.BackendSoftware), AudioStream: -1,
	}
	if p.QualityID == "" {
		p.QualityID = "auto"
	}
	defer func() {
		if err == nil {
			if p.Trace != nil && p.Trace.Video != nil && source.Video != nil {
				v := source.Video
				p.Trace.Video.OutputRange = effectiveRange(v, client.videoCodec(v.Codec))
				if p.VideoAction == "copy" && v.DolbyVisionProfile == 7 {
					p.Trace.Video.OutputRange = assets.RangeHDR10
				}
				if p.VideoAction == "convert" {
					p.Trace.Video.OutputRange = assets.RangeSDR
				}
			}
			p.Renditions = planAudioRenditions(p, source, client, in.Target, in.TranscodingEnabled && in.Policy.Transcode != "never")
		}
	}()
	policy := in.Policy
	p.Policy = &policy
	trace := &DecisionTrace{Version: 1, Client: client.Summary(), Container: source.Container, BitRate: source.BitRate, Routes: []RouteEvaluation{}, Notes: []string{}}
	p.Trace = trace
	if client.Evidence == EvidenceBuiltin {
		trace.Notes = append(trace.Notes, NoteBaselineProfile)
	}
	if in.Source != nil && !source.DetailKnown {
		trace.Notes = append(trace.Notes, NoteDetailUnobserved)
	}

	explicit := -1
	if in.HasAudioChoice {
		explicit = in.AudioStream
	}
	audio, chosenBy := chooseAudio(source.Audio, explicit, in.PreferredAudioLanguages)
	video := source.Video
	if video != nil {
		p.SourceVideoCodec, p.OutputVideoCodec = video.Codec, video.Codec
		trace.Video = &TraceVideo{Codec: video.Codec, Profile: video.Profile, Level: video.Level, BitDepth: video.BitDepth, Width: video.Width, Height: video.Height, FrameRate: video.FrameRate, Interlaced: video.Interlaced, DynamicRange: video.DynamicRange, DolbyVisionProfile: video.DolbyVisionProfile, DolbyVisionLevel: video.DolbyVisionLevel, DolbyVisionCompatibility: video.DolbyVisionCompatibility, BitRate: video.BitRate}
	}
	if audio != nil {
		p.SourceAudioCodec, p.OutputAudioCodec = audio.Codec, audio.Codec
		p.AudioStream = audio.Index
		trace.Audio = &TraceAudio{StreamIndex: audio.Index, Codec: audio.Codec, Channels: audio.Channels, Language: audio.Language, ObjectAudio: audio.ObjectAudio, ChosenBy: chosenBy, Tracks: len(source.Audio)}
	}
	if in.Remote && !in.BurnIn {
		p.Mode = "remote"
		p.Reason = ReasonRemotePreflightCompatible
		p.ReasonCodes = []string{ReasonRemotePreflightCompatible}
		p.OutputContainer = "mp4" // Remote admission uses InspectRemoteMP4.
		trace.Routes = append(trace.Routes, RouteEvaluation{Route: string(DeliveryOriginal), Transport: TransportDirect, Admissible: true, Chosen: true, Rejections: []RouteRejection{}})
		return p, nil
	}

	audioOnly := video == nil
	height := 0
	if video != nil {
		height = video.Height
	}
	hdr := video != nil && video.DynamicRange != "" && video.DynamicRange != assets.RangeSDR
	rungFixed := in.Target.Kind == QualityFixed
	overHeight := in.Target.MaxVideoHeight > 0 && height > in.Target.MaxVideoHeight
	ceiling := in.Target.MaxVideoBitrateBPS
	if client.MaxBitrateBPS > 0 && (ceiling == 0 || client.MaxBitrateBPS < ceiling) {
		ceiling = client.MaxBitrateBPS
		trace.Notes = append(trace.Notes, NoteDeviceBitrate)
	}
	overBitrate := bitrateOver(max(source.BitRate, source.MaxBitRate), ceiling)

	// Rules that close every route which leaves the picture as it is.
	var pictureRules []RouteRejection
	if in.BurnIn {
		pictureRules = append(pictureRules, RouteRejection{ReasonSubtitleBurnIn, ""})
	}
	if rungFixed {
		pictureRules = append(pictureRules, RouteRejection{ReasonQualityRungSelected, in.Target.RungID})
	}
	if overHeight {
		pictureRules = append(pictureRules, RouteRejection{ReasonHeightExceedsPolicy, fmt.Sprintf("%d>%d", height, in.Target.MaxVideoHeight)})
	}
	if overBitrate {
		pictureRules = append(pictureRules, RouteRejection{ReasonBitrateExceedsPolicy, fmt.Sprintf("%d>%d", source.BitRate, ceiling)})
	}

	candidates := []candidate{}
	evaluate := func(route DeliveryStrategy, transport string, rejections []RouteRejection) bool {
		if rejections == nil {
			rejections = []RouteRejection{}
		}
		if in.NoHLS {
			// The device may well take HLS; it is this server that cannot make it.
			for i := range rejections {
				if rejections[i].Code == RejectTransport {
					rejections[i] = RouteRejection{RejectServerOutput, "no converter on this server"}
				}
			}
		}
		trace.Routes = append(trace.Routes, RouteEvaluation{Route: string(route), Transport: transport, Admissible: len(rejections) == 0, Rejections: rejections})
		return len(rejections) == 0
	}
	excluded := func(route DeliveryStrategy) []RouteRejection {
		if r, ok := in.Excluded[route]; ok {
			return []RouteRejection{{RejectClientFailure, r.Code}}
		}
		return nil
	}

	// Route 1: the original file.
	{
		rejections := excluded(DeliveryOriginal)
		rejections = append(rejections, pictureRules...)
		if policy.DirectPlay == "never" {
			rejections = append(rejections, RouteRejection{ReasonDirectPlayRefused, ""})
		}
		var best []RouteRejection
		matched := false
		for _, t := range client.Transports {
			if t.Transport != TransportDirect || !containsString(t.Containers, source.Container) {
				continue
			}
			var r []RouteRejection
			if video != nil {
				r = append(r, videoRejections(video, client, t, in.Target.AllowHDR, false)...)
			}
			if audio != nil {
				r = append(r, audioRejections(audio, client, t)...)
				if audio.Ordinal != 0 && !client.EmbeddedAudioSwitching {
					r = append(r, RouteRejection{RejectAudioTrack, fmt.Sprintf("track %d", audio.Ordinal+1)})
				}
			}
			if !matched || len(r) < len(best) {
				best, matched = r, true
			}
			if len(r) == 0 {
				break
			}
		}
		if !matched {
			rejections = append(rejections, RouteRejection{RejectContainer, source.Container})
		} else {
			rejections = append(rejections, best...)
		}
		if evaluate(DeliveryOriginal, TransportDirect, rejections) {
			code := ReasonDirectPlayCompatible
			if audioOnly {
				code = ReasonDirectAudioCompatible
			}
			candidates = append(candidates, candidate{DeliveryOriginal, costDirect, "direct", TransportDirect, []string{code}, 0})
		} else if policy.DirectPlay == "require" {
			p.ReasonCodes = rejectionCodes(rejections)
			return p, ErrDeliveryRefused{Code: ReasonDirectPlayRequired}
		}
	}

	transports := hlsTransports(client, in.FMP4Output)
	if in.NoHLS {
		// This server has no converter at all: every HLS route closes with the
		// same reason instead of being planned and then failing to start.
		transports = nil
	}
	conversionAllowed := in.TranscodingEnabled && policy.Transcode != "never"

	// Route 2: both streams copied into HLS.
	remuxable := false
	if !audioOnly {
		rejections := excluded(DeliveryCopyRemux)
		rejections = append(rejections, pictureRules...)
		switch {
		case policy.DirectStream == "never" && !cfg.RemuxEnabled:
			rejections = append(rejections, RouteRejection{RejectOwnerRemuxDisabled, ""})
		case policy.DirectStream == "never":
			rejections = append(rejections, RouteRejection{ReasonDirectStreamRefused, ""})
		case !cfg.RemuxEnabled:
			rejections = append(rejections, RouteRejection{RejectOwnerRemuxDisabled, ""})
		}
		transport, r := bestCopyTransport(transports, client, video, audio, in.Target.AllowHDR, true)
		rejections = append(rejections, r...)
		// remuxable describes the source and the device, not the preferences:
		// it is what decides which refusal a closed policy is reported as.
		remuxable = len(r) == 0 && len(pictureRules) == 0
		if evaluate(DeliveryCopyRemux, transport, rejections) {
			candidates = append(candidates, candidate{DeliveryCopyRemux, costRemux, "hls", transport, []string{ReasonContainerRequiresRemux}, 0})
		} else if policy.DirectStream == "require" && len(candidates) == 0 {
			p.ReasonCodes = rejectionCodes(rejections)
			return p, ErrDeliveryRefused{Code: ReasonDirectStreamRequired}
		}
	}

	// Route 3: video copied, audio converted.
	if !audioOnly {
		rejections := excluded(DeliveryAudioConversion)
		rejections = append(rejections, serverConversionRejections(in.Server, in.Hardware, nil, audio, audioConversionTarget(audio, client, in.Target).Codec, false)...)
		rejections = append(rejections, pictureRules...)
		if !conversionAllowed {
			rejections = append(rejections, RouteRejection{RejectTranscodingDisabled, ""})
		}
		if audio == nil {
			rejections = append(rejections, RouteRejection{RejectNoAudio, ""})
		}
		transport, r := bestCopyTransport(transports, client, video, nil, in.Target.AllowHDR, true)
		rejections = append(rejections, r...)
		if evaluate(DeliveryAudioConversion, transport, rejections) {
			candidates = append(candidates, candidate{DeliveryAudioConversion, costAudioConversion, "hls", transport, []string{ReasonAudioCodecRequiresConvert}, 0})
		}
	}

	// Route 4: full conversion (or, for an audio file, audio conversion).
	{
		var rejections []RouteRejection
		audioToDecode := audio
		if t := transportNamed(transports, TransportHLSTS); audio != nil && t != nil && len(audioRejections(audio, client, *t)) == 0 && serverCopyAudio(TransportHLSTS, audio.Codec) && !bitrateOver(audio.BitRate, in.Target.MaxAudioBitrateBPS) && in.Source != nil && !audioOnly {
			audioToDecode = nil
		}
		hardware := in.Hardware
		if in.BurnIn {
			hardware.NativeFilters = false
		}
		rejections = append(rejections, serverConversionRejections(in.Server, hardware, video, audioToDecode, audioConversionTarget(audio, client, in.Target).Codec, hdr && cfg.ToneMapping)...)
		if !conversionAllowed {
			rejections = append(rejections, RouteRejection{RejectTranscodingDisabled, ""})
		}
		transport := ""
		for _, t := range transports {
			if t.Transport == TransportHLSTS && (audioOnly || containsString(t.Video, "h264")) && containsString(t.Audio, "aac") {
				transport = t.Transport
				break
			}
		}
		if transport == "" {
			rejections = append(rejections, RouteRejection{RejectTransport, TransportHLSTS})
		}
		route := DeliveryVideoConversion
		if audioOnly {
			route = DeliveryAudioConversion
		}
		if evaluate(route, transport, rejections) {
			// Every applicable reason is published, not only the first: a 4K HDR
			// HEVC title on a cellular lane is converted for several separate
			// reasons, and a client that shows one of them misexplains the picture.
			codes := []string{}
			if in.BurnIn {
				codes = append(codes, ReasonSubtitleBurnIn)
			}
			if audioOnly {
				codes = append(codes, ReasonAudioCodecRequiresConvert)
			} else {
				copyRoute := routeEvaluation(trace, DeliveryAudioConversion)
				if copyRoute != nil && hasRejection(copyRoute.Rejections, RejectVideoCodec, RejectVideoProfile, RejectVideoLevel, RejectVideoBitDepth, RejectResolution, RejectFrameRate, RejectVideoBitrate, RejectInterlaced, RejectRotation, RejectDynamicRange, RejectDolbyVision) {
					codes = append(codes, ReasonVideoCodecRequiresConvert)
				}
				if hdr && (!in.Target.AllowHDR || (copyRoute != nil && hasRejection(copyRoute.Rejections, ReasonHDRNotAllowed))) {
					codes = append(codes, ReasonHDRNotAllowed)
				}
			}
			if overHeight {
				codes = append(codes, ReasonHeightExceedsPolicy)
			}
			if rungFixed {
				codes = append(codes, ReasonQualityRungSelected)
			}
			if overBitrate || (rungFixed && in.Target.MaxVideoBitrateBPS > 0) {
				codes = append(codes, ReasonBitrateExceedsPolicy)
			}
			if len(codes) == 0 {
				codes = append(codes, ReasonVideoCodecRequiresConvert)
			}
			candidates = append(candidates, candidate{route, costVideoConversion, "hls", transport, codes, 0})
		}
	}

	// The audio-track rule is a preference, not an impossibility: the original
	// still plays, only with its first track. When nothing else is admissible
	// (no converter on this server, conversion turned off) that beats refusing
	// the title, and the trace says which track the viewer is actually hearing.
	if len(candidates) == 0 {
		if original := routeEvaluation(trace, DeliveryOriginal); original != nil && len(original.Rejections) == 1 && original.Rejections[0].Code == RejectAudioTrack && len(source.Audio) > 0 {
			original.Admissible = true
			trace.Notes = append(trace.Notes, NoteFirstAudioTrackPlayed)
			audio = &source.Audio[0]
			p.SourceAudioCodec, p.OutputAudioCodec, p.AudioStream = audio.Codec, audio.Codec, audio.Index
			trace.Audio.StreamIndex, trace.Audio.Codec, trace.Audio.Channels, trace.Audio.Language, trace.Audio.ChosenBy = audio.Index, audio.Codec, audio.Channels, audio.Language, "only_reachable_track"
			code := ReasonDirectPlayCompatible
			candidates = append(candidates, candidate{DeliveryOriginal, costDirect, "direct", TransportDirect, []string{code}, 0})
		}
	}
	if len(candidates) == 0 {
		code := ReasonTranscodeRefused
		if !in.TranscodingEnabled {
			code = ReasonTranscodeDisabledOwner
		} else if policy.DirectStream == "never" && remuxable {
			code = ReasonDirectStreamRefused
		} else if policy.Transcode != "never" {
			code = ReasonNoAdmissibleRoute
		}
		return p, ErrDeliveryRefused{Code: code}
	}

	// Ranking. Explicit preferences outrank the planning policy, because a
	// viewer's "prefer" is a statement about this device, not about this server.
	for i := range candidates {
		c := &candidates[i]
		switch {
		case policy.DirectPlay == "prefer" && c.cost == costDirect:
			c.rank, c.codes = -1, append(c.codes, ReasonDirectPlayPreferred)
		case policy.DirectStream == "prefer" && c.cost == costRemux:
			c.rank, c.codes = -1, append(c.codes, ReasonDirectStreamPreferred)
		case (policy.Transcode == "prefer" || policy.Transcode == "require") && c.cost == costVideoConversion:
			c.rank, c.codes = -1, append(c.codes, ReasonTranscodePreferred)
		}
	}
	planning := cfg.PlanningPolicy
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		if planning == PlanningMaximumCompatible {
			return candidates[i].cost > candidates[j].cost
		}
		return candidates[i].cost < candidates[j].cost
	})
	chosen := candidates[0]
	switch planning {
	case PlanningMaximumCompatible:
		chosen.codes = append(chosen.codes, ReasonPlanningCompatibility)
	case PlanningMinimizeServerWork:
		chosen.codes = append(chosen.codes, ReasonPlanningMinimizeWork)
	default:
		chosen.codes = append(chosen.codes, ReasonPlanningMaximumFidelity)
	}
	// A refused preference is part of the explanation even when another route
	// was admissible.
	if policy.DirectPlay == "never" {
		p.ReasonCodes = append(p.ReasonCodes, ReasonDirectPlayRefused)
	}
	if !audioOnly && (policy.DirectStream == "never" || !cfg.RemuxEnabled) && remuxable {
		p.ReasonCodes = append(p.ReasonCodes, ReasonDirectStreamRefused)
	}
	p.ReasonCodes = append(p.ReasonCodes, chosen.codes...)
	p.Mode, p.Strategy, p.Reason = chosen.mode, chosen.strategy, chosen.codes[0]
	if p.Mode == "hls" && p.Duration > 0 && !hlsDurationSupported(p.Duration) {
		return p, ErrDeliveryRefused{Code: "hls_duration_unsupported"}
	}
	for i := range trace.Routes {
		if trace.Routes[i].Route == string(chosen.strategy) && trace.Routes[i].Admissible {
			trace.Routes[i].Chosen = true
			break
		}
	}

	output := "mpegts_hls"
	if chosen.transport == TransportHLSFMP4 {
		output = "fmp4_hls"
	}
	convertAudio := func() {
		target := audioConversionTarget(audio, client, in.Target)
		p.AudioAction, p.OutputAudioCodec, p.AudioChannels, p.AudioBitrateBPS = "convert", target.Codec, target.Channels, target.Bitrate
		if target.Downmix {
			p.Downmix = DownmixITULimited
		}
	}
	switch chosen.strategy {
	case DeliveryOriginal:
		return p, nil
	case DeliveryCopyRemux:
		p.OutputContainer = output
		p.VideoAction, p.AudioAction = "copy", "copy"
		if audio == nil {
			p.AudioAction = "none"
		}
		p.AudioChannels = 0
		return p, nil
	case DeliveryAudioConversion:
		p.OutputContainer = output
		convertAudio()
		if audioOnly {
			p.VideoAction, p.OutputVideoCodec = "none", ""
		} else {
			p.VideoAction = "copy"
		}
		return p, nil
	}
	p.OutputContainer = output
	p.VideoAction, p.OutputVideoCodec = "convert", "h264"
	// The audio of a converted picture is still copied when the device takes it:
	// re-encoding a track the device can already decode only loses quality.
	if audio == nil {
		p.AudioAction, p.OutputAudioCodec = "none", ""
	} else if t := transportNamed(transports, chosen.transport); t != nil && len(audioRejections(audio, client, *t)) == 0 && serverCopyAudio(chosen.transport, audio.Codec) && !bitrateOver(audio.BitRate, in.Target.MaxAudioBitrateBPS) && in.Source != nil {
		p.AudioAction = "copy"
	} else {
		convertAudio()
	}
	h264 := client.videoCodec("h264")
	p.TargetHeight = in.Target.MaxVideoHeight
	if h264 != nil {
		p.TargetWidth = h264.MaxWidth
	}
	if h264 != nil && h264.MaxHeight > 0 && (p.TargetHeight == 0 || h264.MaxHeight < p.TargetHeight) && height > h264.MaxHeight {
		p.TargetHeight = h264.MaxHeight
	}
	budgetTarget := in.Target
	budgetTarget.MaxVideoBitrateBPS = ceiling
	if p.TargetHeight > 0 {
		budgetTarget.MaxVideoHeight = p.TargetHeight
	}
	p.VideoBitrateBPS = videoBudget(budgetTarget, height)
	// The output of a conversion is 8-bit BT.709. An HDR picture that is not tone
	// mapped on the way there is simply wrong — grey and washed out — so the
	// owner's switch is the only thing that turns it off, and the trace says so.
	if hdr {
		if cfg.ToneMapping {
			p.ToneMap = true
			if video.DynamicRange == assets.RangeDolbyVision && dolbyVisionFallback(video) == "" {
				trace.Notes = append(trace.Notes, NoteDolbyVisionNoLayer)
				// COMPAT-04: only libplacebo applies Profile 5's reshaping; say so
				// in the decision, so clients can warn before play.
				if !decoder.ReshapesDolbyVision(decoder.ConversionRequest{ConvertVideo: true, ToneMap: true, Toolchain: decoder.CurrentToolchain(), Probe: in.Hardware}) {
					p.ReasonCodes = append(p.ReasonCodes, ReasonDolbyVisionApproximate)
				}
			}
		} else {
			trace.Notes = append(trace.Notes, NoteToneMapOwnerOff)
		}
	}
	if video != nil {
		p.Deinterlace = video.Interlaced
		limit := 60.0
		if h264 != nil && h264.MaxFrameRate > 0 {
			limit = h264.MaxFrameRate
		}
		if video.FrameRate > limit+0.01 {
			p.MaxFrameRate = limit
		}
	}
	p.HardwareDevice = cfg.HardwareDevice
	if in.Hardware.Available && in.Hardware.Backend != decoder.BackendSoftware {
		p.HardwareBackend, p.HardwareNative = string(in.Hardware.Backend), in.Hardware.NativeFilters
		p.ReasonCodes = append(p.ReasonCodes, ReasonHardwareBackend)
	} else {
		p.ReasonCodes = append(p.ReasonCodes, ReasonSoftwareFallback)
	}
	return p, nil
}

func transportNamed(transports []ClientTransport, name string) *ClientTransport {
	for i := range transports {
		if transports[i].Transport == name {
			return &transports[i]
		}
	}
	return nil
}

func routeEvaluation(t *DecisionTrace, route DeliveryStrategy) *RouteEvaluation {
	for i := range t.Routes {
		if t.Routes[i].Route == string(route) {
			return &t.Routes[i]
		}
	}
	return nil
}

func hasRejection(rejections []RouteRejection, codes ...string) bool {
	for _, r := range rejections {
		for _, c := range codes {
			if r.Code == c {
				return true
			}
		}
	}
	return false
}

func rejectionCodes(rejections []RouteRejection) []string {
	out := []string{}
	for _, r := range rejections {
		if !containsString(out, r.Code) {
			out = append(out, r.Code)
		}
	}
	return out
}

// bestCopyTransport finds the HLS transport that can carry the video (and, when
// audio is given, that audio track) untouched, both for the device and for this
// server's producers. It returns the transport with the fewest objections, so
// the trace names the nearest miss rather than an arbitrary one.
func bestCopyTransport(transports []ClientTransport, client ClientProfile, video *SourceVideo, audio *SourceAudio, allowHDR bool, copying bool) (string, []RouteRejection) {
	if len(transports) == 0 {
		return "", []RouteRejection{{RejectTransport, "hls"}}
	}
	name, best, found := "", []RouteRejection(nil), false
	for _, t := range transports {
		var r []RouteRejection
		if video != nil {
			r = append(r, videoRejections(video, client, t, allowHDR, copying)...)
			if !serverCopyVideo(t.Transport, video.Codec) && !hasRejection(r, RejectVideoCodec) {
				r = append(r, RouteRejection{RejectServerOutput, video.Codec + " in " + t.Transport})
			}
		}
		if audio != nil {
			r = append(r, audioRejections(audio, client, t)...)
			if !serverCopyAudio(t.Transport, audio.Codec) && !hasRejection(r, RejectAudioCodec) {
				r = append(r, RouteRejection{RejectServerOutput, audio.Codec + " in " + t.Transport})
			}
		}
		if !found || len(r) < len(best) {
			name, best, found = t.Transport, r, true
		}
		if len(r) == 0 {
			break
		}
	}
	return name, best
}

// videoBudget is the encoder's bitrate ceiling. A rung names its own; automatic
// takes the policy ceiling, and failing that the ladder step for the height.
func videoBudget(t QualityTarget, sourceHeight int) int {
	if t.MaxVideoBitrateBPS > 0 {
		return t.MaxVideoBitrateBPS
	}
	height := t.MaxVideoHeight
	if height <= 0 {
		height = sourceHeight
	}
	if height <= 0 {
		// An unmeasured source is planned as 1080p rather than as the smallest
		// rung: guessing small would quietly degrade every unprobed title.
		return 8_000_000
	}
	for i := len(QualityLadder) - 1; i >= 0; i-- {
		if QualityLadder[i].Height >= height {
			return QualityLadder[i].VideoBitrateBPS
		}
	}
	return QualityLadder[0].VideoBitrateBPS
}

func audioBudget(t QualityTarget) int {
	if t.MaxAudioBitrateBPS > 0 {
		return t.MaxAudioBitrateBPS
	}
	return 192_000
}

// CodecArgs is consumed by every HLS producer.
func (p DeliveryPlan) CodecArgs() ([]string, []string, []decoder.HardwareStage, error) {
	graph, err := p.conversionGraph(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	return graph.Input, append(append([]string{}, graph.Video...), graph.Audio...), graph.Stages, nil
}

func (p DeliveryPlan) conversionGraph(burn *decoder.BurnIn) (decoder.ConversionGraph, error) {
	if p.Profile != DeliveryProfileLegacy || p.Mode != "hls" {
		return decoder.ConversionGraph{}, errors.New("delivery plan does not describe ordinary HLS")
	}
	request := decoder.ConversionRequest{
		BurnIn:           burn,
		Toolchain:        decoder.CurrentToolchain(),
		Probe:            decoder.HardwareProbe{Backend: decoder.HardwareBackend(p.HardwareBackend), Available: p.HardwareBackend != "" && p.HardwareBackend != string(decoder.BackendSoftware), NativeFilters: p.HardwareNative, Device: p.HardwareDevice},
		ToneMap:          p.ToneMap,
		ToneMapAlgorithm: p.ToneMapAlgorithm,
		MaxHeight:        p.TargetHeight,
		MaxWidth:         p.TargetWidth,
		VideoBitrateBPS:  p.VideoBitrateBPS,
		AudioBitrateBPS:  p.AudioBitrateBPS,
		AudioChannels:    p.AudioChannels,
		AudioCodec:       p.OutputAudioCodec,
		SoftwarePreset:   p.SoftwarePreset,
		Deinterlace:      p.Deinterlace,
		MaxFrameRate:     p.MaxFrameRate,
		Downmix:          p.Downmix == DownmixITULimited,
	}
	switch p.VideoAction {
	case "copy":
		request.CopyVideo = true
	case "convert":
		request.ConvertVideo = true
	case "none":
	default:
		return decoder.ConversionGraph{}, errors.New("invalid delivery video action")
	}
	switch p.AudioAction {
	case "copy":
		request.CopyAudio = true
	case "convert":
		if (p.OutputAudioCodec != "aac" && p.OutputAudioCodec != "ac3" && p.OutputAudioCodec != "eac3") || (p.AudioChannels != 2 && p.AudioChannels != 6) {
			return decoder.ConversionGraph{}, errors.New("invalid delivery audio action")
		}
		request.ConvertAudio = true
	case "none":
	default:
		return decoder.ConversionGraph{}, errors.New("invalid delivery audio action")
	}
	return decoder.BuildConversion(request)
}

// Legacy sessions without a snapshot return nil; callers must choose an explicit
// legacy compatibility path rather than invent a new immutable decision.
func loadDeliveryPlan(q audioQuery, session string) (*DeliveryPlan, error) {
	p := &DeliveryPlan{}
	var codes, policy, trace, renditions, burn string
	err := q.QueryRow(`SELECT profile,source_id,source_size,source_modified_ns,facts_revision,source_container,source_video,source_audio,duration,mode,strategy,reason,output_container,video_action,audio_action,output_video,output_audio,audio_channels,reason_codes,quality_id,target_height,target_width,video_bitrate,audio_bitrate,tone_map,tone_map_algorithm,hardware_backend,hardware_device,hardware_native,software_preset,policy_json,trace_json,audio_stream,deinterlace,max_frame_rate,downmix,renditions_json,burn_in_json FROM playback_delivery_plans WHERE session_id=?`, session).Scan(&p.Profile, &p.SourceID, &p.SourceSize, &p.SourceModifiedNS, &p.FactsRevision, &p.SourceContainer, &p.SourceVideoCodec, &p.SourceAudioCodec, &p.Duration, &p.Mode, &p.Strategy, &p.Reason, &p.OutputContainer, &p.VideoAction, &p.AudioAction, &p.OutputVideoCodec, &p.OutputAudioCodec, &p.AudioChannels, &codes, &p.QualityID, &p.TargetHeight, &p.TargetWidth, &p.VideoBitrateBPS, &p.AudioBitrateBPS, &p.ToneMap, &p.ToneMapAlgorithm, &p.HardwareBackend, &p.HardwareDevice, &p.HardwareNative, &p.SoftwarePreset, &policy, &trace, &p.AudioStream, &p.Deinterlace, &p.MaxFrameRate, &p.Downmix, &renditions, &burn)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if burn != "" {
		if err = json.Unmarshal([]byte(burn), &p.BurnIn); err != nil {
			return nil, err
		}
	}
	if renditions != "" {
		if err = json.Unmarshal([]byte(renditions), &p.Renditions); err != nil || len(p.Renditions) > 16 {
			return nil, errors.New("invalid stored audio renditions")
		}
	}
	p.ReasonCodes = []string{}
	if codes != "" {
		p.ReasonCodes = strings.Split(codes, ",")
	}
	if policy != "" {
		var resolved ResolvedDeliveryPolicy
		if err = json.Unmarshal([]byte(policy), &resolved); err == nil {
			p.Policy = &resolved
		}
	}
	if trace != "" {
		var decoded DecisionTrace
		if err = json.Unmarshal([]byte(trace), &decoded); err == nil {
			p.Trace = &decoded
		}
	}
	var valid bool
	err = q.QueryRow(`SELECT EXISTS(SELECT 1 FROM playback_source_pins pin JOIN playback_sessions s ON s.id=pin.session_id JOIN catalog_assets a ON a.token=pin.asset_id WHERE s.id=? AND s.asset_id=? AND s.mode=? AND pin.asset_id=s.asset_id AND (EXISTS(SELECT 1 FROM prepared_media_session_pins pp WHERE pp.session_id=s.id) OR (a.size=pin.size AND a.modified_ns=pin.modified_ns)) AND pin.size=? AND pin.modified_ns=?)`, session, p.SourceID, p.Mode, p.SourceSize, p.SourceModifiedNS).Scan(&valid)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrStaleOffer
	}
	var prepared PreparedDelivery
	var raw string
	err = q.QueryRow(`SELECT v.id,v.revision,pin.digest,pin.size,v.source_revision,v.facts_json FROM prepared_media_session_pins pin JOIN prepared_media_versions v ON v.id=pin.version_id WHERE pin.session_id=? AND v.state IN('published','deleting')`, session).Scan(&prepared.VersionID, &prepared.Revision, &prepared.Digest, &prepared.Size, &prepared.SourceRevision, &raw)
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &prepared.Facts); err != nil {
			return nil, err
		}
		p.Prepared = &prepared
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return p, nil
}

func persistDeliveryPlan(tx *sql.Tx, session string, p *DeliveryPlan) error {
	policy := ""
	if p.Policy != nil {
		raw, err := json.Marshal(p.Policy)
		if err != nil {
			return err
		}
		policy = string(raw)
	}
	trace := ""
	if p.Trace != nil {
		if raw, err := json.Marshal(p.Trace); err == nil && len(raw) <= 32<<10 {
			trace = string(raw)
		}
	}
	burn, err := json.Marshal(p.BurnIn)
	if err != nil {
		return err
	}
	renditions, err := json.Marshal(p.Renditions)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO playback_delivery_plans(session_id,profile,source_id,source_size,source_modified_ns,facts_revision,source_container,source_video,source_audio,duration,mode,strategy,reason,output_container,video_action,audio_action,output_video,output_audio,audio_channels,reason_codes,quality_id,target_height,target_width,video_bitrate,audio_bitrate,tone_map,tone_map_algorithm,hardware_backend,hardware_device,hardware_native,software_preset,policy_json,trace_json,audio_stream,deinterlace,max_frame_rate,downmix,renditions_json,burn_in_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session, p.Profile, p.SourceID, p.SourceSize, p.SourceModifiedNS, p.FactsRevision, p.SourceContainer, p.SourceVideoCodec, p.SourceAudioCodec, p.Duration, p.Mode, p.Strategy, p.Reason, p.OutputContainer, p.VideoAction, p.AudioAction, p.OutputVideoCodec, p.OutputAudioCodec, p.AudioChannels,
		strings.Join(p.ReasonCodes, ","), p.QualityID, p.TargetHeight, p.TargetWidth, p.VideoBitrateBPS, p.AudioBitrateBPS, p.ToneMap, p.ToneMapAlgorithm, p.HardwareBackend, p.HardwareDevice, p.HardwareNative, p.SoftwarePreset, policy, trace, p.AudioStream, p.Deinterlace, p.MaxFrameRate, p.Downmix, string(renditions), string(burn))
	return err
}

// Prepared names actual delivery bytes separately from the original source pin.
type PreparedDelivery struct {
	VersionID      string              `json:"versionId"`
	Revision       int64               `json:"revision"`
	Digest         string              `json:"digest"`
	Size           int64               `json:"size"`
	SourceRevision string              `json:"sourceRevision"`
	Facts          preparedmedia.Facts `json:"facts"`
}

func serverConversionRejections(f *decoder.ToolchainFacts, hardware decoder.HardwareProbe, video *SourceVideo, audio *SourceAudio, audioEncoder string, toneMap bool) []RouteRejection {
	if f == nil {
		return nil
	}
	out := []RouteRejection{}
	if video != nil {
		encoder := "libx264"
		if hardware.Available && hardware.Encoder != "" {
			encoder = hardware.Encoder
		}
		if !f.Encoders[encoder] {
			out = append(out, RouteRejection{RejectServerOutput, "H.264 encoder unavailable"})
		}
	}
	for _, codec := range []string{func() string {
		if video != nil {
			return video.Codec
		}
		return ""
	}(), func() string {
		if audio != nil {
			return audio.Codec
		}
		return ""
	}()} {
		if codec == "" {
			continue
		}
		if codec == "dts" {
			codec = "dca"
		}
		if !f.Decoders[codec] {
			out = append(out, RouteRejection{"source_codec_not_decodable", codec})
		}
	}
	if audio != nil && !f.Encoders[audioEncoder] {
		out = append(out, RouteRejection{RejectServerOutput, audioEncoder + " encoder unavailable"})
	}
	_, toneErr := decoder.BuildConversion(decoder.ConversionRequest{ConvertVideo: video != nil, ToneMap: toneMap, Probe: hardware, Toolchain: f})
	if toneMap && toneErr != nil {
		out = append(out, RouteRejection{"hdr_tone_mapping_filter_unavailable", ""})
	}
	return out
}
