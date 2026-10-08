package metadataprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type TMDB struct {
	http  *transport
	token string
	cache screenCache
}

func NewTMDB(token string) (*TMDB, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("Portico TMDB application credential is missing")
	}
	return NewTMDBAt(token, "https://api.themoviedb.org/3")
}

// NewTMDBAt builds the TMDB adapter against an explicit origin. Production
// always uses the documented API origin; the server's own integration tests
// run bounded fixture origins through this same paced transport, so the
// credential header, pacing and error classification they exercise are the
// production ones, not a parallel client.
func NewTMDBAt(token, origin string) (*TMDB, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("Portico TMDB application credential is missing")
	}
	if origin == "" {
		origin = "https://api.themoviedb.org/3"
	}
	return &TMDB{http: newTransport("tmdb", origin, "Portico/0.1 (https://getportico.tv)"), token: token}, nil
}

// Raw fetches one TMDB route as bytes through the shared paced transport: one
// pacing clock, one in-flight gate and one Retry-After deferral for every
// caller. It is deliberately not cached; callers persist what they accept and
// the match logic must see the provider's current answer. base overrides the
// API origin and client the HTTP client when set, so an application that owns a
// configured client still has one paced path to TMDB rather than two.
func (s *TMDB) Raw(ctx context.Context, client *http.Client, base, route string, budget int64) ([]byte, error) {
	return s.http.raw(ctx, client, base, route, s.token, budget)
}

func (s *TMDB) get(ctx context.Context, route string, out any) error {
	return s.cache.get(ctx, route, out, func() (any, error) { var v any; e := s.http.request(ctx, "GET", route, s.token, nil, &v); return v, e })
}

