package downloads

import (
	"context"
	"errors"
	"io"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/operations"
)

// readyDownloadFixture admits one original claim for an arbitrary title and
// drives it to ready, so a test can hold a baseline beside the fenced title.
func readyDownloadFixture(t *testing.T, h *harness, media, operation string) Preparation {
	t.Helper()
	ctx := context.Background()
	batch, e := h.service.Submit(ctx, h.viewer, Request{OperationID: operation, MediaID: media, Quality: QualityOriginal}, nil)
	if e != nil || len(batch.Items) != 1 {
		t.Fatalf("prepare %q: %+v %v", media, batch, e)
	}
	h.advance(t)
	out, e := h.service.Get(ctx, h.viewer, batch.Items[0].ID)
	if e != nil || out.State != StateReady {
		t.Fatalf("claim %q: %+v %v", media, out, e)
	}
	return out
}

// assertNotRemapped is the shared proof that a busy admission stays the typed
// retryable answer instead of becoming a missing, unavailable or refused one.
func assertNotRemapped(t *testing.T, err error, remapped ...error) {
	t.Helper()
	if err == nil {
		t.Fatal("admission behind an admitted mutation was allowed")
	}
	if !errors.Is(err, compactcatalog.ErrEntityMutating) {
		t.Fatalf("admission behind an admitted mutation: %v", err)
	}
	for _, other := range remapped {
		if errors.Is(err, other) {
			t.Fatalf("busy admission was remapped to %v", other)
		}
	}
}

