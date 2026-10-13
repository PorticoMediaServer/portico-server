package playbackv1

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// A profile's last audio and subtitle choice is remembered per show (for every
// episode of it) and per film, as languages rather than track numbers, so it
// carries from one episode's file to the next. It is written when the viewer
// picks a track, and applied at the next start that names none.

// TrackMemory is what is remembered for one show or film.
type TrackMemory struct {
	// AudioLanguage is the language of the audio track last chosen ("" none).
	AudioLanguage string
	// SubtitleMode is "off" (subtitles were turned off), "on" (a track in
	// SubtitleLanguage was chosen) or "" (nothing remembered).
	SubtitleMode     string
	SubtitleLanguage string
}

// trackScopeQuery finds the entity a title's choices are remembered under:
// the show an episode belongs to, else the title itself.
const trackScopeQuery = `WITH RECURSIVE up(id,parent_id,kind,depth) AS (
 SELECT id,parent_id,kind,0 FROM catalog_entities WHERE public_id=pid_blob(?)
 UNION ALL SELECT e.id,e.parent_id,e.kind,up.depth+1 FROM catalog_entities e JOIN up ON e.id=up.parent_id WHERE up.depth<4)
 SELECT id FROM up WHERE depth=0 OR kind=2 ORDER BY (kind=2) DESC,depth LIMIT 1`

// language folds the spellings a file uses for a language to one key.
func trackLanguage(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.IndexAny(v, "-_"); i > 0 {
		v = v[:i]
	}
	if len(v) == 3 {
		if two, ok := subtitles.TwoLetterLanguage(v); ok {
			return two
		}
	}
	if v == "und" || v == "unknown" || len(v) > 16 {
		return ""
	}
	return v
}

// trackMemory reads what this profile last chose for the item's show or film.
func (s *Service) trackMemory(ctx context.Context, p identity.Principal, item string) TrackMemory {
	var out TrackMemory
	if s.DB == nil || item == "" {
		return out
	}
	err := dbwork.QueryRow(ctx, s.DB, `SELECT audio_language,subtitle_mode,subtitle_language FROM playback_track_memory WHERE profile_id=? AND item_id=(`+trackScopeQuery+`)`, identity.PersonalKey(p.Viewer), item).Scan(&out.AudioLanguage, &out.SubtitleMode, &out.SubtitleLanguage)
	if err != nil {
		return TrackMemory{}
	}
	return out
}

// rememberTracks records a choice the viewer made. audio is the language of
// the audio track they picked (nil: they picked none this time); subtitleMode
// and subtitleLanguage likewise ("" mode: no subtitle choice this time). A
// failure to remember never fails playback.
func (s *Service) rememberTracks(ctx context.Context, p identity.Principal, item string, audio *string, subtitleMode, subtitleLanguage string) {
	if s.DB == nil || item == "" || audio == nil && subtitleMode == "" {
		return
	}
	now := s.now().UnixMilli()
	_ = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassEstablishedPlayback, func(ctx context.Context, tx *sql.Tx) error {
		var scope int64
		if err := tx.QueryRowContext(ctx, trackScopeQuery, item).Scan(&scope); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		owner := identity.PersonalKey(p.Viewer)
		if _, err := tx.ExecContext(ctx, `INSERT INTO playback_track_memory(profile_id,item_id,updated_ms) VALUES(?,?,?) ON CONFLICT(profile_id,item_id) DO UPDATE SET updated_ms=excluded.updated_ms`, owner, scope, now); err != nil {
			return err
		}
		if audio != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE playback_track_memory SET audio_language=? WHERE profile_id=? AND item_id=?`, trackLanguage(*audio), owner, scope); err != nil {
				return err
			}
		}
		if subtitleMode != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE playback_track_memory SET subtitle_mode=?,subtitle_language=? WHERE profile_id=? AND item_id=?`, subtitleMode, trackLanguage(subtitleLanguage), owner, scope); err != nil {
				return err
			}
		}
		return nil
	})
}

// rememberedSubtitle turns what is remembered into a subtitle request for one
// presentation's subtitle catalogue: off, the best track in the remembered
// language, or nil when nothing is remembered or no track matches (the
// automatic choice then stands). A full track is preferred to a forced one,
// and one the device is given as text to one that must be burned in.
func rememberedSubtitle(memory TrackMemory, plan subtitles.PlaybackPlan) *SubtitleRequest {
	switch memory.SubtitleMode {
	case "off":
		return &SubtitleRequest{TrackID: TrackOff}
	case "on":
	default:
		return nil
	}
	if memory.SubtitleLanguage == "" {
		return nil
	}
	best, bestScore := "", -1
	for _, r := range plan.Resources {
		if !r.Enabled || trackLanguage(r.Language) != memory.SubtitleLanguage {
			continue
		}
		score := 0
		if !r.Forced {
			score += 2
		}
		if r.Renderer == "external_text" {
			score++
		}
		if score > bestScore {
			best, bestScore = r.ID, score
		}
	}
	if best == "" {
		return nil
	}
	return &SubtitleRequest{TrackID: TrackRef(best)}
}

// rememberPresentationTracks records only a successfully published choice.
func (s *Service) rememberPresentationTracks(ctx context.Context, p identity.Principal, item, media string, view Presentation, audio, subtitle bool) {
	if !audio && !subtitle {
		return
	}
	plan, err := s.Playback.SessionPlan(ctx, media)
	if err != nil || plan == nil {
		return
	}
	var audioLanguage *string
	if audio && plan.Trace != nil && plan.Trace.Audio != nil {
		audioLanguage = &plan.Trace.Audio.Language
	}
	mode, language := "", ""
	if subtitle {
		mode = "off"
		if view.Subtitle != nil {
			mode = "on"
			for _, resource := range s.subtitleResources(ctx, p, item, plan.SourceID) {
				if resource.ID == view.Subtitle.TrackID {
					language = resource.Language
					break
				}
			}
		}
	}
	s.rememberTracks(ctx, p, item, audioLanguage, mode, language)
}
