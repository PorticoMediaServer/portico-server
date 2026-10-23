package httpapi

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// Search's additive expectations start with the recorded membership, ordering,
// counts and paging. New facts come from Tiny's source rows, never Search's
// composer, enrichment or actual answer. The recorded golden remains frozen.
func goldenSearchExtension(t *testing.T, f *goldenFixture, c goldenCase, want string) string {
	t.Helper()
	if !strings.HasPrefix(c.Name, "search:") || c.Status != 200 {
		return want
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(want), &doc); err != nil {
		t.Fatal(err)
	}
	caps := doc["capabilities"].(map[string]any)
	caps["sorts"] = append(caps["sorts"].([]any), "channelTitle")
	for _, raw := range caps["groups"].([]any) {
		raw.(map[string]any)["directions"] = []string{"asc", "desc"}
	}
	for _, raw := range doc["groups"].([]any) {
		group := raw.(map[string]any)
		group["sort"], group["direction"] = "relevance", "desc"
		for _, raw := range group["items"].([]any) {
			e := raw.(map[string]any)
			kind := e["kind"].(string)
			if kind == "person" {
				if subtitle, ok := e["subtitle"].(string); ok && subtitle != "" {
					e["roles"] = strings.Split(subtitle, " · ")
				}
				e["knownFor"] = goldenSearchKnownFor(t, f, c.Viewer, e["title"].(string))
				delete(e, "subtitle")
				continue
			}
			id := f.public[e["id"].(string)]
			e["watchlisted"] = false // Both golden profiles are new; fixture activity belongs to other profiles.
			delete(e, "subtitle")
			var year int
			if err := f.db.QueryRow(`SELECT year FROM catalog_entities WHERE public_id=pid_blob(?)`, id).Scan(&year); err != nil {
				t.Fatal(err)
			}
			if year > 0 {
				e["year"] = year
			}
			if kind == "movie" || kind == "episode" || kind == "song" {
				e["progressMs"], e["watched"] = 0, false
			}
			switch kind {
			case "show":
				e["seasonCount"] = 1 // Tiny gives every show exactly one season; the restricted show has one admitted episode.
			case "episode":
				var show, title string
				var season, episode int
				if err := f.db.QueryRow(`SELECT pid(sh.public_id),sh.title,se.number,ep.number FROM catalog_episodes ep JOIN catalog_entities sh ON sh.id=ep.show_id JOIN catalog_seasons se ON se.entity_id=ep.season_id JOIN catalog_entities e ON e.id=ep.entity_id WHERE e.public_id=pid_blob(?)`, id).Scan(&show, &title, &season, &episode); err != nil {
					t.Fatal(err)
				}
				e["show"], e["seasonNumber"], e["episodeNumber"] = map[string]any{"id": normalise(show), "name": title}, season, episode
			case "song":
				var album, title, artist, artistName string
				var disc, track int
				if err := f.db.QueryRow(`SELECT pid(a.public_id),a.title,pid(ar.public_id),ar.title,s.disc_number,s.track_number FROM catalog_songs s JOIN catalog_entities e ON e.id=s.entity_id JOIN catalog_entities a ON a.id=s.album_id JOIN catalog_albums al ON al.entity_id=a.id JOIN catalog_entities ar ON ar.id=al.artist_id WHERE e.public_id=pid_blob(?)`, id).Scan(&album, &title, &artist, &artistName, &disc, &track); err != nil {
					t.Fatal(err)
				}
				e["album"], e["artist"] = map[string]any{"id": normalise(album), "name": title}, map[string]any{"id": normalise(artist), "name": artistName}
				e["discNumber"], e["trackNumber"], e["favorite"] = disc, track, false
			case "book":
				e["narrators"] = []string{"Narrator"}
			}
			if strings.HasPrefix(c.Name, "search:Person/") {
				var name string
				if err := f.db.QueryRow(`SELECT credited_name FROM catalog_credits cr JOIN catalog_entities e ON e.id=cr.entity_id WHERE e.public_id=pid_blob(?) ORDER BY cr.ord LIMIT 1`, id).Scan(&name); err != nil {
					t.Fatal(err)
				}
				e["matchedOn"] = map[string]any{"field": "credit", "text": name}
			}
		}
	}
	// In this narrowed fixture only movie38 is a unique whole-word Word002
	// prefix. The open viewer also has a matching book, so has no Top result.
	if c.Name == "search:Word002/restricted" {
		var selected map[string]any
		for _, raw := range doc["groups"].([]any)[0].(map[string]any)["items"].([]any) {
			e := raw.(map[string]any)
			if e["id"] == "movie-0000038" {
				selected = e
			}
		}
		if selected == nil {
			t.Fatal("recorded unique Top candidate missing")
		}
		top := map[string]any{}
		for k, v := range selected {
			top[k] = v
		}
		top["contentRating"], top["genres"] = "G", []string{"Thriller", "Fantasy"}
		top["overview"] = strings.Repeat("A synopsis sentence that is long enough to matter on the wire. ", 6)
		doc["top"] = map[string]any{"group": "movies", "entry": top}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return canonical(string(data))
}

func goldenSearchKnownFor(t *testing.T, f *goldenFixture, viewer, name string) map[string]any {
	t.Helper()
	type credit struct {
		id                            int64
		public, title, library, added string
		ordinal                       int
	}
	rows, err := f.db.Query(`SELECT i.id,pid(i.public_id),i.title,l.library_id,COALESCE(d.added_text,''),cr.ord FROM catalog_credits cr JOIN catalog_people p ON p.id=cr.person_id JOIN catalog_entities i ON i.id=cr.entity_id JOIN catalog_libraries l ON l.id=i.library_id LEFT JOIN catalog_item_details d ON d.entity_id=i.id WHERE p.name=?`, name)
	if err != nil {
		t.Fatal(err)
	}
	var credits []credit
	for rows.Next() {
		var x credit
		if err = rows.Scan(&x.id, &x.public, &x.title, &x.library, &x.added, &x.ordinal); err != nil {
			t.Fatal(err)
		}
		credits = append(credits, x)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	sort.Slice(credits, func(i, j int) bool {
		a, b := credits[i], credits[j]
		if a.ordinal != b.ordinal {
			return a.ordinal < b.ordinal
		}
		if a.added != b.added {
			return a.added > b.added
		}
		return a.id < b.id
	})
	for _, x := range credits {
		if viewer == "restricted" {
			if x.library != "fixture-movies" && x.library != "fixture-tv" {
				continue
			}
			attributes, err := f.db.Query(`SELECT field.field,edge.source_value FROM catalog_item_attribute_edges edge JOIN catalog_attribute_terms term ON term.id=edge.term_id JOIN catalog_attribute_fields field ON field.id=term.field_id WHERE edge.item_id=?`, x.id)
			if err != nil {
				t.Fatal(err)
			}
			allowedRating, blocked := false, false
			for attributes.Next() {
				var field, value string
				if err = attributes.Scan(&field, &value); err != nil {
					t.Fatal(err)
				}
				if field == "contentRating" && (value == "G" || value == "PG" || value == "PG-13") {
					allowedRating = true
				}
				if field == "label" && (value == "violence" || value == "nudity") {
					blocked = true
				}
			}
			if err = attributes.Err(); err != nil {
				t.Fatal(err)
			}
			attributes.Close()
			if !allowedRating || blocked {
				continue
			}
		}
		return map[string]any{"id": normalise(x.public), "name": x.title}
	}
	t.Fatal("expected visible source credit for", name)
	return nil
}
