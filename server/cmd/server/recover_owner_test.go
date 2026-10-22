package main

import (
	"io"
	"os"
	"os/exec"
	"portico.local/server/internal/storagepaths"
	"strings"
	"testing"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

// There is no replay key to lose: identity values are plain database columns,
// so recovery works with or without --reset-mfa, and factors survive unless
// the owner asks to remove them.
func TestRecoverOwnerCLINeedsNoKey(t *testing.T) {
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	path := storagepaths.Path(state, "server.sqlite")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('account','owner',X'00','profile',1)`); err != nil {
		t.Fatal(err)
	}
	service, err := identity.New(db, state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Issue("account", "profile", "local", "owner", 1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO identity_account_factors(account_id,secret,confirmed,enrolled_at) VALUES('account','unrecoverable',1,'2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(storagepaths.Path(state, "authorization-replay.key")); !os.IsNotExist(err) {
		t.Fatal("key file present")
	}
	binary := serverBinary(t)
	output, err := exec.Command(binary, "recover-owner", "--state", state).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "One-time owner recovery password:") || strings.Contains(string(output), "setup code") {
		t.Fatalf("owner recovery process failed: %v %s", err, output)
	}
	db, err = persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var factors, live int
	if err = db.QueryRow(`SELECT count(*) FROM identity_account_factors`).Scan(&factors); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM authorization_session_families WHERE revoked=0`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if factors != 1 || live != 0 {
		t.Fatalf("recovery removed authority it should keep: factors=%d families=%d", factors, live)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reset, err := exec.Command(binary, "recover-owner", "--state", state, "--reset-mfa").CombinedOutput()
	if err != nil || !strings.Contains(string(reset), "One-time owner recovery password:") {
		t.Fatalf("owner recovery with reset failed: %v %s", err, reset)
	}
	db, err = persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.QueryRow(`SELECT count(*) FROM identity_account_factors`).Scan(&factors); err != nil || factors != 0 {
		t.Fatalf("reset-mfa left factors: %d %v", factors, err)
	}
}

func TestRecoverOwnerSharesTheServerDataLock(t *testing.T) {
	state := t.TempDir()
	db, err := persistence.Open(storagepaths.Path(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	lock, err := acquireInstanceLock(storagepaths.Path(state, "server.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := recoverOwner([]string{"--state", state}, io.Discard); err == nil || !strings.Contains(err.Error(), "stop Portico") {
		t.Fatalf("recovery did not share the server lock: %v", err)
	}
}
