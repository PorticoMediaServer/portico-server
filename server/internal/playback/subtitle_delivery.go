package playback

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// ChangeSubtitleDeliveryTx is called inside the subtitle selection transaction,
// after its viewer, source, resource and policy fences have been checked.
// The new grant, selected subtitle and ordinary delivery plan commit together.
func (s *Service) ChangeSubtitleDeliveryTx(ctx context.Context, tx *sql.Tx, p identity.Principal, session string, resource *subtitles.Resource) error {
	old, err := loadDeliveryPlan(tx, session)
	if err != nil {
		return err
	}
	if old == nil {
		return subtitles.ErrUnavailable
	}
	source, err := loadDeliverySource(ctx, tx, old.SourceID)
	if err != nil {
		return err
	}
	if source.Video == nil {
		return subtitles.ErrUnsupported
	}
	cfg := deliveryConfiguration(s.deliverySettings)
	edge := DeliveryContextFrom(ctx)
	values := s.viewerPreferences(tx, p, edge)
	policy := ResolveDeliveryPolicy(values, edge.NetworkClass, edge.ServerLocality, edge.TransportClass, cfg)
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1`).Scan(&enabled); err != nil {
		return err
	}
	target, keep, err := replanTarget(ctx, tx, session, old, QualityOffers(source.Video.Height, true, enabled, policy), policy, cfg, source.Video.Height)
	if err != nil {
		return err
	}
	input := DeliveryInput{SourceID: source.ID, Source: &source, Container: source.Container, VideoCodec: source.Video.Codec, Duration: source.Duration, Height: source.Video.Height, Policy: policy, Target: target, Config: cfg, TranscodingEnabled: enabled, Hardware: s.selectHardware(ctx, cfg), BurnIn: resource != nil, Remote: source.Container == "strm"}
	var explicit *int
	if old.AudioStream >= 0 {
		explicit = &old.AudioStream
	}
	if err = s.completeDeliveryInputTx(ctx, tx, p, &input, values, true, explicit); err != nil {
		return err
	}
	plan, err := planDelivery(input)
	if err != nil {
		return err
	}
	if old.Trace != nil && old.Trace.Audio != nil && plan.Trace != nil && plan.Trace.Audio != nil {
		plan.Trace.Audio.ChosenBy = old.Trace.Audio.ChosenBy
	}
	if resource != nil {
		plan.BurnIn = &BurnInSpec{ResourceID: resource.ID, Revision: resource.Revision, Format: resource.Format}
		// Validate the actual graph before committing a new presentation. This
		// constructs arguments only; no probe or process is allowed in this tx.
		_, err = plan.conversionGraph(&decoder.BurnIn{File: filepath.Join(os.TempDir(), "selected.ass"), Format: resource.Format, Width: source.Video.Width, Height: source.Video.Height})
		if err != nil {
			return subtitles.ErrUnsupported
		}
	}
	if plan.Mode == "hls" {
		if err = reserveHLS(tx, session, session, &plan, cfg); err != nil {
			return err
		}
	}
	plan.SourceSize, plan.SourceModifiedNS, plan.FactsRevision = old.SourceSize, old.SourceModifiedNS, old.FactsRevision
	plan.Target = keep
	if _, err = tx.ExecContext(ctx, `DELETE FROM playback_delivery_plans WHERE session_id=?`, session); err != nil {
		return err
	}
	if err = persistDeliveryPlan(tx, session, &plan); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE playback_sessions SET mode=? WHERE id=?`, plan.Mode, session); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM playback_hls_windows WHERE session_id=?`, session); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM playback_hls_stages WHERE session_id=?`, session)
	return err
}

// replanTarget is the target a session's stored plan is planned under again:
// a subtitle burned in or turned off, a copied stream that has to be converted
// after all. A v1 presentation is planned under the ceilings it started with
// (the viewer's limit, the member's remote cap, the owner's), narrowed by the
// owner's ceilings as they are now, and keeps the result: they only ever come
// down. A v1 presentation stored before plans kept those ceilings is refused,
// because what it started under cannot be recovered and no ceiling is not an
// answer. A legacy session names a published rung, as it always has; keep is
// then nil.
func replanTarget(ctx context.Context, q sourceQuery, session string, old *DeliveryPlan, rungs []QualityRung, policy ResolvedDeliveryPolicy, cfg DeliveryConfiguration, height int) (target QualityTarget, keep *QualityTarget, err error) {
	if old.Target != nil {
		target = narrowTarget(*old.Target, policy, cfg)
		return target, &target, nil
	}
	var v1 bool
	if err = q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_v1_sessions WHERE media_session_id=?) OR EXISTS(SELECT 1 FROM playback_v1_prepared WHERE media_session_id=?)`, session, session).Scan(&v1); err != nil {
		return target, nil, err
	}
	rung, ok := SelectedRung(rungs, old.QualityID)
	if v1 || !ok {
		return target, nil, ErrQualityUnavailable
	}
	return rung.Target(policy, height), nil, nil
}

// narrowTarget brings a stored target down to the owner's ceilings as they are
// now. The server-wide video bitrate limit is a remote-connection limit, as it
// is at the start of a presentation; nothing here raises a ceiling.
func narrowTarget(t QualityTarget, policy ResolvedDeliveryPolicy, cfg DeliveryConfiguration) QualityTarget {
	if policy.NetworkClass == NetworkRemote || policy.NetworkClass == NetworkCellular {
		t.MaxVideoBitrateBPS = narrowCeiling(t.MaxVideoBitrateBPS, cfg.MaxVideoBitrateBPS)
	}
	t.MaxAudioBitrateBPS = narrowCeiling(t.MaxAudioBitrateBPS, cfg.MaxAudioBitrateBPS)
	t.MaxVideoHeight = narrowCeiling(t.MaxVideoHeight, cfg.MaxVideoHeight)
	return t
}

func (s *Service) ConfigureSubtitleDelivery(service *subtitles.Service) {
	s.Subtitles = service
	service.ChangeDeliveryTx = s.ChangeSubtitleDeliveryTx
	service.RestartDelivery = s.RestartConversion
	if s.hls != nil {
		s.hls.subtitleService = service
	}
}
