package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestStablePersonIDRoundTripsCanonicalName(t *testing.T) {
	id := stablePersonID("  Zoë   Saldaña ")
	if !strings.HasPrefix(id, "person_") {
		t.Fatalf("stable person id=%q", id)
	}
	name, ok := personNameFromID(id)
	if !ok || name != "zoë saldaña" {
		t.Fatalf("person id round trip name=%q ok=%v", name, ok)
	}
	if _, ok := personNameFromID("person_not-base64!"); ok {
		t.Fatal("malformed person id was accepted")
	}
}

func TestPersonFallbackIdentitySurvivesCreditReorderAndRescan(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	var mediaID string
	if err := db.QueryRow(`SELECT id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 1`).Scan(&mediaID); err != nil {
		t.Fatalf("load media fixture: %v", err)
	}
	insertCredit := func(id string, sortOrder int) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO media_people (id, media_id, name, role, source, sort_order, provider_ids_json, created_at)
			VALUES (?, ?, 'Rescan Stable Performer', 'Actor', 'scanner', ?, '{}', '2026-01-01T00:00:00Z')`, id, mediaID, sortOrder); err != nil {
			t.Fatalf("insert credit %q: %v", id, err)
		}
	}
	personID := func() string {
		t.Helper()
		for _, person := range server.mediaPeopleFor(mediaID) {
			if person.Name == "Rescan Stable Performer" {
				return person.ID
			}
		}
		t.Fatal("rescan person was not projected into the cast")
		return ""
	}

	insertCredit("scanner-credit-order-0", 0)
	before := personID()
	if _, err := db.Exec(`DELETE FROM media_people WHERE id = 'scanner-credit-order-0'`); err != nil {
		t.Fatalf("remove pre-rescan credit: %v", err)
	}
	insertCredit("scanner-credit-order-19", 19)
	after := personID()
	if before == "" || before != after || strings.HasPrefix(before, "person_") || len(before) != 40 {
		t.Fatalf("opaque fallback person URL changed across reorder/rescan: before=%q after=%q", before, after)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var detail PersonDetailResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/people/"+after, nil, &detail)
	if status != http.StatusOK || detail.Person.ID != after || len(detail.Credits) != 1 {
		t.Fatalf("stable post-rescan person URL status=%d body=%s response=%#v", status, body, detail)
	}

	if err := server.replaceProviderMediaPeople(mediaID, []MediaPerson{{
		Name: "Portrait Stable Performer", Role: "Actor", Character: "First Role",
		ImageURL: "https://images.example.test/portrait-v1.jpg", SortOrder: 1,
	}}, "tmdb"); err != nil {
		t.Fatalf("seed portrait-backed person: %v", err)
	}
	portraitBefore := personIDInMediaPeople(server.mediaPeopleFor(mediaID), "Portrait Stable Performer")
	if portraitBefore == "" {
		t.Fatal("portrait-backed person was not projected")
	}
	if err := server.replaceProviderMediaPeople(mediaID, []MediaPerson{{
		Name: "Portrait Stable Performer", Role: "Actor", Character: "Second Role",
		ImageURL: "https://images.example.test/portrait-v2.jpg", SortOrder: 19,
	}}, "tmdb"); err != nil {
		t.Fatalf("refresh portrait-backed person: %v", err)
	}
	portraitAfter := personIDInMediaPeople(server.mediaPeopleFor(mediaID), "Portrait Stable Performer")
	if portraitAfter != portraitBefore {
		t.Fatalf("portrait or character refresh changed person URL: before=%q after=%q", portraitBefore, portraitAfter)
	}
	if _, err := db.Exec(`UPDATE media_people SET source = 'tvdb' WHERE media_id = ? AND name = 'Portrait Stable Performer'`, mediaID); err != nil {
		t.Fatalf("change person source: %v", err)
	}
	if sourceAfter := personIDInMediaPeople(server.mediaPeopleFor(mediaID), "Portrait Stable Performer"); sourceAfter != portraitBefore {
		t.Fatalf("source refresh changed person URL: before=%q after=%q", portraitBefore, sourceAfter)
	}
}

func TestNFOCharacterMetadataDoesNotBecomePersonIdentityRole(t *testing.T) {
	people := peopleFromNFO(nfoDocument{Actors: []nfoActor{{Name: "Alex Smith", Role: "The Pilot", Order: 4}}})
	if len(people) != 1 {
		t.Fatalf("people = %#v", people)
	}
	if people[0].Role != "Actor" || people[0].Character != "The Pilot" || people[0].SortOrder != 4 {
		t.Fatalf("actor credit = %#v", people[0])
	}
}

func TestPersonCanonicalIdentitySurvivesProviderEnrichmentAndReconcilesCredits(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	rows, err := db.Query(`SELECT id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 3`)
	if err != nil {
		t.Fatalf("load media fixtures: %v", err)
	}
	defer rows.Close()
	mediaIDs := []string{}
	for rows.Next() {
		var mediaID string
		if err := rows.Scan(&mediaID); err != nil {
			t.Fatalf("scan media fixture: %v", err)
		}
		mediaIDs = append(mediaIDs, mediaID)
	}
	if len(mediaIDs) < 3 {
		t.Fatalf("media fixtures = %#v", mediaIDs)
	}
	weak := MediaPerson{Name: "Provider Enriched Performer", Role: "Actor"}
	if err := server.replaceProviderMediaPeople(mediaIDs[0], []MediaPerson{weak}, "identity-test"); err != nil {
		t.Fatalf("seed weak credit: %v", err)
	}
	before := personIDInMediaPeople(server.mediaPeopleFor(mediaIDs[0]), weak.Name)
	if before == "" {
		t.Fatal("weak credit did not receive a public identity")
	}
	enriched := weak
	enriched.ProviderIDs = map[string]string{"tmdb": "person-8675309"}
	if err := server.replaceProviderMediaPeople(mediaIDs[0], []MediaPerson{enriched}, "identity-test"); err != nil {
		t.Fatalf("enrich weak credit: %v", err)
	}
	after := personIDInMediaPeople(server.mediaPeopleFor(mediaIDs[0]), weak.Name)
	if after != before {
		t.Fatalf("provider enrichment changed public person URL: before=%q after=%q", before, after)
	}
	if err := server.replaceProviderMediaPeople(mediaIDs[1], []MediaPerson{enriched}, "identity-test"); err != nil {
		t.Fatalf("insert matching provider credit: %v", err)
	}
	reconciled := personIDInMediaPeople(server.mediaPeopleFor(mediaIDs[1]), weak.Name)
	if reconciled != before {
		t.Fatalf("provider credit did not reconcile to durable identity: first=%q second=%q", before, reconciled)
	}
	multiProvider := weak
	multiProvider.ProviderIDs = map[string]string{"imdb": "name-123", "tmdb": "person-8675309"}
	if err := server.replaceProviderMediaPeople(mediaIDs[2], []MediaPerson{multiProvider}, "identity-test"); err != nil {
		t.Fatalf("insert multi-provider credit: %v", err)
	}
	if multi := personIDInMediaPeople(server.mediaPeopleFor(mediaIDs[2]), weak.Name); multi != before {
		t.Fatalf("multi-provider credit ignored its known provider identity: first=%q multi=%q", before, multi)
	}
}

func TestExactWeakNamesakesReceiveDistinctDurableIdentitiesAcrossReorder(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	var mediaID string
	if err := db.QueryRow(`SELECT id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 1`).Scan(&mediaID); err != nil {
		t.Fatalf("load media fixture: %v", err)
	}
	credits := []MediaPerson{
		{Name: "Exact Namesake", Role: "Actor"},
		{Name: "Exact Namesake", Role: "Actor"},
	}
	loadKeys := func() []string {
		t.Helper()
		rows, err := db.Query(`SELECT canonical_person_key FROM media_people WHERE media_id = ? AND source = 'namesake-test' ORDER BY canonical_person_key`, mediaID)
		if err != nil {
			t.Fatalf("load canonical keys: %v", err)
		}
		defer rows.Close()
		keys := []string{}
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				t.Fatalf("scan canonical key: %v", err)
			}
			keys = append(keys, key)
		}
		return keys
	}
	if err := server.replaceProviderMediaPeople(mediaID, credits, "namesake-test"); err != nil {
		t.Fatalf("seed exact namesakes: %v", err)
	}
	before := loadKeys()
	if len(before) != 2 || before[0] == before[1] {
		t.Fatalf("exact namesakes were conflated: %#v", before)
	}
	if err := server.replaceProviderMediaPeople(mediaID, []MediaPerson{credits[1], credits[0]}, "namesake-test"); err != nil {
		t.Fatalf("rescan exact namesakes: %v", err)
	}
	after := loadKeys()
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("exact namesake URLs changed across reorder: before=%#v after=%#v", before, after)
	}
}

func personIDInMediaPeople(people []MediaPerson, name string) string {
	for _, person := range people {
		if person.Name == name {
			return person.ID
		}
	}
	return ""
}

func TestPersonIdentityUsesProviderAndDurableFallbacks(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	rows, err := db.Query(`SELECT id FROM media_items WHERE type = 'movie' ORDER BY id LIMIT 2`)
	if err != nil {
		t.Fatalf("load namesake media: %v", err)
	}
	defer rows.Close()
	mediaIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		mediaIDs = append(mediaIDs, id)
	}
	if len(mediaIDs) != 2 {
		t.Fatalf("need two movie fixtures, got %v", mediaIDs)
	}
	evidenceCanonical := durablePersonCanonicalKey(mediaIDs[0], "Evidence Performer", "Actor", "https://images.example.test/evidence.jpg")
	portraitCanonicalA := durablePersonCanonicalKey(mediaIDs[0], "Portrait Namesake", "Actor", "https://images.example.test/namesake-a.jpg")
	portraitCanonicalB := durablePersonCanonicalKey(mediaIDs[1], "Portrait Namesake", "Actor", "https://images.example.test/namesake-b.jpg")
	credits := []struct {
		id, mediaID, name, providers, imageURL, character, canonicalKey string
	}{
		{"namesake-credit-a", mediaIDs[0], "Shared Namesake", `{}`, "", "", ""},
		{"namesake-credit-b", mediaIDs[1], "Shared Namesake", `{}`, "", "", ""},
		{"merged-credit-a", mediaIDs[0], "Merged Performer", `{"tmdb":"person-merged"}`, "", "", ""},
		{"merged-credit-b", mediaIDs[1], "Merged Performer Alias", `{"tmdb":"person-merged"}`, "", "", ""},
		{"fallback-merged-a", mediaIDs[0], "Evidence Performer", `{}`, "https://images.example.test/evidence.jpg", "", evidenceCanonical},
		{"fallback-merged-b", mediaIDs[1], "Evidence Performer", `{}`, "https://images.example.test/evidence.jpg", "", evidenceCanonical},
		{"fallback-namesake-a", mediaIDs[0], "Portrait Namesake", `{}`, "https://images.example.test/namesake-a.jpg", "", portraitCanonicalA},
		{"fallback-namesake-b", mediaIDs[1], "Portrait Namesake", `{}`, "https://images.example.test/namesake-b.jpg", "", portraitCanonicalB},
		{"weak-merged-a", mediaIDs[0], "Weak Performer", `{}`, "", "Captain North", ""},
		{"weak-merged-b", mediaIDs[1], "Weak Performer", `{}`, "", "Captain North", ""},
		{"weak-namesake-a", mediaIDs[0], "Weak Namesake", `{}`, "", "Detective One", ""},
		{"weak-namesake-b", mediaIDs[1], "Weak Namesake", `{}`, "", "Doctor Two", ""},
		{"canonical-merged-a", mediaIDs[0], "Durable Performer", `{}`, "", "", "manual:person-42"},
		{"canonical-merged-b", mediaIDs[1], "Durable Performer Alias", `{}`, "", "", "manual:person-42"},
	}
	for index, credit := range credits {
		if _, err := db.Exec(`
			INSERT INTO media_people (id, media_id, name, role, source, sort_order, provider_ids_json, image_url, character, canonical_person_key, created_at)
			VALUES (?, ?, ?, 'Actor', 'test', ?, ?, ?, ?, ?, '2026-01-01T00:00:00Z')`,
			credit.id, credit.mediaID, credit.name, index, credit.providers, credit.imageURL, credit.character, credit.canonicalKey); err != nil {
			t.Fatalf("insert person credit %s: %v", credit.id, err)
		}
	}

	firstCast := server.mediaPeopleFor(mediaIDs[0])
	secondCast := server.mediaPeopleFor(mediaIDs[1])
	castIDs := map[string]string{}
	for _, person := range append(firstCast, secondCast...) {
		castIDs[person.Name] = person.ID
	}
	personIDInCast := func(cast []MediaPerson, name string) string {
		t.Helper()
		for _, person := range cast {
			if person.Name == name {
				return person.ID
			}
		}
		return ""
	}
	firstNamesakeID := personIDInCast(firstCast, "Shared Namesake")
	secondNamesakeID := personIDInCast(secondCast, "Shared Namesake")
	if firstNamesakeID == "" || secondNamesakeID == "" || firstNamesakeID == secondNamesakeID {
		t.Fatalf("weak-metadata namesakes did not remain distinct: first=%q second=%q", firstNamesakeID, secondNamesakeID)
	}
	mergedID := castIDs["Merged Performer"]
	if mergedID == "" || castIDs["Merged Performer Alias"] != mergedID || strings.HasPrefix(mergedID, "person_") {
		t.Fatalf("provider-backed credits did not merge deterministically: cast=%#v merged=%q", castIDs, mergedID)
	}
	evidenceID := castIDs["Evidence Performer"]
	if evidenceID == "" || strings.HasPrefix(evidenceID, "person_") {
		t.Fatalf("evidence-backed providerless credits did not aggregate: cast=%#v evidence=%q", castIDs, evidenceID)
	}
	portraitA := encodePersonIdentityID(personIdentitySelector{Kind: "canonical", CanonicalKey: portraitCanonicalA})
	portraitB := encodePersonIdentityID(personIdentitySelector{Kind: "canonical", CanonicalKey: portraitCanonicalB})
	if portraitA == portraitB {
		t.Fatalf("evidence-distinct providerless namesakes collapsed to %q", portraitA)
	}
	weakA := personIDInMediaPeople(firstCast, "Weak Performer")
	weakB := personIDInMediaPeople(secondCast, "Weak Performer")
	if weakA == "" || weakB == "" || weakA == weakB {
		t.Fatalf("character-only weak credits were merged without durable evidence: first=%q second=%q", weakA, weakB)
	}
	weakNamesakeA := personIDInMediaPeople(firstCast, "Weak Namesake")
	weakNamesakeB := personIDInMediaPeople(secondCast, "Weak Namesake")
	if weakNamesakeA == weakNamesakeB {
		t.Fatalf("character-distinct weak namesakes collapsed to %q", weakNamesakeA)
	}
	durableID := castIDs["Durable Performer"]
	if durableID == "" || castIDs["Durable Performer Alias"] != durableID {
		t.Fatalf("durably reconciled weak credits did not merge: cast=%#v durable=%q", castIDs, durableID)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var namesakes SearchResponse
	namesakeRequest := SearchRequest{Query: "Shared Namesake", Group: "people", Limit: 1}
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", namesakeRequest, &namesakes)
	if status != http.StatusOK || len(namesakes.Groups) != 1 || len(namesakes.Groups[0].Items) != 1 || !namesakes.Groups[0].HasMore || namesakes.Groups[0].NextCursor == "" {
		t.Fatalf("namesake search status=%d body=%s response=%#v", status, body, namesakes)
	}
	firstResultID := namesakes.Groups[0].Items[0].ID
	if firstResultID != firstNamesakeID && firstResultID != secondNamesakeID {
		t.Fatalf("namesake search returned an unrelated identity %q", firstResultID)
	}
	var moreNamesakes SearchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{
		Query: "Shared Namesake", Group: "people", Limit: 1, Cursor: namesakes.Groups[0].NextCursor,
	}, &moreNamesakes)
	if status != http.StatusOK || len(moreNamesakes.Groups) != 1 || len(moreNamesakes.Groups[0].Items) != 1 || moreNamesakes.Groups[0].HasMore || moreNamesakes.Groups[0].Items[0].ID == firstResultID {
		t.Fatalf("namesake continuation status=%d body=%s response=%#v", status, body, moreNamesakes)
	}
	var namesakeDetail PersonDetailResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/people/"+firstResultID, nil, &namesakeDetail)
	if status != http.StatusOK || namesakeDetail.Person.ID != firstResultID || len(namesakeDetail.Credits) != 1 {
		t.Fatalf("namesake detail status=%d body=%s response=%#v", status, body, namesakeDetail)
	}
	var merged SearchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "Merged Performer", Group: "people", Limit: 10}, &merged)
	if status != http.StatusOK || len(merged.Groups) != 1 || len(merged.Groups[0].Items) != 1 || merged.Groups[0].Items[0].ID != mergedID {
		t.Fatalf("provider merge search status=%d body=%s response=%#v", status, body, merged)
	}
	var evidence SearchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "Evidence Performer", Group: "people", Limit: 10}, &evidence)
	if status != http.StatusOK || len(evidence.Groups) != 1 || len(evidence.Groups[0].Items) != 1 || evidence.Groups[0].Items[0].ID != evidenceID {
		t.Fatalf("fallback merge search status=%d body=%s response=%#v", status, body, evidence)
	}
	var evidenceDetail PersonDetailResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/people/"+evidenceID, nil, &evidenceDetail)
	if status != http.StatusOK || len(evidenceDetail.Credits) != 2 {
		t.Fatalf("fallback merge detail status=%d body=%s response=%#v", status, body, evidenceDetail)
	}
	var portraitNamesakes SearchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "Portrait Namesake", Group: "people", Limit: 10}, &portraitNamesakes)
	if status != http.StatusOK || len(portraitNamesakes.Groups) != 1 || len(portraitNamesakes.Groups[0].Items) != 2 {
		t.Fatalf("fallback namesake search status=%d body=%s response=%#v", status, body, portraitNamesakes)
	}
	var weak SearchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "Weak Performer", Group: "people", Limit: 10}, &weak)
	if status != http.StatusOK || len(weak.Groups) != 1 || len(weak.Groups[0].Items) != 2 {
		t.Fatalf("weak fallback merge search status=%d body=%s response=%#v", status, body, weak)
	}
}

func TestDiscoveryReleaseEndpointsWorkTogether(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var search SearchResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/search", SearchRequest{Query: "Meridian", Sort: "relevance", Direction: "desc", RecordHistory: true}, &search)
	if status != http.StatusOK || search.Query != "Meridian" {
		t.Fatalf("search status=%d body=%s response=%#v", status, body, search)
	}
	var history SearchHistoryResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/search/history", nil, &history)
	if status != http.StatusOK || len(history.Items) != 1 || history.Items[0].Query != "Meridian" {
		t.Fatalf("history status=%d body=%s response=%#v", status, body, history)
	}

	if _, err := db.Exec(`UPDATE media_items SET summary = 'A safe paginated synopsis.' WHERE parent_id = 'show_northbridge'`); err != nil {
		t.Fatalf("seed child summary: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, art_seed, genres_json, added_at, index_number)
		SELECT 'release-extra', library_id, id, 'extra', 'Release Trailer', 'Release Trailer', 'trailer', '[]', '2026-01-01T00:00:00Z', -1
		FROM media_items WHERE id = 'show_northbridge'`); err != nil {
		t.Fatalf("seed hierarchy extra: %v", err)
	}
	var children MediaCardPageResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/show_northbridge/children?limit=1", nil, &children)
	if status != http.StatusOK || len(children.Items) != 1 || children.Items[0].ID == "release-extra" || children.Items[0].Summary != "A safe paginated synopsis." || children.PageInfo.Total != nil {
		t.Fatalf("children status=%d body=%s response=%#v", status, body, children)
	}
	var showDetail MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/show_northbridge", nil, &showDetail)
	if status != http.StatusOK || len(showDetail.Extras) != 1 || len(showDetail.Extras[0].Items) != 1 || showDetail.Extras[0].Items[0].ID != "release-extra" {
		t.Fatalf("show extras status=%d body=%s response=%#v", status, body, showDetail)
	}
	var hierarchyContainsExtra func([]MediaItem) bool
	hierarchyContainsExtra = func(items []MediaItem) bool {
		for _, item := range items {
			if item.ID == "release-extra" || hierarchyContainsExtra(item.Children) {
				return true
			}
		}
		return false
	}
	if hierarchyContainsExtra(showDetail.Children) {
		t.Fatalf("extra leaked into the show hierarchy: %#v", showDetail.Children)
	}

	if _, err := db.Exec(`
		INSERT INTO media_people (id, media_id, name, role, source, sort_order, provider_ids_json, created_at)
		VALUES ('release-person-credit', 'movie_meridian', 'Ari Vega', 'Actor', 'test', 0, '{"tmdb":"person-42"}', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed person fixture: %v", err)
	}
	personID, err := server.publicPersonIDForIdentity(context.Background(), personIdentitySelector{Kind: "provider", Provider: "tmdb", ExternalID: "person-42"})
	if err != nil {
		t.Fatalf("allocate public person identity: %v", err)
	}
	var person PersonDetailResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/people/"+personID, nil, &person)
	if status != http.StatusOK || person.Person.ID != personID || len(person.Person.ID) != 40 || len(person.Credits) == 0 {
		t.Fatalf("person status=%d body=%s response=%#v", status, body, person)
	}

}

func TestConsumerMediaDetailProjectionRemovesPrivateEvidenceAndAdminActions(t *testing.T) {
	item := MediaItem{
		ID: "movie_private", Type: "movie", SourceURL: "file:///private/movie.mkv",
		ProviderIDs:      []MediaProviderID{{Provider: "tmdb", ExternalID: "1"}},
		MatchCandidates:  []MatchCandidate{{Provider: "tmdb"}},
		IdentityEvidence: []IdentityEvidence{{Path: "/private/evidence.nfo"}},
		LockedFields:     []string{"title"}, Actions: []string{mediaActionPlay, mediaActionMetadataEdit},
		Streams: []Stream{{ID: "stream", SourceURL: "file:///private/movie.mkv"}},
		MediaFiles: []MediaFileVersion{{
			ID: "file", Path: "/private/movie.mkv", OriginalFilename: "private-original.mkv", SourceType: "scanner-type",
			Analysis: "private-analysis", Source: "scanner", ReleaseGroup: "private-group",
		}},
		OptimizedVersions: []OptimizedVersion{{
			ID: "optimized", Path: "/private/optimized.mp4", StreamURL: "/api/media/movie_private/optimized/1080p/stream",
			DownloadURL: "/api/media/movie_private/optimized/1080p/download",
		}},
		MediaImages:        []MediaImage{{ID: "image", Source: "private-image-source", Provider: "private-image-provider", Path: "/private/poster.jpg", RemoteURL: "https://provider.invalid/poster.jpg"}},
		Lyrics:             []MediaLyric{{ID: "lyric", Source: "private-lyric-source", Provider: "private-lyric-provider", Path: "/private/movie.lrc", Text: "safe playback text"}},
		Segments:           []MediaSegment{{ID: "segment", Type: "intro", Source: "private-segment-source", Provider: "private-segment-provider"}},
		AudioNormalization: &AudioNormalization{IntegratedLUFS: -16, Source: "private-normalization-source"},
		People:             []MediaPerson{{ID: stablePersonID("Person"), Name: "Person", Source: "tmdb", ProviderIDs: map[string]string{"tmdb": "1"}}},
	}
	projected := consumerMediaDetailProjection(item, User{})
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"file:///private", "/private/", "provider.invalid", "private-group", "private-original", "scanner-type", "private-analysis",
		"private-image-source", "private-image-provider", "private-lyric-source", "private-lyric-provider",
		"private-segment-source", "private-segment-provider", "private-normalization-source",
		"metadata.edit", "providerIds", "identityEvidence", "matchCandidates",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("consumer projection leaked %q: %s", forbidden, encoded)
		}
	}
	if len(projected.Actions) != 1 || projected.Actions[0] != mediaActionPlay || projected.Lyrics[0].Text == "" ||
		projected.OptimizedVersions[0].StreamURL == "" || projected.OptimizedVersions[0].DownloadURL == "" {
		t.Fatalf("consumer projection removed safe behavior: %#v", projected)
	}
	webEditor := consumerMediaDetailProjection(item, User{Permissions: map[string]bool{"editMetadata": true}})
	if !containsString(webEditor.Actions, mediaActionMetadataEdit) || !containsString(webEditor.Actions, mediaActionPlay) {
		t.Fatalf("authorized web editor did not receive projected administration actions: %#v", webEditor.Actions)
	}
	webEditorJSON, err := json.Marshal(webEditor)
	if err != nil {
		t.Fatal(err)
	}
	serverAdmin := consumerMediaDetailProjection(item, User{Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()})
	if serverAdmin.MediaFiles[0].Path != "/private/movie.mkv" || serverAdmin.MediaFiles[0].OriginalFilename != "private-original.mkv" {
		t.Fatalf("server administrator did not receive source file details: %#v", serverAdmin.MediaFiles[0])
	}
	for _, forbidden := range []string{"file:///private", "/private/", "provider.invalid", "private-original"} {
		if strings.Contains(string(webEditorJSON), forbidden) {
			t.Errorf("authorized ordinary Web projection leaked %q: %s", forbidden, webEditorJSON)
		}
	}
	apiCapableEditor := consumerMediaDetailProjection(item, User{
		AuthProvider: "api_key", APIKeyID: "api_detail_probe",
		Permissions: map[string]bool{"editMetadata": true, "manageLibraries": true},
	})
	apiCapableJSON, err := json.Marshal(apiCapableEditor)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"file:///private", "/private/", "provider.invalid", "private-original", "providerIds"} {
		if strings.Contains(string(apiCapableJSON), forbidden) {
			t.Errorf("API-capable viewer projection leaked %q: %s", forbidden, apiCapableJSON)
		}
	}
}

func TestViewerMediaRouteIgnoresCraftedManagementSurface(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	const mediaID = "movie_meridian"
	if _, err := db.Exec(`UPDATE media_items SET source_url = ? WHERE id = ?`, "file:///private/crafted-surface.mkv", mediaID); err != nil {
		t.Fatalf("seed private viewer evidence: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var item MediaItem
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/media/"+mediaID+"?surface=web-admin", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("crafted viewer detail status=%d body=%s", status, body)
	}
	if strings.Contains(body, "crafted-surface") || strings.Contains(body, "file:///private") || item.SourceURL != "" {
		t.Fatalf("client-selected management surface exposed viewer technical evidence: %s", body)
	}
}

func TestPersonIdentityIsLimitedToProfileVisibleCredits(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	account, err := server.createUser(UserRequest{
		Username: "person-visibility", Email: "person-visibility@example.test", DisplayName: "Person Visibility",
		Password: "Person-visibility-password", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create restricted person viewer: %v", err)
	}
	var mediaID, libraryID string
	if err := db.QueryRow(`SELECT id, library_id FROM media_items WHERE library_id <> '' ORDER BY id LIMIT 1`).Scan(&mediaID, &libraryID); err != nil {
		t.Fatalf("load person visibility fixture media: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_people (id, media_id, name, role, source, sort_order, image_url, provider_ids_json, created_at)
		VALUES ('person-visibility-credit', ?, 'Private Person', 'Director', 'private-provider', 0, 'https://private.invalid/person.jpg', '{}', '2026-01-01T00:00:00Z')`, mediaID); err != nil {
		t.Fatalf("insert private person credit: %v", err)
	}
	_, name, roles, imageURL, found, err := server.personIdentityContext(context.Background(), viewerProfileID(account), personIdentitySelector{Kind: "name", Name: "Private Person"})
	if err != nil {
		t.Fatalf("query restricted person identity: %v", err)
	}
	if found || len(roles) != 0 || imageURL != "" {
		t.Fatalf("restricted profile received identity from hidden credit: name=%q roles=%#v image=%q found=%v", name, roles, imageURL, found)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, ?, ?)`, account.ID, libraryID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("grant person credit library: %v", err)
	}
	_, name, roles, imageURL, found, err = server.personIdentityContext(context.Background(), viewerProfileID(account), personIdentitySelector{Kind: "name", Name: "Private Person"})
	if err != nil || !found || name != "Private Person" || len(roles) != 1 || roles[0] != "Director" || imageURL == "" {
		t.Fatalf("visible person identity was not returned: name=%q roles=%#v image=%q found=%v err=%v", name, roles, imageURL, found, err)
	}
}

func TestChildrenAndPersonWindowsApplyAccessBeforeLimit(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	account, err := server.createUser(UserRequest{
		Username: "window-access", Email: "window-access@example.test", DisplayName: "Window Access",
		Password: "Window-access-password", Role: "user", Permissions: map[string]bool{"playMedia": true},
	})
	if err != nil {
		t.Fatalf("create window viewer: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, libraryID := range []string{"lib_window_visible", "lib_window_hidden"} {
		if _, err := db.Exec(`
			INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
			VALUES (?, ?, 'show', 996, ?, '{}', ?)`, libraryID, libraryID, "/tmp/"+libraryID, now); err != nil {
			t.Fatalf("insert %s: %v", libraryID, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, 'lib_window_visible', ?)`, account.ID, now); err != nil {
		t.Fatalf("grant visible window library: %v", err)
	}
	insertMedia := func(id, libraryID, parentID, mediaType, title string, year, index int) {
		t.Helper()
		var parent any
		if parentID != "" {
			parent = parentID
		}
		if _, err := db.Exec(`
			INSERT INTO media_items (
				id, library_id, parent_id, type, title, sort_title, year, duration_seconds,
				genres_json, tags_json, labels_json, typed_metadata_json, added_at, index_number, random_key
			) VALUES (?, ?, ?, ?, ?, ?, ?, 3600, '[]', '[]', '[]', '{}', ?, ?, ?)`,
			id, libraryID, parent, mediaType, title, title, year, now, index, mediaRandomKey(id)); err != nil {
			t.Fatalf("insert media %s: %v", id, err)
		}
	}
	insertMedia("show_window_parent", "lib_window_visible", "", "show", "Window Parent", 2026, 0)
	insertMedia("episode_window_hidden_0", "lib_window_hidden", "show_window_parent", "episode", "Hidden Episode 0", 2026, 0)
	insertMedia("episode_window_hidden_1", "lib_window_hidden", "show_window_parent", "episode", "Hidden Episode 1", 2026, 1)
	insertMedia("episode_window_visible_2", "lib_window_visible", "show_window_parent", "episode", "Visible Episode 2", 2026, 2)
	insertMedia("episode_window_visible_3", "lib_window_visible", "show_window_parent", "episode", "Visible Episode 3", 2026, 3)
	children, err := server.queryMediaListItemsContext(context.Background(), viewerProfileID(account), `
		WHERE m.parent_id = ? ORDER BY m.index_number ASC, m.id ASC LIMIT ?`, []any{"show_window_parent", 2})
	if err != nil || fmt.Sprint(mediaItemIDs(children)) != fmt.Sprint([]string{"episode_window_visible_2", "episode_window_visible_3"}) {
		t.Fatalf("authorized children window=%v err=%v", mediaItemIDs(children), err)
	}

	insertMedia("movie_window_hidden", "lib_window_hidden", "", "movie", "Hidden Credit", 2030, 0)
	insertMedia("movie_window_visible", "lib_window_visible", "", "movie", "Visible Credit", 2020, 0)
	for _, credit := range []struct{ id, mediaID string }{{"person-window-hidden", "movie_window_hidden"}, {"person-window-visible", "movie_window_visible"}} {
		if _, err := db.Exec(`
			INSERT INTO media_people (id, media_id, name, role, source, sort_order, provider_ids_json, created_at)
			VALUES (?, ?, 'Window Performer', 'Actor', 'test', 0, '{"tmdb":"window-performer"}', ?)`, credit.id, credit.mediaID, now); err != nil {
			t.Fatalf("insert %s: %v", credit.id, err)
		}
	}
	personID, err := server.publicPersonIDForIdentity(context.Background(), personIdentitySelector{Kind: "provider", Provider: "tmdb", ExternalID: "window-performer"})
	if err != nil {
		t.Fatalf("allocate public person identity: %v", err)
	}
	credits, err := server.personMediaIdentityContext(context.Background(), viewerProfileID(account), personID, 1)
	if err != nil || fmt.Sprint(mediaItemIDs(credits)) != fmt.Sprint([]string{"movie_window_visible"}) {
		t.Fatalf("authorized person window=%v err=%v", mediaItemIDs(credits), err)
	}
}

