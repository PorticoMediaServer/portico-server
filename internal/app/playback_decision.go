package app

import (
	"net/url"
	"path/filepath"
	"strings"
)

func mediaPlaybackStreamURL(mediaID string) string {
	return "/api/media/" + url.PathEscape(mediaID) + "/stream"
}

func mediaPlaybackHLSURL(mediaID string, burnInSubtitleID string) string {
	return mediaPlaybackHLSURLWithQuality(mediaID, "", burnInSubtitleID)
}

func mediaPlaybackHLSURLWithQuality(mediaID string, quality string, burnInSubtitleID string) string {
	return mediaPlaybackHLSURLWithOptions(mediaID, quality, burnInSubtitleID, "", "", false)
}

func mediaPlaybackHLSURLWithOptions(mediaID string, quality string, burnInSubtitleID string, audioMode string, audioStreamID string, directStream bool) string {
	path := "/api/media/" + url.PathEscape(mediaID) + "/hls/master.m3u8"
	query := url.Values{}
	if quality = normalizeTranscodeQuality(quality); quality != "" && quality != "auto" {
		query.Set("quality", quality)
	}
	if burnInSubtitleID = normalizeBurnInSubtitleID(burnInSubtitleID); burnInSubtitleID != "" {
		query.Set("subtitle", burnInSubtitleID)
	}
	if audioMode = normalizeTranscodeAudioMode(audioMode); audioMode != "" {
		query.Set("audio", audioMode)
	}
	if audioStreamID = normalizeSelectedAudioStreamID(audioStreamID); audioStreamID != "" {
		query.Set("audioStream", audioStreamID)
	}
	if directStream {
		query.Set("directStream", "1")
	}
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func decideMediaPlayback(item MediaItem, sourceURL string, profile PlaybackClientProfile) PlaybackDecision {
	profile = normalizePlaybackProfile(profile)
	video := firstStreamOfKind(item.Streams, "video")
	audio := firstStreamOfKind(item.Streams, "audio")
	sourceKind := "local"
	if parsed, err := url.Parse(strings.TrimSpace(sourceURL)); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		sourceKind = "remote"
	}
	container := playbackContainerForItem(item, sourceURL)
	protocol := playbackProtocolFor(container)
	videoCodec := normalizeCodec(video.Codec)
	audioCodec := normalizeCodec(audio.Codec)

	containerSupported := container == "" || playbackListContains(profile.SupportedContainers, container)
	videoSupported := videoCodec == "" || playbackVideoCodecSupported(videoCodec, profile)
	audioSupported := audioCodec == "" || playbackAudioCodecSupported(audioCodec, profile)
	videoProfileSupported := playbackVideoProfileSupported(video, profile)
	videoPixelFormatSupported := playbackPixelFormatSupported(video, profile)
	videoBitDepthSupported := playbackBitDepthSupported(video, profile)
	videoFieldOrderSupported := playbackFieldOrderSupported(video)
	hdrSupported := playbackDynamicRangeSupported(video, profile)
	videoFits := true
	audioFits := true
	bitrateFits := true
	reasons := make([]string, 0, 4)
	reasonCodes := make([]string, 0, 4)
	if videoSupported && playbackCodecIsHEVC(videoCodec) && profileLooksLikeWebBrowser(profile) {
		videoSupported = false
		reasons = append(reasons, "browser HEVC playback requires server transcode")
		reasonCodes = appendUniqueString(reasonCodes, "browser_hevc_unsupported")
	}
	if !containerSupported {
		reasons = append(reasons, "container is not in the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "container_incompatible")
	}
	if !videoSupported {
		reasons = append(reasons, "video codec requires conversion")
		reasonCodes = appendUniqueString(reasonCodes, "video_codec_incompatible")
	}
	if !videoProfileSupported {
		reasons = append(reasons, "video profile requires conversion")
		reasonCodes = appendUniqueString(reasonCodes, "video_profile_incompatible")
	}
	if !videoPixelFormatSupported {
		reasons = append(reasons, "video pixel format requires conversion")
		reasonCodes = appendUniqueString(reasonCodes, "pixel_format_incompatible")
	}
	if !videoBitDepthSupported {
		reasons = append(reasons, "video bit depth exceeds the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "bit_depth_incompatible")
	}
	if !videoFieldOrderSupported {
		reasons = append(reasons, "interlaced video requires conversion")
		reasonCodes = appendUniqueString(reasonCodes, "interlaced_video")
	}
	if !hdrSupported {
		reasons = append(reasons, "HDR format is not supported by the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "hdr_incompatible")
	}
	if !audioSupported {
		reasons = append(reasons, "audio codec requires conversion")
		reasonCodes = appendUniqueString(reasonCodes, "audio_codec_incompatible")
	}
	if video.Width > 0 && profile.MaxWidth > 0 && video.Width > profile.MaxWidth {
		videoFits = false
		reasons = append(reasons, "video width exceeds the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "video_width_clamped")
	}
	if video.Height > 0 && profile.MaxHeight > 0 && video.Height > profile.MaxHeight {
		videoFits = false
		reasons = append(reasons, "video height exceeds the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "video_height_clamped")
	}
	if audio.Channels > 0 && profile.MaxAudioChannels > 0 && audio.Channels > profile.MaxAudioChannels {
		audioFits = false
		reasons = append(reasons, "audio channels exceed the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "audio_channels_incompatible")
	}
	if audio.Bitrate > 0 && profile.MaxAudioBitrateKbps > 0 && audio.Bitrate > profile.MaxAudioBitrateKbps*1_000 {
		audioFits = false
		reasons = append(reasons, "audio bitrate exceeds the resolved delivery profile")
		reasonCodes = appendUniqueString(reasonCodes, "audio_bitrate_clamped")
	}
	totalBitrate := video.Bitrate + audio.Bitrate
	if totalBitrate > 0 && profile.MaxBitrate > 0 && totalBitrate > profile.MaxBitrate {
		bitrateFits = false
		reasons = append(reasons, "stream bitrate exceeds the client profile")
		reasonCodes = appendUniqueString(reasonCodes, "bitrate_clamped")
	}

	if containerSupported && videoSupported && videoProfileSupported && videoPixelFormatSupported && videoBitDepthSupported && videoFieldOrderSupported && hdrSupported && audioSupported && videoFits && audioFits && bitrateFits {
		return PlaybackDecision{
			Mode:           "direct_play",
			Reason:         "Client profile supports the source container and codecs.",
			ReasonCodes:    []string{"source_compatible"},
			SourceKind:     sourceKind,
			Protocol:       protocol,
			Container:      container,
			VideoCodec:     videoCodec,
			AudioCodec:     audioCodec,
			IsProxied:      true,
			IsServerCached: false,
		}
	}
	return PlaybackDecision{
		Mode:              "transcode_required",
		Reason:            strings.Join(reasons, "; "),
		ReasonCodes:       reasonCodes,
		SourceKind:        sourceKind,
		Protocol:          protocol,
		Container:         container,
		VideoCodec:        videoCodec,
		AudioCodec:        audioCodec,
		RequiresTranscode: true,
		VideoTranscode:    video.ID != "" && (!videoSupported || !videoProfileSupported || !videoPixelFormatSupported || !videoBitDepthSupported || !videoFieldOrderSupported || !hdrSupported || !videoFits || !bitrateFits),
		AudioTranscode:    !audioSupported || !audioFits || (video.ID == "" && !bitrateFits),
		IsProxied:         true,
		IsServerCached:    false,
	}
}

func (s *Server) applyDirectStreamRemuxPolicy(decision PlaybackDecision, item MediaItem, profile PlaybackClientProfile) PlaybackDecision {
	if !decision.RequiresTranscode || !s.transcodeSettings().DirectStreamRemux {
		return decision
	}
	video := firstStreamOfKind(item.Streams, "video")
	if !directStreamRemuxVideoCodec(video.Codec) {
		return decision
	}
	if decision.VideoTranscode {
		return decision
	}
	if !directStreamRemuxAllowedForClient(video, profile) {
		decision.VideoTranscode = true
		decision.Reason = appendDecisionReason(decision.Reason, "video must be transcoded for this client HLS path")
		return decision
	}
	decision.Mode = "direct_stream"
	decision.Reason = appendDecisionReason(decision.Reason, "compatible video will be copied and remuxed")
	decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "compatible_remux")
	decision.RequiresRemux = true
	decision.VideoTranscode = false
	decision.RequiresTranscode = decision.AudioTranscode
	return decision
}

func (s *Server) applyTranscodeCapabilityNotes(decision PlaybackDecision, item MediaItem) PlaybackDecision {
	if !decision.RequiresTranscode {
		return decision
	}
	settings := s.transcodeSettings()
	if decision.VideoTranscode && mediaHasHDRVideo(item) && settings.HDRToneMapping && !settings.HDRToneMappingFilters {
		decision.Reason = appendDecisionReason(decision.Reason, "HDR tone mapping is configured but FFmpeg zscale/tonemap filters are unavailable")
	}
	if decision.VideoTranscode && settings.HardwareEncoding {
		resolvedDevice := s.resolvedHardwareDevice(settings)
		encoder := hardwareVideoEncoder(resolvedDevice)
		if encoder == "" {
			decision.Reason = appendDecisionReason(decision.Reason, "hardware encoding is configured but the selected device has no mapped encoder")
		} else if !s.cachedFFmpegEncoderAvailable(firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg"), encoder) {
			decision.Reason = appendDecisionReason(decision.Reason, "hardware encoding is configured but FFmpeg does not report "+encoder)
		}
	}
	if decision.VideoTranscode && settings.HardwareAcceleration {
		hwaccel := hardwareAccelValue(s.resolvedHardwareDevice(settings))
		if hwaccel == "" || hwaccel == "auto" {
			decision.Reason = appendDecisionReason(decision.Reason, "hardware decoding is configured but no concrete backend was resolved")
		} else if !s.cachedFFmpegHardwareAccelerationAvailable(firstNonEmpty(s.cfg.FFmpegPath, "ffmpeg"), hwaccel) {
			decision.Reason = appendDecisionReason(decision.Reason, "hardware decoding is configured but FFmpeg does not report "+hwaccel)
		}
	}
	return decision
}

func directStreamRemuxAllowedForClient(video Stream, profile PlaybackClientProfile) bool {
	if playbackCodecIsHEVC(video.Codec) {
		return !profileUsesMSEHLSPlayback(profile)
	}
	return true
}

func playbackCodecIsHEVC(codec string) bool {
	codec = normalizeCodec(codec)
	return codec == "hevc" || codec == "h265"
}

func profileLooksLikeWebBrowser(profile PlaybackClientProfile) bool {
	value := strings.ToLower(strings.TrimSpace(profile.Device + " " + profile.Platform))
	if value == "" {
		return false
	}
	for _, marker := range []string{"mozilla/", "applewebkit", " chrome/", " chromium/", " safari/", "firefox/", "edg/"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return profile.SupportsMSE && (strings.Contains(value, "macintel") || strings.Contains(value, "win32") || strings.Contains(value, "linux"))
}

func profileUsesMSEHLSPlayback(profile PlaybackClientProfile) bool {
	if !profile.SupportsMSE {
		return false
	}
	if profilePrefersNativeHLSPlayback(profile) {
		return false
	}
	return true
}

func profilePrefersNativeHLSPlayback(profile PlaybackClientProfile) bool {
	value := strings.ToLower(strings.TrimSpace(profile.Device + " " + profile.Platform))
	if value == "" {
		return false
	}
	if (strings.Contains(value, "safari") || strings.Contains(value, "apple")) &&
		!strings.Contains(value, "chrome") &&
		!strings.Contains(value, "chromium") &&
		!strings.Contains(value, "crios") &&
		!strings.Contains(value, "fxios") &&
		!strings.Contains(value, "edg") {
		return true
	}
	return strings.Contains(value, "ios") || strings.Contains(value, "ipados") || strings.Contains(value, "tvos")
}

func normalizeBurnInSubtitleID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "sub_none" || strings.HasPrefix(value, "native_subtitle_") {
		return ""
	}
	return value
}

func preferredStreamID(streams []Stream, kind, language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return ""
	}
	for _, stream := range streams {
		if strings.ToLower(strings.TrimSpace(stream.Kind)) != kind {
			continue
		}
		candidate := strings.ToLower(strings.TrimSpace(stream.Language))
		if candidate == language || strings.HasPrefix(candidate, language+"-") || strings.HasPrefix(language, candidate+"-") {
			return strings.TrimSpace(stream.ID)
		}
	}
	return ""
}

func preferredStreamIDForLanguages(streams []Stream, kind string, languages []string, fallback string) string {
	for _, language := range append(append([]string{}, languages...), fallback) {
		if selected := preferredStreamID(streams, kind, language); selected != "" {
			return selected
		}
	}
	return ""
}

func playbackSubtitleIntent(intent PlaybackIntent) (mode string, enabled bool) {
	mode = strings.ToLower(strings.TrimSpace(intent.PreferredSubtitleMode))
	if mode != "text" && mode != "burn_in" {
		mode = "off"
	}
	enabled = mode != "off"
	if intent.SubtitlesEnabled != nil {
		enabled = *intent.SubtitlesEnabled
	}
	if !enabled {
		return "off", false
	}
	return mode, true
}

func appendDecisionReason(current string, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" || strings.Contains(strings.ToLower(current), strings.ToLower(next)) {
		return current
	}
	return current + "; " + next
}

func decideLiveTVPlayback(streamURL string, streamFormat string, source liveTVSourceRecord, profile PlaybackClientProfile) PlaybackDecision {
	profile = normalizePlaybackProfile(profile)
	container := playbackContainerFor(streamURL)
	if streamFormat == "hls" {
		container = "hls"
	}
	protocol := playbackProtocolFor(container)
	cached := streamFormat == "hls"
	if streamFormat == "hls" && (profile.SupportsHLS || profile.SupportsMSE || playbackListContains(profile.SupportedContainers, "hls")) {
		return PlaybackDecision{
			Mode:           "direct_play",
			Reason:         "Live TV is compatible with the client and is served through the Portico HLS pipeline.",
			ReasonCodes:    []string{"source_compatible"},
			Protocol:       protocol,
			Container:      container,
			IsProxied:      true,
			IsServerCached: cached,
			BufferSeconds:  source.StreamBufferSeconds,
		}
	}
	if streamFormat == "direct" && (container == "" || playbackListContains(profile.SupportedContainers, container) || profile.SupportsMPEGTS) {
		return PlaybackDecision{
			Mode:           "direct_play",
			Reason:         "Provider stream is served through the Portico stream proxy.",
			ReasonCodes:    []string{"source_compatible"},
			Protocol:       protocol,
			Container:      container,
			IsProxied:      true,
			IsServerCached: false,
			BufferSeconds:  source.StreamBufferSeconds,
		}
	}
	return PlaybackDecision{
		Mode:              "transcode_required",
		Reason:            "The client profile does not advertise support for this live stream format.",
		ReasonCodes:       []string{"container_incompatible"},
		Protocol:          protocol,
		Container:         container,
		RequiresTranscode: true,
		IsProxied:         true,
		IsServerCached:    cached,
		BufferSeconds:     source.StreamBufferSeconds,
	}
}

func decideLiveTVHLSDelivery(streamURL string, source liveTVSourceRecord, profile PlaybackClientProfile) PlaybackDecision {
	decision := decideLiveTVPlayback(streamURL, "hls", source, profile)
	if isHLSURL(streamURL) {
		return decision
	}
	decision.Mode = "transcode_required"
	decision.Reason = "The provider stream is converted to the server-owned Live TV HLS delivery profile."
	decision.Protocol = "hls"
	decision.Container = "mpegts"
	decision.RequiresTranscode = true
	decision.VideoTranscode = true
	decision.AudioTranscode = true
	decision.IsProxied = true
	decision.IsServerCached = true
	return decision
}

func normalizePlaybackProfile(profile PlaybackClientProfile) PlaybackClientProfile {
	profile.SupportedContainers = normalizePlaybackStringList(profile.SupportedContainers)
	profile.SupportedVideoCodecs = normalizeCodecList(profile.SupportedVideoCodecs)
	profile.SupportedAudioCodecs = normalizeCodecList(profile.SupportedAudioCodecs)
	profile.SupportedVideoProfiles = normalizePlaybackStringList(profile.SupportedVideoProfiles)
	profile.SupportedPixelFormats = normalizePlaybackStringList(profile.SupportedPixelFormats)
	profile.SupportedHDRFormats = normalizePlaybackStringList(profile.SupportedHDRFormats)
	profile.SupportedDolbyVisionProfiles = normalizePlaybackStringList(profile.SupportedDolbyVisionProfiles)
	if len(profile.SupportedContainers) == 0 {
		profile.SupportedContainers = []string{"mp4", "m4v", "mov", "hls", "webm"}
	}
	if len(profile.SupportedVideoCodecs) == 0 {
		profile.SupportedVideoCodecs = []string{"h264", "avc1", "vp8", "vp9", "av1"}
		if profile.SupportsHEVC {
			profile.SupportedVideoCodecs = append(profile.SupportedVideoCodecs, "hevc", "h265")
		}
	}
	if len(profile.SupportedAudioCodecs) == 0 {
		profile.SupportedAudioCodecs = []string{"aac", "mp3", "opus", "vorbis"}
		if profile.SupportsEAC3 {
			profile.SupportedAudioCodecs = append(profile.SupportedAudioCodecs, "eac3")
		}
		if profile.SupportsAC3 {
			profile.SupportedAudioCodecs = append(profile.SupportedAudioCodecs, "ac3")
		}
	}
	if profile.SupportsHEVC && !playbackListContains(profile.SupportedVideoCodecs, "hevc") {
		profile.SupportedVideoCodecs = append(profile.SupportedVideoCodecs, "hevc")
	}
	if profile.SupportsEAC3 && !playbackListContains(profile.SupportedAudioCodecs, "eac3") {
		profile.SupportedAudioCodecs = append(profile.SupportedAudioCodecs, "eac3")
	}
	if profile.SupportsAC3 && !playbackListContains(profile.SupportedAudioCodecs, "ac3") {
		profile.SupportedAudioCodecs = append(profile.SupportedAudioCodecs, "ac3")
	}
	if profile.SupportsHLS && !playbackListContains(profile.SupportedContainers, "hls") {
		profile.SupportedContainers = append(profile.SupportedContainers, "hls")
	}
	if profile.SupportsMPEGTS && !playbackListContains(profile.SupportedContainers, "mpegts") {
		profile.SupportedContainers = append(profile.SupportedContainers, "mpegts")
	}
	return profile
}

func playbackVideoProfileSupported(video Stream, profile PlaybackClientProfile) bool {
	codec := normalizeCodec(video.Codec)
	videoProfile := normalizePlaybackProfileName(video.Profile)
	if videoProfile == "" {
		return true
	}
	if len(profile.SupportedVideoProfiles) > 0 {
		for _, supported := range profile.SupportedVideoProfiles {
			if playbackVideoProfileMatches(codec, videoProfile, supported) {
				return true
			}
		}
		return false
	}
	if profileLooksLikeAppleNative(profile) {
		switch codec {
		case "h264":
			return videoProfile != "high 10" && videoProfile != "high 422" && videoProfile != "high 444"
		case "hevc", "h265":
			return videoProfile == "main" || videoProfile == "main 10" || videoProfile == "main10"
		}
	}
	return true
}

func playbackVideoProfileMatches(codec string, actual string, supported string) bool {
	supported = strings.ToLower(strings.TrimSpace(supported))
	if supported == "" {
		return false
	}
	if strings.Contains(supported, ":") {
		parts := strings.SplitN(supported, ":", 2)
		return normalizeCodec(parts[0]) == codec && normalizePlaybackProfileName(parts[1]) == actual
	}
	return normalizePlaybackProfileName(supported) == actual
}

func normalizePlaybackProfileName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	switch value {
	case "constrained baseline":
		return "baseline"
	case "main10":
		return "main 10"
	default:
		return value
	}
}

func playbackPixelFormatSupported(video Stream, profile PlaybackClientProfile) bool {
	pixelFormat := strings.ToLower(strings.TrimSpace(video.PixelFormat))
	if pixelFormat == "" {
		return true
	}
	if len(profile.SupportedPixelFormats) > 0 && playbackListContains(profile.SupportedPixelFormats, pixelFormat) {
		return true
	}
	if strings.Contains(pixelFormat, "422") || strings.Contains(pixelFormat, "444") {
		return false
	}
	if len(profile.SupportedPixelFormats) == 0 {
		return true
	}
	return false
}

func playbackBitDepthSupported(video Stream, profile PlaybackClientProfile) bool {
	if video.BitDepth <= 0 {
		return true
	}
	maxDepth := profile.MaxVideoBitDepth
	if maxDepth <= 0 && profileLooksLikeAppleNative(profile) {
		maxDepth = 10
	}
	if maxDepth <= 0 {
		return true
	}
	return video.BitDepth <= maxDepth
}

func playbackFieldOrderSupported(video Stream) bool {
	fieldOrder := strings.ToLower(strings.TrimSpace(video.FieldOrder))
	return fieldOrder == "" || fieldOrder == "progressive" || fieldOrder == "unknown"
}

func playbackDynamicRangeSupported(video Stream, profile PlaybackClientProfile) bool {
	dynamicRange := strings.ToLower(strings.TrimSpace(video.DynamicRange))
	if dynamicRange == "" || dynamicRange == "sdr" {
		return true
	}
	if strings.HasPrefix(dynamicRange, "dolby_vision") {
		dvProfile := strings.TrimSpace(video.DolbyVisionProfile)
		if dvProfile == "" {
			dvProfile = strings.TrimPrefix(dynamicRange, "dolby_vision_profile_")
		}
		return profile.SupportsHDR && playbackListContains(profile.SupportedDolbyVisionProfiles, dvProfile)
	}
	if !profile.SupportsHDR {
		return false
	}
	if len(profile.SupportedHDRFormats) == 0 {
		return true
	}
	return playbackListContains(profile.SupportedHDRFormats, dynamicRange)
}

func profileLooksLikeAppleNative(profile PlaybackClientProfile) bool {
	value := strings.ToLower(strings.TrimSpace(profile.Device + " " + profile.Platform))
	return strings.Contains(value, "ios") || strings.Contains(value, "ipados") || strings.Contains(value, "tvos") || strings.Contains(value, "apple")
}

func firstStreamOfKind(streams []Stream, kind string) Stream {
	for _, stream := range streams {
		if strings.EqualFold(stream.Kind, kind) {
			return stream
		}
	}
	return Stream{}
}

func playbackContainerFor(source string) string {
	parsed, err := url.Parse(strings.TrimSpace(source))
	extSource := source
	if err == nil {
		extSource = parsed.Path
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(extSource)), ".")
	switch ext {
	case "m3u8":
		return "hls"
	case "mp4", "m4v", "mov":
		return ext
	case "mkv":
		return "matroska"
	case "ts", "m2ts":
		return "mpegts"
	case "webm":
		return "webm"
	default:
		return ""
	}
}

func playbackContainerForItem(item MediaItem, source string) string {
	if container := playbackContainerFor(source); container != "" {
		return container
	}
	for _, file := range item.MediaFiles {
		if file.Selected && strings.TrimSpace(file.Container) != "" {
			return normalizePlaybackContainer(file.Container)
		}
	}
	for _, file := range item.MediaFiles {
		if strings.TrimSpace(file.Container) != "" {
			return normalizePlaybackContainer(file.Container)
		}
	}
	return ""
}

func normalizePlaybackContainer(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "mkv", "matroska":
		return "matroska"
	case "mp4", "m4v", "mov", "isom", "iso2", "mp41", "mp42", "qt":
		return "mp4"
	case "ts", "m2ts", "mpegts":
		return "mpegts"
	default:
		return value
	}
}

