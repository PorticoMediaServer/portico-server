package catalog

import (
	"errors"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

var ErrListeningPreferencesConflict = errors.New("listening preferences changed; refresh before saving")

type ListeningPreferences struct {
	Revision       int64   `json:"revision"`
	MusicRate      float64 `json:"musicRate"`
	BookRate       float64 `json:"bookRate"`
	AutoplayNext   bool    `json:"autoplayNext"`
	PassoutMinutes int     `json:"passoutMinutes"`
}

func listeningPreferences(snapshot operations.PreferenceSnapshot) ListeningPreferences {
	out := ListeningPreferences{Revision: 1, MusicRate: snapshot.Effective.Float("music.defaultSpeed"), BookRate: snapshot.Effective.Float("audiobooks.defaultSpeed"), AutoplayNext: snapshot.Effective.Bool("playback.autoplayNext"), PassoutMinutes: snapshot.Effective.Int("playback.sleepTimerMinutes")}
	for _, d := range snapshot.Documents {
		if d.Scope == operations.ScopeProfileServer {
			out.Revision = d.Revision
		}
	}
	return out
}

// Listening preferences are a projection of the typed registry, using its
// profile document and revision. There is no second preference store.
func (s *Service) ListeningPreferences(viewer identity.Viewer, authorize operations.Authorize) (ListeningPreferences, error) {
	snapshot, e := operations.New(s.db).Preferences(s.Context(), identity.Principal{Viewer: viewer}, "web", authorize)
	return listeningPreferences(snapshot), e
}
func (s *Service) SaveListeningPreferences(viewer identity.Viewer, p ListeningPreferences, authorize operations.Authorize) (ListeningPreferences, error) {
	snapshot, e := operations.New(s.db).ApplyPreferences(s.Context(), identity.Principal{Viewer: viewer}, authorize, operations.PreferenceChange{
		Scope: operations.ScopeProfileServer, DeviceClass: "web", ExpectedRevision: p.Revision, IdempotencyKey: identity.Token(),
		Values: operations.PreferencePatch{"music.defaultSpeed": p.MusicRate, "audiobooks.defaultSpeed": p.BookRate, "playback.autoplayNext": p.AutoplayNext, "playback.sleepTimerMinutes": p.PassoutMinutes},
	})
	var conflict *operations.ConflictError
	if errors.As(e, &conflict) {
		return p, ErrListeningPreferencesConflict
	}
	if e != nil {
		return p, e
	}
	return listeningPreferences(snapshot), nil
}
