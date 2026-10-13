package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/apikit/contract"
	"portico.local/server/internal/access"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/connectivity"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/servicelog"
	"portico.local/server/internal/webhooks"
)

type administrationFixture struct {
	handler      http.Handler
	owner        string
	admin        string
	member       string
	memberDevice string
	recorder     *servicelog.Recorder
	deps         Dependencies
}

func administrationFixtureNew(t *testing.T) administrationFixture {
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
	if _, e = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','owner-profile',1);
 INSERT INTO accounts VALUES('admin','admin',x'00','admin-profile',1);
 INSERT INTO accounts VALUES('member','member',x'00','member-profile',1);
 UPDATE direct_memberships SET role='admin' WHERE account_id='admin';`); e != nil {
		t.Fatal(e)
	}
	ownerToken, e := ident.Issue("owner", "owner-profile", "local", "owner", 1)
	if e != nil {
		t.Fatal(e)
	}
	adminToken, e := ident.Issue("admin", "admin-profile", "local", "admin", 1)
	if e != nil {
		t.Fatal(e)
	}
	memberToken, e := ident.Issue("member", "member-profile", "local", "member", 1)
	if e != nil {
		t.Fatal(e)
	}
	control, e := hosted.New(db, ident, "http://127.0.0.1:19410", base64.RawURLEncoding.EncodeToString(make([]byte, 32)), tl6ZeroHostedRootID)
	if e != nil {
		t.Fatal(e)
	}
	recorder := servicelog.New(servicelog.Options{Capacity: 50})
	recorder.SetLevel("debug")
	deps := Dependencies{DB: db, Identity: ident, Catalog: catalog.New(db), Hosted: control, Console: operations.New(db),
		Access: AccessArea{Access: access.New(db), Logs: recorder, ClientLogs: servicelog.NewClientLogStore(db), Advertiser: connectivity.NewAdvertiser(connectivity.AdvertiserOptions{})}}
	deps.credentials = newCredentialGate()
	deps.Webhooks = webhooks.New(db)
	deps.self = &selfHandler{}
	handler := New(deps)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := FlushCredentialUse(ctx, handler); err != nil {
			t.Errorf("flush credential use: %v", err)
		}
	})
	return administrationFixture{handler: handler, owner: ownerToken.AccessToken, admin: adminToken.AccessToken, member: memberToken.AccessToken, memberDevice: memberToken.DeviceID, recorder: recorder, deps: deps}
}

func (f administrationFixture) call(method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func TestMemberLimitsScopeCatalogueProjectionAndFence(t *testing.T) {
	f := administrationFixtureNew(t)
	c := catalogtest.New(t, f.deps.DB)
	library := c.Library("movies", "Movies", "movie", "/movies")
	family := c.Movie(library, "/movies/family.mkv", "Family", 2000)
	adult := c.Movie(library, "/movies/adult.mkv", "Adult", 2000)
	unrated := c.Movie(library, "/movies/unknown.mkv", "Unknown", 2000)
	c.Attributes(family.ID, "contentRating", "G")
	c.Attributes(adult.ID, "contentRating", "R")
	c.Drain()
	// The account limit is judged by the same classified ages a profile's is;
	// a spelling the background worker has not classified yet is withheld.
	if _, err := f.deps.Catalog.ClassifyPendingRatings(context.Background()); err != nil {
		t.Fatal(err)
	}
	limits := access.DefaultLimits()
	limits.MaxContentRating = "PG"
	limits.AllowUnrated = false
	current, err := f.deps.Access.Access.Limits(context.Background(), nil, "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.deps.Access.Access.SetLimits(context.Background(), nil, "member", access.LimitsChange{ExpectedRevision: current.Revision, OperationID: "member-catalog-limit", Limits: limits}); err != nil {
		t.Fatal(err)
	}
	p := identity.Principal{Viewer: identity.Viewer{AccountID: "member", ProfileID: "member-profile", Authority: "local", Role: "member"}}
	request := httptest.NewRequest("GET", "/v1/items?libraryId=movies", nil)
	viewer, err := f.deps.catalogViewer(request, p, []string{"movies"}, "member-fence")
	if err != nil {
		t.Fatal(err)
	}
	items, _, err := f.deps.Catalog.List(viewer, "movies", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != family.Public {
		t.Fatalf("member ceiling not applied to page: %+v", items)
	}
	for _, id := range []string{adult.Public, unrated.Public} {
		if err := f.deps.Catalog.VisibleItem(context.Background(), viewer, id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("member item %s: %v", id, err)
		}
	}
	before := catalog.RestrictionFence(viewer.EffectiveRestrictions())
	limits.AllowUnrated = true
	if _, err := f.deps.Access.Access.SetLimits(context.Background(), nil, "member", access.LimitsChange{ExpectedRevision: current.Revision + 1, OperationID: "member-catalog-limit-2", Limits: limits}); err != nil {
		t.Fatal(err)
	}
	viewer, err = f.deps.catalogViewer(request, p, []string{"movies"}, "member-fence")
	if err != nil || catalog.RestrictionFence(viewer.EffectiveRestrictions()) == before {
		t.Fatalf("member limit change did not fence catalogue: %v", err)
	}
}

// The admin tier is the whole point of this workstream, so the boundary is
// asserted route by route rather than once.
func TestAdministrationTierBoundaries(t *testing.T) {
	f := administrationFixtureNew(t)
	adminTier := []string{"/v1/admin/access/members", "/v1/admin/access/invitations", "/v1/admin/access/devices",
		"/v1/admin/logs", "/v1/admin/logs/settings", "/v1/admin/connectivity/status"}
	for _, path := range adminTier {
		if w := f.call("GET", path, f.admin, ""); w.Code != 200 {
			t.Fatalf("admin tier refused %s: %d %s", path, w.Code, w.Body)
		}
		for _, token := range []string{"", f.member} {
			if w := f.call("GET", path, token, ""); w.Code != tl6RefusedStatus(token) {
				t.Fatalf("%s admitted a member: %d", path, w.Code)
			}
		}
	}
	ownerOnly := []string{"/v1/admin/diagnostics/client-logs", "/v1/admin/access/api-keys", "/v1/admin/connectivity/policy"}
	for _, path := range ownerOnly {
		if w := f.call("GET", path, f.owner, ""); w.Code != 200 {
			t.Fatalf("owner refused %s: %d %s", path, w.Code, w.Body)
		}
		for _, token := range []string{"", f.member, f.admin} {
			if w := f.call("GET", path, token, ""); w.Code != tl6RefusedStatus(token) {
				t.Fatalf("%s admitted a non-owner: %d", path, w.Code)
			}
		}
	}
}

// The tier is re-read from the live membership row, so demoting an account
// invalidates a token minted while it was an administrator.
func TestDemotionInvalidatesAnAdministrativeTokenImmediately(t *testing.T) {
	f := administrationFixtureNew(t)
	if w := f.call("GET", "/v1/admin/access/members", f.admin, ""); w.Code != 200 {
		t.Fatalf("setup: %d", w.Code)
	}
	if _, e := f.deps.DB.Exec(`UPDATE direct_memberships SET role='member' WHERE account_id='admin'`); e != nil {
		t.Fatal(e)
	}
	if w := f.call("GET", "/v1/admin/access/members", f.admin, ""); w.Code != 401 {
		t.Fatalf("a demoted account kept its administrative token: %d", w.Code)
	}
}

func TestMemberLimitsAndRoleRoutesCarryRevisionsAndRefuseAdminPeers(t *testing.T) {
	f := administrationFixtureNew(t)
	w := f.call("GET", "/v1/admin/access/members/member/limits", f.admin, "")
	if w.Code != 200 {
		t.Fatalf("read: %d %s", w.Code, w.Body)
	}
	var limits access.LimitsDocument
	if e := json.Unmarshal(w.Body.Bytes(), &limits); e != nil || limits.Revision != 1 {
		t.Fatalf("limits: %v %+v", e, limits)
	}
	body := `{"expectedRevision":1,"operationId":"limits-route-1","limits":{"maxStreams":1,"remoteBitrateKbps":4000,"maxContentRating":"PG-13","allowUnrated":true,"schedule":{"timezone":"UTC","windows":[]},"channelPolicy":{"mode":"all","channels":[]},"tagPolicy":{"deniedLabels":[]}}}`
	if w = f.call("PUT", "/v1/admin/access/members/member/limits", f.admin, body); w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body)
	}
	// The same body again is a stale write and must name the current revision.
	w = f.call("PUT", "/v1/admin/access/members/member/limits", f.admin, strings.Replace(body, "limits-route-1", "limits-route-2", 1))
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"currentRevision":2`) {
		t.Fatalf("conflict: %d %s", w.Code, w.Body)
	}
	// An admin may promote nobody to admin. The admin's own session is fine,
	// so the answer is a refusal (403), never "sign in again" (401).
	if w = f.call("PUT", "/v1/admin/access/members/member/role", f.admin, `{"expectedRevision":1,"operationId":"role-route-1","role":"admin"}`); w.Code != 403 {
		t.Fatalf("an admin promoted an account to admin: %d %s", w.Code, w.Body)
	}
	if w = f.call("PUT", "/v1/admin/access/members/member/role", f.owner, `{"expectedRevision":1,"operationId":"role-route-2","role":"admin"}`); w.Code != 200 {
		t.Fatalf("owner promotion: %d %s", w.Code, w.Body)
	}
}

