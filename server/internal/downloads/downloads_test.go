package downloads

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

// The fixture is one library, one movie and one real file on disk. Every path
// is built with filepath so the tests run identically on Windows, macOS and
// Linux; nothing here concatenates path separators.
type harness struct {
	db           *sql.DB
	service      *Service
	viewer       identity.Principal
	item         string
	itemID       int64
	asset        string
	assetID      int64
	assetPath    string
	show         string
	season       string
	itemIDs      map[string]string
	itemEntities map[string]int64
	root         string
	now          time.Time
}

const filmBody = "portico-download-fixture-bytes"

func newHarness(t *testing.T) *harness {
	t.Helper()
	root := t.TempDir()
	db, e := persistence.Open(filepath.Join(root, "state", "portico.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	film := filepath.Join(root, "Film.mp4")
	if e = os.WriteFile(film, []byte(filmBody), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(film)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('lib','Movies','movie',?)`, root); e != nil {
		t.Fatal(e)
	}
	h := &harness{db: db, root: root, assetPath: film, now: time.Unix(1_760_000_000, 0).UTC()}
	h.item, h.itemID = addCompactFixtureItem(t, db, "lib", compactcatalog.Movie, "item", "Film", 0, "", nil)
	h.asset, h.assetID = addCompactFixtureAsset(t, db, h.itemID, film, info, "mp4", "h264", "aac", 1920, 1080, 60)
	service, e := New(Options{DB: db, Now: func() time.Time { return h.now }, OpenSource: func(_ context.Context, name string, _, _ int64) (io.ReadSeekCloser, error) {
		return os.Open(name)
	}})
	if e != nil {
		t.Fatal(e)
	}
	h.service = service
	h.viewer = identity.Principal{Viewer: identity.Viewer{AccountID: "account", ProfileID: "profile", ServerID: "server", Authority: "local", Role: "owner"}, Hash: "hash", Epoch: 1}
	settleDownloadsProjection(t, db)
	return h
}

func settleDownloadsProjection(t *testing.T, db *sql.DB) {
	t.Helper()
	worker := compactcatalog.NewWorker(db)
	for i := 0; i < 10000; i++ {
		_, err := worker.Step(context.Background(), 100)
		if err != nil {
			t.Fatal(err)
		}
		var pending int
		if err := db.QueryRow(`SELECT (SELECT count(*) FROM catalog_dirty)+(SELECT count(*) FROM catalog_derivations WHERE rebuilding<>0)`).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if pending == 0 {
			return
		}
	}
	t.Fatal("download fixture projection did not settle")
}

// These fixtures seed compact catalogue facts through the production writer
// API so the tests exercise the same ids and derived work as live writes.
func addCompactFixtureItem(t testing.TB, db *sql.DB, library string, kind compactcatalog.Kind, key, title string, year int, added string, facts map[string]any) (string, int64) {
	return addCompactFixtureEntity(t, db, library, kind, key, title, year, added, 0, facts)
}

func addCompactFixtureEntity(t testing.TB, db *sql.DB, library string, kind compactcatalog.Kind, key, title string, year int, added string, parent int64, facts map[string]any) (string, int64) {
	t.Helper()
	var id int64
	var public string
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		lib, err := compactcatalog.LibraryTx(context.Background(), tx, library)
		if err != nil {
			return err
		}
		id, _, err = compactcatalog.UpsertEntityTx(context.Background(), tx, compactcatalog.Entity{Library: lib, Kind: kind, Parent: parent, Key: "fixture:" + key, Title: title, Year: year, Added: added})
		if err != nil {
			return err
		}
		if len(facts) != 0 {
			if err = compactcatalog.SetFactsTx(context.Background(), tx, id, facts); err != nil {
				return err
			}
		}
		return tx.QueryRow(`SELECT pid(public_id) FROM catalog_entities WHERE id=?`, id).Scan(&public)
	})
	if err != nil {
		t.Fatalf("seed compact entity %q: %v", key, err)
	}
	return public, id
}

func addCompactFixtureAsset(t testing.TB, db *sql.DB, item int64, path string, info os.FileInfo, container, video, audio string, width, height int, duration float64) (string, int64) {
	t.Helper()
	var id int64
	var token string
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		assetID, assetToken, err := compactcatalog.UpsertAssetTx(context.Background(), tx, compactcatalog.Asset{Path: path, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: container, VideoCodec: video, AudioCodec: audio, Width: width, Height: height, Duration: duration})
		if err != nil {
			return err
		}
		id, token = assetID, assetToken
		return compactcatalog.LinkAssetTx(context.Background(), tx, item, id, compactcatalog.Link{})
	})
	if err != nil {
		t.Fatalf("seed compact asset %q: %v", path, err)
	}
	return token, id
}

func addCompactFixtureLink(t testing.TB, db *sql.DB, item, asset int64) {
	t.Helper()
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(context.Background(), tx, item, asset, compactcatalog.Link{})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func updateCompactFixtureAssetByToken(t testing.TB, db *sql.DB, token string, change func(*compactcatalog.Asset)) {
	t.Helper()
	var a compactcatalog.Asset
	err := db.QueryRow(`SELECT path,size,modified_ns,container,video_codec,audio_codec,width,height,duration FROM catalog_assets WHERE token=?`, token).Scan(&a.Path, &a.Size, &a.ModifiedNS, &a.Container, &a.VideoCodec, &a.AudioCodec, &a.Width, &a.Height, &a.Duration)
	if err != nil {
		t.Fatal(err)
	}
	change(&a)
	err = dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, _, err := compactcatalog.UpsertAssetTx(context.Background(), tx, a)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func deleteCompactFixtureEntity(t testing.TB, db *sql.DB, id int64) {
	t.Helper()
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error { return compactcatalog.DeleteEntityTx(context.Background(), tx, id) })
	if err != nil {
		t.Fatal(err)
	}
}

func setCompactFixtureTitle(t testing.TB, db *sql.DB, id int64, title string) {
	t.Helper()
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		return compactcatalog.SetFieldsTx(context.Background(), tx, id, compactcatalog.Owner, map[string]any{"title": title})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *harness) advance(t *testing.T) {
	t.Helper()
	if e := h.service.Advance(context.Background()); e != nil {
		t.Fatal(e)
	}
}

// prepare admits one original-quality claim and drives it to ready.
func (h *harness) ready(t *testing.T, operation string) Preparation {
	t.Helper()
	batch, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: operation, MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("expected one preparation, got %d rejected=%v", len(batch.Items), batch.Rejected)
	}
	h.advance(t)
	out, e := h.service.Get(context.Background(), h.viewer, batch.Items[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if out.State != StateReady {
		t.Fatalf("state %q reason %q", out.State, out.Reason)
	}
	return out
}

func TestPreparationReachesReadyWithTheSourceDigest(t *testing.T) {
	h := newHarness(t)
	out := h.ready(t, "op-1")
	sum := sha256.Sum256([]byte(filmBody))
	if out.Artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest %q", out.Artifact.SHA256)
	}
	if out.Artifact.Kind != "source" || out.Artifact.Estimated {
		t.Fatalf("artifact %+v", out.Artifact)
	}
	if out.Progress.BytesTotal != int64(len(filmBody)) || out.Progress.Percent != 100 {
		t.Fatalf("progress %+v", out.Progress)
	}
	if out.ExpiresAt == "" {
		t.Fatal("a ready claim must publish its retention deadline")
	}
	if len(out.Actions) != 1 || out.Actions[0] != ActionRemove {
		t.Fatalf("actions %v", out.Actions)
	}
}

func TestPreparationReplayReturnsTheOriginalBatch(t *testing.T) {
	h := newHarness(t)
	first, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-replay", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	second, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-replay", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !second.Duplicate || len(second.Items) != 1 || second.Items[0].ID != first.Items[0].ID {
		t.Fatalf("replay admitted new work: %+v", second)
	}
	var live int
	if e = h.db.QueryRow(`SELECT count(*) FROM download_preparations`).Scan(&live); e != nil {
		t.Fatal(e)
	}
	if live != 1 {
		t.Fatalf("%d preparations after a replay", live)
	}
	// A different operationId for the same item and quality returns the live
	// claim rather than duplicating it.
	third, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-again", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if third.Accepted != 0 || len(third.Items) != 1 || third.Items[0].ID != first.Items[0].ID {
		t.Fatalf("duplicate claim admitted: %+v", third)
	}
}

func TestActionFencesAndLifecycle(t *testing.T) {
	h := newHarness(t)
	batch, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-life", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	id := batch.Items[0].ID
	current := batch.Items[0]

	// A stale revision is refused and hands back the current row.
	if _, e = h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-stale", Action: ActionPause, ExpectedRevision: current.Revision + 99}); e != ErrConflict {
		t.Fatalf("stale fence accepted: %v", e)
	}
	paused, e := h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-pause", Action: ActionPause, ExpectedRevision: current.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if paused.State != StatePaused {
		t.Fatalf("state %q", paused.State)
	}
	// A paused claim is not advanced by the worker.
	h.advance(t)
	still, e := h.service.Get(context.Background(), h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if still.State != StatePaused {
		t.Fatalf("worker advanced a paused claim to %q", still.State)
	}
	// Pause is not offered from paused.
	if _, e = h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-pause-2", Action: ActionPause, ExpectedRevision: still.Revision}); e != ErrConflict {
		t.Fatalf("illegal action accepted: %v", e)
	}
	resumed, e := h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-resume", Action: ActionResume, ExpectedRevision: still.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if resumed.State != StateQueued {
		t.Fatalf("state %q", resumed.State)
	}
	h.advance(t)
	done, e := h.service.Get(context.Background(), h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if done.State != StateReady {
		t.Fatalf("state %q reason %q", done.State, done.Reason)
	}
	removed, e := h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-remove", Action: ActionRemove, ExpectedRevision: done.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if removed.State != StateCancelled || removed.Reason != ReasonRemoved {
		t.Fatalf("removed %+v", removed)
	}
	// The slot is free again, so a fresh claim is a new row.
	again, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-after-remove", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if again.Accepted != 1 || again.Items[0].ID == id {
		t.Fatalf("removal did not free the claim: %+v", again)
	}
}

func TestRetryReopensAFailedClaim(t *testing.T) {
	h := newHarness(t)
	batch, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-retry", MediaID: h.item, Quality: "1080p"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	id := batch.Items[0].ID
	// Nothing has prepared a 1080p version and no viewer here may order one.
	h.advance(t)
	out, e := h.service.Get(context.Background(), h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if out.State != StateUnavailable || out.Reason != ReasonNotOptimized {
		t.Fatalf("state %q reason %q", out.State, out.Reason)
	}
	if !out.Artifact.Estimated || out.Artifact.Bytes <= 0 {
		t.Fatalf("a rung must publish an estimated size: %+v", out.Artifact)
	}
	retried, e := h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-retry-1", Action: ActionRetry, ExpectedRevision: out.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if retried.State != StateQueued || retried.Reason != "" {
		t.Fatalf("retried %+v", retried)
	}
}

func TestStorageFullRefusesNewPreparations(t *testing.T) {
	h := newHarness(t)
	if _, e := h.service.ApplySettings(context.Background(), h.viewer, SettingsChange{OperationID: "op-settings", ExpectedRevision: 1, MaxPreparedBytes: 4, RetentionDays: 30}); e != nil {
		t.Fatal(e)
	}
	_, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-full", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != ErrStorageFull {
		t.Fatalf("expected storage_full, got %v", e)
	}
	// Lifting the ceiling admits the same request under a new operation id.
	if _, e = h.service.ApplySettings(context.Background(), h.viewer, SettingsChange{OperationID: "op-settings-2", ExpectedRevision: 2, MaxPreparedBytes: 0, RetentionDays: 30}); e != nil {
		t.Fatal(e)
	}
	if _, e = h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-full-2", MediaID: h.item, Quality: QualityOriginal}, nil); e != nil {
		t.Fatal(e)
	}
	usage, e := h.service.Usage(context.Background(), h.viewer, true)
	if e != nil {
		t.Fatal(e)
	}
	if usage.ProfileCount != 1 || usage.ServerBytes != int64(len(filmBody)) {
		t.Fatalf("usage %+v", usage)
	}
	if usage.RemainingBytes != nil {
		t.Fatal("an unlimited ceiling must publish a null remaining")
	}
}

func TestSettingsFenceRefusesAStaleRevision(t *testing.T) {
	h := newHarness(t)
	if _, e := h.service.ApplySettings(context.Background(), h.viewer, SettingsChange{OperationID: "op-s1", ExpectedRevision: 99, MaxPreparedBytes: 0, RetentionDays: 7}); e != ErrConflict {
		t.Fatalf("stale settings write accepted: %v", e)
	}
}

func TestGrantTTLReplayWindowAndRange(t *testing.T) {
	h := newHarness(t)
	ready := h.ready(t, "op-grant")
	grant, e := h.service.IssueGrant(context.Background(), h.viewer, ready.ID, "op-grant-1")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(grant.URL, "/v1/downloads/artifacts/") || grant.Token == "" {
		t.Fatalf("grant %+v", grant)
	}
	// A replayed operationId hands back the same token rather than burning a
	// second window.
	same, e := h.service.IssueGrant(context.Background(), h.viewer, ready.ID, "op-grant-1")
	if e != nil {
		t.Fatal(e)
	}
	if same.Token != grant.Token {
		t.Fatal("a replayed grant request minted a second token")
	}
	transfer, e := h.service.OpenGrant(context.Background(), grant.Token)
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(transfer.Reader)
	transfer.Reader.Close()
	if e != nil || string(body) != filmBody {
		t.Fatalf("body %q %v", body, e)
	}
	// A ranged read seeks against the same artifact; this is what makes a
	// resumed transfer possible.
	transfer, e = h.service.OpenGrant(context.Background(), grant.Token)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transfer.Reader.Seek(5, io.SeekStart); e != nil {
		t.Fatal(e)
	}
	part := make([]byte, 4)
	if _, e = io.ReadFull(transfer.Reader, part); e != nil {
		t.Fatal(e)
	}
	transfer.Reader.Close()
	if string(part) != filmBody[5:9] {
		t.Fatalf("range %q", part)
	}
	// Past the replay window the grant stops working even though its TTL would
	// still allow it.
	h.now = h.now.Add(GrantReplayWindow + time.Minute)
	if _, e = h.service.OpenGrant(context.Background(), grant.Token); e != ErrGrant {
		t.Fatalf("expired replay window accepted: %v", e)
	}
	// A grant never used at all stops at its TTL.
	h.now = h.now.Add(-GrantReplayWindow - time.Minute)
	fresh, e := h.service.IssueGrant(context.Background(), h.viewer, ready.ID, "op-grant-2")
	if e != nil {
		t.Fatal(e)
	}
	h.now = h.now.Add(GrantTTL + time.Second)
	if _, e = h.service.OpenGrant(context.Background(), fresh.Token); e != ErrGrant {
		t.Fatalf("expired grant accepted: %v", e)
	}
	if _, e = h.service.OpenGrant(context.Background(), "not-a-grant"); e != ErrGrant {
		t.Fatalf("unknown grant accepted: %v", e)
	}
}

func TestReceiptSignVerifyRevalidateAndRevoke(t *testing.T) {
	h := newHarness(t)
	ready := h.ready(t, "op-receipt")
	results, e := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-r1", []string{ready.ID})
	if e != nil {
		t.Fatal(e)
	}
	if len(results) != 1 || results[0].Outcome != OutcomeIssued || results[0].Receipt == nil {
		t.Fatalf("results %+v", results)
	}
	envelope := *results[0].Receipt
	keys, e := h.service.ReceiptKeys(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	claims, e := VerifyReceipt(keys, envelope, h.now)
	if e != nil {
		t.Fatalf("verify: %v", e)
	}
	if claims.Artifact.SHA256 != ready.Artifact.SHA256 || claims.ItemID != h.item || claims.Viewer.ProfileID != "profile" {
		t.Fatalf("claims %+v", claims)
	}
	// A tampered payload fails: the signature covers the exact published bytes.
	tampered := envelope
	tampered.Payload = tampered.Payload[:len(tampered.Payload)-4] + "AAAA"
	if _, e = VerifyReceipt(keys, tampered, h.now); e != ErrReceipt {
		t.Fatalf("tampered receipt verified: %v", e)
	}
	// Past its expiry a receipt stops verifying even with a good signature.
	if _, e = VerifyReceipt(keys, envelope, h.now.Add(ReceiptTTL+time.Hour)); e != ErrReceipt {
		t.Fatal("an expired receipt verified")
	}
	// Revalidation renews it.
	h.now = h.now.Add(20 * 24 * time.Hour)
	renewed, e := h.service.Revalidate(context.Background(), h.viewer, "server", "op-r2", []string{envelope.ReceiptID})
	if e != nil {
		t.Fatal(e)
	}
	if len(renewed) != 1 || renewed[0].Outcome != OutcomeRenewed || renewed[0].Receipt == nil {
		t.Fatalf("renewed %+v", renewed)
	}
	if _, e = VerifyReceipt(keys, *renewed[0].Receipt, h.now); e != nil {
		t.Fatalf("renewed receipt does not verify: %v", e)
	}
	// Revocation is immediate, appears in the list, and blocks revalidation.
	if _, e = h.service.Revoke(context.Background(), h.viewer, "op-r3", ReasonRemoved, []string{envelope.ReceiptID}, false); e != nil {
		t.Fatal(e)
	}
	page, e := h.service.Revocations(context.Background(), h.viewer, "", 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 1 || page.Items[0].ReceiptID != envelope.ReceiptID || page.Items[0].Reason != ReasonRemoved {
		t.Fatalf("revocations %+v", page)
	}
	after, e := h.service.Revalidate(context.Background(), h.viewer, "server", "op-r4", []string{envelope.ReceiptID})
	if e != nil {
		t.Fatal(e)
	}
	if after[0].Outcome != OutcomeRefused || after[0].Code != ReasonRevoked {
		t.Fatalf("revoked receipt renewed: %+v", after)
	}
	// Reading from the sequence already seen returns nothing new.
	empty, e := h.service.Revocations(context.Background(), h.viewer, strconv.FormatInt(page.Items[0].Sequence, 10), 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(empty.Items) != 0 {
		t.Fatalf("cursor returned seen entries: %+v", empty)
	}
}

func TestDownloadSwitchRevokesGrantAndReceiptRenewal(t *testing.T) {
	h := newHarness(t)
	ready := h.ready(t, "op-switch-ready")
	grant, err := h.service.IssueGrant(context.Background(), h.viewer, ready.ID, "op-switch-grant")
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-switch-receipt", []string{ready.ID})
	if err != nil || len(receipts) != 1 || receipts[0].Receipt == nil {
		t.Fatalf("initial receipt: %+v %v", receipts, err)
	}
	if _, err = h.db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('account','owner',X'00','profile',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.db.Exec(`INSERT INTO profile_restrictions(profile_id,allow_downloads) VALUES('profile',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.IssueGrant(context.Background(), h.viewer, ready.ID, "op-switch-grant"); err != ErrPolicy {
		t.Fatalf("grant replay survived switch: %v", err)
	}
	if _, err = h.service.OpenGrant(context.Background(), grant.Token); err != ErrPolicy {
		t.Fatalf("open grant survived switch: %v", err)
	}
	if _, err = h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-switch-receipt", []string{ready.ID}); err != ErrPolicy {
		t.Fatalf("receipt replay survived switch: %v", err)
	}
	result, err := h.service.Revalidate(context.Background(), h.viewer, "server", "op-switch-revalidate", []string{receipts[0].Receipt.ReceiptID})
	if err != nil || len(result) != 1 || result[0].Outcome != OutcomeRefused || result[0].Code != ReasonNotAllowed {
		t.Fatalf("receipt renewed after switch: %+v %v", result, err)
	}
}

func TestCancellingAClaimRevokesItsReceipts(t *testing.T) {
	h := newHarness(t)
	ready := h.ready(t, "op-cancel")
	results, e := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-c1", []string{ready.ID})
	if e != nil {
		t.Fatal(e)
	}
	receipt := results[0].ReceiptID
	if _, e = h.service.Act(context.Background(), h.viewer, ready.ID, Command{OperationID: "op-c2", Action: ActionRemove, ExpectedRevision: ready.Revision}); e != nil {
		t.Fatal(e)
	}
	var reason string
	if e = h.db.QueryRow(`SELECT revoked_reason FROM download_receipts WHERE id=?`, receipt).Scan(&reason); e != nil {
		t.Fatal(e)
	}
	if reason != ReasonRemoved {
		t.Fatalf("receipt outlived its preparation: %q", reason)
	}
}

func TestRetentionExpiresAnUnusedReadyClaim(t *testing.T) {
	h := newHarness(t)
	ready := h.ready(t, "op-retention")
	if _, e := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-e1", []string{ready.ID}); e != nil {
		t.Fatal(e)
	}
	if _, e := h.service.ApplySettings(context.Background(), h.viewer, SettingsChange{OperationID: "op-e2", ExpectedRevision: 1, MaxPreparedBytes: 0, RetentionDays: 2}); e != nil {
		t.Fatal(e)
	}
	h.now = h.now.Add(3 * 24 * time.Hour)
	h.advance(t)
	out, e := h.service.Get(context.Background(), h.viewer, ready.ID)
	if e != nil {
		t.Fatal(e)
	}
	if out.State != StateExpired || out.Reason != ReasonRetention {
		t.Fatalf("state %q reason %q", out.State, out.Reason)
	}
	page, e := h.service.Revocations(context.Background(), h.viewer, "", 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 1 || page.Items[0].Reason != ReasonRetention {
		t.Fatalf("expiry did not revoke the receipt: %+v", page)
	}
}

func TestDeferredProgressIsLastWriteWinsByObservation(t *testing.T) {
	h := newHarness(t)
	// Keep observations inside this test movie; duration clamping is tested separately.
	updateCompactFixtureAssetByToken(t, h.db, h.asset, func(a *compactcatalog.Asset) { a.Duration = 3600 })
	settleDownloadsProjection(t, h.db)
	base := h.now
	newer := base.Add(time.Hour)
	older := base.Add(-time.Hour)
	out, e := h.service.SyncProgress(context.Background(), h.viewer, "op-p1", []ProgressEntry{
		{ItemID: h.item, PositionSeconds: 600, ObservedAt: newer.Format(time.RFC3339)},
	})
	if e != nil {
		t.Fatal(e)
	}
	if out.Applied != 1 || out.Entries[0].Outcome != ProgressApplied || out.Entries[0].PositionSeconds != 600 {
		t.Fatalf("first batch %+v", out)
	}
	// An older observation loses and reports the server's current value.
	out, e = h.service.SyncProgress(context.Background(), h.viewer, "op-p2", []ProgressEntry{
		{ItemID: h.item, PositionSeconds: 10, ObservedAt: older.Format(time.RFC3339)},
	})
	if e != nil {
		t.Fatal(e)
	}
	if out.Conflicts != 1 || out.Entries[0].Outcome != ProgressStale || out.Entries[0].PositionSeconds != 600 {
		t.Fatalf("older observation applied %+v", out)
	}
	// A newer one wins, and a watched flag lands in ordinary personal state.
	watched := true
	out, e = h.service.SyncProgress(context.Background(), h.viewer, "op-p3", []ProgressEntry{
		{ItemID: h.item, PositionSeconds: 0, Watched: &watched, ObservedAt: newer.Add(time.Hour).Format(time.RFC3339)},
	})
	if e != nil {
		t.Fatal(e)
	}
	if out.Applied != 1 || !out.Entries[0].Watched {
		t.Fatalf("watched batch %+v", out)
	}
	personal := identity.PersonalKey(h.viewer.Viewer)
	var state string
	if e = h.db.QueryRow(`SELECT state FROM progress_activity WHERE profile_id=? AND item_id=?`, personal, h.itemID).Scan(&state); e != nil {
		t.Fatal(e)
	}
	if state != "ended" {
		t.Fatalf("activity state %q", state)
	}
	// Online activity after the observation wins over a deferred entry.
	if _, e = h.db.Exec(`UPDATE progress_activity SET updated_at=? WHERE profile_id=? AND item_id=?`, base.Add(48*time.Hour).Format(time.RFC3339), personal, h.itemID); e != nil {
		t.Fatal(e)
	}
	out, e = h.service.SyncProgress(context.Background(), h.viewer, "op-p4", []ProgressEntry{
		{ItemID: h.item, PositionSeconds: 42, ObservedAt: base.Add(24 * time.Hour).Format(time.RFC3339)},
	})
	if e != nil {
		t.Fatal(e)
	}
	if out.Conflicts != 1 || out.Entries[0].Outcome != ProgressSuperseded {
		t.Fatalf("online write lost to a stale offline entry: %+v", out)
	}
	// Unreadable and impossible entries are reported, never applied.
	out, e = h.service.SyncProgress(context.Background(), h.viewer, "op-p5", []ProgressEntry{
		{ItemID: h.item, PositionSeconds: 1, ObservedAt: "not-a-time"},
		{ItemID: "missing", PositionSeconds: 1, ObservedAt: base.Format(time.RFC3339)},
		{ItemID: h.item, PositionSeconds: 1, ObservedAt: base.Add(72 * time.Hour).Format(time.RFC3339)},
	})
	if e != nil {
		t.Fatal(e)
	}
	if out.Applied != 0 || out.Conflicts != 3 {
		t.Fatalf("invalid entries applied: %+v", out)
	}
	if out.Entries[0].Outcome != ProgressInvalid || out.Entries[1].Outcome != ProgressMissing || out.Entries[2].Outcome != ProgressFuture {
		t.Fatalf("outcomes %+v", out.Entries)
	}
}

func TestProgressBatchIsIdempotent(t *testing.T) {
	h := newHarness(t)
	entries := []ProgressEntry{{ItemID: h.item, PositionSeconds: 120, ObservedAt: h.now.Format(time.RFC3339)}}
	first, e := h.service.SyncProgress(context.Background(), h.viewer, "op-idem", entries)
	if e != nil {
		t.Fatal(e)
	}
	second, e := h.service.SyncProgress(context.Background(), h.viewer, "op-idem", entries)
	if e != nil {
		t.Fatal(e)
	}
	if second.Applied != first.Applied || second.Conflicts != first.Conflicts {
		t.Fatalf("replay differed: %+v vs %+v", first, second)
	}
}

func TestPolicyRefusesAViewerWithoutDownloads(t *testing.T) {
	h := newHarness(t)
	h.viewer.Authority = "hosted" // The restrictions table is Hosted membership policy.
	if _, e := h.db.Exec(`INSERT INTO restrictions(profile_id,revision,revoked) VALUES('profile',1,1)`); e != nil {
		t.Fatal(e)
	}
	if _, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-policy", MediaID: h.item, Quality: QualityOriginal}, nil); e != ErrPolicy {
		t.Fatalf("a revoked profile was admitted: %v", e)
	}
	view, e := h.service.Options(context.Background(), h.viewer, h.item, true)
	if e != nil {
		t.Fatal(e)
	}
	if view.Policy.AllowDownloads || view.Policy.Reason != ReasonNotAllowed {
		t.Fatalf("policy %+v", view.Policy)
	}
	for _, option := range view.Options {
		if option.Available {
			t.Fatalf("option offered to a viewer who may not download: %+v", option)
		}
	}
}

func TestPolicyRevocationStopsAnAdmittedClaimAndItsReceipts(t *testing.T) {
	h := newHarness(t)
	h.viewer.Authority = "hosted" // Exercise the Hosted revocation, not direct-profile policy.
	ready := h.ready(t, "op-lost")
	if _, e := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-lost-1", []string{ready.ID}); e != nil {
		t.Fatal(e)
	}
	if _, e := h.db.Exec(`INSERT INTO restrictions(profile_id,revision,revoked) VALUES('profile',1,1)`); e != nil {
		t.Fatal(e)
	}
	out, e := h.service.Revalidate(context.Background(), h.viewer, "server", "op-lost-2", []string{})
	if e != ErrInput {
		t.Fatalf("empty revalidation accepted: %v %+v", e, out)
	}
	var receipt string
	if e = h.db.QueryRow(`SELECT id FROM download_receipts LIMIT 1`).Scan(&receipt); e != nil {
		t.Fatal(e)
	}
	out, e = h.service.Revalidate(context.Background(), h.viewer, "server", "op-lost-3", []string{receipt})
	if e != nil {
		t.Fatal(e)
	}
	if out[0].Outcome != OutcomeRefused || out[0].Code != ReasonAccountDisabled {
		t.Fatalf("revalidation outcome %+v", out)
	}
	if _, e = h.service.OpenGrant(context.Background(), "irrelevant"); e != ErrGrant {
		t.Fatal("unknown grant accepted")
	}
}

func TestOptionsPublishTheLadderAndEstimates(t *testing.T) {
	h := newHarness(t)
	view, e := h.service.Options(context.Background(), h.viewer, h.item, true)
	if e != nil {
		t.Fatal(e)
	}
	if view.Options[0].Quality != QualityOriginal || view.Options[0].Estimated {
		t.Fatalf("first option %+v", view.Options[0])
	}
	seen := map[string]QualityOption{}
	for _, option := range view.Options {
		seen[option.Quality] = option
	}
	if _, ok := seen["2160p"]; ok {
		t.Fatal("a rung taller than the source was offered as an upscale")
	}
	rung, ok := seen["720p"]
	if !ok {
		t.Fatalf("720p missing from %v", seen)
	}
	if !rung.Estimated || rung.Bytes <= 0 || rung.PreparedProfileID != "portable-720-v3" {
		t.Fatalf("720p %+v", rung)
	}
	if !rung.RequiresPrepare {
		t.Fatal("an owner must be told the rung needs preparing first")
	}
	if view.Source.Bytes != int64(len(filmBody)) || !view.Source.Available {
		t.Fatalf("source %+v", view.Source)
	}
}

func TestListPagesNewestFirst(t *testing.T) {
	h := newHarness(t)
	var hItemIDs [3]string
	for i, operation := range []string{"op-l1", "op-l2", "op-l3"} {
		h.now = h.now.Add(time.Duration(i+1) * time.Second)
		public, entity := addCompactFixtureItem(t, h.db, "lib", compactcatalog.Movie, operation, "Extra", 0, "", nil)
		addCompactFixtureLink(t, h.db, entity, h.assetID)
		settleDownloadsProjection(t, h.db)
		if _, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: operation, MediaID: public, Quality: QualityOriginal}, nil); e != nil {
			t.Fatal(e)
		}
		hItemIDs[i] = public
	}
	page, e := h.service.List(context.Background(), h.viewer, ListQuery{Limit: 2}, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page %+v", page)
	}
	if page.Items[0].ItemID != hItemIDs[2] || page.Items[1].ItemID != hItemIDs[1] {
		t.Fatalf("order %v %v", page.Items[0].ItemID, page.Items[1].ItemID)
	}
	rest, e := h.service.List(context.Background(), h.viewer, ListQuery{Cursor: page.NextCursor, Limit: 2}, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(rest.Items) != 1 || rest.Items[0].ItemID != hItemIDs[0] || rest.NextCursor != "" {
		t.Fatalf("rest %+v", rest)
	}
	if _, e = h.service.List(context.Background(), h.viewer, ListQuery{State: "nonsense", Limit: 2}, false); e != ErrInput {
		t.Fatal("an unknown state filter was accepted")
	}
	if _, e = h.service.List(context.Background(), h.viewer, ListQuery{Cursor: "bogus", Limit: 2}, false); e != ErrInput {
		t.Fatal("a fabricated cursor was accepted")
	}
}

