package postgres_test

import (
	"context"
	"testing"
	"time"

	"portico.local/hosted/internal/postgres"
)

// A95: the recovery-point marker is written every 30 s while other writes
// happen, and never while the primary is idle.
func TestRPOMarkerOnlyAfterWrites(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	m := &postgres.RPOMarker{}
	now := time.Now()
	if wrote, e := m.Tick(ctx, store, now); e != nil || !wrote {
		t.Fatalf("first tick: %v %v", wrote, e)
	}
	write := func() {
		t.Helper()
		if _, e := store.DB.Exec(`CREATE TEMP TABLE IF NOT EXISTS rpo_probe(x int); INSERT INTO accounts(id,username,display_name) VALUES('acct_rpo_'||md5(random()::text),'rpo'||substr(md5(random()::text),1,8),'R')`); e != nil {
			t.Fatal(e)
		}
	}
	write()
	if wrote, _ := m.Tick(ctx, store, now.Add(10*time.Second)); wrote {
		t.Fatal("marker written inside the 30-second interval")
	}
	if wrote, _ := m.Tick(ctx, store, now.Add(30*time.Second)); !wrote {
		t.Fatal("no marker after writes and the interval")
	}
	// Idle: nothing else written since that marker.
	if wrote, _ := m.Tick(ctx, store, now.Add(30*time.Minute)); wrote {
		t.Fatal("marker written on an idle primary")
	}
	write()
	if wrote, _ := m.Tick(ctx, store, now.Add(31*time.Minute)); !wrote {
		t.Fatal("no marker after activity resumed")
	}
}
