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

const playbackExecutionPlanSchemaVersion = 2

// playbackExecutionPlan is the one private, immutable authority handed from
// playback planning to persistence, grants, URLs, diagnostics, and execution.
// playbackplan.Plan owns all semantic delivery facts. The remaining fields are
// selections that the pure planner cannot know: stable application stream IDs,
// an execution preset, and exact optimized/hardware artifacts.
type playbackExecutionPlan struct {
	SchemaVersion       int               `json:"schemaVersion"`
	Digest              string            `json:"digest"`
	MediaFactsDigest    string            `json:"mediaFactsDigest,omitempty"`
	Quality             string            `json:"quality"`
	AudioStreamID       string            `json:"audioStreamId,omitempty"`
	SubtitleStreamID    string            `json:"subtitleStreamId,omitempty"`
	X264Preset          string            `json:"x264Preset,omitempty"`
	Plan                playbackplan.Plan `json:"plan"`
	OptimizedArtifactID string            `json:"optimizedArtifactId,omitempty"`
	OptimizedPresetID   string            `json:"optimizedPresetId,omitempty"`
	// HardwarePlan contains the exact verified executable/device identity. The
	// canonical Plan intentionally carries only the privacy-safe public route.
	HardwarePlan *playbackhw.Plan `json:"hardwarePlan,omitempty"`
}

func (p playbackExecutionPlan) generation() int { return int(p.Plan.Timeline.Generation) }

func (p playbackExecutionPlan) audioMode() string {
	for _, stream := range p.Plan.Streams {
		if stream.Kind == "audio" && stream.Action == playbackplan.Convert {
			return "transcode"
		}
	}
	return ""
}

func (p playbackExecutionPlan) subtitleMode() string {
	switch p.Plan.Subtitle.Action {
	case playbackplan.ExternalText:
		return "text"
	case playbackplan.BurnIn:
		return "burn_in"
	default:
		return "off"
	}
}

func (p playbackExecutionPlan) directStream() bool {
	return p.Plan.Mode == playbackplan.Remux || p.Plan.Mode == playbackplan.DirectStream
}

func (p playbackExecutionPlan) requiresX264Preset() bool {
	if p.HardwarePlan != nil {
		return false
	}
	for _, stream := range p.Plan.Streams {
		if stream.Kind == "video" && stream.Action == playbackplan.Convert && normalizeCodec(stream.OutputCodec) == "h264" {
			return true
		}
	}
	return false
}

func (p playbackExecutionPlan) canonicalJSON() ([]byte, error) {
	copy := p
	copy.Digest = ""
	if copy.SchemaVersion != playbackExecutionPlanSchemaVersion || copy.Plan.Validate() != nil || copy.Plan.Mode == playbackplan.Unsupported ||
		copy.generation() < 0 ||
		normalizeTranscodeQuality(copy.Quality) != copy.Quality || normalizeSelectedAudioStreamID(copy.AudioStreamID) != copy.AudioStreamID {
		return nil, errPlaybackPlanBinding
	}
	if (copy.requiresX264Preset() && (copy.X264Preset == "" || safeX264Preset(copy.X264Preset) != copy.X264Preset)) ||
		(!copy.requiresX264Preset() && copy.X264Preset != "") {
		return nil, errPlaybackPlanBinding
	}
	if (copy.OptimizedArtifactID == "") != (copy.OptimizedPresetID == "") ||
		(copy.OptimizedArtifactID != "" && copy.Plan.Mode != playbackplan.DirectPlay) {
		return nil, errPlaybackPlanBinding
	}
	switch copy.subtitleMode() {
	case "off":
		if strings.TrimSpace(copy.SubtitleStreamID) != "" {
			return nil, errPlaybackPlanBinding
		}
	case "text":
		if normalizeHLSTextSubtitleID(copy.SubtitleStreamID) == "" || normalizeHLSTextSubtitleID(copy.SubtitleStreamID) != copy.SubtitleStreamID {
			return nil, errPlaybackPlanBinding
		}
	case "burn_in":
		if normalizeBurnInSubtitleID(copy.SubtitleStreamID) == "" || normalizeBurnInSubtitleID(copy.SubtitleStreamID) != copy.SubtitleStreamID {
			return nil, errPlaybackPlanBinding
		}
	}
	if copy.Plan.Hardware.Verified != (copy.HardwarePlan != nil) ||
		(copy.HardwarePlan != nil && (copy.HardwarePlan.Backend != copy.Plan.Hardware.Backend || copy.HardwarePlan.RequiresRuntimeProbe ||
			strings.TrimSpace(copy.HardwarePlan.RuntimeIdentity.ExecutablePath) == "" || strings.TrimSpace(copy.HardwarePlan.RuntimeIdentity.BinaryFingerprint) == "" ||
			strings.TrimSpace(copy.HardwarePlan.RuntimeIdentity.DeviceIdentity) == "" || strings.TrimSpace(copy.HardwarePlan.RuntimeIdentity.DriverIdentity) == "" ||
			strings.TrimSpace(copy.HardwarePlan.RuntimeIdentity.DriverVersion) == "")) {
		return nil, errPlaybackPlanBinding
	}
	return json.Marshal(copy)
}

