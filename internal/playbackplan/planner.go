package playbackplan

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

type candidate struct {
	tuple    playbackcap.DeliveryTuple
	plan     Plan
	cost     int
	fidelity int
}

func Build(req Request) (Plan, error) {
	facts, err := req.Facts.Canonical()
	if err != nil {
		return unsupportedPlan(req, ReasonInvalidInput), fmt.Errorf("canonical media facts: %w", err)
	}
	if req.Policy == "" {
		req.Policy = MaximumFidelity
	}
	if req.Policy != MaximumFidelity && req.Policy != MaximumCompatibility && req.Policy != MinimizeServerWork {
		return unsupportedPlan(req, ReasonInvalidInput), fmt.Errorf("invalid owner policy")
	}
	allowed, preferred, modeErr := normalizeModePolicy(req.AllowedModes, req.PreferredModes)
	if modeErr != nil {
		return unsupportedPlan(req, ReasonInvalidInput), modeErr
	}
	v, a, s, kind, err := selectStreams(facts, req.Selection)
	if err != nil {
		return unsupportedPlan(req, ReasonInvalidInput), err
	}
	var candidates []candidate
	var dvRejected, toneMapRejected bool
	tuples := append([]playbackcap.DeliveryTuple(nil), req.Capabilities.Tuples...)
	sort.SliceStable(tuples, func(i, j int) bool { return tupleKey(tuples[i]) < tupleKey(tuples[j]) })
	for _, t := range tuples {
		if t.Kind != kind || (req.Protocol != "" && token(t.Protocol) != token(req.Protocol)) {
			continue
		}
		p, ok, dvr, tmr := makeCandidate(req, facts, v, a, s, t)
		dvRejected = dvRejected || dvr
		toneMapRejected = toneMapRejected || tmr
		if ok && (len(allowed) == 0 || allowed[p.plan.Mode]) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		r := ReasonNoCompatibleTuple
		if dvRejected {
			r = ReasonDVUnsupported
		} else if toneMapRejected {
			r = ReasonHDRToneMapDisabled
		}
		return unsupportedPlan(req, r), nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if rankA, rankB := preferred[a.plan.Mode], preferred[b.plan.Mode]; rankA != rankB {
			return rankA < rankB
		}
		switch req.Policy {
		case MinimizeServerWork:
			if a.cost != b.cost {
				return a.cost < b.cost
			}
		case MaximumCompatibility:
			if a.fidelity != b.fidelity {
				return a.fidelity < b.fidelity
			}
		default:
			if a.fidelity != b.fidelity {
				return a.fidelity > b.fidelity
			}
		}
		if a.cost != b.cost {
			return a.cost < b.cost
		}
		return tupleKey(a.tuple) < tupleKey(b.tuple)
	})
	p := candidates[0].plan
	p.Reasons = canonicalReasons(p.Reasons)
	p.Digest, _ = p.ComputeDigest()
	return p, nil
}

func normalizeModePolicy(allowedModes, preferredModes []Mode) (map[Mode]bool, map[Mode]int, error) {
	valid := map[Mode]bool{DirectPlay: true, Remux: true, DirectStream: true, VideoTranscode: true}
	allowed := map[Mode]bool{}
	for _, mode := range allowedModes {
		if !valid[mode] {
			return nil, nil, fmt.Errorf("invalid allowed mode")
		}
		allowed[mode] = true
	}
	preferred := map[Mode]int{}
	for index, mode := range preferredModes {
		if !valid[mode] {
			return nil, nil, fmt.Errorf("invalid preferred mode")
		}
		if _, exists := preferred[mode]; !exists {
			preferred[mode] = index
		}
	}
	defaultRank := len(preferredModes) + 1
	for mode := range valid {
		if _, exists := preferred[mode]; !exists {
			preferred[mode] = defaultRank
		}
	}
	return allowed, preferred, nil
}

