package playbackv1

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
	"sync"
)

var errSelectionChanged = errors.New("queue selection changed before capture completed")

// At most two query captures pin a read connection/WAL boundary. The lifetime
// is independent of the HTTP response, bounded like existing background builds.
var queryCaptureSlots = make(chan struct{}, 2)

type queryCapture struct {
	tx    *sql.Tx
	close func()
}

func (s *Service) openCapture(request context.Context) (*queryCapture, error) {
	select {
	case queryCaptureSlots <- struct{}{}:
	case <-request.Done():
		return nil, request.Err()
	default:
		return nil, ErrQueueBuilding
	}
	ctx, cancel := context.WithTimeout(context.Background(), buildLifetime)
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		cancel()
		<-queryCaptureSlots
		return nil, err
	}
	var once sync.Once
	close := func() { once.Do(func() { done(); cancel(); <-queryCaptureSlots }) }
	// Also releases an orphan's permit if a request or worker never reaches it.
	stop := context.AfterFunc(ctx, close)
	return &queryCapture{tx: tx, close: func() { stop(); close() }}, nil
}
func (s *Service) takeCapture(id string) *queryCapture {
	s.build.mu.Lock()
	defer s.build.mu.Unlock()
	c := s.build.captures[id]
	delete(s.build.captures, id)
	return c
}
func (s *Service) releaseCapture(id string) {
	if c := s.takeCapture(id); c != nil {
		c.close()
	}
}
