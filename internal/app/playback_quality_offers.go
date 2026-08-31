package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	playbackQualityOfferContractID    = "PC-PLAYBACK"
	playbackQualityOfferSchemaVersion = "quality-offers.v1"

	playbackQualityModeAutomatic = "automatic"
	playbackQualityModeExplicit  = "explicit"

	playbackQualityKindAutomatic = "automatic"
	playbackQualityKindOriginal  = "original"
	playbackQualityKindFixed     = "fixed"
)

var (
	errPlaybackQualityOfferInput       = errors.New("playback quality offer input is incomplete")
	errPlaybackQualitySelectionInvalid = errors.New("playback quality selection is invalid")
	ErrExplicitQualityUnavailable      = errors.New("explicit quality selection is unavailable")
)

// PlaybackQualitySelection is the complete client quality choice. Automatic
// is intentionally separate from an explicit server-issued offer: clients do
// not submit delivery presets, bitrate ceilings, or display geometry.
type PlaybackQualitySelection struct {
	Mode          string `json:"mode"`
	SelectionID   string `json:"selectionId,omitempty"`
	OfferRevision string `json:"qualityOfferRevision,omitempty"`
}

// PlaybackQualityOffer is display data plus the exact resolved delivery
// ceiling owned by the Server. selectionId is opaque to clients; the target
// fields are descriptive output facts, not request parameters.
type PlaybackQualityOffer struct {
	SelectionID         string `json:"selectionId"`
	Label               string `json:"label"`
	Kind                string `json:"kind"`
	MaxVideoBitrateBps  int64  `json:"maxVideoBitrateBps,omitempty"`
	MaxAudioBitrateBps  int64  `json:"maxAudioBitrateBps,omitempty"`
	TargetDisplayHeight int    `json:"targetDisplayHeight,omitempty"`
}

// PlaybackQualityOfferSet is revision-bound to the exact media version and
// source facts used to issue its ordered offers.
type PlaybackQualityOfferSet struct {
	ContractID     string                 `json:"contractId"`
	SchemaVersion  string                 `json:"schemaVersion"`
	MediaID        string                 `json:"mediaId"`
	VersionID      string                 `json:"versionId"`
	SourceRevision string                 `json:"sourceRevision"`
	OfferRevision  string                 `json:"offerRevision"`
	Offers         []PlaybackQualityOffer `json:"offers"`
}

// ExplicitQualityUnavailableError carries the one refreshed offer set that a
// first-party client may present after a stale, missing, or mismatched choice.
type ExplicitQualityUnavailableError struct {
	Offers PlaybackQualityOfferSet
}

func playbackQualityStartError(err error) *playbackStartHTTPError {
	var unavailable *ExplicitQualityUnavailableError
	if errors.As(err, &unavailable) {
		return &playbackStartHTTPError{
			status: http.StatusUnprocessableEntity, code: "explicit_quality_unavailable",
			message: "The selected quality is no longer available. Choose one of the refreshed offers or Automatic.",
			details: map[string]any{"qualityOffers": unavailable.Offers},
		}
	}
	return &playbackStartHTTPError{status: http.StatusBadRequest, code: "invalid_playback_quality", message: "quality must be Automatic or an exact server-issued explicit offer."}
}

func (e *ExplicitQualityUnavailableError) Error() string {
	return ErrExplicitQualityUnavailable.Error()
}

func (e *ExplicitQualityUnavailableError) Unwrap() error {
	return ErrExplicitQualityUnavailable
}

type playbackQualityOfferIssue struct {
	Item           MediaItem
	VersionID      string
	SourceRevision string
	Policy         ResolvedPlaybackPolicy
	AllowTranscode bool
}

// playbackResolvedQuality is private execution input. The public selectionId
// is never decoded; validation finds it in the current issued authority and
// returns this exact target for the planner to seal into an ExecutionPlan.
type playbackResolvedQuality struct {
	Kind                string
	PresetID            string
	MaxVideoBitrateBps  int64
	MaxAudioBitrateBps  int64
	TargetDisplayHeight int
}

