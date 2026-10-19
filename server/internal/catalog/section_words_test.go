package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Clients show a section's catalogue message (`section.<id>`) in the viewer's language when the
// catalogue has one, else the server's English. A message must say what the server says, and an id
// the server titles in more than one way (or after a library or playlist) must have none.
func TestSectionWordsMatchTheCatalogue(t *testing.T) {
	raw, err := os.ReadFile("../../../packages/i18n/src/catalog/en-US.ts")
	if err != nil {
		t.Fatal(err)
	}
	catalogue := map[string]string{}
	for _, m := range regexp.MustCompile(`'section\.([A-Za-z_]+)': '([^']*)'`).FindAllStringSubmatch(string(raw), -1) {
		catalogue[m[1]] = m[2]
	}
	if len(catalogue) == 0 {
		t.Fatal("no section messages found in the catalogue")
	}
	titles := map[string]map[string]bool{}
	note := func(id, title string) {
		if titles[id] == nil {
			titles[id] = map[string]bool{}
		}
		titles[id][title] = true
	}
	literal := regexp.MustCompile(`(?:contentSection|gridSection)\("([A-Za-z_]+)", (?:"[a-z]+", )?"([^"]+)"`)
	listening := regexp.MustCompile(`listeningSection\{"([A-Za-z_]+)", "([^"]+)"`)
	assigned := regexp.MustCompile(`id, kind, title = "([A-Za-z_]+)", "[a-z]+", "([^"]+)"|id := "([A-Za-z_]+)"\s+kind := "[a-z]+"\s+title := "([^"]+)"`)
	files, _ := filepath.Glob("*.go")
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, _ := os.ReadFile(file)
		for _, m := range literal.FindAllStringSubmatch(string(source), -1) {
			note(m[1], m[2])
		}
		for _, m := range listening.FindAllStringSubmatch(string(source), -1) {
			note(m[1], m[2])
		}
		for _, m := range assigned.FindAllStringSubmatch(string(source), -1) {
			if m[1] != "" {
				note(m[1], m[2])
			} else {
				note(m[3], m[4])
			}
		}
	}
	for id, text := range catalogue {
		seen := titles[id]
		if len(seen) == 0 {
			t.Errorf("section.%s has no fixed server title; remove it from the catalogue", id)
			continue
		}
		if len(seen) > 1 || !seen[text] {
			t.Errorf("section.%s is %q in the catalogue but the server titles it %v", id, text, seen)
		}
	}
	for _, id := range []string{"items", "entries", "files", "releases"} {
		if _, ok := catalogue[id]; ok {
			t.Errorf("section.%s takes a library's or playlist's name, or differs by page: it must not have a catalogue message", id)
		}
	}
}
