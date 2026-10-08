package catalog

import (
	"encoding/json"
	"time"

	"portico.local/server/internal/identity"
)

// Content is the canonical server-owned semantic projection. Geometry, focus,
// virtualization and localization belong to clients; membership and ordering do not.
type ContentScope struct {
	ServerID    string `json:"serverId"`
	LibraryID   string `json:"libraryId"`
	LibraryKind string `json:"libraryKind"`
	View        string `json:"view"`
	EntityID    string `json:"entityId"`
	ViewerFence string `json:"viewerFence"`
}
type ContentRevision struct {
	Catalog int64 `json:"catalog"`
	Viewer  int64 `json:"viewer"`
}
type ContentHeading struct {
	Key      string `json:"key"`
	Fallback string `json:"fallback"`
	// Params fill a parameterised heading ("{genre} for you"); absent for
	// fixed headings.
	Params map[string]string `json:"params,omitempty"`
}

// ServerText is a server-authored label a client names from its own catalogue
// (CON-19): code is the catalogue message id, params fill its placeholders,
// and fallback is the US English text (sentence case, except named features
// such as Continue Watching) for a client that doesn't know the code.
type ServerText struct {
	Code     string            `json:"code"`
	Params   map[string]string `json:"params,omitempty"`
	Fallback string            `json:"fallback"`
}

type ContentNavigation struct {
	View     string `json:"view"`
	EntityID string `json:"entityId,omitempty"`
	Category string `json:"category,omitempty"`
}
type ContentTab struct {
	ID       string `json:"id"`
	LabelKey string `json:"labelKey"`
	View     string `json:"view"`
}
type ContentSort struct {
	ID         string   `json:"id"`
	LabelKey   string   `json:"labelKey"`
	Directions []string `json:"directions"`
}
type ContentFilterOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
}
type ContentFilter struct {
	ID       string                `json:"id"`
	LabelKey string                `json:"labelKey"`
	Options  []ContentFilterOption `json:"options"`
}
type ContentPlayback struct {
	ItemID       string   `json:"itemId"`
	StartSeconds *float64 `json:"startSeconds,omitempty"`
}

// ContentEntityRef names one catalog entity (an album's artist) so a client
// can link to it without a second lookup.
type ContentEntityRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ContentEntry struct {
	WatchedCount   *int64        `json:"watchedCount,omitempty"`
	UnwatchedCount *int64        `json:"unwatchedCount,omitempty"`
	Watched        *bool         `json:"watched,omitempty"`
	TrackNumber    *int          `json:"trackNumber,omitempty"`
	EpisodeNumber  *int          `json:"episodeNumber,omitempty"`
	SeasonNumber   *int          `json:"seasonNumber,omitempty"`
	Hidden         *bool         `json:"hidden,omitempty"`
	Media          *ContentEntry `json:"media,omitempty"`
	LibraryID      string        `json:"libraryId,omitempty"`
	Overview       string        `json:"overview,omitempty"`
	BackdropURL    string        `json:"backdropUrl,omitempty"`
	// StillURL is an episode's own still, never inherited; clients draw the
	// placeholder when it is absent (Spec — Page Content §0.2).
	StillURL string `json:"stillUrl,omitempty"`
	// AirDate is an episode's air date (YYYY-MM-DD): show workspace pages, and
	// episode rows of a browse page ("Recently aired").
	AirDate string `json:"airDate,omitempty"`
	// LatestEpisode is a show's most recently aired episode, sent on a browse page
	// sorted by `latestAired` ("Recently aired"): the card is the show's, its
	// caption the episode's.
	LatestEpisode *LatestEpisode `json:"latestEpisode,omitempty"`
	// AbsoluteNumber is an episode's number across the show's seasons, sent
	// when the show is set to absolute numbering: it reads "Episode 1043".
	AbsoluteNumber *int `json:"absoluteNumber,omitempty"`
	// Rating is the community rating (0–10) and Resolution the best file's
	// picture class ("4k", "1080p", "720p", "sd"): browse pages only, for the
	// list view's columns. Absent when unknown.
	Rating     *float64 `json:"rating,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
	// OverviewSource names the biography source ("Wikipedia") when Overview is
	// provider text; absent for owner/local text. Artist entities only.
	OverviewSource string `json:"overviewSource,omitempty"`
	// OverviewSourceURL is the biography's source article. Artist entities only.
	OverviewSourceURL string `json:"overviewSourceUrl,omitempty"`
	// Country is the artist's MusicBrainz country code. Artist entities only.
	Country string `json:"country,omitempty"`
	// Label is the album's record label. Album entities only.
	Label string `json:"label,omitempty"`
	// AlbumType is the album's release-group type
	// (Album, EP, Single, Compilation). Album entities only.
	AlbumType string `json:"albumType,omitempty"`
	// ActiveBeginYear/ActiveEndYear are the artist's MusicBrainz active years;
	// a missing end means the artist is still active. Artist entities only.
	ActiveBeginYear *int              `json:"activeBeginYear,omitempty"`
	ActiveEndYear   *int              `json:"activeEndYear,omitempty"`
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	Title           string            `json:"title"`
	Subtitle        string            `json:"subtitle,omitempty"`
	PosterURL       string            `json:"posterUrl,omitempty"`
	Available       *bool             `json:"available,omitempty"`
	Duration        *float64          `json:"duration,omitempty"`
	Year            *int              `json:"year,omitempty"`
	Artist          *ContentEntityRef `json:"artist,omitempty"`
	// Author is a book's author, so its card can name them.
	Author string `json:"author,omitempty"`
	// Album is a song's album, so a track table can name and link it.
	Album           *ContentEntityRef  `json:"album,omitempty"`
	Role            string             `json:"role,omitempty"`
	ArtworkPaths    []string           `json:"artworkPaths,omitempty"`
	ProgressSeconds *float64           `json:"progressSeconds,omitempty"`
	AddedAt         *string            `json:"addedAt"`
	Count           *int               `json:"count,omitempty"`
	TotalAtLeast    *int               `json:"totalAtLeast,omitempty"`
	Navigation      *ContentNavigation `json:"navigation,omitempty"`
	Playback        *ContentPlayback   `json:"playback,omitempty"`
	// Home entries (M5): the hero's meta line and its Watchlist toggle; the
	// hero's year is Year above (shared with M14's album year).
	ContentRating string   `json:"contentRating,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Watchlisted   *bool    `json:"watchlisted,omitempty"`
	// UserRating is the viewer's own rating (0.5–5 in half steps) on an item
	// row; absent when the viewer has not rated it (NEW-24).
	UserRating *float64 `json:"userRating,omitempty"`
	// Plays is how many times the viewer has played a song to the end; absent
	// for a song never played and for every other kind.
	Plays *int `json:"plays,omitempty"`
}
type ContentSection struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Heading    ContentHeading `json:"heading"`
	Entries    []ContentEntry `json:"entries"`
	TotalCount int            `json:"totalCount"`
	NextCursor string         `json:"nextCursor"`
	// Start is the index of the first entry in the whole section: 0 unless the
	// request opened a grid at `start` (M10 random access).
	Start int `json:"start"`
	// SeeAll is the complete list behind a personal row: a browse of the
	// library's pivot with the row's filter, sorted For you.
	SeeAll *ContentSeeAll `json:"seeAll,omitempty"`
}

