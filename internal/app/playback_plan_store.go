package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackhw"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

var errPlaybackPlanBinding = errors.New("playback grant is not bound to a current execution plan")

// playbackExecutionBinding is the private, immutable execution projection of
// a canonical playback plan. HTTP handlers consume this record and never
// reconstruct behavior from mutable query parameters.
type playbackExecutionBinding struct {
	SchemaVersion        int             `json:"schemaVersion"`
	Digest               string          `json:"digest"`
	SourceRevision       string          `json:"sourceRevision"`
	MediaFactsDigest     string          `json:"mediaFactsDigest"`
	CapabilityEvidenceID string          `json:"capabilityEvidenceId"`
	Generation           int             `json:"generation"`
	Mode                 string          `json:"mode"`
	Protocol             string          `json:"protocol"`
	Container            string          `json:"container"`
	Quality              string          `json:"quality"`
	AudioMode            string          `json:"audioMode"`
	AudioStreamID        string          `json:"audioStreamId,omitempty"`
	SubtitleMode         string          `json:"subtitleMode"`
	SubtitleStreamID     string          `json:"subtitleStreamId,omitempty"`
	DirectStream         bool            `json:"directStream"`
	X264Preset           string          `json:"x264Preset"`
	Plan                 json.RawMessage `json:"plan"`
	OptimizedArtifactID  string          `json:"optimizedArtifactId,omitempty"`
	OptimizedPresetID    string          `json:"optimizedPresetId,omitempty"`
	// HardwarePlan is private execution authority. It is persisted inside the
	// sealed binding so a later request cannot re-resolve a different device.
	HardwarePlan *playbackhw.Plan `json:"hardwarePlan,omitempty"`
}

func (b playbackExecutionBinding) canonicalJSON() ([]byte, error) {
	copy := b
	copy.Digest = ""
	if copy.SchemaVersion <= 0 || strings.TrimSpace(copy.SourceRevision) == "" || copy.Generation < 0 || len(copy.Plan) == 0 || !json.Valid(copy.Plan) || safeX264Preset(copy.X264Preset) != copy.X264Preset {
		return nil, errPlaybackPlanBinding
	}
	if (copy.OptimizedArtifactID == "") != (copy.OptimizedPresetID == "") {
		return nil, errPlaybackPlanBinding
	}
	return json.Marshal(copy)
}

func (b *playbackExecutionBinding) seal() error {
	encoded, err := b.canonicalJSON()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	b.Digest = "playback-plan-v1:sha256:" + hex.EncodeToString(sum[:])
	return nil
}

func playbackPlanPersistence(decision PlaybackDecision) (playbackExecutionBinding, []byte, error) {
	if decision.execution == nil {
		return playbackExecutionBinding{}, nil, errPlaybackPlanBinding
	}
	binding := *decision.execution
	if err := binding.seal(); err != nil {
		return playbackExecutionBinding{}, nil, err
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return playbackExecutionBinding{}, nil, err
	}
	return binding, encoded, nil
}

func livePlaybackPlanPersistence(item MediaItem, decision PlaybackDecision) (playbackExecutionBinding, []byte, error) {
	mode := playbackplan.Remux
	action := playbackplan.Copy
	switch decision.Mode {
	case "direct_play":
		mode = playbackplan.DirectPlay
	case "direct_stream":
		mode = playbackplan.Remux
	case "transcode_required":
		mode = playbackplan.VideoTranscode
		action = playbackplan.Convert
	default:
		return playbackExecutionBinding{}, nil, errPlaybackPlanBinding
	}
	sourceRevision := "live:" + strings.TrimSpace(item.ID)
	capabilityEvidenceID := "live-server-hls-v1"
	plan := playbackplan.Plan{
		SchemaRevision:       playbackplan.SchemaRevision,
		SourceFingerprint:    "live-channel:" + strings.TrimSpace(item.ID),
		SourceRevision:       sourceRevision,
		CapabilityEvidenceID: capabilityEvidenceID,
		Mode:                 mode,
		Protocol:             firstNonEmpty(strings.TrimSpace(decision.Protocol), "hls"),
		Container:            firstNonEmpty(strings.TrimSpace(decision.Container), "mpegts"),
		Streams:              []playbackplan.StreamAction{{Index: 0, Kind: "video", Action: action, InputCodec: "provider", OutputCodec: map[bool]string{true: "h264"}[action == playbackplan.Convert]}},
		Timeline:             playbackplan.Timeline{Mode: "live", Dynamic: true, Generation: 0},
		Subtitle:             playbackplan.SubtitleDecision{Action: playbackplan.Drop},
	}
	plan.Digest, _ = plan.ComputeDigest()
	if err := plan.Validate(); err != nil {
		return playbackExecutionBinding{}, nil, errPlaybackPlanBinding
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return playbackExecutionBinding{}, nil, err
	}
	binding := playbackExecutionBinding{
		SchemaVersion: 1, SourceRevision: sourceRevision, CapabilityEvidenceID: capabilityEvidenceID,
		Generation: 0, Mode: string(mode), Protocol: plan.Protocol, Container: plan.Container,
		Quality: normalizeTranscodeQuality(decision.DeliveryProfile), AudioMode: "auto", SubtitleMode: "off",
		DirectStream: mode != playbackplan.DirectPlay, X264Preset: safeX264Preset("veryfast"), Plan: planJSON,
	}
	if err := binding.seal(); err != nil {
		return playbackExecutionBinding{}, nil, err
	}
	encoded, err := json.Marshal(binding)
	return binding, encoded, err
}