type playbackQualityOfferAuthority struct {
	set     PlaybackQualityOfferSet
	targets map[string]playbackResolvedQuality
}

type playbackQualityPreset struct {
	label  string
	target playbackResolvedQuality
}

func issuePlaybackQualityOffers(input playbackQualityOfferIssue) (playbackQualityOfferAuthority, error) {
	mediaID := strings.TrimSpace(input.Item.ID)
	versionID := strings.TrimSpace(input.VersionID)
	sourceRevision := strings.TrimSpace(input.SourceRevision)
	if mediaID == "" || versionID == "" || sourceRevision == "" {
		return playbackQualityOfferAuthority{}, errPlaybackQualityOfferInput
	}

	authority := newPlaybackQualityOfferAuthority(mediaID, versionID, sourceRevision)
	authority.add("Automatic", playbackResolvedQuality{Kind: playbackQualityKindAutomatic})
	if originalQualityWithinPolicy(input.Item, input.Policy) {
		authority.add("Original Quality", playbackResolvedQuality{Kind: playbackQualityKindOriginal})
	}
	if input.AllowTranscode && input.Policy.TranscodePolicy != "never" {
		for _, preset := range playbackQualityPresets(input.Item.Type) {
			if playbackQualityTargetWithinPolicy(preset.target, input.Policy) && playbackQualityTargetWithinSource(preset.target, input.Item) {
				authority.add(preset.label, preset.target)
			}
		}
	}
	return authority.finalize()
}

func newPlaybackQualityOfferAuthority(mediaID, versionID, sourceRevision string) playbackQualityOfferAuthority {
	return playbackQualityOfferAuthority{
		set: PlaybackQualityOfferSet{
			ContractID: playbackQualityOfferContractID, SchemaVersion: playbackQualityOfferSchemaVersion,
			MediaID: strings.TrimSpace(mediaID), VersionID: strings.TrimSpace(versionID), SourceRevision: strings.TrimSpace(sourceRevision),
			Offers: []PlaybackQualityOffer{},
		},
		targets: map[string]playbackResolvedQuality{},
	}
}

func (authority playbackQualityOfferAuthority) finalize() (playbackQualityOfferAuthority, error) {
	if authority.set.MediaID == "" || authority.set.VersionID == "" || authority.set.SourceRevision == "" {
		return playbackQualityOfferAuthority{}, errPlaybackQualityOfferInput
	}
	if len(authority.set.Offers) < 2 {
		return playbackQualityOfferAuthority{}, fmt.Errorf("%w: fewer than two authorized offers", errPlaybackQualityOfferInput)
	}
	revisionMaterial, err := json.Marshal(struct {
		ContractID     string                 `json:"contractId"`
		SchemaVersion  string                 `json:"schemaVersion"`
		MediaID        string                 `json:"mediaId"`
		VersionID      string                 `json:"versionId"`
		SourceRevision string                 `json:"sourceRevision"`
		Offers         []PlaybackQualityOffer `json:"offers"`
	}{
		ContractID: authority.set.ContractID, SchemaVersion: authority.set.SchemaVersion,
		MediaID: authority.set.MediaID, VersionID: authority.set.VersionID,
		SourceRevision: authority.set.SourceRevision, Offers: authority.set.Offers,
	})
	if err != nil {
		return playbackQualityOfferAuthority{}, err
	}
	authority.set.OfferRevision = playbackQualityOpaqueID("qrev", revisionMaterial)
	return authority, nil
}

