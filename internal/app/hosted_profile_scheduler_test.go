package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestHostedProfileRefreshSchedulerBoundsLargeOutageAndJoinsOnShutdown(t *testing.T) {
	background, cancel := context.WithCancel(context.Background())
	server := &Server{
		backgroundCtx:    background,
		backgroundCancel: cancel,
		log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	const accountCount = 100_000
	for index := 0; index < accountCount; index++ {
		server.startHostedProfileDirectoryRefresh(
			fmt.Sprintf("outage-account-%d", index),
			hostedProfileSnapshotState{},
			errors.New("simulated outage"),
			time.Now().UTC(),
		)
	}

	server.hostedProfileRefreshMu.Lock()
	retained := len(server.hostedProfileRefreshes)
	server.hostedProfileRefreshMu.Unlock()
	retentionBound := hostedProfileRefreshBacklog + hostedProfileRefreshConcurrency + 1 // one scheduler dispatch slot
	if retained > retentionBound {
		t.Fatalf("retained %d refresh calls for %d accounts; bound=%d", retained, accountCount, retentionBound)
	}

	server.BeginShutdown()
	server.closeOwnedAsync()
	server.hostedProfileRefreshMu.Lock()
	remaining := len(server.hostedProfileRefreshes)
	server.hostedProfileRefreshMu.Unlock()
	if remaining != 0 {
		t.Fatalf("shutdown left %d refresh calls behind", remaining)
	}
}
