package persistence_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func TestMigrationRecoveryCopyHoldsNoLiveSignIns(t *testing.T) {
	prior := persistence.SetMigrationCopyScrubber(identity.ScrubBackupCopy)
	t.Cleanup(func() { persistence.SetMigrationCopyScrubber(prior) })
	state := t.TempDir()
	db, err := persistence.OpenFresh(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ident, err := identity.New(db, state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('account','owner',X'00','profile',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = ident.Issue("account", "profile", "local", "owner", 1); err != nil {
		t.Fatal(err)
	}
	if err = persistence.BackupBeforeMigrationsForTest(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(state, "migration-backups", "before-*.sqlite"))
	if err != nil || len(names) != 1 {
		t.Fatal(names, err)
	}
	copy, err := sql.Open("sqlite", names[0])
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	var live int
	if err = copy.QueryRow(`SELECT count(*) FROM authorization_session_families WHERE revoked=0`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Fatalf("pre-upgrade recovery copy retains %d live sign-ins", live)
	}
	if err = db.QueryRow(`SELECT count(*) FROM authorization_session_families WHERE revoked=0`).Scan(&live); err != nil || live != 1 {
		t.Fatal("copy scrub changed live sign-ins", live, err)
	}
}
