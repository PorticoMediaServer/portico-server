package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type scriptedInventoryBackend struct {
	inventory func(string) (storageInventoryPage, error)
}

func TestRemoteStorageRootCannotEscapeConfiguredRemote(t *testing.T) {
	for _, value := range []string{"..", "../other", "../../other", `/..\\other`, "\x00hidden"} {
		if normalized, err := normalizeRemoteStorageRoot(value); err == nil {
			t.Fatalf("root %q unexpectedly normalized to %q", value, normalized)
		}
	}
	for value, expected := range map[string]string{"": "", "/movies/4k/": "movies/4k", "movies/./family": "movies/family"} {
		normalized, err := normalizeRemoteStorageRoot(value)
		if err != nil || normalized != expected {
			t.Fatalf("root %q normalized=%q err=%v", value, normalized, err)
		}
	}
}

func (b scriptedInventoryBackend) Kind() string { return "webdav" }
func (b scriptedInventoryBackend) Inventory(_ context.Context, cursor string, _ int) (storageInventoryPage, error) {
	return b.inventory(cursor)
}
func (b scriptedInventoryBackend) OpenRange(context.Context, string, int64, int64) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func TestRemoteObjectPathRejectsTraversal(t *testing.T) {
	for _, value := range []string{"../secret", "folder/../../secret", "", "\x00bad"} {
		if _, err := normalizeRemoteObjectPath(value); err == nil {
			t.Fatalf("path %q accepted", value)
		}
	}
	if got, err := normalizeRemoteObjectPath(`/Movies/Film.mkv`); err != nil || got != "Movies/Film.mkv" {
		t.Fatalf("normalized=%q err=%v", got, err)
	}
}

func TestRemoteSchedulerGivesPlaybackForegroundPriority(t *testing.T) {
	q := newRemoteStorageScheduler(2, 1)
	release, err := q.acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := q.acquire(context.Background(), false); err == nil {
		t.Fatal("background storage admitted while playback was active")
	}
}

func TestRemoteSchedulerPreemptsActiveBackgroundReadForPlayback(t *testing.T) {
	q := newRemoteStorageScheduler(1, 1)
	backgroundCtx, releaseBackground, err := q.acquireOperation(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBackground()
	releasePlayback, err := q.acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePlayback()
	select {
	case <-backgroundCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("foreground playback did not preempt active background storage I/O")
	}
}

func TestRcloneReadCloserLetsCompletedChunkExitCleanly(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf film")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	released := false
	reader := &rcloneReadCloser{pipe: pipe, cmd: cmd, expected: 4, release: func() { released = true }}
	data, err := io.ReadAll(reader)
	if err != nil || string(data) != "film" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("completed rclone chunk was treated as terminated: %v", err)
	}
	if !released {
		t.Fatal("rclone playback admission was not released")
	}
}

func TestRcloneReadCloserKillsAProcessThatIgnoresGracefulShutdown(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; while :; do :; done")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	released := false
	reader := &rcloneReadCloser{
		pipe: pipe, cmd: cmd, expected: 4, release: func() { released = true }, closeGrace: 25 * time.Millisecond,
	}
	started := time.Now()
	if err := reader.Close(); err == nil {
		t.Fatal("forced process termination unexpectedly reported a clean exit")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("rclone close exceeded its bounded shutdown window: %v", elapsed)
	}
	if !released {
		t.Fatal("rclone playback admission was not released after forced shutdown")
	}
}

func TestRemotePlaybackProbeLockSerializesWaitersAndReclaimsEntry(t *testing.T) {
	key := "probe-lock-test"
	releaseFirst := acquireRemotePlaybackProbeLock(key)
	acquiredSecond := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan struct{})
	go func() {
		release := acquireRemotePlaybackProbeLock(key)
		close(acquiredSecond)
		<-releaseSecond
		release()
		close(done)
	}()

	select {
	case <-acquiredSecond:
		t.Fatal("a second probe entered while the first still held the key")
	case <-time.After(25 * time.Millisecond):
	}
	releaseFirst()
	select {
	case <-acquiredSecond:
	case <-time.After(time.Second):
		t.Fatal("waiting probe did not acquire the released key")
	}
	remotePlaybackProbeLocks.Lock()
	entry := remotePlaybackProbeLocks.entries[key]
	remotePlaybackProbeLocks.Unlock()
	if entry == nil {
		t.Fatal("probe lock was reclaimed while a waiter still held it")
	}
	close(releaseSecond)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second probe did not release")
	}
	remotePlaybackProbeLocks.Lock()
	_, retained := remotePlaybackProbeLocks.entries[key]
	remotePlaybackProbeLocks.Unlock()
	if retained {
		t.Fatal("unused probe lock entry was retained")
	}
}

