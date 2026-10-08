package identity

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDirectProfileErasureStopsOnlyItsContainerDownloads(t *testing.T) {
	s, db, login := directFixture(t)
	ctx := context.Background()
	profiles, e := s.CreateDirectProfile(ctx, login.AccountToken, "Delete downloads", "blue")
	if e != nil {
		t.Fatal(e)
	}
	child := childProfile(t, profiles)
	for _, authority := range []string{"local", "hosted"} {
		key, _ := json.Marshal([]string{authority, "account", child.ID})
		if _, e = db.Exec(`INSERT INTO download_requests(id,profile_key,operation_id,request_hash,principal,device_id,target_kind,target_id,quality,episodes,keep_next,selection_fence,created_ms,updated_ms) VALUES(?,?,'op','hash','{}','device','show',1,'original','all',0,'fence',1,1)`, authority, string(key)); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(`INSERT INTO download_preparations(id,profile_key,authority,account_id,profile_id,item_id,library_id,quality,origin,state,created_ms,updated_ms) VALUES(?,?,?,'account',?,1,'lib','original','container','queued',1,1)`, authority, string(key), authority, child.ID); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(`INSERT INTO download_preparation_authority(preparation_id,principal,device_id) VALUES(?,'{}','device')`, authority); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.DeleteDirectProfile(ctx, login.AccountToken, child.ID, child.Revision, true); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"download_requests", "download_preparation_authority"} {
		var n int
		if e = db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); e != nil || n != 1 {
			t.Fatal(table, n, e)
		}
	}
	for _, authority := range []string{"local", "hosted"} {
		var state string
		if e = db.QueryRow(`SELECT state FROM download_preparations WHERE id=?`, authority).Scan(&state); e != nil {
			t.Fatal(e)
		}
		want := "queued"
		if authority == "local" {
			want = "cancelled"
		}
		if state != want {
			t.Fatal(authority, state)
		}
	}
}
