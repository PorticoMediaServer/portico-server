package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type tmdbSearchResponse struct {
	Results []tmdbSearchResult `json:"results"`
}

type tmdbSearchResult struct {
	ID                  int                   `json:"id"`
	Title               string                `json:"title"`
	Name                string                `json:"name"`
	ReleaseDate         string                `json:"release_date"`
	FirstAirDate        string                `json:"first_air_date"`
	Overview            string                `json:"overview"`
	Popularity          float64               `json:"popularity"`
	VoteAverage         float64               `json:"vote_average"`
	GenreIDs            []int                 `json:"genre_ids"`
	PosterPath          string                `json:"poster_path"`
	BackdropPath        string                `json:"backdrop_path"`
	Credits             tmdbCredits           `json:"credits"`
	OriginalTitle       string                `json:"original_title"`
	OriginalName        string                `json:"original_name"`
	OriginalLanguage    string                `json:"original_language"`
	SpokenLanguages     []tmdbLanguage        `json:"spoken_languages"`
	ProductionCountries []tmdbCountry         `json:"production_countries"`
	OriginCountry       []string              `json:"origin_country"`
	ProductionCompanies []tmdbCompany         `json:"production_companies"`
	Networks            []tmdbCompany         `json:"networks"`
	CreatedBy           []tmdbCreditPerson    `json:"created_by"`
	Keywords            tmdbKeywords          `json:"keywords"`
	BelongsToCollection *tmdbCollection       `json:"belongs_to_collection"`
	ReleaseDates        tmdbReleaseDates      `json:"release_dates"`
	ContentRatings      tmdbContentRatings    `json:"content_ratings"`
	ExternalIDs         map[string]any        `json:"external_ids"`
	AlternativeTitles   tmdbAlternativeTitles `json:"alternative_titles"`
	Status              string                `json:"status"`
	Runtime             int                   `json:"runtime"`
	EpisodeRunTime      []int                 `json:"episode_run_time"`
	Images              tmdbImages            `json:"images"`
}

