package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func readingHTTP(t *testing.T, handler http.Handler, method, path, token, revision string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if revision != "" {
		r.Header.Set("If-Match", `"`+revision+`"`)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func readingOpened(t *testing.T, r *readingFixture, h http.Handler) ReadingSessionResponse {
	t.Helper()
	w := readingHTTP(t, h, "POST", "/v1/reading/sessions", r.f.owner.AccessToken, "", ReadingSessionRequest{r.item, r.source})
	if w.Code != 200 {
		t.Fatalf("prepare: %d %s", w.Code, w.Body.String())
	}
	var manifest ReadingSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SessionID == "" || manifest.ItemID != r.item || manifest.SourceID != r.source || len(manifest.Revision) != 64 || manifest.Size <= 0 || manifest.Metadata.Title == "" || manifest.Metadata.Author != "" {
		t.Fatal("manifest must project actual selected source and catalogue identity", manifest)
	}
	for _, private := range []string{r.path, r.f.owner.AccessToken, "file://", "sourceUrl"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("private source escaped manifest")
		}
	}
	if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("ETag") != `"`+manifest.Revision+`"` {
		t.Fatal(w.Header())
	}
	return manifest
}

func TestReadingRegisteredPDFLifecycle(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	h := New(r.d)
	token := r.f.owner.AccessToken
	if w := readingHTTP(t, h, "GET", "/v1/reading/items", "", "", nil); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := readingHTTP(t, h, "GET", "/v1/reading/items?libraryId="+url.QueryEscape(r.library)+"&limit=1", token, "", nil)
	var list ReadingItemsResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Items) != 1 || list.Items[0].ID != r.item || list.Items[0].SourceID != r.source || !list.Items[0].InventoryAvailable {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"limit=101", "limit=1&limit=2", "profileId=other"} {
		if w = readingHTTP(t, h, "GET", "/v1/reading/items?"+query, token, "", nil); w.Code != 400 {
			t.Fatal(query, w.Code, w.Body.String())
		}
	}
	w = readingHTTP(t, h, "GET", "/v1/items/"+r.item+"/reading/versions", token, "", nil)
	var versions ReadingVersionsResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &versions) != nil || len(versions.Versions) != 1 || versions.Versions[0].SourceID != r.source {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = readingHTTP(t, h, "POST", "/v1/reading/sessions", token, "", ReadingSessionRequest{ItemID: r.item}); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	m := readingOpened(t, r, h)
	base := "/v1/reading/sessions/" + m.SessionID
	if w = readingHTTP(t, h, "HEAD", base+"/source", token, "", nil); w.Code != 428 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = readingHTTP(t, h, "HEAD", base+"/source", token, m.Revision, nil)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != strconv.FormatInt(m.Size, 10) || w.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	req := httptest.NewRequest("GET", base+"/source", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-Match", `"`+m.Revision+`"`)
	req.Header.Set("Range", "bytes=0-63")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.Len() != 64 || w.Header().Get("Content-Range") != fmt.Sprintf("bytes 0-63/%d", m.Size) || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	if w = readingHTTP(t, h, "GET", base+"/source", token, m.Revision, nil); w.Code != 416 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = readingHTTP(t, h, "GET", base+"/progress", token, strings.Repeat("0", 64), nil); w.Code != 412 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = readingHTTP(t, h, "GET", base+"/progress", token, m.Revision, nil)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "{\"progress\":null}" {
		t.Fatal(w.Code, w.Body.String())
	}
	page := int64(0)
	body := ReadingProgressRequest{ExpectedRevision: 0, Location: ReadingLocation{Format: "pdf", PageIndex: &page, Progression: .25}}
	w = readingHTTP(t, h, "PUT", base+"/progress", token, m.Revision, body)
	var saved ReadingProgressSavedResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &saved) != nil || !saved.Saved || saved.Progress == nil || saved.Progress.Revision != 1 || saved.Progress.ObservedAt <= 0 || saved.Progress.SourceRevision != m.Revision {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = readingHTTP(t, h, "PUT", base+"/progress", token, m.Revision, body); w.Code != 412 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = readingHTTP(t, h, "PUT", base+"/progress", token, m.Revision, map[string]any{"location": body.Location}); w.Code != 428 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = readingHTTP(t, h, "PUT", base+"/progress", token, m.Revision, map[string]any{"expectedRevision": 1, "location": body.Location, "profileId": "other"}); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = readingHTTP(t, h, "DELETE", base, token, "", nil)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = readingHTTP(t, h, "GET", base+"/source", token, m.Revision, nil); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	current := readingOpened(t, r, h)
	if err := r.f.id.LogoutToken(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if w = readingHTTP(t, h, "GET", "/v1/reading/sessions/"+current.SessionID+"/progress", token, current.Revision, nil); w.Code != 401 {
		t.Fatal("revoked family reached registered session", w.Code, w.Body.String())
	}
}

func TestReadingRegisteredArchiveAssets(t *testing.T) {
	for _, tc := range []struct{ kind, format string }{{"book", "epub"}, {"comic", "cbz"}} {
		t.Run(tc.format, func(t *testing.T) {
			r := newReadingFixture(t, tc.kind, tc.format)
			h := New(r.d)
			m := readingOpened(t, r, h)
			base := "/v1/reading/sessions/" + m.SessionID
			token := r.f.owner.AccessToken
			if w := readingHTTP(t, h, "GET", base+"/source", token, m.Revision, nil); w.Code != 404 {
				t.Fatal("online archive download", w.Code, w.Body.String())
			}
			path := ""
			if tc.format == "epub" {
				if len(m.Resources) == 0 || len(m.Spine) == 0 || len(m.PageNames) != 0 {
					t.Fatal(m)
				}
				path = base + "/assets?path=" + url.QueryEscape(m.Spine[0])
			} else {
				if len(m.PageNames) != 2 || m.PageNames[0] != "page1.png" || m.PageNames[1] != "page2.png" || len(m.Resources) != 0 {
					t.Fatal(m)
				}
				path = base + "/pages/0?width=32"
			}
			w := readingHTTP(t, h, "GET", path, token, m.Revision, nil)
			if w.Code != 200 || w.Body.Len() == 0 || w.Header().Get("ETag") != `"`+m.Revision+`"` {
				t.Fatal(w.Code, w.Header(), w.Body.String())
			}
			if tc.format == "epub" {
				var size int64
				for _, resource := range m.Resources {
					if resource.Path == m.Spine[0] {
						size = resource.Size
					}
				}
				if int64(w.Body.Len()) != size || !strings.Contains(w.Header().Get("Content-Type"), "xhtml") {
					t.Fatal("asset byte/size/MIME fence", w.Header(), w.Body.Len(), size)
				}
			} else if !bytes.HasPrefix(w.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
				t.Fatal("actual raster bytes required")
			}
			if w = readingHTTP(t, h, "GET", path, "", m.Revision, nil); w.Code != 401 {
				t.Fatal(w.Code, w.Body.String())
			}
			if w = readingHTTP(t, h, "DELETE", base, token, "", nil); w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
			if w = readingHTTP(t, h, "GET", path, token, m.Revision, nil); w.Code != 403 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
