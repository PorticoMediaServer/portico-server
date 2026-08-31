package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackExecutionPlanBindsX264PresetOnlyWhenSoftwareH264EncodeConsumesIt(t *testing.T) {
	directStream := testPlaybackExecutionPlan(t, nil)
	encoded, err := json.Marshal(directStream)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "x264Preset") {
		t.Fatalf("remux plan bound an unused x264 preset: %s", encoded)
	}
	directStream.X264Preset = "veryfast"
	if err := directStream.seal(); err == nil {
		t.Fatal("remux plan accepted an unused x264 preset")
	}

	softwareEncode := testPlaybackExecutionPlan(t, func(plan *playbackExecutionPlan) {
		plan.Plan.Mode = playbackplan.VideoTranscode
		plan.Plan.Streams = []playbackplan.StreamAction{{Index: 0, Kind: "video", Action: playbackplan.Convert, InputCodec: "mpeg2video", OutputCodec: "h264"}}
	})
	if softwareEncode.X264Preset != "veryfast" {
		t.Fatalf("software H.264 plan omitted its consumed preset: %#v", softwareEncode)
	}
	softwareEncode.X264Preset = ""
	if err := softwareEncode.seal(); err == nil {
		t.Fatal("software H.264 plan accepted a missing x264 preset")
	}
}

func TestPlaybackExecutionPlanHLSParametersRejectEveryBehaviorMutation(t *testing.T) {
	plan := testPlaybackExecutionPlan(t, func(plan *playbackExecutionPlan) {
		plan.Quality = "720p-medium"
		plan.AudioStreamID = "audio_2"
		plan.SubtitleStreamID = "subtitle_4"
		plan.Plan.Mode = playbackplan.VideoTranscode
		plan.Plan.Streams = []playbackplan.StreamAction{
			{Index: 0, Kind: "video", Action: playbackplan.Convert, InputCodec: "mpeg2video", OutputCodec: "h264"},
			{Index: 1, Kind: "audio", Action: playbackplan.Convert, InputCodec: "mp2", OutputCodec: "aac", InputLayout: "mono", OutputLayout: "mono"},
		}
		plan.Plan.Audio = playbackplan.AudioDecision{Codec: "aac", Layout: "mono", Channels: 1}
		plan.Plan.Subtitle = playbackplan.SubtitleDecision{Index: intPointer(2), Action: playbackplan.BurnIn}
	})
	accepted := httptest.NewRequest("GET", "/api/media/m/hls/segment?quality=720p-medium&audio=transcode&audioStream=audio_2&subtitle=subtitle_4", nil)
	quality, subtitle, textSubtitle, audio, audioStream, direct, err := playbackPlanHLSParameters(plan, accepted)
	if err != nil || quality != "720p-medium" || subtitle != "subtitle_4" || textSubtitle != "" || audio != "transcode" || audioStream != "audio_2" || direct {
		t.Fatalf("stored plan was not projected exactly: quality=%q subtitle=%q text=%q audio=%q stream=%q direct=%v err=%v", quality, subtitle, textSubtitle, audio, audioStream, direct, err)
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
			if _, _, _, _, _, _, err := playbackPlanHLSParameters(plan, r); err == nil {
				t.Fatalf("behavior-changing query was accepted: %s", mutation)
			}
		})
	}
}

func TestPlaybackExecutionPlanHLSParametersAllowsResourceAddressOnly(t *testing.T) {
	plan := testPlaybackExecutionPlan(t, nil)
	r := httptest.NewRequest("GET", "/api/media/m/hls/segment?name=segment_00001.m4s", nil)
	quality, subtitle, textSubtitle, audio, audioStream, direct, err := playbackPlanHLSParameters(plan, r)
	if err != nil || quality != "original" || subtitle != "" || textSubtitle != "" || audio != "" || audioStream != "" || !direct {
		t.Fatalf("resource-only query did not preserve stored plan: quality=%q subtitle=%q text=%q audio=%q stream=%q direct=%v err=%v", quality, subtitle, textSubtitle, audio, audioStream, direct, err)
	}
}
