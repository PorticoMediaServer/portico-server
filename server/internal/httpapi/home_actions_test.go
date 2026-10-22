package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/server/internal/administration"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/operations"
)

func TestHomeActionAuthorityUsesLiveOwnerAndUnavailableServices(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	d := f.d
	d.Metadata = metadata.New(f.db, "")
	d.Administration = administration.New(f.db)
	d.Scheduler = operations.NewScheduler(operations.New(f.db))
	d.v1 = d.PlaybackV1
	owner, err := d.Identity.Authenticate(f.token)
	if err != nil {
		t.Fatal(err)
	}
	member, err := d.Identity.Authenticate(f.kids.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	read := func(p identity.Principal) catalog.HomeActionAuthority {
		t.Helper()
		var authority catalog.HomeActionAuthority
		if err := dbwork.WithReadSnapshot(context.Background(), f.db, func(ctx context.Context) error {
			var e error
			authority, e = d.homeActionAuthority(ctx, p)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		return authority
	}
	if a := read(owner); !a.Personal || !a.Metadata || !a.Refresh || a.DeleteLibrary == nil || !a.Organize || !a.Playback || !a.Queue || a.Downloads {
		t.Fatalf("owner/service authority: %+v", a)
	}
	if a := read(member); !a.Personal || !a.Playback || a.Metadata || a.Refresh || a.DeleteLibrary != nil {
		t.Fatalf("member authority: %+v", a)
	}
	// A Hosted-shaped owner claim cannot reuse a valid local token's rights.
	hosted := owner
	hosted.Authority = "hosted"
	if a := read(hosted); a.Personal || a.Metadata || a.Refresh || a.DeleteLibrary != nil || a.Playback {
		t.Fatalf("role string granted Hosted authority: %+v", a)
	}
	d.Downloads, err = downloads.New(downloads.Options{DB: f.db})
	if err != nil {
		t.Fatal(err)
	}
	if a := read(member); !a.Downloads {
		t.Fatal("allowed download service omitted")
	}
	policy, err := d.Identity.ProfileRestrictions(context.Background(), f.token, member.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.Identity.SetProfileRestrictions(context.Background(), f.token, member.ProfileID, identity.ProfileRestrictionEdit{
		ExpectedRevision: policy.Revision, RatingSystem: policy.RatingSystem, MaximumAgeRating: policy.MaximumAgeRating, AllowUnrated: policy.AllowUnrated, BlockedLabels: policy.BlockedLabels,
		AllowDownloads: false, AllowLiveTV: policy.AllowLiveTV, AllowDVR: policy.AllowDVR, AllowWatchTogether: policy.AllowWatchTogether,
	}); err != nil {
		t.Fatal(err)
	}
	if a := read(member); a.Downloads {
		t.Fatal("download revocation retained a grant")
	}
	d.Metadata = nil
	d.v1 = nil
	if a := read(owner); a.Metadata || a.Refresh || a.Playback || a.Queue {
		t.Fatalf("unavailable services advertised: %+v", a)
	}
	f.c.Exec(`UPDATE authorization_access SET revoked=1 WHERE hash=?`, owner.Hash)
	if _, err := d.homeActionFenceAfter(httptest.NewRequest("GET", "/v1/home", nil), owner); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("post-composition family proof accepted revocation: %v", err)
	}
	if err := dbwork.WithReadSnapshot(context.Background(), f.db, func(ctx context.Context) error {
		_, err := d.homeActionAuthority(ctx, owner)
		return err
	}); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("revoked session was not rejected: %v", err)
	}
}

func TestHomeActionHTTPRecomposesLibraryPolicyAndWithholdsPrivateOffers(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	f.d.Metadata = metadata.New(f.db, "")
	f.d.Scheduler = operations.NewScheduler(operations.New(f.db))
	f.d.Administration = administration.New(f.db)
	f.h = New(f.d)
	manifest := homeAPIRequest(t, f.h, f.token, "GET", "/v1/home?limit=12", "")
	if manifest.Code != http.StatusOK {
		t.Fatalf("Home spotlight %d: %s", manifest.Code, manifest.Body.String())
	}
	var document catalog.HomeDocument
	if err := json.Unmarshal(manifest.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Spotlight) == 0 {
		t.Fatal("fixture returned no direct spotlight entries")
	}
	for _, candidate := range document.Spotlight {
		if candidate.Entry.HomeActions == nil || !candidate.Entry.HomeActions.MyList || !candidate.Entry.HomeActions.EditMetadata {
			t.Fatalf("direct spotlight grants: %+v", candidate.Entry)
		}
	}
	read := func(token string, ifNoneMatch string) (catalog.HomeRow, string) {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/home/rows/new_on_server?limit=12", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if ifNoneMatch != "" {
			r.Header.Set("If-None-Match", ifNoneMatch)
		}
		req := httptest.NewRecorder()
		f.h.ServeHTTP(req, r)
		if req.Code != http.StatusOK {
			t.Fatalf("Home row %d: %s", req.Code, req.Body.String())
		}
		if req.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("command offers may be reused without recomposition")
		}
		var row catalog.HomeRow
		if err := json.Unmarshal(req.Body.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		return row, req.Body.String()
	}
	before, _ := read(f.token, "")
	if before.ViewerFence != document.ViewerFence || before.Revision != document.Revision {
		t.Fatal("spotlight and row did not retain the same Home authority/revision")
	}
	if len(before.Entries) == 0 {
		t.Fatal("fixture returned no Home media")
	}
	for _, e := range before.Entries {
		if e.HomeActions == nil || !e.HomeActions.EditMetadata || e.HomeActions.Delete {
			t.Fatalf("initial owner offers: %+v", e.HomeActions)
		}
	}
	p, err := f.d.Identity.Authenticate(f.token)
	if err != nil {
		t.Fatal(err)
	}
	config, err := f.d.Administration.LibrarySettingsFor(context.Background(), f.d.administrationAuthority(p), "movies")
	if err != nil {
		t.Fatal(err)
	}
	config.Settings.AllowMediaDeletion = true
	if _, err = f.d.Administration.SaveLibrarySettings(context.Background(), f.d.administrationAuthority(p), "movies", administration.Change[administration.LibrarySettings]{ExpectedRevision: config.Revision, OperationID: "home-offer-deletion-policy", Settings: config.Settings}); err != nil {
		t.Fatal(err)
	}
	after, _ := read(f.token, `"old-home-validator"`)
	for _, e := range after.Entries {
		if e.LibraryID == "movies" && (e.HomeActions == nil || !e.HomeActions.Delete) {
			t.Fatalf("new library grant missing: %+v", e)
		}
	}
	f.restrictLiveTV(false)
	kid, raw := read(f.kids.AccessToken, "")
	for _, e := range kid.Entries {
		if e.HomeActions == nil || e.HomeActions.EditMetadata || e.HomeActions.Delete || e.LibraryID != "movies" || strings.Contains(e.Title, "Adult") || strings.Contains(e.Title, "Private") {
			t.Fatalf("restricted/member offers: %+v", e)
		}
	}
	for _, secret := range []string{"Northwind Adult", "Northwind Private", "/private/", "filePath", "apiKey", "password"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("private Home data leaked: %s", secret)
		}
	}
	if _, err = f.db.Exec(`UPDATE authorization_access SET revoked=1 WHERE hash=?`, p.Hash); err != nil {
		t.Fatal(err)
	}
	w := homeAPIRequest(t, f.h, f.token, "GET", "/v1/home/rows/new_on_server", "")
	if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "homeActions") {
		t.Fatalf("revoked Home returned offers: %d %s", w.Code, w.Body.String())
	}
}

func TestHomeChannelOffersUseActualGuideCommandsAndFeatureAuthority(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	w := homeAPIRequest(t, f.h, f.token, "GET", "/v1/home/rows/on_now?limit=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("On now %d: %s", w.Code, w.Body.String())
	}
	var row catalog.HomeRow
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row.Live == nil || len(row.Live.Channels) == 0 {
		t.Fatal("no actual guide channels")
	}
	for _, card := range row.Live.Channels {
		if card.Channel.SourceID == "" || card.Channel.ID == "" || !card.Actions.Favorite || card.Actions.Play || card.Channel.TuneAvailable {
			t.Fatalf("channel offers/identity: %+v", card)
		}
		encoded, err := json.Marshal(card.Actions)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != `{"play":false,"favorite":true}` {
			t.Fatalf("catalogue action fabricated for a channel: %s", encoded)
		}
	}
	f.restrictLiveTV(false)
	w = homeAPIRequest(t, f.h, f.kids.AccessToken, "GET", "/v1/home/rows/on_now?limit=2", "")
	if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "channel") || strings.Contains(w.Body.String(), "favorite") {
		t.Fatalf("denied channel offers leaked: %d %s", w.Code, w.Body.String())
	}
}
