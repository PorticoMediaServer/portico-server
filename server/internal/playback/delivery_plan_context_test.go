package playback

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

// One plan load must carry its caller's context through every read (plan, pin
// existence, prepared version and conversion target). With the pool genuinely
// exhausted by a held connection, a cancelled caller must return at once instead
// of waiting for a background-context query to get a slot.

const planContextBound = 2 * time.Second

func openPlanContextFixture(t *testing.T) (*HLS, string) {
	t.Helper()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, item, _ := catalogFixture(t, db, "lib", "/test", compactcatalog.Movie, "item", "Film", compactcatalog.Asset{Path: "/test/file.mp4", Size: 20, ModifiedNS: 30, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 100})
	principal := identity.Principal{Hash: "session", Viewer: identity.Viewer{AccountID: "owner", ProfileID: "profile", Authority: "local", Role: "owner"}}
	insertFixtureAccount(t, db, principal.AccountID, principal.ProfileID)
	session, err := New(db).Create(principal, item, "automatic", "request")
	if err != nil {
		t.Fatal(err)
	}
	return &HLS{db: db, finite: &FiniteHLS{}}, session.ID
}

// exhaustPlanPool leaves the only pool slot in the caller's hand; the caller
// closes it (registered here) before the test ends, so no goroutine outlives it.
func exhaustPlanPool(t *testing.T, h *HLS) *sql.Conn {
	t.Helper()
	h.db.SetMaxOpenConns(1)
	held, err := h.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	return held
}

func TestPlanLoadCanceledContextDoesNotWaitForPoolSlot(t *testing.T) {
	h, id := openPlanContextFixture(t)
	_ = exhaustPlanPool(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadDeliveryPlan(ctx, h.db, id); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled plan load returned %v, want context.Canceled", err)
	}
}

func TestHLSReadyCanceledContextDoesNotWaitForPoolSlot(t *testing.T) {
	h, id := openPlanContextFixture(t)
	_ = exhaustPlanPool(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Ready(ctx, id); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Ready returned %v, want context.Canceled", err)
	}
}

func TestHLSFileContextCanceledDoesNotWaitForPoolSlot(t *testing.T) {
	h, _ := openPlanContextFixture(t)
	_ = exhaustPlanPool(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.FileContext(ctx, "grant", "master.m3u8"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled FileContext returned %v, want context.Canceled", err)
	}
}

func TestPlanLoadLiveReadAfterPoolRelease(t *testing.T) {
	h, id := openPlanContextFixture(t)
	held := exhaustPlanPool(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), planContextBound)
	defer cancel()
	type result struct {
		plan *DeliveryPlan
		err  error
	}
	done := make(chan result, 1)
	go func() { plan, err := loadDeliveryPlan(ctx, h.db, id); done <- result{plan, err} }()
	if stats := h.db.Stats(); stats.WaitCount == 0 {
		t.Fatal("live read did not wait for the held pool slot")
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.plan == nil || got.plan.SourceSize != 20 {
			t.Fatalf("live read after release: %+v %v", got.plan, got.err)
		}
	case <-time.After(planContextBound):
		t.Fatal("live read did not settle after the pool slot was released")
	}
}

func TestPlanLoadProducerCancellationUnblocksLookup(t *testing.T) {
	h, id := openPlanContextFixture(t)
	held := exhaustPlanPool(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := loadDeliveryPlan(ctx, h.db, id); done <- err }()
	if stats := h.db.Stats(); stats.WaitCount == 0 {
		t.Fatal("lookup did not wait for the held pool slot")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("producer cancellation returned %v, want context.Canceled", err)
		}
	case <-time.After(planContextBound):
		t.Fatal("producer cancellation left the lookup waiting")
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPlanLoadMissingPlanStaysNil(t *testing.T) {
	h, _ := openPlanContextFixture(t)
	plan, err := loadDeliveryPlan(context.Background(), h.db, "no-such-session")
	if err != nil || plan != nil {
		t.Fatalf("missing plan = %+v, %v; want nil, nil", plan, err)
	}
}
