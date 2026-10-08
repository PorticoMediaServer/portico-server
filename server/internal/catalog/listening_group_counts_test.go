package catalog

import (
	"context"
	"database/sql"
	"testing"

	"portico.local/server/internal/compactcatalog"
)

func TestRestrictedBookGroupCountTracksVisibleBooks(t *testing.T) {
	s, c, names := phase34ListeningFixture(t)
	if _, err := c.DB.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('r',17),('pg',10)`); err != nil {
		t.Fatal(err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetAttributesTx(ctx, tx, names["part-b1"].ID, "contentRating", []string{"R"})
	})
	c.Drain()
	if more, err := s.ClassifyPendingRatings(context.Background()); err != nil || more {
		t.Fatal("pending ratings", more, err)
	}
	if err := s.RebuildVisibilityClass(context.Background(), "books", phase34RestrictionOf(phase34Ceiling(13), false)); err != nil {
		t.Fatal(err)
	}
	c.Drain()
	r := phase34ListeningRequest("books", "series", "")
	r.Viewer.Restrictions = phase34RestrictionOf(phase34Ceiling(13), false)
	page, err := s.Content(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Sections) != 1 || len(page.Sections[0].Entries) != 1 || page.Sections[0].Entries[0].Count == nil || *page.Sections[0].Entries[0].Count != 1 {
		t.Fatalf("restricted series count: %+v", page.Sections)
	}
	r.View = "book_series"
	r.EntityID = page.Sections[0].Entries[0].ID
	r.Sort = "order"
	child, err := s.Content(r)
	if err != nil || len(child.Sections) != 1 || child.Sections[0].TotalCount != 1 || child.Sections[0].Entries[0].ID != names["book-a"].Public {
		t.Fatalf("restricted series books: %+v %v", child.Sections, err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetAttributesTx(ctx, tx, names["part-b1"].ID, "contentRating", []string{"PG"})
	})
	c.Drain()
	if more, err := s.ClassifyPendingRatings(context.Background()); err != nil || more {
		t.Fatal("pending ratings", more, err)
	}
	// A class one revision behind serves its last published generation.
	if _, err = s.Content(r); err != nil {
		t.Fatalf("a stale class failed the page: %v", err)
	}
	if err = s.RebuildVisibilityClass(context.Background(), "books", phase34RestrictionOf(phase34Ceiling(13), false)); err != nil {
		t.Fatal(err)
	}
	c.Drain()
	r.View = "series"
	r.EntityID = ""
	r.Sort = ""
	page, err = s.Content(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Sections) != 1 || len(page.Sections[0].Entries) != 1 || page.Sections[0].Entries[0].Count == nil || *page.Sections[0].Entries[0].Count != 2 {
		t.Fatalf("updated restricted series count: %+v", page.Sections)
	}
}
