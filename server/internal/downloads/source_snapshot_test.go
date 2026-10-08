package downloads

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

const sourceSnapshotFilmBody = "portico-download-fixture-bytes"

// sourceSnapshotHarness keeps these tests independently runnable by
// test-only.sh while still building catalogue facts through catalogtest.
type sourceSnapshotHarness struct {
	catalog   *catalogtest.Catalog
	db        *sql.DB
	service   *Service
	viewer    identity.Principal
	item      string
	itemID    int64
	asset     string
	assetID   int64
	assetPath string
	libraryID int64
	root      string
	now       time.Time
}

func newSnapshotHarness(t *testing.T) *sourceSnapshotHarness {
	t.Helper()
	root := t.TempDir()
	assetPath := filepath.Join(root, "Film.mp4")
	if err := os.WriteFile(assetPath, []byte(sourceSnapshotFilmBody), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	c := catalogtest.Open(t)
	library := c.Library("lib", "Movies", "movie", root)
	film := c.Movie(library, assetPath, "Film", 0)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{
			Path: assetPath, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: "mp4",
			VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 60,
		})
		return err
	})
	c.Drain()
	h := &sourceSnapshotHarness{
		catalog: c, db: c.DB,
		viewer: identity.Principal{Viewer: identity.Viewer{AccountID: "account", ProfileID: "profile", ServerID: "server", Authority: "local", Role: "owner"}, Hash: "hash", Epoch: 1},
		item:   film.Public, itemID: film.ID, asset: film.Token, assetID: film.Asset, assetPath: assetPath,
		libraryID: library, root: root, now: time.Unix(1_760_000_000, 0).UTC(),
	}
	service, err := New(Options{DB: c.DB, Now: func() time.Time { return h.now }, OpenSource: func(_ context.Context, name string, _, _ int64) (io.ReadSeekCloser, error) {
		return os.Open(name)
	}})
	if err != nil {
		t.Fatal(err)
	}
	h.service = service
	return h
}

