package dbwork

import (
	"context"
	"time"
)

// PaceBackground gives foreground requests a brief scheduling opportunity after
// a completed background quantum. Call it only outside transactions, after the
// batch has released its writer/read leases. With no foreground pressure the
// next batch starts immediately. Under sustained pressure every batch still
// progresses: the pause is bounded and cancellation interrupts it.
//
// Yield remains a nonblocking cancellation boundary; this separate operation is
// intended for the continuous bulk/derived-data loops that can otherwise consume
// CPU and fill the WAL as quickly as they commit.
func PaceBackground(ctx context.Context, batchStarted time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	if !ForegroundWorkActive() {
		return true
	}
	elapsed := time.Since(batchStarted)
	pause := 50 * time.Millisecond
	// Compare before multiplication so even a stale start cannot overflow.
	if elapsed < pause/3 {
		pause = max(2*time.Millisecond, 3*elapsed)
	}
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-timer.C:
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}
