package app

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type scannerEpisodeInfo struct {
	Season        int
	Episode       int
	EpisodeEnd    int
	Year          int
	MatchedToken  string
	DateBased     bool
	AbsoluteBased bool
}

type scannerMediaVersion struct {
	Label        string
	Resolution   string
	Source       string
	VideoCodec   string
	AudioCodec   string
	DynamicRange string
	ReleaseGroup string
	ThreeD       bool
	QualityRank  int
}

var (
	episodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[\s._-])s(\d{1,2})[\s._-]*e(\d{1,3})(?:[\s._-]|$)`),
		regexp.MustCompile(`(?i)(?:^|[\s._-])(\d{1,2})x(\d{1,3})(?:[\s._-]|$)`),
	}
	multiEpisodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[\s._-])s(\d{1,2})[\s._-]*e(\d{1,3})[\s._-]*e(\d{1,3})(?:[\s._-]|$)`),
		regexp.MustCompile(`(?i)(?:^|[\s._-])s(\d{1,2})[\s._-]*e(\d{1,3})[\s._-]*(?:-|to)[\s._-]*e?(\d{1,3})(?:[\s._-]|$)`),
		regexp.MustCompile(`(?i)(?:^|[\s._-])(\d{1,2})x(\d{1,3})[\s._-]*(?:-|to|x)[\s._-]*(\d{1,3})(?:[\s._-]|$)`),
	}
	dateEpisodePattern     = regexp.MustCompile(`(?i)(?:^|[\s._-])((19|20)\d{2})[\s._-](0?[1-9]|1[0-2])[\s._-](0?[1-9]|[12]\d|3[01])(?:[\s._-]|$)`)
	absoluteEpisodePattern = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:ep(?:isode)?[\s._-]*)?(\d{2,4})(?:[\s._-]|$)`)
	animeAbsolutePatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[\s._\-\[\(])(?:ep(?:isode)?|#)[\s._-]*(\d{1,4})(?:[\s._-]*(?:-|to|~)[\s._-]*(\d{1,4}))?(?:[\s._\-\]\)]|$)`),
		regexp.MustCompile(`(?i)(?:^|[\s._-])(\d{1,4})[\s._-]*(?:-|to|~)[\s._-]*(\d{1,4})(?:[\s._-]|$)`),
		regexp.MustCompile(`(?i)(?:^|[\s._-])(\d{1,4})(?:[\s._-]|$)`),
	}
	seasonFolderPattern          = regexp.MustCompile(`(?i)^s(?:eason)?[\s._-]*(\d{1,2})$`)
	resolutionPattern            = regexp.MustCompile(`(?i)(?:^|[\s._-])(4320p|2160p|4k|1080p|720p|576p|480p)(?:[\s._-]|$)`)
	movieYearPattern             = regexp.MustCompile(`(?i)(?:^|[\s._\-\(\[])(19\d{2}|20\d{2})(?:[\s._\-\)\]]|$)`)
	initialedCreditSuffixPattern = regexp.MustCompile(`\s+-\s+(?:[A-Z]\.?\s*){1,4}[A-Z][A-Za-z'.-]+(?:\s*[\(\[]?(?:19|20)\d{2}\b.*)?$`)
)

func cleanMediaTitle(value string) string {
	for _, pattern := range episodePatterns {
		value = pattern.ReplaceAllString(value, " ")
	}
	replacer := strings.NewReplacer(".", " ", "_", " ", "-", " ")
	value = replacer.Replace(value)
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "Untitled"
	}
	fields = trimReleaseInfoFields(fields)
	for i, field := range fields {
		if len(field) > 1 {
			fields[i] = strings.ToUpper(field[:1]) + field[1:]
		}
	}
	return strings.Join(fields, " ")
}

func stripInitialedCreditSuffix(value string) string {
	return strings.TrimSpace(initialedCreditSuffixPattern.ReplaceAllString(value, ""))
}

func trimReleaseInfoFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		normalized := strings.ToLower(strings.Trim(field, "[]().,;:!"))
		if providerTitleStopToken(normalized) {
			break
		}
		if len(normalized) == 4 {
			if year, err := strconv.Atoi(normalized); err == nil && year >= 1900 && year <= time.Now().Year()+2 {
				break
			}
		}
		out = append(out, field)
	}
	if len(out) == 0 {
		return fields
	}
	return out
}

func parseEpisodeNumbers(value string) (int, int) {
	info := parseEpisodeInfo("show", "", value)
	return info.Season, info.Episode
}

