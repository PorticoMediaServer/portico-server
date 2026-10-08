package subtitles

import (
	"context"
	"database/sql"
	"strings"
)

// The viewer's subtitle preferences existed and nothing read them: a profile set
// to "subtitles on, Spanish" started every title with subtitles off, and a film
// whose aliens speak in a forced track showed no translation unless someone went
// looking for it. The selection below runs once, when a session's subtitle state
// is first created, and only ever chooses a text track that is already stored:
// it never extracts, converts or burns anything in, so it adds one indexed read
// to starting a title and cannot fail it.

// AutoChoice is what the viewer wants without being asked.
type AutoChoice struct {
	ForeignAudio bool
	// Enabled is the viewer's "always show subtitles" preference.
	Enabled bool
	// Off is the viewer's "never choose subtitles for me": no track is chosen, not even
	// a forced one or one for audio in a language they did not ask for.
	Off bool
	// Languages is the viewer's ordered subtitle language list.
	Languages []string
	// AudioLanguage is the language of the audio track the session will play. A
	// forced track is only meaningful beside audio in the same language.
	AudioLanguage string
}

// AutoCandidate is one stored track that could be chosen.
type AutoCandidate struct {
	ID       string
	Revision int64
	Language string
	Title    string
	Forced   bool
	Default  bool
	OffsetUS int64
}

func subtitleLanguageKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_"); i > 0 {
		s = s[:i]
	}
	if two, ok := iso639ThreeToTwo[s]; ok {
		return two
	}
	if s == "und" {
		return ""
	}
	return s
}

var iso639ThreeToTwo = map[string]string{
	"eng": "en", "fre": "fr", "fra": "fr", "ger": "de", "deu": "de", "spa": "es", "ita": "it", "jpn": "ja", "kor": "ko",
	"chi": "zh", "zho": "zh", "por": "pt", "rus": "ru", "dut": "nl", "nld": "nl", "swe": "sv", "nor": "no", "nob": "no",
	"dan": "da", "fin": "fi", "pol": "pl", "tur": "tr", "ara": "ar", "heb": "he", "hin": "hi", "tha": "th", "vie": "vi",
	"cze": "cs", "ces": "cs", "gre": "el", "ell": "el", "hun": "hu", "rum": "ro", "ron": "ro", "ukr": "uk", "ind": "id",
}

func hearingImpairedTitle(title string) bool {
	t := strings.ToLower(title)
	return strings.Contains(t, "sdh") || strings.Contains(t, "hearing") || strings.Contains(t, "[cc]") || strings.Contains(t, "closed caption")
}

// ChooseAutomatic applies the rule. With subtitles off altogether: nothing. With subtitles on: the first preferred
// language that has a track, taking an ordinary track over an SDH one and never a
// forced one (a forced track is a fragment, not the dialogue). With subtitles
// off: a forced track in the language of the audio, because that is the part of
// the film the audio does not carry. Otherwise nothing.
func ChooseAutomatic(choice AutoChoice, candidates []AutoCandidate) *AutoCandidate {
	if choice.Off {
		return nil
	}
	if choice.Enabled || choice.ForeignAudio {
		for _, want := range choice.Languages {
			key := subtitleLanguageKey(want)
			if key == "" {
				continue
			}
			var fallback *AutoCandidate
			for i := range candidates {
				c := &candidates[i]
				if c.Forced || subtitleLanguageKey(c.Language) != key {
					continue
				}
				if !hearingImpairedTitle(c.Title) {
					return c
				}
				if fallback == nil {
					fallback = c
				}
			}
			if fallback != nil {
				return fallback
			}
		}
		return nil
	}
	audio := subtitleLanguageKey(choice.AudioLanguage)
	if audio == "" {
		return nil
	}
	for i := range candidates {
		if c := &candidates[i]; c.Forced && subtitleLanguageKey(c.Language) == audio {
			return c
		}
	}
	return nil
}

// ViewerKey is the key a viewer's personal subtitle tracks are stored under.
func ViewerKey(authority, account, profile, server string) string {
	return identityDigest(authority + "\x00" + account + "\x00" + profile + "\x00" + server)
}

// AutoSelectTx creates a session's subtitle state and, the first time only,
// applies the viewer's preferences to the tracks already stored for the source.
// owner is the viewer key personal tracks are stored under.
func AutoSelectTx(ctx context.Context, tx *sql.Tx, session string, generation int, item, source, owner string, choice AutoChoice) error {
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO playback_subtitle_state(session_id,generation) VALUES(?,?)`, session, generation)
	if err != nil {
		return err
	}
	if created, _ := result.RowsAffected(); created != 1 {
		return nil
	}
	if !choice.Enabled && !choice.ForeignAudio && choice.AudioLanguage == "" {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,v.revision,v.language,v.title,COALESCE(rr.is_forced,0),COALESCE(rr.is_default,0),v.offset_us FROM subtitle_resources r JOIN subtitle_revisions v ON v.resource_id=r.id AND v.revision=r.current_revision LEFT JOIN subtitle_render_revisions rr ON rr.resource_id=r.id AND rr.revision=v.revision JOIN catalog_assets a ON a.token=r.source_id WHERE r.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND r.source_id=? AND r.deleted=0 AND (r.scope='shared' OR r.owner=?) AND COALESCE(rr.renderer,'external_text')='external_text' AND COALESCE(rr.source_evidence,'')='' AND v.source_size=a.size AND v.source_modified_ns=a.modified_ns ORDER BY COALESCE(rr.is_default,0) DESC,r.id LIMIT 64`, item, source, owner)
	if err != nil {
		// A schema this query does not fit must never stop a title from starting.
		return nil
	}
	var candidates []AutoCandidate
	for rows.Next() {
		var c AutoCandidate
		if rows.Scan(&c.ID, &c.Revision, &c.Language, &c.Title, &c.Forced, &c.Default, &c.OffsetUS) != nil {
			break
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	chosen := ChooseAutomatic(choice, candidates)
	if chosen == nil {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO playback_subtitle_pins VALUES(?,?,?)`, session, chosen.ID, chosen.Revision); err != nil {
		return nil
	}
	_, _ = tx.ExecContext(ctx, `UPDATE playback_subtitle_state SET resource_id=?,resource_revision=?,offset_us=?,renderer='external_text' WHERE session_id=? AND generation=?`, chosen.ID, chosen.Revision, chosen.OffsetUS, session, generation)
	return nil
}