func (p *playbackExecutionPlan) seal() error {
	encoded, err := p.canonicalJSON()
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	p.Digest = "playback-execution-plan-v2:sha256:" + hex.EncodeToString(sum[:])
	return nil
}

func playbackPlanPersistence(decision PlaybackDecision) (playbackExecutionPlan, []byte, error) {
	if decision.executionPlan == nil {
		return playbackExecutionPlan{}, nil, errPlaybackPlanBinding
	}
	plan := *decision.executionPlan
	if err := plan.seal(); err != nil {
		return playbackExecutionPlan{}, nil, err
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return playbackExecutionPlan{}, nil, err
	}
	return plan, encoded, nil
}

// livePlaybackPlanPersistence adapts the dynamic provider decision to the same
// persisted schema. Live planning remains independently owned; this adapter is
// deliberately not a call into the VOD builder (PORTICO-QA-F067).
func livePlaybackPlanPersistence(item MediaItem, decision PlaybackDecision) (playbackExecutionPlan, []byte, error) {
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
		return playbackExecutionPlan{}, nil, errPlaybackPlanBinding
	}
	plan := playbackplan.Plan{
		SchemaRevision:       playbackplan.SchemaRevision,
		SourceFingerprint:    "live-channel:" + strings.TrimSpace(item.ID),
		SourceRevision:       "live:" + strings.TrimSpace(item.ID),
		CapabilityEvidenceID: "live-server-hls-v1",
		Mode:                 mode,
		Protocol:             firstNonEmpty(strings.TrimSpace(decision.Protocol), "hls"),
		Container:            firstNonEmpty(strings.TrimSpace(decision.Container), "mpegts"),
		Streams:              []playbackplan.StreamAction{{Index: 0, Kind: "video", Action: action, InputCodec: "provider", OutputCodec: map[bool]string{true: "h264"}[action == playbackplan.Convert]}},
		Timeline:             playbackplan.Timeline{Mode: "live", Dynamic: true, Generation: 0},
		Subtitle:             playbackplan.SubtitleDecision{Action: playbackplan.Drop},
	}
	plan.Digest, _ = plan.ComputeDigest()
	if err := plan.Validate(); err != nil {
		return playbackExecutionPlan{}, nil, errPlaybackPlanBinding
	}
	execution := playbackExecutionPlan{
		SchemaVersion: playbackExecutionPlanSchemaVersion,
		Quality:       normalizeTranscodeQuality(decision.DeliveryProfile),
		Plan:          plan,
	}
	if execution.requiresX264Preset() {
		execution.X264Preset = safeX264Preset("veryfast")
	}
	if err := execution.seal(); err != nil {
		return playbackExecutionPlan{}, nil, err
	}
	encoded, err := json.Marshal(execution)
	return execution, encoded, err
}

