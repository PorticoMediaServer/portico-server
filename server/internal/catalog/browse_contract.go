package catalog

import (
	"errors"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/personalstate"
	"sort"
	"strings"
)

// The browse vocabulary is server-owned. Capabilities, expression validation,
// SQL compilation, facets and saved-view validation all read this one table, so
// a field can never be advertised without being executable.

const (
	BrowseMaximumDepth    = 5
	BrowseMaximumClauses  = 40
	BrowseMaximumBytes    = 64 << 10
	BrowseDefaultLimit    = 40
	BrowseMaximumLimit    = 100
	BrowseCursorTTLSecond = 1800
	BrowseMaximumSorts    = 3
	BrowseMaximumValues   = 100
	browseFacetLimit      = 200
)

type browseValueType string

const (
	valueString  browseValueType = "string"
	valueEnum    browseValueType = "enum"
	valueNumber  browseValueType = "number"
	valueDate    browseValueType = "date"
	valueBoolean browseValueType = "boolean"
	valueSet     browseValueType = "identity-set"
)

// browseField describes one queryable fact. Entity fields compile to a scalar
// expression over the browse_entities row; item fields compile to an EXISTS over
// the entity's member items, which is what makes one predicate work for a movie,
// a show and an artist without a separate query per library kind.
type browseField struct {
	ID              string
	LabelKey        string
	Type            browseValueType
	Operators       []string
	ControlHint     string
	Complexity      string
	Cost            string
	ApplicableKinds []string
	AllowedValues   []string
	Facet           string // facet field id when the values are counted
	// Entity is the scalar SQL over `e` for entity-scoped fields.
	Entity string
	// Join and Value describe an item-scoped value set. Join is appended after
	// `FROM catalog_browse_memberships bei` and may reference bei.item_id (the
	// INTEGER entity id).
	Join  string
	Value string
	// Profile marks a join that needs the viewer profile key bound first.
	Profile int
}

func stringOperators(set bool) []string {
	if set {
		return []string{"contains", "contains-any", "contains-all", "equals", "not-equals", "starts-with", "in", "not-in", "is-present", "is-missing"}
	}
	return []string{"equals", "not-equals", "contains", "starts-with", "in", "not-in", "is-present", "is-missing"}
}

