package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/buildinfo"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/persistence"
)

// startedState is a state folder as a first start leaves it: an authority
// incarnation, a database, and the server identity with its signing key.
func startedState(t *testing.T) (state, serverID string) {
	t.Helper()
	state = t.TempDir()
	if e := os.Chmod(state, 0700); e != nil {
		t.Fatal(e)
	}
	runner, e := openCurrentAuthority(state)
	if e != nil {
		t.Fatal(e)
	}
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	keys, e := networking.OpenCurrentKeys(context.Background(), db, state, runner)
	if e != nil {
		t.Fatal(e)
	}
	keys.Close()
	if e = db.QueryRow(`SELECT value FROM configuration WHERE key='id'`).Scan(&serverID); e != nil {
		t.Fatal(e)
	}
	return state, serverID
}

// stageBareDatabase stages a copy of from's database, marked with probe, as a
// bare-file restore on the server at state.
func stageBareDatabase(t *testing.T, state, serverID, from, probe string) {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(from, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	bare := filepath.Join(t.TempDir(), "bare.db")
	if e = os.WriteFile(bare, raw, 0600); e != nil {
		t.Fatal(e)
	}
	candidate, e := persistence.Open(bare)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = candidate.Exec(`INSERT INTO configuration(key,value) VALUES('restore_probe',?)`, probe); e != nil {
		t.Fatal(e)
	}
	candidate.Close()
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion()).Stage(context.Background(), backup.Source{Path: bare}); e != nil {
		t.Fatal(e)
	}
}

func restoredProbe(t *testing.T, state string) (serverID, probe string) {
	t.Helper()
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.QueryRow(`SELECT value,COALESCE((SELECT value FROM configuration WHERE key='restore_probe'),'') FROM configuration WHERE key='id'`).Scan(&serverID, &probe); e != nil {
		t.Fatal(e)
	}
	return serverID, probe
}

// A bare database carries no identity keys. One taken from another server
// names a signing key this state folder does not hold, so the server could
// never start on it: the restore must be refused while the previous database
// can still be put back, not recorded as restored.
func TestRestoreOfAnotherServersBareDatabaseIsRolledBack(t *testing.T) {
	ctx := context.Background()
	state, serverID := startedState(t)
	other, otherID := startedState(t)
	if serverID == otherID {
		t.Fatal("fixture servers share an identity")
	}
	stageBareDatabase(t, state, serverID, other, "foreign")
	if e := backup.ApplyStaged(ctx, state, verifyRestoredState(state)); e == nil {
		t.Fatal("a database whose identity keys are not in this state folder was kept")
	}
	if backup.RestoreUnresolved(state) {
		t.Fatal("the refused restore was left half applied")
	}
	if id, probe := restoredProbe(t, state); id != serverID || probe != "" {
		t.Fatalf("the previous database was not put back: server %q probe %q", id, probe)
	}
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	last, e := backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion()).LastRestore()
	if e != nil || last == nil || last.Outcome != "rolled_back" || !strings.Contains(last.Reason, "state folder") {
		t.Fatalf("the refusal was not recorded for the owner: %+v %v", last, e)
	}
	// The server still starts on what was put back.
	runner, e := openCurrentAuthority(state)
	if e != nil {
		t.Fatal(e)
	}
	keys, e := networking.OpenCurrentKeys(ctx, db, state, runner)
	if e != nil {
		t.Fatalf("the server no longer opens its own keys: %v", e)
	}
	keys.Close()
	entries, e := os.ReadDir(filepath.Join(state, "networking-keys"))
	if e != nil || len(entries) != 1 {
		t.Fatalf("verification changed the key folder: %d %v", len(entries), e)
	}
}

// The same server's own bare database names a key this folder holds.
func TestRestoreOfThisServersBareDatabaseIsKept(t *testing.T) {
	state, serverID := startedState(t)
	stageBareDatabase(t, state, serverID, state, "own")
	if e := backup.ApplyStaged(context.Background(), state, verifyRestoredState(state)); e != nil {
		t.Fatal(e)
	}
	if id, probe := restoredProbe(t, state); id != serverID || probe != "own" {
		t.Fatalf("this server's own database was not restored: server %q probe %q", id, probe)
	}
}

// Asking the question changes nothing: a database from before setup is
// accepted only beside an empty key folder, and no folder or key is created.
func TestStateKeyVerificationIsReadOnly(t *testing.T) {
	ctx := context.Background()
	fresh := t.TempDir()
	db, e := persistence.Open(filepath.Join(fresh, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = networking.VerifyStateKeys(ctx, db, fresh); e != nil {
		t.Fatalf("an untouched state was refused: %v", e)
	}
	if _, e = os.Lstat(filepath.Join(fresh, "networking-keys")); !os.IsNotExist(e) {
		t.Fatalf("verification created the key folder: %v", e)
	}
	var identities int
	if e = db.QueryRow(`SELECT count(*) FROM networking_server_identities`).Scan(&identities); e != nil || identities != 0 {
		t.Fatalf("verification wrote an identity: %d %v", identities, e)
	}
	// The same unmarked database beside another server's keys is not startable.
	started, _ := startedState(t)
	if e = networking.VerifyStateKeys(ctx, db, started); e == nil {
		t.Fatal("a database with no identity was accepted beside existing keys")
	}
}
