package ownedcapture

import (
	"context"
	"database/sql"

	"portico.local/server/internal/dbwork"
)

func (s *Service) checkOwnerScanTx(ctx context.Context, tx *sql.Tx, target OwnerScanTarget) error {
	if target == (OwnerScanTarget{}) {
		return nil
	}
	resolver, ok := s.producer.(OwnerScanProducer)
	if !ok || tx == nil || !target.valid() {
		return ErrCurrent
	}
	return resolver.ResolveOwnerScanTx(ctx, tx, target)
}

// Only CaptureOwnerScan constructs this request branch, after the original
// source resolver checked a fresh snapshot. The accepting producer MUST recheck
// it; process-local sealing does not confer authority or replace its SQL proof.
type ownerScanRequest struct {
	service *Service
	target  OwnerScanTarget
}

// OwnerScanTarget returns a copy for the source producer's closed admission
// branch. A true result is not permission to open bytes, nor a playback grant.
func (r Request) OwnerScanTarget() (OwnerScanTarget, bool) {
	if r.ownerScan == nil {
		return OwnerScanTarget{}, false
	}
	return r.ownerScan.target, true
}

// CaptureOwnerScan runs outside any caller transaction, just like Capture. The
// exact target is selected by the original inventory analysis invocation, never
// by finding a latest job, owner, asset or execution. No public purpose exists.
func (s *Service) CaptureOwnerScan(ctx context.Context, target OwnerScanTarget) (Receipt, error) {
	if ctx == nil || dbwork.Snapshot(ctx) != nil || !target.valid() {
		return Receipt{}, ErrInput
	}
	resolver, ok := s.producer.(OwnerScanProducer)
	if !ok {
		return Receipt{}, ErrCurrent
	}
	if !s.enter() {
		return Receipt{}, ErrCurrent
	}
	defer s.wg.Done()
	ctx, done := bindLife(ctx, s.life)
	defer done()
	view, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return Receipt{}, err
	}
	err = resolver.ResolveOwnerScanTx(ctx, view.Tx(), target)
	view.Rollback()
	if err != nil {
		return Receipt{}, err
	}
	return s.Capture(ctx, Request{AssetID: target.AssetID, ownerScan: &ownerScanRequest{service: s, target: target}})
}
