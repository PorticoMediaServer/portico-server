package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
)

func TestAttentionDismissalAndLowConfidenceSurviveRefresh(t *testing.T) {
	s, db, _ := screenFixture(t)
	item := publicationFixtureMovie(t, db)
	screenConfirm(t, s)
	ctx := context.Background()
	publicationExec(t, db, `UPDATE screen_metadata_work SET year_seed=0,title_seed='Film',status='pending' WHERE target_id=?`, item.ID)
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	page, e := s.Attention(ctx, "lib", "", 25)
	if e != nil || len(page.Entries) != 1 || page.Entries[0].Reason != "low_confidence" || len(page.Entries[0].Candidates) != 1 {
		t.Fatal(page, e)
	}
	entry := page.Entries[0]
	if e = s.AttentionAction(ctx, "lib", AttentionCommand{Kind: entry.Kind, EntityID: entry.EntityID, ExpectedRevision: entry.Revision, Action: "dismiss"}, MBActor{}, nil); e != nil {
		t.Fatal(e)
	}
	page, e = s.Attention(ctx, "lib", "", 25)
	if e != nil || len(page.Entries) != 0 {
		t.Fatal(page, e)
	}
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='pending',generation=generation+1 WHERE target_id=?`, item.ID)
	if e = s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	page, e = s.Attention(ctx, "lib", "", 25)
	if e != nil || len(page.Entries) != 1 || page.Entries[0].Reason != "low_confidence" {
		t.Fatal("refresh promoted a guess to certainty", page, e)
	}
	entry = page.Entries[0]
	if e = s.AttentionAction(ctx, "lib", AttentionCommand{Kind: entry.Kind, EntityID: entry.EntityID, ExpectedRevision: entry.Revision, Action: "accept", CandidateKey: entry.Candidates[0].Key, Order: "official"}, MBActor{Authority: "direct", AccountID: "owner"}, nil); e != nil {
		t.Fatal(e)
	}
	if e = s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	page, e = s.Attention(ctx, "lib", "", 25)
	if e != nil || len(page.Entries) != 0 {
		t.Fatal(page, e)
	}
}
func TestAttentionScopeAndRevisionAreEnforced(t *testing.T) {
	s, db, _ := screenFixture(t)
	item := publicationFixtureMovie(t, db)
	ctx := context.Background()
	page, e := s.Attention(ctx, "lib", "", 1)
	if e != nil || len(page.Entries) != 1 {
		t.Fatal(page, e)
	}
	entry := page.Entries[0]
	in := AttentionCommand{Kind: "item", EntityID: item.Public, ExpectedRevision: entry.Revision + 1, Action: "dismiss"}
	if e = s.AttentionAction(ctx, "lib", in, MBActor{}, nil); !errors.Is(e, ErrScreenConflict) {
		t.Fatal(e)
	}
	in.ExpectedRevision = entry.Revision
	if e = s.AttentionAction(ctx, "another-library", in, MBActor{}, nil); !errors.Is(e, ErrRepairInput) {
		t.Fatal(e)
	}
	denied := errors.New("denied")
	if e = s.AttentionAction(ctx, "lib", in, MBActor{}, func(_ *sql.Tx) error { return denied }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
}
func TestNFORetirementReleasesFieldOwnership(t *testing.T) {
	s, db, p := screenFixture(t)
	item := publicationFixtureMovie(t, db)
	screenConfirm(t, s)
	ctx := context.Background()
	write, e := dbwork.Begin(ctx, db, dbwork.ClassInteractive)
	if e != nil {
		t.Fatal(e)
	}
	b, e := readScreenBase(ctx, write.Tx(), "item", item.Public)
	if e != nil {
		t.Fatal(e)
	}
	b.NFO = []assets.VideoNFO{{Title: "Local title", Overview: "Local plot"}}
	if e = s.applyScreenLocal(ctx, write.Tx(), b); e != nil {
		t.Fatal(e)
	}
	if e = write.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if got := publicationScalar(t, db, `SELECT overview FROM catalog_item_details WHERE entity_id=?`, item.ID); got != p.record.Overview {
		t.Fatal("removed NFO still owns overview", got)
	}
}
func TestMetadataSettingsNeverReturnCredentials(t *testing.T) {
	s, _, _ := screenFixture(t)
	ctx := context.Background()
	current, e := s.MetadataSettings(ctx)
	if e != nil {
		t.Fatal(e)
	}
	token := "owner-test-token"
	key := "owner-test-key"
	out, e := s.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: 4 << 30, TMDBReadToken: &token, TVDBProjectKey: &key, IncludeAdult: true}, nil)
	if e != nil || out.LocalArtworkLimitBytes != 4<<30 || !out.TMDBCredentialConfigured || !out.TVDBCredentialConfigured || !out.IncludeAdult {
		t.Fatal(out, e)
	}
	raw, e := json.Marshal(out)
	if e != nil || strings.Contains(string(raw), token) || strings.Contains(string(raw), key) {
		t.Fatal("credential returned", e)
	}
	if _, e = s.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: 1 << 30}, nil); !errors.Is(e, ErrScreenConflict) {
		t.Fatal(e)
	}
}

func TestLocaleChangeAndExplicitShowRefreshBypassHeldDocuments(t *testing.T) {
	s, db, p, _, _, settle := episodeFreshnessFixture(t)
	settle()
	before := p.showCalls
	policy, e := s.ScreenPolicy(context.Background(), "tv")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateScreenPolicy(context.Background(), "tv", ScreenPolicyUpdate{ExpectedRevision: policy.Revision, ExpectedConsentRevision: policy.ConsentRevision, Enabled: true, Providers: []string{"tmdb"}, Language: "fr-FR", Region: "FR", RefreshMode: "replace_unlocked"}, nil); e != nil {
		t.Fatal(e)
	}
	settle()
	if p.showCalls <= before {
		t.Fatal("language change reused old provider body")
	}
	var show string
	if e = db.QueryRow(`SELECT pid(public_id) FROM catalog_entities WHERE kind=2 AND library_id=(SELECT id FROM catalog_libraries WHERE library_id='tv')`).Scan(&show); e != nil {
		t.Fatal(e)
	}
	before = p.showCalls
	if e = s.QueueRefresh(show); e != nil {
		t.Fatal(e)
	}
	settle()
	if p.showCalls <= before {
		t.Fatal("explicit show refresh reused held body")
	}
}
