package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

var errPlaybackFactsUnavailable = errors.New("playback analysis facts are unavailable for the selected source")

func (s *Server) planMediaPlayback(ctx context.Context, item MediaItem, profile PlaybackClientProfile, policy ResolvedPlaybackPolicy, selectedAudioID, selectedSubtitleID, subtitleMode string) (PlaybackDecision, error) {
	facts, factsDigest, err := s.mediaFactsForPlayback(ctx, item)
	if err != nil {
		return PlaybackDecision{}, err
	}
	if _, _, parseErr := parseRemoteStorageLocator(strings.TrimSpace(item.SourceURL)); parseErr == nil {
		previousStreams := item.Streams
		item.Streams, _ = s.listStreamsContext(ctx, item.ID)
		scopePlaybackStreamsToSelectedVersion(&item)
		selectedAudioID = remapReplacedScannerStream(previousStreams, item.Streams, selectedAudioID, "audio")
		selectedSubtitleID = remapReplacedScannerStream(previousStreams, item.Streams, selectedSubtitleID, "subtitle")
	}
	capabilities, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		return PlaybackDecision{}, err
	}
	selection := playbackplan.Selection{}
	if index, ok := playbackStreamIndex(item.Streams, selectedAudioID, "audio"); ok {
		selection.AudioIndex = &index
	}
	if strings.EqualFold(strings.TrimSpace(subtitleMode), "text") || strings.EqualFold(strings.TrimSpace(subtitleMode), "burn_in") {
		if index, ok := playbackStreamIndex(item.Streams, selectedSubtitleID, "subtitle"); ok {
			selection.SubtitleIndex = &index
		}
	}
	settings := s.transcodeSettings()
	request := playbackplan.Request{
		Facts: facts, Capabilities: capabilities, Policy: playbackplan.OwnerPolicy(settings.PlanningPolicy), Selection: selection,
		DisableToneMapping: !settings.HDRToneMapping, ToneMapAlgorithm: safeToneMappingAlgorithm(settings.HDRToneMappingAlgorithm),
		Constraints: playbackplan.Constraints{
			MaxVideoBitrate: int64(max(0, policy.MaxVideoBitrateMbps)) * 1_000_000,
			MaxAudioBitrate: int64(max(0, policy.MaxAudioBitrateKbps)) * 1_000,
			MaxHeight:       max(0, policy.MaxVideoHeight),
		},
	}
	request.AllowedModes, request.PreferredModes = playbackPlanModePolicy(policy, settings.DirectStreamRemux)
	var optimizedArtifact *optimizedV2ReadyArtifact
	if strings.TrimSpace(selectedAudioID) == "" && strings.TrimSpace(selectedSubtitleID) == "" &&
		(strings.TrimSpace(subtitleMode) == "" || strings.EqualFold(strings.TrimSpace(subtitleMode), "off")) {
		if candidateItem, candidateFacts, candidateDigest, artifact, ok := s.optimizedPlaybackPreferenceSource(ctx, item, facts, factsDigest); ok {
			candidateRequest := request
			candidateRequest.Facts = candidateFacts
			candidateRequest.Selection = playbackplan.Selection{}
			candidatePlan, candidateErr := playbackplan.Build(candidateRequest)
			if candidateErr == nil && candidatePlan.Mode == playbackplan.DirectPlay && candidatePlan.Protocol == "http" {
				item, facts, factsDigest, request, optimizedArtifact = candidateItem, candidateFacts, candidateDigest, candidateRequest, artifact
			}
		}
	}
	plan, err := playbackplan.Build(request)
	if err != nil {
		return PlaybackDecision{}, err
	}
	// Every generated VOD plan must match the executor that will consume it.
	// Progressive direct play remains HTTP; all remux/encode graphs are rebuilt
	// against an HLS tuple instead of silently changing packaging later.
	if plan.Mode != playbackplan.DirectPlay && plan.Mode != playbackplan.Unsupported && plan.Protocol != "hls" {
		request.Protocol = "hls"
		plan, err = playbackplan.Build(request)
		if err != nil {
			return PlaybackDecision{}, err
		}
	}
	var hardwarePlan *playbackhw.Plan
	if route, executable := s.resolvePlaybackHardwareRoute(ctx, settings, facts, plan, item.SourceURL); executable != nil {
		request.Hardware = route
		plan, err = playbackplan.Build(request)
		if err != nil {
			return PlaybackDecision{}, err
		}
		if plan.Hardware.Verified && plan.Hardware.Backend == executable.Backend {
			hardwarePlan = executable
		} else {
			// A verified execution route is all-or-nothing. If rebuilding selected
			// a graph the evidence did not cover, persist the software plan.
			request.Hardware = playbackplan.HardwareRoute{}
			plan, err = playbackplan.Build(request)
			if err != nil {
				return PlaybackDecision{}, err
			}
		}
	}
	if playbackPlanRequiresSoftwareToneMapping(plan) && !settings.HDRToneMappingFilters {
		return PlaybackDecision{}, errors.New("playback is unsupported because verified FFmpeg zscale and tonemap filters are unavailable")
	}
	decision, err := playbackDecisionFromPlan(plan, hardwarePlan, factsDigest, item, selectedAudioID, selectedSubtitleID, subtitleMode, policy, settings.X264Preset)
	if err != nil {
		return PlaybackDecision{}, err
	}
	if optimizedArtifact != nil {
		if decision.execution == nil || plan.Mode != playbackplan.DirectPlay {
			return PlaybackDecision{}, errPlaybackPlanBinding
		}
		decision.execution.OptimizedArtifactID = optimizedArtifact.ID
		decision.execution.OptimizedPresetID = optimizedArtifact.PresetID
		if err := decision.execution.seal(); err != nil {
			return PlaybackDecision{}, err
		}
		decision.Mode = "optimized_version"
		decision.DeliveryProfile = optimizedArtifact.PresetID
		decision.SourceKind = "optimized"
		decision.Reason = "A current optimized artifact is the highest-fidelity direct-play tuple supported by this client."
		decision.PlanDigest = decision.execution.Digest
		decision.IsProxied = true
	}
	return decision, nil
}