// issueContinuousPlaybackQualityOffers projects the Server's existing
// continuous-HLS presets into the same opaque revisioned authority used by
// ordinary media. The returned private PresetID is the only bridge to the HLS
// URL/grant implementation; clients never receive or submit it.
func issueContinuousPlaybackQualityOffers(mediaID, versionMaterial, sourceMaterial string, policy ResolvedPlaybackPolicy, fixedIDs []string, allowOriginal bool) (playbackQualityOfferAuthority, error) {
	authority := newPlaybackQualityOfferAuthority(
		mediaID,
		playbackQualityOpaqueID("qver", []byte(versionMaterial)),
		playbackQualityOpaqueID("qsrc", []byte(sourceMaterial)),
	)
	authority.add("Automatic", playbackResolvedQuality{Kind: playbackQualityKindAutomatic})
	if allowOriginal && resolvedMaxVideoBitrateBps(policy) <= 0 && policy.MaxVideoHeight <= 0 {
		authority.add("Original Quality", playbackResolvedQuality{Kind: playbackQualityKindOriginal})
	}
	if policy.TranscodePolicy != "never" {
		for _, id := range fixedIDs {
			constraint := liveTVQualityConstraintFor(id)
			preset := transcodePresets[constraint.id]
			target := playbackResolvedQuality{
				Kind: playbackQualityKindFixed, PresetID: constraint.id,
				MaxVideoBitrateBps: int64(constraint.maxBandwidth), MaxAudioBitrateBps: int64(preset.audioK) * 1_000,
				TargetDisplayHeight: constraint.maxHeight,
			}
			if target.PresetID == "auto" || target.PresetID == "source" || !playbackQualityTargetWithinPolicy(target, policy) {
				continue
			}
			authority.add(playbackVideoQualityLabel(target.TargetDisplayHeight, int(target.MaxVideoBitrateBps/1_000)), target)
		}
	}
	return authority.finalize()
}

func resolveContinuousPlaybackQualityForRequest(authority playbackQualityOfferAuthority, selection PlaybackQualitySelection, policy ResolvedPlaybackPolicy, profile PlaybackClientProfile) (playbackResolvedQuality, ResolvedPlaybackPolicy, PlaybackClientProfile, string, error) {
	quality, resolvedPolicy, resolvedProfile, err := resolvePlaybackQualityForRequest(authority, selection, policy, profile, "live_channel")
	if err != nil {
		return playbackResolvedQuality{}, policy, profile, "", err
	}
	selected := liveTVQualityForResolvedPolicy(policy)
	switch quality.Kind {
	case playbackQualityKindOriginal:
		selected = "source"
	case playbackQualityKindFixed:
		selected = quality.PresetID
	}
	return quality, resolvedPolicy, resolvedProfile, selected, nil
}

func (s *Server) issuePlaybackQualityOffersForItem(ctx context.Context, item MediaItem, policy ResolvedPlaybackPolicy, allowTranscode bool) (playbackQualityOfferAuthority, error) {
	facts, _, err := s.mediaFactsForPlayback(ctx, item)
	if err != nil {
		return playbackQualityOfferAuthority{}, err
	}
	rawRevision := strings.TrimSpace(facts.Source.Revision)
	if rawRevision == "" {
		return playbackQualityOfferAuthority{}, errPlaybackQualityOfferInput
	}
	versionID := strings.TrimSpace(selectedPlaybackVersionID(item))
	if versionID == "" {
		versionID = playbackQualityOpaqueID("qver", []byte(item.ID+"\x00"+facts.Source.Fingerprint))
	}
	return issuePlaybackQualityOffers(playbackQualityOfferIssue{
		Item: item, VersionID: versionID,
		SourceRevision: playbackQualityOpaqueID("qsrc", []byte(rawRevision)),
		Policy:         policy, AllowTranscode: allowTranscode,
	})
}

func normalizedPlaybackQualitySelection(selection PlaybackQualitySelection) PlaybackQualitySelection {
	selection.Mode = strings.ToLower(strings.TrimSpace(selection.Mode))
	selection.SelectionID = strings.TrimSpace(selection.SelectionID)
	selection.OfferRevision = strings.TrimSpace(selection.OfferRevision)
	if selection.Mode == "" {
		selection.Mode = playbackQualityModeAutomatic
	}
	return selection
}

