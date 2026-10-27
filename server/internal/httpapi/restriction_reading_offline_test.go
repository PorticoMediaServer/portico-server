package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/downloads"
	"portico.local/server/internal/identity"
)

// The Reading adapter receives typed public errors from the services it calls.
// A forged receipt is a definite client refusal, not an internal fault, while
// an unknown internal error stays sanitized.
func TestReadingFailureWritesTypedAPIErrorsAndConcealsUnknown(t *testing.T) {
	w := httptest.NewRecorder()
	readingFailure(w, downloadError(downloads.ErrReceipt))
	if w.Code != 400 || bodyErrorCode(w.Body.String()) != "invalid_request" {
		t.Fatalf("typed receipt refusal: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	readingFailure(w, fmt.Errorf("private-database-path-and-token: %w", errors.New("boom")))
	if w.Code != 500 || bodyErrorCode(w.Body.String()) != "internal" {
		t.Fatalf("unknown internal failure: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-database-path-and-token") {
		t.Fatalf("internal failure leaked its cause: %s", w.Body.String())
	}
}

// Offline progress carries a real signed publication receipt, not a caller's
// item id or an exemption from current content policy. Exercise its actual
// preparation/issue path before hiding the item and then its library.
func readingOfflineRestrictionCheck(t *testing.T) {
	r := newReadingFixture(t, "book", "pdf")
	offline, err := downloads.New(downloads.Options{DB: r.f.db,
		OpenSource: func(ctx context.Context, path string, size, modified int64) (io.ReadSeekCloser, error) {
			return r.d.Storage.OpenPlayback(ctx, path, size, modified)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.d.Downloads = offline
	h := New(r.d)
	ctx := context.Background()
	owner := readingOwnerLogin(t, r)
	profiles, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Offline matrix reader", "blue")
	if err != nil {
		t.Fatal(err)
	}
	var child identity.DirectProfile
	for _, profile := range profiles.Profiles {
		if !profile.Primary && profile.Name == "Offline matrix reader" {
			child = profile
		}
	}
	if child.ID == "" {
		t.Fatal("no real offline reader profile")
	}
	selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
	if err != nil || selected.Session.AccessToken == "" {
		t.Fatal("select offline reader", err)
	}
	token := selected.Session.AccessToken
	w := readingHTTP(t, h, "POST", "/v1/downloads/preparations", token, "", map[string]any{
		"operationId": "matrix-reading-prepare", "mediaId": r.item, "versionId": r.source, "quality": "original",
	})
	var batch downloads.Batch
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &batch) != nil || len(batch.Items) != 1 {
		t.Fatal("actual Reading preparation", w.Code, w.Body.String())
	}
	if err = offline.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	w = readingHTTP(t, h, "POST", "/v1/downloads/receipts", token, "", map[string]any{
		"operationId": "matrix-reading-receipt", "preparationIds": []string{batch.Items[0].ID},
	})
	var issued downloadReceiptResults
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &issued) != nil || len(issued.Results) != 1 || issued.Results[0].Receipt == nil {
		t.Fatal("actual signed Reading receipt", w.Code, w.Body.String())
	}
	receipt := *issued.Results[0].Receipt
	keys, err := offline.ReceiptKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := downloads.VerifyReceipt(keys, receipt, time.Now())
	if err != nil || claims.ItemID != r.item || claims.Publication == nil || claims.Publication.SourceID != r.source || claims.Publication.Format != "pdf" {
		t.Fatal("actual receipt publication binding", err)
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, claims.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	observed := max(time.Now().UnixMilli(), issuedAt.UnixMilli())
	request := func(bearer string, envelope downloads.Receipt, revision int64) *httptest.ResponseRecorder {
		page, progression := int64(0), .25
		return readingHTTP(t, h, "POST", "/v1/reading/offline/progress", bearer, "", ReadingOfflineProgressRequest{
			Receipt: envelope, ExpectedRevision: &revision, StartedAt: observed, ObservedAt: observed,
			Location: ReadingOfflineLocation{Format: "pdf", PageIndex: &page, Progression: &progression},
		})
	}
	assertPrivateRefusal := func(w *httptest.ResponseRecorder) {
		t.Helper()
		if w.Code < 400 || w.Code == 401 || w.Code >= 500 {
			t.Fatalf("valid offline reader refusal: %d %s", w.Code, w.Body.String())
		}
		for _, hidden := range []string{r.item, r.source, r.library, r.path, receipt.ReceiptID, receipt.Payload, receipt.Signature} {
			if hidden != "" && strings.Contains(w.Body.String(), hidden) {
				t.Errorf("offline refusal leaked private identity %q", hidden)
			}
		}
	}
	before := readingRestrictionProgress(t, r)
	if w = request("", receipt, 0); w.Code != 401 {
		t.Fatal("anonymous offline request", w.Code, w.Body.String())
	}
	assertPrivateRefusal(request(r.f.owner.AccessToken, receipt, 0))
	// An attacker cannot retarget the signed payload to either a real hidden id
	// or an absent id. Keep the real signature and require the same definite
	// rejection, rather than accepting a malformed-receipt 500 as a refusal.
	forge := func(item string) downloads.Receipt {
		changed := receipt
		changed.Claims.ItemID = item
		changed.Claims.PreparationID += "-forged"
		raw, err := json.Marshal(changed.Claims)
		if err != nil {
			t.Fatal(err)
		}
		changed.Payload = base64.RawURLEncoding.EncodeToString(raw)
		return changed
	}
	if after := readingRestrictionProgress(t, r); after != before {
		t.Fatal("foreign receipt changed Reading private state")
	}
	var applied ReadingOfflineProgressResponse
	w = request(token, receipt, 0)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &applied) != nil || !applied.Saved || applied.Outcome != "applied" || applied.Progress == nil || applied.Progress.SourceID != r.source || applied.Progress.Revision != 1 {
		t.Fatal("real entitled offline progress control", w.Code, w.Body.String())
	}
	before = readingRestrictionProgress(t, r)
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 1, AllowUnrated: false, AllowDownloads: true}); err != nil {
		t.Fatal(err)
	}
	hidden, absent := request(token, forge(r.item), 1), request(token, forge(identity.Token()), 1)
	assertPrivateRefusal(hidden)
	assertPrivateRefusal(absent)
	if hidden.Code != absent.Code || bodyErrorCode(hidden.Body.String()) != bodyErrorCode(absent.Body.String()) {
		t.Fatal("forged hidden/absent receipt differs", hidden.Code, absent.Code)
	}
	assertPrivateRefusal(request(token, receipt, 1))
	if after := readingRestrictionProgress(t, r); after != before {
		t.Fatal("title-hidden signed receipt changed Reading private state")
	}
	if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 2, AllowUnrated: true, AllowDownloads: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.f.id.EditDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectProfileEdit{ExpectedRevision: child.Revision, Policy: &identity.ProfilePolicy{AllowedLibraries: []string{}}}); err != nil {
		t.Fatal(err)
	}
	assertPrivateRefusal(request(token, receipt, 1))
	if after := readingRestrictionProgress(t, r); after != before {
		t.Fatal("library-hidden signed receipt changed Reading private state")
	}
}
