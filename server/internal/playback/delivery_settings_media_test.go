package playback

import "testing"

func TestStreamingMediaConfigurationContract(t *testing.T) {
	defaults := DefaultDeliveryConfiguration()
	if defaults.ConversionMode != "all" || defaults.OutputFormat != "auto" || defaults.DownloadMaxQuality != "original" {
		t.Fatal(defaults)
	}
	for _, mode := range []string{"all", "sound", "never"} {
		c := defaults
		c.ConversionMode = mode
		c.OutputFormat = "h264"
		c.DownloadMaxQuality = "720p-high"
		got := c.Normalized()
		if got.ConversionMode != mode || got.OutputFormat != "h264" || got.DownloadMaxQuality != "720p-high" {
			t.Fatal(got)
		}
	}
	bad := defaults
	bad.ConversionMode = "copy"
	bad.OutputFormat = "av1"
	bad.DownloadMaxQuality = "automatic"
	got := bad.Normalized()
	if got.ConversionMode != "" || got.OutputFormat != "auto" || got.DownloadMaxQuality != "original" {
		t.Fatal(got)
	}
}

func TestAbsentConversionModeRetainsActualOwnerSwitch(t *testing.T) {
	for _, mode := range []string{"", "unknown"} {
		for _, enabled := range []bool{false, true} {
			cfg := DefaultDeliveryConfiguration()
			cfg.ConversionMode = mode
			if got := cfg.Normalized().ConversionMode; got != "" {
				t.Fatalf("unpublished mode became a policy write: %q", got)
			}
			source := videoSource("mkv", "h264", avcDetail, SourceAudio{Index: 1, Codec: "dts", Channels: 6})
			in := DeliveryInput{Source: source, Client: chromeProfile(), FMP4Output: true, Policy: openPolicy(), Target: QualityTarget{RungID: "original", Kind: QualityAutomatic, AllowHDR: true}, Config: cfg, TranscodingEnabled: enabled}
			plan, err := planDelivery(in)
			if !enabled && err == nil {
				t.Fatal("absent mode bypassed disabled owner conversion", plan)
			}
			if enabled && (err != nil || plan.Strategy != DeliveryAudioConversion) {
				t.Fatal("absent mode lost the owner's enabled policy", plan, err)
			}
		}
	}
}

func TestStreamingConversionModeConstrainsRealPlanner(t *testing.T) {
	for _, mode := range []string{"all", "sound", "never"} {
		t.Run(mode, func(t *testing.T) {
			cfg := DefaultDeliveryConfiguration()
			cfg.ConversionMode = mode
			source := videoSource("mkv", "h264", avcDetail, SourceAudio{Index: 1, Codec: "dts", Channels: 6})
			in := DeliveryInput{Source: source, Client: chromeProfile(), FMP4Output: true, Policy: openPolicy(), Target: QualityTarget{RungID: "original", Kind: QualityAutomatic, AllowHDR: true}, Config: cfg, TranscodingEnabled: true}
			plan, e := planDelivery(in)
			if mode == "never" {
				if e == nil {
					t.Fatal("never admitted conversion", plan)
				}
				return
			}
			if e != nil || plan.Strategy != DeliveryAudioConversion {
				t.Fatal(plan, e)
			}
			source.Video.Codec = "vc1"
			plan, e = planDelivery(in)
			if mode == "sound" && e == nil {
				t.Fatal("sound mode converted picture", plan)
			}
			if mode == "all" && (e != nil || plan.VideoAction != "convert") {
				t.Fatal(plan, e)
			}
		})
	}
}
