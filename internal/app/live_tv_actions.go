package app

import (
	"strings"
	"time"
)

const (
	liveTVActionSourceEdit      = "live.source.edit"
	liveTVActionSourceRefresh   = "live.source.refresh"
	liveTVActionSourceDelete    = "live.source.delete"
	liveTVActionPlay            = "live.play"
	liveTVActionChannelManage   = "live.channel.manage"
	liveTVActionFavoriteAdd     = "favorite.add"
	liveTVActionFavoriteRemove  = "favorite.remove"
	liveTVActionDVRRecord       = "dvr.record"
	liveTVActionDVRRecordSeries = "dvr.record-series"
	liveTVActionDVRPlay         = "dvr.play"
	liveTVActionDVRCancel       = "dvr.cancel"
	liveTVActionDVRDelete       = "dvr.delete"
	liveTVActionDVREdit         = "dvr.edit"
	liveTVActionDVREnable       = "dvr.enable"
	liveTVActionDVRDisable      = "dvr.disable"
	liveTVActionDVRRuleCreate   = "dvr.rule.create"
)

func liveTVSourceActionsForUser(_ LiveTVSource, user User) []string {
	if !canManageLiveTVSources(user) {
		return []string{}
	}
	return []string{liveTVActionSourceEdit, liveTVActionSourceRefresh, liveTVActionSourceDelete}
}

func applyLiveTVSourceActions(source *LiveTVSource, user User) {
	if source != nil {
		source.Actions = liveTVSourceActionsForUser(*source, user)
	}
}

func applyLiveTVSourcesActions(sources []LiveTVSource, user User) {
	for index := range sources {
		applyLiveTVSourceActions(&sources[index], user)
	}
}

func liveTVChannelActionsForUser(channel LiveTVChannel, user User) []string {
	actions := []string{}
	if !channel.Enabled {
		return actions
	}
	if canPlayLiveTV(user) && !channel.Hidden {
		actions = append(actions, liveTVActionPlay)
	}
	if canViewLiveTV(user) {
		if channel.Favorite {
			actions = append(actions, liveTVActionFavoriteRemove)
		} else {
			actions = append(actions, liveTVActionFavoriteAdd)
		}
	}
	if canManageLiveTVSources(user) {
		actions = append(actions, liveTVActionChannelManage)
	}
	return actions
}

func applyLiveTVChannelActions(channel *LiveTVChannel, user User) {
	if channel != nil {
		channel.Actions = liveTVChannelActionsForUser(*channel, user)
	}
}

func applyLiveTVChannelsActions(channels []LiveTVChannel, user User) {
	for index := range channels {
		applyLiveTVChannelActions(&channels[index], user)
	}
}

func liveTVProgramActionsForUser(program LiveTVProgram, user User, now time.Time) []string {
	actions := []string{}
	if strings.TrimSpace(program.ChannelID) == "" {
		return actions
	}
	if canPlayLiveTV(user) {
		actions = append(actions, liveTVActionPlay)
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(program.EndAt))
	if canScheduleDVR(user) && (err != nil || end.After(now)) {
		actions = append(actions, liveTVActionDVRRecord, liveTVActionDVRRecordSeries)
	}
	return actions
}

func applyLiveTVProgramActions(program *LiveTVProgram, user User, now time.Time) {
	if program != nil {
		program.Actions = liveTVProgramActionsForUser(*program, user, now)
	}
}

func applyLiveTVProgramsActions(programs []LiveTVProgram, user User, now time.Time) {
	for index := range programs {
		applyLiveTVProgramActions(&programs[index], user, now)
	}
}

func applyLiveTVGuideActions(guide *LiveTVGuideResponse, user User) {
	if guide == nil {
		return
	}
	applyLiveTVSourceActions(&guide.Source, user)
	applyLiveTVChannelsActions(guide.Channels, user)
	applyLiveTVProgramsActions(guide.Programs, user, time.Now().UTC())
	guide.Capabilities = LiveTVGuideCapabilities{
		CanPlay:                 canPlayLiveTV(user),
		CanFavoriteChannels:     canViewLiveTV(user),
		CanScheduleRecordings:   canScheduleDVR(user),
		CanManageRecordingRules: canScheduleDVR(user),
		CanManageSources:        canManageLiveTVSources(user),
	}
}

func dvrRuleActionsForUser(rule DVRRecordingRule, user User) []string {
	if !userCanModifyDVRRule(user, rule) {
		return []string{}
	}
	actions := []string{liveTVActionDVREdit}
	if rule.Enabled {
		actions = append(actions, liveTVActionDVRDisable)
	} else {
		actions = append(actions, liveTVActionDVREnable)
	}
	return append(actions, liveTVActionDVRDelete)
}

func applyDVRRuleActions(rule *DVRRecordingRule, user User) {
	if rule != nil {
		rule.Actions = dvrRuleActionsForUser(*rule, user)
	}
}

func applyDVRRulesActions(rules []DVRRecordingRule, user User) {
	for index := range rules {
		applyDVRRuleActions(&rules[index], user)
	}
}

func dvrRecordingActionsForUser(recording DVRRecording, user User) []string {
	actions := []string{}
	status := strings.ToLower(strings.TrimSpace(recording.Status))
	channelAllowed := liveTVChannelAllowedByPolicy(recording.ChannelID, user.ChannelPolicy)
	playable := status == "running" && strings.TrimSpace(recording.ChannelID) != "" && channelAllowed && canPlayLiveTV(user)
	playable = playable || (dvrRecordingPlayableFileStatus(status) && strings.TrimSpace(recording.Path) != "" && channelAllowed)
	if playable && hasPermission(user, "playMedia") {
		actions = append(actions, liveTVActionDVRPlay)
	}
	if status == "scheduled" && userCanDeleteDVRRecording(user, recording) {
		actions = append(actions, liveTVActionDVRCancel, liveTVActionDVREdit)
	} else if dvrRecordingFinishedStatus(status) && userCanDeleteDVRRecording(user, recording) {
		actions = append(actions, liveTVActionDVRDelete)
	}
	return actions
}

func applyDVRRecordingActions(recording *DVRRecording, user User) {
	if recording != nil {
		recording.Actions = dvrRecordingActionsForUser(*recording, user)
	}
}

func dvrRecordingPlayableFileStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "complete", "completed":
		return true
	default:
		return false
	}
}

func applyDVRRecordingsActions(recordings []DVRRecording, user User) {
	for index := range recordings {
		applyDVRRecordingActions(&recordings[index], user)
	}
}

func applyDVRRecordingGroupActions(groups []DVRRecordingGroup, user User) {
	for index := range groups {
		applyDVRRecordingsActions(groups[index].Recordings, user)
	}
}
