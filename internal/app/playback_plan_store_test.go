package app

import (
	"net/http/httptest"
	"testing"
)

func TestPlaybackBindingHLSParametersRejectsEveryBehaviorMutation(t *testing.T) {
	binding := playbackExecutionBinding{
		Mode:             "video_transcode",
		Protocol:         "hls",
		Quality:          "720p-medium",
		AudioMode:        "transcode",
		AudioStreamID:    "audio_2",
		SubtitleMode:     "burn_in",
		SubtitleStreamID: "subtitle_4",
		DirectStream:     false,
	}
	accepted := httptest.NewRequest("GET", "/api/media/m/hls/segment?quality=720p-medium&audio=transcode&audioStream=audio_2&subtitle=subtitle_4", nil)
	quality, subtitle, textSubtitle, audio, audioStream, direct, err := playbackBindingHLSParameters(binding, accepted)
	if err != nil || quality != "720p-medium" || subtitle != "subtitle_4" || textSubtitle != "" || audio != "transcode" || audioStream != "audio_2" || direct {
		t.Fatalf("stored binding was not projected exactly: quality=%q subtitle=%q text=%q audio=%q stream=%q direct=%v err=%v", quality, subtitle, textSubtitle, audio, audioStream, direct, err)
	}
	mutations := []string{
		"quality=1080p-high",
		"audio=copy",
		"audioStream=audio_9",
		"subtitle=subtitle_9",
		"textSubtitle=subtitle_4",
		"directStream=1",
		"directStream=1&directStream=1",
	}
	for _, mutation := range mutations {
		t.Run(mutation, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/media/m/hls/segment?"+mutation, nil)
			if _, _, _, _, _, _, err := playbackBindingHLSParameters(binding, r); err == nil {
				t.Fatalf("behavior-changing query was accepted: %s", mutation)
			}
		})
	}
}

func TestPlaybackBindingHLSParametersAllowsResourceAddressOnly(t *testing.T) {
	binding := playbackExecutionBinding{Mode: "remux", Protocol: "hls", Quality: "original", SubtitleMode: "off", DirectStream: true}
	r := httptest.NewRequest("GET", "/api/media/m/hls/segment?name=segment_00001.m4s", nil)
	quality, subtitle, textSubtitle, audio, audioStream, direct, err := playbackBindingHLSParameters(binding, r)
	if err != nil || quality != "original" || subtitle != "" || textSubtitle != "" || audio != "" || audioStream != "" || !direct {
		t.Fatalf("resource-only query did not preserve stored plan: quality=%q subtitle=%q text=%q audio=%q stream=%q direct=%v err=%v", quality, subtitle, textSubtitle, audio, audioStream, direct, err)
	}
}
