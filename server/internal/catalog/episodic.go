package catalog

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

var seasonFolder = regexp.MustCompile(`(?i)^season[ ._-]*([0-9]{1,4})$`)
var seasonalToken = regexp.MustCompile(`(?i)s[0-9]{1,4}e[0-9]{1,4}(?:(?:e|-e?)[0-9]{1,4})*|[0-9]{1,4}x[0-9]{1,4}(?:x[0-9]{1,4})*`)
var airDateToken = regexp.MustCompile(`(?:^|[ ._-])([0-9]{4})[ ._-]([0-9]{2})[ ._-]([0-9]{2})(?:[^0-9]|$)`)
var numberToken = regexp.MustCompile(`[0-9]+`)
var absoluteToken = regexp.MustCompile(`^(.*?)\s+-\s+([0-9]{1,4})(?:v[0-9]+)?(?:\s|\[|$)`)
var groupPrefix = regexp.MustCompile(`^\[[^\]]{1,100}\]\s*`)

// A stacked part is only a trailing suffix right before the extension:
// "S01E05 - pt1", "S01E05.part2", "S01E05-cd1", "S01E05 disc 2". A title that
// merely contains "Part 1" (for example "Jupiter Jazz (Part 1)") is a title.
var stackSuffix = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:part|pt|cd|disc|disk)[ ._-]?[0-9]{1,2}$`)

func cleanSeries(s string) (string, int) {
	s = groupPrefix.ReplaceAllString(s, "")
	return FilenameTitle(strings.TrimSpace(strings.Trim(s, " ._-")) + ".mkv")
}
func ParseEpisode(relative, kind string) EpisodeNaming {
	out := EpisodeNaming{Season: -1, Issue: "unrecognized_episode_name"}
	name := strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
	locs := seasonalToken.FindAllStringIndex(name, -1)
	folder := filepath.Base(filepath.Dir(relative))
	expectedSeason := -1
	showFolder := ""
	if parts := seasonFolder.FindStringSubmatch(folder); parts != nil {
		expectedSeason, _ = strconv.Atoi(parts[1])
		showFolder = filepath.Base(filepath.Dir(filepath.Dir(relative)))
	} else if strings.EqualFold(folder, "specials") {
		expectedSeason = 0
		showFolder = filepath.Base(filepath.Dir(filepath.Dir(relative)))
	}
	if len(locs) > 1 {
		out.Issue = "conflicting_episode_markers"
		return out
	}
	if len(locs) == 1 {
		loc := locs[0]
		if loc[1] < len(name) && name[loc[1]] >= '0' && name[loc[1]] <= '9' {
			out.Issue = "episode_number_out_of_range"
			return out
		}
		rest := name[loc[1]:]
		if stackSuffix.MatchString(strings.TrimRight(rest, " ._-")) {
			out.Issue = "multipart_episode_requires_assignment"
			return out
		}
		raw := name[loc[0]:loc[1]]
		numbers := numberToken.FindAllString(raw, -1)
		out.Season, _ = strconv.Atoi(numbers[0])
		out.Numbering = "seasonal"
		if expectedSeason >= 0 && expectedSeason != out.Season {
			out.Issue = "folder_and_filename_season_conflict"
			return out
		}
		for _, value := range numbers[1:] {
			n, _ := strconv.Atoi(value)
			if n < 1 || n > 9999 {
				out.Issue = "episode_number_out_of_range"
				return out
			}
			out.Numbers = append(out.Numbers, n)
		}
		if strings.Contains(raw, "-") {
			if len(out.Numbers) != 2 || out.Numbers[1] < out.Numbers[0] || out.Numbers[1]-out.Numbers[0] >= 8 {
				out.Issue = "ambiguous_episode_range"
				return out
			}
			first, last := out.Numbers[0], out.Numbers[1]
			out.Numbers = nil
			for n := first; n <= last; n++ {
				out.Numbers = append(out.Numbers, n)
			}
		}
		if len(out.Numbers) > 8 {
			out.Issue = "too_many_episode_links"
			return out
		}
		for i, n := range out.Numbers {
			if i > 0 && n <= out.Numbers[i-1] {
				out.Issue = "conflicting_episode_order"
				return out
			}
		}
		out.ShowTitle, out.Year = cleanSeries(name[:loc[0]])
	} else if loc := airDateToken.FindStringSubmatchIndex(name); loc != nil {
		date := name[loc[2]:loc[3]] + "-" + name[loc[4]:loc[5]] + "-" + name[loc[6]:loc[7]]
		air, err := time.Parse("2006-01-02", date)
		if err != nil || air.Year() < 1900 {
			out.Issue = "invalid_air_date"
			return out
		}
		out.AirDate, out.Numbering, out.Season, out.Numbers = date, "date", air.Year(), []int{air.YearDay()}
		if expectedSeason >= 0 && expectedSeason != out.Season {
			out.Issue = "folder_and_filename_season_conflict"
			return out
		}
		out.ShowTitle, out.Year = cleanSeries(name[:loc[0]])
	} else if kind == "anime" {
		match := absoluteToken.FindStringSubmatch(groupPrefix.ReplaceAllString(name, ""))
		if match == nil {
			return out
		}
		n, _ := strconv.Atoi(match[2])
		if n < 1 {
			return out
		}
		out.ShowTitle, out.Year = cleanSeries(match[1])
		out.Numbers = []int{n}
		out.Numbering = "absolute"
	} else {
		return out
	}
	if out.ShowTitle == "" && showFolder != "" && showFolder != "." {
		out.ShowTitle, out.Year = cleanSeries(showFolder)
	}
	if showFolder != "" && showFolder != "." {
		title, year := cleanSeries(showFolder)
		if strings.EqualFold(title, out.ShowTitle) && out.Year == 0 {
			out.Year = year
		} else if !strings.EqualFold(title, out.ShowTitle) {
			out.Issue = "folder_and_filename_series_conflict"
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