func remapReplacedScannerStream(previous, current []Stream, selectedID, kind string) string {
	selectedID = strings.TrimSpace(selectedID)
	if selectedID == "" {
		return ""
	}
	for _, stream := range current {
		if stream.ID == selectedID && stream.Kind == kind {
			return selectedID
		}
	}
	wasScannerSelection := false
	for _, stream := range previous {
		if stream.ID == selectedID && stream.Kind == kind && stream.SourceKind == "scanner" {
			wasScannerSelection = true
			break
		}
	}
	if !wasScannerSelection {
		return selectedID
	}
	for _, stream := range current {
		if stream.Kind == kind && stream.Default {
			return stream.ID
		}
	}
	for _, stream := range current {
		if stream.Kind == kind {
			return stream.ID
		}
	}
	return ""
}

func playbackPlanModePolicy(policy ResolvedPlaybackPolicy, directStreamRemux bool) ([]playbackplan.Mode, []playbackplan.Mode) {
	allowed := []playbackplan.Mode{playbackplan.DirectPlay, playbackplan.Remux, playbackplan.DirectStream, playbackplan.VideoTranscode}
	remove := func(targets ...playbackplan.Mode) {
		blocked := map[playbackplan.Mode]bool{}
		for _, target := range targets {
			blocked[target] = true
		}
		filtered := allowed[:0]
		for _, mode := range allowed {
			if !blocked[mode] {
				filtered = append(filtered, mode)
			}
		}
		allowed = filtered
	}
	if policy.DirectPlayPolicy == "never" {
		remove(playbackplan.DirectPlay)
	}
	if !directStreamRemux {
		remove(playbackplan.Remux, playbackplan.DirectStream)
	}
	if policy.DirectStreamPolicy == "never" {
		remove(playbackplan.Remux, playbackplan.DirectStream)
	}
	if policy.TranscodePolicy == "never" {
		remove(playbackplan.DirectStream, playbackplan.VideoTranscode)
	}
	if policy.TranscodePolicy == "require" {
		allowed = []playbackplan.Mode{playbackplan.VideoTranscode}
	}
	preferred := []playbackplan.Mode{}
	if policy.DirectPlayPolicy == "prefer" {
		preferred = append(preferred, playbackplan.DirectPlay)
	}
	if policy.DirectStreamPolicy == "prefer" {
		preferred = append(preferred, playbackplan.Remux, playbackplan.DirectStream)
	}
	if policy.TranscodePolicy == "prefer" {
		preferred = append(preferred, playbackplan.VideoTranscode)
	}
	return allowed, preferred
}

