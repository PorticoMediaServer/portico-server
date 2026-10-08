package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/ingestion"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
	"portico.local/server/internal/storage"
)

// v1Fixture is a real server (router, identity, catalog, playback) with an owner
// and a library of direct-playable films, for Playback Protocol v1 tests.
type v1Fixture struct {
	t             *testing.T
	db            *sql.DB
	id            *identity.Service
	cat           *catalog.Service
	handler       http.Handler
	owner         identity.Envelope
	library       string
	libraryHandle int64
	items         []string
	records       []catalogtest.Item
	names         catalogtest.Names
	catalogTest   *catalogtest.Catalog
	root          string
	v1            *playbackv1.Service
	deps          Dependencies // what the router was built from, with its v1 service
}

func newV1Fixture(t *testing.T, films int) *v1Fixture {
	t.Helper()
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	id, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(db)
	host, err := hosted.New(db, id, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	helperBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := &v1Fixture{t: t, db: db, id: id, cat: cat, root: root, v1: &playbackv1.Service{}, names: catalogtest.Names{}}
	f.deps = Dependencies{DB: db, Identity: id, Catalog: cat, Ingestion: ingestion.New(db, cat, assets.Probe{}), Playback: playback.New(db), Hosted: host, Storage: storage.New(helperBinary), PlaybackV1: f.v1}
	f.handler = New(f.deps)
	f.deps.v1 = f.v1
	secret, err := os.ReadFile(filepath.Join(root, "setup-token"))
	if err != nil {
		t.Fatal(err)
	}
	w := f.raw("POST", "/v1/setup", "", nil, map[string]string{"setupToken": string(secret), "username": "owner", "password": "long-test-password", "name": "Test"})
	if w.Code != 201 {
		t.Fatalf("setup %d %s", w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &f.owner); err != nil {
		t.Fatal(err)
	}
	f.catalogTest = catalogtest.New(t, db)
	f.library = "movies"
	f.libraryHandle = f.catalogTest.Library(f.library, "Movies", "movie", root)
	for i := range films {
		title := fmt.Sprintf("Film %02d", i)
		path := filepath.Join(root, title+".mp4")
		if err = os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
			t.Fatal(err)
		}
		item := createPlaybackMovie(t, f.catalogTest, f.libraryHandle, path, title, 2000, 600, "mp4", "h264", "aac")
		f.records = append(f.records, item)
		f.items = append(f.items, item.Public)
		f.names[fmt.Sprintf("film%02d", i)] = item
	}
	f.catalogTest.Drain()
	if len(f.items) != films {
		t.Fatalf("%d films committed, want %d", len(f.items), films)
	}
	return f
}

func createPlaybackMovie(t *testing.T, c *catalogtest.Catalog, library int64, path, title string, year int, duration float64, container, video, audio string) catalogtest.Item {
	t.Helper()
	item := c.Movie(library, path, title, year)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		asset, token, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: container, VideoCodec: video, AudioCodec: audio, Width: 1920, Height: 1080, Duration: duration})
		if err != nil {
			return err
		}
		if err = compactcatalog.LinkAssetTx(ctx, tx, item.ID, asset, compactcatalog.Link{}); err != nil {
			return err
		}
		item.Asset, item.Token = asset, token
		return nil
	})
	return item
}

func drainPlaybackCatalogue(t *testing.T, db *sql.DB) {
	t.Helper()
	catalogtest.New(t, db).Drain()
}

