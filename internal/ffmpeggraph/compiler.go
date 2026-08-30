// Package ffmpeggraph compiles an immutable playback plan and the exact media
// facts it was built from into deterministic FFmpeg arguments. It performs no
// probing or process execution and deliberately rejects incomplete graphs.
package ffmpeggraph

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

type Output struct {
	ManifestPath   string
	SegmentPattern string
	InitFilename   string
	SegmentSeconds int
	StartNumber    int
	StartSeconds   int
	// Event publishes an append-only HLS event playlist for a finite source
	// whose duration is not yet known. Unlike Live it retains every segment,
	// never deletes old media, and lets FFmpeg append ENDLIST on clean EOF.
	Event           bool
	Live            bool
	ProgressivePath string
}

type Request struct {
	Plan                  playbackplan.Plan
	Facts                 mediafacts.Facts
	SourcePath            string
	SubtitlePath          string
	SubtitleStreamOrdinal *int
	Hardware              *playbackhw.Plan
	X264Preset            string
	Output                Output
}

type Result struct {
	Args          []string
	VideoMap      string
	AudioMap      string
	SubtitleMap   string
	VideoFilter   string
	UsesHardware  bool
	SegmentFormat string
}

type ErrorCode string

const (
	InvalidPlan      ErrorCode = "invalid_plan"
	FactsMismatch    ErrorCode = "facts_mismatch"
	UnsupportedGraph ErrorCode = "unsupported_graph"
	InvalidOutput    ErrorCode = "invalid_output"
	HardwareMismatch ErrorCode = "hardware_mismatch"
)

type CompileError struct {
	Code   ErrorCode
	Detail string
}

func (e *CompileError) Error() string { return fmt.Sprintf("ffmpeggraph: %s: %s", e.Code, e.Detail) }
func fail(code ErrorCode, format string, args ...any) error {
	return &CompileError{code, fmt.Sprintf(format, args...)}
}

