package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/playback"
)

// End-to-end over the mux: claim a download, transfer it with Range, and check
// the validators a resumable client depends on. Paths are built with filepath
// throughout so this runs unchanged on Windows.
func TestDownloadTransferSupportsRangeAndValidators(t *testing.T) {
	root := t.TempDir()
	db, e := persistence.Open(filepath.Join(root, "state", "portico.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	id, e := identity.New(db, root)
	if e != nil {
		t.Fatal(e)
	}
	cat := catalog.New(db)
	host, e := hosted.New(db, id, "", "", "")
	if e != nil {
		t.Fatal(e)
	}
	offline, e := downloads.New(downloads.Options{DB: db, OpenSource: func(_ context.Context, name string, _, _ int64) (io.ReadSeekCloser, error) {
		return os.Open(name)
	}})
	if e != nil {
		t.Fatal(e)
	}
	handler := New(Dependencies{DB: db, Identity: id, Catalog: cat, Playback: playback.New(db), Hosted: host, Downloads: offline})
	request := func(method, path, token string, body any) *httptest.ResponseRecorder {
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	secret, e := os.ReadFile(filepath.Join(root, "setup-token"))
	if e != nil {
		t.Fatal(e)
	}
	w := request("POST", "/v1/setup", "", map[string]string{"setupToken": string(secret), "username": "owner", "password": "long-test-password", "name": "Test"})
	if w.Code != 201 {
		t.Fatalf("setup %d %s", w.Code, w.Body.String())
	}
	var auth identity.Envelope
	if e = json.Unmarshal(w.Body.Bytes(), &auth); e != nil {
		t.Fatal(e)
	}
	c := catalogtest.New(t, db)
	lib := c.Library("movies", "Movies", "movie", root)
	film := filepath.Join(root, "Film.mp4")
	body := "0123456789abcdef"
	if e = os.WriteFile(film, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(film)
	if e != nil {
		t.Fatal(e)
	}
	item := c.Movie(lib, film, "Film", 2000)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		asset, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: film, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 60})
		if err != nil {
			return err
		}
		return compactcatalog.LinkAssetTx(ctx, tx, item.ID, asset, compactcatalog.Link{})
	})
	c.Drain()

	w = request("GET", "/v1/items/"+item.Public+"/download-options", auth.AccessToken, nil)
	if w.Code != 200 {
		t.Fatalf("options %d %s", w.Code, w.Body.String())
	}
	var options downloads.DownloadOptionsView
	if e = json.Unmarshal(w.Body.Bytes(), &options); e != nil {
		t.Fatal(e)
	}
	if !options.Policy.AllowDownloads || len(options.Options) == 0 || options.Options[0].Quality != downloads.QualityOriginal {
		t.Fatalf("options %+v", options)
	}

	w = request("POST", "/v1/downloads/preparations", auth.AccessToken, map[string]any{"operationId": "op-http-1", "mediaId": item.Public, "quality": "original"})
	if w.Code != 201 {
		t.Fatalf("prepare %d %s", w.Code, w.Body.String())
	}
	var batch downloads.Batch
	if e = json.Unmarshal(w.Body.Bytes(), &batch); e != nil {
		t.Fatal(e)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("batch %+v", batch)
	}
	preparation := batch.Items[0].ID
	if e = offline.Advance(context.Background()); e != nil {
		t.Fatal(e)
	}
	w = request("GET", "/v1/downloads/preparations/"+preparation, auth.AccessToken, nil)
	if w.Code != 200 {
		t.Fatalf("read %d %s", w.Code, w.Body.String())
	}
	var ready downloads.Preparation
	if e = json.Unmarshal(w.Body.Bytes(), &ready); e != nil {
		t.Fatal(e)
	}
	if ready.State != downloads.StateReady || ready.Artifact.SHA256 == "" {
		t.Fatalf("preparation %+v", ready)
	}

	w = request("POST", "/v1/downloads/preparations/"+preparation+"/grant", auth.AccessToken, map[string]any{"operationId": "op-http-2"})
	if w.Code != 201 {
		t.Fatalf("grant %d %s", w.Code, w.Body.String())
	}
	var grant downloads.Grant
	if e = json.Unmarshal(w.Body.Bytes(), &grant); e != nil {
		t.Fatal(e)
	}

	// The grant is the whole authority: no bearer token is sent here.
	r := httptest.NewRequest("GET", grant.URL, nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != body {
		t.Fatalf("transfer %d %q", w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag != "\""+"sha256-"+ready.Artifact.SHA256+"\"" {
		t.Fatalf("etag %q", etag)
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("accept-ranges %q", w.Header().Get("Accept-Ranges"))
	}

	r = httptest.NewRequest("GET", grant.URL, nil)
	r.Header.Set("Range", "bytes=4-7")
	r.Header.Set("If-Range", etag)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusPartialContent || w.Body.String() != body[4:8] {
		t.Fatalf("range %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Range") != "bytes 4-7/16" {
		t.Fatalf("content-range %q", w.Header().Get("Content-Range"))
	}

	// A receipt is issued, verifies against the published keys, and revalidates.
	w = request("POST", "/v1/downloads/receipts", auth.AccessToken, map[string]any{"operationId": "op-http-3", "preparationIds": []string{preparation}})
	if w.Code != 201 {
		t.Fatalf("receipts %d %s", w.Code, w.Body.String())
	}
	var issued struct {
		Results []downloads.ReceiptOutcome `json:"results"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &issued); e != nil {
		t.Fatal(e)
	}
	if len(issued.Results) != 1 || issued.Results[0].Receipt == nil {
		t.Fatalf("issued %+v", issued)
	}
	w = request("GET", "/v1/downloads/receipt-keys", auth.AccessToken, nil)
	if w.Code != 200 {
		t.Fatalf("keys %d %s", w.Code, w.Body.String())
	}
	var published struct {
		Keys []downloads.ReceiptKey `json:"keys"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &published); e != nil {
		t.Fatal(e)
	}
	if _, e = downloads.VerifyReceipt(published.Keys, *issued.Results[0].Receipt, time.Now().UTC()); e != nil {
		t.Fatalf("published keys do not verify the issued receipt: %v", e)
	}

	// Deferred progress lands in ordinary personal state.
	w = request("POST", "/v1/downloads/progress", auth.AccessToken, map[string]any{
		"operationId": "op-http-4",
		"entries":     []map[string]any{{"itemId": item.Public, "positionSeconds": 12.5, "observedAt": "2026-09-14T09:12:00Z"}},
	})
	if w.Code != 200 {
		t.Fatalf("progress %d %s", w.Code, w.Body.String())
	}
	var receipt downloads.ProgressReceipt
	if e = json.Unmarshal(w.Body.Bytes(), &receipt); e != nil {
		t.Fatal(e)
	}
	if receipt.Applied != 1 || receipt.Entries[0].PositionSeconds != 12.5 {
		t.Fatalf("progress receipt %+v", receipt)
	}

	// Usage reflects the one committed claim.
	w = request("GET", "/v1/downloads/usage", auth.AccessToken, nil)
	if w.Code != 200 {
		t.Fatalf("usage %d %s", w.Code, w.Body.String())
	}
	var usage downloads.Usage
	if e = json.Unmarshal(w.Body.Bytes(), &usage); e != nil {
		t.Fatal(e)
	}
	if usage.ProfileCount != 1 || usage.ProfileBytes != int64(len(body)) {
		t.Fatalf("usage %+v", usage)
	}
	// Existing capability URLs must consult the switch on their next use.
	if _, e = db.Exec(`INSERT INTO profile_restrictions(profile_id,allow_downloads) VALUES(?,0)`, auth.Viewer.ProfileID); e != nil {
		t.Fatal(e)
	}
	w = request("GET", grant.URL, "", nil)
	if w.Code != 403 || tl11DownloadErrorCode(t, w) != "downloads_not_allowed" {
		t.Fatalf("disabled grant %d %s", w.Code, w.Body.String())
	}
	if _, e = db.Exec(`DELETE FROM profile_restrictions WHERE profile_id=?`, auth.Viewer.ProfileID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO profile_restrictions(profile_id,maximum_age,allow_unrated) VALUES(?,13,0)`, auth.Viewer.ProfileID); e != nil {
		t.Fatal(e)
	}
	c.Attributes(item.ID, "contentRating", "R")
	c.Drain()
	w = request("POST", "/v1/downloads/preparations", auth.AccessToken, map[string]any{"operationId": "restricted-http", "mediaId": item.Public, "quality": "original"})
	if w.Code != 404 {
		t.Fatalf("restricted preparation %d %s", w.Code, w.Body.String())
	}
	w = request("POST", "/v1/downloads/preparations/"+preparation+"/grant", auth.AccessToken, map[string]any{"operationId": "restricted-grant"})
	if w.Code != 404 {
		t.Fatalf("restricted grant %d %s", w.Code, w.Body.String())
	}
	w = request("POST", "/v1/downloads/receipts/revalidate", auth.AccessToken, map[string]any{"operationId": "restricted-revalidate", "receiptIds": []string{issued.Results[0].ReceiptID}})
	if w.Code != 200 {
		t.Fatalf("restricted receipt revalidation %d %s", w.Code, w.Body.String())
	}
	var revalidated struct {
		Results []downloads.ReceiptOutcome `json:"results"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &revalidated); e != nil {
		t.Fatal(e)
	}
	if len(revalidated.Results) != 1 || revalidated.Results[0].Outcome != downloads.OutcomeRefused {
		t.Fatalf("restricted receipt %+v", revalidated)
	}

	// An unauthenticated read is refused, and a fabricated grant is not a way in.
	if w = request("GET", "/v1/downloads/preparations", "", nil); w.Code != 401 {
		t.Fatalf("unauthenticated list %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/v1/downloads/artifacts/fabricated", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("fabricated grant %d %s", w.Code, w.Body.String())
	}
}

func tl11DownloadErrorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error %v: %s", err, w.Body.String())
	}
	return body.Error.Code
}