// Channel allow/deny lists take canonical v1 ids only: anything else is
// invalid_request naming limits.channelPolicy.channels.
func TestMemberLimitsChannelPolicyRejectsNonCanonicalIDs(t *testing.T) {
	f := administrationFixtureNew(t)
	for _, id := range []string{"library:news", "live:source:ch"} {
		body := `{"expectedRevision":1,"operationId":"canon-ok-` + id + `","limits":{"maxStreams":0,"remoteBitrateKbps":0,"maxContentRating":"","allowUnrated":true,"schedule":{"timezone":"","windows":[]},"channelPolicy":{"mode":"deny","channels":["` + id + `"]},"tagPolicy":{"deniedLabels":[]}}}`
		// Fresh fixture per id would be cleaner, but revisions bump: read first.
		w := f.call("GET", "/v1/admin/access/members/member/limits", f.admin, "")
		var doc access.LimitsDocument
		_ = json.Unmarshal(w.Body.Bytes(), &doc)
		body = strings.Replace(body, `"expectedRevision":1`, `"expectedRevision":`+strconv.FormatInt(doc.Revision, 10), 1)
		if w = f.call("PUT", "/v1/admin/access/members/member/limits", f.admin, body); w.Code != 200 {
			t.Fatalf("canonical %q was rejected: %d %s", id, w.Code, w.Body.String())
		}
	}
	for _, id := range []string{"news", "live:onlyone", "library:", "live:a:b:c"} {
		w := f.call("GET", "/v1/admin/access/members/member/limits", f.admin, "")
		var doc access.LimitsDocument
		_ = json.Unmarshal(w.Body.Bytes(), &doc)
		body := `{"expectedRevision":` + strconv.FormatInt(doc.Revision, 10) + `,"operationId":"canon-bad-` + id + `-00000000","limits":{"maxStreams":0,"remoteBitrateKbps":0,"maxContentRating":"","allowUnrated":true,"schedule":{"timezone":"","windows":[]},"channelPolicy":{"mode":"deny","channels":["` + id + `"]},"tagPolicy":{"deniedLabels":[]}}}`
		w = f.call("PUT", "/v1/admin/access/members/member/limits", f.admin, body)
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) || !strings.Contains(w.Body.String(), "limits.channelPolicy.channels") {
			t.Fatalf("non-canonical %q: %d %s", id, w.Code, w.Body.String())
		}
	}
}

