package remotemedia

import (
	"errors"
	"testing"
)

func TestObservationNeverSubstitutesServedETagForProviderEvidence(t *testing.T) {
	o := FileObservation{SourceID: "source", Incarnation: "1", Object: "film.mkv", Size: 10, ModifiedKnown: true, ServeETag: `"served-mtime-size"`}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if o.ProviderETag != "" || o.ProviderHash != "" {
		t.Fatal("invented provider evidence")
	}
	next := o
	next.ProviderETag = `"new-provider-observation"`
	if !errors.Is(o.Check(next), ErrObservationChanged) {
		t.Fatal("validator availability change did not invalidate")
	}
	next = o
	next.ModifiedKnown = false
	if !errors.Is(o.Check(next), ErrObservationChanged) {
		t.Fatal("missing metadata accepted")
	}
	// Equal observable facts do not detect a provider that changed bytes while
	// reusing all metadata. This type deliberately provides no immutable Version.
	if err := o.Check(o); err != nil {
		t.Fatal(err)
	}
}
