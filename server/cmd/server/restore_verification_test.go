package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/storagepaths"
	"strings"
	"testing"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/buildinfo"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/mediaartifact"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storage"
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
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
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
	raw, e := os.ReadFile(storagepaths.Path(from, "server.sqlite"))
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
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
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
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
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
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
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
	entries, e := os.ReadDir(storagepaths.Path(state, "networking-keys"))
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
	db, e := persistence.Open(storagepaths.Path(fresh, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = networking.VerifyStateKeys(ctx, db, fresh); e != nil {
		t.Fatalf("an untouched state was refused: %v", e)
	}
	if _, e = os.Lstat(storagepaths.Path(fresh, "networking-keys")); !os.IsNotExist(e) {
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
	if e := tryStageBackupFolder(t, state, serverID, probe, change); e != nil {
		t.Fatal(e)
	}
}

// tryStageBackupFolder is stageBackupFolder for a backup that staging itself
// may refuse; it returns that refusal.
func tryStageBackupFolder(t *testing.T, state, serverID, probe string, change func(folder string)) error {
	t.Helper()
	ctx := context.Background()
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	service := backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion())
	id, e := service.Create(ctx, backup.KindManual)
	if e != nil {
		t.Fatal(e)
	}
	folder := storagepaths.Path(state, backup.BackupsDir, id)
	candidate, e := persistence.Open(filepath.Join(folder, backup.DatabaseName))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = candidate.Exec(`INSERT INTO configuration(key,value) VALUES('restore_probe',?)`, probe); e != nil {
		t.Fatal(e)
	}
	if e = candidate.Close(); e != nil {
		t.Fatal(e)
	}
	refreshBackupDatabaseManifest(t, folder)
	if change != nil {
		change(folder)
	}
	_, e = service.Stage(ctx, backup.Source{BackupID: id})
	return e
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

// addBackupFile adds a file to a backup folder and lists it in the manifest,
// as a backup that carried that file would.
func addBackupFile(t *testing.T, folder, name string, content []byte) {
	t.Helper()
	target := filepath.Join(folder, filepath.FromSlash(name))
	if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(target, content, 0600); e != nil {
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
	sum := sha256.Sum256(content)
	manifest["files"] = append(manifest["files"].([]any), map[string]any{"name": name, "bytes": len(content), "sha256": hex.EncodeToString(sum[:])})
	if raw, e = json.Marshal(manifest); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
}

// changeBackupDatabase runs statements against a backup folder's database.
func changeBackupDatabase(t *testing.T, folder string, statements ...string) {
	t.Helper()
	candidate, e := persistence.Open(filepath.Join(folder, backup.DatabaseName))
	if e != nil {
		t.Fatal(e)
	}
	defer candidate.Close()
	// One connection, so a pragma set for the change applies to it.
	connection, e := candidate.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer connection.Close()
	for _, statement := range statements {
		if _, e = connection.ExecContext(context.Background(), statement); e != nil {
			t.Fatalf("%s: %v", statement, e)
		}
	}
	if e = connection.Close(); e != nil {
		t.Fatal(e)
	}
	if e = candidate.Close(); e != nil {
		t.Fatal(e)
	}
	refreshBackupDatabaseManifest(t, folder)
}

// refreshBackupDatabaseManifest records a deliberate fixture edit as content
// produced by the backup writer. Restore tests must reach the startup checks,
// rather than all passing at the earlier database-checksum boundary.
func refreshBackupDatabaseManifest(t *testing.T, folder string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(folder, backup.DatabaseName))
	if err != nil {
		t.Fatal(err)
	}
	replaceBackupFile(t, folder, backup.DatabaseName, raw)
}

// startable runs the loads start-up performs on a state folder, with the code
// start-up uses, and reports the first that fails.
func startable(t *testing.T, state string) error {
	t.Helper()
	ctx := context.Background()
	if err := validateStateConfiguration(state); err != nil {
		return err
	}
	runner, e := openCurrentAuthority(state)
	if e != nil {
		return e
	}
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
	if e != nil {
		return e
	}
	defer db.Close()
	if e = persistence.RebaseStatePaths(ctx, db, state); e != nil {
		return e
	}
	if e = persistence.VerifyNetworkingClaims(ctx, db); e != nil {
		return e
	}
	keys, e := networking.OpenCurrentKeys(ctx, db, state, runner)
	if e != nil {
		return e
	}
	if e = keys.Close(); e != nil {
		return e
	}
	if _, e = initializeLiveChannels(db, state); e != nil {
		return e
	}
	if _, e = identity.New(db, state); e != nil {
		return e
	}
	if e = mounts.InspectPrivateDirectory(state); e != nil {
		return e
	}
	priority, e := storedBackgroundPriority(ctx, db)
	if e != nil {
		return e
	}
	if e = storage.CheckBackgroundTaskPriority(priority); e != nil {
		return e
	}
	if _, e = mediaartifact.New(storagepaths.Path(state, "subtitles")); e != nil {
		return e
	}
	if _, e = dvr.ReadStoragePolicy(ctx, db); e != nil {
		return e
	}
	// The rest of start-up runs statements across the schema and reads the
	// rows a fresh install has.
	if e = persistence.VerifySchemaObjects(ctx, db); e != nil {
		return e
	}
	return persistence.VerifySeededRows(ctx, db)
}

// A restore is kept only when the server can start on the result. Each case
// is a backup that passes the database's own checks and is missing, or has
// spoiled, exactly one thing start-up needs and would not recreate. Left in
// place, every one of them is a server that refuses each start. All must be
// rolled back while the previous state can still be put back, and the server
// must start on what the rollback leaves.
func TestRestoreIsRefusedForEveryMissingStartupRequirement(t *testing.T) {
	authority := "Keys/supervisor/authority-incarnation.v1"
	cases := map[string]func(t *testing.T, state, folder string){
		"configuration is invalid TOML": func(t *testing.T, _, folder string) {
			addBackupFile(t, folder, "portico.toml", []byte("[unterminated"))
		},
		"configuration has a value of the wrong type": func(t *testing.T, _, folder string) {
			addBackupFile(t, folder, "portico.toml", []byte("[backup]\ninclude_artwork = \"often\"\n"))
		},
		"configuration has an unknown choice": func(t *testing.T, _, folder string) {
			addBackupFile(t, folder, "portico.toml", []byte("[general]\nheavy_work = \"sometimes\"\n"))
		},
		"authority file is not a valid value": func(t *testing.T, _, folder string) {
			replaceBackupFile(t, folder, authority, []byte(strings.Repeat("z", 64)))
		},
		"authority file has the wrong length": func(t *testing.T, _, folder string) {
			replaceBackupFile(t, folder, authority, []byte(strings.Repeat("a", 63)))
		},
		"signing key is another server's": func(t *testing.T, state, folder string) {
			entries, e := os.ReadDir(storagepaths.Path(state, "networking-keys"))
			if e != nil || len(entries) != 1 {
				t.Fatalf("fixture keys: %d %v", len(entries), e)
			}
			_, other, e := ed25519.GenerateKey(rand.Reader)
			if e != nil {
				t.Fatal(e)
			}
			replaceBackupFile(t, folder, "Keys/networking-keys/"+entries[0].Name(), other)
		},
		"claim schema: an identity protection is missing": func(t *testing.T, _, folder string) {
			changeBackupDatabase(t, folder, `DROP TRIGGER networking_identity_no_update`)
		},
		"claim schema: the version marker is missing": func(t *testing.T, _, folder string) {
			changeBackupDatabase(t, folder, `DROP TABLE networking_claim_schema`)
		},
		"schema: a table is missing": func(t *testing.T, _, folder string) {
			changeBackupDatabase(t, folder, `DROP TABLE dvr_retention_retry`)
		},
		"schema: an index is missing": func(t *testing.T, _, folder string) {
			changeBackupDatabase(t, folder, `DROP INDEX dvr_recordings_due`)
		},
		"schema: a view is missing": func(t *testing.T, _, folder string) {
			changeBackupDatabase(t, folder, `DROP VIEW inventory_item_availability`)
		},
		"schema: a column start-up writes is missing": func(t *testing.T, _, folder string) {
			changeBackupDatabase(t, folder, `ALTER TABLE managed_mounts RENAME COLUMN observed TO seen`)
		},
		"subtitles folder is a file": func(t *testing.T, _, folder string) {
			addBackupFile(t, folder, "subtitles", []byte("not a folder"))
		},
		"subtitle objects folder is a file": func(t *testing.T, _, folder string) {
			addBackupFile(t, folder, "subtitles/objects", []byte("not a folder"))
		},
		"mount configuration folder is a file": func(t *testing.T, _, folder string) {
			addBackupFile(t, folder, "managed-storage", []byte("not a folder"))
		},
	}
	// Every row a fresh install starts with, one at a time: the maintenance
	// policy and the recording storage policy that start-up reads directly,
	// and each of the others its services read.
	seeded, e := persistence.SeededTables()
	if e != nil || len(seeded) < 10 {
		t.Fatalf("seeded tables: %d %v", len(seeded), e)
	}
	for _, table := range seeded {
		cases["seeded row is missing: "+table] = func(t *testing.T, _, folder string) {
			statements := []string{`PRAGMA foreign_keys=OFF`}
			if table == "remote_source_choice_definitions" {
				// Corrupt only this disposable backup's rows, retaining its exact schema.
				statements = append(statements, `DROP TRIGGER remote_source_choice_definition_immutable_delete`)
			}
			statements = append(statements, `DELETE FROM "`+table+`"`)
			if table == "remote_source_choice_definitions" {
				statements = append(statements, `CREATE TRIGGER remote_source_choice_definition_immutable_delete BEFORE DELETE ON remote_source_choice_definitions
 BEGIN SELECT RAISE(ABORT,'source choice definitions are immutable'); END`)
			}
			changeBackupDatabase(t, folder, statements...)
		}
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state, serverID := startedState(t)
			before, e := os.ReadFile(storagepaths.Path(state, filepath.FromSlash(authority)))
			if e != nil {
				t.Fatal(e)
			}
			// Staging already refuses some of these; the rest are refused when
			// the restore is applied. Either way nothing may be kept.
			staged := tryStageBackupFolder(t, state, serverID, "refused", func(folder string) { change(t, state, folder) }) == nil
			applied := backup.ApplyStaged(context.Background(), state, verifyRestoredState(state))
			if staged && applied == nil {
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
			if after, e := os.ReadFile(storagepaths.Path(state, filepath.FromSlash(authority))); e != nil || string(after) != string(before) {
				t.Fatalf("the previous authority file was not put back: %v", e)
			}
			db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			last, e := backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion()).LastRestore()
			if e != nil || staged && (last == nil || last.Outcome != "rolled_back" || last.Reason == "") || last != nil && last.Outcome == "restored" {
				t.Fatalf("the refusal was not recorded for the owner: %+v %v", last, e)
			}
		})
	}
}

