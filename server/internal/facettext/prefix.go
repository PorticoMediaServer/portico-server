package facettext

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

const IndexedSearchPrefixRunes = 6

type SearchPrefix struct {
	Prefix string `json:"prefix"`
	Tier   int    `json:"tier"`
}

// SearchPrefixes indexes the short prefixes typed most often. Longer words and
// CJK retain the full-text index. Each document contributes once per prefix.
func SearchPrefixes(document, title string) string {
	prefixes := map[string]int{}
	titleTerms := map[string]bool{}
	for _, word := range strings.Fields(SearchDocument(title)) {
		titleTerms[word] = true
	}
	for _, word := range strings.Fields(document) {
		if strings.ContainsFunc(word, CJK) {
			continue
		}
		runes := []rune(word)
		for n := 1; n <= min(len(runes), IndexedSearchPrefixRunes); n++ {
			prefix := string(runes[:n])
			tier := 0
			if titleTerms[word] {
				tier = 1
			}
			if strings.HasPrefix(title, prefix) {
				tier = 2
			}
			if title == prefix {
				tier = 3
			}
			if old, ok := prefixes[prefix]; !ok || tier > old {
				prefixes[prefix] = tier
			}
		}
	}
	out := make([]SearchPrefix, 0, len(prefixes))
	for p, tier := range prefixes {
		out = append(out, SearchPrefix{p, tier})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Prefix < out[j].Prefix })
	raw, _ := json.Marshal(out)
	return string(raw)
}

func IndexedSearchTerms(q string) ([]string, bool) {
	words := strings.Fields(SearchFold(q))
	for _, word := range words {
		if utf8.RuneCountInString(word) > IndexedSearchPrefixRunes || strings.ContainsFunc(word, CJK) {
			return nil, false
		}
	}
	return words, len(words) > 0
}
