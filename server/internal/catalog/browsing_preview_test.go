package catalog

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"portico.local/server/internal/catalogtest"
)

// Check the published JSON, not just an intermediate projection: each card
// must be able to paint its preview without making an item-detail request.
func checkBrowsingPreviewJSON(t *testing.T, document any, expected map[string]string) int {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	checked := 0
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case []any:
			for _, child := range value {
				visit(child)
			}
		case map[string]any:
			id, _ := value["id"].(string)
			if want, ok := expected[id]; ok && value["kind"] != nil && value["title"] != nil {
				if got, _ := value["overview"].(string); got != want || !utf8.ValidString(got) {
					t.Errorf("published preview %s has synopsis %q, want %q", id, got, want)
				}
				checked++
			}
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(value)
	return checked
}

func TestHomeAndDiscoverPublishReadyBoundedPreviewDescriptions(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	expected := map[string]string{}
	long := strings.Repeat("界🙂é", 600)
	for name, item := range homeNames(t, db) {
		text := "The description of " + name
		if name == "m1" {
			text = long
		}
		c.Fields(item.ID, map[string]any{"overview": text})
		runes := []rune(text)
		if len(runes) > 1000 {
			runes = runes[:1000]
		}
		expected[item.Public] = string(runes)
	}
	c.Drain()
	request := homeRequestFixture()
	document, err := s.HomeRows(request)
	if err != nil {
		t.Fatal(err)
	}
	if checked := checkBrowsingPreviewJSON(t, document, expected); checked < 4 {
		t.Fatalf("Home fixture only exercised %d cards", checked)
	}
	for _, row := range document.Rows {
		single, err := s.HomeSingleRow(request, row.ID, HomeRowPage{Limit: 2})
		if err != nil {
			t.Fatal(row.ID, err)
		}
		checkBrowsingPreviewJSON(t, single, expected)
		if single.NextCursor != "" {
			next, err := s.HomeSingleRow(request, row.ID, HomeRowPage{Limit: 2, Cursor: single.NextCursor})
			if err != nil {
				t.Fatal(row.ID, err)
			}
			checkBrowsingPreviewJSON(t, next, expected)
		}
	}
	for _, library := range []string{"movies", "tv", "music", "books"} {
		t.Run(library, func(t *testing.T) {
			page, err := s.Content(ContentRequest{Viewer: request.Viewer, Library: library, ServerID: request.ServerID, View: "discover", Limit: 12, Now: request.Now})
			if err != nil {
				t.Fatal(err)
			}
			if checkBrowsingPreviewJSON(t, page, expected) == 0 {
				t.Fatal("Discover exercised no preview cards")
			}
		})
	}
	// A description edit is visible on the next publication, including a card
	// already seen in an earlier Home document. Detail responses remain whole.
	movie := homeItem(t, db, "m1")
	item, err := s.Get("p", movie.Public)
	if err != nil || item.Overview != long {
		t.Fatalf("preview truncation changed detail text: %v", err)
	}
	c.Fields(movie.ID, map[string]any{"overview": "Updated description"})
	c.Drain()
	expected[movie.Public] = "Updated description"
	document, err = s.HomeRows(request)
	if err != nil {
		t.Fatal(err)
	}
	checkBrowsingPreviewJSON(t, document, expected)
	// A full grid has no focused hero and retains its lean projection.
	grid, err := s.Content(ContentRequest{Viewer: request.Viewer, Library: "movies", View: "browse", Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range grid.Sections {
		for _, entry := range section.Entries {
			if entry.Overview != "" {
				t.Fatal("full grid unexpectedly received preview descriptions")
			}
		}
	}
}

func TestHomeWorkCardsRetainTheirResolvedBackdrops(t *testing.T) {
	db := homeFixture(t)
	c := catalogtest.New(t, db)
	digest := strings.Repeat("a", 64)
	c.Exec(`INSERT INTO artwork_objects VALUES(?, 'image/png',1920,1080,100,'now','ready')`, digest)
	ids := []string{}
	for _, name := range []string{"show", "s1", "album", "book"} {
		item := homeItem(t, db, name)
		kind := name
		if name == "s1" {
			kind = "season"
		}
		candidate := "preview-" + name
		c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) VALUES(?,?,?,'backdrop','','fixture',?,'local','fixture','fence','now')`, candidate, kind, item.ID, name)
		c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES(?,?,'backdrop','',?,?,?,0,1,'fixture','now')`, kind, item.ID, candidate, digest, digest)
		ids = append(ids, item.Public)
	}
	entries, err := New(db).homeEngineEntries("p", ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(ids) {
		t.Fatalf("lost work cards: %+v", entries)
	}
	for _, entry := range entries {
		if entry.BackdropURL == "" {
			t.Fatal("focused work preview lost its resolved backdrop", entry.Kind, entry.ID)
		}
	}
}

func TestPreviewDescriptionsRespectRestrictedMembership(t *testing.T) {
	db, service, names := tl10RestrictionFixture(t)
	c := catalogtest.New(t, db)
	c.Fields(names["kids"].ID, map[string]any{"overview": "An allowed synopsis"})
	c.Fields(names["adult"].ID, map[string]any{"overview": "FORBIDDEN_PREVIEW_TEXT"})
	c.Fields(names["show"].ID, map[string]any{"overview": "FORBIDDEN_SERIES_TEXT"})
	c.Drain()
	viewer := Viewer{Profile: "p", Fence: "restricted", Libraries: []string{"a", "tv"}, Restrictions: restrictionOf(ceiling(8), true)}
	home, err := service.HomeRows(HomeRequest{Viewer: viewer, ServerID: "server"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.Content(ContentRequest{Viewer: viewer, Library: "a", View: "discover", Limit: 12})
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range []any{home, page} {
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "FORBIDDEN_PREVIEW_TEXT") || strings.Contains(string(raw), "FORBIDDEN_SERIES_TEXT") || strings.Contains(string(raw), names["adult"].Public) || strings.Contains(string(raw), names["show"].Public) {
			t.Fatal("restricted title or its description leaked into browsing previews")
		}
		if checkBrowsingPreviewJSON(t, document, map[string]string{names["kids"].Public: "An allowed synopsis"}) == 0 {
			t.Fatal("allowed title lost its preview")
		}
	}
}

func TestPreviewArtistBiographyKeepsAttributionAndOwnerPrecedence(t *testing.T) {
	db := homeFixture(t)
	c := catalogtest.New(t, db)
	artist := homeItem(t, db, "artist")
	c.Exec(`INSERT INTO music_artist_evidence(artist_id,mbid,observed_at,bio,bio_source,bio_source_url) VALUES(?,?,'2026-10-18',?,'Wikipedia','https://en.wikipedia.org/wiki/Artist')`, artist.ID, "55555555-5555-5555-5555-555555555555", strings.Repeat("音", 1200))
	s := New(db)
	entries := []ContentEntry{{ID: artist.Public, Kind: "artist", Title: "Artist"}}
	if err := s.homeEnrich("p", entries); err != nil {
		t.Fatal(err)
	}
	if entries[0].Overview != strings.Repeat("音", 1000) || entries[0].OverviewSource != "Wikipedia" || entries[0].OverviewSourceURL != "https://en.wikipedia.org/wiki/Artist" {
		t.Fatal("provider biography lost its bound or attribution", entries[0])
	}
	c.Fields(artist.ID, map[string]any{"overview": "The owner's biography"})
	c.Drain()
	if err := s.homeEnrich("p", entries); err != nil {
		t.Fatal(err)
	}
	if entries[0].Overview != "The owner's biography" || entries[0].OverviewSource != "" || entries[0].OverviewSourceURL != "" {
		t.Fatal("owner biography inherited provider attribution", entries[0])
	}
}
