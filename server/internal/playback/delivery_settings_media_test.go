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
	if got.ConversionMode != "all" || got.OutputFormat != "auto" || got.DownloadMaxQuality != "original" {
		t.Fatal(got)
	}
}
