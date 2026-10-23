package livechannels

import (
	"reflect"
	"testing"
)

func TestSavedSourceSelectionUsesOnlyItsChosenMode(t *testing.T) {
	for _, tc := range []struct {
		mode             string
		groups, channels []string
		want             []string
	}{
		{"all", []string{"News"}, []string{"two"}, []string{"one", "two", "three"}},
		{"groups", []string{"News"}, []string{"two"}, []string{"one", "three"}},
		{"groups", []string{}, []string{"two"}, []string{}},
		{"picked", []string{"News"}, []string{"two"}, []string{"two"}},
		{"picked", []string{"News"}, []string{}, []string{}},
		{"filter", []string{"Sports"}, []string{"two"}, []string{"one"}},
	} {
		t.Run(tc.mode+"/"+stringListKey(tc.groups)+"/"+stringListKey(tc.channels), func(t *testing.T) {
			settings := DefaultLiveSourceSettings()
			settings.ChannelSelection = tc.mode
			settings.EnabledGroups, settings.EnabledChannels = tc.groups, tc.channels
			settings.Filters.Categories.Values = []string{"News"}
			settings.Filters.Countries.Values = []string{"CA"}
			settings.Filters.Keywords.Values = []string{"Shopping"}
			in := testInput()
			in.configuration = &settings
			in.Playlist = "#EXTM3U\n#EXTINF:-1 tvg-id=\"one\" group-title=\"News\" tvg-country=\"CA\",News\nhttps://fixture.invalid/one\n#EXTINF:-1 tvg-id=\"two\" group-title=\"Sports\" tvg-country=\"US\",Sports\nhttps://fixture.invalid/two\n#EXTINF:-1 tvg-id=\"three\" group-title=\"News\" tvg-country=\"CA\",Shopping\nhttps://fixture.invalid/three\n"
			data, err := parse(in)
			if err != nil {
				t.Fatal(err)
			}
			defer data.close()
			got := []string{}
			for _, channel := range data.channels {
				got = append(got, channel.key)
			}
			if !reflect.DeepEqual(got, tc.want) || len(data.choices) != 3 {
				t.Fatalf("chosen=%v want=%v available=%d", got, tc.want, len(data.choices))
			}
			for _, programme := range data.programmes {
				found := false
				for _, key := range got {
					found = found || key == programme.channel
				}
				if !found {
					t.Fatalf("programme from an excluded channel: %+v", programme)
				}
			}
		})
	}
}

func stringListKey(values []string) string {
	if len(values) == 0 {
		return "empty"
	}
	return values[0]
}

func TestSourceLogoChoiceResolvesInheritance(t *testing.T) {
	for _, tc := range []struct {
		mode         string
		server, want bool
	}{
		{"follow", false, false}, {"follow", true, true},
		{"import", false, true}, {"import", true, true},
		{"replace", false, true}, {"replace", true, true},
		{"none", false, false}, {"none", true, false},
		{"unknown", true, false},
	} {
		if got := (LogoImport{Mode: tc.mode}).Enabled(tc.server); got != tc.want {
			t.Fatalf("%s with server=%t: %t", tc.mode, tc.server, got)
		}
	}
}
