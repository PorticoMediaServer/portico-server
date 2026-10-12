package main

import (
	"context"
	"errors"

	"portico.local/server/internal/backup"
	"portico.local/server/internal/networking"
	"portico.local/server/internal/persistence"
)

// verifyRestoredState is what a restored state folder must pass before the
// restore is kept. Beyond the database's own checks, it asks what the next
// start-up steps ask before they open the server's identity, in the same
// order and with the same code, reading only:
//
//   - the authority incarnation in the folder is present and well formed;
//   - the database's networking claim schema is exactly the current one;
//   - the folder holds the signing key of the identity the database records.
//
// A server left on a state that fails any of these would refuse every start.
// Failing here, while the previous state can still be put back, rolls the
// restore back instead. Nothing is created or adopted to make a restore pass:
// a bare database from another server has no keys here and is refused, and a
// whole backup folder is judged on the files it brought.
func verifyRestoredState(state string) backup.VerifyFunc {
	return func(ctx context.Context, path string) error {
		if err := backup.VerifyRestored(ctx, path); err != nil {
			return err
		}
		if _, err := readCurrentAuthority(state); err != nil {
			return errors.New("backup: the restored state has no usable server authority file; restore from a whole, unmodified backup folder, or move the whole state folder")
		}
		db, err := persistence.Open(path)
		if err != nil {
			return err
		}
		defer db.Close()
		if err = persistence.VerifyNetworkingClaims(ctx, db); err != nil {
			return errors.New("backup: the restored database's server identity tables are not in the shape this version requires")
		}
		if err = networking.VerifyStateKeys(ctx, db, state); err != nil {
			return errors.New("backup: the restored database belongs to a server whose identity keys are not in this state folder; restore from a whole backup folder, or move the whole state folder")
		}
		return nil
	}
}