func selectStreams(f mediafacts.Facts, sel Selection) (*mediafacts.Video, *mediafacts.Audio, *mediafacts.Subtitle, playbackcap.MediaKind, error) {
	findV := func() *mediafacts.Video {
		for i := range f.Video {
			if sel.VideoIndex != nil && f.Video[i].Index == *sel.VideoIndex {
				return &f.Video[i]
			}
		}
		if sel.VideoIndex == nil && len(f.Video) > 0 {
			return &f.Video[0]
		}
		return nil
	}
	findA := func() *mediafacts.Audio {
		for i := range f.Audio {
			if sel.AudioIndex != nil && f.Audio[i].Index == *sel.AudioIndex {
				return &f.Audio[i]
			}
		}
		if sel.AudioIndex == nil && len(f.Audio) > 0 {
			return &f.Audio[0]
		}
		return nil
	}
	findS := func() *mediafacts.Subtitle {
		if sel.SubtitleIndex == nil {
			return nil
		}
		for i := range f.Subtitles {
			if f.Subtitles[i].Index == *sel.SubtitleIndex {
				return &f.Subtitles[i]
			}
		}
		return nil
	}
	v, a, s := findV(), findA(), findS()
	if a == nil {
		return nil, nil, nil, "", fmt.Errorf("selected audio stream not found")
	}
	if sel.VideoIndex != nil && v == nil {
		return nil, nil, nil, "", fmt.Errorf("selected video stream not found")
	}
	if sel.SubtitleIndex != nil && s == nil {
		return nil, nil, nil, "", fmt.Errorf("selected subtitle stream not found")
	}
	kind := playbackcap.MediaAudio
	if v != nil {
		kind = playbackcap.MediaAudiovisual
	} else if s != nil {
		return nil, nil, nil, "", fmt.Errorf("audio-only playback cannot select subtitles")
	}
	return v, a, s, kind, nil
}