// Compile returns arguments excluding the FFmpeg executable. Paths are passed
// as distinct argv values; only a subtitle filename embedded in a filter graph
// is escaped with FFmpeg's filter-expression rules.
func Compile(req Request) (Result, error) {
	if err := req.Plan.Validate(); err != nil {
		return Result{}, fail(InvalidPlan, "%v", err)
	}
	if req.Plan.Mode == playbackplan.Unsupported {
		return Result{}, fail(UnsupportedGraph, "unsupported playback plan")
	}
	facts, err := req.Facts.Canonical()
	if err != nil {
		return Result{}, fail(FactsMismatch, "invalid facts: %v", err)
	}
	if facts.Source.Fingerprint != req.Plan.SourceFingerprint || facts.Source.Revision != req.Plan.SourceRevision {
		return Result{}, fail(FactsMismatch, "plan source identity does not match facts")
	}
	if strings.TrimSpace(req.SourcePath) == "" {
		return Result{}, fail(InvalidOutput, "source path is required")
	}
	video, audio, subtitle, err := selected(facts, req.Plan.Selection)
	if err != nil {
		return Result{}, err
	}
	if err := validateActions(req.Plan, video, audio, subtitle); err != nil {
		return Result{}, err
	}
	if err := validateStages(req.Plan); err != nil {
		return Result{}, err
	}

	r := Result{SegmentFormat: req.Plan.SegmentFormat}
	args := []string{"-hide_banner", "-nostdin", "-y"}
	if req.Hardware != nil {
		if !req.Plan.Hardware.Verified || req.Plan.Hardware.Backend == "" || req.Hardware.Backend != req.Plan.Hardware.Backend || req.Hardware.RequiresRuntimeProbe {
			return Result{}, fail(HardwareMismatch, "hardware arguments require the exact verified plan backend")
		}
		if err := validateHardware(req.Plan.Hardware, *req.Hardware); err != nil {
			return Result{}, err
		}
		args = append(args, req.Hardware.InputArgs...)
		r.UsesHardware = true
	} else if req.Plan.Hardware.Verified && req.Plan.Hardware.Backend != "" {
		return Result{}, fail(HardwareMismatch, "verified playback route requires its hardware plan")
	}
	if req.Output.StartSeconds < 0 {
		return Result{}, fail(InvalidOutput, "start seconds cannot be negative")
	}
	if req.Output.StartSeconds > 0 {
		args = append(args, "-ss", strconv.Itoa(req.Output.StartSeconds))
	}
	if video != nil && video.Rotation != 0 {
		// Rotation is represented explicitly in the sealed graph below. Disable
		// FFmpeg's implicit display-matrix autorotation or the transform can be
		// applied twice, with results that depend on the installed FFmpeg build.
		args = append(args, "-noautorotate")
	}
	args = append(args, "-i", req.SourcePath)
	bitmapBurn := subtitle != nil && req.Plan.Subtitle.Action == playbackplan.BurnIn && token(subtitle.Kind) == "bitmap"
	bitmapInput := ""
	if bitmapBurn {
		if strings.TrimSpace(req.SubtitlePath) == "" || !filepath.IsAbs(req.SubtitlePath) {
			return Result{}, fail(UnsupportedGraph, "bitmap subtitle burn requires an exact subtitle path to an absolute source")
		}
		if filepath.Clean(req.SubtitlePath) == filepath.Clean(req.SourcePath) {
			bitmapInput = streamMap(subtitle.Index)
		} else {
			if req.Output.StartSeconds > 0 {
				args = append(args, "-ss", strconv.Itoa(req.Output.StartSeconds))
			}
			args = append(args, "-i", req.SubtitlePath)
			bitmapInput = "1:0"
		}
	}

	videoAction := action(req.Plan, "video")
	if video != nil {
		videoEncoderName := ""
		switch videoAction.Action {
		case playbackplan.Copy:
			if needsVideoFilter(req.Plan, *video) {
				return Result{}, fail(UnsupportedGraph, "video copy cannot execute required filters")
			}
			r.VideoMap = streamMap(video.Index)
			args = append(args, "-map", r.VideoMap, "-c:v", "copy")
			args = append(args, containerVideoArgs(req.Plan, videoAction.OutputCodec, true)...)
		case playbackplan.Convert:
			filter, ferr := videoFilters(req, *video)
			if ferr != nil {
				return Result{}, ferr
			}
			r.VideoFilter = filter
			if bitmapBurn {
				postOverlayFilter := filter
				if postOverlayFilter == "" {
					postOverlayFilter = "null"
				}
				// Bitmap subtitle coordinates are defined against the source raster.
				// Composite before geometry transforms so subtitles and video are
				// rotated/scaled together instead of cropping a full-size PGS canvas
				// onto an already-downscaled video.
				complex := fmt.Sprintf("[%s]setpts=PTS-STARTPTS[portico_sub];[%s][portico_sub]overlay=eof_action=pass:shortest=0[portico_composited];[portico_composited]%s[portico_video]", bitmapInput, streamMap(video.Index), postOverlayFilter)
				r.VideoMap = "[portico_video]"
				r.VideoFilter = complex
				args = append(args, "-filter_complex", complex, "-map", r.VideoMap)
			} else {
				r.VideoMap = streamMap(video.Index)
				args = append(args, "-map", r.VideoMap)
			}
			if filter != "" && !bitmapBurn {
				args = append(args, "-vf", filter)
			}
			if req.Hardware != nil {
				args = append(args, req.Hardware.OutputArgs...)
			} else {
				enc, ok := videoEncoder(videoAction.OutputCodec)
				if !ok {
					return Result{}, fail(UnsupportedGraph, "unsupported video encoder %q", videoAction.OutputCodec)
				}
				videoEncoderName = enc
				args = append(args, "-c:v", enc)
				qualityArgs, qualityErr := softwareVideoQuality(enc, req.X264Preset, req.Plan.Constraints.MaxVideoBitrate)
				if qualityErr != nil {
					return Result{}, qualityErr
				}
				args = append(args, qualityArgs...)
			}
			colorArgs, cerr := colorEncodingArgs(req.Plan, *video, videoEncoderName)
			if cerr != nil {
				return Result{}, cerr
			}
			args = append(args, colorArgs...)
			args = append(args, containerVideoArgs(req.Plan, videoAction.OutputCodec, false)...)
		default:
			return Result{}, fail(UnsupportedGraph, "unsupported video action %q", videoAction.Action)
		}
	}
	r.AudioMap = streamMap(audio.Index)
	args = append(args, "-map", r.AudioMap)

	audioAction := action(req.Plan, "audio")
	if audioAction.Action != playbackplan.Copy && audioAction.Action != playbackplan.Convert {
		return Result{}, fail(UnsupportedGraph, "unsupported audio action %q", audioAction.Action)
	}
	audioArgs, err := CompileAudio(AudioRequest{
		InputCodec: audio.Codec, InputLayout: audio.Layout, InputChannels: audio.Channels, InputSampleRate: audio.SampleRate,
		OutputCodec: audioAction.OutputCodec, OutputLayout: req.Plan.Audio.Layout, OutputChannels: req.Plan.Audio.Channels, OutputContainer: req.Plan.Container,
		Copy: audioAction.Action == playbackplan.Copy, MaxBitrate: req.Plan.Constraints.MaxAudioBitrate, Gapless: req.Plan.Audio.Gapless,
	})
	if err != nil {
		return Result{}, err
	}
	args = append(args, audioArgs...)

	if subtitle != nil && req.Plan.Subtitle.Action == playbackplan.ExternalText {
		if token(req.Plan.Protocol) == "hls" {
			return Result{}, fail(UnsupportedGraph, "external HLS subtitles require a separately compiled rendition")
		}
		r.SubtitleMap = streamMap(subtitle.Index)
		args = append(args, "-map", r.SubtitleMap)
		codec, ok := subtitleCodec(req.Plan.Container, subtitle.Codec)
		if !ok {
			return Result{}, fail(UnsupportedGraph, "subtitle %q cannot be represented in %s", subtitle.Codec, req.Plan.Container)
		}
		args = append(args, "-c:s", codec)
	}
	if video != nil && videoAction.Action == playbackplan.Convert && token(req.Plan.Protocol) == "hls" {
		if req.Output.SegmentSeconds <= 0 {
			return Result{}, fail(InvalidOutput, "HLS video conversion requires a positive segment duration")
		}
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", req.Output.SegmentSeconds), "-sc_threshold", "0")
	}

	out, err := outputArgs(req.Plan, req.Output)
	if err != nil {
		return Result{}, err
	}
	r.Args = append(args, out...)
	return r, nil
}

