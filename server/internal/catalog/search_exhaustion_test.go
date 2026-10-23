package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
)

func TestSearchExhaustionStableRevision1203(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("films", "Films", "movie", "/films")
	hidden := c.Library("private", "Private", "movie", "/private")
	items := searchBulkMovies(t, c, lib, "/films", 1203, func(i int) string { return fmt.Sprintf("Harbor %04d", i) })
	restricted := c.Movie(lib, "/films/restricted.mkv", "Harbor restricted", 2026)
	c.Attributes(restricted.ID, "label", "Adult")
	c.Movie(hidden, "/private/hidden.mkv", "Harbor private", 2026)
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "viewer", Fence: "current", Libraries: []string{"films"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}
	for _, sort := range []string{"relevance", "title"} {
		t.Run(sort, func(t *testing.T) {
			r := SearchRequest{Viewer: viewer, Q: "Harbor", Group: "movies", Sort: sort, Direction: "asc", Limit: 37}
			got := []string{}
			seen := map[string]bool{}
			var revision ContentRevision
			var previousCursor string
			for {
				page, err := s.Search(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Groups) != 1 || page.Groups[0].Status != "success" {
					t.Fatalf("group failure %+v", page.Groups)
				}
				g := page.Groups[0]
				replay, err := s.Search(context.Background(), r)
				if err != nil || len(replay.Groups) != 1 {
					t.Fatalf("identical replay failed %v", err)
				}
				replayed := replay.Groups[0]
				// Signed continuations can be issued on opposite sides of a
				// second. Compare their decoded position and revision as well as
				// every returned fact; only their expiry may differ.
				if g.NextCursor != "" {
					var anchors []cursorValue
					for _, raw := range []string{g.NextCursor, replayed.NextCursor} {
						payload, _, _ := strings.Cut(raw, ".")
						data, decodeErr := base64.RawURLEncoding.DecodeString(payload)
						var anchor cursorValue
						if decodeErr != nil || json.Unmarshal(data, &anchor) != nil {
							t.Fatal("invalid replay continuation")
						}
						if _, decodeErr = s.decodeSearchCursor(raw, anchor.Scope, page.Revision); decodeErr != nil {
							t.Fatal(decodeErr)
						}
						anchor.Expires = 0
						anchors = append(anchors, anchor)
					}
					if !reflect.DeepEqual(anchors[0], anchors[1]) {
						t.Fatal("replay changed continuation position/revision")
					}
				}
				g.NextCursor, replayed.NextCursor = "", ""
				if !reflect.DeepEqual(g, replayed) {
					t.Fatal("identical replay changed page facts")
				}
				g.NextCursor = replay.Groups[0].NextCursor
				if len(got) == 0 {
					revision = page.Revision
				}
				if revision != page.Revision {
					t.Fatal("mixed revision")
				}
				prefix := append([]string{}, got...)
				for _, e := range g.Items {
					if seen[e.ID] || e.ID == restricted.Public || e.LibraryID != "films" {
						t.Fatal("duplicate/restricted identity", e.ID)
					}
					seen[e.ID] = true
					got = append(got, e.ID)
				}
				if !reflect.DeepEqual(prefix, got[:len(prefix)]) {
					t.Fatal("loaded prefix changed")
				}
				if g.NextCursor == "" {
					break
				}
				if g.NextCursor == previousCursor {
					t.Fatal("repeated continuation")
				}
				previousCursor = g.NextCursor
				r.Cursor = g.NextCursor
			}
			if len(got) != 1203 {
				t.Fatalf("true exhaustion returned %d", len(got))
			}
			for _, item := range items {
				if !seen[item.Public] {
					t.Fatal("source member omitted", item.Public)
				}
			}
			r.Cursor = ""
			first, err := s.Search(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			r.Cursor = first.Groups[0].NextCursor
			c.Exec(`UPDATE library_revisions SET revision=revision+1 WHERE library_id='films'`)
			if _, err = s.Search(context.Background(), r); !errors.Is(err, ErrStaleContinuation) {
				t.Fatalf("changed revision did not request restart: %v", err)
			}
		})
	}
}
