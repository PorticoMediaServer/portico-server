package metadataprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTMDBAbsoluteCoordinatesAndSeriesAggregateCredits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tv/5":
			fmt.Fprint(w, `{"id":5,"name":"Anime","seasons":[{"season_number":0,"episode_count":3},{"season_number":1,"episode_count":12},{"season_number":2,"episode_count":10}],"aggregate_credits":{"cast":[{"id":9,"name":"Earlier cast member","roles":[{"character":"Lead"}]}]}}`)
		case "/tv/5/season/2/episode/2":
			fmt.Fprint(w, `{"id":14,"name":"Episode Fourteen","season_number":2,"episode_number":2,"still_path":"/still.jpg"}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p, e := NewTMDBAt("fixture", server.URL)
	if e != nil {
		t.Fatal(e)
	}
	p.http.interval = 0
	show, e := p.ScreenDetails(context.Background(), "show", "5", "en", "")
	if e != nil || len(show.Credits) != 1 || show.Credits[0].Name != "Earlier cast member" {
		t.Fatal(show, e)
	}
	episode, e := p.ScreenAbsoluteEpisode(context.Background(), "5", 14, "official", "en")
	if e != nil || episode.Coordinates == nil || episode.Coordinates.Season != nil || *episode.Coordinates.Absolute != 14 || *episode.Coordinates.OriginalSeason != 2 || *episode.Coordinates.OriginalEpisode != 2 || episode.Title != "Episode Fourteen" {
		t.Fatal(episode, e)
	}
}
func TestOptionalProviderListsAreTrimmedPerEntry(t *testing.T) {
	r := ScreenRecord{Identity: ScreenID{"tmdb", "movie", "1"}, Title: "Film"}
	for i := 0; i < 300; i++ {
		r.Aliases = append(r.Aliases, fmt.Sprintf("Title %d", i))
	}
	r.Genres = []ScreenName{{"1", "Drama"}, {"2", string([]byte{0})}}
	r = BoundScreenRecord(r)
	if len(r.Aliases) != 256 || len(r.Genres) != 1 || ValidateScreenRecord(r) != nil {
		t.Fatal(r)
	}
}
func TestProviderConcurrentBodiesRemainBounded(t *testing.T) {
	var running, peak atomic.Int32
	tr := fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		n := running.Add(1)
		defer running.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			var out any
			if e := tr.request(context.Background(), "GET", "/body", "", nil, &out); e != nil {
				t.Error(e)
			}
		}()
	}
	group.Wait()
	if peak.Load() < 2 || peak.Load() > 4 {
		t.Fatal("concurrency was not bounded", peak.Load())
	}
}