func selected(f mediafacts.Facts, s playbackplan.Selection) (*mediafacts.Video, *mediafacts.Audio, *mediafacts.Subtitle, error) {
	var v *mediafacts.Video
	if s.VideoIndex != nil {
		for i := range f.Video {
			if f.Video[i].Index == *s.VideoIndex {
				v = &f.Video[i]
				break
			}
		}
		if v == nil {
			return nil, nil, nil, fail(FactsMismatch, "selected video is absent")
		}
	} else if len(f.Video) > 0 {
		v = &f.Video[0]
	}
	var a *mediafacts.Audio
	if s.AudioIndex != nil {
		for i := range f.Audio {
			if f.Audio[i].Index == *s.AudioIndex {
				a = &f.Audio[i]
				break
			}
		}
	} else if len(f.Audio) > 0 {
		a = &f.Audio[0]
	}
	if a == nil {
		return nil, nil, nil, fail(FactsMismatch, "selected audio is absent")
	}
	var sub *mediafacts.Subtitle
	if s.SubtitleIndex != nil {
		for i := range f.Subtitles {
			if f.Subtitles[i].Index == *s.SubtitleIndex {
				sub = &f.Subtitles[i]
				break
			}
		}
		if sub == nil {
			return nil, nil, nil, fail(FactsMismatch, "selected subtitle is absent")
		}
	}
	return v, a, sub, nil
}

