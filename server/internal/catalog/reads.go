package catalog

import (
	"context"
	"database/sql"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// Every read in this package goes through s.read(). That is what lets a
// composite handler wrap its whole body in one `dbwork.WithReadSnapshot` and
// have the fifty-odd queries underneath it join that snapshot instead of taking
// a pooled connection each.
//
// Two things follow from it, and both matter more than the connection count.
// One: the rows a page is built from all come from one WAL frame boundary, so
// the "did the catalogue move under me?" check that several composers ran twice
// is answered by construction. Two: the statements carry the request's context,
// so the lane's deadline reaches them and the per-request statement counter can
// attribute them.
//
// A read outside a request keeps working exactly as before: with no snapshot and
// no context in hand, read() is `s.db` with a background context, which is what
// every call site had.

// reader is the read surface shared by a pooled handle and a transaction.
type reader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// reads binds a context to whichever handle should serve it.
type reads struct {
	ctx context.Context
	to  reader
}

func (r reads) Query(query string, args ...any) (*sql.Rows, error) {
	return r.to.QueryContext(r.ctx, query, args...)
}

func (r reads) QueryRow(query string, args ...any) *sql.Row {
	return r.to.QueryRowContext(r.ctx, query, args...)
}

// QueryContext and QueryRowContext keep the explicit-context call sites working
// unchanged, while still resolving the snapshot.
func (r reads) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return r.resolve(ctx).QueryContext(ctx, query, args...)
}

func (r reads) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return r.resolve(ctx).QueryRowContext(ctx, query, args...)
}

func (r reads) resolve(ctx context.Context) reader {
	if tx := dbwork.Snapshot(ctx); tx != nil {
		return tx
	}
	return r.to
}

// read returns the reader for this Service's request context.
func (s *Service) read() reads {
	ctx := s.rctx
	if ctx == nil {
		ctx = context.Background()
	}
	if tx := dbwork.Snapshot(ctx); tx != nil {
		return reads{ctx: ctx, to: tx}
	}
	return reads{ctx: ctx, to: dbwork.ReadHandle(ctx, s.db)}
}

// Context returns the request context this Service was bound to, or the
// background context when it was not bound to one.
func (s *Service) Context() context.Context {
	if s.rctx == nil {
		return context.Background()
	}
	return s.rctx
}

// WithContext binds a Service to one request. The returned value shares this
// Service's database handle and its caches — only the context differs — so a
// handler can hand its deadline and its read snapshot to the catalogue without
// the catalogue having to thread a context through two hundred call sites.
func (s *Service) WithContext(ctx context.Context) *Service {
	if s == nil || ctx == nil {
		return s
	}
	clone := *s
	clone.rctx = ctx
	return &clone
}

// PrepareRead initializes the catalog's cursor signing key before a read
// snapshot opens. Rating classification runs in a background worker; a request
// never drains that queue.
func (s *Service) PrepareRead(ctx context.Context, restrictions interface{ Active() bool }) error {
	if s == nil {
		return nil
	}
	if _, err := s.cursorKey(); err != nil {
		return err
	}
	return nil
}

// WithRecommendationRestrictions scopes related-item projections alongside the
// request snapshot. It never mutates the process-wide Service.
func (s *Service) WithRecommendationRestrictions(r identity.ContentRestrictions) *Service {
	clone := *s
	clone.recRestrictions = r
	return &clone
}