func (h *sourceSnapshotHarness) advance(t *testing.T) {
	t.Helper()
	if err := h.service.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (h *sourceSnapshotHarness) ready(t *testing.T, operation string) Preparation {
	t.Helper()
	batch, err := h.service.Submit(context.Background(), h.viewer, Request{OperationID: operation, MediaID: h.item, Quality: QualityOriginal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("expected one preparation, got %d rejected=%v", len(batch.Items), batch.Rejected)
	}
	h.advance(t)
	ready, err := h.service.Get(context.Background(), h.viewer, batch.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != StateReady {
		t.Fatalf("state %q reason %q", ready.State, ready.Reason)
	}
	return ready
}

func updateSnapshotAssetByToken(t *testing.T, h *sourceSnapshotHarness, token string, change func(*compactcatalog.Asset)) {
	t.Helper()
	var asset compactcatalog.Asset
	err := h.db.QueryRow(`SELECT path,size,modified_ns,container,video_codec,audio_codec,width,height,duration FROM catalog_assets WHERE token=?`, token).Scan(
		&asset.Path, &asset.Size, &asset.ModifiedNS, &asset.Container, &asset.VideoCodec, &asset.AudioCodec, &asset.Width, &asset.Height, &asset.Duration,
	)
	if err != nil {
		t.Fatal(err)
	}
	change(&asset)
	err = dbwork.WithWriteTx(context.Background(), h.db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, _, err := compactcatalog.UpsertAssetTx(context.Background(), tx, asset)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	h.catalog.Drain()
}

func (h *sourceSnapshotHarness) addItem(t *testing.T, key, title string, year int, added string) catalogtest.Item {
	t.Helper()
	item := h.catalog.Entity(compactcatalog.Entity{
		Library: h.libraryID, Kind: compactcatalog.Movie, Key: "fixture:" + key,
		Title: title, Year: year, Added: added,
	}, nil)
	h.catalog.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(ctx, tx, item.ID, h.assetID, compactcatalog.Link{})
	})
	h.catalog.Drain()
	return item
}

func TestLegacyPartialHashNeverMixesSourceRevisions(t *testing.T) {
	h := newSnapshotHarness(t)
	ctx := context.Background()
	batch, err := h.service.Submit(ctx, h.viewer, Request{OperationID: "legacy-checkpoint", MediaID: h.item, Quality: QualityOriginal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := batch.Items[0].ID
	digest := sha256.New()
	digest.Write([]byte(sourceSnapshotFilmBody[:8]))
	checkpoint, err := digest.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.db.Exec(`UPDATE download_preparations SET state='running',artifact_kind='source',artifact_ref=?,bytes_done=8,hash_state=? WHERE id=?`, h.asset, checkpoint, id); err != nil {
		t.Fatal(err)
	}
	replaceOriginal(t, h)
	h.advance(t)
	ready, err := h.service.Get(ctx, h.viewer, id)
	want := sha256.Sum256([]byte(strings.Repeat("x", len(sourceSnapshotFilmBody))))
	if err != nil || ready.State != StateReady || ready.Artifact.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("legacy checkpoint combined revisions: %+v %v", ready, err)
	}
}

func TestSnapshotUsesOneOpenedSourceAcrossPathReplacement(t *testing.T) {
	h := newSnapshotHarness(t)
	opens := 0
	h.service.openSource = func(_ context.Context, path string, _, _ int64) (io.ReadSeekCloser, error) {
		opens++
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		replaceOriginal(t, h)
		return f, nil
	}
	ready := h.ready(t, "replace-during-copy")
	want := sha256.Sum256([]byte(sourceSnapshotFilmBody))
	if opens != 1 || ready.Artifact.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("snapshot source changed: opens=%d digest=%s", opens, ready.Artifact.SHA256)
	}
}

func replaceOriginal(t *testing.T, h *sourceSnapshotHarness) {
	t.Helper()
	name := filepath.Join(h.root, "Film.mp4")
	tmp := name + ".replacement"
	if err := os.WriteFile(tmp, []byte(strings.Repeat("x", len(sourceSnapshotFilmBody))), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, name); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	updateSnapshotAssetByToken(t, h, h.asset, func(a *compactcatalog.Asset) { a.ModifiedNS = info.ModTime().UnixNano() })
}

func TestOriginalSnapshotSurvivesSameSizeReplacementAndRestart(t *testing.T) {
	previous := hashChunk
	hashChunk = 8
	t.Cleanup(func() { hashChunk = previous })
	h := newSnapshotHarness(t)
	ctx := context.Background()
	batch, err := h.service.Submit(ctx, h.viewer, Request{OperationID: "snapshot", MediaID: h.item, Quality: QualityOriginal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := batch.Items[0].ID
	h.advance(t)
	partial, err := h.service.Get(ctx, h.viewer, id)
	if err != nil || partial.Progress.BytesDone != 8 {
		t.Fatalf("partial checkpoint: %+v %v", partial, err)
	}
	replaceOriginal(t, h)
	// A service restart recovers the exact persisted snapshot, not the current
	// asset path. A source opener here would prove accidental source reopening.
	h.service, err = New(Options{DB: h.db, Now: func() time.Time { return h.now }, OpenSource: func(context.Context, string, int64, int64) (io.ReadSeekCloser, error) {
		t.Fatal("resumed hash reopened mutable source")
		return nil, ErrGrant
	}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		h.advance(t)
	}
	ready, err := h.service.Get(ctx, h.viewer, id)
	want := sha256.Sum256([]byte(sourceSnapshotFilmBody))
	if err != nil || ready.State != StateReady || ready.Artifact.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("wrong identity: %+v %v", ready, err)
	}
	grant, err := h.service.IssueGrant(ctx, h.viewer, id, "snapshot-grant")
	if err != nil {
		t.Fatal(err)
	}
	replaceOriginal(t, h)
	for _, offset := range []int64{0, 9} {
		transfer, err := h.service.OpenGrant(ctx, grant.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = transfer.Reader.Seek(offset, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(transfer.Reader)
		transfer.Reader.Close()
		if err != nil || string(body) != sourceSnapshotFilmBody[offset:] || transfer.Artifact.SHA256 != ready.Artifact.SHA256 {
			t.Fatalf("range %d changed bytes: %q %v", offset, body, err)
		}
	}
	// Selecting a different original asset also leaves this already published
	// download's immutable representation intact.
	info, err := os.Stat(h.assetPath)
	if err != nil {
		t.Fatal(err)
	}
	err = dbwork.WithWriteTx(ctx, h.db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		oldID, err := compactcatalog.AssetByTokenTx(ctx, tx, h.asset)
		if err != nil {
			return err
		}
		if err = compactcatalog.UnlinkAssetTx(ctx, tx, h.itemID, oldID); err != nil {
			return err
		}
		altID, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: h.assetPath + ".other", Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 60})
		if err != nil {
			return err
		}
		return compactcatalog.LinkAssetTx(ctx, tx, h.itemID, altID, compactcatalog.Link{})
	})
	if err != nil {
		t.Fatal(err)
	}
	h.catalog.Drain()
	transfer, err := h.service.OpenGrant(ctx, grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer transfer.Reader.Close()
	body, err := io.ReadAll(transfer.Reader)
	if err != nil || string(body) != sourceSnapshotFilmBody {
		t.Fatalf("alternate asset changed bytes: %q %v", body, err)
	}
}

func TestSnapshotRetentionReclaimsBytesAndAbandonedCopies(t *testing.T) {
	h := newSnapshotHarness(t)
	ready := h.ready(t, "snapshot-retention")
	path := filepath.Join(h.service.snapshotRoot, ready.ID+".source")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(h.service.snapshotRoot, ".copy-crashed")
	if err := os.WriteFile(partial, []byte("incomplete"), 0600); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.Add(31 * 24 * time.Hour)
	h.advance(t)
	for _, name := range []string{path, partial} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("retired bytes remain: %s %v", name, err)
		}
	}
}

func TestLegacySourceGrantCannotAdvertiseUnpinnedDigest(t *testing.T) {
	h := newSnapshotHarness(t)
	ready := h.ready(t, "legacy-source")
	grant, err := h.service.IssueGrant(context.Background(), h.viewer, ready.ID, "legacy-grant")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.db.Exec(`UPDATE download_preparations SET source_version_json='' WHERE id=?`, ready.ID); err != nil {
		t.Fatal(err)
	}
	if transfer, err := h.service.OpenGrant(context.Background(), grant.Token); err != ErrGrant || transfer != nil {
		t.Fatalf("legacy grant served: %+v %v", transfer, err)
	}
	var state, reason string
	var revoked int
	if err = h.db.QueryRow(`SELECT state,reason FROM download_preparations WHERE id=?`, ready.ID).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	if err = h.db.QueryRow(`SELECT revoked FROM download_grants WHERE preparation_id=?`, ready.ID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if state != StateUnavailable || reason != ReasonSourceChanged || revoked != 1 {
		t.Fatalf("legacy authority remains: %s %s %d", state, reason, revoked)
	}
}

func TestListMillisecondBoundariesAndConcurrentInsert(t *testing.T) {
	h := newSnapshotHarness(t)
	ctx := context.Background()
	want := map[string]bool{}
	base := h.now.Truncate(time.Second)
	for i, offset := range []time.Duration{700, 800, 800, 900, 1100} {
		id := []string{"ms-a", "ms-b", "ms-c", "ms-d", "ms-e"}[i]
		h.now = base.Add(offset * time.Millisecond)
		fixture := h.addItem(t, id, "Extra", 0, "")
		batch, err := h.service.Submit(ctx, h.viewer, Request{OperationID: id, MediaID: fixture.Public, Quality: QualityOriginal}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want[batch.Items[0].ID] = true
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 10; page++ {
		out, err := h.service.List(ctx, h.viewer, ListQuery{Limit: 2, Cursor: cursor}, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range out.Items {
			if seen[item.ID] || !want[item.ID] {
				t.Fatalf("unexpected/duplicate %s", item.ID)
			}
			seen[item.ID] = true
		}
		if page == 0 {
			h.now = base.Add(2 * time.Second)
			if _, err := h.service.Submit(ctx, h.viewer, Request{OperationID: "concurrent", MediaID: h.item, Quality: QualityOriginal}, nil); err != nil {
				t.Fatal(err)
			}
		}
		cursor = out.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("traversal skipped records: got %d want %d", len(seen), len(want))
	}
}
