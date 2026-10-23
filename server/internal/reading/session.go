package reading

import (
	"context"
	"errors"
	"sync"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

const sessionIdle = 30 * time.Minute

// Session is an ephemeral reader lifecycle, never a bearer grant. Current
// durable identity/library/title authority is checked for every acquisition and
// extent. It captures exactly one publication version and cannot be retargeted.
type Session struct {
	ID           string
	selection    Selection
	actor, owner string
	observed     storage.ObservedFileObservation
	life         context.Context
	cancel       context.CancelFunc
	timer        *time.Timer
	mu           sync.Mutex
	closed       bool
}

func (s *Session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.cancel()
		if s.timer != nil {
			s.timer.Stop()
		}
	}
}
func (s *Session) touch() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.life.Err() != nil {
		return false
	}
	s.timer.Reset(sessionIdle)
	return true
}

func (s *Service) BeginSession(ctx context.Context, p identity.Principal, item, sourceID string) (*Session, error) {
	f, err := s.Open(ctx, p, item, sourceID, "")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = f.Validate(); err != nil {
		return nil, err
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.sessionsClosed {
		return nil, ErrUnavailable
	}
	if s.sessions == nil {
		s.sessions = map[string]*Session{}
	}
	owner := identity.PersonalKey(p.Viewer)
	count := 0
	for id, old := range s.sessions {
		if old.life.Err() != nil {
			delete(s.sessions, id)
			continue
		}
		if old.actor == p.Hash && old.selection.ItemID == item && old.selection.SourceID == f.SourceID && old.selection.Revision == f.Revision && old.touch() {
			return old, nil
		}
		if old.owner == owner {
			count++
		}
	}
	if len(s.sessions) >= 128 || count >= 8 {
		return nil, ErrLimit
	}
	life, cancel := context.WithCancel(context.Background())
	out := &Session{ID: identity.Token(), selection: f.Selection, actor: p.Hash, owner: owner, observed: f.observed, life: life, cancel: cancel}
	out.timer = time.AfterFunc(sessionIdle, out.close)
	s.sessions[out.ID] = out
	return out, nil
}

func (s *Service) findSession(p identity.Principal, id string) (*Session, error) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.sessionsClosed {
		return nil, ErrUnavailable
	}
	session := s.sessions[id]
	if session == nil || session.actor != p.Hash || session.owner != identity.PersonalKey(p.Viewer) || !session.touch() {
		return nil, identity.ErrForbidden
	}
	return session, nil
}

// OpenSession binds the request's cancellation to the retained reader lifetime.
// Local acquisitions are freshly rooted/observed and must match the captured
// version; remote leases will attach here only through their checked producer.
func (s *Service) OpenSession(ctx context.Context, p identity.Principal, id string) (*Source, error) {
	session, err := s.findSession(p, id)
	if err != nil {
		return nil, err
	}
	request, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(session.life, cancel)
	f, err := s.Open(request, p, session.selection.ItemID, session.selection.SourceID, session.selection.Revision)
	if err != nil {
		stop()
		cancel()
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			session.close()
		}
		return nil, err
	}
	if f.observed.ObjectBindingID != session.observed.ObjectBindingID || f.observed.ChangeToken != session.observed.ChangeToken {
		f.Close()
		stop()
		cancel()
		session.close()
		return nil, ErrChanged
	}
	f.onClose = func() { stop(); cancel() }
	f.onFailure = session.close
	return f, nil
}

func (s *Service) CloseSession(p identity.Principal, id string) error {
	session, err := s.findSession(p, id)
	if err != nil {
		return err
	}
	session.close()
	s.sessionMu.Lock()
	delete(s.sessions, id)
	s.sessionMu.Unlock()
	return nil
}
func (s *Service) Close() error {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.sessionsClosed = true
	for id, session := range s.sessions {
		session.close()
		delete(s.sessions, id)
	}
	return nil
}
