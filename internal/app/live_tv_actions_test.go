package app

import (
	"testing"
	"time"
)

func TestLiveTVActionsArePermissionAndResourceStateFiltered(t *testing.T) {
	viewer := User{ID: "usr_live_viewer", Permissions: map[string]bool{"viewLiveTV": true, "playLiveTV": true}}
	channel := LiveTVChannel{ID: "channel_1", Enabled: true, Favorite: false}
	actions := stringSet(liveTVChannelActionsForUser(channel, viewer))
	for _, required := range []string{liveTVActionPlay, liveTVActionFavoriteAdd} {
		if !actions[required] {
			t.Errorf("channel actions omitted %q: %#v", required, actions)
		}
	}
	if actions[liveTVActionChannelManage] {
		t.Errorf("viewer received channel-management action: %#v", actions)
	}

	channel.Favorite = true
	actions = stringSet(liveTVChannelActionsForUser(channel, viewer))
	if !actions[liveTVActionFavoriteRemove] || actions[liveTVActionFavoriteAdd] {
		t.Errorf("favorite state was not reflected in channel actions: %#v", actions)
	}
	channel.Hidden = true
	if hiddenActions := stringSet(liveTVChannelActionsForUser(channel, viewer)); hiddenActions[liveTVActionPlay] {
		t.Errorf("hidden channel advertised playback: %#v", hiddenActions)
	}

	program := LiveTVProgram{ID: "program_1", ChannelID: channel.ID, EndAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}
	actions = stringSet(liveTVProgramActionsForUser(program, viewer, time.Now().UTC()))
	if !actions[liveTVActionPlay] || actions[liveTVActionDVRRecord] || actions[liveTVActionDVRRecordSeries] {
		t.Errorf("viewer program actions = %#v", actions)
	}
	scheduler := User{ID: "usr_scheduler", Permissions: map[string]bool{"viewLiveTV": true, "scheduleDVR": true}}
	actions = stringSet(liveTVProgramActionsForUser(program, scheduler, time.Now().UTC()))
	if actions[liveTVActionPlay] || !actions[liveTVActionDVRRecord] || !actions[liveTVActionDVRRecordSeries] {
		t.Errorf("scheduler program actions = %#v", actions)
	}
}

func TestDVRResourceActionsUseOwnershipPermissionAndStatus(t *testing.T) {
	scheduler := User{ID: "usr_scheduler", AccountID: "usr_scheduler", ProfileID: "usr_scheduler", ProfileIsPrimary: true, Permissions: map[string]bool{"scheduleDVR": true, "viewDVR": true, "playMedia": true}}
	rule := DVRRecordingRule{ID: "rule_1", UserID: scheduler.ID, Enabled: true}
	actions := stringSet(dvrRuleActionsForUser(rule, scheduler))
	for _, required := range []string{liveTVActionDVREdit, liveTVActionDVRDisable, liveTVActionDVRDelete} {
		if !actions[required] {
			t.Errorf("owned rule actions omitted %q: %#v", required, actions)
		}
	}
	if actions[liveTVActionDVREnable] {
		t.Errorf("enabled rule advertised enable: %#v", actions)
	}
	rule.Enabled = false
	if disabledActions := stringSet(dvrRuleActionsForUser(rule, scheduler)); !disabledActions[liveTVActionDVREnable] || disabledActions[liveTVActionDVRDisable] {
		t.Errorf("disabled rule actions = %#v", disabledActions)
	}
	rule.UserID = "usr_other"
	if otherActions := dvrRuleActionsForUser(rule, scheduler); len(otherActions) != 0 {
		t.Errorf("scheduler received actions for another member's rule: %#v", otherActions)
	}

	scheduled := DVRRecording{ID: "recording_1", UserID: scheduler.ID, Status: "scheduled"}
	actions = stringSet(dvrRecordingActionsForUser(scheduled, scheduler))
	if !actions[liveTVActionDVRCancel] || !actions[liveTVActionDVREdit] || actions[liveTVActionDVRPlay] || actions[liveTVActionDVRDelete] {
		t.Errorf("scheduled recording actions = %#v", actions)
	}
	completed := DVRRecording{ID: "recording_2", UserID: scheduler.ID, ChannelID: "channel_1", Status: "complete", Path: "/recordings/example.ts"}
	actions = stringSet(dvrRecordingActionsForUser(completed, scheduler))
	if !actions[liveTVActionDVRPlay] || actions[liveTVActionDVRDelete] {
		t.Errorf("completed scheduler recording actions = %#v", actions)
	}
	deleter := scheduler
	deleter.Permissions["deleteDVRRecordings"] = true
	actions = stringSet(dvrRecordingActionsForUser(completed, deleter))
	if !actions[liveTVActionDVRPlay] || !actions[liveTVActionDVRDelete] {
		t.Errorf("completed deleter recording actions = %#v", actions)
	}
	failed := completed
	failed.Status = "failed"
	if failedActions := stringSet(dvrRecordingActionsForUser(failed, deleter)); failedActions[liveTVActionDVRPlay] || !failedActions[liveTVActionDVRDelete] {
		t.Errorf("failed recording actions = %#v", failedActions)
	}
	running := DVRRecording{ID: "recording_3", UserID: scheduler.ID, Status: "running", ChannelID: "channel_1"}
	if runningActions := stringSet(dvrRecordingActionsForUser(running, scheduler)); runningActions[liveTVActionDVRPlay] {
		t.Errorf("running recording advertised playback without Live TV playback permission: %#v", runningActions)
	}
	scheduler.Permissions["playLiveTV"] = true
	if runningActions := stringSet(dvrRecordingActionsForUser(running, scheduler)); !runningActions[liveTVActionDVRPlay] {
		t.Errorf("running recording omitted playback with Live TV playback permission: %#v", runningActions)
	}
}

func TestLiveTVGuideCapabilitiesAndNestedActionsAreExplicit(t *testing.T) {
	user := User{ID: "usr_live_admin", Permissions: map[string]bool{
		"playLiveTV": true, "scheduleDVR": true, "manageDVR": true,
	}}
	guide := LiveTVGuideResponse{
		Source:   LiveTVSource{ID: "source_1", Enabled: true},
		Channels: []LiveTVChannel{{ID: "channel_1", Enabled: true}},
		Programs: []LiveTVProgram{{ID: "program_1", ChannelID: "channel_1", EndAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}},
	}
	applyLiveTVGuideActions(&guide, user)
	if !guide.Capabilities.CanPlay || !guide.Capabilities.CanScheduleRecordings || !guide.Capabilities.CanManageRecordingRules || guide.Capabilities.CanManageSources {
		t.Fatalf("guide capabilities = %#v", guide.Capabilities)
	}
	if len(guide.Source.Actions) != 0 || len(guide.Channels[0].Actions) == 0 || len(guide.Programs[0].Actions) == 0 {
		t.Fatalf("guide nested actions were not projected: %#v", guide)
	}
}
