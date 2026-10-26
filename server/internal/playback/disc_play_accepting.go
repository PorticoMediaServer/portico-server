package playback

import (
	"context"
	"database/sql"

	"portico.local/server/internal/disc"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playbackdisc"
)

// DiscPlayContinuation is process-private current-POST custody. Its zero value
// refuses. It cannot be constructed from a snapshot/preparation ID or a public
// request, and is never returned on the wire. Close follows the OUTER accepting
// commit/rollback; no reader may launch using its preparation descriptor.
type DiscPlayContinuation struct {
	service *Service
	borrow  *discPlayBorrow
}

// continuationContext is the original factory's accepting handoff. Readiness
// alone does not issue it: this live POST must supply its separate proof and
// hold the resulting borrow through both real session writers.
func (e *discPlayFactoryEntry) continuationContext(ctx context.Context, service *Service, current playbackdisc.PreparationContinuationCheck) (context.Context, bool, error) {
	p, ready, err := e.prepared(ctx)
	if err != nil || !ready {
		return ctx, false, err
	}
	if service == nil || service.discInputs == nil || service.db != p.db || service.discInputs.store != p.store || service.discInputs.captures != p.captures {
		return ctx, false, disc.ErrProtocol
	}
	b, err := p.borrowPrepared(ctx, current)
	if err != nil {
		return ctx, false, err
	}
	bound, err := b.withContext(ctx)
	if err != nil {
		_ = b.release(context.WithoutCancel(ctx))
		return ctx, false, err
	}
	return bound, true, nil
}

// DiscPlayContinuation reads only a capability already issued by the original
// factory; absence is the ordinary local path, never permission for a remote
// fallback. Every operation below rechecks exact service and borrow custody.
func (s *Service) DiscPlayContinuation(ctx context.Context) (*DiscPlayContinuation, error) {
	if ctx == nil {
		return nil, disc.ErrProtocol
	}
	b, ok := ctx.Value(discPlayBorrowKey{}).(*discPlayBorrow)
	if !ok {
		return nil, nil
	}
	c := &DiscPlayContinuation{service: s, borrow: b}
	if err := c.held(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *DiscPlayContinuation) held() error {
	if c == nil || c.service == nil || c.borrow == nil || c.borrow.owner == nil || c.service.discInputs == nil || c.service.db != c.borrow.owner.db || c.service.discInputs.store != c.borrow.owner.store || c.service.discInputs.captures != c.borrow.owner.captures {
		return disc.ErrProtocol
	}
	return c.borrow.held()
}

// OriginalStart validates the exact original caller/body/key/target. The
// returned strings are the first invocation's private candidate/physical
// selection, not a SessionView, a replacement recipe or an admission result.
func (c *DiscPlayContinuation) OriginalStart(principal identity.Principal, device, key, body, item string, part int) (candidate, asset, request string, err error) {
	if err = c.held(); err != nil {
		return
	}
	original := c.borrow.owner.start
	if !sameDiscPlayPrincipal(principal, original.Principal) || device != original.Key.DeviceID || identity.Digest(key) != original.Key.StartKeyDigest || body != original.BodyDigest || item != original.ItemID || part != original.PartIndex || original.CandidateID == "" || original.LegacyRequestID != "v1."+original.CandidateID+".1" {
		err = playbackdisc.ErrCurrent
		return
	}
	return original.CandidateID, original.AssetID, original.LegacyRequestID, nil
}

func (c *DiscPlayContinuation) CreateV1(ctx context.Context, principal identity.Principal, item, request string, choice V1Choice, replacement *SessionReplacement) (Session, error) {
	if err := c.held(); err != nil {
		return Session{}, err
	}
	return c.borrow.createLegacy(ctx, c.service, principal, item, request, choice, replacement)
}

func (c *DiscPlayContinuation) result(session string, legacy Session) (playbackdisc.PreparationAccepted, error) {
	if err := c.held(); err != nil {
		return playbackdisc.PreparationAccepted{}, err
	}
	if session != c.borrow.owner.start.CandidateID {
		return playbackdisc.PreparationAccepted{}, playbackdisc.ErrCurrent
	}
	c.borrow.measurement.mu.Lock()
	id := c.borrow.measurement.snapshot.ID
	c.borrow.measurement.mu.Unlock()
	return playbackdisc.PreparationAccepted{SessionID: session, SnapshotID: id, Legacy: playbackdisc.Key{SessionID: legacy.ID, Generation: legacy.Generation}}, nil
}

func (c *DiscPlayContinuation) ActivateTx(ctx context.Context, tx *sql.Tx, session string, legacy Session) error {
	result, err := c.result(session, legacy)
	if err != nil {
		return err
	}
	return c.borrow.activateTx(ctx, tx, result)
}

func (c *DiscPlayContinuation) AcceptTx(ctx context.Context, tx *sql.Tx, session string, legacy Session) error {
	result, err := c.result(session, legacy)
	if err != nil {
		return err
	}
	return c.borrow.acceptTx(ctx, tx, result)
}

// Close releases only the original preparation reference, never an accepted
// capture or another presentation. It must run outside SQL after settlement.
func (c *DiscPlayContinuation) Close(ctx context.Context) error {
	if c == nil || c.borrow == nil {
		return disc.ErrProtocol
	}
	return c.borrow.release(ctx)
}
