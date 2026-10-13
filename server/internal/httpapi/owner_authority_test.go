package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/hosted"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
)

func hostedAdministrationFixture(t *testing.T, role string) (Dependencies, identity.Envelope, identity.Principal) {
	t.Helper()
	root := t.TempDir()
	db, err := persistence.Open(filepath.Join(root, "owner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	id, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h, err := hosted.New(db, id, "http://127.0.0.1:1", testHostedRootPin(), testHostedRootID())
	if err != nil {
		t.Fatal(err)
	}
	policy := hosted.Policy{ServerID: id.ID(), Revision: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), Members: []hosted.Member{{AccountID: "account", ProfileID: "profile", Role: role, AllLibraries: true}}}
	raw, _ := json.Marshal(policy)
	if err = h.Apply(certifiedHostedPolicy(t, private, "owner-test", raw)); err != nil {
		t.Fatal(err)
	}
	envelope, err := issuePorticoFixture(t, db, id, "account", "profile", role)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := id.Authenticate(envelope.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	return Dependencies{DB: db, Identity: id, Hosted: h, Console: operations.New(db)}, envelope, principal
}

func TestPorticoOwnerCanReadAdministrationButMemberCannot(t *testing.T) {
	for _, role := range []string{"owner", "member"} {
		t.Run(role, func(t *testing.T) {
			d, session, p := hostedAdministrationFixture(t, role)
			mux := http.NewServeMux()
			// As the production router does: the console routes the registry
			// already serves are not mounted a second time.
			registry, err := FoundationRegistry(d)
			if err != nil {
				t.Fatal(err)
			}
			d.registry = registry
			d.foundationRoutes(mux)
			d.consoleRoutes(mux)
			d.operationsRoutes(mux)
			for _, path := range []string{"/v1/admin/console/settings", "/v1/admin/operations/database"} {
				w := supportRequest(mux, session.AccessToken, path)
				expected := 200
				if role != "owner" {
					expected = 403 // CD-51: a valid session that is not the owner
				}
				if w.Code != expected {
					t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
				}
			}
			r := httptest.NewRequest("GET", "/v1/setup/state", nil)
			r.Header.Set("Authorization", "Bearer "+session.AccessToken)
			_, err = d.localOwner(r)
			if (err == nil) != (role == "owner") {
				t.Fatal("local membership owner boundary", err)
			}
			tx, err := d.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err = d.consoleAuthority(p, true)(context.Background(), tx, "")
			if (err == nil) != (role == "owner") {
				t.Fatal("transaction owner authority", err)
			}
		})
	}
}

func TestPorticoOwnerRechecksMembershipAndFamilyInsideTransaction(t *testing.T) {
	cases := map[string]string{
		"downgraded":      `UPDATE direct_memberships SET role='member' WHERE account_id='account'`,
		"removed":         `DELETE FROM direct_memberships WHERE account_id='account'`,
		"wrong account":   `UPDATE authorization_session_families SET account_id='other'`,
		"wrong profile":   `UPDATE authorization_session_families SET profile_id='other'`,
		"expired token":   `UPDATE authorization_family_tokens SET expires_at='2000-01-01T00:00:00Z'`,
		"revoked profile": `UPDATE direct_profiles SET deleted=1 WHERE id='profile'`,
		"revoked session": `UPDATE authorization_session_families SET revoked=1`,
		"retired token":   `UPDATE authorization_family_tokens SET retired=1`,
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			d, _, p := hostedAdministrationFixture(t, "owner")
			d.DB.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			tx, err := d.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err = d.ownerAuthorityTx(ctx, tx, p); err != nil {
				t.Fatal("initial owner", err)
			}
			if _, err = tx.ExecContext(ctx, query); err != nil {
				t.Fatal(err)
			}
			for _, check := range []func(context.Context, *sql.Tx, identity.Principal) error{d.ownerAuthorityTx, d.manualMetadataAuthorize} {
				if err = check(ctx, tx, p); !errors.Is(err, identity.ErrUnauthorized) {
					t.Fatal("changed authority admitted", err)
				}
			}
		})
	}
}

func TestPorticoOwnerLateRevocationDropsReadResponse(t *testing.T) {
	d, session, _ := hostedAdministrationFixture(t, "owner")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/admin/operations/{panel}", d.operationsHandler(func(ctx context.Context, db *sql.DB, name string) (operations.Panel, error) {
		panel, err := operations.Read(ctx, db, name)
		if _, e := db.Exec(`UPDATE direct_memberships SET role='member' WHERE account_id='account'`); e != nil {
			t.Fatal(e)
		}
		return panel, err
	}))
	w := supportRequest(mux, session.AccessToken, "/v1/admin/operations/memory")
	if w.Code != 401 || strings.Contains(w.Body.String(), "heapObjectsBytes") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPorticoOwnerSettingsChangePersistsAndDowngradeBlocksFurtherWrites(t *testing.T) {
	d, session, _ := hostedAdministrationFixture(t, "owner")
	mux := http.NewServeMux()
	registry, err := FoundationRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	d.registry = registry
	d.foundationRoutes(mux)
	d.consoleRoutes(mux)
	var read struct {
		Data operations.SettingsDocument `json:"data"`
	}
	w := supportRequest(mux, session.AccessToken, "/v1/admin/console/settings")
	if err := json.Unmarshal(w.Body.Bytes(), &read); err != nil {
		t.Fatal(err)
	}
	values := read.Data.Requested
	values.Name = "Owner updated server"
	change := operations.SettingsChange{ExpectedRevision: read.Data.Revision, IdempotencyKey: "owner-settings-update-0001", Values: values}
	apply := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(change)
		r := httptest.NewRequest("PATCH", "/v1/admin/console/settings", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+session.AccessToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w = apply(); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = supportRequest(mux, session.AccessToken, "/v1/admin/console/settings")
	if !strings.Contains(w.Body.String(), "Owner updated server") {
		t.Fatal("setting did not persist", w.Body.String())
	}
	if _, err := d.DB.Exec(`UPDATE direct_memberships SET role='member' WHERE account_id='account'`); err != nil {
		t.Fatal(err)
	}
	if w = apply(); w.Code != 401 {
		t.Fatal("downgraded owner wrote settings", w.Code, w.Body.String())
	}
}
