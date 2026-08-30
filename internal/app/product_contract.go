package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/app/productlanguage"
	"github.com/PorticoMediaServer/portico-server/internal/browsecontract"
	"github.com/PorticoMediaServer/portico-server/internal/catalogkind"
)

const (
	productContractRevision = "v1"
	mediaActionRevision     = "v1"
	browseDefaultLimit      = 60
	browseMaximumLimit      = 200
)

type CanonicalProductContract struct {
	APIVersion         string                       `json:"apiVersion"`
	ActionRevision     string                       `json:"actionRevision"`
	Language           productlanguage.Reference    `json:"language"`
	LibraryKinds       []ProductLibraryKind         `json:"libraryKinds"`
	EntityKinds        []string                     `json:"entityKinds"`
	EntitySemantics    []EntitySemantic             `json:"entitySemantics"`
	ArtworkRoles       []ArtworkRoleSemantic        `json:"artworkRoles"`
	BrowseFields       []BrowseFieldCapability      `json:"browseFields"`
	BrowseSorts        []BrowseSortCapability       `json:"browseSorts"`
	BrowseOperators    []BrowseOperator             `json:"browseOperators"`
	PresentationFields []string                     `json:"presentationFields"`
	QueryLimits        BrowseQueryLimits            `json:"queryLimits"`
	Search             SearchContract               `json:"search"`
	MediaActions       []MediaActionCapability      `json:"mediaActions"`
	EventTransports    []string                     `json:"eventTransports"`
	ApplicationEvents  ApplicationEventCapabilities `json:"applicationEvents"`
	LongPoll           LongPollCapabilities         `json:"longPoll"`
	ServerCapabilities []string                     `json:"serverCapabilities"`
	Compatibility      CompatibilityEnvelope        `json:"compatibility"`
}

type ApplicationEventCapabilities struct {
	Revision                     string   `json:"revision"`
	EventTypes                   []string `json:"eventTypes"`
	Tags                         []string `json:"tags"`
	AuthoritativeResetErrorCodes []string `json:"authoritativeResetErrorCodes"`
	LongPollResetField           string   `json:"longPollResetField"`
}

type LongPollCapabilities struct {
	DefaultWaitSeconds       int `json:"defaultWaitSeconds"`
	MaximumWaitSeconds       int `json:"maximumWaitSeconds"`
	MaximumConcurrentStreams int `json:"maximumConcurrentStreams"`
}

type SearchContract struct {
	Revision        string                   `json:"revision"`
	Endpoint        string                   `json:"endpoint"`
	GroupOrder      []string                 `json:"groupOrder"`
	Groups          []SearchGroupCapability  `json:"groups"`
	Sorts           []SearchSortCapability   `json:"sorts"`
	Filters         []SearchFilterCapability `json:"filters"`
	FacetMode       string                   `json:"facetMode"`
	Limits          SearchLimits             `json:"limits"`
	Cursor          SearchCursorSemantics    `json:"cursor"`
	ResultSemantics SearchResultSemantics    `json:"resultSemantics"`
}

type SearchGroupCapability struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	EntityKind           string   `json:"entityKind"`
	ResultKinds          []string `json:"resultKinds"`
	SupportsLibraryScope bool     `json:"supportsLibraryScope"`
	Sorts                []string `json:"sorts"`
}

type SearchSortCapability struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Directions       []string `json:"directions"`
	DefaultDirection string   `json:"defaultDirection"`
	ApplicableGroups []string `json:"applicableGroups"`
}

type SearchFilterCapability struct {
	ID            string              `json:"id"`
	Label         string              `json:"label"`
	ValueType     string              `json:"valueType"`
	Multiple      bool                `json:"multiple"`
	AllowedValues []string            `json:"allowedValues,omitempty"`
	Source        *SearchFilterSource `json:"source,omitempty"`
}

type SearchFilterSource struct {
	Endpoint   string `json:"endpoint"`
	ValueField string `json:"valueField"`
	LabelField string `json:"labelField"`
}

type SearchLimits struct {
	MinimumQueryLength        int `json:"minimumQueryLength"`
	MaximumQueryLength        int `json:"maximumQueryLength"`
	DefaultGroupLimit         int `json:"defaultGroupLimit"`
	MaximumGroupLimit         int `json:"maximumGroupLimit"`
	QuickInitialGroupLimit    int `json:"quickInitialGroupLimit"`
	QuickMaximumGroups        int `json:"quickMaximumGroups"`
	QuickMaximumItemsPerGroup int `json:"quickMaximumItemsPerGroup"`
	FullDefaultGroupLimit     int `json:"fullDefaultGroupLimit"`
}

type SearchCursorSemantics struct {
	Mode                string   `json:"mode"`
	Opaque              bool     `json:"opaque"`
	RequiresSingleGroup bool     `json:"requiresSingleGroup"`
	PrincipalBound      bool     `json:"principalBound"`
	ScopeFields         []string `json:"scopeFields"`
	TTLSeconds          int      `json:"ttlSeconds"`
	ExpiredErrorCode    string   `json:"expiredErrorCode"`
	InvalidErrorCode    string   `json:"invalidErrorCode"`
}

type SearchResultSemantics struct {
	DestinationSource string                    `json:"destinationSource"`
	HierarchySource   string                    `json:"hierarchySource"`
	ArtworkRoleSource string                    `json:"artworkRoleSource"`
	KindMappings      []SearchResultKindMapping `json:"kindMappings"`
}

