package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogueAuthorizationScopeSharesOnlyEquivalentPolicies(t *testing.T) {
	primary := User{
		ID: "account-primary", AccountID: "account-primary", ProfileID: "profile-primary", Role: "user", LibraryIDs: []string{"library-b", "library-a"},
		Permissions: map[string]bool{"stream": true}, Preferences: defaultUserPreferences(),
	}
	secondary := primary
	secondary.ID = "account-secondary"
	secondary.AccountID = "account-secondary"
	secondary.ProfileID = "profile-secondary"
	secondary.LibraryIDs = []string{"library-a", "library-b"}
	if catalogueAuthorizationScope(primary) != catalogueAuthorizationScope(secondary) {
		t.Fatal("equivalent profile catalogue policies should share one catalogue scope")
	}
	restricted := secondary
	restricted.LibraryIDs = []string{"library-a"}
	if catalogueAuthorizationScope(primary) == catalogueAuthorizationScope(restricted) {
		t.Fatal("different library authorization must not share a catalogue scope")
	}
	restricted = secondary
	restricted.MaxContentRating = "PG"
	if catalogueAuthorizationScope(primary) == catalogueAuthorizationScope(restricted) {
		t.Fatal("different content policy must not share a catalogue scope")
	}
	if homeCacheKey(primary) == homeCacheKey(secondary) {
		t.Fatal("profile-state Home responses must remain isolated")
	}
}

func TestHomeManifestPreviewHydrationHasBoundedConcurrency(t *testing.T) {
	server := &Server{}
	rows := make([]HomeRow, 12)
	for index := range rows {
		rows[index] = homeRowDescriptor(fmt.Sprintf("row-%02d", index), "Row", "test", "poster", "", index, true)
	}
	var active atomic.Int32
	var maximum atomic.Int32
	loader := func(ctx context.Context, _ User, rowID string, _, _ int) (HomeRow, error, bool) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			return HomeRow{}, ctx.Err(), false
		}
		return HomeRow{ID: rowID, Items: []MediaItem{{ID: rowID + "-item"}}, Total: 1}, nil, true
	}
	if err := server.hydrateHomeManifestPreviews(context.Background(), User{ID: "profile"}, rows, loader); err != nil {
		t.Fatalf("hydrate previews: %v", err)
	}
	if got := maximum.Load(); got > maximumConcurrentHomePreviews || got < 2 {
		t.Fatalf("maximum concurrency = %d, want 2..%d", got, maximumConcurrentHomePreviews)
	}
	for _, row := range rows {
		if len(row.Items) != 1 {
			t.Fatalf("row %q was not hydrated: %+v", row.ID, row)
		}
	}
}

func TestHomeResponseOuterCacheRemains(t *testing.T) {
	server := &Server{}
	user := User{ID: "account", AccountID: "account", ProfileID: "profile", Role: "user", Preferences: defaultUserPreferences()}
	key := homeCacheKey(user)
	want := HomeResponse{Rows: []HomeRow{{ID: "continue", Items: []MediaItem{{ID: "cached-item"}}}}}
	server.storeHomeResponse(key, user.ProfileID, want)
	got, ok := server.cachedHomeResponse(key, user.ProfileID)
	if !ok || len(got.Rows) != 1 || len(got.Rows[0].Items) != 1 || got.Rows[0].Items[0].ID != "cached-item" {
		t.Fatalf("outer Home response cache miss: ok=%v response=%+v", ok, got)
	}
	wait, owner := server.beginHomeResponseBuild(key)
	if !owner {
		t.Fatal("first outer Home response build should own the singleflight")
	}
	followerWait, owner := server.beginHomeResponseBuild(key)
	if owner || followerWait != wait {
		t.Fatal("concurrent outer Home response build did not join the existing singleflight")
	}
	server.finishHomeResponseBuild(key)
	select {
	case <-followerWait:
	default:
		t.Fatal("outer Home response singleflight did not release its waiter")
	}
}