var numberOperators = []string{"equals", "not-equals", "less-than", "at-most", "greater-than", "at-least", "between", "in", "not-in", "is-present", "is-missing"}
var dateOperators = []string{"equals", "not-equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}
var enumOperators = []string{"equals", "not-equals", "in", "not-in"}

const (
	videoKinds   = "movie,episode,show,season,collection"
	audioKinds   = "song,album,artist"
	bookKinds    = "audiobook_file,book,author"
	commonKinds  = "movie,episode,show,season,collection,song,album,artist,audiobook_file,book,author"
	playedKinds  = "movie,episode,show,season,collection,song,album,artist,audiobook_file,book"
	assetedKinds = "movie,episode,show,season,collection,song,album,artist,audiobook_file,book"
)

func kinds(list string) []string { return strings.Split(list, ",") }

// attributeJoin builds the member-item join for a descriptive attribute kept in
// the compact attribute edges (catalog_item_attribute_edges, one row per value
// with the original spelling in source_value).
func attributeJoin(field string) string {
	return `JOIN catalog_item_attribute_edges ca ON ca.item_id=bei.item_id JOIN catalog_attribute_terms ca_term ON ca_term.id=ca.term_id AND ca_term.field_id=(SELECT id FROM catalog_attribute_fields WHERE field='` + field + `')`
}

var browseFields = []browseField{
	{ID: "entityKind", LabelKey: "browse.field.entityKind", Type: valueEnum, Operators: enumOperators, ControlHint: "select", Complexity: "quick", Cost: "indexed", ApplicableKinds: kinds(commonKinds), AllowedValues: []string{"movie", "episode", "show", "season", "song", "album", "artist", "audiobook_file", "book", "author", "collection"}, Entity: `CASE e.kind WHEN 1 THEN 'movie' WHEN 2 THEN 'show' WHEN 3 THEN 'season' WHEN 4 THEN 'episode' WHEN 5 THEN 'artist' WHEN 6 THEN 'album' WHEN 7 THEN 'song' WHEN 8 THEN 'book' WHEN 9 THEN 'audiobook_file' WHEN 10 THEN 'collection' WHEN 11 THEN 'extra' WHEN 12 THEN 'disc' WHEN 13 THEN 'author' END`},
	// Match: whether an album's files carried tags. An untagged file lands under the placeholder
	// artist the scanner makes ("Unknown artist"); the owner's Unmatched view of a music library
	// lists those albums. Clients do not offer it as a filter to members.
	{ID: "match", LabelKey: "browse.field.match", Type: valueEnum, Operators: enumOperators, ControlHint: "select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds("album"), AllowedValues: []string{"matched", "unmatched"},
		Entity: `CASE WHEN EXISTS(SELECT 1 FROM catalog_albums al JOIN catalog_entities ar ON ar.id=al.artist_id WHERE al.entity_id=e.entity_id AND ar.title='Unknown artist') THEN 'unmatched' ELSE 'matched' END`},
	{ID: "title", LabelKey: "browse.field.title", Type: valueString, Operators: stringOperators(false), ControlHint: "text", Complexity: "quick", Cost: "indexed", ApplicableKinds: kinds(commonKinds), Entity: "e.title"},
	{ID: "year", LabelKey: "browse.field.year", Type: valueNumber, Operators: numberOperators, ControlHint: "number-range", Complexity: "quick", Cost: "indexed", ApplicableKinds: kinds("movie,episode,show,season,album"), Facet: "year", Entity: "NULLIF(e.year,0)"},
	{ID: "decade", LabelKey: "browse.field.decade", Type: valueNumber, Operators: []string{"equals", "not-equals", "in", "not-in", "between"}, ControlHint: "facet-multi-select", Complexity: "quick", Cost: "indexed", ApplicableKinds: kinds("movie,episode,show,season,album"), Facet: "decade", Entity: "CASE WHEN e.year BETWEEN 1800 AND 2199 THEN (e.year/10)*10 END"},
	{ID: "releaseDate", LabelKey: "browse.field.releaseDate", Type: valueDate, Operators: dateOperators, ControlHint: "date-range", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds), Join: attributeJoin("releaseDate"), Value: "ca.source_value"},
	{ID: "dateAdded", LabelKey: "browse.field.dateAdded", Type: valueDate, Operators: dateOperators, ControlHint: "date-range", Complexity: "quick", Cost: "indexed", ApplicableKinds: kinds(commonKinds), Entity: "NULLIF(e.added_text,'')"},
	{ID: "playState", LabelKey: "browse.field.playState", Type: valueEnum, Operators: enumOperators, ControlHint: "select", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(playedKinds), AllowedValues: []string{"unplayed", "in-progress", "played"},
		Join:  `CROSS JOIN (SELECT ? AS profile) pscope LEFT JOIN progress pr ON pr.item_id=bei.item_id AND pr.profile_id=?`,
		Value: `CASE WHEN ` + personalstate.CompactSQL("pscope.profile", "bei.item_id") + `=1 THEN 'played' WHEN COALESCE(pr.position,0)>0 THEN 'in-progress' ELSE 'unplayed' END`, Profile: 2},
	{ID: "favorite", LabelKey: "browse.field.favorite", Type: valueBoolean, Operators: []string{"equals", "not-equals"}, ControlHint: "toggle", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(playedKinds),
		Join: `JOIN personal_items pi ON pi.item_id=bei.item_id AND pi.profile_id=?`, Value: "pi.favorite", Profile: 1},
	{ID: "watchlisted", LabelKey: "browse.field.watchlisted", Type: valueBoolean, Operators: []string{"equals", "not-equals"}, ControlHint: "toggle", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(playedKinds),
		Join: `JOIN personal_items pi ON pi.item_id=bei.item_id AND pi.profile_id=?`, Value: "pi.watchlisted", Profile: 1},
	{ID: "personalRating", LabelKey: "browse.field.personalRating", Type: valueNumber, Operators: numberOperators, ControlHint: "number-range", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(playedKinds),
		Join: `JOIN personal_items pi ON pi.item_id=bei.item_id AND pi.profile_id=?`, Value: "pi.rating", Profile: 1},
	{ID: "lastPlayedAt", LabelKey: "browse.field.lastPlayedAt", Type: valueDate, Operators: dateOperators, ControlHint: "date-range", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(playedKinds),
		Join: `JOIN personal_items pi ON pi.item_id=bei.item_id AND pi.profile_id=?`, Value: "NULLIF(pi.last_played_at,'')", Profile: 1},
	{ID: "genre", LabelKey: "browse.field.genre", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(commonKinds), Facet: "genre",
		// A show owns its genres: an episode matches through its show, and a show's row through itself.
		Join: `JOIN catalog_term_sources ts ON ts.entity_id IN(bei.item_id,bei.entity_id,(SELECT ep.show_id FROM catalog_episodes ep WHERE ep.entity_id=bei.item_id)) JOIN catalog_terms mg ON mg.id=ts.term_id AND mg.vocab=1`, Value: "COALESCE(ts.label_override,mg.label)"},
	{ID: "tag", LabelKey: "browse.field.tag", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(commonKinds), Facet: "tag",
		Join: attributeJoin("tag"), Value: "ca.source_value"},
	{ID: "label", LabelKey: "browse.field.label", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(commonKinds), Facet: "label",
		Join: attributeJoin("label"), Value: "ca.source_value"},
	{ID: "collection", LabelKey: "browse.field.collection", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(commonKinds), Facet: "collection",
		Join: `JOIN catalog_collection_members cim ON cim.item_id=bei.item_id JOIN catalog_entities col ON col.id=cim.collection_id`, Value: "pid(col.public_id)"},
	{ID: "contentRating", LabelKey: "browse.field.contentRating", Type: valueString, Operators: stringOperators(false), ControlHint: "facet-multi-select", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds), Facet: "contentRating",
		Join: attributeJoin("contentRating"), Value: "ca.source_value"},
	{ID: "communityRating", LabelKey: "browse.field.communityRating", Type: valueNumber, Operators: numberOperators, ControlHint: "number-range", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(commonKinds),
		Join: `JOIN metadata_ratings mr ON mr.item_id=bei.item_id AND mr.provider IN('tmdb','imdb','musicbrainz','audible')`, Value: "mr.value"},
	{ID: "criticRating", LabelKey: "browse.field.criticRating", Type: valueNumber, Operators: numberOperators, ControlHint: "number-range", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds),
		Join: `JOIN metadata_ratings mr ON mr.item_id=bei.item_id AND mr.provider IN('metacritic','rottentomatoes')`, Value: "mr.value"},
	{ID: "availability", LabelKey: "browse.field.availability", Type: valueEnum, Operators: enumOperators, ControlHint: "select", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(assetedKinds), AllowedValues: []string{"available", "partial", "unavailable"},
		Entity: `CASE WHEN NOT EXISTS(SELECT 1 FROM catalog_browse_memberships bei JOIN catalog_asset_links iav ON iav.entity_id=bei.item_id WHERE bei.entity_id=e.entity_id AND iav.available=1) THEN 'unavailable' WHEN EXISTS(SELECT 1 FROM catalog_browse_memberships bei JOIN catalog_asset_links iav ON iav.entity_id=bei.item_id WHERE bei.entity_id=e.entity_id AND iav.available=0) THEN 'partial' ELSE 'available' END`},
	{ID: "resolution", LabelKey: "browse.field.resolution", Type: valueEnum, Operators: enumOperators, ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds), AllowedValues: []string{"sd", "720p", "1080p", "4k"}, Facet: "resolution",
		Join:  `JOIN catalog_asset_links ia ON ia.entity_id=bei.item_id JOIN catalog_assets ast ON ast.id=ia.asset_id`,
		Value: `CASE WHEN ast.height>=2000 THEN '4k' WHEN ast.height>=1000 THEN '1080p' WHEN ast.height>=700 THEN '720p' ELSE 'sd' END`},
	{ID: "dynamicRange", LabelKey: "browse.field.dynamicRange", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds),
		Join: attributeJoin("dynamicRange"), Value: "ca.source_value"},
	{ID: "audioLanguage", LabelKey: "browse.field.audioLanguage", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(commonKinds), Facet: "audioLanguage",
		Join: attributeJoin("audioLanguage"), Value: "ca.source_value"},
	{ID: "studio", LabelKey: "browse.field.studio", Type: valueString, Operators: stringOperators(false), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds), Facet: "studio",
		Join: attributeJoin("studio"), Value: "ca.source_value"},
	{ID: "network", LabelKey: "browse.field.network", Type: valueString, Operators: stringOperators(false), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds("show,season,episode"), Facet: "network",
		Join: attributeJoin("network"), Value: "ca.source_value"},
	{ID: "actor", LabelKey: "browse.field.actor", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "advanced", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds),
		Join: `JOIN catalog_credits cc ON cc.entity_id=bei.item_id JOIN catalog_credit_labels cd ON cd.id=cc.department_id AND cd.label='Acting'`, Value: "cc.credited_name"},
	{ID: "director", LabelKey: "browse.field.director", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "advanced", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds),
		Join: `JOIN catalog_credits cc ON cc.entity_id=bei.item_id JOIN catalog_credit_labels cr ON cr.id=cc.role_id JOIN catalog_credit_labels cd ON cd.id=cc.department_id AND (cr.label='Director' OR cd.label='Directing')`, Value: "cc.credited_name"},
	{ID: "writer", LabelKey: "browse.field.writer", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "advanced", Cost: "indexed-join", ApplicableKinds: kinds(videoKinds),
		Join: `JOIN catalog_credits cc ON cc.entity_id=bei.item_id JOIN catalog_credit_labels cr ON cr.id=cc.role_id JOIN catalog_credit_labels cd ON cd.id=cc.department_id AND (cr.label IN('Writer','Screenplay') OR cd.label='Writing')`, Value: "cc.credited_name"},
	{ID: "durationSeconds", LabelKey: "browse.field.durationSeconds", Type: valueNumber, Operators: numberOperators, ControlHint: "number-range", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(assetedKinds),
		Join: `JOIN catalog_asset_links ia ON ia.entity_id=bei.item_id JOIN catalog_assets ast ON ast.id=ia.asset_id`, Value: "ast.duration"},
	{ID: "artist", LabelKey: "browse.field.artist", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(audioKinds),
		Join: `JOIN catalog_song_artists sar ON sar.song_id=bei.item_id JOIN catalog_entities art ON art.id=sar.artist_id`, Value: "art.title"},
	{ID: "album", LabelKey: "browse.field.album", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(audioKinds),
		Join: `JOIN catalog_songs sng ON sng.entity_id=bei.item_id JOIN catalog_entities alb ON alb.id=sng.album_id`, Value: "alb.title"},
	{ID: "author", LabelKey: "browse.field.author", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(bookKinds),
		Join: `JOIN catalog_book_files bfl ON bfl.entity_id=bei.item_id JOIN catalog_books bok ON bok.entity_id=bfl.book_id`, Value: "NULLIF(bok.author,'')"},
	{ID: "narrator", LabelKey: "browse.field.narrator", Type: valueSet, Operators: stringOperators(true), ControlHint: "text", Complexity: "quick", Cost: "indexed-join", ApplicableKinds: kinds(bookKinds),
		Join: `JOIN catalog_book_files bfl ON bfl.entity_id=bei.item_id JOIN catalog_books bok ON bok.entity_id=bfl.book_id`, Value: "NULLIF(bok.narrator,'')"},
	{ID: "series", LabelKey: "browse.field.series", Type: valueSet, Operators: stringOperators(true), ControlHint: "facet-multi-select", Complexity: "standard", Cost: "indexed-join", ApplicableKinds: kinds(bookKinds), Facet: "series",
		Join: attributeJoin("series"), Value: "ca.source_value"},
}

