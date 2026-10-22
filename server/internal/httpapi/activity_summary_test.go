package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/playbackv1"
	"reflect"
	"strings"
	"testing"
	"time"

	"portico.local/apikit"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func activitySummaryFixture(t *testing.T) (Dependencies, map[string]identity.Envelope, http.Handler) {
	t.Helper()
	root := t.TempDir()
	db, e := persistence.Open(filepath.Join(root, "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	ident, e := identity.New(db, root)
	if e != nil {
		t.Fatal(e)
	}
	tokens := map[string]identity.Envelope{}
	for _, role := range []string{"owner", "admin", "member"} {
		if _, e = db.Exec(`INSERT INTO accounts VALUES(?,?,x'00',?,1)`, role, role, role+"-profile"); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(`UPDATE direct_memberships SET role=? WHERE account_id=?`, role, role); e != nil {
			t.Fatal(e)
		}
		v, e := ident.Issue(role, role+"-profile", "local", role, 1)
		if e != nil {
			t.Fatal(e)
		}
		tokens[role] = v
	}
	d := Dependencies{DB: db, Identity: ident, Administration: administration.New(db)}
	registry := apikit.NewContextual(d.v1Authorize, policyCatalogue())
	if e = registerActivitySummary(registry, d); e != nil {
		t.Fatal(e)
	}
	return d, tokens, registry
}
func activitySummaryRequest(h http.Handler, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/v1/admin/activity", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestActivitySummaryAuthorityQuietAndRevocation(t *testing.T) {
	d, tokens, h := activitySummaryFixture(t)
	for _, role := range []string{"", "member", "admin"} {
		w := activitySummaryRequest(h, tokens[role].AccessToken)
		want := 403
		if role == "" {
			want = 401
		}
		if w.Code != want || strings.Contains(w.Body.String(), "serverName") || strings.Contains(w.Body.String(), "streams") {
			t.Fatalf("%s got %d %s", role, w.Code, w.Body.String())
		}
	}
	owner := tokens["owner"]
	w := activitySummaryRequest(h, owner.AccessToken)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	var out ActivitySummary
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	if out.Streams.Count != 0 || len(out.Streams.Items) != 0 || len(out.Scans) != 0 || out.Tasks.Count != 0 || out.Update.Available || out.Health.Degraded {
		t.Fatalf("quiet snapshot: %+v", out)
	}
	if out.Scope.ServerID == "" || out.Scope.ViewerFence == "" || out.ObservedAt == "" {
		t.Fatal("missing authority fence")
	}
	p, e := d.Identity.Authenticate(owner.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = d.DB.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id IN(SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, p.Hash); e != nil {
		t.Fatal(e)
	}
	if w = activitySummaryRequest(h, owner.AccessToken); w.Code != 401 || strings.Contains(w.Body.String(), "streams") {
		t.Fatal("revocation leaked summary", w.Code, w.Body.String())
	}
	if _, e = d.readActivitySummary(context.Background(), p); e == nil {
		t.Fatal("stale principal reached summary")
	}
}
func TestActivitySummaryActualScanProgressAndTaskCaps(t *testing.T) {
	d, tokens, h := activitySummaryFixture(t)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := d.DB.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO libraries(id,name,kind,root) VALUES('lib','Movies','movie','/owner-secret-root')`)
	for n := 0; n < 2; n++ {
		id := fmt.Sprint(n)
		exec(`INSERT INTO library_sources(id,library_id,configured_root,root) VALUES(?,'lib',?,?)`, id, "/private/"+id, "/private/"+id)
		exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,'lib','running','2026-10-22')`, id)
		exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,discovered,analyzed,inventory_completed_at) VALUES(?,?,1,'inc','root',1,'basic_running',10,4,?)`, id, id, []string{"2026-10-22", ""}[n])
		exec(`INSERT INTO inventory_source_active VALUES(?,?)`, id, id)
	}
	// The console scan receipt wraps these same source jobs. It sorts before
	// real background work so filtering after the three-row cap would fail.
	exec(`INSERT INTO console_operations(id,kind,resource,actor,trigger,state,phase,revision,attempt,created_ms,updated_ms,next_ms,domain_id,error_code,predecessor,settings_revision,processed) VALUES('scan-wrapper','library-scan','lib','owner','manual','running','basic_running',1,0,0,1,1,'scan-wrapper','','',1,20)`)
	for _, job := range []string{"0", "1"} {
		exec(`INSERT INTO inventory_operation_jobs(operation_id,job_id) VALUES('scan-wrapper',?)`, job)
	}
	for n := 0; n < 5; n++ {
		id := fmt.Sprint(n)
		exec(`INSERT INTO console_operations(id,kind,resource,actor,trigger,state,phase,revision,attempt,created_ms,updated_ms,next_ms,domain_id,error_code,predecessor,settings_revision,processed) VALUES(?,'test',?,'private-actor','manual','running','working',1,0,1,1,1,'private-domain','','',1,7)`, "task-"+id, "resource-"+id)
	}
	get := func() ActivitySummary {
		w := activitySummaryRequest(h, tokens["owner"].AccessToken)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "owner-secret") || strings.Contains(w.Body.String(), "private-") {
			t.Fatal("non-display/private fields leaked", w.Body.String())
		}
		var out ActivitySummary
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	out := get()
	if len(out.Scans) != 1 || out.Scans[0].Discovered != 20 || out.Scans[0].Analyzed != 8 || out.Scans[0].Total != nil || out.Scans[0].Current != "" {
		t.Fatalf("unknown total must stay unknown: %+v", out.Scans)
	}
	if out.Tasks.Count != 5 || len(out.Tasks.Items) != 3 || out.Tasks.Items[0].Processed == nil || *out.Tasks.Items[0].Processed != 7 || out.Tasks.Items[0].Total != nil {
		t.Fatalf("capped actual tasks: %+v", out.Tasks)
	}
	for _, task := range out.Tasks.Items {
		if task.Kind == "library-scan" {
			t.Fatal("scan duplicated as task", out.Tasks)
		}
	}
	exec(`UPDATE inventory_runs SET inventory_completed_at='2026-10-22'`)
	out = get()
	if out.Scans[0].Total == nil || *out.Scans[0].Total != 20 {
		t.Fatalf("known total: %+v", out.Scans)
	}
	exec(`UPDATE inventory_runs SET finished_at='2026-10-22T01:00:00Z' WHERE job_id='0'`)
	out = get()
	if out.LastScanFinishedAt != "2026-10-22T01:00:00Z" {
		t.Fatalf("last scan missing: %+v", out)
	}
}
func TestActivitySummaryRegistrationIsOwnerScoped(t *testing.T) {
	d, _, _ := activitySummaryFixture(t)
	r := apikit.NewContextual(d.v1Authorize, policyCatalogue())
	if e := registerActivitySummary(r, d); e != nil {
		t.Fatal(e)
	}
	d := r.Definitions()[0]
	if d.Access != apikit.Owner || d.Requires != apikit.ScopeAdmin || d.Admission != laneAdmin {
		t.Fatalf("summary authority changed: %+v", d)
	}
}

func TestActivitySummaryPublishedBounds(t *testing.T) {
	shape, e := apikit.Schema(reflect.TypeFor[ActivitySummary]())
	if e != nil {
		t.Fatal(e)
	}
	props := shape["properties"].(map[string]any)
	for key, want := range map[string]int{"streams": 4, "tasks": 3} {
		fields := props[key].(map[string]any)["properties"].(map[string]any)
		if fields["items"].(map[string]any)["maxItems"] != want || fields["count"].(map[string]any)["minimum"] != 0 {
			t.Fatalf("wire bounds missing for %s: %+v", key, fields)
		}
	}
}

func TestActivitySummaryChecksumTotalMatchesAnalyzedInput(t *testing.T) {
	for _, tc := range []struct {
		name, path, container string
		size, processed       int64
		known                 bool
	}{
		{"local input", "movie.mkv", "mkv", 10000000, 2000000, true},
		{"STRM target bytes exceed descriptor", "movie.STRM", "strm", 100, 2000000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, tokens, h := activitySummaryFixture(t)
			c := catalogtest.New(t, d.DB)
			lib := c.Library("lib", "Movies", "movie", "/private-root")
			item := c.Movie(lib, "/private-root/"+tc.path, "Movie", 2026)
			c.Exec(`UPDATE catalog_assets SET container=?,size=? WHERE token=?`, tc.container, tc.size, item.Token)
			c.Exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES('job','lib','running','2026-10-22')`)
			c.Exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase,discovered,analyzed,inventory_completed_at) VALUES('job','lib',1,'inc','root',1,'advanced_running',1,0,'2026-10-22')`)
			c.Exec(`INSERT INTO inventory_source_active VALUES('lib','job')`)
			c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) VALUES('object','lib',?,'inc',?,'revision','{}',?,1)`, item.Token, tc.path, tc.size)
			c.Exec(`INSERT INTO analysis_stage_runs(job_id,object_id,source_revision,policy_revision,stage,algorithm,state,processed_bytes,updated_ms) VALUES('job','object','revision',1,'checksum','checksum-v1','running',?,1)`, tc.processed)
			w := activitySummaryRequest(h, tokens["owner"].AccessToken)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var out ActivitySummary
			if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
				t.Fatal(e)
			}
			if out.Tasks.Count != 1 || len(out.Tasks.Items) != 1 {
				t.Fatalf("analysis task missing: %+v", out.Tasks)
			}
			task := out.Tasks.Items[0]
			if task.Processed == nil || *task.Processed != tc.processed || task.Unit != "bytes" || (task.Total != nil) != tc.known {
				t.Fatalf("input progress mismatch: %+v", task)
			}
			if tc.known && *task.Total != tc.size {
				t.Fatalf("local input total: %d", *task.Total)
			}
			if strings.Contains(w.Body.String(), "private-root") || strings.Contains(w.Body.String(), tc.path) {
				t.Fatal("analysis input leaked", w.Body.String())
			}
			// A stale stage must not survive the existing revision fence.
			c.Exec(`UPDATE analysis_stage_runs SET source_revision='stale'`)
			w = activitySummaryRequest(h, tokens["owner"].AccessToken)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
				t.Fatal(e)
			}
			if out.Tasks.Count != 0 || len(out.Tasks.Items) != 0 {
				t.Fatalf("stale input progress exposed: %+v", out.Tasks)
			}
		})
	}
}

