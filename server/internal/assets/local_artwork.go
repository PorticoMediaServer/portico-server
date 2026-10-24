package assets

// LocalArtworkCandidate is an actual decoded, admitted sidecar image. Slot is
// the existing item/poster, show/backdrop or season/N/poster catalogue slot;
// Key is the local content-addressed image key. Neither is a path or a grant.
type LocalArtworkCandidate struct {
	Slot string
	Key  string
}
