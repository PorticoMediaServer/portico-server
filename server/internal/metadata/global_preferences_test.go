package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/metadataprovider"
)

func saveGlobal(t *testing.T, s *Service, change func(*MetadataSettingsUpdate)) MetadataSettings {
	t.Helper()
	current, e := s.MetadataSettings(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	in := MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: current.LocalArtworkLimitBytes, IncludeAdult: current.IncludeAdult}
	change(&in)
	out, e := s.UpdateMetadataSettings(context.Background(), in, nil)
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestGlobalMetadataSettingsProducerDefaultsSecretsAndRevision(t *testing.T) {
	s, db, _ := screenFixture(t)
	ctx := context.Background()
	initial, e := s.MetadataSettings(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if initial.DefaultLanguage != "en" || initial.DefaultRegion != "US" || initial.ArtworkPreference != "folders" || initial.ArtworkLanguage != "same" || initial.RefreshPolicy != "regular" || initial.FallbackLanguage != "en" || initial.AnimeTitles != "language" || initial.ArtworkMaxEdge != 1920 || !initial.CastPhotos {
		t.Fatalf("wrong defaults %+v", initial)
	}
	if e = s.ConfigureAcoustID("environment-acoustid"); e != nil {
		t.Fatal(e)
	}
	environment := s.acousticConfiguration()
	language, region, preference, artLanguage, refresh, fallback, anime, key := "fr", "CA", "online", "textless", "never", "blank", "romaji", "owner-private-key"
	edge, photos := 2560, false
	out := saveGlobal(t, s, func(in *MetadataSettingsUpdate) {
		in.DefaultLanguage = &language
		in.DefaultRegion = &region
		in.ArtworkPreference = &preference
		in.ArtworkLanguage = &artLanguage
		in.RefreshPolicy = &refresh
		in.FallbackLanguage = &fallback
		in.AnimeTitles = &anime
		in.ArtworkMaxEdge = &edge
		in.CastPhotos = &photos
		in.AcoustIDKey = &key
	})
	if !out.AcoustIDKeyConfigured || out.DefaultLanguage != "fr" || out.ArtworkMaxEdge != 2560 || out.CastPhotos || s.acousticConfiguration().digest == environment.digest {
		t.Fatal(out)
	}
	data, _ := json.Marshal(out)
	if strings.Contains(string(data), key) || strings.Contains(string(data), "environment-acoustid") || strings.Contains(string(data), "acoustidKey\"") {
		t.Fatalf("secret echoed %s", data)
	}
	c := catalogtest.New(t, db)
	c.Library("new", "Nouveau", "movie", "/new")
	for _, table := range []string{"screen_metadata_policies", "metadata_provider_policies"} {
		if got := publicationScalar(t, db, `SELECT language||':'||region FROM `+table+` WHERE library_id='new'`); got != "fr:CA" {
			t.Fatal(table, got)
		}
	}
	if got := publicationScalar(t, db, `SELECT language FROM screen_metadata_policies WHERE library_id='lib'`); got != "en" {
		t.Fatal("existing library changed", got)
	}
	empty := ""
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.AcoustIDKey = &empty })
	if s.acousticConfiguration().digest != environment.digest {
		t.Fatal("clearing did not restore environment")
	}
	stale, _ := s.MetadataSettings(ctx)
	screenWithdraw(t, s)
	if _, e = s.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: stale.Revision, LocalArtworkLimitBytes: stale.LocalArtworkLimitBytes}, nil); !errors.Is(e, ErrScreenConflict) {
		t.Fatal("stale consent revision accepted", e)
	}
	for _, bad := range []string{"bad", "folders|online", ""} {
		if _, e = s.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: 1, LocalArtworkLimitBytes: 1, ArtworkPreference: &bad}, nil); !errors.Is(e, ErrRepairInput) {
			t.Fatal("invalid enum accepted", bad, e)
		}
	}
	denied := errors.New("denied")
	now, _ := s.MetadataSettings(ctx)
	if _, e = s.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: now.Revision, LocalArtworkLimitBytes: 1}, func(*sql.Tx) error { return denied }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
}
func TestGlobalOnlineWithdrawalStopsActualMetadataProducersAndKeepsLibraryChoices(t *testing.T) {
	s, db, p := screenFixture(t)
	item := publicationFixtureMovie(t, db)
	screenConfirm(t, s)
	ctx := context.Background()
	off := false
	out := saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.OnlineLookups = &off })
	if out.OnlineLookups {
		t.Fatal(out)
	}
	before := p.calls
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.calls != before {
		t.Fatal("provider called with global lookup off")
	}
	if got := publicationScalar(t, db, `SELECT enabled FROM screen_metadata_policies WHERE library_id='lib'`); got != "1" {
		t.Fatal("global switch erased library choice", got)
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	enabled, e := s.artworkProviderEnabled(ctx, tx, RepairTarget{"item", item.Public}, "tmdb")
	tx.Rollback()
	if e != nil || enabled {
		t.Fatal("artwork provider bypass", enabled, e)
	}
	if _, e = s.RefreshAllMetadata(ctx, MetadataRefreshRequest{ExpectedRevision: out.Revision}, nil); !errors.Is(e, ErrLocalMetadataOnly) {
		t.Fatal(e)
	}
	on := true
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.OnlineLookups = &on })
	if e = s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.calls <= before {
		t.Fatal("online lookup did not resume")
	}
}
func TestGlobalRefreshPolicyAffectsHeldDocumentsAndExplicitRefreshAllowance(t *testing.T) {
	s, db, p, item := freshnessFixture(t)
	ctx := context.Background()
	settleScreenPublication(t, s, db, item)
	now := s.publicationTime()
	if e := s.noteDocumentChecked(ctx, "tmdb", "movie", screenDocumentResource("42", "en-US", "CA"), documentValidators{}, true, now.Add(-100*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	never := "never"
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.RefreshPolicy = &never })
	before := p.fetches()
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='pending',lease='',lease_until='' WHERE target_id=?`, item.ID)
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.fetches() != before {
		t.Fatal("never refreshed held evidence")
	}
	if e := s.QueueRefresh(item.Public); e != nil {
		t.Fatal(e)
	}
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.fetches() <= before {
		t.Fatal("explicit refresh ignored")
	}
	for n := 0; n < refreshBudgetHour; n++ {
		if _, e := s.admitRefresh(ctx, "tmdb", now); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.QueueRefresh(item.Public); e != nil {
		t.Fatal(e)
	}
	before = p.fetches()
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.fetches() != before || publicationScalar(t, db, `SELECT error FROM screen_metadata_work WHERE target_id=?`, item.ID) != "refresh_budget" {
		t.Fatal("explicit refresh bypassed allowance")
	}
}
func TestGlobalRefreshSweepIsBoundedDurableAndPreservesOwnerChoices(t *testing.T) {
	s, db, p, item := freshnessFixture(t)
	ctx := context.Background()
	settleScreenPublication(t, s, db, item)
	current, e := s.MetadataSettings(ctx)
	if e != nil {
		t.Fatal(e)
	}
	status, e := s.RefreshAllMetadata(ctx, MetadataRefreshRequest{current.Revision}, nil)
	if e != nil || !status.Queueing {
		t.Fatal(status, e)
	}
	before := p.fetches()
	for n := 0; n < 4; n++ {
		if e = s.GlobalRefreshStep(ctx); e != nil {
			t.Fatal(e)
		}
	}
	status, e = s.MetadataRefreshStatus(ctx)
	if e != nil || status.Queueing || status.Queued < 1 || status.EnqueuedAt == "" {
		t.Fatal(status, e)
	}
	if p.fetches() != before {
		t.Fatal("sweep performed network inside writer")
	}
	if got := publicationScalar(t, db, `SELECT force_refresh FROM screen_metadata_work WHERE target_id=?`, item.ID); got != "1" {
		t.Fatal(got)
	}
	if e = s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if p.fetches() <= before {
		t.Fatal("queued sweep did not reach actual provider")
	}
}

func TestGlobalArtworkSelectionPreferenceKeepsOwnerLocksAndFolderFallback(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	entity := resolveArtworkTestEntity(t, db, target)
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(p08cPNG(t, 20, 20))), Header: http.Header{}}, nil
	})}
	if e := s.seedArtwork(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.ArtworkStep(ctx); e != nil {
		t.Fatal(e)
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	fence, e := artworkFence(ctx, tx, target)
	if e != nil {
		t.Fatal(e)
	}
	// The production publication guard must yield to folders under the default.
	localJob := artworkWork{target: target, role: "poster", provider: "local", fence: fence}
	held, e := artworkSlotHeld(ctx, tx, localJob, entity)
	tx.Rollback()
	if e != nil || held {
		t.Fatal("folder preference did not replace automatic online art", held, e)
	}
	online := "online"
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkPreference = &online })
	// Update the candidate's fence to simulate a current provider choice following the preference change.
	tx, e = db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	fence, e = artworkFence(ctx, tx, target)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(ctx, `UPDATE artwork_candidates SET source_fence=? WHERE id=(SELECT candidate_id FROM artwork_selections WHERE kind='item' AND entity_id=? AND role='poster')`, fence, entity); e != nil {
		t.Fatal(e)
	}
	localJob.fence = fence
	held, e = artworkSlotHeld(ctx, tx, localJob, entity)
	if e != nil || !held {
		t.Fatal("online preference did not hold current online art", held, e)
	}
	if _, e = tx.ExecContext(ctx, `UPDATE artwork_selections SET locked=1 WHERE entity_id=?`, entity); e != nil {
		t.Fatal(e)
	}
	held, e = artworkSlotHeld(ctx, tx, localJob, entity)
	tx.Rollback()
	if e != nil || !held {
		t.Fatal("owner lock was displaced", held, e)
	}
}

func TestGlobalArtworkLanguagePhotosAndSizeUseProductionConsumers(t *testing.T) {
	s, _, _ := screenFixture(t)
	ctx := context.Background()
	textless, photos, small := "textless", false, 1280
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) {
		in.ArtworkLanguage = &textless
		in.CastPhotos = &photos
		in.ArtworkMaxEdge = &small
	})
	requests := 0
	s.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		q := r.URL.Query()
		if strings.Contains(q.Get("append_to_response"), "credits") || q.Get("include_image_language") != "null,fr,en" {
			t.Errorf("preferences did not reach request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"images":{"posters":[{"file_path":"/french.jpg","iso_639_1":"fr","vote_average":10},{"file_path":"/textless.jpg","iso_639_1":null,"vote_average":1}]},"credits":{"cast":[{"id":7,"profile_path":"/person.jpg"}]}}`))}, nil
	})}
	s.base = "http://fixture.invalid"
	images, e := s.discoverArtworkFor(ctx, "tmdb", "42", "item", "fr")
	if e != nil || len(images) != 2 || !strings.Contains(images[0].origin, "textless.jpg") || requests != 1 {
		t.Fatal(images, e, requests)
	}
	sized, e := s.artworkSizeContext(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, _, w, h, e := normalizeArtworkFormatsContext(sized, p08cPNG(t, 2200, 1100), map[string]bool{"png": true})
	if e != nil || w != 1280 || h != 640 {
		t.Fatal("saved size did not reach normalization", w, h, e)
	}
	_, _, w, h, e = normalizeArtworkFormatsContext(sized, p08cPNG(t, 200, 100), map[string]bool{"png": true})
	if e != nil || w != 200 || h != 100 {
		t.Fatal("small picture was enlarged", w, h, e)
	}
	large := 2560
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &large })
	sized, e = s.artworkSizeContext(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, _, w, h, e = normalizeArtworkFormatsContext(sized, p08cPNG(t, 2600, 1300), map[string]bool{"png": true})
	if e != nil || w != 2560 || h != 1280 {
		t.Fatal("large picture limit ignored", w, h, e)
	}
}