func TestRcloneInventoryPaginatesOneLargeDirectoryDeterministically(t *testing.T) {
	backend := &managedRclone{
		Binary: "ignored", Config: t.TempDir() + "/rclone.conf", Remote: "archive", Scheduler: newRemoteStorageScheduler(2, 1),
		command: func(context.Context, string, ...string) *exec.Cmd {
			return exec.Command("sh", "-c", `printf '%s' '[{"Name":"c.mkv","Size":3},{"Name":"a.mkv","Size":1},{"Name":"b.mkv","Size":2}]'`)
		},
	}
	if err := os.WriteFile(backend.Config, []byte("[archive]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := backend.Inventory(context.Background(), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Objects) != 2 || first.Objects[0].Path != "a.mkv" || first.Objects[1].Path != "b.mkv" || first.NextCursor == "" || first.Authoritative {
		t.Fatalf("first=%+v", first)
	}
	second, err := backend.Inventory(context.Background(), first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Objects) != 1 || second.Objects[0].Path != "c.mkv" || !second.Authoritative || second.NextCursor != "" {
		t.Fatalf("second=%+v", second)
	}
}

func TestRcloneStatReadsProviderRevision(t *testing.T) {
	var invoked []string
	backend := &managedRclone{
		Binary: "ignored", Config: filepath.Join(t.TempDir(), "rclone.conf"), Remote: "archive", Scheduler: newRemoteStorageScheduler(2, 1),
		command: func(_ context.Context, _ string, args ...string) *exec.Cmd {
			invoked = append([]string(nil), args...)
			return exec.Command("sh", "-c", `printf '%s' '{"ID":"provider-id","Size":42,"ModTime":"2026-08-22T12:00:00Z","MimeType":"video/x-matroska","Hashes":{"sha1":"abc"}}'`)
		},
	}
	if err := os.WriteFile(backend.Config, []byte("[archive]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	object, err := backend.Stat(context.Background(), "Movies/Film.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if object.Path != "Movies/Film.mkv" || object.Size != 42 || object.ObjectID != "provider-id" || object.Hash != "abc" || object.Revision == "" {
		t.Fatalf("object=%+v", object)
	}
	if !slices.Contains(invoked, "--stat") || !slices.Contains(invoked, "--hash") {
		t.Fatalf("rclone args=%v", invoked)
	}
}

func TestRcloneStatPreservesPlaybackPreemption(t *testing.T) {
	invoked := make(chan struct{}, 1)
	scheduler := newRemoteStorageScheduler(1, 1)
	backend := &managedRclone{
		Binary: "ignored", Config: filepath.Join(t.TempDir(), "rclone.conf"), Remote: "archive", Scheduler: scheduler,
		command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			invoked <- struct{}{}
			return exec.CommandContext(ctx, "sh", "-c", "sleep 5")
		},
	}
	if err := os.WriteFile(backend.Config, []byte("[archive]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := backend.Stat(context.Background(), "Movies/Film.mkv")
		result <- err
	}()
	select {
	case <-invoked:
	case <-time.After(time.Second):
		t.Fatal("rclone stat command was not invoked")
	}
	releasePlayback, err := scheduler.acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePlayback()
	select {
	case statErr := <-result:
		if !errors.Is(statErr, errRemoteStoragePreempted) {
			t.Fatalf("stat error=%v", statErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rclone stat did not stop after playback preemption")
	}
}

func TestCommandOutputLimitTerminatesOversizedRcloneOutput(t *testing.T) {
	cmd := exec.Command("sh", "-c", "printf 123456789")
	if _, err := commandOutputLimit(cmd, 4); err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("err=%v", err)
	}
}

func TestWebDAVInventoryUsesObjectListingAndRangeReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			if r.Header.Get("Depth") != "1" && r.Header.Get("Depth") != "0" {
				t.Errorf("Depth=%q", r.Header.Get("Depth"))
			}
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:sync-token>token-2</d:sync-token><d:response><d:href>/media/Movies/Film.mkv</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:getetag>etag-1</d:getetag><d:getcontentlength>42</d:getcontentlength><d:getlastmodified>Wed, 21 Oct 2015 07:28:00 GMT</d:getlastmodified><d:getcontenttype>video/x-matroska</d:getcontenttype><d:resourcetype/></d:prop></d:propstat></d:response></d:multistatus>`)
		case http.MethodGet:
			if r.Header.Get("Range") != "bytes=10-13" {
				t.Errorf("Range=%q", r.Header.Get("Range"))
			}
			w.Header().Set("Content-Range", "bytes 10-13/42")
			w.Header().Set("Content-Length", "4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "film")
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/media/")
	b := &webDAVBackend{BaseURL: base, Scheduler: newRemoteStorageScheduler(2, 1)}
	page, err := b.Inventory(context.Background(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Authoritative || page.SyncToken != "token-2" || len(page.Objects) != 1 || page.Objects[0].Path != "Movies/Film.mkv" || page.Objects[0].ETag != "etag-1" {
		t.Fatalf("page=%#v", page)
	}
	stat, err := b.Stat(context.Background(), "Movies/Film.mkv")
	if err != nil || stat.Path != "Movies/Film.mkv" || stat.Size != 42 || stat.ETag != "etag-1" || stat.Revision == "" {
		t.Fatalf("stat=%+v err=%v", stat, err)
	}
	reader, err := b.OpenRange(context.Background(), "Movies/Film.mkv", 10, 4)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(data) != "film" {
		t.Fatalf("range=%q", data)
	}
}

func TestWebDAVContentRangeMustMatchRequestedChunk(t *testing.T) {
	for value, valid := range map[string]bool{
		"bytes 10-13/42": true,
		"bytes 10-13/*":  true,
		"bytes 0-3/42":   false,
		"bytes 10-14/42": false,
		"items 10-13/42": false,
		"":               false,
	} {
		if got := validWebDAVContentRange(value, 10, 4); got != valid {
			t.Fatalf("validWebDAVContentRange(%q)=%v want %v", value, got, valid)
		}
	}
}

func TestWebDAVInventoryPaginatesDirectoryBFS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" || r.Header.Get("Depth") != "1" {
			t.Fatalf("request=%s %s depth=%q", r.Method, r.URL.Path, r.Header.Get("Depth"))
		}
		w.WriteHeader(http.StatusMultiStatus)
		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/media":
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/media/A/</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response><d:response><d:href>/media/B/</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`)
		case "/media/A":
			_, _ = io.WriteString(w, webDAVTestFileResponse("/media/A/a.mkv", "a"))
		case "/media/B":
			_, _ = io.WriteString(w, webDAVTestFileResponse("/media/B/b.mkv", "b"))
		default:
			t.Fatalf("unexpected WebDAV path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/media/")
	backend := &webDAVBackend{BaseURL: base, Scheduler: newRemoteStorageScheduler(2, 1)}
	var cursor string
	var objects []string
	for calls := 0; calls < 8; calls++ {
		page, err := backend.Inventory(context.Background(), cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, object := range page.Objects {
			objects = append(objects, object.Path)
		}
		if page.Authoritative {
			break
		}
		if page.NextCursor == "" {
			t.Fatal("non-authoritative page omitted its resume cursor")
		}
		cursor = page.NextCursor
	}
	if strings.Join(objects, ",") != "A/a.mkv,B/b.mkv" {
		t.Fatalf("objects=%v", objects)
	}
}

func TestWebDAVRedirectDoesNotForwardCredentialsAcrossOrigins(t *testing.T) {
	forwardedAuthorization := ""
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusFound)
	}))
	defer source.Close()
	base, _ := url.Parse(source.URL + "/media/")
	backend := &webDAVBackend{BaseURL: base, Username: "owner", Password: "secret", Scheduler: newRemoteStorageScheduler(2, 1)}
	if _, err := backend.Inventory(context.Background(), "", 10); err == nil {
		t.Fatal("cross-origin WebDAV redirect was accepted")
	}
	if forwardedAuthorization != "" {
		t.Fatal("WebDAV credentials were forwarded across origins")
	}
}

