package eventfeed

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/worker"
)

func TestHubDoesNotWakeItselfAfterDrainingItsOwnFlush(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "feed.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO api_change_pending(audience,type,resource_kind,resource_id,revision,data) VALUES('server','settings.changed','server','settings','1','{}')`)
	if err != nil {
		t.Fatal(err)
	}
	hub := New(db)
	wake, release := hub.Subscribe()
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	dbwork.ProfileStatements(true)
	defer dbwork.ProfileStatements(false)
	go func() { defer close(done); hub.Run(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case <-wake:
	case <-time.After(3 * time.Second):
		t.Fatal("initial outbox flush did not notify")
	}
	// A complete batch has no pending work. Its own api_events publication
	// must not schedule a second transaction at the feed's two-second floor.
	time.Sleep(2300 * time.Millisecond)
	var reads int64
	for _, p := range dbwork.StatementProfile() {
		if strings.HasPrefix(p.Shape, "SELECT audience,type,resource_kind") {
			reads += p.Count
		}
	}
	if reads != 1 {
		t.Fatalf("own flush scheduled %d outbox passes", reads)
	}
}

func TestRetiringOwnFlushWakePreservesPeerPublications(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "peer.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	signal := worker.NewSignal()
	stop := dbwork.WakeOnTables(signal, "api_events")
	defer stop()
	commit := func() {
		t.Helper()
		if err := dbwork.WithWriteTx(ctx, db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `INSERT INTO api_events(audience,type,at_ms) VALUES('server','settings.changed',0)`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := dbwork.Publications()
	commit()
	retireOwnFlushWake(signal, before)
	if signal.Take() {
		t.Fatal("own covered publication retained work")
	}
	before = dbwork.Publications()
	commit()
	commit()
	retireOwnFlushWake(signal, before)
	if !signal.Take() {
		t.Fatal("peer publication during flush was lost")
	}
	before = dbwork.Publications()
	commit()
	retireOwnFlushWake(signal, before)
	commit()
	if !signal.Take() {
		t.Fatal("peer publication after retirement was lost")
	}
}
