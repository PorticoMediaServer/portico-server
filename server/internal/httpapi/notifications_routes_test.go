package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/notify"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
)

func notificationFixture(t *testing.T) (Dependencies, *http.ServeMux, identity.Envelope, identity.Envelope) {
	t.Helper()
	root := t.TempDir()
	db, e := persistence.Open(filepath.Join(root, "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	id, e := identity.New(db, root)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','owner-profile',1);INSERT INTO accounts VALUES('member','member',x'00','member-profile',1)`); e != nil {
		t.Fatal(e)
	}
	owner, e := id.Issue("owner", "owner-profile", "local", "owner", 1)
	if e != nil {
		t.Fatal(e)
	}
	member, e := id.Issue("member", "member-profile", "local", "member", 1)
	if e != nil {
		t.Fatal(e)
	}
	d := Dependencies{DB: db, Identity: id, Console: operations.New(db)}
	mux := http.NewServeMux()
	d.notificationRoutes(mux)
	return d, mux, owner, member
}

func notificationRequest(h http.Handler, method, token, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, path, reader)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func raiseNotice(t *testing.T, d Dependencies, scope, key, title string) notify.Result {
	t.Helper()
	tx, e := d.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	out, e := notify.Raise(tx, time.Now().UnixMilli(), notify.Draft{
		Audience: notify.AudienceProfile, Scope: scope, Severity: notify.SeverityInfo,
		Source: notify.SourceDownloads, Category: "download.finished", DedupeKey: key,
		Title: title, Body: "A title finished downloading.",
	})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return out
}

// The documented surface: the inbox document, the cheap poll, and the single
// batch write, all bounded, all no-store, all requiring a session.
func TestNotificationRoutesDocumentAndBatch(t *testing.T) {
	d, mux, owner, member := notificationFixture(t)
	scope := notify.ProfileScope("local", "member", "member-profile")
	first := raiseNotice(t, d, scope, "download:one", "Download ready")
	raiseNotice(t, d, scope, "download:two", "Second download ready")

	for _, path := range []string{"/v1/notifications/inbox", "/v1/notifications/unread-count", "/v1/notifications/capabilities"} {
		if w := notificationRequest(mux, "GET", "", path, ""); w.Code != 401 {
			t.Fatal("anonymous", path, w.Code)
		}
	}
	w := notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/inbox", "")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Length") == "" {
		t.Fatal("inbox", w.Code, w.Body.String())
	}
	var document struct {
		Scope struct {
			ServerID string `json:"serverId"`
			Fence    string `json:"viewerFence"`
		}
		Data operations.NoticeInbox
	}
	if e := json.Unmarshal(w.Body.Bytes(), &document); e != nil {
		t.Fatal(e, w.Body.String())
	}
	if document.Scope.ServerID != d.Identity.ID() || len(document.Scope.Fence) != 64 {
		t.Fatalf("scope: %+v", document.Scope)
	}
	if len(document.Data.Items) != 2 || document.Data.Counts.Unread != 2 || document.Data.Revision == 0 {
		t.Fatalf("inbox document: %+v", document.Data)
	}
	// One document: the page, the counts, the revision and the retention window
	// all arrive together, so an inbox needs no second request to render.
	if document.Data.RetentionDays != 180 || document.Data.State != "all" {
		t.Fatalf("inbox document is incomplete: %+v", document.Data)
	}
	if w = notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/inbox?state=archived", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("archived filter", w.Body.String())
	}
	for _, bad := range []string{"?state=nonsense", "?audience=nonsense", "?limit=0", "?limit=500", "?unknown=1"} {
		if w = notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/inbox"+bad, ""); w.Code != 400 {
			t.Fatal("validation", bad, w.Code)
		}
	}
	// A member asking for the administrator audience is refused, not filtered.
	if w = notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/inbox?audience=account-admin", ""); w.Code != 401 {
		t.Fatal("audience", w.Code, w.Body.String())
	}
	body := `{"operationId":"batch-one","expectedRevision":` + itoa(document.Data.Revision) + `,"audience":"profile","operations":[{"action":"read","ids":["` + first.ID + `"]}]}`
	w = notificationRequest(mux, "POST", member.AccessToken, "/v1/notifications/inbox/actions", body)
	if w.Code != 200 {
		t.Fatal("batch", w.Code, w.Body.String())
	}
	var applied struct{ Data operations.NoticeBatchResult }
	if e := json.Unmarshal(w.Body.Bytes(), &applied); e != nil || applied.Data.Applied != 1 || applied.Data.Counts.Unread != 1 {
		t.Fatalf("batch result: %v %+v", e, applied.Data)
	}
	// A stale fence is a 409 that names the revision to refresh to.
	w = notificationRequest(mux, "POST", member.AccessToken, "/v1/notifications/inbox/actions", body)
	if w.Code != 200 {
		t.Fatal("replay must return the recorded answer", w.Code, w.Body.String())
	}
	stale := strings.Replace(body, "batch-one", "batch-two", 1)
	w = notificationRequest(mux, "POST", member.AccessToken, "/v1/notifications/inbox/actions", stale)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "currentRevision") {
		t.Fatal("fence", w.Code, w.Body.String())
	}
	// An unknown body field is refused rather than silently ignored.
	if w = notificationRequest(mux, "POST", member.AccessToken, "/v1/notifications/inbox/actions",
		`{"operationId":"batch-three","operations":[{"action":"read","ids":["x"]}],"surprise":true}`); w.Code != 400 {
		t.Fatal("strict body", w.Code, w.Body.String())
	}
	// Owner-only routes reject a member session.
	for _, path := range []string{"/v1/admin/notifications/settings"} {
		if w = notificationRequest(mux, "GET", member.AccessToken, path, ""); w.Code != 403 {
			t.Fatal("owner boundary", path, w.Code)
		}
		if w = notificationRequest(mux, "GET", owner.AccessToken, path, ""); w.Code != 200 {
			t.Fatal("owner", path, w.Code, w.Body.String())
		}
	}
	w = notificationRequest(mux, "POST", owner.AccessToken, "/v1/admin/notifications/broadcast",
		`{"operationId":"b-1","audience":"profile","severity":"info","dedupeKey":"notice-one","title":"Scheduled restart","body":"Tonight at 02:00.","actions":[],"expiresInDays":1}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"delivered":2`) {
		t.Fatal("broadcast", w.Code, w.Body.String())
	}
}

