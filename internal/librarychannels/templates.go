package librarychannels

import "encoding/json"

const BuiltInTemplateVersion = 1

type ChannelTemplate struct {
	Key                   string
	Version               int
	Name                  string
	Description           string
	RuleName              string
	Query                 json.RawMessage
	SelectionMode         SelectionMode
	EpisodeMode           EpisodeMode
	MinimumCandidates     int
	MinimumDistinctSeries int
	RequiredEntityKinds   []string
	CandidateLimit        int
	RecencyDays           int
	Sort                  []TemplateSort
}

type TemplateSort struct {
	Field     string
	Direction string
}

type BlockPreset struct {
	Key          string
	Version      int
	Name         string
	Description  string
	Weekdays     WeekdayMask
	StartMinute  int
	EndMinute    int
	Anchored     bool
	AllowOverrun bool
}

// BuiltInChannelTemplates returns immutable product defaults as fresh values.
// Installation remains an explicit administration action: inventory analysis
// must omit inapplicable templates rather than creating enabled empty channels.
func BuiltInChannelTemplates() []ChannelTemplate {
	templates := []ChannelTemplate{
		movieTemplate("movie-time", "Movie Time", "A broad mix of movies from across this server.", nil),
		movieTemplate("classic-cinema", "Classic Cinema", "Movies released before 1970.", filters(filter("year", "less-than", 1970))),
		movieTemplate("movies-1970s", "The '70s", "Movies released from 1970 through 1979.", filters(filter("year", "between", []int{1970, 1979}))),
		movieTemplate("movies-1980s", "The '80s", "Movies released from 1980 through 1989.", filters(filter("year", "between", []int{1980, 1989}))),
		movieTemplate("movies-1990s", "The '90s", "Movies released from 1990 through 1999.", filters(filter("year", "between", []int{1990, 1999}))),
		movieTemplate("movies-2000s", "The 2000s", "Movies released from 2000 through 2009.", filters(filter("year", "between", []int{2000, 2009}))),
		movieTemplate("action-movies", "Action", "Action movies from this server.", filters(filter("genre", "contains", "Action"))),
		movieTemplate("adventure-movies", "Adventure", "Adventure movies from this server.", filters(filter("genre", "contains", "Adventure"))),
		movieTemplate("comedy-movies", "Comedy", "Comedy movies from this server.", filters(filter("genre", "contains", "Comedy"))),
		movieTemplate("drama-movies", "Drama", "Drama movies from this server.", filters(filter("genre", "contains", "Drama"))),
		movieTemplate("science-fiction-movies", "Science Fiction", "Science-fiction movies from this server.", filters(filter("genre", "contains-any", []string{"Science Fiction", "Sci-Fi"}))),
		movieTemplate("horror-after-dark", "Horror After Dark", "Horror movies for late-night viewing.", filters(filter("genre", "contains", "Horror"))),
		movieTemplate("thriller-movies", "Thrillers", "Thriller movies from this server.", filters(filter("genre", "contains", "Thriller"))),
		movieTemplate("family-movie-night", "Family Movie Night", "Family-friendly movies for everyone.", filters(filter("genre", "contains-any", []string{"Family", "Children"}))),
		movieTemplate("documentary-films", "Documentary", "Documentary films from this server.", filters(filter("genre", "contains", "Documentary"))),
		movieTemplate("animated-films", "Animation", "Animated movies from this server.", filters(filter("genre", "contains", "Animation"))),
		showTemplate("all-television", "All Television", "A broad mix of television from this server.", nil, EpisodeInOrder),
		showTemplate("sitcoms", "Sitcoms", "Television comedies from this server.", filters(filter("genre", "contains-any", []string{"Comedy", "Sitcom"})), EpisodeInOrder),
		showTemplate("television-drama", "TV Drama", "Television dramas from this server.", filters(filter("genre", "contains", "Drama")), EpisodeInOrder),
		showTemplate("crime-and-mystery", "Crime & Mystery", "Crime and mystery television from this server.", filters(filter("genre", "contains-any", []string{"Crime", "Mystery"})), EpisodeInOrder),
		showTemplate("science-fiction-and-fantasy", "Sci-Fi & Fantasy", "Science-fiction and fantasy television.", filters(filter("genre", "contains-any", []string{"Science Fiction", "Sci-Fi", "Fantasy"})), EpisodeInOrder),
		showTemplate("reality-and-competition", "Reality & Competition", "Reality and competition series from this server.", filters(filter("genre", "contains-any", []string{"Reality", "Game Show"})), EpisodeInOrder),
		showTemplate("kids-and-family", "Kids & Family", "Television selected for younger viewers and families.", filters(filter("genre", "contains-any", []string{"Children", "Family"})), EpisodeInOrder),
		showTemplate("saturday-morning-cartoons", "Saturday Morning Cartoons", "Animated television in a classic Saturday-morning block.", filters(filter("genre", "contains", "Animation")), EpisodeInOrder),
		showTemplate("anime", "Anime", "Anime series from this server.", filters(filter("genre", "contains", "Anime")), EpisodeInOrder),
		{
			Key: "recently-added", Version: BuiltInTemplateVersion, Name: "Recently Added",
			Description: "A rotating mix of recently added movies and television.", RuleName: "Recently added media",
			Query:         query(map[string]any{}),
			Sort:          []TemplateSort{{Field: "dateAdded", Direction: "desc"}},
			SelectionMode: SelectionSequential, EpisodeMode: EpisodeNone, MinimumCandidates: 12,
			RequiredEntityKinds: []string{"movie", "show"}, CandidateLimit: 200, RecencyDays: 90,
		},
		showTemplate("marathon-tv", "Marathon TV", "One series at a time, with episodes shown in order.", nil, EpisodeMarathon),
	}
	result := make([]ChannelTemplate, len(templates))
	for index, template := range templates {
		result[index] = template
		result[index].Query = append(json.RawMessage(nil), template.Query...)
		result[index].Sort = append([]TemplateSort(nil), template.Sort...)
		result[index].RequiredEntityKinds = append([]string(nil), template.RequiredEntityKinds...)
	}
	return result
}

