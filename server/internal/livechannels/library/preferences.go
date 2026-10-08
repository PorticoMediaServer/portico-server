package librarychannels

import (
	"context"
	"database/sql"
	"portico.local/server/internal/livechannels"
)

func (s *Store) SetPreference(ctx context.Context, a Authority, o livechannels.Owner, in livechannels.PreferenceInput) (int64, error) {
	if !validID(in.ChannelID) || in.SourceID != in.ChannelID {
		return 0, ErrInvalid
	}
	var revision int64
	e := s.transaction(ctx, a, false, func(tx *sql.Tx, scope Scope) error {
		c, e := readChannel(ctx, tx, in.ChannelID)
		if e != nil {
			return e
		}
		if !scope.permits(c.Config) {
			return ErrDenied
		}
		in.SourceID = "library:" + in.ChannelID
		revision, e = livechannels.SetPreferenceTx(ctx, tx, o, in)
		return e
	})
	return revision, e
}
