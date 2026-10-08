package catalog

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHomeReadOnlyScale(t *testing.T) {
	path := os.Getenv("PORTICO_HOME_SCALE_DB")
	if path == "" {
		t.Skip("set isolated scale fixture database")
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	s := New(db)
	for _, mode := range []string{"continue_watching", "continue_listening"} {
		q := homeContinueSQL(mode)
		q = strings.Replace(q, "?", "'viewer'", 1)
		q = strings.Replace(q, "?", "'large'", 1)
		scalePlan(t, db, `SELECT i.id,a.updated_at`+q+` ORDER BY a.updated_at DESC,a.item_id DESC LIMIT 12`)
	}
	scalePlan(t, db, `SELECT e.entity_id FROM catalog_browse_rows e INDEXED BY catalog_browse_feature JOIN catalog_entities ce ON ce.id=e.entity_id WHERE e.library_id=(SELECT id FROM catalog_libraries WHERE library_id='large') AND e.kind=1 AND e.available=1 AND e.backdrop=1 ORDER BY e.entity_id LIMIT 1`)
	for n := 0; n < 11; n++ {
		start := time.Now()
		p, e := s.Home(HomeRequest{Viewer: Viewer{Profile: "viewer", Fence: "home", Libraries: []string{"large", "other"}}, Profile: "viewer", ViewerFence: "home", Libraries: []string{"large", "other"}})
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("Home iteration%d %s sections=%d", n, time.Since(start), len(p.Sections))
	}
}
