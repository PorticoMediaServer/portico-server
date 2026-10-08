package downloads

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/preparedmedia"
)

// SourceDescription is the untouched file behind an item: what a client gets if
// it downloads `original`.
type SourceDescription struct {
	Container       string  `json:"container"`
	VideoCodec      string  `json:"videoCodec,omitempty"`
	AudioCodec      string  `json:"audioCodec,omitempty"`
	Height          int     `json:"height,omitempty"`
	DurationSeconds float64 `json:"durationSeconds"`
	Bytes           int64   `json:"bytes"`
	Available       bool    `json:"available"`
}

// QualityOption is one thing a client may ask to download. estimatedBytes is
// exact for the original and for a rung that is already prepared; everywhere
// else it is bitrate times duration and `estimated` says so.
type QualityOption struct {
	Quality            string `json:"quality"`
	Label              string `json:"label"`
	Kind               string `json:"kind"`
	TargetHeight       int    `json:"targetDisplayHeight,omitempty"`
	MaxVideoBitrateBPS int    `json:"maxVideoBitrateBps,omitempty"`
	MaxAudioBitrateBPS int    `json:"maxAudioBitrateBps,omitempty"`
	Bytes              int64  `json:"estimatedBytes"`
	Estimated          bool   `json:"estimated"`
	Available          bool   `json:"available"`
	Reason             string `json:"reason,omitempty"`
	RequiresPrepare    bool   `json:"requiresPreparation"`
	PreparedProfileID  string `json:"preparedProfileId,omitempty"`
	PreparedVersionID  string `json:"preparedVersionId,omitempty"`
	ExistingID         string `json:"preparationId,omitempty"`
	ExistingState      string `json:"preparationState,omitempty"`
}

// DownloadPolicy is what this profile is allowed to do, published next to the
// options so a client renders a disabled control with a reason rather than
// discovering the refusal on submit.
type DownloadPolicy struct {
	AllowDownloads bool   `json:"allowDownloads"`
	Reason         string `json:"reason,omitempty"`
}

// DownloadStorage is how much more the server is willing to prepare.
type DownloadStorage struct {
	MaxPreparedBytes int64  `json:"maxPreparedBytes"`
	CommittedBytes   int64  `json:"committedBytes"`
	RemainingBytes   *int64 `json:"remainingBytes"`
	RetentionDays    int    `json:"retentionDays"`
}

// DownloadOptionsView answers one item's download menu in a single read: the
// source, every rung with its estimate, the policy and the storage headroom. A
// client needs no second request to render the sheet.
type DownloadOptionsView struct {
	ItemID  string            `json:"itemId"`
	Kind    string            `json:"kind"`
	Source  SourceDescription `json:"source"`
	Options []QualityOption   `json:"options"`
	Policy  DownloadPolicy    `json:"policy"`
	Storage DownloadStorage   `json:"storage"`
}

