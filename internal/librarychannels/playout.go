package librarychannels

import "time"

// ResolvePlayoutSource is the mandatory tune-time authorization boundary. Guide
// metadata is never authority: the caller supplies the current profile decision
// again immediately before starting or joining a stream.
func ResolvePlayoutSource(entry ScheduleEntry, at time.Time, decide func(string) AccessDecision) (PlayoutSource, error) {
	if at.Before(entry.StartsAt) || !at.Before(entry.EndsAt) {
		return PlayoutSource{}, invalid("tuneTime", "must fall within the scheduled program")
	}
	switch entry.Kind {
	case EntrySlate:
		if err := validatePlayoutSource(entry); err != nil {
			return PlayoutSource{}, err
		}
		return entry.PlayoutSource, nil
	case EntryUnavailable:
		return PlayoutSource{}, ErrProgramUnavailable
	case EntryMedia:
		if decide == nil || decide(entry.MediaID) != AccessAllowed {
			return PlayoutSource{}, ErrProgramRestricted
		}
		if err := validatePlayoutSource(entry); err != nil {
			return PlayoutSource{}, err
		}
		source := entry.PlayoutSource
		source.OffsetSeconds = entry.MediaOffsetSeconds + int(at.Sub(entry.StartsAt)/time.Second)
		if source.OffsetSeconds >= source.DurationSeconds {
			return PlayoutSource{}, ErrProgramUnavailable
		}
		return source, nil
	default:
		return PlayoutSource{}, ErrProgramUnavailable
	}
}