func playbackDecisionWithGeneration(decision PlaybackDecision, generation int) (PlaybackDecision, error) {
	if decision.execution == nil || generation < 1 {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	binding := *decision.execution
	var plan playbackplan.Plan
	if json.Unmarshal(binding.Plan, &plan) != nil || plan.Validate() != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	plan.Timeline.Generation = uint64(generation)
	digest, err := plan.ComputeDigest()
	if err != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	plan.Digest = digest
	binding.Generation = generation
	binding.Plan, err = json.Marshal(plan)
	if err != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	if err := binding.seal(); err != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	decision.Generation = generation
	decision.PlanDigest = binding.Digest
	decision.execution = &binding
	return decision, nil
}

func decodePlaybackExecutionBinding(raw string) (playbackExecutionBinding, error) {
	var binding playbackExecutionBinding
	if json.Unmarshal([]byte(raw), &binding) != nil {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	storedDigest := binding.Digest
	if err := binding.seal(); err != nil || binding.Digest != storedDigest {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	var plan playbackplan.Plan
	if json.Unmarshal(binding.Plan, &plan) != nil || plan.Validate() != nil {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	if plan.SourceRevision != binding.SourceRevision || plan.CapabilityEvidenceID != binding.CapabilityEvidenceID ||
		int(plan.Timeline.Generation) != binding.Generation || string(plan.Mode) != binding.Mode ||
		plan.Protocol != binding.Protocol || plan.Container != binding.Container {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	if plan.Hardware.Verified != (binding.HardwarePlan != nil) ||
		(binding.HardwarePlan != nil && (binding.HardwarePlan.Backend != plan.Hardware.Backend || binding.HardwarePlan.RequiresRuntimeProbe ||
			strings.TrimSpace(binding.HardwarePlan.RuntimeIdentity.ExecutablePath) == "" || strings.TrimSpace(binding.HardwarePlan.RuntimeIdentity.BinaryFingerprint) == "" ||
			strings.TrimSpace(binding.HardwarePlan.RuntimeIdentity.DeviceIdentity) == "" || strings.TrimSpace(binding.HardwarePlan.RuntimeIdentity.DriverIdentity) == "" ||
			strings.TrimSpace(binding.HardwarePlan.RuntimeIdentity.DriverVersion) == "")) {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	return binding, nil
}

func playbackDecisionFromBinding(binding playbackExecutionBinding, item MediaItem) (PlaybackDecision, error) {
	validated, err := decodePlaybackExecutionBinding(mustJSONPlaybackBinding(binding))
	if err != nil {
		return PlaybackDecision{}, err
	}
	binding = validated
	var plan playbackplan.Plan
	if json.Unmarshal(binding.Plan, &plan) != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	reasons := make([]string, 0, len(plan.Reasons))
	videoAction, audioAction := "", ""
	for _, stream := range plan.Streams {
		switch stream.Kind {
		case "video":
			videoAction = string(stream.Action)
		case "audio":
			audioAction = string(stream.Action)
		}
	}
	for _, reason := range plan.Reasons {
		reasons = append(reasons, string(reason))
	}
	decision := PlaybackDecision{
		Mode: string(plan.Mode), Reason: playbackPlanReason(plan), ReasonCodes: reasons,
		SourceKind: playbackSourceKind(item.SourceURL), Protocol: plan.Protocol, Container: plan.Container,
		AudioCodec: plan.Audio.Codec, DeliveryProfile: binding.Quality,
		PlanSchemaVersion: binding.SchemaVersion, PlanDigest: binding.Digest,
		SourceRevision: binding.SourceRevision, CapabilityEvidenceID: binding.CapabilityEvidenceID,
		Generation: binding.Generation, VideoAction: videoAction, AudioAction: audioAction,
		SubtitleAction: string(plan.Subtitle.Action), execution: &binding,
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
		if binding.OptimizedArtifactID != "" {
			decision.Mode = "optimized_version"
			decision.DeliveryProfile = binding.OptimizedPresetID
		}
	case playbackplan.Remux:
		decision.Mode, decision.RequiresRemux = "direct_stream", true
	case playbackplan.DirectStream:
		decision.Mode, decision.RequiresRemux, decision.RequiresTranscode, decision.AudioTranscode = "direct_stream", true, true, true
	case playbackplan.VideoTranscode:
		decision.Mode, decision.RequiresTranscode, decision.VideoTranscode = "transcode_required", true, true
		decision.AudioTranscode = audioAction == string(playbackplan.Convert)
	default:
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	decision.IsProxied = decision.Mode != "direct_play" || strings.EqualFold(decision.SourceKind, "remote")
	return decision, nil
}

func mustJSONPlaybackBinding(binding playbackExecutionBinding) string {
	raw, _ := json.Marshal(binding)
	return string(raw)
}

func (s *Server) playbackPlanForSession(ctx context.Context, sessionID, mediaID string) (playbackExecutionBinding, error) {
	var raw, digest, revision string
	var generation int
	if err := s.queryUserRow(ctx, `
		SELECT plan_json, plan_digest, source_revision, playback_generation
		FROM playback_sessions
		WHERE id = ? AND media_id = ? AND ended_at = '' AND state <> 'stopped'`,
		strings.TrimSpace(sessionID), strings.TrimSpace(mediaID)).Scan(&raw, &digest, &revision, &generation); err != nil {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	binding, err := decodePlaybackExecutionBinding(raw)
	if err != nil || binding.Digest != digest || binding.SourceRevision != revision || binding.Generation != generation {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	return binding, nil
}

func playbackBindingHLSParameters(binding playbackExecutionBinding, r *http.Request) (quality, burnSubtitleID, textSubtitleID, audioMode, audioStreamID string, directStream bool, err error) {
	quality, burnSubtitleID, textSubtitleID, audioMode, audioStreamID, directStream, err = playbackBindingHLSValues(binding)
	if err != nil {
		return
	}
	query := r.URL.Query()
	checks := []struct {
		name, expected string
		normalize      func(string) string
	}{
		{"quality", quality, normalizeTranscodeQuality},
		{"subtitle", burnSubtitleID, normalizeBurnInSubtitleID},
		{"textSubtitle", textSubtitleID, normalizeHLSTextSubtitleID},
		{"audio", audioMode, normalizeTranscodeAudioMode},
		{"audioStream", audioStreamID, normalizeSelectedAudioStreamID},
	}
	for _, check := range checks {
		if raw, present := query[check.name]; present && (len(raw) != 1 || check.normalize(raw[0]) != check.expected) {
			err = errPlaybackPlanBinding
			return
		}
	}
	if raw, present := query["directStream"]; present {
		if len(raw) != 1 {
			err = errPlaybackPlanBinding
			return
		}
		requested := raw[0] == "1" || strings.EqualFold(raw[0], "true")
		if requested != directStream {
			err = errPlaybackPlanBinding
			return
		}
	}
	return
}

func playbackBindingHLSValues(binding playbackExecutionBinding) (quality, burnSubtitleID, textSubtitleID, audioMode, audioStreamID string, directStream bool, err error) {
	if binding.Protocol != "hls" || binding.Mode == string(playbackplan.DirectPlay) {
		err = errPlaybackPlanBinding
		return
	}
	quality = normalizeTranscodeQuality(binding.Quality)
	audioMode = normalizeTranscodeAudioMode(binding.AudioMode)
	audioStreamID = normalizeSelectedAudioStreamID(binding.AudioStreamID)
	directStream = binding.DirectStream
	switch binding.SubtitleMode {
	case "off", "":
	case "burn_in":
		burnSubtitleID = normalizeBurnInSubtitleID(binding.SubtitleStreamID)
		if burnSubtitleID == "" {
			err = errPlaybackPlanBinding
			return
		}
	case "text":
		textSubtitleID = normalizeHLSTextSubtitleID(binding.SubtitleStreamID)
		if textSubtitleID == "" {
			err = errPlaybackPlanBinding
			return
		}
	default:
		err = errPlaybackPlanBinding
		return
	}
	return
}

func (s *Server) playbackPlanForMediaGrant(ctx context.Context, r *http.Request, mediaID string) (playbackExecutionBinding, error) {
	token := mediaGrantFromRequest(r)
	if !strings.HasPrefix(token, "ptc_mg_") || strings.TrimSpace(mediaID) == "" {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	var planJSON, grantDigest, grantRevision string
	var grantGeneration int
	err := s.queryUserRow(ctx, `
		SELECT g.plan_json, g.plan_digest, g.source_revision, g.playback_generation
		FROM playback_media_grants g
		JOIN playback_sessions ps ON ps.id = g.playback_session_id
		WHERE g.token_hash = ? AND g.resource_kind = 'media' AND g.resource_id = ?
			AND g.revoked_at = '' AND g.expires_at > ?
			AND ps.ended_at = '' AND ps.state <> 'stopped'
		LIMIT 1`, hashToken(token), strings.TrimSpace(mediaID), time.Now().UTC().Format(time.RFC3339)).Scan(
		&planJSON, &grantDigest, &grantRevision, &grantGeneration)
	if err != nil || grantDigest == "" || grantRevision == "" {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	binding, decodeErr := decodePlaybackExecutionBinding(planJSON)
	if decodeErr != nil || binding.Digest != grantDigest || binding.SourceRevision != grantRevision || binding.Generation != grantGeneration {
		return playbackExecutionBinding{}, errPlaybackPlanBinding
	}
	return binding, nil
}
