package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ingestion"
	"portico.local/server/internal/persistence"
)

// GET /v1/guide/images/{digest}: 401 without a credential, 404 for an unknown
// or malformed digest, 200 with the stored bytes and cache headers.
func TestGuideImageServing(t *testing.T) {
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ident, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','profile',1);INSERT INTO libraries(id,name,kind,root) VALUES('lib','Films','movie','/media/films')`); err != nil {
		t.Fatal(err)
	}
	owner, err := ident.Issue("owner", "profile", "local", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(db)
	control, err := hosted.New(db, ident, "http://127.0.0.1:19410", base64.RawURLEncoding.EncodeToString(make([]byte, 32)), zeroHostedRootID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{DB: db, Identity: ident, Catalog: cat, Ingestion: ingestion.New(db, cat, assets.Probe{}), Hosted: control})
	request := func(method, path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	// Store one image directly: the digest of its own bytes, served from the
	// state directory's guide-images root.
	image := bytes.Repeat([]byte{7}, 1024)
	sum := sha256.Sum256(image)
	digest := hex.EncodeToString(sum[:])
	if _, err = db.Exec(`INSERT INTO live_programme_images(url,digest,media_type,width,height,stored_ms) VALUES(?,?,?,?,?,?)`, "https://img.invalid/kept.png", digest, "image/png", 64, 64, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, "guide-images"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "guide-images", digest+".png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/v1/guide/images/"+digest, ""); w.Code != 401 {
		t.Fatalf("no credential answered %d: %s", w.Code, w.Body.String())
	}
	for _, path := range []string{
		"/v1/guide/images/" + strings.Repeat("0", 64),
		"/v1/guide/images/not-a-digest",
		"/v1/guide/images/" + strings.ToUpper(digest),
	} {
		if w := request("GET", path, owner.AccessToken); w.Code != 404 {
			t.Fatalf("GET %s answered %d: %s", path, w.Code, w.Body.String())
		}
	}
	w := request("GET", "/v1/guide/images/"+digest, owner.AccessToken)
	if w.Code != 200 {
		t.Fatalf("stored image answered %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), image) {
		t.Fatal("stored image bytes differ")
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type %q", got)
	}
	if got := w.Header().Get("ETag"); got != `"`+digest+`"` {
		t.Fatalf("etag %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("cache control %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("content type options %q", got)
	}
	if strings.Contains(w.Body.String()+w.Header().Get("ETag"), "img.invalid") {
		t.Fatal("provider URL leaked in response")
	}
}