func BuiltInBlockPresets() []BlockPreset {
	allDays := WeekdayMask(127)
	weekdays := WeekdayMask((1 << 1) | (1 << 2) | (1 << 3) | (1 << 4) | (1 << 5))
	weekends := WeekdayMask((1 << 0) | (1 << 6))
	return []BlockPreset{
		{Key: "movie-night", Version: 1, Name: "Movie Night", Description: "An anchored evening movie block.", Weekdays: allDays, StartMinute: 19 * 60, EndMinute: 23 * 60, Anchored: true},
		{Key: "saturday-morning-cartoons", Version: 1, Name: "Saturday Morning Cartoons", Description: "A Saturday morning animation block.", Weekdays: 1 << 6, StartMinute: 7 * 60, EndMinute: 12 * 60, Anchored: true},
		{Key: "weekday-sitcoms", Version: 1, Name: "Weekday Sitcoms", Description: "A weekday early-evening comedy block.", Weekdays: weekdays, StartMinute: 17 * 60, EndMinute: 19 * 60},
		{Key: "primetime-drama", Version: 1, Name: "Primetime Drama", Description: "An anchored evening drama block.", Weekdays: allDays, StartMinute: 20 * 60, EndMinute: 23 * 60, Anchored: true},
		{Key: "late-night-horror", Version: 1, Name: "Late Night Horror", Description: "A late-night horror block.", Weekdays: weekends, StartMinute: 22 * 60, EndMinute: 2 * 60, Anchored: true},
		{Key: "weekend-marathon", Version: 1, Name: "Weekend Marathon", Description: "A long weekend series block.", Weekdays: weekends, StartMinute: 10 * 60, EndMinute: 22 * 60, AllowOverrun: true},
		{Key: "documentary-hour", Version: 1, Name: "Documentary Hour", Description: "A recurring documentary block.", Weekdays: allDays, StartMinute: 18 * 60, EndMinute: 19 * 60, Anchored: true},
		{Key: "family-afternoon", Version: 1, Name: "Family Afternoon", Description: "An afternoon family-viewing block.", Weekdays: weekends, StartMinute: 13 * 60, EndMinute: 17 * 60},
		{Key: "throwback-night", Version: 1, Name: "Throwback Night", Description: "A block for a selected decade or era.", Weekdays: 1 << 5, StartMinute: 19 * 60, EndMinute: 0, Anchored: true},
		{Key: "new-this-week", Version: 1, Name: "New This Week", Description: "A showcase for recently added media.", Weekdays: 1 << 0, StartMinute: 18 * 60, EndMinute: 21 * 60, Anchored: true},
		{Key: "studio-spotlight", Version: 1, Name: "Studio Spotlight", Description: "A block limited to a selected studio.", Weekdays: allDays, StartMinute: 14 * 60, EndMinute: 17 * 60},
		{Key: "network-spotlight", Version: 1, Name: "Network Spotlight", Description: "A block limited to a selected network.", Weekdays: allDays, StartMinute: 14 * 60, EndMinute: 17 * 60},
		{Key: "genre-showcase", Version: 1, Name: "Genre Showcase", Description: "A block focused on a selected genre.", Weekdays: allDays, StartMinute: 19 * 60, EndMinute: 22 * 60},
		{Key: "seasonal-holiday", Version: 1, Name: "Seasonal & Holiday", Description: "A temporary seasonal programming block.", Weekdays: allDays, StartMinute: 8 * 60, EndMinute: 22 * 60},
		{Key: "all-day-marathon", Version: 1, Name: "All-Day Marathon", Description: "A full-day ordered series marathon.", Weekdays: weekends, StartMinute: 0, EndMinute: 0, AllowOverrun: true},
	}
}

