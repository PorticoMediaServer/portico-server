package preparedmedia

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// beginPreparedMutation admits a real catalog mutation through the production
// writer API, exactly as SeparateFolder retirement does. No root or status is
// forged: the row is the one BeginEntityMutationTx writes and commits.
func beginPreparedMutation(t *testing.T, catalog *catalogtest.Catalog, entity int64, owner string) compactcatalog.MutationRef {
	t.Helper()
	var ref compactcatalog.MutationRef
	catalog.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: entity, OwnerKind: "title", OwnerID: owner, OwnerGeneration: 1, Mode: "separate", SplitSourceID: "source-incarnation", SplitDirectory: "/fixture", SplitObjectID: "object", SplitObjectRevision: "revision"})
		return e
	})
	if ref.ID == "" || ref.Generation == 0 {
		t.Fatal("catalog mutation was not admitted")
	}
	return ref
}

// finishPreparedMutation retires that mutation through the real terminal
// transition, releasing the entity for admission again.
func finishPreparedMutation(t *testing.T, catalog *catalogtest.Catalog, ref compactcatalog.MutationRef, outcome string) {
	t.Helper()
	catalog.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, e := compactcatalog.FinishEntityMutationTx(ctx, tx, ref, outcome)
		return e
	})
}

// preparedPendingRequest reads the current source choice but does not submit,
// so a test can admit a mutation between the read and the admission.
func preparedPendingRequest(t *testing.T, s *Service, p identity.Principal, item catalogtest.Item, key string) Request {
	t.Helper()
	v, e := s.View(context.Background(), p, item.Public)
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Sources) != 1 || !v.Sources[0].Available {
		t.Fatalf("source: %+v", v)
	}
	return Request{IdempotencyKey: key, ItemID: item.Public, SourceID: item.Token, ExpectedSourceRevision: v.Sources[0].Revision, ProfileID: "portable-720p-high-v3", TargetID: TargetID}
}

