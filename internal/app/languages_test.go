package app

import "testing"

func TestApplyLanguagePreferencesPrioritizesUserAudioAndServerSubtitles(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'languages'`, `{"audio":"English","subtitle":"Japanese","subtitleMode":"always"}`); err != nil {
		t.Fatalf("save language settings: %v", err)
	}
	user := User{Preferences: UserPreferences{AudioLanguage: "French"}}
	audio := []Stream{
		{ID: "audio_en", Kind: "audio", Language: "English", DisplayTitle: "English"},
		{ID: "audio_fr", Kind: "audio", Language: "French", DisplayTitle: "French"},
	}
	subtitles := []Stream{
		{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"},
		{ID: "sub_ja", Kind: "subtitle", Language: "Japanese", DisplayTitle: "Japanese"},
	}

	audio, subtitles = server.applyLanguagePreferences(user, audio, subtitles)
	if audio[0].ID != "audio_fr" {
		t.Fatalf("audio order = %#v", audio)
	}
	if subtitles[0].ID != "sub_ja" {
		t.Fatalf("subtitle order = %#v", subtitles)
	}
}

func TestApplyLanguagePreferencesDefaultsSubtitlesOffInManualMode(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'languages'`, `{"audio":"English","subtitle":"English","subtitleMode":"manual","preferForcedSubs":false}`); err != nil {
		t.Fatalf("save language settings: %v", err)
	}
	subtitles := []Stream{
		{ID: "sub_en", Kind: "subtitle", Language: "English", DisplayTitle: "English"},
		{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"},
	}
	_, subtitles = server.applyLanguagePreferences(User{}, nil, subtitles)
	if subtitles[0].ID != "sub_none" {
		t.Fatalf("manual subtitle mode should keep None first: %#v", subtitles)
	}
}

func TestApplyLanguagePreferencesEnablesSubtitlesForForeignAudio(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'languages'`, `{"audio":"English","subtitle":"English","subtitleMode":"foreignAudio","preferForcedSubs":true}`); err != nil {
		t.Fatalf("save language settings: %v", err)
	}
	audio := []Stream{
		{ID: "audio_ja", Kind: "audio", Language: "Japanese", DisplayTitle: "Japanese"},
	}
	subtitles := []Stream{
		{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"},
		{ID: "sub_en", Kind: "subtitle", Language: "English", DisplayTitle: "English"},
	}

	_, subtitles = server.applyLanguagePreferences(User{}, audio, subtitles)
	if subtitles[0].ID != "sub_en" {
		t.Fatalf("foreign audio should prioritize preferred subtitles: %#v", subtitles)
	}
}

func TestApplyLanguagePreferencesPrioritizesForcedSubtitles(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'languages'`, `{"audio":"English","subtitle":"English","subtitleMode":"manual","preferForcedSubs":true}`); err != nil {
		t.Fatalf("save language settings: %v", err)
	}
	subtitles := []Stream{
		{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"},
		{ID: "sub_en", Kind: "subtitle", Language: "English", DisplayTitle: "English"},
		{ID: "sub_forced", Kind: "subtitle", Language: "English", DisplayTitle: "English Forced"},
	}

	_, subtitles = server.applyLanguagePreferences(User{}, nil, subtitles)
	if subtitles[0].ID != "sub_forced" {
		t.Fatalf("forced subtitles should be first when preferred: %#v", subtitles)
	}
}