// createPlaybackCollection uses catalogue writes for scale fixtures too. Every
// entry shares the fixture movie's available media, matching the old queue
// stress fixture while keeping the catalogue itself on the supported API.
func createPlaybackCollection(t *testing.T, f *v1Fixture, n int, keyPrefix, titlePrefix string) (catalogtest.Item, []string) {
	t.Helper()
	c := f.catalogTest
	var root string
	if err := f.db.QueryRow(`SELECT root FROM catalog_libraries WHERE id=?`, f.libraryHandle).Scan(&root); err != nil {
		t.Fatal(err)
	}
	asset := f.records[0].Asset
	var firstID int64
	entityIDs := make([]int64, n)
	var collection catalogtest.Item
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for i := range n {
			title := fmt.Sprintf("%s %07d", titlePrefix, i)
			path := filepath.Join(root, fmt.Sprintf("%s%07d.mp4", keyPrefix, i))
			id, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{Library: f.libraryHandle, Kind: compactcatalog.Movie, Key: compactcatalog.ItemKey(root, path, 0), Title: title, Year: 2000})
			if err != nil {
				return err
			}
			if firstID == 0 {
				firstID = id
			}
			entityIDs[i] = id
			if err = compactcatalog.LinkAssetTx(ctx, tx, id, asset, compactcatalog.Link{}); err != nil {
				return err
			}
		}
		var err error
		collection.ID, _, err = compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{Library: f.libraryHandle, Kind: compactcatalog.Collection, Key: compactcatalog.CollectionKey(keyPrefix), Title: titlePrefix + " collection"})
		if err != nil {
			return err
		}
		if err = compactcatalog.SetFactsTx(ctx, tx, collection.ID, map[string]any{"library_id": f.libraryHandle, "name_key": keyPrefix, "created_at": "2026-01-01T00:00:00.000Z"}); err != nil {
			return err
		}
		for i, id := range entityIDs {
			if err = compactcatalog.SetCollectionMemberTx(ctx, tx, collection.ID, id, fmt.Sprintf("%07d", i), true); err != nil {
				return err
			}
		}
		return nil
	})
	collection.Public = c.Public(collection.ID)
	f.names[keyPrefix] = collection
	rows, err := f.db.Query(`SELECT id,pid(public_id) FROM catalog_entities WHERE library_id=? AND kind=(SELECT id FROM catalog_kinds WHERE name='movie') AND id>=? ORDER BY sort_key,id`, f.libraryHandle, firstID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, n)
	index := 0
	for rows.Next() {
		var id int64
		var public string
		if err = rows.Scan(&id, &public); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		name := fmt.Sprintf("%s%07d", keyPrefix, index)
		f.names[name] = catalogtest.Item{ID: id, Public: public, Asset: asset, Token: f.records[0].Token}
		ids = append(ids, public)
		index++
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if index != n {
		t.Fatalf("created %d %s entries, want %d", index, keyPrefix, n)
	}
	if len(ids) > 0 {
		// Preserve the fixture's public ids in the readable-name map and assert
		// its canonical order matches the collection's source order.
		for i, public := range ids {
			if f.names[fmt.Sprintf("%s%07d", keyPrefix, i)].Public != public {
				t.Fatalf("%s fixture order diverged at %d", keyPrefix, i)
			}
		}
	}
	c.Drain()
	return collection, ids
}

func addPlaybackCollectionMovie(t *testing.T, f *v1Fixture, key, title string, collection catalogtest.Item) catalogtest.Item {
	t.Helper()
	var root string
	if err := f.db.QueryRow(`SELECT root FROM catalog_libraries WHERE id=?`, f.libraryHandle).Scan(&root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, key+".mp4")
	item := f.catalogTest.Entity(compactcatalog.Entity{Library: f.libraryHandle, Kind: compactcatalog.Movie, Key: compactcatalog.ItemKey(root, path, 0), Title: title, Year: 2000}, nil)
	f.catalogTest.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := compactcatalog.LinkAssetTx(ctx, tx, item.ID, f.records[0].Asset, compactcatalog.Link{}); err != nil {
			return err
		}
		return compactcatalog.SetCollectionMemberTx(ctx, tx, collection.ID, item.ID, key, true)
	})
	f.catalogTest.Drain()
	return item
}

