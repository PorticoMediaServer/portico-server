package httpapi

import (
	"os"
	"strings"
	"testing"
)

// BE-API-12: the published document contradicted the server (plural library
// kinds, blanket "clamps"). The coverage test only checks paths, so enum and
// wording drift escaped. This pins the fixed lines as text, the same way the
// coverage test reads the document.
func TestOpenAPILibraryKindsMatchServer(t *testing.T) {
	raw, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	// The server's real enum (catalog.go: Create allows movie, tv, anime,
	// music, audiobook). The Library schema must publish exactly it.
	if !strings.Contains(doc, "kind: { type: string, enum: [movie, tv, anime, music, audiobook] }") {
		t.Error("Library kind enum does not match the server's movie/tv/anime/music/audiobook")
	}
	if strings.Contains(doc, "enum: [movies, shows, music, books]") {
		t.Error("stale plural library-kind enum is back")
	}
	if strings.Contains(doc, "(`movies`, `shows`,") {
		t.Error("stale plural library kinds in the list-libraries description")
	}
	if strings.Contains(doc, "The server clamps it to its own maximum.") {
		t.Error("blanket clamps wording is back; page-size behaviour varies by surface")
	}
}
