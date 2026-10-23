package assets

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInterlaceFieldOrderRequiresActualObservation(t *testing.T) {
	for _, fieldOrder := range []string{"", "unknown", "und", "0", "height=1080", "progressive", "tt", "bb", "tb", "bt"} {
		t.Run(fieldOrder, func(t *testing.T) {
			d := streamDetail("video", "h264", 1920, 1080, probeStreamDetail{FieldOrder: fieldOrder})
			value := d.InterlacedFact()
			switch fieldOrder {
			case "progressive":
				if value == nil || *value || d.Interlaced {
					t.Fatalf("explicit progressive lost: %+v", d)
				}
			case "tt", "bb", "tb", "bt":
				if value == nil || !*value || !d.Interlaced {
					t.Fatalf("explicit interlace lost: %+v", d)
				}
			default:
				if value != nil || d.InterlaceState != "" || d.Interlaced {
					t.Fatalf("unknown became an observation: %+v", d)
				}
			}
		})
	}
}

func TestDecodedInterlaceFlagsPreservePresence(t *testing.T) {
	for _, tc := range []struct {
		name, flags, initial, state, analysis string
	}{
		{"omitted", `{}, {}`, "", "", ""},
		{"null", `{"interlaced_frame":null},{"interlaced_frame":null}`, "", "", ""},
		{"partly_observed", `{"interlaced_frame":0},{}`, "", "", ""},
		{"invalid_flag", `{"interlaced_frame":2},{"interlaced_frame":-1}`, "", "", ""},
		{"progressive", `{"interlaced_frame":0},{"interlaced_frame":0}`, "", "progressive", "decoded_frames"},
		{"interlaced_with_missing", `{}, {"interlaced_frame":1}`, "", "interlaced", "decoded_frames"},
		{"mixed", `{"interlaced_frame":0},{"interlaced_frame":1}`, "", "interlaced", "decoded_frames"},
		{"override_progressive_header", `{"interlaced_frame":0},{"interlaced_frame":1}`, "progressive", "interlaced", "decoded_frames"},
		{"preserve_interlaced_header", `{"interlaced_frame":0},{"interlaced_frame":0}`, "interlaced", "interlaced", ""},
		{"preserve_known_progressive", `{}, {}`, "progressive", "progressive", ""},
		{"short_window", `{"interlaced_frame":0}`, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := StreamDetail{StreamBreadth: StreamBreadth{InterlaceState: tc.initial}, Interlaced: tc.initial == "interlaced"}
			analyzeFrames([]byte(`{"frames":[`+tc.flags+`]}`), &d)
			if d.InterlaceState != tc.state || d.InterlaceAnalysis != tc.analysis || d.Interlaced != (tc.state == "interlaced") {
				t.Fatalf("got state=%q analysis=%q legacy=%v; want state=%q analysis=%q", d.InterlaceState, d.InterlaceAnalysis, d.Interlaced, tc.state, tc.analysis)
			}
		})
	}
}

func TestFrameCadenceDoesNotInventInterlace(t *testing.T) {
	frames := make([]string, 10)
	for i := range frames {
		frames[i] = fmt.Sprintf(`{"best_effort_timestamp_time":"%.3f"}`, float64(i)/25)
	}
	var d StreamDetail
	analyzeFrames([]byte(`{"frames":[`+strings.Join(frames, ",")+`]}`), &d)
	if d.FrameRateMode != "constant" || d.InterlacedFact() != nil || d.InterlaceAnalysis != "" {
		t.Fatalf("cadence and interlace presence were conflated: %+v", d)
	}
}

func TestInterlaceWireValueOmitsUnknownAndKeepsFalse(t *testing.T) {
	for _, state := range []string{"", "invalid", "progressive", "interlaced"} {
		wire := struct {
			Interlaced *bool `json:"interlaced,omitempty"`
		}{(StreamBreadth{InterlaceState: state}).InterlacedFact()}
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		want := `{}`
		if state == "progressive" {
			want = `{"interlaced":false}`
		} else if state == "interlaced" {
			want = `{"interlaced":true}`
		}
		if string(raw) != want {
			t.Fatalf("state %q: got %s, want %s", state, raw, want)
		}
	}
}
