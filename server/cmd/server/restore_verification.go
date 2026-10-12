package main

import (
	"context"
	"errors"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/persistence"
)

// verifyRestoredState is what a restored database must pass before the restore
// is kept. Beyond the database's own checks, it asks the question the next
// startup step asks: does this state folder hold the signing key of the server
// identity the database records? A bare database from another server does
// not, and a server left on it would refuse to start every time. Failing here,
// while the previous database can still be put back, rolls the restore back
// instead. A whole backup folder carries its keys with it and passes.
func verifyRestoredState(state string) backup.VerifyFunc {
	return func(ctx context.Context, path string) error {
		if err := backup.VerifyRestored(ctx, path); err != nil {
			return err
		}
		db, err := persistence.Open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		if err = networking.VerifyStateKeys(ctx, db, state); err != nil {
			return errors.New("backup: the restored database belongs to a server whose identity keys are not in this state folder; restore from a whole backup folder, or move the whole state folder")
		}
		return nil
	}
}
