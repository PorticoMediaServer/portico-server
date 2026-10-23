package remotemedia

import (
	"errors"
	"strings"
)

var ErrObservationChanged = errors.New("remote file observation changed; stop and rescan")
var ErrObservation = errors.New("remote file observation is incomplete")

// FileObservation records the scan facts, scoped independently of validators.
// Provider validators must come from fresh provider metadata, never an rclone
// served ETag. Empty provider fields explicitly mean the provider supplied none.
// Equality detects observable changes; it does not establish immutable bytes.
type FileObservation struct {
	SourceID, Incarnation, Root, Object string
	Size, ModifiedNS                    int64
	ModifiedKnown                       bool
	ProviderHashKind, ProviderHash      string
	ProviderETag                        string
	ServeETag                           string
}

func (o FileObservation) Validate() error {
	if o.SourceID == "" || o.Incarnation == "" || o.Object == "" || !cleanDAVRelative(o.Root) || !cleanDAVRelative(o.Object) || strings.Contains(o.Root+o.Object, ":") || o.Size < 0 || !o.ModifiedKnown || !serveETag(o.ServeETag) {
		return ErrObservation
	}
	if (o.ProviderHashKind == "") != (o.ProviderHash == "") {
		return ErrObservation
	}
	for _, v := range []string{o.SourceID, o.Incarnation, o.ProviderHashKind, o.ProviderHash, o.ProviderETag} {
		if len(v) > 4096 || strings.ContainsAny(v, "\x00\r\n") {
			return ErrObservation
		}
	}
	return nil
}

func (o FileObservation) Check(next FileObservation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if next.Validate() != nil || next != o {
		return ErrObservationChanged
	}
	return nil
}
