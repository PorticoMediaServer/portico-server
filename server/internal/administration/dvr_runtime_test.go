package administration

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
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
	}{{`INSERT INTO live_source_identities VALUES(?)`, []any{src}}, {`INSERT INTO live_generations VALUES(?,?,'Guide',2,1,1,?)`, []any{gen, src, start}}, {`INSERT INTO live_sources VALUES(?,'Aerial',1,'active',2,?,?)`, []any{src, gen, start}}, {`INSERT INTO live_channel_versions(generation_id,channel_id,provider_key,name,number,source_number,group_name,position,locator_sealed,available_start,available_end) VALUES(?,?,'channel','Channel','1',NULL,'',1,x'00','','')`, []any{gen, ch}}, {`INSERT INTO live_programmes VALUES(?,?,?,'programme','Episode',?,?,'lineage')`, []any{gen, prog, ch, start, end}}, {`INSERT INTO live_programme_metadata(generation_id,id,series_id,episode_id,new_evidence,description) VALUES(?,?,'series','episode','new','')`, []any{gen, prog}}, {`INSERT INTO admin_dvr_groups VALUES('legacy','keyword','Legacy','news','',1,1,'{}',1,1)`, nil}}
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
	if rule.Owner != c.Owner || rule.Options.PrePaddingMs == nil || *rule.Options.PrePaddingMs != 60000 {
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
	// A rule created through the DVR API can carry criteria and options that
	// are absent from the administrative projection.
	var raw string
	if err = db.QueryRow(`SELECT config_json FROM dvr_rules WHERE id=?`, rule.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var original dvr.RuleConfig
	if err = json.Unmarshal([]byte(raw), &original); err != nil {
		t.Fatal(err)
	}
	original.AllowedChannels = []string{ch}
	original.BlockedChannels = []string{strings.Repeat("f", 64)}
	original.BlockedKeywords = []string{"repeat"}
	original.Keywords = []string{"episode"}
	original.Options = dvr.Options{BeforeMs: 3600000, AfterMs: 7200000, Priority: 500, RetentionDays: 30, EpisodeLimit: 5000}
	direct, err := s.DVR.SaveRule(ctx, groupAuthority(allow), c.Owner, dvr.RuleInput{ID: rule.ID, Mutation: dvr.Mutation{ExpectedRevision: rule.Revision, RequestID: strings.Repeat("e", 48)}, Config: original})
	if err != nil {
		t.Fatal(err)
	}
	projected := groupFromRule(direct, c.Owner)
	c.ExpectedRevision, c.OperationID, c.Options, c.Name = direct.Revision, "rename-rule", projected.Options, "Renamed"
	updated, err := s.SaveRecordingGroup(ctx, allow, rule.ID, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT config_json FROM dvr_rules WHERE id=?`, rule.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var retained dvr.RuleConfig
	if err = json.Unmarshal([]byte(raw), &retained); err != nil {
		t.Fatal(err)
	}
	original.Name = "Renamed"
	if !reflect.DeepEqual(retained, original) {
		t.Fatalf("projection lost original config: got %+v want %+v", retained, original)
	}
	if _, err = s.SaveRecordingGroup(ctx, allow, rule.ID, c); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	stale := c
	stale.OperationID = "stale-edit"
	if _, err = s.SaveRecordingGroup(ctx, allow, rule.ID, stale); err != ErrConflict {
		t.Fatalf("stale edit: %v", err)
	}
	c.ExpectedRevision, c.OperationID = updated.Revision, "change-policy"
	c.Options.Keep = &KeepPolicy{Mode: "keep-days", KeepDays: 14}
	updated, err = s.SaveRecordingGroup(ctx, allow, rule.ID, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT config_json FROM dvr_rules WHERE id=?`, rule.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(raw), &retained); err != nil {
		t.Fatal(err)
	}
	original.Options.RetentionDays, original.Options.EpisodeLimit = 14, 0
	if !reflect.DeepEqual(retained, original) {
		t.Fatalf("policy edit changed other config: got %+v want %+v", retained, original)
	}
	rule = updated
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
