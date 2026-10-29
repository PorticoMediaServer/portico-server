package httpapi

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
)

// This measures actual route executions on the observed physical connections,
// not db.Prepare handles. Native SQLite automatic reprepares are a separate
// driver-level check: an LRU hit alone does not prove those absent.
func TestLibraryListWarmLoopReusesPreparedStatements(t *testing.T) {
	d, session := completedLocalFoundationFixture(t)
	c := catalogtest.New(t, d.DB)
	for i := range 11 {
		id := fmt.Sprintf("reuse-%d", i)
		c.Library(id, id, "movie", "/"+id)
	}
	read := func() {
		t.Helper()
		r := httptest.NewRequest("GET", "/v1/libraries", nil)
		r.Header.Set("Authorization", "Bearer "+session.AccessToken)
		w := httptest.NewRecorder()
		d.libraryList(w, r)
		if w.Code != 200 {
			t.Fatalf("libraries: %d %s", w.Code, w.Body.String())
		}
	}
	for range 3 {
		read()
	}
	before := dbwork.Reads().StatementCache
	for range 100 {
		read()
	}
	after := dbwork.Reads().StatementCache
	misses, hits := after.Misses-before.Misses, after.Hits-before.Hits
	t.Logf("100 warm library requests: cache misses=%d hits=%d evictions=%d", misses, hits, after.Evictions-before.Evictions)
	if misses != 0 || hits == 0 {
		t.Fatalf("warm library route reparsed SQL: misses=%d hits=%d", misses, hits)
	}
}
