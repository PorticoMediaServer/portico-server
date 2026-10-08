package hosted

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"portico.local/server/internal/hostedtrust"
	"testing"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func attachmentFamilyFixture(t *testing.T) (*sql.DB, *Service, *identity.Service, time.Time) {
	t.Helper()
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ident, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(db, ident, "http://127.0.0.1:1", base64.RawURLEncoding.EncodeToString(public), trust.KeyID(public))
	if err != nil {
		t.Fatal(err)
	}
	horizon := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	policy := Policy{ServerID: ident.ID(), Revision: 4, IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: horizon.Format(time.RFC3339), Members: []Member{{AccountID: "account", ProfileID: "profile", Role: "member", AllowedLibraries: []string{"a"}}}}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Signed{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, raw)), KeyID: "attachment-fixture", Certificate: testSigningCertificate(private, "attachment-fixture")}); err != nil {
		t.Fatal(err)
	}
	return db, s, ident, horizon
}

func TestAttachmentFamilyIssuesWithinCurrentCachedPolicy(t *testing.T) {
	db, s, ident, horizon := attachmentFamilyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first, err := s.issueAttachmentFamily(ctx, "account", "profile", 4)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.issueAttachmentFamily(ctx, "account", "profile", 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, envelope := range []identity.Envelope{first, second} {
		principal, err := ident.AuthenticateContext(ctx, envelope.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		family, err := ident.SessionFamilyTx(ctx, tx, principal)
		tx.Rollback()
		if err != nil || !family.AuthorizationHorizon.Equal(horizon) {
			t.Fatalf("horizon mismatch: %v", err)
		}
		if envelope.Viewer.AccountID != "account" || envelope.Viewer.ProfileID != "profile" || envelope.Viewer.Authority != "hosted" {
			t.Fatal("attachment scope changed")
		}
	}
	var distinct int
	if err = db.QueryRow(`SELECT count(DISTINCT family_id) FROM authorization_family_tokens`).Scan(&distinct); err != nil || distinct != 2 {
		t.Fatalf("independent attachments merged: %d %v", distinct, err)
	}
}

func TestAttachmentFamilyRejectsStalePolicyAndRestrictionWithoutIssuing(t *testing.T) {
	cases := []struct {
		name, mutation, account, profile string
		revision                         int64
	}{
		{"missing member", "", "other", "profile", 4},
		{"stale revision", "", "account", "profile", 5},
		{"revoked profile", `INSERT INTO restrictions VALUES('profile',1,1,'[]')`, "account", "profile", 4},
		{"expired policy", `UPDATE policy SET expires_at='2000-01-01T00:00:00Z'`, "account", "profile", 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, s, _, _ := attachmentFamilyFixture(t)
			if c.mutation != "" {
				if _, err := db.Exec(c.mutation); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.issueAttachmentFamily(context.Background(), c.account, c.profile, c.revision); !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatalf("invalid attachment admitted: %v", err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM authorization_session_families`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed attachment issued family: %d %v", count, err)
			}
		})
	}
}

func TestAttachmentFamilyHorizonUsesCallerTransactionAndRejectsRoleChange(t *testing.T) {
	db, s, ident, horizon := attachmentFamilyFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	p := identity.Principal{Viewer: identity.Viewer{AccountID: "account", ProfileID: "profile", ServerID: ident.ID(), Authority: "hosted", Role: "member"}}
	if got, err := s.AuthorizationHorizonTx(ctx, tx, p); err != nil || !got.Equal(horizon) {
		t.Fatalf("horizon: %v", err)
	}
	p.Role = "owner"
	if _, err := s.AuthorizationHorizonTx(ctx, tx, p); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("stale role: %v", err)
	}
	p.Role = "member"
	if _, err = tx.ExecContext(ctx, `INSERT INTO restrictions VALUES('profile',1,1,'[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthorizationHorizonTx(ctx, tx, p); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("uncommitted restriction: %v", err)
	}
	cancel()
	if _, err = s.AuthorizationHorizonTx(ctx, tx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