type tmdbLanguage struct {
	ISO6391     string `json:"iso_639_1"`
	EnglishName string `json:"english_name"`
	Name        string `json:"name"`
}
type tmdbCountry struct {
	ISO31661 string `json:"iso_3166_1"`
	Name     string `json:"name"`
}
type tmdbCompany struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	OriginCountry string `json:"origin_country"`
	LogoPath      string `json:"logo_path"`
}
type tmdbCollection struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
}
type tmdbKeyword struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
type tmdbKeywords struct {
	Keywords []tmdbKeyword `json:"keywords"`
	Results  []tmdbKeyword `json:"results"`
}
type tmdbAlternativeTitle struct {
	ISO31661 string `json:"iso_3166_1"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}
type tmdbAlternativeTitles struct {
	Titles  []tmdbAlternativeTitle `json:"titles"`
	Results []tmdbAlternativeTitle `json:"results"`
}
type tmdbReleaseDate struct {
	Certification string `json:"certification"`
	ISO6391       string `json:"iso_639_1"`
	ReleaseDate   string `json:"release_date"`
	Type          int    `json:"type"`
}
type tmdbReleaseDateCountry struct {
	ISO31661     string            `json:"iso_3166_1"`
	ReleaseDates []tmdbReleaseDate `json:"release_dates"`
}
type tmdbReleaseDates struct {
	Results []tmdbReleaseDateCountry `json:"results"`
}
type tmdbContentRating struct {
	ISO31661 string `json:"iso_3166_1"`
	Rating   string `json:"rating"`
}
type tmdbContentRatings struct {
	Results []tmdbContentRating `json:"results"`
}
type tmdbImage struct {
	FilePath    string  `json:"file_path"`
	ISO6391     string  `json:"iso_639_1"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	AspectRatio float64 `json:"aspect_ratio"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}
type tmdbImages struct {
	Backdrops []tmdbImage `json:"backdrops"`
	Logos     []tmdbImage `json:"logos"`
	Posters   []tmdbImage `json:"posters"`
	Stills    []tmdbImage `json:"stills"`
}

type tmdbEpisodeDetails struct {
	Name          string             `json:"name"`
	Overview      string             `json:"overview"`
	AirDate       string             `json:"air_date"`
	EpisodeNumber int                `json:"episode_number"`
	SeasonNumber  int                `json:"season_number"`
	VoteAverage   float64            `json:"vote_average"`
	StillPath     string             `json:"still_path"`
	Credits       tmdbCredits        `json:"credits"`
	GuestStars    []tmdbCreditPerson `json:"guest_stars"`
}

type tmdbEpisodeGroupDetails struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Groups []tmdbEpisodeGroup `json:"groups"`
}

type tmdbEpisodeGroup struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Order    int                  `json:"order"`
	Episodes []tmdbEpisodeDetails `json:"episodes"`
}

type tmdbSeasonDetails struct {
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	AirDate      string `json:"air_date"`
	SeasonNumber int    `json:"season_number"`
	PosterPath   string `json:"poster_path"`
}

type tmdbCredits struct {
	Cast []tmdbCreditPerson `json:"cast"`
	Crew []tmdbCreditPerson `json:"crew"`
}

type tmdbCreditPerson struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
}

type metadataAgentSettings struct {
	Movies               string
	TV                   string
	Anime                string
	Music                string
	LocalNFO             bool
	EmbeddedTags         bool
	CacheOriginalArtwork bool
	RefreshDays          int
	MetadataLanguage     string
}

type aniListGraphQLResponse struct {
	Data   aniListData    `json:"data"`
	Errors []aniListError `json:"errors"`
}

type aniListData struct {
	Media  aniListMedia  `json:"Media"`
	Search aniListSearch `json:"Page"`
}

type aniListSearch struct {
	Media []aniListMedia `json:"media"`
}

type aniListError struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

type aniListMedia struct {
	ID           int                  `json:"id"`
	IDMal        int                  `json:"idMal"`
	Title        aniListTitle         `json:"title"`
	Description  string               `json:"description"`
	Format       string               `json:"format"`
	Status       string               `json:"status"`
	Episodes     int                  `json:"episodes"`
	Duration     int                  `json:"duration"`
	Season       string               `json:"season"`
	SeasonYear   int                  `json:"seasonYear"`
	AverageScore int                  `json:"averageScore"`
	IsAdult      bool                 `json:"isAdult"`
	StartDate    aniListFuzzyDate     `json:"startDate"`
	EndDate      aniListFuzzyDate     `json:"endDate"`
	Source       string               `json:"source"`
	Country      string               `json:"countryOfOrigin"`
	Synonyms     []string             `json:"synonyms"`
	Genres       []string             `json:"genres"`
	Tags         []aniListTag         `json:"tags"`
	CoverImage   aniListCoverImage    `json:"coverImage"`
	BannerImage  string               `json:"bannerImage"`
	Studios      aniListStudioEdge    `json:"studios"`
	Staff        aniListStaffEdge     `json:"staff"`
	Characters   aniListCharacterEdge `json:"characters"`
	Relations    aniListRelationEdge  `json:"relations"`
}

type aniListFuzzyDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type aniListTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

type aniListCoverImage struct {
	ExtraLarge string `json:"extraLarge"`
	Large      string `json:"large"`
	Medium     string `json:"medium"`
	Color      string `json:"color"`
}

type aniListPageInfo struct {
	Total       int  `json:"total"`
	PerPage     int  `json:"perPage"`
	CurrentPage int  `json:"currentPage"`
	LastPage    int  `json:"lastPage"`
	HasNextPage bool `json:"hasNextPage"`
}

type aniListTag struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
}

type aniListStudioEdge struct {
	Nodes []aniListStudio `json:"nodes"`
}

type aniListStudio struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type aniListStaffEdge struct {
	Edges    []aniListStaffCredit `json:"edges"`
	PageInfo aniListPageInfo      `json:"pageInfo"`
}

type aniListStaffCredit struct {
	Role string        `json:"role"`
	Node aniListPerson `json:"node"`
}

type aniListCharacterEdge struct {
	Edges    []aniListCharacterCredit `json:"edges"`
	PageInfo aniListPageInfo          `json:"pageInfo"`
}

type aniListRelationEdge struct {
	Edges    []aniListRelationCredit `json:"edges"`
	PageInfo aniListPageInfo         `json:"pageInfo"`
}
type aniListRelationCredit struct {
	RelationType string              `json:"relationType"`
	Node         aniListRelatedMedia `json:"node"`
}
type aniListRelatedMedia struct {
	ID     int          `json:"id"`
	IDMal  int          `json:"idMal"`
	Type   string       `json:"type"`
	Format string       `json:"format"`
	Title  aniListTitle `json:"title"`
}

type aniListCharacterCredit struct {
	Role        string           `json:"role"`
	Node        aniListCharacter `json:"node"`
	VoiceActors []aniListPerson  `json:"voiceActors"`
}

type aniListCharacter struct {
	ID    int          `json:"id"`
	Name  aniListName  `json:"name"`
	Image aniListImage `json:"image"`
}

type aniListPerson struct {
	ID    int          `json:"id"`
	Name  aniListName  `json:"name"`
	Image aniListImage `json:"image"`
}

type aniListName struct {
	Full   string `json:"full"`
	Native string `json:"native"`
}

type aniListImage struct {
	Large  string `json:"large"`
	Medium string `json:"medium"`
}

type musicBrainzRecordingSearchResponse struct {
	Recordings []musicBrainzRecording `json:"recordings"`
}

type musicBrainzReleaseGroupSearchResponse struct {
	ReleaseGroups []musicBrainzReleaseGroup `json:"release-groups"`
}

type musicBrainzArtistCredit struct {
	Name       string            `json:"name"`
	JoinPhrase string            `json:"joinphrase"`
	Artist     musicBrainzArtist `json:"artist"`
}

type musicBrainzArtist struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	SortName       string             `json:"sort-name"`
	Country        string             `json:"country"`
	Disambiguation string             `json:"disambiguation"`
	Aliases        []musicBrainzAlias `json:"aliases"`
	ISNIs          []string           `json:"isnis"`
	IPIs           []string           `json:"ipis"`
}

type musicBrainzAlias struct {
	Name     string `json:"name"`
	SortName string `json:"sort-name"`
	Locale   string `json:"locale"`
	Type     string `json:"type"`
	Primary  bool   `json:"primary"`
}
type musicBrainzLabel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortName  string `json:"sort-name"`
	Country   string `json:"country"`
	LabelCode int    `json:"label-code"`
}
type musicBrainzLabelInfo struct {
	CatalogNumber string           `json:"catalog-number"`
	Label         musicBrainzLabel `json:"label"`
}
type musicBrainzMedia struct {
	Format             string                        `json:"format"`
	Position           int                           `json:"position"`
	TrackCount         int                           `json:"track-count"`
	Title              string                        `json:"title"`
	Tracks             []musicBrainzTrack            `json:"tracks"`
	TrackOffset        int                           `json:"track-offset"`
	TextRepresentation musicBrainzTextRepresentation `json:"text-representation"`
}
type musicBrainzTextRepresentation struct {
	Language string `json:"language"`
	Script   string `json:"script"`
}
type musicBrainzTrack struct {
	ID        string                `json:"id"`
	Number    string                `json:"number"`
	Position  int                   `json:"position"`
	Title     string                `json:"title"`
	Length    int                   `json:"length"`
	Recording *musicBrainzRecording `json:"recording"`
}

type musicBrainzWork struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Language  string   `json:"language"`
	Languages []string `json:"languages"`
	ISWCs     []string `json:"iswcs"`
}
type musicBrainzRelation struct {
	TargetType string                `json:"target-type"`
	Type       string                `json:"type"`
	Direction  string                `json:"direction"`
	Begin      string                `json:"begin"`
	End        string                `json:"end"`
	Ended      bool                  `json:"ended"`
	Attributes []string              `json:"attributes"`
	Artist     *musicBrainzArtist    `json:"artist"`
	Recording  *musicBrainzRecording `json:"recording"`
	Work       *musicBrainzWork      `json:"work"`
}

type musicBrainzRecording struct {
	ID             string                    `json:"id"`
	Title          string                    `json:"title"`
	Score          int                       `json:"score"`
	Length         int                       `json:"length"`
	ArtistCredit   []musicBrainzArtistCredit `json:"artist-credit"`
	Releases       []musicBrainzRelease      `json:"releases"`
	Genres         []musicBrainzTag          `json:"genres"`
	Tags           []musicBrainzTag          `json:"tags"`
	Aliases        []musicBrainzAlias        `json:"aliases"`
	ISRCs          []string                  `json:"isrcs"`
	Disambiguation string                    `json:"disambiguation"`
	Relations      []musicBrainzRelation     `json:"relations"`
}

type musicBrainzRelease struct {
	ID                 string                        `json:"id"`
	Title              string                        `json:"title"`
	Date               string                        `json:"date"`
	ArtistCredit       []musicBrainzArtistCredit     `json:"artist-credit"`
	ReleaseGroup       musicBrainzReleaseGroup       `json:"release-group"`
	Country            string                        `json:"country"`
	Status             string                        `json:"status"`
	Barcode            string                        `json:"barcode"`
	ASIN               string                        `json:"asin"`
	LabelInfo          []musicBrainzLabelInfo        `json:"label-info"`
	Media              []musicBrainzMedia            `json:"media"`
	Packaging          string                        `json:"packaging"`
	TextRepresentation musicBrainzTextRepresentation `json:"text-representation"`
}

type musicBrainzReleaseGroup struct {
	ID               string                    `json:"id"`
	Title            string                    `json:"title"`
	Score            int                       `json:"score"`
	FirstReleaseDate string                    `json:"first-release-date"`
	PrimaryType      string                    `json:"primary-type"`
	ArtistCredit     []musicBrainzArtistCredit `json:"artist-credit"`
	Releases         []musicBrainzRelease      `json:"releases"`
	Genres           []musicBrainzTag          `json:"genres"`
	Tags             []musicBrainzTag          `json:"tags"`
	SecondaryTypes   []string                  `json:"secondary-types"`
	Disambiguation   string                    `json:"disambiguation"`
	Aliases          []musicBrainzAlias        `json:"aliases"`
	Relations        []musicBrainzRelation     `json:"relations"`
}

type musicBrainzTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

const (
	maxMetadataProviderResponseBytes  int64 = 2 << 20
	maxCoverArtArchiveResponseBytes   int64 = 1 << 20
	maxProviderJSONResponseBytes            = maxMetadataProviderResponseBytes
	providerResponseFailureRequest          = "request"
	providerResponseFailureHTTPStatus       = "http_status"
	providerResponseFailureBodyRead         = "body_read"
	providerResponseFailureTooLarge         = "too_large"
	providerResponseFailureMalformed        = "malformed_json"
)

var (
	errProviderRequest           = errors.New("provider request failed")
	errProviderHTTPStatus        = errors.New("provider returned a non-2xx response")
	errProviderResponseBodyRead  = errors.New("provider response body read failed")
	errProviderResponseTooLarge  = errors.New("provider response exceeded its maximum size")
	errProviderResponseMalformed = errors.New("provider response was malformed")
	errTMDBCredentialsMissing    = errors.New("TMDB credentials are not configured")
)

type providerResponseError struct {
	Provider   string
	Kind       string
	StatusCode int
	Status     string
	Limit      int64
	Cause      error
}

func (e *providerResponseError) Error() string {
	if e == nil {
		return "provider response failed"
	}
	provider := firstNonEmpty(strings.TrimSpace(e.Provider), "provider")
	switch e.Kind {
	case providerResponseFailureRequest:
		if e.Cause == nil {
			return provider + " request failed"
		}
		return fmt.Sprintf("%s request failed: %v", provider, e.Cause)
	case providerResponseFailureHTTPStatus:
		status := strings.TrimSpace(e.Status)
		if status == "" {
			status = http.StatusText(e.StatusCode)
		}
		return fmt.Sprintf("%s response returned HTTP %d (%s)", provider, e.StatusCode, status)
	case providerResponseFailureBodyRead:
		if e.Cause == nil {
			return provider + " response body read failed"
		}
		return fmt.Sprintf("%s response body read failed: %v", provider, e.Cause)
	case providerResponseFailureTooLarge:
		return fmt.Sprintf("%s response exceeded the %d-byte maximum", provider, e.Limit)
	case providerResponseFailureMalformed:
		if e.Cause == nil {
			return provider + " response was malformed"
		}
		return fmt.Sprintf("%s response was malformed: %v", provider, e.Cause)
	default:
		if e.Cause == nil {
			return provider + " response failed"
		}
		return fmt.Sprintf("%s response failed: %v", provider, e.Cause)
	}
}

func (e *providerResponseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *providerResponseError) Is(target error) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case providerResponseFailureRequest:
		if target == errProviderRequest {
			return true
		}
	case providerResponseFailureHTTPStatus:
		if target == errProviderHTTPStatus {
			return true
		}
	case providerResponseFailureBodyRead:
		if target == errProviderResponseBodyRead {
			return true
		}
	case providerResponseFailureTooLarge:
		if target == errProviderResponseTooLarge {
			return true
		}
	case providerResponseFailureMalformed:
		if target == errProviderResponseMalformed {
			return true
		}
	}
	return errors.Is(e.Cause, target)
}

type providerBodyReadResult struct {
	data []byte
	err  error
}

type providerHTTPResponse struct {
	StatusCode int
	Status     string
	Header     http.Header
}

// decodeProviderJSONResponse owns the complete response body lifecycle. The
// request context must remain live until this function returns so body reads
// and JSON decoding are covered by the same timeout/cancellation budget as the
// request headers.
func decodeProviderJSONResponse(ctx context.Context, provider string, resp *http.Response, maxBytes int64, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if resp == nil {
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureBodyRead, Cause: errProviderResponseBodyRead}
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &providerResponseError{
			Provider: provider, Kind: providerResponseFailureHTTPStatus,
			StatusCode: resp.StatusCode, Status: resp.Status,
			Cause: errProviderHTTPStatus,
		}
	}
	if maxBytes <= 0 {
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureTooLarge, Limit: maxBytes, Cause: errProviderResponseTooLarge}
	}
	if resp.Body == nil {
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureBodyRead, Cause: errProviderResponseBodyRead}
	}
	if resp.ContentLength > maxBytes {
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureTooLarge, Limit: maxBytes, Cause: errProviderResponseTooLarge}
	}

	resultCh := make(chan providerBodyReadResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
		resultCh <- providerBodyReadResult{data: data, err: err}
	}()
	var result providerBodyReadResult
	select {
	case <-ctx.Done():
		_ = resp.Body.Close()
		return ctx.Err()
	case result = <-resultCh:
	}
	if result.err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureBodyRead, Cause: result.err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if int64(len(result.data)) > maxBytes {
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureTooLarge, Limit: maxBytes, Cause: errProviderResponseTooLarge}
	}
	if err := json.Unmarshal(result.data, out); err != nil {
		return &providerResponseError{Provider: provider, Kind: providerResponseFailureMalformed, Cause: err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// doProviderJSONRequest keeps the request context active across the complete
// HTTP response lifecycle and returns status metadata for bounded retries.
func doProviderJSONRequest(ctx context.Context, provider string, req *http.Request, maxBytes int64, out any) (providerHTTPResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return providerHTTPResponse{}, &providerResponseError{Provider: provider, Kind: providerResponseFailureRequest, Cause: errProviderRequest}
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return providerHTTPResponse{}, ctxErr
		}
		return providerHTTPResponse{}, &providerResponseError{Provider: provider, Kind: providerResponseFailureRequest, Cause: err}
	}
	response := providerHTTPResponse{StatusCode: resp.StatusCode, Status: resp.Status, Header: resp.Header.Clone()}
	return response, decodeProviderJSONResponse(ctx, provider, resp, maxBytes, out)
}

func providerResponseStatusCode(err error) int {
	var responseErr *providerResponseError
	if errors.As(err, &responseErr) {
		return responseErr.StatusCode
	}
	return 0
}

var (
	musicBrainzThrottleMu  sync.Mutex
	musicBrainzLastRequest time.Time
	aniListThrottleMu      sync.Mutex
	aniListLastRequest     time.Time
	tmdbThrottleMu         sync.Mutex
	tmdbLastRequest        time.Time
	acoustIDThrottleMu     sync.Mutex
	acoustIDLastRequest    time.Time
)

type coverArtArchiveResponse struct {
	Images  []coverArtArchiveImage `json:"images"`
	Release string                 `json:"release"`
}
type coverArtArchiveImage struct {
	ID         int64             `json:"id"`
	Image      string            `json:"image"`
	Front      bool              `json:"front"`
	Back       bool              `json:"back"`
	Types      []string          `json:"types"`
	Comment    string            `json:"comment"`
	Approved   bool              `json:"approved"`
	Thumbnails map[string]string `json:"thumbnails"`
}

type MetadataProvider interface {
	ID() string
	Supports(item MediaItem) bool
	Refresh(ctx context.Context, server *Server, item MediaItem) (MediaItem, error)
}

type MetadataSaver interface {
	ID() string
	Save(server *Server, userID string, mediaID string, update UpdateMediaRequest) (MediaItem, error)
}

type tmdbMetadataProvider struct{}
type aniListMetadataProvider struct{}
type musicBrainzMetadataProvider struct{}
type sqliteMetadataSaver struct{}

const (
	libraryMetadataRefreshDefaultLimit = 100
	libraryMetadataRefreshMaxLimit     = 250
	libraryMetadataRefreshJobBudget    = 2 * time.Minute
	metadataCascadePageSize            = 100
	metadataCascadeClaimSize           = 25
	metadataCascadeLease               = 5 * time.Minute
)

func (tmdbMetadataProvider) ID() string { return "tmdb" }

func (tmdbMetadataProvider) Supports(item MediaItem) bool {
	return tmdbSearchType(item.Type) != ""
}

func (tmdbMetadataProvider) Refresh(ctx context.Context, server *Server, item MediaItem) (MediaItem, error) {
	return server.refreshMediaMetadataFromTMDB(ctx, item)
}

func (aniListMetadataProvider) ID() string { return "anilist" }

func (aniListMetadataProvider) Supports(item MediaItem) bool {
	return item.Type == "anime"
}

func (aniListMetadataProvider) Refresh(ctx context.Context, server *Server, item MediaItem) (MediaItem, error) {
	return server.refreshMediaMetadataFromAniList(ctx, item)
}

func (musicBrainzMetadataProvider) ID() string { return "musicbrainz" }

func (musicBrainzMetadataProvider) Supports(item MediaItem) bool {
	return item.Type == "track" || item.Type == "album" || item.Type == "artist"
}

func (musicBrainzMetadataProvider) Refresh(ctx context.Context, server *Server, item MediaItem) (MediaItem, error) {
	return server.refreshMediaMetadataFromMusicBrainz(ctx, item)
}

func (s *Server) metadataProviderByID(id string) (MetadataProvider, bool) {
	switch normalizedMetadataProvider(id) {
	case "tmdb":
		return tmdbMetadataProvider{}, true
	case "anilist":
		return aniListMetadataProvider{}, true
	case "musicbrainz":
		return musicBrainzMetadataProvider{}, true
	default:
		return nil, false
	}
}

func (sqliteMetadataSaver) ID() string { return "sqlite" }

func (sqliteMetadataSaver) Save(server *Server, userID string, mediaID string, update UpdateMediaRequest) (MediaItem, error) {
	return server.updateMediaForMetadata(userID, mediaID, update)
}

func (s *Server) saveMetadataUpdate(userID string, mediaID string, update UpdateMediaRequest) (MediaItem, error) {
	return sqliteMetadataSaver{}.Save(s, userID, mediaID, update)
}

func (s *Server) runMetadataRefresh(ctx context.Context, job Job) {
	if job.ResourceType != "media" || strings.TrimSpace(job.ResourceID) == "" {
		_ = s.setJobMessage(job.ID, "failed", 100, "Metadata refresh failed because no media item was selected.")
		return
	}
	_ = s.setJobMessage(job.ID, "running", 18, "Preparing metadata refresh.")
	item, err := s.getMetadataRefreshSeedContext(ctx, job.ResourceID)
	if err != nil {
		_ = s.setJobMessage(job.ID, "failed", 100, "Metadata refresh failed because the media item was not found.")
		return
	}
	next, refreshed, refreshErrs := s.refreshMediaMetadataCascadeOperation(ctx, job.ID+":"+item.ID, item)
	if len(refreshErrs) > 0 && refreshed == 0 {
		err := refreshErrs[0]
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := "Metadata refresh failed for " + item.Title + ": " + err.Error()
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("warn", message, map[string]string{"job": job.ID, "media": item.ID})
		return
	}
	message := fmt.Sprintf("Metadata refreshed for %s.", next.Title)
	if refreshed > 1 {
		message = fmt.Sprintf("Metadata refreshed for %s and %d related items.", next.Title, refreshed-1)
	}
	if len(refreshErrs) > 0 {
		message = fmt.Sprintf("%s %d related item(s) could not be matched.", message, len(refreshErrs))
	}
	if diagnostics := s.metadataRefreshDiagnosticSummary(metadataCascadeIDs(item)); diagnostics != "" {
		message = fmt.Sprintf("%s %s", message, diagnostics)
	}
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "media": item.ID})
}

func (s *Server) runLibraryMetadataRefresh(ctx context.Context, job Job) {
	if job.ResourceType != "library" || strings.TrimSpace(job.ResourceID) == "" {
		_ = s.setJobMessage(job.ID, "failed", 100, "Metadata refresh failed because no library was selected.")
		return
	}
	library, err := s.getLibrary(job.ResourceID)
	if err != nil {
		_ = s.setJobMessage(job.ID, "failed", 100, "Metadata refresh failed because the library no longer exists.")
		return
	}
	_ = s.setJobMessage(job.ID, "running", 5, fmt.Sprintf("Finding metadata refresh candidates in %s.", library.Name))
	items, hasMore, err := s.libraryMetadataRefreshItemsPage(ctx, library.ID, job.Metadata)
	if err != nil {
		if s.deferMaintenanceJob(job.ID, err) {
			return
		}
		message := fmt.Sprintf("Metadata refresh failed for %s: %s", library.Name, err.Error())
		_ = s.setJobMessage(job.ID, "failed", 100, message)
		s.recordLog("warn", message, map[string]string{"job": job.ID, "library": library.ID})
		return
	}
	if len(items) == 0 {
		message := fmt.Sprintf("Metadata refresh completed for %s. No eligible items needed refresh.", library.Name)
		_ = s.setJobMessage(job.ID, "complete", 100, message)
		s.recordLog("info", message, map[string]string{"job": job.ID, "library": library.ID})
		return
	}
	refreshed := 0
	failed := 0
	skipped := 0
	processed := 0
	deadline := time.Now().Add(libraryMetadataRefreshBudget(job.Metadata))
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			if s.deferMaintenanceJob(job.ID, err) {
				return
			}
			s.recordLog("info", fmt.Sprintf("Metadata refresh cancelled for %s.", library.Name), map[string]string{"job": job.ID, "library": library.ID})
			return
		}
		progress := 8 + int(float64(i)/float64(len(items))*87)
		_ = s.setJobMessage(job.ID, "running", progress, fmt.Sprintf("Refreshing %s (%d/%d).", item.Title, i+1, len(items)))
		if !s.automaticMetadataRefreshSupported(item) {
			skipped++
			processed = i + 1
			if time.Now().After(deadline) && (processed < len(items) || hasMore) {
				break
			}
			continue
		}
		_, count, errs := s.refreshMediaMetadataCascadeOperation(ctx, job.ID+":"+item.ID, item)
		if count > 0 {
			refreshed += count
		}
		if len(errs) > 0 && count == 0 {
			if s.deferMaintenanceJob(job.ID, errs[0]) {
				return
			}
			failed++
		} else if len(errs) > 0 {
			failed += len(errs)
		}
		processed = i + 1
		if time.Now().After(deadline) && (processed < len(items) || hasMore) {
			break
		}
	}
	continued := false
	if (processed < len(items) || hasMore) && processed > 0 && len(jobMetadataMediaIDs(job.Metadata["mediaIds"], 1)) == 0 {
		last := items[processed-1]
		if _, err := s.queueLibraryMetadataRefreshContinuation(job, library, last); err != nil {
			s.log.Warn("metadata refresh continuation queue failed", "job", job.ID, "library", library.ID, "error", err)
			_ = s.setJobFailure(job.ID, "continuation_enqueue_failed", "Metadata refresh paused, but its continuation could not be durably queued.")
			return
		} else {
			continued = true
		}
	}
	message := fmt.Sprintf("Metadata refresh completed for %s. Refreshed %d, skipped %d, failed %d.", library.Name, refreshed, skipped, failed)
	if continued {
		message = fmt.Sprintf("Metadata refresh paused for %s after %d item%s. Refreshed %d, skipped %d, failed %d; continuation queued.", library.Name, processed, pluralSuffix(processed), refreshed, skipped, failed)
	}
	_ = s.setJobMessage(job.ID, "complete", 100, message)
	s.recordLog("info", message, map[string]string{"job": job.ID, "library": library.ID})
}

func (s *Server) libraryMetadataRefreshItems(ctx context.Context, libraryID string, metadata map[string]string) ([]MediaItem, error) {
	items, _, err := s.libraryMetadataRefreshItemsPage(ctx, libraryID, metadata)
	return items, err
}

func (s *Server) libraryMetadataRefreshItemsPage(ctx context.Context, libraryID string, metadata map[string]string) ([]MediaItem, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	limit := max(1, min(libraryMetadataRefreshMaxLimit, parsePositiveInt(metadata["limit"], libraryMetadataRefreshDefaultLimit)))
	refreshDays := parsePositiveInt(metadata["refreshDays"], 0)
	if mediaIDs := jobMetadataMediaIDs(metadata["mediaIds"], 0); len(mediaIDs) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(mediaIDs)), ",")
		args := make([]any, 0, len(mediaIDs)+1)
		args = append(args, libraryID)
		for _, mediaID := range mediaIDs {
			args = append(args, mediaID)
		}
		items, err := s.queryMediaListItemsContext(ctx, "", "WHERE m.library_id = ? AND m.id IN ("+placeholders+") ORDER BY m.added_at ASC", args)
		if err != nil {
			return nil, false, err
		}
		items, err = s.filterAutomaticMetadataRefreshItemsContext(ctx, items, true)
		return items, false, err
	}
	where := "WHERE m.library_id = ? AND m.type IN ('movie', 'show', 'anime', 'episode', 'season', 'album', 'artist', 'track')"
	args := []any{libraryID}
	if cursorAddedAt, cursorID := metadata["cursorAddedAt"], metadata["cursorId"]; strings.TrimSpace(cursorAddedAt) != "" && strings.TrimSpace(cursorID) != "" {
		where += " AND (m.added_at > ? OR (m.added_at = ? AND m.id > ?))"
		args = append(args, cursorAddedAt, cursorAddedAt, cursorID)
	}
	if refreshDays > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(refreshDays) * 24 * time.Hour).Format(time.RFC3339)
		where += " AND (m.metadata_refreshed_at = '' OR m.metadata_refreshed_at <= ?)"
		args = append(args, cutoff)
	}
	args = append(args, limit+1)
	items, err := s.queryMediaListItemsContext(ctx, "", where+" ORDER BY m.added_at ASC, m.id ASC LIMIT ?", args)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	items, err = s.filterAutomaticMetadataRefreshItemsContext(ctx, items, false)
	if err != nil {
		return nil, false, err
	}
	return items, hasMore, nil
}

func libraryMetadataRefreshBudget(metadata map[string]string) time.Duration {
	seconds := parsePositiveInt(metadata["timeBudgetSeconds"], 0)
	if seconds > 0 {
		return time.Duration(min(seconds, 3600)) * time.Second
	}
	return libraryMetadataRefreshJobBudget
}

func (s *Server) queueLibraryMetadataRefreshContinuation(parent Job, library Library, last MediaItem) (Job, error) {
	metadata := map[string]string{}
	for key, value := range parent.Metadata {
		switch key {
		case "mediaIds", "cursorAddedAt", "cursorId":
			continue
		default:
			metadata[key] = value
		}
	}
	metadata["libraryId"] = library.ID
	metadata["libraryName"] = library.Name
	metadata["cursorAddedAt"] = last.AddedAt
	metadata["cursorId"] = last.ID
	metadata["continuationOf"] = parent.ID
	metadata = normalizeJobMetadata(metadata)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	child := Job{
		ID: randomID("job"), Type: "metadata_refresh_library", Status: "queued",
		Message:      fmt.Sprintf("Metadata refresh continuation queued for %s.", library.Name),
		ResourceType: "library", ResourceID: library.ID, Metadata: metadata,
		ParentOperationID: parent.ID, Priority: "normal", Phase: "queued",
		CreatedAt: now, UpdatedAt: now,
	}
	child.ActiveKey = jobActiveKeyFor(child.Type, child.ResourceType, child.ResourceID, metadata)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return Job{}, err
	}
	durableJobEnqueueMu.Lock()
	defer durableJobEnqueueMu.Unlock()
	child, err = s.withJobAdmission(func() (Job, error) {
		err := s.withPrioritizedTxTaggedForViewer(context.Background(), sqliteWriteBackground, "job_continuation_enqueue", durableJobEnqueueRetry, "", "", []string{"jobs"}, func(tx *sql.Tx) error {
			result, err := tx.ExecContext(context.Background(), `
				UPDATE jobs SET active_key = '', updated_at = ?
				WHERE id = ? AND status = 'running' AND active_key = ?`, now, parent.ID, child.ActiveKey)
			if err != nil {
				return err
			}
			if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
				if rowsErr != nil {
					return rowsErr
				}
				return fmt.Errorf("parent metadata refresh no longer owns its active claim")
			}
			_, err = tx.ExecContext(context.Background(), `INSERT INTO jobs (
				id, type, status, progress, message, resource_type, resource_id, metadata_json,
				attempt_count, next_run_at, leased_by, lease_expires_at, last_error,
				parent_operation_id, idempotency_key, active_key, priority, phase,
				progress_current, progress_total, result_reference, error_code, retry_eligible,
				cancellation_requested_at, worker_acknowledged_at, interrupted_at, retention_until,
				created_at, updated_at
			) VALUES (?, ?, 'queued', 0, ?, ?, ?, ?, 0, '', '', '', '', ?, '', ?, 'normal', 'queued', 0, 0, '', '', 0, '', '', '', '', ?, ?)`,
				child.ID, child.Type, child.Message, child.ResourceType, child.ResourceID,
				string(metadataJSON), parent.ID, child.ActiveKey, now, now)
			return err
		})
		return child, err
	})
	if err == nil {
		s.signalJobWake()
	}
	return child, err
}

func (s *Server) getMetadataRefreshSeedContext(ctx context.Context, mediaID string) (MediaItem, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return MediaItem{}, sql.ErrNoRows
	}
	items, err := s.queryMediaListItemsContext(ctx, "", "WHERE m.id = ? LIMIT 1", []any{mediaID})
	if err != nil {
		return MediaItem{}, err
	}
	if len(items) == 0 {
		return MediaItem{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (s *Server) filterAutomaticMetadataRefreshItemsContext(ctx context.Context, items []MediaItem, collapseMusicTracks bool) ([]MediaItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filtered := make([]MediaItem, 0, len(items))
	seen := map[string]bool{}
	parentAlbumIDs := []string{}
	parentAlbumSeen := map[string]bool{}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if collapseMusicTracks && item.Type == "track" && strings.TrimSpace(item.ParentID) != "" && s.metadataProviderForItem(item) == "musicbrainz" {
			if !parentAlbumSeen[item.ParentID] {
				parentAlbumIDs = append(parentAlbumIDs, item.ParentID)
				parentAlbumSeen[item.ParentID] = true
			}
			continue
		}
		if s.automaticMetadataRefreshSupported(item) {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			filtered = append(filtered, item)
		}
	}
	if len(parentAlbumIDs) == 0 {
		return filtered, nil
	}
	parentAlbums, err := s.metadataRefreshParentAlbumsContext(ctx, parentAlbumIDs)
	if err != nil {
		return nil, err
	}
	for _, parentID := range parentAlbumIDs {
		album, ok := parentAlbums[parentID]
		if !ok || seen[album.ID] || !s.automaticMetadataRefreshSupported(album) {
			continue
		}
		seen[album.ID] = true
		filtered = append(filtered, album)
	}
	return filtered, nil
}

func (s *Server) metadataRefreshParentAlbumsContext(ctx context.Context, albumIDs []string) (map[string]MediaItem, error) {
	albumIDs = uniqueNonEmptyStrings(albumIDs)
	if len(albumIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(albumIDs)), ",")
	args := make([]any, 0, len(albumIDs))
	for _, albumID := range albumIDs {
		args = append(args, albumID)
	}
	items, err := s.queryMediaListItemsContext(ctx, "", "WHERE m.id IN ("+placeholders+") AND m.type = 'album'", args)
	if err != nil {
		return nil, err
	}
	albums := make(map[string]MediaItem, len(items))
	for _, item := range items {
		albums[item.ID] = item
	}
	return albums, nil
}

func jobMetadataMediaIDs(value string, limit int) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, part := range strings.Split(value, ",") {
		mediaID := strings.TrimSpace(part)
		if mediaID == "" || seen[mediaID] {
			continue
		}
		seen[mediaID] = true
		ids = append(ids, mediaID)
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	return ids
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (s *Server) refreshMediaMetadataCascade(ctx context.Context, item MediaItem) (MediaItem, int, []error) {
	return s.refreshMediaMetadataCascadeOperation(ctx, randomID("mcascade")+":"+item.ID, item)
}

// refreshMediaMetadataCascadeOperation traverses a cascade through durable,
// keyset-paged cursors. Replaying operationID resumes unfinished discovery and
// reclaims expired item leases without repeating terminal item work.
func (s *Server) refreshMediaMetadataCascadeOperation(ctx context.Context, operationID string, root MediaItem) (MediaItem, int, []error) {
	store := NewMetadataContinuationStore(s.db)
	provider := s.metadataProviderForItem(root)
	op, err := store.Start(ctx, MetadataContinuationStart{
		ID: operationID, RootKind: root.Type, RootID: root.ID, Provider: provider,
		PolicyRevision: "hierarchy-v1", ProviderRevision: provider + "-v1",
		InitialPhase: "descendants", InitialCursor: "",
	})
	if err != nil {
		return root, 0, []error{err}
	}
	if op.Status == "cancelled" {
		return root, op.Processed, []error{context.Canceled}
	}
	if op.Status == "completed" || op.Status == "completed_with_failures" {
		failures, failureErr := store.Failures(ctx, operationID)
		return root, op.Processed, metadataContinuationErrors(failures, failureErr)
	}
	// Seed the root exactly once. RecordPage is idempotent if a worker died
	// after committing the page but before continuing.
	if err = store.RecordPage(ctx, operationID, "descendants", "", "", "", true, []MetadataContinuationItemInput{{Key: root.ID, Kind: root.Type, ProviderID: provider}}); err != nil {
		return root, op.Processed, []error{err}
	}

	for {
		if err = ctx.Err(); err != nil {
			// Leave the operation and leases intact: a restarted job can reclaim
			// them safely after the lease, rather than falsely completing.
			return root, metadataContinuationProcessed(ctx, store, operationID), []error{err}
		}
		advanced, discoveryErr := s.advanceMetadataCascadeCursor(ctx, store, operationID)
		if discoveryErr != nil {
			return root, 0, []error{discoveryErr}
		}
		claimed, claimErr := store.ClaimReadyItems(ctx, operationID, metadataCascadeClaimSize, metadataCascadeLease)
		if claimErr != nil {
			return root, 0, []error{claimErr}
		}
		for _, queued := range claimed {
			if err = ctx.Err(); err != nil {
				_ = store.RetryItem(context.Background(), operationID, queued.Key, err.Error(), time.Now().UTC())
				return root, metadataContinuationProcessed(ctx, store, operationID), []error{err}
			}
			current, loadErr := s.getMetadataRefreshSeedContext(ctx, queued.Key)
			if loadErr == nil {
				var updated MediaItem
				updated, loadErr = s.refreshMediaMetadata(ctx, current)
				if queued.Key == root.ID && loadErr == nil {
					root = updated
				}
			}
			if loadErr != nil {
				if errors.Is(loadErr, context.Canceled) || errors.Is(loadErr, context.DeadlineExceeded) || maintenanceFailureKind(loadErr) != "" {
					_ = store.RetryItem(context.Background(), operationID, queued.Key, loadErr.Error(), time.Now().UTC().Add(time.Minute))
					return root, 0, []error{loadErr}
				}
				_ = store.FailItem(ctx, operationID, queued.Key, loadErr.Error())
				continue
			}
			if err = store.SucceedItem(ctx, operationID, queued.Key); err != nil {
				return root, metadataContinuationProcessed(ctx, store, operationID), []error{err}
			}
		}
		complete, completeErr := store.TryComplete(ctx, operationID)
		if completeErr != nil {
			return root, metadataContinuationProcessed(ctx, store, operationID), []error{completeErr}
		}
		if complete {
			final, _ := store.Get(ctx, operationID)
			failures, failureErr := store.Failures(ctx, operationID)
			return root, final.Processed, metadataContinuationErrors(failures, failureErr)
		}
		if !advanced && len(claimed) == 0 {
			return root, 0, []error{errors.New("resource busy: metadata cascade is waiting for an active lease or retry")}
		}
	}
}

func (s *Server) advanceMetadataCascadeCursor(ctx context.Context, store *MetadataContinuationStore, operationID string) (bool, error) {
	var parentID, cursor string
	err := s.db.QueryRowContext(ctx, `SELECT parent_key,cursor FROM metadata_continuation_cursors WHERE operation_id=? AND exhausted=0 ORDER BY updated_at,parent_key LIMIT 1`, operationID).Scan(&parentID, &cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	items, err := s.queryMediaListItemsContext(ctx, "", "WHERE m.parent_id = ? AND m.id > ? ORDER BY m.id ASC LIMIT ?", []any{parentID, cursor, metadataCascadePageSize})
	if err != nil {
		return false, err
	}
	next := cursor
	inputs := make([]MetadataContinuationItemInput, 0, len(items))
	for _, child := range items {
		next = child.ID
		inputs = append(inputs, MetadataContinuationItemInput{Key: child.ID, ParentKey: parentID, Kind: child.Type, ProviderID: s.metadataProviderForItem(child)})
	}
	return true, store.RecordPage(ctx, operationID, "descendants", parentID, cursor, next, len(items) < metadataCascadePageSize, inputs)
}

func metadataContinuationProcessed(ctx context.Context, store *MetadataContinuationStore, operationID string) int {
	op, err := store.Get(ctx, operationID)
	if err != nil {
		return 0
	}
	return op.Processed
}

func metadataContinuationErrors(failures []MetadataContinuationFailure, err error) []error {
	errs := []error{}
	if err != nil {
		errs = append(errs, err)
	}
	for _, failure := range failures {
		errs = append(errs, fmt.Errorf("%s: %s", failure.Key, failure.Error))
	}
	return errs
}

func metadataCascadeIDs(item MediaItem) []string {
	ids := []string{item.ID}
	for _, child := range metadataCascadeChildren(item) {
		ids = append(ids, child.ID)
	}
	return ids
}

func (s *Server) metadataCascadeChildrenContext(ctx context.Context, item MediaItem) ([]MediaItem, error) {
	if children := metadataCascadeChildren(item); len(children) > 0 {
		return children, nil
	}
	switch item.Type {
	case "show", "anime":
		return s.groupedMetadataCascadeChildrenContext(ctx, item.ID)
	case "artist":
		return s.allMetadataCascadeChildrenContext(ctx, item.ID)
	case "season":
		return s.allMetadataCascadeChildrenContext(ctx, item.ID)
	default:
		return nil, nil
	}
}

func (s *Server) groupedMetadataCascadeChildrenContext(ctx context.Context, parentID string) ([]MediaItem, error) {
	groups, err := s.allMetadataCascadeChildrenContext(ctx, parentID)
	if err != nil {
		return nil, err
	}
	children := make([]MediaItem, 0, len(groups))
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return children, err
		}
		children = append(children, group)
		nested, err := s.allMetadataCascadeChildrenContext(ctx, group.ID)
		if err != nil {
			return children, err
		}
		children = append(children, nested...)
	}
	return children, nil
}

func (s *Server) allMetadataCascadeChildrenContext(ctx context.Context, parentID string) ([]MediaItem, error) {
	var all []MediaItem
	cursor := ""
	for {
		page, err := s.queryMediaListItemsContext(ctx, "", "WHERE m.parent_id = ? AND m.id > ? ORDER BY m.id ASC LIMIT ?", []any{parentID, cursor, metadataCascadePageSize})
		if err != nil {
			return all, err
		}
		all = append(all, page...)
		if len(page) < metadataCascadePageSize {
			return all, nil
		}
		cursor = page[len(page)-1].ID
	}
}

func (s *Server) metadataCascadeChildPageContext(ctx context.Context, parentID string, limit int) ([]MediaItem, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	items, err := s.queryMediaListItemsContext(ctx, "", "WHERE m.parent_id = ? ORDER BY m.index_number ASC, m.sort_title ASC, m.id ASC LIMIT ?", []any{parentID, limit + 1})
	if err != nil {
		return nil, false, err
	}
	if len(items) <= limit {
		return items, false, nil
	}
	return items[:limit], true, nil
}

func metadataCascadeChildren(item MediaItem) []MediaItem {
	var children []MediaItem
	switch item.Type {
	case "show", "anime":
		for _, season := range item.Children {
			children = append(children, season)
			children = append(children, season.Children...)
		}
	case "season":
		children = append(children, item.Children...)
	case "artist":
		for _, album := range item.Children {
			children = append(children, album)
		}
	}
	return children
}

func (s *Server) metadataRefreshDiagnosticSummary(mediaIDs []string) string {
	var best MatchCandidate
	bestMediaID := ""
	for _, mediaID := range mediaIDs {
		candidate, ok := s.latestMatchCandidateForMedia(mediaID)
		if !ok {
			continue
		}
		if best.ExternalID == "" || candidate.Score > best.Score || (candidate.Accepted && !best.Accepted) {
			best = candidate
			bestMediaID = mediaID
		}
	}
	if best.ExternalID == "" {
		return ""
	}
	codes := make([]string, 0, len(best.Reasons))
	for _, reason := range best.Reasons {
		if strings.TrimSpace(reason.Code) != "" {
			codes = append(codes, reason.Code)
		}
	}
	status := "Best candidate"
	if best.Accepted {
		status = "Matched"
	}
	target := fmt.Sprintf("%s %s:%s", best.Provider, best.ExternalType, best.ExternalID)
	if bestMediaID != "" && len(mediaIDs) > 1 {
		target = fmt.Sprintf("%s on %s", target, bestMediaID)
	}
	reasons := ""
	if len(codes) > 0 {
		reasons = " (" + strings.Join(codes, ", ") + ")"
	}
	if best.Accepted {
		return fmt.Sprintf("%s %s with score %.1f%s.", status, target, best.Score, reasons)
	}
	return fmt.Sprintf("%s %s was not accepted; score %.1f%s.", status, target, best.Score, reasons)
}

func (s *Server) latestMatchCandidateForMedia(mediaID string) (MatchCandidate, bool) {
	var candidate MatchCandidate
	var accepted int
	var reasonJSON string
	var rawResultJSON string
	err := s.queryUserRow(context.Background(), `
		SELECT provider, external_id, external_type, source, score, CASE WHEN status = 'accepted' THEN 1 ELSE 0 END, reason_codes_json, raw_result_json, created_at
		FROM media_match_candidates
		WHERE media_id = ?
		ORDER BY CASE WHEN status = 'accepted' THEN 1 ELSE 0 END DESC, score DESC, created_at DESC
		LIMIT 1`, mediaID).Scan(&candidate.Provider, &candidate.ExternalID, &candidate.ExternalType, &candidate.Source, &candidate.Score, &accepted, &reasonJSON, &rawResultJSON, &candidate.CreatedAt)
	if err != nil {
		return MatchCandidate{}, false
	}
	candidate.Accepted = accepted == 1
	_ = json.Unmarshal([]byte(reasonJSON), &candidate.Reasons)
	enrichMatchCandidateFromRaw(&candidate, rawResultJSON)
	return candidate, true
}

func (s *Server) refreshMediaMetadata(ctx context.Context, item MediaItem) (MediaItem, error) {
	providerID := s.metadataProviderForItem(item)
	provider, ok := s.metadataProviderByID(providerID)
	if !ok || !provider.Supports(item) {
		return MediaItem{}, fmt.Errorf("metadata provider %q is not available for %s", providerID, item.Type)
	}
	if provider.ID() == "musicbrainz" {
		if item.Type == "track" && s.sonicFingerprintingEnabled(item) {
			if updated, err := s.refreshTrackMetadataFromAcoustID(ctx, item); err == nil {
				return updated, nil
			} else {
				s.log.Warn("sonic fingerprint metadata lookup failed", "media", item.ID, "error", err)
			}
		}
	}
	return provider.Refresh(ctx, s, item)
}

func (s *Server) searchMediaMatchCandidates(ctx context.Context, userID, mediaID, query string) ([]MatchCandidate, error) {
	return s.searchMediaMatchCandidatesWithOptions(ctx, userID, mediaID, query, 0, "")
}

func (s *Server) searchMediaMatchCandidatesWithOptions(ctx context.Context, userID, mediaID, query string, yearOverride int, languageOverride string) ([]MatchCandidate, error) {
	item, err := s.getMediaContext(ctx, userID, mediaID)
	if err != nil {
		return nil, err
	}
	searchItem := item
	query = strings.TrimSpace(query)
	if query != "" {
		searchItem.Title = query
		searchItem.OriginalTitle = query
	}
	if yearOverride > 0 {
		searchItem.Year = yearOverride
	}
	languageOverride = strings.TrimSpace(languageOverride)
	provider := s.metadataProviderForItem(item)
	switch provider {
	case "tmdb":
		mediaType := tmdbSearchType(item.Type)
		if mediaType == "" {
			return nil, errors.New("TMDB matching is only available for movies, shows, seasons, and episodes")
		}
		var explicitTitles []string
		if query == "" {
			if item.Type == "season" || item.Type == "episode" {
				if seriesTitle := s.seriesTitleForTMDBSearch(item); seriesTitle != "" {
					searchItem.Title = seriesTitle
					searchItem.OriginalTitle = seriesTitle
					searchItem.SourceURL = ""
					explicitTitles = tmdbQueryTitleCandidates(seriesTitle)
				}
			}
		}
		titles := tmdbQueryTitlesForItem(searchItem)
		if len(explicitTitles) > 0 {
			titles = explicitTitles
		}
		if query != "" {
			titles = uniqueNonEmptyStrings(append(tmdbQueryTitleCandidates(query), tmdbSourceTitleCandidatesForItem(searchItem)...))
		}
		language := s.metadataLanguageForItem(item)
		if languageOverride != "" {
			language = normalizeMetadataLanguage(languageOverride)
		}
		_, err = s.searchTMDBCandidates(ctx, item, mediaType, titles, s.tmdbSearchYearForItem(searchItem), language)
	case "musicbrainz":
		err = s.searchMusicBrainzCandidates(ctx, item, searchItem)
	case "anilist":
		err = s.searchAniListCandidates(ctx, item, searchItem)
	default:
		err = fmt.Errorf("metadata provider %q is not available for manual matching", provider)
	}
	if err != nil {
		return nil, err
	}
	return s.matchCandidatesForMediaContext(ctx, mediaID), nil
}

func (s *Server) seriesTitleForTMDBSearch(item MediaItem) string {
	if item.Type == "episode" && strings.TrimSpace(item.GrandparentTitle) != "" {
		return strings.TrimSpace(item.GrandparentTitle)
	}
	if item.Type == "season" && strings.TrimSpace(item.ParentTitle) != "" {
		return strings.TrimSpace(item.ParentTitle)
	}
	if item.Type == "episode" && strings.TrimSpace(item.ParentID) != "" {
		var title string
		err := s.queryUserRow(context.Background(), `
				SELECT COALESCE(grandparent.title, '')
				FROM media_items parent
			LEFT JOIN media_items grandparent ON grandparent.id = parent.parent_id
			WHERE parent.id = ?`, item.ParentID).Scan(&title)
		if err == nil && strings.TrimSpace(title) != "" {
			return strings.TrimSpace(title)
		}
	}
	if strings.TrimSpace(item.Title) != "" {
		return strings.TrimSpace(item.Title)
	}
	return ""
}

func (s *Server) searchMusicBrainzCandidates(ctx context.Context, item MediaItem, searchItem MediaItem) error {
	switch item.Type {
	case "track":
		query := musicBrainzRecordingQuery(searchItem)
		if query == "" {
			return nil
		}
		var response musicBrainzRecordingSearchResponse
		if err := s.getMusicBrainz(ctx, "/recording", map[string]string{"query": query, "limit": "8", "inc": "artist-credits+releases+release-groups+media+labels+aliases+isrcs+genres+tags"}, &response); err != nil {
			return err
		}
		best := bestMusicBrainzRecording(response.Recordings, searchItem)
		for _, candidate := range response.Recordings {
			if candidate.ID == "" {
				continue
			}
			score := musicBrainzRecordingCandidateScore(candidate, searchItem)
			_ = s.recordMatchCandidate(item.ID, "musicbrainz", candidate.ID, "recording", "manual-search", score, best.ID != "" && candidate.ID == best.ID, query, candidate)
		}
	case "album":
		query := musicBrainzReleaseGroupQuery(searchItem)
		if query == "" {
			return nil
		}
		var response musicBrainzReleaseGroupSearchResponse
		if err := s.getMusicBrainz(ctx, "/release-group", map[string]string{"query": query, "limit": "8", "inc": "artist-credits+releases+media+labels+aliases+genres+tags"}, &response); err != nil {
			return err
		}
		best := bestMusicBrainzReleaseGroup(response.ReleaseGroups, searchItem)
		for _, candidate := range response.ReleaseGroups {
			if candidate.ID == "" {
				continue
			}
			score := musicBrainzReleaseGroupCandidateScore(candidate, searchItem)
			_ = s.recordMatchCandidate(item.ID, "musicbrainz", candidate.ID, "release-group", "manual-search", score, best.ID != "" && candidate.ID == best.ID, query, candidate)
		}
	case "artist":
		query := `artist:"` + musicBrainzQueryValue(searchItem.Title) + `"`
		var response struct {
			Artists []musicBrainzArtist `json:"artists"`
		}
		if err := s.getMusicBrainz(ctx, "/artist", map[string]string{"query": query, "limit": "8"}, &response); err != nil {
			return err
		}
		best := bestMusicBrainzArtist(response.Artists, searchItem)
		for _, candidate := range response.Artists {
			if candidate.ID == "" {
				continue
			}
			score := musicBrainzArtistCandidateScore(candidate, searchItem)
			_ = s.recordMatchCandidate(item.ID, "musicbrainz", candidate.ID, "artist", "manual-search", score, best.ID != "" && candidate.ID == best.ID, query, candidate)
		}
	default:
		return errors.New("MusicBrainz matching is only available for artists, albums, and tracks")
	}
	return nil
}

func (s *Server) searchAniListCandidates(ctx context.Context, item MediaItem, searchItem MediaItem) error {
	if item.Type != "anime" {
		return errors.New("AniList matching is only available for anime libraries")
	}
	query := firstNonEmpty(strings.TrimSpace(searchItem.OriginalTitle), strings.TrimSpace(searchItem.Title))
	if query == "" {
		return nil
	}
	results, err := s.searchAniListMedia(ctx, query)
	if err != nil {
		return err
	}
	best := bestAniListMedia(results, searchItem)
	for _, candidate := range results {
		if candidate.ID == 0 {
			continue
		}
		score := aniListCandidateScore(candidate, searchItem)
		_ = s.recordMatchCandidate(item.ID, "anilist", strconv.Itoa(candidate.ID), "anime", "manual-search", score, best.ID != 0 && candidate.ID == best.ID, query, candidate)
	}
	return nil
}

func (s *Server) applyManualMediaMatch(ctx context.Context, userID, mediaID string, req ManualMediaMatchRequest) (MediaItem, error) {
	item, err := s.getMediaDetailShell(userID, mediaID)
	if err != nil {
		return MediaItem{}, err
	}
	provider := normalizedMetadataProvider(req.Provider)
	externalID := strings.TrimSpace(req.ExternalID)
	externalType := strings.TrimSpace(req.ExternalType)
	if provider == "" || externalID == "" {
		return MediaItem{}, errors.New("provider and externalId are required")
	}
	switch provider {
	case "tmdb":
		return s.applyManualTMDBMatch(ctx, userID, item, externalID, externalType)
	case "musicbrainz":
		return s.applyManualMusicBrainzMatch(ctx, userID, item, externalID, externalType)
	case "anilist":
		return s.applyManualAniListMatch(ctx, userID, item, externalID, externalType)
	default:
		return MediaItem{}, fmt.Errorf("metadata provider %q is not supported", provider)
	}
}

func (s *Server) applyManualAniListMatch(ctx context.Context, userID string, item MediaItem, externalID, externalType string) (MediaItem, error) {
	if item.Type != "anime" {
		return MediaItem{}, errors.New("AniList matching is only available for anime libraries")
	}
	id, err := strconv.Atoi(externalID)
	if err != nil || id <= 0 {
		return MediaItem{}, errors.New("AniList externalId must be a positive integer")
	}
	result, err := s.aniListMediaByID(ctx, id)
	if err != nil {
		return MediaItem{}, err
	}
	if result.ID == 0 {
		result.ID = id
	}
	update := aniListUpdateForResult(result)
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataOrigin = metadataSourceProvider
	update.metadataSource = "manual-match"
	update.metadataProvider = "anilist"
	update.metadataActor = userID
	update.metadataRefreshed = true
	rich := mapAniListProviderRich(result)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{
		{Provider: "anilist", ExternalID: strconv.Itoa(result.ID), ExternalType: "anime", Confidence: 1, ExplicitAcceptance: true},
	}
	if result.IDMal > 0 {
		update.metadataIdentities = append(update.metadataIdentities, metadataProviderIdentityProposal{Provider: "mal", ExternalID: strconv.Itoa(result.IDMal), ExternalType: "anime", Confidence: .8})
	}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	score := aniListCandidateScore(result, item)
	score.add("manual_match", 100, "Selected by a user in the metadata editor")
	_ = s.recordMatchCandidate(item.ID, "anilist", strconv.Itoa(result.ID), "anime", "manual-match", score, true, item.Title, result)
	return s.getMediaDetailShell(userID, updated.ID)
}

func (s *Server) applyManualTMDBMatch(ctx context.Context, userID string, item MediaItem, externalID, externalType string) (MediaItem, error) {
	mediaType := strings.TrimSpace(externalType)
	if mediaType == "" {
		mediaType = tmdbSearchType(item.Type)
	}
	if mediaType == "" {
		return MediaItem{}, errors.New("TMDB matching is only available for movies, shows, seasons, and episodes")
	}
	id, err := strconv.Atoi(externalID)
	if err != nil || id <= 0 {
		return MediaItem{}, errors.New("TMDB externalId must be a positive integer")
	}
	language := s.metadataLanguageForItem(item)
	result, err := s.tmdbDetails(ctx, mediaType, id, language)
	if err != nil {
		return MediaItem{}, err
	}
	update, err := s.tmdbUpdateForResult(ctx, item, mediaType, result, language)
	if err != nil {
		return MediaItem{}, err
	}
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataOrigin = metadataSourceProvider
	update.metadataSource = "manual-match"
	update.metadataProvider = "tmdb"
	update.metadataActor = userID
	update.metadataRefreshed = true
	rich := mapTMDBProviderRich(result)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "tmdb", ExternalID: externalID, ExternalType: mediaType, Confidence: 1, ExplicitAcceptance: true}}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	score := tmdbResultCandidateScore(result, item, tmdbQueryTitlesForItem(item), metadataSearchYear(item))
	score.add("manual_match", 100, "Selected by a user in the metadata editor")
	_ = s.recordMatchCandidate(item.ID, "tmdb", externalID, mediaType, "manual-match", score, true, item.Title, result)
	return s.getMediaDetailShell(userID, updated.ID)
}

func (s *Server) applyManualMusicBrainzMatch(ctx context.Context, userID string, item MediaItem, externalID, externalType string) (MediaItem, error) {
	switch firstNonEmpty(strings.TrimSpace(externalType), manualMusicBrainzExternalType(item.Type)) {
	case "recording":
		var recording musicBrainzRecording
		if err := s.getMusicBrainz(ctx, "/recording/"+url.PathEscape(externalID), map[string]string{"inc": "artist-credits+releases+release-groups+media+labels+aliases+isrcs+genres+tags"}, &recording); err != nil {
			return MediaItem{}, err
		}
		if recording.ID == "" {
			recording.ID = externalID
		}
		trackArtistName, trackArtistID := musicBrainzArtistCreditNames(recording.ArtistCredit)
		release := s.preferredMusicBrainzReleaseForTrack(item, recording.Releases)
		albumArtistName, albumArtistID := musicBrainzReleaseArtistCreditNames(release)
		if albumArtistName == "" {
			albumArtistName = trackArtistName
			albumArtistID = trackArtistID
		}
		albumTitle := firstNonEmpty(release.Title, release.ReleaseGroup.Title)
		allowParentAlbumUpdate := item.ParentID != "" && !s.musicBrainzAlbumIdentityEstablished(item.ParentID)
		update := UpdateMediaRequest{}
		if recording.Title != "" {
			update.Title = &recording.Title
			sortTitle := sortableTitle(recording.Title)
			update.SortTitle = &sortTitle
		}
		if trackArtistName != "" {
			update.Studio = &trackArtistName
		}
		typed := map[string]string{}
		if trackArtistName != "" {
			typed["trackArtist"] = trackArtistName
			typed["artist"] = trackArtistName
		}
		if albumArtistName != "" {
			typed["albumArtist"] = albumArtistName
		}
		if albumTitle != "" {
			typed["albumTitle"] = albumTitle
		}
		if len(typed) > 0 {
			update.TypedMetadata = &typed
		}
		if year := yearFromMusicBrainzDate(release.Date); year > 0 {
			update.Year = &year
		}
		if genres := musicBrainzGenres(recording.Genres, recording.Tags); len(genres) > 0 {
			update.Genres = &genres
		}
		if recording.Length > 0 {
			duration := int((recording.Length + 500) / 1000)
			update.DurationSeconds = &duration
		}
		update.ExpectedRevision = &item.MetadataRevision
		update.metadataOrigin = metadataSourceProvider
		update.metadataSource = "manual-match"
		update.metadataProvider = "musicbrainz"
		update.metadataActor = userID
		update.metadataRefreshed = true
		rich := mapMusicBrainzRecordingProviderRich(recording)
		s.enrichMusicBrainzProposalWithCoverArt(ctx, &rich, release.ReleaseGroup.ID, release.ID)
		update.metadataRich = &rich
		update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: recording.ID, ExternalType: "recording", Confidence: 1, ExplicitAcceptance: true}}
		updated, err := s.saveMetadataUpdate("", item.ID, update)
		if err != nil {
			return MediaItem{}, err
		}
		_ = s.updateMusicParentsFromTrack(item, albumArtistName, albumArtistID, albumTitle, release, allowParentAlbumUpdate, "manual-match", .90)
		score := musicBrainzRecordingCandidateScore(recording, item)
		score.add("manual_match", 100, "Selected by a user in the metadata editor")
		_ = s.recordMatchCandidate(item.ID, "musicbrainz", recording.ID, "recording", "manual-match", score, true, musicBrainzRecordingQuery(item), recording)
		return s.getMediaDetailShell(userID, updated.ID)
	case "release-group":
		var group musicBrainzReleaseGroup
		if err := s.getMusicBrainz(ctx, "/release-group/"+url.PathEscape(externalID), map[string]string{"inc": "artist-credits+releases+media+labels+aliases+genres+tags"}, &group); err != nil {
			return MediaItem{}, err
		}
		if group.ID == "" {
			group.ID = externalID
		}
		artistName, artistID := musicBrainzArtistCreditNames(group.ArtistCredit)
		update := UpdateMediaRequest{}
		if group.Title != "" {
			update.Title = &group.Title
			sortTitle := sortableTitle(group.Title)
			update.SortTitle = &sortTitle
		}
		typed := map[string]string{}
		if artistName != "" {
			update.Studio = &artistName
			typed["albumArtist"] = artistName
		}
		if len(typed) > 0 {
			update.TypedMetadata = &typed
		}
		if year := yearFromMusicBrainzDate(group.FirstReleaseDate); year > 0 {
			update.Year = &year
		}
		if genres := musicBrainzGenres(group.Genres, group.Tags); len(genres) > 0 {
			update.Genres = &genres
		}
		releaseID := musicBrainzReleaseIDForGroup(group)
		if artwork := musicBrainzArtworkMap(group.ID, releaseID); len(artwork) > 0 {
			update.Artwork = &artwork
		}
		update.ExpectedRevision = &item.MetadataRevision
		update.metadataOrigin = metadataSourceProvider
		update.metadataSource = "manual-match"
		update.metadataProvider = "musicbrainz"
		update.metadataActor = userID
		update.metadataRefreshed = true
		rich := mapMusicBrainzReleaseGroupProviderRich(group)
		s.enrichMusicBrainzProposalWithCoverArt(ctx, &rich, group.ID, releaseID)
		update.metadataRich = &rich
		update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: group.ID, ExternalType: "release-group", Confidence: 1, ExplicitAcceptance: true}}
		if releaseID != "" {
			update.metadataIdentities = append(update.metadataIdentities, metadataProviderIdentityProposal{Provider: "musicbrainz", ExternalID: releaseID, ExternalType: "release", Confidence: .9})
		}
		updated, err := s.saveMetadataUpdate("", item.ID, update)
		if err != nil {
			return MediaItem{}, err
		}
		if artistID != "" && item.ParentID != "" {
			_ = s.applyProviderIdentitiesToMedia(ctx, item.ParentID, "musicbrainz", "manual-match", []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: artistID, ExternalType: "artist", Confidence: .90}})
		}
		score := musicBrainzReleaseGroupCandidateScore(group, item)
		score.add("manual_match", 100, "Selected by a user in the metadata editor")
		_ = s.recordMatchCandidate(item.ID, "musicbrainz", group.ID, "release-group", "manual-match", score, true, musicBrainzReleaseGroupQuery(item), group)
		return s.getMediaDetailShell(userID, updated.ID)
	case "artist":
		var artist musicBrainzArtist
		if err := s.getMusicBrainz(ctx, "/artist/"+url.PathEscape(externalID), nil, &artist); err != nil {
			return MediaItem{}, err
		}
		if artist.ID == "" {
			artist.ID = externalID
		}
		update := UpdateMediaRequest{}
		if artist.Name != "" {
			update.Title = &artist.Name
			sortTitle := sortableTitle(artist.Name)
			update.SortTitle = &sortTitle
			typed := map[string]string{"artist": artist.Name}
			update.TypedMetadata = &typed
		}
		update.ExpectedRevision = &item.MetadataRevision
		update.metadataOrigin = metadataSourceProvider
		update.metadataSource = "manual-match"
		update.metadataProvider = "musicbrainz"
		update.metadataActor = userID
		update.metadataRefreshed = true
		rich := mapMusicBrainzArtistProviderRich(artist)
		update.metadataRich = &rich
		update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: artist.ID, ExternalType: "artist", Confidence: 1, ExplicitAcceptance: true}}
		updated, err := s.saveMetadataUpdate("", item.ID, update)
		if err != nil {
			return MediaItem{}, err
		}
		score := musicBrainzArtistCandidateScore(artist, item)
		score.add("manual_match", 100, "Selected by a user in the metadata editor")
		_ = s.recordMatchCandidate(item.ID, "musicbrainz", artist.ID, "artist", "manual-match", score, true, item.Title, artist)
		return s.getMediaDetailShell(userID, updated.ID)
	default:
		return MediaItem{}, errors.New("unsupported MusicBrainz match type for this media item")
	}
}

func manualMusicBrainzExternalType(mediaType string) string {
	switch mediaType {
	case "track":
		return "recording"
	case "album":
		return "release-group"
	case "artist":
		return "artist"
	default:
		return ""
	}
}

func (s *Server) sonicFingerprintingEnabled(item MediaItem) bool {
	if item.Type != "track" || strings.TrimSpace(item.LibraryID) == "" {
		return false
	}
	library, err := s.getLibrary(item.LibraryID)
	if err != nil || library.Type != "music" {
		return false
	}
	return settingBool(library.Settings, "sonicFingerprinting", false)
}

func (s *Server) refreshMediaMetadataFromTMDB(ctx context.Context, item MediaItem) (MediaItem, error) {
	if !s.tmdbConfigured() {
		return MediaItem{}, errTMDBCredentialsMissing
	}
	item = s.withMediaFilesForMatching(item)
	mediaType := tmdbSearchType(item.Type)
	if mediaType == "" {
		return MediaItem{}, errors.New("TMDB refresh is only available for movies, shows, anime, and episodes")
	}
	queryTitles := tmdbQueryTitlesForItem(item)
	if item.Type == "season" && item.ParentTitle != "" {
		queryTitles = tmdbQueryTitleCandidates(item.ParentTitle)
	}
	if item.Type == "episode" && item.GrandparentTitle != "" {
		queryTitles = tmdbQueryTitleCandidates(item.GrandparentTitle)
	}
	language := s.metadataLanguageForItem(item)
	result, source, err := s.resolveTMDBResult(ctx, item, mediaType, queryTitles, s.tmdbSearchYearForItem(item), language)
	if err != nil {
		return MediaItem{}, err
	}
	if result.ID == 0 {
		return MediaItem{}, errors.New("no TMDB match was found")
	}
	if details, err := s.tmdbDetails(ctx, mediaType, result.ID, language); err == nil && details.ID != 0 {
		result = mergeTMDBSearchResult(result, details)
	}
	update, err := s.tmdbUpdateForResult(ctx, item, mediaType, result, language)
	if err != nil {
		return MediaItem{}, err
	}
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataSource = source
	update.metadataProvider = "tmdb"
	update.metadataRefreshed = true
	rich := mapTMDBProviderRich(result)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "tmdb", ExternalID: strconv.Itoa(result.ID), ExternalType: mediaType, Confidence: .85}}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	return updated, nil
}

func (s *Server) tmdbUpdateForResult(ctx context.Context, item MediaItem, mediaType string, result tmdbSearchResult, language string) (UpdateMediaRequest, error) {
	update := UpdateMediaRequest{}
	if item.Type == "episode" && item.EpisodeNumber > 0 {
		details, err := s.tmdbEpisodeDetailsForItem(ctx, item, result.ID, language)
		if err != nil {
			return UpdateMediaRequest{}, err
		}
		if details.Name != "" {
			title := details.Name
			update.Title = &title
			sortTitle := sortableTitle(title)
			update.SortTitle = &sortTitle
		}
		if details.Overview != "" {
			update.Summary = &details.Overview
		}
		if year := yearFromTMDBDate(details.AirDate); year > 0 {
			update.Year = &year
		}
		if details.VoteAverage > 0 {
			rating := details.VoteAverage
			update.CommunityRating = &rating
		}
		if artwork := tmdbArtworkMap("", "", details.StillPath); len(artwork) > 0 {
			update.Artwork = &artwork
		}
		if people := tmdbEpisodePeople(details); len(people) > 0 {
			update.People = &people
		}
	} else if item.Type == "season" && seasonNumberForMetadata(item) >= 0 {
		seasonNumber := seasonNumberForMetadata(item)
		if item.SeasonNumber != seasonNumber {
			update.SeasonNumber = &seasonNumber
		}
		details, err := s.tmdbSeasonDetails(ctx, result.ID, seasonNumber, language)
		if err != nil {
			return UpdateMediaRequest{}, err
		}
		if details.Name != "" {
			title := details.Name
			update.Title = &title
			sortTitle := sortableTitle(title)
			update.SortTitle = &sortTitle
		}
		if details.Overview != "" {
			update.Summary = &details.Overview
		}
		if year := yearFromTMDBDate(details.AirDate); year > 0 {
			update.Year = &year
		}
		if artwork := tmdbArtworkMap(details.PosterPath, result.BackdropPath, ""); len(artwork) > 0 {
			update.Artwork = &artwork
		}
	}
	if title := result.displayTitle(); title != "" && item.Type != "episode" && update.Title == nil {
		update.Title = &title
		sortTitle := sortableTitle(title)
		update.SortTitle = &sortTitle
	}
	if result.Overview != "" && update.Summary == nil {
		update.Summary = &result.Overview
	}
	if year := result.displayYear(); year > 0 && update.Year == nil {
		update.Year = &year
	}
	if result.VoteAverage > 0 && update.CommunityRating == nil {
		rating := result.VoteAverage
		update.CommunityRating = &rating
	}
	genres := tmdbGenreNames(mediaType, result.GenreIDs)
	if len(genres) > 0 {
		update.Genres = &genres
	}
	if update.Artwork == nil {
		if artwork := tmdbArtworkMap(result.PosterPath, result.BackdropPath, ""); len(artwork) > 0 {
			update.Artwork = &artwork
		}
	}
	if update.People == nil {
		if people := tmdbPeopleFromCredits(result.Credits); len(people) > 0 {
			update.People = &people
		}
	}
	return update, nil
}

func seasonNumberForMetadata(item MediaItem) int {
	if item.SeasonNumber > 0 {
		return item.SeasonNumber
	}
	if item.Type == "season" && item.IndexNumber > 0 {
		return item.IndexNumber
	}
	return item.SeasonNumber
}

func mergeTMDBSearchResult(base, details tmdbSearchResult) tmdbSearchResult {
	if details.ID != 0 {
		base.ID = details.ID
	}
	if details.Title != "" {
		base.Title = details.Title
	}
	if details.Name != "" {
		base.Name = details.Name
	}
	if details.ReleaseDate != "" {
		base.ReleaseDate = details.ReleaseDate
	}
	if details.FirstAirDate != "" {
		base.FirstAirDate = details.FirstAirDate
	}
	if details.Overview != "" {
		base.Overview = details.Overview
	}
	if details.Popularity > 0 {
		base.Popularity = details.Popularity
	}
	if details.VoteAverage > 0 {
		base.VoteAverage = details.VoteAverage
	}
	if len(details.GenreIDs) > 0 {
		base.GenreIDs = details.GenreIDs
	}
	if details.PosterPath != "" {
		base.PosterPath = details.PosterPath
	}
	if details.BackdropPath != "" {
		base.BackdropPath = details.BackdropPath
	}
	if len(details.Credits.Cast) > 0 || len(details.Credits.Crew) > 0 {
		base.Credits = details.Credits
	}
	base.OriginalTitle = firstNonEmpty(details.OriginalTitle, base.OriginalTitle)
	base.OriginalName = firstNonEmpty(details.OriginalName, base.OriginalName)
	base.OriginalLanguage = firstNonEmpty(details.OriginalLanguage, base.OriginalLanguage)
	base.Status = firstNonEmpty(details.Status, base.Status)
	if details.Runtime > 0 {
		base.Runtime = details.Runtime
	}
	if len(details.EpisodeRunTime) > 0 {
		base.EpisodeRunTime = details.EpisodeRunTime
	}
	if len(details.SpokenLanguages) > 0 {
		base.SpokenLanguages = details.SpokenLanguages
	}
	if len(details.ProductionCountries) > 0 {
		base.ProductionCountries = details.ProductionCountries
	}
	if len(details.OriginCountry) > 0 {
		base.OriginCountry = details.OriginCountry
	}
	if len(details.ProductionCompanies) > 0 {
		base.ProductionCompanies = details.ProductionCompanies
	}
	if len(details.Networks) > 0 {
		base.Networks = details.Networks
	}
	if len(details.CreatedBy) > 0 {
		base.CreatedBy = details.CreatedBy
	}
	if len(details.Keywords.Keywords)+len(details.Keywords.Results) > 0 {
		base.Keywords = details.Keywords
	}
	if details.BelongsToCollection != nil {
		base.BelongsToCollection = details.BelongsToCollection
	}
	if len(details.ReleaseDates.Results) > 0 {
		base.ReleaseDates = details.ReleaseDates
	}
	if len(details.ContentRatings.Results) > 0 {
		base.ContentRatings = details.ContentRatings
	}
	if len(details.ExternalIDs) > 0 {
		base.ExternalIDs = details.ExternalIDs
	}
	if len(details.AlternativeTitles.Titles)+len(details.AlternativeTitles.Results) > 0 {
		base.AlternativeTitles = details.AlternativeTitles
	}
	if len(details.Images.Posters)+len(details.Images.Backdrops)+len(details.Images.Logos)+len(details.Images.Stills) > 0 {
		base.Images = details.Images
	}
	return base
}

func tmdbEpisodePeople(details tmdbEpisodeDetails) []MediaPerson {
	credits := details.Credits
	if len(details.GuestStars) > 0 {
		credits.Cast = append(append([]tmdbCreditPerson{}, details.GuestStars...), credits.Cast...)
	}
	return tmdbPeopleFromCredits(credits)
}

func tmdbPeopleFromCredits(credits tmdbCredits) []MediaPerson {
	people := make([]MediaPerson, 0, min(len(credits.Cast)+len(credits.Crew), 48))
	seen := map[string]bool{}
	add := func(person MediaPerson) {
		person.Name = strings.TrimSpace(person.Name)
		person.Role = strings.TrimSpace(person.Role)
		person.Character = strings.TrimSpace(person.Character)
		person.ImageURL = strings.TrimSpace(person.ImageURL)
		if person.Name == "" || person.Role == "" {
			return
		}
		key := strings.ToLower(person.Role + "\x00" + person.Name + "\x00" + person.Character)
		if seen[key] || len(people) >= 48 {
			return
		}
		seen[key] = true
		people = append(people, person)
	}
	for _, credit := range credits.Cast {
		name := strings.TrimSpace(credit.Name)
		if name == "" {
			continue
		}
		add(MediaPerson{
			Name:      name,
			Role:      "Actor",
			Character: strings.TrimSpace(credit.Character),
			SortOrder: max(0, credit.Order),
			ImageURL:  tmdbProfileImageURL(credit.ProfilePath),
			ProviderIDs: map[string]string{
				"tmdb": strconv.Itoa(credit.ID),
			},
		})
	}
	for _, credit := range credits.Crew {
		role := tmdbCrewRole(credit)
		if role == "" {
			continue
		}
		add(MediaPerson{
			Name:      strings.TrimSpace(credit.Name),
			Role:      role,
			SortOrder: len(people) + 1000,
			ImageURL:  tmdbProfileImageURL(credit.ProfilePath),
			ProviderIDs: map[string]string{
				"tmdb": strconv.Itoa(credit.ID),
			},
		})
	}
	return people
}

func tmdbCrewRole(credit tmdbCreditPerson) string {
	job := strings.TrimSpace(credit.Job)
	department := strings.TrimSpace(credit.Department)
	switch strings.ToLower(job) {
	case "director", "writer", "screenplay", "teleplay", "story", "creator", "executive producer", "producer":
		if strings.EqualFold(job, "screenplay") || strings.EqualFold(job, "teleplay") || strings.EqualFold(job, "story") {
			return "Writer"
		}
		return job
	}
	switch strings.ToLower(department) {
	case "directing":
		return "Director"
	case "writing":
		return "Writer"
	case "production":
		if strings.Contains(strings.ToLower(job), "producer") {
			return job
		}
	}
	return ""
}

func tmdbProfileImageURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return ""
	}
	return "https://image.tmdb.org/t/p/w185" + path
}

func (s *Server) replaceProviderMediaPeople(mediaID string, people []MediaPerson, provider string) error {
	provider = normalizedMetadataProvider(provider)
	if provider == "" {
		provider = "provider"
	}
	// Provider portraits are ingested before persistence so no Portico API ever
	// discloses or causes clients to fetch a third-party image URL. A failed
	// download leaves the portrait empty; metadata itself is still useful and a
	// later refresh can repair the image without blocking the client request.
	for index := range people {
		remoteURL := strings.TrimSpace(people[index].ImageURL)
		people[index].ImageURL = ""
		if remoteURL == "" {
			continue
		}
		identity := strings.TrimSpace(people[index].Name) + "\x00" + strings.TrimSpace(people[index].Role)
		if encoded, err := json.Marshal(people[index].ProviderIDs); err == nil {
			identity += "\x00" + string(encoded)
		}
		sum := sha256.Sum256([]byte(identity))
		kind := "person-" + hex.EncodeToString(sum[:8])
		if path, ok := s.cacheProviderOriginalArtwork(context.Background(), mediaID, kind, provider, remoteURL); ok {
			people[index].ImageURL = path
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.withUserTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return replaceScannedPeople(tx, mediaID, people, provider, now)
	}); err != nil {
		return err
	}
	s.publishDataChanged("data.changed", []string{"media", "library-items", "metadata"}, "media", mediaID, map[string]string{"source": provider, "kind": "people"})
	return nil
}

func (s *Server) tmdbEpisodeDetailsForItem(ctx context.Context, item MediaItem, showID int, language string) (tmdbEpisodeDetails, error) {
	if groupID := s.tmdbEpisodeGroupIDForItem(item); groupID != "" {
		details, err := s.tmdbEpisodeGroupEpisodeDetails(ctx, groupID, item.SeasonNumber, item.EpisodeNumber, language)
		if err == nil && (details.Name != "" || details.Overview != "" || details.StillPath != "") {
			return details, nil
		}
	}
	return s.tmdbEpisodeDetails(ctx, showID, item.SeasonNumber, item.EpisodeNumber, language)
}

func (s *Server) tmdbEpisodeGroupIDForItem(item MediaItem) string {
	if item.LibraryID != "" {
		if library, err := s.getLibrary(item.LibraryID); err == nil {
			if groupID := settingString(library.Settings, "tmdbEpisodeGroupId", ""); strings.TrimSpace(groupID) != "" {
				return strings.TrimSpace(groupID)
			}
		}
	}
	if item.GrandparentID != "" {
		if groupID, ok := s.mediaProviderID(item.GrandparentID, "tmdb", "episode_group"); ok {
			return groupID
		}
	}
	return ""
}

func (s *Server) tmdbEpisodeGroupEpisodeDetails(ctx context.Context, groupID string, seasonNumber, episodeNumber int, language string) (tmdbEpisodeDetails, error) {
	var details tmdbEpisodeGroupDetails
	path := "/tv/episode_group/" + url.PathEscape(groupID)
	if err := s.getTMDB(ctx, path, map[string]string{"language": normalizeMetadataLanguage(language)}, &details); err != nil {
		return tmdbEpisodeDetails{}, err
	}
	for _, group := range details.Groups {
		if group.Order != seasonNumber {
			continue
		}
		for _, episode := range group.Episodes {
			if episode.EpisodeNumber == episodeNumber {
				return episode, nil
			}
		}
	}
	return tmdbEpisodeDetails{}, sql.ErrNoRows
}

func (s *Server) refreshMediaMetadataFromAniList(ctx context.Context, item MediaItem) (MediaItem, error) {
	if item.Type != "anime" {
		return MediaItem{}, errors.New("AniList refresh is only available for anime library roots")
	}
	result, source, err := s.resolveAniListMedia(ctx, item)
	if err != nil {
		return MediaItem{}, err
	}
	if result.ID == 0 {
		return MediaItem{}, errors.New("no AniList anime match was found")
	}
	update := aniListUpdateForResult(result)
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataSource = source
	update.metadataProvider = "anilist"
	update.metadataRefreshed = true
	rich := mapAniListProviderRich(result)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "anilist", ExternalID: strconv.Itoa(result.ID), ExternalType: "anime", Confidence: .86}}
	if result.IDMal > 0 {
		update.metadataIdentities = append(update.metadataIdentities, metadataProviderIdentityProposal{Provider: "mal", ExternalID: strconv.Itoa(result.IDMal), ExternalType: "anime", Confidence: .8})
	}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	return updated, nil
}

func (s *Server) resolveAniListMedia(ctx context.Context, item MediaItem) (aniListMedia, string, error) {
	if id, ok := s.mediaProviderID(item.ID, "anilist", "anime"); ok {
		parsed, err := strconv.Atoi(id)
		if err == nil && parsed > 0 {
			result, err := s.aniListMediaByID(ctx, parsed)
			if err == nil && result.ID != 0 {
				score := aniListCandidateScore(result, item)
				_ = s.recordMatchCandidate(item.ID, "anilist", strconv.Itoa(result.ID), "anime", "provider-id", score, score.accepted(50), item.Title, result)
				if score.accepted(50) {
					return result, "provider-id", nil
				}
			}
		}
	}
	query := firstNonEmpty(strings.TrimSpace(item.OriginalTitle), strings.TrimSpace(item.Title))
	if query == "" {
		return aniListMedia{}, "", nil
	}
	results, err := s.searchAniListMedia(ctx, query)
	if err != nil {
		return aniListMedia{}, "", err
	}
	best := bestAniListMedia(results, item)
	for _, candidate := range results {
		if candidate.ID == 0 {
			continue
		}
		score := aniListCandidateScore(candidate, item)
		_ = s.recordMatchCandidate(item.ID, "anilist", strconv.Itoa(candidate.ID), "anime", "provider-search", score, best.ID != 0 && candidate.ID == best.ID, query, candidate)
	}
	return best, "provider-search", nil
}

func (s *Server) refreshMediaMetadataFromMusicBrainz(ctx context.Context, item MediaItem) (MediaItem, error) {
	switch item.Type {
	case "track":
		return s.refreshTrackMetadataFromMusicBrainz(ctx, item)
	case "album":
		return s.refreshAlbumMetadataFromMusicBrainz(ctx, item)
	case "artist":
		return s.refreshArtistMetadataFromMusicBrainz(ctx, item)
	default:
		return MediaItem{}, errors.New("MusicBrainz refresh is only available for artists, albums, and tracks")
	}
}

func (s *Server) refreshTrackMetadataFromMusicBrainz(ctx context.Context, item MediaItem) (MediaItem, error) {
	recording, source, err := s.resolveMusicBrainzRecording(ctx, item)
	if err != nil {
		return MediaItem{}, err
	}
	if recording.ID == "" {
		return MediaItem{}, errors.New("no MusicBrainz recording match was found")
	}
	trackArtistName, trackArtistID := musicBrainzArtistCreditNames(recording.ArtistCredit)
	release := s.preferredMusicBrainzReleaseForTrack(item, recording.Releases)
	albumArtistName, albumArtistID := musicBrainzReleaseArtistCreditNames(release)
	if albumArtistName == "" {
		albumArtistName = trackArtistName
		albumArtistID = trackArtistID
	}
	albumTitle := release.Title
	if albumTitle == "" && release.ReleaseGroup.Title != "" {
		albumTitle = release.ReleaseGroup.Title
	}
	allowParentAlbumUpdate := item.ParentID != "" && !s.musicBrainzAlbumIdentityEstablished(item.ParentID)
	update := UpdateMediaRequest{}
	if recording.Title != "" {
		title := recording.Title
		update.Title = &title
		sortTitle := sortableTitle(title)
		update.SortTitle = &sortTitle
	}
	if trackArtistName != "" {
		update.Studio = &trackArtistName
	}
	typed := map[string]string{}
	if trackArtistName != "" {
		typed["trackArtist"] = trackArtistName
		typed["artist"] = trackArtistName
	}
	if albumArtistName != "" {
		typed["albumArtist"] = albumArtistName
	}
	if albumTitle != "" {
		typed["albumTitle"] = albumTitle
	}
	if recording.ID != "" {
		typed["recordingID"] = recording.ID
	}
	if release.ID != "" {
		typed["releaseID"] = release.ID
	}
	if release.ReleaseGroup.ID != "" {
		typed["releaseGroupID"] = release.ReleaseGroup.ID
	}
	if len(typed) > 0 {
		update.TypedMetadata = &typed
	}
	if year := yearFromMusicBrainzDate(release.Date); year > 0 && item.Year == 0 {
		update.Year = &year
	}
	genres := musicBrainzGenres(recording.Genres, recording.Tags)
	if len(genres) > 0 {
		update.Genres = &genres
	}
	if recording.Length > 0 && item.DurationSeconds == 0 {
		duration := int((recording.Length + 500) / 1000)
		update.DurationSeconds = &duration
	}
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataSource = source
	update.metadataProvider = "musicbrainz"
	update.metadataRefreshed = true
	rich := mapMusicBrainzRecordingProviderRich(recording)
	s.enrichMusicBrainzProposalWithCoverArt(ctx, &rich, release.ReleaseGroup.ID, release.ID)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: recording.ID, ExternalType: "recording", Confidence: .86}}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	_ = s.updateMusicParentsFromTrack(item, albumArtistName, albumArtistID, albumTitle, release, allowParentAlbumUpdate, source, .82)
	return updated, nil
}

func (s *Server) updateMusicParentsFromTrack(item MediaItem, artistName, artistID, albumTitle string, release musicBrainzRelease, allowAlbumUpdate bool, source string, confidence float64) error {
	if item.ParentID != "" && allowAlbumUpdate && (albumTitle != "" || artistName != "" || release.ReleaseGroup.ID != "") {
		update := UpdateMediaRequest{}
		if albumTitle != "" {
			update.Title = &albumTitle
			sortTitle := sortableTitle(albumTitle)
			update.SortTitle = &sortTitle
		}
		if artistName != "" {
			update.Studio = &artistName
		}
		typed := map[string]string{}
		if artistName != "" {
			typed["albumArtist"] = artistName
			typed["artist"] = artistName
		}
		if albumTitle != "" {
			typed["albumTitle"] = albumTitle
		}
		if release.ID != "" {
			typed["releaseID"] = release.ID
		}
		if release.ReleaseGroup.ID != "" {
			typed["releaseGroupID"] = release.ReleaseGroup.ID
		}
		if len(typed) > 0 {
			update.TypedMetadata = &typed
		}
		if year := yearFromMusicBrainzDate(release.Date); year > 0 {
			update.Year = &year
		}
		if artwork := musicBrainzArtworkMap(release.ReleaseGroup.ID, release.ID); len(artwork) > 0 {
			update.Artwork = &artwork
		}
		update.metadataOrigin = metadataSourceProvider
		update.metadataSource = source
		update.metadataProvider = "musicbrainz"
		if release.ReleaseGroup.ID != "" {
			update.metadataIdentities = append(update.metadataIdentities, metadataProviderIdentityProposal{Provider: "musicbrainz", ExternalID: release.ReleaseGroup.ID, ExternalType: "release-group", Confidence: confidence})
		}
		if release.ID != "" {
			update.metadataIdentities = append(update.metadataIdentities, metadataProviderIdentityProposal{Provider: "musicbrainz", ExternalID: release.ID, ExternalType: "release", Confidence: maxFloat(0, confidence-.04)})
		}
		if _, err := s.saveMetadataUpdate("", item.ParentID, update); err != nil {
			return err
		}
	}
	if item.GrandparentID != "" && artistName != "" {
		update := UpdateMediaRequest{Title: &artistName}
		sortTitle := sortableTitle(artistName)
		update.SortTitle = &sortTitle
		update.metadataOrigin = metadataSourceProvider
		update.metadataSource = source
		update.metadataProvider = "musicbrainz"
		if artistID != "" {
			update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: artistID, ExternalType: "artist", Confidence: confidence}}
		}
		if _, err := s.saveMetadataUpdate("", item.GrandparentID, update); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) preferredMusicBrainzReleaseForTrack(item MediaItem, releases []musicBrainzRelease) musicBrainzRelease {
	if len(releases) == 0 {
		return musicBrainzRelease{}
	}
	if item.ParentID != "" {
		if releaseID, ok := s.mediaProviderID(item.ParentID, "musicbrainz", "release"); ok {
			for _, release := range releases {
				if strings.TrimSpace(release.ID) == releaseID {
					return release
				}
			}
		}
		if groupID, ok := s.mediaProviderID(item.ParentID, "musicbrainz", "release-group"); ok {
			for _, release := range releases {
				if strings.TrimSpace(release.ReleaseGroup.ID) == groupID {
					return release
				}
			}
		}
	}
	return musicBrainzPreferredRelease(releases, item.ParentTitle)
}

func (s *Server) musicBrainzAlbumIdentityEstablished(mediaID string) bool {
	if strings.TrimSpace(mediaID) == "" {
		return false
	}
	if _, ok := s.mediaProviderID(mediaID, "musicbrainz", "release-group"); ok {
		return true
	}
	if _, ok := s.mediaProviderID(mediaID, "musicbrainz", "release"); ok {
		return true
	}
	return false
}

func (s *Server) refreshAlbumMetadataFromMusicBrainz(ctx context.Context, item MediaItem) (MediaItem, error) {
	group, source, err := s.resolveMusicBrainzReleaseGroup(ctx, item)
	if err != nil {
		return MediaItem{}, err
	}
	if group.ID == "" {
		return MediaItem{}, errors.New("no MusicBrainz release group match was found")
	}
	artistName, artistID := musicBrainzArtistCreditNames(group.ArtistCredit)
	update := UpdateMediaRequest{}
	if group.Title != "" {
		update.Title = &group.Title
		sortTitle := sortableTitle(group.Title)
		update.SortTitle = &sortTitle
	}
	if artistName != "" {
		update.Studio = &artistName
	}
	typed := map[string]string{}
	if artistName != "" {
		typed["albumArtist"] = artistName
		typed["artist"] = artistName
	}
	if group.ID != "" {
		typed["releaseGroupID"] = group.ID
	}
	if len(typed) > 0 {
		update.TypedMetadata = &typed
	}
	if year := yearFromMusicBrainzDate(group.FirstReleaseDate); year > 0 && item.Year == 0 {
		update.Year = &year
	}
	genres := musicBrainzGenres(group.Genres, group.Tags)
	if len(genres) > 0 {
		update.Genres = &genres
	}
	releaseID := musicBrainzReleaseIDForGroup(group)
	if artwork := musicBrainzArtworkMap(group.ID, releaseID); len(artwork) > 0 {
		update.Artwork = &artwork
	}
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataSource = source
	update.metadataProvider = "musicbrainz"
	update.metadataRefreshed = true
	rich := mapMusicBrainzReleaseGroupProviderRich(group)
	s.enrichMusicBrainzProposalWithCoverArt(ctx, &rich, group.ID, releaseID)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: group.ID, ExternalType: "release-group", Confidence: .84}}
	if releaseID != "" {
		update.metadataIdentities = append(update.metadataIdentities, metadataProviderIdentityProposal{Provider: "musicbrainz", ExternalID: releaseID, ExternalType: "release", Confidence: .8})
	}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	if artistID != "" && item.ParentID != "" {
		_ = s.applyProviderIdentitiesToMedia(ctx, item.ParentID, "musicbrainz", source, []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: artistID, ExternalType: "artist", Confidence: .80}})
	}
	return updated, nil
}

func (s *Server) refreshArtistMetadataFromMusicBrainz(ctx context.Context, item MediaItem) (MediaItem, error) {
	artist, source, err := s.resolveMusicBrainzArtist(ctx, item)
	if err != nil {
		return MediaItem{}, err
	}
	if artist.ID == "" {
		return MediaItem{}, errors.New("no MusicBrainz artist match was found")
	}
	update := UpdateMediaRequest{}
	if artist.Name != "" {
		update.Title = &artist.Name
		sortTitle := sortableTitle(artist.Name)
		update.SortTitle = &sortTitle
		typed := map[string]string{"artist": artist.Name}
		update.TypedMetadata = &typed
	}
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataSource = source
	update.metadataProvider = "musicbrainz"
	update.metadataRefreshed = true
	rich := mapMusicBrainzArtistProviderRich(artist)
	update.metadataRich = &rich
	update.metadataIdentities = []metadataProviderIdentityProposal{{Provider: "musicbrainz", ExternalID: artist.ID, ExternalType: "artist", Confidence: .84}}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	return updated, nil
}

func (s *Server) resolveTMDBResult(ctx context.Context, item MediaItem, mediaType string, queryTitles []string, year int, language string) (tmdbSearchResult, string, error) {
	if id, ok := s.tmdbProviderIDForItem(item, mediaType); ok {
		result, err := s.tmdbDetails(ctx, mediaType, id, language)
		if err == nil && result.ID != 0 {
			score := tmdbResultCandidateScore(result, item, queryTitles, year)
			_ = s.recordMatchCandidate(item.ID, "tmdb", strconv.Itoa(result.ID), mediaType, "provider-id", score, score.accepted(50), strings.Join(queryTitles, " | "), result)
			if score.accepted(50) {
				return result, "provider-id", nil
			}
		}
	}
	result, err := s.searchTMDBCandidates(ctx, item, mediaType, queryTitles, year, language)
	if err != nil {
		return tmdbSearchResult{}, "", err
	}
	return result, "provider-search", nil
}

func (s *Server) tmdbProviderIDForItem(item MediaItem, mediaType string) (int, bool) {
	for _, mediaID := range uniqueNonEmptyStrings([]string{item.ID, item.GrandparentID, item.ParentID}) {
		externalID, ok := s.mediaProviderID(mediaID, "tmdb", mediaType)
		if !ok {
			continue
		}
		id, err := strconv.Atoi(externalID)
		if err == nil && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func (s *Server) resolveMusicBrainzRecording(ctx context.Context, item MediaItem) (musicBrainzRecording, string, error) {
	if id, providerSource, ok := s.mediaProviderIDWithSource(item.ID, "musicbrainz", "recording"); ok {
		var recording musicBrainzRecording
		if err := s.getMusicBrainz(ctx, "/recording/"+url.PathEscape(id), map[string]string{"inc": "artist-credits+releases+release-groups+media+labels+aliases+isrcs+genres+tags"}, &recording); err == nil && recording.ID != "" {
			score := musicBrainzRecordingCandidateScore(recording, item)
			if strings.HasPrefix(providerSource, "acoustid") {
				score.add("acoustid_recording_match", 35, providerSource)
			}
			_ = s.recordMatchCandidate(item.ID, "musicbrainz", recording.ID, "recording", "provider-id", score, strings.HasPrefix(providerSource, "acoustid") || score.accepted(50), musicBrainzRecordingQuery(item), recording)
			if strings.HasPrefix(providerSource, "acoustid") || score.accepted(50) {
				return recording, "provider-id", nil
			}
		}
	}
	query := musicBrainzRecordingQuery(item)
	if query == "" {
		return musicBrainzRecording{}, "", nil
	}
	var response musicBrainzRecordingSearchResponse
	if err := s.getMusicBrainz(ctx, "/recording", map[string]string{"query": query, "limit": "5", "inc": "artist-credits+releases+release-groups+media+labels+aliases+isrcs+genres+tags"}, &response); err != nil {
		return musicBrainzRecording{}, "", err
	}
	if len(response.Recordings) == 0 {
		return musicBrainzRecording{}, "provider-search", nil
	}
	best := bestMusicBrainzRecording(response.Recordings, item)
	for _, candidate := range response.Recordings {
		if candidate.ID == "" {
			continue
		}
		score := musicBrainzRecordingCandidateScore(candidate, item)
		_ = s.recordMatchCandidate(item.ID, "musicbrainz", candidate.ID, "recording", "provider-search", score, best.ID != "" && candidate.ID == best.ID, query, candidate)
	}
	return best, "provider-search", nil
}

func (s *Server) resolveMusicBrainzReleaseGroup(ctx context.Context, item MediaItem) (musicBrainzReleaseGroup, string, error) {
	if id, ok := s.mediaProviderID(item.ID, "musicbrainz", "release-group"); ok {
		var group musicBrainzReleaseGroup
		if err := s.getMusicBrainz(ctx, "/release-group/"+url.PathEscape(id), map[string]string{"inc": "artist-credits+releases+media+labels+aliases+genres+tags"}, &group); err == nil && group.ID != "" {
			score := musicBrainzReleaseGroupCandidateScore(group, item)
			_ = s.recordMatchCandidate(item.ID, "musicbrainz", group.ID, "release-group", "provider-id", score, score.accepted(50), musicBrainzReleaseGroupQuery(item), group)
			if score.accepted(50) {
				return group, "provider-id", nil
			}
		}
	}
	query := musicBrainzReleaseGroupQuery(item)
	if query == "" {
		return musicBrainzReleaseGroup{}, "", nil
	}
	var response musicBrainzReleaseGroupSearchResponse
	if err := s.getMusicBrainz(ctx, "/release-group", map[string]string{"query": query, "limit": "5", "inc": "artist-credits+releases+media+labels+aliases+genres+tags"}, &response); err != nil {
		return musicBrainzReleaseGroup{}, "", err
	}
	if len(response.ReleaseGroups) == 0 {
		return musicBrainzReleaseGroup{}, "provider-search", nil
	}
	best := bestMusicBrainzReleaseGroup(response.ReleaseGroups, item)
	for _, candidate := range response.ReleaseGroups {
		if candidate.ID == "" {
			continue
		}
		score := musicBrainzReleaseGroupCandidateScore(candidate, item)
		_ = s.recordMatchCandidate(item.ID, "musicbrainz", candidate.ID, "release-group", "provider-search", score, best.ID != "" && candidate.ID == best.ID, query, candidate)
	}
	return best, "provider-search", nil
}

func (s *Server) resolveMusicBrainzArtist(ctx context.Context, item MediaItem) (musicBrainzArtist, string, error) {
	if id, ok := s.mediaProviderID(item.ID, "musicbrainz", "artist"); ok {
		var artist musicBrainzArtist
		if err := s.getMusicBrainz(ctx, "/artist/"+url.PathEscape(id), nil, &artist); err == nil && artist.ID != "" {
			return artist, "provider-id", nil
		}
	}
	query := `artist:"` + musicBrainzQueryValue(item.Title) + `"`
	var response struct {
		Artists []musicBrainzArtist `json:"artists"`
	}
	if err := s.getMusicBrainz(ctx, "/artist", map[string]string{"query": query, "limit": "5"}, &response); err != nil {
		return musicBrainzArtist{}, "", err
	}
	if len(response.Artists) == 0 {
		return musicBrainzArtist{}, "provider-search", nil
	}
	best := bestMusicBrainzArtist(response.Artists, item)
	for _, candidate := range response.Artists {
		if candidate.ID == "" {
			continue
		}
		score := musicBrainzArtistCandidateScore(candidate, item)
		_ = s.recordMatchCandidate(item.ID, "musicbrainz", candidate.ID, "artist", "provider-search", score, best.ID != "" && candidate.ID == best.ID, query, candidate)
	}
	return best, "provider-search", nil
}

func (s *Server) getMusicBrainz(ctx context.Context, path string, queryParams map[string]string, out any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Second * time.Duration(attempt)
			if retryErr, ok := lastErr.(musicBrainzRetryError); ok && retryErr.after > delay {
				delay = retryErr.after
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		err := s.getMusicBrainzOnce(ctx, path, queryParams, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if _, ok := err.(musicBrainzRetryError); !ok {
			return err
		}
	}
	return lastErr
}

type musicBrainzRetryError struct {
	status string
	after  time.Duration
	cause  error
}

func (e musicBrainzRetryError) Error() string { return "MusicBrainz request returned " + e.status }

func (e musicBrainzRetryError) Unwrap() error { return e.cause }

func (s *Server) getMusicBrainzOnce(ctx context.Context, path string, queryParams map[string]string, out any) error {
	musicBrainzThrottleMu.Lock()
	wait := time.Second - time.Since(musicBrainzLastRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			musicBrainzThrottleMu.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}
	musicBrainzLastRequest = time.Now()
	musicBrainzThrottleMu.Unlock()

	baseURL := strings.TrimRight(s.cfg.MusicBrainzBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://musicbrainz.org/ws/2"
	}
	endpoint, err := url.Parse(baseURL + path)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	query.Set("fmt", "json")
	for key, value := range queryParams {
		if strings.TrimSpace(value) != "" {
			query.Set(key, value)
		}
	}
	endpoint.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		cancel()
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Portico/0.1 ( https://getportico.tv )")
	response, responseErr := doProviderJSONRequest(requestCtx, "MusicBrainz", req, maxMetadataProviderResponseBytes, out)
	cancel()
	if response.StatusCode == http.StatusServiceUnavailable {
		return musicBrainzRetryError{status: response.Status, after: retryAfterDuration(response.Header.Get("Retry-After")), cause: responseErr}
	}
	return responseErr
}

// getCoverArtArchive performs bounded discovery only. Callers stage the returned
// proposal with the same metadata apply operation as the MusicBrainz payload.
// A missing release or transient CAA failure intentionally produces no proposal.
func (s *Server) getCoverArtArchive(ctx context.Context, entityType, entityID string) (metadataProviderRichProposal, bool) {
	entityType, entityID = strings.TrimSpace(entityType), strings.TrimSpace(entityID)
	if (entityType != "release" && entityType != "release-group") || entityID == "" {
		return metadataProviderRichProposal{}, false
	}
	baseURL := strings.TrimRight(s.cfg.CoverArtArchiveBaseURL, "/")
	if baseURL == "" {
		return metadataProviderRichProposal{}, false
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, baseURL+"/"+entityType+"/"+url.PathEscape(entityID), nil)
	if err != nil {
		cancel()
		return metadataProviderRichProposal{}, false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Portico/0.1 ( https://getportico.tv )")
	var payload coverArtArchiveResponse
	_, responseErr := doProviderJSONRequest(requestCtx, "Cover Art Archive", req, maxCoverArtArchiveResponseBytes, &payload)
	cancel()
	if responseErr != nil {
		return metadataProviderRichProposal{}, false
	}
	if len(payload.Images) > 24 {
		payload.Images = payload.Images[:24]
	}
	proposal := mapCoverArtArchiveProviderRich(entityType, entityID, payload)
	return proposal, len(proposal.Images) > 0
}

func (s *Server) enrichMusicBrainzProposalWithCoverArt(ctx context.Context, proposal *metadataProviderRichProposal, groupID, releaseID string) {
	if proposal == nil {
		return
	}
	// A release is the most precise source. Fall back to its release group and
	// stop after the first useful response, keeping discovery bounded.
	for _, candidate := range [][2]string{{"release", releaseID}, {"release-group", groupID}} {
		if artwork, ok := s.getCoverArtArchive(ctx, candidate[0], candidate[1]); ok {
			// Keep CAA as its own evidence source. Folding these rows into the
			// MusicBrainz proposal would discard the CAA snapshot and falsely
			// attribute its artwork and relationships to MusicBrainz.
			proposal.Supplements = append(proposal.Supplements, artwork)
			return
		}
	}
}

const aniListMediaFields = `id idMal title { romaji english native } synonyms description format status episodes duration season seasonYear averageScore isAdult startDate { year month day } endDate { year month day } source countryOfOrigin genres tags { name rank } coverImage { extraLarge large medium color } bannerImage studios(isMain: true) { nodes { id name } } staff(sort: RELEVANCE, page: 1, perPage: 25) { pageInfo { total perPage currentPage lastPage hasNextPage } edges { role node { id name { full native } image { large medium } } } } characters(sort: ROLE, page: 1, perPage: 25) { pageInfo { total perPage currentPage lastPage hasNextPage } edges { role node { id name { full native } image { large medium } } voiceActors(sort: RELEVANCE) { id name { full native } image { large medium } } } } relations { pageInfo { total perPage currentPage lastPage hasNextPage } edges { relationType node { id idMal type format title { romaji english native } } } }`

func (s *Server) searchAniListMedia(ctx context.Context, query string) ([]aniListMedia, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var response aniListGraphQLResponse
	err := s.postAniListGraphQL(ctx, `query($search: String!) { Page(page: 1, perPage: 8) { media(type: ANIME, search: $search) { `+aniListMediaFields+` } } }`, map[string]any{"search": query}, &response)
	if err != nil {
		return nil, err
	}
	return response.Data.Search.Media, nil
}

func (s *Server) aniListMediaByID(ctx context.Context, id int) (aniListMedia, error) {
	var response aniListGraphQLResponse
	err := s.postAniListGraphQL(ctx, `query($id: Int!) { Media(id: $id, type: ANIME) { `+aniListMediaFields+` } }`, map[string]any{"id": id}, &response)
	if err != nil {
		return aniListMedia{}, err
	}
	// AniList connections are explicitly bounded: at most three 25-edge pages per
	// connection. pageInfo/total is retained so the proposal can prove truncation.
	for page := 2; page <= 3 && (response.Data.Media.Staff.PageInfo.HasNextPage || response.Data.Media.Characters.PageInfo.HasNextPage); page++ {
		var continuation aniListGraphQLResponse
		query := `query($id: Int!, $page: Int!) { Media(id: $id, type: ANIME) { staff(sort: RELEVANCE, page: $page, perPage: 25) { pageInfo { total perPage currentPage lastPage hasNextPage } edges { role node { id name { full native } image { large medium } } } } characters(sort: ROLE, page: $page, perPage: 25) { pageInfo { total perPage currentPage lastPage hasNextPage } edges { role node { id name { full native } image { large medium } } voiceActors(sort: RELEVANCE) { id name { full native } image { large medium } } } } } }`
		if err := s.postAniListGraphQL(ctx, query, map[string]any{"id": id, "page": page}, &continuation); err != nil {
			return aniListMedia{}, err
		}
		media := &response.Data.Media
		if media.Staff.PageInfo.HasNextPage {
			media.Staff.Edges = append(media.Staff.Edges, continuation.Data.Media.Staff.Edges...)
			media.Staff.PageInfo = continuation.Data.Media.Staff.PageInfo
		}
		if media.Characters.PageInfo.HasNextPage {
			media.Characters.Edges = append(media.Characters.Edges, continuation.Data.Media.Characters.Edges...)
			media.Characters.PageInfo = continuation.Data.Media.Characters.PageInfo
		}
	}
	return response.Data.Media, nil
}

func (s *Server) postAniListGraphQL(ctx context.Context, graphQL string, variables map[string]any, out *aniListGraphQLResponse) error {
	baseURL := strings.TrimRight(s.cfg.AniListBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://graphql.anilist.co"
	}
	body, err := json.Marshal(map[string]any{"query": graphQL, "variables": variables})
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := waitForAniListSlot(ctx); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, baseURL, bytes.NewReader(body))
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Portico/0.1 ( https://getportico.tv )")
		response, responseErr := doProviderJSONRequest(requestCtx, "AniList", req, maxMetadataProviderResponseBytes, out)
		retry := retryAfterDuration(response.Header.Get("Retry-After"))
		cancel()
		if responseErr != nil {
			if response.StatusCode == http.StatusTooManyRequests && attempt == 0 && retry > 0 && retry <= 30*time.Second {
				timer := time.NewTimer(retry)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
					continue
				}
			}
			return responseErr
		}
		break
	}
	if len(out.Errors) > 0 {
		messages := make([]string, 0, len(out.Errors))
		for _, apiErr := range out.Errors {
			messages = append(messages, strings.TrimSpace(apiErr.Message))
		}
		return fmt.Errorf("AniList request failed: %s", strings.Join(uniqueNonEmptyStrings(messages), "; "))
	}
	return nil
}

func waitForAniListSlot(ctx context.Context) error {
	aniListThrottleMu.Lock()
	wait := 2*time.Second - time.Since(aniListLastRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			aniListThrottleMu.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}
	aniListLastRequest = time.Now()
	aniListThrottleMu.Unlock()
	return nil
}

func retryAfterDuration(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return time.Until(at)
	}
	return 0
}

func (s *Server) mediaProviderID(mediaID, provider, externalType string) (string, bool) {
	externalID, _, ok := s.mediaProviderIDWithSource(mediaID, provider, externalType)
	return externalID, ok
}

func (s *Server) mediaProviderIDWithSource(mediaID, provider, externalType string) (string, string, bool) {
	var externalID, source string
	err := s.queryUserRow(context.Background(), `
		SELECT external_id, source FROM media_provider_ids
		WHERE media_id = ? AND provider = ? AND external_type = ? AND status = 'accepted'
		ORDER BY updated_at DESC LIMIT 1`,
		mediaID, normalizedMetadataProvider(provider), externalType).Scan(&externalID, &source)
	externalID = strings.TrimSpace(externalID)
	return externalID, strings.TrimSpace(source), err == nil && externalID != ""
}

func (s *Server) upsertMediaProviderID(mediaID, provider, externalID, externalType string, confidence float64, source string) error {
	provider = normalizedMetadataProvider(provider)
	externalID = strings.TrimSpace(externalID)
	if mediaID == "" || provider == "" || externalID == "" {
		return nil
	}
	return s.withUserTxTagged(context.Background(), []string{"media", mediaID, "metadata", "provider-identity"}, func(tx *sql.Tx) error {
		return upsertMediaProviderIdentityTx(tx, mediaID, provider, externalID, externalType, confidence, source,
			metadataSourceRequestsIdentityAcceptance(source), "", time.Now().UTC().Format(time.RFC3339Nano))
	})
}

func (s *Server) applyProviderIdentitiesToMedia(ctx context.Context, mediaID, provider, source string, identities []metadataProviderIdentityProposal) error {
	item, err := s.getMediaContext(ctx, "", mediaID)
	if err != nil {
		return err
	}
	_, err = s.applyMetadata(ctx, metadataApplyRequest{
		MediaID: item.ID, ExpectedRevision: item.MetadataRevision, Origin: metadataSourceProvider,
		Source: source, Provider: provider, Identities: identities,
	})
	return err
}

func (s *Server) tmdbDetails(ctx context.Context, mediaType string, id int, language string) (tmdbSearchResult, error) {
	var details tmdbSearchResult
	path := "/" + mediaType + "/" + strconv.Itoa(id)
	if err := s.getTMDB(ctx, path, map[string]string{"language": normalizeMetadataLanguage(language), "append_to_response": "credits,alternative_titles,keywords,release_dates,content_ratings,external_ids,images", "include_image_language": tmdbImageLanguage(language) + ",null"}, &details); err != nil {
		return tmdbSearchResult{}, err
	}
	if details.ID == 0 {
		details.ID = id
	}
	return details, nil
}

func tmdbImageLanguage(value string) string {
	language := normalizeMetadataLanguage(value)
	if separator := strings.IndexAny(language, "-_"); separator > 0 {
		language = language[:separator]
	}
	return strings.ToLower(strings.TrimSpace(language))
}

func (s *Server) tmdbEpisodeDetails(ctx context.Context, showID, seasonNumber, episodeNumber int, language string) (tmdbEpisodeDetails, error) {
	var details tmdbEpisodeDetails
	path := fmt.Sprintf("/tv/%d/season/%d/episode/%d", showID, seasonNumber, episodeNumber)
	if err := s.getTMDB(ctx, path, map[string]string{"language": normalizeMetadataLanguage(language), "append_to_response": "credits"}, &details); err != nil {
		return tmdbEpisodeDetails{}, err
	}
	return details, nil
}

func (s *Server) tmdbSeasonDetails(ctx context.Context, showID, seasonNumber int, language string) (tmdbSeasonDetails, error) {
	var details tmdbSeasonDetails
	path := fmt.Sprintf("/tv/%d/season/%d", showID, seasonNumber)
	if err := s.getTMDB(ctx, path, map[string]string{"language": normalizeMetadataLanguage(language)}, &details); err != nil {
		return tmdbSeasonDetails{}, err
	}
	return details, nil
}

func (s *Server) metadataAgentSettings() metadataAgentSettings {
	settings, err := s.loadSettings()
	if err != nil {
		return metadataAgentSettings{Movies: "TMDB", TV: "TMDB", Anime: "AniList", Music: "MusicBrainz", LocalNFO: true, EmbeddedTags: true, RefreshDays: 7}
	}
	group, _ := settings["metadataAgents"].(map[string]any)
	return metadataAgentSettings{
		Movies:               settingString(group, "movies", "TMDB"),
		TV:                   settingString(group, "tv", "TMDB"),
		Anime:                settingString(group, "anime", "AniList"),
		Music:                settingString(group, "music", "MusicBrainz"),
		LocalNFO:             settingBool(group, "localNFO", true),
		EmbeddedTags:         settingBool(group, "embeddedTags", true),
		CacheOriginalArtwork: settingBool(group, "cacheOriginalArtwork", false),
		RefreshDays:          max(1, settingInt(group, "refreshDays", 7)),
		MetadataLanguage:     normalizeMetadataLanguage(settingString(group, "metadataLanguage", "en-US")),
	}
}

func (s *Server) metadataProviderFor(mediaType string) string {
	settings := s.metadataAgentSettings()
	provider := settings.TV
	if mediaType == "movie" {
		provider = settings.Movies
	} else if mediaType == "anime" {
		provider = settings.Anime
	} else if mediaType == "music" || mediaType == "artist" || mediaType == "album" || mediaType == "track" {
		provider = settings.Music
	}
	switch normalizedMetadataProvider(provider) {
	case "", "default", "tmdb":
		if mediaType == "music" || mediaType == "artist" || mediaType == "album" || mediaType == "track" {
			return "musicbrainz"
		}
		return "tmdb"
	case "musicbrainz":
		return "musicbrainz"
	case "anilist":
		return "anilist"
	case "local", "none":
		return "none"
	default:
		return normalizedMetadataProvider(provider)
	}
}

func (s *Server) metadataProviderForItem(item MediaItem) string {
	if library, err := s.getLibrary(item.LibraryID); err == nil {
		disabled := metadataProviderSet(metadataProviderListFromSetting(library.Settings["disabledMetadataProviders"]))
		providerOrder := metadataProviderListFromSetting(library.Settings["metadataProviderOrder"])
		for _, provider := range providerOrder {
			if disabled[provider] || provider == "default" || provider == "local" {
				continue
			}
			if s.metadataProviderSupportsItem(provider, item) {
				return provider
			}
		}
		if len(providerOrder) > 0 {
			return "none"
		}
		if provider := normalizedMetadataProvider(settingString(library.Settings, "metadataProvider", "")); provider != "" && provider != "default" {
			if provider == "local" {
				return "none"
			}
			if provider == "tmdb" && !s.tmdbConfigured() {
				return "none"
			}
			if !disabled[provider] && (provider == "none" || s.metadataProviderSupportsItem(provider, item)) {
				return provider
			}
		}
	}
	provider := s.metadataProviderFor(item.Type)
	if provider == "tmdb" && !s.tmdbConfigured() {
		return "none"
	}
	return provider
}

func (s *Server) metadataProviderSupportsItem(provider string, item MediaItem) bool {
	if provider == "tmdb" && !s.tmdbConfigured() {
		return false
	}
	resolved, ok := s.metadataProviderByID(provider)
	return ok && resolved.Supports(item)
}

func metadataProviderSet(providers []string) map[string]bool {
	set := map[string]bool{}
	for _, provider := range providers {
		if provider != "" {
			set[provider] = true
		}
	}
	return set
}

func metadataProviderListFromSetting(value any) []string {
	var raw []string
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok {
				raw = append(raw, value)
			}
		}
	case []string:
		raw = typed
	case string:
		raw = strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == '\n' || r == ';'
		})
	}
	out := make([]string, 0, len(raw))
	for _, provider := range raw {
		provider = normalizedMetadataProvider(provider)
		if provider == "" {
			continue
		}
		out = append(out, provider)
	}
	return uniqueNonEmptyStrings(out)
}

func (s *Server) metadataLanguageForItem(item MediaItem) string {
	if library, err := s.getLibrary(item.LibraryID); err == nil {
		if language := settingString(library.Settings, "metadataLanguage", ""); strings.TrimSpace(language) != "" {
			return normalizeMetadataLanguage(language)
		}
	}
	return s.metadataAgentSettings().MetadataLanguage
}

func tmdbQueryTitlesForItem(item MediaItem) []string {
	var titles []string
	for _, candidate := range append([]string{item.OriginalTitle, item.Title}, matchingSourceNamesForItem(item)...) {
		titles = append(titles, tmdbQueryTitleCandidates(candidate)...)
	}
	return uniqueNonEmptyStrings(titles)
}

func tmdbSourceTitleCandidatesForItem(item MediaItem) []string {
	var titles []string
	for _, candidate := range matchingSourceNamesForItem(item) {
		titles = append(titles, tmdbQueryTitleCandidates(candidate)...)
	}
	return uniqueNonEmptyStrings(titles)
}

func metadataSearchYear(item MediaItem) int {
	for _, value := range append(matchingSourceNamesForItem(item), item.OriginalTitle, item.Title) {
		if year := yearFromReleaseName(value); year > 0 {
			return year
		}
	}
	return item.Year
}

func (s *Server) withMediaFilesForMatching(item MediaItem) MediaItem {
	if len(item.MediaFiles) > 0 || s == nil || s.db == nil || strings.TrimSpace(item.ID) == "" {
		return item
	}
	item.MediaFiles = s.mediaFilesFor(item.ID, item.SourceURL)
	return item
}

func matchingSourceNamesForItem(item MediaItem) []string {
	values := make([]string, 0, len(item.MediaFiles)*2+1)
	for _, file := range item.MediaFiles {
		values = append(values, file.OriginalFilename, file.Path)
	}
	values = append(values, item.SourceURL)
	return uniqueNonEmptyStrings(values)
}

func enrichMatchCandidateFromRaw(candidate *MatchCandidate, rawResultJSON string) {
	if candidate == nil || strings.TrimSpace(rawResultJSON) == "" {
		return
	}
	var raw struct {
		Title        string `json:"title"`
		Name         string `json:"name"`
		ReleaseDate  string `json:"release_date"`
		FirstAirDate string `json:"first_air_date"`
		PosterPath   string `json:"poster_path"`
		Overview     string `json:"overview"`
	}
	if err := json.Unmarshal([]byte(rawResultJSON), &raw); err != nil {
		return
	}
	candidate.Title = firstNonEmpty(candidate.Title, strings.TrimSpace(raw.Title), strings.TrimSpace(raw.Name))
	if candidate.Year == 0 {
		candidate.Year = yearFromTMDBDate(firstNonEmpty(raw.ReleaseDate, raw.FirstAirDate))
	}
	candidate.PosterURL = firstNonEmpty(candidate.PosterURL, tmdbPosterImageURL(raw.PosterPath))
	candidate.Overview = firstNonEmpty(candidate.Overview, strings.TrimSpace(raw.Overview))
}

func tmdbPosterImageURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return ""
	}
	return "https://image.tmdb.org/t/p/w185" + path
}

func (s *Server) tmdbSearchYearForItem(item MediaItem) int {
	switch item.Type {
	case "season":
		if year := s.mediaItemYear(item.ParentID); year > 0 {
			return year
		}
		return 0
	case "episode":
		showID := strings.TrimSpace(item.GrandparentID)
		if showID == "" && strings.TrimSpace(item.ParentID) != "" {
			_ = s.queryUserRow(context.Background(), `SELECT COALESCE(parent_id, '') FROM media_items WHERE id = ?`, item.ParentID).Scan(&showID)
		}
		if year := s.mediaItemYear(showID); year > 0 {
			return year
		}
		return 0
	default:
		return metadataSearchYear(item)
	}
}

func (s *Server) mediaItemYear(mediaID string) int {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return 0
	}
	var year int
	if err := s.queryUserRow(context.Background(), `SELECT COALESCE(year, 0) FROM media_items WHERE id = ?`, mediaID).Scan(&year); err != nil {
		return 0
	}
	return max(0, year)
}

func musicBrainzRecordingQuery(item MediaItem) string {
	parts := []string{}
	if title := musicBrainzQueryValue(firstNonEmpty(item.Title, titleFromSourcePath(item.SourceURL))); title != "" {
		parts = append(parts, `recording:"`+title+`"`)
	}
	if artist := musicBrainzQueryValue(firstNonEmpty(item.Studio, item.GrandparentTitle)); artist != "" {
		parts = append(parts, `artist:"`+artist+`"`)
	}
	if album := musicBrainzQueryValue(item.ParentTitle); album != "" {
		parts = append(parts, `release:"`+album+`"`)
	}
	return strings.Join(parts, " AND ")
}

func musicBrainzReleaseGroupQuery(item MediaItem) string {
	parts := []string{}
	if title := musicBrainzQueryValue(item.Title); title != "" {
		parts = append(parts, `releasegroup:"`+title+`"`)
	}
	if artist := musicBrainzQueryValue(firstNonEmpty(item.Studio, item.ParentTitle, item.GrandparentTitle)); artist != "" {
		parts = append(parts, `artist:"`+artist+`"`)
	}
	return strings.Join(parts, " AND ")
}

func musicBrainzQueryValue(value string) string {
	value = cleanProviderSearchTitle(value)
	value = strings.ReplaceAll(value, `"`, " ")
	value = strings.ReplaceAll(value, `\`, " ")
	return strings.Join(strings.Fields(value), " ")
}

func musicBrainzArtistCreditNames(credits []musicBrainzArtistCredit) (string, string) {
	var names []string
	artistID := ""
	for _, credit := range credits {
		name := firstNonEmpty(credit.Name, credit.Artist.Name)
		if name == "" {
			continue
		}
		if artistID == "" {
			artistID = credit.Artist.ID
		}
		names = append(names, name)
	}
	return strings.Join(names, ", "), artistID
}

func musicBrainzReleaseArtistCreditNames(release musicBrainzRelease) (string, string) {
	artistName, artistID := musicBrainzArtistCreditNames(release.ArtistCredit)
	if artistName != "" {
		return artistName, artistID
	}
	return musicBrainzArtistCreditNames(release.ReleaseGroup.ArtistCredit)
}

func musicBrainzPreferredRelease(releases []musicBrainzRelease, albumTitle string) musicBrainzRelease {
	if len(releases) == 0 {
		return musicBrainzRelease{}
	}
	albumKey := strings.ToLower(strings.TrimSpace(albumTitle))
	if albumKey != "" {
		for _, release := range releases {
			if strings.EqualFold(strings.TrimSpace(release.Title), albumKey) || strings.EqualFold(strings.TrimSpace(release.ReleaseGroup.Title), albumKey) {
				return release
			}
		}
	}
	return releases[0]
}

func musicBrainzReleaseIDForGroup(group musicBrainzReleaseGroup) string {
	for _, release := range group.Releases {
		if strings.TrimSpace(release.ID) != "" && (strings.TrimSpace(release.Title) == "" || strings.EqualFold(strings.TrimSpace(release.Title), strings.TrimSpace(group.Title))) {
			return strings.TrimSpace(release.ID)
		}
	}
	for _, release := range group.Releases {
		if strings.TrimSpace(release.ID) != "" {
			return strings.TrimSpace(release.ID)
		}
	}
	return ""
}

func bestMusicBrainzRecording(recordings []musicBrainzRecording, item MediaItem) musicBrainzRecording {
	var best musicBrainzRecording
	bestScore := -1.0
	for _, recording := range recordings {
		score := musicBrainzRecordingCandidateScore(recording, item)
		if score.Score > bestScore {
			best = recording
			bestScore = score.Score
		}
	}
	if bestScore < 35 {
		return musicBrainzRecording{}
	}
	return best
}

func musicBrainzRecordingMatchScore(recording musicBrainzRecording, item MediaItem) float64 {
	return musicBrainzRecordingCandidateScore(recording, item).Score
}

func musicBrainzRecordingCandidateScore(recording musicBrainzRecording, item MediaItem) candidateScore {
	var score candidateScore
	artistName, _ := musicBrainzArtistCreditNames(recording.ArtistCredit)
	if delta, reasons := providerScoreReason(float64(recording.Score), 0.35); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if delta, reasons := titleScoreReasons([]string{firstNonEmpty(item.Title, titleFromSourcePath(item.SourceURL))}, recording.Title, 45); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if artist := artistName; artist != "" {
		expectedArtist := providerMatchKey(firstNonEmpty(item.Studio, item.GrandparentTitle))
		if expectedArtist != "" {
			delta := providerTitleSimilarity(expectedArtist, providerMatchKey(artist)) * 20
			code := "artist_similar"
			if delta >= 19.6 {
				code = "artist_exact"
			}
			score.add(code, delta, artist)
		}
	}
	if item.Year > 0 {
		for _, release := range recording.Releases {
			if year := yearFromMusicBrainzDate(release.Date); year > 0 {
				if delta, reasons := yearScoreReason(item.Year, year, 10, 3, 20); delta != 0 {
					score.add(reasons[0].Code, delta, reasons[0].Detail)
				}
				break
			}
		}
	}
	release := musicBrainzPreferredRelease(recording.Releases, item.ParentTitle)
	addIdentityEvidenceScore(&score, item, map[string]string{
		"title":  recording.Title,
		"artist": firstNonEmpty(artistName, item.GrandparentTitle),
		"album":  firstNonEmpty(release.Title, release.ReleaseGroup.Title),
		"year":   strconv.Itoa(yearFromMusicBrainzDate(release.Date)),
	})
	return score
}

func bestMusicBrainzReleaseGroup(groups []musicBrainzReleaseGroup, item MediaItem) musicBrainzReleaseGroup {
	var best musicBrainzReleaseGroup
	bestScore := -1.0
	for _, group := range groups {
		score := musicBrainzReleaseGroupCandidateScore(group, item)
		if score.Score > bestScore {
			best = group
			bestScore = score.Score
		}
	}
	if bestScore < 35 {
		return musicBrainzReleaseGroup{}
	}
	return best
}

func musicBrainzReleaseGroupMatchScore(group musicBrainzReleaseGroup, item MediaItem) float64 {
	return musicBrainzReleaseGroupCandidateScore(group, item).Score
}

func musicBrainzReleaseGroupCandidateScore(group musicBrainzReleaseGroup, item MediaItem) candidateScore {
	var score candidateScore
	if delta, reasons := providerScoreReason(float64(group.Score), 0.35); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if delta, reasons := titleScoreReasons([]string{firstNonEmpty(item.Title, titleFromSourcePath(item.SourceURL))}, group.Title, 45); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if artist, _ := musicBrainzArtistCreditNames(group.ArtistCredit); artist != "" {
		expectedArtist := providerMatchKey(firstNonEmpty(item.Studio, item.ParentTitle, item.GrandparentTitle))
		if expectedArtist != "" {
			delta := providerTitleSimilarity(expectedArtist, providerMatchKey(artist)) * 20
			code := "artist_similar"
			if delta >= 19.6 {
				code = "artist_exact"
			}
			score.add(code, delta, artist)
		}
	}
	if item.Year > 0 {
		if year := yearFromMusicBrainzDate(group.FirstReleaseDate); year > 0 {
			if delta, reasons := yearScoreReason(item.Year, year, 10, 3, 20); delta != 0 {
				score.add(reasons[0].Code, delta, reasons[0].Detail)
			}
		}
	}
	artist, _ := musicBrainzArtistCreditNames(group.ArtistCredit)
	addIdentityEvidenceScore(&score, item, map[string]string{
		"title":  group.Title,
		"artist": artist,
		"year":   strconv.Itoa(yearFromMusicBrainzDate(group.FirstReleaseDate)),
	})
	return score
}

func bestMusicBrainzArtist(artists []musicBrainzArtist, item MediaItem) musicBrainzArtist {
	var best musicBrainzArtist
	bestScore := -1.0
	for _, artist := range artists {
		score := musicBrainzArtistCandidateScore(artist, item)
		if score.Score > bestScore {
			best = artist
			bestScore = score.Score
		}
	}
	return best
}

func musicBrainzArtistCandidateScore(artist musicBrainzArtist, item MediaItem) candidateScore {
	var score candidateScore
	if delta, reasons := titleScoreReasons([]string{item.Title, item.GrandparentTitle, item.ParentTitle}, artist.Name, 80); delta != 0 {
		code := strings.Replace(reasons[0].Code, "title", "artist", 1)
		score.add(code, delta, reasons[0].Detail)
	}
	addIdentityEvidenceScore(&score, item, map[string]string{"artist": artist.Name})
	return score
}

func musicBrainzGenres(genres []musicBrainzTag, tags []musicBrainzTag) []string {
	values := make([]string, 0, len(genres)+len(tags))
	for _, tag := range append(genres, tags...) {
		if tag.Name != "" && tag.Count >= 0 {
			values = append(values, strings.Title(strings.ToLower(tag.Name)))
		}
	}
	return normalizeStringList(values)
}

func yearFromMusicBrainzDate(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	if year < 1850 || year > time.Now().Year()+1 {
		return 0
	}
	return year
}

func yearFromReleaseName(value string) int {
	value = filepathBaseWithoutExtension(value)
	replacer := strings.NewReplacer(".", " ", "_", " ", "-", " ", "[", " ", "]", " ", "(", " ", ")", " ")
	for _, field := range strings.Fields(replacer.Replace(value)) {
		year, err := strconv.Atoi(strings.Trim(field, ".,;:!"))
		if err == nil && year >= 1900 && year <= time.Now().Year()+2 {
			return year
		}
	}
	return 0
}

func musicBrainzArtworkMap(releaseGroupID, releaseID string) map[string]string {
	payload := map[string]string{"source": "musicbrainz"}
	if strings.TrimSpace(releaseGroupID) != "" {
		payload["releaseGroupID"] = strings.TrimSpace(releaseGroupID)
	}
	if strings.TrimSpace(releaseID) != "" {
		payload["releaseID"] = strings.TrimSpace(releaseID)
	}
	if len(payload) == 1 {
		return nil
	}
	return payload
}

func tmdbQueryTitleCandidates(value string) []string {
	cleaned := cleanProviderSearchTitle(value)
	if cleaned == "" {
		return nil
	}
	candidates := []string{cleaned}
	if withoutCredit := stripTrailingInitialCreditWords(cleaned); withoutCredit != "" && !strings.EqualFold(withoutCredit, cleaned) {
		candidates = append(candidates, withoutCredit)
	}
	lower := strings.ToLower(cleaned)
	for _, suffix := range []string{" the movie", " movie"} {
		if strings.HasSuffix(lower, suffix) {
			trimmed := strings.TrimSpace(cleaned[:len(cleaned)-len(suffix)])
			if trimmed != "" {
				candidates = append(candidates, trimmed)
			}
		}
	}
	candidates = append(candidates, trailingWordFallbackTitles(cleaned)...)
	return uniqueNonEmptyStrings(candidates)
}

func stripTrailingInitialCreditWords(value string) string {
	fields := strings.Fields(value)
	if len(fields) < 4 {
		return value
	}
	initialStart := len(fields) - 2
	for initialStart >= 0 && isSingleLetterWord(fields[initialStart]) {
		initialStart--
	}
	initialStart++
	if initialStart >= len(fields)-1 || initialStart == 0 || !looksLikePersonSurname(fields[len(fields)-1]) {
		return value
	}
	return strings.TrimSpace(strings.Join(fields[:initialStart], " "))
}

func trailingWordFallbackTitles(value string) []string {
	fields := strings.Fields(value)
	if len(fields) <= 4 {
		return nil
	}
	minFields := max(4, len(fields)-4)
	fallbacks := make([]string, 0, len(fields)-minFields)
	for end := len(fields) - 1; end >= minFields; end-- {
		fallbacks = append(fallbacks, strings.Join(fields[:end], " "))
	}
	return fallbacks
}

func isSingleLetterWord(value string) bool {
	value = strings.Trim(value, ".,;:!()[]{}'\"")
	return len([]rune(value)) == 1
}

func looksLikePersonSurname(value string) bool {
	value = strings.Trim(value, ".,;:!()[]{}'\"")
	if len([]rune(value)) < 3 {
		return false
	}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '\'' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func cleanProviderSearchTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = stripInitialedCreditSuffix(value)
	value = strings.TrimSuffix(filepathBaseWithoutExtension(value), "")
	replacer := strings.NewReplacer(".", " ", "_", " ", "-", " ", "[", " ", "]", " ", "(", " ", ")", " ")
	value = replacer.Replace(value)
	fields := strings.Fields(value)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		normalized := strings.ToLower(strings.Trim(field, ".,;:!"))
		if providerTitleStopToken(normalized) {
			break
		}
		if _, err := strconv.Atoi(normalized); err == nil && len(normalized) == 4 {
			year, _ := strconv.Atoi(normalized)
			if year >= 1900 && year <= time.Now().Year()+2 {
				break
			}
		}
		out = append(out, field)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

func providerMatchKey(value string) string {
	value = cleanProviderSearchTitle(value)
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(":", " ", "'", "", "\"", "", "&", " and ")
	value = replacer.Replace(value)
	fields := strings.Fields(value)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "the" || field == "a" || field == "an" {
			continue
		}
		out = append(out, field)
	}
	return strings.Join(out, " ")
}

func providerTitleSimilarity(queryTitle string, resultTitle string) float64 {
	if queryTitle == "" || resultTitle == "" {
		return 0
	}
	if queryTitle == resultTitle {
		return 1
	}
	if strings.Contains(resultTitle, queryTitle) || strings.Contains(queryTitle, resultTitle) {
		shorter := min(len(queryTitle), len(resultTitle))
		longer := max(len(queryTitle), len(resultTitle))
		if longer > 0 {
			return maxFloat(0.72, float64(shorter)/float64(longer))
		}
	}
	queryTokens := strings.Fields(queryTitle)
	resultTokens := strings.Fields(resultTitle)
	if len(queryTokens) == 0 || len(resultTokens) == 0 {
		return 0
	}
	resultSet := map[string]bool{}
	for _, token := range resultTokens {
		resultSet[token] = true
	}
	matches := 0
	for _, token := range queryTokens {
		if resultSet[token] {
			matches++
		}
	}
	precision := float64(matches) / float64(len(resultTokens))
	recall := float64(matches) / float64(len(queryTokens))
	if precision+recall == 0 {
		return 0
	}
	return 2 * precision * recall / (precision + recall)
}

func providerTitleStopToken(value string) bool {
	switch value {
	case "480p", "576p", "720p", "1080p", "2160p", "4k", "8k",
		"bluray", "brrip", "web", "webdl", "web-dl", "webrip", "hdtv", "hdrip", "dvdrip",
		"x264", "x265", "h264", "h265", "hevc", "avc", "aac", "aac5", "ac3", "eac3", "dts",
		"hdr", "dv", "dolby", "atmos", "remux", "proper", "repack", "extended", "unrated",
		"hybrid", "yts", "rarbg", "eztv":
		return true
	default:
		return false
	}
}

func titleFromSourcePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Path != "" {
		value = parsed.Path
	}
	return filepathBaseWithoutExtension(value)
}

func filepathBaseWithoutExtension(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimSuffix(value, "/")
	slash := strings.LastIndexAny(value, `/\`)
	if slash >= 0 {
		value = value[slash+1:]
	}
	if dot := strings.LastIndex(value, "."); dot > 0 {
		value = value[:dot]
	}
	return value
}

func aniListUpdateForResult(result aniListMedia) UpdateMediaRequest {
	update := UpdateMediaRequest{}
	if title := aniListDisplayTitle(result.Title); title != "" {
		update.Title = &title
		sortTitle := sortableTitle(title)
		update.SortTitle = &sortTitle
	}
	if original := firstNonEmpty(result.Title.Romaji, result.Title.Native); original != "" {
		update.OriginalTitle = &original
	}
	if summary := plainAniListDescription(result.Description); summary != "" {
		update.Summary = &summary
	}
	if result.SeasonYear > 0 {
		update.Year = &result.SeasonYear
	}
	if result.AverageScore > 0 {
		rating := float64(result.AverageScore) / 10
		update.CommunityRating = &rating
	}
	if len(result.Genres) > 0 {
		genres := titleCaseList(result.Genres)
		update.Genres = &genres
	}
	if studio := firstAniListStudio(result); studio != "" {
		update.Studio = &studio
	}
	typed := map[string]string{}
	setTyped := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			typed[key] = value
		}
	}
	setTyped("provider", "AniList")
	setTyped("format", result.Format)
	setTyped("status", result.Status)
	setTyped("season", result.Season)
	if result.Episodes > 0 {
		typed["episodes"] = strconv.Itoa(result.Episodes)
	}
	if result.Duration > 0 {
		typed["episodeRuntimeMinutes"] = strconv.Itoa(result.Duration)
	}
	if result.ID > 0 {
		typed["anilistID"] = strconv.Itoa(result.ID)
	}
	if result.IDMal > 0 {
		typed["malID"] = strconv.Itoa(result.IDMal)
	}
	if len(typed) > 0 {
		update.TypedMetadata = &typed
	}
	if artwork := aniListArtworkMap(result); len(artwork) > 0 {
		update.Artwork = &artwork
	}
	if people := aniListPeople(result); len(people) > 0 {
		update.People = &people
	}
	return update
}

func aniListDisplayTitle(title aniListTitle) string {
	return firstNonEmpty(strings.TrimSpace(title.English), strings.TrimSpace(title.Romaji), strings.TrimSpace(title.Native))
}

func titleCaseList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, strings.Title(strings.ToLower(value)))
	}
	return uniqueNonEmptyStrings(out)
}

func aniListSearchTitles(media aniListMedia) []string {
	return uniqueNonEmptyStrings([]string{media.Title.English, media.Title.Romaji, media.Title.Native})
}

func plainAniListDescription(value string) string {
	value = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(value, "\n")
	value = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, "")
	return strings.TrimSpace(html.UnescapeString(value))
}

func aniListArtworkMap(result aniListMedia) map[string]string {
	payload := map[string]string{"source": "anilist"}
	if poster := firstNonEmpty(result.CoverImage.ExtraLarge, result.CoverImage.Large, result.CoverImage.Medium); poster != "" {
		payload["posterURL"] = poster
	}
	if backdrop := strings.TrimSpace(result.BannerImage); backdrop != "" {
		payload["backdropURL"] = backdrop
	}
	if len(payload) == 1 {
		return nil
	}
	return payload
}

func firstAniListStudio(result aniListMedia) string {
	for _, studio := range result.Studios.Nodes {
		if strings.TrimSpace(studio.Name) != "" {
			return strings.TrimSpace(studio.Name)
		}
	}
	return ""
}

func aniListPeople(result aniListMedia) []MediaPerson {
	people := make([]MediaPerson, 0, 32)
	seen := map[string]bool{}
	add := func(person MediaPerson) {
		person.Name = strings.TrimSpace(person.Name)
		person.Role = strings.TrimSpace(person.Role)
		person.Character = strings.TrimSpace(person.Character)
		person.ImageURL = strings.TrimSpace(person.ImageURL)
		if person.Name == "" || person.Role == "" {
			return
		}
		key := strings.ToLower(person.Role + "\x00" + person.Name + "\x00" + person.Character)
		if seen[key] || len(people) >= 32 {
			return
		}
		seen[key] = true
		people = append(people, person)
	}
	for _, edge := range result.Characters.Edges {
		characterName := firstNonEmpty(edge.Node.Name.Full, edge.Node.Name.Native)
		for _, actor := range edge.VoiceActors {
			name := firstNonEmpty(actor.Name.Full, actor.Name.Native)
			if name == "" {
				continue
			}
			add(MediaPerson{
				Name:      name,
				Role:      "Voice",
				Character: characterName,
				SortOrder: len(people),
				ImageURL:  firstNonEmpty(actor.Image.Large, actor.Image.Medium),
				ProviderIDs: map[string]string{
					"anilist": strconv.Itoa(actor.ID),
				},
			})
			break
		}
	}
	for _, edge := range result.Staff.Edges {
		role := aniListStaffRole(edge.Role)
		if role == "" {
			continue
		}
		name := firstNonEmpty(edge.Node.Name.Full, edge.Node.Name.Native)
		add(MediaPerson{
			Name:      name,
			Role:      role,
			SortOrder: len(people) + 1000,
			ImageURL:  firstNonEmpty(edge.Node.Image.Large, edge.Node.Image.Medium),
			ProviderIDs: map[string]string{
				"anilist": strconv.Itoa(edge.Node.ID),
			},
		})
	}
	return people
}

func aniListStaffRole(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "director"):
		return "Director"
	case strings.Contains(normalized, "original creator"):
		return "Creator"
	case strings.Contains(normalized, "series composition"), strings.Contains(normalized, "script"), strings.Contains(normalized, "screenplay"):
		return "Writer"
	case strings.Contains(normalized, "producer"):
		return "Producer"
	default:
		return ""
	}
}

func bestAniListMedia(results []aniListMedia, item MediaItem) aniListMedia {
	var best aniListMedia
	bestScore := -1.0
	for _, result := range results {
		if result.ID == 0 {
			continue
		}
		score := aniListCandidateScore(result, item)
		if score.Score > bestScore || (score.Score == bestScore && result.AverageScore > best.AverageScore) {
			best = result
			bestScore = score.Score
		}
	}
	if bestScore < 35 {
		return aniListMedia{}
	}
	return best
}

func aniListCandidateScore(result aniListMedia, item MediaItem) candidateScore {
	var score candidateScore
	if delta, reasons := titleScoreReasons(tmdbQueryTitlesForItem(item), aniListDisplayTitle(result.Title), 70); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if delta, reasons := titleScoreReasons(tmdbQueryTitlesForItem(item), firstNonEmpty(aniListSearchTitles(result)...), 55); delta != 0 {
		score.add("alternate_"+reasons[0].Code, delta, reasons[0].Detail)
	}
	year := metadataSearchYear(item)
	if year == 0 {
		year = item.Year
	}
	if delta, reasons := yearScoreReason(year, result.SeasonYear, 24, 8, 30); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	addIdentityEvidenceScore(&score, item, map[string]string{
		"title": aniListDisplayTitle(result.Title),
		"year":  strconv.Itoa(result.SeasonYear),
	})
	return score
}

func tmdbArtworkMap(posterPath, backdropPath, thumbPath string) map[string]string {
	payload := map[string]string{"source": "tmdb"}
	if strings.TrimSpace(posterPath) != "" {
		payload["posterPath"] = posterPath
	}
	if strings.TrimSpace(backdropPath) != "" {
		payload["backdropPath"] = backdropPath
	}
	if strings.TrimSpace(thumbPath) != "" {
		payload["thumbPath"] = thumbPath
	}
	if len(payload) == 1 {
		return nil
	}
	return payload
}

func normalizedMetadataProvider(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "":
		return ""
	case "default", "server default", "server defaults":
		return "default"
	case "tmdb", "the movie database":
		return "tmdb"
	case "anilist", "ani list":
		return "anilist"
	case "musicbrainz", "music brainz":
		return "musicbrainz"
	case "local", "local media assets":
		return "local"
	default:
		return normalized
	}
}

func (s *Server) tmdbConfigured() bool {
	return s.tmdbReadAccessToken() != "" || s.tmdbAPIKey() != ""
}

func (s *Server) tmdbReadAccessToken() string {
	if value := strings.TrimSpace(s.secretSetting(tmdbReadAccessTokenSettingKey)); value != "" {
		return value
	}
	if strings.TrimSpace(s.secretSetting(tmdbAPIKeySettingKey)) != "" {
		// Persisted credentials are one owner-selected pair. A custom API key
		// must not be shadowed by a configured bearer token.
		return ""
	}
	return strings.TrimSpace(s.cfg.TMDBReadAccessToken)
}

func (s *Server) tmdbAPIKey() string {
	if value := strings.TrimSpace(s.secretSetting(tmdbAPIKeySettingKey)); value != "" {
		return value
	}
	return strings.TrimSpace(s.cfg.TMDBAPIKey)
}

func normalizeMetadataLanguage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "en-US"
	}
	value = strings.ReplaceAll(value, "_", "-")
	parts := strings.Split(value, "-")
	if len(parts) == 1 {
		lower := strings.ToLower(parts[0])
		switch lower {
		case "en":
			return "en-US"
		case "fr":
			return "fr-FR"
		case "es":
			return "es-ES"
		case "de":
			return "de-DE"
		case "ja":
			return "ja-JP"
		default:
			return lower
		}
	}
	return strings.ToLower(parts[0]) + "-" + strings.ToUpper(parts[1])
}

func (s *Server) searchTMDB(ctx context.Context, mediaType, title string, year int, language string) (tmdbSearchResult, error) {
	results, err := s.searchTMDBResults(ctx, mediaType, title, year, language)
	if err != nil || len(results) == 0 {
		return tmdbSearchResult{}, err
	}
	item := MediaItem{Title: title, Year: year}
	return bestTMDBResult(results, item, []string{title}, year), nil
}

func (s *Server) searchTMDBResults(ctx context.Context, mediaType, title string, year int, language string) ([]tmdbSearchResult, error) {
	if !s.tmdbConfigured() {
		return nil, errTMDBCredentialsMissing
	}
	baseURL := strings.TrimRight(s.cfg.TMDBBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.themoviedb.org/3"
	}
	endpoint, err := url.Parse(baseURL + "/search/" + url.PathEscape(mediaType))
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("query", title)
	query.Set("language", normalizeMetadataLanguage(language))
	if apiKey := s.tmdbAPIKey(); apiKey != "" && s.tmdbReadAccessToken() == "" {
		query.Set("api_key", apiKey)
	}
	if year > 0 && mediaType == "movie" {
		query.Set("year", strconv.Itoa(year))
	}
	if year > 0 && mediaType == "tv" {
		query.Set("first_air_date_year", strconv.Itoa(year))
	}
	endpoint.RawQuery = query.Encode()

	var payload tmdbSearchResponse
	for attempt := 0; attempt < 2; attempt++ {
		if err := waitForTMDBSlot(ctx); err != nil {
			return nil, err
		}
		requestCtx, cancel := context.WithTimeout(ctx, tmdbRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if token := s.tmdbReadAccessToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, responseErr := doProviderJSONRequest(requestCtx, "TMDB", req, maxMetadataProviderResponseBytes, &payload)
		retry := retryAfterDuration(response.Header.Get("Retry-After"))
		cancel()
		if responseErr != nil {
			if response.StatusCode == http.StatusTooManyRequests && attempt == 0 && retry > 0 && retry <= 30*time.Second {
				timer := time.NewTimer(retry)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
					continue
				}
			}
			return nil, responseErr
		}
		break
	}
	return payload.Results, nil
}

func (s *Server) searchTMDBCandidates(ctx context.Context, item MediaItem, mediaType string, titles []string, year int, language string) (tmdbSearchResult, error) {
	if len(titles) == 0 {
		return tmdbSearchResult{}, nil
	}
	var candidates []tmdbSearchResult
	for _, title := range uniqueNonEmptyStrings(titles) {
		results, err := s.searchTMDBResults(ctx, mediaType, title, year, language)
		if err != nil {
			return tmdbSearchResult{}, err
		}
		candidates = append(candidates, results...)
	}
	best := bestTMDBResult(candidates, item, titles, year)
	for _, candidate := range candidates {
		if candidate.ID == 0 {
			continue
		}
		score := tmdbResultCandidateScore(candidate, item, titles, year)
		accepted := best.ID != 0 && candidate.ID == best.ID
		_ = s.recordMatchCandidate(item.ID, "tmdb", strconv.Itoa(candidate.ID), mediaType, "provider-search", score, accepted, strings.Join(titles, " | "), candidate)
	}
	return best, nil
}

func bestTMDBResult(results []tmdbSearchResult, item MediaItem, queryTitles []string, year int) tmdbSearchResult {
	var best tmdbSearchResult
	bestScore := -1.0
	for _, result := range results {
		if result.ID == 0 {
			continue
		}
		score := tmdbResultCandidateScore(result, item, queryTitles, year)
		if score.Score > bestScore || (score.Score == bestScore && result.Popularity > best.Popularity) {
			best = result
			bestScore = score.Score
		}
	}
	if bestScore < 35 {
		return tmdbSearchResult{}
	}
	return best
}

func tmdbResultMatchScore(result tmdbSearchResult, queryTitles []string, year int) float64 {
	return tmdbResultCandidateScore(result, MediaItem{Title: firstNonEmpty(queryTitles...), Year: year}, queryTitles, year).Score
}

func tmdbResultCandidateScore(result tmdbSearchResult, item MediaItem, queryTitles []string, year int) candidateScore {
	var score candidateScore
	if delta, reasons := titleScoreReasons(queryTitles, result.displayTitle(), 70); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if delta, reasons := yearScoreReason(year, result.displayYear(), 24, 8, 30); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if delta, reasons := popularityScoreReason(result.Popularity); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	addIdentityEvidenceScore(&score, item, map[string]string{
		"title": result.displayTitle(),
		"year":  strconv.Itoa(result.displayYear()),
	})
	return score
}

func (s *Server) getTMDB(ctx context.Context, path string, queryParams map[string]string, out any) error {
	if !s.tmdbConfigured() {
		return errTMDBCredentialsMissing
	}
	baseURL := strings.TrimRight(s.cfg.TMDBBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.themoviedb.org/3"
	}
	endpoint, err := url.Parse(baseURL + path)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	for key, value := range queryParams {
		if strings.TrimSpace(value) != "" {
			query.Set(key, value)
		}
	}
	if apiKey := s.tmdbAPIKey(); apiKey != "" && s.tmdbReadAccessToken() == "" {
		query.Set("api_key", apiKey)
	}
	endpoint.RawQuery = query.Encode()
	for attempt := 0; attempt < 2; attempt++ {
		if err := waitForTMDBSlot(ctx); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, tmdbRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Accept", "application/json")
		if token := s.tmdbReadAccessToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, responseErr := doProviderJSONRequest(requestCtx, "TMDB", req, maxMetadataProviderResponseBytes, out)
		retry := retryAfterDuration(response.Header.Get("Retry-After"))
		cancel()
		if responseErr != nil {
			if response.StatusCode == http.StatusTooManyRequests && attempt == 0 && retry > 0 && retry <= 30*time.Second {
				timer := time.NewTimer(retry)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
					continue
				}
			}
			return responseErr
		}
		return nil
	}
	return fmt.Errorf("TMDB request returned %s", http.StatusText(http.StatusTooManyRequests))
}

func waitForTMDBSlot(ctx context.Context) error {
	tmdbThrottleMu.Lock()
	wait := 50*time.Millisecond - time.Since(tmdbLastRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			tmdbThrottleMu.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}
	tmdbLastRequest = time.Now()
	tmdbThrottleMu.Unlock()
	return nil
}

func tmdbSearchType(mediaType string) string {
	switch mediaType {
	case "movie":
		return "movie"
	case "show", "anime", "episode", "season":
		return "tv"
	default:
		return ""
	}
}

func (result tmdbSearchResult) displayTitle() string {
	if result.Title != "" {
		return result.Title
	}
	return result.Name
}

func (result tmdbSearchResult) displayYear() int {
	date := result.ReleaseDate
	if date == "" {
		date = result.FirstAirDate
	}
	if len(date) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(date[:4])
	return year
}

func yearFromTMDBDate(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	return year
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func tmdbGenreNames(mediaType string, ids []int) []string {
	movieGenres := map[int]string{
		12: "Adventure", 14: "Fantasy", 16: "Animation", 18: "Drama", 27: "Horror", 28: "Action", 35: "Comedy", 36: "History", 37: "Western", 53: "Thriller", 80: "Crime", 878: "Science Fiction", 9648: "Mystery", 10402: "Music", 10749: "Romance", 10751: "Family", 10752: "War", 10770: "TV Movie", 99: "Documentary",
	}
	tvGenres := map[int]string{
		16: "Animation", 18: "Drama", 35: "Comedy", 37: "Western", 80: "Crime", 99: "Documentary", 9648: "Mystery", 10751: "Family", 10759: "Action & Adventure", 10762: "Kids", 10763: "News", 10764: "Reality", 10765: "Sci-Fi & Fantasy", 10766: "Soap", 10767: "Talk", 10768: "War & Politics",
	}
	source := movieGenres
	if mediaType == "tv" {
		source = tvGenres
	}
	var genres []string
	for _, id := range ids {
		if name := source[id]; name != "" {
			genres = append(genres, name)
		}
	}
	return normalizeStringList(genres)
}
