package catalog

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

// The maintained facet counts equal a live count over the same facts after
// every kind of change: values added and removed, an item deleted, a term
// renamed, a year changed, a collection joined.
func TestFacetCountsFollowEveryChange(t *testing.T) {
	c := catalogtest.Open(t)
	films := c.Library("m", "Movies", "movie", "/m")
	items := []catalogtest.Item{}
	for i := 0; i < 12; i++ {
		it := c.Movie(films, fmt.Sprintf("/m/%02d.mkv", i), fmt.Sprintf("Film %02d", i), 1990+i)
		c.Genres(it.ID, "tmdb", []string{"Drama", "Comedy", "Horror"}[i%3])
		c.Attributes(it.ID, "studio", []string{"Acme", "Zenith"}[i%2])
		c.Attributes(it.ID, "tag", "Tag "+fmt.Sprint(i%4), "Shared")
		items = append(items, it)
	}
	c.Drain()
	s := New(c.DB)
	check := func(stage string) {
		t.Helper()
		if err := compactcatalog.CheckFacetCounts(context.Background(), c.DB); err != nil {
			t.Fatal(stage, err)
		}
		for field, source := range facetSources {
			got, err := s.facetsFromCounts(FacetRequest{Library: "m", Field: field})
			if err != nil {
				t.Fatal(stage, field, err)
			}
			where := `e.library_id=(SELECT id FROM catalog_libraries WHERE library_id='m') AND e.item_id IS NOT NULL`
			if field == "decade" || field == "year" {
				where += ` AND e.year BETWEEN 1800 AND 2199`
			}
			rows, err := c.DB.Query(`SELECT ` + source.Value + `,count(DISTINCT e.entity_id) FROM catalog_browse_rows e ` + source.Join + ` WHERE ` + where + ` GROUP BY 1`)
			if err != nil {
				t.Fatal(stage, field, err)
			}
			want := []string{}
			for rows.Next() {
				var value string
				var n int
				if err = rows.Scan(&value, &n); err != nil {
					t.Fatal(err)
				}
				if value != "" {
					want = append(want, fmt.Sprintf("%s=%d", value, n))
				}
			}
			rows.Close()
			have := []string{}
			for _, v := range got {
				have = append(have, fmt.Sprintf("%s=%d", v.Value, v.Count))
			}
			sort.Strings(want)
			sort.Strings(have)
			if fmt.Sprint(have) != fmt.Sprint(want) {
				t.Fatalf("%s %s:\n maintained %v\n live       %v", stage, field, have, want)
			}
		}
	}
	check("after the import")
	c.Genres(items[0].ID, "tmdb", "Drama", "Western")
	c.Attributes(items[1].ID, "tag")
	c.Delete(items[2].ID)
	c.Fields(items[3].ID, map[string]any{"year": 2021})
	c.Collection(films, "Saga", items[4], items[5])
	c.Drain()
	check("after edits")
	c.Exec(`UPDATE catalog_terms SET label='Dramatic' WHERE label='Drama'`)
	c.Drain()
	check("after a term rename")
}
