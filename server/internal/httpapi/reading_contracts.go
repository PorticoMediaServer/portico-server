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

type ReadingMetadata struct {
	Title     string           `json:"title,omitempty"`
	Author    string           `json:"author,omitempty"`
	Language  string           `json:"language,omitempty"`
	Direction ReadingDirection `json:"direction,omitempty"`
}
type ReadingResource struct {
	Path string `json:"path"`
	MIME string `json:"mime"`
	Size int64  `json:"size"`
}
type ReadingItem struct {
	ID                 string         `json:"id"`
	LibraryID          string         `json:"libraryId"`
	Title              string         `json:"title"`
	Format             ReadingFormat  `json:"format"`
	SourceID           string         `json:"sourceId"`
	InventoryAvailable bool           `json:"inventoryAvailable"`
	Author             *ReadingAuthor `json:"author,omitempty"`
	Series             *ReadingSeries `json:"series,omitempty"`
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
	Sequential bool              `json:"sequential,omitempty"`
	SessionID  string            `json:"sessionId"`
	ItemID     string            `json:"itemId"`
	SourceID   string            `json:"sourceId"`
	Revision   string            `json:"revision"`
	Format     ReadingFormat     `json:"format"`
	Size       int64             `json:"size"`
	Metadata   ReadingMetadata   `json:"metadata"`
	Resources  []ReadingResource `json:"resources,omitempty"`
	Spine      []string          `json:"spine,omitempty"`
	PageNames  []string          `json:"pageNames,omitempty"`
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
