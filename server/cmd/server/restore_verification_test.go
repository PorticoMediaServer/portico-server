package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// stageBackupFolder makes a whole backup of the server at state, lets change
// alter the backup folder, and stages it as a restore. The backup's database
// is marked with probe so a kept restore can be told from the previous state.
func stageBackupFolder(t *testing.T, state, serverID, probe string, change func(folder string)) {
	t.Helper()
	ctx := context.Background()
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	service := backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion())
	id, e := service.Create(ctx, backup.KindManual)
	if e != nil {
		t.Fatal(e)
	}
	folder := filepath.Join(state, backup.BackupsDir, id)
	candidate, e := persistence.Open(filepath.Join(folder, backup.DatabaseName))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = candidate.Exec(`INSERT INTO configuration(key,value) VALUES('restore_probe',?)`, probe); e != nil {
		t.Fatal(e)
	}
	candidate.Close()
	if change != nil {
		change(folder)
	}
	if _, e = service.Stage(ctx, backup.Source{BackupID: id}); e != nil {
		t.Fatal(e)
	}
}

// replaceBackupFile rewrites one file of a backup folder and records the new
// bytes in its manifest, as a backup that was made with that content would.
func replaceBackupFile(t *testing.T, folder, name string, content []byte) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(folder, filepath.FromSlash(name)), content, 0600); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(folder, backup.ManifestName)
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var manifest map[string]any
	if e = json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, entry := range manifest["files"].([]any) {
		file := entry.(map[string]any)
		if file["name"] == name {
			sum := sha256.Sum256(content)
			file["bytes"], file["sha256"], found = len(content), hex.EncodeToString(sum[:]), true
		}
	}
	if !found {
		t.Fatalf("the backup does not carry %s", name)
	}
	if raw, e = json.Marshal(manifest); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
}

// startable reports whether the steps that open the server's identity at
// start-up accept the state folder as it now stands.
func startable(t *testing.T, state string) error {
	t.Helper()
	runner, e := openCurrentAuthority(state)
	if e != nil {
		return e
	}
	db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
	if e != nil {
		return e
	}
	defer db.Close()
	if e = persistence.VerifyNetworkingClaims(context.Background(), db); e != nil {
		return e
	}
	keys, e := networking.OpenCurrentKeys(context.Background(), db, state, runner)
	if e != nil {
		return e
	}
	return keys.Close()
}

// A restore is kept only when the server can open its identity on the result.
// Each of these backups passes the database's own checks and carries the right
// signing keys, yet would leave a server that refuses every start: they must be
// rolled back while the previous state can still be put back.
func TestRestoreThatStartupWouldRefuseIsRolledBack(t *testing.T) {
	authority := filepath.Join("supervisor", "authority-incarnation.v1")
	for name, change := range map[string]func(t *testing.T, folder string){
		"authority incarnation is not a valid value": func(t *testing.T, folder string) {
			replaceBackupFile(t, folder, filepath.ToSlash(authority), []byte(strings.Repeat("z", 64)))
		},
		"authority incarnation has the wrong length": func(t *testing.T, folder string) {
			replaceBackupFile(t, folder, filepath.ToSlash(authority), []byte(strings.Repeat("a", 63)))
		},
		"an identity protection is missing from the schema": func(t *testing.T, folder string) {
			candidate, e := persistence.Open(filepath.Join(folder, backup.DatabaseName))
			if e != nil {
				t.Fatal(e)
			}
			defer candidate.Close()
			if _, e = candidate.Exec(`DROP TRIGGER networking_identity_no_update`); e != nil {
				t.Fatal(e)
			}
		},
		"a claim table is missing from the schema": func(t *testing.T, folder string) {
			candidate, e := persistence.Open(filepath.Join(folder, backup.DatabaseName))
			if e != nil {
				t.Fatal(e)
			}
			defer candidate.Close()
			if _, e = candidate.Exec(`DROP TABLE networking_claim_schema`); e != nil {
				t.Fatal(e)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			state, serverID := startedState(t)
			before, e := os.ReadFile(filepath.Join(state, authority))
			if e != nil {
				t.Fatal(e)
			}
			stageBackupFolder(t, state, serverID, "refused", func(folder string) { change(t, folder) })
			if e = backup.ApplyStaged(context.Background(), state, verifyRestoredState(state)); e == nil {
				t.Error("a restore the server could never start on was kept")
			}
			if backup.RestoreUnresolved(state) {
				t.Fatal("the refused restore was left half applied")
			}
			if e = startable(t, state); e != nil {
				t.Fatalf("the server cannot start on what the restore left behind: %v", e)
			}
			if id, probe := restoredProbe(t, state); id != serverID || probe != "" {
				t.Fatalf("the previous database was not put back: server %q probe %q", id, probe)
			}
			if after, e := os.ReadFile(filepath.Join(state, authority)); e != nil || string(after) != string(before) {
				t.Fatalf("the previous authority incarnation was not put back: %v", e)
			}
			db, e := persistence.Open(filepath.Join(state, "server.sqlite"))
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			last, e := backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion()).LastRestore()
			if e != nil || last == nil || last.Outcome != "rolled_back" || last.Reason == "" {
				t.Fatalf("the refusal was not recorded for the owner: %+v %v", last, e)
			}
		})
	}
}

// A whole backup of this server, untouched, passes every one of those checks.
func TestRestoreOfThisServersBackupFolderIsKept(t *testing.T) {
	state, serverID := startedState(t)
	stageBackupFolder(t, state, serverID, "folder", nil)
	if e := backup.ApplyStaged(context.Background(), state, verifyRestoredState(state)); e != nil {
		t.Fatal(e)
	}
	if id, probe := restoredProbe(t, state); id != serverID || probe != "folder" {
		t.Fatalf("this server's own backup was not restored: server %q probe %q", id, probe)
	}
	if e := startable(t, state); e != nil {
		t.Fatalf("the server cannot start on its own restored backup: %v", e)
	}
}