func validateActions(p playbackplan.Plan, v *mediafacts.Video, a *mediafacts.Audio, s *mediafacts.Subtitle) error {
	seen := map[string]bool{}
	for _, x := range p.Streams {
		if seen[x.Kind] {
			return fail(InvalidPlan, "multiple %s actions", x.Kind)
		}
		seen[x.Kind] = true
		var codec string
		switch x.Kind {
		case "video":
			if v == nil || x.Index != v.Index {
				return fail(FactsMismatch, "video action mismatch")
			}
			codec = v.Codec
		case "audio":
			if a == nil || x.Index != a.Index {
				return fail(FactsMismatch, "audio action mismatch")
			}
			codec = a.Codec
		case "subtitle":
			if s == nil || x.Index != s.Index {
				return fail(FactsMismatch, "subtitle action mismatch")
			}
			codec = s.Codec
		default:
			return fail(InvalidPlan, "unknown stream kind %q", x.Kind)
		}
		if token(codec) != token(x.InputCodec) {
			return fail(FactsMismatch, "%s codec mismatch", x.Kind)
		}
	}
	if a != nil && !seen["audio"] || v != nil && !seen["video"] {
		return fail(InvalidPlan, "selected streams lack actions")
	}
	return nil
}

func validateStages(p playbackplan.Plan) error {
	have := map[string]bool{}
	allowed := map[string]bool{"video:copy": true, "video:decode": true, "video:encode": true, "video:preserve": true, "video:tone_map_sdr": true, "video:downgrade_hdr10plus": true, "video:use_verified_base": true, "subtitle:burn_in": true, "audio:copy": true, "audio:encode": true, "mux:package": true}
	for _, stage := range p.Stages {
		key := token(stage.Kind) + ":" + token(stage.Operation)
		if !allowed[key] {
			return fail(InvalidPlan, "unsupported graph stage %s", key)
		}
		if ex := token(stage.Execution); ex != "software" && ex != "hardware" && ex != "stream" {
			return fail(InvalidPlan, "unsupported execution %q", stage.Execution)
		}
		if have[key] {
			return fail(InvalidPlan, "duplicate graph stage %s", key)
		}
		have[key] = true
	}
	require := func(key string) error {
		if !have[key] {
			return fail(InvalidPlan, "graph lacks required %s stage", key)
		}
		return nil
	}
	if err := require("mux:package"); err != nil {
		return err
	}
	if x := action(p, "video"); x.Kind != "" {
		if x.Action == playbackplan.Copy {
			if err := require("video:copy"); err != nil {
				return err
			}
		} else if x.Action == playbackplan.Convert {
			if err := require("video:decode"); err != nil {
				return err
			}
			if err := require("video:encode"); err != nil {
				return err
			}
		} else {
			return fail(InvalidPlan, "unsupported video action %q", x.Action)
		}
	}
	if x := action(p, "audio"); x.Action == playbackplan.Copy {
		if err := require("audio:copy"); err != nil {
			return err
		}
	} else if x.Action == playbackplan.Convert {
		if err := require("audio:encode"); err != nil {
			return err
		}
	}
	if p.Subtitle.Action == playbackplan.BurnIn {
		if err := require("subtitle:burn_in"); err != nil {
			return err
		}
	}
	if p.Color != nil && p.Color.Action != "preserve" {
		if err := require("video:" + token(p.Color.Action)); err != nil {
			return err
		}
	}
	return nil
}

func validateHardware(route playbackplan.HardwareRoute, hw playbackhw.Plan) error {
	if len(route.Stages) == 0 || len(hw.Stages) == 0 {
		return fail(HardwareMismatch, "verified hardware route has no verified stages")
	}
	available := map[string]bool{}
	for _, s := range hw.Stages {
		available[string(s.Operation)+":"+string(s.Execution)] = true
	}
	for _, s := range route.Stages {
		if !available[token(s.Operation)+":"+token(s.Execution)] {
			return fail(HardwareMismatch, "hardware plan lacks verified %s/%s stage", s.Operation, s.Execution)
		}
	}
	return nil
}

func action(p playbackplan.Plan, kind string) playbackplan.StreamAction {
	for _, x := range p.Streams {
		if x.Kind == kind {
			return x
		}
	}
	return playbackplan.StreamAction{}
}
func streamMap(i int) string { return "0:" + strconv.Itoa(i) }
func token(s string) string  { return strings.ToLower(strings.TrimSpace(s)) }

