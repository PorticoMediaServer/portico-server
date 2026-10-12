package catalog

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
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

// A frame size ("1920x1080") and the x264/x265 codec written straight after a
// channel layout ("DD5.1x264") have the shape of a season-by-episode marker and
// are neither.
var frameSizeToken = regexp.MustCompile(`(?i)^([0-9]{3,4})x([0-9]{3,4})$`)
var codecToken = regexp.MustCompile(`(?i)^[0-9]x26[45]$`)

// frameWidths are the picture widths video is encoded at. A pair of long
// numbers is a frame size only when the first is one of these and the second
// can be its height (cropped, anamorphic or portrait); any other pair, such as
// "101x102", is still season 101 episode 102.
var frameWidths = map[int]bool{7680: true, 4096: true, 3840: true, 3440: true, 2560: true, 2048: true, 1920: true, 1440: true, 1280: true, 1080: true, 1024: true, 960: true, 854: true, 852: true, 848: true, 800: true, 768: true, 720: true, 704: true, 640: true, 624: true, 608: true, 576: true, 544: true, 512: true, 480: true, 426: true}

func frameSize(raw string) bool {
	m := frameSizeToken.FindStringSubmatch(raw)
	if m == nil {
		return false
	}
	width, _ := strconv.Atoi(m[1])
	height, _ := strconv.Atoi(m[2])
	return frameWidths[width] && height >= 240 && 3*height >= width && 5*height <= 12*width
}

// episodeMarkers splits the marker-shaped runs in a name into the ones that
// are episode markers and the technical tags that only look like one.
func episodeMarkers(name string) (markers, tags [][]int) {
	markers = [][]int{}
	for _, loc := range seasonalToken.FindAllStringIndex(name, -1) {
		raw := name[loc[0]:loc[1]]
		if frameSize(raw) || codecToken.MatchString(raw) && loc[0] >= 2 && name[loc[0]-1] == '.' && name[loc[0]-2] >= '0' && name[loc[0]-2] <= '9' {
			tags = append(tags, loc)
			continue
		}
		markers = append(markers, loc)
	}
	return markers, tags
}

// seriesText is the part of a name before its episode marker, without the
// technical tags in it: "Show - 1920x1080 - S01E02" names the series "Show",
// and 1920 is not its year.
func seriesText(name string, end int, tags [][]int) string {
	var out strings.Builder
	from := 0
	for _, loc := range tags {
		if loc[1] > end {
			break
		}
		out.WriteString(name[from:loc[0]])
		out.WriteByte(' ')
		from = loc[1]
	}
	out.WriteString(name[from:end])
	return out.String()
}

// sameSeriesName compares a folder's and a file's series name. An accented
// letter can be written composed or decomposed and looks the same either way,
// so both are brought to one form before the case-insensitive comparison.
func sameSeriesName(a, b string) bool {
	return strings.EqualFold(norm.NFC.String(a), norm.NFC.String(b))
}

func cleanSeries(s string) (string, int) {
	s = groupPrefix.ReplaceAllString(s, "")
	return FilenameTitle(strings.TrimSpace(strings.Trim(s, " ._-")) + ".mkv")
}
func ParseEpisode(relative, kind string) EpisodeNaming {
	out := EpisodeNaming{Season: -1, Issue: "unrecognized_episode_name"}
	name := strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
	locs, tags := episodeMarkers(name)
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
		out.ShowTitle, out.Year = cleanSeries(seriesText(name, loc[0], tags))
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
		out.ShowTitle, out.Year = cleanSeries(seriesText(name, loc[0], tags))
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
		if sameSeriesName(title, out.ShowTitle) && out.Year == 0 {
			out.Year = year
		} else if !sameSeriesName(title, out.ShowTitle) {
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