func playbackQualitySelectionForResolved(authority playbackQualityOfferAuthority, quality playbackResolvedQuality) (PlaybackQualitySelection, error) {
	if quality.Kind == playbackQualityKindAutomatic {
		return PlaybackQualitySelection{Mode: playbackQualityModeAutomatic}, nil
	}
	for selectionID, target := range authority.targets {
		if target == quality {
			return PlaybackQualitySelection{
				Mode: playbackQualityModeExplicit, SelectionID: selectionID, OfferRevision: authority.set.OfferRevision,
			}, nil
		}
	}
	return PlaybackQualitySelection{}, &ExplicitQualityUnavailableError{Offers: authority.set}
}

func resolvePlaybackQualityForRequest(authority playbackQualityOfferAuthority, selection PlaybackQualitySelection, policy ResolvedPlaybackPolicy, profile PlaybackClientProfile, mediaType string) (playbackResolvedQuality, ResolvedPlaybackPolicy, PlaybackClientProfile, error) {
	selection = normalizedPlaybackQualitySelection(selection)
	quality, err := resolvePlaybackQualitySelection(authority, selection)
	if err != nil {
		return playbackResolvedQuality{}, policy, profile, err
	}
	policy = applyResolvedPlaybackQuality(policy, quality, mediaType)
	if quality.Kind != playbackQualityKindAutomatic {
		profile = applyResolvedPlaybackPolicy(profile, mediaType, policy)
	}
	return quality, policy, profile, nil
}

func (a *playbackQualityOfferAuthority) add(label string, target playbackResolvedQuality) {
	if strings.TrimSpace(label) == "" || !target.valid() {
		return
	}
	material, _ := json.Marshal(struct {
		Kind                string `json:"kind"`
		MaxVideoBitrateBps  int64  `json:"maxVideoBitrateBps,omitempty"`
		MaxAudioBitrateBps  int64  `json:"maxAudioBitrateBps,omitempty"`
		TargetDisplayHeight int    `json:"targetDisplayHeight,omitempty"`
	}{
		Kind: target.Kind, MaxVideoBitrateBps: target.MaxVideoBitrateBps,
		MaxAudioBitrateBps: target.MaxAudioBitrateBps, TargetDisplayHeight: target.TargetDisplayHeight,
	})
	selectionID := playbackQualityOpaqueID("qsel", material)
	if _, duplicate := a.targets[selectionID]; duplicate {
		return
	}
	a.targets[selectionID] = target
	a.set.Offers = append(a.set.Offers, PlaybackQualityOffer{
		SelectionID: selectionID, Label: label, Kind: target.Kind,
		MaxVideoBitrateBps:  target.MaxVideoBitrateBps,
		MaxAudioBitrateBps:  target.MaxAudioBitrateBps,
		TargetDisplayHeight: target.TargetDisplayHeight,
	})
}

func (q playbackResolvedQuality) valid() bool {
	switch q.Kind {
	case playbackQualityKindAutomatic, playbackQualityKindOriginal:
		return q.PresetID == "" && q.MaxVideoBitrateBps == 0 && q.MaxAudioBitrateBps == 0 && q.TargetDisplayHeight == 0
	case playbackQualityKindFixed:
		return strings.TrimSpace(q.PresetID) != "" &&
			(q.MaxVideoBitrateBps > 0 || q.MaxAudioBitrateBps > 0 || q.TargetDisplayHeight > 0)
	default:
		return false
	}
}

func playbackQualityOpaqueID(prefix string, material []byte) string {
	digest := sha256.Sum256(material)
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(digest[:18])
}

func resolvePlaybackQualitySelection(authority playbackQualityOfferAuthority, selection PlaybackQualitySelection) (playbackResolvedQuality, error) {
	mode := strings.ToLower(strings.TrimSpace(selection.Mode))
	switch mode {
	case playbackQualityModeAutomatic:
		if strings.TrimSpace(selection.SelectionID) != "" || strings.TrimSpace(selection.OfferRevision) != "" {
			return playbackResolvedQuality{}, errPlaybackQualitySelectionInvalid
		}
		return playbackResolvedQuality{Kind: playbackQualityKindAutomatic}, nil
	case playbackQualityModeExplicit:
		selectionID := strings.TrimSpace(selection.SelectionID)
		revision := strings.TrimSpace(selection.OfferRevision)
		if selectionID == "" || revision == "" || revision != authority.set.OfferRevision {
			return playbackResolvedQuality{}, &ExplicitQualityUnavailableError{Offers: authority.set}
		}
		target, ok := authority.targets[selectionID]
		if !ok || target.Kind == playbackQualityKindAutomatic {
			return playbackResolvedQuality{}, &ExplicitQualityUnavailableError{Offers: authority.set}
		}
		return target, nil
	default:
		return playbackResolvedQuality{}, errPlaybackQualitySelectionInvalid
	}
}