func parseEpisodeInfo(libraryType, rel, base string) scannerEpisodeInfo {
	for _, pattern := range multiEpisodePatterns {
		match := pattern.FindStringSubmatch(base)
		if len(match) != 4 {
			continue
		}
		season, _ := strconv.Atoi(match[1])
		episode, _ := strconv.Atoi(match[2])
		episodeEnd, _ := strconv.Atoi(match[3])
		if episodeEnd < episode {
			episodeEnd = episode
		}
		return scannerEpisodeInfo{Season: season, Episode: episode, EpisodeEnd: episodeEnd, MatchedToken: match[0]}
	}
	for _, pattern := range episodePatterns {
		match := pattern.FindStringSubmatch(base)
		if len(match) != 3 {
			continue
		}
		season, _ := strconv.Atoi(match[1])
		episode, _ := strconv.Atoi(match[2])
		return scannerEpisodeInfo{Season: season, Episode: episode, MatchedToken: match[0]}
	}
	if match := dateEpisodePattern.FindStringSubmatch(base); len(match) == 5 {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[3])
		day, _ := strconv.Atoi(match[4])
		return scannerEpisodeInfo{
			Season:       year,
			Episode:      month*100 + day,
			Year:         year,
			MatchedToken: match[0],
			DateBased:    true,
		}
	}
	if strings.EqualFold(libraryType, "anime") {
		if info, ok := parseAnimeAbsoluteEpisodeInfo(rel, base); ok {
			return info
		}
		if match := absoluteEpisodePattern.FindStringSubmatch(base); len(match) == 2 {
			if info, ok := animeAbsoluteInfoFromMatch(rel, match[0], match[1], ""); ok {
				return info
			}
		}
	}
	season := 0
	if parsedSeason, ok := seasonNumberFromPath(rel); ok {
		season = parsedSeason
	}
	if season == 0 {
		season = 1
	}
	// Unknown numbering is not episode one. Keeping an explicit zero lets the
	// scanner preserve each file as an unmatched episode for owner/provider
	// reconciliation instead of silently collapsing unrelated files into S01E01.
	return scannerEpisodeInfo{Season: season}
}

func movieScannerIdentity(libraryID, title string, year int) string {
	identity := filepath.Join(libraryID, strings.ToLower(strings.TrimSpace(title)))
	if year > 0 {
		identity = filepath.Join(identity, strconv.Itoa(year))
	}
	return identity
}

func parseAnimeAbsoluteEpisodeInfo(rel, base string) (scannerEpisodeInfo, bool) {
	for _, pattern := range animeAbsolutePatterns {
		match := pattern.FindStringSubmatch(base)
		if len(match) < 2 {
			continue
		}
		episodeEnd := ""
		if len(match) > 2 {
			episodeEnd = match[2]
		}
		if info, ok := animeAbsoluteInfoFromMatch(rel, match[0], match[1], episodeEnd); ok {
			return info, true
		}
	}
	return scannerEpisodeInfo{}, false
}

func animeAbsoluteInfoFromMatch(rel, matchedToken, episodeValue, episodeEndValue string) (scannerEpisodeInfo, bool) {
	episode, err := strconv.Atoi(strings.TrimSpace(episodeValue))
	if err != nil || !validAnimeAbsoluteEpisodeNumber(episode) {
		return scannerEpisodeInfo{}, false
	}
	episodeEnd := 0
	if strings.TrimSpace(episodeEndValue) != "" {
		parsedEnd, err := strconv.Atoi(strings.TrimSpace(episodeEndValue))
		if err == nil && validAnimeAbsoluteEpisodeNumber(parsedEnd) && parsedEnd >= episode {
			episodeEnd = parsedEnd
		}
	}
	season := 1
	if parsedSeason, ok := seasonNumberFromPath(rel); ok && parsedSeason > 0 {
		season = parsedSeason
	}
	return scannerEpisodeInfo{
		Season:        season,
		Episode:       episode,
		EpisodeEnd:    episodeEnd,
		MatchedToken:  matchedToken,
		AbsoluteBased: true,
	}, true
}

func validAnimeAbsoluteEpisodeNumber(episode int) bool {
	if episode <= 0 {
		return false
	}
	return episode < 1900 || episode > time.Now().Year()+2
}

func cleanEpisodeTitle(base string, info scannerEpisodeInfo) string {
	if strings.TrimSpace(info.MatchedToken) != "" {
		base = strings.Replace(base, info.MatchedToken, " ", 1)
	}
	return cleanMediaTitle(base)
}

