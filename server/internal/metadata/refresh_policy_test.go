package metadata

import (
	"context"
	"testing"
	"time"
)

// staleHeldDocument ages the settled item's document beyond every window, so
// only the owner's policy decides whether a wake asks the provider again.
func staleHeldDocument(t *testing.T, s *Service) {
	t.Helper()
	if e := s.noteDocumentChecked(context.Background(), "tmdb", "movie", screenDocumentResource("42", "en-US", "CA"), documentValidators{ETag: `W/"v1"`}, true, s.publicationTime().UTC().Add(-90*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
}

// Check for new details: "never" and "recent" leave an old, ordinary title
// alone; "regular" asks for it again.
func TestRefreshPolicyDecidesWhichHeldTitlesAreAskedForAgain(t *testing.T) {
	for policy, wantAsked := range map[string]bool{"regular": true, "recent": false, "never": false} {
		t.Run(policy, func(t *testing.T) {
			ctx := context.Background()
			s, db, p, item := freshnessFixture(t)
			settleScreenPublication(t, s, db, item)
			// The fixture movie was added just now; make it an ordinary title.
			publicationExec(t, db, `UPDATE catalog_item_details SET added_text='2001-01-01T00:00:00Z' WHERE entity_id=?`, item.ID)
			s.refreshPolicy.Store(policy)
			staleHeldDocument(t, s)
			before := p.fetches()
			publicationExec(t, db, `UPDATE screen_metadata_work SET status='pending',lease='',lease_until='' WHERE target_id=?`, item.ID)
			if e := s.ScreenStep(ctx); e != nil {
				t.Fatal(e)
			}
			if asked := p.fetches() > before; asked != wantAsked {
				t.Fatalf("asked=%v", asked)
			}
			if publicationScalar(t, db, `SELECT status FROM screen_metadata_work WHERE target_id=?`, item.ID) != "matched" {
				t.Fatal("the title was left unsettled")
			}
		})
	}
}

// Refresh all wakes every held title through a paced sweep and makes each
// document stale even under "never", while still spending the allowance.
func TestRefreshAllWakesHeldTitlesWithinTheAllowance(t *testing.T) {
	ctx := context.Background()
	s, db, p, item := freshnessFixture(t)
	settleScreenPublication(t, s, db, item)
	s.refreshPolicy.Store("never")
	settings, e := s.RefreshAll(ctx, nil)
	if e != nil || !settings.RefreshAll.Running || settings.RefreshAll.RequestedAt == "" {
		t.Fatal(e, settings.RefreshAll)
	}
	revision := settings.Revision
	if e = s.RefreshAllStep(ctx); e != nil {
		t.Fatal(e)
	}
	if got := publicationScalar(t, db, `SELECT status FROM screen_metadata_work WHERE target_id=?`, item.ID); got != "pending" {
		t.Fatal("the sweep did not wake the held title:", got)
	}
	if publicationScalar(t, db, `SELECT refresh_all_running FROM metadata_runtime_settings`) != "0" {
		t.Fatal("a short page must finish the sweep")
	}
	before := p.fetches()
	if e = s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.fetches() <= before {
		t.Fatal("Refresh all did not reach the provider under the never policy")
	}
	after, e := s.MetadataSettings(ctx)
	if e != nil || after.Revision != revision || after.RefreshAll.Running {
		t.Fatal("Refresh all is an action: it must not move the settings revision", e, after.Revision, revision)
	}

	// With the allowance spent, a new sweep waits instead of waking anything.
	publicationExec(t, db, `INSERT INTO metadata_refresh_budget(provider,span,window_start,spent) VALUES('tmdb','hour',?,?) ON CONFLICT(provider,span) DO UPDATE SET window_start=excluded.window_start,spent=excluded.spent`, s.publicationTime().Unix(), refreshBudgetHour)
	if _, e = s.RefreshAll(ctx, nil); e != nil {
		t.Fatal(e)
	}
	if e = s.RefreshAllStep(ctx); e != nil {
		t.Fatal(e)
	}
	if got := publicationScalar(t, db, `SELECT status FROM screen_metadata_work WHERE target_id=?`, item.ID); got != "matched" {
		t.Fatal("a sweep with no allowance left woke a title:", got)
	}
	if publicationScalar(t, db, `SELECT refresh_all_running FROM metadata_runtime_settings`) != "1" {
		t.Fatal("the waiting sweep must keep its place")
	}
}

// Cast and crew photos off drops film and series portraits from discovery,
// and leaves every other image and a music artist's portrait alone.
func TestCastPhotosOffDropsCastPortraits(t *testing.T) {
	s := &Service{}
	images := []discoveredArtwork{{role: "poster"}, {role: "portrait", subject: "tmdb:1"}, {role: "portrait", subject: "tvdb:2"}, {role: "portrait", subject: "artist"}}
	s.castPhotos.Store(true)
	if got := s.withoutCastPhotos(append([]discoveredArtwork{}, images...)); len(got) != 4 {
		t.Fatal("on keeps every image", got)
	}
	s.castPhotos.Store(false)
	got := s.withoutCastPhotos(append([]discoveredArtwork{}, images...))
	if len(got) != 2 || got[0].role != "poster" || got[1].subject != "artist" {
		t.Fatal("off drops cast portraits only", got)
	}
}