func webDAVTestFileResponse(href, etag string) string {
	return `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>` + href + `</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:getetag>` + etag + `</d:getetag><d:getcontentlength>42</d:getcontentlength><d:getlastmodified>Wed, 21 Oct 2015 07:28:00 GMT</d:getlastmodified><d:getcontenttype>video/x-matroska</d:getcontenttype><d:resourcetype/></d:prop></d:propstat></d:response></d:multistatus>`
}

func TestSTRMParsingIsStrictAndScannerKeepsDescriptorPrivate(t *testing.T) {
	locator, err := parseSTRMLocator([]byte("https://cdn.example.test/video.mkv?token=secret\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(locator, "token=secret") {
		t.Fatalf("locator=%q", locator)
	}
	for _, raw := range []string{"file:///tmp/movie.mkv", "https://one.example/a\nhttps://two.example/b", "https://user:pass@example.test/a"} {
		if _, err := parseSTRMLocator([]byte(raw)); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if !isMediaFileForLibrary("movie", "Movie (2026).strm") {
		t.Fatal("STRM not accepted by movie scanner")
	}
}

func TestRemoteStorageHTTPRangeParsing(t *testing.T) {
	for _, test := range []struct {
		value      string
		start, end int64
		partial    bool
	}{{"", 0, 99, false}, {"bytes=10-19", 10, 19, true}, {"bytes=90-", 90, 99, true}, {"bytes=-5", 95, 99, true}} {
		start, end, partial, err := parseStorageHTTPRange(test.value, 100)
		if err != nil || start != test.start || end != test.end || partial != test.partial {
			t.Fatalf("range %q = %d-%d partial=%v err=%v", test.value, start, end, partial, err)
		}
	}
	for _, value := range []string{"bytes=100-101", "bytes=10-1", "items=0-1", "bytes=1-2,4-5"} {
		if _, _, _, err := parseStorageHTTPRange(value, 100); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestRemoteInventoryResumesCommittedCursorAfterTransientFailure(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO storage_sources(id,library_id,configured_path,classification,classification_source,backend_kind,display_name,created_at,updated_at) VALUES('storage-resume',?,'webdav://resume','network','owner','webdav','Resume',?,?)`, library.ID, now, now); err != nil {
		t.Fatal(err)
	}
	first := scriptedInventoryBackend{inventory: func(cursor string) (storageInventoryPage, error) {
		switch cursor {
		case "":
			return storageInventoryPage{Objects: []storageObject{{Path: "Movies/One.mkv", Revision: "one", Size: 1, ModTime: time.Now()}}, NextCursor: "page-2"}, nil
		case "page-2":
			return storageInventoryPage{}, errors.New("provider temporarily unavailable")
		default:
			return storageInventoryPage{}, errors.New("unexpected cursor " + cursor)
		}
	}}
	failed, err := server.inventoryRemoteSource(context.Background(), "storage-resume", first, 10)
	if err == nil {
		t.Fatal("transient provider failure was hidden")
	}
	var status, cursor string
	if err := server.db.QueryRow(`SELECT status,cursor FROM storage_inventory_runs WHERE id=?`, failed.RunID).Scan(&status, &cursor); err != nil {
		t.Fatal(err)
	}
	if status != "degraded" || cursor != "page-2" {
		t.Fatalf("status=%q cursor=%q", status, cursor)
	}
	resumedFrom := ""
	second := scriptedInventoryBackend{inventory: func(cursor string) (storageInventoryPage, error) {
		resumedFrom = cursor
		return storageInventoryPage{Objects: []storageObject{{Path: "Movies/Two.mkv", Revision: "two", Size: 2, ModTime: time.Now()}}, Authoritative: true}, nil
	}}
	completed, err := server.inventoryRemoteSource(context.Background(), "storage-resume", second, 10)
	if err != nil {
		t.Fatal(err)
	}
	if resumedFrom != "page-2" || completed.RunID != failed.RunID || completed.ObjectsSeen != 1 {
		t.Fatalf("resumedFrom=%q failed=%q completed=%+v", resumedFrom, failed.RunID, completed)
	}
	if err := server.db.QueryRow(`SELECT status FROM storage_inventory_runs WHERE id=?`, completed.RunID).Scan(&status); err != nil || status != "healthy" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestRemoteInventoryRestartsWithANewGenerationAfterCursorInvalidation(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Mutable remote", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO storage_sources(id,library_id,configured_path,classification,classification_source,backend_kind,display_name,created_at,updated_at) VALUES('storage-mutated',?,'webdav://mutated','network','owner','webdav','Mutated',?,?)`, library.ID, now, now); err != nil {
		t.Fatal(err)
	}
	rootCalls := 0
	backend := scriptedInventoryBackend{inventory: func(cursor string) (storageInventoryPage, error) {
		switch cursor {
		case "":
			rootCalls++
			if rootCalls == 1 {
				return storageInventoryPage{Objects: []storageObject{{Path: "old.mkv", Revision: "old", Size: 1, ModTime: time.Now()}}, NextCursor: "stale-page"}, nil
			}
			return storageInventoryPage{Objects: []storageObject{{Path: "new.mkv", Revision: "new", Size: 2, ModTime: time.Now()}}, Authoritative: true}, nil
		case "stale-page":
			return storageInventoryPage{}, fmt.Errorf("%w: directory shrank", errRemoteInventoryCursorInvalid)
		default:
			return storageInventoryPage{}, fmt.Errorf("unexpected cursor %q", cursor)
		}
	}}
	completed, err := server.inventoryRemoteSource(context.Background(), "storage-mutated", backend, 10)
	if err != nil {
		t.Fatal(err)
	}
	if rootCalls != 2 || completed.ObjectsSeen != 1 || !completed.Authoritative {
		t.Fatalf("rootCalls=%d completed=%+v", rootCalls, completed)
	}
	var failedRuns, healthyRuns int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM storage_inventory_runs WHERE source_id='storage-mutated' AND status='failed'`).Scan(&failedRuns); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM storage_inventory_runs WHERE source_id='storage-mutated' AND status='healthy'`).Scan(&healthyRuns); err != nil {
		t.Fatal(err)
	}
	if failedRuns != 1 || healthyRuns != 1 {
		t.Fatalf("failed runs=%d healthy runs=%d", failedRuns, healthyRuns)
	}
}

func TestDeletingLibraryRemovesManagedRemoteStorageArtifacts(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Disposable remote", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(server.cfg.AppDataDir, "remote-storage", "remote-delete")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "rclone.conf")
	if err := os.WriteFile(configPath, []byte("[archive]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO managed_rclone_installations(id,binary_path,binary_version,binary_sha256,config_path,approved_at,last_validated_at) VALUES('install-delete','/usr/bin/false','test',?, ?,?,?)`, strings.Repeat("a", 64), configPath, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO storage_sources(id,library_id,configured_path,classification,classification_source,backend_kind,display_name,rclone_remote_name,rclone_installation_id,created_at,updated_at) VALUES('remote-delete',?,'rclone://archive','network','owner','rclone','Archive','archive','install-delete',?,?)`, library.ID, now, now); err != nil {
		t.Fatal(err)
	}
	schedulerForRemoteSource("remote-delete", 2)
	if err := server.deleteLibrary(library.ID); err != nil {
		t.Fatal(err)
	}
	for table, key := range map[string]string{"storage_sources": "remote-delete", "managed_rclone_installations": "install-delete"} {
		var count int
		if err := server.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id=?`, key).Scan(&count); err != nil || count != 0 {
			t.Fatalf("table=%s count=%d err=%v", table, count, err)
		}
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed rclone config remained after library deletion: %v", err)
	}
	if _, retained := remoteSourceSchedulers.Load("remote-delete"); retained {
		t.Fatal("remote scheduler remained after library deletion")
	}
}

func TestWebDAVSourceConfigurationKeepsCredentialWriteOnly(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{Kind: "webdav", Name: "Archive", Endpoint: "https://dav.example.test/media?discard=yes", Root: "Movies", Username: "portico", Password: "super-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if source.Name != "Archive" || source.Endpoint != "https://dav.example.test/media" || !source.CredentialPresent {
		t.Fatalf("source=%#v", source)
	}
	if source.AnalysisMode != "basic" {
		t.Fatalf("default analysis mode=%q", source.AnalysisMode)
	}
	updated, err := server.updateRemoteStorageSource(context.Background(), library.ID, source.ID, RemoteStorageSourcePatchRequest{AnalysisMode: "custom"})
	if err != nil || updated.AnalysisMode != "custom" {
		t.Fatalf("custom source=%#v err=%v", updated, err)
	}
	updated, err = server.updateRemoteStorageSource(context.Background(), library.ID, source.ID, RemoteStorageSourcePatchRequest{AnalysisMode: "file_list_only"})
	if err != nil || updated.AnalysisMode != "file_list_only" {
		t.Fatalf("updated source=%#v err=%v", updated, err)
	}
	if _, err := server.updateRemoteStorageSource(context.Background(), library.ID, source.ID, RemoteStorageSourcePatchRequest{AnalysisMode: "deep_magic"}); err == nil {
		t.Fatal("unsupported remote analysis mode was accepted")
	}
	var stored string
	if err := server.db.QueryRow(`SELECT secret_envelope FROM storage_source_credentials WHERE source_id=?`, source.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "super-secret") {
		t.Fatal("WebDAV password was stored in plaintext")
	}
}

func TestWebDAVCredentialsRequireHTTPS(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{
		Kind: "webdav", Name: "Unsafe DAV", Endpoint: "http://dav.example.test/media", Username: "portico", Password: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "require an HTTPS endpoint") {
		t.Fatalf("cleartext WebDAV credentials were accepted: %v", err)
	}
}

func TestDeletingRemoteSourceImmediatelyRetiresItsCatalogFiles(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{
		Kind: "webdav", Name: "Disposable DAV", Endpoint: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	locator := remoteStorageLocator(source.ID, "Movies/Film.mkv")
	insertIdentityMedia(t, server, "remote_movie", library.ID, "movie", "", now)
	if _, err := server.db.Exec(`INSERT INTO media_files(id,media_id,library_id,path,container,source_type,size_bytes,mod_time,available,first_seen_at,last_seen_at) VALUES('remote_file','remote_movie',?,?,'mkv','webdav',1000,?,1,?,?)`, library.ID, locator, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET source_url=? WHERE id='remote_movie'`, locator); err != nil {
		t.Fatal(err)
	}
	scheduler := schedulerForRemoteSource(source.ID, 2)
	releasePlayback, err := scheduler.acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.deleteRemoteStorageSource(context.Background(), library.ID, source.ID); !errors.Is(err, errRemoteStorageSourceInUse) {
		t.Fatalf("active remote source delete error = %v, want in-use conflict", err)
	}
	releasePlayback()
	if err := server.deleteRemoteStorageSource(context.Background(), library.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	var available int
	var sourceURL string
	if err := server.db.QueryRow(`SELECT available FROM media_files WHERE id='remote_file'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT source_url FROM media_items WHERE id='remote_movie'`).Scan(&sourceURL); err != nil {
		t.Fatal(err)
	}
	if available != 0 || strings.TrimSpace(sourceURL) != "" {
		t.Fatalf("deleted remote source remained playable: available=%d sourceURL=%q", available, sourceURL)
	}
}

func TestRemoteStorageRemovalFencesWaitingBackgroundAdmission(t *testing.T) {
	scheduler := newRemoteStorageScheduler(1, 1)
	firstCtx, firstRelease, err := scheduler.acquireOperation(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer firstRelease()
	firstReleased := make(chan struct{})
	go func() {
		<-firstCtx.Done()
		firstRelease()
		close(firstReleased)
	}()
	secondResult := make(chan error, 1)
	go func() {
		_, release, acquireErr := scheduler.acquireOperation(context.Background(), false)
		if release != nil {
			release()
		}
		secondResult <- acquireErr
	}()
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !scheduler.beginRemoval(ctx) {
		t.Fatal("removal did not drain the admitted background operation")
	}
	<-firstReleased
	if acquireErr := <-secondResult; !errors.Is(acquireErr, errRemoteStorageSourceRemoved) {
		t.Fatalf("waiting background admission error = %v", acquireErr)
	}
}

func TestRemoteInventoryOnlyQueuesNoContentAnalysis(t *testing.T) {
	contentReads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"><d:response><d:href>/Movies/Film.mp4</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:getetag>etag-1</d:getetag><d:getcontentlength>1000</d:getcontentlength><d:getlastmodified>Wed, 21 Oct 2015 07:28:00 GMT</d:getlastmodified><d:getcontenttype>video/mp4</d:getcontenttype><d:resourcetype/></d:prop></d:propstat></d:response><d:response><d:href>/Movies/Private.strm</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:getetag>etag-strm</d:getetag><d:getcontentlength>64</d:getcontentlength><d:getlastmodified>Wed, 21 Oct 2015 07:28:00 GMT</d:getlastmodified><d:getcontenttype>text/plain</d:getcontenttype><d:resourcetype/></d:prop></d:propstat></d:response></d:multistatus>`)
			return
		}
		if r.Method == http.MethodGet {
			contentReads++
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	t.Cleanup(upstream.Close)
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{Kind: "webdav", Name: "Archive", Endpoint: upstream.URL, AnalysisMode: "file_list_only"})
	if err != nil {
		t.Fatal(err)
	}
	roots, err := server.remoteLibraryRootEvidence(context.Background(), library.ID)
	if err != nil || len(roots) != 1 || roots[0].Health != "unknown" {
		t.Fatalf("unvalidated source health was not preserved as unknown: roots=%+v err=%v", roots, err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := server.scanRemoteStorageSources(context.Background(), library, libraryScanRun{ID: "remote-policy-run", LibraryID: library.ID, Mode: "reconcile", StartedAt: now}, "remote-policy-generation", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesIndexed != 1 || result.FilesSkipped != 1 || result.AnalysisQueued != 0 || contentReads != 0 {
		t.Fatalf("inventory-only result=%+v contentReads=%d", result, contentReads)
	}
	if _, err := server.updateRemoteStorageSource(context.Background(), library.ID, source.ID, RemoteStorageSourcePatchRequest{AnalysisMode: "custom"}); err != nil {
		t.Fatal(err)
	}
	result, err = server.scanRemoteStorageSources(context.Background(), library, libraryScanRun{ID: "remote-policy-run-2", LibraryID: library.ID, Mode: "reconcile", StartedAt: now}, "remote-policy-generation-2", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisQueued != 0 || contentReads != 0 {
		t.Fatalf("Custom without enabled operations result=%+v contentReads=%d", result, contentReads)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{"probeStreams":true}' WHERE id=?`, library.ID); err != nil {
		t.Fatal(err)
	}
	library.Settings["probeStreams"] = true
	result, err = server.scanRemoteStorageSources(context.Background(), library, libraryScanRun{ID: "remote-policy-run-3", LibraryID: library.ID, Mode: "reconcile", StartedAt: now}, "remote-policy-generation-3", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.AnalysisQueued != 1 || contentReads != 0 {
		t.Fatalf("Custom selected probe result=%+v contentReads=%d", result, contentReads)
	}
	var remoteMediaID string
	if err := server.db.QueryRow(`SELECT media_id FROM media_files WHERE path=?`, remoteStorageLocator(source.ID, "Movies/Film.mp4")).Scan(&remoteMediaID); err != nil {
		t.Fatal(err)
	}
	deferred, err := server.createJobForWithMetadata("media_analyze", "Deferred remote analysis.", "media", remoteMediaID, mediaAnalysisMetadata(mediaAnalysisModeProbe))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status='deferred',phase='deferred',deferred_until=? WHERE id=?`, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), deferred.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.updateRemoteStorageSource(context.Background(), library.ID, source.ID, RemoteStorageSourcePatchRequest{AnalysisMode: "file_list_only"}); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM scanner_backlog WHERE kind='analysis' AND status='queued'`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("queued analysis remained after inventory-only switch: count=%d err=%v", queued, err)
	}
	var deferredStatus, deferredUntil string
	if err := server.db.QueryRow(`SELECT status,deferred_until FROM jobs WHERE id=?`, deferred.ID).Scan(&deferredStatus, &deferredUntil); err != nil {
		t.Fatal(err)
	}
	if deferredStatus != "cancelled" || deferredUntil != "" {
		t.Fatalf("deferred remote analysis remained eligible: status=%q deferredUntil=%q", deferredStatus, deferredUntil)
	}
}

func TestRemoteStorageTranscodeSourceStreamsThroughManagedBackend(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Movies/Film.mkv" || r.Header.Get("Range") != "bytes=0-3" {
			t.Fatalf("request=%s %s range=%q", r.Method, r.URL.Path, r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "film")
	}))
	t.Cleanup(upstream.Close)
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{Kind: "webdav", Name: "Archive", Endpoint: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO storage_remote_objects(source_id,object_path,revision,size_bytes,first_seen_generation,last_seen_generation,updated_at) VALUES(?,'Movies/Film.mkv','rev-1',4,'gen-1','gen-1',?)`, source.ID, now); err != nil {
		t.Fatal(err)
	}
	locator := remoteStorageLocator(source.ID, "Movies/Film.mkv")
	reader, err := server.openRemoteStorageTranscodeSource(context.Background(), MediaItem{LibraryID: library.ID}, locator)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil || string(data) != "film" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestRemoteDirectPlaybackUsesOneProviderStream(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Range") != "bytes=0-3" {
			t.Fatalf("range=%q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "film")
	}))
	t.Cleanup(upstream.Close)
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{Kind: "webdav", Name: "Archive", Endpoint: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`INSERT INTO storage_remote_objects(source_id,object_path,revision,size_bytes,content_type,first_seen_generation,last_seen_generation,updated_at) VALUES(?,'Movies/Film.mkv','rev-1',4,'video/x-matroska','gen-1','gen-1',?)`, source.ID, now); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/playback", nil)
	recorder := httptest.NewRecorder()
	if err := server.serveRemoteStorageObject(recorder, request, MediaItem{LibraryID: library.ID}, remoteStorageLocator(source.ID, "Movies/Film.mkv")); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || recorder.Code != http.StatusOK || recorder.Body.String() != "film" {
		t.Fatalf("requests=%d status=%d body=%q", requests, recorder.Code, recorder.Body.String())
	}
	item := MediaItem{ID: "remote-film", LibraryID: library.ID, Title: "Film", SourceURL: remoteStorageLocator(source.ID, "Movies/Film.mkv")}
	download, err := server.downloadSourceForRequestContext(context.Background(), item, "source")
	if err != nil {
		t.Fatalf("resolve remote download: %v", err)
	}
	if download.sourceURL != item.SourceURL || download.sourceRevision != "rev-1" || download.sizeBytes != 4 || download.sourceKind != "remote-storage" {
		t.Fatalf("unexpected remote download source: %#v", download)
	}
	head := httptest.NewRequest(http.MethodHead, "/api/media/remote-film/download", nil)
	headRecorder := httptest.NewRecorder()
	if err := server.servePlaybackSource(headRecorder, head, item, download.sourceURL); err != nil {
		t.Fatalf("head remote download: %v", err)
	}
	if headRecorder.Code != http.StatusOK || headRecorder.Header().Get("Content-Length") != "4" || requests != 1 {
		t.Fatalf("HEAD status=%d length=%q providerRequests=%d", headRecorder.Code, headRecorder.Header().Get("Content-Length"), requests)
	}
	fingerprint := downloadGrantVersionFingerprint(item.ID, "source", "source", sourceDownloadGrantVersionID(item, download), download)
	if _, err := server.db.Exec(`UPDATE storage_remote_objects SET revision='rev-2' WHERE source_id=? AND object_path='Movies/Film.mkv'`, source.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := server.downloadSourceForRequestContext(context.Background(), item, "source")
	if err != nil {
		t.Fatal(err)
	}
	if updated.sourceRevision != "rev-2" || fingerprint == downloadGrantVersionFingerprint(item.ID, "source", "source", sourceDownloadGrantVersionID(item, updated), updated) {
		t.Fatal("remote object revision did not invalidate the download grant fingerprint")
	}
	if _, err := server.db.Exec(`UPDATE storage_remote_objects SET missing_since=? WHERE source_id=? AND object_path='Movies/Film.mkv'`, now, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.downloadSourceForRequestContext(context.Background(), item, "source"); err == nil {
		t.Fatal("missing remote object remained downloadable")
	}
}
