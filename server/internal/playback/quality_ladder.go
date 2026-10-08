package playback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

// QualityRung kinds. Exactly one automatic rung exists in a published set; it is
// the server deciding, and it is always first.
const (
	QualityAutomatic = "automatic"
	QualityOriginal  = "original"
	QualityFixed     = "fixed"
)

// QualityRung is one selectable delivery target. A fixed rung carries its own
// ceilings; automatic and original carry none, because their ceiling is the
// resolved policy and the source respectively.
type QualityRung struct {
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	Label               string `json:"label"`
	MaxVideoBitrateBPS  int    `json:"maxVideoBitrateBps,omitempty"`
	MaxAudioBitrateBPS  int    `json:"maxAudioBitrateBps,omitempty"`
	TargetDisplayHeight int    `json:"targetDisplayHeight,omitempty"`
	Enabled             bool   `json:"enabled"`
	Reason              string `json:"reason,omitempty"`
}

// QualityPreset is one ladder step. Audio budgets step down with the video
// budget: the top three rungs keep 192 kbps, 720p and 480p take 128, and the
// smallest rung takes 96.
type QualityPreset struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	Height             int    `json:"targetDisplayHeight"`
	VideoBitrateBPS    int    `json:"maxVideoBitrateBps"`
	MaxAudioBitrateBPS int    `json:"maxAudioBitrateBps"`
}

// QualityLadder is the published ladder, widest first. It is a fixed table, not
// a computation: a client renders exactly these names.
var QualityLadder = []QualityPreset{
	{"2160p", "4K 2160p", 2160, 20_000_000, 192_000},
	{"1440p", "1440p", 1440, 12_000_000, 192_000},
	{"1080p", "1080p", 1080, 8_000_000, 192_000},
	{"720p", "720p", 720, 4_000_000, 128_000},
	{"480p", "480p", 480, 2_000_000, 96_000},
	{"360p", "360p", 360, 1_000_000, 96_000},
}

// LadderPreset finds a rung by id.
func LadderPreset(id string) (QualityPreset, bool) {
	for _, p := range QualityLadder {
		if p.ID == id {
			return p, true
		}
	}
	return QualityPreset{}, false
}

// Rung reason codes. They travel to the client, so they are stable.
const (
	RungReasonAboveSource     = "above_source_height"
	RungReasonTranscodeOff    = "transcoding_disabled"
	RungReasonExceedsPolicy   = "exceeds_network_policy"
	RungReasonSourceUnusable  = "source_unavailable"
	RungReasonOriginalRefused = "original_exceeds_policy"
)

// QualityOffers builds the published rung set for one source.
//
// sourceHeight caps the ladder: a rung taller than the source is dropped rather
// than offered as an upscale. When the owner has turned transcoding off, only
// rungs the source already satisfies remain — in practice automatic and
// original — and every fixed rung is published disabled with its reason, so the
// client can explain the empty menu instead of inventing one.
func QualityOffers(sourceHeight int, available bool, transcoding bool, policy ResolvedDeliveryPolicy) []QualityRung {
	out := []QualityRung{{ID: "auto", Kind: QualityAutomatic, Label: "Automatic", Enabled: available}}
	if !available {
		out[0].Reason = RungReasonSourceUnusable
	}
	original := QualityRung{ID: "original", Kind: QualityOriginal, Label: "Original", Enabled: available}
	switch {
	case !available:
		original.Reason = RungReasonSourceUnusable
	case policy.MaxVideoHeight > 0 && sourceHeight > 0 && sourceHeight > policy.MaxVideoHeight && !transcoding:
		original.Enabled, original.Reason = false, RungReasonOriginalRefused
	}
	out = append(out, original)
	for _, preset := range QualityLadder {
		if sourceHeight > 0 && preset.Height > sourceHeight {
			continue
		}
		rung := QualityRung{ID: preset.ID, Kind: QualityFixed, Label: preset.Label, MaxVideoBitrateBPS: preset.VideoBitrateBPS, MaxAudioBitrateBPS: preset.MaxAudioBitrateBPS, TargetDisplayHeight: preset.Height, Enabled: true}
		switch {
		case !available:
			rung.Enabled, rung.Reason = false, RungReasonSourceUnusable
		case !transcoding:
			rung.Enabled, rung.Reason = false, RungReasonTranscodeOff
		case policy.MaxVideoHeight > 0 && preset.Height > policy.MaxVideoHeight:
			rung.Enabled, rung.Reason = false, RungReasonExceedsPolicy
		case policy.MaxVideoBitrateBPS > 0 && preset.VideoBitrateBPS > policy.MaxVideoBitrateBPS:
			rung.Enabled, rung.Reason = false, RungReasonExceedsPolicy
		}
		out = append(out, rung)
	}
	// A source shorter than the smallest rung would otherwise offer nothing to
	// convert to. Keep the smallest rung so a constrained lane still has a
	// target it can actually reach.
	if sourceHeight > 0 && len(out) == 2 {
		smallest := QualityLadder[len(QualityLadder)-1]
		rung := QualityRung{ID: smallest.ID, Kind: QualityFixed, Label: smallest.Label, MaxVideoBitrateBPS: smallest.VideoBitrateBPS, MaxAudioBitrateBPS: smallest.MaxAudioBitrateBPS, TargetDisplayHeight: sourceHeight, Enabled: available && transcoding}
		if !rung.Enabled {
			rung.Reason = RungReasonTranscodeOff
			if !available {
				rung.Reason = RungReasonSourceUnusable
			}
		}
		out = append(out, rung)
	}
	return out
}