type tmdbName struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type tmdbCredit struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Character  string `json:"character"`
	Job        string `json:"job"`
	Department string `json:"department"`
}
type tmdbScreen struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Name          string     `json:"name"`
	OriginalTitle string     `json:"original_title"`
	OriginalName  string     `json:"original_name"`
	ReleaseDate   string     `json:"release_date"`
	FirstAirDate  string     `json:"first_air_date"`
	AirDate       string     `json:"air_date"`
	Overview      string     `json:"overview"`
	Tagline       string     `json:"tagline"`
	Status        string     `json:"status"`
	Language      string     `json:"original_language"`
	Poster        string     `json:"poster_path"`
	Backdrop      string     `json:"backdrop_path"`
	Still         string     `json:"still_path"`
	IMDb          string     `json:"imdb_id"`
	Adult         *bool      `json:"adult"`
	Runtime       *int       `json:"runtime"`
	EpisodeCount  *int       `json:"number_of_episodes"`
	Season        int        `json:"season_number"`
	Episode       int        `json:"episode_number"`
	Average       float64    `json:"vote_average"`
	Votes         int        `json:"vote_count"`
	Genres        []tmdbName `json:"genres"`
	Companies     []tmdbName `json:"production_companies"`
	Networks      []tmdbName `json:"networks"`
	Collection    *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"belongs_to_collection"`
	External struct {
		TVDB *int64 `json:"tvdb_id"`
		IMDb string `json:"imdb_id"`
	} `json:"external_ids"`
	Alternative struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	} `json:"alternative_titles"`
	Credits struct {
		Cast []tmdbCredit `json:"cast"`
		Crew []tmdbCredit `json:"crew"`
	} `json:"credits"`
	Keywords struct {
		Keywords []tmdbName `json:"keywords"`
		Results  []tmdbName `json:"results"`
	} `json:"keywords"`
	Recommendations struct {
		Results []struct {
			ID        int64  `json:"id"`
			MediaType string `json:"media_type"`
		} `json:"results"`
	} `json:"recommendations"`
	Guests []tmdbCredit `json:"guest_stars"`
	Crew   []tmdbCredit `json:"crew"`
	Groups struct {
		Results []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type int    `json:"type"`
		} `json:"results"`
	} `json:"episode_groups"`
	ContentRatings struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
	Releases struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
}

func tmdbKind(kind string) (string, error) {
	switch kind {
	case "movie":
		return "movie", nil
	case "show":
		return "tv", nil
	}
	return "", errors.New("unsupported TMDB entity type")
}
func (s *TMDB) SearchScreen(ctx context.Context, kind, title string, year int, language, region string) ([]ScreenRecord, error) {
	route, e := tmdbKind(kind)
	if e != nil {
		return nil, e
	}
	if strings.TrimSpace(title) == "" || len(title) > 512 || year < 0 || year > 9999 {
		return nil, errors.New("invalid TMDB search")
	}
	q := url.Values{"query": {title}, "include_adult": {"true"}, "page": {"1"}}
	if year > 0 {
		if kind == "movie" {
			q.Set("year", strconv.Itoa(year))
		} else {
			q.Set("first_air_date_year", strconv.Itoa(year))
		}
	}
	if language != "" {
		q.Set("language", language)
	}
	if region != "" && kind == "movie" {
		q.Set("region", region)
	}
	var page struct {
		Results []tmdbScreen `json:"results"`
	}
	if e = s.get(ctx, "/search/"+route+"?"+q.Encode(), &page); e != nil {
		return nil, e
	}
	if len(page.Results) > 25 {
		return nil, &Error{Provider: "tmdb", Code: "invalid_search"}
	}
	out := []ScreenRecord{}
	for _, v := range page.Results {
		r := v.record(kind)
		if e = ValidateScreenRecord(r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}

// tmdbDetailParts is the one detail document a movie or series is read as,
// shared by the plain and the conditional request so a refresh can never
// carry less than the first read did. recommendations is TMDB's first page
// of similar titles, in the same request.
func tmdbDetailParts(kind string) string {
	if kind == "show" {
		return "external_ids,alternative_titles,credits,content_ratings,episode_groups,keywords,recommendations"
	}
	return "external_ids,alternative_titles,credits,release_dates,keywords,recommendations"
}
func (s *TMDB) ScreenDetails(ctx context.Context, kind, id, language, region string) (ScreenRecord, error) {
	route, e := tmdbKind(kind)
	if e != nil {
		return ScreenRecord{}, e
	}
	if !ValidScreenID(ScreenID{"tmdb", kind, id}) {
		return ScreenRecord{}, errors.New("invalid TMDB identity")
	}
	q := url.Values{"append_to_response": {tmdbDetailParts(kind)}}
	if language != "" {
		q.Set("language", language)
	}
	var v tmdbScreen
	if e = s.get(ctx, "/"+route+"/"+id+"?"+q.Encode(), &v); e != nil {
		return ScreenRecord{}, e
	}
	r := v.record(kind)
	if r.Identity.ID != id {
		return r, &Error{Provider: "tmdb", Code: "identity_mismatch"}
	}
	return r, ValidateScreenRecord(r)
}
func (v tmdbScreen) record(kind string) ScreenRecord {
	title, original, date := v.Title, v.OriginalTitle, v.ReleaseDate
	if kind == "show" {
		title, original, date = v.Name, v.OriginalName, v.FirstAirDate
	}
	if kind == "episode" {
		title, original, date = v.Name, "", v.AirDate
	}
	r := ScreenRecord{Identity: ScreenID{"tmdb", kind, numberID(v.ID)}, Title: title, OriginalTitle: original, Date: date, Year: screenYear(date), Overview: v.Overview, Tagline: v.Tagline, Language: v.Language, Status: v.Status, Adult: v.Adult, AdvisoryRuntimeMinutes: v.Runtime, EpisodeCount: v.EpisodeCount, PosterPath: v.Poster, BackdropPath: v.Backdrop}
	if kind == "episode" {
		r.StillPath = v.Still
	}
	for _, a := range v.Alternative.Titles {
		r.Aliases = append(r.Aliases, a.Title)
	}
	for _, a := range v.Alternative.Results {
		r.Aliases = append(r.Aliases, a.Title)
	}
	if v.External.TVDB != nil && *v.External.TVDB > 0 {
		r.Crosswalk = append(r.Crosswalk, ScreenID{"tvdb", kind, numberID(*v.External.TVDB)})
	}
	// A movie carries its IMDb id at the top level as well; external_ids is
	// the one both movies and series have. A malformed id is dropped alone.
	imdb := v.External.IMDb
	if imdb == "" {
		imdb = v.IMDb
	}
	if id := (ScreenID{"imdb", "title", imdb}); ValidScreenID(id) {
		r.Crosswalk = append(r.Crosswalk, id)
	}
	if kind == "movie" || kind == "show" {
		for _, rec := range v.Recommendations.Results {
			typ := map[string]string{"movie": "movie", "tv": "show"}[rec.MediaType]
			if typ == "" {
				continue
			}
			r.Similar = appendSimilar(r.Similar, r.Identity, ScreenID{"tmdb", typ, numberID(rec.ID)})
		}
	}
	for _, g := range v.Genres {
		r.Genres = append(r.Genres, ScreenName{numberID(g.ID), g.Name})
	}
	// Keywords are deduplicated and capped before validation: TMDB keyword
	// payloads are not under this server's count cap, and one oversized
	// keyword payload must drop the duplicate keyword, never the record.
	addKeyword := func(id, name string) {
		name = strings.TrimSpace(name)
		if id == "" || name == "" || len(name) > 200 || len(id) > 128 {
			return
		}
		for _, t := range r.Tags {
			if t.ID == id {
				return
			}
		}
		if len(r.Tags) >= 64 {
			return
		}
		r.Tags = append(r.Tags, ScreenName{ID: id, Name: name})
	}
	for _, k := range v.Keywords.Keywords {
		addKeyword(numberID(k.ID), k.Name)
	}
	for _, k := range v.Keywords.Results {
		addKeyword(numberID(k.ID), k.Name)
	}
	credits := []ScreenCredit{}
	addCredit := func(c tmdbCredit, department, role string) {
		credits = append(credits, ScreenCredit{numberID(c.ID), c.Name, role, department, 0})
	}
	for _, c := range v.Credits.Cast {
		addCredit(c, "Acting", c.Character)
	}
	for _, c := range v.Credits.Crew {
		addCredit(c, c.Department, c.Job)
	}
	for _, c := range v.Guests {
		addCredit(c, "Guest", c.Character)
	}
	for _, c := range v.Crew {
		addCredit(c, c.Department, c.Job)
	}
	r.Credits = budgetScreenCredits(credits)
	for _, c := range v.Companies {
		r.Relations = append(r.Relations, ScreenRelation{"production_company", ScreenID{"tmdb", "company", numberID(c.ID)}, c.Name})
	}
	for _, c := range v.Networks {
		r.Relations = append(r.Relations, ScreenRelation{"network", ScreenID{"tmdb", "company", numberID(c.ID)}, c.Name})
	}
	if v.Collection != nil && v.Collection.ID > 0 {
		r.Relations = append(r.Relations, ScreenRelation{"collection", ScreenID{"tmdb", "collection", numberID(v.Collection.ID)}, v.Collection.Name})
	}
	if kind == "show" {
		r.Orders = []ScreenOrder{{"official", "Original broadcast order"}}
		for _, g := range v.Groups.Results {
			if validTMDBGroup(g.ID) {
				r.Orders = append(r.Orders, ScreenOrder{"tmdb-group:" + g.ID, g.Name})
			}
		}
	}
	if v.Votes > 0 {
		r.Rating = &ScreenRating{v.Average, 10, v.Votes}
	}
	for _, c := range v.ContentRatings.Results {
		if c.Rating != "" {
			r.Certifications = append(r.Certifications, ScreenCertification{c.Country, c.Rating})
		}
	}
	seen := map[string]bool{}
	for _, c := range v.Releases.Results {
		for _, d := range c.Dates {
			key := c.Country + ":" + d.Certification
			if d.Certification != "" && !seen[key] {
				r.Certifications = append(r.Certifications, ScreenCertification{c.Country, d.Certification})
				seen[key] = true
			}
		}
	}
	return r
}
func validTMDBGroup(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func (s *TMDB) ScreenEpisode(ctx context.Context, show string, season, episode int, order, language string) (ScreenRecord, error) {
	if !ValidScreenID(ScreenID{"tmdb", "show", show}) || season < 0 || episode < 0 || season > 9999 || episode > 1000000 {
		return ScreenRecord{}, errors.New("invalid TMDB episode coordinates")
	}
	originalSeason, originalEpisode := season, episode
	if strings.HasPrefix(order, "tmdb-group:") {
		group := strings.TrimPrefix(order, "tmdb-group:")
		if !validTMDBGroup(group) {
			return ScreenRecord{}, errors.New("invalid TMDB episode group")
		}
		// Verify group belongs to the accepted show before using its coordinates.
		series, err := s.ScreenDetails(ctx, "show", show, language, "")
		if err != nil {
			return ScreenRecord{}, err
		}
		found := false
		for _, o := range series.Orders {
			found = found || o.ID == order
		}
		if !found {
			return ScreenRecord{}, &Error{Provider: "tmdb", Code: "order_not_in_series"}
		}
		var data struct {
			ID     string `json:"id"`
			Groups []struct {
				Order    int `json:"order"`
				Episodes []struct {
					tmdbScreen
					Order int `json:"order"`
				} `json:"episodes"`
			} `json:"groups"`
		}
		if err = s.get(ctx, "/tv/episode_group/"+group, &data); err != nil {
			return ScreenRecord{}, err
		}
		matches := 0
		for _, g := range data.Groups {
			for _, ep := range g.Episodes {
				if g.Order == season && ep.Order+1 == episode {
					originalSeason, originalEpisode = ep.Season, ep.Episode
					matches++
				}
			}
		}
		if data.ID != group || matches != 1 {
			return ScreenRecord{}, &Error{Provider: "tmdb", Code: "episode_order_unresolved"}
		}
	} else if order != "official" {
		return ScreenRecord{}, &Error{Provider: "tmdb", Code: "order_unsupported"}
	}
	q := url.Values{"append_to_response": {"external_ids,credits"}}
	if language != "" {
		q.Set("language", language)
	}
	var v tmdbScreen
	if err := s.get(ctx, fmt.Sprintf("/tv/%s/season/%d/episode/%d?%s", show, originalSeason, originalEpisode, q.Encode()), &v); err != nil {
		return ScreenRecord{}, err
	}
	r := v.record("episode")
	if v.Season != originalSeason || v.Episode != originalEpisode {
		return r, &Error{Provider: "tmdb", Code: "episode_identity_mismatch"}
	}
	r.Coordinates = &ScreenCoordinates{ShowID: show, Season: &season, Episode: episode, Order: order, OriginalSeason: &originalSeason, OriginalEpisode: &originalEpisode}
	r.Relations = append(r.Relations, ScreenRelation{"show", ScreenID{"tmdb", "show", show}, ""})
	return r, ValidateScreenRecord(r)
}

// ScreenShowAggregateCredits reads a series' aggregated cast and crew: the
// fallback for a TVDB-matched show whose own characters are empty. Cast comes
// first in billing order, then crew; person ids are TMDB's, shared with
// movies, so the fallback cast links to the same people.
func (s *TMDB) ScreenShowAggregateCredits(ctx context.Context, id, language string) ([]ScreenCredit, error) {
	if !ValidScreenID(ScreenID{"tmdb", "show", id}) {
		return nil, errors.New("invalid TMDB show identity")
	}
	q := url.Values{}
	if language != "" {
		q.Set("language", language)
	}
	var v struct {
		ID   int64 `json:"id"`
		Cast []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Order int    `json:"order"`
			Roles []struct {
				Character string `json:"character"`
			} `json:"roles"`
		} `json:"cast"`
		Crew []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Jobs []struct {
				Job        string `json:"job"`
				Department string `json:"department"`
			} `json:"jobs"`
		} `json:"crew"`
	}
	route := "/tv/" + id + "/aggregate_credits"
	if len(q) > 0 {
		route += "?" + q.Encode()
	}
	if err := s.get(ctx, route, &v); err != nil {
		return nil, err
	}
	if numberID(v.ID) != id || len(v.Cast) > 1000 || len(v.Crew) > 1000 {
		return nil, &Error{Provider: "tmdb", Code: "invalid_aggregate_credits"}
	}
	out := []ScreenCredit{}
	add := func(personID int64, name, role, department string) {
		name, role, department = strings.TrimSpace(name), strings.TrimSpace(role), strings.TrimSpace(department)
		if personID <= 0 || personID > 9007199254740991 || name == "" || len(name) > 300 || len(role) > 500 || len(department) > 200 || department == "" {
			return
		}
		out = append(out, ScreenCredit{numberID(personID), name, role, department, 0})
	}
	for _, c := range v.Cast {
		role := ""
		if len(c.Roles) > 0 {
			role = c.Roles[0].Character
		}
		add(c.ID, c.Name, role, "Acting")
	}
	for _, c := range v.Crew {
		for _, j := range c.Jobs {
			department := strings.TrimSpace(j.Department)
			if department == "" {
				department = "Crew"
			}
			add(c.ID, c.Name, j.Job, department)
		}
	}
	return budgetScreenCredits(out), nil
}

// screenCreditBudget is how many credits one record carries; keyCrewReserve of them are held
// for the people a title page names.
const (
	screenCreditBudget = 128
	keyCrewReserve     = 24
)

// keyCrewRank orders the crew a title page names (director, writers, creators, composer,
// producers); -1 is everyone else.
func keyCrewRank(department, job string) int {
	d, j := strings.ToLower(strings.TrimSpace(department)), strings.ToLower(strings.TrimSpace(job))
	switch {
	case j == "director" || j == "series director":
		return 0
	case d == "writing" || strings.Contains(j, "screenplay") || strings.Contains(j, "writer"):
		return 1
	case d == "creator" || strings.Contains(j, "creator") || strings.Contains(j, "created by"):
		return 2
	case strings.Contains(j, "composer"):
		return 3
	case j == "producer" || j == "executive producer":
		return 4
	}
	return -1
}

// budgetScreenCredits keeps a record's credits within its budget without losing the people a
// title page names. TMDB lists crew in no useful order (a large production's stunt team comes
// before its director), so the best-ranked key crew are held first and the rest of the budget
// goes to the list in its own order, cast first. The result keeps the source order.
func budgetScreenCredits(all []ScreenCredit) []ScreenCredit {
	if len(all) > screenCreditBudget {
		type ranked struct{ index, rank int }
		key := []ranked{}
		for i, c := range all {
			if rank := keyCrewRank(c.Department, c.Role); rank >= 0 {
				key = append(key, ranked{i, rank})
			}
		}
		sort.SliceStable(key, func(a, b int) bool { return key[a].rank < key[b].rank })
		if len(key) > keyCrewReserve {
			key = key[:keyCrewReserve]
		}
		held := make(map[int]bool, len(key))
		for _, k := range key {
			held[k.index] = true
		}
		room := screenCreditBudget - len(held)
		kept := all[:0:0]
		for i, c := range all {
			if held[i] {
				kept = append(kept, c)
			} else if room > 0 {
				room--
				kept = append(kept, c)
			}
		}
		all = kept
	}
	for i := range all {
		all[i].Ordinal = i
	}
	return all
}

// ScreenDetailsConditional is ScreenDetails with the document's stored validators. It returns
// ErrNotModified when TMDB says the stored document is still current, which costs no body and
// no parsing; the validators to store next time come back either way.
//
// It bypasses the short-lived response cache on purpose: a refresh is asking the provider a
// question, and the cache exists to collapse bursts of identical reads inside one screen.
func (s *TMDB) ScreenDetailsConditional(ctx context.Context, kind, id, language, region string, in Conditional) (ScreenRecord, Conditional, error) {
	route, e := tmdbKind(kind)
	if e != nil {
		return ScreenRecord{}, in, e
	}
	if !ValidScreenID(ScreenID{"tmdb", kind, id}) {
		return ScreenRecord{}, in, errors.New("invalid TMDB identity")
	}
	q := url.Values{"append_to_response": {tmdbDetailParts(kind)}}
	if language != "" {
		q.Set("language", language)
	}
	raw, out, e := s.http.rawConditional(ctx, nil, "", "/"+route+"/"+id+"?"+q.Encode(), s.token, 4<<20, in)
	if e != nil {
		return ScreenRecord{}, out, e
	}
	var v tmdbScreen
	if e = json.Unmarshal(raw, &v); e != nil {
		return ScreenRecord{}, out, &Error{Provider: "tmdb", Code: "invalid_response"}
	}
	r := v.record(kind)
	if r.Identity.ID != id {
		return r, out, &Error{Provider: "tmdb", Code: "identity_mismatch"}
	}
	return r, out, ValidateScreenRecord(r)
}