// validateResolvedPlaybackQualityExecution is the final quality assertion at
// the boundary where the immutable ExecutionPlan takes ownership. It never
// chooses a fallback: a plan that does not implement the resolved explicit
// offer fails with the same refreshed offer set used for stale selections.
func validateResolvedPlaybackQualityExecution(authority playbackQualityOfferAuthority, quality playbackResolvedQuality, decision PlaybackDecision) error {
	if quality.Kind == playbackQualityKindAutomatic {
		return nil
	}
	unavailable := func() error {
		return &ExplicitQualityUnavailableError{Offers: authority.set}
	}
	if decision.executionPlan == nil {
		return unavailable()
	}
	if quality.Kind == playbackQualityKindOriginal {
		if decision.VideoTranscode || decision.executionPlan.Quality != "original" {
			return unavailable()
		}
		return nil
	}
	if quality.Kind != playbackQualityKindFixed {
		return unavailable()
	}
	plan := decision.executionPlan
	if plan.Quality != quality.PresetID ||
		plan.Plan.Constraints.MaxVideoBitrate != quality.MaxVideoBitrateBps ||
		plan.Plan.Constraints.MaxAudioBitrate != quality.MaxAudioBitrateBps ||
		plan.Plan.Constraints.MaxHeight != quality.TargetDisplayHeight {
		return unavailable()
	}
	return nil
}

func applyResolvedPlaybackQuality(policy ResolvedPlaybackPolicy, quality playbackResolvedQuality, mediaType string) ResolvedPlaybackPolicy {
	policy.explicitQuality = quality
	switch quality.Kind {
	case playbackQualityKindAutomatic:
		policy.explicitQuality = playbackResolvedQuality{}
		return policy
	case playbackQualityKindOriginal:
		policy.QualityProfile = "original"
		if isAudioMediaType(mediaType) {
			policy.DeliveryProfile = "audio-original"
		} else {
			policy.DeliveryProfile = "video-original"
		}
	case playbackQualityKindFixed:
		policy.QualityProfile = quality.PresetID
		policy.DeliveryProfile = quality.PresetID
		if quality.MaxVideoBitrateBps > 0 {
			policy.maxVideoBitrateBps = minPositiveInt64(resolvedMaxVideoBitrateBps(policy), quality.MaxVideoBitrateBps)
		}
		if quality.MaxAudioBitrateBps > 0 {
			policy.maxAudioBitrateBps = minPositiveInt64(resolvedMaxAudioBitrateBps(policy), quality.MaxAudioBitrateBps)
		}
		if quality.TargetDisplayHeight > 0 {
			policy.MaxVideoHeight = minPositive(policy.MaxVideoHeight, quality.TargetDisplayHeight)
		}
	}
	return policy
}

func explicitPlaybackDeliveryProfile(policy ResolvedPlaybackPolicy, mediaType string) (string, bool) {
	quality := policy.explicitQuality
	if !quality.valid() || quality.Kind == playbackQualityKindAutomatic {
		return "", false
	}
	if quality.Kind == playbackQualityKindOriginal {
		if isAudioMediaType(mediaType) {
			return "audio-original", true
		}
		return "video-original", true
	}
	return quality.PresetID, true
}

func resolvedMaxVideoBitrateBps(policy ResolvedPlaybackPolicy) int64 {
	return minPositiveInt64(int64(max(0, policy.MaxVideoBitrateMbps))*1_000_000, policy.maxVideoBitrateBps)
}

