package playback

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// V1Choice is what a Playback Protocol v1 client asks for (spec §5.1–§5.4),
// narrowed by admin ceilings. Quality is decided only by the request and those
// ceilings: the viewer's server-side per-network presets are not consulted,
// because "Auto" is the client's own choice of a limit (spec §5.2).
type V1Choice struct {
	// AssetID is the version to play; empty plays the item's first available one.
	AssetID string
	// AudioStream is the absolute source stream index the viewer chose.
	AudioStream *int
	// Limit is false for {mode: original}. The ceilings are the client's
	// {mode: limit} values; zero means none.
	Limit              bool
	MaxVideoBitrateBPS int
	MaxAudioBitrateBPS int
	MaxVideoHeight     int
	// AdminMaxVideoBitrateBPS is the member's remote bitrate cap from access
	// admission (spec §5.3); zero means none.
	AdminMaxVideoBitrateBPS int
	// Remote is true when the request came from outside the server's LAN.
	// Remote caps (the member's and the server-wide one) apply only then.
	Remote bool
	// PrivateFor makes this a private presentation (spec §18.2): the prepared
	// next item of the given (live) presentation, sharing its stream slot and
	// serving only its first audio-render window until released.
	PrivateFor     string
	PrivateUntilMs int64
	// AudioPlanV2 pins the version 2 audio plan (spec §18.1: the client decodes)
	// to an audio presentation, from the device's audioDecode.
	AudioPlanV2 bool
	AudioDecode []AudioDecodeCap
}

func narrowCeiling(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0 || a < b:
		return a
	default:
		return b
	}
}

// target is the plan target for this choice: ceilings, never a forced rung, so
// a source within them is still played as original or copied.
func (c *V1Choice) target(policy ResolvedDeliveryPolicy, cfg DeliveryConfiguration) QualityTarget {
	out := QualityTarget{RungID: "original", Kind: QualityAutomatic, AllowHDR: policy.AllowHDR}
	if c.Limit {
		out.RungID = "limit"
		out.MaxVideoBitrateBPS, out.MaxAudioBitrateBPS, out.MaxVideoHeight = c.MaxVideoBitrateBPS, c.MaxAudioBitrateBPS, c.MaxVideoHeight
	}
	if c.remote(policy) {
		// The server-wide limit is a remote-connection limit (delivery settings
		// adapter); a LAN viewer is never capped by it.
		out.MaxVideoBitrateBPS = narrowCeiling(narrowCeiling(out.MaxVideoBitrateBPS, c.AdminMaxVideoBitrateBPS), cfg.MaxVideoBitrateBPS)
	}
	out.MaxAudioBitrateBPS = narrowCeiling(out.MaxAudioBitrateBPS, cfg.MaxAudioBitrateBPS)
	out.MaxVideoHeight = narrowCeiling(out.MaxVideoHeight, cfg.MaxVideoHeight)
	return out
}

func (c *V1Choice) remote(policy ResolvedDeliveryPolicy) bool {
	return c.Remote || policy.NetworkClass == NetworkRemote || policy.NetworkClass == NetworkCellular
}

// AdminCapped reports whether the admin cap, not the client's own request, set
// the video ceiling (the decision then says admin_cap_remote_bitrate).
func (c V1Choice) AdminCapped() bool {
	if c.AdminMaxVideoBitrateBPS <= 0 || !c.Remote {
		return false
	}
	return !c.Limit || c.MaxVideoBitrateBPS <= 0 || c.AdminMaxVideoBitrateBPS < c.MaxVideoBitrateBPS
}

// selectAsset picks the version a session plays: the chosen one when it belongs
// to the item and is available, otherwise the item's first available asset.
func (s *Service) selectAsset(ctx context.Context, item string, v1 *V1Choice) *sql.Row {
	if v1 != nil && v1.AssetID != "" {
		return dbwork.QueryRow(ctx, s.db, `SELECT a.token FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id WHERE e.public_id=pid_blob(?) AND a.token=? AND a.available=1`, item, v1.AssetID)
	}
	return dbwork.QueryRow(ctx, s.db, `SELECT a.token FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id WHERE e.public_id=pid_blob(?) AND a.available=1 ORDER BY l.part_index,a.token LIMIT 1`, item)
}

// SelectedAsset is the asset a v1 start of item plays (the chosen version, else
// the first available one), for work that must happen before the start (audio
// facts, spec §18.7).
func (s *Service) SelectedAsset(ctx context.Context, item string, v1 *V1Choice) (string, error) {
	var id string
	return id, s.selectAsset(ctx, item, v1).Scan(&id)
}

// CreateV1 starts one presentation (a playback_sessions row with its own grant)
// for a v1 session. A replacement ends the previous presentation atomically and
// takes its slot, which is how a track or quality change mints a new generation.
func (s *Service) CreateV1(ctx context.Context, p identity.Principal, item, request string, choice V1Choice, replacement *SessionReplacement) (Session, error) {
	return s.create(ctx, p, item, "auto", request, replacement, &choice)
}