// The event stream: a hello frame with the revision, an id on every frame, and
// a Last-Event-ID resume that replays exactly what changed.
func TestNotificationEventStreamResume(t *testing.T) {
	d, mux, _, member := notificationFixture(t)
	scope := notify.ProfileScope("local", "member", "member-profile")
	first := raiseNotice(t, d, scope, "download:one", "Download ready")

	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/v1/notifications/events", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+member.AccessToken)
	w := httptest.NewRecorder()
	// The handler returns when the request context is cancelled; cancel as soon
	// as the first frames are written so the test does not wait on a heartbeat.
	go func() {
		time.Sleep(400 * time.Millisecond)
		cancel()
	}()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatal("stream", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: hello") || !strings.Contains(body, "id: ") {
		t.Fatal("hello frame", body)
	}
	if !strings.Contains(body, `"heartbeatSeconds":20`) {
		t.Fatal("heartbeat interval must be published on the stream", body)
	}
	if !strings.Contains(body, "event: receiver.inbox_changed\ndata: {\"kind\":\"resync\"}") {
		t.Fatal("missing receiver reconnect hint", body)
	}
	if routeLanes["GET /v1/notifications/wait"] != laneRealtime {
		t.Fatal("long poll is not in realtime lane")
	}
	// Resuming from the revision the client already saw replays the record that
	// changed after it, and nothing else.
	second := raiseNotice(t, d, scope, "download:two", "Second download ready")
	ctx, cancel = context.WithCancel(context.Background())
	r = httptest.NewRequest("GET", "/v1/notifications/events", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+member.AccessToken)
	r.Header.Set("Last-Event-ID", itoa(first.Revision))
	w = httptest.NewRecorder()
	go func() {
		time.Sleep(400 * time.Millisecond)
		cancel()
	}()
	mux.ServeHTTP(w, r)
	body = w.Body.String()
	if !strings.Contains(body, "event: change") || !strings.Contains(body, second.ID) {
		t.Fatal("resume did not replay", body)
	}
	if strings.Contains(body, first.ID) {
		t.Fatal("resume replayed a record the client already had", body)
	}
	if !strings.Contains(body, "id: "+itoa(second.Revision)) {
		t.Fatal("every frame must carry the revision as its id", body)
	}
	// An unauthenticated stream is a JSON error, not an event stream.
	w = notificationRequest(mux, "GET", "", "/v1/notifications/events", "")
	if w.Code != 401 || strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatal("anonymous stream", w.Code, w.Header().Get("Content-Type"))
	}
}

