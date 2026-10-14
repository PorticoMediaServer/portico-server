package catalog

import (
	"context"
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"testing"
)

func TestNumericPrefixesKeepUnicodeAndTypingSteps(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("m", "Movies", "movie", "/m")
	arabic := c.Movie(lib, "/m/a.mkv", "Serial ١٢٣٤٥٦٧٨٩", 2000)
	padded := c.Movie(lib, "/m/b.mkv", "Title 000000000001", 2000)
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"m"}}
	for _, test := range []struct{ q, id string }{{"١", arabic.Public}, {"١٢", arabic.Public}, {"Title 0", padded.Public}, {"Title 00", padded.Public}, {"Title 000", padded.Public}, {"Title 0000", padded.Public}, {"Title 00000", padded.Public}, {"Title 000000", padded.Public}} {
		out, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: test.q, Group: "movies", Limit: 10})
		if err != nil || len(out.Groups) != 1 || out.Groups[0].Status != "success" || len(out.Groups[0].Items) != 1 || out.Groups[0].Items[0].ID != test.id {
			t.Fatalf("q=%s out=%+v err=%v", test.q, out.Groups, err)
		}
	}
}

func TestAmbiguousLabelsNeverClaimPermissionCounts(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("m", "Movies", "movie", "/m")
	for i := 0; i < 20; i++ {
		item := c.Movie(lib, fmt.Sprintf("/m/%d.mkv", i), "Harbor", 2000)
		if i%2 == 0 {
			c.Attributes(item.ID, "label", "Hidden|Private")
		}
	}
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"m"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Hidden|Private"}}}
	out, err := s.Search(context.Background(), SearchRequest{Viewer: viewer, Q: "Harbor", Group: "movies", Limit: 3})
	if err != nil || out.Groups[0].Status != "success" || len(out.Groups[0].Items) != 3 || out.Groups[0].TotalCountExact {
		t.Fatalf("out=%+v err=%v", out.Groups, err)
	}
	for _, item := range out.Groups[0].Items {
		var denied bool
		if err = c.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM catalog_entities e JOIN catalog_item_attribute_edges x ON x.item_id=e.id JOIN catalog_attribute_terms t ON t.id=x.term_id WHERE e.public_id=pid_blob(?) AND t.value_key='hidden|private')`, item.ID).Scan(&denied); err != nil || denied {
			t.Fatalf("denied=%v err=%v", denied, err)
		}
	}
}