func browseFieldByID(id string) (browseField, bool) {
	for _, field := range browseFields {
		if field.ID == id {
			return field, true
		}
	}
	return browseField{}, false
}

// browseSort names a sortable scalar. A column sort has a counted block
// ordering (compactcatalog.Order*) that serves any position, and the same key
// on a visibility class row (ClassKey, over v). A profile sort reads the
// viewer's personal state and has neither: it pages its valued rows from the
// profile's own history and the rest in title order (browse_personal.go).
type browseSort struct {
	ID               string
	LabelKey         string
	Directions       []string
	DefaultDirection string
	ApplicableKinds  []string
	Expression       string
	Ordering         int
	ClassKey         string
	Profile          bool
}

var browseSorts = []browseSort{
	{ID: "title", LabelKey: "sort.title", Directions: []string{"asc", "desc"}, DefaultDirection: "asc", ApplicableKinds: kinds(commonKinds), Expression: "e.sort_key COLLATE NOCASE", Ordering: compactcatalog.OrderTitle, ClassKey: "v.sort_key COLLATE NOCASE"},
	{ID: "added", LabelKey: "sort.added", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds(commonKinds), Expression: "COALESCE(e.added_text,'')", Ordering: compactcatalog.OrderAdded, ClassKey: "v.added"},
	{ID: "year", LabelKey: "sort.year", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds("movie,episode,show,season,album"), Expression: "e.year", Ordering: compactcatalog.OrderYear, ClassKey: "v.year"},
	{ID: "duration", LabelKey: "sort.duration", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds(assetedKinds), Expression: "COALESCE(e.duration_max,0)", Ordering: compactcatalog.OrderDuration, ClassKey: "v.duration"},
	{ID: "communityRating", LabelKey: "sort.communityRating", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds(commonKinds), Expression: "COALESCE(e.rating_max,0)", Ordering: compactcatalog.OrderRating, ClassKey: "v.rating"},
	{ID: "personalRating", LabelKey: "sort.personalRating", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds(playedKinds), Profile: true, Ordering: -1,
		Expression: "(SELECT COALESCE(max(pi.rating),0) FROM catalog_browse_memberships bei JOIN personal_items pi ON pi.item_id=bei.item_id AND pi.profile_id=? WHERE bei.entity_id=e.entity_id)"},
	// For you: the viewer's recommendation ranking of the pivot (rec_browse.go),
	// then the rest in title order. It orders on its own (no second sort).
	{ID: "forYou", LabelKey: "sort.forYou", Directions: []string{"desc"}, DefaultDirection: "desc", ApplicableKinds: kinds("movie,show,album,book"), Profile: true, Ordering: -1,
		Expression: "(SELECT 0 WHERE ? IS NOT NULL)"},
	// Aired: an episode's air date (the item's release date), for "Recently aired". It is an
	// expression over the item's details, not a column of the browse rows: it has no counted
	// ordering, so a page is ordered by a pass over the library's episodes (Backlog: give it one).
	{ID: "aired", LabelKey: "sort.aired", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds("episode"), Ordering: -1,
		Expression: "COALESCE((SELECT d.release_date FROM catalog_item_details d WHERE d.entity_id=e.entity_id),'')"},
	// Latest aired: a show by its most recently aired episode, for "Recently aired": one card
	// per show, however many of its episodes are new (Justin, 2 Oct 2026). Like `aired` it is an
	// expression with no counted ordering; a page is ordered by a pass over the library's episodes.
	{ID: "latestAired", LabelKey: "sort.aired", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds("show"), Ordering: -1,
		Expression: "COALESCE((SELECT max(d.release_date) FROM catalog_episodes ep JOIN catalog_item_details d ON d.entity_id=ep.entity_id WHERE ep.show_id=e.entity_id),'')"},
	{ID: "lastPlayed", LabelKey: "sort.lastPlayed", Directions: []string{"asc", "desc"}, DefaultDirection: "desc", ApplicableKinds: kinds(playedKinds), Profile: true, Ordering: -1,
		Expression: "(SELECT COALESCE(max(pi.last_played_at),'') FROM catalog_browse_memberships bei JOIN personal_items pi ON pi.item_id=bei.item_id AND pi.profile_id=? WHERE bei.entity_id=e.entity_id)"},
}

