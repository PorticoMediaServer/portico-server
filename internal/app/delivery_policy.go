package app

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	playbackNetworkLocal    = "local"
	playbackNetworkWiFi     = "wifi"
	playbackNetworkCellular = "cellular"
	playbackNetworkRemote   = "remote"
	playbackNetworkUnknown  = "unknown"
	playbackTransportWired  = "wired"
)

// resolvePlaybackPolicyForRequest turns portable client intent into an
// effective policy. Client intent is never treated as authority: a remote
// request cannot describe itself as local, and account/server clamps only
// narrow the result.
func (s *Server) resolvePlaybackPolicyForRequest(ctx context.Context, r *http.Request, user User, item MediaItem, intent PlaybackIntent, profile PlaybackClientProfile) (ResolvedPlaybackPolicy, PlaybackClientProfile) {
	serverLocality := s.playbackNetworkClassForRequest(r)
	transportClass := normalizePlaybackTransportClass(intent.TransportClass)
	if transportClass == playbackNetworkUnknown {
		transportClass = normalizePlaybackTransportClass(intent.NetworkClass)
	}
	networkClass := effectivePlaybackNetworkClass(intent.NetworkClass, transportClass, serverLocality)
	policy := defaultResolvedPlaybackPolicy(item.Type, networkClass)

	applyPlaybackIntent(&policy, item.Type, intent)
	policy.TransportClass = transportClass
	policy.ServerLocality = serverLocality
	policy.DeliveryProfile = resolvedDeliveryProfile(item.Type, policy)
	profile = applyResolvedPlaybackPolicy(profile, item.Type, policy)

	beforeBitrate := profile.MaxBitrate
	profile = s.applyPlaybackPolicyForRequest(ctx, r, user, profile)
	if beforeBitrate != profile.MaxBitrate && profile.MaxBitrate > 0 {
		policy.MaxVideoBitrateMbps = minPositive(policy.MaxVideoBitrateMbps, profile.MaxBitrate/1_000_000)
		policy.ServerClamps = appendUniqueString(policy.ServerClamps, "account_or_server_bitrate_limit")
	}
	if limit := s.remoteAccessBitrateLimitMbpsForRequest(r); limit > 0 {
		policy.MaxVideoBitrateMbps = minPositive(policy.MaxVideoBitrateMbps, limit)
		policy.ServerClamps = appendUniqueString(policy.ServerClamps, "remote_access_bitrate_limit")
	}
	policy.DeliveryProfile = resolvedDeliveryProfile(item.Type, policy)
	profile = applyResolvedPlaybackPolicy(profile, item.Type, policy)
	if authority, ok := s.authorizePlaybackCapabilityEvidence(ctx, user, profile); ok {
		profile.capabilityAuthority = authority
		profile.ClientFamily = authority.Family
		profile.Platform = authority.Platform
		profile.Device = authority.Device
	} else {
		profile.capabilityAuthority = playbackCapabilityAuthority{}
	}
	return policy, profile
}

func (s *Server) playbackNetworkClassForRequest(r *http.Request) string {
	if r == nil {
		return playbackNetworkUnknown
	}
	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if s.requestFromTrustedProxy(r) {
		forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
		if forwarded == "" {
			// A trusted proxy hop is not evidence of a LAN viewer. Missing client
			// provenance therefore receives the conservative remote policy.
			return playbackNetworkRemote
		}
		remoteAddr = forwarded
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.Trim(remoteAddr, "[]")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return playbackNetworkUnknown
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return playbackNetworkLocal
	}
	// A router or trusted reverse proxy can hairpin a viewer on the server's
	// own LAN and preserve the household public address. Treat an exact match
	// to Hosted's recently observed server public IP as a locality signal only.
	// It never grants authentication or authorization.
	if s.db != nil {
		settings, err := s.remoteAccessSettings()
		if err != nil {
			return playbackNetworkRemote
		}
		observed := net.ParseIP(strings.TrimSpace(settings.LastPublicIPAddress))
		if observed != nil && observed.Equal(ip) {
			return playbackNetworkLocal
		}
	}
	return playbackNetworkRemote
}

