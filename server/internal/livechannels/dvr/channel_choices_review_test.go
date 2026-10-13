package dvr

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/persistence"
)

func TestChannelChoicesPagePastRestrictedPrefix(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	src, gen := strings.Repeat("a", 48), strings.Repeat("b", 48)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO live_source_identities VALUES(?)`, []any{src}},
		{`INSERT INTO live_generations VALUES(?,?,'Guide',2,400,0,'now')`, []any{gen, src}},
		{`INSERT INTO live_sources VALUES(?,'Aerial',1,'active',2,?,'now')`, []any{src, gen}},
	} {
		if _, err = db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("%064x", i+1)
		if _, err = db.Exec(`INSERT INTO live_channel_versions VALUES(?,?,'provider','Channel','1','',?,x'00','','')`, gen, id, i); err != nil {
			t.Fatal(err)
		}
	}
	auth := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "restricted", func(source, channel string) bool {
			return source == src && (channel == "" || channel >= fmt.Sprintf("%064x", 351))
		}, nil
	}
	owner := livechannels.Owner{Authority: "local", AccountID: "account", ProfileID: "profile"}
	s := &Store{db: db}
	cursor := ""
	seen := map[string]bool{}
	for pages := 0; pages < 20; pages++ {
		page, err := s.ChannelChoices(context.Background(), auth, owner, src, "", cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, ch := range page.Channels {
			if ch.ID < fmt.Sprintf("%064x", 351) || seen[ch.ID] {
				t.Fatalf("excluded or duplicated channel %s", ch.ID)
			}
			seen[ch.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		if page.NextCursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = page.NextCursor
	}
	if len(seen) != 50 {
		t.Fatalf("got %d allowed choices, want 50", len(seen))
	}
}
