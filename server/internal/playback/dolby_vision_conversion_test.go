package playback

import (
	"portico.local/server/internal/assets"
	"portico.local/server/internal/decoder"
	"strings"
	"testing"
)

func TestDolbyVisionWithoutABaseRequiresQualifiedReshaping(t *testing.T) {
	detail := hdr10Detail
	detail.DynamicRange = assets.RangeDolbyVision
	detail.DolbyVisionProfile = 5
	detail.CodecTag = "dvh1"
	source := videoSource("mp4", "hevc", detail, SourceAudio{Index: 1, Codec: "eac3", Channels: 6})
	client := appleTVProfile()
	client.Video[1].DolbyVisionProfiles = nil
	facts := plannerToolchain()
	facts.Filters["libplacebo"] = true
	input := DeliveryInput{Source: source, Client: client, Server: facts, FMP4Output: true, Policy: openPolicy(), Target: QualityTarget{RungID: "original", Kind: QualityAutomatic, AllowHDR: true}, Config: DefaultDeliveryConfiguration(), TranscodingEnabled: true}
	if _, err := planDelivery(input); err == nil {
		t.Fatal("unqualified Profile 5 conversion was admitted")
	}
	facts.VulkanAvailable = true
	plan, err := planDelivery(input)
	if err != nil || !plan.ToneMap {
		t.Fatal(plan, err)
	}
	graph, err := plan.conversionGraph(nil)
	if err != nil || !strings.Contains(strings.Join(graph.Video, " "), "libplacebo") {
		t.Fatal(graph, err)
	}
	if graph.Backend != decoder.BackendSoftware || !conversionJobHardware(&plan, graph.Stages) {
		t.Fatal("qualified GPU filter lost its existing device grants", graph)
	}
	input.Client = appleTVProfile()
	facts.VulkanAvailable = false
	plan, err = planDelivery(input)
	if err != nil || plan.Strategy != DeliveryOriginal {
		t.Fatal(plan, err)
	}
}
