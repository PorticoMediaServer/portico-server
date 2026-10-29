package downloads

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/selectionactions"
)

// beginDownloadMutation admits a real catalog mutation through the production
// writer API, the same call the SeparateFolder retirement owner makes. The
// committed row is the real admitted mutation; nothing here forges a root.
func beginDownloadMutation(t *testing.T, db *sql.DB, entity int64, owner string) compactcatalog.MutationRef {
	t.Helper()
	var ref compactcatalog.MutationRef
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(context.Background(), tx, compactcatalog.MutationIntent{Entity: entity, OwnerKind: "title", OwnerID: owner, OwnerGeneration: 1, Mode: "separate", SplitSourceID: "source-incarnation", SplitDirectory: "/fixture", SplitObjectID: "object", SplitObjectRevision: "revision"})
		return e
	})
	if err != nil {
		t.Fatalf("admit catalog mutation: %v", err)
	}
	if ref.ID == "" || ref.Generation == 0 {
		t.Fatal("catalog mutation was not admitted")
	}
	return ref
}

// finishDownloadMutation retires that mutation through the real terminal
// transition, releasing the entity for admission again.
func finishDownloadMutation(t *testing.T, db *sql.DB, ref compactcatalog.MutationRef, outcome string) {
	t.Helper()
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		_, e := compactcatalog.FinishEntityMutationTx(context.Background(), tx, ref, outcome)
		return e
	})
	if err != nil {
		t.Fatalf("finish catalog mutation: %v", err)
	}
}

// addDownloadSibling adds one real film on disk plus its catalogue rows, so a
// test can prove an unrelated title still admits behind another's mutation.
func addDownloadSibling(t *testing.T, h *harness, key string) string {
	t.Helper()
	path := filepath.Join(h.root, key+".mp4")
	if e := os.WriteFile(path, []byte(filmBody+" "+key), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	public, entity := addCompactFixtureItem(t, h.db, "lib", compactcatalog.Movie, key, key, 0, "", nil)
	addCompactFixtureAsset(t, h.db, entity, path, info, "mp4", "h264", "aac", 1920, 1080, 60)
	settleDownloadsProjection(t, h.db)
	return public
}

func downloadScalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var v string
	if e := db.QueryRow(q, args...).Scan(&v); e != nil {
		t.Fatal(e)
	}
	return v
}

func downloadCount(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if e := db.QueryRow(q, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestDownloadAdmissionFencesCatalogMutation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	baseline, e := h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-base", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil || len(baseline.Items) != 1 {
		t.Fatalf("baseline admission: %+v %v", baseline, e)
	}
	sibling := addDownloadSibling(t, h, "sibling")
	claims := downloadCount(t, h.db, `SELECT count(*) FROM download_preparations`)
	ref := beginDownloadMutation(t, h.db, h.itemID, h.item)
	_, e = h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-busy", MediaID: h.item, Quality: QualityOriginal}, nil)
	if !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("admission behind an admitted mutation: %v", e)
	}
	for _, remapped := range []error{ErrNotFound, ErrUnavailable, ErrSourceOffline, ErrPolicy, ErrConflict, ErrStorageFull, ErrRequestConflict} {
		if errors.Is(e, remapped) {
			t.Fatalf("busy admission was remapped to %v", remapped)
		}
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_preparations`); n != claims {
		t.Fatalf("refused admission created a claim: %d want %d", n, claims)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM console_receipts WHERE scope=? AND key=?`, "downloads-prepare:"+operations.ViewerKey(h.viewer), "fence-busy"); n != 0 {
		t.Fatalf("refused admission saved a receipt: %d", n)
	}
	// A batch that names the busy title answers with the same retryable
	// sentinel and admits none of its other members either.
	if _, e = h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-batch", MediaIDs: []string{sibling, h.item}, Quality: QualityOriginal}, nil); !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("batch admission behind a mutation: %v", e)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_preparations`); n != claims {
		t.Fatalf("refused batch left a claim behind: %d want %d", n, claims)
	}
	// A sibling title outside the mutated entity is unaffected.
	other, e := h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-sibling", MediaID: sibling, Quality: QualityOriginal}, nil)
	if e != nil || other.Accepted != 1 {
		t.Fatalf("sibling admission: %+v %v", other, e)
	}
	finishDownloadMutation(t, h.db, ref, "separated")
	// The terminal transition releases the title, and the refused attempt left
	// no receipt: the same operation id now admits and replays idempotently.
	admitted, e := h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-busy", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil || len(admitted.Items) != 1 || admitted.Duplicate {
		t.Fatalf("admission after terminal release: %+v %v", admitted, e)
	}
	replay, e := h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-busy", MediaID: h.item, Quality: QualityOriginal}, nil)
	if e != nil || !replay.Duplicate || replay.Items[0].ID != admitted.Items[0].ID {
		t.Fatalf("replay after release: %+v %v", replay, e)
	}
}

