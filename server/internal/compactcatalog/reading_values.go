package compactcatalog

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Reading values are library-scoped attributions, never people or audio Author
// entities. Source rules consume accepted inventory paths; they do no file I/O
// and cannot authorize a scan, archive read or publication.
type ReadingValueSource string

const (
	ReadingFileName     ReadingValueSource = "file-name"
	ReadingFolder       ReadingValueSource = "folder"
	ReadingOwner        ReadingValueSource = "owner"
	ReadingFileMetadata ReadingValueSource = "file-metadata"
)

var ErrReadingValue = errors.New("invalid reading attribution value")

type ReadingValue struct {
	Value  string
	Source ReadingValueSource
}

type ReadingNumber struct {
	Value  float64
	Source ReadingValueSource
}

type ReadingValues struct {
	Title          string
	Author, Series *ReadingValue
	SeriesNumber   *ReadingNumber
}

type ReadingRuleInput struct {
	Item                     int64
	SourceRoot, RelativePath string // Actual folder-source identity, not asset token.
	Kind                     Kind
}

type ReadingOwnerValue struct {
	Value  string
	Locked bool
}

var readingTags = regexp.MustCompile(`\[[^\[\]]*\]|\([^()]*\)`)
var readingMarkedNumber = regexp.MustCompile(`(?i)(?:^|[ _.-])(?:v(?:ol(?:ume)?\.?)?|book|#)\s*([0-9]+(?:\.[0-9]+)?)(?:$|[ _-])`)
var readingDashNumber = regexp.MustCompile(`\s+-\s+([0-9]+(?:\.[0-9]+)?)(?:\s+-\s+|$)`)
var readingOnlyNumber = regexp.MustCompile(`(?i)^(?:(?:v(?:ol(?:ume)?\.?)?|book|#)\s*)?[0-9]+(?:\.[0-9]+)?$`)
var readingStartsNumber = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(?:$|[ _-])`)
var readingDate = regexp.MustCompile(`\b(?:19|20)[0-9]{2}(?:-[0-9]{2}(?:-[0-9]{2})?|[0-9]{4})\b`)
var readingOwnerNumber = regexp.MustCompile(`^[+]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func readingDirectoryKey(in ReadingRuleInput, directory string) string {
	return in.SourceRoot + "\x00" + directory
}

func readingFileAuthor(stem string) (string, string) {
	left, right, ok := strings.Cut(stem, " - ")
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if !ok || left == "" || right == "" || readingOnlyNumber.MatchString(left) || readingStartsNumber.MatchString(right) {
		return "", stem
	}
	return left, right
}

func readingCleanTitle(s string) string {
	s = readingTags.ReplaceAllString(s, " ")
	s = strings.NewReplacer("_", " ", ".", " ").Replace(s)
	return strings.Trim(strings.Join(strings.Fields(s), " "), " -")
}

func readingStripPrefix(value, prefix string) (string, bool) {
	v, p := []rune(value), []rune(prefix)
	if len(v) < len(p) || !strings.EqualFold(string(v[:len(p)]), prefix) {
		return value, false
	}
	if len(v) > len(p) && !strings.ContainsRune(" _.-#0123456789", v[len(p)]) {
		return value, false
	}
	return strings.TrimLeft(string(v[len(p):]), " _.-"), true
}

func readingNumber(stem, series string) (*ReadingNumber, string) {
	for _, pattern := range []*regexp.Regexp{readingMarkedNumber, readingDashNumber} {
		for _, match := range pattern.FindAllStringSubmatchIndex(stem, -1) {
			overlapsDate := false
			for _, date := range readingDate.FindAllStringIndex(stem, -1) {
				if match[2] < date[1] && match[3] > date[0] {
					overlapsDate = true
				}
			}
			if overlapsDate {
				continue
			}
			n, err := strconv.ParseFloat(stem[match[2]:match[3]], 64)
			if err == nil && !math.IsInf(n, 0) {
				prefix := stem[:match[0]]
				if series != "" && strings.EqualFold(readingCleanTitle(prefix), readingCleanTitle(series)) {
					prefix = ""
				}
				return &ReadingNumber{n, ReadingFileName}, prefix + " " + stem[match[1]:]
			}
		}
	}
	if rest, ok := readingStripPrefix(stem, series); ok && series != "" && !readingDate.MatchString(rest) {
		match := readingStartsNumber.FindStringSubmatch(rest)
		if len(match) > 1 {
			n, err := strconv.ParseFloat(match[1], 64)
			if err == nil && !math.IsInf(n, 0) {
				return &ReadingNumber{n, ReadingFileName}, strings.TrimLeft(rest[len(match[0]):], " -")
			}
		}
	}
	return nil, stem
}

