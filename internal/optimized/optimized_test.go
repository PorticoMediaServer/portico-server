package optimized

import (
	"strings"
	"testing"
)

func TestPresetRegistryGolden(t *testing.T) {
	want := []struct {
		id                                   string
		container                            Container
		codec                                VideoCodec
		width, height, quality               int
		sourceSize                           bool
		firstAudio                           string
		colorHDR10, colorHLG, colorHDR10Plus HDRAction
	}{
		{"universal-1080p", ContainerMP4, CodecH264, 1920, 1080, 20, false, "aac", HDRToneMapSDR, HDRToneMapSDR, HDRToneMapSDR},
		{"universal-720p", ContainerMP4, CodecH264, 1280, 720, 21, false, "aac", HDRToneMapSDR, HDRToneMapSDR, HDRToneMapSDR},
		{"universal-480p", ContainerMP4, CodecH264, 854, 480, 22, false, "aac", HDRToneMapSDR, HDRToneMapSDR, HDRToneMapSDR},
		{"efficient-4k", ContainerMKV, CodecHEVC, 3840, 2160, 20, false, "copy", HDRPreserve, HDRPreserve, HDRDowngradeHDR10},
		{"efficient-1080p", ContainerMKV, CodecHEVC, 1920, 1080, 21, false, "copy", HDRPreserve, HDRPreserve, HDRDowngradeHDR10},
		{"efficient-720p", ContainerMKV, CodecHEVC, 1280, 720, 22, false, "copy", HDRPreserve, HDRPreserve, HDRDowngradeHDR10},
		{"maximum-compression-source", ContainerMKV, CodecAV1, 0, 0, 28, true, "copy", HDRPreserve, HDRPreserve, HDRDowngradeHDR10},
		{"maximum-compression-1080p", ContainerMKV, CodecAV1, 1920, 1080, 30, false, "copy", HDRPreserve, HDRPreserve, HDRDowngradeHDR10},
	}
	got := List()
	if err := ValidateRegistry(got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d presets", len(got))
	}
	for i, w := range want {
		p := got[i]
		if p.ID != w.id || p.Order != i+1 || p.Version != 1 || p.Container != w.container || p.VideoCodec != w.codec || p.MaxWidth != w.width || p.MaxHeight != w.height || p.SourceSize != w.sourceSize || p.RouteQualities[0].Quality.Mode != QualityCRF || p.RouteQualities[0].Quality.Value != w.quality || p.Audio.Steps[0].Codec != w.firstAudio || p.Color.HDR10 != w.colorHDR10 || p.Color.HLG != w.colorHLG || p.Color.HDR10Plus != w.colorHDR10Plus {
			t.Errorf("preset %d differs from golden: %+v", i+1, p)
		}
		if !p.Artifact.ReprobeBeforePublish || !p.Artifact.AtomicDurablePublish || p.Artifact.RetainSupersededHours != 168 {
			t.Errorf("%s artifact policy is not durable", p.ID)
		}
	}
	// Returned definitions must not provide a mutation path into the registry.
	got[0].Audio.Steps[0].Codec = "mutated"
	got[0].Color.DolbyVisionProfiles["5"] = DVUseVerifiedBaseHDR10
	p, _ := Lookup("universal-1080p")
	if p.Audio.Steps[0].Codec != "aac" || p.Color.DolbyVisionProfiles["5"] != DVUnsupported {
		t.Fatal("registry was mutated through List")
	}
}

func TestPlanNeverUpscalesAndPreservesDisplayAspect(t *testing.T) {
	tests := []struct {
		name, id string
		source   SourceFacts
		w, h     int
	}{
		{"small source", "universal-1080p", SourceFacts{Width: 640, Height: 360}, 640, 360},
		{"4k ceiling", "universal-1080p", SourceFacts{Width: 3840, Height: 2160}, 1920, 1080},
		{"anamorphic dvd", "universal-1080p", SourceFacts{Width: 720, Height: 480, SARNumerator: 32, SARDenominator: 27}, 852, 480},
		{"rotated source", "universal-720p", SourceFacts{Width: 1920, Height: 1080, Rotation: 90}, 404, 720},
		{"source size anamorphic", "maximum-compression-source", SourceFacts{Width: 720, Height: 480, SARNumerator: 32, SARDenominator: 27}, 852, 480},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := Lookup(tt.id)
			plan, err := Plan(p, tt.source)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Geometry.Width != tt.w || plan.Geometry.Height != tt.h || plan.Geometry.SampleAspectRatio != "1:1" {
				t.Fatalf("geometry=%+v", plan.Geometry)
			}
		})
	}
}

