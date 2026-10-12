package playback

import (
	"strings"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
)

// Converted audio never leaves the server above 48 kHz, or above what the
// device declares for the output codec; a source already within that keeps
// its own rate.
func TestConvertedAudioSampleRateIsBounded(t *testing.T) {
	picture := assets.StreamDetail{Width: 1920, Height: 1080, BitDepth: 8, DynamicRange: "sdr"}
	audio := func(codec string, channels, rate int) SourceAudio {
		return SourceAudio{Codec: codec, Channels: channels, StreamDetail: assets.StreamDetail{SampleRate: rate}}
	}
	limited := chromeProfile()
	for i := range limited.Audio {
		if limited.Audio[i].Codec == "aac" {
			limited.Audio[i].MaxSampleRate = 44100
		}
	}
	capped := chromeProfile()
	for i := range capped.Audio {
		if capped.Audio[i].Codec == "aac" {
			capped.Audio[i].MaxSampleRate = 48000
		}
	}
	for _, test := range []struct {
		name   string
		source SourceAudio
		client ClientProfile
		rate   int
	}{
		{"96 kHz lossless", audio("truehd", 6, 96000), chromeProfile(), 48000},
		{"88.2 kHz lossless", audio("dts", 6, 88200), chromeProfile(), 48000},
		{"192 kHz lossless", audio("truehd", 2, 192000), chromeProfile(), 48000},
		{"48 kHz", audio("dts", 6, 48000), chromeProfile(), 0},
		{"44.1 kHz", audio("dts", 2, 44100), chromeProfile(), 0},
		{"unknown rate", audio("dts", 2, 0), chromeProfile(), 0},
		{"96 kHz in a codec the device plays to 48 kHz", audio("aac", 2, 96000), capped, 48000},
		{"48 kHz for a device that takes 44.1 kHz", audio("dts", 2, 48000), limited, 44100},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := decide(t, videoSource("mkv", "h264", picture, test.source), test.client, nil)
			if p.AudioAction != "convert" {
				t.Fatalf("audio was not converted: %+v", p)
			}
			if p.AudioSampleRate != test.rate {
				t.Fatalf("planned %d Hz, want %d", p.AudioSampleRate, test.rate)
			}
			_, args, _, err := p.CodecArgs()
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if test.rate == 0 && strings.Contains(joined, "-ar ") {
				t.Fatalf("a rate within bounds was resampled: %s", joined)
			}
			if test.rate != 0 && !strings.HasSuffix(joined, " -ar "+itoa(test.rate)) {
				t.Fatalf("arguments do not set the output rate: %s", joined)
			}
		})
	}
	t.Run("alternate renditions", func(t *testing.T) {
		p := decide(t, videoSource("mkv", "h264", picture, audio("truehd", 6, 96000), audio("dts", 2, 44100)), chromeProfile(), nil)
		if len(p.Renditions) != 2 || p.Renditions[0].Action != "convert" || p.Renditions[0].SampleRate != 48000 || p.Renditions[1].SampleRate != 0 {
			t.Fatalf("renditions %+v", p.Renditions)
		}
	})
}

// The planned rate is part of the stored plan: every later production window
// of the session converts at it.
func TestConvertedAudioSampleRateIsStoredWithThePlan(t *testing.T) {
	db, s, p, _, item, _ := activityFixture(t)
	s.ConfigureHLS(&HLS{})
	asset := firstCatalogAssetToken(t, db)
	updateCatalogAsset(t, db, asset, func(a *compactcatalog.Asset) { a.Container, a.AudioCodec = "mkv", "truehd" })
	mustAudioExec(t, db, `INSERT INTO asset_stream_facts(asset_id,revision,size,modified_ns,fingerprint,detail_version) VALUES(?,1,1,1,'test',?)`, asset, assets.StreamDetailVersion)
	mustAudioExec(t, db, `INSERT INTO asset_streams(asset_id,stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,detail_json) VALUES(?,0,'video','h264','','',0,'',1,0,'{"width":640,"height":360,"bitDepth":8}'),(?,1,'audio','truehd','eng','',6,'5.1',1,0,'{"sampleRate":96000}')`, asset, asset)
	session, err := s.Create(p, item, "auto", "high-rate")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := loadDeliveryPlan(db, session.ID)
	if err != nil || stored == nil || stored.AudioAction != "convert" || stored.AudioSampleRate != 48000 || stored.Policy == nil {
		t.Fatalf("stored plan %+v %v", stored, err)
	}
	_, args, _, err := stored.CodecArgs()
	if err != nil || !strings.HasSuffix(strings.Join(args, " "), " -ar 48000") {
		t.Fatalf("stored plan arguments %v %v", args, err)
	}
}
