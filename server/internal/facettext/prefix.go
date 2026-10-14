package facettext

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const IndexedSearchPrefixRunes = 12
const NumericSearchPrefixRunes = 6

type SearchPrefix struct {
	Prefix   string `json:"prefix"`
	Tier     int    `json:"tier"`
	Terminal int    `json:"terminal"`
}

func NumericSearchTerm(word string) bool {
	return word != "" && !strings.ContainsFunc(word, func(r rune) bool { return !unicode.IsNumber(r) })
}

// Text prefixes have direct postings. Numeric words share their six-character
// terminal key: short numeric queries expand a bounded dictionary range rather
// than storing six identical copies of long padded numbers in every document.
func SearchPrefixes(document, title string) string {
	prefixes := map[string]SearchPrefix{}
	titleTerms := map[string]bool{}
	for _, word := range strings.Fields(SearchDocument(title)) {
		titleTerms[word] = true
	}
	for _, word := range strings.Fields(document) {
		if strings.ContainsFunc(word, CJK) {
			continue
		}
		runes := []rune(word)
		last := min(len(runes), IndexedSearchPrefixRunes)
		first, terminal := 1, 0
		if NumericSearchTerm(word) {
			last = min(last, NumericSearchPrefixRunes)
			first, terminal = last, 1
		}
		for n := first; n <= last; n++ {
			prefix := string(runes[:n])
			tier := 0
			if titleTerms[word] {
				tier = 1
			}
			if strings.HasPrefix(title, prefix) {
				tier = 2
			}
			key := string(rune(terminal)) + prefix
			if old, ok := prefixes[key]; !ok || tier > old.Tier {
				prefixes[key] = SearchPrefix{prefix, tier, terminal}
			}
		}
	}
	out := make([]SearchPrefix, 0, len(prefixes))
	for _, p := range prefixes {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Prefix == out[j].Prefix {
			return out[i].Terminal < out[j].Terminal
		}
		return out[i].Prefix < out[j].Prefix
	})
	raw, _ := json.Marshal(out)
	return string(raw)
}

func IndexedSearchTerms(q string) ([]string, bool) {
	words := strings.Fields(SearchFold(q))
	for _, word := range words {
		maximum := IndexedSearchPrefixRunes
		if NumericSearchTerm(word) {
			maximum = NumericSearchPrefixRunes
		}
		if utf8.RuneCountInString(word) > maximum || strings.ContainsFunc(word, CJK) {
			return nil, false
		}
	}
	return words, len(words) > 0
}