func playbackPlanRequiresSoftwareToneMapping(plan playbackplan.Plan) bool {
	for _, stage := range plan.Stages {
		if stage.Operation == "tone_map_sdr" && stage.Execution != "hardware" {
			return true
		}
	}
	return false
}

func playbackDecisionFromPlan(plan playbackplan.Plan, hardwarePlan *playbackhw.Plan, factsDigest string, item MediaItem, selectedAudioID, selectedSubtitleID, subtitleMode string, policy ResolvedPlaybackPolicy, x264Preset string) (PlaybackDecision, error) {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return PlaybackDecision{}, err
	}
	quality := "original"
	if plan.Mode == playbackplan.VideoTranscode {
		quality = normalizeTranscodeQuality(policy.DeliveryProfile)
	}
	audioMode := ""
	videoAction, audioAction := "", ""
	for _, stream := range plan.Streams {
		switch stream.Kind {
		case "video":
			videoAction = string(stream.Action)
		case "audio":
			audioAction = string(stream.Action)
			if stream.Action == playbackplan.Convert {
				audioMode = "transcode"
			}
		}
	}
	binding := &playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: plan.SourceRevision, MediaFactsDigest: factsDigest,
		CapabilityEvidenceID: plan.CapabilityEvidenceID, Generation: int(plan.Timeline.Generation),
		Mode: string(plan.Mode), Protocol: plan.Protocol, Container: plan.Container, Quality: quality,
		AudioMode: audioMode, AudioStreamID: normalizeSelectedAudioStreamID(selectedAudioID),
		SubtitleMode: strings.ToLower(strings.TrimSpace(subtitleMode)), SubtitleStreamID: strings.TrimSpace(selectedSubtitleID),
		DirectStream: plan.Mode == playbackplan.Remux || plan.Mode == playbackplan.DirectStream, Plan: encoded,
		X264Preset:   safeX264Preset(x264Preset),
		HardwarePlan: hardwarePlan,
	}
	if binding.SubtitleMode != "text" && binding.SubtitleMode != "burn_in" {
		binding.SubtitleMode, binding.SubtitleStreamID = "off", ""
	}
	if err := binding.seal(); err != nil {
		return PlaybackDecision{}, err
	}
	reasons := make([]string, 0, len(plan.Reasons))
	for _, reason := range plan.Reasons {
		reasons = append(reasons, string(reason))
	}
	decision := PlaybackDecision{
		Mode: string(plan.Mode), Reason: playbackPlanReason(plan), ReasonCodes: reasons,
		SourceKind: playbackSourceKind(item.SourceURL), Protocol: plan.Protocol, Container: plan.Container,
		AudioCodec: plan.Audio.Codec, DeliveryProfile: quality, PlanSchemaVersion: binding.SchemaVersion,
		PlanDigest: binding.Digest, SourceRevision: binding.SourceRevision,
		CapabilityEvidenceID: binding.CapabilityEvidenceID, Generation: binding.Generation,
		VideoAction: videoAction, AudioAction: audioAction, SubtitleAction: string(plan.Subtitle.Action), execution: binding,
	}
	if plan.Hardware.Verified {
		decision.HardwareBackend = string(plan.Hardware.Backend)
	}
	for _, action := range plan.Streams {
		if action.Kind == "video" {
			decision.VideoCodec = action.OutputCodec
			break
		}
	}
	switch plan.Mode {
	case playbackplan.DirectPlay:
		decision.Mode = "direct_play"
	case playbackplan.Remux:
		decision.Mode, decision.RequiresRemux = "direct_stream", true
	case playbackplan.DirectStream:
		decision.Mode, decision.RequiresRemux, decision.RequiresTranscode, decision.AudioTranscode = "direct_stream", true, true, true
	case playbackplan.VideoTranscode:
		decision.Mode, decision.RequiresTranscode, decision.VideoTranscode = "transcode_required", true, true
		decision.AudioTranscode = audioMode == "transcode"
	case playbackplan.Unsupported:
		decision.Mode = "unavailable"
	}
	decision.IsProxied = decision.Mode != "direct_play" || strings.EqualFold(decision.SourceKind, "remote")
	decision.IsServerCached = false
	return decision, nil
}

