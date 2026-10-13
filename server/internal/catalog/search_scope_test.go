package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"testing"
)

func TestSearchNormalizedScope(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("films", "Films", "movie", "/films")
	fixtures := []string{"Spider-Man", "WALL·E", "M*A*S*H", "X", "千と千尋の神隠し", "기생충", "Éclair", "Rocky 10", "Rocky 2"}
	for i, title := range fixtures {
		c.Movie(library, fmt.Sprintf("/films/%d.mkv", i), title, 1979)
	}
	film := c.Movie(library, "/films/alias.mkv", "Harbor", 2020)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := compactcatalog.SetFieldsTx(ctx, tx, film.ID, compactcatalog.Automatic, map[string]any{"original_title": "Lumière"}); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO provider_evidence VALUES(?,'fixture',1,'{"aliases":["Beacon"]}','now')`, film.ID)
		return err
	})
	c.Drain()
	s := New(c.DB)
	for _, term := range []string{"spiderman", "wall e", "mash", "M*A*S*H", "X", "神隠し", "생", "eclair", "lumiere", "beac"} {
		t.Run(term, func(t *testing.T) {
			out, err := s.Search(context.Background(), SearchRequest{Viewer: Viewer{Profile: "p", Fence: "f", Libraries: []string{"films"}}, Q: term, Group: "movies", Limit: 10})
			if err != nil || len(out.Groups) != 1 || out.Groups[0].Status != "success" || len(out.Groups[0].Items) == 0 {
				t.Fatalf("search %q: %+v %v", term, out, err)
			}
		})
	}
	out, err := s.Search(context.Background(), SearchRequest{Viewer: Viewer{Profile: "p", Fence: "f", Libraries: []string{"films"}}, Q: "Rocky", Group: "movies", Sort: "title", Limit: 10})
	if err != nil || len(out.Groups[0].Items) != 2 || out.Groups[0].Items[0].Title != "Rocky 2" {
		t.Fatalf("natural search order %+v %v", out, err)
	}
}

func TestAggregatePagingBeyondTwoHundred(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("books", "Books", "audiobook", "/books")
	for i := 0; i < 205; i++ {
		book := c.Book(library, fmt.Sprintf("Book %d", i), "Author")
		part := c.BookFile(book, 1, fmt.Sprintf("/books/%d.m4b", i))
		c.Attributes(part.ID, "series", fmt.Sprintf("Series %d", i))
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			return compactcatalog.SetFieldsTx(ctx, tx, book.ID, compactcatalog.Automatic, map[string]any{"series": fmt.Sprintf("Series %d", i)})
		})
	}
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"books"}}
	req := BrowseRequest{Library: "books", Pivot: "series", Limit: 100}
	first, err := s.BrowseEntities(viewer, req)
	if err != nil || first.PageInfo.Total != 205 || len(first.Entries) != 100 || !first.PageInfo.HasMore {
		t.Fatalf("first %+v %v", first, err)
	}
	req.Cursor = first.PageInfo.NextCursor
	second, err := s.BrowseEntities(viewer, req)
	if err != nil || second.PageInfo.Start != 100 || len(second.Entries) != 100 {
		t.Fatalf("second %+v %v", second, err)
	}
	req.Cursor = second.PageInfo.NextCursor
	last, err := s.BrowseEntities(viewer, req)
	if err != nil || last.PageInfo.Start != 200 || len(last.Entries) != 5 || last.PageInfo.HasMore {
		t.Fatalf("last %+v %v", last, err)
	}
	seen := map[string]bool{}
	for _, page := range []BrowseResult{first, second, last} {
		for _, entry := range page.Entries {
			if seen[entry.ID] {
				t.Fatal("duplicate", entry.ID)
			}
			seen[entry.ID] = true
		}
	}
	req.Cursor = ""
	req.Range = &BrowseRange{Start: 203, Revision: first.PageInfo.Revision}
	direct, err := s.BrowseEntities(viewer, req)
	if err != nil || len(direct.Entries) != 2 {
		t.Fatalf("range %+v %v", direct, err)
	}
}
