package localmetadata

import (
	"strings"
	"testing"
)

func TestVideoNFOTypedRootsAndEdition(t *testing.T) {
	for _, tc := range []struct {
		raw, kind, provider, typ string
		anime                    bool
	}{
		{`<movie><title>Film</title><year>2020</year><edition>Extended</edition><uniqueid type="tmdb">42</uniqueid></movie>`, "movie", "tmdb", "movie", false},
		{`<tvshow><title>Show</title><uniqueid type="tvdb">7</uniqueid></tvshow>`, "show", "tvdb", "show", false},
		{`<episodedetails><title>Episode</title><season>2</season><episode>1</episode><uniqueid type="tmdb">9</uniqueid></episodedetails>`, "episode", "tmdb", "episode", false},
		{`<tvshow><title>Anime</title><uniqueid type="anilist">1</uniqueid></tvshow>`, "show", "anilist", "anime", true},
	} {
		v, e := ParseVideoNFO([]byte(tc.raw), tc.anime)
		if e != nil || len(v) != 1 || v[0].Kind != tc.kind || len(v[0].IDs) != 1 || v[0].IDs[0].Type != tc.typ || v[0].IDs[0].Provider != tc.provider {
			t.Fatal(v, e)
		}
	}
}
func TestVideoNFONeverResolvesExternalEntitiesAndRejectsConflictingIDs(t *testing.T) {
	for _, raw := range []string{`<!DOCTYPE movie [<!ENTITY x SYSTEM "file:///etc/passwd">]><movie><plot>&x;</plot></movie>`, `<movie><uniqueid type="tmdb">1</uniqueid><uniqueid type="tmdb">2</uniqueid></movie>`, `<movie><uniqueid type="tmdb">https://example.org/42</uniqueid></movie>`, `<episodedetails><uniqueid type="anilist">1</uniqueid></episodedetails>`, `<movie><year>-1</year></movie>`, strings.Repeat("x", 256*1024+1)} {
		if _, e := ParseVideoNFO([]byte(raw), true); e == nil {
			t.Fatal("accepted invalid NFO")
		}
	}
}
func TestVideoNFOMultiEpisodeCoordinatesRemainDistinct(t *testing.T) {
	v, e := ParseVideoNFO([]byte(`<episodedetails><title>One</title><season>1</season><episode>1</episode></episodedetails><episodedetails><title>Two</title><season>2</season><episode>1</episode></episodedetails>`), false)
	if e != nil || len(v) != 2 || *v[0].Season == *v[1].Season || v[0].Digest != v[1].Digest {
		t.Fatal(v, e)
	}
}
