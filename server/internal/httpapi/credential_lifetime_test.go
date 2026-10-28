package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/access"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// A restore signs everyone out and a backup holds no sign-in, but an API key
// is not a sign-in: what the owner connected keeps working across both. A key
// that was already revoked stays revoked.
func TestAKeyOutlivesABackupAndARestoreAndSessionsDoNot(t *testing.T) {
	f := administrationFixtureNew(t)
	ctx := context.Background()
	key := issueCredential(t, f, "/v1/admin/api-keys", f.owner, `{"operationId":"key-restore-1","name":"Download manager","scopes":["read"]}`)
	token := issueCredential(t, f, "/v1/me/access-tokens", f.member, `{"operationId":"token-restore-1","name":"Member script","scopes":["read"]}`)
	gone := issueCredential(t, f, "/v1/me/access-tokens", f.owner, `{"operationId":"token-restore-2","name":"Old script","scopes":["read"]}`)
	if w := f.call("DELETE", "/v1/me/access-tokens/"+gone.Credential.ID, f.owner, ""); w.Code != 200 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body)
	}

	// The backup copy, scrubbed as every backup is.
	copyPath := filepath.Join(t.TempDir(), "portico.db")
	if _, err := f.deps.DB.Exec(`VACUUM INTO ?`, copyPath); err != nil {
		t.Fatal(err)
	}
	scrubbed, err := dbwork.OpenCandidate(filepath.Dir(copyPath), filepath.Base(copyPath))
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.ScrubBackupCopy(ctx, scrubbed); err != nil {
		t.Fatal(err)
	}
	if err = scrubbed.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, secret := range map[string]string{"token": token.Secret, "revoked token": gone.Secret} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("the backup copy holds the %s's secret", name)
		}
	}
	copyDB, err := dbwork.OpenHandle(copyPath, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	// Named API keys are owner-recoverable, and that recovery survives backup.
	// Personal tokens remain digest-only; human sign-ins are still scrubbed below.
	recovered, err := access.New(copyDB).RevealCredential(ctx, func(context.Context, *sql.Tx) error { return nil }, key.Credential.ID, identity.CredentialKindKey)
	if err != nil || recovered.Secret != key.Secret {
		t.Fatalf("backup lost owner key recovery: %v", err)
	}
	var recoverableTokens int
	if err = copyDB.QueryRow(`SELECT count(*) FROM api_credentials WHERE kind='token' AND secret<>''`).Scan(&recoverableTokens); err != nil || recoverableTokens != 0 {
		t.Fatalf("backup changed personal token storage: %d %v", recoverableTokens, err)
	}
	for what, check := range map[string]struct {
		query string
		want  int
	}{
		"sessions that are not a live key or token": {`SELECT count(*) FROM authorization_session_families WHERE revoked=0 AND id NOT IN(SELECT family_id FROM api_credentials WHERE revoked_ms=0)`, 0},
		"usable tokens of anything else":            {`SELECT count(*) FROM authorization_family_tokens WHERE retired=0 AND family_id NOT IN(SELECT family_id FROM api_credentials WHERE revoked_ms=0)`, 0},
		"live keys and tokens":                      {`SELECT count(*) FROM authorization_session_families f JOIN authorization_family_tokens t ON t.family_id=f.id WHERE f.revoked=0 AND t.retired=0 AND f.id IN(SELECT family_id FROM api_credentials WHERE revoked_ms=0)`, 2},
		"revoked keys brought back":                 {`SELECT count(*) FROM authorization_session_families WHERE revoked=0 AND id IN(SELECT family_id FROM api_credentials WHERE revoked_ms<>0)`, 0},
	} {
		var n int
		if err = copyDB.QueryRow(check.query).Scan(&n); err != nil || n != check.want {
			t.Fatalf("the backup copy holds %d %s, want %d: %v", n, what, check.want, err)
		}
	}

	// The restore, on the running server.
	acted, err := f.deps.Identity.SignEveryoneOutAfterRestore(ctx, "2026-01-01T00:00:00Z")
	if err != nil || !acted {
		t.Fatalf("sign everyone out: %v %v", acted, err)
	}
	dbwork.BumpAuthority()
	for name, session := range map[string]string{"the owner": f.owner, "the administrator": f.admin, "the member": f.member} {
		if w := f.call("GET", "/v1/me", session, ""); w.Code != 401 {
			t.Fatalf("%s is still signed in after a restore: %d %s", name, w.Code, w.Body)
		}
	}
	if w := f.call("GET", "/v1/me", key.Secret, ""); w.Code != 200 {
		t.Fatalf("the key stopped working at a restore: %d %s", w.Code, w.Body)
	}
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 200 {
		t.Fatalf("the member's token stopped working at a restore: %d %s", w.Code, w.Body)
	}
	if w := f.call("GET", "/v1/me", gone.Secret, ""); w.Code != 401 {
		t.Fatalf("a restore brought a revoked token back: %d %s", w.Code, w.Body)
	}
}

// Deleting a member through the server ends their keys and tokens with their
// sessions: nothing of theirs is left signed in, and nobody else's is touched.
func TestDeletingAMemberEndsTheirKeysWithTheirSessions(t *testing.T) {
	f := administrationFixtureNew(t)
	ctx := context.Background()
	token := issueCredential(t, f, "/v1/me/access-tokens", f.member, `{"operationId":"token-delete-1","name":"Member script","scopes":["read"]}`)
	other := issueCredential(t, f, "/v1/me/access-tokens", f.admin, `{"operationId":"token-delete-2","name":"Admin script","scopes":["read"]}`)
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 200 {
		t.Fatalf("before: %d %s", w.Code, w.Body)
	}
	var family string
	if err := f.deps.DB.QueryRow(`SELECT family_id FROM api_credentials WHERE id=?`, token.Credential.ID).Scan(&family); err != nil {
		t.Fatal(err)
	}
	if err := dbwork.WithWriteTx(ctx, f.deps.DB, dbwork.ClassSecurityFence, func(tx *sql.Tx) error {
		return f.deps.Identity.EraseMemberTx(ctx, tx, "member")
	}); err != nil {
		t.Fatal(err)
	}
	dbwork.BumpAuthority()
	if w := f.call("GET", "/v1/me", token.Secret, ""); w.Code != 401 {
		t.Fatalf("a deleted member's token works: %d %s", w.Code, w.Body)
	}
	if w := f.call("GET", "/v1/me", f.member, ""); w.Code != 401 {
		t.Fatalf("a deleted member is still signed in: %d %s", w.Code, w.Body)
	}
	var live, stored int
	if err := f.deps.DB.QueryRow(`SELECT (SELECT count(*) FROM authorization_session_families WHERE revoked=0 AND (id=? OR account_id='member')),(SELECT count(*) FROM api_credentials WHERE account_id='member')`, family).Scan(&live, &stored); err != nil || live != 0 || stored != 0 {
		t.Fatalf("after deleting the member: %d live sessions of theirs, %d stored tokens: %v", live, stored, err)
	}
	if w := f.call("GET", "/v1/me", other.Secret, ""); w.Code != 200 {
		t.Fatalf("deleting one member ended another's token: %d %s", w.Code, w.Body)
	}
}
