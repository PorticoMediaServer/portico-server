package dbwork

import (
	"context"
	"database/sql"
	"sync"
	"testing"
)

// The driver seam holds the copying boundary, not a writer transaction. The
// physical-WAL tests separately exercise real SQLite copying, reader pressure
// and file truncation. Here a writer deterministically crosses that boundary:
// the reset must defer even when PASSIVE reported every old frame copied.
func TestCheckpointDoesNotResetAcrossAnotherWriter(t *testing.T) {
	for _, activeAtCopy := range []bool{false, true} {
		name := "writer-between-copy-and-reset"
		if activeAtCopy {
			name = "writer-already-in-flight"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var release func()
			if activeAtCopy {
				var err error
				release, err = WriteGate().Acquire(ctx, ClassBackgroundMedia)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			}
			var once sync.Once
			conn := &checkpointCleanupConn{busy: 3210, checkpoint: func() {
				once.Do(func() {
					if release != nil {
						release()
						return
					}
					free, err := WriteGate().Acquire(ctx, ClassBackgroundMedia)
					if err != nil {
						t.Fatal(err)
					}
					free()
				})
			}}
			db := sql.OpenDB(checkpointCleanupConnector{conn})
			defer db.Close()
			path := schedulePressureFile(t)
			result := checkpoint(ctx, db, path)
			if result.Err != nil || result.Mode != "passive" {
				t.Fatalf("reset raced a writer after copying: %+v", result)
			}
			// The next pass, with no intervening writer, must still reset: a
			// defensive fence cannot strand permanent physical WAL pressure.
			conn.checkpoint = nil
			result = checkpoint(ctx, db, path)
			if result.Err != nil || result.Mode != "truncate" {
				t.Fatalf("reset did not recover at a clean copy boundary: %+v", result)
			}
		})
	}
}
