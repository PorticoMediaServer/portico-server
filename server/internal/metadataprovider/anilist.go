package metadataprovider

import (
	"context"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
)

type AniList struct {
	http  *transport
	cache screenCache
}

func NewAniList() *AniList {
	return NewAniListAt("https://graphql.anilist.co")
}

// NewAniListAt builds the AniList adapter against an explicit origin, for the
// server's bounded fixture-origin integration tests. No credential is
// involved: AniList is a public GraphQL endpoint.
func NewAniListAt(origin string) *AniList {
	if origin == "" {
		origin = "https://graphql.anilist.co"
	}
	return &AniList{http: newTransport("anilist", origin, "Portico/0.1 (https://getportico.tv)")}
}

const animeFields = `id type title { romaji english native } synonyms description(asHtml:false) format status source season seasonYear episodes isAdult startDate { year month day } genres averageScore stats { scoreDistribution { amount } } studios(isMain:true) { nodes { id name } } relations { edges { relationType node { id type title { romaji english native } } } } staff(perPage:50) { edges { role node { id name { full } } } } tags { id name rank isGeneralSpoiler isMediaSpoiler isAdult }`

// animeDetailFields is one work's detail read: the search fields plus the
// community's recommendations, best rated first (ties by id, so the order is
// stable). Search stays without them: 25 works each carrying 25 more nodes is
// query complexity no candidate list needs.
const animeDetailFields = animeFields + ` recommendations(sort:[RATING_DESC,ID],perPage:25) { nodes { rating mediaRecommendation { id type } } }`

type aniTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

func (t aniTitle) name(language string) string {
	if strings.HasPrefix(language, "ja") && t.Native != "" {
		return t.Native
	}
	if strings.HasPrefix(language, "en") && t.English != "" {
		return t.English
	}
	if t.Romaji != "" {
		return t.Romaji
	}
	if t.English != "" {
		return t.English
	}
	return t.Native
}

type aniMedia struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"`
	Title       aniTitle `json:"title"`
	Synonyms    []string `json:"synonyms"`
	Description string   `json:"description"`
	Format      string   `json:"format"`
	Status      string   `json:"status"`
	Source      string   `json:"source"`
	Season      string   `json:"season"`
	SeasonYear  int      `json:"seasonYear"`
	Episodes    *int     `json:"episodes"`
	Adult       *bool    `json:"isAdult"`
	Average     *float64 `json:"averageScore"`
	Start       struct {
		Year  *int `json:"year"`
		Month *int `json:"month"`
		Day   *int `json:"day"`
	} `json:"startDate"`
	Genres []string `json:"genres"`
	Stats  struct {
		Scores []struct {
			Amount int `json:"amount"`
		} `json:"scoreDistribution"`
	} `json:"stats"`
	Studios struct {
		Nodes []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"studios"`
	Relations struct {
		Edges []struct {
			Relation string `json:"relationType"`
			Node     struct {
				ID    int64    `json:"id"`
				Type  string   `json:"type"`
				Title aniTitle `json:"title"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"relations"`
	Staff struct {
		Edges []struct {
			Role string `json:"role"`
			Node struct {
				ID   int64 `json:"id"`
				Name struct {
					Full string `json:"full"`
				} `json:"name"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"staff"`
	Tags            []aniTag `json:"tags"`
	Recommendations struct {
		Nodes []struct {
			Rating int `json:"rating"`
			Media  *struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"mediaRecommendation"`
		} `json:"nodes"`
	} `json:"recommendations"`
}

// aniTag is the AniList Tag evidence the schema carries. Themes carry a
// relevance rank plus spoiler/adult flags, which is what makes a typed,
// non-spoiler theme projection possible without scraping descriptions.
type aniTag struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Rank         int    `json:"rank"`
	GeneralSpoil *bool  `json:"isGeneralSpoiler"`
	MediaSpoil   *bool  `json:"isMediaSpoiler"`
	Adult        *bool  `json:"isAdult"`
}

// aniMinTagRank drops the lowest-voted noise: a theme almost nobody tagged is
// not usable discovery evidence.
const aniMinTagRank = 30

// aniMaxTags bounds one record's theme list.
const aniMaxTags = 32

var aniTags = regexp.MustCompile(`<[^>]*>`)