func playbackProtocolFor(container string) string {
	switch container {
	case "hls":
		return "hls"
	case "mpegts":
		return "mpegts"
	default:
		return "http"
	}
}

func playbackVideoCodecSupported(codec string, profile PlaybackClientProfile) bool {
	if codec == "" {
		return true
	}
	if codec == "hevc" || codec == "h265" {
		return profile.SupportsHEVC || playbackListContains(profile.SupportedVideoCodecs, codec)
	}
	return playbackListContains(profile.SupportedVideoCodecs, codec)
}

func playbackAudioCodecSupported(codec string, profile PlaybackClientProfile) bool {
	if codec == "" {
		return true
	}
	if codec == "eac3" {
		return profile.SupportsEAC3 || playbackListContains(profile.SupportedAudioCodecs, codec)
	}
	if codec == "ac3" {
		return profile.SupportsAC3 || playbackListContains(profile.SupportedAudioCodecs, codec)
	}
	return playbackListContains(profile.SupportedAudioCodecs, codec)
}

func playbackListContains(items []string, target string) bool {
	target = normalizeCodec(target)
	for _, item := range items {
		if normalizeCodec(item) == target {
			return true
		}
	}
	return false
}

func normalizePlaybackStringList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		clean := strings.ToLower(strings.TrimSpace(item))
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func normalizeCodecList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		clean := normalizeCodec(item)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func normalizeCodec(codec string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	codec = strings.Split(codec, ".")[0]
	switch codec {
	case "avc", "avc1", "h.264", "x264":
		return "h264"
	case "hev1", "hvc1", "h.265", "x265":
		return "hevc"
	case "mp4a":
		return "aac"
	case "mp3", "mpeg3":
		return "mp3"
	case "e-ac-3", "ec-3":
		return "eac3"
	case "ac-3":
		return "ac3"
	default:
		return codec
	}
}