func needsVideoFilter(p playbackplan.Plan, v mediafacts.Video) bool {
	return v.Rotation != 0 || token(v.FieldOrder) != "" && token(v.FieldOrder) != "progressive" || p.Color != nil && p.Color.Action != "preserve" || p.Subtitle.Action == playbackplan.BurnIn || p.Constraints.MaxWidth > 0 && v.CodedWidth > p.Constraints.MaxWidth || p.Constraints.MaxHeight > 0 && v.CodedHeight > p.Constraints.MaxHeight || v.SampleAspectRatio.Num != v.SampleAspectRatio.Den
}

func videoFilters(req Request, v mediafacts.Video) (string, error) {
	if req.Hardware != nil {
		// Hardware planner owns all crossings and device-specific filters. Mixing
		// generic geometry into it without a verified probe would be unsafe.
		if v.Rotation != 0 || v.SampleAspectRatio.Num != v.SampleAspectRatio.Den {
			return "", fail(UnsupportedGraph, "verified hardware route does not encode rotation/aspect transformation")
		}
		if req.Plan.Subtitle.Action == playbackplan.BurnIn {
			return "", fail(UnsupportedGraph, "subtitle burn is not represented by the verified hardware graph")
		}
		return req.Hardware.Filter, nil
	}
	var f []string
	videoAction := action(req.Plan, "video")
	field := token(v.FieldOrder)
	if field != "" && field != "progressive" {
		f = append(f, "bwdif=mode=send_frame:parity=auto:deint=interlaced")
	}
	if v.SampleAspectRatio.Num <= 0 || v.SampleAspectRatio.Den <= 0 {
		return "", fail(UnsupportedGraph, "video sample aspect ratio is not known")
	}
	if v.SampleAspectRatio.Num != v.SampleAspectRatio.Den {
		// Convert the coded raster to square pixels before rotation or bounding.
		// Merely setting SAR=1 changes the displayed geometry for anamorphic
		// sources. Use the sealed rational instead of mutable container metadata.
		f = append(f, fmt.Sprintf("scale=w='max(2\\,trunc(iw*%d/%d/2)*2)':h=ih:flags=lanczos", v.SampleAspectRatio.Num, v.SampleAspectRatio.Den), "setsar=1")
	}
	switch v.Rotation {
	case 0:
	case 90, -270:
		f = append(f, "transpose=clock")
	case -90, 270:
		f = append(f, "transpose=cclock")
	case 180, -180:
		f = append(f, "hflip", "vflip")
	default:
		return "", fail(UnsupportedGraph, "unsupported rotation %d", v.Rotation)
	}
	if req.Plan.Constraints.MaxWidth > 0 || req.Plan.Constraints.MaxHeight > 0 {
		w, h := "-2", "-2"
		if req.Plan.Constraints.MaxWidth > 0 {
			w = fmt.Sprintf("min(iw\\,%d)", req.Plan.Constraints.MaxWidth)
		}
		if req.Plan.Constraints.MaxHeight > 0 {
			h = fmt.Sprintf("min(ih\\,%d)", req.Plan.Constraints.MaxHeight)
		}
		f = append(f, fmt.Sprintf("scale=w='%s':h='%s':force_original_aspect_ratio=decrease:force_divisible_by=2", w, h))
	}
	if req.Plan.Color != nil {
		switch req.Plan.Color.Action {
		case "preserve":
		case "tone_map_sdr":
			algorithm := toneMapAlgorithm(req.Plan.Color)
			switch req.Plan.Color.Input {
			case "pq", "hdr10plus":
				f = append(f, "zscale=matrixin=bt2020nc:transferin=smpte2084:primariesin=bt2020:transfer=linear:npl=100", "format=gbrpf32le", "tonemap=tonemap="+algorithm+":desat=0", "zscale=matrix=bt709:transfer=bt709:primaries=bt709:range=tv", "format=yuv420p")
			case "hlg":
				f = append(f, "zscale=matrixin=bt2020nc:transferin=arib-std-b67:primariesin=bt2020:transfer=linear:npl=100", "format=gbrpf32le", "tonemap=tonemap="+algorithm+":desat=0", "zscale=matrix=bt709:transfer=bt709:primaries=bt709:range=tv", "format=yuv420p")
			default:
				return "", fail(UnsupportedGraph, "tone mapping source %q unsupported", req.Plan.Color.Input)
			}
		case "downgrade_hdr10plus":
			f = append(f, "zscale=matrix=bt2020nc:transfer=smpte2084:primaries=bt2020:range=tv", "format=yuv420p10le")
		case "use_verified_base":
			algorithm := toneMapAlgorithm(req.Plan.Color)
			base, ok := verifiedDolbyBase(v)
			if !ok {
				return "", fail(UnsupportedGraph, "Dolby Vision base layer lacks exact fallback evidence")
			}
			target := token(req.Plan.Color.Output)
			switch {
			case target == "sdr" && base == "sdr":
				f = append(f, "format=yuv420p")
			case target == "sdr" && base == "pq":
				f = append(f, "zscale=matrixin=bt2020nc:transferin=smpte2084:primariesin=bt2020:transfer=linear:npl=100", "format=gbrpf32le", "tonemap=tonemap="+algorithm+":desat=0", "zscale=matrix=bt709:transfer=bt709:primaries=bt709:range=tv", "format=yuv420p")
			case target == "sdr" && base == "hlg":
				f = append(f, "zscale=matrixin=bt2020nc:transferin=arib-std-b67:primariesin=bt2020:transfer=linear:npl=100", "format=gbrpf32le", "tonemap=tonemap="+algorithm+":desat=0", "zscale=matrix=bt709:transfer=bt709:primaries=bt709:range=tv", "format=yuv420p")
			case target == "pq" && base == "pq":
				f = append(f, "zscale=matrix=bt2020nc:transfer=smpte2084:primaries=bt2020:range=tv", "format=yuv420p10le")
			case target == "hlg" && base == "hlg":
				f = append(f, "zscale=matrix=bt2020nc:transfer=arib-std-b67:primaries=bt2020:range=tv", "format=yuv420p10le")
			default:
				return "", fail(UnsupportedGraph, "Dolby Vision fallback %s cannot produce %s", base, target)
			}
		default:
			return "", fail(UnsupportedGraph, "unknown color action %q", req.Plan.Color.Action)
		}
	}
	if req.Plan.Subtitle.Action == playbackplan.BurnIn {
		if strings.TrimSpace(req.SubtitlePath) == "" {
			return "", fail(UnsupportedGraph, "subtitle burn requires an exact subtitle path")
		}
		if !filepath.IsAbs(req.SubtitlePath) {
			return "", fail(UnsupportedGraph, "subtitle burn path must be absolute")
		}
		// Bitmap streams are decoded as a second video input and overlaid by the
		// caller's filter_complex. libass's subtitles filter is text-only and
		// must never be used for PGS/VOBSUB/DVB bitmap captions.
		if token(req.Plan.Subtitle.Kind) == "bitmap" {
			return strings.Join(f, ","), nil
		}
		filter := "subtitles=filename=" + filterQuote(req.SubtitlePath)
		if req.SubtitleStreamOrdinal != nil {
			if *req.SubtitleStreamOrdinal < 0 {
				return "", fail(UnsupportedGraph, "subtitle stream ordinal cannot be negative")
			}
			filter += ":stream_index=" + strconv.Itoa(*req.SubtitleStreamOrdinal)
		}
		f = append(f, filter)
	}
	// The canonical H.264 delivery tuple is 8-bit 4:2:0. Source pixel format is
	// not an output contract: without this explicit conversion libx264 retains
	// a Main10 source as High10, which browsers truthfully reject.
	if token(videoAction.OutputCodec) == "h264" && (v.BitDepth > 8 || token(v.PixelFormat) != "yuv420p") && !containsVideoFormatFilter(f, "yuv420p") {
		f = append(f, "format=yuv420p")
	}
	return strings.Join(f, ","), nil
}

