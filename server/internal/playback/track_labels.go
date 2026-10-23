package playback

import "strings"

// TrackLabelParts are source facts plus a joined fallback label. Clients may
// localise the language/source words and split the first part from the rest;
// codec and object-format names remain proper names. No subtitle/source URL
// or local filename is part of a label.
type TrackLabelParts struct {
	Title       string `json:"title,omitempty"`
	Language    string `json:"language,omitempty"`
	Format      string `json:"format"`
	ObjectAudio string `json:"objectAudio,omitempty"`
	Layout      string `json:"layout,omitempty"`
	Origin      string `json:"origin,omitempty"`
	Label       string `json:"label"`
}

func labelLayout(layout string) string {
	switch layout {
	case "5.1(side)":
		return "5.1"
	case "7.1(wide)", "7.1(wide-side)":
		return "7.1"
	}
	return publicAudioText(layout, 64)
}
func AudioTrackLabel(title, language, codec, variant, objects, layout string) TrackLabelParts {
	out := TrackLabelParts{Title: publicAudioText(title, 96), Language: publicAudioText(language, 32), Layout: labelLayout(layout)}
	switch variant {
	case "dtsx", "dts_hd_ma":
		out.Format = "DTS-HD Master Audio"
	case "dts_hd_hra":
		out.Format = "DTS-HD High Resolution"
	case "dts_core":
		out.Format = "DTS"
	case "dts_express":
		out.Format = "DTS Express"
	case "truehd", "truehd_atmos":
		out.Format = "Dolby TrueHD"
	case "eac3", "eac3_joc":
		out.Format = "Dolby Digital Plus"
	case "ac3":
		out.Format = "Dolby Digital"
	case "ac4":
		out.Format = "Dolby AC-4"
	case "lpcm":
		out.Format = "LPCM"
	default:
		out.Format = map[string]string{"aac": "AAC", "flac": "FLAC", "alac": "ALAC", "pcm": "LPCM", "truehd": "Dolby TrueHD", "eac3": "Dolby Digital Plus", "ac3": "Dolby Digital", "dts": "DTS", "dts_hd": "DTS-HD", "mp3": "MP3", "mp2": "MPEG Audio", "opus": "Opus", "vorbis": "Vorbis"}[codec]
		if out.Format == "" {
			out.Format = publicAudioText(codec, 64)
		}
	}
	// Objects are observed metadata, never inferred from E-AC-3 or DTS family.
	if objects == "atmos" || objects == "dtsx" {
		out.ObjectAudio = objects
	}
	head := out.Title
	if head == "" {
		head = out.Language
	}
	parts := []string{}
	if head != "" {
		parts = append(parts, head)
	}
	format := out.Format
	if out.ObjectAudio == "atmos" {
		format += " with Atmos"
	}
	if format != "" {
		parts = append(parts, format)
	}
	if out.Layout != "" {
		parts = append(parts, out.Layout)
	}
	out.Label = strings.Join(parts, ", ")
	if out.ObjectAudio == "dtsx" {
		out.Label += " (DTS:X)"
	}
	return out
}
func SubtitleTrackLabel(title, language, format, source string) TrackLabelParts {
	out := TrackLabelParts{Title: publicAudioText(title, 96), Language: publicAudioText(language, 32)}
	out.Format = map[string]string{"ass": "ASS", "ssa": "ASS", "subrip": "SRT", "srt": "SRT", "webvtt": "WebVTT", "vtt": "WebVTT", "hdmv_pgs_subtitle": "PGS", "pgs": "PGS", "dvd_subtitle": "VobSub", "vobsub": "VobSub", "dvb_subtitle": "DVB", "dvb": "DVB", "eia_608": "CEA-608", "cea608": "CEA-608", "eia_708": "CEA-708", "cea708": "CEA-708"}[format]
	if out.Format == "" {
		out.Format = publicAudioText(format, 48)
	}
	switch source {
	case "embedded", "sidecar", "provider", "upload", "downloaded":
		out.Origin = source
	}
	head := out.Title
	if head == "" {
		head = out.Language
	}
	parts := []string{}
	if head != "" {
		parts = append(parts, head)
	}
	if out.Format != "" {
		parts = append(parts, out.Format)
	}
	out.Label = strings.Join(parts, ", ")
	// Localised clients use Source; HLS NAME has this plain fallback.
	if source == "sidecar" {
		out.Label += ", beside the file"
	} else if source == "downloaded" || source == "provider" {
		out.Label += ", downloaded"
	}
	return out
}
