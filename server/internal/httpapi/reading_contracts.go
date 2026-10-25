package httpapi

// Reading wire types contain publication facts and the caller's own location.
// Neither a filesystem path, a book-origin URL nor a credential is a resource.
type ReadingFormat string

// Wire vocabulary is additive and build-independent: every build publishes the
// same enum. Whether this server can actually ingest and open a format is a
// runtime capability, never a contract shape.
func (ReadingFormat) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"epub", "pdf", "cbz", "cb7"}}
}

// ReadingSourceFormat describes the real original file. Conversion never widens
// ReadingFormat, which remains the engine/publication coordinate union.
type ReadingSourceFormat string

func (ReadingSourceFormat) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"epub", "pdf", "cbz", "mobi", "azw3", "cb7"}}
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
	Format             ReadingSourceFormat          `json:"format"`
	SourceID           string                       `json:"sourceId"`
	InventoryAvailable bool                         `json:"inventoryAvailable"`
	Author             *ReadingAuthor               `json:"author,omitempty"`
	Series             *ReadingSeries               `json:"series,omitempty"`
	// The current profile's own explicit completion mark on this exact item, when the server
	// published one. Absent is not "unread": it means no mark was recorded for this viewer.
	ReadingMark *ReadingItemMark `json:"readingMark,omitempty"`
}
type ReadingItemMark struct {
	ItemID   string `json:"itemId"`
	Read     bool   `json:"read"`
	Revision int64  `json:"revision"`
}

func (ReadingItemMark) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"itemId", "read", "revision"}, "properties": map[string]any{
		"itemId": annotationIDSchema(), "read": map[string]any{"type": "boolean"}, "revision": annotationInteger(9007199254740991)}}
}
type ReadingItemsResponse struct {
	Items      []ReadingItem `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}
type ReadingContinueItem struct {
	Conversion *ReadingConversionProvenance `json:"conversion,omitempty"`
	Item       ReadingItem                  `json:"item"`
	Progress   ReadingProgress              `json:"progress"`
}
type ReadingContinueResponse struct {
	Items      []ReadingContinueItem `json:"items"`
	NextCursor string                `json:"nextCursor,omitempty"`
}
type ReadingVersion struct {
	SourceID           string              `json:"sourceId"`
	Format             ReadingSourceFormat `json:"format"`
	Size               int64               `json:"size"`
	InventoryAvailable bool                `json:"inventoryAvailable"`
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
	CanSaveFile bool                         `json:"canSaveFile,omitempty"`
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

// ReadingSearchHit is one exact hit in the selected publication's persisted
// text index. The offset is UTF-16 units in the server's normalized section
// text, and Match is a UTF-16 range inside Excerpt; no CFI or position is
// invented.
type ReadingSearchHit struct {
	SectionPath  string `json:"sectionPath"`
	SectionIndex int    `json:"sectionIndex"`
	UTF16Offset  int    `json:"utf16Offset"`
	Excerpt      string `json:"excerpt"`
	MatchStart   int    `json:"matchStart"`
	MatchLength  int    `json:"matchLength"`
}

// ReadingSearchCoverage is the exact index state for the captured session. An
// empty page with pending coverage is not a claim that the book has no matches.
type ReadingSearchCoverage struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type ReadingSearchResponse struct {
	SessionID      string                `json:"sessionId"`
	SourceID       string                `json:"sourceId"`
	SourceRevision string                `json:"sourceRevision"`
	IndexRevision  int64                 `json:"indexRevision"`
	Coverage       ReadingSearchCoverage `json:"coverage"`
	Hits           []ReadingSearchHit    `json:"hits"`
	Next           string                `json:"next,omitempty"`
}

func (ReadingSearchCoverage) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"status"},
		"properties": map[string]any{
			"status": map[string]any{"type": "string", "enum": []string{"indexed", "pending", "unsupported", "evicted", "excluded"}},
			"reason": map[string]any{"type": "string", "maxLength": 64},
		},
	}
}

func (ReadingSearchResponse) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"sessionId", "sourceId", "sourceRevision", "indexRevision", "coverage", "hits"},
		"properties": map[string]any{
			"sessionId":      map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			"sourceId":       map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			"sourceRevision": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			"indexRevision":  map[string]any{"type": "integer", "minimum": 0},
			"coverage": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"status"},
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"indexed", "pending", "unsupported", "evicted", "excluded"}},
					"reason": map[string]any{"type": "string", "maxLength": 64},
				}},
			"hits": map[string]any{"type": "array", "maxItems": 50, "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"sectionPath", "sectionIndex", "utf16Offset", "excerpt", "matchStart", "matchLength"},
				"properties": map[string]any{
					"sectionPath":  map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
					"sectionIndex": map[string]any{"type": "integer", "minimum": 0, "maximum": 4095},
					"utf16Offset":  map[string]any{"type": "integer", "minimum": 0},
					"excerpt":      map[string]any{"type": "string", "maxLength": 8192},
					"matchStart":   map[string]any{"type": "integer", "minimum": 0},
					"matchLength":  map[string]any{"type": "integer", "minimum": 1},
				},
			}},
			"next": map[string]any{"type": "string", "maxLength": 512},
		},
	}
}
