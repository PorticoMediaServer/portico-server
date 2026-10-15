package playbackv1

import (
	"context"
	"strings"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/subtitles"
)

// The viewer's Settings (preferred audio and subtitle languages, subtitle
// mode) decide the tracks a playback starts with. An audio or subtitle track
// picked in the player is for that playback only: it carries from one queue
// entry to the next (autoplay, Up Next, a binge), as languages rather than
// track numbers so it fits the next file, and a playback started afresh goes
// back to the Settings. Nothing is remembered per title, profile or device.

// TrackCarry is the audio and subtitle choice one playback carries from one
// queue entry to the next. It travels in the session's stored request.
type TrackCarry struct {
	// AudioLanguage is the language of the audio track picked ("" none).
	AudioLanguage string `json:"audioLanguage,omitempty"`
	// SubtitleMode is "off" (subtitles were turned off), "on" (a track in
	// SubtitleLanguage was picked) or "" (no subtitle choice).
	SubtitleMode     string `json:"subtitleMode,omitempty"`
	SubtitleLanguage string `json:"subtitleLanguage,omitempty"`
}

func (c *TrackCarry) empty() bool {
	return c == nil || c.AudioLanguage == "" && c.SubtitleMode == ""
}

// trackLanguage folds the spellings a file uses for a language to one key.
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

// carriedSubtitle turns a carried choice into a subtitle request for one
// presentation's subtitle catalogue: off, the best track in the carried
// language, or nil when nothing is carried or no track matches (the Settings
// then decide). A full track is preferred to a forced one, and one the device
// is given as text to one that must be burned in.
func carriedSubtitle(carry *TrackCarry, plan subtitles.PlaybackPlan) *SubtitleRequest {
	if carry == nil {
		return nil
	}
	switch carry.SubtitleMode {
	case "off":
		return &SubtitleRequest{TrackID: TrackOff}
	case "on":
	default:
		return nil
	}
	if carry.SubtitleLanguage == "" {
		return nil
	}
	best, bestScore := "", -1
	for _, r := range plan.Resources {
		if !r.Enabled || trackLanguage(r.Language) != carry.SubtitleLanguage {
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

// carryChoice folds a choice the viewer just made in the player (audio,
// subtitle, or both) into what the playback carries, as the languages of the
// tracks the published presentation actually plays. A part not chosen now
// keeps what was carried before. Nothing is written anywhere but the session.
func (s *Service) carryChoice(ctx context.Context, p identity.Principal, item, media string, view Presentation, audio, subtitle bool, prior *TrackCarry) *TrackCarry {
	if !audio && !subtitle {
		return prior
	}
	plan, err := s.Playback.SessionPlan(ctx, media)
	if err != nil || plan == nil {
		return prior
	}
	next := TrackCarry{}
	if prior != nil {
		next = *prior
	}
	if audio && plan.Trace != nil && plan.Trace.Audio != nil {
		next.AudioLanguage = trackLanguage(plan.Trace.Audio.Language)
	}
	if subtitle {
		next.SubtitleMode, next.SubtitleLanguage = "off", ""
		if view.Subtitle != nil {
			next.SubtitleMode = "on"
			for _, resource := range s.subtitleResources(ctx, p, item, plan.SourceID) {
				if resource.ID == view.Subtitle.TrackID {
					next.SubtitleLanguage = trackLanguage(resource.Language)
					break
				}
			}
		}
	}
	if next.empty() {
		return nil
	}
	return &next
}

// queueCarry is what a queue's next start carries from the session the queue
// last played: a playback continued (an advance, Up Next, a restart in place)
// keeps the tracks picked in the player. A new queue has no session, so a
// playback started afresh carries nothing.
func (s *Service) queueCarry(ctx context.Context, c Caller, session string) *TrackCarry {
	if session == "" {
		return nil
	}
	old, err := loadRow(ctx, s.DB, session)
	if err != nil || !owns(c, old) {
		return nil
	}
	carry := old.stored().Carry
	if carry.empty() {
		return nil
	}
	return carry
}
