package catalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

func TestRetainedCaptureRefusesLatePersonalWrites(t *testing.T) {
	f := newCaptureFixture(t)
	f.c.Write(func(_ context.Context, tx *sql.Tx) error {
		return storePersonalByID(tx, f.owner, f.item, PersonalState{Favorite: true, Revision: 1})
	})
	beginCaptureFixture(t, f)
	identity := "local:/reading|book:late-personal"
	captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{identity}, true, f.account, f.profile, f.lifetime, f.membership)
	if _, _, err := captureStepOnce(t, f.c, f.m); err != nil {
		t.Fatal(err)
	}
	// An accepted input precedes capture. A new write during its private copy
	// must be refused in the same writer, rather than disappearing behind it.
	for name, write := range map[string]func(*sql.Tx) error{
		"personal": func(tx *sql.Tx) error {
			return storePersonalByID(tx, f.owner, f.item, PersonalState{Revision: 2})
		},
		"resume": func(tx *sql.Tx) error {
			return explicitResumeByID(tx, f.owner, f.item, 42)
		},
	} {
		err := dbwork.WithWriteTx(context.Background(), f.c.DB, dbwork.ClassInteractive, write)
		if !errors.Is(err, compactcatalog.ErrEntityMutating) {
			t.Fatalf("late %s write: %v", name, err)
		}
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=? AND favorite=1 AND revision=1`, f.owner, f.item); n != 1 {
		t.Fatal("refused write changed accepted personal state")
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM progress WHERE profile_id=? AND item_id=?`, f.owner, f.item); n != 0 {
		t.Fatal("refused resume created progress")
	}
	if published, _ := captureDrive(t, f.c, f.m, 64); !published {
		t.Fatal("capture did not publish the accepted state")
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state s JOIN retained_title_identities i ON i.state_id=s.id WHERE i.identity=? AND i.profile_id=? AND s.ready=1 AND s.favorite=1`, identity, f.owner); n != 1 {
		t.Fatal("accepted state was lost during private copying")
	}
}
