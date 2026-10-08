// Package sorttext owns deterministic catalog ordering, independent of host locale.
package sorttext

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"strings"
	"unicode"
)

var articles = map[string][]string{
	"en": {"the ", "an ", "a "},
	"fr": {"les ", "une ", "des ", "le ", "la ", "un ", "l'", "l’"},
	"es": {"los ", "las ", "una ", "unos ", "unas ", "el ", "la ", "un "},
	"de": {"der ", "die ", "das ", "ein ", "eine "},
	"it": {"gli ", "uno ", "una ", "il ", "lo ", "la ", "le ", "un ", "i ", "l'", "l’"},
	"pt": {"uma ", "uns ", "umas ", "os ", "as ", "um ", "o ", "a "},
	"nl": {"een ", "het ", "de "},
}

func Fold(value string) string {
	value = cases.Fold().String(strings.TrimSpace(value))
	value = norm.NFD.String(value)
	var b strings.Builder
	for _, r := range value {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return norm.NFC.String(b.String())
}

// Key honors an explicit sort title exactly (apart from Unicode folding).
// Unknown metadata languages do not guess which words are articles.
func Key(title, explicit, language string) string {
	if strings.TrimSpace(explicit) != "" {
		return Fold(explicit)
	}
	title = strings.TrimSpace(title)
	base := strings.ToLower(strings.SplitN(language, "-", 2)[0])
	// No metadata language (music and audiobook entities, local-only libraries) sorts as the
	// product's source language, so "The Beatles" files under B like "The Avengers" does.
	if base == "" {
		base = "en"
	}
	lower := strings.ToLower(title)
	for _, article := range articles[base] {
		if strings.HasPrefix(lower, article) && len(title) > len(article) {
			title = title[len(article):]
			break
		}
	}
	return Fold(title)
}
func Letter(value string) string {
	for _, r := range Fold(value) {
		if unicode.IsLetter(r) {
			return string(unicode.ToUpper(r))
		}
		return "#"
	}
	return "#"
}
