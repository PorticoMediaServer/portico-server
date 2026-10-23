package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func TestBackupSettingsHTTPSecretBoundaryAndEncryptedRestore(t *testing.T) {
	state := t.TempDir()
	db, e := persistence.Open(filepath.Join(state, "Database", "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ident, e := identity.New(db, state)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','profile',1),('member','member',x'00','member',1)`); e != nil {
		t.Fatal(e)
	}
	owner, e := ident.Issue("owner", "profile", "local", "owner", 1)
	if e != nil {
		t.Fatal(e)
	}
	member, e := ident.Issue("member", "member", "local", "member", 1)
	if e != nil {
		t.Fatal(e)
	}
	service := backup.New(state, db, ident.ID(), "fixture", persistence.SchemaVersion())
	service.SetCopyScrubber(identity.ScrubBackupCopy)
	service.SetPolicyStore(backup.NewPolicyStore(db, state))
	handler := New(Dependencies{DB: db, Identity: ident, Backups: service})
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{"", member.AccessToken} {
		for _, method := range []string{"GET", "PUT"} {
			w := request(method, "/v1/admin/backups/settings", token, `{}`)
			if token == "" && w.Code != 401 || token != "" && w.Code != 403 {
				t.Fatal("nonowner settings boundary", w.Code, w.Body.String())
			}
		}
	}
	password := "HTTP private fixture phrase!"
	destination := t.TempDir()
	payload, _ := json.Marshal(map[string]any{"backupFolder": destination, "includeArtwork": true, "passphraseProtected": true, "passphrase": password})
	saved := request("PUT", "/v1/admin/backups/settings", owner.AccessToken, string(payload))
	if saved.Code != 200 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	assertSpecResponse(t, "PUT", "/v1/admin/backups/settings", saved)
	if bytes.Contains(saved.Body.Bytes(), []byte(password)) || !bytes.Contains(saved.Body.Bytes(), []byte(`"passphraseSet":true`)) {
		t.Fatal("secret readback", saved.Body.String())
	}
	read := request("GET", "/v1/admin/backups/settings", owner.AccessToken, "")
	if read.Code != 200 || bytes.Contains(read.Body.Bytes(), []byte(password)) {
		t.Fatal("settings read", read.Code)
	}
	assertSpecResponse(t, "GET", "/v1/admin/backups/settings", read)
	invalid := request("PUT", "/v1/admin/backups/settings", owner.AccessToken, `{"backupFolder":"/"}`)
	if invalid.Code != 400 {
		t.Fatal("root destination accepted", invalid.Code)
	}
	job, e := service.Start(context.Background(), backup.KindManual, "http-encrypted-fixture")
	if e != nil {
		t.Fatal(e)
	}
	j, _ := service.Job(job)
	if e = j.Result(); e != nil {
		t.Fatal(e)
	}
	entries, e := service.List()
	if e != nil || len(entries) != 1 || !entries[0].Encrypted {
		t.Fatal(entries, e)
	}
	if _, e = os.Stat(filepath.Join(destination, entries[0].ID, "archive.pba")); e != nil {
		t.Fatal(e)
	}
	for i, phrase := range []string{"wrong fixture passphrase", password} {
		body, _ := json.Marshal(map[string]any{"operationId": "http-restore-" + string(rune('a'+i)), "source": map[string]string{"backupId": entries[0].ID}, "passphrase": phrase})
		started := request("POST", "/v1/admin/backups/restore", owner.AccessToken, string(body))
		if started.Code != 202 {
			t.Fatal(started.Code, started.Body.String())
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			progress := service.RestoreProgress()
			if progress.Phase == "failed" || progress.Phase == "staged" {
				if i == 0 && progress.ErrorCode != "restore_integrity_failed" {
					t.Fatal("wrong password refusal", progress)
				}
				if i == 1 && progress.Result == nil {
					t.Fatal("valid restore failed", progress)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("restore timeout")
			}
			time.Sleep(10 * time.Millisecond)
		}
	} // A genuine producer failure must survive running disappearing in HTTP status.
	service.SetCopyScrubber(func(context.Context, string) error { return os.ErrPermission })
	failed, e := service.Start(context.Background(), backup.KindManual, "http-failing-fixture")
	if e != nil {
		t.Fatal(e)
	}
	failure, _ := service.Job(failed)
	if e = failure.Result(); e == nil {
		t.Fatal("producer failure accepted")
	}
	deadline := time.Now().Add(5 * time.Second)
	for service.LastBackup() == nil || service.LastBackup().JobID != failed {
		if time.Now().After(deadline) {
			t.Fatal("failure status missing")
		}
		time.Sleep(time.Millisecond)
	}

	status := request("GET", "/v1/admin/backups", owner.AccessToken, "")
	if status.Code != 200 || bytes.Contains(status.Body.Bytes(), []byte(password)) || !bytes.Contains(status.Body.Bytes(), []byte(`"outcome":"failed"`)) {
		t.Fatal("secret status leak", status.Code)
	}
}

func TestBackupMutationsRefuseNonownersAndExpiredSessionsWithoutSideEffects(t *testing.T) {
	f := administrationFixtureNew(t)
	state := t.TempDir()
	service := backup.New(state, f.deps.DB, f.deps.Identity.ID(), "fixture", persistence.SchemaVersion())
	store := backup.NewPolicyStore(f.deps.DB, state)
	service.SetPolicyStore(store)
	f.deps.Backups = service
	f.handler = New(f.deps)
	before, err := store.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"backupFolder": t.TempDir(), "includeArtwork": true, "passphraseProtected": true, "passphrase": "permission fixture private phrase"})
	routes := []struct{ method, path, body string }{
		{"PUT", "/v1/admin/backups/settings", string(body)},
		{"POST", "/v1/admin/backups", `{"operationId":"denied-backup-op"}`},
		{"DELETE", "/v1/admin/backups/2026-10-23T030000Z", ""},
		{"POST", "/v1/admin/backups/restore", `{"operationId":"denied-restore-op","source":{"path":"/unread-fixture"},"passphrase":"permission fixture private phrase"}`},
	}
	for _, token := range []string{f.admin, f.member} {
		for _, route := range routes {
			w := f.call(route.method, route.path, token, route.body)
			if w.Code != 403 {
				t.Fatalf("nonowner mutation %s %s: %d", route.method, route.path, w.Code)
			}
		}
	}
	// Revoke the actual owner family using its normal sign-out operation, then
	// replay that credential against every mutation.
	signedOut := f.call("DELETE", "/v1/sessions/current", f.owner, "")
	if signedOut.Code != 204 {
		t.Fatalf("owner logout failed: %d %s", signedOut.Code, signedOut.Body.String())
	}
	for _, route := range routes {
		w := f.call(route.method, route.path, f.owner, route.body)
		if w.Code != 401 {
			t.Fatalf("ended session mutation %s %s: %d", route.method, route.path, w.Code)
		}
	}
	after, err := store.Settings(context.Background())
	if err != nil || after != before {
		t.Fatal("denied writes changed policy", after, err)
	}
	if _, err = os.Stat(filepath.Join(state, "Keys", "backup-passphrase.key")); !os.IsNotExist(err) {
		t.Fatal("denied write minted a key", err)
	}
	if _, running := service.Running(); running || service.RestoreProgress() != nil || service.LastBackup() != nil {
		t.Fatal("denied write started work")
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 0 {
		t.Fatal("denied write created files", entries, err)
	}
}

func TestBackupRejectsMalformedSecretAndSourceBodiesBeforeStartingWork(t *testing.T) {
	f := administrationFixtureNew(t)
	state := t.TempDir()
	service := backup.New(state, f.deps.DB, f.deps.Identity.ID(), "fixture", persistence.SchemaVersion())
	store := backup.NewPolicyStore(f.deps.DB, state)
	service.SetPolicyStore(store)
	f.deps.Backups = service
	f.handler = New(f.deps)
	for _, test := range []struct{ path, body string }{
		{"/v1/admin/backups/settings", `{"passphraseProtected":true,"passphrase":42}`},
		{"/v1/admin/backups/settings", `{"passphraseProtected":true,"passphrase":"short"}`},
		{"/v1/admin/backups/settings", `{"includeArtwork":true,"unknownSecret":"private fixture"}`},
		{"/v1/admin/backups/restore", `{"operationId":"malformed-restore","source":{"path":"/unread-fixture","passphrase":"private fixture"}}`},
		{"/v1/admin/backups/restore", `{"operationId":"malformed-restore","source":{"path":42},"passphrase":"private fixture"}`},
	} {
		method := "POST"
		if strings.HasSuffix(test.path, "/settings") {
			method = "PUT"
		}
		w := f.call(method, test.path, f.owner, test.body)
		if w.Code != 400 {
			t.Fatalf("malformed request %s accepted: %d %s", test.path, w.Code, w.Body.String())
		}
	}
	if _, running := service.Running(); running || service.RestoreProgress() != nil {
		t.Fatal("malformed request started work")
	}
	settings, err := store.Settings(context.Background())
	if err != nil || settings.IncludeArtwork || settings.PassphraseProtected || settings.PassphraseSet {
		t.Fatal("malformed request persisted policy", settings, err)
	}
	if _, err = os.Stat(filepath.Join(state, "Keys", "backup-passphrase.key")); !os.IsNotExist(err) {
		t.Fatal("malformed body minted a key", err)
	}
}