func TestHomeRowPolicyKeepsEssentialRowsVisible(t *testing.T) {
	critical := homeRowDescriptor("continue", "Continue Watching", "continue-watching", "continue", "", 10, true)
	if !critical.Required || critical.Hideable || !critical.Reorderable || !reflect.DeepEqual(critical.Controls, []string{"reorder"}) {
		t.Fatalf("critical Home policy=%#v", critical)
	}
	optional := homeRowDescriptor("favorites", "Favorites", "favorites", "poster", "", 80, false)
	if optional.Required || !optional.Hideable || !optional.Reorderable || !reflect.DeepEqual(optional.Controls, []string{"hide", "reorder"}) {
		t.Fatalf("optional Home policy=%#v", optional)
	}
}

func TestBrowseSeekAndExpandedFieldsCompileCanonically(t *testing.T) {
	request := BrowseLibraryRequest{Sort: []BrowseSort{{Field: "title", Direction: "asc"}}, Seek: &BrowseSeek{Prefix: "M"}}
	where, args, issues := applyBrowseSeek("WHERE 1 = 1", nil, request, []browseSortSpec{{Field: "title", Direction: "ASC", SQL: "m.sort_title"}})
	if len(issues) > 0 || !strings.Contains(where, "m.sort_title COLLATE NOCASE >= ?") || !reflect.DeepEqual(args, []any{"M"}) {
		t.Fatalf("seek where=%q args=%#v issues=%#v", where, args, issues)
	}
	request.Cursor = "opaque"
	continuedWhere, continuedArgs, issues := applyBrowseSeek("WHERE 1 = 1", nil, request, []browseSortSpec{{Field: "title", Direction: "ASC", SQL: "m.sort_title"}})
	if len(issues) > 0 || !strings.Contains(continuedWhere, "m.sort_title COLLATE NOCASE >= ?") || !reflect.DeepEqual(continuedArgs, []any{"M"}) {
		t.Fatalf("continued seek where=%q args=%#v issues=%#v", continuedWhere, continuedArgs, issues)
	}
	for _, field := range []string{"releaseDate", "personalRating", "label", "studio", "actor", "resolution", "dynamicRange", "mediaVersion"} {
		capability, ok := browseFieldCapability(field)
		if !ok || capability.ControlHint == "" || capability.Complexity == "" || capability.Cost == "" {
			t.Errorf("expanded browse capability %q=%#v ok=%v", field, capability, ok)
		}
	}
}
