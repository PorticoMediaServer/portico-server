package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/personalstate"
	"portico.local/server/internal/reading"
)

func TestReadingDiscoverSeriesCountsWholeReadyMembershipBeforePreviewLimit(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	var items []string
	for i := 0; i < 5; i++ {
		path := readingTreeFile(t, r, fmt.Sprintf("Author/Saga/Title %d.epub", i))
		readingScan(t, r.f, r.library)
		items = append(items, readingPathItem(t, r, path))
	}
	for i, item := range items {
		name := "Saga"
		if i == 0 {
			name = "SAGA"
		}
		readingSeriesOwnerField(t, r, item, "series", name)
		readingSeriesOwnerField(t, r, item, "seriesNumber", fmt.Sprint(i+1))
	}
	tx, err := r.f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 4} {
		var entity int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, items[i]).Scan(&entity); err != nil {
			t.Fatal(err)
		}
		if err = personalstate.Write(tx, identity.PersonalKey(r.p.Viewer), entity, true); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = r.f.db.Exec(`UPDATE library_sources SET enabled=0 WHERE library_id=?`, r.library); err != nil {
		t.Fatal(err)
	}
	readingSeriesDrain(t, r)
	out, err := r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{Recent: 1, Series: 1})
	if err != nil || len(out.Series) != 1 {
		t.Fatalf("series composition %+v %v", out, err)
	}
	series := out.Series[0]
	if series.ID != compactcatalog.ReadingSeriesID(r.library, "saga") || series.LibraryID != r.library || series.Name != "Saga" || series.Count != 5 || series.ReadCount != 2 || len(series.Preview) != 3 {
		t.Fatalf("exact series %+v", series)
	}
	if out.Importing || out.Revision == nil {
		t.Fatalf("ready image facts %+v", out)
	}
	imageRevision := *out.Revision
	for i, item := range series.Preview {
		if item.ID != items[i] || item.InventoryAvailable || item.Series == nil || item.Series.ID != series.ID {
			t.Fatalf("canonical unavailable preview %+v", item)
		}
	}
	// A pending owner edit no longer withdraws the series composition. The
	// unconditional read answers from the published ready image while input is
	// still pending, reports that importing state from the same projection
	// snapshot, and keeps the image's membership and revision rather than input's.
	readingSeriesOwnerField(t, r, items[0], "series", "Changed")
	readingSeriesInputPending(t, r)
	out, err = r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{Series: 1})
	if err != nil || len(out.Series) != 1 {
		t.Fatalf("pending input series %+v %v", out, err)
	}
	if !out.Importing {
		t.Fatalf("pending input must report importing: %+v", out)
	}
	if out.Revision == nil || *out.Revision != imageRevision {
		t.Fatalf("pending input revision %+v want %+v", out.Revision, imageRevision)
	}
	published := out.Series[0]
	if published.ID != series.ID || published.Name != "Saga" || published.Count != 5 || published.ReadCount != 2 || len(published.Preview) != 3 {
		t.Fatalf("pending input must keep the published membership %+v", published)
	}
	for i, item := range published.Preview {
		if item.ID != items[i] {
			t.Fatalf("pending input preview %d %+v", i, item)
		}
	}
	// The pending "Changed" series is not part of the published image.
	for _, entry := range out.Series {
		if entry.Name == "Changed" || entry.ID != series.ID {
			t.Fatalf("pending input leaked into the published image %+v", entry)
		}
	}
	// An existing caller that did not opt into ready series keeps its recent row.
	if _, err = r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{}); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{-1, 21} {
		if _, err = r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{Series: limit}); !errors.Is(err, reading.ErrInvalid) {
			t.Fatalf("invalid limit %d: %v", limit, err)
		}
	}
	if _, err = r.s.Discover(ctx, r.p, "", reading.DiscoverLimits{Series: 1}); !errors.Is(err, reading.ErrInvalid) {
		t.Fatalf("mixed-library series %v", err)
	}
}