func TestPlanAudioColorAndDolbyVisionLadders(t *testing.T) {
	universal, _ := Lookup("universal-1080p")
	efficient, _ := Lookup("efficient-4k")
	tests := []struct {
		name                   string
		p                      Preset
		s                      SourceFacts
		hdr                    HDRAction
		out                    DynamicRange
		codec                  string
		channels               int
		copy, objects, downmix bool
		dv                     DVAction
		wantErr                string
	}{
		{"universal hdr and surround", universal, SourceFacts{Width: 3840, Height: 2160, DynamicRange: RangeHDR10, AudioCodec: "truehd", AudioChannels: 8, AudioLayout: "7.1", AudioHasObjects: true}, HDRToneMapSDR, RangeSDR, "aac", 2, false, false, true, "", ""},
		{"universal mono becomes stereo", universal, SourceFacts{Width: 720, Height: 480, AudioCodec: "pcm", AudioChannels: 1, AudioLayout: "mono"}, HDRPreserve, RangeSDR, "aac", 2, false, false, false, "", ""},
		{"efficient hlg exact audio", efficient, SourceFacts{Width: 3840, Height: 2160, DynamicRange: RangeHLG, AudioCodec: "truehd", AudioChannels: 8, AudioLayout: "7.1", AudioHasObjects: true}, HDRPreserve, RangeHLG, "truehd", 8, true, true, false, "", ""},
		{"efficient hdr10 plus downgrade", efficient, SourceFacts{Width: 1920, Height: 1080, DynamicRange: RangeHDR10Plus, AudioCodec: "vorbis", AudioChannels: 2}, HDRDowngradeHDR10, RangeHDR10, "eac3", 2, false, false, false, "", ""},
		{"universal dv8 base", universal, SourceFacts{Width: 1920, Height: 1080, DynamicRange: RangeDolbyVision, DolbyVisionProfile: "8", VerifiedBaseRange: RangeHDR10}, HDRToneMapSDR, RangeSDR, "aac", 2, false, false, false, DVUseVerifiedBaseSDR, ""},
		{"efficient dv7 base", efficient, SourceFacts{Width: 3840, Height: 2160, DynamicRange: RangeDolbyVision, DolbyVisionProfile: "7", VerifiedBaseRange: RangeHDR10}, HDRDowngradeHDR10, RangeHDR10, "eac3", 2, false, false, false, DVUseVerifiedBaseHDR10, ""},
		{"dv5 explicit unsupported", efficient, SourceFacts{Width: 3840, Height: 2160, DynamicRange: RangeDolbyVision, DolbyVisionProfile: "5"}, "", "", "", 0, false, false, false, "", "profile \"5\" is unsupported"},
		{"dv fallback unverified", efficient, SourceFacts{Width: 3840, Height: 2160, DynamicRange: RangeDolbyVision, DolbyVisionProfile: "8"}, "", "", "", 0, false, false, false, "", "requires a verified base layer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Plan(tt.p, tt.s)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.HDRAction != tt.hdr || got.DynamicRange != tt.out || got.DolbyVisionAction != tt.dv || got.Audio.Codec != tt.codec || got.Audio.Channels != tt.channels || got.Audio.Copy != tt.copy || got.Audio.ObjectsPreserved != tt.objects || got.Audio.Downmixed != tt.downmix {
				t.Fatalf("plan=%+v", got)
			}
		})
	}
}

func TestPlanSelectsRouteSpecificQuality(t *testing.T) {
	p, _ := Lookup("efficient-1080p")
	source := SourceFacts{Width: 1920, Height: 1080}
	software, err := PlanForRoute(p, source, RouteSoftwareHEVC)
	if err != nil {
		t.Fatal(err)
	}
	hardware, err := PlanForRoute(p, source, RouteNVENC)
	if err != nil {
		t.Fatal(err)
	}
	if software.Quality.Mode != QualityCRF || software.Quality.Control != "crf" || software.EncoderRoute != RouteSoftwareHEVC {
		t.Fatalf("software plan=%+v", software)
	}
	if hardware.Quality.Mode != QualityCQ || hardware.Quality.Control != "cq" || hardware.EncoderRoute != RouteNVENC {
		t.Fatalf("hardware plan=%+v", hardware)
	}
	if _, err := PlanForRoute(p, source, RouteSoftwareAV1); err == nil {
		t.Fatal("ineligible route unexpectedly planned")
	}
}

func TestPlanRejectsNonDolbyUnsupportedColorPolicy(t *testing.T) {
	p, _ := Lookup("efficient-1080p")
	p.Color.HLG = HDRUnsupported
	_, err := Plan(p, SourceFacts{Width: 1920, Height: 1080, DynamicRange: RangeHLG})
	if err == nil || !strings.Contains(err.Error(), `dynamic range "hlg" is unsupported`) {
		t.Fatalf("error=%v", err)
	}
}

func TestRevisionFingerprintGolden(t *testing.T) {
	const want = "856cc813fb53248d7019feed1404d23641c14f5b8f0d6dc4596d8299db29b33c"
	got := RevisionFingerprint()
	if got != want {
		t.Fatalf("fingerprint changed: got %s", got)
	}
	p, _ := Lookup("efficient-1080p")
	plan, err := Plan(p, SourceFacts{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RevisionFingerprint != got {
		t.Fatal("plan did not bind registry/planner revision")
	}
}

func TestInvalidDefinitions(t *testing.T) {
	base, _ := Lookup("universal-1080p")
	tests := []struct {
		name   string
		mutate func(*Preset)
	}{
		{"missing id", func(p *Preset) { p.ID = "" }}, {"bad dimensions", func(p *Preset) { p.MaxWidth = 1919 }}, {"both source and ceiling", func(p *Preset) { p.SourceSize = true }},
		{"bad quality", func(p *Preset) { p.RouteQualities[0].Quality.Value = 101 }}, {"missing route quality", func(p *Preset) { p.RouteQualities = p.RouteQualities[1:] }}, {"no audio", func(p *Preset) { p.Audio.Steps = nil }}, {"missing dv", func(p *Preset) { delete(p.Color.DolbyVisionProfiles, "8") }},
		{"invalid hdr action", func(p *Preset) { p.Color.HDR10 = "invented" }}, {"unsafe artifact", func(p *Preset) { p.Artifact.AtomicDurablePublish = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := clonePreset(base)
			tt.mutate(&p)
			if err := ValidatePreset(p); err == nil {
				t.Fatal("definition unexpectedly valid")
			}
		})
	}
	if err := ValidateRegistry(List()[:7]); err == nil {
		t.Fatal("seven-preset registry unexpectedly valid")
	}
	presets := List()
	presets[0].ID = "renamed"
	if err := ValidateRegistry(presets); err == nil {
		t.Fatal("renamed stable preset unexpectedly valid")
	}
}