func resolvedMaxAudioBitrateBps(policy ResolvedPlaybackPolicy) int64 {
	return minPositiveInt64(int64(max(0, policy.MaxAudioBitrateKbps))*1_000, policy.maxAudioBitrateBps)
}

func minPositiveInt64(current, candidate int64) int64 {
	if candidate <= 0 {
		return current
	}
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}

func playbackQualityPresets(mediaType string) []playbackQualityPreset {
	if isAudioMediaType(mediaType) {
		return []playbackQualityPreset{
			playbackAudioQualityPreset("audio-high"),
			playbackAudioQualityPreset("audio-standard"),
			playbackAudioQualityPreset("audio-data-saver"),
		}
	}
	ids := []string{
		"video-high", "1080p-high", "1080p-medium", "1080p-standard", "1080p-low",
		"720p-high", "720p-medium", "720p-standard", "720p-low", "480p",
	}
	result := make([]playbackQualityPreset, 0, len(ids))
	for _, id := range ids {
		preset := transcodePresets[id]
		result = append(result, playbackQualityPreset{
			label: playbackVideoQualityLabel(preset.height, preset.videoK),
			target: playbackResolvedQuality{
				Kind: playbackQualityKindFixed, PresetID: id,
				MaxVideoBitrateBps:  int64(preset.videoK) * 1_000,
				MaxAudioBitrateBps:  int64(preset.audioK) * 1_000,
				TargetDisplayHeight: preset.height,
			},
		})
	}
	return result
}

func playbackAudioQualityPreset(id string) playbackQualityPreset {
	preset := transcodePresets[id]
	return playbackQualityPreset{
		label: playbackAudioQualityLabel(preset.audioK, ""),
		target: playbackResolvedQuality{
			Kind: playbackQualityKindFixed, PresetID: id,
			MaxAudioBitrateBps: int64(preset.audioK) * 1_000,
		},
	}
}

func playbackQualityTargetWithinPolicy(target playbackResolvedQuality, policy ResolvedPlaybackPolicy) bool {
	if target.Kind != playbackQualityKindFixed {
		return true
	}
	if maximum := resolvedMaxVideoBitrateBps(policy); maximum > 0 && target.MaxVideoBitrateBps > maximum {
		return false
	}
	if maximum := resolvedMaxAudioBitrateBps(policy); maximum > 0 && target.MaxAudioBitrateBps > maximum {
		return false
	}
	return policy.MaxVideoHeight <= 0 || target.TargetDisplayHeight <= 0 || target.TargetDisplayHeight <= policy.MaxVideoHeight
}

func playbackQualityTargetWithinSource(target playbackResolvedQuality, item MediaItem) bool {
	if target.Kind != playbackQualityKindFixed || target.TargetDisplayHeight <= 0 || isAudioMediaType(item.Type) {
		return true
	}
	sourceHeight := sourceVideoHeight(item, 0)
	return sourceHeight <= 0 || target.TargetDisplayHeight <= sourceHeight
}

func originalQualityWithinPolicy(item MediaItem, policy ResolvedPlaybackPolicy) bool {
	audioOnly := isAudioMediaType(item.Type)
	maxVideoBitrateBps := resolvedMaxVideoBitrateBps(policy)
	maxAudioBitrateBps := resolvedMaxAudioBitrateBps(policy)
	for _, stream := range item.Streams {
		switch stream.Kind {
		case "video":
			if audioOnly {
				continue
			}
			if policy.MaxVideoHeight > 0 && (stream.Height <= 0 || stream.Height > policy.MaxVideoHeight) {
				return false
			}
			if maxVideoBitrateBps > 0 {
				bitrate := int64(stream.Bitrate)
				if bitrate <= 0 || bitrate > maxVideoBitrateBps {
					return false
				}
			}
		case "audio":
			if !audioOnly {
				continue
			}
			if maxAudioBitrateBps > 0 {
				bitrate := int64(stream.Bitrate)
				if bitrate <= 0 || bitrate > maxAudioBitrateBps {
					return false
				}
			}
		}
	}
	return true
}