// effectivePlaybackNetworkClass keeps transport preference and server
// locality independent. A route hostname is not locality evidence: a viewer
// can reach a public-direct hostname from the server's own LAN, and a Wi-Fi
// client can be remote. Locality controls trust/clamps; transport selects the
// user's quality bucket.
func effectivePlaybackNetworkClass(legacyNetworkClass, transportClass, serverLocality string) string {
	legacy := normalizePlaybackNetworkClass(legacyNetworkClass)
	transport := normalizePlaybackTransportClass(transportClass)
	locality := normalizePlaybackServerLocality(serverLocality)
	if locality == playbackNetworkLocal {
		return playbackNetworkLocal
	}
	if transport == playbackNetworkWiFi || transport == playbackNetworkCellular {
		return transport
	}
	if legacy == playbackNetworkWiFi || legacy == playbackNetworkCellular {
		return legacy
	}
	if locality == playbackNetworkRemote {
		return playbackNetworkRemote
	}
	return playbackNetworkUnknown
}

func normalizePlaybackTransportClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case playbackNetworkWiFi, "wi-fi":
		return playbackNetworkWiFi
	case playbackNetworkCellular:
		return playbackNetworkCellular
	case playbackTransportWired, "ethernet":
		return playbackTransportWired
	default:
		return playbackNetworkUnknown
	}
}

func normalizePlaybackServerLocality(value string) string {
	switch normalizePlaybackNetworkClass(value) {
	case playbackNetworkLocal:
		return playbackNetworkLocal
	case playbackNetworkRemote:
		return playbackNetworkRemote
	default:
		return playbackNetworkUnknown
	}
}

func mustRemoteAccessSettings(s *Server) RemoteAccessSettings {
	settings, err := s.remoteAccessSettings()
	if err != nil || !settings.Enabled || settings.PublicPortMode == "disabled" {
		return RemoteAccessSettings{}
	}
	return settings
}

func defaultResolvedPlaybackPolicy(mediaType, networkClass string) ResolvedPlaybackPolicy {
	policy := qualityDefaultsForNetwork(mediaType, normalizePlaybackNetworkClass(networkClass))
	if strings.EqualFold(strings.TrimSpace(mediaType), "live_channel") || strings.EqualFold(strings.TrimSpace(mediaType), "library_channel") {
		policy.LiveHLS = &LiveHLSPlaybackPolicy{
			AuthorizationTransport: "header_or_secure_http_only_cookie",
			PlaylistScope:          "playback_session",
			SegmentScope:           "playback_session",
			CredentialQueryAllowed: false,
		}
	}
	return policy
}

func qualityDefaultsForNetwork(mediaType, networkClass string) ResolvedPlaybackPolicy {
	policy := ResolvedPlaybackPolicy{
		NetworkClass:       networkClass,
		QualityProfile:     "automatic",
		DirectPlayPolicy:   "prefer",
		DirectStreamPolicy: "allow",
		TranscodePolicy:    "allow",
		AllowHDR:           true,
		ServerClamps:       []string{},
	}
	if isAudioMediaType(mediaType) {
		switch networkClass {
		case playbackNetworkCellular:
			policy.QualityProfile = "data_saver"
		case playbackNetworkRemote, playbackNetworkUnknown:
			policy.QualityProfile = "standard"
		default:
			policy.QualityProfile = "original"
		}
		return policy
	}
	switch networkClass {
	case playbackNetworkLocal:
		policy.QualityProfile = "original"
	case playbackNetworkWiFi:
		policy.QualityProfile = "high"
	case playbackNetworkCellular:
		policy.QualityProfile, policy.AllowHDR = "data_saver", false
	case playbackNetworkRemote:
		policy.QualityProfile, policy.AllowHDR = "standard", false
	default:
		policy.NetworkClass = playbackNetworkUnknown
		policy.QualityProfile, policy.AllowHDR = "standard", false
	}
	return policy
}

func applyPlaybackIntent(policy *ResolvedPlaybackPolicy, mediaType string, intent PlaybackIntent) {
	if policy == nil {
		return
	}
	// NetworkClass is already resolved from server-observed locality and the
	// client's transport hint before this function runs. Never let the legacy
	// client field overwrite that result: a phone on Wi-Fi can still be remote,
	// while a public-direct request can be a same-household hairpin and should
	// retain local/original defaults.
	if value := normalizePlaybackQualityProfile(intent.QualityProfile); value != "" {
		applyQualityProfileNarrowing(policy, value)
	}
	if value := normalizeDirectDeliveryPolicy(intent.DirectPlayPolicy); value != "" {
		policy.DirectPlayPolicy = value
	}
	if value := normalizeDirectDeliveryPolicy(intent.DirectStreamPolicy); value != "" {
		policy.DirectStreamPolicy = value
	}
	if value := normalizeTranscodePolicy(intent.TranscodePolicy); value != "" {
		policy.TranscodePolicy = value
	}
	if intent.MaxVideoBitrateMbps > 0 {
		policy.MaxVideoBitrateMbps = minPositive(policy.MaxVideoBitrateMbps, intent.MaxVideoBitrateMbps)
	}
	if intent.MaxAudioBitrateKbps > 0 {
		policy.MaxAudioBitrateKbps = minPositive(policy.MaxAudioBitrateKbps, intent.MaxAudioBitrateKbps)
	}
	if intent.MaxVideoHeight > 0 {
		policy.MaxVideoHeight = minPositive(policy.MaxVideoHeight, intent.MaxVideoHeight)
	}
	if intent.AllowHDR != nil {
		policy.AllowHDR = policy.AllowHDR && *intent.AllowHDR
	}
}

