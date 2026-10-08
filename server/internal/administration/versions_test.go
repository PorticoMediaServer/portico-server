package administration

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSemanticVersionOrdering(t *testing.T) {
	ordered := []string{"1.2.0-1", "1.2.0-alpha", "1.2.0-alpha.1", "1.2.0-beta.1", "1.2.0-beta.2", "1.2.0-beta.10", "1.2.0-rc.1", "1.2.0", "1.2.1", "2.0.0", "99999999999999999999999999999.0.0"}
	for i, a := range ordered {
		for j, b := range ordered {
			cmp := compareVersions(a, b)
			if i < j && cmp >= 0 || i > j && cmp <= 0 || i == j && cmp != 0 {
				t.Fatalf("%s vs %s: %d", a, b, cmp)
			}
		}
	}
	if compareVersions("1.2.0+build.2", "1.2.0+build.1") != 0 {
		t.Fatal("metadata changed precedence")
	}
	for _, v := range []string{"", "1", "v1.2.0", "1.02.0", "1.2.0-", "1.2.0+", "1.2.0-beta.01", "1.2.0-a..b", "1.2.0+bad+again"} {
		if _, ok := parseVersion(v); ok {
			t.Fatalf("accepted invalid version %q", v)
		}
	}
}
func TestUpdatesSelectsNewestValidPrerelease(t *testing.T) {
	s, _, _ := newService(t)
	s.Fetch = func(context.Context, string) ([]byte, error) {
		return json.Marshal(updateFeed{Channels: map[string][]UpdateBuild{"beta": {{Version: "1.2.0-beta.2"}, {Version: "garbage"}, {Version: "1.2.0-beta.10"}, {Version: "1.2.0-beta.1"}}}})
	}
	got := s.Updates(context.Background(), UpdateBuild{Version: "1.2.0-beta.1"}, "beta", "https://example.invalid")
	if !got.UpdateAvailable || got.Latest.Version != "1.2.0-beta.10" {
		t.Fatalf("%+v", got)
	}
}