// The long-poll variant answers immediately when the client is behind and parks
// otherwise, with the same revision contract as the stream.
func TestNotificationLongPollRoute(t *testing.T) {
	d, mux, _, member := notificationFixture(t)
	scope := notify.ProfileScope("local", "member", "member-profile")
	first := raiseNotice(t, d, scope, "download:one", "Download ready")
	second := raiseNotice(t, d, scope, "download:two", "Second download ready")

	w := notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/wait?revision="+itoa(first.Revision), "")
	if w.Code != 200 {
		t.Fatal("wait", w.Code, w.Body.String())
	}
	var out struct{ Data operations.NoticeDelta }
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || out.Data.Revision != second.Revision || len(out.Data.Items) != 1 {
		t.Fatalf("wait payload: %v %+v", e, out.Data)
	}
	start := time.Now()
	w = notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/wait?revision="+itoa(second.Revision)+"&waitSeconds=1", "")
	if w.Code != 200 || time.Since(start) < 900*time.Millisecond {
		t.Fatal("an up-to-date client must park for the requested wait", w.Code, time.Since(start))
	}
	for _, bad := range []string{"?waitSeconds=600", "?revision=-1", "?nonsense=1"} {
		if w = notificationRequest(mux, "GET", member.AccessToken, "/v1/notifications/wait"+bad, ""); w.Code != 400 {
			t.Fatal("wait validation", bad, w.Code)
		}
	}
}

// Feedback over HTTP: capabilities before the form, a submit that recognises a
// repeat, and an administrative list whose counts arrive with the page.
func TestFeedbackRoutes(t *testing.T) {
	_, mux, owner, member := notificationFixture(t)
	w := notificationRequest(mux, "GET", member.AccessToken, "/v1/feedback/capabilities", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"canSubmit":true`) {
		t.Fatal("capabilities", w.Code, w.Body.String())
	}
	submit := `{"operationId":"r-1","kind":"playback","category":"buffering","message":"Playback keeps pausing every minute.","itemId":"","playbackSessionId":"","attachDiagnostics":false}`
	w = notificationRequest(mux, "POST", member.AccessToken, "/v1/feedback/reports", submit)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"created":true`) {
		t.Fatal("submit", w.Code, w.Body.String())
	}
	var submitted struct {
		Data operations.FeedbackSubmissionResult
	}
	if e := json.Unmarshal(w.Body.Bytes(), &submitted); e != nil {
		t.Fatal(e)
	}
	repeat := strings.Replace(submit, "r-1", "r-2", 1)
	w = notificationRequest(mux, "POST", member.AccessToken, "/v1/feedback/reports", repeat)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatal("duplicate", w.Code, w.Body.String())
	}
	// The admin list is owner-only and carries its status counts.
	if w = notificationRequest(mux, "GET", member.AccessToken, "/v1/admin/feedback/reports", ""); w.Code != 403 {
		t.Fatal("owner boundary", w.Code)
	}
	w = notificationRequest(mux, "GET", owner.AccessToken, "/v1/admin/feedback/reports", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"statusCounts"`) {
		t.Fatal("admin list", w.Code, w.Body.String())
	}
	transition := `{"operationId":"t-1","expectedRevision":` + itoa(submitted.Data.Report.Revision) + `,"status":"resolved","reply":"Fixed the source."}`
	w = notificationRequest(mux, "POST", owner.AccessToken, "/v1/admin/feedback/reports/"+submitted.Data.Report.ID+"/status", transition)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"resolved"`) {
		t.Fatal("transition", w.Code, w.Body.String())
	}
	// The reporter sees the thread and their own report only.
	w = notificationRequest(mux, "GET", member.AccessToken, "/v1/feedback/reports/"+submitted.Data.Report.ID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Fixed the source.") {
		t.Fatal("reporter thread", w.Code, w.Body.String())
	}
	// And the submission notified the administrators.
	w = notificationRequest(mux, "GET", owner.AccessToken, "/v1/notifications/inbox?audience=account-admin", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "feedback.received") {
		t.Fatal("admin notice", w.Code, w.Body.String())
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	out := ""
	for v > 0 {
		out = string(rune('0'+v%10)) + out
		v /= 10
	}
	if negative {
		return "-" + out
	}
	return out
}
