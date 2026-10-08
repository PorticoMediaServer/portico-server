package administration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/recordingaccess"
)

func TestAdministrativeGroupOwnsRealRuleAndLegacyRemainsDisabled(t *testing.T) {
	s, db, _ := newService(t)
	live, err := livechannels.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.DVR, err = dvr.New(db, live, func(ctx context.Context, tx *sql.Tx, o livechannels.Owner, _, _ string) error {
		return recordingaccess.GrantedTx(ctx, tx, o)
	})
	if err != nil {
		t.Fatal(err)
	}
	src, gen, ch, prog := strings.Repeat("a", 48), strings.Repeat("b", 48), strings.Repeat("c", 64), strings.Repeat("d", 64)
	now := time.Now().UTC()
	start, end := now.Add(time.Hour).Format(time.RFC3339), now.Add(2*time.Hour).Format(time.RFC3339)
	queries := []struct {
		q    string
		args []any
	}{{`INSERT INTO live_source_identities VALUES(?)`, []any{src}}, {`INSERT INTO live_generations VALUES(?,?,'Guide',2,1,1,?)`, []any{gen, src, start}}, {`INSERT INTO live_sources VALUES(?,'Aerial',1,'active',2,?,?)`, []any{src, gen, start}}, {`INSERT INTO live_channel_versions VALUES(?,?,'channel','Channel','1','',1,x'00','','')`, []any{gen, ch}}, {`INSERT INTO live_programmes VALUES(?,?,?,'programme','Episode',?,?,'lineage')`, []any{gen, prog, ch, start, end}}, {`INSERT INTO live_programme_metadata(generation_id,id,series_id,episode_id,new_evidence,description) VALUES(?,?,'series','episode','new','')`, []any{gen, prog}}, {`INSERT INTO admin_dvr_groups VALUES('legacy','keyword','Legacy','news','',1,1,'{}',1,1)`, nil}}
	for _, q := range queries {
		if _, err = db.Exec(q.q, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	c := RecordingGroupChange{Kind: "series", Name: "Series", Match: "series", SourceID: src, OperationID: "real-rule", Anchor: dvr.Occurrence{SourceID: src, ChannelID: ch, Generation: gen, ProgrammeID: prog}}
	if _, err = s.SaveRecordingGroup(ctx, allow, "", c); err == nil {
		t.Fatal("ownerless group accepted")
	}
	c.Owner = livechannels.Owner{Authority: "local", AccountID: "account", ProfileID: "profile"}
	if _, err = s.SaveRecordingGroup(ctx, allow, "", c); err == nil {
		t.Fatal("administration bypassed explicit grant")
	}
	if _, err = db.Exec(`INSERT INTO dvr_recording_grants VALUES('local','account','profile',1,0,1)`); err != nil {
		t.Fatal(err)
	}
	rule, err := s.SaveRecordingGroup(ctx, allow, "", c)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Owner != c.Owner || rule.Options.PrePaddingSeconds == nil || *rule.Options.PrePaddingSeconds != 60 {
		t.Fatal(rule)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM dvr_rule_work WHERE rule_id=?`, rule.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("saved group has no scheduler intent", count, err)
	}
	page, err := s.RecordingGroups(ctx, allow, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatal(page)
	}
	for _, g := range page.Items {
		if g.ID == "legacy" && (g.Enabled || g.Status != "needs-owner-and-guide-selection") {
			t.Fatal(g)
		}
	}
	c.ExpectedRevision = rule.Revision
	c.OperationID = "enable-rule"
	c.Enabled = true
	if _, err = s.SaveRecordingGroup(ctx, allow, rule.ID, c); err != ErrUnavailable {
		t.Fatal("promised capture with no driver", err)
	}
	if err = s.DeleteRecordingGroup(ctx, allow, rule.ID, rule.Revision, "remove-rule"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM dvr_rules WHERE id=? AND deleted=1`, rule.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
