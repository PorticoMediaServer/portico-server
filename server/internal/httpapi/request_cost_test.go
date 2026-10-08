package httpapi

import (
	"portico.local/server/internal/httpapi/fixture"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

// The statement counter is the instrument every later phase is measured with, so
// it gets its own test: one request in, a number out, attributed to the route
// that produced it.
func TestRequestCostIsAttributedToOneRoute(t *testing.T) {
	tier := performanceTier{name: "cost", shape: fixture.Tiny(), concurrentViewers: 1, iterations: 1, maximumP95: time.Minute, maximumP99: time.Minute, allowBoundedOverload: true}
	f := newLoadFixture(t, tier)

	ResetRouteCosts()
	token := f.viewers[0].AccessToken
	if code, _, body := f.callTimed(token, "GET", "/v1/me", nil); code != 200 {
		t.Fatalf("GET /v1/me %d %s", code, body)
	}
	me := RouteCostFor("GET /v1/me")
	if me.Requests != 1 {
		t.Fatalf("expected one measured request, got %d", me.Requests)
	}
	if me.Statements == 0 {
		t.Fatal("authentication ran no statements, which cannot be true")
	}
	if me.Acquisitions == 0 {
		t.Fatal("authentication took no pooled connection, which cannot be true")
	}
	t.Logf("GET /v1/me statements=%d transactions=%d acquisitions=%d", me.Statements, me.Transactions, me.Acquisitions)

	ResetRouteCosts()
	if code, _, body := f.callTimed(token, "GET", "/v1/home", nil); code != 200 {
		t.Fatalf("GET /v1/home %d %s", code, body)
	}
	home := RouteCostFor("GET /v1/home")
	if home.Requests != 1 || home.Statements == 0 {
		t.Fatalf("home cost was not recorded: %#v", home)
	}
	t.Logf("GET /v1/home statements=%d transactions=%d acquisitions=%d", home.Statements, home.Transactions, home.Acquisitions)
	// A composite page must cost more than an authentication, or the counter is
	// not seeing the composition.
	if home.Statements <= me.Statements {
		t.Fatalf("home (%d statements) did not cost more than /v1/me (%d)", home.Statements, me.Statements)
	}
	// Nothing else may have been attributed to this route.
	if other := RouteCostFor("GET /v1/me"); other.Requests != 0 {
		t.Fatalf("a second route was charged for this request: %#v", other)
	}
}

// measureRoute reports what one request cost, counted two ways: the attributed
// per-request counter, and the process-wide counter. The two agree only when
// every statement on the path reached the database through a context. Where they
// disagree, the difference is exactly the set of call sites still using
// `database/sql`'s context-free forms — which is a useful number in its own
// right while the read paths are being converted.
func measureRoute(t *testing.T, f *loadFixture, token, pattern, method, path string, body any) (RouteCost, int64) {
	t.Helper()
	ResetRouteCosts()
	before := dbworkReads()
	code, _, response := f.callTimed(token, method, path, body)
	if code < 200 || code > 299 {
		t.Fatalf("%s %s -> %d %s", method, path, code, response)
	}
	return RouteCostFor(pattern), dbworkReads() - before
}

func dbworkReads() int64 { return int64(dbwork.Reads().Statements) }

// The statement budgets in the audit's load-test tier are the numbers this
// records. It is a report, not a gate: the gates live in the load test.
func TestReadPathStatementBaseline(t *testing.T) {
	tier := performanceTier{name: "cost", shape: fixture.Tiny(), concurrentViewers: 1, iterations: 1, maximumP95: time.Minute, maximumP99: time.Minute, allowBoundedOverload: true}
	f := newLoadFixture(t, tier)
	token := f.viewers[0].AccessToken
	item := f.items[0]
	cases := []struct {
		name, pattern, method, path string
		body                        any
	}{
		{"auth", "GET /v1/me", "GET", "/v1/me", nil},
		{"libraries", "GET /v1/libraries", "GET", "/v1/libraries", nil},
		{"items", "GET /v1/items", "GET", "/v1/items?limit=100", nil},
		{"home", "GET /v1/home", "GET", "/v1/home", nil},
		{"browse", "POST /v1/libraries/{id}/browse", "POST", "/v1/libraries/" + f.library + "/browse",
			map[string]any{"pivot": "movies", "sort": []map[string]string{{"field": "title", "direction": "asc"}}, "limit": 40}},
		{"detail", "GET /v1/items/{id}/detail", "GET", "/v1/items/" + item + "/detail", nil},
		{"search", "GET /v1/search", "GET", "/v1/search?q=Title&limit=10", nil},
		{"facets", "GET /v1/libraries/{id}/facets", "GET", "/v1/libraries/" + f.library + "/facets?field=contentRating", nil},
	}
	for _, c := range cases {
		cost, total := measureRoute(t, f, token, c.pattern, c.method, c.path, c.body)
		t.Logf("%-10s attributed statements=%3d transactions=%2d acquisitions=%2d | observed statements=%3d",
			c.name, cost.Statements, cost.Transactions, cost.Acquisitions, total)
	}
}
