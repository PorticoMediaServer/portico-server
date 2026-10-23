package catalog

import (
	"portico.local/server/internal/catalogtest"
	"testing"
)

func TestReadPrerequisitesLibraryProducer(t *testing.T) {
	c := catalogtest.Open(t)
	for _, kind := range []string{"movie", "tv", "anime", "music", "audiobook", "book", "comic"} {
		c.Library(kind, kind, kind, "/"+kind)
	}
	c.Drain()
	s := New(c.DB)
	all, err := s.Libraries()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 7 {
		t.Fatalf("libraries: %d", len(all))
	}
	for _, library := range all {
		want := LibraryCapabilities{TV: true, CardShape: "poster"}
		switch library.Kind {
		case "music", "audiobook":
			want.CardShape = "square"
		case "book", "comic":
			want.TV, want.CardShape = false, "portrait"
		}
		if library.Capabilities == nil || *library.Capabilities != want {
			t.Fatalf("%s capabilities: %+v, want %+v", library.Kind, library.Capabilities, want)
		}
		// The single-library browse producer must publish the same facts as
		// the directory, even when this kind has no executable browse pivot.
		one, err := s.library(library.ID)
		if err != nil || one.Capabilities == nil || *one.Capabilities != want {
			t.Fatalf("single library: %+v %v", one, err)
		}
	}
	unknown := capabilitiesForLibrary("future")
	if unknown.TV || unknown.Reading {
		t.Fatal("unknown media advertised a capability")
	}
}
