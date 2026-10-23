package administration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/livechannels"
)

func TestLiveSettingsAndBufferBudgetShareCommittedNormalizedPolicy(t *testing.T) {
	s, db, _ := newService(t)
	ctx := context.Background()
	first, err := s.LiveDefaultsDocument(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	first.Settings.TimeShiftMinutes = 120
	saved, err := s.SaveLiveDefaults(ctx, nil, Change[LiveDefaults]{ExpectedRevision: first.Revision, OperationID: "live-rewind-two-hours", Settings: first.Settings})
	if err != nil {
		t.Fatal(err)
	}
	duration, bytes, err := livechannels.ReadTimeShiftLimits(ctx, db)
	if err != nil || duration != 2*time.Hour || bytes != 16<<30 {
		t.Fatalf("new buffer did not see committed settings: %v %d %v", duration, bytes, err)
	}
	if _, err = s.SaveLiveDefaults(ctx, nil, Change[LiveDefaults]{ExpectedRevision: first.Revision, OperationID: "live-rewind-stale-edit", Settings: first.Settings}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale owner write: %v", err)
	}
	var graphs int
	if err = db.QueryRow(`SELECT count(*) FROM admin_documents WHERE scope='live'`).Scan(&graphs); err != nil || graphs != 0 {
		t.Fatalf("live policy mirrored into a JSON graph: %d %v", graphs, err)
	}
	// A bad TOML edit rolls back its source annotation as well as the value.
	err = dbwork.WithWriteTx(ctx, db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, err := (LiveConfiguration{}).Import(ctx, tx, map[string]any{"live.timeshift_minutes": int64(31)}, map[string]string{}, 0)
		return err
	})
	if err == nil {
		t.Fatal("unsupported rewind edit committed")
	}
	current, err := s.LiveDefaultsDocument(ctx, nil)
	if err != nil || current.Revision != saved.Revision || current.Settings.TimeShiftMinutes != 120 {
		t.Fatalf("invalid import changed committed value: %+v %v", current, err)
	}
	err = dbwork.WithWriteTx(ctx, db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, err := (LiveConfiguration{}).Import(ctx, tx, map[string]any{"live.timeshift_minutes": int64(10)}, map[string]string{}, 0)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	duration, bytes, err = livechannels.ReadTimeShiftLimits(ctx, db)
	if err != nil || duration != 10*time.Minute || bytes != 1_500_000_000 {
		t.Fatalf("new buffer did not see file edit: %v %d %v", duration, bytes, err)
	}
}
