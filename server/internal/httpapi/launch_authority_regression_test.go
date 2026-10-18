package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/ingestion"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

func recoveryRequest(f administrationFixture, peer, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = peer
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func TestLaunchRecoveryLocalityAcrossAccountAndRefreshRoutes(t *testing.T) {
	for _, mode := range []string{"private-recovery", "remote-enabled", "hosted-admitted"} {
		t.Run(mode, func(t *testing.T) {
			f := administrationFixtureNew(t)
			remote := mode == "remote-enabled"
			if _, err := dbwork.ExecWrite(context.Background(), f.deps.DB, dbwork.ClassSecurityFence, `INSERT INTO onboarding_state_v1(singleton,auth_mode,operation_id,request_hash,setup_hash,receipt_until,recovery_account,recovery_confirmed,remote_recovery,ready) VALUES(1,'hosted','launch-recovery','hash','hash',0,'owner',1,?,1)`, remote); err != nil {
				t.Fatal(err)
			}
			session, err := f.deps.Identity.Issue("owner", "owner-profile", "local", "owner", 1)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "hosted-admitted" {
				if _, err = f.deps.DB.Exec(`INSERT INTO portico_admitted_families(family_id,signer_key_id) VALUES(?,'')`, session.SessionFamilyID); err != nil {
					t.Fatal(err)
				}
			}
			before := func() string {
				var p, m, g int
				if err := f.deps.DB.QueryRow(`SELECT (SELECT count(*) FROM direct_profiles),(SELECT count(*) FROM direct_memberships),(SELECT current_generation FROM authorization_session_families WHERE id=?)`, session.SessionFamilyID).Scan(&p, &m, &g); err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal([]int{p, m, g})
				return string(raw)
			}
			state := before()
			routes := []struct {
				method, path, body string
				status             int
			}{
				{"GET", "/v1/me", "", 200},
				{"GET", "/v1/direct/members", "", 200},
				{"POST", "/v1/direct/profiles", `{"name":"Launch profile","art":"blue"}`, 201},
				{"POST", "/v1/direct/members", `{"username":"launchmember","password":"launch-password-123","name":"Launch member","allowedLibraries":[]}`, 201},
			}
			for _, q := range routes {
				w := recoveryRequest(f, remotePeer, q.method, q.path, session.AccessToken, q.body)
				want := q.status
				if mode == "private-recovery" {
					want = 401
				}
				if w.Code != want {
					t.Fatalf("remote %s %s: %d want %d: %s", q.method, q.path, w.Code, want, w.Body.String())
				}
			}
			requestID := identity.Token()
			refresh := func(peer, secret, id string) *httptest.ResponseRecorder {
				raw, _ := json.Marshal(map[string]string{"refreshToken": secret, "installationId": session.InstallationID, "requestId": id})
				return recoveryRequest(f, peer, "POST", "/v1/auth/refresh", "", string(raw))
			}
			if mode == "private-recovery" {
				if w := refresh(remotePeer, session.RefreshToken, requestID); w.Code != 401 {
					t.Fatalf("remote refresh: %d %s", w.Code, w.Body.String())
				}
				if before() != state {
					t.Fatal("denied remote account/refresh request mutated state")
				}
				if _, err = f.deps.Identity.DirectMe(context.Background(), session.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
					t.Fatalf("missing locality admitted recovery account: %v", err)
				}
				if _, err = f.deps.Identity.RefreshSession(context.Background(), session.RefreshToken, session.InstallationID, requestID); !errors.Is(err, identity.ErrUnauthorized) {
					t.Fatalf("missing locality admitted recovery renewal: %v", err)
				}
				for _, q := range routes {
					if w := recoveryRequest(f, homePeer, q.method, q.path, session.AccessToken, q.body); w.Code != q.status {
						t.Fatalf("home control %s: %d %s", q.path, w.Code, w.Body.String())
					}
				}
			}
			peer := remotePeer
			if mode == "private-recovery" {
				peer = homePeer
			}
			w := refresh(peer, session.RefreshToken, requestID)
			if w.Code != 200 {
				t.Fatalf("allowed refresh: %d %s", w.Code, w.Body.String())
			}
			var rotated identity.Envelope
			if err = json.Unmarshal(w.Body.Bytes(), &rotated); err != nil || rotated.AccessToken == "" {
				t.Fatalf("refresh response: %v %s", err, w.Body.String())
			}
			replay := refresh(remotePeer, session.RefreshToken, requestID)
			if mode == "private-recovery" {
				if replay.Code != 401 {
					t.Fatalf("remote replay: %d", replay.Code)
				}
				// Even a mismatched replay must not revoke a LAN-only family remotely.
				if w = refresh(remotePeer, session.RefreshToken, identity.Token()); w.Code != 401 {
					t.Fatalf("remote reuse: %d", w.Code)
				}
				replay = refresh(homePeer, session.RefreshToken, requestID)
			}
			if replay.Code != 200 || replay.Body.String() != string(mustRefreshBody(t, refresh(peer, session.RefreshToken, requestID))) {
				t.Fatalf("exact replay not preserved: %d %s", replay.Code, replay.Body.String())
			}
			if w = refresh(peer, rotated.RefreshToken, identity.Token()); w.Code != 200 {
				t.Fatalf("successor unavailable after denied replay: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func mustRefreshBody(t *testing.T, w *httptest.ResponseRecorder) []byte {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("refresh replay: %d %s", w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

func TestLaunchAdminLibraryVisibilityAndManagement(t *testing.T) {
	f := administrationFixtureNew(t)
	c := catalogtest.New(t, f.deps.DB)
	a := c.Library("allowed", "Allowed", "movie", "/allowed")
	b := c.Library("future", "Future", "movie", "/future")
	item := c.Movie(b, "/future/film.mp4", "Future Film", 2001)
	c.Movie(a, "/allowed/film.mp4", "Allowed Film", 2000)
	c.Drain()
	if _, err := f.deps.DB.Exec(`UPDATE direct_memberships SET allowed_libraries='["allowed"]' WHERE account_id='member'`); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []struct {
		token string
		count int
	}{{f.owner, 2}, {f.admin, 2}, {f.member, 1}} {
		w := f.call("GET", "/v1/libraries", actor.token, "")
		if w.Code != 200 {
			t.Fatalf("libraries: %d %s", w.Code, w.Body.String())
		}
		var out struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Items) != actor.count {
			t.Fatalf("library count want %d: %s %v", actor.count, w.Body.String(), err)
		}
	}
	for _, path := range []string{"/v1/admin/libraries", "/v1/admin/libraries/allowed", "/v1/admin/library-settings", "/v1/admin/libraries/future/settings", "/v1/items/" + item.Public + "/detail", "/v1/items/" + item.Public + "/metadata/manual"} {
		if w := f.call("GET", path, f.admin, ""); w.Code != 200 {
			t.Fatalf("admin %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := f.call("GET", "/v1/items/"+item.Public+"/detail", f.member, ""); w.Code != 404 {
		t.Fatalf("member read ungranted item: %d", w.Code)
	}
	for _, path := range []string{"/v1/admin/libraries", "/v1/admin/library-settings", "/v1/items/" + item.Public + "/metadata/manual"} {
		if w := f.call("GET", path, f.member, ""); w.Code != 403 {
			t.Fatalf("member administration %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := f.call("GET", "/v1/items/"+item.Public+"/metadata/manual", f.admin, "")
	var manual catalog.ManualMetadata
	if err := json.Unmarshal(w.Body.Bytes(), &manual); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"expectedRevision": manual.Revision, "title": "Administrator edit"})
	if w = f.call("PATCH", "/v1/items/"+item.Public+"/metadata/manual", f.admin, string(body)); w.Code != 200 {
		t.Fatalf("admin metadata write: %d %s", w.Code, w.Body.String())
	}
	if w = f.call("PUT", "/v1/admin/libraries/order", f.admin, `{"libraryIds":["future","allowed"]}`); w.Code != 200 {
		t.Fatalf("admin library write: %d %s", w.Code, w.Body.String())
	}
	var settings administration.LibraryDocument
	w = f.call("GET", "/v1/admin/libraries/future/settings", f.admin, "")
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(administration.Change[administration.LibrarySettings]{ExpectedRevision: settings.Revision, OperationID: "launch-admin-settings", Settings: settings.Settings})
	if w = f.call("PUT", "/v1/admin/libraries/future/settings", f.admin, string(raw)); w.Code != 200 {
		t.Fatalf("admin library settings: %d %s", w.Code, w.Body.String())
	}
	// These operations still require the security owner, even with library access.
	for _, path := range []string{"/v1/admin/maintenance/settings", "/v1/admin/connectivity/policy", "/v1/setup/state", "/v1/admin/backups", "/v1/admin/metadata/settings"} {
		if w = f.call("GET", path, f.admin, ""); w.Code != 403 {
			t.Fatalf("admin owner-only %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestLaunchLibraryAuthorityRechecksLiveTierInTransaction(t *testing.T) {
	for _, change := range []string{`UPDATE direct_memberships SET role='member' WHERE account_id='admin'`, `UPDATE direct_memberships SET disabled=1 WHERE account_id='admin'`, `UPDATE authorization_session_families SET revoked=1 WHERE account_id='admin'`} {
		t.Run(change, func(t *testing.T) {
			f := administrationFixtureNew(t)
			p, err := f.deps.Identity.Authenticate(f.admin)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.deps.DB.Exec(change); err != nil {
				t.Fatal(err)
			}
			tx, err := f.deps.DB.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err = f.deps.libraryAdministrationAuthorize(context.Background(), tx, p, ""); !errors.Is(err, identity.ErrUnauthorized) && !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("stale admin library authority accepted: %v", err)
			}
			if err = f.deps.manualMetadataAuthorize(context.Background(), tx, p); !errors.Is(err, identity.ErrUnauthorized) && !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("stale admin metadata authority accepted: %v", err)
			}
		})
	}
	// A viewing session of an administrative account never borrows its tier.
	f := administrationFixtureNew(t)
	narrow, err := f.deps.Identity.Issue("admin", "admin-profile", "local", "member", 1)
	if err != nil {
		t.Fatal(err)
	}
	if w := f.call("GET", "/v1/admin/libraries", narrow.AccessToken, ""); w.Code != 403 {
		t.Fatalf("narrow admin session: %d %s", w.Code, w.Body.String())
	}
}

func TestLaunchAdminVisibilityRetainsProfileAndRecordingPrivacy(t *testing.T) {
	f := administrationFixtureNew(t)
	c := catalogtest.New(t, f.deps.DB)
	a := c.Library("shared", "Shared", "movie", "/shared-root")
	b := c.Library("recordings", "Private Recording Library", "movie", "/private-recording-root")
	shared := c.Movie(a, "/shared-root/film.mp4", "Shared film", 2000)
	recording := c.Movie(b, "/private-recording-root/secret.mp4", "Private recording", 2001)
	c.Drain()
	if _, err := f.deps.DB.Exec(`INSERT INTO dvr_private_libraries VALUES('recordings','local','member','member-profile');
 INSERT INTO jobs(id,library_id,status,error,created_at) VALUES('zz-private-scan','recordings','failed','private scanner detail','2026-10-18'),('shared-scan','shared','failed','','2026-10-17')`); err != nil {
		t.Fatal(err)
	}
	f.deps.Ingestion = ingestion.New(f.deps.DB, f.deps.Catalog, assets.Probe{})
	f.handler = New(f.deps)
	lists := []string{"/v1/admin/libraries?limit=1", "/v1/admin/jobs?limit=1", "/v1/admin/library-settings?limit=1"}
	privateMarkers := []string{"recordings", "Private Recording Library", "private-recording-root", "zz-private-scan", "private scanner detail", "secret.mp4"}
	for _, path := range lists {
		w := f.call("GET", path, f.admin, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "shared") {
			t.Fatalf("admin allowed listing %s: %d %s", path, w.Code, w.Body.String())
		}
		for _, marker := range privateMarkers {
			if strings.Contains(w.Body.String(), marker) {
				t.Fatalf("private admin listing %s disclosed %q: %s", path, marker, w.Body.String())
			}
		}
		// The owner's existing configuration view is intentional and is retained.
		w = f.call("GET", strings.ReplaceAll(path, "?limit=1", ""), f.owner, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "recordings") {
			t.Fatalf("owner configuration view changed %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	libraryPaths := func(id, job string) []string {
		return []string{"/v1/admin/libraries/" + id, "/v1/admin/libraries/" + id + "/settings", "/v1/admin/libraries/" + id + "/inventory", "/v1/admin/libraries/" + id + "/inventory-config", "/v1/admin/jobs?libraryId=" + id, "/v1/admin/ingestion/jobs/" + job}
	}
	for _, path := range libraryPaths("shared", "shared-scan") {
		if w := f.call("GET", path, f.admin, ""); w.Code != 200 {
			t.Fatalf("admin allowed resource %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range libraryPaths("recordings", "zz-private-scan") {
		w := f.call("GET", path, f.admin, "")
		if w.Code != 404 {
			t.Fatalf("private admin resource %s: %d %s", path, w.Code, w.Body.String())
		}
		for _, marker := range privateMarkers {
			if strings.Contains(w.Body.String(), marker) {
				t.Fatalf("private refusal disclosed %q: %s", marker, w.Body.String())
			}
		}
	}
	for _, token := range []string{f.owner, f.admin} {
		for _, suffix := range []string{"/detail", "/metadata/manual"} {
			if w := f.call("GET", "/v1/items/"+recording.Public+suffix, token, ""); w.Code != 404 {
				t.Fatalf("administrator read another profile's recording %s: %d %s", suffix, w.Code, w.Body.String())
			}
		}
		if w := f.call("GET", "/v1/libraries", token, ""); w.Code != 200 || strings.Contains(w.Body.String(), `"id":"recordings"`) {
			t.Fatalf("private viewer library disclosed: %d %s", w.Code, w.Body.String())
		}
	}
	if _, err := f.deps.DB.Exec(`UPDATE direct_profiles SET allowed_libraries='[]',revision=revision+1 WHERE id='admin-profile'`); err != nil {
		t.Fatal(err)
	}
	for _, path := range lists {
		w := f.call("GET", path, f.admin, "")
		if w.Code != 200 {
			t.Fatalf("narrowed listing %s: %d %s", path, w.Code, w.Body.String())
		}
		for _, marker := range append(privateMarkers, "shared", "Shared film", "shared-root", "shared-scan") {
			if strings.Contains(w.Body.String(), marker) {
				t.Fatalf("narrowed admin listing %s disclosed %q: %s", path, marker, w.Body.String())
			}
		}
	}
	for _, path := range append(libraryPaths("shared", "shared-scan"), "/v1/items/"+shared.Public+"/detail", "/v1/items/"+shared.Public+"/metadata/manual") {
		if w := f.call("GET", path, f.admin, ""); w.Code != 404 {
			t.Fatalf("narrowed admin resource %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, q := range []struct{ method, path, body string }{
		{"PUT", "/v1/admin/libraries/shared/settings", `{"expectedRevision":1,"operationId":"denied-admin-settings","settings":{}}`},
		{"POST", "/v1/admin/libraries/shared/sources", `{"expectedRevision":1,"name":"Denied","path":"/denied"}`},
		{"DELETE", "/v1/admin/ingestion/jobs/shared-scan", ``},
		{"PUT", "/v1/admin/libraries/order", `{"libraryIds":["shared"]}`},
	} {
		if w := f.call(q.method, q.path, f.admin, q.body); w.Code != 404 {
			t.Fatalf("narrowed admin mutation %s: %d %s", q.path, w.Code, w.Body.String())
		}
	}
}

func TestLaunchRecoveryUsesVerifiedProxyLocality(t *testing.T) {
	f := administrationFixtureNew(t)
	if _, err := f.deps.DB.Exec(`INSERT INTO onboarding_state_v1(singleton,auth_mode,operation_id,request_hash,setup_hash,receipt_until,recovery_account,recovery_confirmed,remote_recovery,ready) VALUES(1,'hosted','proxy-recovery','hash','hash',0,'owner',1,0,1)`); err != nil {
		t.Fatal(err)
	}
	doc, err := f.deps.Console.Settings(context.Background(), operations.AllowServerScope)
	if err != nil {
		t.Fatal(err)
	}
	values := doc.Requested
	values.TrustedProxies = []string{"127.0.0.1/32"}
	p, err := f.deps.Identity.Authenticate(f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.deps.Console.ApplySettings(context.Background(), p, operations.AllowServerScope, operations.SettingsChange{ExpectedRevision: doc.Revision, IdempotencyKey: "launch-proxy-settings", Values: values}); err != nil {
		t.Fatal(err)
	}
	f.handler = New(f.deps)
	for _, q := range []struct {
		peer, forwarded string
		status          int
	}{
		{"127.0.0.1:1234", "198.51.100.2", 401},
		{"127.0.0.1:1234", "192.168.1.2", 200},
		{"198.51.100.2:1234", "192.168.1.2", 401},
		{"127.0.0.1:1234", "", 401},
	} {
		r := httptest.NewRequest("GET", "/v1/direct/members", nil)
		r.RemoteAddr = q.peer
		r.Header.Set("Authorization", "Bearer "+f.owner)
		if q.forwarded != "" {
			r.Header.Set("X-Forwarded-For", q.forwarded)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != q.status {
			t.Fatalf("peer=%s forwarded=%s: %d want %d: %s", q.peer, q.forwarded, w.Code, q.status, w.Body.String())
		}
	}
}
