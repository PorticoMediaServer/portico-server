package facettext

import (
	"portico.local/server/internal/sorttext"
	"strings"
	"unicode"
)

// SearchFold preserves word boundaries and removes punctuation within words.
// Thus Spider-Man and spiderman share a token, while WALL·E matches wall e
// through the component tokens supplied by SearchDocument.
func SearchFold(value string) string {
	var b strings.Builder
	for _, r := range sorttext.Fold(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func CJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// Thai runs without spaces between words, like Chinese: a query is a substring
// of a phrase, not a space-separated word.
func Thai(r rune) bool {
	return unicode.In(r, unicode.Thai)
}

// Subword reports scripts with no word spaces. Their queries are indexed as
// bigrams and verified as substrings rather than matched as whole words. The
// prolonged sound mark joins katakana words (タワー) but Unicode files it
// outside the Katakana script, so it is named here: without it a word
// containing one would index and query as one token and never match inside.
func Subword(r rune) bool {
	return CJK(r) || Thai(r) || r == 'ー'
}

// SearchDocument adds punctuation-separated words and CJK unigrams/bigrams.
// CJK candidates are checked for the actual substring after the indexed seek.
// The expansion is linear in input size; hostile metadata cannot grow it without bound.
func SearchDocument(value string) string {
	if len(value) > 65536 {
		value = value[:65536]
	}
	folded := sorttext.Fold(value)
	words := strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	tokens := append(strings.Fields(SearchFold(folded)), words...)
	for _, word := range words {
		runes := []rune(word)
		for i, r := range runes {
			if !Subword(r) {
				continue
			}
			tokens = append(tokens, string(r))
			if i+1 < len(runes) && Subword(runes[i+1]) {
				tokens = append(tokens, string(runes[i:i+2]))
			}
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if !seen[token] {
			out = append(out, token)
			seen[token] = true
		}
	}
	return strings.Join(out, " ")
}

func SearchTerms(value string) []string {
	words := strings.Fields(SearchFold(value))
	out := []string{}
	for _, word := range words {
		runes := []rune(word)
		allSubword := true
		for _, r := range runes {
			if !Subword(r) {
				allSubword = false
				break
			}
		}
		if allSubword && len(runes) > 1 {
			for i := 0; i+1 < len(runes); i++ {
				out = append(out, string(runes[i:i+2]))
			}
		} else {
			out = append(out, word)
		}
	}
	return out
}
