package catalog

import (
	"context"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// Scripts without word spaces match by substring: a Japanese, Korean or Thai
// query meets the bigram index and is verified against the folded title, so a
// mid-title fragment finds its title without spaces to split on.
func TestSearchSubwordScriptsMatchSubstring(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("m", "Movies", "movie", "/m")
	tokyo := c.Movie(lib, "/m/tokyo.mkv", "東京タワー", 2000)
	spirit := c.Movie(lib, "/m/spirit.mkv", "千と千尋の神隠し", 2001)
	seoul := c.Movie(lib, "/m/seoul.mkv", "서울특별시", 2002)
	bangkok := c.Movie(lib, "/m/bangkok.mkv", "กรุงเทพมหานคร", 2003)
	harbor := c.Movie(lib, "/m/harbor.mkv", "Harbor Movie", 2004)
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"m"}}
	for _, test := range []struct{ q, id string }{
		{"東京", tokyo.Public},
		{"タワー", tokyo.Public},
		{"千尋", spirit.Public},
		{"서울", seoul.Public},
		{"특별", seoul.Public},
		{"เทพ", bangkok.Public},
		{"มหานคร", bangkok.Public},
		{"Harbor", harbor.Public}, // the Latin control: the same path must still find Latin titles
	} {
		out, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: test.q, Group: "movies", Limit: 10})
		if err != nil || len(out.Groups) != 1 || out.Groups[0].Status != "success" {
			t.Fatalf("q=%s out=%+v err=%v", test.q, out.Groups, err)
		}
		items := out.Groups[0].Items
		if len(items) != 1 || items[0].ID != test.id {
			t.Fatalf("q=%s matched %+v, want %s", test.q, items, test.id)
		}
	}
}