// ReadingPathValues applies the documented Author/Series/Title and Author -
// Title shapes. A direct folder with >=2 items is a series unless its author
// role is evidenced by a nested series or a matching filename attribution.
// Ambiguous names are not classified as people by spelling/capitalisation.
func ReadingPathValues(items []ReadingRuleInput) (map[int64]ReadingValues, error) {
	counts, authorFolders, seen := map[string]int{}, map[string]bool{}, map[int64]bool{}
	for _, in := range items {
		if in.Item < 1 || seen[in.Item] || in.SourceRoot == "" || len(in.RelativePath) > 4096 || !utf8.ValidString(in.RelativePath) || path.Clean(in.RelativePath) != in.RelativePath || strings.HasPrefix(in.RelativePath, "/") || strings.ContainsAny(in.RelativePath, "\\\x00") || in.RelativePath == "." || strings.HasPrefix(in.RelativePath, "../") || (in.Kind != Ebook && in.Kind != Comic) {
			return nil, ErrReadingValue
		}
		seen[in.Item] = true
		dir := path.Dir(in.RelativePath)
		counts[readingDirectoryKey(in, dir)]++
		parts := strings.Split(in.RelativePath, "/")
		if len(parts) == 3 {
			authorFolders[readingDirectoryKey(in, parts[0])] = true
		}
		stem := strings.TrimSuffix(path.Base(in.RelativePath), path.Ext(in.RelativePath))
		author, _ := readingFileAuthor(stem)
		if dir != "." && author != "" && localGenreKey(author) == localGenreKey(path.Base(dir)) {
			authorFolders[readingDirectoryKey(in, dir)] = true
		}
	}
	out := make(map[int64]ReadingValues, len(items))
	for _, in := range items {
		dir := path.Dir(in.RelativePath)
		stem := strings.TrimSuffix(path.Base(in.RelativePath), path.Ext(in.RelativePath))
		author, title := readingFileAuthor(stem)
		v := ReadingValues{}
		if author != "" {
			v.Author = &ReadingValue{author, ReadingFileName}
		}
		parts := strings.Split(in.RelativePath, "/")
		if v.Author == nil && len(parts) == 3 {
			v.Author = &ReadingValue{parts[0], ReadingFolder}
		}
		key := readingDirectoryKey(in, dir)
		if dir != "." {
			if authorFolders[key] {
				if v.Author == nil {
					v.Author = &ReadingValue{path.Base(dir), ReadingFolder}
				}
			} else if counts[key] >= 2 {
				v.Series = &ReadingValue{path.Base(dir), ReadingFolder}
			}
		}
		series := ""
		if v.Series != nil {
			series = v.Series.Value
		}
		v.SeriesNumber, title = readingNumber(readingTags.ReplaceAllString(title, " "), series)
		v.Title = readingCleanTitle(title)
		if series != "" && v.SeriesNumber == nil {
			if rest, ok := readingStripPrefix(v.Title, readingCleanTitle(series)); ok && rest != "" {
				v.Title = rest
			}
		}
		if v.Title == "" {
			v.Title = readingCleanTitle(stem)
		}
		if v.Title == "" {
			return nil, ErrReadingValue
		}
		out[in.Item] = v
	}
	return out, nil
}