func containsVideoFormatFilter(filters []string, pixelFormat string) bool {
	want := "format=" + token(pixelFormat)
	for _, filter := range filters {
		if token(filter) == want {
			return true
		}
	}
	return false
}

func toneMapAlgorithm(color *playbackplan.ColorDecision) string {
	if color != nil {
		switch token(color.ToneMapAlgorithm) {
		case "clip", "linear", "gamma", "reinhard", "hable", "mobius":
			return token(color.ToneMapAlgorithm)
		}
	}
	return "mobius"
}

func verifiedDolbyBase(v mediafacts.Video) (string, bool) {
	if v.DolbyVision == nil || (v.DolbyVision.Profile != 7 && v.DolbyVision.Profile != 8) ||
		!v.DolbyVision.BaseLayerPresentKnown || !v.DolbyVision.BaseLayerPresent {
		return "", false
	}
	switch token(v.DolbyVision.Fallback) {
	case "hdr10", "pq":
		return "pq", true
	case "hlg":
		return "hlg", true
	case "sdr":
		return "sdr", true
	default:
		return "", false
	}
}

func filterQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ":", "\\:")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	return "'" + s + "'"
}

func videoEncoder(c string) (string, bool) {
	switch token(c) {
	case "h264", "avc":
		return "libx264", true
	case "hevc", "h265":
		return "libx265", true
	case "av1":
		return "libsvtav1", true
	default:
		return "", false
	}
}
func audioEncoder(c string) (string, bool) {
	switch token(c) {
	case "aac":
		return "aac", true
	case "alac":
		return "alac", true
	case "ac3":
		return "ac3", true
	case "eac3", "e-ac-3":
		return "eac3", true
	case "opus":
		return "libopus", true
	case "vorbis":
		return "libvorbis", true
	case "flac":
		return "flac", true
	case "mp3":
		return "libmp3lame", true
	case "pcm", "pcm_s16le":
		return "pcm_s16le", true
	case "pcm_s24le":
		return "pcm_s24le", true
	default:
		return "", false
	}
}
func softwareVideoQuality(enc, x264Preset string, max int64) ([]string, error) {
	a := []string{}
	switch enc {
	case "libx264":
		if !validX264Preset(x264Preset) {
			return nil, fail(InvalidOutput, "invalid sealed x264 preset %q", x264Preset)
		}
		a = append(a, "-preset", x264Preset, "-crf", "20")
	case "libx265":
		a = append(a, "-preset", "medium", "-crf", "22")
	case "libsvtav1":
		a = append(a, "-preset", "6", "-crf", "28")
	}
	if max > 0 {
		a = append(a, "-maxrate", strconv.FormatInt(max, 10), "-bufsize", strconv.FormatInt(max*2, 10))
	}
	return a, nil
}

