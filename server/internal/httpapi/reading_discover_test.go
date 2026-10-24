package httpapi

import (
	"context"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/reading"
)

// Discover is composed only from facts already in the snapshot: the catalogue's
// added_ms for recently added and the accepted series projection for series. It
// invents no dates, counts, kinds or people, and an all-library request returns
// no library-scoped series row.
func TestReadingDiscoverRowsFromAcceptedFacts(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	onePath := readingTreeFile(t, r, "Author/Series/One.epub")
	twoPath := readingTreeFile(t, r, "Author/Series/Two.epub")
	standalonePath := readingTreeFile(t, r, "Author/Standalone.epub")
	readingScan(t, r.f, r.library)
	one, two := readingPathItem(t, r, onePath), readingPathItem(t, r, twoPath)
	standalone := readingPathItem(t, r, standalonePath)
	out, err := r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{})
	if err != nil {
		t.Fatal(err)
	}
	recent := map[string]bool{}
	for _, row := range out.Recent {
		recent[row.ID] = true
	}
	for _, id := range []string{one, two, standalone, r.item} {
		if !recent[id] {
			t.Fatalf("recently added lost %s: %+v", id, out.Recent)
		}
	}
	if len(out.Series) != 1 || out.Series[0].Name != "Series" || out.Series[0].Count != 2 || out.Series[0].ID != compactcatalog.ReadingSeriesID(r.library, "Series") {
		t.Fatalf("series row=%+v", out.Series)
	}
	if len(out.Series[0].Items) != 2 {
		t.Fatalf("series items=%+v", out.Series[0].Items)
	}
	for _, row := range out.Series[0].Items {
		if row.Author == nil || row.Author.Value != "Author" || row.Series == nil || row.Series.Name != "Series" {
			t.Fatalf("series item attribution=%+v", row)
		}
	}
	// Value caps apply to the row, and a filled scan window admits it may be
	// incomplete instead of pretending the row is whole.
	small, err := r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{Recent: 2, Series: 1, SeriesItems: 1})
	if err != nil || len(small.Recent) != 2 || len(small.Series) != 1 || len(small.Series[0].Items) != 1 || small.SeriesLowerBound {
		t.Fatalf("capped discover=%+v %v", small, err)
	}
	window, err := r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{Scan: 2})
	if err != nil || !window.SeriesLowerBound {
		t.Fatalf("scan window=%+v %v", window, err)
	}
	// A series id is library-scoped, so an all-library request has no series row.
	all, err := r.s.Discover(ctx, r.p, "", reading.DiscoverLimits{})
	if err != nil || len(all.Recent) == 0 || len(all.Series) != 0 {
		t.Fatalf("all libraries=%+v %v", all, err)
	}
	// An owner override wins over the folder facts and regroups the row.
	if _, err = r.f.db.Exec(`INSERT INTO metadata_owner_fields(kind,entity_id,field,value,automatic_value,locked,actor,observed_at)
 SELECT 'item',id,'series','Dune','',1,?,datetime('now') FROM catalog_entities WHERE public_id=pid_blob(?)`, identity.PersonalKey(r.p.Viewer), one); err != nil {
		t.Fatal(err)
	}
	out, err = r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{})
	if err != nil || len(out.Series) != 2 {
		t.Fatalf("owner regrouping=%+v %v", out.Series, err)
	}
	names := map[string]int{}
	for _, series := range out.Series {
		names[series.Name] = series.Count
	}
	if names["Dune"] != 1 || names["Series"] != 1 {
		t.Fatalf("series counts=%+v", names)
	}
}

// The producer shares List's profile/library/restriction predicate.
func TestReadingDiscoverHonorsCurrentProfileRestrictions(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	readingTreeFile(t, r, "Author/Series/One.epub")
	readingScan(t, r.f, r.library)
	owner := readingOwnerLogin(t, r)
	profiles, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Discover reader", "blue")
	if err != nil {
		t.Fatal(err)
	}
	var child identity.DirectProfile
	for _, p := range profiles.Profiles {
		if !p.Primary && p.Name == "Discover reader" {
			child = p
		}
	}
	selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.f.id.Authenticate(selected.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 1, AllowUnrated: false}); err != nil {
		t.Fatal(err)
	}
	p, err = r.f.id.Authenticate(selected.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := r.s.Discover(ctx, p, r.library, reading.DiscoverLimits{})
	if err != nil || len(restricted.Recent) != 0 || len(restricted.Series) != 0 {
		t.Fatalf("restricted discover=%+v %v", restricted, err)
	}
	if owner, err := r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{}); err != nil || len(owner.Recent) == 0 {
		t.Fatalf("owner discover=%+v %v", owner, err)
	}
}