type SearchResultKindMapping struct {
	ResultKind string `json:"resultKind"`
	EntityKind string `json:"entityKind"`
}

type MediaActionCapability struct {
	ID            string                  `json:"id"`
	Mutating      bool                    `json:"mutating"`
	BulkSupported bool                    `json:"bulkSupported"`
	Presentation  MediaActionPresentation `json:"presentation"`
	Command       MediaActionCommand      `json:"command"`
	Confirmation  MediaActionConfirmation `json:"confirmation"`
	Invalidates   []string                `json:"invalidates"`
}

type MediaActionPresentation struct {
	LabelMessageID string   `json:"labelMessageId"`
	IconID         string   `json:"iconId"`
	Group          string   `json:"group"`
	Priority       int      `json:"priority"`
	Surfaces       []string `json:"surfaces"`
}

type MediaActionCommand struct {
	Kind           string         `json:"kind"`
	Execution      string         `json:"execution"`
	Method         string         `json:"method,omitempty"`
	PathTemplate   string         `json:"pathTemplate,omitempty"`
	StaticBody     map[string]any `json:"staticBody,omitempty"`
	RequiredInputs []string       `json:"requiredInputs,omitempty"`
	FlowID         string         `json:"flowId,omitempty"`
	ResultHandling string         `json:"resultHandling"`
}

type MediaActionConfirmation struct {
	Required bool   `json:"required"`
	Tone     string `json:"tone"`
}

type EntitySemantic struct {
	ID                 string   `json:"id"`
	Container          bool     `json:"container"`
	Playable           bool     `json:"playable"`
	ParentKinds        []string `json:"parentKinds"`
	ChildKinds         []string `json:"childKinds"`
	ChildOrder         []string `json:"childOrder"`
	DefaultDestination string   `json:"defaultDestination"`
	PrimaryArtworkRole string   `json:"primaryArtworkRole"`
}

type ArtworkRoleSemantic struct {
	ID          string  `json:"id"`
	AspectRatio float64 `json:"aspectRatio"`
	Fit         string  `json:"fit"`
	Purpose     string  `json:"purpose"`
}

type ProductLibraryKind struct {
	ID          string                  `json:"id"`
	Label       string                  `json:"label"`
	Description string                  `json:"description"`
	Pivots      []BrowsePivotCapability `json:"pivots"`
	SortOrder   int                     `json:"sortOrder"`
}

type BrowsePivotCapability struct {
	ID                 string       `json:"id"`
	Label              string       `json:"label"`
	EntityKinds        []string     `json:"entityKinds"`
	DefaultView        string       `json:"defaultView"`
	SupportedViews     []string     `json:"supportedViews"`
	DefaultSort        []BrowseSort `json:"defaultSort"`
	BrowseSupported    bool         `json:"browseSupported"`
	EndpointTemplate   string       `json:"endpointTemplate"`
	PresentationFields []string     `json:"presentationFields"`
}

type BrowseFieldCapability struct {
	ID              string             `json:"id"`
	Label           string             `json:"label"`
	ValueType       string             `json:"valueType"`
	Operators       []string           `json:"operators"`
	ApplicableKinds []string           `json:"applicableKinds,omitempty"`
	CaseSensitive   bool               `json:"caseSensitive,omitempty"`
	AllowedValues   []string           `json:"allowedValues,omitempty"`
	FacetSource     *BrowseFacetSource `json:"facetSource,omitempty"`
	ControlHint     string             `json:"controlHint"`
	Complexity      string             `json:"complexity"`
	Cost            string             `json:"cost"`
}

type BrowseFacetSource struct {
	EndpointTemplate string `json:"endpointTemplate"`
	FilterField      string `json:"filterField"`
	FilterPrefix     string `json:"filterPrefix"`
	ValueField       string `json:"valueField"`
	LabelField       string `json:"labelField"`
	CountField       string `json:"countField"`
}

type BrowseSortCapability struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Directions       []string `json:"directions"`
	DefaultDirection string   `json:"defaultDirection"`
	ApplicableKinds  []string `json:"applicableKinds,omitempty"`
	Expensive        bool     `json:"expensive"`
}

type BrowseOperator struct {
	ID         string   `json:"id"`
	ValueTypes []string `json:"valueTypes"`
}

type BrowseQueryLimits struct {
	MaximumDepth     int `json:"maximumDepth"`
	MaximumClauses   int `json:"maximumClauses"`
	MaximumBytes     int `json:"maximumBytes"`
	DefaultLimit     int `json:"defaultLimit"`
	MaximumLimit     int `json:"maximumLimit"`
	CursorTTLSeconds int `json:"cursorTtlSeconds"`
}

type LibraryBrowseScope struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type LibraryBrowseCapabilities struct {
	APIVersion         string                  `json:"apiVersion"`
	Library            LibraryBrowseScope      `json:"library"`
	Pivots             []BrowsePivotCapability `json:"pivots"`
	ResolvedPivot      *BrowsePivotCapability  `json:"resolvedPivot,omitempty"`
	Fields             []BrowseFieldCapability `json:"fields"`
	Sorts              []BrowseSortCapability  `json:"sorts"`
	PresentationFields []string                `json:"presentationFields"`
	Actions            []string                `json:"actions"`
	QueryLimits        BrowseQueryLimits       `json:"queryLimits"`
}

