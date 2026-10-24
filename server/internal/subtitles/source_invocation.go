package subtitles

import (
	"context"
	"database/sql"

	"portico.local/server/internal/identity"
)

// PlaybackSourceFacts are captured by Select after current viewer/session
// authorization. These are server-private facts, not authority by context.
// SessionID/SessionGeneration name playback_sessions and its exact generation,
// not a V1 session, physical source, queue occurrence or mapping generation.
// Size/ModifiedNS are the actual playback source pin, not an inventory revision.
type PlaybackSourceFacts struct {
	Principal         identity.Principal `json:"-"`
	ItemID            string             `json:"-"`
	AssetID           string             `json:"-"`
	SessionID         string             `json:"-"`
	SessionGeneration int                `json:"-"`
	SourceSize        int64              `json:"-"`
	SourceModifiedNS  int64              `json:"-"`
}

// PlaybackSourceInvocation is immutable outside this package. A retained source
// resolver must call CheckTx in its own capture/publication snapshots and bind
// the real source/root/observation separately. No transport is activated here.
type PlaybackSourceInvocation struct {
	facts   PlaybackSourceFacts
	checkTx func(context.Context, *sql.Tx) error
}

func (i PlaybackSourceInvocation) Facts() PlaybackSourceFacts { return i.facts }

func (i PlaybackSourceInvocation) CheckTx(ctx context.Context, tx *sql.Tx) error {
	if ctx == nil || tx == nil || i.checkTx == nil {
		return ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return i.checkTx(ctx, tx)
}

type playbackSourceInvocationKey struct{}

func PlaybackSourceInvocationFromContext(ctx context.Context) (PlaybackSourceInvocation, bool) {
	if ctx == nil {
		return PlaybackSourceInvocation{}, false
	}
	i, ok := ctx.Value(playbackSourceInvocationKey{}).(PlaybackSourceInvocation)
	return i, ok && i.checkTx != nil
}

func (s *Service) playbackSourceContext(ctx context.Context, p identity.Principal, item, session string, scope sessionScope) context.Context {
	facts := PlaybackSourceFacts{Principal: p, ItemID: item, AssetID: scope.source, SessionID: session, SessionGeneration: scope.generation, SourceSize: scope.size, SourceModifiedNS: scope.modified}
	i := PlaybackSourceInvocation{facts: facts}
	i.checkTx = func(check context.Context, tx *sql.Tx) error {
		current, err := s.authorize(check, tx, facts.Principal, facts.ItemID)
		if err != nil {
			return err
		}
		now, err := sessionTx(check, tx, current, facts.ItemID, facts.SessionID)
		if err != nil {
			return err
		}
		if !now.available || now.source != facts.AssetID || now.generation != facts.SessionGeneration || now.size != facts.SourceSize || now.modified != facts.SourceModifiedNS {
			return ErrConflict
		}
		return nil
	}
	return context.WithValue(ctx, playbackSourceInvocationKey{}, i)
}
