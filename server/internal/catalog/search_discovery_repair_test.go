package catalog

import (
	"context"
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"reflect"
	"strings"
	"testing"
)

func TestSearchDiscoveryBoundsColdAndContinuationWork(t *testing.T) {
	type work struct{ cold, next int64 }
	costs := []work{}
	for _, n := range []int{256, 1203} {
		c := catalogtest.Open(t)
		lib := c.Library("movies", "Movies", "movie", "/movies")
		for i := 0; i < n; i++ {
			m := c.Movie(lib, fmt.Sprintf("/movies/%04d.mkv", i), fmt.Sprintf("Film %04d", i), 2026)
			label := "drama"
			if i == 0 {
				label = "Drama"
			}
			c.Genres(m.ID, "local", label)
		}
		c.Drain()
		s := New(c.DB)
		v := Viewer{Profile: "p", Fence: "f", Libraries: []string{"movies"}}
		cursor := ""
		total := 0
		var w work
		pages := 0
		for {
			ctx, measure := dbwork.Measure(context.Background())
			ctx, trace := dbwork.TraceStatements(ctx)
			page, err := s.WithContext(ctx).SearchDiscovery(v, "server", cursor, 12)
			if err != nil {
				t.Fatal(err)
			}
			steps := measure().VMSteps
			for _, q := range trace() {
				if strings.Contains(q, "targets AS MATERIALIZED") || strings.Contains(q, "e AS MATERIALIZED") {
					t.Fatal("unbounded authorized title/member relation", q)
				}
			}
			if pages == 0 {
				w.cold = steps
				if len(page.Genres) != 0 || page.GenresNextCursor == "" {
					t.Fatal("cold request completed an unbounded library count", page)
				}
			}
			if pages == 1 {
				w.next = steps
			}
			replay, err := s.SearchDiscovery(v, "server", cursor, 12)
			if err != nil || !reflect.DeepEqual(page.Genres, replay.Genres) || page.GenresNextCursor != replay.GenresNextCursor && cursor != "" {
				t.Fatal("nonidentical continuation replay", err)
			}
			for _, g := range page.Genres {
				want := n - 1
				if g.Value == "Drama" {
					want = 1
				}
				if (g.Value != "Drama" && g.Value != "drama") || g.Count != want || g.CountIsLowerBound {
					t.Fatal("untruthful contextual count", g)
				}
				total++
			}
			pages++
			if page.GenresNextCursor == "" {
				break
			}
			if page.GenresNextCursor == cursor || pages > 100 {
				t.Fatal("nonexhausting continuation")
			}
			cursor = page.GenresNextCursor
		}
		if total != 2 || pages < 3 {
			t.Fatal("display cap mistaken for exhaustion", total, pages)
		}
		t.Logf("source=%d cold_vm=%d continuation_vm=%d pages=%d", n, w.cold, w.next, pages)
		costs = append(costs, w)
	}
	for i, small := range []int64{costs[0].cold, costs[0].next} {
		large := []int64{costs[1].cold, costs[1].next}[i]
		if small <= 0 || large > small*3/2+1000 {
			t.Fatalf("work grew with full library: small=%d large=%d", small, large)
		}
	}
}

func TestSearchDiscoveryLargeShowMembersStayPrivateAndExhaust(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("tv", "TV", "tv", "/tv")
	show := c.Show(lib, "A show", 2026)
	season := c.Season(show, 1)
	for i := 0; i < 1203; i++ {
		e := c.Episode(show, season, i+1, fmt.Sprintf("/tv/%04d.mkv", i))
		if i < 1202 {
			c.Genres(e.ID, "local", "Secret hidden genre")
			c.Attributes(e.ID, "label", "Adult")
		} else {
			c.Genres(e.ID, "local", "Visible final genre")
		}
	}
	c.Drain()
	s := New(c.DB)
	v := Viewer{Profile: "p", Fence: "f", Libraries: []string{"tv"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}
	cursor := ""
	seen := 0
	first := ""
	for pages := 0; ; pages++ {
		page, err := s.SearchDiscovery(v, "server", cursor, 12)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range page.Genres {
			if g.Value != "Visible final genre" || g.Count != 1 || g.CountIsLowerBound {
				t.Fatal("hidden member advertised identity/count", g)
			}
			seen++
		}
		if pages == 0 {
			first = page.GenresNextCursor
		}
		if page.GenresNextCursor == "" {
			break
		}
		if pages > 100 {
			t.Fatal("huge show did not exhaust")
		}
		cursor = page.GenresNextCursor
	}
	if seen != 1 || first == "" {
		t.Fatal("missed last visible member", seen)
	}
	v.Restrictions = identity.ContentRestrictions{}
	if _, err := s.SearchDiscovery(v, "server", first, 12); err == nil {
		t.Fatal("restriction change reused private accumulator")
	}
}

func TestSearchDiscoveryGenreLabelOrderingAndOverrides(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("movies", "Movies", "movie", "/movies")
	for i, label := range []string{"Zebra", "apple", "Banana", "banana"} {
		m := c.Movie(lib, fmt.Sprintf("/movies/%d.mkv", i), fmt.Sprint(i), 2026)
		c.Genres(m.ID, "local", label)
	}
	c.Drain()
	var overrides int
	if err := c.DB.QueryRow(`SELECT count(*) FROM catalog_term_sources ts JOIN catalog_terms t ON t.id=ts.term_id WHERE t.vocab=1 AND ts.label_override IS NOT NULL`).Scan(&overrides); err != nil || overrides == 0 {
		t.Fatal("fixture did not produce real label overrides", overrides, err)
	}
	s := New(c.DB)
	v := Viewer{Profile: "p", Fence: "f", Libraries: []string{"movies"}}
	cursor := ""
	got := []string{}
	for {
		page, err := s.SearchDiscovery(v, "s", cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range page.Genres {
			if g.Count != 1 {
				t.Fatal(g)
			}
			got = append(got, g.Value)
		}
		if page.GenresNextCursor == "" {
			break
		}
		cursor = page.GenresNextCursor
		if len(got) > 4 {
			t.Fatal("did not exhaust labels")
		}
	}
	if !reflect.DeepEqual(got, []string{"apple", "Banana", "banana", "Zebra"}) {
		t.Fatal("genre label continuation changed prior NOCASE/value ordering", got)
	}
}