func TestActivitySummaryAuthorityChangesDuringOutstandingRead(t *testing.T) {
	for name, mutation := range map[string]string{
		"revoked family":                  `UPDATE authorization_session_families SET revoked=1 WHERE account_id='owner'`,
		"owner demoted":                   `UPDATE direct_memberships SET role='admin' WHERE account_id='owner'`,
		"profile loses primary authority": `UPDATE direct_profiles SET is_primary=0 WHERE id='owner-profile'`,
		"family role changes":             `UPDATE authorization_session_families SET role='admin' WHERE account_id='owner'`,
	} {
		t.Run(name, func(t *testing.T) {
			d, tokens, _ := activitySummaryFixture(t)
			entered, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			d.v1 = &playbackv1.Service{DB: d.DB, Now: func() time.Time {
				close(entered)
				<-release
				return time.Now()
			}}
			r := apikit.NewContextual(d.v1Authorize, policyCatalogue())
			if e := registerActivitySummary(r, d); e != nil {
				t.Fatal(e)
			}
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() { response <- activitySummaryRequest(r, tokens["owner"].AccessToken) }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("summary never reached the authenticated read")
			}
			if _, e := d.DB.Exec(mutation); e != nil {
				t.Fatal(e)
			}
			// Release once here; the deferred close remains for early failures.
			release <- struct{}{}
			select {
			case w := <-response:
				if w.Code != 401 || strings.Contains(w.Body.String(), "streams") || strings.Contains(w.Body.String(), "serverName") || strings.Contains(w.Body.String(), "current") {
					t.Fatalf("changed authority released private body: %d %s", w.Code, w.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("summary did not finish after authority changed")
			}
		})
	}
}