func canonicalProductContract() CanonicalProductContract {
	return CanonicalProductContract{
		APIVersion:         productContractRevision,
		ActionRevision:     mediaActionRevision,
		Language:           productlanguage.CanonicalReference(),
		LibraryKinds:       productLibraryKinds(),
		EntityKinds:        canonicalEntityKinds(),
		EntitySemantics:    canonicalEntitySemantics(),
		ArtworkRoles:       canonicalArtworkRoles(),
		BrowseFields:       canonicalBrowseFields(),
		BrowseSorts:        canonicalBrowseSorts(),
		BrowseOperators:    canonicalBrowseOperators(),
		PresentationFields: canonicalPresentationFields(),
		QueryLimits:        canonicalBrowseQueryLimits(),
		Search:             canonicalSearchContract(),
		MediaActions:       canonicalMediaActionCapabilities(),
		EventTransports:    []string{"sse", "long-poll"},
		ApplicationEvents: ApplicationEventCapabilities{
			Revision:   "v1",
			EventTypes: []string{"data.changed", "library.scan.completed"},
			Tags: []string{
				"account", "audiobook-entities", "collections", "dashboard:history", "dashboard:jobs", "dashboard:live",
				"database", "devices", "display-preferences", "downloads", "dvr", "home", "jobs", "libraries",
				"library-channels", "library-items", "live-tv", "media", "media-state", "metadata", "notifications",
				"playback", "playback-progress", "playlists", "profiles", "quick-connect", "remote-access", "saved",
				"search", "settings", "users", "watch-with-friends",
			},
			AuthoritativeResetErrorCodes: []string{"invalid_poll_cursor"},
			LongPollResetField:           "resetRequired",
		},
		LongPoll: LongPollCapabilities{
			DefaultWaitSeconds: 20, MaximumWaitSeconds: 25, MaximumConcurrentStreams: 4,
		},
		ServerCapabilities: serverCapabilityCatalogIDs(),
	}
}

func (s *Server) canonicalProductContract() CanonicalProductContract {
	contract := canonicalProductContract()
	contract.Compatibility = s.compatibilityEnvelope()
	contract.ServerCapabilities = availableServerCapabilityIDs(contract.Compatibility.Capabilities)
	return contract
}

func canonicalSearchContract() SearchContract {
	groupOrder := make([]string, 0, len(searchGroupDefinitions))
	groups := make([]SearchGroupCapability, 0, len(searchGroupDefinitions))
	mediaSorts := []string{searchSortRelevance, searchSortTitle, searchSortReleaseYear, searchSortDateAdded}
	allGroups := make([]string, 0, len(searchGroupDefinitions))
	mediaGroups := make([]string, 0, len(searchGroupDefinitions)-1)
	titleGroups := make([]string, 0, len(searchGroupDefinitions)-1)
	entityFilters := make([]string, 0, len(searchGroupDefinitions)*2)
	for _, definition := range searchGroupDefinitions {
		groupOrder = append(groupOrder, definition.ID)
		allGroups = append(allGroups, definition.ID)
		sorts := mediaSorts
		if definition.Live {
			sorts = []string{searchSortRelevance}
		} else if definition.People {
			sorts = []string{searchSortRelevance, searchSortTitle}
			titleGroups = append(titleGroups, definition.ID)
		} else {
			mediaGroups = append(mediaGroups, definition.ID)
			titleGroups = append(titleGroups, definition.ID)
		}
		entityFilters = append(entityFilters, definition.ID)
		if definition.EntityKind != definition.ID {
			entityFilters = append(entityFilters, definition.EntityKind)
		}
		for _, storageType := range definition.Types {
			if !containsString(entityFilters, storageType) {
				entityFilters = append(entityFilters, storageType)
			}
			kind := string(catalogkind.Public(storageType))
			if !containsString(entityFilters, kind) {
				entityFilters = append(entityFilters, kind)
			}
		}
		resultKinds := append([]string(nil), definition.Types...)
		if definition.People {
			resultKinds = []string{"person"}
		}
		groups = append(groups, SearchGroupCapability{
			ID: definition.ID, Title: definition.Title, EntityKind: definition.EntityKind,
			ResultKinds: resultKinds, SupportsLibraryScope: !definition.Live,
			Sorts: append([]string(nil), sorts...),
		})
	}
	return SearchContract{
		Revision: productContractRevision, Endpoint: "/api/search", GroupOrder: groupOrder, Groups: groups,
		Sorts: []SearchSortCapability{
			{ID: searchSortRelevance, Label: "Best match", Directions: []string{searchDirectionDesc}, DefaultDirection: searchDirectionDesc, ApplicableGroups: allGroups},
			{ID: searchSortTitle, Label: "Title", Directions: []string{searchDirectionAsc, searchDirectionDesc}, DefaultDirection: searchDirectionAsc, ApplicableGroups: titleGroups},
			{ID: searchSortReleaseYear, Label: "Release year", Directions: []string{searchDirectionAsc, searchDirectionDesc}, DefaultDirection: searchDirectionDesc, ApplicableGroups: mediaGroups},
			{ID: searchSortDateAdded, Label: "Date added", Directions: []string{searchDirectionAsc, searchDirectionDesc}, DefaultDirection: searchDirectionDesc, ApplicableGroups: mediaGroups},
		},
		Filters: []SearchFilterCapability{
			{ID: "entityKinds", Label: "Result types", ValueType: "enum", Multiple: true, AllowedValues: entityFilters},
			{ID: "libraryIds", Label: "Libraries", ValueType: "identity", Multiple: true, Source: &SearchFilterSource{Endpoint: "/api/libraries", ValueField: "id", LabelField: "name"}},
			{ID: "group", Label: "Result group", ValueType: "enum", Multiple: false, AllowedValues: groupOrder},
		},
		FacetMode: "none",
		Limits: SearchLimits{
			MinimumQueryLength: 1, MaximumQueryLength: 120, DefaultGroupLimit: searchPreviewLimit, MaximumGroupLimit: 50,
			QuickInitialGroupLimit: 3, QuickMaximumGroups: 6, QuickMaximumItemsPerGroup: 6, FullDefaultGroupLimit: 50,
		},
		Cursor: SearchCursorSemantics{
			Mode: "independent-group", Opaque: true, RequiresSingleGroup: true, PrincipalBound: true,
			ScopeFields: []string{"query", "group", "libraryIds", "sort", "direction"}, TTLSeconds: int(cursorDefaultTTL.Seconds()),
			ExpiredErrorCode: "cursor_expired", InvalidErrorCode: "invalid_cursor",
		},
		ResultSemantics: SearchResultSemantics{
			DestinationSource: "entitySemantics.defaultDestination", HierarchySource: "entitySemantics.parentKinds+childKinds+childOrder",
			ArtworkRoleSource: "entitySemantics.primaryArtworkRole",
			KindMappings:      searchResultKindMappings(),
		},
	}
}

