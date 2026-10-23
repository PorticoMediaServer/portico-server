package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"portico.local/server/internal/httpapi/fixture"
)

// Listening facts are additive to the recorded read-path answers. Expectations
// use the original expected entry IDs and Tiny's raw facts, never an actual
// response or the listening/browse enrichment implementation.
func goldenListeningExtension(t *testing.T, f *goldenFixture, c goldenCase, want string) string {
	t.Helper()
	if c.Status != 200 || !(strings.HasPrefix(c.Name, "browse:") || strings.HasPrefix(c.Name, "content:")) {
		return want
	}
	var doc any
	if err := json.Unmarshal([]byte(want), &doc); err != nil {
		t.Fatal(err)
	}
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				visit(item)
			}
		case map[string]any:
			id, hasID := v["id"].(string)
			_, hasTitle := v["title"].(string)
			if hasID && hasTitle && f.public[id] != "" {
				switch v["kind"] {
				case "song":
					// These two newly issued golden profiles have no favorite events.
					v["favorite"] = false
				case "artist":
					v["albumCount"] = fixture.Tiny().AlbumsPerArtist
				case "book":
					v["narrators"] = []string{"Narrator"}
					rows, err := f.db.Query(`SELECT max(a.duration) FROM catalog_entities b JOIN catalog_book_files p ON p.book_id=b.id JOIN catalog_asset_links l ON l.entity_id=p.entity_id JOIN catalog_assets a ON a.id=l.asset_id WHERE b.public_id=pid_blob(?) GROUP BY p.entity_id`, f.public[id])
					if err != nil {
						t.Fatal(err)
					}
					parts, seconds := 0, 0.0
					for rows.Next() {
						var n float64
						if err = rows.Scan(&n); err != nil {
							t.Fatal(err)
						}
						if n <= 0 {
							t.Fatal("Tiny book has no known duration", id)
						}
						parts++
						seconds += n
					}
					if err = rows.Err(); err != nil {
						t.Fatal(err)
					}
					rows.Close()
					if parts != fixture.Tiny().FilesPerBook {
						t.Fatal("Tiny book part count", id, parts)
					}
					v["durationMs"] = int64(seconds * 1000)
				}
			}
			for _, child := range v {
				visit(child)
			}
		}
	}
	visit(doc)
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return canonical(string(b))
}
