package identity

import (
	"strings"
	"unicode"
)

// Age ratings arrive from providers and NFO files as free text ("PG-13", "TV-14",
// "12A", "US:R", "de/FSK 16"). A profile restriction is expressed in one rating
// system, but a library mixes systems, so every rating is normalised to a single
// ordered scale: the minimum admission age the issuing body attaches to it.
//
// Comparing minimum ages is the only cross-system comparison this server makes.
// It is deliberately coarse and deliberately conservative: a rating the table does
// not know is treated as unrated, and unrated is governed by AllowUnrated rather
// than being silently admitted.

// RatingValue is one entry of one rating system.
type RatingValue struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	MinimumAge int    `json:"minimumAge"`
}

// RatingSystem is an ordered table of the values one body issues, ascending.
type RatingSystem struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Region  string        `json:"region"`
	Values  []RatingValue `json:"values"`
	Screens []string      `json:"screens"`
}

var ratingSystems = []RatingSystem{
	{ID: "mpaa", Name: "MPA (United States film)", Region: "US", Screens: []string{"movie"}, Values: []RatingValue{
		{Code: "G", Label: "G — General audiences", MinimumAge: 0},
		{Code: "PG", Label: "PG — Parental guidance suggested", MinimumAge: 8},
		{Code: "PG-13", Label: "PG-13 — Parents strongly cautioned", MinimumAge: 13},
		{Code: "R", Label: "R — Restricted", MinimumAge: 17},
		{Code: "NC-17", Label: "NC-17 — Adults only", MinimumAge: 18},
	}},
	{ID: "us-tv", Name: "US TV Parental Guidelines", Region: "US", Screens: []string{"show", "season", "episode"}, Values: []RatingValue{
		{Code: "TV-Y", Label: "TV-Y — All children", MinimumAge: 0},
		// CD-03: ascending by admission age, as the table promises; TV-Y7 used
		// to precede TV-G, and clients refused the whole ladder as out of order.
		{Code: "TV-G", Label: "TV-G — General audience", MinimumAge: 0},
		{Code: "TV-Y7", Label: "TV-Y7 — Older children", MinimumAge: 7},
		{Code: "TV-PG", Label: "TV-PG — Parental guidance", MinimumAge: 8},
		{Code: "TV-14", Label: "TV-14 — Parents strongly cautioned", MinimumAge: 14},
		{Code: "TV-MA", Label: "TV-MA — Mature audience", MinimumAge: 17},
	}},
	{ID: "bbfc", Name: "BBFC (United Kingdom)", Region: "GB", Screens: []string{"movie", "show", "season", "episode"}, Values: []RatingValue{
		{Code: "U", Label: "U — Universal", MinimumAge: 0},
		{Code: "PG", Label: "PG — Parental guidance", MinimumAge: 8},
		{Code: "12A", Label: "12A — Cinema, under 12 accompanied", MinimumAge: 12},
		{Code: "12", Label: "12 — Suitable for 12 years and over", MinimumAge: 12},
		{Code: "15", Label: "15 — Suitable only for 15 years and over", MinimumAge: 15},
		{Code: "18", Label: "18 — Suitable only for adults", MinimumAge: 18},
	}},
	{ID: "fsk", Name: "FSK (Germany)", Region: "DE", Screens: []string{"movie", "show", "season", "episode"}, Values: []RatingValue{
		{Code: "0", Label: "FSK 0", MinimumAge: 0},
		{Code: "6", Label: "FSK 6", MinimumAge: 6},
		{Code: "12", Label: "FSK 12", MinimumAge: 12},
		{Code: "16", Label: "FSK 16", MinimumAge: 16},
		{Code: "18", Label: "FSK 18", MinimumAge: 18},
	}},
	{ID: "cnc", Name: "CNC (France)", Region: "FR", Screens: []string{"movie", "show", "season", "episode"}, Values: []RatingValue{
		{Code: "TP", Label: "Tous publics", MinimumAge: 0},
		{Code: "-10", Label: "Interdit aux moins de 10 ans", MinimumAge: 10},
		{Code: "-12", Label: "Interdit aux moins de 12 ans", MinimumAge: 12},
		{Code: "-16", Label: "Interdit aux moins de 16 ans", MinimumAge: 16},
		{Code: "-18", Label: "Interdit aux moins de 18 ans", MinimumAge: 18},
	}},
	{ID: "acb", Name: "ACB (Australia)", Region: "AU", Screens: []string{"movie", "show", "season", "episode"}, Values: []RatingValue{
		{Code: "G", Label: "G — General", MinimumAge: 0},
		{Code: "PG", Label: "PG — Parental guidance", MinimumAge: 8},
		{Code: "M", Label: "M — Mature", MinimumAge: 15},
		{Code: "MA15+", Label: "MA 15+ — Mature accompanied", MinimumAge: 15},
		{Code: "R18+", Label: "R 18+ — Restricted", MinimumAge: 18},
		{Code: "X18+", Label: "X 18+ — Restricted", MinimumAge: 18},
	}},
	{ID: "age", Name: "Minimum age", Region: "", Screens: []string{"movie", "show", "season", "episode"}, Values: []RatingValue{
		{Code: "0+", Label: "All ages", MinimumAge: 0},
		{Code: "6+", Label: "6 and over", MinimumAge: 6},
		{Code: "12+", Label: "12 and over", MinimumAge: 12},
		{Code: "16+", Label: "16 and over", MinimumAge: 16},
		{Code: "18+", Label: "18 and over", MinimumAge: 18},
	}},
}