func applyQualityProfileNarrowing(policy *ResolvedPlaybackPolicy, requested string) {
	if requested == "automatic" {
		return
	}
	policy.QualityProfile = requested
	if requested == "standard" || requested == "data_saver" {
		policy.AllowHDR = false
	}
}

func applyResolvedPlaybackPolicy(profile PlaybackClientProfile, mediaType string, policy ResolvedPlaybackPolicy) PlaybackClientProfile {
	deliveryProfile := resolvedDeliveryProfile(mediaType, policy)
	preset, hasPreset := transcodePresets[deliveryProfile]
	videoLimitMbps := policy.MaxVideoBitrateMbps
	audioLimitKbps := policy.MaxAudioBitrateKbps
	heightLimit := policy.MaxVideoHeight
	if hasPreset {
		videoLimitMbps = minPositive(videoLimitMbps, preset.videoK/1000)
		audioLimitKbps = minPositive(audioLimitKbps, preset.audioK)
		if !isAudioMediaType(mediaType) {
			heightLimit = minPositive(heightLimit, preset.height)
		}
	}
	if videoLimitMbps > 0 {
		profile.MaxBitrate = minPositive(profile.MaxBitrate, videoLimitMbps*1_000_000)
	}
	if audioLimitKbps > 0 {
		profile.MaxAudioBitrateKbps = minPositive(profile.MaxAudioBitrateKbps, audioLimitKbps)
		if isAudioMediaType(mediaType) {
			profile.MaxBitrate = minPositive(profile.MaxBitrate, audioLimitKbps*1_000)
		}
	}
	if heightLimit > 0 {
		profile.MaxHeight = minPositive(profile.MaxHeight, heightLimit)
	}
	if !policy.AllowHDR {
		profile.SupportsHDR = false
		profile.SupportedHDRFormats = []string{}
		profile.SupportedDolbyVisionProfiles = []string{}
	}
	return profile
}

