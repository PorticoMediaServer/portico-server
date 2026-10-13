package catalog

import (
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/identity"
)

// AcceptPlaybackProgressAt is AcceptPlaybackProgress for evidence recorded at
// an earlier moment: a play of a downloaded copy, made with no server in reach
// and reported later. It is the same writer with the same rules; only the time
// differs, and it is the time the play happened that goes into both histories.
//
// The watched intent the writer records is dated to the play as well. An
// intent is ordered against the marks a viewer makes on a whole show or album
// by its date, so one dated "now" would beat a "mark the show unwatched" that
// the viewer made after watching the episode on a plane.
func (s *Service) AcceptPlaybackProgressAt(tx *sql.Tx, v identity.Viewer, playbackID, item string, sequence int64, position, duration float64, state string, canonical bool, at time.Time) error {
	entity, e := personalID(tx, item)
	if e != nil {
		return e
	}
	profile := identity.PersonalKey(v)
	intent := func() (string, error) {
		var authored string
		e := tx.QueryRow(`SELECT authored_at FROM personal_watched_intents WHERE profile_id=? AND item_id=?`, profile, entity).Scan(&authored)
		if errors.Is(e, sql.ErrNoRows) {
			return "", nil
		}
		return authored, e
	}
	before, e := intent()
	if e != nil {
		return e
	}
	if e = acceptPlaybackProgress(tx, v, playbackID, item, sequence, position, duration, state, canonical, at.UTC()); e != nil {
		return e
	}
	after, e := intent()
	if e != nil || after == before {
		return e
	}
	_, e = tx.Exec(`UPDATE personal_watched_intents SET authored_at=? WHERE profile_id=? AND item_id=?`, at.UTC().Format("2006-01-02T15:04:05.000000000Z"), profile, entity)
	return e
}