func newPlaybackMovieEntity(f *v1Fixture, library int64, key, title string, year int) catalogtest.Item {
	f.t.Helper()
	var root string
	if err := f.db.QueryRow(`SELECT root FROM catalog_libraries WHERE id=?`, library).Scan(&root); err != nil {
		f.t.Fatal(err)
	}
	path := filepath.Join(root, key+".mp4")
	return f.catalogTest.Entity(compactcatalog.Entity{Library: library, Kind: compactcatalog.Movie, Key: compactcatalog.ItemKey(root, path, 0), Title: title, Year: year}, nil)
}

// device issues another device session for the owner (another installation).
func (f *v1Fixture) device(installation string) identity.Envelope {
	f.t.Helper()
	return f.deviceFor(f.owner.Viewer.AccountID, f.owner.Viewer.ProfileID, "owner", installation)
}

// deviceFor issues a device session for any account (another installation).
func (f *v1Fixture) deviceFor(account, profile, role, installation string) identity.Envelope {
	f.t.Helper()
	// Installation ids are long opaque tokens; pad the readable name into one.
	id := (installation + strings.Repeat("0", 40))[:40]
	ctx, err := identity.WithIssuingDevice(context.Background(), identity.DeviceRegistration{InstallationID: id, Name: installation, Platform: "test", App: "test", AppVersion: "1"}, "127.0.0.1")
	if err != nil {
		f.t.Fatal(err)
	}
	gated, err := dbwork.Begin(ctx, f.db, dbwork.ClassSecurityFence)
	if err != nil {
		f.t.Fatal(err)
	}
	defer gated.Rollback()
	env, err := f.id.IssueTx(ctx, gated.Tx(), account, profile, "local", role, 1, time.Time{})
	if err != nil {
		f.t.Fatal(err)
	}
	if err = gated.Commit(); err != nil {
		f.t.Fatal(err)
	}
	return env
}

func (f *v1Fixture) raw(method, path, token string, headers map[string]string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var data []byte
	if body != nil {
		if s, ok := body.(string); ok {
			data = []byte(s)
		} else {
			data, _ = json.Marshal(body)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		ct := "application/json"
		if method == "PATCH" {
			ct = "application/merge-patch+json"
		}
		r.Header.Set("Content-Type", ct)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

// call sends a request as the owner's first device and decodes a JSON reply.
func (f *v1Fixture) call(method, path string, headers map[string]string, body any, want int, out any) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.callAs(f.owner.AccessToken, method, path, headers, body, want, out)
}

func (f *v1Fixture) callAs(token, method, path string, headers map[string]string, body any, want int, out any) *httptest.ResponseRecorder {
	f.t.Helper()
	w := f.raw(method, path, token, headers, body)
	if want != 0 && w.Code != want {
		f.t.Fatalf("%s %s: status %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	if out != nil && w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			f.t.Fatalf("%s %s: %v: %s", method, path, err, w.Body.String())
		}
	}
	return w
}

// v1Code reads the API error code of a reply.
func v1Code(w *httptest.ResponseRecorder) string {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code
}

// webCapabilities is a browser that direct-plays MP4 (H.264/AAC) and plays HLS.
func webCapabilities() map[string]any {
	return map[string]any{
		"form":       "browser",
		"network":    map[string]any{"class": "local"},
		"containers": []map[string]any{{"container": "mp4", "direct": true}, {"container": "webm", "direct": true}},
		"streaming":  map[string]any{"hls": map[string]any{"fmp4": true, "ts": true}, "progressive": true},
		"video":      []map[string]any{{"codec": "h264", "maxBitDepth": 8, "maxWidth": 3840, "maxHeight": 2160, "maxFps": 60}},
		"audio":      []map[string]any{{"codec": "aac", "maxChannels": 6}, {"codec": "mp3", "maxChannels": 2}},
		"subtitles":  []map[string]any{{"format": "webvtt", "render": "native"}},
		"features":   map[string]any{"chapters": true, "embeddedAudioSwitching": true},
	}
}
