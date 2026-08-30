package app

import (
	"strings"
	"testing"
)

func TestPlaybackCapabilityDiagnosticIsBoundedNormalizedAndSecretFree(t *testing.T) {
	profile := PlaybackClientProfile{
		ClientFamily: " Chromium ", Platform: "WEB", ClientVersion: "140.0.1", CapabilitySchemaVersion: "v2",
		Device: "must-not-log-device", CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID: "must-not-log-evidence-id", Producer: "must-not-log-producer", Tuples: []PlaybackCapabilityTuple{{
				Protocol: " HLS ", Container: "MPEGTS", Video: PlaybackCapabilityVideo{Codec: "H264"},
				Audio: PlaybackCapabilityAudio{Codec: "AAC", Layout: "mono", MaxChannels: 1},
			}},
		}, {
			Tuples: make([]PlaybackCapabilityTuple, 70),
		}},
	}
	diagnostic := playbackCapabilityDiagnosticFor(profile)
	if diagnostic.ClientFamily != "chromium" || diagnostic.Platform != "web" || diagnostic.TupleCount != 71 || len(diagnostic.Tuples) != 48 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Tuples[0] != "kind=none,protocol=hls,container=mpegts,video=h264:none:none:none:none:0:0x0:0,audio=aac:none:mono:1:none,subtitle=none:none:none" {
		t.Fatalf("tuple diagnostic = %q", diagnostic.Tuples[0])
	}
	encoded := strings.Join(diagnostic.Tuples, " ") + diagnostic.ClientFamily + diagnostic.Platform + diagnostic.SchemaVersion + diagnostic.ClientVersion
	for _, secret := range []string{profile.Device, profile.CapabilityEvidence[0].ID, profile.CapabilityEvidence[0].Producer} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("diagnostic leaked %q", secret)
		}
	}
	if got := playbackDiagnosticToken("attacker\nvalue"); got != "invalid" {
		t.Fatalf("unsafe diagnostic token = %q", got)
	}
}