func TestInvitationRouteIssuesACodeOnceAndAcceptanceCreatesTheAccount(t *testing.T) {
	f := administrationFixtureNew(t)
	w := f.call("POST", "/v1/admin/access/invitations", f.admin, `{"operationId":"invite-route-1","email":"guest@example.com","role":"member","allowedLibraries":[],"expiresInHours":24}`)
	if w.Code != 201 {
		t.Fatalf("invite: %d %s", w.Code, w.Body)
	}
	var invitation access.Invitation
	if e := json.Unmarshal(w.Body.Bytes(), &invitation); e != nil || invitation.Code == "" {
		t.Fatalf("invitation: %v %+v", e, invitation)
	}
	if w = f.call("GET", "/v1/admin/access/invitations", f.admin, ""); !strings.Contains(w.Body.String(), `"state":"pending"`) || strings.Contains(w.Body.String(), invitation.Code) {
		t.Fatalf("a list leaked the code or lost the row: %s", w.Body)
	}
	// The acceptance route is unauthenticated on purpose.
	body := `{"code":"` + invitation.Code + `","username":"guest","password":"Correct-Horse-9","name":"Guest"}`
	if w = f.call("POST", "/v1/access/invitations/accept", "", body); w.Code != 201 {
		t.Fatalf("accept: %d %s", w.Code, w.Body)
	}
	if w = f.call("POST", "/v1/access/invitations/accept", "", body); w.Code != 410 {
		t.Fatalf("a redeemed code was accepted again: %d %s", w.Code, w.Body)
	}
	if w = f.call("POST", "/v1/access/invitations/accept", "", `{"code":"`+invitation.Code+`","username":"guest3","password":"weak"}`); w.Code != 400 {
		t.Fatalf("a weak password was accepted: %d %s", w.Code, w.Body)
	}
}