// PlanV1 is what CreateV1 would decide for this device, with no side effects
// (spec §4.1 `plan`). It returns the asset it planned for.
func (s *Service) PlanV1(ctx context.Context, p identity.Principal, item string, choice V1Choice) (DeliveryPlan, error) {
	var aid string
	if e := s.selectAsset(ctx, item, &choice).Scan(&aid); e != nil {
		return DeliveryPlan{}, e
	}
	tx, done, e := dbwork.BeginRead(ctx, s.db)
	if e != nil {
		return DeliveryPlan{}, e
	}
	defer done()
	var container, video, audio string
	var duration float64
	if e = tx.QueryRow(`SELECT a.container,a.video_codec,a.audio_codec,a.duration FROM catalog_assets a WHERE a.token=?`, aid).Scan(&container, &video, &audio, &duration); e != nil {
		return DeliveryPlan{}, e
	}
	if container == "strm" {
		// A remote source is played as-is once it has been prepared at start.
		return DeliveryPlan{SourceID: aid, Mode: "remote", Strategy: DeliveryOriginal, VideoAction: "copy", AudioAction: "copy", AudioStream: -1, Duration: duration}, nil
	}
	inputs, e := s.planInputsTx(ctx, tx, p, aid, container, video, audio, duration, false, true, "auto", choice.AudioStream, &choice)
	if e != nil {
		return DeliveryPlan{}, e
	}
	plan, e := planDelivery(inputs.input)
	if e != nil {
		return DeliveryPlan{}, e
	}
	if plan.Mode == "hls" && s.hls != nil && s.hls.finite != nil {
		plan.Strategy, plan.VideoAction, plan.OutputVideoCodec = DeliveryVideoConversion, "convert", "h264"
		plan.ReasonCodes = append(plan.ReasonCodes, ReasonFiniteNormalization)
	}
	if !inputs.ownerTranscoding && (plan.AudioAction == "convert" || plan.VideoAction == "convert") {
		return plan, ErrTranscodingDisabled
	}
	return plan, nil
}

// SessionPlan is the immutable plan a presentation was created with.
func (s *Service) SessionPlan(ctx context.Context, session string) (*DeliveryPlan, error) {
	tx, done, e := dbwork.BeginRead(ctx, s.db)
	if e != nil {
		return nil, e
	}
	defer done()
	return loadDeliveryPlan(tx, session)
}

// ReadyWithin reports whether a presentation can be played now, waiting at most
// d. Production continues in the background either way (it runs on the
// service's context, not the request's), so false means "ask again".
func (s *Service) ReadyWithin(ctx context.Context, out Session, d time.Duration) (bool, error) {
	wait, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	err := s.Ready(wait, out)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return false, nil
	}
	return false, err
}

// StopV1 ends one presentation for the v1 layer, which has already decided who
// may stop it (the session's device, an admin, a lease expiry, a transfer
// commit). The grant is refused from the next media request on (invariant 4).
func (s *Service) StopV1(ctx context.Context, id string) error {
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassEstablishedPlayback)
	if err != nil {
		return err
	}
	defer gated.Rollback()
	if err = terminateControlOccurrence(ctx, gated.Tx(), id, "stopped"); err != nil {
		return err
	}
	if err = gated.Commit(); err != nil {
		return err
	}
	s.stopHLS(id)
	return nil
}

// ReleasePrivateTx makes a private presentation (spec §18.3) an ordinary one
// at commit: its grant then serves the media and every render window, and it
// takes the stream slot its owner leaves. It fails when the preparation has
// expired, converting is no longer allowed, or the source changed.
func (s *Service) ReleasePrivateTx(ctx context.Context, tx *sql.Tx, id string) error {
	var until int64
	if err := tx.QueryRowContext(ctx, `SELECT expires_ms FROM playback_private_presentations WHERE session_id=?`, id).Scan(&until); err != nil {
		return identity.ErrUnauthorized
	}
	if until <= time.Now().UnixMilli() {
		return ErrPreparationExpired
	}
	// Converting must still be allowed only for a converted plan: a direct one
	// (the client decodes the original file, spec §18.1) converts nothing.
	converted := true
	var generation int
	if err := tx.QueryRowContext(ctx, `SELECT generation FROM playback_sessions WHERE id=?`, id).Scan(&generation); err != nil {
		return err
	}
	if plan, err := readAudioPlan(tx, id, generation); err != nil {
		return err
	} else if plan != nil && plan.Mode == "direct" {
		converted = false
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1`).Scan(&allowed); err != nil {
		return err
	}
	if converted && !allowed {
		return ErrTranscodingDisabled
	}
	var changed int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM playback_sessions ps JOIN playback_source_pins pin ON pin.session_id=ps.id JOIN catalog_assets a ON a.token=pin.asset_id WHERE ps.id=? AND (a.size<>pin.size OR a.modified_ns<>pin.modified_ns OR a.available=0)`, id).Scan(&changed); err != nil {
		return err
	}
	if changed != 0 {
		return ErrSourceChanged
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM playback_private_presentations WHERE session_id=?`, id)
	return err
}

var (
	// ErrPreparationExpired: a private presentation past its expiry.
	ErrPreparationExpired = errors.New("preparation expired")
	// ErrSourceChanged: the file behind a presentation changed since it was planned.
	ErrSourceChanged = errors.New("source changed")
)