func makeCandidate(req Request, f mediafacts.Facts, v *mediafacts.Video, a *mediafacts.Audio, s *mediafacts.Subtitle, t playbackcap.DeliveryTuple) (candidate, bool, bool, bool) {
	p := Plan{SchemaRevision: SchemaRevision, SourceFingerprint: f.Source.Fingerprint, SourceRevision: f.Source.Revision, CapabilityEvidenceID: req.Capabilities.EvidenceID, Policy: req.Policy, MediaKind: t.Kind, Protocol: token(t.Protocol), Container: token(t.Container), Selection: req.Selection, Constraints: req.Constraints, Hardware: cleanHardware(req.Hardware), Timeline: Timeline{Mode: "vod", DurationUS: f.DurationUS, Generation: 1}}
	if f.DurationUS == 0 || f.DurationConfidence == mediafacts.ConfidenceUnknown {
		p.Timeline.Mode = "event"
		p.Timeline.Dynamic = true
	}
	p.SegmentFormat = segmentFormat(p.Protocol, p.Container)
	containerCopy := token(f.Container) == p.Container
	videoCopy := v == nil || videoMatches(*v, t.Video)
	videoConstraintExceeded := v != nil && ((req.Constraints.MaxVideoBitrate > 0 && (v.Bitrate <= 0 || v.Bitrate > req.Constraints.MaxVideoBitrate)) ||
		(req.Constraints.MaxWidth > 0 && v.CodedWidth > req.Constraints.MaxWidth) ||
		(req.Constraints.MaxHeight > 0 && v.CodedHeight > req.Constraints.MaxHeight))
	if videoConstraintExceeded {
		videoCopy = false
	}
	seekForcedEncode := v != nil && videoCopy && token(t.Protocol) == "hls" && !verifiedExactSeek(*v, f.Source.Revision)
	if seekForcedEncode {
		videoCopy = false
	}
	audioLayout, audioChannels, audioCopy, audioOK := audioRoute(*a, t.Audio)
	if !audioOK {
		return candidate{}, false, false, false
	}
	audioConstraintExceeded := req.Constraints.MaxAudioBitrate > 0 && (a.Bitrate <= 0 || a.Bitrate > req.Constraints.MaxAudioBitrate)
	if audioConstraintExceeded {
		audioCopy = false
	}
	color, valid, dvr, tmr := decideColor(v, t.Video, videoCopy, req.DisableToneMapping, req.ToneMapAlgorithm)
	if !valid {
		return candidate{}, false, dvr, tmr
	}
	p.Color = color
	if color != nil {
		switch color.Action {
		case "preserve":
			if color.Input == "dolby_vision" {
				p.Reasons = append(p.Reasons, ReasonDVPreserved)
			} else if color.Input != "sdr" {
				p.Reasons = append(p.Reasons, ReasonHDRPreserved)
			}
		case "use_verified_base":
			p.Reasons = append(p.Reasons, ReasonDVVerifiedBase)
		case "tone_map_sdr":
			p.Reasons = append(p.Reasons, ReasonHDRToneMapped)
		case "downgrade_hdr10plus":
			p.Reasons = append(p.Reasons, ReasonHDR10PlusDowngraded)
		}
	}
	if v != nil {
		act := Copy
		if !videoCopy {
			act = Convert
		}
		p.Streams = append(p.Streams, StreamAction{v.Index, "video", act, v.Codec, t.Video.Codec, "", ""})
	}
	if seekForcedEncode {
		p.Reasons = append(p.Reasons, ReasonExactSeekUnavailable)
	}
	if videoConstraintExceeded {
		p.Reasons = append(p.Reasons, ReasonVideoConstraint)
	}
	action := Copy
	if !audioCopy {
		action = Convert
	}
	if audioConstraintExceeded {
		p.Reasons = append(p.Reasons, ReasonAudioConstraint)
	}
	p.Streams = append(p.Streams, StreamAction{a.Index, "audio", action, a.Codec, t.Audio.Codec, a.Layout, audioLayout})
	p.Audio = AudioDecision{Codec: token(t.Audio.Codec), Layout: audioLayout, Channels: audioChannels, Passthrough: audioCopy, ObjectsPreserved: a.ObjectAudio == "" || (audioCopy && t.Audio.ObjectPassthrough)}
	timingCopy := audioCopy && containerCopy && p.Protocol == "http"
	p.Audio.Gapless = decideGapless(*a, timingCopy)
	switch p.Audio.Gapless.Status {
	case "preserved":
		p.Reasons = append(p.Reasons, ReasonGaplessPreserved)
	case "unverified":
		p.Reasons = append(p.Reasons, ReasonGaplessUnverified)
	case "unknown":
		p.Reasons = append(p.Reasons, ReasonGaplessFactsUnknown)
	}
	if p.Audio.Channels < a.Channels {
		p.Audio.Downmixed = true
		p.Reasons = append(p.Reasons, ReasonAudioLayoutReduced)
	}
	if a.ObjectAudio != "" && !p.Audio.ObjectsPreserved {
		p.Reasons = append(p.Reasons, ReasonObjectAudioLost)
	}
	subOK := applySubtitle(&p, s, t.Subtitle)
	if !subOK {
		return candidate{}, false, false, false
	}
	filters := s != nil && p.Subtitle.Action == BurnIn || (color != nil && color.Action != "preserve") || (v != nil && (v.Rotation != 0 || token(v.FieldOrder) != "" && token(v.FieldOrder) != "progressive"))
	// A progressive source is not already an HLS/DASH presentation merely
	// because its elementary streams and ISO-BMFF container are compatible.
	// Non-HTTP delivery still requires an explicit packaging/remux stage.
	packagingCopy := p.Protocol == "http"
	if videoCopy && audioCopy && containerCopy && packagingCopy && !filters {
		p.Mode = DirectPlay
		p.Reasons = append(p.Reasons, ReasonExactTuple)
	} else if videoCopy && audioCopy && !filters {
		p.Mode = Remux
		p.Reasons = append(p.Reasons, ReasonContainerChange)
	} else if videoCopy && !audioCopy && !filters {
		p.Mode = DirectStream
		p.Reasons = append(p.Reasons, ReasonAudioConversion)
	} else {
		p.Mode = VideoTranscode
		p.Reasons = append(p.Reasons, ReasonVideoConversion)
		if req.Hardware.Backend != "" && !req.Hardware.Verified {
			return candidate{}, false, false, false
		}
	}
	p.Stages = graph(p, v, a, s)
	cost := map[Mode]int{DirectPlay: 0, Remux: 1, DirectStream: 2, VideoTranscode: 3}[p.Mode]
	fidelity := 100 - cost*10 - p.Audio.DownmixPenalty() + audioFidelityScore(*a, p.Audio)
	if color != nil && color.Action != "preserve" {
		fidelity -= 20
	}
	return candidate{t, p, cost, fidelity}, true, false, false
}

