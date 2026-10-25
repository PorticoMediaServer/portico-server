package httpapi

// Reading wire types contain publication facts and the caller's own location.
// Neither a filesystem path, a book-origin URL nor a credential is a resource.
type ReadingFormat string

func (ReadingFormat) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"epub", "pdf", "cbz"}}
}

type ReadingDirection string

func (ReadingDirection) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"ltr", "rtl", "vertical"}}
}

// Publication classification is optional descriptive metadata, separate from ebook/comic identity.
type ReadingPublicationType string

func (ReadingPublicationType) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"book", "magazine", "newspaper"}}
}

type ReadingPublicationTypeSource string

func (ReadingPublicationTypeSource) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"owner", "file-metadata"}}
}

type ReadingPublicationTypeValue struct {
	Value  ReadingPublicationType       `json:"value"`
	Source ReadingPublicationTypeSource `json:"source"`
}

type ReadingMetadata struct {
	EstimatedSeconds *ReadingEstimatedSeconds     `json:"estimatedSeconds,omitempty"`
	WordCount        *ReadingWordCount            `json:"wordCount,omitempty"`
	PageCount        *ReadingPageCount            `json:"pageCount,omitempty"`
	OpeningLines     ReadingOpeningLines          `json:"openingLines,omitempty"`
	Series           *ReadingManifestSeries       `json:"series,omitempty"`
	PublicationType  *ReadingPublicationTypeValue `json:"publicationType,omitempty"`
	CoverURL         string                       `json:"coverUrl,omitempty"`
	Title            string                       `json:"title,omitempty"`
	Author           string                       `json:"author,omitempty"`
	Language         string                       `json:"language,omitempty"`
	Direction        ReadingDirection             `json:"direction,omitempty"`
}
type ReadingResource struct {
	Path string `json:"path"`
	MIME string `json:"mime"`
	Size int64  `json:"size"`
}
type ReadingItem struct {
	PublicationType    *ReadingPublicationTypeValue `json:"publicationType,omitempty"`
	CoverURL           string                       `json:"coverUrl,omitempty"`
	ID                 string                       `json:"id"`
	LibraryID          string                       `json:"libraryId"`
	Title              string                       `json:"title"`
	Format             ReadingFormat                `json:"format"`
	SourceID           string                       `json:"sourceId"`
	InventoryAvailable bool                         `json:"inventoryAvailable"`
	Author             *ReadingAuthor               `json:"author,omitempty"`
	Series             *ReadingSeries               `json:"series,omitempty"`
}
type ReadingItemsResponse struct {
	Items      []ReadingItem `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}
type ReadingContinueItem struct {
	Item     ReadingItem     `json:"item"`
	Progress ReadingProgress `json:"progress"`
}
type ReadingContinueResponse struct {
	Items      []ReadingContinueItem `json:"items"`
	NextCursor string                `json:"nextCursor,omitempty"`
}
type ReadingVersion struct {
	SourceID           string        `json:"sourceId"`
	Format             ReadingFormat `json:"format"`
	Size               int64         `json:"size"`
	InventoryAvailable bool          `json:"inventoryAvailable"`
}
type ReadingVersionsResponse struct {
	Versions []ReadingVersion `json:"versions"`
}
type ReadingSessionRequest struct {
	ItemID   string `json:"itemId"`
	SourceID string `json:"sourceId"`
}
type ReadingSessionResponse struct {
	Conversion  *ReadingConversionProvenance `json:"conversion,omitempty"`
	CanAnnotate bool                         `json:"canAnnotate,omitempty"`
	Contents    ReadingContents              `json:"contents,omitempty"`
	Sequential  bool                         `json:"sequential,omitempty"`
	SessionID   string                       `json:"sessionId"`
	ItemID      string                       `json:"itemId"`
	SourceID    string                       `json:"sourceId"`
	Revision    string                       `json:"revision"`
	Format      ReadingFormat                `json:"format"`
	Size        int64                        `json:"size"`
	Metadata    ReadingMetadata              `json:"metadata"`
	Resources   []ReadingResource            `json:"resources,omitempty"`
	Spine       []string                     `json:"spine,omitempty"`
	PageNames   []string                     `json:"pageNames,omitempty"`
}

// ReadingContentsEntry is an archive-internal EPUB navigation target, not a
// bearer URL, inferred chapter time, or a stored Reading position.
type ReadingContentsEntry struct {
	Title    string `json:"title"`
	Section  string `json:"section"`
	Fragment string `json:"fragment,omitempty"`
	Depth    int    `json:"depth"`
}
type ReadingContents []ReadingContentsEntry

func (ReadingContents) JSONSchema() map[string]any {
	return map[string]any{"type": "array", "maxItems": 512, "items": map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"title", "section", "depth"},
		"properties": map[string]any{
			"title":    map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
			"section":  map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
			"fragment": map[string]any{"type": "string", "maxLength": 1024},
			"depth":    map[string]any{"type": "integer", "minimum": 0, "maximum": 16},
		},
	}}
}

type ReadingLocation struct {
	Format      ReadingFormat `json:"format"`
	CFI         string        `json:"cfi,omitempty"`
	Section     string        `json:"section,omitempty"`
	PageIndex   *int64        `json:"pageIndex,omitempty"`
	Progression float64       `json:"progression"`
}
type ReadingProgress struct {
	SourceID       string          `json:"sourceId"`
	SourceRevision string          `json:"sourceRevision"`
	Location       ReadingLocation `json:"location"`
	Revision       int64           `json:"revision"`
	ObservedAt     int64           `json:"observedAt"`
	Stale          bool            `json:"stale"`
}
type ReadingProgressResponse struct {
	Progress *ReadingProgress `json:"progress"`
}
type ReadingProgressRequest struct {
	ExpectedRevision int64           `json:"expectedRevision"`
	Location         ReadingLocation `json:"location"`
}
type ReadingProgressSavedResponse struct {
	Progress *ReadingProgress `json:"progress"`
	Saved    bool             `json:"saved"`
}
