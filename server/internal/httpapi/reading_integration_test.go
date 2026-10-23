package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ingestion"
	"portico.local/server/internal/reading"
	"portico.local/server/internal/readingtest"
	"portico.local/server/internal/recordingaccess"
)

type readingFixture struct {
	f                           *v1Fixture
	d                           Dependencies
	s                           *reading.Service
	p                           identity.Principal
	library, item, source, path string
}

// The scan, identity, permissions, schema, helper and filesystem are real. No
// replacement Reading tables, permissive authorizer or playback source trick
// is installed. This private-boundary check does not claim generated API/UI
// composition; that is checked separately after canonical contracts land.
func newReadingFixture(t *testing.T, kind, format string) *readingFixture {
	t.Helper()
	f := newV1Fixture(t, 0)
	root := t.TempDir()
	name := "Original publication." + format
	var raw []byte
	switch format {
	case "epub":
		raw = readingtest.EPUB()
	case "pdf":
		raw = readingtest.PDF()
	case "cbz":
		raw = readingtest.CBZ()
	}
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	f.cat.SetStorage(f.deps.Storage)
	library, err := f.cat.Create("Original reading", kind, root)
	if err != nil {
		t.Fatal(err)
	}
	scan := ingestion.New(f.db, f.cat, assets.Probe{})
	scan.SetStorage(f.deps.Storage)
	job, err := scan.Queue(library.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); scan.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("reading scanner did not stop")
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		state, err := scan.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == "complete" || state.Status == "complete_with_warnings" {
			if state.Warnings != 0 {
				t.Fatalf("reading scan warnings: %+v", state)
			}
			break
		}
		if state.Status == "failed" || time.Now().After(deadline) {
			t.Fatalf("reading scan did not publish: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	d := f.deps
	d.Hosted = nil
	d.directSchema = recordingaccess.NewSchema()
	d.Reading = nil
	s := d.readingService()
	d.Reading = s
	t.Cleanup(func() { s.Close() })
	p, err := f.id.Authenticate(f.owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	r := &readingFixture{f: f, d: d, s: s, p: p, library: library.ID, path: path}
	if err = f.db.QueryRow(`SELECT pid(e.public_id),a.token FROM reading_publications p JOIN catalog_entities e ON e.id=p.item_id JOIN catalog_libraries cl ON cl.id=e.library_id JOIN catalog_assets a ON a.id=p.asset_id WHERE cl.library_id=?`, library.ID).Scan(&r.item, &r.source); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *readingFixture) bytesRequest(t *testing.T, method, path string, session *reading.Session, revision, token string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{"sourceId": {r.source}, "sessionId": {session.ID}}
	req := httptest.NewRequest(method, "/v1/read/items/"+r.item+path+"?"+query.Encode(), nil)
	req.SetPathValue("id", r.item)
	if strings.HasPrefix(path, "/assets/") {
		req.SetPathValue("asset", strings.TrimPrefix(path, "/assets/"))
	}
	if strings.HasPrefix(path, "/pages/") {
		req.SetPathValue("page", strings.TrimPrefix(path, "/pages/"))
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if revision != "" {
		req.Header.Set("If-Match", `"`+revision+`"`)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	switch {
	case path == "/source":
		r.d.serveReadingSource(w, req)
	case strings.HasPrefix(path, "/assets/"):
		r.d.serveReadingAsset(w, req)
	default:
		r.d.serveReadingPage(w, req)
	}
	return w
}

func TestReadingPrivateLocalPDFRangesProgressAndRevocation(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	session, document, err := r.s.Prepare(ctx, r.p, r.item, r.source)
	if err != nil {
		t.Fatal(err)
	}
	if document.Selection.Format != "pdf" || document.Selection.SourceID != r.source || document.Metadata.Title != "Original publication" {
		t.Fatal(document)
	}
	for _, tc := range []struct {
		method, rangeHeader string
		want                int
	}{{"HEAD", "", 200}, {"GET", "bytes=0-4", 206}, {"GET", "", 416}, {"GET", "bytes=0-999999999", 416}, {"GET", "bytes=0-1,3-4", 416}} {
		w := r.bytesRequest(t, tc.method, "/source", session, document.Selection.Revision, r.f.owner.AccessToken, map[string]string{"Range": tc.rangeHeader})
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.method, tc.rangeHeader, w.Code, w.Body.String())
		}
		if tc.want == 206 && (w.Body.String() != "%PDF-" || !strings.HasPrefix(w.Header().Get("Content-Range"), "bytes 0-4/")) {
			t.Fatal("wrong exact range", w.Header(), w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal(w.Header())
		}
	}
	if w := r.bytesRequest(t, "GET", "/source", session, "", r.f.owner.AccessToken, map[string]string{"Range": "bytes=0-4"}); w.Code != 428 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := r.bytesRequest(t, "GET", "/source", session, strings.Repeat("0", 64), r.f.owner.AccessToken, map[string]string{"Range": "bytes=0-4"}); w.Code != 412 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := r.bytesRequest(t, "GET", "/source", session, document.Selection.Revision, "", map[string]string{"Range": "bytes=0-4"}); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	page := int64(0)
	location := reading.Location{Format: "pdf", PageIndex: &page, Progression: .25}
	position, saved, err := r.s.SaveSessionProgress(ctx, r.p, session.ID, 0, location)
	if err != nil || !saved || position.Revision != 1 || position.ObservedAt == 0 {
		t.Fatal(position, saved, err)
	}
	if _, _, err = r.s.SaveSessionProgress(ctx, r.p, session.ID, 0, location); !errors.Is(err, reading.ErrConflict) {
		t.Fatal("CAS", err)
	}
	again, err := r.s.SessionProgress(ctx, r.p, session.ID)
	if err != nil || again.Revision != 1 || again.Stale || *again.Location.PageIndex != 0 {
		t.Fatal(again, err)
	}
	source, err := r.s.OpenSession(ctx, r.p, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err = r.f.id.LogoutToken(ctx, r.f.owner.AccessToken); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 5)
	if n, err := source.ReadAt(b, 0); n != 0 || err == nil {
		t.Fatalf("revoked source returned%d bytes %q: %v", n, b, err)
	}
	if _, _, err = r.s.SaveSessionProgress(ctx, r.p, session.ID, 1, location); err == nil {
		t.Fatal("revoked session saved progress")
	}
}

func TestReadingPrivateEPUBAndComicActualAssets(t *testing.T) {
	for _, tc := range []struct{ kind, format string }{{"book", "epub"}, {"comic", "cbz"}} {
		t.Run(tc.format, func(t *testing.T) {
			r := newReadingFixture(t, tc.kind, tc.format)
			session, document, err := r.s.Prepare(context.Background(), r.p, r.item, r.source)
			if err != nil {
				t.Fatal(err)
			}
			if document.Metadata.Author != "" {
				t.Fatal("file author resurrected over authoritative omission")
			}
			var path string
			if tc.format == "epub" {
				if len(document.Resources) != 5 {
					t.Fatal(document.Resources)
				}
				path = "/assets/EPUB/chapter.xhtml"
			} else {
				if len(document.PageNames) != 2 || document.PageNames[0] != "page1.png" {
					t.Fatal(document.PageNames)
				}
				path = "/pages/0"
			}
			w := r.bytesRequest(t, "GET", path, session, document.Selection.Revision, r.f.owner.AccessToken, nil)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.format == "epub" && !bytes.Contains(w.Body.Bytes(), []byte("Original chapter")) {
				t.Fatal("not actual EPUB bytes")
			}
			if tc.format == "cbz" && !bytes.HasPrefix(w.Body.Bytes(), []byte{137, 'P', 'N', 'G'}) {
				t.Fatal("not actual image bytes")
			}
			if w = r.bytesRequest(t, "GET", "/source", session, document.Selection.Revision, r.f.owner.AccessToken, nil); w.Code != 404 {
				t.Fatal("archive raw source was exposed", w.Code)
			}
			if err = r.s.CloseSession(r.p, session.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = r.s.OpenSession(context.Background(), r.p, session.ID); err == nil {
				t.Fatal("closed session reopened")
			}
		})
	}
}

func TestReadingPrivateSourceChangeRootEscapeAndSourceDisable(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	session, document, err := r.s.Prepare(ctx, r.p, r.item, r.source)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(r.path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-2] = 'X'
	if err = os.WriteFile(r.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(r.path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	// Size and mtime still match; the real opened object's change token does not.
	if _, err = r.s.OpenSession(ctx, r.p, session.ID); err == nil {
		t.Fatal("equal-size/mtime rewrite was accepted")
	}
	w := r.bytesRequest(t, "GET", "/source", session, document.Selection.Revision, r.f.owner.AccessToken, map[string]string{"Range": "bytes=0-4"})
	if w.Code == 206 {
		t.Fatal("changed bytes leaked")
	}
	if _, err = dbwork.ExecWrite(ctx, r.f.db, dbwork.ClassSecurityFence, `UPDATE library_sources SET enabled=0 WHERE library_id=?`, r.library); err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.Resolve(ctx, r.p, r.item, r.source); err == nil {
		t.Fatal("disabled source was eligible")
	}
	// The real rooted helper cannot follow a replaced document outside the root.
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err = os.WriteFile(outside, readingtest.PDF(), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(r.path); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, r.path); err != nil {
		t.Fatal(err)
	}
	if _, err = dbwork.ExecWrite(ctx, r.f.db, dbwork.ClassSecurityFence, `UPDATE library_sources SET enabled=1 WHERE library_id=?`, r.library); err != nil {
		t.Fatal(err)
	}
	if _, err = r.s.Open(ctx, r.p, r.item, r.source, document.Selection.Revision); err == nil {
		t.Fatal("escaped symlink opened")
	}
}

func TestReadingPrivatePauseHistoryAndVersionIsolation(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	session, _, err := r.s.Prepare(ctx, r.p, r.item, r.source)
	if err != nil {
		t.Fatal(err)
	}
	page := int64(0)
	location := reading.Location{Format: "pdf", PageIndex: &page, Progression: .1}
	first, _, err := r.s.SaveSessionProgress(ctx, r.p, session.ID, 0, location)
	if err != nil {
		t.Fatal(err)
	}
	// Use the actual preference API, including its account/profile authority and CAS.
	prefs := r.f.raw("GET", "/v1/preferences", r.f.owner.AccessToken, nil, nil)
	if prefs.Code != 200 {
		t.Fatal(prefs.Code, prefs.Body.String())
	}
	var doc struct {
		Data struct {
			Revision int64 `json:"revision"`
		} `json:"data"`
	}
	if err = json.Unmarshal(prefs.Body.Bytes(), &doc); err != nil || doc.Data.Revision < 1 {
		t.Fatal(prefs.Body.String(), err)
	}
	paused := r.f.raw("PATCH", "/v1/preferences", r.f.owner.AccessToken, nil, map[string]any{
		"expectedRevision": doc.Data.Revision, "idempotencyKey": "reading-pause", "scope": "profile-server", "deviceClass": "web",
		"values": map[string]any{"privacy.pauseWatchHistory": true},
	})
	if paused.Code != 200 {
		t.Fatal(paused.Code, paused.Body.String())
	}
	location.Progression = .9
	same, saved, err := r.s.SaveSessionProgress(ctx, r.p, session.ID, first.Revision, location)
	if err != nil || saved || same == nil || same.Revision != first.Revision || same.Location.Progression != .1 {
		t.Fatal("Pause history changed Reading state", same, saved, err)
	}
	// Reading-specific persistence is independent of the timed-media progress table.
	var count int
	if err = r.f.db.QueryRow(`SELECT count(*) FROM progress WHERE profile_id=?`, identity.PersonalKey(r.p.Viewer)).Scan(&count); err != nil || count != 0 {
		t.Fatal("reading wrote audio/video positions", count, err)
	}
	read, err := r.s.SessionProgress(ctx, r.p, session.ID)
	if err != nil || read.Revision != first.Revision {
		t.Fatal(read, err)
	}
	// No body-supplied profile identity or source id is accepted by session writes.
	other := r.p
	other.ProfileID = "other"
	if _, _, err = r.s.SaveSessionProgress(ctx, other, session.ID, 1, location); err == nil {
		t.Fatal("profile crossover")
	}
	if _, err = r.s.Open(ctx, r.p, r.item, "another-version", first.SourceRevision); err == nil {
		t.Fatal("version crossover")
	}
}

func readingOwnerLogin(t *testing.T, r *readingFixture) identity.DirectSignIn {
	t.Helper()
	login, err := r.f.id.DirectLogin(context.Background(), "owner", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	return login
}

func TestReadingPrivateMemberAndProfileAccessIsLive(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	owner := readingOwnerLogin(t, r)
	member, err := r.f.id.CreateDirectMember(ctx, owner.AccountToken, identity.DirectMemberCreate{
		Username: "reader", Password: "different-test-password", Name: "Reader", AllowedLibraries: []string{r.library},
	})
	if err != nil {
		t.Fatal(err)
	}
	login, err := r.f.id.DirectLogin(ctx, "reader", "different-test-password")
	if err != nil || login.Session == nil {
		t.Fatal(login, err)
	}
	principal, err := r.f.id.Authenticate(login.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := r.s.Prepare(ctx, principal, r.item, r.source)
	if err != nil {
		t.Fatal("ordinary member could not read", err)
	}
	list, err := r.s.List(ctx, principal, r.library, "", 1)
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != r.item {
		t.Fatal(list, err)
	}
	source, err := r.s.OpenSession(ctx, principal, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err = r.f.id.UpdateDirectMember(ctx, owner.AccountToken, member.ID, identity.DirectMemberInput{
		Username: member.Username, Name: "Reader", ExpectedRevision: member.Revision, AllowedLibraries: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := source.ReadAt(make([]byte, 5), 0); n != 0 || err == nil {
		t.Fatal("removed library grant leaked bytes", n, err)
	}
	list, err = r.s.List(ctx, principal, r.library, "", 1)
	if err == nil && len(list.Items) != 0 {
		t.Fatal("hidden item leaked into bounded list", list)
	}

	profiles, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Restricted reader", "blue")
	if err != nil {
		t.Fatal(err)
	}
	var child identity.DirectProfile
	for _, p := range profiles.Profiles {
		if !p.Primary && p.Name == "Restricted reader" {
			child = p
		}
	}
	selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err = r.f.id.Authenticate(selected.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err = r.s.Prepare(ctx, principal, r.item, r.source)
	if err != nil {
		t.Fatal("non-primary profile could not read", err)
	}
	source, err = r.s.OpenSession(ctx, principal, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{
		ExpectedRevision: 1, AllowUnrated: false,
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := source.ReadAt(make([]byte, 5), 0); n != 0 || err == nil {
		t.Fatal("new title restriction leaked bytes", n, err)
	}
	if _, err = r.s.Versions(ctx, principal, r.item); err == nil {
		t.Fatal("restricted versions leaked")
	}
	list, err = r.s.List(ctx, principal, r.library, "", 1)
	if err == nil && len(list.Items) != 0 {
		t.Fatal("restricted title listed", list)
	}
}

func TestReadingPrivateProfileErasurePreservesSiblingAndArchive(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	ctx := context.Background()
	owner := readingOwnerLogin(t, r)
	snapshot, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Erase reader", "blue")
	if err != nil {
		t.Fatal(err)
	}
	var child identity.DirectProfile
	for _, p := range snapshot.Profiles {
		if !p.Primary && p.Name == "Erase reader" {
			child = p
		}
	}
	selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := r.f.id.Authenticate(selected.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	page := int64(0)
	loc := reading.Location{Format: "pdf", PageIndex: &page, Progression: .3}
	childSession, _, err := r.s.Prepare(ctx, principal, r.item, r.source)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.s.SaveSessionProgress(ctx, principal, childSession.ID, 0, loc); err != nil {
		t.Fatal(err)
	}
	ownerSession, _, err := r.s.Prepare(ctx, r.p, r.item, r.source)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.s.SaveSessionProgress(ctx, r.p, ownerSession.ID, 0, loc); err != nil {
		t.Fatal(err)
	}
	childKey := identity.PersonalKey(principal.Viewer)
	ownerKey := identity.PersonalKey(r.p.Viewer)
	// A disconnected authority's archived owner tuple may share the raw IDs.
	// It is persisted data, never an authenticated principal for this service.
	archivedViewer := principal.Viewer
	archivedViewer.Authority = "hosted"
	archivedKey := identity.PersonalKey(archivedViewer)
	err = dbwork.WithWriteTx(ctx, r.f.db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO reading_progress(owner_key,item_id,asset_id,source_revision,format,cfi,section,page_index,progression,revision,observed_at) SELECT ?,item_id,asset_id,source_revision,format,cfi,section,page_index,progression,revision,observed_at FROM reading_progress WHERE owner_key=?`, archivedKey, childKey)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Catalogue removal retains all three owner tuples without touching files.
	if err = r.f.cat.RemoveTitle(ctx, r.item); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{childKey, ownerKey, archivedKey} {
		var n int
		if err = r.f.db.QueryRow(`SELECT count(*) FROM retained_reading_progress p JOIN retained_title_state s ON s.id=p.state_id WHERE s.profile_id=?`, key).Scan(&n); err != nil || n != 1 {
			t.Fatal("retained owner", key, n, err)
		}
	}
	if err = r.f.id.DeleteDirectProfile(ctx, owner.AccountToken, child.ID, child.Revision, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key  string
		want int
	}{{childKey, 0}, {ownerKey, 1}, {archivedKey, 1}} {
		var n int
		if err = r.f.db.QueryRow(`SELECT count(*) FROM retained_reading_progress p JOIN retained_title_state s ON s.id=p.state_id WHERE s.profile_id=?`, tc.key).Scan(&n); err != nil || n != tc.want {
			t.Fatal("erasure crossed owner", tc.key, n, err)
		}
	}
	if _, err = r.s.OpenSession(ctx, principal, childSession.ID); err == nil {
		t.Fatal("erased profile session reopened")
	}
	if _, err = os.Stat(r.path); err != nil {
		t.Fatal("erasure removed publication", err)
	}
	var n int
	if err = r.f.db.QueryRow(`SELECT count(*) FROM reading_progress WHERE owner_key=?`, childKey).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