func TestInvitationPreviewIsOpaqueAndSharesAcceptBudget(t *testing.T) {
	f := administrationFixtureNew(t)
	w := f.call("POST", "/v1/admin/access/invitations", f.owner, `{"operationId":"preview-route-1","email":"justin@example.com","role":"member","allowedLibraries":["private-lib"],"expiresInHours":24}`)
	if w.Code != 201 {
		t.Fatalf("invite: %d %s", w.Code, w.Body)
	}
	var invite access.Invitation
	if err := json.Unmarshal(w.Body.Bytes(), &invite); err != nil {
		t.Fatal(err)
	}
	body := `{"code":"` + invite.Code + `"}`
	w = f.call("POST", "/v1/access/invitations/preview", "", body)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	var preview access.InvitationPreview
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil || preview.ServerName != f.deps.Identity.Name() || preview.Email != "j***@example.com" || preview.LibraryCount != 1 || preview.ExpiresAt != invite.ExpiresAt || preview.Role != "member" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if strings.Contains(w.Body.String(), "private-lib") || strings.Contains(w.Body.String(), "justin@example.com") {
		t.Fatalf("preview leaked private detail: %s", w.Body)
	}
	if os.Getenv("PORTICO_CONTRACT_UPDATE") == "1" {
		if err := contract.Record("../../testdata/contracts/preview-invitation.json", "PreviewInvitationResponse", "POST", "/v1/access/invitations/preview", []byte(`{"code":"example-invitation-code"}`), w); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{"wrong", ""} {
		w = f.call("POST", "/v1/access/invitations/preview", "", `{"code":"`+invalid+`"}`)
		if w.Code != 404 || errorCode(t, w) != "invitation_not_found" {
			t.Fatalf("invalid preview: %d %s", w.Code, w.Body)
		}
	}
	if _, err := f.deps.DB.Exec(`UPDATE access_invitations SET expires_ms=0 WHERE id=?`, invite.ID); err != nil {
		t.Fatal(err)
	}
	w = f.call("POST", "/v1/access/invitations/preview", "", body)
	if w.Code != 404 || errorCode(t, w) != "invitation_not_found" {
		t.Fatalf("expired preview: %d %s", w.Code, w.Body)
	}
	if _, err := f.deps.DB.Exec(`UPDATE access_invitations SET expires_ms=?,state='accepted' WHERE id=?`, time.Now().Add(time.Hour).UnixMilli(), invite.ID); err != nil {
		t.Fatal(err)
	}
	w = f.call("POST", "/v1/access/invitations/preview", "", body)
	if w.Code != 404 || errorCode(t, w) != "invitation_not_found" {
		t.Fatalf("used preview: %d %s", w.Code, w.Body)
	}
	for i := 0; i < 5; i++ {
		_ = f.call("POST", "/v1/access/invitations/preview", "", `{"code":"wrong"}`)
	}
	w = f.call("POST", "/v1/access/invitations/accept", "", `{"code":"`+invite.Code+`","username":"guest","password":"Correct-Horse-9"}`)
	if w.Code != 429 || errorCode(t, w) != "rate_limited" {
		t.Fatalf("preview did not spend accept budget: %d %s", w.Code, w.Body)
	}
}

func TestMessageLogReadsFilterStreamAndOpenADebugWindow(t *testing.T) {
	f := administrationFixtureNew(t)
	f.recorder.Record("error", "playback", "the encoder stalled")
	f.recorder.Record("debug", "scan", "walking a directory")
	w := f.call("GET", "/v1/admin/logs?level=warn", f.admin, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "the encoder stalled") || strings.Contains(w.Body.String(), "walking a directory") {
		t.Fatalf("level filter: %d %s", w.Code, w.Body)
	}
	if w = f.call("GET", "/v1/admin/logs?level=nonsense", f.admin, ""); w.Code != 400 {
		t.Fatalf("an unknown level was accepted: %d", w.Code)
	}
	if w = f.call("GET", "/v1/admin/logs?category=playback&level=debug", f.admin, ""); strings.Contains(w.Body.String(), "walking a directory") {
		t.Fatalf("category filter: %s", w.Body)
	}
	// The debug window raises the effective level without moving the settings
	// revision, which is what makes it runtime state rather than a setting.
	before := f.call("GET", "/v1/admin/logs/settings", f.admin, "").Body.String()
	w = f.call("POST", "/v1/admin/logs/debug-window", f.admin, `{"minutes":5,"operationId":"debug-window-1"}`)
	if w.Code != 200 {
		t.Fatalf("debug window: %d %s", w.Code, w.Body)
	}
	var settings LogSettings
	if e := json.Unmarshal(w.Body.Bytes(), &settings); e != nil || settings.EffectiveLevel != "debug" || settings.DebugUntil == "" {
		t.Fatalf("settings: %v %+v", e, settings)
	}
	var earlier LogSettings
	if e := json.Unmarshal([]byte(before), &earlier); e != nil || earlier.Revision != settings.Revision {
		t.Fatalf("a debug window moved the settings revision: %v %d %d", e, earlier.Revision, settings.Revision)
	}
	if w = f.call("POST", "/v1/admin/logs/debug-window", f.admin, `{"minutes":9999,"operationId":"debug-window-2"}`); w.Code != 400 {
		t.Fatalf("an unbounded window was accepted: %d", w.Code)
	}
}

func TestLogSettingsWriteIsOwnerOnlyAndTakesEffectWithoutARestart(t *testing.T) {
	f := administrationFixtureNew(t)
	read := f.call("GET", "/v1/admin/logs/settings", f.admin, "")
	var settings LogSettings
	if e := json.Unmarshal(read.Body.Bytes(), &settings); e != nil {
		t.Fatal(e)
	}
	body := `{"expectedRevision":` + tl6Itoa(settings.Revision) + `,"operationId":"log-settings-1","logLevel":"error","retention":[{"category":"scan","days":2}]}`
	if w := f.call("PATCH", "/v1/admin/logs/settings", f.admin, body); w.Code != 403 {
		t.Fatalf("an admin wrote the log settings: %d %s", w.Code, w.Body)
	}
	w := f.call("PATCH", "/v1/admin/logs/settings", f.owner, body)
	if w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body)
	}
	if level, _ := f.recorder.Effective(); level != "error" {
		t.Fatalf("the recorder did not adopt the saved level: %q", level)
	}
	f.recorder.Record("info", "server", "should be dropped")
	if page := f.call("GET", "/v1/admin/logs?level=debug", f.admin, ""); strings.Contains(page.Body.String(), "should be dropped") {
		t.Fatalf("a record below the new level was kept: %s", page.Body)
	}
	if w = f.call("PATCH", "/v1/admin/logs/settings", f.owner, body); w.Code != 409 {
		t.Fatalf("a stale settings write was accepted: %d %s", w.Code, w.Body)
	}
}