// PERF-S23: a container is never a synchronous batch target (that path cut a
// season at 100 and knew no shows); containers are durable requests
// (requests_test.go). The batch still resolves the one next episode.
func TestNextEpisodeTargetAndOneFormOnly(t *testing.T) {
	h := newHarness(t)
	_, showID := addCompactFixtureEntity(t, h.db, "lib", compactcatalog.Show, "show", "Show", 0, "", 0, map[string]any{"local_key": "show"})
	seasonIDs := make([]int64, 2)
	for i := range seasonIDs {
		_, seasonIDs[i] = addCompactFixtureEntity(t, h.db, "lib", compactcatalog.Season, fmt.Sprintf("season-%d", i+1), fmt.Sprintf("Season %d", i+1), 0, "", showID, map[string]any{"show_id": showID, "number": i + 1})
	}
	episodes := make([]string, 3)
	for i, spec := range []struct {
		key            string
		season, number int
	}{{"e1", 0, 1}, {"e2", 0, 2}, {"e3", 1, 1}} {
		public, id := addCompactFixtureEntity(t, h.db, "lib", compactcatalog.Episode, spec.key, spec.key, 0, "2020-01-01T00:00:00Z", seasonIDs[spec.season], map[string]any{"show_id": showID, "season_id": seasonIDs[spec.season], "numbering": "seasonal", "number": spec.number, "local_identity_status": "parsed"})
		episodes[i] = public
		addCompactFixtureLink(t, h.db, id, h.assetID)
	}
	settleDownloadsProjection(t, h.db)
	next, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-next", NextAfterMediaID: episodes[1], Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if next.Accepted != 1 || next.Items[0].ItemID != episodes[2] || next.Items[0].Origin != OriginNext {
		t.Fatalf("next %+v", next)
	}
	if _, e = h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-end", NextAfterMediaID: episodes[2], Quality: QualityOriginal}, nil); e != ErrNotFound {
		t.Fatalf("a last episode produced a next: %v", e)
	}
	if _, e = h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-two", MediaID: episodes[0], MediaIDs: []string{episodes[1]}, Quality: QualityOriginal}, nil); e != ErrInput {
		t.Fatal("two target forms were accepted")
	}
	if _, e = h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-bad", MediaID: episodes[0], Quality: "4320p"}, nil); e != ErrInput {
		t.Fatal("an unpublished quality was accepted")
	}
}