func validX264Preset(value string) bool {
	switch value {
	case "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower":
		return true
	default:
		return false
	}
}
func colorEncodingArgs(p playbackplan.Plan, v mediafacts.Video, encoder string) ([]string, error) {
	if p.Color == nil || p.Color.Output == "sdr" {
		return nil, nil
	}
	if p.Color.Action == "preserve" && v.DolbyVision != nil {
		return nil, fail(UnsupportedGraph, "Dolby Vision cannot be preserved through a generic video encode")
	}
	if p.Color.Action == "preserve" && v.HDR10Plus {
		return nil, fail(UnsupportedGraph, "HDR10+ dynamic metadata cannot be preserved through a generic video encode")
	}
	switch token(p.Color.Output) {
	case "pq", "hdr10", "hdr10plus":
		args := []string{"-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", "-color_range", "tv"}
		if encoder == "libx265" {
			args = append(args, "-x265-params", x265ColorParams("smpte2084", v))
		}
		return args, nil
	case "hlg":
		args := []string{"-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-color_range", "tv"}
		if encoder == "libx265" {
			args = append(args, "-x265-params", x265ColorParams("arib-std-b67", v))
		}
		return args, nil
	default:
		return nil, fail(UnsupportedGraph, "unsupported encoded color target %q", p.Color.Output)
	}
}