func searchResultKindMappings() []SearchResultKindMapping {
	result := make([]SearchResultKindMapping, 0)
	for _, definition := range searchGroupDefinitions {
		if definition.People {
			result = append(result, SearchResultKindMapping{ResultKind: "person", EntityKind: string(catalogkind.Person)})
			continue
		}
		for _, storageType := range definition.Types {
			result = append(result, SearchResultKindMapping{ResultKind: storageType, EntityKind: string(catalogkind.Public(storageType))})
		}
	}
	return result
}

func productLibraryKinds() []ProductLibraryKind {
	contentViews := []string{"grid", "compact-grid", "list", "table"}
	pivot := func(id, label string, entityKinds []string, view string, sort []BrowseSort, browse bool, endpoint string) BrowsePivotCapability {
		views := append([]string(nil), contentViews...)
		if !browse {
			views = []string{view}
		}
		return BrowsePivotCapability{
			ID: id, Label: label, EntityKinds: entityKinds, DefaultView: view,
			SupportedViews: views, DefaultSort: sort, BrowseSupported: browse,
			EndpointTemplate: endpoint, PresentationFields: canonicalPresentationFields(),
		}
	}
	title := []BrowseSort{{Field: "title", Direction: "asc"}}
	recent := []BrowseSort{{Field: "dateAdded", Direction: "desc"}, {Field: "title", Direction: "asc"}}
	episodes := []BrowseSort{{Field: "seasonNumber", Direction: "asc"}, {Field: "episodeNumber", Direction: "asc"}}
	tracks := []BrowseSort{{Field: "album", Direction: "asc"}, {Field: "trackNumber", Direction: "asc"}}
	return []ProductLibraryKind{
		{ID: "movies", Label: "Movies", Description: "Feature films, shorts, and cinematic collections.", SortOrder: 10, Pivots: []BrowsePivotCapability{
			pivot("discover", "Discover", []string{"movie"}, "shelves", recent, false, "/api/libraries/{libraryId}/discover"),
			pivot("movies", "Movies", []string{"movie"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("collections", "Collections", []string{"collection"}, "grid", title, false, "/api/collections?libraryId={libraryId}"),
			pivot("categories", "Categories", []string{"category"}, "facets", title, false, "/api/libraries/{libraryId}/categories"),
		}},
		{ID: "tv", Label: "TV Shows", Description: "Episodic television with explicit show, season, and episode relationships.", SortOrder: 20, Pivots: []BrowsePivotCapability{
			pivot("discover", "Discover", []string{"show"}, "shelves", recent, false, "/api/libraries/{libraryId}/discover"),
			pivot("shows", "Shows", []string{"show"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("episodes", "Episodes", []string{"episode"}, "list", episodes, true, "/api/libraries/{libraryId}/browse"),
			pivot("collections", "Collections", []string{"collection"}, "grid", title, false, "/api/collections?libraryId={libraryId}"),
			pivot("categories", "Categories", []string{"category"}, "facets", title, false, "/api/libraries/{libraryId}/categories"),
		}},
		{ID: "anime", Label: "Anime", Description: "Anime series and films using the television hierarchy with independent defaults.", SortOrder: 30, Pivots: []BrowsePivotCapability{
			pivot("discover", "Discover", []string{"show"}, "shelves", recent, false, "/api/libraries/{libraryId}/discover"),
			pivot("shows", "Shows", []string{"show"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("episodes", "Episodes", []string{"episode"}, "list", episodes, true, "/api/libraries/{libraryId}/browse"),
			pivot("collections", "Collections", []string{"collection"}, "grid", title, false, "/api/collections?libraryId={libraryId}"),
			pivot("categories", "Categories", []string{"category"}, "facets", title, false, "/api/libraries/{libraryId}/categories"),
		}},
		{ID: "music", Label: "Music", Description: "Artists, albums, tracks, playlists, and genres.", SortOrder: 40, Pivots: []BrowsePivotCapability{
			pivot("discover", "Discover", []string{"album", "track"}, "shelves", recent, false, "/api/libraries/{libraryId}/discover"),
			pivot("artists", "Artists", []string{"artist"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("albums", "Albums", []string{"album"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("tracks", "Tracks", []string{"track"}, "list", tracks, true, "/api/libraries/{libraryId}/browse"),
			pivot("playlists", "Playlists", []string{"playlist"}, "list", title, false, "/api/playlists?libraryId={libraryId}"),
			pivot("genres", "Genres", []string{"category"}, "facets", title, false, "/api/libraries/{libraryId}/categories"),
		}},
		{ID: "audiobooks", Label: "Audiobooks", Description: "Authors, books, series, and chapter-aware long-form playback.", SortOrder: 50, Pivots: []BrowsePivotCapability{
			pivot("discover", "Discover", []string{"book"}, "shelves", recent, false, "/api/libraries/{libraryId}/discover"),
			pivot("authors", "Authors", []string{"author"}, "grid", title, false, "/api/libraries/{libraryId}/authors"),
			pivot("books", "Books", []string{"book"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("series", "Series", []string{"audiobook-series"}, "grid", title, false, "/api/libraries/{libraryId}/series"),
			pivot("collections", "Collections", []string{"collection"}, "grid", title, false, "/api/collections?libraryId={libraryId}"),
		}},
		{ID: "recorded-tv", Label: "Recorded TV", Description: "Completed recordings grouped by show and recording schedule.", SortOrder: 60, Pivots: []BrowsePivotCapability{
			pivot("recordings", "Recordings", []string{"recording"}, "grid", recent, true, "/api/libraries/{libraryId}/browse"),
			pivot("shows", "Shows", []string{"recording"}, "grid", title, true, "/api/libraries/{libraryId}/browse"),
			pivot("schedule", "Schedule", []string{"recording"}, "timeline", recent, false, "/api/dvr/schedule"),
			pivot("categories", "Categories", []string{"category"}, "facets", title, false, "/api/libraries/{libraryId}/categories"),
		}},
	}
}

func canonicalEntityKinds() []string {
	return catalogkind.PublicKinds()
}

func canonicalArtworkRoles() []ArtworkRoleSemantic {
	return []ArtworkRoleSemantic{
		{ID: "poster", AspectRatio: 2.0 / 3.0, Fit: "cover", Purpose: "Primary portrait artwork for video, books, recordings, and collections."},
		{ID: "square", AspectRatio: 1, Fit: "cover", Purpose: "Primary artwork for artists, albums, tracks, and music collections."},
		{ID: "backdrop", AspectRatio: 16.0 / 9.0, Fit: "cover", Purpose: "Wide detail, hero, and playback background artwork."},
		{ID: "thumb", AspectRatio: 16.0 / 9.0, Fit: "cover", Purpose: "Episode, chapter, and compact landscape artwork."},
		{ID: "logo", AspectRatio: 3, Fit: "contain", Purpose: "Transparent title or channel mark; never a primary-card substitute."},
		{ID: "banner", AspectRatio: 5.4, Fit: "cover", Purpose: "Optional extra-wide promotional artwork."},
	}
}

func canonicalEntitySemantics() []EntitySemantic {
	semantic := func(id string, container, playable bool, parents, children, order []string, destination, artwork string) EntitySemantic {
		return EntitySemantic{
			ID: id, Container: container, Playable: playable,
			ParentKinds: nonNilStrings(parents), ChildKinds: nonNilStrings(children), ChildOrder: nonNilStrings(order),
			DefaultDestination: destination, PrimaryArtworkRole: artwork,
		}
	}
	return []EntitySemantic{
		semantic("movie", false, true, nil, []string{"extra"}, []string{"title", "id"}, "detail", "poster"),
		semantic("show", true, false, nil, []string{"season", "special", "episode"}, []string{"seasonNumber", "episodeNumber", "title", "id"}, "detail", "poster"),
		semantic("season", true, false, []string{"show"}, []string{"episode", "special"}, []string{"episodeNumber", "title", "id"}, "series-detail", "poster"),
		semantic("episode", false, true, []string{"season", "show"}, []string{"extra"}, []string{"title", "id"}, "series-detail", "poster"),
		semantic("special", false, true, []string{"season", "show"}, nil, nil, "detail", "poster"),
		semantic("artist", true, false, nil, []string{"album", "track"}, []string{"releaseDate", "title", "id"}, "detail", "square"),
		semantic("album", true, false, []string{"artist"}, []string{"track"}, []string{"discNumber", "trackNumber", "title", "id"}, "detail", "square"),
		semantic("track", false, true, []string{"album", "artist"}, nil, nil, "detail", "square"),
		semantic("author", true, false, nil, []string{"audiobook-series", "book"}, []string{"seriesIndex", "releaseDate", "title", "id"}, "detail", "poster"),
		semantic("audiobook-series", true, false, []string{"author"}, []string{"book"}, []string{"seriesIndex", "releaseDate", "title", "id"}, "children", "poster"),
		semantic("book", true, true, []string{"author", "audiobook-series"}, []string{"chapter"}, []string{"chapterNumber", "title", "id"}, "detail", "poster"),
		semantic("chapter", false, true, []string{"book"}, nil, nil, "detail", "thumb"),
		semantic("collection", true, false, nil, []string{"movie", "show", "season", "episode", "album", "track", "book", "recording"}, []string{"membershipOrder", "title", "id"}, "children", "poster"),
		semantic("playlist", true, false, nil, []string{"movie", "episode", "track", "book", "recording"}, []string{"entryOrder", "id"}, "children", "poster"),
		semantic("category", true, false, nil, []string{"movie", "show", "episode", "album", "track", "book", "recording"}, []string{"title", "id"}, "children", "poster"),
		semantic("recording", false, true, []string{"show"}, nil, nil, "detail", "poster"),
		semantic("live-channel", false, true, nil, []string{"live-program"}, []string{"startTime", "id"}, "detail", "logo"),
		semantic("live-program", false, false, []string{"live-channel"}, nil, nil, "detail", "thumb"),
		semantic("person", true, false, nil, []string{"movie", "show", "episode"}, []string{"releaseDate", "title", "id"}, "detail", "poster"),
		semantic("extra", false, true, []string{"movie", "show", "season", "episode"}, nil, nil, "detail", "poster"),
		semantic("unsupported", false, false, nil, nil, nil, "detail", "poster"),
	}
}

func canonicalBrowseFields() []BrowseFieldCapability {
	labels := map[string]string{
		"entityKind": "Media type", "title": "Title", "year": "Year", "decade": "Decade",
		"alternateTitle": "Alternate title", "originalTitle": "Original title", "language": "Language", "status": "Status",
		"dateAdded": "Date added", "playState": "Playback", "favorite": "Favorite",
		"watchlisted": "Saved", "personalRating": "Your rating", "genre": "Genre", "tag": "Tag", "label": "Label", "author": "Author",
		"narrator": "Narrator", "series": "Series", "contentRating": "Content rating", "regionalCertification": "Regional certification",
		"communityRating": "Community rating", "criticRating": "Critic rating", "audienceRating": "Audience rating",
		"availability": "Availability", "acceptedProviderIdentity": "Provider identity",
		"durationSeconds": "Duration", "lastPlayedAt": "Last played", "releaseDate": "Release date",
		"studio": "Studio", "company": "Company", "network": "Network", "country": "Country",
		"keyword": "Keyword", "collection": "Collection", "franchise": "Franchise",
		"actor": "Actor", "director": "Director", "writer": "Writer", "creator": "Creator", "credit": "Credit",
		"resolution": "Resolution", "dynamicRange": "Dynamic range", "source": "Source", "mediaVersion": "Media version",
	}
	applicableKinds := map[string][]string{
		"author":       {"book"},
		"narrator":     {"book"},
		"series":       {"book", "audiobook-series"},
		"studio":       {"movie", "show", "season", "episode", "special"},
		"company":      {"movie", "show", "season", "episode", "special"},
		"network":      {"show", "season", "episode", "special", "recording"},
		"actor":        {"movie", "show", "season", "episode", "special"},
		"director":     {"movie", "show", "season", "episode", "special"},
		"writer":       {"movie", "show", "season", "episode", "special"},
		"creator":      {"show", "season", "episode"},
		"credit":       {"movie", "show", "season", "episode", "special", "artist", "album", "track", "book", "recording"},
		"label":        {"artist", "album", "track"},
		"resolution":   {"movie", "episode", "special", "recording"},
		"dynamicRange": {"movie", "episode", "special", "recording"},
		"source":       {"movie", "episode", "special", "track", "book", "recording"},
		"mediaVersion": {"movie", "episode", "special", "recording"},
	}
	result := make([]BrowseFieldCapability, 0, len(browsecontract.Fields()))
	for _, field := range browsecontract.Fields() {
		capability := BrowseFieldCapability{
			ID: field.ID, Label: labels[field.ID], ValueType: string(field.ValueType),
			Operators:       append([]string(nil), field.Operators...),
			AllowedValues:   append([]string(nil), field.AllowedValues...),
			ApplicableKinds: append([]string(nil), applicableKinds[field.ID]...),
			ControlHint:     browseControlHint(field), Complexity: browseFieldComplexity(field.ID), Cost: browseFieldCost(field.ID),
		}
		if field.ID == "genre" || field.ID == "tag" || field.ID == "label" {
			capability.FacetSource = canonicalFacetSource(field.ID)
		}
		result = append(result, capability)
	}
	return result
}

func browseControlHint(field browsecontract.Field) string {
	switch field.ValueType {
	case browsecontract.ValueBoolean:
		return "toggle"
	case browsecontract.ValueEnum:
		return "select"
	case browsecontract.ValueNumber, browsecontract.ValueDuration:
		return "number-range"
	case browsecontract.ValueDate:
		return "date-range"
	case browsecontract.ValueIdentitySet:
		return "facet-multi-select"
	default:
		return "text"
	}
}

func browseFieldComplexity(id string) string {
	switch id {
	case "entityKind", "title", "genre", "playState", "favorite", "watchlisted":
		return "quick"
	case "year", "decade", "dateAdded", "releaseDate", "contentRating", "availability", "personalRating", "studio", "network":
		return "standard"
	default:
		return "advanced"
	}
}

func browseFieldCost(id string) string {
	switch id {
	case "actor", "director", "writer", "creator", "resolution", "dynamicRange", "source", "mediaVersion":
		return "indexed-join"
	default:
		return "indexed"
	}
}

func canonicalFacetSource(group string) *BrowseFacetSource {
	return &BrowseFacetSource{
		EndpointTemplate: "/api/libraries/{libraryId}/categories",
		FilterField:      "filter", FilterPrefix: group + ":",
		ValueField: "name", LabelField: "name", CountField: "count",
	}
}

func nonNilStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func canonicalBrowseSorts() []BrowseSortCapability {
	both := []string{"asc", "desc"}
	return []BrowseSortCapability{
		{ID: "title", Label: "Title", Directions: both, DefaultDirection: "asc"},
		{ID: "sortTitle", Label: "Sort title", Directions: both, DefaultDirection: "asc"},
		{ID: "dateAdded", Label: "Date added", Directions: both, DefaultDirection: "desc"},
		{ID: "releaseDate", Label: "Release date", Directions: both, DefaultDirection: "desc"},
		{ID: "year", Label: "Year", Directions: both, DefaultDirection: "desc"},
		{ID: "communityRating", Label: "Community rating", Directions: both, DefaultDirection: "desc"},
		{ID: "criticRating", Label: "Critic rating", Directions: both, DefaultDirection: "desc"},
		{ID: "durationSeconds", Label: "Duration", Directions: both, DefaultDirection: "desc"},
		{ID: "lastPlayedAt", Label: "Last played", Directions: both, DefaultDirection: "desc"},
		{ID: "seasonNumber", Label: "Season", Directions: both, DefaultDirection: "asc", ApplicableKinds: []string{"episode"}},
		{ID: "episodeNumber", Label: "Episode", Directions: both, DefaultDirection: "asc", ApplicableKinds: []string{"episode"}},
		{ID: "artist", Label: "Artist", Directions: both, DefaultDirection: "asc", ApplicableKinds: []string{"artist", "album", "track"}},
		{ID: "album", Label: "Album", Directions: both, DefaultDirection: "asc", ApplicableKinds: []string{"album", "track"}},
		{ID: "trackNumber", Label: "Track", Directions: both, DefaultDirection: "asc", ApplicableKinds: []string{"track"}},
		{ID: "author", Label: "Author", Directions: both, DefaultDirection: "asc", ApplicableKinds: []string{"book"}},
	}
}

func canonicalBrowseOperators() []BrowseOperator {
	order := []string{"equals", "not-equals", "contains", "not-contains", "starts-with", "in", "not-in", "less-than", "at-most", "greater-than", "at-least", "between", "is", "contains-any", "contains-all", "is-present", "is-missing"}
	valueTypes := map[string][]string{}
	for _, field := range browsecontract.Fields() {
		for _, operator := range field.Operators {
			valueType := string(field.ValueType)
			if operator == "is-present" || operator == "is-missing" {
				valueType = string(browsecontract.ValuePresence)
			}
			if !containsString(valueTypes[operator], valueType) {
				valueTypes[operator] = append(valueTypes[operator], valueType)
			}
		}
	}
	result := make([]BrowseOperator, 0, len(order))
	for _, operator := range order {
		if types := valueTypes[operator]; len(types) > 0 {
			result = append(result, BrowseOperator{ID: operator, ValueTypes: types})
		}
	}
	return result
}

func canonicalPresentationFields() []string {
	return []string{
		"year", "durationSeconds", "contentRating", "communityRating", "criticRating",
		"playState", "progressSeconds", "availability", "parentTitle", "seasonNumber", "episodeNumber",
	}
}

func canonicalBrowseQueryLimits() BrowseQueryLimits {
	return BrowseQueryLimits{
		MaximumDepth: browsecontract.MaximumDepth, MaximumClauses: browsecontract.MaximumClauses,
		MaximumBytes: browsecontract.MaximumBytes,
		DefaultLimit: browseDefaultLimit, MaximumLimit: browseMaximumLimit,
		CursorTTLSeconds: int(cursorDefaultTTL.Seconds()),
	}
}

func canonicalLibraryKind(storageKind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(storageKind)) {
	case "movie", "movies":
		return "movies", true
	case "show", "tv", "tv-shows":
		return "tv", true
	case "anime":
		return "anime", true
	case "music":
		return "music", true
	case "audiobook", "audiobooks":
		return "audiobooks", true
	case "recorded-tv":
		return "recorded-tv", true
	default:
		return "", false
	}
}

func productLibraryKindByID(id string) (ProductLibraryKind, bool) {
	for _, kind := range productLibraryKinds() {
		if kind.ID == id {
			return kind, true
		}
	}
	return ProductLibraryKind{}, false
}

func browsePivotByID(kind ProductLibraryKind, id string) (BrowsePivotCapability, bool) {
	for _, pivot := range kind.Pivots {
		if pivot.ID == id {
			return pivot, true
		}
	}
	return BrowsePivotCapability{}, false
}

func (s *Server) handleProductContract(w http.ResponseWriter, r *http.Request, _ User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	// The contract embeds the active build identity. Reusing it across an atomic
	// release switch can pair build N's contract with build N+1's System response
	// and correctly trip the client's fail-closed compatibility check.
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, s.canonicalProductContract())
}

func (s *Server) handleLibraryBrowseCapabilities(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	libraryID, ok := libraryIDFromCanonicalBrowsePath(r.URL.Path, "browse-capabilities")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "Library browse capabilities were not found.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), navigationRequestTimeout)
	defer cancel()
	library, kind, ok := s.authorizedCanonicalLibrary(ctx, user, libraryID, w)
	if !ok {
		return
	}
	actions := []string{"browse"}
	if canInteractivelyManageServer(user) {
		actions = append(actions, "manageLibrary", "scanLibrary")
	}
	fields := canonicalBrowseFields()
	sorts := canonicalBrowseSorts()
	presentationFields := canonicalPresentationFields()
	var resolvedPivot *BrowsePivotCapability
	if pivotID := strings.TrimSpace(r.URL.Query().Get("pivot")); pivotID != "" {
		pivot, exists := browsePivotByID(kind, pivotID)
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "unsupported_browse_pivot", "The requested pivot is not available for this library.")
			return
		}
		resolvedPivot = &pivot
		presentationFields = append([]string(nil), pivot.PresentationFields...)
		if pivot.BrowseSupported {
			fields = browseFieldsForPivot(pivot)
			sorts = browseSortsForPivot(pivot)
		} else {
			fields = []BrowseFieldCapability{}
			sorts = []BrowseSortCapability{}
			actions = removeString(actions, "browse")
		}
	}
	writeJSON(w, http.StatusOK, LibraryBrowseCapabilities{
		APIVersion:         productContractRevision,
		Library:            LibraryBrowseScope{ID: library.ID, Name: library.Name, Kind: kind.ID},
		Pivots:             kind.Pivots,
		ResolvedPivot:      resolvedPivot,
		Fields:             fields,
		Sorts:              sorts,
		PresentationFields: presentationFields,
		Actions:            actions,
		QueryLimits:        canonicalBrowseQueryLimits(),
	})
}

func removeString(values []string, remove string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != remove {
			result = append(result, value)
		}
	}
	return result
}

func browseFieldsForPivot(pivot BrowsePivotCapability) []BrowseFieldCapability {
	fields := make([]BrowseFieldCapability, 0, len(canonicalBrowseFields()))
	for _, field := range canonicalBrowseFields() {
		if len(field.ApplicableKinds) > 0 && !stringSlicesIntersect(field.ApplicableKinds, pivot.EntityKinds) {
			continue
		}
		if field.ID == "entityKind" {
			field.AllowedValues = append([]string(nil), pivot.EntityKinds...)
		}
		fields = append(fields, field)
	}
	return fields
}

func browseSortsForPivot(pivot BrowsePivotCapability) []BrowseSortCapability {
	sorts := make([]BrowseSortCapability, 0, len(canonicalBrowseSorts()))
	for _, item := range canonicalBrowseSorts() {
		if len(item.ApplicableKinds) == 0 || stringSlicesIntersect(item.ApplicableKinds, pivot.EntityKinds) {
			sorts = append(sorts, item)
		}
	}
	return sorts
}

func stringSlicesIntersect(left, right []string) bool {
	for _, candidate := range left {
		for _, value := range right {
			if candidate == value {
				return true
			}
		}
	}
	return false
}

func (s *Server) authorizedCanonicalLibrary(ctx context.Context, user User, libraryID string, w http.ResponseWriter) (Library, ProductLibraryKind, bool) {
	allowed, err := s.libraryAccessAllowedContext(ctx, user, libraryID)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "library_access_failed", "Unable to inspect library access.")
		return Library{}, ProductLibraryKind{}, false
	}
	// Do not distinguish an inaccessible library from an unknown library.
	if !allowed {
		writeError(w, http.StatusNotFound, "library_not_found", "Library was not found.")
		return Library{}, ProductLibraryKind{}, false
	}
	library, err := s.getCanonicalLibraryScopeContext(ctx, libraryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "library_not_found", "Library was not found.")
		} else {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "library_access_failed", "Unable to load the library.")
		}
		return Library{}, ProductLibraryKind{}, false
	}
	kindID, supported := canonicalLibraryKind(library.Type)
	if !supported {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_library_kind", "This library kind does not support canonical browsing.")
		return Library{}, ProductLibraryKind{}, false
	}
	kind, exists := productLibraryKindByID(kindID)
	if !exists {
		writeError(w, http.StatusInternalServerError, "product_contract_invalid", "Library capabilities are unavailable.")
		return Library{}, ProductLibraryKind{}, false
	}
	return library, kind, true
}

// Canonical browse only needs the stable identity and kind of a library. Avoid
// loading source paths and scan summaries here: those are administrative
// projections and would add unrelated database work to every consumer page.
func (s *Server) getCanonicalLibraryScopeContext(ctx context.Context, libraryID string) (Library, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var library Library
	err := s.queryUserRow(ctx, `
		SELECT id, name, type
		FROM libraries
		WHERE id = ?
		LIMIT 1`, strings.TrimSpace(libraryID)).Scan(&library.ID, &library.Name, &library.Type)
	if err != nil {
		return Library{}, err
	}
	return library, nil
}

func libraryIDFromCanonicalBrowsePath(path, resource string) (string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/libraries/"), "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != resource {
		return "", false
	}
	return parts[0], true
}

var errUnsupportedBrowsePivot = errors.New("unsupported browse pivot")
