package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

func TestHomeActionProjectionUsesDisplayedEntitiesAndBatchesReturnedItems(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	show, episode := homeItem(t, db, "show"), homeItem(t, db, "e101")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision) VALUES('p',?,1,1,NULL,7),('p',?,0,0,8,3)`, show.ID, episode.ID)
	c.Drain()
	movie := homeItem(t, db, "m1")
	makeRows := func() []HomeRow {
		return []HomeRow{{Entries: []ContentEntry{
			{ID: show.Public, LibraryID: "tv", Kind: "show", Playback: &ContentPlayback{ItemID: episode.Public}},
			{ID: movie.Public, LibraryID: "movies", Kind: "movie"},
			{ID: movie.Public, LibraryID: "movies", Kind: "movie"},
			{ID: homeItem(t, db, "album").Public, LibraryID: "music", Kind: "album"},
			{ID: homeItem(t, db, "book").Public, LibraryID: "books", Kind: "book"},
		}, Resumes: []HomeResume{{EntryID: show.Public, Target: ContentEntry{ID: episode.Public, LibraryID: "tv", Kind: "episode"}}}}}
	}
	rows := makeRows()
	deletionReads := 0
	authority := HomeActionAuthority{Personal: true, Organize: true, Bulk: true, Playback: true, Queue: true, Metadata: true, Refresh: true, Downloads: true, DeleteLibrary: func(library string) (bool, error) { deletionReads++; return library == "movies", nil }}
	ctx, cost := dbwork.Measure(context.Background())
	if err := dbwork.WithReadSnapshot(ctx, db, func(ctx context.Context) error {
		return s.WithContext(ctx).ProjectHomeActions(homeRequestFixture().Viewer, authority, rows, nil)
	}); err != nil {
		t.Fatal(err)
	}
	// One exact-key SQL batch, independent of duplicate cards and the number
	// of descendants under the returned aggregate titles.
	if cost().Statements != 1 || deletionReads != 2 {
		t.Fatalf("statements=%d deletion policies=%d", cost().Statements, deletionReads)
	}
	a := rows[0].Entries[0].HomeActions
	if a == nil || !a.MyList || !a.Favorite || !a.EditMetadata || a.Delete || !a.State.Watchlisted || !a.State.Favorite || a.State.Rating != nil || a.State.PersonalRevision != 7 {
		t.Fatalf("show offers/state: %+v", a)
	}
	if rows[0].Entries[0].ID != show.Public || rows[0].Entries[0].Playback.ItemID != episode.Public {
		t.Fatal("aggregate identity replaced by a child")
	}
	if child := rows[0].Resumes[0].Target.HomeActions; child == nil || child.State.Rating == nil || *child.State.Rating != 8 || child.State.PersonalRevision != 3 {
		t.Fatalf("resume state: %+v", child)
	}
	if a := rows[0].Entries[1].HomeActions; a == nil || !a.Delete || !a.Watched || a.Played {
		t.Fatalf("movie offers: %+v", a)
	}
	if a := rows[0].Entries[3].HomeActions; a == nil || a.MyList || a.Watched || !a.Played || a.Delete || a.RefreshMetadata {
		t.Fatalf("album offers: %+v", a)
	}
	if a := rows[0].Entries[4].HomeActions; a == nil || !a.MyList || a.Watched || !a.Played || a.Delete {
		t.Fatalf("book offers: %+v", a)
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"/movies/", "/tv/", "sourcePath", "filePath", "apiKey", "rootIdentity"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private field leaked: %s", private)
		}
	}
}

func TestHomeActionProjectionFailsClosedForMissingAuthorityAndWrongIdentity(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	movie := homeItem(t, db, "m1")
	rows := []HomeRow{{Entries: []ContentEntry{
		{ID: movie.Public, LibraryID: "movies", Kind: "movie"},
		{ID: movie.Public, LibraryID: "private", Kind: "movie"},
		{ID: movie.Public, LibraryID: "movies", Kind: "show"},
		{ID: homeItem(t, db, "book").Public, LibraryID: "books", Kind: "book"},
	}}}
	if err := dbwork.WithReadSnapshot(context.Background(), db, func(ctx context.Context) error {
		return s.WithContext(ctx).ProjectHomeActions(homeRequestFixture("movies").Viewer, HomeActionAuthority{}, rows, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if a := rows[0].Entries[0].HomeActions; a == nil || a.Play || a.Queue || a.Playlist || a.Collection || a.MyList || a.Favorite || a.Watched || a.EditMetadata || a.RefreshMetadata || a.Delete || a.Download {
		t.Fatalf("ungranted offers: %+v", a)
	}
	for _, entry := range rows[0].Entries[1:] {
		if entry.HomeActions != nil {
			t.Fatalf("wrong or inaccessible identity got offers: %+v", entry)
		}
	}
	if err := s.ProjectHomeActions(homeRequestFixture().Viewer, HomeActionAuthority{}, rows, nil); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("projection outside snapshot: %v", err)
	}
}

func TestHomeActionProjectionUnavailableMediaAndCollectionAreIndependent(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	movie := homeItem(t, db, "m1")
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetAssetAvailableTx(ctx, tx, movie.Asset, false)
	})
	c.Drain()
	collection, err := s.CreateCollection("movies", "Actual series")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetCollectionItem(collection.ID, homeItem(t, db, "m2").Public, true); err != nil {
		t.Fatal(err)
	}
	c.Drain()
	rows := []HomeRow{{Entries: []ContentEntry{{ID: movie.Public, LibraryID: "movies", Kind: "movie"}}}, {Collection: &HomeCollection{MediaKind: "movie", Collection: collection, Members: []ContentEntry{{ID: homeItem(t, db, "m2").Public, LibraryID: "movies", Kind: "movie"}}}}}
	if err = dbwork.WithReadSnapshot(context.Background(), db, func(ctx context.Context) error {
		return s.WithContext(ctx).ProjectHomeActions(homeRequestFixture().Viewer, HomeActionAuthority{Personal: true, Playback: true, Queue: true, Organize: true, Bulk: true, Metadata: true, Downloads: true, DeleteLibrary: func(string) (bool, error) { return true, nil }}, rows, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if a := rows[0].Entries[0].HomeActions; a == nil || a.Play || a.Queue || a.Download || !a.MyList {
		t.Fatalf("unavailable offers: %+v", a)
	}
	banner := rows[1].Collection
	if banner.Actions == nil || !banner.Actions.Details || !banner.Actions.Playlist || banner.Actions.Delete || banner.Actions.EditMetadata || banner.Actions.MyList || banner.Actions.Download {
		t.Fatalf("collection grants: %+v", banner.Actions)
	}
	if banner.Members[0].HomeActions == nil || !banner.Members[0].HomeActions.Delete || banner.Actions == banner.Members[0].HomeActions {
		t.Fatal("banner borrowed member grants")
	}
	if banner.Collection.ID != collection.ID || banner.MediaKind != "movie" {
		t.Fatal("canonical collection identity/semantics lost")
	}
}

func TestHomeActionProjectionRestrictedAvailabilityUsesVisibleMembers(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	for _, name := range []string{"e102", "e201", "e202"} {
		c.Attributes(homeItem(t, db, name).ID, "label", "adult")
	}
	visible := homeItem(t, db, "e101")
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetAssetAvailableTx(ctx, tx, visible.Asset, false)
	})
	c.Drain()
	v := homeRequestFixture("tv").Viewer
	v.Restrictions = visibilityRestrictionOf(nil, false, "adult")
	read := func() *HomeActions {
		t.Helper()
		rows := []HomeRow{{Entries: []ContentEntry{{ID: homeItem(t, db, "show").Public, LibraryID: "tv", Kind: "show"}}}}
		if err := dbwork.WithReadSnapshot(context.Background(), db, func(ctx context.Context) error {
			return s.WithContext(ctx).ProjectHomeActions(v, HomeActionAuthority{Personal: true, Playback: true, Queue: true, Downloads: true}, rows, nil)
		}); err != nil {
			t.Fatal(err)
		}
		return rows[0].Entries[0].HomeActions
	}
	if a := read(); a == nil || a.Play || a.Queue || a.Download || !a.MyList {
		t.Fatalf("hidden available episodes supplied a false grant: %+v", a)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetAssetAvailableTx(ctx, tx, visible.Asset, true)
	})
	c.Drain()
	if a := read(); a == nil || !a.Play || !a.Queue || !a.Download {
		t.Fatalf("visible available episode did not supply the action proof: %+v", a)
	}
}

func TestHomeActionSpotlightKeepsAggregateGrantsSeparateFromResume(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	show, episode := homeItem(t, db, "show"), homeItem(t, db, "e101")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,revision) VALUES('p',?,1,1,5),('p',?,0,0,2)`, show.ID, episode.ID)
	c.Drain()
	spotlight := []HomeSpotlight{{Entry: ContentEntry{ID: show.Public, LibraryID: "tv", Kind: "show"}, ResumeTarget: &ContentEntry{ID: episode.Public, LibraryID: "tv", Kind: "episode", Playback: &ContentPlayback{ItemID: episode.Public}}}}
	ctx, cost := dbwork.Measure(context.Background())
	if err := dbwork.WithReadSnapshot(ctx, db, func(ctx context.Context) error {
		return s.WithContext(ctx).ProjectHomeActions(homeRequestFixture("tv").Viewer, HomeActionAuthority{Personal: true, Playback: true, Queue: true, Metadata: true}, nil, spotlight)
	}); err != nil {
		t.Fatal(err)
	}
	if cost().Statements != 1 {
		t.Fatalf("spotlight cost %d statements", cost().Statements)
	}
	if a := spotlight[0].Entry.HomeActions; a == nil || !a.MyList || !a.State.Watchlisted || a.State.PersonalRevision != 5 || !a.EditMetadata {
		t.Fatalf("displayed spotlight offers: %+v", a)
	}
	if a := spotlight[0].ResumeTarget.HomeActions; a == nil || a.State.Watchlisted || a.State.PersonalRevision != 2 {
		t.Fatalf("resume offers borrowed aggregate state: %+v", a)
	}
	if spotlight[0].Entry.ID != show.Public || spotlight[0].ResumeTarget.Playback.ItemID != episode.Public {
		t.Fatal("spotlight identity changed")
	}
}