func decideGapless(a mediafacts.Audio, timingCopy bool) GaplessDecision {
	d := GaplessDecision{
		SourceConfidence: a.GaplessConfidence, SourceEvidence: a.GaplessEvidence,
		EncoderDelaySamples: a.EncoderDelaySamples, EncoderPaddingSamples: a.EncoderPaddingSamples,
		SampleRate: a.SampleRate, StartTime: a.Timing.StartTime, Duration: a.Timing.Duration, TimeBase: a.Timing.TimeBase,
	}
	if a.GaplessConfidence != mediafacts.ConfidenceExact || a.SampleRate <= 0 {
		d.Status, d.Reason = "unknown", "source delay/padding was not established by exact probe evidence"
		return d
	}
	if timingCopy {
		d.Status, d.Method = "preserved", "packet_copy"
		d.Reason = "codec, channel layout, packets, and authoritative skip/discard timing are copied"
		return d
	}
	// FFmpeg can consume authoritative skip/discard samples during decode, but
	// a lossy encoder's new priming and muxer metadata are output facts. They
	// must be probed after encoding before Portico calls the result gapless.
	d.Status = "unverified"
	if token(a.Codec) != "" {
		d.Method = "decode_trim_or_remux"
	}
	d.Reason = "converted or repackaged output requires post-mux packet/side-data verification"
	return d
}

func verifiedExactSeek(video mediafacts.Video, sourceRevision string) bool {
	if video.ExactSeekSafe == nil || !*video.ExactSeekSafe || strings.TrimSpace(video.KeyframeEvidenceRevision) != strings.TrimSpace(sourceRevision) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(video.KeyframeEvidenceAt))
	return err == nil
}

func (a AudioDecision) DownmixPenalty() int {
	if a.Downmixed {
		return 15
	}
	return 0
}
func videoMatches(v mediafacts.Video, c playbackcap.Video) bool {
	hdr := string(v.DynamicRange())
	fr := float64(v.FrameRate.Num) / float64(v.FrameRate.Den)
	return same(c.Codec, v.Codec) && videoProfileMatches(v.Codec, c.Profile, v.Profile) && wild(c.Level, v.Level) && wild(c.Tag, v.CodecTag) && wild(c.PixelFormat, v.PixelFormat) && wild(c.Chroma, v.ChromaSubsampling) && wild(c.HDR, hdr) && (c.BitDepth == 0 || v.BitDepth <= c.BitDepth) && (c.MaxWidth == 0 || v.CodedWidth <= c.MaxWidth) && (c.MaxHeight == 0 || v.CodedHeight <= c.MaxHeight) && (c.MaxFrameRate == 0 || fr <= c.MaxFrameRate) && (v.DolbyVision == nil || c.DolbyVisionProfile == v.DolbyVision.Profile)
}