func TestDownloadRetryFencesCatalogMutation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	batch, e := h.service.Submit(ctx, h.viewer, Request{OperationID: "fence-retry", MediaID: h.item, Quality: "720p-low"}, nil)
	if e != nil || len(batch.Items) != 1 {
		t.Fatalf("retry baseline: %+v %v", batch, e)
	}
	id := batch.Items[0].ID
	h.advance(t)
	ended, e := h.service.Get(ctx, h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if ended.State != StateUnavailable || ended.Reason != ReasonNotOptimized {
		t.Fatalf("state %q reason %q", ended.State, ended.Reason)
	}
	ref := beginDownloadMutation(t, h.db, h.itemID, h.item)
	_, e = h.service.Act(ctx, h.viewer, id, Command{OperationID: "fence-retry-busy", Action: ActionRetry, ExpectedRevision: ended.Revision})
	if !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("retry behind an admitted mutation: %v", e)
	}
	still, e := h.service.Get(ctx, h.viewer, id)
	if e != nil {
		t.Fatal(e)
	}
	if still.State != ended.State || still.Revision != ended.Revision {
		t.Fatalf("refused retry changed the claim: %+v want %+v", still, ended)
	}
	finishDownloadMutation(t, h.db, ref, "separated")
	retried, e := h.service.Act(ctx, h.viewer, id, Command{OperationID: "fence-retry-released", Action: ActionRetry, ExpectedRevision: ended.Revision})
	if e != nil {
		t.Fatalf("retry after terminal release: %v", e)
	}
	if retried.State != StateQueued || retried.Reason != "" {
		t.Fatalf("released retry %+v", retried)
	}
}

func TestContainerRequestFencesCatalogMutation(t *testing.T) {
	h := requestHarness(t, 3)
	ctx := context.Background()
	var show int64
	if e := h.db.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, h.show).Scan(&show); e != nil {
		t.Fatal(e)
	}
	ref := beginDownloadMutation(t, h.db, show, h.show)
	if _, e := h.service.SubmitContainer(ctx, h.viewer, newContainerRequest("fence-container", h.show)); !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("container request behind an admitted mutation: %v", e)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_requests WHERE operation_id=?`, "fence-container"); n != 0 {
		t.Fatalf("refused container request was recorded: %d", n)
	}
	finishDownloadMutation(t, h.db, ref, "separated")
	r, e := h.service.SubmitContainer(ctx, h.viewer, newContainerRequest("fence-container", h.show))
	if e != nil {
		t.Fatalf("container request after terminal release: %v", e)
	}
	drainRequestAdmission(t, h, ctx, r.RequestID)
}

func TestContainerRequestExpansionFencesCatalogMutation(t *testing.T) {
	h := requestHarness(t, 3)
	ctx := context.Background()
	r, e := h.service.SubmitContainer(ctx, h.viewer, newContainerRequest("fence-expansion", h.show))
	if e != nil {
		t.Fatal(e)
	}
	// The capture walk records membership only and commits before admission.
	if e = h.service.advanceRequests(ctx); e != nil {
		t.Fatal(e)
	}
	if state := downloadScalar(t, h.db, `SELECT state FROM download_requests WHERE id=?`, r.RequestID); state != "admitting" {
		t.Fatalf("request state %q after capture", state)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_request_items WHERE request_id=?`, r.RequestID); n != 3 {
		t.Fatalf("captured members %d, want 3", n)
	}
	ref := beginDownloadMutation(t, h.db, h.itemEntities["ep-0003"], h.itemIDs["ep-0003"])
	var advanceErr error
	for i := 0; i < 5 && advanceErr == nil; i++ {
		advanceErr = h.service.advanceRequests(ctx)
	}
	if !errors.Is(advanceErr, compactcatalog.ErrEntityMutating) {
		t.Fatalf("expansion behind an admitted mutation: %v", advanceErr)
	}
	// Admission is refused as a whole transaction: a busy member does not
	// become a rejected member, and no sibling member is left half-admitted.
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_preparations`); n != 0 {
		t.Fatalf("partial admission behind a mutation: %d claims", n)
	}
	if state := downloadScalar(t, h.db, `SELECT state FROM download_requests WHERE id=?`, r.RequestID); state != "admitting" {
		t.Fatalf("refused request changed state to %q", state)
	}
	finishDownloadMutation(t, h.db, ref, "separated")
	drainRequestAdmission(t, h, ctx, r.RequestID)
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_preparations`); n != 3 {
		t.Fatalf("released request admitted %d claims, want 3", n)
	}
}

func TestSelectionDownloadAdmissionFencesCatalogMutation(t *testing.T) {
	h := selectionDownloadHarness(t, 0)
	target := selectionDownloadTarget(t, h, h.item, h.itemID)
	ref := beginDownloadMutation(t, h.db, h.itemID, "confirmed-selection-fence")
	checkpointed := false
	apply := func() error {
		return h.service.ApplySelectionDownload(context.Background(), h.viewer, "download", selectionDownloadOptions(), target, nil, selectionDownloadAuthorize, func(*sql.Tx, selectionactions.Effect) error { checkpointed = true; return nil })
	}
	if err := apply(); !errors.Is(err, compactcatalog.ErrEntityMutating) || checkpointed {
		t.Fatalf("confirmed action admitted behind custody: %v checkpoint=%v", err, checkpointed)
	}
	selectionDownloadCounts(t, h, 0, 0)
	finishDownloadMutation(t, h.db, ref, "separated")
	if err := apply(); err != nil || !checkpointed {
		t.Fatal(err, checkpointed)
	}
	selectionDownloadCounts(t, h, 1, 1)
}
