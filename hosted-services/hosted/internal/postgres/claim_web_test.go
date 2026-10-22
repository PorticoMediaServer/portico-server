package postgres_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"strings"
	"testing"
	"time"
)

func TestWebClaimApprovalPushAndAccountIsolation(t *testing.T) {
	f := currentClaimJourney(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := claimweb.Request{Purpose: "setup", Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, AccountID: f.expected.AccountID, PublicKey: f.input.PublicKey, Name: "Living Room", Address: "http://192.168.1.10:32500", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(4 * time.Minute)}
	p, e := f.store.RegisterWebClaim(ctx, q, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	other := f.expected
	other.AccountID = "strict-other"
	if _, e = f.store.LookupWebClaim(ctx, other, p.Code); e == nil {
		t.Fatal("other account read claim")
	}
	found, e := f.store.LookupWebClaim(ctx, f.expected, p.Code)
	if e != nil || found.Name != q.Name || found.Address != q.Address || found.RequestedAt.IsZero() {
		t.Fatal(found, e)
	}
	done := make(chan error, 1)
	go func() {
		raw, e := f.store.WaitWebClaim(ctx, q)
		if e == nil && !json.Valid(raw) {
			t.Error("invalid pushed envelope")
		}
		done <- e
	}()
	if e = f.store.DecideWebClaim(ctx, f.expected, p.Code, "claim_profile", "", true, f.service); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("no approval push")
	}
	if e = f.store.DecideWebClaim(ctx, f.expected, p.Code, "claim_profile", "", false, f.service); e == nil {
		t.Fatal("approved claim changed to denial")
	}
}

// Plex-style web claims: the server asks without an account; whoever approves
// the code with their account becomes the owner, and the approval envelope
// names that account.
func TestUnboundWebClaimBindsTheApprover(t *testing.T) {
	f := currentClaimJourney(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := claimweb.Request{Purpose: "setup", Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, Name: "Den", Address: "http://192.168.1.10:32500", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(4 * time.Minute)}
	p, e := f.store.RegisterWebClaim(ctx, q, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	if found, e := f.store.LookupWebClaim(ctx, f.expected, p.Code); e != nil || found.Name != "Den" {
		t.Fatal("the signed-in person holding the code can't see the claim", found, e)
	}
	done := make(chan json.RawMessage, 1)
	go func() {
		raw, e := f.store.WaitWebClaim(ctx, q)
		if e != nil {
			t.Error(e)
		}
		done <- raw
	}()
	// The web page sends no owner profile when the account has only one; the
	// approval then uses the account's own profile (it was refused as
	// invalid_request, so no web claim could finish; demo, 24 Sep).
	if e = f.store.DecideWebClaim(ctx, f.expected, p.Code, "", "", true, f.service); e != nil {
		t.Fatal(e)
	}
	var ownerProfile string
	if e = f.store.DB.QueryRowContext(ctx, `SELECT owner_profile_id FROM claim_operations WHERE id=$1`, q.OperationID).Scan(&ownerProfile); e != nil || ownerProfile != "claim_profile" {
		t.Fatalf("owner profile %q %v", ownerProfile, e)
	}
	var envelope struct {
		Payload string `json:"payload"`
	}
	select {
	case raw := <-done:
		if json.Unmarshal(raw, &envelope) != nil {
			t.Fatal(string(raw))
		}
		// What the server receives must be the envelope's canonical encoding
		// (jsonb storage reorders it; the server refused that as invalid).
		var decoded claims.Envelope
		canonical, _ := json.Marshal(func() claims.Envelope { _ = json.Unmarshal(raw, &decoded); return decoded }())
		if !bytes.Equal(raw, canonical) {
			t.Fatalf("pushed envelope is not canonical:\n%s\n%s", raw, canonical)
		}
	case <-ctx.Done():
		t.Fatal("no approval push")
	}
	payload, e := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if e != nil || !strings.Contains(string(payload), `"accountId":"`+f.expected.AccountID+`"`) {
		t.Fatalf("approval does not bind the approver: %s", payload)
	}
	// After approval the claim belongs to that account: nobody else sees it.
	other := f.expected
	other.AccountID = "strict-other"
	if _, e = f.store.LookupWebClaim(ctx, other, p.Code); e == nil {
		t.Fatal("another account read an approved claim")
	}
}

// A48: pending web claims are capped per server key, so naming a victim's
// account (or none) can't exhaust anyone else's claims.
func TestWebClaimsAreCappedPerServer(t *testing.T) {
	f := currentClaimJourney(t)
	ctx := context.Background()
	for i := range 4 {
		q := claimweb.Request{Purpose: "setup", Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: fmt.Sprintf("op_cap_%d", i), ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, Name: "Den", Address: "http://192.168.1.10:32500", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(4 * time.Minute)}
		_, e := f.store.RegisterWebClaim(ctx, q, make([]byte, 32))
		if i < 3 && e != nil {
			t.Fatal(i, e)
		}
		if i == 3 && e == nil {
			t.Fatal("a fourth live claim for one server was accepted")
		}
	}
}

// A server whose earlier Connect went unanswered re-sends the same operation
// with a newer request. While nobody has decided it, the newer request replaces
// the old one (the demo could never connect again after a cancel that did not
// reach Hosted, 24 Sep); once decided, a different request is stale.
func TestWebClaimNewerRequestReplacesAnUndecidedOne(t *testing.T) {
	f := currentClaimJourney(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q := claimweb.Request{Purpose: "setup", Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, Name: "Den", Address: "http://192.168.1.10:32500", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(4 * time.Minute)}
	first, e := f.store.RegisterWebClaim(ctx, q, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	newer := q
	newer.IssuedAt = q.IssuedAt.Add(time.Minute)
	newer.ExpiresAt = q.ExpiresAt.Add(time.Minute)
	again, e := f.store.RegisterWebClaim(ctx, newer, make([]byte, 32))
	if e != nil {
		t.Fatalf("a newer request for an undecided claim was refused: %v", e)
	}
	if again.Code != first.Code || !again.ExpiresAt.Equal(newer.ExpiresAt) {
		t.Fatalf("replacement kept the code and took the newer expiry: %+v", again)
	}
	if e = f.store.DecideWebClaim(ctx, f.expected, again.Code, "", "", true, f.service); e != nil {
		t.Fatal(e)
	}
	latest := newer
	latest.ExpiresAt = newer.ExpiresAt.Add(time.Minute)
	if _, e = f.store.RegisterWebClaim(ctx, latest, make([]byte, 32)); !errors.Is(e, claims.ErrStale) {
		t.Fatalf("a different request after approval: %v", e)
	}
}

func TestWebClaimV2RefreshCannotReplaceReviewedIntent(t *testing.T) {
	f := currentClaimJourney(t)
	ctx := context.Background()
	q := claimweb.Request{Purpose: "setup", Kind: "portico.claim.web.v2", Audience: "https://hosted.example", OperationID: f.input.OperationID, ServerID: f.input.ServerID, PublicKey: f.input.PublicKey, Name: "Original", Address: "https://server.example:32500", IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(4 * time.Minute)}
	if _, e := f.store.RegisterWebClaim(ctx, q, make([]byte, 32)); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*claimweb.Request){"purpose": func(q *claimweb.Request) {
		q.Purpose = "change-owner"
		q.CurrentOwnerAccountID = "strict-owner"
		q.PreviousOwnerRole = "admin"
	}, "name": func(q *claimweb.Request) { q.Name = "Different" }, "address": func(q *claimweb.Request) { q.Address = "https://attacker.example" }, "account": func(q *claimweb.Request) { q.AccountID = "strict-other" }, "registration": func(q *claimweb.Request) { q.ReturnRegistrationID = "ret_changed" }} {
		t.Run(name, func(t *testing.T) {
			next := q
			next.IssuedAt = q.IssuedAt.Add(time.Second)
			next.ExpiresAt = q.ExpiresAt.Add(time.Second)
			change(&next)
			if _, e := f.store.RegisterWebClaim(ctx, next, make([]byte, 32)); !errors.Is(e, claims.ErrStale) {
				t.Fatal("reviewed intent changed", e)
			}
		})
	}
}
