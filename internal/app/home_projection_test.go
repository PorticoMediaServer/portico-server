package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestHomeProjectionKeySharesOnlyEquivalentCatalogueAuthorization(t *testing.T) {
	primary := User{
		ID: "account-primary", AccountID: "account-primary", ProfileID: "profile-primary", Role: "user", LibraryIDs: []string{"library-b", "library-a"},
		Permissions: map[string]bool{"stream": true}, Preferences: defaultUserPreferences(),
	}
	secondary := primary
	secondary.ID = "account-secondary"
	secondary.AccountID = "account-secondary"
	secondary.ProfileID = "profile-secondary"
	secondary.LibraryIDs = []string{"library-a", "library-b"}
	if homeProjectionCacheKey(primary) != homeProjectionCacheKey(secondary) {
		t.Fatal("equivalent profile catalogue policies should share one projection")
	}
	restricted := secondary
	restricted.LibraryIDs = []string{"library-a"}
	if homeProjectionCacheKey(primary) == homeProjectionCacheKey(restricted) {
		t.Fatal("different library authorization must not share a projection")
	}
	restricted = secondary
	restricted.MaxContentRating = "PG"
	if homeProjectionCacheKey(primary) == homeProjectionCacheKey(restricted) {
		t.Fatal("different content policy must not share a projection")
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

func TestHomeProjectionCacheStripsProfileStateAndIsBounded(t *testing.T) {
	server := &Server{homeProjectionCache: map[string]homeProjectionCacheEntry{}}
	user := User{ID: "profile", Role: "user", Preferences: defaultUserPreferences()}
	server.storeHomeProjection(user, []HomeRow{{ID: "recent-library", Items: []MediaItem{{ID: "private-progress-item"}}, NextCursor: "profile-cursor"}})
	rows, ok := server.cachedHomeProjection(user)
	if !ok || len(rows) != 1 {
		t.Fatalf("projection cache miss: ok=%v rows=%+v", ok, rows)
	}
	if len(rows[0].Items) != 0 || rows[0].NextCursor != "" {
		t.Fatalf("viewer state escaped into shared projection: %+v", rows[0])
	}
}
