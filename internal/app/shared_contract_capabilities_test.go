package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalProductContractPublishesEventTransportsAndConsumerDVRActions(t *testing.T) {
	contract := canonicalProductContract()
	if len(contract.EventTransports) != 2 || contract.EventTransports[0] != "sse" || contract.EventTransports[1] != "long-poll" {
		t.Fatalf("event transports = %#v", contract.EventTransports)
	}
	if contract.LongPoll.DefaultWaitSeconds != 20 || contract.LongPoll.MaximumWaitSeconds != 25 || contract.LongPoll.MaximumConcurrentStreams != 4 {
		t.Fatalf("long-poll capabilities = %#v", contract.LongPoll)
	}
	wantActions := map[string]bool{
		"dvr.record": true, "dvr.record-series": true, "dvr.play": true,
		"dvr.cancel": true, "dvr.delete": true, "dvr.edit": true,
		"dvr.enable": true, "dvr.disable": true, "dvr.rule.create": true,
	}
	for _, action := range contract.MediaActions {
		if !wantActions[action.ID] {
			continue
		}
		delete(wantActions, action.ID)
		if action.Presentation.Group != "recording" && action.ID != "dvr.play" {
			t.Errorf("action %s group = %q", action.ID, action.Presentation.Group)
		}
		for _, surface := range action.Presentation.Surfaces {
			if surface == "web-admin" {
				t.Errorf("consumer DVR action %s leaked onto web-admin", action.ID)
			}
		}
	}
	if len(wantActions) != 0 {
		t.Fatalf("missing consumer DVR descriptors: %#v", wantActions)
	}
}

func TestSharedLiveTVDVRAndVersionProductLanguageExistsAtSource(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "api", "product-language", "en-US.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Messages map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"action.play-version", "playback.version-title", "playback.version-description",
		"live-tv.source-label", "live-tv.choose-source", "live-tv.tab.guide", "live-tv.tab.channels", "live-tv.tab.dvr", "live-tv.tab.library-channels",
		"dvr.recording-scheduled", "dvr.series-recording-scheduled", "dvr.rule-updated", "dvr.rule-deleted", "dvr.recording-cancelled", "dvr.recording-deleted",
		"action.delete-recording", "action.edit-recording", "action.enable-recording-rule", "action.disable-recording-rule",
	} {
		if len(catalog.Messages[id]) == 0 {
			t.Errorf("Product Language source is missing %s", id)
		}
	}
}
