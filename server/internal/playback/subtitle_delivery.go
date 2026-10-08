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
	rungs := QualityOffers(source.Video.Height, true, enabled, policy)
	rung, ok := SelectedRung(rungs, old.QualityID)
	if !ok {
		return ErrQualityUnavailable
	}
	input := DeliveryInput{SourceID: source.ID, Source: &source, Container: source.Container, VideoCodec: source.Video.Codec, Duration: source.Duration, Height: source.Video.Height, Policy: policy, Target: rung.Target(policy, source.Video.Height), Config: cfg, TranscodingEnabled: enabled, Hardware: s.selectHardware(ctx, cfg), BurnIn: resource != nil, Remote: source.Container == "strm"}
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

func (s *Service) ConfigureSubtitleDelivery(service *subtitles.Service) {
	s.Subtitles = service
	service.ChangeDeliveryTx = s.ChangeSubtitleDeliveryTx
	service.RestartDelivery = s.RestartConversion
	if s.hls != nil {
		s.hls.subtitleService = service
	}
}