func TestReadingDiscoverSeriesUsesCurrentProfilePolicyAndReadState(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	// Two real titles share one series. One is unrated and one is rated PG, so a
	// live profile ceiling admits only the unrated title: membership and count
	// are filtered at read time, never resolved once and cached.
	ratedPath := readingTreeFile(t, r, "Author/Protected series/Rated.epub")
	readingScan(t, r.f, r.library)
	rated := readingPathItem(t, r, ratedPath)
	readingSeriesOwnerField(t, r, r.item, "series", "Protected series")
	readingSeriesOwnerField(t, r, rated, "series", "Protected series")
	c := catalogtest.New(t, r.f.db)
	c.Attributes(c.ID(rated), "contentRating", "PG")
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('pg',8) ON CONFLICT(value_key) DO UPDATE SET minimum_age=excluded.minimum_age`)
	readingSeriesDrain(t, r)
	owner := readingOwnerLogin(t, r)
	profiles, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Series discover reader", "blue")
	if err != nil {
		t.Fatal(err)
	}
	var child identity.DirectProfile
	for _, profile := range profiles.Profiles {
		if !profile.Primary && profile.Name == "Series discover reader" {
			child = profile
		}
	}
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 1, AllowUnrated: true, RatingSystem: "mpaa", MaximumAgeRating: "G"}); err != nil {
		t.Fatal(err)
	}
	selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.f.id.Authenticate(selected.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	// The owner's own read is the published image; the child sees its filtered
	// member and count, not the owner's.
	if ownerOut, err := r.s.Discover(ctx, r.p, r.library, reading.DiscoverLimits{Series: 1}); err != nil || len(ownerOut.Series) != 1 || ownerOut.Series[0].Count != 2 {
		t.Fatalf("owner series %+v %v", ownerOut, err)
	}
	out, err := r.s.Discover(ctx, p, r.library, reading.DiscoverLimits{Series: 1})
	if err != nil || len(out.Series) != 1 || out.Series[0].Count != 1 || out.Series[0].ReadCount != 0 || len(out.Series[0].Preview) != 1 || out.Series[0].Preview[0].ID != r.item {
		t.Fatalf("profile series %+v %v", out, err)
	}
	// Pending input is not a reason to widen live privacy: the published image
	// stays the same, its hidden member stays absent, and its count stays filtered.
	readingSeriesOwnerField(t, r, r.item, "series", "Pending rename")
	readingSeriesInputPending(t, r)
	out, err = r.s.Discover(ctx, p, r.library, reading.DiscoverLimits{Series: 1})
	if err != nil || len(out.Series) != 1 || out.Series[0].Name != "Protected series" || out.Series[0].Count != 1 || len(out.Series[0].Preview) != 1 || out.Series[0].Preview[0].ID != r.item {
		t.Fatalf("pending import leaked a hidden member %+v %v", out, err)
	}
	if !out.Importing || out.Revision == nil {
		t.Fatalf("pending import facts %+v", out)
	}
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 2, AllowUnrated: false, RatingSystem: "mpaa", MaximumAgeRating: "G"}); err != nil {
		t.Fatal(err)
	}
	out, err = r.s.Discover(ctx, p, r.library, reading.DiscoverLimits{Series: 1})
	if err != nil || len(out.Series) != 0 {
		t.Fatalf("restricted series leaked %+v %v", out, err)
	}
	if _, err = r.f.id.EditDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectProfileEdit{ExpectedRevision: child.Revision, Policy: &identity.ProfilePolicy{AllowedLibraries: []string{}}}); err != nil {
		t.Fatal(err)
	}
	out, err = r.s.Discover(ctx, p, r.library, reading.DiscoverLimits{Series: 1})
	if err == nil || len(out.Series) != 0 || len(out.Recent) != 0 || out.Revision != nil || out.Importing {
		t.Fatalf("revoked library returned Discover facts %+v %v", out, err)
	}
}

func TestReadingDiscoverSeriesHTTPOptInAndActualAuthority(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	readingSeriesOwnerField(t, r, r.item, "series", "Actual series")
	readingSeriesDrain(t, r)
	h := New(r.d)
	path := "/v1/reading/discover?libraryId=" + r.library
	w := readingHTTP(t, h, "GET", path, r.f.owner.AccessToken, "", nil)
	var ordinary ReadingDiscoverResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &ordinary) != nil || len(ordinary.Rows) != 1 || ordinary.Rows[0].Series != nil {
		t.Fatalf("legacy recent request %d %s", w.Code, w.Body.String())
	}
	if ordinary.Revision != nil || ordinary.Importing {
		t.Fatalf("recent-only read attached series facts %+v", ordinary)
	}
	path += "&seriesLimit=1"
	w = readingHTTP(t, h, "GET", path, r.f.owner.AccessToken, "", nil)
	var out ReadingDiscoverResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Rows) != 2 || out.Rows[1].Key != "series" || len(out.Rows[1].Series) != 1 {
		t.Fatalf("series response %d %s", w.Code, w.Body.String())
	}
	assertSpecResponse(t, "GET", "/v1/reading/discover", w)
	if out.Rows[1].Series[0].Count != 1 || out.Rows[1].Series[0].Preview[0].ID != r.item {
		t.Fatalf("actual summary %+v", out)
	}
	if out.Importing || out.Revision == nil {
		t.Fatalf("ready image facts %+v", out)
	}
	for _, bad := range []string{"/v1/reading/discover?seriesLimit=1", path + "&seriesLimit=2", "/v1/reading/discover?libraryId=" + r.library + "&seriesLimit=0", "/v1/reading/discover?libraryId=" + r.library + "&seriesLimit="} {
		w = readingHTTP(t, h, "GET", bad, r.f.owner.AccessToken, "", nil)
		if w.Code != 400 {
			t.Fatalf("invalid series query %d %s", w.Code, w.Body.String())
		}
	}
	// The pending edit is not served as part of the published image: the
	// unconditional request still answers 200 with the ready summary, its
	// importing state, and the revision of that same image.
	readingSeriesOwnerField(t, r, r.item, "series", "Changed")
	readingSeriesInputPending(t, r)
	w = readingHTTP(t, h, "GET", path, r.f.owner.AccessToken, "", nil)
	var published ReadingDiscoverResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &published) != nil || len(published.Rows) != 2 || published.Rows[0].Key != "recent" || len(published.Rows[0].Items) != 1 || published.Rows[1].Key != "series" || len(published.Rows[1].Series) != 1 {
		t.Fatalf("pending input series %d %s", w.Code, w.Body.String())
	}
	ready := out.Rows[1].Series[0]
	// Published membership, identity and count come from the ready image only;
	// every served summary must be that exact series, so a pending summary or
	// group cannot escape under another row.
	for _, row := range published.Rows {
		for _, summary := range row.Series {
			if summary.ID != ready.ID || summary.LibraryID != r.library || summary.Name != "Actual series" || summary.Count != 1 || summary.ReadCount != ready.ReadCount || len(summary.Preview) != 1 || summary.Preview[0].ID != r.item {
				t.Fatalf("pending input must keep the published summary %+v", summary)
			}
		}
	}
	if summary := published.Rows[1].Series[0]; summary.ID != compactcatalog.ReadingSeriesID(r.library, "Actual series") {
		t.Fatalf("published series identity %+v", summary)
	}
	if !published.Importing || published.Revision == nil || *published.Revision != *out.Revision {
		t.Fatalf("pending input facts %+v want revision %+v", published, out.Revision)
	}
	// Item-level descriptive metadata is live presentation, not published
	// membership: both the recent row and the preview item carry the pending
	// owner value while the summary membership and count stay frozen.
	for _, item := range []ReadingItem{published.Rows[0].Items[0], published.Rows[1].Series[0].Preview[0]} {
		if item.ID != r.item || item.Series == nil || item.Series.Name != "Changed" || item.Series.Source != ReadingAttributionSource(compactcatalog.ReadingOwner) {
			t.Fatalf("live item detail %+v", item)
		}
	}
	readingSeriesDrain(t, r)
	if err := r.f.id.LogoutToken(context.Background(), r.f.owner.AccessToken); err != nil {
		t.Fatal(err)
	}
	w = readingHTTP(t, h, "GET", path, r.f.owner.AccessToken, "", nil)
	if w.Code != 401 {
		t.Fatalf("retired family %d %s", w.Code, w.Body.String())
	}
}

// readingSeriesInputPending fails unless the library's real Reading projection
// has input newer than its published ready generation. It is the fixture's
// independent proof that the pending state is real, so a 200 importing response
// cannot pass by serving input or by reporting a state the projection lacks.
func readingSeriesInputPending(t *testing.T, r *readingFixture) {
	t.Helper()
	var input, ready int64
	var phase string
	if err := r.f.db.QueryRow(`SELECT s.input_generation,s.ready_generation,s.phase FROM reading_attribution_state s JOIN catalog_libraries c ON c.id=s.library_id WHERE c.library_id=?`, r.library).Scan(&input, &ready, &phase); err != nil {
		t.Fatal(err)
	}
	if ready == 0 || input <= ready || phase == "ready" {
		t.Fatalf("Reading input is not pending: input=%d ready=%d phase=%s", input, ready, phase)
	}
}
