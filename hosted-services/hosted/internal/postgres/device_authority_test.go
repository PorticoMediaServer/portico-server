package postgres_test

import (
	"context"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"testing"
	"time"
)

func TestDeviceDecisionRejectsRevokedApproverAndExpiredLockWait(t *testing.T) {
	store, svc, ids, a, _ := activation(t)
	ctx := context.Background()
	auth := startTV(t, svc, 1)
	preview, e := svc.Review(ctx, a.Account.ID, "192.0.2.2", auth.UserCode)
	if e != nil {
		t.Fatal(e)
	}
	expected, e := store.CaptureClaimAuthority(ctx, security.Hash(a.AccessToken))
	if e != nil {
		t.Fatal(e)
	}
	// Capture followed by logout models the HTTP authentication/decision gap.
	if e = ids.Revoke(ctx, a.AccessToken); e != nil {
		t.Fatal(e)
	}
	if e = store.DecideDevice(ctx, expected, preview.RequestID, make([]byte, 32), "approve"); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("retired approver", e)
	}
	if e = svc.Decide(ctx, a.AccessToken, "192.0.2.2", preview.RequestID, auth.UserCode, "approve"); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("service accepted retired token", e)
	}
	fresh, e := ids.Login(ctx, "tvowner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	// Expire an access token while its captured expectation waits behind the
	// account lock. Wall-clock expiry must be checked after the lock releases.
	expected, e = store.CaptureClaimAuthority(ctx, security.Hash(fresh.AccessToken))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.Exec(`UPDATE account_sessions SET expires_at=clock_timestamp()+interval '200 milliseconds' WHERE token_hash=$1`, expected.TokenHash); e != nil {
		t.Fatal(e)
	}
	blocker, e := store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback()
	if _, e = blocker.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, fresh.Account.ID); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- store.DecideDevice(ctx, expected, preview.RequestID, make([]byte, 32), "approve") }()
	time.Sleep(300 * time.Millisecond)
	if e = blocker.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("expired after lock", e)
	}
}
