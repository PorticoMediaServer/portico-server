package livechannels

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkGuideChannelMapping(b *testing.B) {
	var playlist, guide strings.Builder
	playlist.WriteString("#EXTM3U\n")
	guide.WriteString("<tv>")
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&playlist, "#EXTINF:-1 tvg-id=\"%d\" tvg-chno=\"%d\",Channel %d\nhttps://provider.invalid/%d\n", i, i, i, i)
		fmt.Fprintf(&guide, `<channel id="%d"><display-name>Channel %d</display-name></channel>`, i, i)
	}
	guide.WriteString("</tv>")
	in := SourceInput{Name: "Guide", Playlist: playlist.String(), Guide: guide.String()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, e := parse(in)
		if e != nil {
			b.Fatal(e)
		}
		p.close()
	}
}

func TestGuideChannelMappingPreservesAliasesAndExplicitChoice(t *testing.T) {
	in := SourceInput{Name: "Guide", Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"id\",One\nhttps://provider.invalid/1\n#EXTINF:-1 tvg-id=\"two\" tvg-chno=\"2\", TWO\nhttps://provider.invalid/2\n#EXTINF:-1 tvg-id=\"three\",Three\nhttps://provider.invalid/3\n", Mappings: []ChannelMapping{{ChannelKey: "three", GuideKey: "chosen"}}, Guide: `<tv><channel id="id"><display-name>Two</display-name></channel><channel id="2"/><channel id="bad"><display-name>Three</display-name></channel><programme channel="id" start="20261013120000" stop="20261013130000"><title>Aliases</title></programme><programme channel="bad" start="20261013120000" stop="20261013130000"><title>Unchosen</title></programme><programme channel="chosen" start="20261013120000" stop="20261013130000"><title>Chosen</title></programme></tv>`}
	p, e := parse(in)
	if e != nil {
		t.Fatal(e)
	}
	defer p.close()
	got := map[string]string{}
	for _, row := range p.programmes {
		got[row.channel] = row.title
	}
	if len(got) != 3 || got["id"] != "Aliases" || got["two"] != "Aliases" || got["three"] != "Chosen" {
		t.Fatal(got)
	}
}
