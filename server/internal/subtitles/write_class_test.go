package subtitles

import (
	"context"
	"testing"

	"portico.local/server/internal/dbwork"
)

// A viewer's own subtitle request waits for the write gate in the class of
// its request, not behind bulk catalogue work; work that names no class (the
// scan's import) stays background.
func TestSubtitleRequestsTakeTheWriteGateInTheirOwnClass(t *testing.T) {
	f := newSubtitleFixture(t)
	acquired := func(class dbwork.Class) uint64 {
		return dbwork.WriteGate().Stats().ByClass[class.String()].Acquired
	}
	for _, class := range []dbwork.Class{dbwork.ClassEstablishedPlayback, dbwork.ClassInteractive} {
		own, background := acquired(class), acquired(dbwork.ClassBackgroundMedia)
		if _, err := f.s.List(dbwork.WithClass(context.Background(), class), f.p, f.item.Public); err != nil {
			t.Fatal(err)
		}
		if acquired(class) != own+1 || acquired(dbwork.ClassBackgroundMedia) != background {
			t.Fatalf("a %s request took the gate as %s %d time(s) and as background %d time(s)", class, class, acquired(class)-own, acquired(dbwork.ClassBackgroundMedia)-background)
		}
	}
	background := acquired(dbwork.ClassBackgroundMedia)
	if _, err := f.s.List(context.Background(), f.p, f.item.Public); err != nil {
		t.Fatal(err)
	}
	if acquired(dbwork.ClassBackgroundMedia) != background+1 {
		t.Fatal("work that names no class no longer takes the gate as background")
	}
}