func playbackDecisionWithGeneration(decision PlaybackDecision, item MediaItem, generation int) (PlaybackDecision, error) {
	if decision.executionPlan == nil || generation < 1 {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	execution := *decision.executionPlan
	execution.Plan = execution.Plan.Clone()
	execution.Plan.Timeline.Generation = uint64(generation)
	digest, err := execution.Plan.ComputeDigest()
	if err != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	execution.Plan.Digest = digest
	if err := execution.seal(); err != nil {
		return PlaybackDecision{}, errPlaybackPlanBinding
	}
	return playbackDecisionFromExecutionPlan(execution, item)
}

func decodePlaybackExecutionPlan(raw string) (playbackExecutionPlan, error) {
	var plan playbackExecutionPlan
	if json.Unmarshal([]byte(raw), &plan) != nil {
		return playbackExecutionPlan{}, errPlaybackPlanBinding
	}
	storedDigest := plan.Digest
	if err := plan.seal(); err != nil || plan.Digest != storedDigest {
		return playbackExecutionPlan{}, errPlaybackPlanBinding
	}
	return plan, nil
}

func playbackDecisionFromExecutionPlan(execution playbackExecutionPlan, item MediaItem) (PlaybackDecision, error) {
	validated, err := decodePlaybackExecutionPlan(mustJSONPlaybackExecutionPlan(execution))
	if err != nil {
		return PlaybackDecision{}, err
	}
	execution = validated
	plan := execution.Plan
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
		AudioCodec: plan.Audio.Codec, DeliveryProfile: execution.Quality,
		PlanSchemaVersion: execution.SchemaVersion, PlanDigest: execution.Digest,
		SourceRevision: plan.SourceRevision, CapabilityEvidenceID: plan.CapabilityEvidenceID,
		Generation: execution.generation(), VideoAction: videoAction, AudioAction: audioAction,
		SubtitleAction: string(plan.Subtitle.Action), executionPlan: &execution,
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
		if execution.OptimizedArtifactID != "" {
			decision.Mode = "optimized_version"
			decision.DeliveryProfile = execution.OptimizedPresetID
			decision.SourceKind = "optimized"
			decision.Reason = "A current optimized artifact is the highest-fidelity direct-play tuple supported by this client."
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

func mustJSONPlaybackExecutionPlan(plan playbackExecutionPlan) string {
	raw, _ := json.Marshal(plan)
	return string(raw)
}

func (s *Server) playbackPlanForSession(ctx context.Context, sessionID, mediaID string) (playbackExecutionPlan, error) {
	var raw, digest, revision string
	var generation int
	if err := s.queryUserRow(ctx, `
		SELECT plan_json, plan_digest, source_revision, playback_generation
		FROM playback_sessions
		WHERE id = ? AND media_id = ? AND ended_at = '' AND state <> 'stopped'`,
		strings.TrimSpace(sessionID), strings.TrimSpace(mediaID)).Scan(&raw, &digest, &revision, &generation); err != nil {
		return playbackExecutionPlan{}, errPlaybackPlanBinding
	}
	plan, err := decodePlaybackExecutionPlan(raw)
	if err != nil || plan.Digest != digest || plan.Plan.SourceRevision != revision || plan.generation() != generation {
		return playbackExecutionPlan{}, errPlaybackPlanBinding
	}
	return plan, nil
}

func playbackPlanHLSParameters(plan playbackExecutionPlan, r *http.Request) (quality, burnSubtitleID, textSubtitleID, audioMode, audioStreamID string, directStream bool, err error) {
	quality, burnSubtitleID, textSubtitleID, audioMode, audioStreamID, directStream, err = playbackPlanHLSValues(plan)
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
		}
	}
	return
}

func playbackPlanHLSValues(plan playbackExecutionPlan) (quality, burnSubtitleID, textSubtitleID, audioMode, audioStreamID string, directStream bool, err error) {
	if plan.Plan.Protocol != "hls" || plan.Plan.Mode == playbackplan.DirectPlay {
		err = errPlaybackPlanBinding
		return
	}
	quality = plan.Quality
	audioMode = plan.audioMode()
	audioStreamID = plan.AudioStreamID
	directStream = plan.directStream()
	switch plan.subtitleMode() {
	case "off":
	case "burn_in":
		burnSubtitleID = plan.SubtitleStreamID
	case "text":
		textSubtitleID = plan.SubtitleStreamID
	default:
		err = errPlaybackPlanBinding
	}
	return
}

func (s *Server) playbackPlanForMediaGrant(ctx context.Context, r *http.Request, mediaID string) (playbackExecutionPlan, error) {
	token := mediaGrantFromRequest(r)
	if !strings.HasPrefix(token, "ptc_mg_") || strings.TrimSpace(mediaID) == "" {
		return playbackExecutionPlan{}, errPlaybackPlanBinding
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
		return playbackExecutionPlan{}, errPlaybackPlanBinding
	}
	plan, decodeErr := decodePlaybackExecutionPlan(planJSON)
	if decodeErr != nil || plan.Digest != grantDigest || plan.Plan.SourceRevision != grantRevision || plan.generation() != grantGeneration {
		return playbackExecutionPlan{}, errPlaybackPlanBinding
	}
	return plan, nil
}