// ReadingEffectiveValues never mutates the automatic facts or owner rows.
// Empty or unlocked owner fields restore the genuine producer value/absence.
func ReadingEffectiveValues(automatic ReadingValues, owner map[string]ReadingOwnerValue) (ReadingValues, error) {
	out := automatic
	for field, target := range map[string]**ReadingValue{"author": &out.Author, "series": &out.Series} {
		value := owner[field]
		if !value.Locked || strings.TrimSpace(value.Value) == "" {
			continue
		}
		if len(value.Value) > 4096 || !utf8.ValidString(value.Value) || strings.ContainsAny(value.Value, "\x00\r\n") {
			return ReadingValues{}, ErrReadingValue
		}
		*target = &ReadingValue{value.Value, ReadingOwner}
	}
	if value := owner["seriesNumber"]; value.Locked && strings.TrimSpace(value.Value) != "" {
		raw := strings.TrimSpace(value.Value)
		n, err := strconv.ParseFloat(raw, 64)
		if !readingOwnerNumber.MatchString(raw) || err != nil || math.IsInf(n, 0) || math.IsNaN(n) || n < 0 {
			return ReadingValues{}, ErrReadingValue
		}
		out.SeriesNumber = &ReadingNumber{n, ReadingOwner}
	}
	return out, nil
}

func ReadingValueKey(value string) string { return localGenreKey(value) }

func ReadingSeriesID(library, name string) string {
	sum := sha256.Sum256([]byte("portico.reading.series.v1\x00" + library + "\x00" + ReadingValueKey(name)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type ReadingFacetItem struct {
	Item     int64
	TitleKey string // The catalogue's actual title sort key.
	Values   ReadingValues
}

type ReadingValueFacet struct {
	Value string
	Count int
}
type ReadingSeriesFacet struct {
	ID, Name string
	Count    int
}
type ReadingValueFacets struct {
	Authors []ReadingValueFacet
	Series  []ReadingSeriesFacet
}

// ReadingFacets consumes only the caller's already-authorized, library-scoped
// accepted rows. The SQL producer must filter visibility before counts/LIMIT.
func ReadingFacets(library string, items []ReadingFacetItem) ReadingValueFacets {
	type group struct {
		count          int
		labels, owners map[string]int
	}
	authors, series := map[string]*group{}, map[string]*group{}
	add := func(groups map[string]*group, value *ReadingValue) {
		if value == nil {
			return
		}
		key := ReadingValueKey(value.Value)
		if key == "" {
			return
		}
		g := groups[key]
		if g == nil {
			g = &group{labels: map[string]int{}, owners: map[string]int{}}
			groups[key] = g
		}
		g.count++
		g.labels[value.Value]++
		if value.Source == ReadingOwner {
			g.owners[value.Value]++
		}
	}
	for _, item := range items {
		add(authors, item.Values.Author)
		add(series, item.Values.Series)
	}
	label := func(g *group) string {
		counts := g.labels
		if len(g.owners) > 0 {
			counts = g.owners
		}
		best, number := "", -1
		for name, count := range counts {
			if count > number || count == number && name < best {
				best, number = name, count
			}
		}
		return best
	}
	out := ReadingValueFacets{}
	for _, g := range authors {
		out.Authors = append(out.Authors, ReadingValueFacet{label(g), g.count})
	}
	for _, g := range series {
		name := label(g)
		out.Series = append(out.Series, ReadingSeriesFacet{ReadingSeriesID(library, name), name, g.count})
	}
	sort.Slice(out.Authors, func(i, j int) bool {
		return ReadingValueKey(out.Authors[i].Value) < ReadingValueKey(out.Authors[j].Value)
	})
	sort.Slice(out.Series, func(i, j int) bool { return ReadingValueKey(out.Series[i].Name) < ReadingValueKey(out.Series[j].Name) })
	return out
}

func ReadingSeriesItems(library, id string, items []ReadingFacetItem) ([]ReadingFacetItem, bool) {
	var out []ReadingFacetItem
	for _, item := range items {
		if item.Values.Series != nil && ReadingSeriesID(library, item.Values.Series.Value) == id {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Values.SeriesNumber, out[j].Values.SeriesNumber
		if (a == nil) != (b == nil) {
			return a != nil
		}
		if a != nil && a.Value != b.Value {
			return a.Value < b.Value
		}
		if out[i].TitleKey != out[j].TitleKey {
			return out[i].TitleKey < out[j].TitleKey
		}
		return out[i].Item < out[j].Item
	})
	return out, len(out) != 0
}