// Options publishes the download menu for one item. owner says whether this
// viewer can order the conversion a rung needs; a viewer who cannot sees the
// rung as unavailable rather than being allowed to queue work that will never
// start.
func (s *Service) Options(ctx context.Context, p identity.Principal, item string, owner bool) (DownloadOptionsView, error) {
	out := DownloadOptionsView{ItemID: item, Options: []QualityOption{}}
	if !validID.MatchString(item) {
		return out, ErrInput
	}
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	allowed, e := ProfileAllowsDownloads(tx, p.Viewer)
	if e != nil {
		return out, e
	}
	out.Policy = DownloadPolicy{AllowDownloads: allowed}
	if !allowed {
		out.Policy.Reason = ReasonNotAllowed
	}
	settings, e := readSettings(tx)
	if e != nil {
		return out, e
	}
	var committed sql.NullInt64
	if e = tx.QueryRowContext(ctx, `SELECT sum(bytes_total) FROM download_preparations WHERE state IN `+committedStates).Scan(&committed); e != nil {
		return out, e
	}
	out.Storage = DownloadStorage{MaxPreparedBytes: settings.MaxPreparedBytes, CommittedBytes: committed.Int64, RetentionDays: settings.RetentionDays}
	if settings.MaxPreparedBytes > 0 {
		remaining := settings.MaxPreparedBytes - committed.Int64
		if remaining < 0 {
			remaining = 0
		}
		out.Storage.RemainingBytes = &remaining
	}
	found, err := resolveSource(ctx, tx, item)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	out.Kind = found.Kind
	out.Source = SourceDescription{Container: found.Container, VideoCodec: found.VideoCodec, AudioCodec: found.AudioCodec, Height: found.Height, DurationSeconds: found.Duration, Bytes: found.Size, Available: found.Available && found.Parts == 1}
	transcoding := false
	if e = tx.QueryRowContext(ctx, `SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1`).Scan(&transcoding); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	versions, e := preparedmedia.VersionsTx(ctx, tx, item)
	if e != nil {
		return out, e
	}
	live := map[string][2]string{}
	rows, e := tx.QueryContext(ctx, `SELECT quality,id,state FROM download_preparations WHERE profile_key=? AND item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND state IN `+committedStates, operations.ViewerKey(p), item)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var quality, id, state string
		if e = rows.Scan(&quality, &id, &state); e != nil {
			rows.Close()
			return out, e
		}
		live[quality] = [2]string{id, state}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}

	original := QualityOption{Quality: QualityOriginal, Label: "Original", Kind: "source", Bytes: found.Size, Available: out.Source.Available && allowed, TargetHeight: found.Height}
	if !out.Source.Available {
		original.Reason = ReasonSourceMissing
	} else if !allowed {
		original.Reason = ReasonNotAllowed
	}
	out.Options = append(out.Options, decorate(original, live))

	// An audio source has exactly one optimized target. Walking the video ladder
	// for it would publish six copies of the same AAC recipe under names like
	// "4K 2160p", which describes nothing about an audio file.
	presets := playback.QualityLadder
	if audioKind(found.Kind) {
		preset, _ := playback.LadderPreset(canonicalAudioRung)
		presets = []playback.QualityPreset{preset}
	}
	for _, preset := range presets {
		// A rung taller than the source would be an upscale, not a download.
		if found.Height > 0 && preset.Height > found.Height && !audioKind(found.Kind) {
			continue
		}
		option := QualityOption{
			Quality: preset.ID, Label: preset.Label, Kind: "optimized",
			TargetHeight: preset.Height, MaxVideoBitrateBPS: preset.VideoBitrateBPS, MaxAudioBitrateBPS: preset.MaxAudioBitrateBPS,
			PreparedProfileID: preparedProfile(found.Kind, preset),
		}
		if audioKind(found.Kind) {
			option.Label, option.TargetHeight, option.MaxVideoBitrateBPS, option.MaxAudioBitrateBPS = "AAC 192 kbps", 0, 0, audioRecipeBitrateBPS
		}
		option.Bytes, option.Estimated = estimateBytes(preset.ID, found)
		version := selectableVersion(versions, option.PreparedProfileID)
		switch {
		case !out.Source.Available:
			option.Reason = ReasonSourceMissing
		case !allowed:
			option.Reason = ReasonNotAllowed
		case version != nil:
			option.Available, option.Estimated, option.Bytes = true, false, version.Size
			option.PreparedVersionID = version.ID
		case !transcoding:
			option.Reason = playback.RungReasonTranscodeOff
		case owner:
			option.Available, option.RequiresPrepare = true, true
		default:
			option.Reason = ReasonNotOptimized
		}
		out.Options = append(out.Options, decorate(option, live))
	}
	return out, gated.Commit()
}

func decorate(option QualityOption, live map[string][2]string) QualityOption {
	if entry, ok := live[option.Quality]; ok {
		option.ExistingID, option.ExistingState = entry[0], entry[1]
	}
	return option
}

func selectableVersion(versions []preparedmedia.Version, profile string) *preparedmedia.Version {
	for i := range versions {
		v := &versions[i]
		if v.ProfileID == profile && v.Selectable && validDigest.MatchString(v.Digest) {
			return v
		}
	}
	return nil
}

// ladderRung resolves a published rung id.
func ladderRung(quality string) (playback.QualityPreset, bool) { return playback.LadderPreset(quality) }
