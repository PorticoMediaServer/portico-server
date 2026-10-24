package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/readingtest"
)

func readingTreeFile(t *testing.T, r *readingFixture, relative string) string {
	t.Helper()
	name := filepath.Join(filepath.Dir(r.path), filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	var data []byte
	switch filepath.Ext(name) {
	case ".epub":
		data = readingtest.EPUB()
	case ".cbz":
		data = readingtest.CBZ()
	default:
		data = readingtest.PDF()
	}
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func readingPathItem(t *testing.T, r *readingFixture, path string) string {
	t.Helper()
	var id string
	if err := r.f.db.QueryRow(`SELECT pid(e.public_id) FROM catalog_assets a JOIN reading_publications p ON p.asset_id=a.id JOIN catalog_entities e ON e.id=p.item_id WHERE a.path=?`, path).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestReadingPrivatePathProducerGeneratedTree(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	for _, relative := range []string{
		"Frank Herbert/Dune Saga/Dune.epub", "Frank Herbert/Dune Saga/Dune Messiah.pdf",
		"Frank Herbert/Dune (12)/Dune.epub", "Frank Herbert/Dune (12)/Dune.pdf",
		"Isaac Asimov/Foundation.pdf", "Isaac Asimov/Isaac Asimov - Foundation.epub",
		"Books/Sci-fi/Frank Herbert/Dune.pdf", "Ebooks/Foundation.pdf",
		"Some Book [Group] (Digital) 1080p.pdf", "[Group] (Digital).pdf",
	} {
		readingTreeFile(t, r, relative)
	}
	readingScan(t, r.f, r.library)
	list, err := r.s.List(ctx, r.p, r.library, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]int{}
	for i, v := range list.Items {
		byID[v.ID] = i
	}
	for _, tc := range []struct {
		path, title, author, series string
		source                      compactcatalog.ReadingValueSource
	}{
		{"Frank Herbert/Dune Saga/Dune.epub", "Dune", "Frank Herbert", "Dune Saga", compactcatalog.ReadingFolder},
		{"Frank Herbert/Dune Saga/Dune Messiah.pdf", "Dune Messiah", "Frank Herbert", "Dune Saga", compactcatalog.ReadingFolder},
		{"Frank Herbert/Dune (12)/Dune.epub", "Dune", "Frank Herbert", "", compactcatalog.ReadingFolder},
		{"Isaac Asimov/Foundation.pdf", "Foundation", "Isaac Asimov", "", compactcatalog.ReadingFolder},
		{"Isaac Asimov/Isaac Asimov - Foundation.epub", "Foundation", "Isaac Asimov", "", compactcatalog.ReadingFileName},
		{"Books/Sci-fi/Frank Herbert/Dune.pdf", "Dune", "Frank Herbert", "", compactcatalog.ReadingFolder},
		{"Ebooks/Foundation.pdf", "Foundation", "", "", ""},
		{"Some Book [Group] (Digital) 1080p.pdf", "Some Book", "", "", ""},
		{"[Group] (Digital).pdf", "[Group] (Digital)", "", "", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			id := readingPathItem(t, r, filepath.Join(filepath.Dir(r.path), filepath.FromSlash(tc.path)))
			i, ok := byID[id]
			if !ok {
				t.Fatal("readable file was dropped")
			}
			row := list.Items[i]
			if row.Title != tc.title {
				t.Errorf("title=%q want %q", row.Title, tc.title)
			}
			if tc.author == "" {
				if row.Author != nil {
					t.Errorf("generic/root attribution invented: %+v", row.Author)
				}
			} else if row.Author == nil || row.Author.Value != tc.author || row.Author.Source != tc.source {
				t.Errorf("author=%+v want %q/%s", row.Author, tc.author, tc.source)
			}
			if tc.series == "" {
				if row.Series != nil {
					t.Errorf("per-book folder became series: %+v", row.Series)
				}
			} else if row.Series == nil || row.Series.Name != tc.series || row.Series.Source != compactcatalog.ReadingFolder {
				t.Errorf("series=%+v want %q", row.Series, tc.series)
			}
		})
	}
	first := readingPathItem(t, r, filepath.Join(filepath.Dir(r.path), "Frank Herbert/Dune (12)/Dune.epub"))
	second := readingPathItem(t, r, filepath.Join(filepath.Dir(r.path), "Frank Herbert/Dune (12)/Dune.pdf"))
	if first != second {
		t.Fatal("formats counted/attached as two titles")
	}
	versions, err := r.s.Versions(ctx, r.p, first)
	if err != nil || len(versions) != 2 {
		t.Fatal("actual alternative versions lost", versions, err)
	}
}

func TestReadingPrivatePathComicFolderReclassification(t *testing.T) {
	r := newReadingFixture(t, "comic", "cbz")
	ctx := context.Background()
	first := readingTreeFile(t, r, "Little Nemo/Little Nemo 01.cbz")
	readingScan(t, r.f, r.library)
	id := readingPathItem(t, r, first)
	get := func() compactcatalog.ReadingValues {
		t.Helper()
		var entity int64
		if err := r.f.db.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, id).Scan(&entity); err != nil {
			t.Fatal(err)
		}
		g, err := dbwork.BeginSnapshot(ctx, r.f.db)
		if err != nil {
			t.Fatal(err)
		}
		defer g.Rollback()
		v, err := compactcatalog.ReadingAutomaticFieldsTx(ctx, g.Tx(), entity)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v := get()
	if v.Author == nil || v.Author.Value != "Little Nemo" || v.Series != nil {
		t.Fatal("one-title folder guess is not genuine", v)
	}
	second := readingTreeFile(t, r, "Little Nemo/02.cbz")
	readingScan(t, r.f, r.library)
	v = get()
	if v.Author != nil || v.Series == nil || v.Series.Value != "Little Nemo" || v.SeriesNumber == nil || v.SeriesNumber.Value != 1 {
		t.Fatal("1-to-2 folder refresh failed", v)
	}
	secondID := readingPathItem(t, r, second)
	list, err := r.s.List(ctx, r.p, r.library, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, row := range list.Items {
		if row.ID == secondID {
			seen = row.Series != nil && row.Series.Number != nil && *row.Series.Number == 2
		}
	}
	if !seen {
		t.Fatal("bare sequence token was not published")
	}
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	readingScan(t, r.f, r.library)
	v = get()
	if v.Author == nil || v.Author.Value != "Little Nemo" || v.Series != nil {
		t.Fatal("missing sibling did not refresh available title", v)
	}
}
