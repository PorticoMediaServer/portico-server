package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"

	"portico.local/server/internal/disc"
)

var measuredDiscIdentity = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,256}$`)
var measuredDiscFactsRevision = regexp.MustCompile(`^[a-f0-9]{64}$`)

// MeasuredDiscTitleRef projects metadata from an original sealed Play
// measurement. The caller must retain and reprove its exact borrow or accepted
// presentation before using it. This pure function establishes no authority,
// readiness, input custody, source currentness or grant, and performs no lookup.
//
// The title identity is stable for this physical observation, measured structure
// and selected key. Facts changes change the offer revision. Session, snapshot,
// capture and actor identities deliberately have no place in this metadata
// projection; equal references cannot transfer their independent byte custody.
// Local inventory selections/tokens and ResolveDiscTitleTx remain unchanged.
func MeasuredDiscTitleRef(assetID, objectID, sourceRevision string, structure disc.Probe, selectedKey, factsRevision string) (*DiscTitleRef, error) {
	for _, value := range []string{assetID, objectID, sourceRevision, selectedKey} {
		if !measuredDiscIdentity.MatchString(value) {
			return nil, ErrDiscSelectionChanged
		}
	}
	if !measuredDiscFactsRevision.MatchString(factsRevision) {
		return nil, ErrDiscSelectionChanged
	}
	// Bound the supplied graph before encoding. The actual disc protocol decoder
	// below validates all structure, field sizes, order and selected-title keys.
	if len(structure.Titles) > disc.MaxTitles || len(structure.Kind) > 16 || len(structure.Packaging) > 16 || len(structure.Encryption) > 16 {
		return nil, disc.ErrProtocol
	}
	children := 0
	for _, title := range structure.Titles {
		if len(title.PlaylistFile) > 10 {
			return nil, disc.ErrProtocol
		}
		for _, n := range []int{len(title.Clips), len(title.Streams), len(title.Chapters)} {
			if n > 131072-children {
				return nil, disc.ErrProtocol
			}
			children += n
		}
		for _, clip := range title.Clips {
			if len(clip.File) > 10 {
				return nil, disc.ErrProtocol
			}
		}
		for _, stream := range title.Streams {
			if len(stream.Kind) > 11 || len(stream.Language) > 3 || len(stream.VideoRole) > 11 {
				return nil, disc.ErrProtocol
			}
		}
	}
	raw, err := json.Marshal(structure)
	if err != nil {
		return nil, disc.ErrProtocol
	}
	checked, err := disc.DecodeProbe(raw)
	if err != nil {
		return nil, err
	}
	found := false
	for _, title := range checked.Titles {
		if title.Key(checked.Kind) == selectedKey {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrDiscSelectionChanged
	}
	structureDigest := sha256.Sum256(raw)
	metadata, _ := json.Marshal([]string{assetID, objectID, sourceRevision, hex.EncodeToString(structureDigest[:]), selectedKey})
	tokenDigest := sha256.Sum256(append([]byte("disc_measured_title_metadata_v1\x00"), metadata...))
	token := hex.EncodeToString(tokenDigest[:])
	return &DiscTitleRef{ID: discPublicID(token), Revision: discTitleRevision(sourceRevision, token, factsRevision)}, nil
}
