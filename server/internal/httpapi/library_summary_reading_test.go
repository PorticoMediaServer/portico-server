package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/telemetry"
)

func TestLibrarySummaryCoreHTTPUsesLiveAuthorityAndReadyProjection(t *testing.T) {
	for _, kind := range []string{"book", "comic"} {
		t.Run(kind, func(t *testing.T) {
			format, want := "pdf", "ebook"
			if kind == "comic" {
				format, want = "cbz", "comic"
			}
			r := newReadingFixture(t, kind, format)
			readingSeriesDrain(t, r)
			h := New(r.d)
			check := func(token string, status int, present bool, count int) {
				t.Helper()
				w := readingHTTP(t, h, "GET", "/v1/libraries", token, "", nil)
				if w.Code != status {
					t.Fatalf("libraries: %d %s", w.Code, w.Body.String())
				}
				if status != http.StatusOK {
					return
				}
				var body struct {
					Items []catalog.Library `json:"items"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, library := range body.Items {
					if library.ID != r.library {
						continue
					}
					found = true
					summary := library.Summary
					if summary == nil || len(summary.Counts) != 1 || summary.Counts[0].Kind != want || summary.Counts[0].Count != count || summary.Empty != (count == 0) || !summary.EmptyKnown {
						t.Fatalf("Reading summary: %+v", summary)
					}
				}
				if found != present {
					t.Fatalf("library presence: %v want %v", found, present)
				}
				if !present && strings.Contains(w.Body.String(), r.library) {
					t.Fatal("revoked library fact escaped")
				}
			}
			check(r.f.owner.AccessToken, http.StatusOK, true, 1)
			before := telemetry.LibrarySummaryCacheCounters()
			check(r.f.owner.AccessToken, http.StatusOK, true, 1)
			if telemetry.LibrarySummaryCacheCounters()[telemetry.SummaryHitEntry] < before[telemetry.SummaryHitEntry]+1 {
				t.Fatalf("HTTP repeated read did not reuse core: before=%v after=%v", before, telemetry.LibrarySummaryCacheCounters())
			}

			check("", http.StatusUnauthorized, false, 0)
			readingSeriesOwnerField(t, r, r.item, "author", "Changed attribution")
			check(r.f.owner.AccessToken, http.StatusPreconditionFailed, false, 0)
			readingSeriesDrain(t, r)
			owner := readingOwnerLogin(t, r)
			profiles, err := r.f.id.CreateDirectProfile(context.Background(), owner.AccountToken, "Summary reader", "blue")
			if err != nil {
				t.Fatal(err)
			}
			var child identity.DirectProfile
			for _, profile := range profiles.Profiles {
				if !profile.Primary && profile.Name == "Summary reader" {
					child = profile
				}
			}
			// The owner's main profile records by default and a new profile does not;
			// Recordings is part of the summary policy, so give the child the same.
			if _, err = r.f.db.Exec(`INSERT INTO profile_restrictions(profile_id,allow_dvr) VALUES(?,1)`, child.ID); err != nil {
				t.Fatal(err)
			}
			selected, err := r.f.id.SelectDirectProfile(context.Background(), owner.AccountToken, child.ID, identity.DirectSelection{})
			if err != nil {
				t.Fatal(err)
			}
			before = telemetry.LibrarySummaryCacheCounters()
			check(selected.Session.AccessToken, http.StatusOK, true, 1)
			if telemetry.LibrarySummaryCacheCounters()[telemetry.SummaryHitEntry] < before[telemetry.SummaryHitEntry]+1 {
				t.Fatal("same-policy cross-user HTTP did not share")
			}
			rotated, err := r.f.id.RefreshSession(context.Background(), selected.Session.RefreshToken, selected.Session.InstallationID, identity.Token())
			if err != nil {
				t.Fatal(err)
			}
			selected.Session = rotated
			before = telemetry.LibrarySummaryCacheCounters()
			check(rotated.AccessToken, http.StatusOK, true, 1)
			if telemetry.LibrarySummaryCacheCounters()[telemetry.SummaryHitEntry] < before[telemetry.SummaryHitEntry]+1 {
				t.Fatal("session rotation invalidated core")
			}

			if _, err = r.f.id.SetProfileRestrictions(context.Background(), owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 1, AllowUnrated: false}); err != nil {
				t.Fatal(err)
			}
			check(selected.Session.AccessToken, http.StatusOK, true, 0)
			if _, err = r.f.id.EditDirectProfile(context.Background(), owner.AccountToken, child.ID, identity.DirectProfileEdit{ExpectedRevision: child.Revision, Policy: &identity.ProfilePolicy{AllowedLibraries: []string{}}}); err != nil {
				t.Fatal(err)
			}
			check(selected.Session.AccessToken, http.StatusOK, false, 0)
			if err = r.f.id.LogoutToken(context.Background(), r.f.owner.AccessToken); err != nil {
				t.Fatal(err)
			}
			check(r.f.owner.AccessToken, http.StatusUnauthorized, false, 0)
		})
	}
}
