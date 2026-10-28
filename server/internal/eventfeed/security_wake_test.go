package eventfeed

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

// TestSecurityWakeIgnoresPrivateProgressAndKeepsSourceAuthority runs the real
// feed loop, so its security subscription list and Hub.Wake are the production
// ones. The relocated progress statement is ordinary catalogue work; a real
// source revision must still wake every stream.
func TestSecurityWakeIgnoresPrivateProgressAndKeepsSourceAuthority(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	if _, err = db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('progress','Progress','movie',?)`, root); err != nil {
		t.Fatal(err)
	}
	hub := New(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subscribed, unsubscribe := hub.Subscribe()
	defer unsubscribe()
	go hub.Run(ctx)
	// Wait until Run's security subscription is live before testing silence.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err = dbwork.ExecWrite(ctx, db, dbwork.ClassSecurityFence, `UPDATE library_sources SET enabled=1-enabled WHERE id='progress'`); err != nil {
			t.Fatal(err)
		}
		select {
		case <-subscribed:
			goto primed
		case <-time.After(50 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("the real feed never woke for a source authority write")
			}
		}
	}
primed:
	// Let the priming wakes settle, then drain them: the progress write must
	// announce nothing of its own.
	time.Sleep(100 * time.Millisecond)
	for {
		select {
		case <-subscribed:
		default:
			goto drained
		}
	}
drained:
	before := dbwork.AuthorityGeneration()
	if _, err = dbwork.ExecWrite(ctx, db, dbwork.ClassBackgroundMedia, `INSERT INTO library_source_progress(source_id,last_progress_at) SELECT id,? FROM library_sources WHERE id=? ON CONFLICT(source_id) DO UPDATE SET last_progress_at=excluded.last_progress_at`, "2026-10-27T00:00:00.000000000Z", "progress"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscribed:
		t.Fatal("the private progress write woke a real feed stream")
	case <-time.After(200 * time.Millisecond):
	}
	if got := dbwork.AuthorityGeneration(); got != before {
		t.Fatalf("the private progress write moved AuthorityGeneration %d → %d", before, got)
	}
	if _, err = dbwork.ExecWrite(ctx, db, dbwork.ClassSecurityFence, `UPDATE library_sources SET enabled=0 WHERE id='progress'`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscribed:
	case <-time.After(3 * time.Second):
		t.Fatal("a real source disable did not wake a real feed stream")
	}
	if got := dbwork.AuthorityGeneration(); got <= before {
		t.Fatalf("a real source disable did not move AuthorityGeneration: %d", got)
	}
}
