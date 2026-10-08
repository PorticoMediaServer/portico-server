package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/httpapi/fixture"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/playback"
)

// The read path is being rebuilt underneath these routes: counts become
// maintained counters, the browse union view becomes a materialised table, the
// letter index becomes a prefix sum, facets become bucketed rollups. Every one
// of those changes is a promise that the *answer* does not change — only where
// it comes from.
//
// This is the test that holds the promise. It drives a representative matrix of
// route × pivot × sort × restriction against a deterministic catalogue,
// normalises the handful of values that cannot be deterministic across runs (a
// session hash, an HMAC-signed cursor, a wall clock), and compares the result
// against a file recorded on the behaviour that existed before any of the read
// models were built.
//
// A case records its status code and its whole normalised body, so a change of
// membership, of ordering, of a count, of a letter index or of an error is a
// diff on a named case rather than a silent difference nobody looks at. The
// golden file is regenerated only by running with PORTICO_GOLDEN_UPDATE=1, and
// regenerating it is the thing this workstream is not allowed to do.

// volatileKeys are the JSON keys whose values cannot be stable across two runs
// of the same catalogue: they carry a session hash, an HMAC signature, a wall
// clock or a per-install identity. Each is replaced by a marker that still
// records whether a value was present, so "a cursor appeared" and "no cursor
// appeared" remain different results.
var volatileKeys = map[string]bool{
	"viewerFence": true, "generatedAt": true, "nextCursor": true, "serverId": true,
	"cursor": true, "expiresAt": true, "issuedAt": true, "updatedAt": true,
	"receiptId": true, "operationId": true, "etag": true,
}

// stringRevisionKeys are keys whose value is a revision *digest* (a string) that
// folds the viewer fence in, as opposed to a revision *counter* (a number) that
// does not. Only the digest form is volatile.
func normaliseValue(key string, value any) any {
	if volatileKeys[key] {
		if value == nil {
			return nil
		}
		if text, ok := value.(string); ok {
			if text == "" {
				return ""
			}
			return "<volatile>"
		}
	}
	if key == "revision" {
		if text, ok := value.(string); ok && text != "" {
			return "<volatile>"
		}
	}
	return normalise(value)
}

// digestValue matches a value that is a bare cryptographic digest — a person id
// derived from a per-install salt, a viewer fence, a signed revision. Nothing in
// the fixture's own identifiers looks like one: they are all spelled
// `movie-0000000`, `fixture-music`, `collection-00000`.
var digestValue = regexp.MustCompile(`^[0-9a-f]{32}(?:[0-9a-f]{32})?$`)

// goldenNames spells every public id as its entity's source key. Public ids are
// random per install; the source key (`movie-0000000`) is what the fixture chose.
var goldenNames = strings.NewReplacer()

func normalise(value any) any {
	if text, ok := value.(string); ok && digestValue.MatchString(text) {
		return "<digest>"
	}
	if text, ok := value.(string); ok {
		return goldenNames.Replace(text)
	}
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, inner := range typed {
			out[key] = normaliseValue(key, inner)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, inner := range typed {
			out = append(out, normalise(inner))
		}
		return out
	}
	return value
}

// goldenCase is one request and the answer it produced.
type goldenCase struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
	Viewer string `json:"viewer"`
	Status int    `json:"status"`
	// Response is the normalised body, re-encoded with sorted keys.
	Response string `json:"response"`
}

// goldenFixture names the deterministic catalogue the matrix addresses.
type goldenFixture struct {
	library fixture.Libraries
	item    string
}