func preparedCount(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if e := db.QueryRow(q, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestPreparedSubmitFencesCatalogMutation(t *testing.T) {
	s, p, catalog, item := preparedFixture(t)
	ctx := context.Background()
	job, e := s.Submit(ctx, p, preparedPendingRequest(t, s, p, item, "fence-baseline"))
	if e != nil {
		t.Fatal(e)
	}
	claims := preparedCount(t, s.db, `SELECT count(*) FROM prepared_media_jobs`)
	ref := beginPreparedMutation(t, catalog, item.ID, item.Public)
	// Cancellation of work already admitted is not a producer admission and is
	// deliberately unfenced: the retirement units' own cancel path must keep
	// working while the mutation owns the title.
	cancelled, e := s.Command(ctx, p, job.ID, "cancel", Command{IdempotencyKey: "fence-cancel", ExpectedRevision: job.Revision})
	if e != nil {
		t.Fatalf("cancel behind an admitted mutation: %v", e)
	}
	if cancelled.State != "cancelled" {
		t.Fatalf("cancelled job %+v", cancelled)
	}
	busy := preparedPendingRequest(t, s, p, item, "fence-busy")
	_, e = s.Submit(ctx, p, busy)
	if !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("admission behind an admitted mutation: %v", e)
	}
	for _, remapped := range []error{ErrSourceChanged, ErrUnavailable, ErrConflict, ErrInput, ErrConfiguration} {
		if errors.Is(e, remapped) {
			t.Fatalf("busy admission was remapped to %v", remapped)
		}
	}
	if n := preparedCount(t, s.db, `SELECT count(*) FROM prepared_media_jobs`); n != claims {
		t.Fatalf("refused admission created a job: %d want %d", n, claims)
	}
	if n := preparedCount(t, s.db, `SELECT count(*) FROM console_receipts WHERE scope=? AND key=?`, "prepared-submit:"+operations.AccountKey(p), "fence-busy"); n != 0 {
		t.Fatalf("refused admission saved a receipt: %d", n)
	}
	finishPreparedMutation(t, catalog, ref, "separated")
	// The terminal transition releases the title, and the refused attempt left
	// no receipt: the very same idempotency key now admits.
	admitted, e := s.Submit(ctx, p, busy)
	if e != nil {
		t.Fatalf("admission after terminal release: %v", e)
	}
	if admitted.ID == "" || admitted.State != "queued" {
		t.Fatalf("released job %+v", admitted)
	}
	if n := preparedCount(t, s.db, `SELECT count(*) FROM prepared_media_jobs`); n != claims+1 {
		t.Fatalf("released admission created %d jobs, want %d", n, claims+1)
	}
}

func TestPreparedAdmissionFenceCoversBoundedAncestors(t *testing.T) {
	s, p, catalog, _ := preparedFixture(t)
	ctx := context.Background()
	movies := catalog.Handle("library")
	sibling := catalog.Movie(movies, "/fixture/sibling.mp4", "Sibling", 2024)
	shows := catalog.Library("shows", "Shows", "tv", "/shows")
	show := catalog.Show(shows, "Show", 2024)
	season := catalog.Season(show, 1)
	episode := catalog.Episode(show, season, 1, "/shows/episode-1.mp4")
	catalog.Drain()
	if _, e := s.Submit(ctx, p, preparedPendingRequest(t, s, p, episode, "ancestor-baseline")); e != nil {
		t.Fatalf("episode baseline admission: %v", e)
	}
	requests := preparedCount(t, s.db, `SELECT count(*) FROM prepared_media_jobs`)
	// The mutation owns the show; the episode is its bounded descendant.
	ref := beginPreparedMutation(t, catalog, show.ID, show.Public)
	blocked := preparedPendingRequest(t, s, p, episode, "ancestor-busy")
	if _, e := s.Submit(ctx, p, blocked); !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("descendant admission behind an ancestor mutation: %v", e)
	}
	if n := preparedCount(t, s.db, `SELECT count(*) FROM prepared_media_jobs`); n != requests {
		t.Fatalf("refused descendant admission created a job: %d want %d", n, requests)
	}
	// A title outside the mutated ancestor is unaffected.
	siblingJob, e := s.Submit(ctx, p, preparedPendingRequest(t, s, p, sibling, "ancestor-sibling"))
	if e != nil || siblingJob.ID == "" {
		t.Fatalf("sibling admission: %+v %v", siblingJob, e)
	}
	finishPreparedMutation(t, catalog, ref, "separated")
	if _, e = s.Submit(ctx, p, blocked); e != nil {
		t.Fatalf("descendant admission after terminal release: %v", e)
	}
}

func TestPreparedRetryFencesCatalogMutation(t *testing.T) {
	s, p, catalog, item := preparedFixture(t)
	ctx := context.Background()
	_, job := requestPrepared(t, s, p, item, "retry-fence")
	execPrepared(t, s.db, `UPDATE prepared_media_jobs SET state='failed',error_code='encoder_failed' WHERE id=?`, job.ID)
	var revision int64
	if e := s.db.QueryRow(`SELECT revision FROM prepared_media_jobs WHERE id=?`, job.ID).Scan(&revision); e != nil {
		t.Fatal(e)
	}
	ref := beginPreparedMutation(t, catalog, item.ID, item.Public)
	_, e := s.Command(ctx, p, job.ID, "retry", Command{IdempotencyKey: "retry-busy", ExpectedRevision: revision})
	if !errors.Is(e, compactcatalog.ErrEntityMutating) {
		t.Fatalf("retry behind an admitted mutation: %v", e)
	}
	if state := scalarPrepared(t, s, `SELECT state FROM prepared_media_jobs WHERE id=?`, job.ID); state != "failed" {
		t.Fatalf("refused retry changed the job to %q", state)
	}
	finishPreparedMutation(t, catalog, ref, "separated")
	retried, e := s.Command(ctx, p, job.ID, "retry", Command{IdempotencyKey: "retry-released", ExpectedRevision: revision})
	if e != nil {
		t.Fatalf("retry after terminal release: %v", e)
	}
	if retried.State != "queued" {
		t.Fatalf("released retry %+v", retried)
	}
}
