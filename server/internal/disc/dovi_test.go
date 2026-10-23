package disc

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDolbyVisionDependencyIsAnObservedSameClipVideoPID(t *testing.T) {
	_, fixtures := discFixtures(t)
	var source Probe
	if err := json.Unmarshal(fixtures[0].Expected, &source); err != nil {
		t.Fatal(err)
	}
	basePID := 4113
	source.Titles[0].Streams = []Stream{
		{ClipSequence: 0, PID: basePID, Kind: "video", Coding: 0x24},
		{ClipSequence: 0, PID: 4117, Kind: "video", Coding: 0x24,
			DolbyVisionEnhancementKnown: true, DolbyVisionEnhancementLayer: true, DolbyVisionBasePID: &basePID},
	}
	check := func(t *testing.T, p Probe, want bool) {
		t.Helper()
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeProbe(raw)
		if want {
			if err != nil || got.Titles[0].Streams[1].DolbyVisionBasePID == nil || *got.Titles[0].Streams[1].DolbyVisionBasePID != basePID {
				t.Fatalf("lost observed relationship: %v", err)
			}
		} else if !errors.Is(err, ErrProtocol) {
			t.Fatalf("accepted unsupported relationship: %v", err)
		}
	}
	check(t, source, true)
	for _, test := range []struct {
		name   string
		mutate func(*Probe)
	}{
		{"unknown", func(p *Probe) { p.Titles[0].Streams[1].DolbyVisionEnhancementKnown = false }},
		{"not-enhancement", func(p *Probe) { p.Titles[0].Streams[1].DolbyVisionEnhancementLayer = false }},
		{"wrong-codec", func(p *Probe) { p.Titles[0].Streams[1].Coding = 0x1b }},
		{"self", func(p *Probe) { p.Titles[0].Streams[1].PID = basePID }},
		{"missing-base", func(p *Probe) { p.Titles[0].Streams[0].PID = 4100 }},
		{"other-clip", func(p *Probe) { p.Titles[0].Streams[0].ClipSequence = 1 }},
		{"base-is-enhancement", func(p *Probe) {
			p.Titles[0].Streams[0].DolbyVisionEnhancementKnown = true
			p.Titles[0].Streams[0].DolbyVisionEnhancementLayer = true
		}},
		{"base-is-audio", func(p *Probe) { p.Titles[0].Streams[0].Kind = "audio" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := source
			copy.Titles = append([]Title(nil), source.Titles...)
			copy.Titles[0].Streams = append([]Stream(nil), source.Titles[0].Streams...)
			test.mutate(&copy)
			check(t, copy, false)
		})
	}
	// STN's explicit EL membership is genuine even if a bounded PMT read did
	// not establish a base PID. It must stay unknown rather than guessed.
	source.Titles[0].Streams[1].DolbyVisionBasePID = nil
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeProbe(raw)
	if err != nil || got.Titles[0].Streams[1].DolbyVisionBasePID != nil {
		t.Fatalf("invented base PID: %v", err)
	}
}
