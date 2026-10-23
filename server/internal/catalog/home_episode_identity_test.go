package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

// System surfaces consume the same genuine episode facts as Home. In
// particular, a date episode's stored day-of-year is not a display episode
// number, and an absolute episode has no inferred season.
func TestHomeEpisodeIdentityAcrossContinueAndEngineHydration(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	show := homeItem(t, db, "show")
	c.Fields(show.ID, map[string]any{"title": "Harbor · S9 E9"})
	absolute := homeAddAbsoluteEpisode(t, db, "show", "absolute37", "The crossing", "/tv/absolute37.mkv", 37)
	special := c.Episode(show, c.Season(show, 0), 2, "/tv/special2.mkv")
	daily := c.Episode(show, c.Season(show, 2024), 60, "/tv/daily.mkv")
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetFactsTx(ctx, tx, daily.ID, map[string]any{"numbering": "date", "ordering_basis": "date", "air_date": time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC).Unix() / 86400})
	})
	seasonal := homeItem(t, db, "e101")
	for _, item := range []catalogtest.Item{seasonal, absolute, daily, special} {
		c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES('p',?,120000,0,?)`, item.ID, item.Public)
		c.Exec(`INSERT INTO progress_activity VALUES('p','tv',?,'2026-09-02T00:00:00.000Z','paused')`, item.ID)
	}
	c.Drain()
	ids := []string{seasonal.Public, absolute.Public, daily.Public, special.Public}
	want := map[string]struct {
		season, episode, absolute *int
		date, special             bool
		airDate                   string
	}{
		seasonal.Public: {season: identityNumber(1), episode: identityNumber(1)},
		absolute.Public: {episode: identityNumber(37), absolute: identityNumber(37)},
		daily.Public:    {season: identityNumber(2024), episode: identityNumber(60), date: true, airDate: "2024-02-29"},
		special.Public:  {season: identityNumber(0), episode: identityNumber(2), special: true},
	}
	check := func(entries []ContentEntry) {
		t.Helper()
		if len(entries) != len(want) {
			t.Fatalf("episode hydration count: got %d want %d", len(entries), len(want))
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			facts, ok := want[entry.ID]
			if !ok || seen[entry.ID] {
				t.Fatalf("unexpected/duplicate episode: %+v", entry)
			}
			seen[entry.ID] = true
			if entry.Kind != "episode" || entry.Show == nil || entry.Show.ID != show.Public || entry.Show.Name != "Harbor · S9 E9" || entry.AirDate != facts.airDate || entry.DateBased != facts.date || entry.Special != facts.special || !sameIdentityNumber(entry.SeasonNumber, facts.season) || !sameIdentityNumber(entry.EpisodeNumber, facts.episode) || !sameIdentityNumber(entry.AbsoluteNumber, facts.absolute) {
				t.Fatalf("genuine identity lost or fabricated for %s: %+v show=%+v", entry.ID, entry, entry.Show)
			}
		}
	}
	// Exercise the real Continue hydration with an explicit selected page. Row
	// selection intentionally chooses only one continuation per show.
	continued, err := s.homeContinueEntries(homeRequestFixture("tv").Viewer, ids)
	if err != nil {
		t.Fatal(err)
	}
	check(continued)
	// Recommendation, saved-view and recent rows all use this hydration path
	// when the actual selected candidate is an episode rather than a show.
	engine, err := s.homeEngineEntries("p", ids)
	if err != nil {
		t.Fatal(err)
	}
	check(engine)
	for _, spec := range []homeRowSpec{{ID: "recommended"}, {ID: "new_on_server"}} {
		entries, err := s.homeRowEntries("p", spec, ids)
		if err != nil {
			t.Fatal(err)
		}
		check(entries)
	}
}

func TestHomeRemovedEpisodeDoesNotInventParentOrNumbering(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	c := catalogtest.New(t, db)
	episode := homeItem(t, db, "e101")
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES('p',?,120000,0,'removed-identity')`, episode.ID)
	c.Exec(`INSERT INTO progress_activity VALUES('p','tv',?,'2026-09-02T00:00:00.000Z','paused')`, episode.ID)
	c.Drain()
	c.Delete(episode.ID)
	entries, err := s.homeContinueEntries(homeRequestFixture("tv").Viewer, []string{episode.Public})
	if err != nil || len(entries) != 1 {
		t.Fatalf("removed episode missing: %+v %v", entries, err)
	}
	entry := entries[0]
	if !entry.Removed || entry.Kind != "episode" || entry.Title != "Episode e101" || entry.Playback != nil {
		t.Fatalf("removed title identity changed: %+v", entry)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"show", "airDate", "dateBased", "special", "seasonNumber", "episodeNumber", "absoluteNumber"} {
		if _, exists := wire[key]; exists {
			t.Fatalf("removed episode fabricated %s: %s", key, raw)
		}
	}
}

func identityNumber(value int) *int { return &value }

func sameIdentityNumber(a, b *int) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
