package playback

import (
	"portico.local/server/internal/assets"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/subtitles"
	"testing"
)

func TestApprovedAudioLanguageAndTrackPreferences(t *testing.T) {
	tracks := []SourceAudio{{Index: 1, Language: "ja", Codec: "aac", Channels: 2, Default: true}, {Index: 2, Language: "eng", Codec: "truehd", Channels: 8}, {Index: 3, Language: "eng", Codec: "aac", Channels: 2}, {Index: 4, Language: "eng", Codec: "aac", Channels: 2, AudioDescription: true}}
	values := operations.PreferenceValues{}
	langs := resolvePreferenceLanguages(values, "playback.preferredAudioLanguages", "en-CA")
	if len(langs) != 2 || langs[0] != "en-CA" || langs[1] != "@original" {
		t.Fatal(langs)
	}
	for _, tc := range []struct {
		policy      string
		description bool
		want        int
	}{{"best", false, 3}, {"channels", false, 2}, {"stereo", false, 3}, {"best", true, 4}} {
		got, _ := chooseAudioWithPolicy(tracks, -1, "", langs, tc.policy, tc.description, chromeProfile())
		if got == nil || got.Index != tc.want {
			t.Fatal(tc, got)
		}
	}
	got, _ := chooseAudioWithPolicy(tracks, 2, "", langs, "best", true, chromeProfile())
	if got == nil || got.Index != 2 {
		t.Fatal("explicit session choice overridden", got)
	}
	got, _ = chooseAudioWithPolicy(tracks, -1, "", []string{"@original", "en"}, "best", false, chromeProfile())
	if got == nil || got.Index != 1 {
		t.Fatal("original marker lost its ordered priority", got)
	}
	got, _ = chooseAudioWithPolicy(tracks, -1, "", nil, "best", false, chromeProfile())
	if got == nil || got.Index != 1 {
		t.Fatal("empty audio languages lost file default", got)
	}
}

func TestForeignAndForcedSubtitleModesAreDistinct(t *testing.T) {
	values := operations.PreferenceValues{}
	trace := &DecisionTrace{Audio: &TraceAudio{Language: "jpn"}}
	foreign := automaticSubtitles(values, trace, "en-US")
	if !foreign.ForeignAudio || len(foreign.Languages) != 1 || foreign.Languages[0] != "en-US" {
		t.Fatal(foreign)
	}
	tracks := []subtitles.AutoCandidate{{ID: "full", Language: "eng"}, {ID: "signs", Language: "jpn", Forced: true}, {ID: "sdh", Language: "eng", HearingImpaired: true}}
	if got := subtitles.ChooseAutomatic(foreign, tracks); got == nil || got.ID != "full" {
		t.Fatal(got)
	}
	values["playback.subtitleMode"] = "forced"
	forced := automaticSubtitles(values, trace, "en-US")
	if forced.ForeignAudio {
		t.Fatal(forced)
	}
	if got := subtitles.ChooseAutomatic(forced, tracks); got == nil || got.ID != "signs" {
		t.Fatal(got)
	}
	values["playback.subtitleMode"] = "always"
	values["playback.preferSdhSubtitles"] = true
	if got := subtitles.ChooseAutomatic(automaticSubtitles(values, trace, "en-US"), tracks); got == nil || got.ID != "sdh" {
		t.Fatal(got)
	}
	values["playback.preferredSubtitleLanguages"] = []string{}
	if got := subtitles.ChooseAutomatic(automaticSubtitles(values, trace, "en-US"), tracks); got != nil {
		t.Fatal("explicit no preference chose a subtitle", got)
	}
	values["playback.preferredSubtitleLanguages"] = []string{"en"}
	values["playback.subtitleMode"] = "forced"
	values["playback.forcedSubtitleFallback"] = "full"
	if got := subtitles.ChooseAutomatic(automaticSubtitles(values, trace, "en-US"), tracks[:1]); got == nil || got.ID != "full" {
		t.Fatal(got)
	}
}

func TestAudioSelectionPolicyReachesActualPlan(t *testing.T) {
	source := videoSource("mp4", "h264", assets.StreamDetail{Width: 1920, Height: 1080, BitDepth: 8, DynamicRange: "sdr"}, SourceAudio{Codec: "truehd", Channels: 8, Language: "en", Default: true}, SourceAudio{Codec: "aac", Channels: 2, Language: "en"})
	plan := decide(t, source, chromeProfile(), func(in *DeliveryInput) { in.PreferredAudioLanguages = []string{"en"}; in.AudioTrackChoice = "best" })
	if plan.AudioStream != 2 || plan.SourceAudioCodec != "aac" || plan.AudioAction == "transcode" {
		t.Fatal(plan.AudioStream, plan.SourceAudioCodec, plan.AudioAction)
	}
}