func playbackPlanReason(plan playbackplan.Plan) string {
	if len(plan.Reasons) == 0 {
		return "Server selected the highest-fidelity compatible playback route."
	}
	return "Server selected a compatible route: " + strings.ReplaceAll(string(plan.Reasons[0]), "_", " ") + "."
}

func playbackSourceKind(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return "remote"
	}
	return "local"
}

func playbackStreamIndex(streams []Stream, id, kind string) (int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, false
	}
	for position, stream := range streams {
		if stream.ID == id && stream.Kind == kind {
			if stream.Index >= 0 {
				return stream.Index, true
			}
			return 100000 + position, true
		}
	}
	return 0, false
}

func (s *Server) mediaFactsForPlayback(ctx context.Context, item MediaItem) (mediafacts.Facts, string, error) {
	fileID := selectedPlaybackVersionID(item)
	if facts, digest, ok := s.persistedPlaybackFacts(ctx, item, fileID); ok {
		return facts, digest, nil
	}
	if _, _, err := parseRemoteStorageLocator(strings.TrimSpace(item.SourceURL)); err == nil {
		releaseProbeLock := acquireRemotePlaybackProbeLock(item.ID + "\x00" + fileID)
		defer releaseProbeLock()
		if facts, digest, ok := s.persistedPlaybackFacts(ctx, item, fileID); ok {
			return facts, digest, nil
		}
		if err := s.probeRemotePlaybackFacts(ctx, item, item.SourceURL); err != nil {
			return mediafacts.Facts{}, "", fmt.Errorf("%w: remote probe failed: %v", errPlaybackFactsUnavailable, err)
		}
		if facts, digest, ok := s.persistedPlaybackFacts(ctx, item, fileID); ok {
			return facts, digest, nil
		}
		return mediafacts.Facts{}, "", errPlaybackFactsUnavailable
	}
	return s.estimatedPlaybackFacts(ctx, item, fileID)
}

func (s *Server) persistedPlaybackFacts(ctx context.Context, item MediaItem, fileID string) (mediafacts.Facts, string, bool) {
	var raw, digest, storedRevision, storedFingerprint string
	identityCurrent := true
	var err error
	if fileID == "" {
		err = s.queryUserRow(ctx, `SELECT facts_json, facts_digest FROM media_analysis_facts WHERE media_id = ? AND media_file_id = ''`, item.ID).Scan(&raw, &digest)
	} else {
		var currentFingerprint, currentModTime string
		var currentSize int64
		err = s.queryUserRow(ctx, `SELECT facts.facts_json, facts.facts_digest, facts.source_revision, facts.source_fingerprint,
			COALESCE(file.content_fingerprint, ''), COALESCE(file.size_bytes, 0), COALESCE(file.mod_time, '')
			FROM media_analysis_facts facts JOIN media_files file ON file.id=facts.media_file_id AND file.media_id=facts.media_id
			WHERE facts.media_id=? AND facts.media_file_id=? AND file.available=1`, item.ID, fileID).
			Scan(&raw, &digest, &storedRevision, &storedFingerprint, &currentFingerprint, &currentSize, &currentModTime)
		if err == nil {
			current := canonicalAnalysisFileIdentity(fileID, currentFingerprint, currentSize, currentModTime)
			identityCurrent = storedFingerprint == current.Fingerprint && storedRevision == current.revision()
		}
	}
	if err == nil && identityCurrent {
		var facts mediafacts.Facts
		if json.Unmarshal([]byte(raw), &facts) == nil {
			canonical, canonicalErr := facts.Canonical()
			actualDigest, digestErr := canonical.Digest()
			if canonicalErr == nil && digestErr == nil && actualDigest == digest {
				canonical = augmentPlaybackSubtitleFacts(canonical, item.Streams)
				actualDigest, _ = canonical.Digest()
				return canonical, actualDigest, true
			}
		}
	}
	return mediafacts.Facts{}, "", false
}