// FFmpeg reports H.264 constrained baseline as "Constrained Baseline", while
// browser capability probes use the codec-family label "baseline". These are
// equivalent delivery constraints; treating them as different would make a
// compatible MP4 source appear to require video conversion.
func videoProfileMatches(codec, want, have string) bool {
	if token(want) == "" || token(have) == "" {
		return true
	}
	if same(codec, "h264") || same(codec, "avc1") || same(codec, "avc") {
		wantProfile, haveProfile := canonicalH264Profile(want), canonicalH264Profile(have)
		wantRank, haveRank := h264ProfileRank(wantProfile), h264ProfileRank(haveProfile)
		if wantRank > 0 && haveRank > 0 {
			return wantRank >= haveRank
		}
		return wantProfile == haveProfile
	}
	return same(want, have)
}

func canonicalH264Profile(value string) string {
	switch token(value) {
	case "constrained baseline", "constrained-baseline", "baseline":
		return "baseline"
	case "main":
		return "main"
	case "high":
		return "high"
	default:
		return token(value)
	}
}

func h264ProfileRank(value string) int {
	switch value {
	case "baseline":
		return 1
	case "main":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}
func decideColor(v *mediafacts.Video, out playbackcap.Video, copy, disableToneMapping bool, toneMapAlgorithm string) (*ColorDecision, bool, bool, bool) {
	if v == nil {
		return nil, true, false, false
	}
	in := string(v.DynamicRange())
	target := token(out.HDR)
	if target == "" {
		target = "sdr"
	}
	d := &ColorDecision{Input: in, Output: target, Action: "preserve"}
	if v.DolbyVision != nil {
		d.DolbyVisionProfile = v.DolbyVision.Profile
		if copy && target == "dolby_vision" {
			return d, true, false, false
		}
		fb := token(v.DolbyVision.Fallback)
		verified := v.DolbyVision.BaseLayerPresentKnown && v.DolbyVision.BaseLayerPresent &&
			(fb == "hdr10" || fb == "pq" || fb == "hlg" || fb == "sdr")
		if (v.DolbyVision.Profile == 7 || v.DolbyVision.Profile == 8) && verified && target != "dolby_vision" {
			compatible := fb == target || fb == "hdr10" && target == "pq" || target == "sdr"
			if !compatible {
				return nil, false, true, false
			}
			if target == "sdr" && fb != "sdr" {
				if disableToneMapping {
					return nil, false, false, true
				}
				d.ToneMapAlgorithm = normalizedToneMapAlgorithm(toneMapAlgorithm)
			}
			d.Action = "use_verified_base"
			return d, true, false, false
		}
		return nil, false, true, false
	}
	if target == "dolby_vision" {
		return nil, false, true, false
	}
	if in == "hdr10plus" && target == "pq" {
		d.Action = "downgrade_hdr10plus"
		return d, true, false, false
	}
	if in != "sdr" && target == "sdr" {
		if disableToneMapping {
			return nil, false, false, true
		}
		d.Action = "tone_map_sdr"
		d.ToneMapAlgorithm = normalizedToneMapAlgorithm(toneMapAlgorithm)
		return d, true, false, false
	}
	if in != target {
		return nil, false, false, false
	}
	return d, true, false, false
}

func normalizedToneMapAlgorithm(value string) string {
	switch token(value) {
	case "clip", "linear", "gamma", "reinhard", "hable", "mobius":
		return token(value)
	default:
		return "mobius"
	}
}
func applySubtitle(p *Plan, s *mediafacts.Subtitle, c playbackcap.Subtitle) bool {
	if s == nil {
		p.Subtitle = SubtitleDecision{Action: Drop}
		return c.Mode == playbackcap.SubtitleNone
	}
	if token(c.Codec) != token(s.Codec) || token(c.Kind) != token(s.Kind) {
		return false
	}
	act := ExternalText
	switch c.Mode {
	case playbackcap.SubtitleNative, playbackcap.SubtitleConvert:
		act = ExternalText
	case playbackcap.SubtitleBurn:
		act = BurnIn
	default:
		return false
	}
	p.Subtitle = SubtitleDecision{&s.Index, s.Codec, s.Kind, act, s.Language, s.Disposition.Default, s.Disposition.Forced, s.Disposition.HearingImpaired}
	p.Streams = append(p.Streams, StreamAction{s.Index, "subtitle", act, s.Codec, s.Codec, "", ""})
	if act == BurnIn {
		p.Reasons = append(p.Reasons, ReasonSubtitleBurn)
	} else {
		p.Reasons = append(p.Reasons, ReasonSubtitleExternal)
	}
	return true
}
func graph(p Plan, v *mediafacts.Video, a *mediafacts.Audio, s *mediafacts.Subtitle) []Stage {
	var x []Stage
	if v != nil {
		if actionForKind(p.Streams, "video") == Copy {
			x = append(x, Stage{"video", "copy", "stream"})
		} else {
			x = append(x, Stage{"video", "decode", execution(p)})
			if p.Color != nil && p.Color.Action != "preserve" {
				x = append(x, Stage{"video", p.Color.Action, execution(p)})
			}
			if p.Subtitle.Action == BurnIn {
				x = append(x, Stage{"subtitle", "burn_in", execution(p)})
			}
			x = append(x, Stage{"video", "encode", execution(p)})
		}
	}
	if p.Audio.Passthrough {
		x = append(x, Stage{"audio", "copy", "stream"})
	} else if a != nil {
		x = append(x, Stage{"audio", "encode", "software"})
	}
	x = append(x, Stage{"mux", "package", "stream"})
	return x
}

func actionForKind(actions []StreamAction, kind string) Action {
	for _, action := range actions {
		if action.Kind == kind {
			return action.Action
		}
	}
	return Drop
}
func execution(p Plan) string {
	if p.Hardware.Verified && p.Hardware.Backend != "" {
		return "hardware"
	}
	return "software"
}
func cleanHardware(h HardwareRoute) HardwareRoute {
	if !h.Verified {
		return HardwareRoute{}
	}
	q := HardwareRoute{Verified: true, Backend: h.Backend, SoftwareFallbackVerified: h.SoftwareFallbackVerified}
	allowedOp := map[string]bool{"decode": true, "upload": true, "download": true, "scale": true, "deinterlace": true, "tone_map": true, "subtitle_burn": true, "encode": true}
	allowedExec := map[string]bool{"hardware": true, "software": true}
	for _, stage := range h.Stages {
		op, ex := token(stage.Operation), token(stage.Execution)
		if allowedOp[op] && allowedExec[ex] {
			q.Stages = append(q.Stages, Stage{Kind: "hardware", Operation: op, Execution: ex})
		}
	}
	return q
}
func unsupportedPlan(r Request, reason ReasonCode) Plan {
	p := Plan{SchemaRevision: SchemaRevision, Policy: r.Policy, Mode: Unsupported, CapabilityEvidenceID: r.Capabilities.EvidenceID, Hardware: cleanHardware(r.Hardware), Reasons: []ReasonCode{reason}}
	if c, e := r.Facts.Canonical(); e == nil {
		p.SourceFingerprint = c.Source.Fingerprint
		p.SourceRevision = c.Source.Revision
	}
	p.Digest, _ = p.ComputeDigest()
	return p
}
func segmentFormat(protocol, container string) string {
	switch token(protocol) {
	case "hls":
		if container == "mp4" {
			return "fmp4"
		}
		return "mpegts"
	case "dash":
		return "fmp4"
	default:
		return "progressive"
	}
}
func same(a, b string) bool       { return token(a) == token(b) }
func wild(have, want string) bool { return token(have) == "" || same(have, want) }
func tupleKey(t playbackcap.DeliveryTuple) string {
	b := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%02d|%s|%s|%02d|%s|%s", t.Kind, token(t.Protocol), token(t.Container), token(t.Video.Codec), token(t.Video.Profile), token(t.Video.HDR), token(t.Audio.Codec), t.Audio.MaxChannels, token(t.Audio.Layout), token(t.Audio.Route), t.Video.DolbyVisionProfile, token(t.Subtitle.Codec), t.Subtitle.Mode)
	return b
}
