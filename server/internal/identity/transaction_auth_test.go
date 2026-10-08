package identity

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/persistence"
)

func transactionAuthorityFixture(t *testing.T) (*sql.DB, *Service, Principal) {
	t.Helper()
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('account','owner',X'00','profile',1)`); err != nil {
		t.Fatal(err)
	}
	envelope, err := s.Issue("account", "profile", "local", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Authenticate(envelope.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	return db, s, p
}

func TestReauthorizeTxUsesOwnedTransactionAndSeesRevocation(t *testing.T) {
	db, service, principal := transactionAuthorityFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	current, err := service.ReauthorizeTx(ctx, tx, principal)
	if err != nil || current != principal {
		t.Fatalf("current principal mismatch: %v", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE authorization_session_families SET revoked=1 WHERE id IN (SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, principal.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ReauthorizeTx(ctx, tx, principal); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("uncommitted revocation must deny: %v", err)
	}
}

func TestReauthorizeTxRejectsStalePrincipalAndAccountChanges(t *testing.T) {
	cases := map[string]string{
		"session profile":   `UPDATE authorization_session_families SET profile_id='other'`,
		"session role":      `UPDATE authorization_session_families SET role='viewer'`,
		"session account":   `UPDATE authorization_session_families SET account_id='other'`,
		"session authority": `UPDATE authorization_session_families SET authority='hosted'`,
		"session epoch":     `UPDATE authorization_session_families SET epoch=2`,
		"account epoch":     `UPDATE accounts SET epoch=2`,
		"account profile":   `UPDATE accounts SET profile_id='other'`,
		"expired":           `UPDATE authorization_family_tokens SET expires_at='2000-01-01T00:00:00Z'`,
		"invalid expiry":    `UPDATE authorization_family_tokens SET expires_at='not-a-time'`,
		"deleted session":   `DELETE FROM authorization_family_tokens`,
	}
	for name, mutation := range cases {
		t.Run(name, func(t *testing.T) {
			db, service, principal := transactionAuthorityFixture(t)
			if _, err := db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = service.ReauthorizeTx(context.Background(), tx, principal); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("stale principal authorized: %v", err)
			}
		})
	}
}

func TestReauthorizeTxCancellationAndHostedIdentityBoundary(t *testing.T) {
	db, service, _ := transactionAuthorityFixture(t)
	// Identity does not invent Hosted authority. This unit test supplies the
	// caller-verified horizon explicitly; Hosted claim/policy verification is
	// tested by the hosted package, not by the retired Issue wrapper.
	if _, err := service.Issue("hosted-account", "hosted-profile", "hosted", "member", 1); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Hosted wrapper invented authority: %v", err)
	}
	horizon := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := db.Exec(`INSERT INTO policy(server_id,revision,payload,expires_at) VALUES(?,1,'{}',?)`, service.ID(), horizon.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	issueTx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer issueTx.Rollback()
	issued, err := service.IssueTx(context.Background(), issueTx, "hosted-account", "hosted-profile", "hosted", "member", 1, horizon)
	if err != nil {
		t.Fatal(err)
	}
	if err = issueTx.Commit(); err != nil {
		t.Fatal(err)
	}
	principal, err := service.Authenticate(issued.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = service.ReauthorizeTx(context.Background(), tx, principal); err != nil {
		t.Fatalf("hosted identity without password row: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = service.ReauthorizeTx(ctx, tx, principal); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	wrongServer := principal
	wrongServer.ServerID = "another-server"
	if _, err = service.ReauthorizeTx(context.Background(), tx, wrongServer); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong server accepted: %v", err)
	}
}