func x265ColorParams(transfer string, v mediafacts.Video) string {
	params := []string{"colorprim=bt2020", "transfer=" + transfer, "colormatrix=bt2020nc", "range=limited", "repeat-headers=1"}
	if transfer == "smpte2084" {
		params = append(params, "hdr10=1")
	}
	if m := v.MasteringDisplay; m != nil {
		scale := func(r mediafacts.Rational, factor int64) int64 {
			return (r.Num*factor + r.Den/2) / r.Den
		}
		params = append(params, fmt.Sprintf(
			"master-display=G(%d,%d)B(%d,%d)R(%d,%d)WP(%d,%d)L(%d,%d)",
			scale(m.Green.X, 50_000), scale(m.Green.Y, 50_000),
			scale(m.Blue.X, 50_000), scale(m.Blue.Y, 50_000),
			scale(m.Red.X, 50_000), scale(m.Red.Y, 50_000),
			scale(m.WhitePoint.X, 50_000), scale(m.WhitePoint.Y, 50_000),
			scale(m.MaxLuminance, 10_000), scale(m.MinLuminance, 10_000),
		))
	}
	if v.MaxCLL > 0 || v.MaxFALL > 0 {
		params = append(params, fmt.Sprintf("max-cll=%d,%d", v.MaxCLL, v.MaxFALL))
	}
	return strings.Join(params, ":")
}

func containerVideoArgs(p playbackplan.Plan, codec string, copy bool) []string {
	c := token(codec)
	if token(p.Container) == "mp4" && (c == "hevc" || c == "h265") {
		if p.Color != nil && token(p.Color.Output) == "dolby_vision" && token(p.Color.Action) == "preserve" {
			return []string{"-tag:v", "dvh1"}
		}
		return []string{"-tag:v", "hvc1"}
	}
	if copy && p.SegmentFormat == "mpegts" {
		switch c {
		case "h264", "avc":
			return []string{"-bsf:v", "h264_mp4toannexb"}
		case "hevc", "h265":
			return []string{"-bsf:v", "hevc_mp4toannexb"}
		}
	}
	return nil
}
func subtitleCodec(container, input string) (string, bool) {
	switch token(container) {
	case "mp4":
		return "mov_text", true
	case "matroska", "mkv":
		return "copy", true
	case "mpegts":
		if token(input) == "dvb_subtitle" {
			return "copy", true
		}
	}
	return "", false
}

func outputArgs(p playbackplan.Plan, o Output) ([]string, error) {
	switch token(p.Protocol) {
	case "hls":
		if o.ManifestPath == "" || o.SegmentPattern == "" || o.SegmentSeconds <= 0 {
			return nil, fail(InvalidOutput, "HLS manifest, segment pattern, and positive duration required")
		}
		if o.Event && o.Live {
			return nil, fail(InvalidOutput, "HLS output cannot be both event and live")
		}
		flags := "independent_segments+temp_file"
		size := "0"
		if o.Live {
			flags += "+delete_segments+omit_endlist+program_date_time"
			size = "12"
		}
		a := []string{
			"-muxdelay", "0", "-muxpreload", "0", "-output_ts_offset", strconv.Itoa(o.StartSeconds),
			"-f", "hls", "-hls_time", strconv.Itoa(o.SegmentSeconds), "-hls_list_size", size,
			"-start_number", strconv.Itoa(o.StartNumber), "-hls_flags", flags,
		}
		if o.Event {
			a = append(a, "-hls_playlist_type", "event")
		} else if !o.Live {
			a = append(a, "-hls_playlist_type", "vod")
		}
		switch p.SegmentFormat {
		case "fmp4":
			init := o.InitFilename
			if init == "" {
				init = "init.mp4"
			}
			a = append(a, "-hls_segment_options", "movflags=+frag_keyframe+empty_moov+default_base_moof:use_editlist=0", "-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", init)
		case "mpegts":
			a = append(a, "-hls_segment_options", "mpegts_copyts=0")
		default:
			return nil, fail(UnsupportedGraph, "unsupported HLS segment format %q", p.SegmentFormat)
		}
		return append(a, "-hls_segment_filename", o.SegmentPattern, o.ManifestPath), nil
	case "progressive", "http":
		if o.ProgressivePath == "" {
			return nil, fail(InvalidOutput, "progressive output path required")
		}
		return []string{"-f", muxer(p.Container), o.ProgressivePath}, nil
	default:
		return nil, fail(UnsupportedGraph, "protocol %q unsupported", p.Protocol)
	}
}
func muxer(c string) string {
	switch token(c) {
	case "mp4", "mov":
		return "mp4"
	case "matroska", "mkv":
		return "matroska"
	case "mpegts", "ts":
		return "mpegts"
	case "webm":
		return "webm"
	default:
		return token(c)
	}
}
