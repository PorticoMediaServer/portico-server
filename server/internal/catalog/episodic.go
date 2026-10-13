package catalog

import (
	"portico.local/server/internal/naming"
	"time"
)

type EpisodeNaming struct {
	AirDate   string
	ShowTitle string
	Year      int
	Season    int
	Numbers   []int
	Numbering string
	Issue     string
}

// ParseEpisode keeps the current catalogue contract while the scanner and
// matcher adopt the full reading. File evidence wins folder conflicts.
func ParseEpisode(relative, kind string) EpisodeNaming {
	out := EpisodeNaming{Season: -1, Issue: "unrecognized_episode_name"}
	r := naming.Read(relative, kind)
	out.ShowTitle, out.Year = r.PreferredTitle()
	if len(r.Diagnostics) > 0 {
		out.Issue = r.Diagnostics[0]
		return out
	}
	if len(r.AirDates) > 0 {
		out.AirDate = r.AirDates[0].Value
		date, _ := time.Parse("2006-01-02", out.AirDate)
		out.Numbering, out.Season, out.Numbers = "date", date.Year(), []int{date.YearDay()}
	} else if len(r.Episodes) > 0 && len(r.Seasons) > 0 {
		out.Numbering, out.Season, out.Numbers = "seasonal", r.Seasons[0].Value, r.Episodes[0].Value
	} else if len(r.AbsoluteNumbers) > 0 {
		out.Numbering, out.Numbers = "absolute", []int{r.AbsoluteNumbers[0].Value}
	} else {
		return out
	}
	// The existing catalogue cannot attach stacked parts or episode zero yet;
	// keep those restrictions here, while returning their evidence to new users.
	if len(r.Parts) > 0 {
		out.Issue = "multipart_episode_requires_assignment"
		return out
	}
	for _, n := range out.Numbers {
		if n < 1 {
			out.Issue = "episode_number_out_of_range"
			return out
		}
	}
	if out.ShowTitle == "" {
		out.Issue = "series_identity_missing"
		return out
	}
	out.Issue = ""
	return out
}
