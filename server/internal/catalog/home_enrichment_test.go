package catalog

import (
	"context"
	"reflect"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
)

func TestHomePreviewDecorationBatchesRowsAndKeepsPerRowFields(t *testing.T) {
	db, s, names := tl10RestrictionFixture(t)
	c := catalogtest.New(t, db)
	c.Genres(names["kids"].ID, "tmdb", "Comedy", "Family", "Adventure")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,revision) VALUES('p',?,1,0,1)`, names["kids"].ID)
	c.Drain()
	makeRows := func() []HomeRow {
		return []HomeRow{
			{Entries: []ContentEntry{{ID: names["kids"].Public, Kind: "movie"}, {ID: names["show"].Public, Kind: "show"}}},
			{Entries: []ContentEntry{}},
			{Entries: []ContentEntry{{ID: names["family"].Public, Kind: "movie"}, {ID: names["kids"].Public, Kind: "movie"}}},
		}
	}
	expected, actual := makeRows(), makeRows()
	for _, row := range expected {
		if err := s.homeEnrich("p", row.Entries); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cost := dbwork.Measure(context.Background())
	if err := s.WithContext(ctx).homeEnrichRows("p", actual); err != nil {
		t.Fatal(err)
	}
	if cost().Statements != 4 {
		t.Fatalf("preview decoration cost %d statements, want four for all rows", cost().Statements)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("batched preview fields differ from individual row decoration:\nactual: %+v\nexpected: %+v", actual, expected)
	}
	first := actual[0].Entries[0]
	if first.Year == nil || *first.Year != 2000 || first.ContentRating != "G" || !reflect.DeepEqual(first.Genres, []string{"Comedy", "Family"}) || first.Watchlisted == nil || !*first.Watchlisted {
		t.Fatalf("item decoration missing: %+v", first)
	}
	if actual[0].Entries[1].Watchlisted != nil {
		t.Fatal("container received item watchlist state")
	}
	c.Exec(`UPDATE personal_items SET watchlisted=0 WHERE profile_id='p' AND item_id=?`, names["kids"].ID)
	fresh := makeRows()
	if err := s.homeEnrichRows("p", fresh); err != nil {
		t.Fatal(err)
	}
	if fresh[0].Entries[0].Watchlisted == nil || *fresh[0].Entries[0].Watchlisted {
		t.Fatal("a later preview reused old personal state")
	}
}

func TestHomeDocumentAndSingleRowCarryTheSameDecoration(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	movie := homeItem(t, db, "m1")
	c.Attributes(movie.ID, "contentRating", "PG")
	c.Genres(movie.ID, "tmdb", "Comedy", "Family", "Adventure")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,revision) VALUES('p',?,1,0,1)`, movie.ID)
	c.Drain()
	r := homeRequestFixture()
	document, err := s.HomeRows(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Rows) < 2 {
		t.Fatal("fixture needs multiple preview rows")
	}
	for _, row := range document.Rows {
		single, err := s.HomeSingleRow(r, row.ID, HomeRowPage{Limit: row.Limit})
		if err != nil {
			t.Fatal(row.ID, err)
		}
		if !reflect.DeepEqual(row.Entries, single.Entries) {
			t.Fatalf("Home and row endpoint decoration disagree for %s", row.ID)
		}
	}
}