func applyResolvedDeliveryMode(decision PlaybackDecision, item MediaItem, policy ResolvedPlaybackPolicy, remuxEnabled bool, transcodeAvailable bool) PlaybackDecision {
	decision.DeliveryProfile = policy.DeliveryProfile
	directStreamExactSeekMissing := decision.Mode == "direct_stream" && !remuxEnabled
	if directStreamExactSeekMissing {
		decision.Reason = appendDecisionReason(decision.Reason, "copy remux is unavailable without exact-seek keyframe evidence")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "direct_stream_exact_seek_evidence_missing")
	}
	video := firstStreamOfKind(item.Streams, "video")
	audio := firstStreamOfKind(item.Streams, "audio")
	compatibleDelivery := decision.Mode == "direct_play" || decision.Mode == "optimized_version" || decision.Mode == "direct_stream"
	expectsAudio := isAudioMediaType(item.Type)
	expectsVideo := !expectsAudio && item.Type != "live_channel"
	profile := resolvedDeliveryProfile(item.Type, policy)
	preset, hasPreset := transcodePresets[profile]
	effectiveVideoLimit := policy.MaxVideoBitrateMbps
	effectiveHeightLimit := policy.MaxVideoHeight
	effectiveAudioLimit := policy.MaxAudioBitrateKbps
	if hasPreset {
		effectiveVideoLimit = minPositive(effectiveVideoLimit, preset.videoK/1000)
		effectiveHeightLimit = minPositive(effectiveHeightLimit, preset.height)
		effectiveAudioLimit = minPositive(effectiveAudioLimit, preset.audioK)
	}
	videoUnknown := expectsVideo && (video.Kind != "video" ||
		(effectiveVideoLimit > 0 && video.Bitrate <= 0) ||
		(effectiveHeightLimit > 0 && video.Height <= 0))
	audioUnknown := (audio.Kind == "audio" && effectiveAudioLimit > 0 && audio.Bitrate <= 0) ||
		(expectsAudio && audio.Kind != "audio" && effectiveAudioLimit > 0)
	if compatibleDelivery && (videoUnknown || audioUnknown) {
		decision.Mode = "transcode_required"
		decision.RequiresTranscode = true
		decision.RequiresRemux = false
		decision.VideoTranscode = video.ID != ""
		decision.AudioTranscode = audio.ID != ""
		decision.Reason = appendDecisionReason(decision.Reason, "source metadata is insufficient to prove the resolved delivery limits")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_source_metadata_unknown")
	}
	if policy.TranscodePolicy == "never" && decision.RequiresTranscode {
		decision.Mode = "unavailable"
		decision.RequiresRemux = false
		decision.Reason = appendDecisionReason(decision.Reason, "the resolved delivery policy does not permit transcoding")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_transcode_disabled")
		return decision
	}
	if policy.TranscodePolicy == "require" {
		decision.Mode = "transcode_required"
		decision.RequiresTranscode = true
		decision.VideoTranscode = firstStreamOfKind(item.Streams, "video").ID != ""
		decision.AudioTranscode = firstStreamOfKind(item.Streams, "audio").ID != ""
		decision.RequiresRemux = false
		decision.Reason = appendDecisionReason(decision.Reason, "transcoding was requested by the resolved delivery policy")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_transcode_required")
		return decision
	}
	if policy.TranscodePolicy == "prefer" {
		if transcodeAvailable {
			decision.Mode = "transcode_required"
			decision.RequiresTranscode = true
			decision.VideoTranscode = firstStreamOfKind(item.Streams, "video").ID != ""
			decision.AudioTranscode = true
			decision.RequiresRemux = false
			decision.Reason = appendDecisionReason(decision.Reason, "transcoding was preferred by the resolved delivery policy")
			decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_transcode_preferred")
			return decision
		}
		decision.Reason = appendDecisionReason(decision.Reason, "preferred transcoding is unavailable, so compatible direct delivery is retained")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_transcode_preference_unavailable")
	}
	if decision.Mode == "direct_stream" && !remuxEnabled {
		decision.Mode = "transcode_required"
		decision.RequiresTranscode = true
		decision.RequiresRemux = false
		decision.VideoTranscode = firstStreamOfKind(item.Streams, "video").ID != ""
		decision.AudioTranscode = firstStreamOfKind(item.Streams, "audio").ID != ""
	}
	if (decision.Mode == "direct_play" || decision.Mode == "optimized_version") && policy.DirectPlayPolicy == "never" {
		if remuxEnabled && policy.DirectStreamPolicy != "never" && directStreamRemuxVideoCodec(firstStreamOfKind(item.Streams, "video").Codec) {
			decision.Mode = "direct_stream"
			decision.RequiresRemux = true
			decision.RequiresTranscode = false
			decision.VideoTranscode = false
			decision.Reason = appendDecisionReason(decision.Reason, "direct play was disabled and a compatible remux is available")
			decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_direct_stream")
			return decision
		}
		decision.Mode, decision.RequiresTranscode = "transcode_required", true
		decision.VideoTranscode = firstStreamOfKind(item.Streams, "video").ID != ""
		decision.AudioTranscode = firstStreamOfKind(item.Streams, "audio").ID != ""
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_direct_play_disabled")
	}
	if (decision.Mode == "direct_play" || decision.Mode == "optimized_version") && policy.DirectStreamPolicy == "prefer" && remuxEnabled && directStreamRemuxVideoCodec(firstStreamOfKind(item.Streams, "video").Codec) {
		decision.Mode, decision.RequiresRemux = "direct_stream", true
		decision.Reason = appendDecisionReason(decision.Reason, "direct stream was preferred and the source can be remuxed")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_direct_stream")
	}
	if decision.Mode == "direct_stream" && policy.DirectStreamPolicy == "never" {
		decision.Mode, decision.RequiresTranscode, decision.RequiresRemux = "transcode_required", true, false
		decision.VideoTranscode = firstStreamOfKind(item.Streams, "video").ID != ""
		decision.AudioTranscode = firstStreamOfKind(item.Streams, "audio").ID != ""
		decision.Reason = appendDecisionReason(decision.Reason, "direct stream was disabled by the resolved delivery policy")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_direct_stream_disabled")
	}
	if policy.TranscodePolicy == "never" && decision.RequiresTranscode {
		decision.Mode = "unavailable"
		decision.RequiresRemux = false
		decision.Reason = appendDecisionReason(decision.Reason, "the resolved delivery policy does not permit the required fallback transcode")
		decision.ReasonCodes = appendUniqueString(decision.ReasonCodes, "delivery_policy_transcode_disabled")
	}
	return decision
}

