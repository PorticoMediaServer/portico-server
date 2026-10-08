package eventfeed

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"portico.local/server/internal/apievents"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/notify"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
)

func TestCommittedNotificationScopeAndRollback(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hub := New(db)
	member := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "member", ProfileID: "one"}}
	other := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "member", ProfileID: "two"}}
	put := func(scope string, commit bool) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		_, err = notify.Bump(tx, time.Now().UnixMilli(), scope, "profile")
		if err != nil {
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	put(operations.ViewerKey(member), false)
	page, err := hub.Read(context.Background(), member, false, 0, true)
	if err != nil || len(page.Events) != 0 {
		t.Fatalf("rolled-back notification was visible: %+v %v", page, err)
	}
	put(operations.ViewerKey(other), true)
	put(operations.ViewerKey(member), true)
	page, err = hub.Read(context.Background(), member, false, 0, true)
	if err != nil || len(page.Events) != 1 || page.Events[0].Type != "notification.changed" || page.Events[0].Resource.ID != "inbox" {
		t.Fatalf("member-scope events: %+v %v", page, err)
	}
	if page.NextAfter != "2" {
		t.Fatalf("cursor failed to advance over invisible event: %q", page.NextAfter)
	}
	if _, err = db.Exec(`DELETE FROM api_events WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	page, err = hub.Read(context.Background(), member, false, 0, true)
	if err != nil || len(page.Events) != 1 || page.Events[0].Type != "stream.resync" {
		t.Fatalf("pruned cursor needs resync: %+v %v", page, err)
	}
}

func TestHubWakesOnCommittedOutboxWrite(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := New(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake, release := h.Subscribe()
	defer release()
	started := make(chan struct{})
	go func() {
		close(started)
		h.Run(ctx)
	}()
	<-started
	// Wait until Run has installed its commit subscription. The initial pass
	// wakes readers, and then this test drains that wake before writing.
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("hub did not start")
	}
	_, err = dbwork.ExecWrite(ctx, db, dbwork.ClassInteractive, `INSERT INTO notification_inbox(scope,audience,revision,updated_ms) VALUES('viewer','profile',1,?)`, time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("committed outbox write did not wake subscribers")
	}
}

func TestStreamReplacementCancelsOldReader(t *testing.T) {
	h := New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release, ok := h.AdmitStream("installation", cancel)
	if !ok {
		t.Fatal("first stream refused")
	}
	defer release()
	if _, ok = h.AdmitStream("installation", func() {}); ok {
		t.Fatal("duplicate stream admitted")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("old stream stayed live")
	}
	if _, ok = h.AdmitStream("installation", func() {}); !ok {
		t.Fatal("retry after retirement refused")
	}
}

func TestOperationEventsRequireLiveOwnerAuthority(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO console_operations(id,kind,resource,actor,trigger,state,phase,revision,attempt,created_ms,updated_ms,next_ms,domain_id,error_code,predecessor,settings_revision)
		 VALUES('op1','library.scan','library','actor','owner','queued','waiting',1,0,1,1,1,'','','',1)`); err != nil {
			return err
		}
		return operations.AppendOperationTx(tx, "op1")
	})
	if err != nil {
		t.Fatal(err)
	}
	h := New(db)
	p := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "member", ProfileID: "one"}}
	member, err := h.Read(context.Background(), p, false, 0, true)
	if err != nil || len(member.Events) != 0 || member.NextAfter != "1" {
		t.Fatalf("member operation visibility: %+v %v", member, err)
	}
	owner, err := h.Read(context.Background(), p, true, 0, true)
	if err != nil || len(owner.Events) != 1 || owner.Events[0].Type != "operation.updated" || owner.Events[0].Resource.ID != "op1" {
		t.Fatalf("owner operation visibility: %+v %v", owner, err)
	}
}

func appendLibraryEvent(t *testing.T, db *sql.DB, library, state string, found int64) {
	t.Helper()
	err := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		return apievents.Append(tx, apievents.LibraryAudience(library), "library.scan.updated", "library", library, strconv.FormatInt(time.Now().UnixMilli(), 10), map[string]any{"state": state, "found": found})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeedDeliversLibraryEventsOnlyToViewersWhoSeeTheLibrary(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	appendLibraryEvent(t, db, "A", "started", 10)
	appendLibraryEvent(t, db, "B", "started", 20)
	h := New(db)
	h.LibraryVisible = func(tx *sql.Tx, p identity.Principal, library string) bool {
		if library == "A" {
			return true
		}
		if library == "B" {
			return p.ProfileID == "one"
		}
		return false
	}
	first := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "member", ProfileID: "one"}}
	second := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "member", ProfileID: "two"}}
	gotFirst, err := h.Read(context.Background(), first, false, 0, true)
	if err != nil || len(gotFirst.Events) != 2 {
		t.Fatalf("first viewer library visibility: %+v %v", gotFirst, err)
	}
	gotSecond, err := h.Read(context.Background(), second, false, 0, true)
	if err != nil || len(gotSecond.Events) != 1 {
		t.Fatalf("second viewer library visibility: %+v %v", gotSecond, err)
	}
	if gotSecond.Events[0].Resource.ID != "A" || gotSecond.Events[0].Type != "library.scan.updated" {
		t.Fatalf("second viewer received wrong event: %+v", gotSecond.Events[0])
	}
	for _, e := range gotSecond.Events {
		if e.Resource.ID == "B" {
			t.Fatalf("second viewer received hidden library B event: %+v", gotSecond)
		}
	}
}

func TestFeedPagingWithHiddenLibraryEvents(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 150; i++ {
		library := "A"
		if i%2 == 1 {
			library = "B"
		}
		appendLibraryEvent(t, db, library, "progress", int64(i))
	}
	h := New(db)
	h.LibraryVisible = func(tx *sql.Tx, p identity.Principal, library string) bool {
		return library == "A"
	}
	p := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "member", ProfileID: "one"}}
	var maximum int64
	if err = db.QueryRow(`SELECT max(id) FROM api_events`).Scan(&maximum); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var collected []string
	after := int64(0)
	pages := 0
	for {
		page, err := h.Read(context.Background(), p, false, after, true)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if pages > 10 {
			t.Fatalf("paging did not terminate: after=%d next=%s collected=%d", after, page.NextAfter, len(collected))
		}
		next, err := strconv.ParseInt(page.NextAfter, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if next <= after {
			t.Fatalf("NextAfter did not advance past hidden rows: after=%d next=%d", after, next)
		}
		for _, e := range page.Events {
			if seen[e.ID] {
				t.Fatalf("repeated visible event %s", e.ID)
			}
			seen[e.ID] = true
			collected = append(collected, e.ID)
			if e.Resource.ID != "A" {
				t.Fatalf("hidden library event leaked: %+v", e)
			}
		}
		after = next
		if after >= maximum {
			if page.NextAfter != strconv.FormatInt(maximum, 10) {
				t.Fatalf("last page must end at newest id %d, got %s", maximum, page.NextAfter)
			}
			break
		}
		if len(page.Events) == 0 && after >= maximum {
			break
		}
	}
	if len(collected) != 75 {
		t.Fatalf("visible count: got %d want 75", len(collected))
	}
	for i := 1; i < len(collected); i++ {
		prev, _ := strconv.ParseInt(collected[i-1], 10, 64)
		cur, _ := strconv.ParseInt(collected[i], 10, 64)
		if cur <= prev {
			t.Fatalf("visible events out of order or skipped: %v", collected)
		}
	}
}
