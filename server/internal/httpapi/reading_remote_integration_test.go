//go:build rcloneintegration

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/readingtest"
	"portico.local/server/internal/remotesources"
)

// This is an actual rclone LOCAL backend with real scanned publications. It
// proves Reading's actor/book/lease composition, not all-provider behavior or
// immutable origin identity. No permissive source/actor callback is installed.
func TestReadingRemoteRegistered(t *testing.T) {
	program := os.Getenv("PORTICO_RCLONE_TEST_PROGRAM")
	if program == "" {
		t.Fatal("locked rclone fixture executable is required")
	}
	lockRaw, err := os.ReadFile("../../../third_party/rclone/bundle.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Targets map[string]struct{ Files map[string]string }
	}
	if err = json.Unmarshal(lockRaw, &lock); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(program)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	clear(binary)
	if hex.EncodeToString(digest[:]) != lock.Targets["linux-amd64"].Files["rclone"] {
		t.Fatal("fixture executable differs from repository lock")
	}
	t.Setenv("PORTICO_RCLONE", program)
	f := newV1Fixture(t, 0)
	managed, err := mounts.New(f.db, f.root, "", f.deps.Storage.Binary, f.deps.Storage)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, kind := range []string{"book", "comic"} {
		if err = os.Mkdir(filepath.Join(root, kind), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for format, raw := range map[string][]byte{"pdf": readingtest.PDF(), "epub": readingtest.EPUB(), "cbz": readingtest.CBZ()} {
		kind := "book"
		if format == "cbz" {
			kind = "comic"
		}
		if err = os.WriteFile(filepath.Join(root, kind, "Original."+format), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	mount, err := managed.Create(context.Background(), mounts.CreateInput{Name: "Reading fixture", Executable: program, Remote: "fixture:" + root, Config: "[fixture]\ntype = local\n"})
	if err != nil {
		t.Fatal("approved actual remote configuration", err)
	}
	remote, err := remotesources.New(f.db, f.root, managed)
	if err != nil {
		t.Fatal(err)
	}
	f.deps.Storage.Remote = remote
	f.cat.SetStorage(f.deps.Storage)
	life, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); remote.Run(life) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("owned remote source controller did not stop")
		}
	})
	// Wait for the actual source runtime through its metadata operation; no
	// fixed sleep is treated as proof that its shared process is ready.
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err = remote.InspectRoot(life, mount.MountPath)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("source runtime readiness", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	d := f.deps
	d.Hosted, d.Mounts, d.RemoteSources = nil, managed, remote
	d.Reading = NewReadingService(d)
	t.Cleanup(func() { _ = d.Reading.Close() })
	h := New(d)
	owner, err := f.id.DirectLogin(context.Background(), "owner", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"book", "comic"} {
		t.Run(kind, func(t *testing.T) {
			library, err := f.cat.Create("Real remote "+kind, kind, filepath.Join(mount.MountPath, kind))
			if err != nil {
				t.Fatal("real remote library", err)
			}
			readingScan(t, f, library.ID)
			member, err := f.id.CreateDirectMember(context.Background(), owner.AccountToken, identity.DirectMemberCreate{Username: "reader-" + kind, Password: "different-test-password", Name: "Reader", AllowedLibraries: []string{library.ID}})
			if err != nil {
				t.Fatal(err)
			}
			login, err := f.id.DirectLogin(context.Background(), member.Username, "different-test-password")
			if err != nil || login.Session == nil {
				t.Fatal("actual member", err)
			}
			token := login.Session.AccessToken
			w := readingHTTP(t, h, "GET", "/v1/reading/items?libraryId="+library.ID, token, "", nil)
			var items ReadingItemsResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items.Items) != map[string]int{"book": 2, "comic": 1}[kind] {
				t.Fatal("actual remote scan/list", w.Code, w.Body.String())
			}
			for _, item := range items.Items {
				w = readingHTTP(t, h, "POST", "/v1/reading/sessions", token, "", ReadingSessionRequest{item.ID, item.SourceID})
				var manifest ReadingSessionResponse
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &manifest) != nil || manifest.SessionID == "" || manifest.Sequential {
					t.Fatal("real remote prepare", w.Code, w.Body.String())
				}
				base := "/v1/reading/sessions/" + manifest.SessionID
				read := func() *httptest.ResponseRecorder {
					if item.Format == "pdf" {
						req := httptest.NewRequest("GET", base+"/source", nil)
						req.Header.Set("Authorization", "Bearer "+token)
						req.Header.Set("If-Match", `"`+manifest.Revision+`"`)
						req.Header.Set("Range", "bytes=0-4")
						w := httptest.NewRecorder()
						h.ServeHTTP(w, req)
						return w
					}
					path := base + "/pages/0?width=32"
					if item.Format == "epub" {
						path = base + "/assets?path=" + manifest.Spine[0]
					}
					return readingHTTP(t, h, "GET", path, token, manifest.Revision, nil)
				}
				w = read()
				want := 200
				if item.Format == "pdf" {
					want = 206
				}
				if w.Code != want || w.Body.Len() == 0 {
					t.Fatal("actual remote bytes", item.Format, w.Code, w.Body.String())
				}
				if item.Format == "pdf" && w.Body.String() != "%PDF-" || item.Format == "cbz" && !bytes.HasPrefix(w.Body.Bytes(), []byte{137, 'P', 'N', 'G'}) {
					t.Fatal("wrong actual remote bytes")
				}
				page := int64(0)
				location := ReadingLocation{Format: item.Format, PageIndex: &page, Progression: .25}
				if item.Format == "epub" {
					location.PageIndex = nil
					location.CFI = "epubcfi(/6/4!/4/2:0)"
					location.Section = manifest.Spine[0]
				}
				w = readingHTTP(t, h, "PUT", base+"/progress", token, manifest.Revision, ReadingProgressRequest{Location: location})
				if w.Code != 200 {
					t.Fatal("real remote location", w.Code, w.Body.String())
				}
				if w = readingHTTP(t, h, "GET", base+"/progress", f.owner.AccessToken, manifest.Revision, nil); w.Code != 403 {
					t.Fatal("foreign profile reached remote reader", w.Code, w.Body.String())
				}
				w = read()
				if w.Code != want {
					t.Fatal("foreign request retired actual remote reader", w.Code, w.Body.String())
				}
				if w = readingHTTP(t, h, "DELETE", base, token, "", nil); w.Code != 204 {
					t.Fatal(w.Code, w.Body.String())
				}
				if w = read(); w.Code != 403 {
					t.Fatal("closed remote reader delivered bytes", w.Code)
				}
			}
			if err = f.id.UpdateDirectMember(context.Background(), owner.AccountToken, member.ID, identity.DirectMemberInput{Username: member.Username, Name: "Reader", ExpectedRevision: member.Revision, AllowedLibraries: []string{}}); err != nil {
				t.Fatal(err)
			}
			item := items.Items[0]
			if w = readingHTTP(t, h, "POST", "/v1/reading/sessions", token, "", ReadingSessionRequest{item.ID, item.SourceID}); w.Code != 404 {
				t.Fatal("revoked remote library still readable", w.Code, w.Body.String())
			}
			t.Log(fmt.Sprintf("real remote %s scan/member/source/location/close qualified", kind))
		})
	}
}
