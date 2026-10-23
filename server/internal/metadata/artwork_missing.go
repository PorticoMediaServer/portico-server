package metadata

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
)

type missingArtwork struct {
	Target        RepairTarget
	Role, Subject string
}

// Reads coalesce repair hints in a bounded queue; only the worker writes. A
// missing object never makes a response wait for the writer gate.
func (s *Service) queueMissingArtwork(t RepairTarget, role, subject string) {
	s.missingMu.Lock()
	if s.missingArtwork == nil {
		s.missingArtwork = make(map[missingArtwork]struct{})
	}
	if len(s.missingArtwork) < 1024 {
		s.missingArtwork[missingArtwork{t, role, subject}] = struct{}{}
	}
	s.missingMu.Unlock()
	s.missingWake.Wake()
}
func (s *Service) RepairMissingArtworkStep(ctx context.Context) error {
	s.missingMu.Lock()
	var batch []missingArtwork
	for v := range s.missingArtwork {
		batch = append(batch, v)
		delete(s.missingArtwork, v)
		if len(batch) == 32 {
			break
		}
	}
	s.missingMu.Unlock()
	for i, v := range batch {
		entity, err := entityid.Resolve(ctx, s.db, v.Target.ID)
		if errors.Is(err, entityid.ErrNotFound) {
			// The target is gone; drop its hint like a completed repair would.
			continue
		}
		if err != nil {
			for _, pending := range batch[i:] {
				s.queueMissingArtwork(pending.Target, pending.Role, pending.Subject)
			}
			return err
		}
		if err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `UPDATE artwork_jobs SET status='pending',attempts=0,next_attempt='' WHERE kind=? AND entity_id=? AND role=? AND subject=? AND status='complete'`, v.Target.Kind, entity, v.Role, v.Subject); err != nil {
				return err
			}
			// Cold local migration has no completed job yet. Reseeding reuses
			// the recorded local origin and all normal policy/selection fences.
			_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO artwork_dirty(kind,entity_id) VALUES(?,?)`, v.Target.Kind, entity)
			return err
		}); err != nil {
			for _, pending := range batch[i:] {
				s.queueMissingArtwork(pending.Target, pending.Role, pending.Subject)
			}
			return err
		}
	}
	return nil
}
