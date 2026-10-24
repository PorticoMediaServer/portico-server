package playbackv1

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/identity"
)

// An initial Cast start has no occurrence to adopt yet: the narrow start
// authority binds the viewer's live device queue and the exact selected entry
// and resolved item, and the accepting writer repeats those facts before it
// inserts. A queue that gains a session meanwhile is refused.
func TestInitialCastStartBindsSelectedEntryAndRefusesConcurrentSession(t *testing.T) {
	c, s, caller, q := sonicQueueFixture(t)
	cast := caller
	cast.Principal.Grant = identity.GrantCastTarget

	tx, err := c.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.readLayout(context.Background(), tx, q)
	if err != nil {
		t.Fatal(err)
	}
	entryID, item, ok := l.entryAt(0)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !ok || entryID == "" || item == "" {
		t.Fatal("fixture queue has no first entry")
	}

	admitted := 0
	var gotQueue, gotEntry, gotItem string
	s.CastQueueStartAuthority = func(ctx context.Context, tx *sql.Tx, c Caller, queue, entry, item string) error {
		if tx == nil || c.Principal != cast.Principal {
			t.Fatalf("start authority lost its transaction or caller: %v %v", tx, c.Principal)
		}
		admitted++
		gotQueue, gotEntry, gotItem = queue, entry, item
		return nil
	}
	resolved, initial, err := s.withMusicLayoutStart(context.Background(), cast, q.id, entryID, sonicEntryWindow(entryID), func(*layout) error { return nil })
	if err != nil || !initial || resolved != item {
		t.Fatalf("initial start did not bind: item=%q initial=%v err=%v", resolved, initial, err)
	}
	if admitted != 1 || gotQueue != q.id || gotEntry != entryID || gotItem != item {
		t.Fatalf("start authority saw %q %q %q after %d admissions", gotQueue, gotEntry, gotItem, admitted)
	}

	// The accepting writer repeats the bound facts and refuses a queue that
	// gained a session while the start was in flight.
	bound := queueInitialStart{queue: q.id, entry: entryID, item: item}
	base := row{queueID: q.id, entryID: entryID, item: item}
	check := func() error {
		checkTx, err := c.DB.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer checkTx.Rollback()
		return s.recheckInitialQueueStartTx(context.Background(), checkTx, cast, bound, base)
	}
	if err = check(); err != nil {
		t.Fatalf("matching facts refused: %v", err)
	}
	c.Exec(`UPDATE queues_v1 SET session_id='other' WHERE id='q_capture'`)
	if err = check(); !errors.Is(err, ErrRevision) {
		t.Fatalf("gained session not refused at publication: %v", err)
	}
	if _, _, err = s.withMusicLayoutStart(context.Background(), cast, q.id, entryID, sonicEntryWindow(entryID), func(*layout) error { return nil }); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("session queue skipped the adopted-occurrence guard: %v", err)
	}
	c.Exec(`UPDATE queues_v1 SET session_id='' WHERE id='q_capture'`)
	base.item = "other-item"
	if err = check(); !errors.Is(err, ErrRevision) {
		t.Fatalf("mismatched bound item not refused: %v", err)
	}

	// A refused narrow admission stops before any capture or projection.
	s.CastQueueStartAuthority = func(context.Context, *sql.Tx, Caller, string, string, string) error {
		return identity.ErrForbidden
	}
	if _, _, err = s.withMusicLayoutStart(context.Background(), cast, q.id, entryID, sonicEntryWindow(entryID), func(*layout) error { return nil }); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("refused start authority was ignored: %v", err)
	}
}