// A grant is the authority to fetch bytes, so a new one and a renewal are both
// admission. Neither may be written behind an admitted catalog mutation, while
// reads, completed replays and an unrelated title are unaffected.
func TestGrantAdmissionFencesCatalogMutation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ready := h.ready(t, "op-auth-grant-ready")
	sibling := readyDownloadFixture(t, h, addDownloadSibling(t, h, "grant-sibling"), "op-auth-grant-sibling-ready")
	first, e := h.service.IssueGrant(ctx, h.viewer, ready.ID, GrantRequest{OperationID: "op-auth-grant-first"})
	if e != nil || first.Token == "" {
		t.Fatalf("baseline grant: %+v %v", first, e)
	}
	if _, e = h.service.IssueGrant(ctx, h.viewer, sibling.ID, GrantRequest{OperationID: "op-auth-grant-sibling-first"}); e != nil {
		t.Fatalf("baseline sibling grant: %v", e)
	}
	grants := downloadCount(t, h.db, `SELECT count(*) FROM download_grants`)
	used := downloadScalar(t, h.db, `SELECT CAST(used_ms AS TEXT) FROM download_preparations WHERE id=?`, ready.ID)
	address := downloadScalar(t, h.db, `SELECT CAST(MAX(expires_ms) AS TEXT) FROM download_grants WHERE preparation_id=? AND revoked=0`, ready.ID)
	ref := beginDownloadMutation(t, h.db, h.itemID, h.item)

	// A brand new address for the owned title is refused and nothing is left.
	_, e = h.service.IssueGrant(ctx, h.viewer, ready.ID, GrantRequest{OperationID: "op-auth-grant-new"})
	assertNotRemapped(t, e, ErrNotFound, ErrNotReady, ErrUnavailable, ErrSourceOffline, ErrPolicy, ErrGrant)
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_grants`); n != grants {
		t.Fatalf("refused grant left %d addresses, want %d", n, grants)
	}
	if now := downloadScalar(t, h.db, `SELECT CAST(used_ms AS TEXT) FROM download_preparations WHERE id=?`, ready.ID); now != used {
		t.Fatalf("refused grant moved the claim's use: %s want %s", now, used)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM console_receipts WHERE scope=? AND key=?`, "downloads-grant:"+operations.ViewerKey(h.viewer), "op-auth-grant-new"); n != 0 {
		t.Fatalf("refused grant saved %d receipts", n)
	}

	// Extending a held address writes fresh authorization too.
	_, e = h.service.IssueGrant(ctx, h.viewer, ready.ID, GrantRequest{OperationID: "op-auth-grant-renew", Renew: first.Token})
	assertNotRemapped(t, e, ErrNotFound, ErrNotReady, ErrUnavailable, ErrSourceOffline, ErrPolicy, ErrGrant)
	if now := downloadScalar(t, h.db, `SELECT CAST(MAX(expires_ms) AS TEXT) FROM download_grants WHERE preparation_id=? AND revoked=0`, ready.ID); now != address {
		t.Fatalf("refused renewal moved the address: %s want %s", now, address)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM console_receipts WHERE scope=? AND key=?`, "downloads-grant:"+operations.ViewerKey(h.viewer), "op-auth-grant-renew"); n != 0 {
		t.Fatalf("refused renewal saved %d receipts", n)
	}

	// A completed replay returns the address it already handed out. It writes
	// no authority and cannot revive a withdrawn one: the row it names is
	// checked when the address is used.
	replay, e := h.service.IssueGrant(ctx, h.viewer, ready.ID, GrantRequest{OperationID: "op-auth-grant-first"})
	if e != nil || replay.Token != first.Token {
		t.Fatalf("completed grant replay behind a mutation: %+v %v", replay, e)
	}

	// Fetching is not admission and is not barred: the pinned bytes still open.
	transfer, e := h.service.OpenGrant(ctx, first.Token)
	if e != nil {
		t.Fatalf("read behind a mutation: %v", e)
	}
	body, e := io.ReadAll(transfer.Reader)
	transfer.Reader.Close()
	if e != nil || string(body) != filmBody {
		t.Fatalf("body %q %v", body, e)
	}

	// The unrelated title admits while the mutation holds the other one.
	if other, e := h.service.IssueGrant(ctx, h.viewer, sibling.ID, GrantRequest{OperationID: "op-auth-grant-sibling-second"}); e != nil || other.Token == "" {
		t.Fatalf("sibling grant behind another title's mutation: %+v %v", other, e)
	}

	finishDownloadMutation(t, h.db, ref, "separated")

	// The terminal release admits the retries of both refused operations.
	fresh, e := h.service.IssueGrant(ctx, h.viewer, ready.ID, GrantRequest{OperationID: "op-auth-grant-new"})
	if e != nil || fresh.Token == "" || fresh.Token == first.Token || fresh.Renewed {
		t.Fatalf("grant retry after terminal release: %+v %v", fresh, e)
	}
	renewed, e := h.service.IssueGrant(ctx, h.viewer, ready.ID, GrantRequest{OperationID: "op-auth-grant-renew", Renew: first.Token})
	if e != nil || !renewed.Renewed || renewed.Token != first.Token {
		t.Fatalf("renewal retry after terminal release: %+v %v", renewed, e)
	}
	if transfer, e = h.service.OpenGrant(ctx, first.Token); e != nil {
		t.Fatalf("renewed address after release: %v", e)
	}
	transfer.Reader.Close()
}

// A receipt is offline authorization, so issuing one and renewing one are both
// admission. Revocation, completed replays and an unrelated title keep working,
// and a revoked receipt is refused, never revived, even while a mutation holds
// the title.
func TestReceiptAdmissionFencesCatalogMutation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ready := h.ready(t, "op-auth-receipt-ready")
	sibling := readyDownloadFixture(t, h, addDownloadSibling(t, h, "receipt-sibling"), "op-auth-receipt-sibling-ready")
	issue := func(t *testing.T, operation string, preparation string) ReceiptOutcome {
		t.Helper()
		out, e := h.service.IssueReceipts(ctx, h.viewer, "server", operation, []string{preparation})
		if e != nil || len(out) != 1 || out[0].Outcome != OutcomeIssued || out[0].Receipt == nil {
			t.Fatalf("issue %q: %+v %v", operation, out, e)
		}
		return out[0]
	}
	first := issue(t, "op-auth-receipt-first", ready.ID)
	siblingFirst := issue(t, "op-auth-receipt-sibling-first", sibling.ID)
	doomed := issue(t, "op-auth-receipt-doomed", ready.ID)
	if _, e := h.service.Revoke(ctx, h.viewer, "op-auth-receipt-revoke", ReasonRemoved, []string{doomed.ReceiptID}, false); e != nil {
		t.Fatalf("baseline revocation: %v", e)
	}
	receipts := downloadCount(t, h.db, `SELECT count(*) FROM download_receipts`)
	envelope := downloadScalar(t, h.db, `SELECT CAST(issued_ms AS TEXT)||'/'||CAST(revision AS TEXT) FROM download_receipts WHERE id=?`, first.ReceiptID)
	revoked := downloadScalar(t, h.db, `SELECT CAST(revoked_ms AS TEXT)||'/'||CAST(revision AS TEXT) FROM download_receipts WHERE id=?`, doomed.ReceiptID)
	ref := beginDownloadMutation(t, h.db, h.itemID, h.item)

	// A new receipt for the owned title is refused and no row is written.
	_, e := h.service.IssueReceipts(ctx, h.viewer, "server", "op-auth-receipt-new", []string{ready.ID})
	assertNotRemapped(t, e, ErrNotFound, ErrNotReady, ErrUnavailable, ErrSourceOffline, ErrPolicy, ErrReceipt)
	if n := downloadCount(t, h.db, `SELECT count(*) FROM download_receipts`); n != receipts {
		t.Fatalf("refused issue left %d receipts, want %d", n, receipts)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM console_receipts WHERE scope=? AND key=?`, "downloads-receipts:"+operations.ViewerKey(h.viewer), "op-auth-receipt-new"); n != 0 {
		t.Fatalf("refused issue saved %d receipts", n)
	}

	// Renewal writes a fresh signed document, so it is refused as well.
	_, e = h.service.Revalidate(ctx, h.viewer, "server", "op-auth-receipt-renew", []string{first.ReceiptID}, nil)
	assertNotRemapped(t, e, ErrNotFound, ErrNotReady, ErrUnavailable, ErrPolicy, ErrReceipt)
	if now := downloadScalar(t, h.db, `SELECT CAST(issued_ms AS TEXT)||'/'||CAST(revision AS TEXT) FROM download_receipts WHERE id=?`, first.ReceiptID); now != envelope {
		t.Fatalf("refused renewal rewrote the receipt: %s want %s", now, envelope)
	}
	if n := downloadCount(t, h.db, `SELECT count(*) FROM console_receipts WHERE scope=? AND key=?`, "downloads-revalidate:"+operations.ViewerKey(h.viewer), "op-auth-receipt-renew"); n != 0 {
		t.Fatalf("refused renewal saved %d receipts", n)
	}

	// Genuine revocation is still decided before admission: the withdrawn
	// receipt answers refused, and stays withdrawn.
	withdrawn, e := h.service.Revalidate(ctx, h.viewer, "server", "op-auth-receipt-withdrawn", []string{doomed.ReceiptID}, nil)
	if e != nil || len(withdrawn) != 1 || withdrawn[0].Outcome != OutcomeRefused || withdrawn[0].Code != ReasonRevoked {
		t.Fatalf("revoked receipt behind a mutation: %+v %v", withdrawn, e)
	}
	if now := downloadScalar(t, h.db, `SELECT CAST(revoked_ms AS TEXT)||'/'||CAST(revision AS TEXT) FROM download_receipts WHERE id=?`, doomed.ReceiptID); now != revoked {
		t.Fatalf("revoked receipt was revived: %s want %s", now, revoked)
	}

	// A completed issue replay answers from the same row without writing new
	// authority; it re-checks the receipt rather than trusting the envelope.
	replay, e := h.service.IssueReceipts(ctx, h.viewer, "server", "op-auth-receipt-first", []string{ready.ID})
	if e != nil || len(replay) != 1 || replay[0].Outcome != OutcomeIssued || replay[0].ReceiptID != first.ReceiptID {
		t.Fatalf("completed receipt replay behind a mutation: %+v %v", replay, e)
	}

	// The unrelated title still issues and renews.
	siblingSecond := issue(t, "op-auth-receipt-sibling-second", sibling.ID)
	siblingRenewed, e := h.service.Revalidate(ctx, h.viewer, "server", "op-auth-receipt-sibling-renew", []string{siblingFirst.ReceiptID}, nil)
	if e != nil || len(siblingRenewed) != 1 || siblingRenewed[0].Outcome != OutcomeRenewed || siblingRenewed[0].Receipt == nil {
		t.Fatalf("sibling renewal behind another title's mutation: %+v %v", siblingRenewed, e)
	}

	// Withdrawing authority is not admission and keeps working while the
	// mutation holds the title.
	if _, e = h.service.Revoke(ctx, h.viewer, "op-auth-receipt-sibling-revoke", ReasonRemoved, []string{siblingSecond.ReceiptID}, false); e != nil {
		t.Fatalf("revocation behind a mutation: %v", e)
	}

	finishDownloadMutation(t, h.db, ref, "separated")

	// The terminal release admits both refused retries.
	fresh, e := h.service.IssueReceipts(ctx, h.viewer, "server", "op-auth-receipt-new", []string{ready.ID})
	if e != nil || len(fresh) != 1 || fresh[0].Outcome != OutcomeIssued || fresh[0].Receipt == nil || fresh[0].ReceiptID == first.ReceiptID {
		t.Fatalf("receipt retry after terminal release: %+v %v", fresh, e)
	}
	renewed, e := h.service.Revalidate(ctx, h.viewer, "server", "op-auth-receipt-renew", []string{first.ReceiptID}, nil)
	if e != nil || len(renewed) != 1 || renewed[0].Outcome != OutcomeRenewed || renewed[0].Receipt == nil {
		t.Fatalf("receipt renewal retry after terminal release: %+v %v", renewed, e)
	}
}
