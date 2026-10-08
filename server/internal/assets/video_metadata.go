package assets

import "portico.local/server/internal/metadataprovider"

// VideoNFO is bounded descriptive evidence, never probe facts or source identity.
// Locator is an opaque sidecar digest, not a client-visible filesystem path.
type VideoNFO struct {
	Kind                   string                          `json:"kind"`
	Locator                string                          `json:"locator"`
	Digest                 string                          `json:"digest"`
	Size                   int64                           `json:"size"`
	ModifiedNS             int64                           `json:"modifiedNs"`
	Revision               string                          `json:"revision,omitempty"`
	Title                  string                          `json:"title,omitempty"`
	OriginalTitle          string                          `json:"originalTitle,omitempty"`
	SortTitle              string                          `json:"sortTitle,omitempty"`
	Year                   int                             `json:"year,omitempty"`
	Overview               string                          `json:"overview,omitempty"`
	Tagline                string                          `json:"tagline,omitempty"`
	Date                   string                          `json:"date,omitempty"`
	Edition                string                          `json:"edition,omitempty"`
	Collection             string                          `json:"collection,omitempty"`
	Certification          string                          `json:"certification,omitempty"`
	Genres                 []string                        `json:"genres,omitempty"`
	Studios                []string                        `json:"studios,omitempty"`
	IDs                    []metadataprovider.ScreenID     `json:"ids,omitempty"`
	Credits                []metadataprovider.ScreenCredit `json:"credits,omitempty"`
	Season                 *int                            `json:"season,omitempty"`
	Episode                *int                            `json:"episode,omitempty"`
	Absolute               *int                            `json:"absolute,omitempty"`
	AdvisoryRuntimeMinutes *int                            `json:"advisoryRuntimeMinutes,omitempty"`
}
