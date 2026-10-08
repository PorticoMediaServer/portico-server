package backup

import (
	"context"
	"database/sql"
	"log"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/workpolicy"
)

// Scheduler runs the backup schedule: when a maintenance window holding the
// backup task is open and no scheduled backup has been created since that
// occurrence started, it creates one and prunes to the keep count.
type Scheduler struct {
	service *Service
	db      *sql.DB
	keep    func(context.Context) (int, error)
	now     func() time.Time
}

// Schedule builds the scheduler on the live database. keep answers the
// owner's BackupKeepCount; a failing keep falls back to the shipped default.
func (s *Service) Schedule(db *sql.DB, keep func(context.Context) (int, error)) *Scheduler {
	return &Scheduler{service: s, db: db, keep: keep, now: time.Now}
}

// Run checks the schedule on start and every five minutes until ctx ends.
func (s *Scheduler) Run(ctx context.Context) {
	s.check(ctx)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.check(ctx)
		}
	}
}

func (s *Scheduler) check(ctx context.Context) {
	ctx = dbwork.WithClass(ctx, dbwork.ClassMaintenance)
	policy, err := s.readPolicy(ctx)
	if err != nil {
		return
	}
	now := s.now()
	start, ok := workpolicy.OccurrenceStart(policy, "backup", now)
	if !ok {
		return
	}
	latest, err := s.latestScheduled()
	if err != nil {
		return
	}
	if !latest.IsZero() && !latest.Before(start) {
		return
	}
	id, err := s.service.CreateKind(ctx, KindScheduled)
	if err != nil {
		log.Printf("Scheduled backup: %v", err)
		return
	}
	log.Printf("Scheduled backup %s", id)
	keep := DefaultKeepCount
	if s.keep != nil {
		if n, err := s.keep(ctx); err == nil && n > 0 {
			keep = n
		}
	}
	if err = s.service.Prune(keep); err != nil {
		log.Printf("Scheduled backup prune: %v", err)
	}
}

func (s *Scheduler) readPolicy(ctx context.Context) (workpolicy.Policy, error) {
	var out workpolicy.Policy
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer snapshot.Rollback()
	return workpolicy.ReadTx(ctx, snapshot.Tx())
}

func (s *Scheduler) latestScheduled() (time.Time, error) {
	listed, err := s.service.List()
	if err != nil {
		return time.Time{}, err
	}
	var latest time.Time
	for _, info := range listed {
		if info.Kind != KindScheduled {
			continue
		}
		at, err := time.Parse(time.RFC3339, info.CreatedAt)
		if err != nil {
			continue
		}
		if at.After(latest) {
			latest = at
		}
	}
	return latest, nil
}
