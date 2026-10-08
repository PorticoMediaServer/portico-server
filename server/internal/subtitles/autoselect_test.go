package subtitles

import (
	"context"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/testauth"
)

func TestChooseAutomatic(t *testing.T) {
	tracks := []AutoCandidate{
		{ID: "en-sdh", Language: "eng", Title: "English [SDH]"},
		{ID: "en", Language: "en", Title: "English"},
		{ID: "en-forced", Language: "eng", Title: "Forced", Forced: true},
		{ID: "es", Language: "spa", Title: "Español"},
	}
	pick := func(c AutoChoice) string {
		if chosen := ChooseAutomatic(c, tracks); chosen != nil {
			return chosen.ID
		}
		return ""
	}
	if got := pick(AutoChoice{Enabled: true, Languages: []string{"en-GB"}}); got != "en" {
		t.Fatal("ordinary track not preferred over SDH and forced:", got)
	}
	if got := pick(AutoChoice{Enabled: true, Languages: []string{"de", "es"}}); got != "es" {
		t.Fatal("second preferred language not used:", got)
	}
	if got := pick(AutoChoice{Enabled: true, Languages: []string{"ja"}}); got != "" {
		t.Fatal("chose a language the viewer did not ask for:", got)
	}
	// Subtitles off: only the forced track beside audio in its own language.
	if got := pick(AutoChoice{AudioLanguage: "eng"}); got != "en-forced" {
		t.Fatal("forced track not shown beside matching audio:", got)
	}
	if got := pick(AutoChoice{AudioLanguage: "fra"}); got != "" {
		t.Fatal("forced track shown beside audio in another language:", got)
	}
	if got := pick(AutoChoice{}); got != "" {
		t.Fatal(got)
	}
	// Only an SDH track in the language: better than nothing.
	tracks = tracks[:1]
	if got := pick(AutoChoice{Enabled: true, Languages: []string{"en"}}); got != "en-sdh" {
		t.Fatal(got)
	}
}

func TestAutoSelectAppliesOnceAndOnlyToStoredText(t *testing.T) {
	c := catalogtest.Open(t)
	db := c.DB
	library := c.Library("lib", "Movies", "movie", "/m")
	item := c.Movie(library, "/m/f.mkv", "Film", 2024)
	c.Drain()
	c.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','profile',1)`)
	c.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,grant_token,grant_hash,mode,duration,state,expires_at,request_id) VALUES('play','login','owner','profile',?,?,1,'g','h','hls',100,'starting','2099-01-01T00:00:00Z','r')`, item.ID, item.Token)
	c.Exec(`INSERT INTO subtitle_resources(id,item_id,source_id,scope,owner,current_revision) VALUES('text',?,?,'shared','',1),('burn',?,?,'shared','',1)`, item.ID, item.Token, item.ID, item.Token)
	c.Exec(`INSERT INTO subtitle_revisions(resource_id,revision,digest,size,format,language,title,origin,rights,original_digest,offset_us,source_size,source_modified_ns,source_facts_revision,created_at) VALUES('text',1,'d',1,'srt','spa','Español','sidecar','r','o',250000,1000,1,1,'now'),('burn',1,'d2',1,'pgs','spa','Español PGS','embedded','r','o2',0,1000,1,1,'now')`)
	c.Exec(`INSERT INTO subtitle_render_revisions VALUES('text',1,'external_text',0,0,''),('burn',1,'burn_in',1,0,'')`)
	testauth.InsertSession(t, db, "login", "owner", "profile", "local", "owner", "2099-01-01T00:00:00Z")
	ctx := context.Background()
	tx, _ := db.Begin()
	if err := AutoSelectTx(ctx, tx, "play", 1, item.Public, item.Token, "viewer", AutoChoice{Enabled: true, Languages: []string{"es"}}); err != nil {
		t.Fatal(err)
	}
	var resource, renderer string
	var offset int64
	if err := tx.QueryRow(`SELECT resource_id,renderer,offset_us FROM playback_subtitle_state WHERE session_id='play'`).Scan(&resource, &renderer, &offset); err != nil || resource != "text" || renderer != "external_text" || offset != 250000 {
		t.Fatal(resource, renderer, offset, err)
	}
	// A second call (a plan read, a recovery) never overrides what is there.
	if _, err := tx.Exec(`UPDATE playback_subtitle_state SET resource_id='',resource_revision=0`); err != nil {
		t.Fatal(err)
	}
	if err := AutoSelectTx(ctx, tx, "play", 1, item.Public, item.Token, "viewer", AutoChoice{Enabled: true, Languages: []string{"es"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`SELECT resource_id FROM playback_subtitle_state WHERE session_id='play'`).Scan(&resource); err != nil || resource != "" {
		t.Fatal("the viewer's own choice of off was overridden", resource, err)
	}
	tx.Rollback()
}
