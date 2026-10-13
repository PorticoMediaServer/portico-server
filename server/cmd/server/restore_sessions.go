package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/identity"
)

// signOutAfterRestore runs once on the first start after a restore was
// applied. Everyone is signed out of the restored database, and the copies of
// the previous database that restores set aside ("pre-restore" backups) have
// their sessions ended as well: those copies are the live database of a moment
// ago moved into the backups folder, so they never went through the scrub a
// written backup gets.
//
// A sign-out that cannot be done stops the start. Serving from a restored
// database with its old sessions intact is the thing this exists to prevent.
func signOutAfterRestore(ctx context.Context, ident *identity.Service, state string) error {
	result, err := backup.ReadRestoreResult(state)
	if err != nil || result == nil || result.Outcome != "restored" {
		// No restore, one that was rolled back, or an unreadable record of an
		// old one: the database in place is the one that was already running.
		return nil
	}
	acted, err := ident.SignEveryoneOutAfterRestore(ctx, result.At)
	if err != nil {
		return err
	}
	if !acted {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(state, backup.BackupsDir))
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), "-pre-restore") {
			continue
		}
		copyPath := filepath.Join(state, backup.BackupsDir, entry.Name(), backup.DatabaseName)
		if _, err = os.Stat(copyPath); err != nil {
			continue
		}
		if err = identity.ScrubBackupCopy(ctx, copyPath); err != nil {
			log.Printf("The copy of the previous database kept by the restore (%s) still holds its sign-ins: %v", entry.Name(), err)
		}
	}
	return nil
}
