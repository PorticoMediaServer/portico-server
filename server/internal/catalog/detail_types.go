package catalog

import (
	"encoding/json"

	"portico.local/server/internal/segmentmarkers"
)

type DetailScope struct {
	ServerID    string `json:"serverId"`
	LibraryID   string `json:"libraryId"`
	ItemID      string `json:"itemId"`
	ViewerFence string `json:"viewerFence"`
}
type PersonalConflictChoice struct {
	OperationID string          `json:"operationId"`
	DeviceID    string          `json:"deviceId"`
	AuthoredAt  string          `json:"authoredAt"`
	Value       json.RawMessage `json:"value"`
}
type PersonalConflict struct {
	ID           string                   `json:"id"`
	Field        string                   `json:"field"`
	BaseRevision int64                    `json:"baseRevision"`
	BaseValue    json.RawMessage          `json:"baseValue"`
	Choices      []PersonalConflictChoice `json:"choices"`
}
type PersonalOfflineMutation struct {
	DeviceID         string `json:"deviceId"`
	DeviceMutationID string `json:"deviceMutationId"`
	Sequence         int64  `json:"sequence"`
	BaseRevision     int64  `json:"baseRevision"`
	AuthoredAt       string `json:"authoredAt"`
}
type PersonalResolution struct {
	ConflictID        string `json:"conflictId"`
	ChoiceOperationID string `json:"choiceOperationId"`
}
type PersonalState struct {
	ContinueDismissed bool               `json:"continueDismissed,omitempty"`
	Watched           bool               `json:"watched"`
	ProgressSeconds   float64            `json:"progressSeconds"`
	LastPlayedAt      string             `json:"lastPlayedAt"`
	Status            string             `json:"status"`
	Conflicts         []PersonalConflict `json:"conflicts"`
	Watchlisted       bool               `json:"watchlisted"`
	Favorite          bool               `json:"favorite"`
	// NotInterested hides the title from every recommendation and counts as
	// a mild "less like this"; absent means false.
	NotInterested bool     `json:"notInterested,omitempty"`
	Rating        *float64 `json:"rating"`
	Revision      int64    `json:"revision"`
}
type DetailAction struct {
	ID       string           `json:"id"`
	LabelKey string           `json:"labelKey"`
	Enabled  bool             `json:"enabled"`
	Min      *float64         `json:"min,omitempty"`
	Max      *float64         `json:"max,omitempty"`
	Step     *float64         `json:"step,omitempty"`
	Playback *ContentPlayback `json:"playback,omitempty"`
}
type ProviderRating struct {
	Provider   string  `json:"provider"`
	Value      float64 `json:"value"`
	Max        float64 `json:"max"`
	Votes      int     `json:"votes"`
	SourceURL  string  `json:"sourceUrl"`
	ObservedAt string  `json:"observedAt"`
}
type Genre struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}
type Credit struct {
	PortraitURL string `json:"portraitUrl,omitempty"`
	ID          string `json:"id"`
	// PersonID is the canonical person this credit resolves to, so a client
	// opens one person page from any item that credits them.
	PersonID   string `json:"personId,omitempty"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Department string `json:"department"`
	Provider   string `json:"provider"`
}
type MetadataSource struct {
	Provider   string `json:"provider"`
	SourceURL  string `json:"sourceUrl"`
	ObservedAt string `json:"observedAt"`
}
type DetailMetadata struct {
	Sources      []MetadataSource `json:"sources"`
	Status       string           `json:"status"`
	Ratings      []ProviderRating `json:"ratings"`
	Genres       []Genre          `json:"genres"`
	Credits      []Credit         `json:"credits"`
	CreditTotals CreditTotals     `json:"creditTotals"`
	Attributions []string         `json:"attributions,omitempty"`
}

// TitleFacts are the item's own facts from catalog_item_details (the editor's General
// fields), for the page's facts line and Details section. Absent fields are omitted.
type TitleFacts struct {
	ContentRating string `json:"contentRating,omitempty"`
	Studio        string `json:"studio,omitempty"`
	Network       string `json:"network,omitempty"`
	Tagline       string `json:"tagline,omitempty"`
	ReleaseDate   string `json:"releaseDate,omitempty"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	Country       string `json:"country,omitempty"`
	Edition       string `json:"edition,omitempty"`
}
type Detail struct {
	Facts *TitleFacts `json:"facts,omitempty"`
	// Files is every file of the title with its tracks, for the page's Files list.
	Files    []TitleFile     `json:"files,omitempty"`
	Related  *RelatedMovies  `json:"related,omitempty"`
	Extras   []DetailExtra   `json:"extras,omitempty"`
	Scope    DetailScope     `json:"scope"`
	Revision ContentRevision `json:"revision"`
	Item     Item            `json:"item"`
	Personal PersonalState   `json:"personal"`
	Actions  []DetailAction  `json:"actions"`
	Metadata DetailMetadata  `json:"metadata"`
	// Markers are projected by the request scope that authorized this detail
	// read; analysis owns detection and the skip decision, catalog only carries
	// the result. An empty list means "no markers for this viewer", never
	// "skip anyway".
	Markers []segmentmarkers.Marker `json:"markers"`
}
type PersonalMutation struct {
	ContinueDismissed *bool                    `json:"continueDismissed,omitempty"`
	Watched           *bool                    `json:"watched,omitempty"`
	ProgressSeconds   *float64                 `json:"progressSeconds,omitempty"`
	Offline           *PersonalOfflineMutation `json:"offline,omitempty"`
	Resolution        *PersonalResolution      `json:"resolution,omitempty"`
	OperationID       string                   `json:"operationId"`
	ExpectedRevision  int64                    `json:"expectedRevision"`
	Watchlisted       *bool                    `json:"watchlisted,omitempty"`
	Favorite          *bool                    `json:"favorite,omitempty"`
	NotInterested     *bool                    `json:"notInterested,omitempty"`
	Rating            json.RawMessage          `json:"rating,omitempty"`
}
type PersonalReceipt struct {
	Current                *PersonalState `json:"current,omitempty"`
	ReceiptLifetimeSeconds int64          `json:"receiptLifetimeSeconds"`
	ServerID               string         `json:"serverId"`
	OperationID            string         `json:"operationId"`
	ItemID                 string         `json:"itemId"`
	LibraryID              string         `json:"libraryId"`
	ViewerFence            string         `json:"viewerFence"`
	Personal               PersonalState  `json:"personal"`
}
