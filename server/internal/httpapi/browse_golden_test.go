package httpapi

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/httpapi/fixture"
	"portico.local/server/internal/identity"
)

// Extend the recorded answers for CB-1/2/3/6/7 without re-recording them. These
// expectations read fixture facts directly, never Browse/Facets or the actual
// response. Recorded membership, ordering, counts and unrelated fields stay exact.
func goldenBrowseViewer(f *goldenFixture, viewer string) catalog.Viewer {
	v := catalog.Viewer{Profile: "golden-open-profile", Libraries: []string{f.library.Movies, f.library.Shows, f.library.Music, f.library.Books}}
	if viewer == "restricted" {
		r := fixture.Restriction(f.library)
		v.Profile = "golden-limited-profile"
		v.Libraries = r.Libraries
		v.Restrictions = identity.ContentRestrictions{MaximumAge: &r.MaximumAge, BlockUnrated: true, BlockedLabels: r.BlockedLabels}
	}
	return v
}

type goldenPlayable struct {
	id            int64
	public, title string
	year          int
	seconds       float64
}

func goldenBrowsePlayables(t *testing.T, f *goldenFixture, c goldenCase) []goldenPlayable {
	t.Helper()
	library := strings.Split(strings.TrimPrefix(c.Path, "/v1/libraries/"), "/")[0]
	var request struct {
		Pivot string              `json:"pivot"`
		Query *catalog.BrowseNode `json:"query"`
	}
	if err := json.Unmarshal([]byte(c.Body), &request); err != nil {
		t.Fatal(err)
	}
	query, err := json.Marshal(request.Query)
	if err != nil {
		t.Fatal(err)
	}
	key := library + ":" + request.Pivot + ":" + c.Viewer + ":" + string(query)
	if f.browsePlayables == nil {
		f.browsePlayables = map[string][]goldenPlayable{}
	}
	if cached, ok := f.browsePlayables[key]; ok {
		return cached
	}
	rows, err := f.db.Query(`SELECT e.id,pid(e.public_id),e.title,e.year,COALESCE((SELECT max(a.duration) FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id WHERE l.entity_id=e.id),0) FROM catalog_entities e JOIN catalog_libraries l ON l.id=e.library_id WHERE l.library_id=? AND e.kind IN(1,4,7,9)`, library)
	if err != nil {
		t.Fatal(err)
	}
	all := []goldenPlayable{}
	for rows.Next() {
		var p goldenPlayable
		if err = rows.Scan(&p.id, &p.public, &p.title, &p.year, &p.seconds); err != nil {
			t.Fatal(err)
		}
		all = append(all, p)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	visible := []goldenPlayable{}
	viewer := goldenBrowseViewer(f, c.Viewer)
	for _, p := range all {
		if catalog.New(f.db).VisibleItem(context.Background(), viewer, p.public) != nil {
			continue
		}
		if request.Pivot == "collections" {
			var n int
			if err = f.db.QueryRow(`SELECT count(*) FROM catalog_collection_members WHERE item_id=?`, p.id).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				continue
			}
		}
		if q := request.Query; q != nil {
			keep := true
			switch q.Field {
			case "year":
				keep = p.year >= 2000
			case "title":
				keep = strings.HasPrefix(p.title, "W")
			case "genre":
				var n int
				err = f.db.QueryRow(`SELECT count(*) FROM catalog_term_sources s JOIN catalog_terms t ON t.id=s.term_id WHERE s.entity_id=? AND t.vocab=1 AND COALESCE(s.label_override,t.label)='Drama'`, p.id).Scan(&n)
				keep = n > 0
			case "contentRating", "label":
				rows, e := f.db.Query(`SELECT edge.source_value FROM catalog_item_attribute_edges edge JOIN catalog_attribute_terms term ON term.id=edge.term_id JOIN catalog_attribute_fields field ON field.id=term.field_id WHERE edge.item_id=? AND field.field=?`, p.id, q.Field)
				if e != nil {
					t.Fatal(e)
				}
				keep = false
				for rows.Next() {
					var value string
					if e = rows.Scan(&value); e != nil {
						t.Fatal(e)
					}
					if q.Field == "label" && value == "violence" || q.Field == "contentRating" && (value == "G" || value == "PG") {
						keep = true
					}
				}
				err = rows.Close()
			case "playState", "availability": // golden profiles have no personal state; all fixture files are live.
			default:
				t.Fatalf("specify independent golden predicate %s", q.Field)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !keep {
				continue
			}
		}
		visible = append(visible, p)
	}
	f.browsePlayables[key] = visible
	return visible
}

func goldenBrowseExtension(t *testing.T, f *goldenFixture, c goldenCase, want string) string {
	t.Helper()
	var doc map[string]any
	if json.Unmarshal([]byte(want), &doc) != nil {
		return want
	}
	if strings.HasPrefix(c.Name, "browse:") && c.Status == 200 {
		page, ok := doc["pageInfo"].(map[string]any)
		if !ok {
			return want
		}
		duration := float64(0)
		lower := false
		if page["revision"] != "" && page["revision"] != nil {
			for _, p := range goldenBrowsePlayables(t, f, c) {
				duration += p.seconds * 1000
				lower = lower || p.seconds <= 0
			}
		}
		page["durationMs"], page["durationIsLowerBound"] = duration, lower
		goldenWholeSelection(t, f, c, doc, page)
		if strings.HasPrefix(c.Name, "browse:query:") {
			anchors := []any{}
			if page["total"].(float64) > 0 {
				anchors = append(anchors, map[string]any{"key": "W", "index": 0})
			}
			doc["positionIndex"] = anchors
		}
	}
	if strings.HasPrefix(c.Name, "facets:") && c.Status == 200 {
		doc["totalIsLowerBound"] = false
		for _, v := range doc["values"].([]any) {
			v.(map[string]any)["countIsLowerBound"] = false
		}
	}
	if strings.HasPrefix(c.Name, "capabilities/") && c.Status == 200 {
		fields := doc["fields"].([]any)
		match := map[string]any{"id": "match", "labelKey": "browse.field.match", "type": "enum", "operators": []string{"equals", "not-equals", "in", "not-in"}, "controlHint": "select", "complexity": "standard", "cost": "indexed-join", "applicableKinds": []string{"movie", "show", "season", "episode", "artist", "album", "song"}, "allowedValues": []string{"matched", "unmatched"}}
		next := []any{fields[0], match}
		next = append(next, fields[1:]...)
		doc["fields"] = next
		for _, v := range next {
			field := v.(map[string]any)
			id := field["id"]
			// The approved relative-date vocabulary adds this operator to
			// these three existing date fields. Preserve every recorded
			// operator and all unrelated capability facts exactly.
			switch id {
			case "releaseDate", "dateAdded", "lastPlayedAt":
				field["operators"] = append([]any{"within-last-days"}, field["operators"].([]any)...)
			}
			if id == "actor" || id == "director" || id == "writer" {
				field["controlHint"] = "facet-multi-select"
				field["facetSource"] = map[string]any{"endpoint": "/v1/libraries/{id}/facets", "field": id}
			}
		}
		for _, v := range doc["sorts"].([]any) {
			s := v.(map[string]any)
			s["ownerOnly"] = s["id"] == "fileSize"
		}
		addMatch := func(p map[string]any) {
			old := p["filters"].([]any)
			if len(old) == 0 {
				return
			}
			n := []any{old[0], "match"}
			p["filters"] = append(n, old[1:]...)
		}
		for _, v := range doc["pivots"].([]any) {
			p := v.(map[string]any)
			if p["id"] != "collections" {
				addMatch(p)
			}
		}
		addMatch(doc["resolvedPivot"].(map[string]any))
		for _, v := range doc["surfaces"].([]any) {
			addMatch(v.(map[string]any))
		}
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return canonical(string(encoded))
}

// The Select-all descriptor counts the independent fixture's authorized
// complete view. It must never be inferred from this response's loaded window.
func goldenWholeSelection(t *testing.T, f *goldenFixture, c goldenCase, doc, page map[string]any) {
	t.Helper()
	var request struct {
		Pivot string `json:"pivot"`
		Range, Seek, Cursor json.RawMessage
	}
	if err := json.Unmarshal([]byte(c.Body), &request); err != nil { t.Fatal(err) }
	kind := map[string]string{"movies":"movie","episodes":"episode","songs":"song"}[request.Pivot]
	if kind == "" || len(request.Range)>0 || len(request.Seek)>0 || len(request.Cursor)>0 || page["revision"]==nil || page["revision"]=="" { return }
	library := strings.Split(strings.TrimPrefix(c.Path,"/v1/libraries/"),"/")[0]
	applied := doc["applied"].(map[string]any)
	query := map[string]any{"libraryId":library,"pivot":request.Pivot,"sort":applied["sort"]}
	if applied["query"] != nil { query["filter"]=applied["query"] }
	n := len(goldenBrowsePlayables(t,f,c))
	types := []any{}
	if n>0 {
		// Both fixed viewers are members; this fixture installs neither a
		// bulk scheduler, downloads nor owner mutation services. All its
		// visible files are available, so only Play and Queue are offered.
		counts := map[string]any{"play":n,"queue":n,"playlist":0,"myList":0,"watched":0,"collection":0,"favorite":0,"download":0,"edit":0,"refresh":0,"delete":0}
		types=append(types,map[string]any{"kind":kind,"count":n,"capabilityCounts":counts,"libraries":[]any{map[string]any{"libraryId":library,"count":n}}})
	}
	doc["wholeSelection"]=map[string]any{"selector":map[string]any{"query":query},"catalogRevision":page["revision"],"total":n,"types":types}
}

func goldenNewBrowseFacets(t *testing.T, f *goldenFixture, c goldenCase) (goldenCase, bool) {
	t.Helper()
	u, err := url.Parse(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	field := u.Query().Get("field")
	if !strings.HasPrefix(c.Name, "facets:") || (field != "actor" && field != "director" && field != "writer" && field != "library") {
		return goldenCase{}, false
	}
	counts := map[string]int{}
	labels := map[string]string{}
	members := goldenBrowsePlayables(t, f, goldenCase{Path: "/v1/libraries/" + f.library.Movies + "/browse", Body: `{"pivot":"movies"}`, Viewer: c.Viewer})
	if field == "library" && len(members) > 0 {
		counts[f.library.Movies] = len(members)
		var label string
		if err = f.db.QueryRow(`SELECT name FROM libraries WHERE id=?`, f.library.Movies).Scan(&label); err != nil {
			t.Fatal(err)
		}
		labels[f.library.Movies] = label
	}
	for _, p := range members {
		if field == "library" {
			continue
		}
		// Tiny intentionally seeds old provider department spellings. Acting is
		// therefore empty, while its explicit Director/Writer role facts are real.
		role := "Director"
		if field == "writer" {
			role = "Writer"
		}
		if field == "actor" {
			continue
		}
		rows, e := f.db.Query(`SELECT DISTINCT c.credited_name FROM catalog_credits c JOIN catalog_credit_labels r ON r.id=c.role_id WHERE c.entity_id=? AND r.label=?`, p.id, role)
		if e != nil {
			t.Fatal(e)
		}
		for rows.Next() {
			var name string
			if e = rows.Scan(&name); e != nil {
				t.Fatal(e)
			}
			counts[name]++
			labels[name] = name
		}
		if e = rows.Close(); e != nil {
			t.Fatal(e)
		}
	}
	keys := []string{}
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return strings.ToLower(labels[keys[i]]) < strings.ToLower(labels[keys[j]])
	})
	values := []any{}
	for _, key := range keys {
		values = append(values, map[string]any{"value": key, "label": labels[key], "count": counts[key], "countIsLowerBound": false})
	}
	doc := map[string]any{"field": field, "values": values, "total": len(keys), "totalIsLowerBound": false, "revision": "<volatile>"}
	raw, e := json.Marshal(doc)
	if e != nil {
		t.Fatal(e)
	}
	c.Status = 200
	c.Response = canonical(string(raw))
	return c, true
}
