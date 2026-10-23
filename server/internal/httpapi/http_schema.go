package httpapi

import (
	"reflect"

	"portico.local/apikit"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/catalog"
	librarychannels "portico.local/server/internal/livechannels/library"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/playback"
)

// PublishedSchemas derives reusable schemas from the same wire types as the
// routes. Bounds here describe limits already enforced by their owning services.
func PublishedSchemas() (map[string]reflect.Type, map[reflect.Type]map[string]any, error) {
	named := map[string]reflect.Type{
		"LibraryChannelConfig": reflect.TypeFor[librarychannels.Config](),
		"HomeLayout":           reflect.TypeFor[operations.HomeLayoutDocument](),
		"HomeDocument":         reflect.TypeFor[catalog.HomeDocument](),
		"DetailAction":         reflect.TypeFor[catalog.DetailAction](),
		"AudioRenditionPlan":   reflect.TypeFor[playback.AudioRenditionPlan](),
		"Capture":              reflect.TypeFor[operations.Capture](),
		"FeedbackSubmission":   reflect.TypeFor[operations.FeedbackSubmission](),
		"ContentEntry":         reflect.TypeFor[catalog.ContentEntry](),
		"SearchMatch":          reflect.TypeFor[catalog.SearchMatch](),
		"SearchChannel":        reflect.TypeFor[catalog.SearchChannel](),
		"SearchProgramme":      reflect.TypeFor[catalog.SearchProgramme](),
		"SearchGenre":          reflect.TypeFor[catalog.SearchGenre](),
		"SearchDiscovery":      reflect.TypeFor[catalog.SearchDiscovery](),
		"Category":             reflect.TypeFor[catalog.Category](),
		"FilesystemPage":       reflect.TypeFor[administration.FilesystemPage](),
	}
	wire := map[reflect.Type]map[string]any{}
	for name, typ := range named {
		shape, err := apikit.Schema(typ)
		if err != nil {
			return nil, nil, err
		}
		props, _ := shape["properties"].(map[string]any)
		if name == "ContentEntry" {
			if roles, ok := props["roles"].(map[string]any); ok {
				roles["maxItems"] = 16
			}
		}
		if name == "SearchMatch" {
			props["field"].(map[string]any)["enum"] = []string{"credit", "artist", "album", "author", "narrator", "series", "show"}
			props["text"].(map[string]any)["maxLength"] = 2048
		}
		if name == "SearchChannel" {
			props["provenance"].(map[string]any)["enum"] = []string{"live-source", "library-channel"}
			for _, f := range []string{"id", "sourceId", "generation", "number"} {
				props[f].(map[string]any)["maxLength"] = 256
			}
		}
		if name == "SearchGenre" {
			props["posterUrls"].(map[string]any)["maxItems"] = 2
		}
		if name == "SearchDiscovery" {
			props["genres"].(map[string]any)["maxItems"] = 24
			props["onNow"].(map[string]any)["maxItems"] = 4
		}
		if name == "SearchProgramme" {
			props["moreAirings"].(map[string]any)["minimum"] = 0
		}

		field := func(key string) map[string]any { v, _ := props[key].(map[string]any); return v }
		switch name {
		case "HomeDocument":
			field("rows")["maxItems"] = 64
			field("spotlight")["maxItems"] = 5
		case "HomeLayout":
			for _, key := range []string{"rowOrder", "hiddenRowIds"} {
				v := field(key)
				v["maxItems"] = 64
				if item, ok := v["items"].(map[string]any); ok {
					item["maxLength"] = 128
				}
			}
		case "FilesystemPage":
			field("roots")["maxItems"] = 64
			field("entries")["maxItems"] = 200
			field("suggestions")["maxItems"] = 8
		case "Capture":
			field("component")["enum"] = []string{"scheduler"}
		case "FeedbackSubmission":
			field("message")["minLength"] = 8
			field("message")["maxLength"] = 2000
		}
		wire[typ] = shape
	}
	return named, wire, nil
}

type libraryCreationRequest struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	Path             string   `json:"path,omitempty"`
	Folders          []string `json:"folders,omitempty"`
	MetadataAgent    string   `json:"metadataAgent,omitempty"`
	MetadataLanguage string   `json:"metadataLanguage,omitempty"`
}

func (libraryCreationRequest) JSONSchema() map[string]any {
	type wire libraryCreationRequest
	shape, _ := apikit.Schema(reflect.TypeFor[wire]())
	shape["properties"].(map[string]any)["kind"].(map[string]any)["enum"] = []string{"movie", "tv", "anime", "music", "audiobook", "book", "comic"}
	folders := shape["properties"].(map[string]any)["folders"].(map[string]any)
	folders["minItems"] = 1
	folders["maxItems"] = 64
	return shape
}
