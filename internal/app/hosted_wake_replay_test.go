package app

import (
	"testing"
	"time"
)

func TestHostedServerWakeReplayReceiptSurvivesRepeatedDelivery(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	now := time.Now().UTC()
	wake := validHostedWakeForTest(now)
	duplicate, err := server.consumeHostedServerWake(t.Context(), wake, wake.ServerID, now)
	if err != nil || duplicate {
		t.Fatalf("first wake receipt duplicate=%t err=%v", duplicate, err)
	}
	duplicate, err = server.consumeHostedServerWake(t.Context(), wake, wake.ServerID, now.Add(time.Second))
	if err != nil || !duplicate {
		t.Fatalf("replayed wake receipt duplicate=%t err=%v", duplicate, err)
	}
	var receipts int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM hosted_wake_replays WHERE wake_id = ?`, wake.WakeID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("durable wake receipts=%d, want 1", receipts)
	}
}