func browseSortByID(id string) (browseSort, bool) {
	for _, entry := range browseSorts {
		if entry.ID == id {
			return entry, true
		}
	}
	return browseSort{}, false
}

// browsePivot binds one navigational surface to the entity rows it selects.
type browsePivot struct {
	ID             string
	LabelKey       string
	EntityKinds    []string
	SupportedViews []string
	DefaultSort    []BrowseSortSelection
	// Aggregate pivots project a facet (decade, genre, series) instead of entities.
	Aggregate string
	Browsable bool
}

var browsePivots = map[string][]browsePivot{
	"movie": {
		{ID: "discover", LabelKey: "library.discover", EntityKinds: []string{"movie"}, SupportedViews: []string{"rail"}, DefaultSort: titleAscending()},
		{ID: "movies", LabelKey: "library.movies", EntityKinds: []string{"movie"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "collections", LabelKey: "library.collections", EntityKinds: []string{"collection"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "categories", LabelKey: "library.categories", EntityKinds: []string{"category"}, SupportedViews: []string{"grid"}, DefaultSort: titleAscending(), Aggregate: "decade"},
	},
	"tv": {
		{ID: "discover", LabelKey: "library.discover", EntityKinds: []string{"show"}, SupportedViews: []string{"rail"}, DefaultSort: titleAscending()},
		{ID: "shows", LabelKey: "library.shows", EntityKinds: []string{"show"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "episodes", LabelKey: "library.episodes", EntityKinds: []string{"episode"}, SupportedViews: []string{"list", "grid"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "collections", LabelKey: "library.collections", EntityKinds: []string{"collection"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "categories", LabelKey: "library.categories", EntityKinds: []string{"category"}, SupportedViews: []string{"grid"}, DefaultSort: titleAscending(), Aggregate: "decade"},
	},
	"music": {
		{ID: "discover", LabelKey: "library.discover", EntityKinds: []string{"artist"}, SupportedViews: []string{"rail"}, DefaultSort: titleAscending()},
		{ID: "artists", LabelKey: "library.artists", EntityKinds: []string{"artist"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "albums", LabelKey: "library.albums", EntityKinds: []string{"album"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "songs", LabelKey: "library.songs", EntityKinds: []string{"song"}, SupportedViews: []string{"list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "genres", LabelKey: "library.genres", EntityKinds: []string{"category"}, SupportedViews: []string{"grid"}, DefaultSort: titleAscending(), Aggregate: "genre"},
	},
	"audiobook": {
		{ID: "discover", LabelKey: "library.discover", EntityKinds: []string{"book"}, SupportedViews: []string{"rail"}, DefaultSort: titleAscending()},
		{ID: "authors", LabelKey: "library.authors", EntityKinds: []string{"author"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "books", LabelKey: "library.books", EntityKinds: []string{"book"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
		{ID: "series", LabelKey: "library.series", EntityKinds: []string{"book_series"}, SupportedViews: []string{"grid"}, DefaultSort: titleAscending(), Aggregate: "series"},
		{ID: "collections", LabelKey: "library.collections", EntityKinds: []string{"collection"}, SupportedViews: []string{"grid", "list"}, DefaultSort: titleAscending(), Browsable: true},
	},
}

func titleAscending() []BrowseSortSelection {
	return []BrowseSortSelection{{Field: "title", Direction: "asc"}}
}

// anime shares the television pivot set; the kind only changes presentation.
func pivotsForKind(kind string) []browsePivot {
	if kind == "anime" {
		kind = "tv"
	}
	return browsePivots[kind]
}

func pivotForKind(kind, id string) (browsePivot, bool) {
	for _, pivot := range pivotsForKind(kind) {
		if pivot.ID == id {
			return pivot, true
		}
	}
	return browsePivot{}, false
}

// defaultPivot is the first browsable pivot of a library kind.
func defaultPivot(kind string) string {
	for _, pivot := range pivotsForKind(kind) {
		if pivot.Browsable {
			return pivot.ID
		}
	}
	return ""
}

// entityKindMembers maps a pivot's advertised kinds onto the compact kind names
// values stored in browse_entities.
func entityKindMembers(pivot browsePivot) []string {
	out := []string{}
	for _, kind := range pivot.EntityKinds {
		switch kind {
		case "song":
			out = append(out, "song")
		case "book":
			out = append(out, "book")
		default:
			out = append(out, kind)
		}
	}
	return out
}

// ---- published capability shapes ----

type BrowseFacetSource struct {
	Endpoint string `json:"endpoint"`
	Field    string `json:"field"`
}
type BrowseFieldCapability struct {
	ID              string             `json:"id"`
	LabelKey        string             `json:"labelKey"`
	Type            string             `json:"type"`
	Operators       []string           `json:"operators"`
	ControlHint     string             `json:"controlHint"`
	Complexity      string             `json:"complexity"`
	Cost            string             `json:"cost"`
	ApplicableKinds []string           `json:"applicableKinds"`
	AllowedValues   []string           `json:"allowedValues,omitempty"`
	FacetSource     *BrowseFacetSource `json:"facetSource,omitempty"`
}
type BrowseSortCapability struct {
	ID               string   `json:"id"`
	LabelKey         string   `json:"labelKey"`
	Directions       []string `json:"directions"`
	DefaultDirection string   `json:"defaultDirection"`
	Expensive        bool     `json:"expensive"`
	ApplicableKinds  []string `json:"applicableKinds"`
}
type BrowsePivotCapability struct {
	ID             string                `json:"id"`
	LabelKey       string                `json:"labelKey"`
	EntityKinds    []string              `json:"entityKinds"`
	DefaultSort    []BrowseSortSelection `json:"defaultSort"`
	SupportedViews []string              `json:"supportedViews"`
	Browsable      bool                  `json:"browsable"`
	Aggregate      string                `json:"aggregate,omitempty"`
}
type BrowseQuickFilter struct {
	ID       string      `json:"id"`
	LabelKey string      `json:"labelKey"`
	Query    *BrowseNode `json:"query"`
}
type BrowseQueryLimits struct {
	MaximumDepth     int `json:"maximumDepth"`
	MaximumClauses   int `json:"maximumClauses"`
	MaximumBytes     int `json:"maximumBytes"`
	MaximumSorts     int `json:"maximumSorts"`
	DefaultLimit     int `json:"defaultLimit"`
	MaximumLimit     int `json:"maximumLimit"`
	CursorTTLSeconds int `json:"cursorTtlSeconds"`
}
type BrowseCapabilities struct {
	Library       Library                 `json:"library"`
	Pivots        []BrowsePivotCapability `json:"pivots"`
	ResolvedPivot *BrowsePivotCapability  `json:"resolvedPivot,omitempty"`
	Fields        []BrowseFieldCapability `json:"fields"`
	Sorts         []BrowseSortCapability  `json:"sorts"`
	QuickFilters  []BrowseQuickFilter     `json:"quickFilters"`
	QueryLimits   BrowseQueryLimits       `json:"queryLimits"`
}

func quickFilter(id, key, field, operator string, value any) BrowseQuickFilter {
	return BrowseQuickFilter{ID: id, LabelKey: key, Query: &BrowseNode{Field: field, Operator: operator, Value: value}}
}

// BrowseQuickFilters are named presets of the same expression grammar; a client
// may send them verbatim or fold them into a larger query.
func BrowseQuickFilters() []BrowseQuickFilter {
	return []BrowseQuickFilter{
		quickFilter("in-progress", "filter.inProgress", "playState", "equals", "in-progress"),
		quickFilter("unwatched", "filter.unwatched", "playState", "equals", "unplayed"),
		quickFilter("watched", "filter.watched", "playState", "equals", "played"),
		quickFilter("favorites", "filter.favorites", "favorite", "equals", true),
		quickFilter("watchlist", "filter.watchlist", "watchlisted", "equals", true),
		quickFilter("available", "filter.available", "availability", "equals", "available"),
		quickFilter("missing", "filter.missing", "availability", "equals", "unavailable"),
	}
}

func browseQueryLimits() BrowseQueryLimits {
	return BrowseQueryLimits{MaximumDepth: BrowseMaximumDepth, MaximumClauses: BrowseMaximumClauses, MaximumBytes: BrowseMaximumBytes, MaximumSorts: BrowseMaximumSorts, DefaultLimit: BrowseDefaultLimit, MaximumLimit: BrowseMaximumLimit, CursorTTLSeconds: BrowseCursorTTLSecond}
}

// BrowseCapabilitiesFor publishes the vocabulary for one library, narrowed to a
// pivot when the caller names one.
func (s *Service) BrowseCapabilitiesFor(library, pivot string) (BrowseCapabilities, error) {
	out := BrowseCapabilities{Pivots: []BrowsePivotCapability{}, Fields: []BrowseFieldCapability{}, Sorts: []BrowseSortCapability{}, QuickFilters: BrowseQuickFilters(), QueryLimits: browseQueryLimits()}
	lib, err := s.library(library)
	if err != nil {
		return out, err
	}
	out.Library = lib
	available := pivotsForKind(lib.Kind)
	if len(available) == 0 {
		return out, errors.New("this library kind publishes no browse pivots")
	}
	for _, entry := range available {
		out.Pivots = append(out.Pivots, BrowsePivotCapability{ID: entry.ID, LabelKey: entry.LabelKey, EntityKinds: entry.EntityKinds, DefaultSort: entry.DefaultSort, SupportedViews: entry.SupportedViews, Browsable: entry.Browsable, Aggregate: entry.Aggregate})
	}
	kindFilter := map[string]bool{}
	if pivot != "" {
		resolved, ok := pivotForKind(lib.Kind, pivot)
		if !ok {
			return out, errors.New("pivot is not published for this library")
		}
		for index := range out.Pivots {
			if out.Pivots[index].ID == pivot {
				copied := out.Pivots[index]
				out.ResolvedPivot = &copied
			}
		}
		for _, kind := range entityKindMembers(resolved) {
			kindFilter[kind] = true
		}
	}
	for _, field := range browseFields {
		if len(kindFilter) > 0 && !anyKind(field.ApplicableKinds, kindFilter) {
			continue
		}
		capability := BrowseFieldCapability{ID: field.ID, LabelKey: field.LabelKey, Type: string(field.Type), Operators: field.Operators, ControlHint: field.ControlHint, Complexity: field.Complexity, Cost: field.Cost, ApplicableKinds: field.ApplicableKinds, AllowedValues: field.AllowedValues}
		if field.Facet != "" {
			capability.FacetSource = &BrowseFacetSource{Endpoint: "/v1/libraries/{id}/facets", Field: field.Facet}
		}
		out.Fields = append(out.Fields, capability)
	}
	for _, entry := range browseSorts {
		if len(kindFilter) > 0 && !anyKind(entry.ApplicableKinds, kindFilter) {
			continue
		}
		out.Sorts = append(out.Sorts, BrowseSortCapability{ID: entry.ID, LabelKey: entry.LabelKey, Directions: entry.Directions, DefaultDirection: entry.DefaultDirection, Expensive: entry.Profile, ApplicableKinds: entry.ApplicableKinds})
	}
	return out, nil
}

func anyKind(list []string, allowed map[string]bool) bool {
	for _, kind := range list {
		if allowed[kind] {
			return true
		}
	}
	return false
}

func (s *Service) library(id string) (Library, error) {
	var lib Library
	if err := s.compactProjectionReady(); err != nil {
		return lib, err
	}
	err := s.read().QueryRow(`SELECT id,name,kind FROM libraries WHERE id=?`, id).Scan(&lib.ID, &lib.Name, &lib.Kind)
	lib.DefaultView = DefaultLibraryView(lib.Kind)
	return lib, err
}

// BrowseFacetFields lists the fields the facet endpoint counts.
func BrowseFacetFields() []string {
	out := []string{}
	for _, field := range browseFields {
		if field.Facet != "" {
			out = append(out, field.Facet)
		}
	}
	sort.Strings(out)
	return out
}