// RatingSystems returns the ordered table clients render when a viewer picks a
// maximum rating. The order inside a system is ascending by admission age.
func RatingSystems() []RatingSystem {
	out := make([]RatingSystem, len(ratingSystems))
	copy(out, ratingSystems)
	return out
}

// ratingKey folds a raw rating to its comparison key: upper case, with only
// letters, digits and the +/- the bodies use, and without a leading "RATED".
func ratingKey(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '-' {
			b.WriteRune(r)
		}
	}
	return strings.TrimPrefix(b.String(), "RATED")
}

// ratingKeys yields the keys to try for one raw rating, most specific first. A
// provider may qualify a rating by region ("US:PG-13", "de/FSK 16"), so the
// qualifier is stripped as a fallback — but only as a fallback, because a bare
// spelling like "N/A" would otherwise be read as its own last segment and, in
// that example, resolve to an adults-only rating instead of "unrated".
func ratingKeys(raw string) []string {
	keys := []string{ratingKey(raw)}
	if cut := strings.LastIndexAny(raw, ":/"); cut >= 0 && cut+1 < len(raw) {
		if stripped := ratingKey(raw[cut+1:]); stripped != "" && stripped != keys[0] {
			keys = append(keys, stripped)
		}
	}
	return keys
}

var ratingIndex = func() map[string]int {
	index := map[string]int{}
	for _, system := range ratingSystems {
		for _, value := range system.Values {
			key := ratingKey(value.Code)
			if age, seen := index[key]; !seen || value.MinimumAge > age {
				index[key] = value.MinimumAge
			}
		}
	}
	// Spellings providers emit that are not a body's own code.
	for key, age := range map[string]int{
		"NR": -1, "UNRATED": -1, "NOTRATED": -1, "UR": -1, "NA": -1, "NONE": -1,
		"TVY": 0, "TVY7FV": 7, "ALLAGES": 0, "E": 0, "EC": 0, "KA": 0, "A": 18, "AO": 18,
		"FSK0": 0, "FSK6": 6, "FSK12": 12, "FSK16": 16, "FSK18": 18,
		"M/PG": 8, "GP": 8, "M15+": 15, "R21": 18, "X": 18, "XXX": 18,
	} {
		index[key] = age
	}
	return index
}()

// RatingAge maps one raw rating string to the minimum admission age it carries.
// The second result is false when the string is empty, unknown, or explicitly a
// "not rated" spelling; such content is governed by AllowUnrated.
func RatingAge(raw string) (int, bool) {
	for _, key := range ratingKeys(raw) {
		if key == "" {
			continue
		}
		if age, ok := ratingIndex[key]; ok {
			if age < 0 {
				return 0, false
			}
			return age, true
		}
		// A bare number ("12", "16", "-12", "16+") is the minimum age in most
		// European systems.
		if age, ok := bareAge(key); ok {
			return age, true
		}
	}
	return 0, false
}

func bareAge(key string) (int, bool) {
	digits := strings.TrimSuffix(strings.TrimPrefix(key, "-"), "+")
	if digits == "" {
		return 0, false
	}
	age := 0
	for _, r := range digits {
		if r < '0' || r > '9' {
			return 0, false
		}
		age = age*10 + int(r-'0')
		if age > 21 {
			return 0, false
		}
	}
	return age, true
}

// RatingCeiling resolves the profile's chosen maximum into an admission age.
// An empty system or code means "no ceiling" and returns -1.
func RatingCeiling(system, code string) (int, bool) {
	if strings.TrimSpace(code) == "" {
		return -1, true
	}
	for _, candidate := range ratingSystems {
		if candidate.ID != system {
			continue
		}
		for _, value := range candidate.Values {
			if value.Code == code {
				return value.MinimumAge, true
			}
		}
		return 0, false
	}
	return 0, false
}

// KnownRatingKeys returns every normalised key whose admission age exceeds the
// ceiling. The catalog predicate uses it rather than evaluating Go in SQL: the
// vocabulary is small, fixed at build time and already indexed by value_key.
func KnownRatingKeys(ceiling int) []string {
	if ceiling < 0 {
		return nil
	}
	out := []string{}
	for key, age := range ratingIndex {
		if age > ceiling {
			out = append(out, key)
		}
	}
	return out
}