func TestGlobalArtworkLanguageSelectsRankedImageAfterSave(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	textless := "textless"
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkLanguage = &textless })
	s.base = "http://fixture.invalid"
	s.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"images":{"posters":[{"file_path":"/french.jpg","iso_639_1":"fr","vote_average":10},{"file_path":"/textless.jpg","iso_639_1":null,"vote_average":1}]}}`))}, nil
	})}
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(p08cPNG(t, 20, 20)))}, nil
	})}
	if e := s.ScreenStep(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.GlobalArtworkPreferencesStep(ctx); e != nil {
		t.Fatal(e)
	}
	// The ordinary seeder records the generic poster as evidence but does not
	// select it ahead of the owner's explicit artwork-language choice.
	if e := s.ArtworkDiscoveryStep(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.ArtworkStep(ctx); e != nil {
		t.Fatal(e)
	}
	entity := resolveArtworkTestEntity(t, db, target)
	got := publicationScalar(t, db, `SELECT c.origin FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE a.kind='item' AND a.entity_id=? AND a.role='poster'`, entity)
	if !strings.Contains(got, "textless.jpg") {
		t.Fatal("generic poster defeated the saved language preference", got)
	}
}

func TestGlobalMetadataTransactionFacadeRollsBackWithConfigurationImport(t *testing.T) {
	s, db, _ := screenFixture(t)
	ctx := context.Background()
	before, e := s.MetadataSettings(ctx)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	language := "de"
	out, e := ApplyMetadataSettingsTx(ctx, tx, MetadataSettingsUpdate{ExpectedRevision: before.Revision, LocalArtworkLimitBytes: before.LocalArtworkLimitBytes, DefaultLanguage: &language}, time.Now())
	if e != nil || out.DefaultLanguage != "de" {
		t.Fatal(out, e)
	}
	tx.Rollback()
	after, e := s.MetadataSettings(ctx)
	if e != nil || after.Revision != before.Revision || after.DefaultLanguage != before.DefaultLanguage {
		t.Fatal("rolled-back configuration changed metadata", after, e)
	}
}

func TestGlobalWithdrawalStopsMusicArtistChainBeforeNextProvider(t *testing.T) {
	s := musicArtistDB(t)
	calls := map[string]int{}
	mb, wd, wp, commons := musicArtistFakes(calls)
	s.wikidata = wd
	s.wikipedia = wp
	s.commons = commons
	mb.artist = func(context.Context, string) (metadataprovider.MusicArtist, error) {
		calls["mb"]++
		off := false
		saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.OnlineLookups = &off })
		return metadataprovider.MusicArtist{ID: mbArtistID, Name: "Artist", WikidataID: "Q123"}, nil
	}
	_, err := s.fetchMusicArtist(context.Background(), mb, mbArtistID)
	if !errors.Is(err, ErrLocalMetadataOnly) || calls["mb"] != 1 || calls["wikidata"] != 0 || calls["wikipedia"] != 0 || calls["commons"] != 0 {
		t.Fatal("withdrawal did not stop provider chain", calls, err)
	}
}