// OffersRevision fences a rung selection. A client that names a rung also names
// the revision it read; the server refuses a selection made against a different
// ladder rather than silently applying a rung the viewer never saw.
func OffersRevision(sourceID string, rungs []QualityRung, policy ResolvedDeliveryPolicy) string {
	raw, _ := json.Marshal([]any{sourceID, rungs, policy.NetworkClass, policy.MaxVideoBitrateBPS, policy.MaxAudioBitrateBPS, policy.MaxVideoHeight, policy.AllowHDR})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// SelectedRung resolves a selection id against a published set. An unknown or
// disabled id is refused; the caller republishes the set rather than guessing.
func SelectedRung(rungs []QualityRung, id string) (QualityRung, bool) {
	if id == "" {
		id = "auto"
	}
	for _, r := range rungs {
		if r.ID == id {
			return r, r.Enabled
		}
	}
	return QualityRung{}, false
}

// QualityTarget is the rung a plan is actually built for, after the resolved
// policy has narrowed whatever the rung itself asked for.
type QualityTarget struct {
	RungID             string
	Kind               string
	MaxVideoBitrateBPS int
	MaxAudioBitrateBPS int
	MaxVideoHeight     int
	AllowHDR           bool
}

// Target narrows one rung by the resolved policy. The narrower of the two wins
// on every axis, so a fixed rung can never buy more than the network lane allows.
func (r QualityRung) Target(policy ResolvedDeliveryPolicy, sourceHeight int) QualityTarget {
	out := QualityTarget{RungID: r.ID, Kind: r.Kind, AllowHDR: policy.AllowHDR}
	narrow := func(a, b int) int {
		if a <= 0 {
			return b
		}
		if b <= 0 || a < b {
			return a
		}
		return b
	}
	switch r.Kind {
	case QualityOriginal:
		out.MaxVideoBitrateBPS, out.MaxAudioBitrateBPS, out.MaxVideoHeight = 0, 0, 0
	case QualityFixed:
		out.MaxVideoBitrateBPS = narrow(r.MaxVideoBitrateBPS, policy.MaxVideoBitrateBPS)
		out.MaxAudioBitrateBPS = narrow(r.MaxAudioBitrateBPS, policy.MaxAudioBitrateBPS)
		out.MaxVideoHeight = narrow(r.TargetDisplayHeight, policy.MaxVideoHeight)
	default:
		out.MaxVideoBitrateBPS = policy.MaxVideoBitrateBPS
		out.MaxAudioBitrateBPS = policy.MaxAudioBitrateBPS
		out.MaxVideoHeight = policy.MaxVideoHeight
	}
	if sourceHeight > 0 && out.MaxVideoHeight > sourceHeight {
		out.MaxVideoHeight = sourceHeight
	}
	return out
}

func (t QualityTarget) String() string {
	return t.RungID + ":" + strconv.Itoa(t.MaxVideoHeight) + ":" + strconv.Itoa(t.MaxVideoBitrateBPS)
}

// ErrQualityUnavailable is returned when a client names a rung this source and
// policy do not publish. The client republishes the offer set rather than
// falling back to a rung the viewer never chose.
var ErrQualityUnavailable = errors.New("The selected quality is no longer available. Choose another.")

// validQualitySelection accepts only the published rung identifiers.
func validQualitySelection(id string) bool {
	if id == "auto" || id == "original" {
		return true
	}
	_, ok := LadderPreset(id)
	return ok
}
