package qualityladder

// MusicPreset is a saved music quality, separate from the video ladder. The
// approved preference values are bitrate strings; requests use audio limits.
type MusicPreset struct {
	ID            string  `json:"id"`
	AudioKbps     int     `json:"audioKbps"`
	DataMBPerHour float64 `json:"dataMBPerHour"`
}

func Music() []MusicPreset {
	return []MusicPreset{{ID: "original"},
		{ID: "320", AudioKbps: 320, DataMBPerHour: 144},
		{ID: "256", AudioKbps: 256, DataMBPerHour: 115.2},
		{ID: "192", AudioKbps: 192, DataMBPerHour: 86.4},
		{ID: "128", AudioKbps: 128, DataMBPerHour: 57.6},
		{ID: "96", AudioKbps: 96, DataMBPerHour: 43.2}}
}

func MusicIDs() []string {
	out := []string{}
	for _, p := range Music() {
		out = append(out, p.ID)
	}
	return out
}

func FindMusic(id string) (MusicPreset, bool) {
	for _, p := range Music() {
		if p.ID == id {
			return p, true
		}
	}
	return MusicPreset{}, false
}
