package app

import "strings"

type languageSettings struct {
	AudioLanguage    string
	SubtitleLanguage string
	SubtitleMode     string
	PreferForcedSubs bool
}

func (s *Server) applyLanguagePreferences(user User, audio []Stream, subtitles []Stream) ([]Stream, []Stream) {
	settings := s.languageSettings()
	audioPreference := firstNonEmpty(user.Preferences.AudioLanguage, settings.AudioLanguage, "English")
	subtitlePreference := firstNonEmpty(user.Preferences.SubtitleLanguage, settings.SubtitleLanguage, "English")
	subtitleMode := strings.ToLower(firstNonEmpty(settings.SubtitleMode, "manual"))

	audio = prioritizeLanguageStreams(audio, audioPreference, false)
	selectedAudio := Stream{}
	if len(audio) > 0 {
		selectedAudio = audio[0]
	}
	switch subtitleMode {
	case "always", "on":
		subtitles = prioritizeSubtitleStreams(subtitles, subtitlePreference, selectedAudio, settings.PreferForcedSubs, true)
	case "foreignaudio", "foreign":
		if selectedAudio.ID != "" && !streamMatchesLanguage(selectedAudio, audioPreference) {
			subtitles = prioritizeSubtitleStreams(subtitles, subtitlePreference, selectedAudio, settings.PreferForcedSubs, true)
		} else if settings.PreferForcedSubs {
			subtitles = prioritizeForcedSubtitles(subtitles, subtitlePreference, selectedAudio, false)
		} else {
			subtitles = prioritizeNoneSubtitle(subtitles)
		}
	default:
		if settings.PreferForcedSubs {
			subtitles = prioritizeForcedSubtitles(subtitles, subtitlePreference, selectedAudio, false)
		} else {
			subtitles = prioritizeNoneSubtitle(subtitles)
		}
	}
	return audio, subtitles
}

func (s *Server) languageSettings() languageSettings {
	settings, err := s.loadSettings()
	if err != nil {
		return languageSettings{AudioLanguage: "English", SubtitleLanguage: "English", SubtitleMode: "manual", PreferForcedSubs: true}
	}
	group, _ := settings["languages"].(map[string]any)
	return languageSettings{
		AudioLanguage:    settingString(group, "audio", "English"),
		SubtitleLanguage: settingString(group, "subtitle", "English"),
		SubtitleMode:     settingString(group, "subtitleMode", "manual"),
		PreferForcedSubs: settingBool(group, "preferForcedSubs", true),
	}
}

func prioritizeSubtitleStreams(streams []Stream, language string, selectedAudio Stream, preferForced bool, keepNone bool) []Stream {
	if preferForced {
		prioritized := prioritizeForcedSubtitles(streams, language, selectedAudio, keepNone)
		if len(prioritized) > 0 && prioritized[0].ID != "sub_none" && isForcedSubtitle(prioritized[0]) {
			return prioritized
		}
	}
	return prioritizeLanguageStreams(streams, language, keepNone)
}

func prioritizeForcedSubtitles(streams []Stream, subtitleLanguage string, selectedAudio Stream, keepNone bool) []Stream {
	if len(streams) == 0 {
		return streams
	}
	forcedPreferred := []Stream{}
	forcedOther := []Stream{}
	rest := []Stream{}
	none := []Stream{}
	for _, stream := range streams {
		if stream.ID == "sub_none" {
			none = append(none, stream)
			continue
		}
		if !isForcedSubtitle(stream) {
			rest = append(rest, stream)
			continue
		}
		if streamMatchesLanguage(stream, subtitleLanguage) || (selectedAudio.ID != "" && streamMatchesLanguage(stream, selectedAudio.Language)) {
			forcedPreferred = append(forcedPreferred, stream)
		} else {
			forcedOther = append(forcedOther, stream)
		}
	}
	out := append(forcedPreferred, forcedOther...)
	out = append(out, rest...)
	if keepNone {
		out = append(out, none...)
	} else if len(forcedPreferred) == 0 && len(forcedOther) == 0 {
		out = append(none, out...)
	} else {
		out = append(out, none...)
	}
	if len(out) == 0 {
		return streams
	}
	return out
}

func prioritizeLanguageStreams(streams []Stream, language string, keepNone bool) []Stream {
	if len(streams) == 0 {
		return streams
	}
	matches := []Stream{}
	rest := []Stream{}
	none := []Stream{}
	for _, stream := range streams {
		if stream.ID == "sub_none" {
			none = append(none, stream)
			continue
		}
		if languageMatches(stream.Language, stream.DisplayTitle, language) {
			matches = append(matches, stream)
		} else {
			rest = append(rest, stream)
		}
	}
	out := append(matches, rest...)
	if keepNone {
		out = append(out, none...)
	} else {
		out = append(none, out...)
	}
	if len(out) == 0 {
		return streams
	}
	return out
}

func prioritizeNoneSubtitle(streams []Stream) []Stream {
	none := []Stream{}
	rest := []Stream{}
	for _, stream := range streams {
		if stream.ID == "sub_none" {
			none = append(none, stream)
		} else {
			rest = append(rest, stream)
		}
	}
	return append(none, rest...)
}

func languageMatches(language, displayTitle, preference string) bool {
	preference = strings.ToLower(strings.TrimSpace(preference))
	if preference == "" {
		return false
	}
	language = strings.ToLower(strings.TrimSpace(language))
	displayTitle = strings.ToLower(strings.TrimSpace(displayTitle))
	return language == preference ||
		strings.HasPrefix(language, preference[:min(len(preference), 2)]) ||
		strings.Contains(displayTitle, preference)
}

func streamMatchesLanguage(stream Stream, preference string) bool {
	return languageMatches(stream.Language, stream.DisplayTitle, preference)
}

func isForcedSubtitle(stream Stream) bool {
	return strings.Contains(strings.ToLower(stream.DisplayTitle), "forced")
}
