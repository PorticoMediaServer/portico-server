package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func playbackAuthorityFixture(t *testing.T) (Dependencies, identity.Principal, string, string) {
	t.Helper()
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "authority.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ident, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('a','A','movie','/a'),('b','B','movie','/b')`); err != nil {
		t.Fatal(err)
	}
	itemA, _ := seedHTTPAPICatalogEntity(t, db, "a", compactcatalog.Movie, "item-a", "A", 0, nil)
	itemB, _ := seedHTTPAPICatalogEntity(t, db, "b", compactcatalog.Movie, "item-b", "B", 0, nil)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	control, err := hosted.New(db, ident, "http://127.0.0.1:1", testHostedRootPin(), testHostedRootID())
	if err != nil {
		t.Fatal(err)
	}
	policy := hosted.Policy{ServerID: ident.ID(), Revision: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Members: []hosted.Member{{AccountID: "account", ProfileID: "profile", Role: "member", AllowedLibraries: []string{"a"}}}}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err = control.Apply(certifiedHostedPolicy(t, private, "transaction-fixture", raw)); err != nil {
		t.Fatal(err)
	}
	envelope, err := issueHostedFixture(t, db, ident, "account", "profile", "member")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := ident.Authenticate(envelope.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	return Dependencies{DB: db, Identity: ident, Hosted: control}, principal, itemA, itemB
}

func TestPlaybackAuthorityUsesOneTransactionForSessionAndItemPolicy(t *testing.T) {
	d, principal, itemA, itemB := playbackAuthorityFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if current, err := d.playbackAuthorityTx(ctx, tx, principal, itemA); err != nil || current != principal {
		t.Fatalf("authorized: %v", err)
	}
	for _, item := range []string{itemB, "missing"} {
		if _, err := d.playbackAuthorityTx(ctx, tx, principal, item); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatalf("item %s: %v", item, err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO restrictions VALUES('profile',1,0,'[]')`); err != nil {
		t.Fatal(err)
	}
	// The changed library restriction is visible to item authority in the same
	// transaction, so the command must be denied before the transaction ends.
	if _, err = d.playbackAuthorityTx(ctx, tx, principal, itemA); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("uncommitted item policy: %v", err)
	}
	if _, err = d.playbackAuthorityTx(ctx, tx, principal, ""); err != nil {
		t.Fatalf("account command: %v", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE authorization_session_families SET revoked=1 WHERE id=(SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, principal.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err = d.playbackAuthorityTx(ctx, tx, principal, ""); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("uncommitted session revocation: %v", err)
	}
}

func TestPlaybackAuthorityRejectsCachedRestrictionAndPolicyChanges(t *testing.T) {
	mutations := map[string]string{
		"library restriction": `INSERT INTO restrictions VALUES('profile',1,0,'[]')`,
		"profile revocation":  `INSERT INTO restrictions VALUES('profile',1,1,'["a"]')`,
		"policy expired":      `UPDATE policy SET expires_at='2000-01-01T00:00:00Z'`,
		"membership removed":  `UPDATE policy SET payload=json_set(payload,'$.members',json('[]'))`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			d, principal, itemA, _ := playbackAuthorityFixture(t)
			tx, err := d.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if _, err = d.playbackAuthorityTx(context.Background(), tx, principal, itemA); !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatalf("changed authority admitted: %v", err)
			}
		})
	}
}

func TestPlaybackAuthorityFailsClosedWithoutDependenciesAndOnCancellation(t *testing.T) {
	d, principal, itemA, _ := playbackAuthorityFixture(t)
	tx, err := d.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, missing := range []Dependencies{{}, {Identity: d.Identity}} {
		if _, err = missing.playbackAuthorityTx(context.Background(), tx, principal, ""); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatalf("missing dependency: %v", err)
		}
	}
	var absent *sql.Tx
	if _, err = d.playbackAuthorityTx(context.Background(), absent, principal, ""); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("nil tx: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = d.playbackAuthorityTx(ctx, tx, principal, itemA); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if err = d.Hosted.AllowedTxContext(ctx, principal, "a", tx); !errors.Is(err, context.Canceled) {
		t.Fatalf("policy cancellation: %v", err)
	}
}