func buildGoldenFixture(t *testing.T) (*goldenFixture, func(method, path, body, viewer string) (int, string)) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "db")
	libraries, err := fixture.Build(context.Background(), fixture.Tiny(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stablePublicIDs(t, db)
	ident, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.Verify(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(db)
	// The production classifier runs after catalogue publication. Settle the
	// fixture's pending ratings before comparing stable restricted projections.
	if _, err = cat.ClassifyPendingRatings(context.Background()); err != nil {
		t.Fatal(err)
	}
	cat.Clock = func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) }
	host, err := hosted.New(db, ident, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{DB: db, Identity: ident, Catalog: cat, Hosted: host, Playback: playback.New(db)})

	all := fmt.Sprintf(`["%s","%s","%s","%s"]`, libraries.Movies, libraries.Shows, libraries.Music, libraries.Books)
	restriction := fixture.Restriction(libraries)
	narrowed, _ := json.Marshal(restriction.Libraries)
	blocked, _ := json.Marshal(restriction.BlockedLabels)

	// Fixed identities: a golden file cannot depend on a random account id.
	if _, err = db.Exec(`INSERT INTO accounts VALUES('golden-owner','golden-owner',x'00','golden-owner-profile',1)`); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	owner, err := ident.Issue("golden-owner", "golden-owner-profile", "local", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	tokens["owner"] = owner.AccessToken
	for _, viewer := range []struct {
		name, account, profile, libraries string
		restricted                        bool
	}{
		{"open", "golden-open", "golden-open-profile", all, false},
		{"restricted", "golden-limited", "golden-limited-profile", string(narrowed), true},
	} {
		if _, err = db.Exec(`INSERT INTO accounts VALUES(?,?,x'00',?,1)`, viewer.account, viewer.account, viewer.profile); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO direct_memberships(account_id,role,allowed_libraries,revision,disabled) VALUES(?,'member',?,1,0)
 ON CONFLICT(account_id) DO UPDATE SET role='member',allowed_libraries=excluded.allowed_libraries,disabled=0`, viewer.account, all); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO direct_profiles(id,account_id,name,is_primary,position,allowed_libraries) VALUES(?,?,?,1,0,?)
 ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id,is_primary=1,deleted=0,allowed_libraries=excluded.allowed_libraries`,
			viewer.profile, viewer.account, viewer.name, viewer.libraries); err != nil {
			t.Fatal(err)
		}
		if viewer.restricted {
			if _, err = db.Exec(`INSERT INTO profile_restrictions(profile_id,maximum_age,blocked_labels,allow_unrated,revision)
 VALUES(?,?,?,0,1) ON CONFLICT(profile_id) DO UPDATE SET maximum_age=excluded.maximum_age,blocked_labels=excluded.blocked_labels,allow_unrated=0`,
				viewer.profile, restriction.MaximumAge, string(blocked)); err != nil {
				t.Fatal(err)
			}
		}
		envelope, issueErr := ident.Issue(viewer.account, viewer.profile, "local", "member", 1)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		tokens[viewer.name] = envelope.AccessToken
	}

	// Stable restricted collection/category reads use a completed policy class;
	// the production worker builds it outside request transactions.
	settleCompactCatalogue(t, db)
	for _, library := range restriction.Libraries {
		if err = cat.RebuildVisibilityClass(context.Background(), library, identity.ContentRestrictions{MaximumAge: &restriction.MaximumAge, BlockUnrated: true, BlockedLabels: restriction.BlockedLabels}); err != nil {
			t.Fatal(err)
		}
	}
	settleCompactCatalogue(t, db)
	call := func(method, path, body, viewer string) (int, string) {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+tokens[viewer])
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	rows, err := db.Query(`SELECT pid(public_id),source_key FROM catalog_identities UNION ALL SELECT token,'asset-'||replace(replace(path,'/fixture/',''),'.mp4','') FROM catalog_assets`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs []string
	item := ""
	for rows.Next() {
		var public, source string
		if err = rows.Scan(&public, &source); err != nil {
			t.Fatal(err)
		}
		// `file:/fixture/movie-0000000.mp4` and `show:show-000` name what the fixture called them.
		if rest, ok := strings.CutPrefix(source, "file:/fixture/"); ok {
			source = strings.TrimSuffix(rest, ".mp4")
		} else if _, rest, ok := strings.Cut(source, ":"); ok {
			source = rest
		}
		pairs = append(pairs, public, source)
		if source == "movie-0000000" {
			item = public
		}
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if item == "" {
		t.Fatal("the fixture has no movie-0000000")
	}
	goldenNames = strings.NewReplacer(pairs...)
	return &goldenFixture{library: libraries, item: item}, call
}

// goldenMatrix is the representative set: every browsable pivot of every library
// kind, every sort the pivot publishes, the aggregate pivots, the paging shapes
// (first page, offset range, seek prefix), the facet fields, search, people,
// detail, the composite pages and every home row — each as an unrestricted and
// as a restricted viewer.
func goldenMatrix(libraries fixture.Libraries, item string) []goldenCase {
	cases := []goldenCase{}
	add := func(name, method, path, body string) {
		for _, viewer := range []string{"open", "restricted"} {
			cases = append(cases, goldenCase{Name: name + "/" + viewer, Method: method, Path: path, Body: body, Viewer: viewer})
		}
	}

	add("home", "GET", "/v1/home", "")
	add("content", "GET", "/v1/content?limit=10", "")
	add("suggestions", "GET", "/v1/suggestions?limit=10", "")
	for _, row := range []string{"continue", "continue_listening", "ondeck", "recommended", "trending",
		"recent_" + libraries.Movies, "recent_" + libraries.Shows, "recent_" + libraries.Music, "recent_" + libraries.Books} {
		add("row:"+row, "GET", "/v1/home/rows/"+row+"?limit=6", "")
		add("row:"+row+":page2", "GET", "/v1/home/rows/"+row+"?limit=6&start=6", "")
	}

	type pivotCase struct{ library, pivot string }
	pivots := []pivotCase{
		{libraries.Movies, "movies"}, {libraries.Movies, "collections"}, {libraries.Movies, "categories"},
		{libraries.Shows, "shows"}, {libraries.Shows, "episodes"}, {libraries.Shows, "collections"}, {libraries.Shows, "categories"},
		{libraries.Music, "artists"}, {libraries.Music, "albums"}, {libraries.Music, "songs"}, {libraries.Music, "genres"},
		{libraries.Books, "authors"}, {libraries.Books, "books"}, {libraries.Books, "collections"}, {libraries.Books, "series"},
	}
	sorts := []struct{ field, direction string }{
		{"title", "asc"}, {"title", "desc"}, {"added", "desc"}, {"year", "desc"},
		{"duration", "desc"}, {"communityRating", "desc"}, {"personalRating", "desc"}, {"lastPlayed", "desc"},
	}
	for _, pivot := range pivots {
		// The name carries the library as well as the pivot: three library kinds
		// publish a `collections` pivot and they are different sets.
		name := "browse:" + pivot.library + ":" + pivot.pivot
		for _, sort := range sorts {
			body := fmt.Sprintf(`{"pivot":%q,"sort":[{"field":%q,"direction":%q}],"limit":8}`, pivot.pivot, sort.field, sort.direction)
			add(name+":"+sort.field+":"+sort.direction, "POST", "/v1/libraries/"+pivot.library+"/browse", body)
		}
		// Paging shapes over the default sort: a direct range (the offset path)
		// and a seek prefix (the letter index path).
		add(name+":range", "POST", "/v1/libraries/"+pivot.library+"/browse",
			fmt.Sprintf(`{"pivot":%q,"limit":8,"range":{"start":5}}`, pivot.pivot))
		add(name+":seek", "POST", "/v1/libraries/"+pivot.library+"/browse",
			fmt.Sprintf(`{"pivot":%q,"limit":8,"seek":{"prefix":"W"}}`, pivot.pivot))
	}
	// Expression predicates, which are what the materialised row must keep
	// answering identically: an entity-scoped field, an item-scoped join, a
	// personal-state join and the availability rollup.
	for _, query := range []struct{ name, expression string }{
		{"genre", `{"field":"genre","operator":"contains","value":"Drama"}`},
		{"year", `{"field":"year","operator":"at-least","value":2000}`},
		{"playState", `{"field":"playState","operator":"equals","value":"unplayed"}`},
		{"availability", `{"field":"availability","operator":"equals","value":"available"}`},
		{"contentRating", `{"field":"contentRating","operator":"in","value":["G","PG"]}`},
		{"label", `{"field":"label","operator":"contains-any","value":["violence"]}`},
		{"title", `{"field":"title","operator":"starts-with","value":"W"}`},
	} {
		add("browse:query:"+query.name, "POST", "/v1/libraries/"+libraries.Movies+"/browse",
			fmt.Sprintf(`{"pivot":"movies","query":%s,"limit":8}`, query.expression))
	}

	for _, field := range catalog.BrowseFacetFields() {
		add("facets:"+field, "GET", "/v1/libraries/"+libraries.Movies+"/facets?field="+field, "")
	}
	add("facets:genre:music", "GET", "/v1/libraries/"+libraries.Music+"/facets?field=genre", "")
	add("facets:genre:q", "GET", "/v1/libraries/"+libraries.Movies+"/facets?field=genre&q=ram", "")

	for _, term := range []string{"Word001", "Word002", "Volume", "Release", "Person"} {
		add("search:"+term, "GET", "/v1/search?q="+term+"&limit=8", "")
	}
	add("people", "GET", "/v1/people?q=Person&limit=10", "")
	add("detail", "GET", "/v1/items/"+item+"/detail", "")
	add("item", "GET", "/v1/items/"+item, "")
	add("items", "GET", "/v1/items?libraryId="+libraries.Movies+"&limit=10", "")
	add("libraries", "GET", "/v1/libraries", "")
	for _, view := range []string{"discover", "browse", "collections", "categories"} {
		add("content:"+view, "GET", "/v1/libraries/"+libraries.Movies+"/content?view="+view+"&limit=8", "")
		add("content:music:"+view, "GET", "/v1/libraries/"+libraries.Music+"/content?view="+view+"&limit=8", "")
	}
	add("capabilities", "GET", "/v1/libraries/"+libraries.Movies+"/browse-capabilities?pivot=movies", "")
	return cases
}

// canonical re-encodes a body with map keys sorted and volatile values replaced,
// so two runs of the same catalogue produce byte-identical text.
func canonical(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return trimmed
	}
	encoded, err := json.Marshal(normalise(decoded))
	if err != nil {
		return trimmed
	}
	return string(encoded)
}

const goldenPath = "testdata/readpath_golden.json"

func TestReadPathGoldenEquivalence(t *testing.T) {
	f, call := buildGoldenFixture(t)
	cases := goldenMatrix(f.library, f.item)
	for index := range cases {
		status, body := call(cases[index].Method, cases[index].Path, cases[index].Body, cases[index].Viewer)
		cases[index].Status = status
		cases[index].Response = canonical(body)
	}
	sort.SliceStable(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	seen := map[string]bool{}
	for _, entry := range cases {
		if seen[entry.Name] {
			t.Fatalf("two matrix cases are named %q; a golden file cannot hold both", entry.Name)
		}
		seen[entry.Name] = true
	}

	if os.Getenv("PORTICO_GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.MarshalIndent(cases, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(goldenPath, append(encoded, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d golden cases", len(cases))
		return
	}

	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%v: record it with PORTICO_GOLDEN_UPDATE=1", err)
	}
	var expected []goldenCase
	if err = json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	byName := map[string]goldenCase{}
	for _, entry := range expected {
		byName[entry.Name] = entry
	}
	if len(expected) != len(cases) {
		t.Errorf("the matrix has %d cases and the golden file has %d", len(cases), len(expected))
	}
	differences := 0
	for _, actual := range cases {
		want, known := byName[actual.Name]
		if !known {
			t.Errorf("%s is not in the golden file", actual.Name)
			continue
		}
		if want.Status != actual.Status {
			differences++
			t.Errorf("%s: status %d, golden %d\n  body: %s", actual.Name, actual.Status, want.Status, truncate(actual.Response))
			continue
		}
		if want.Response != actual.Response {
			differences++
			if differences <= 8 {
				t.Errorf("%s: the answer changed\n  now:    %s\n  golden: %s", actual.Name, truncate(actual.Response), truncate(want.Response))
			}
		}
	}
	if differences > 8 {
		t.Errorf("%d cases differ in total", differences)
	}
}

func truncate(text string) string {
	if len(text) <= 900 {
		return text
	}
	return text[:900] + "…"
}

// stablePublicIDs replaces the fixture's random public ids with ones derived
// from each entity's source key. Recommendations rotate ties by a hash of the
// public id (a fresh order per install), so only fixed ids give a fixed answer.
func stablePublicIDs(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT public_id,root,source_key FROM catalog_identities`)
	if err != nil {
		t.Fatal(err)
	}
	type change struct{ from, to []byte }
	var changes []change
	for rows.Next() {
		var public []byte
		var root, key string
		if err = rows.Scan(&public, &root, &key); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(root + "\x00" + key))
		changes = append(changes, change{public, sum[:16]})
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		for _, statement := range []string{`UPDATE catalog_entities SET public_id=?2 WHERE public_id=?1`, `UPDATE catalog_identities SET public_id=?2 WHERE public_id=?1`} {
			if _, err = db.Exec(statement, c.from, c.to); err != nil {
				t.Fatal(err)
			}
		}
	}
}
