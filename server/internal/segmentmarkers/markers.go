// Package segmentmarkers holds the viewer-facing shape of detected media
// segments (intro, recap, credits, commercial, outro) and the single server-side
// rule that decides whether a client may skip one without asking. Clients never
// infer skip safety from confidence or provenance: they read automaticSafe.
//
// This package is deliberately a leaf. Analysis owns detection and storage,
// catalog owns item detail and playback owns offers; all three publish this
// same shape.
package segmentmarkers

import "strings"

// Marker is one projected segment on the viewer clock of the played source.
// Positions are seconds from the start of the logical item, never file offsets.
type Marker struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	StartSeconds  float64 `json:"startSeconds"`
	EndSeconds    float64 `json:"endSeconds"`
	AutomaticSafe bool    `json:"automaticSafe"`
}

// Set fences a marker projection to the exact source and marker revision it was
// read from, the way chapters are fenced. A client that has moved to another
// source or missed an owner edit must refetch rather than seek on stale facts.
type Set struct {
	SourceID string   `json:"sourceId"`
	Revision string   `json:"revision"`
	Markers  []Marker `json:"markers"`
}

// ViewerKind reports the kinds a viewer client may act on. Owner-only kinds
// (for example a manual "chapter" marker) never reach the viewer path.
func ViewerKind(kind string) bool {
	switch kind {
	case "intro", "recap", "credits", "commercial", "outro":
		return true
	}
	return false
}

// AutomaticConfidence is the measured-confidence floor an unapproved detection
// must clear before the server will authorize an unattended skip.
const AutomaticConfidence = 0.85

// DetectorProvenance reports whether a provenance string names a measured
// detector. Owner-authored markers carry their own approval, and a marker
// derived from an embedded chapter *title* is a text heuristic about someone
// else's labelling, not a measurement of the media; neither may authorize an
// unattended skip on confidence alone. A detector that recorded its own
// semantic ambiguity is excluded for the same reason.
func DetectorProvenance(provenance string) bool {
	if provenance == "" || strings.HasPrefix(provenance, "owner:") {
		return false
	}
	if strings.HasPrefix(provenance, "embedded_chapter_label") {
		return false
	}
	return !strings.Contains(provenance, "semantic_ambiguous")
}

// AutomaticSafe is the whole skip policy. An owner-approved marker is safe
// because a person accepted it. An unapproved marker is safe only when a
// measured detector produced it with confidence at or above the floor.
// Dismissed markers are never projected and so never reach this function.
func AutomaticSafe(approved bool, confidence float64, provenance string) bool {
	if approved {
		return true
	}
	return confidence >= AutomaticConfidence && DetectorProvenance(provenance)
}