// ContentSeeAll is a browse request a client can open as it is.
type ContentSeeAll struct {
	Pivot string                `json:"pivot"`
	Query *BrowseNode           `json:"query,omitempty"`
	Sort  []BrowseSortSelection `json:"sort"`
}
type ContentQuery struct {
	Sort       string `json:"sort"`
	Direction  string `json:"direction"`
	Category   string `json:"category"`
	Q          string `json:"q"`
	Limit      int    `json:"limit"`
	SearchMode string `json:"searchMode"`
}
type ContentEnvelope struct {
	Entity           *ContentEntry     `json:"entity,omitempty"`
	Listening        *ListeningJourney `json:"listening,omitempty"`
	Actions          *[]string         `json:"actions,omitempty"`
	PlaylistRevision *int64            `json:"playlistRevision,omitempty"`
	Heading          ContentHeading    `json:"heading"`
	Scope            ContentScope      `json:"scope"`
	Revision         ContentRevision   `json:"revision"`
	Navigation       []ContentTab      `json:"navigation"`
	Query            ContentQuery      `json:"query"`
	Sorts            []ContentSort     `json:"sorts"`
	Filters          []ContentFilter   `json:"filters"`
	Sections         []ContentSection  `json:"sections"`
	Empty            *ContentHeading   `json:"empty,omitempty"`
}
type ContentRequest struct {
	Viewer                                                                                        Viewer
	ServerID, Library, Profile, ViewerFence, View, EntityID, Sort, Direction, Category, Q, Cursor string
	// Filter is a named server-defined predicate (saved lists: all, unwatched,
	// inProgress). Clients pick a published name; they never send a predicate.
	Filter string
	Limit  int
	// Start opens a grid section (views browse, collection, collections,
	// artist) at this index instead of the top; 0 is the top. Other views
	// refuse a non-zero start.
	Start int
	// Restrictions travels to whichever surface the view resolves to.
	Restrictions identity.ContentRestrictions
	// Now fences the daily rotation and recency curves; tests pin it so a
	// composition is deterministic. Zero means now.
	Now time.Time
}

func (r ContentRequest) scoped() ContentRequest {
	r.Profile, r.ViewerFence, r.Restrictions = r.Viewer.Profile, r.Viewer.Fence, r.Viewer.EffectiveRestrictions()
	return r
}

func (e ContentEntry) MarshalJSON() ([]byte, error) {
	if e.Kind == "playlist_entry" {
		return json.Marshal(struct {
			ID     string        `json:"id"`
			Kind   string        `json:"kind"`
			Hidden *bool         `json:"hidden"`
			Media  *ContentEntry `json:"media,omitempty"`
		}{e.ID, e.Kind, e.Hidden, e.Media})
	}
	type plain ContentEntry
	return json.Marshal(plain(e))
}

// LatestEpisode names the newest of a show's aired episodes and how many aired
// with it that day.
type LatestEpisode struct {
	AirDate       string `json:"airDate"`
	SeasonNumber  *int   `json:"seasonNumber,omitempty"`
	EpisodeNumber int    `json:"episodeNumber"`
	Title         string `json:"title,omitempty"`
	// Count is the episodes of the show that aired on AirDate (a double bill, a
	// season released at once).
	Count int `json:"count"`
}
