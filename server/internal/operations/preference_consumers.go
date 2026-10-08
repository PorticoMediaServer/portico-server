package operations

// Each field has an explicit owner. Client owners identify the published
// preference contract the frontend must consume; server owners enforce policy.
// A new registry field without an owner fails the registry contract test.
var preferenceConsumers = func() map[string]string {
	m := map[string]string{
		"playback.skipBackSeconds":            "client.playback.controls",
		"playback.skipForwardSeconds":         "client.playback.controls",
		"playback.autoplayNext":               "client.playback.continuation",
		"playback.upNextCountdownSeconds":     "client.playback.continuation",
		"playback.passoutProtection":          "client.playback.continuation",
		"playback.passoutAfterEpisodes":       "client.playback.continuation",
		"playback.introSkip":                  "client.playback.markers",
		"playback.creditsSkip":                "client.playback.markers",
		"playback.recapSkip":                  "client.playback.markers",
		"music.defaultSpeed":                  "server.catalog.listening_preferences",
		"audiobooks.defaultSpeed":             "server.catalog.listening_preferences",
		"playback.sleepTimerMinutes":          "server.catalog.listening_preferences",
		"playback.defaultSpeed":               "client.playback.speed",
		"playback.startedThresholdPercent":    "server.catalog.personal_activity",
		"playback.playedThresholdPercent":     "server.catalog.personal_activity",
		"playback.preferredAudioLanguages":    "client.playback.tracks",
		"playback.preferredSubtitleLanguages": "client.playback.tracks",
		"playback.subtitleMode":               "client.playback.subtitles",
		"playback.subtitleSize":               "client.playback.subtitles",
		"playback.subtitleBackground":         "client.playback.subtitles",
		"playback.showSyncedLyrics":           "client.playback.lyrics",
		"delivery.directPlay":                 "server.playback.delivery_policy",
		"delivery.directStream":               "server.playback.delivery_policy",
		"delivery.transcode":                  "server.playback.delivery_policy",
		"music.audioNormalization":            "client.audio_effects",
		"music.crossfadeSeconds":              "client.audio_effects",
		"music.gapless":                       "client.audio_effects",
		"privacy.pauseWatchHistory":           "server.catalog.personal_activity",
		"privacy.showActivityToMembers":       "server.catalog.recommendation_engine",
		"privacy.includeInWatchTogether":      "server.social.privacy",
		"search.rememberHistory":              "server.httpapi.search_history",
		"region.locale":                       "client.region",
		"region.hourCycle":                    "client.region",
		"home.rowOrder":                       "server.catalog.home_layout",
		"home.hiddenRowIds":                   "server.catalog.home_layout",
		"appearance.showBackdrops":            "client.appearance",
		"appearance.cardSizePercent":          "client.appearance",
		"appearance.reduceMotion":             "client.appearance",
		"notifications.badges":                "client.notifications",
	}
	for _, network := range []string{"local", "wifi", "cellular", "unknown"} {
		for _, field := range []string{"mode", "maxVideoBitrateMbps", "maxAudioBitrateKbps", "maxVideoHeight", "allowHDR"} {
			m["quality."+network+"."+field] = "server.playback.delivery_policy"
		}
	}
	return m
}()

// AudioEffectPreferences is the playback renderer's settings object, derived
// only from the effective registry values. Clients apply it to their renderer
// and save edits with PATCH /v1/preferences rather than a second settings store.
type AudioEffectPreferences struct {
	Gapless          bool   `json:"gapless"`
	CrossfadeSeconds int    `json:"crossfadeSeconds"`
	Normalization    string `json:"normalization"`
}

func (v PreferenceValues) AudioEffects() AudioEffectPreferences {
	return AudioEffectPreferences{Gapless: v.Bool("music.gapless"), CrossfadeSeconds: v.Int("music.crossfadeSeconds"), Normalization: v.Text("music.audioNormalization")}
}