// Copy-remux is fail-closed until media analysis persists evidence that every
// HLS grid boundary has a source keyframe. Full transcoding creates that grid;
// arbitrary GOP sources must never be advertised as exact-seek Direct Stream.
func directStreamExactSeekEvidence(item MediaItem) bool {
	video := firstStreamOfKind(item.Streams, "video")
	if video.Kind != "video" || !video.ExactSeekSafe || strings.TrimSpace(video.KeyframeEvidenceAt) == "" {
		return false
	}
	evidenceAt, err := time.Parse(time.RFC3339, video.KeyframeEvidenceAt)
	if err != nil {
		return false
	}
	for _, file := range item.MediaFiles {
		if !file.Selected && len(item.MediaFiles) > 1 {
			continue
		}
		if strings.TrimSpace(file.ModTime) == "" {
			continue
		}
		modifiedAt, parseErr := time.Parse(time.RFC3339, file.ModTime)
		if parseErr != nil || evidenceAt.Before(modifiedAt) {
			return false
		}
		break
	}
	return true
}

func normalizePlaybackNetworkClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case playbackNetworkLocal, "lan":
		return playbackNetworkLocal
	case playbackNetworkWiFi, "wi-fi":
		return playbackNetworkWiFi
	case playbackNetworkCellular:
		return playbackNetworkCellular
	case playbackNetworkRemote:
		return playbackNetworkRemote
	default:
		return playbackNetworkUnknown
	}
}

func normalizePlaybackQualityProfile(value string) string {
	value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
	switch value {
	case "automatic", "original", "high", "standard", "data_saver":
		return value
	default:
		return ""
	}
}

func normalizeDirectDeliveryPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow", "prefer", "never":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeTranscodePolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow", "prefer", "require", "never":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func resolvedDeliveryProfile(mediaType string, policy ResolvedPlaybackPolicy) string {
	if isAudioMediaType(mediaType) {
		if policy.QualityProfile == "original" && policy.MaxAudioBitrateKbps <= 0 {
			return "audio-original"
		}
		maximumRank := 0
		switch policy.QualityProfile {
		case "standard":
			maximumRank = 1
		case "data_saver":
			maximumRank = 2
		}
		candidates := []string{"audio-high", "audio-standard", "audio-data-saver"}
		for index := maximumRank; index < len(candidates); index++ {
			preset := transcodePresets[candidates[index]]
			if policy.MaxAudioBitrateKbps <= 0 || preset.audioK <= policy.MaxAudioBitrateKbps {
				return candidates[index]
			}
		}
		return "audio-data-saver"
	}
	if policy.QualityProfile == "original" && policy.MaxVideoBitrateMbps <= 0 && policy.MaxVideoHeight <= 0 && policy.MaxAudioBitrateKbps <= 0 {
		return "video-original"
	}
	maximumRank := 0
	switch policy.QualityProfile {
	case "standard":
		maximumRank = 1
	case "data_saver":
		maximumRank = 2
	}
	candidates := []string{"video-high", "video-standard", "video-data-saver", "video-low"}
	for index := maximumRank; index < len(candidates); index++ {
		id := candidates[index]
		preset := transcodePresets[id]
		if policy.MaxVideoBitrateMbps > 0 && preset.videoK > policy.MaxVideoBitrateMbps*1000 {
			continue
		}
		if policy.MaxVideoHeight > 0 && preset.height > policy.MaxVideoHeight {
			continue
		}
		if policy.MaxAudioBitrateKbps > 0 && preset.audioK > policy.MaxAudioBitrateKbps {
			continue
		}
		return id
	}
	return "video-low"
}

func transcodeQualityForResolvedPolicy(mediaType string, policy ResolvedPlaybackPolicy) string {
	profile := strings.TrimSpace(policy.DeliveryProfile)
	if profile == "" {
		profile = resolvedDeliveryProfile(mediaType, policy)
	}
	if profile == "video-original" || profile == "audio-original" {
		return "original"
	}
	if _, ok := transcodePresets[profile]; ok {
		return profile
	}
	return "auto"
}

func minPositive(current, candidate int) int {
	if candidate <= 0 {
		return current
	}
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isAudioMediaType(mediaType string) bool {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "track", "album", "artist", "audiobook", "book", "podcast", "podcast_episode":
		return true
	default:
		return false
	}
}