func seasonNumberFromPath(rel string) (int, bool) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := len(parts) - 2; i >= 0; i-- {
		normalized := strings.TrimSpace(parts[i])
		if strings.EqualFold(normalized, "specials") {
			return 0, true
		}
		match := seasonFolderPattern.FindStringSubmatch(normalized)
		if len(match) == 2 {
			season, _ := strconv.Atoi(match[1])
			return season, true
		}
	}
	return 0, false
}

func episodeTitle(base string, info scannerEpisodeInfo) string {
	return episodeTitleForShow(base, info, "")
}

func episodeTitleForShow(base string, info scannerEpisodeInfo, showTitle string) string {
	title := cleanEpisodeTitle(base, info)
	title = trimEpisodeShowTitlePrefix(title, showTitle)
	if title == "Untitled" {
		if info.DateBased && info.Year > 0 {
			return fmt.Sprintf("%04d-%02d-%02d", info.Year, info.Episode/100, info.Episode%100)
		}
		return fmt.Sprintf("Episode %d", info.Episode)
	}
	return title
}

func trimEpisodeShowTitlePrefix(title, showTitle string) string {
	title = strings.TrimSpace(title)
	showTitle = strings.TrimSpace(showTitle)
	if title == "" || showTitle == "" {
		return title
	}
	titleFields := strings.Fields(strings.ToLower(title))
	showFields := strings.Fields(strings.ToLower(showTitle))
	if len(showFields) == 0 || len(titleFields) <= len(showFields) {
		return title
	}
	for index, field := range showFields {
		if titleFields[index] != field {
			return title
		}
	}
	originalFields := strings.Fields(title)
	return strings.Join(originalFields[len(showFields):], " ")
}

func showTitleFromPath(rel, base string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 1 {
		return cleanMediaTitle(parts[0])
	}
	return cleanMediaTitle(base)
}

func movieYearFromName(base string) int {
	if match := movieYearPattern.FindStringSubmatch(base); len(match) == 2 {
		year, _ := strconv.Atoi(match[1])
		if year >= 1900 && year <= time.Now().Year()+2 {
			return year
		}
	}
	return 0
}

func movieEditionFromName(base string) string {
	normalized := strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ", "'", " ").Replace(base))
	value := " " + strings.Join(strings.Fields(normalized), " ") + " "
	switch {
	case strings.Contains(value, " director s cut ") || strings.Contains(value, " directors cut "):
		return "Director's Cut"
	case strings.Contains(value, " extended cut ") || strings.Contains(value, " extended edition ") || strings.Contains(value, " extended "):
		return "Extended"
	case strings.Contains(value, " unrated cut ") || strings.Contains(value, " unrated edition ") || strings.Contains(value, " unrated "):
		return "Unrated"
	case strings.Contains(value, " theatrical cut ") || strings.Contains(value, " theatrical edition "):
		return "Theatrical Cut"
	case strings.Contains(value, " ultimate cut ") || strings.Contains(value, " ultimate edition "):
		return "Ultimate Edition"
	case strings.Contains(value, " remastered "):
		return "Remastered"
	case strings.Contains(value, " criterion "):
		return "Criterion"
	default:
		return ""
	}
}

func parseMediaVersionInfo(path string) scannerMediaVersion {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	normalized := strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(base))
	tokens := strings.Fields(normalized)
	tokenSet := map[string]bool{}
	for _, token := range tokens {
		tokenSet[strings.Trim(token, "[]().,;:!")] = true
	}

	version := scannerMediaVersion{}
	if match := resolutionPattern.FindStringSubmatch(base); len(match) == 2 {
		version.Resolution = normalizeResolutionLabel(match[1])
		version.QualityRank += resolutionQualityRank(version.Resolution)
	}
	version.Source = detectVersionSource(tokenSet)
	version.QualityRank += sourceQualityRank(version.Source)
	version.VideoCodec = detectVideoCodec(tokenSet)
	version.AudioCodec = detectAudioCodec(tokenSet)
	version.DynamicRange = detectDynamicRange(tokenSet)
	version.ReleaseGroup = detectReleaseGroup(base)
	version.ThreeD = detectThreeD(tokenSet)
	if version.DynamicRange != "" {
		version.QualityRank += 50
	}
	if version.ThreeD {
		version.QualityRank += 25
	}
	version.Label = mediaVersionLabel(version)
	return version
}

func normalizeResolutionLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "4k":
		return "2160p"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func resolutionQualityRank(resolution string) int {
	switch resolution {
	case "4320p":
		return 5000
	case "2160p":
		return 4000
	case "1080p":
		return 3000
	case "720p":
		return 2000
	case "576p":
		return 1200
	case "480p":
		return 1000
	default:
		return 0
	}
}

func detectVersionSource(tokens map[string]bool) string {
	switch {
	case tokens["bluray"] || tokens["blu"] && tokens["ray"] || tokens["bdrip"] || tokens["brrip"]:
		return "Blu-ray"
	case tokens["webdl"] || tokens["web"] && tokens["dl"] || tokens["web-dl"]:
		return "WEB-DL"
	case tokens["webrip"] || tokens["web"] && tokens["rip"]:
		return "WEBRip"
	case tokens["hdtv"]:
		return "HDTV"
	case tokens["dvdrip"] || tokens["dvd"]:
		return "DVD"
	default:
		return ""
	}
}

func sourceQualityRank(source string) int {
	switch source {
	case "Blu-ray":
		return 80
	case "WEB-DL":
		return 70
	case "WEBRip":
		return 60
	case "HDTV":
		return 40
	case "DVD":
		return 20
	default:
		return 0
	}
}

func detectVideoCodec(tokens map[string]bool) string {
	switch {
	case tokens["av1"]:
		return "av1"
	case tokens["h265"] || tokens["x265"] || tokens["hevc"]:
		return "hevc"
	case tokens["h264"] || tokens["x264"] || tokens["avc"]:
		return "h264"
	case tokens["mpeg2"]:
		return "mpeg2"
	default:
		return ""
	}
}

func detectAudioCodec(tokens map[string]bool) string {
	switch {
	case tokens["atmos"]:
		return "atmos"
	case tokens["truehd"]:
		return "truehd"
	case tokens["dts"] || tokens["dts-hd"] || tokens["dtshd"]:
		return "dts"
	case tokens["eac3"] || tokens["e-ac3"] || tokens["ddp"] || tokens["dd+"]:
		return "eac3"
	case tokens["ac3"]:
		return "ac3"
	case tokens["aac"]:
		return "aac"
	default:
		return ""
	}
}

func detectDynamicRange(tokens map[string]bool) string {
	switch {
	case tokens["dv"] || tokens["dovi"] || tokens["dolby"] && tokens["vision"]:
		return "Dolby Vision"
	case tokens["hdr10plus"] || tokens["hdr10+"] || tokens["hdr"] && tokens["10"] && tokens["plus"]:
		return "HDR10+"
	case tokens["hdr10"] || tokens["hdr"]:
		return "HDR"
	case tokens["sdr"]:
		return "SDR"
	default:
		return ""
	}
}

func detectThreeD(tokens map[string]bool) bool {
	return tokens["3d"] || tokens["sbs"] || tokens["hsbs"] || tokens["half"] && tokens["sbs"] || tokens["mvc"]
}

func detectReleaseGroup(base string) string {
	match := regexp.MustCompile(`-([A-Za-z0-9][A-Za-z0-9._]{1,31})$`).FindStringSubmatch(strings.TrimSpace(base))
	if len(match) != 2 {
		return ""
	}
	group := strings.Trim(match[1], ".-_")
	if group == "" {
		return ""
	}
	normalized := strings.ToLower(group)
	if providerTitleStopToken(normalized) {
		return ""
	}
	return group
}

func mediaVersionLabel(version scannerMediaVersion) string {
	parts := []string{}
	for _, part := range []string{version.Resolution, version.Source, version.DynamicRange, strings.ToUpper(version.VideoCodec), strings.ToUpper(version.AudioCodec)} {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	if version.ThreeD {
		parts = append(parts, "3D")
	}
	return strings.Join(parts, " / ")
}

func albumTitleFromPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 1 {
		return cleanMediaTitle(parts[len(parts)-2])
	}
	return "Scanned Tracks"
}

func audiobookAuthorTitleFromPath(rel, fallbackTitle string) (string, string) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 3 {
		author := cleanMediaTitle(parts[len(parts)-3])
		title := cleanMediaTitle(parts[len(parts)-2])
		return author, firstNonEmpty(title, fallbackTitle)
	}
	if len(parts) >= 2 {
		return "", cleanMediaTitle(parts[len(parts)-2])
	}
	return "", fallbackTitle
}

func artistTitleFromPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 2 {
		return cleanMediaTitle(parts[len(parts)-3])
	}
	return "Unknown Artist"
}