func TestConfigurationValidationDoesNotCreateOrChangeState(t *testing.T) {
	state := t.TempDir()
	path := storagepaths.Path(state, "portico.toml")
	if err := validateStateConfiguration(state); err != nil {
		t.Fatal("missing optional configuration was refused", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("validation created configuration", err)
	}
	for _, raw := range [][]byte{[]byte("[unterminated"), []byte("name = 'valid'\n")} {
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		err := validateStateConfiguration(state)
		if strings.HasPrefix(string(raw), "[") && err == nil {
			t.Fatal("invalid configuration accepted")
		}
		if !strings.HasPrefix(string(raw), "[") && err != nil {
			t.Fatal("valid configuration refused", err)
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil || string(after) != string(raw) {
			t.Fatal("validation changed the configuration", readErr)
		}
	}
}

func TestStagedRestoreCanReplaceAnInvalidCurrentConfiguration(t *testing.T) {
	state, serverID := startedState(t)
	path := storagepaths.Path(state, "portico.toml")
	if err := os.WriteFile(path, []byte("[invalid current config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateStateConfiguration(state); err == nil {
		t.Fatal("fixture current config is not invalid")
	}
	stageBackupFolder(t, state, serverID, "valid-config", func(folder string) {
		replaceBackupFile(t, folder, "portico.toml", []byte("# restored configuration\n"))
	})
	// Startup now applies the pending restore before validating the current
	// config. The restore's own verifier still rejects an invalid replacement.
	if err := backup.ApplyStaged(context.Background(), state, verifyRestoredState(state)); err != nil {
		t.Fatal(err)
	}
	if err := validateStateConfiguration(state); err != nil {
		t.Fatal("valid staged config did not repair startup", err)
	}
	if _, probe := restoredProbe(t, state); probe != "valid-config" {
		t.Fatal("configuration repair did not restore the database", probe)
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

// A background priority start-up would not accept never reaches a restore: the
// column's own constraint makes the copy fail its integrity check when it is
// staged. The start-up requirement refuses it as well, on its own, should a
// value ever get that far.
func TestRestoreRefusesABackgroundPriorityStartupWouldNotAccept(t *testing.T) {
	ctx := context.Background()
	state, serverID := startedState(t)
	db, e := persistence.Open(storagepaths.Path(state, "server.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	service := backup.New(state, db, serverID, buildinfo.Info()["version"], persistence.SchemaVersion())
	id, e := service.Create(ctx, backup.KindManual)
	if e != nil {
		t.Fatal(e)
	}
	folder := storagepaths.Path(state, backup.BackupsDir, id)
	changeBackupDatabase(t, folder, `PRAGMA ignore_check_constraints=ON`, `UPDATE maintenance_policy SET background_priority='whenever'`)
	if _, e = service.Stage(ctx, backup.Source{BackupID: id}); !errors.Is(e, persistence.ErrCandidateIntegrity) {
		t.Fatalf("a database with a setting outside its allowed values was staged: %v", e)
	}
	spoiled, e := persistence.Open(filepath.Join(folder, backup.DatabaseName))
	if e != nil {
		t.Fatal(e)
	}
	defer spoiled.Close()
	refused := 0
	for _, requirement := range restoredStateRequirements {
		if requirement.check(ctx, spoiled, state) != nil {
			refused++
			if !strings.Contains(requirement.reason, "background work setting") {
				t.Fatalf("the wrong requirement refused it: %s", requirement.reason)
			}
		}
	}
	if refused != 1 {
		t.Fatalf("%d start-up requirements refused the setting, want exactly the one that reads it", refused)
	}
	// And every requirement accepts the server's own, untouched state.
	for _, requirement := range restoredStateRequirements {
		if e = requirement.check(ctx, db, state); e != nil {
			t.Fatalf("an untouched state fails a start-up requirement (%s): %v", requirement.reason, e)
		}
	}
}

// Judging a restore reads the restored state and nothing else. A healthy
// backup is kept even when the system's temporary folder cannot be used:
// nothing about the check is built there.
func TestRestoreOfAHealthyBackupDoesNotNeedTheTemporaryFolder(t *testing.T) {
	state, serverID := startedState(t)
	stageBackupFolder(t, state, serverID, "healthy", nil)
	t.Setenv("TMPDIR", storagepaths.Path(state, "no-such-folder"))
	if e := backup.ApplyStaged(context.Background(), state, verifyRestoredState(state)); e != nil {
		t.Fatalf("a healthy backup was refused because the temporary folder is unusable: %v", e)
	}
	if _, probe := restoredProbe(t, state); probe != "healthy" {
		t.Fatalf("the healthy backup was not restored: probe %q", probe)
	}
}
