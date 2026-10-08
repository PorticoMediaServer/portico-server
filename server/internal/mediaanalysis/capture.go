package mediaanalysis

import (
	"context"
	"database/sql"
)

// capturedSource retains a STRM descriptor and the exact, policy-checked remote
// version while a request runs. Network IO is always outside SQL transactions.
type capturedSource struct {
	source Source
	input  Input
}

func (s *Service) Capture(ctx context.Context, a Access, t Target) (Access, func(), error) {
	noop := func() {}
	if a.captured != nil {
		return a, noop, nil
	}
	gated, e := s.begin(ctx, a)
	if e != nil {
		return a, noop, e
	}
	tx := gated.Tx()
	preflight := t
	preflight.MappingRevision = ""
	v, e := s.resolve(ctx, tx, a, preflight)
	gated.Rollback()
	if e != nil {
		return a, noop, e
	}
	if v.Container != "strm" {
		return a, noop, nil
	}
	input, e := s.options.Open(ctx, a.ItemID, v.ID)
	if e != nil {
		return a, noop, e
	}
	close := func() { _ = input.Close() }
	if e = input.Validate(ctx); e != nil {
		close()
		return a, noop, e
	}
	v.Evidence = input.Evidence()
	if assurance(v.Evidence) != "strong_version" {
		close()
		return a, noop, ErrUnsupported
	}
	a.captured = &capturedSource{v, input}
	return a, close, nil
}
func (a Access) validateCapture(ctx context.Context) error {
	if a.captured != nil {
		return a.captured.input.Validate(ctx)
	}
	return ctx.Err()
}

// RequestProbeRefresh is used only by the explicit queue/retry transaction, not
// by browsing. A changed target must pass Basic again before any derived work.
func (s *Service) RequestProbeRefresh(ctx context.Context, tx *sql.Tx, a Access, t Target) error {
	if e := s.AuthorizeTarget(ctx, tx, a, t); e != nil {
		return e
	}
	v, e := s.resolve(ctx, tx, a, t)
	if e != nil {
		return e
	}
	if v.NeedsProbe {
		_, e = tx.ExecContext(ctx, `UPDATE inventory_objects SET analysis_revision='',analysis_state='pending' WHERE id=? AND revision=?`, v.ObjectID, v.Revision)
	}
	return e
}
