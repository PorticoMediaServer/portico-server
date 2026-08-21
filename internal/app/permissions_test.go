package app

import "testing"

func TestLiveDVRPermissionHelpersUseGranularScopes(t *testing.T) {
	regular := User{Permissions: map[string]bool{}}
	if canViewLiveTV(regular) || canPlayLiveTV(regular) || canViewDVR(regular) || canManageDVR(regular) {
		t.Fatal("regular user unexpectedly received Live TV or DVR access")
	}

	liveViewer := User{Permissions: map[string]bool{"viewLiveTV": true}}
	if !canViewLiveTV(liveViewer) || canPlayLiveTV(liveViewer) || canManageLiveTV(liveViewer) {
		t.Fatal("viewLiveTV should view guide data without playback or source management")
	}

	livePlayer := User{Permissions: map[string]bool{"playLiveTV": true}}
	if !canViewLiveTV(livePlayer) || !canPlayLiveTV(livePlayer) || canManageLiveTV(livePlayer) {
		t.Fatal("playLiveTV should include guide visibility and playback only")
	}

	scheduler := User{Permissions: map[string]bool{"scheduleDVR": true}}
	if !canViewDVR(scheduler) || !canScheduleDVR(scheduler) || canDeleteDVRRecording(scheduler) || canManageDVR(scheduler) {
		t.Fatal("scheduleDVR should view/schedule DVR without delete or management rights")
	}

	manager := User{Permissions: map[string]bool{"manageDVR": true}}
	if !canViewLiveTV(manager) || canManageLiveTV(manager) || !canViewDVR(manager) || !canScheduleDVR(manager) || !canDeleteDVRRecording(manager) {
		t.Fatal("manageDVR should provide viewer-safe Live TV plus DVR workflow access without source administration")
	}
	if canPlayLiveTV(manager) {
		t.Fatal("manageDVR should not grant Live TV playback by itself")
	}
	serverManager := User{Role: "owner", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true}}
	if !canManageLiveTV(serverManager) || !canManageLiveTVSources(serverManager) {
		t.Fatal("manageServer should authorize Live TV source administration")
	}
}

func TestScopedSettingsPermissionsDoNotExposeGlobalServerSettings(t *testing.T) {
	dvrManager := User{Permissions: map[string]bool{"manageDVR": true}}
	if canViewServerSettings(dvrManager) {
		t.Fatal("ordinary user with manageDVR should not be able to open Server Settings")
	}
	if canWriteSettingGroup(dvrManager, "dvr") {
		t.Fatal("ordinary user with manageDVR should not be able to write DVR settings")
	}
	for _, key := range []string{"server", "network", "transcoder", "library", "devices"} {
		if canWriteSettingGroup(dvrManager, key) {
			t.Fatalf("manageDVR user should not be able to write %q settings", key)
		}
	}
	if keys := allowedSettingKeysForUser(dvrManager); len(keys) != 0 {
		t.Fatalf("ordinary user allowed setting keys = %#v, expected none", keys)
	}

	allowedSummary := allowedSettingsSummaryIDsForUser(dvrManager)
	if len(allowedSummary) != 0 {
		t.Fatalf("ordinary user settings summary = %#v, expected none", allowedSummary)
	}
}
