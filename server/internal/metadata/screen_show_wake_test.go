package metadata

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"portico.local/server/internal/metadataprovider"
)

func showWakeState(t *testing.T, db *sql.DB) (publications, generation int64, status, reason string) {
	t.Helper()
	if err := db.QueryRow(`SELECT count(*) FROM screen_metadata_publications WHERE target_kind='show'`).Scan(&publications); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT generation,status,error FROM screen_metadata_work WHERE target_kind='show'`).Scan(&generation, &status, &reason); err != nil {
		t.Fatal(err)
	}
	return
}

func matchedEpisodes(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM screen_metadata_work w JOIN catalog_episodes e ON e.entity_id=w.target_id WHERE w.target_kind='item' AND w.status='matched'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A scan that adds a matched show's episodes page by page, with the matcher
// working between and during the pages, matches the show once in all and
// fetches each new episode once. Every new episode used to send the show back
// to pending: the show was matched again after every page.
func TestNewEpisodesDoNotRematchAMatchedShow(t *testing.T) {
	s, db, p, now, add, settle := episodeFreshnessFixture(t)
	settle()
	publications, generation, status, _ := showWakeState(t, db)
	if publications != 1 || status != "matched" {
		t.Fatalf("fixture show: %d publications, status %q", publications, status)
	}
	showCalls := p.showCalls
	// Past every document's window, so matching the show again would have to
	// ask the provider.
	*now = now.Add(90 * 24 * time.Hour)
	number := 4
	for page := 0; page < 3; page++ {
		for i := 0; i < 3; i++ {
			number++
			add(number)
			// The matcher keeps working while the scan writes its page.
			if err := s.ScreenStep(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		settle()
	}
	after, afterGeneration, afterStatus, _ := showWakeState(t, db)
	if after != 1 || afterGeneration != generation || afterStatus != "matched" || p.showCalls != showCalls {
		t.Fatalf("new episodes re-matched the show: publications %d -> %d, generation %d -> %d, status %q, show requests %d -> %d", publications, after, generation, afterGeneration, afterStatus, showCalls, p.showCalls)
	}
	for n := 5; n <= number; n++ {
		if got := p.episodeCalls["500/1/"+strconv.Itoa(n)+"/official/en-US"]; got != 1 {
			t.Fatalf("episode %d was fetched %d times: %v", n, got, p.episodeCalls)
		}
	}
	if p.total() != number {
		t.Fatalf("%d episode requests for %d episodes: %v", p.total(), number, p.episodeCalls)
	}
	if got := matchedEpisodes(t, db); got != number {
		t.Fatalf("%d of %d episodes matched", got, number)
	}
}

// blockingShowProvider holds the first show search until the test lets it go.
type blockingShowProvider struct {
	*episodeFixtureProvider
	asked, release chan struct{}
	held           bool
}

func (p *blockingShowProvider) SearchScreen(ctx context.Context, kind, query string, year int, language, region string) ([]metadataprovider.ScreenRecord, error) {
	if !p.held {
		p.held = true
		close(p.asked)
		<-p.release
	}
	return p.episodeFixtureProvider.SearchScreen(ctx, kind, query, year, language, region)
}

// A show whose match is in flight keeps it when the scan adds episodes: the
// result is published, not discarded, and the new episodes are matched when
// the show's dispatch completes, without matching the show again.
func TestInFlightShowMatchSurvivesNewEpisodes(t *testing.T) {
	s, db, p, _, add, settle := episodeFreshnessFixture(t)
	blocking := &blockingShowProvider{episodeFixtureProvider: p, asked: make(chan struct{}), release: make(chan struct{})}
	s.screenProviders = map[string]ScreenProvider{"tmdb": blocking}
	_, generation, _, _ := showWakeState(t, db)
	done := make(chan error, 1)
	go func() { done <- s.ScreenStep(context.Background()) }()
	select {
	case <-blocking.asked:
	case err := <-done:
		t.Fatalf("the first step did not search for the show: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("the show was never searched for")
	}
	add(5)
	add(6)
	close(blocking.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	publications, afterGeneration, status, reason := showWakeState(t, db)
	if publications != 1 || afterGeneration != generation || status != "pending_children" || reason == "stale_result_discarded" {
		t.Fatalf("the in-flight match was discarded: publications %d, generation %d -> %d, status %q, error %q", publications, generation, afterGeneration, status, reason)
	}
	showCalls := p.showCalls
	settle()
	publications, _, status, _ = showWakeState(t, db)
	if publications != 1 || status != "matched" || p.showCalls != showCalls {
		t.Fatalf("the show was matched again: publications %d, status %q, show requests %d -> %d", publications, status, showCalls, p.showCalls)
	}
	if got := matchedEpisodes(t, db); got != 6 {
		t.Fatalf("%d of 6 episodes matched after the show's dispatch", got)
	}
	if p.episodeCalls["500/1/5/official/en-US"] != 1 || p.episodeCalls["500/1/6/official/en-US"] != 1 {
		t.Fatalf("new episodes fetched other than once: %v", p.episodeCalls)
	}
}

// A show not yet matched is still woken by a new episode, as before.
func TestNewEpisodeStillWakesAShowNotYetMatched(t *testing.T) {
	_, db, _, _, add, _ := episodeFreshnessFixture(t)
	publicationExec(t, db, `UPDATE screen_metadata_work SET status='unmatched',next_attempt='2099-01-01T00:00:00Z' WHERE target_kind='show'`)
	_, generation, _, _ := showWakeState(t, db)
	add(5)
	_, after, status, _ := showWakeState(t, db)
	if after == generation || status != "pending" {
		t.Fatalf("an unmatched show was not woken by a new episode: generation %d -> %d, status %q", generation, after, status)
	}
}
