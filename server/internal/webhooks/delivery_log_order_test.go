package webhooks

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func deliveryLogFixture(t *testing.T) (*Store, string) {
	t.Helper()
	s, db := open(t)
	id := "delivery-log-order"
	// This normalized fixture exercises the real log reader independently of
	// the separate current integration-settings producer/schema work.
	_, err := db.Exec(`INSERT INTO webhooks(id,name,url,secret,events,created_ms,updated_ms) VALUES(?,'Log fixture','https://receiver.invalid/','fixture-signing-secret',',server.started,backup.failed,',1,1)`, id)
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestWebhookDeliveryLogUsesSequenceForEqualTimestamps(t *testing.T) {
	s, id := deliveryLogFixture(t)
	now := time.Now().UnixMilli()
	for _, row := range []struct {
		id, event, state, problem string
		status                    int
	}{
		{"z-earlier", ServerStarted, "delivered", "", 204},
		{"a-later", BackupFailed, "pending", "status_500", 500},
	} {
		if _, err := s.DB.Exec(`INSERT INTO webhook_deliveries(id,webhook_id,type,data,state,attempts,created_ms,next_attempt_ms,status,error) VALUES(?,?,?,'{}',?,1,?,?,?,?)`, row.id, id, row.event, row.state, now, now+30000, row.status, row.problem); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Deliveries(context.Background(), nil, id, "", 0)
	if err != nil || len(page.Items) != 2 || page.Items[0].Event != BackupFailed || page.Items[0].State != "pending" || page.Items[0].Attempts != 1 || page.Items[0].Status != 500 || page.Items[0].Error != "status_500" || page.Items[0].NextAttempt == "" || page.Items[1].Event != ServerStarted || page.Items[1].State != "delivered" {
		t.Fatalf("equal timestamps reordered the actual delivery log: %#v, %v", page.Items, err)
	}
}

func TestWebhookDeliveryLogPagesEqualTimestampsAfterBoundaryDeletion(t *testing.T) {
	s, id := deliveryLogFixture(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	// The later timestamp sorts first even though it was inserted before the
	// equal-time rows. Within a timestamp, newest means insertion sequence.
	if _, err := s.DB.Exec(`INSERT INTO webhook_deliveries(id,webhook_id,type,data,next_attempt_ms,created_ms) VALUES('latest-time',?,?,'{}',?,?)`, id, ServerStarted, now+1, now+1); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := s.DB.Exec(`INSERT INTO webhook_deliveries(id,webhook_id,type,data,next_attempt_ms,created_ms) VALUES(?,?,?,'{}',?,?)`, fmt.Sprintf("row-%d", 5-i), id, BackupFailed, now, now); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.Deliveries(ctx, nil, id, "", 2)
	if err != nil || len(first.Items) != 2 || first.Items[0].ID != "latest-time" || first.Items[1].ID != "row-1" || first.NextCursor == "" {
		t.Fatalf("first sequence page: %#v, %v", first, err)
	}
	if _, err := s.DB.Exec(`DELETE FROM webhook_deliveries WHERE id=?`, first.Items[1].ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.Deliveries(ctx, nil, id, first.NextCursor, 2)
	if err != nil || len(second.Items) != 2 || second.Items[0].ID != "row-2" || second.Items[1].ID != "row-3" || second.NextCursor == "" {
		t.Fatalf("deleted boundary changed next sequence page: %#v, %v", second, err)
	}
	last, err := s.Deliveries(ctx, nil, id, second.NextCursor, 2)
	if err != nil || len(last.Items) != 2 || last.Items[0].ID != "row-4" || last.Items[1].ID != "row-5" || last.NextCursor != "" {
		t.Fatalf("last sequence page: %#v, %v", last, err)
	}
}
