package scanevents

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/eventfeed"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"testing"
	"time"
)

func TestScanAnalysisProgressChangesWithStableFoundCount(t *testing.T) {
	db, e := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	setupScanLibrary(t, db, "lib", "src", "job")
	insertActive(t, db, "src", "job")
	analyzed := int64(1)
	phase := "inventory"
	p := New(db, func(context.Context, string) (catalog.InventoryStatus, error) {
		return statusWith(catalog.InventorySourceStatus{ID: "src", Status: "running", Discovered: 10, Analyzed: analyzed, Phase: phase}), nil
	}, nil)
	at := time.Now()
	if _, e = p.tick(context.Background(), at); e != nil {
		t.Fatal(e)
	}
	analyzed = 2
	phase = "basic_running"
	if _, e = p.tick(context.Background(), at.Add(100*time.Millisecond)); e != nil {
		t.Fatal(e)
	}
	if len(readScanEvents(t, db)) != 1 {
		t.Fatal("running updates exceeded one per second")
	}
	if _, e = p.tick(context.Background(), at.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	events := readScanEvents(t, db)
	if len(events) != 2 || events[1].data["state"] != "progress" || len(events[1].data) != 2 {
		t.Fatalf("analysis hint missing or viewer payload widened: %+v", events)
	}
}
func TestBackgroundActivityHintsAreAdminOnlyAndRateBound(t *testing.T) {
	db, e := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	if _, e = db.Exec(`INSERT INTO segment_files(object_id,source_revision,library_id,mode,state,updated_ms) VALUES('file','rev','lib','standard','pending',1)`); e != nil {
		t.Fatal(e)
	}
	p := New(db, nil, nil)
	at := time.Now()
	if _, e = p.tickTaskProgress(ctx, at); e != nil {
		t.Fatal(e)
	}
	db.Exec(`UPDATE segment_files SET updated_ms=2`)
	if _, e = p.tickTaskProgress(ctx, at.Add(time.Millisecond)); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM api_events WHERE audience='admin' AND type='job.updated' AND data=''`).Scan(&count); e != nil || count != 1 {
		t.Fatalf("admin hint/rate: %d %v", count, e)
	}
	if _, e = p.tickTaskProgress(ctx, at.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM api_events WHERE audience='admin' AND type='job.updated' AND data=''`).Scan(&count); e != nil || count != 2 {
		t.Fatal(count, e)
	}
	db.Exec(`UPDATE segment_files SET state='complete'`)
	due, e := p.tickTaskProgress(ctx, at.Add(1100*time.Millisecond))
	if e != nil || due != nil || len(p.tasks) != 0 {
		t.Fatal("finished task retained timer/state", due, e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM api_events WHERE audience<>'admin' OR data<>''`).Scan(&count); e != nil || count != 0 {
		t.Fatal("private task facts leaked", count, e)
	}
}

func TestActivityTaskHintsNeverReachMembers(t *testing.T) {
	db, e := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(`INSERT INTO segment_files(object_id,source_revision,library_id,mode,state,updated_ms) VALUES('private-file','r','private-library','standard','pending',1)`); e != nil {
		t.Fatal(e)
	}
	p := New(db, nil, nil)
	if _, e = p.tickTaskProgress(context.Background(), time.Now()); e != nil {
		t.Fatal(e)
	}
	h := eventfeed.New(db)
	for _, role := range []string{"member", "administrator", "owner"} {
		principal := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: role, ProfileID: role, Role: role}}
		out, e := h.Read(context.Background(), principal, role != "member", 0, true)
		if e != nil {
			t.Fatal(e)
		}
		want := 1
		if role == "member" {
			want = 0
		}
		if len(out.Events) != want {
			t.Fatalf("%s events: %+v", role, out)
		}
		for _, event := range out.Events {
			if len(event.Data) != 0 || event.Type != "job.updated" {
				t.Fatalf("private fields escaped to %s: %+v", role, event)
			}
		}
	}
}