func TestClientLogUploadsAreBoundedStoredPerDeviceAndListedForAdmins(t *testing.T) {
	f := administrationFixtureNew(t)
	w := f.call("POST", "/v1/diagnostics/client-logs", f.member, `{"deviceId":"living-room","platform":"tvos","appVersion":"1.0","body":"a client trace"}`)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	if w = f.call("POST", "/v1/diagnostics/client-logs", f.member, `{"body":""}`); w.Code != 400 {
		t.Fatalf("an empty body was accepted: %d", w.Code)
	}
	oversized := `{"body":"` + strings.Repeat("x", servicelog.MaxClientUploadBytes+1) + `"}`
	if w = f.call("POST", "/v1/diagnostics/client-logs", f.member, oversized); w.Code != 400 {
		t.Fatalf("an oversized upload was accepted: %d", w.Code)
	}
	if w = f.call("POST", "/v1/diagnostics/client-logs", "", `{"body":"anonymous"}`); w.Code != 401 {
		t.Fatalf("an unauthenticated upload was accepted: %d", w.Code)
	}
	w = f.call("GET", f.owner, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"deviceId":"`+f.memberDevice+`"`) || strings.Contains(w.Body.String(), "a client trace") {
		t.Fatalf("a list must carry metadata but not bodies: %d %s", w.Code, w.Body)
	}
	var page servicelog.ClientUploadPage
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Items) != 1 {
		t.Fatalf("page: %v %+v", e, page)
	}
	w = f.call("GET", "/v1/admin/diagnostics/client-logs/"+page.Items[0].ID, f.owner, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "a client trace") {
		t.Fatalf("single read: %d %s", w.Code, w.Body)
	}
	if w = f.call("GET", "/v1/admin/diagnostics/client-logs/"+page.Items[0].ID, f.member, ""); w.Code != 403 {
		t.Fatalf("a member read another account's upload: %d", w.Code)
	}
}

func TestClientLogUploadsRedactAndEnforceAccountBudget(t *testing.T) {
	f := administrationFixtureNew(t)
	secret := strings.Repeat("Q", 48)
	body, _ := json.Marshal(servicelog.ClientUploadRequest{DeviceID: "spoofed", Platform: "tvos", Body: "Bearer " + secret + " GET /v1/media/asset?grant=" + secret})
	w := f.call("POST", "/v1/diagnostics/client-logs", f.member, string(body))
	if w.Code != 201 || strings.Contains(w.Body.String(), "spoofed") || !strings.Contains(w.Body.String(), f.memberDevice) {
		t.Fatalf("device binding: %d %s", w.Code, w.Body)
	}
	var uploaded servicelog.ClientUpload
	if err := json.Unmarshal(w.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	w = f.call("GET", "/v1/admin/diagnostics/client-logs/"+uploaded.ID, f.owner, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), secret) || !strings.Contains(w.Body.String(), "[REDACTED]") {
		t.Fatalf("unredacted body: %d %s", w.Code, w.Body)
	}
	chunk := strings.Repeat("x", 220<<10)
	for i := 0; i < 4; i++ {
		body, _ = json.Marshal(servicelog.ClientUploadRequest{Platform: "tvos", Body: chunk})
		if w = f.call("POST", "/v1/diagnostics/client-logs", f.member, string(body)); w.Code != 201 {
			t.Fatalf("budget upload %d: %d %s", i, w.Code, w.Body)
		}
	}
	if w = f.call("POST", "/v1/diagnostics/client-logs", f.member, string(body)); w.Code != 429 {
		t.Fatalf("account daily budget: %d %s", w.Code, w.Body)
	}
}

func TestClientLogRetentionIsPerAuthenticatedDevice(t *testing.T) {
	f := administrationFixtureNew(t)
	for i := 0; i < servicelog.ClientUploadsPerDevice+2; i++ {
		body, _ := json.Marshal(servicelog.ClientUploadRequest{DeviceID: "spoofed-" + strconv.Itoa(i), Platform: "tvos", Body: "trace " + strconv.Itoa(i)})
		if w := f.call("POST", "/v1/diagnostics/client-logs", f.member, string(body)); w.Code != 201 {
			t.Fatalf("upload %d: %d %s", i, w.Code, w.Body)
		}
	}
	w := f.call("GET", f.owner, "")
	var page servicelog.ClientUploadPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || len(page.Items) != servicelog.ClientUploadsPerDevice {
		t.Fatalf("device retention: %d %v %+v", w.Code, err, page)
	}
	for _, item := range page.Items {
		if item.DeviceID != f.memberDevice {
			t.Fatalf("body supplied a retention identity: %+v", item)
		}
	}
}

func TestDiagnosticsBundleIsOwnerOnlyAndIsAZip(t *testing.T) {
	f := administrationFixtureNew(t)
	if w := f.call("GET", "/v1/admin/diagnostics/bundle", f.admin, ""); w.Code != 403 {
		t.Fatalf("an admin exported the bundle: %d", w.Code)
	}
	w := f.call("GET", "/v1/admin/diagnostics/bundle", f.owner, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("bundle: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		t.Fatal("the body is not a zip archive")
	}
}

func TestConnectivityPolicyRoundTripsAndStatusReportsTheHost(t *testing.T) {
	f := administrationFixtureNew(t)
	w := f.call("GET", "/v1/admin/connectivity/policy", f.owner, "")
	var policy connectivity.Policy
	if e := json.Unmarshal(w.Body.Bytes(), &policy); e != nil || policy.RemoteSignIn != "allow" {
		t.Fatalf("defaults: %v %+v", e, policy)
	}
	body := `{"expectedRevision":` + tl6Itoa(policy.Revision) + `,"operationId":"connectivity-1","remoteSignInPolicy":"owner-only","remoteBitrateLimitKbps":8000,"lanNetworks":["192.168.1.5/24"],"accessUrls":["https://media.example.com/"],"lanDiscoveryEnabled":false}`
	w = f.call("PATCH", "/v1/admin/connectivity/policy", f.owner, body)
	if w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body)
	}
	if e := json.Unmarshal(w.Body.Bytes(), &policy); e != nil {
		t.Fatal(e)
	}
	if policy.RemoteSignIn != "owner-only" || policy.RemoteBitrateLimitKbps != 8000 || policy.LANDiscoveryEnabled {
		t.Fatalf("policy: %+v", policy)
	}
	// A network is stored masked, so a host address cannot masquerade as one.
	if len(policy.LANNetworks) != 1 || policy.LANNetworks[0] != "192.168.1.0/24" {
		t.Fatalf("lanNetworks: %+v", policy.LANNetworks)
	}
	if len(policy.AccessURLs) != 1 || policy.AccessURLs[0] != "https://media.example.com" {
		t.Fatalf("accessUrls: %+v", policy.AccessURLs)
	}
	bad := `{"expectedRevision":` + tl6Itoa(policy.Revision) + `,"operationId":"connectivity-2","lanNetworks":["not-a-network"]}`
	if w = f.call("PATCH", "/v1/admin/connectivity/policy", f.owner, bad); w.Code != 400 || !strings.Contains(w.Body.String(), "lanNetworks") {
		t.Fatalf("an invalid network was accepted: %d %s", w.Code, w.Body)
	}
	w = f.call("GET", "/v1/admin/connectivity/status", f.admin, "")
	if w.Code != 200 {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
	var status connectivity.Status
	if e := json.Unmarshal(w.Body.Bytes(), &status); e != nil {
		t.Fatal(e)
	}
	if status.Policy.RemoteSignIn != "owner-only" || status.ObservedAt == "" || status.Warnings == nil {
		t.Fatalf("report: %+v", status)
	}
	// The server-wide remote bitrate ceiling reaches the delivery seam once
	// the snapshot is refreshed, which a settings save triggers in production.
	adapter := &RegistryDeliverySettings{Console: f.deps.Console}
	adapter.Refresh(context.Background())
	delivery := adapter.Delivery()
	if delivery.MaxVideoBitrateBPS != 8_000_000 {
		t.Fatalf("the remote bitrate ceiling did not reach delivery: %d", delivery.MaxVideoBitrateBPS)
	}
}

func TestLiveLogTailStreamsEvents(t *testing.T) {
	f := administrationFixtureNew(t)
	r := httptest.NewRequest("GET", "/v1/admin/logs/events?level=info", nil)
	r.Header.Set("Authorization", "Bearer "+f.admin)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// The handler writes frames from its own goroutine while this one reads them,
	// so the recorder has to be safe for both. httptest.ResponseRecorder is not.
	w := &streamRecorder{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() {
		f.handler.ServeHTTP(w, r.WithContext(ctx))
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.recorder.Record("error", "server", "tailed line")
		if strings.Contains(w.body(), "tailed line") {
			cancel()
			<-done
			if !strings.Contains(w.body(), "event: message") {
				t.Fatalf("stream framing: %s", w.body())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("no event arrived: %s", w.body())
}

// streamRecorder is an httptest.ResponseRecorder a test may read while the
// handler is still writing to it.
type streamRecorder struct {
	mu sync.Mutex
	*httptest.ResponseRecorder
}

func (s *streamRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ResponseRecorder.Write(p)
}

func (s *streamRecorder) WriteString(value string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ResponseRecorder.WriteString(value)
}

func (s *streamRecorder) WriteHeader(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResponseRecorder.WriteHeader(code)
}

func (s *streamRecorder) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ResponseRecorder.Flush()
}

func (s *streamRecorder) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ResponseRecorder.Body.String()
}

func TestOwnerCanDownloadAndDeleteClientReport(t *testing.T) {
	f := administrationFixtureNew(t)
	uploaded := f.call("POST", "/v1/diagnostics/client-logs", f.member, `{"platform":"android","deviceClass":"phone","appVersion":"1.2.3","body":"Bearer private-secret"}`)
	if uploaded.Code != 201 {
		t.Fatal(uploaded.Code, uploaded.Body.String())
	}
	var report servicelog.ClientUpload
	if err := json.Unmarshal(uploaded.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.DeviceClass != "phone" {
		t.Fatal(report)
	}
	path := "/v1/admin/diagnostics/client-logs/" + report.ID
	if response := f.call("GET", path+"/download", f.admin, ""); response.Code != 403 {
		t.Fatal("admin read owner report", response.Code)
	}
	downloaded := f.call("GET", path+"/download", f.owner, "")
	if downloaded.Code != 200 || !strings.Contains(downloaded.Body.String(), "REDACTED") || downloaded.Header().Get("Content-Disposition") == "" || downloaded.Header().Get("Content-Type") != "application/json" {
		t.Fatal(downloaded.Code, downloaded.Body.String())
	}
	if removed := f.call("DELETE", path, f.owner, ""); removed.Code != 200 {
		t.Fatal(removed.Code, removed.Body.String())
	}
	if response := f.call("GET", path, f.owner, ""); response.Code != 404 {
		t.Fatal("deleted report remained", response.Code)
	}
}

func TestClientReportsPageKeepsTimestampTiesAndGlobalBound(t *testing.T) {
	f := administrationFixtureNew(t)
	ctx := context.Background()
	for n := 0; n < 105; n++ {
		id := fmt.Sprintf("%022d", n)
		if _, err := f.deps.DB.Exec(`INSERT INTO client_log_uploads(id,account_id,profile_id,device_id,platform,app_version,device_class,received_ms,bytes,body) VALUES(?,?,?,?,?,?,?,1000,1,'x')`, id, "other", "other", id, "test", "1", "phone"); err != nil {
			t.Fatal(err)
		}
	}
	// One signed-in upload enforces the global cap, independently of device retention.
	response := f.call("POST", "/v1/diagnostics/client-logs", f.member, `{"platform":"test","body":"new"}`)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var count int
	f.deps.DB.QueryRow(`SELECT count(*) FROM client_log_uploads`).Scan(&count)
	if count != 100 {
		t.Fatal("global bound", count)
	}
	store := f.deps.Access.ClientLogs
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := store.List(ctx, cursor, 25)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate", item.ID)
			}
			seen[item.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 100 {
		t.Fatal("timestamp ties lost", len(seen))
	}
}

func TestClientReportRedactionCannotExpandPastStorageLimit(t *testing.T) {
	f := administrationFixtureNew(t)
	body, _ := json.Marshal(servicelog.ClientUploadRequest{Body: strings.Repeat("Bearer x ", servicelog.MaxClientUploadBytes/9), DeviceClass: "phone"})
	response := f.call("POST", "/v1/diagnostics/client-logs", f.member, string(body))
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	var report servicelog.ClientUpload
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	stored, err := f.deps.Access.ClientLogs.Read(context.Background(), report.ID)
	if err != nil || len(stored.Body) > servicelog.MaxClientUploadBytes || stored.Bytes != int64(len(stored.Body)) || strings.Contains(stored.Body, "Bearer x") {
		t.Fatal(stored.Bytes, len(stored.Body), err)
	}
}
