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
	"strings"
	"testing"
	"time"

	trust "portico.local/server/internal/hostedtrust"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func hostedFamilyPolicyFixture(t *testing.T) (*sql.DB, *Service, *identity.Service, Policy, func(any) error) {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.Open(filepath.Join(dir, "family-policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	id, err := identity.New(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(db, id, "", base64.RawURLEncoding.EncodeToString(pub), trust.KeyID(pub))
	if err != nil {
		t.Fatal(err)
	}
	p := Policy{ServerID: id.ID(), Revision: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), Members: []Member{
		{AccountID: "account", ProfileID: "profile", Role: "member", AllowedLibraries: []string{"a", "b"}},
		{AccountID: "peer", ProfileID: "peer-profile", Role: "member", AllowedLibraries: []string{"a"}},
	}}
	apply := func(next any) error {
		raw, err := json.Marshal(next)
		if err != nil {
			return err
		}
		return s.Apply(Signed{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, raw)), KeyID: "family-policy", Certificate: testSigningCertificate(key, "family-policy")})
	}
	if err = apply(p); err != nil {
		t.Fatal(err)
	}
	return db, s, id, p, apply
}

func TestHostedFamilyRevocationRemovalRoleAndRegrant(t *testing.T) {
	for _, reason := range []string{"removed", "role"} {
		t.Run(reason, func(t *testing.T) {
			_, s, id, p, apply := hostedFamilyPolicyFixture(t)
			first, err := s.issueAttachmentFamily(context.Background(), "account", "profile", 1)
			if err != nil {
				t.Fatal(err)
			}
			second, err := s.issueAttachmentFamily(context.Background(), "account", "profile", 1)
			if err != nil {
				t.Fatal(err)
			}
			peer, err := s.issueAttachmentFamily(context.Background(), "peer", "peer-profile", 1)
			if err != nil {
				t.Fatal(err)
			}
			original := append([]Member(nil), p.Members...)
			p.Revision++
			if reason == "removed" {
				p.Members = p.Members[1:]
			} else {
				p.Members[0].Role = "owner"
			}
			if err = apply(p); err != nil {
				t.Fatal(err)
			}
			for _, e := range []identity.Envelope{first, second} {
				if _, err = id.Authenticate(e.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
					t.Fatalf("lost scope remained authorized: %v", err)
				}
			}
			if _, err = id.Authenticate(peer.AccessToken); err != nil {
				t.Fatal("peer revoked", err)
			}
			p.Revision++
			p.Members = original
			if err = apply(p); err != nil {
				t.Fatal(err)
			}
			if _, err = id.Authenticate(first.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatal("regrant resurrected old family", err)
			}
			fresh, err := s.issueAttachmentFamily(context.Background(), "account", "profile", p.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.SessionFamilyID == first.SessionFamilyID {
				t.Fatal("regrant reused old family")
			}
			if _, err = id.Authenticate(fresh.AccessToken); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHostedFamilyRevocationNarrowingRestrictionAndRollback(t *testing.T) {
	t.Run("narrowing and explicit restriction", func(t *testing.T) {
		_, s, id, p, apply := hostedFamilyPolicyFixture(t)
		e, err := s.issueAttachmentFamily(context.Background(), "account", "profile", 1)
		if err != nil {
			t.Fatal(err)
		}
		peer, err := s.issueAttachmentFamily(context.Background(), "peer", "peer-profile", 1)
		if err != nil {
			t.Fatal(err)
		}
		p.Revision++
		p.Members[0].AllowedLibraries = []string{"b"}
		if err = apply(p); err != nil {
			t.Fatal(err)
		}
		principal, err := id.Authenticate(e.AccessToken)
		if err != nil {
			t.Fatal("library narrowing revoked entire family", err)
		}
		if err = s.Allowed(principal, "a"); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatal("removed library still allowed", err)
		}
		if err = s.Allowed(principal, "b"); err != nil {
			t.Fatal("retained library denied", err)
		}
		if _, err = id.Authenticate(peer.AccessToken); err != nil {
			t.Fatal("peer revoked", err)
		}
	})
	t.Run("atomic policy rollback", func(t *testing.T) {
		db, s, id, p, apply := hostedFamilyPolicyFixture(t)
		e, err := s.issueAttachmentFamily(context.Background(), "account", "profile", 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`CREATE TRIGGER reject_family_revoke BEFORE UPDATE OF revoked ON authorization_session_families WHEN NEW.revoked=1 BEGIN SELECT RAISE(ABORT,'controlled family failure');END`); err != nil {
			t.Fatal(err)
		}
		p.Revision++
		p.Members = p.Members[1:]
		if err = apply(p); err == nil {
			t.Fatal("controlled revoke failure ignored")
		}
		var revision int64
		if err = db.QueryRow(`SELECT revision FROM policy`).Scan(&revision); err != nil || revision != 1 {
			t.Fatal("partial policy escaped rollback", revision, err)
		}
		if _, err = id.Authenticate(e.AccessToken); err != nil {
			t.Fatal("rolled-back policy revoked family", err)
		}
	})
}

// Go policy decoding owns authority semantics even for ambiguous signed JSON.
func TestHostedFamilyRevocationUsesDecodedPolicy(t *testing.T) {
	for _, mode := range []string{"members", "member field"} {
		t.Run(mode, func(t *testing.T) {
			_, s, id, p, apply := hostedFamilyPolicyFixture(t)
			e, err := s.issueAttachmentFamily(context.Background(), "account", "profile", 1)
			if err != nil {
				t.Fatal(err)
			}
			p.Revision++
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "members" {
				raw = append(raw[:len(raw)-1], []byte(`,"members":[]}`)...)
			} else {
				raw = []byte(strings.Replace(string(raw), `"accountId":"account"`, `"accountId":"account","accountId":"other"`, 1))
			}
			if err = apply(json.RawMessage(raw)); err != nil {
				t.Fatal(err)
			}
			if _, err = id.Authenticate(e.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatal("decoded removal failed to revoke", err)
			}
			p.Revision++
			if err = apply(p); err != nil {
				t.Fatal(err)
			}
			if _, err = id.Authenticate(e.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatal("ambiguous policy permitted later revival", err)
			}
		})
	}
}