func movieTemplate(key, name, description string, extra []any) ChannelTemplate {
	base := []any{filter("entityKind", "equals", "movie")}
	base = append(base, extra...)
	return ChannelTemplate{
		Key: key, Version: BuiltInTemplateVersion, Name: name, Description: description,
		RuleName: name, Query: query(map[string]any{"all": base}),
		SelectionMode: SelectionShuffleBag, EpisodeMode: EpisodeNone, MinimumCandidates: 12,
		RequiredEntityKinds: []string{"movie"},
	}
}

func showTemplate(key, name, description string, extra []any, episodeMode EpisodeMode) ChannelTemplate {
	base := []any{filter("entityKind", "equals", "show")}
	base = append(base, extra...)
	return ChannelTemplate{
		Key: key, Version: BuiltInTemplateVersion, Name: name, Description: description,
		RuleName: name, Query: query(map[string]any{"all": base}),
		SelectionMode: SelectionShuffleBag, EpisodeMode: episodeMode, MinimumCandidates: 12,
		MinimumDistinctSeries: 3, RequiredEntityKinds: []string{"show"},
	}
}

type TemplateInventory struct {
	CandidateCount int
	DistinctSeries int
	EntityKinds    map[string]int
}

// EvaluateTemplateApplicability fails closed before defaults are offered. The
// query compiler remains responsible for producing the inventory for the
// exact template query.
func EvaluateTemplateApplicability(template ChannelTemplate, inventory TemplateInventory) bool {
	if inventory.CandidateCount < template.MinimumCandidates || inventory.DistinctSeries < template.MinimumDistinctSeries {
		return false
	}
	for _, kind := range template.RequiredEntityKinds {
		if inventory.EntityKinds[kind] > 0 {
			return true
		}
	}
	return len(template.RequiredEntityKinds) == 0
}

func filters(values ...any) []any {
	return values
}

func filter(field, operator string, value any) map[string]any {
	return map[string]any{"field": field, "operator": operator, "value": value}
}

func query(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