func (s *Server) estimatedPlaybackFacts(ctx context.Context, item MediaItem, fileID string) (mediafacts.Facts, string, error) {
	var fingerprint, container, modTime string
	var sizeBytes int64
	if fileID != "" {
		_ = s.queryUserRow(ctx, `SELECT COALESCE(content_fingerprint, ''), COALESCE(container, ''), COALESCE(size_bytes, 0), COALESCE(mod_time, '') FROM media_files WHERE id = ? AND media_id = ?`, fileID, item.ID).Scan(&fingerprint, &container, &sizeBytes, &modTime)
	}
	if container == "" {
		for _, version := range item.MediaFiles {
			if version.ID == fileID || fileID == "" && version.Selected {
				container, sizeBytes, modTime = version.Container, version.SizeBytes, version.ModTime
				break
			}
		}
	}
	container = canonicalPlaybackContainer(container, item.SourceURL)
	seed, _ := json.Marshal(struct {
		MediaID, FileID, Fingerprint, Container, ModTime string
		SizeBytes                                        int64
		Streams                                          []Stream
	}{item.ID, fileID, fingerprint, container, modTime, sizeBytes, item.Streams})
	sum := sha256.Sum256(seed)
	derived := hex.EncodeToString(sum[:])
	if strings.TrimSpace(fingerprint) == "" {
		fingerprint = "estimated:sha256:" + derived
	}
	facts := mediafacts.Facts{Version: mediafacts.SchemaVersion, Source: mediafacts.Source{Fingerprint: fingerprint, Revision: "estimated:" + derived, SizeBytes: sizeBytes}, Container: container, DurationUS: int64(max(0, item.DurationSeconds)) * 1_000_000, DurationConfidence: mediafacts.ConfidenceEstimated, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown}
	var streamDuration *mediafacts.Rational
	streamDurationConfidence := mediafacts.ConfidenceUnknown
	if item.DurationSeconds > 0 {
		streamDuration = &mediafacts.Rational{Num: int64(item.DurationSeconds), Den: 1}
		streamDurationConfidence = mediafacts.ConfidenceEstimated
	}
	used := map[int]bool{}
	for position, stream := range item.Streams {
		index := stream.Index
		if index < 0 || used[index] {
			index = 100000 + position
		}
		used[index] = true
		disposition := mediafacts.Disposition{Default: stream.Default, Forced: stream.Forced, HearingImpaired: stream.HearingImpaired}
		switch stream.Kind {
		case "video":
			if stream.Width <= 0 || stream.Height <= 0 || strings.TrimSpace(stream.DolbyVisionProfile) != "" {
				return mediafacts.Facts{}, "", errPlaybackFactsUnavailable
			}
			frameRate := rationalFromFloat(stream.FrameRate)
			if frameRate.Den == 0 {
				frameRate = mediafacts.Rational{Num: 30, Den: 1}
			}
			dar := rationalFromAspect(stream.AspectRatio, stream.Width, stream.Height)
			pixelFormat := firstNonEmpty(strings.ToLower(strings.TrimSpace(stream.PixelFormat)), "unknown")
			facts.Video = append(facts.Video, mediafacts.Video{Index: index, Bitrate: int64(max(0, stream.Bitrate)), Codec: firstNonEmpty(normalizeCodec(stream.Codec), "unknown"), Profile: stream.Profile, Level: playbackLevelString(stream.Level), CodedWidth: stream.Width, CodedHeight: stream.Height, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: dar, PixelFormat: pixelFormat, BitDepth: stream.BitDepth, ChromaSubsampling: chromaFromPixelFormat(pixelFormat), ColorPrimaries: stream.ColorPrimaries, ColorTransfer: stream.ColorTransfer, ColorMatrix: stream.ColorSpace, FieldOrder: stream.FieldOrder, FrameRate: frameRate, AverageFrameRate: &frameRate, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, Timing: mediafacts.Timing{Duration: streamDuration, DurationConfidence: streamDurationConfidence}, Disposition: disposition})
		case "audio":
			if stream.Channels <= 0 {
				continue
			}
			facts.Audio = append(facts.Audio, mediafacts.Audio{Index: index, Bitrate: int64(max(0, stream.Bitrate)), Codec: firstNonEmpty(normalizeCodec(stream.Codec), "unknown"), Profile: stream.Profile, Layout: stream.ChannelLayout, Channels: stream.Channels, SampleRate: stream.SampleRate, Language: stream.Language, Disposition: disposition, Timing: mediafacts.Timing{Duration: streamDuration, DurationConfidence: streamDurationConfidence}})
		case "subtitle":
			codec := normalizeCodec(stream.Codec)
			sdh := stream.HearingImpaired
			facts.Subtitles = append(facts.Subtitles, mediafacts.Subtitle{Index: index, Codec: firstNonEmpty(codec, "unknown"), Kind: playbackSubtitleFactKind(codec), SDH: &sdh, Language: stream.Language, Disposition: disposition, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
		}
	}
	if len(facts.Video) == 0 && len(facts.Audio) == 0 {
		return mediafacts.Facts{}, "", errPlaybackFactsUnavailable
	}
	canonical, err := facts.Canonical()
	if err != nil {
		return mediafacts.Facts{}, "", fmt.Errorf("%w: %v", errPlaybackFactsUnavailable, err)
	}
	digest, err := canonical.Digest()
	return canonical, digest, err
}

func playbackLevelString(level int) string {
	if level <= 0 {
		return ""
	}
	return strconv.Itoa(level)
}

func augmentPlaybackSubtitleFacts(facts mediafacts.Facts, streams []Stream) mediafacts.Facts {
	seen := map[int]bool{}
	for _, subtitle := range facts.Subtitles {
		seen[subtitle.Index] = true
	}
	for position, stream := range streams {
		if stream.Kind != "subtitle" || stream.Index >= 0 && seen[stream.Index] {
			continue
		}
		index := stream.Index
		if index < 0 || seen[index] {
			index = 100000 + position
		}
		seen[index] = true
		sdh := stream.HearingImpaired
		codec := normalizeCodec(stream.Codec)
		facts.Subtitles = append(facts.Subtitles, mediafacts.Subtitle{Index: index, Codec: firstNonEmpty(codec, "unknown"), Kind: playbackSubtitleFactKind(codec), SDH: &sdh, Language: stream.Language, Disposition: mediafacts.Disposition{Default: stream.Default, Forced: stream.Forced, HearingImpaired: stream.HearingImpaired}, Timing: mediafacts.Timing{DurationConfidence: mediafacts.ConfidenceUnknown}})
	}
	canonical, err := facts.Canonical()
	if err == nil {
		return canonical
	}
	return facts
}

func playbackSubtitleFactKind(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "hdmv_pgs_subtitle", "pgs", "dvd_subtitle", "vobsub", "dvb_subtitle", "xsub":
		return "bitmap"
	default:
		return "text"
	}
}

func canonicalPlaybackContainer(container, source string) string {
	value := strings.ToLower(strings.TrimSpace(container))
	switch value {
	case "mkv":
		return "matroska"
	case "m4v", "mov":
		return "mp4"
	case "ts", "m2ts":
		return "mpegts"
	}
	if value != "" {
		return value
	}
	if parsed, err := url.Parse(strings.TrimSpace(source)); err == nil {
		ext := strings.TrimPrefix(strings.ToLower(pathExtension(parsed.Path)), ".")
		if ext != "" {
			return canonicalPlaybackContainer(ext, "")
		}
	}
	return "unknown"
}

func pathExtension(path string) string {
	if index := strings.LastIndex(path, "."); index >= 0 {
		return path[index:]
	}
	return ""
}

func rationalFromFloat(value float64) mediafacts.Rational {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return mediafacts.Rational{}
	}
	return mediafacts.Rational{Num: int64(math.Round(value * 1000)), Den: 1000}
}

func rationalFromAspect(value string, width, height int) mediafacts.Rational {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool { return r == ':' || r == '/' })
	if len(parts) == 2 {
		n, nErr := strconv.ParseInt(parts[0], 10, 64)
		d, dErr := strconv.ParseInt(parts[1], 10, 64)
		if nErr == nil && dErr == nil && n > 0 && d > 0 {
			return mediafacts.Rational{Num: n, Den: d}
		}
	}
	if width > 0 && height > 0 {
		return mediafacts.Rational{Num: int64(width), Den: int64(height)}
	}
	return mediafacts.Rational{Num: 1, Den: 1}
}
