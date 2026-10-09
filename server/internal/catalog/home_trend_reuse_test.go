package catalog

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

func TestHomeRecommendedReusesTheSameRequestTrendingRanking(t *testing.T) {
	f := recommendationFixtureDB(t)
	r := engineRequest().scoped()
	engineExec(t, f, `UPDATE screen_metadata_consent SET confirmed=1`)
	engineExec(t, f, `INSERT INTO screen_metadata_policies(library_id,providers) VALUES('movies','["tmdb"]') ON CONFLICT(library_id) DO UPDATE SET providers=excluded.providers`)
	for n := 1; n <= 3; n++ {
		engineMeta(t, f, fmt.Sprintf("m%d", n), "tmdb", fmt.Sprint(100+n))
		engineExec(t, f, `INSERT INTO metadata_discovery_items VALUES('tmdb','movie',?,?,?,?)`, fmt.Sprint(100+n), n, r.Now.Add(-time.Hour).Format(time.RFC3339), r.Now.Add(time.Hour).Format(time.RFC3339))
	}
	f.c.Drain()
	expected, err := New(f.db).recommendationCandidates(r, "trending_now", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) != 3 {
		t.Fatalf("fixture trends=%d, want three", len(expected))
	}
	s := New(f.db)
	s.recSources = map[string]homeSource{}
	recommended, err := s.homeEngineSource(r, homeRowSpec{ID: "recommended"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cost := dbwork.Measure(context.Background())
	trending, err := s.WithContext(ctx).homeEngineSource(r, homeRowSpec{ID: "trending_now"})
	if err != nil {
		t.Fatal(err)
	}
	if cost().Statements != 0 {
		t.Fatalf("Trending rescored an existing request ranking with %d statements", cost().Statements)
	}
	if !reflect.DeepEqual(trending.candidates, expected) {
		t.Fatal("reused trend ranking differs from independent provider ranking")
	}
	for _, candidate := range recommended.candidates {
		for _, trend := range expected {
			if candidate.Work == trend.Work {
				t.Fatal("recommended failed to suppress the visible trend preview")
			}
		}
	}
	// A customized order may compose Trending first; either order must produce
	// the same rankings, without changing the live provider order.
	reversed := New(f.db)
	reversed.recSources = map[string]homeSource{}
	first, err := reversed.homeEngineSource(r, homeRowSpec{ID: "trending_now"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := reversed.homeEngineSource(r, homeRowSpec{ID: "recommended"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.candidates, trending.candidates) || !reflect.DeepEqual(second.candidates, recommended.candidates) {
		t.Fatal("Home row order changed its rankings")
	}
	// Request entry points allocate a new source map, so revoking consent must
	// remove the old provider rows instead of reusing a previous request's list.
	engineExec(t, f, `UPDATE screen_metadata_consent SET confirmed=0`)
	row, err := s.HomeSingleRow(r, "trending_now", HomeRowPage{})
	if err != nil {
		t.Fatal(err)
	}
	if row.Total != 0 || len(row.Entries) != 0 {
		t.Fatal("a new request reused trends after consent was revoked")
	}
}

func TestHomeHiddenTrendingDoesNotSuppressRecommended(t *testing.T) {
	f := recommendationFixtureDB(t)
	r := engineRequest().scoped()
	r.HiddenRowIDs = []string{"trending_now"}
	s := New(f.db)
	s.recSources = map[string]homeSource{}
	expected, err := s.modelledCandidates(r, "recommended")
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.homeEngineSource(r, homeRowSpec{ID: "recommended"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source.candidates, expected) {
		t.Fatal("hidden Trending changed Recommended")
	}
	if _, ok := s.recSources["trending_now:"+idsJSON(r.Libraries)]; ok {
		t.Fatal("hidden Trending was unnecessarily scored")
	}
}
