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
	var current *ReplanCeilings
	if ceilings, ok := replanCeilingsFrom(ctx); ok {
		current = &ceilings
	}
	target, replanned, err := replanTarget(ctx, tx, session, old, QualityOffers(source.Video.Height, true, enabled, policy), policy, cfg, source.Video.Height, current, true)
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
	plan.Target, plan.TargetReach = replanned.target, replanned.reach
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

// ReplanCeilings is what the caller's admission allows right now, on the
// connection its request arrived on, for a change that makes a presentation
// produce new bytes without being admitted again (a subtitle burned in or
// turned off). Remote is admission's own view of the connection, not a hint
// the client sent. MaxVideoBitrateBPS is the tightest video bitrate the member
// and the server's upload budget allow there; zero is none.
type ReplanCeilings struct {
	Remote             bool
	MaxVideoBitrateBPS int
}

type replanCeilingsKey struct{}

// WithReplanCeilings attaches them to the request that may re-plan. A v1
// presentation is not re-planned for a request that carries none.
func WithReplanCeilings(ctx context.Context, c ReplanCeilings) context.Context {
	return context.WithValue(ctx, replanCeilingsKey{}, c)
}

func replanCeilingsFrom(ctx context.Context) (ReplanCeilings, bool) {
	c, ok := ctx.Value(replanCeilingsKey{}).(ReplanCeilings)
	return c, ok
}

const reachLocal, reachRemote = "local", "remote"

// keptTarget is what a re-planned session stores for its next re-plan: a v1
// presentation its narrowed target, and any session the reach admission saw.
type keptTarget struct {
	target *QualityTarget
	reach  string
}

// replanTarget is the target a session's stored plan is planned under again:
// a subtitle burned in or turned off, a copied stream that has to be converted
// after all.
//
// A request brings what its caller is allowed right now (current): admission's
// view of the connection and the member's ceiling there. One that brings none
// is refused, whatever kind of session it names. The conversion fallback has
// no request; its current is what fallbackCeilings read for the session.
//
// A v1 presentation is planned under the ceilings it was last planned with
// (the viewer's limit, the member's remote cap, the owner's), narrowed by what
// applies now, and keeps the result: they only ever come down. The owner's
// ceilings apply as they are now; the remote ones do when admission has called
// this presentation's connection remote, at its start, at an earlier re-plan
// or for this request, or when that was never recorded. A v1 presentation
// stored before plans kept their ceilings is refused: what it started under
// cannot be recovered, and no ceiling is not an answer.
//
// A legacy session names a published rung, as it always has, and that rung's
// target is narrowed in the same way by what applies now. It keeps no target,
// only the reach, so that a later fallback knows admission has seen it remote.
func replanTarget(ctx context.Context, q sourceQuery, session string, old *DeliveryPlan, rungs []QualityRung, policy ResolvedDeliveryPolicy, cfg DeliveryConfiguration, height int, current *ReplanCeilings, request bool) (QualityTarget, keptTarget, error) {
	if request && current == nil {
		return QualityTarget{}, keptTarget{}, ErrQualityUnavailable
	}
	if old.Target != nil {
		remote, member := old.TargetReach != reachLocal, 0
		if current != nil {
			remote, member = remote || current.Remote, current.MaxVideoBitrateBPS
		}
		target, reach := narrowTarget(*old.Target, remote, member, cfg), reachLocal
		if remote {
			reach = reachRemote
		}
		return target, keptTarget{&target, reach}, nil
	}
	var v1 bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_v1_sessions WHERE media_session_id=?) OR EXISTS(SELECT 1 FROM playback_v1_prepared WHERE media_session_id=?)`, session, session).Scan(&v1); err != nil {
		return QualityTarget{}, keptTarget{}, err
	}
	rung, ok := SelectedRung(rungs, old.QualityID)
	if v1 || !ok {
		return QualityTarget{}, keptTarget{}, ErrQualityUnavailable
	}
	target := rung.Target(policy, height)
	if current == nil {
		// The fallback of a legacy session no request has shown to be remote.
		return target, keptTarget{nil, old.TargetReach}, nil
	}
	remote, reach := old.TargetReach == reachRemote || current.Remote, reachLocal
	if remote {
		reach = reachRemote
	}
	return narrowTarget(target, remote, current.MaxVideoBitrateBPS, cfg), keptTarget{nil, reach}, nil
}

// narrowTarget brings a stored target down to what applies now: the member's
// current video bitrate ceiling (zero: none) and the owner's ceilings. The
// server-wide video bitrate limit is a remote-connection limit, as it is at
// the start of a presentation. Nothing here raises a ceiling.
func narrowTarget(t QualityTarget, remote bool, member int, cfg DeliveryConfiguration) QualityTarget {
	if remote {
		t.MaxVideoBitrateBPS = narrowCeiling(t.MaxVideoBitrateBPS, cfg.MaxVideoBitrateBPS)
	}
	t.MaxVideoBitrateBPS = narrowCeiling(t.MaxVideoBitrateBPS, member)
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