func TestMissingSourceIsRejectedPerTarget(t *testing.T) {
	h := newHarness(t)
	lonely, _ := addCompactFixtureItem(t, h.db, "lib", compactcatalog.Movie, "lonely", "No file", 0, "", nil)
	settleDownloadsProjection(t, h.db)
	batch, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-mixed", MediaIDs: []string{h.item, lonely, "ghost"}, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if batch.Accepted != 1 || len(batch.Rejected) != 2 {
		t.Fatalf("batch %+v", batch)
	}
	codes := map[string]string{}
	for _, rejection := range batch.Rejected {
		codes[rejection.ItemID] = rejection.Code
	}
	if codes[lonely] != ReasonItemDeleted || codes["ghost"] != ReasonItemDeleted {
		t.Fatalf("codes %v", codes)
	}
}

func TestResumeContinuesAPartialHash(t *testing.T) {
	// Shrinking the hashing window makes the checkpoint observable: the digest
	// must match a single-pass hash of the whole file even though it was folded
	// in four windows, across a pause and a resume.
	restore := hashChunk
	hashChunk = 8
	t.Cleanup(func() { hashChunk = restore })
	h := newHarness(t)
	big := filepath.Join(h.root, "Big.mp4")
	body := strings.Repeat("portico", 5) // 35 bytes: five windows, last one short
	if e := os.WriteFile(big, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(big)
	if e != nil {
		t.Fatal(e)
	}
	bigItem, bigID := addCompactFixtureItem(t, h.db, "lib", compactcatalog.Movie, "bigitem", "Big", 0, "", nil)
	_, _ = addCompactFixtureAsset(t, h.db, bigID, big, info, "mp4", "h264", "aac", 1920, 1080, 60)
	settleDownloadsProjection(t, h.db)
	batch, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-big", MediaID: bigItem, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	id := batch.Items[0].ID
	h.advance(t)
	partial, e := h.service.Get(context.Background(), h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if partial.State != StateRunning || partial.Progress.BytesDone != 8 {
		t.Fatalf("after one window: %q %+v", partial.State, partial.Progress)
	}
	var checkpoint []byte
	if e = h.db.QueryRow(`SELECT hash_state FROM download_preparations WHERE id=?`, id).Scan(&checkpoint); e != nil {
		t.Fatal(e)
	}
	if len(checkpoint) == 0 {
		t.Fatal("a partial hash left no checkpoint, so a pause would lose the work")
	}
	// Pausing mid-hash keeps the bytes already folded in.
	paused, e := h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-big-pause", Action: ActionPause, ExpectedRevision: partial.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if paused.Progress.BytesDone != 8 {
		t.Fatalf("pause discarded work: %+v", paused.Progress)
	}
	h.advance(t)
	resumed, e := h.service.Act(context.Background(), h.viewer, id, Command{OperationID: "op-big-resume", Action: ActionResume, ExpectedRevision: paused.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if resumed.Progress.BytesDone != 8 {
		t.Fatalf("resume restarted the hash: %+v", resumed.Progress)
	}
	for i := 0; i < 8; i++ {
		h.advance(t)
	}
	out, e := h.service.Get(context.Background(), h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if out.State != StateReady {
		t.Fatalf("state %q reason %q progress %+v", out.State, out.Reason, out.Progress)
	}
	sum := sha256.Sum256([]byte(body))
	if out.Artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("checkpointed digest %q does not match a single-pass hash", out.Artifact.SHA256)
	}
}

func TestADeletedItemRetiresItsReadyClaimAndReceipts(t *testing.T) {
	h := newHarness(t)
	ready := h.ready(t, "op-gone")
	if _, e := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-gone-1", []string{ready.ID}); e != nil {
		t.Fatal(e)
	}
	deleteCompactFixtureEntity(t, h.db, h.itemID)
	h.advance(t)
	out, e := h.service.Get(context.Background(), h.viewer, ready.ID)
	if e != nil {
		t.Fatal(e)
	}
	if out.State != StateUnavailable || out.Reason != ReasonItemDeleted {
		t.Fatalf("state %q reason %q", out.State, out.Reason)
	}
	// The claim no longer counts against the owner's ceiling.
	usage, e := h.service.Usage(context.Background(), h.viewer, false)
	if e != nil {
		t.Fatal(e)
	}
	if usage.ServerBytes != 0 || usage.ServerCount != 0 {
		t.Fatalf("a retired claim still holds storage: %+v", usage)
	}
	page, e := h.service.Revocations(context.Background(), h.viewer, "", 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 1 || page.Items[0].Reason != ReasonItemDeleted {
		t.Fatalf("revocations %+v", page)
	}
}

func TestGrantsAreRefusedBeforeAClaimIsReady(t *testing.T) {
	h := newHarness(t)
	batch, e := h.service.Submit(context.Background(), h.viewer, Request{OperationID: "op-early", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.service.IssueGrant(context.Background(), h.viewer, batch.Items[0].ID, "op-early-1"); e != ErrNotReady {
		t.Fatalf("a grant was minted before the artifact existed: %v", e)
	}
	if _, e = h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-early-2", []string{batch.Items[0].ID}); e != nil {
		t.Fatal(e)
	}
	results, e := h.service.IssueReceipts(context.Background(), h.viewer, "server", "op-early-3", []string{batch.Items[0].ID})
	if e != nil {
		t.Fatal(e)
	}
	if results[0].Outcome != OutcomeRefused || results[0].Receipt != nil {
		t.Fatalf("a receipt vouched for bytes that do not exist: %+v", results)
	}
	// Another viewer cannot read, act on, grant or receipt this claim.
	stranger := identity.Principal{Viewer: identity.Viewer{AccountID: "other", ProfileID: "other", ServerID: "server", Authority: "local", Role: "viewer"}, Hash: "hash", Epoch: 1}
	if _, e = h.service.Get(context.Background(), stranger, batch.Items[0].ID); e != ErrNotFound {
		t.Fatalf("another viewer read the claim: %v", e)
	}
	if _, e = h.service.Act(context.Background(), stranger, batch.Items[0].ID, Command{OperationID: "op-steal", Action: ActionCancel, ExpectedRevision: 1}); e != ErrNotFound {
		t.Fatalf("another viewer acted on the claim: %v", e)
	}
	if _, e = h.service.IssueGrant(context.Background(), stranger, batch.Items[0].ID, "op-steal-2"); e != ErrNotFound {
		t.Fatalf("another viewer took a grant: %v", e)
	}
}

func TestAudioItemsPublishOneOptimizedOption(t *testing.T) {
	h := newHarness(t)
	song := filepath.Join(h.root, "Track.flac")
	if e := os.WriteFile(song, []byte("flac"), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(song)
	if e != nil {
		t.Fatal(e)
	}
	track, trackID := addCompactFixtureItem(t, h.db, "lib", compactcatalog.Track, "track", "Track", 0, "", nil)
	_, _ = addCompactFixtureAsset(t, h.db, trackID, song, info, "flac", "", "flac", 0, 0, 240)
	settleDownloadsProjection(t, h.db)
	view, e := h.service.Options(context.Background(), h.viewer, track, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(view.Options) != 2 {
		t.Fatalf("an audio item published %d options: %+v", len(view.Options), view.Options)
	}
	optimized := view.Options[1]
	if optimized.Label != "AAC 192 kbps" || optimized.TargetHeight != 0 || optimized.MaxVideoBitrateBPS != 0 {
		t.Fatalf("audio option describes video: %+v", optimized)
	}
	if optimized.PreparedProfileID != "audio-aac-v3" {
		t.Fatalf("audio recipe %q", optimized.PreparedProfileID)
	}
	// 192 kbps over four minutes, plus container overhead.
	if optimized.Bytes < 5_500_000 || optimized.Bytes > 6_200_000 {
		t.Fatalf("audio estimate %d", optimized.Bytes)
	}
	// Every rung resolves to the same recipe, so every rung estimates the same.
	for _, rung := range []string{"360p", "720p", "2160p"} {
		other, _ := estimateBytes(rung, source{Kind: "song", Duration: 240})
		if other != optimized.Bytes {
			t.Fatalf("rung %s estimated %d, not %d", rung, other, optimized.Bytes)
		}
	}
}
