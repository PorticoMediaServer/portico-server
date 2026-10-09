package librarychannels

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"portico.local/server/internal/livechannels"
)

func seedDirectoryLibrary(t *testing.T, f *fixtureLibrary, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("channel-%02d", i)
		gen := "generation-" + id
		c := Config{Version: ProtocolVersion, ID: id, Name: id, Position: i, Enabled: true, ViewerAccess: "server-members"}
		raw, _ := json.Marshal(c)
		f.exec(t, `INSERT INTO lc_channels(id,revision,config_json,enabled,position,name,active_generation,state) VALUES(?,1,?,1,?,?,?,'ready')`, id, string(raw), i, id, gen)
		f.exec(t, `INSERT INTO lc_generations(id,channel_id,config_revision,config_json,catalog_fence,seed,status,phase,base_generation,start_ms,end_ms,boundary_ms,cursor_ms,created_ms,published_ms) VALUES(?,?,1,?,'','','published','complete','',?,?,?,0,?,?)`, gen, id, string(raw), f.now.Add(-time.Hour).UnixMilli(), f.now.Add(time.Hour).UnixMilli(), f.now.UnixMilli(), f.now.UnixMilli(), f.now.UnixMilli())
		f.exec(t, `INSERT INTO lc_entries(generation_id,occurrence_id,channel_id,start_ms,end_ms,item_id,asset_id,library_id,source_fence,title,rule_id,block_id,source_offset_ms,slate_reason) VALUES(?,?,?, ?,?,0,'','','','Fixture','','',0,'')`, gen, "entry-"+id, id, f.now.Add(-time.Minute).UnixMilli(), f.now.Add(time.Minute).UnixMilli())
	}
}

func TestLibraryGuideHonorsChannelSelectionRowsOnlyAndGroups(t *testing.T) {
	f := openFixture(t)
	seedDirectoryLibrary(t, f, 3)
	s, _ := New(f.db)
	q := livechannels.GuideQuery{Kind: livechannels.LibraryChannel, Start: f.now.Add(-time.Minute), End: f.now.Add(time.Minute), Timezone: "UTC", Limit: 50, ChannelIDs: []string{"channel-02"}}
	key := make([]byte, 32)
	ctx := context.Background()
	guide, err := s.Guide(ctx, ownerAuthority(nil), q, key)
	if err != nil || len(guide.Channels) != 1 || guide.Channels[0].ID != "channel-02" || len(guide.Channels[0].Programmes) != 1 {
		t.Fatalf("selection ignored: %+v %v", guide, err)
	}
	q.Group = "News"
	guide, err = s.Guide(ctx, ownerAuthority(nil), q, key)
	if err != nil || len(guide.Channels) != 0 {
		t.Fatalf("unrelated group leaked channels: %+v %v", guide, err)
	}
	q.Group = "Library Channels"
	q.NoProgrammes = true
	// This proves the rows projection bypasses schedules and all item facts/access
	// work, not simply that it throws the programme array away afterwards.
	f.exec(t, `DROP TABLE lc_entries`)
	guide, err = s.Guide(ctx, ownerAuthority(nil), q, key)
	if err != nil || len(guide.Channels) != 1 || len(guide.Channels[0].Programmes) != 0 || guide.Channels[0].Generation == "" || len(guide.Sources) != 1 {
		t.Fatalf("rows-only selected guide: %+v %v", guide, err)
	}
	q.ChannelIDs = make([]string, 51)
	if _, err = s.Guide(ctx, ownerAuthority(nil), q, key); err != ErrInvalid {
		t.Fatalf("oversized selection admitted: %v", err)
	}
}

func TestLibraryDirectoryBypassesScheduleAndKeepsProfilePreferences(t *testing.T) {
	f := openFixture(t)
	seedDirectoryLibrary(t, f, 3)
	s, _ := New(f.db)
	viewer := livechannels.Owner{Authority: "local", AccountID: "owner", ProfileID: "owner"}
	f.exec(t, `INSERT INTO live_channel_preferences VALUES('local','owner','owner','library:channel-01','channel-01',1,1,7)`)
	f.exec(t, `DROP TABLE lc_entries`)
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	rows, fence, err := s.DirectoryRowsTx(context.Background(), tx, ownerAuthority(nil), viewer)
	if err != nil || len(rows) != 3 || fence == "" {
		t.Fatalf("directory failed: %+v %v", rows, err)
	}
	if !rows[1].Channel.Favorite || !rows[1].Channel.Hidden || rows[1].Channel.PreferenceRevision != 7 {
		t.Fatalf("preference identity lost: %+v", rows[1])
	}
	for _, row := range rows {
		if len(row.Channel.Programmes) != 0 || row.Channel.Generation == "" || row.Source.Provenance != livechannels.LibraryChannel {
			t.Fatalf("directory native handle incomplete: %+v", row)
		}
	}
}
