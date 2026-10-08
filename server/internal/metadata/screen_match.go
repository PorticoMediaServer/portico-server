package metadata

import (
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"portico.local/server/internal/metadataprovider"
)

var screenYearPattern = regexp.MustCompile(`(?:^|[ ._\[(])((?:18|19|20)\d{2})(?:[ ._\])]|$)`)
var screenReleasePattern = regexp.MustCompile(`(?i)(?:^|[ ._\[(])(?:480p|576p|720p|1080[pi]|2160p|4320p|4k|8k|blu[ ._-]?ray|bdrip|brrip|web[ ._-]?dl|webrip|hdtv|dvdrip|remux|x26[45]|h[ .]?26[45]|hevc|av1|aac|dts|truehd|ddp|hdr10?\+?|dolby[ .]?vision)(?:\b|_)`)
var screenEpisodePattern = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:s\d{1,2}e\d{1,4}|\d{1,2}x\d{1,4}|-\s*\d{1,4}(?:v\d)?)(?:\b|_)`)
var screenGroupPattern = regexp.MustCompile(`^\[[^\]]{1,128}\]\s*`)
var screenEditionPattern = regexp.MustCompile(`(?i)\{edition-([^}]{1,128})\}|\b(director'?s[ ._]cut|extended[ ._](?:cut|edition)|theatrical[ ._](?:cut|edition)|unrated|remastered|special[ ._]edition)\b`)
var screenIDPattern = regexp.MustCompile(`(?i)[\[{](tmdb|tvdb|anilist)(?:id)?[-=: ]([1-9][0-9]{0,14})[\]}]`)

func intString(v int64) string { return strconv.FormatInt(v, 10) }
func normalizedScreenName(s string) string {
	var out strings.Builder
	for _, r := range norm.NFKD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

// Cleanup never writes back to the source or collapses editions/physical assets.
func screenFilename(s string) (title string, year int, edition string) {
	ext := strings.ToLower(filepath.Ext(s))
	switch ext {
	case ".mkv", ".mp4", ".m4v", ".avi", ".mov", ".ts", ".m2ts", ".webm", ".strm":
		s = strings.TrimSuffix(s, filepath.Ext(s))
	}
	if m := screenEditionPattern.FindStringSubmatch(s); len(m) > 0 {
		edition = m[1]
		if edition == "" {
			edition = m[2]
		}
		s = screenEditionPattern.ReplaceAllString(s, " ")
	}
	s = screenIDPattern.ReplaceAllString(s, " ")
	s = screenGroupPattern.ReplaceAllString(s, "")
	if m := screenYearPattern.FindStringSubmatchIndex(s); len(m) > 0 {
		year, _ = strconv.Atoi(s[m[2]:m[3]])
		s = s[:m[0]]
	}
	if pos := screenEpisodePattern.FindStringIndex(s); len(pos) > 0 {
		s = s[:pos[0]]
	}
	if pos := screenReleasePattern.FindStringIndex(s); len(pos) > 0 {
		s = s[:pos[0]]
	}
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	return strings.Trim(strings.Join(strings.Fields(s), " "), " -[]()"), year, strings.ReplaceAll(edition, "_", " ")
}
func screenQueries(b screenBase) ([]string, int, []metadataprovider.ScreenID) {
	out := []string{}
	seen := map[string]bool{}
	ids := []metadataprovider.ScreenID{}
	idseen := map[metadataprovider.ScreenID]bool{}
	year := b.Year
	add := func(s string) {
		s = strings.TrimSpace(s)
		key := normalizedScreenName(s)
		if len(s) > 0 && len(s) <= 512 && screenQuerySafe(s) && !seen[key] && len(out) < 4 {
			out = append(out, s)
			seen[key] = true
		}
	}
	addID := func(id metadataprovider.ScreenID) {
		if metadataprovider.ValidScreenID(id) && !idseen[id] {
			ids = append(ids, id)
			idseen[id] = true
		}
	}
	if b.Query != "" {
		add(b.Query)
		return out, year, ids
	}
	for _, n := range b.NFO {
		add(n.Title)
		add(n.OriginalTitle)
		if n.Year > 0 {
			if year == 0 {
				year = n.Year
			}
		}
		for _, id := range n.IDs {
			addID(id)
		}
	}
	for _, name := range append([]string{b.Title}, b.Names...) {
		title, y, _ := screenFilename(name)
		if year == 0 && y > 0 {
			year = y
		}
		add(title)
		for _, m := range screenIDPattern.FindAllStringSubmatch(name, 8) {
			kind := b.ItemKind
			if kind == "episode" {
				continue
			}
			if strings.EqualFold(m[1], "anilist") {
				kind = "anime"
			}
			addID(metadataprovider.ScreenID{Provider: strings.ToLower(m[1]), Type: kind, ID: m[2]})
		}
	}
	return out, year, ids
}

type screenCandidate struct {
	DetailsVerified bool                          `json:"-"`
	Key             string                        `json:"key"`
	Record          metadataprovider.ScreenRecord `json:"record"`
	Confidence      float64                       `json:"confidence"`
	Reasons         []string                      `json:"reasons"`
	StrongSignals   int                           `json:"strongSignals"`
	Contradiction   bool                          `json:"contradiction"`
	SourceKind      string                        `json:"sourceKind"`
	ObservedAt      string                        `json:"observedAt"`
	SourceURL       string                        `json:"sourceUrl"`
	Attribution     string                        `json:"attribution"`
	InputDigest     string                        `json:"-"`
	QueryDigest     string                        `json:"-"`
}

func nameSimilarity(a, b string) float64 {
	if a == b && a != "" {
		return 1
	}
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 || len(br) == 0 || len(ar) > 512 || len(br) > 512 {
		return 0
	}
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, x := range ar {
		cur := make([]int, len(br)+1)
		cur[0] = i + 1
		for j, y := range br {
			cost := 1
			if x == y {
				cost = 0
			}
			cur[j+1] = min(cur[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = cur
	}
	return 1 - float64(prev[len(br)])/float64(max(len(ar), len(br)))
}
func scoreScreen(r metadataprovider.ScreenRecord, names []string, year int, exact bool) screenCandidate {
	c := screenCandidate{Record: r, Reasons: []string{}, SourceKind: "provider_query", SourceURL: metadataprovider.ScreenSourceURL(r.Identity), Attribution: metadataprovider.ScreenAttribution(r.Identity.Provider)}
	if exact {
		c.DetailsVerified = true
		c.Confidence = 1
		c.StrongSignals = 2
		c.Reasons = []string{"validated_typed_identifier"}
		c.SourceKind = "typed_identifier"
		return c
	}
	best := 0.0
	for _, q := range names {
		for _, name := range append([]string{r.Title, r.OriginalTitle}, r.Aliases...) {
			best = math.Max(best, nameSimilarity(normalizedScreenName(q), normalizedScreenName(name)))
		}
	}
	if best == 1 {
		c.Confidence = .64
		c.StrongSignals++
		c.Reasons = append(c.Reasons, "exact_title_or_alias")
	} else if best >= .92 {
		c.Confidence = .58
		c.StrongSignals++
		c.Reasons = append(c.Reasons, "close_title_or_alias")
	} else {
		c.Confidence = .5 * best
		c.Reasons = append(c.Reasons, "weak_title_similarity")
	}
	if year > 0 && r.Year > 0 {
		if year == r.Year {
			c.Confidence += .28
			c.StrongSignals++
			c.Reasons = append(c.Reasons, "release_year_agrees")
		} else {
			c.Contradiction = true
			c.Reasons = append(c.Reasons, "release_year_conflicts")
		}
	} else {
		c.Reasons = append(c.Reasons, "release_year_unknown")
	}
	c.Confidence = math.Round(c.Confidence*1000) / 1000
	return c
}
func sameScreenIdentity(a, b metadataprovider.ScreenRecord) bool {
	if a.Identity == b.Identity {
		return true
	}
	for _, id := range a.Crosswalk {
		if id == b.Identity {
			return true
		}
	}
	for _, id := range b.Crosswalk {
		if id == a.Identity {
			return true
		}
	}
	return false
}
func screenWinner(cs []screenCandidate) (int, string) {
	if len(cs) == 0 {
		return -1, "unmatched"
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Confidence == cs[j].Confidence {
			return screenDigest(cs[i].Record.Identity) < screenDigest(cs[j].Record.Identity)
		}
		return cs[i].Confidence > cs[j].Confidence
	})
	top := cs[0]
	if !top.DetailsVerified || top.Contradiction || top.StrongSignals < 2 || top.Confidence < .85 {
		return -1, "needs_selection"
	}
	for _, r := range cs[1:] {
		if !sameScreenIdentity(top.Record, r.Record) && top.Confidence-r.Confidence < .12 {
			return -1, "needs_selection"
		}
	}
	return 0, "matched"
}

func screenQuerySafe(s string) bool {
	if strings.Contains(s, "://") || strings.ContainsAny(s, "\\\r\n\x00") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// A remux/edition's many sidecars must agree; a first-row year is not authority.
func screenLocalYearConflict(b screenBase) bool {
	years := map[int]bool{}
	if b.Year > 0 {
		years[b.Year] = true
	}
	for _, n := range b.NFO {
		if n.Year > 0 {
			years[n.Year] = true
		}
	}
	for _, name := range append([]string{b.Title}, b.Names...) {
		_, y, _ := screenFilename(name)
		if y > 0 {
			years[y] = true
		}
	}
	return len(years) > 1
}