func (v aniMedia) record(language string) (ScreenRecord, error) {
	r := ScreenRecord{Identity: ScreenID{"anilist", "anime", numberID(v.ID)}, Title: v.Title.name(language), OriginalTitle: v.Title.Native, Aliases: append([]string{}, v.Synonyms...), Overview: html.UnescapeString(aniTags.ReplaceAllString(v.Description, "")), Status: v.Status, Format: v.Format, SourceMaterial: v.Source, Season: v.Season, Year: v.SeasonYear, EpisodeCount: v.Episodes, Adult: v.Adult}
	if v.Type != "ANIME" {
		return r, &Error{Provider: "anilist", Code: "entity_type_mismatch"}
	}
	for _, title := range []string{v.Title.English, v.Title.Native, v.Title.Romaji} {
		if title != "" {
			r.Aliases = append(r.Aliases, title)
		}
	}
	if v.Start.Year != nil {
		r.Year = *v.Start.Year
		r.Date = strconv.Itoa(*v.Start.Year)
		if v.Start.Month != nil && *v.Start.Month >= 1 && *v.Start.Month <= 12 {
			r.Date += "-" + twoDigits(*v.Start.Month)
			if v.Start.Day != nil && *v.Start.Day >= 1 && *v.Start.Day <= 31 {
				r.Date += "-" + twoDigits(*v.Start.Day)
			}
		}
	}
	for _, g := range v.Genres {
		r.Genres = append(r.Genres, ScreenName{strings.ToLower(g), g})
	}
	for _, s := range v.Studios.Nodes {
		r.Relations = append(r.Relations, ScreenRelation{"studio", ScreenID{"anilist", "studio", numberID(s.ID)}, s.Name})
	}
	for _, e := range v.Relations.Edges {
		if e.Node.Type == "ANIME" {
			r.Relations = append(r.Relations, ScreenRelation{strings.ToLower(e.Relation), ScreenID{"anilist", "anime", numberID(e.Node.ID)}, e.Node.Title.name(language)})
		}
	}
	for _, e := range v.Staff.Edges {
		r.Credits = append(r.Credits, ScreenCredit{numberID(e.Node.ID), e.Node.Name.Full, e.Role, "Staff", len(r.Credits)})
	}
	// Themes are typed evidence, not a description scrape: never a spoiler or
	// adult tag, and only themes the community ranked. Duplicates and oversized
	// input are dropped per tag, never at the cost of the record.
	for _, tag := range v.Tags {
		if len(r.Tags) >= aniMaxTags {
			break
		}
		if tag.Name == "" || tag.Rank < aniMinTagRank {
			continue
		}
		if (tag.GeneralSpoil != nil && *tag.GeneralSpoil) || (tag.MediaSpoil != nil && *tag.MediaSpoil) || (tag.Adult != nil && *tag.Adult) {
			continue
		}
		name := strings.TrimSpace(tag.Name)
		id := numberID(tag.ID)
		if name == "" || id == "" || len(name) > 200 {
			continue
		}
		duplicate := false
		for _, t := range r.Tags {
			duplicate = duplicate || t.ID == id
		}
		if duplicate {
			continue
		}
		r.Tags = append(r.Tags, ScreenName{ID: id, Name: name})
	}
	// Recommendations keep AniList's order as rank: anime only (a manga
	// recommendation is not a watchable title), and only those the community
	// rated up. A deleted target arrives as null and is skipped.
	for _, n := range v.Recommendations.Nodes {
		if n.Rating > 0 && n.Media != nil && n.Media.Type == "ANIME" {
			r.Similar = appendSimilar(r.Similar, r.Identity, ScreenID{"anilist", "anime", numberID(n.Media.ID)})
		}
	}
	votes := 0
	for _, d := range v.Stats.Scores {
		votes += d.Amount
	}
	if v.Average != nil && votes > 0 {
		r.Rating = &ScreenRating{*v.Average, 100, votes}
	}
	// AniList describes works, not uniquely identified episodes. These are local
	// coordinate policies; no fake AniList episode IDs or regional certificates.
	r.Orders = []ScreenOrder{{"absolute", "Absolute episode numbers"}, {"seasonal", "One explicitly selected local season"}}
	return r, ValidateScreenRecord(r)
}
func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
func (s *AniList) query(ctx context.Context, key, query string, variables map[string]any, out any) error {
	return s.cache.get(ctx, key, out, func() (any, error) {
		var v struct {
			Data   any `json:"data"`
			Errors []struct {
				Message string `json:"message"`
				Status  int    `json:"status"`
			} `json:"errors"`
		}
		if e := s.http.request(ctx, "POST", "/", "", map[string]any{"query": query, "variables": variables}, &v); e != nil {
			return nil, e
		}
		if len(v.Errors) > 0 || v.Data == nil {
			return nil, &Error{Provider: "anilist", Code: "graphql_error"}
		}
		return v.Data, nil
	})
}
func (s *AniList) SearchScreen(ctx context.Context, kind, title string, year int, language, region string) ([]ScreenRecord, error) {
	if kind != "anime" || strings.TrimSpace(title) == "" || len(title) > 512 {
		return nil, errors.New("invalid anime query")
	}
	var data struct {
		Page struct {
			Media []aniMedia `json:"media"`
		} `json:"Page"`
	}
	q := `query($search:String){Page(page:1,perPage:25){media(type:ANIME,search:$search,sort:SEARCH_MATCH){` + animeFields + `}}}`
	if e := s.query(ctx, "search:"+title, q, map[string]any{"search": title}, &data); e != nil {
		return nil, e
	}
	if len(data.Page.Media) > 25 {
		return nil, &Error{Provider: "anilist", Code: "invalid_search"}
	}
	out := []ScreenRecord{}
	for _, v := range data.Page.Media {
		r, e := v.record(language)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *AniList) ScreenDetails(ctx context.Context, kind, id, language, region string) (ScreenRecord, error) {
	if kind != "anime" || !ValidScreenID(ScreenID{"anilist", "anime", id}) {
		return ScreenRecord{}, errors.New("invalid anime identity")
	}
	n, _ := strconv.ParseInt(id, 10, 32)
	var data struct {
		Media aniMedia `json:"Media"`
	}
	q := `query($id:Int){Media(id:$id,type:ANIME){` + animeDetailFields + `}}`
	if e := s.query(ctx, "id:"+id, q, map[string]any{"id": n}, &data); e != nil {
		return ScreenRecord{}, e
	}
	if data.Media.ID != n {
		return ScreenRecord{}, &Error{Provider: "anilist", Code: "identity_mismatch"}
	}
	return data.Media.record(language)
}
