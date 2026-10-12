package social

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"strings"
	"testing"
)

func TestWatchTogetherPrivacyHidesPresenceAndActivityImmediately(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	p := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile"}}
	group := seedGroup(t, s, p)
	ctx := context.Background()
	publish := func() {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err = s.publish(ctx, tx, group, EventReadiness, map[string]any{"memberId": "mem_seeded", "positionUs": "500"}); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	publish()
	if _, err = db.Exec(`INSERT INTO console_documents(scope,revision,body,updated_ms) VALUES(?,1,?,0)`, "profile:"+operations.ViewerScopeKey(p.Viewer), `{"privacy.includeInWatchTogether":false}`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := loadGroup(ctx, tx, group)
	if err != nil {
		t.Fatal(err)
	}
	members, summary, err := s.members(ctx, tx, g)
	tx.Rollback()
	if err != nil || len(members) != 0 || summary.MemberCount != 0 {
		t.Fatal(members, summary, err)
	}
	publish()
	events, _, err := s.EventsSince(ctx, group, 0)
	if err != nil || len(events) != 1 {
		t.Fatal(events, err)
	}
	if strings.Contains(string(events[0].Payload), "500") || strings.Contains(string(events[0].Payload), "mem_seeded") {
		t.Fatal("historical activity disclosed after opt-out", string(events[0].Payload))
	}
	if _, err = db.Exec(`UPDATE console_documents SET body='{"privacy.includeInWatchTogether":true}'`); err != nil {
		t.Fatal(err)
	}
	publish()
	events, _, err = s.EventsSince(ctx, group, 0)
	if err != nil || len(events) != 2 {
		t.Fatal(events, err)
	}
}
