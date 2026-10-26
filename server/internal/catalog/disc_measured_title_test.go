package catalog

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/disc"
)

// Metadata projection only: these supplied protocol values do not qualify an
// input, a decoder, a real disc, a private receipt or a playable presentation.
func measuredTitleStructure() disc.Probe {
	return disc.Probe{Schema: disc.Schema, Kind: "dvd", Packaging: "iso", Encryption: "none", Titles: []disc.Title{
		{DiscOrder: 0, Title: 1, VTS: 1, PGC: 1, DurationMs: 120000, AngleCount: 1, Clips: []disc.Clip{}, Streams: []disc.Stream{}, Chapters: []disc.Chapter{{StartMs: 0}, {StartMs: 60000}}},
		{DiscOrder: 1, Title: 2, VTS: 1, PGC: 2, DurationMs: 90000, AngleCount: 1, Clips: []disc.Clip{}, Streams: []disc.Stream{}, Chapters: []disc.Chapter{{StartMs: 0}}},
	}}
}

func measuredTitleRef(t *testing.T, asset, object, observation, selected, facts string, p disc.Probe) DiscTitleRef {
	t.Helper()
	ref, err := MeasuredDiscTitleRef(asset, object, observation, p, selected, facts)
	if err != nil || ref == nil {
		t.Fatal(ref, err)
	}
	return *ref
}

func TestMeasuredDiscTitleMetadataIsStableAndFactsVersioned(t *testing.T) {
	p := measuredTitleStructure()
	before, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	first := measuredTitleRef(t, "asset", "object", "observation", "dvd:2", strings.Repeat("a", 64), p)
	second := measuredTitleRef(t, "asset", "object", "observation", "dvd:2", strings.Repeat("a", 64), p)
	if first != second || len(first.ID) != 64 || len(first.Revision) != 64 {
		t.Fatal("same exact measured metadata changed its reference", first, second)
	}
	changedFacts := measuredTitleRef(t, "asset", "object", "observation", "dvd:2", strings.Repeat("b", 64), p)
	if changedFacts.ID != first.ID || changedFacts.Revision == first.Revision {
		t.Fatal("facts changed title identity or failed to change offer revision")
	}
	after, err := json.Marshal(p)
	if err != nil || string(before) != string(after) {
		t.Fatal("metadata projection mutated original measured structure", err)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(encoded, &fields); err != nil || len(fields) != 2 || fields["id"] != first.ID || fields["revision"] != first.Revision {
		t.Fatal("projection changed the existing public DiscTitleRef shape", err)
	}
}

func TestMeasuredDiscTitleMetadataDoesNotRetargetAcrossPhysicalStructureOrTitle(t *testing.T) {
	for _, change := range []string{"asset", "object", "observation", "title", "chapter", "duration"} {
		t.Run(change, func(t *testing.T) {
			p := measuredTitleStructure()
			asset, object, observation, selected := "asset", "object", "observation", "dvd:1"
			first := measuredTitleRef(t, asset, object, observation, selected, strings.Repeat("a", 64), p)
			switch change {
			case "asset":
				asset = "different-asset"
			case "object":
				object = "different-object"
			case "observation":
				observation = "different-observation"
			case "title":
				selected = "dvd:2"
			case "chapter":
				p.Titles[0].Chapters[1].StartMs++
			case "duration":
				p.Titles[0].DurationMs++
			}
			second := measuredTitleRef(t, asset, object, observation, selected, strings.Repeat("a", 64), p)
			if first.ID == second.ID || first.Revision == second.Revision {
				t.Fatal("different physical/title/structure metadata reused an old reference")
			}
		})
	}
}

func TestMeasuredDiscTitleMetadataRejectsUnboundOrInvalidStructures(t *testing.T) {
	for _, change := range []string{"asset", "object", "observation", "selected", "facts", "unknown-title", "duplicate-title", "encrypted", "malformed", "title-budget", "field-budget"} {
		t.Run(change, func(t *testing.T) {
			p := measuredTitleStructure()
			asset, object, observation, selected, facts := "asset", "object", "observation", "dvd:1", strings.Repeat("a", 64)
			switch change {
			case "asset":
				asset = ""
			case "object":
				object = "bad\x00object"
			case "observation":
				observation = strings.Repeat("x", 257)
			case "selected":
				selected = ""
			case "facts":
				facts = ""
			case "unknown-title":
				selected = "dvd:99"
			case "duplicate-title":
				p.Titles[1].Title = p.Titles[0].Title
			case "encrypted":
				p.Encryption, p.Titles = "css", []disc.Title{}
			case "malformed":
				p.Titles[0].Chapters[1].StartMs = p.Titles[0].DurationMs
			case "title-budget":
				p.Titles = make([]disc.Title, disc.MaxTitles+1)
			case "field-budget":
				p.Titles[0].PlaylistFile = strings.Repeat("x", disc.MaxOutputBytes+1)
			}
			ref, err := MeasuredDiscTitleRef(asset, object, observation, p, selected, facts)
			if err == nil || ref != nil {
				t.Fatal("invalid metadata projected a checked-looking reference", ref, err)
			}
			if change == "encrypted" && !errors.Is(err, disc.ErrEncrypted) {
				t.Fatal("encrypted structure lost its actual refusal", err)
			}
		})
	}
}

func TestMeasuredBlurayTitleUsesItsExactPlaylistKey(t *testing.T) {
	playlist := 0
	p := disc.Probe{Schema: disc.Schema, Kind: "bluray", Packaging: "iso", Encryption: "none", Titles: []disc.Title{{
		DiscOrder: 0, Playlist: &playlist, PlaylistFile: "00000.mpls", DurationMs: 120000, AngleCount: 1,
		Clips:   []disc.Clip{{Sequence: 0, File: "00001.m2ts", InTime90k: 0, OutTime90k: 10800000}},
		Streams: []disc.Stream{{ClipSequence: 0, PID: 256, Kind: "video", Coding: 0x1b}}, Chapters: []disc.Chapter{{StartMs: 0}},
	}}}
	first := measuredTitleRef(t, "asset", "object", "observation", "bd:00000", strings.Repeat("a", 64), p)
	raw, _ := json.Marshal(p)
	var copied disc.Probe
	if err := json.Unmarshal(raw, &copied); err != nil {
		t.Fatal(err)
	}
	second := measuredTitleRef(t, "asset", "object", "observation", "bd:00000", strings.Repeat("a", 64), copied)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("an independent copy of the same measured playlist changed metadata identity")
	}
	if ref, err := MeasuredDiscTitleRef("asset", "object", "observation", p, "bd:00001", strings.Repeat("a", 64)); err == nil || ref != nil {
		t.Fatal("projection substituted a different playlist")
	}
}
