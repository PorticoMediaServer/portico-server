package postgres_test

import (
	"context"
	"testing"
	"time"

	"portico.local/hosted/internal/directory"
)

// Presence is "last seen": the latest check-in or other authenticated server
// call, with no periodic renewal and no live online state (Justin: minimal
// contact with Hosted; apps ask the server itself whether it is up).
func TestServerListLastSeen(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	seen := func() *time.Time {
		t.Helper()
		items, e := f.store.List(ctx, f.owner.Account.ID)
		if e != nil || len(items) != 1 {
			t.Fatal(items, e)
		}
		return items[0].Presence.LastSeenAt
	}
	if seen() == nil {
		t.Fatal("a claimed server has a last-seen time")
	}
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	before := time.Now().Add(-time.Second)
	if _, e = f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{}); e != nil {
		t.Fatal(e)
	}
	if at := seen(); at == nil || at.Before(before) {
		t.Fatalf("check-in not reflected: %v", at)
	}
}
