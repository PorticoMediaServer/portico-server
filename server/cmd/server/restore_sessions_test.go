package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func writeRestoreResult(t *testing.T, state, outcome, at string) {
	t.Helper()
	body := `{"at":"` + at + `","outcome":"` + outcome + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(state, backup.RestoreResultFile), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// What the first start after a restore does to sign-ins, and what a start
// that follows no restore, or a restore that was rolled back, leaves alone.
func TestStartAfterARestoreSignsEveryoneOutAndScrubsTheSetAsideCopy(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := persistence.Open(filepath.Join(state, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ident, err := identity.New(db, state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('account','owner',X'00','profile',1)`); err != nil {
		t.Fatal(err)
	}
	session, err := ident.Issue("account", "profile", "local", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	// The copy of the previous database a restore sets aside: a real database
	// with a live session in it, in a "-pre-restore" backup folder.
	setAside := filepath.Join(state, backup.BackupsDir, "2026-10-13T100000Z-pre-restore")
	if err = os.MkdirAll(setAside, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`VACUUM INTO ?`, filepath.Join(setAside, backup.DatabaseName)); err != nil {
		t.Fatal(err)
	}
	liveIn := func(path string) int {
		t.Helper()
		copyDB, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
		if err != nil {
			t.Fatal(err)
		}
		defer copyDB.Close()
		var n int
		if err = copyDB.QueryRow(`SELECT count(*) FROM authorization_session_families WHERE revoked=0`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// No restore has happened.
	if err = signOutAfterRestore(ctx, ident, state); err != nil {
		t.Fatal(err)
	}
	if _, err = ident.Authenticate(session.AccessToken); err != nil {
		t.Fatalf("a start with no restore signed someone out: %v", err)
	}
	// A restore that was rolled back: the database in place is the one that
	// was already running, and its sessions stand.
	writeRestoreResult(t, state, "rolled_back", "2026-10-13T10:00:00Z")
	if err = signOutAfterRestore(ctx, ident, state); err != nil {
		t.Fatal(err)
	}
	if _, err = ident.Authenticate(session.AccessToken); err != nil {
		t.Fatalf("a rolled-back restore signed someone out: %v", err)
	}
	if liveIn(filepath.Join(setAside, backup.DatabaseName)) != 1 {
		t.Fatal("the set-aside copy was changed before any restore was applied")
	}
	// A restore that was applied.
	writeRestoreResult(t, state, "restored", "2026-10-13T10:05:00Z")
	if err = signOutAfterRestore(ctx, ident, state); err != nil {
		t.Fatal(err)
	}
	if _, err = ident.Authenticate(session.AccessToken); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("a session survived a restore: %v", err)
	}
	if n := liveIn(filepath.Join(setAside, backup.DatabaseName)); n != 0 {
		t.Fatalf("the copy of the previous database still holds %d live sessions", n)
	}
	// The next ordinary start leaves a new sign-in alone.
	again, err := ident.Issue("account", "profile", "local", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = signOutAfterRestore(ctx, ident, state); err != nil {
		t.Fatal(err)
	}
	if _, err = ident.Authenticate(again.AccessToken); err != nil {
		t.Fatalf("a later start signed out someone who signed in after the restore: %v", err)
	}
}
