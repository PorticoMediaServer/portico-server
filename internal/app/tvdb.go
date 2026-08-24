package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	tvdbRequestTimeout        = 8 * time.Second
	tvdbTokenLifetime         = 24 * time.Hour
	maxTVDBLoginResponseBytes = 64 << 10
)

type tvdbEnvelope[T any] struct {
	Status string `json:"status"`
	Data   T      `json:"data"`
}

type tvdbLoginData struct {
	Token string `json:"token"`
}

type tvdbSearchResult struct {
	TVDBID      string `json:"tvdb_id"`
	Name        string `json:"name"`
	Year        string `json:"year"`
	Type        string `json:"type"`
	Overview    string `json:"overview"`
	ImageURL    string `json:"image_url"`
	PrimaryType string `json:"primary_type"`
}

type tvdbNamedValue struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
type tvdbRemoteID struct {
	ID         string `json:"id"`
	SourceName string `json:"sourceName"`
	Type       int    `json:"type"`
}
type tvdbCharacter struct {
	ID         int    `json:"id"`
	PeopleID   int    `json:"peopleId"`
	Name       string `json:"name"`
	PersonName string `json:"personName"`
	Type       int    `json:"type"`
	Image      string `json:"image"`
}

type tvdbExtendedRecord struct {
	ID         int              `json:"id"`
	Name       string           `json:"name"`
	Overview   string           `json:"overview"`
	FirstAired string           `json:"firstAired"`
	Year       string           `json:"year"`
	Score      float64          `json:"score"`
	Image      string           `json:"image"`
	Genres     []tvdbNamedValue `json:"genres"`
	Characters []tvdbCharacter  `json:"characters"`
	RemoteIDs  []tvdbRemoteID   `json:"remoteIds"`
	Seasons    []tvdbSeason     `json:"seasons"`
}

type tvdbSeason struct {
	ID     int            `json:"id"`
	Number int            `json:"number"`
	Type   tvdbSeasonType `json:"type"`
}

type tvdbSeasonType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type tvdbEpisode struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	Aired        string  `json:"aired"`
	Image        string  `json:"image"`
	Number       int     `json:"number"`
	SeasonNumber int     `json:"seasonNumber"`
	Year         string  `json:"year"`
	Score        float64 `json:"score"`
}

type tvdbEpisodePage struct {
	Episodes []tvdbEpisode `json:"episodes"`
}

func tvdbSearchType(mediaType string) string {
	switch mediaType {
	case "movie":
		return "movie"
	case "show", "anime", "season", "episode":
		return "series"
	default:
		return ""
	}
}

func (s *Server) tvdbAPIKey() string {
	if value := strings.TrimSpace(s.secretSetting(tvdbAPIKeySettingKey)); value != "" {
		return value
	}
	return strings.TrimSpace(s.cfg.TVDBAPIKey)
}

func (s *Server) tvdbConfigured() bool { return s.tvdbAPIKey() != "" }

func (s *Server) tvdbBaseURL() string {
	return strings.TrimRight(firstNonEmpty(strings.TrimSpace(s.cfg.TVDBBaseURL), "https://api4.thetvdb.com/v4"), "/")
}

func (s *Server) tvdbAccessToken(ctx context.Context, force bool) (string, error) {
	key := s.tvdbAPIKey()
	if key == "" {
		return "", errTVDBCredentialsMissing
	}
	digest := sha256.Sum256([]byte(key))
	s.tvdbTokenMu.Lock()
	defer s.tvdbTokenMu.Unlock()
	if !force && s.tvdbToken != "" && s.tvdbTokenCredentialHash == digest && time.Now().Before(s.tvdbTokenExpiresAt) {
		return s.tvdbToken, nil
	}
	body, err := json.Marshal(map[string]string{"apikey": key})
	if err != nil {
		return "", err
	}
	requestCtx, cancel := context.WithTimeout(ctx, tvdbRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, s.tvdbBaseURL()+"/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	var payload tvdbEnvelope[tvdbLoginData]
	if _, err = doProviderJSONRequest(requestCtx, "TheTVDB", req, maxTVDBLoginResponseBytes, &payload); err != nil {
		return "", err
	}
	token := strings.TrimSpace(payload.Data.Token)
	if token == "" {
		return "", fmt.Errorf("TheTVDB login response did not contain a token")
	}
	s.tvdbToken, s.tvdbTokenExpiresAt, s.tvdbTokenCredentialHash = token, time.Now().Add(tvdbTokenLifetime), digest
	return token, nil
}

func (s *Server) getTVDB(ctx context.Context, path string, queryParams map[string]string, out any) error {
	if !s.tvdbConfigured() {
		return errTVDBCredentialsMissing
	}
	endpoint, err := url.Parse(s.tvdbBaseURL() + path)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	for key, value := range queryParams {
		if strings.TrimSpace(value) != "" {
			query.Set(key, value)
		}
	}
	endpoint.RawQuery = query.Encode()
	for attempt := 0; attempt < 2; attempt++ {
		token, err := s.tvdbAccessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		if err = waitForTVDBSlot(ctx); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, tvdbRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		response, requestErr := doProviderJSONRequest(requestCtx, "TheTVDB", req, maxMetadataProviderResponseBytes, out)
		retryAfter := retryAfterDuration(response.Header.Get("Retry-After"))
		cancel()
		if requestErr == nil {
			return nil
		}
		if response.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt == 0 && retryAfter > 0 && retryAfter <= 30*time.Second {
			timer := time.NewTimer(retryAfter)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		return requestErr
	}
	return fmt.Errorf("TheTVDB request could not be authenticated")
}

func waitForTVDBSlot(ctx context.Context) error {
	tvdbThrottleMu.Lock()
	defer tvdbThrottleMu.Unlock()
	wait := 100*time.Millisecond - time.Since(tvdbLastRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	tvdbLastRequest = time.Now()
	return nil
}

func (s *Server) searchTVDB(ctx context.Context, item MediaItem, title, language string) ([]tvdbSearchResult, error) {
	providerType := tvdbSearchType(item.Type)
	if providerType == "" {
		return nil, errors.New("TheTVDB matching is only available for movies and television")
	}
	var payload tvdbEnvelope[[]tvdbSearchResult]
	if err := s.getTVDB(ctx, "/search", map[string]string{"query": title, "type": providerType, "language": normalizeMetadataLanguage(language)}, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func (s *Server) searchTVDBCandidates(ctx context.Context, item, searchItem MediaItem, languageOverride string) error {
	titles := tmdbQueryTitlesForItem(searchItem)
	if len(titles) == 0 {
		return nil
	}
	language := s.metadataLanguageForItem(item)
	if strings.TrimSpace(languageOverride) != "" {
		language = normalizeMetadataLanguage(languageOverride)
	}
	var all []tvdbSearchResult
	for _, title := range titles {
		results, err := s.searchTVDB(ctx, item, title, language)
		if err != nil {
			return err
		}
		all = append(all, results...)
	}
	best := bestTVDBResult(all, searchItem, titles, metadataSearchYear(searchItem))
	for _, candidate := range all {
		if strings.TrimSpace(candidate.TVDBID) == "" {
			continue
		}
		score := tvdbCandidateScore(candidate, searchItem, titles, metadataSearchYear(searchItem))
		_ = s.recordMatchCandidate(item.ID, "tvdb", candidate.TVDBID, tvdbSearchType(item.Type), "manual-search", score, candidate.TVDBID == best.TVDBID, strings.Join(titles, " | "), candidate)
	}
	return nil
}

func (s *Server) refreshMediaMetadataFromTVDB(ctx context.Context, item MediaItem) (MediaItem, error) {
	if !s.tvdbConfigured() {
		return MediaItem{}, errTVDBCredentialsMissing
	}
	item = s.withMediaFilesForMatching(item)
	providerType := tvdbSearchType(item.Type)
	if providerType == "" {
		return MediaItem{}, errors.New("TheTVDB refresh is only available for movies and television")
	}
	var result tvdbExtendedRecord
	source := "provider-id"
	if externalID, ok := s.tvdbProviderIDForItem(item, providerType); ok {
		id, _ := strconv.Atoi(externalID)
		var err error
		result, err = s.tvdbDetailsForItem(ctx, item, providerType, id)
		if err != nil {
			return MediaItem{}, err
		}
	} else {
		titles := tmdbQueryTitlesForItem(item)
		var candidates []tvdbSearchResult
		for _, title := range titles {
			found, err := s.searchTVDB(ctx, item, title, s.metadataLanguageForItem(item))
			if err != nil {
				return MediaItem{}, err
			}
			candidates = append(candidates, found...)
		}
		best := bestTVDBResult(candidates, item, titles, metadataSearchYear(item))
		id, _ := strconv.Atoi(best.TVDBID)
		if id <= 0 {
			return MediaItem{}, fmt.Errorf("%w: TheTVDB returned no match", errNoMetadataMatch)
		}
		var err error
		result, err = s.tvdbDetailsForItem(ctx, item, providerType, id)
		if err != nil {
			return MediaItem{}, err
		}
		source = "provider-search"
		score := tvdbCandidateScore(best, item, titles, metadataSearchYear(item))
		_ = s.recordMatchCandidate(item.ID, "tvdb", best.TVDBID, providerType, source, score, true, strings.Join(titles, " | "), best)
	}
	if result.ID <= 0 {
		return MediaItem{}, fmt.Errorf("%w: TheTVDB returned no details", errNoMetadataMatch)
	}
	update := tvdbUpdateForResult(item, result)
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataOrigin, update.metadataSource, update.metadataProvider, update.metadataRefreshed = metadataSourceProvider, source, "tvdb", true
	update.metadataIdentities = tvdbIdentityProposals(result, providerType, .85)
	return s.saveMetadataUpdate("", item.ID, update)
}

func (s *Server) applyManualTVDBMatch(ctx context.Context, userID string, item MediaItem, externalID, externalType string) (MediaItem, error) {
	expectedType := tvdbSearchType(item.Type)
	if expectedType == "" {
		return MediaItem{}, errors.New("TheTVDB matching is only available for movies and television")
	}
	providerType := firstNonEmpty(strings.TrimSpace(externalType), expectedType)
	if providerType != expectedType {
		return MediaItem{}, fmt.Errorf("TheTVDB externalType %q does not match %s item identity %q", providerType, item.Type, expectedType)
	}
	id, err := strconv.Atoi(externalID)
	if err != nil || id <= 0 {
		return MediaItem{}, errors.New("TheTVDB externalId must be a positive integer")
	}
	result, err := s.tvdbDetailsForItem(ctx, item, providerType, id)
	if err != nil {
		return MediaItem{}, err
	}
	update := tvdbUpdateForResult(item, result)
	update.ExpectedRevision = &item.MetadataRevision
	update.metadataOrigin, update.metadataSource, update.metadataProvider, update.metadataActor, update.metadataRefreshed = metadataSourceProvider, "manual-match", "tvdb", userID, true
	update.metadataIdentities = tvdbIdentityProposals(result, providerType, 1)
	if len(update.metadataIdentities) > 0 {
		update.metadataIdentities[0].ExplicitAcceptance = true
	}
	updated, err := s.saveMetadataUpdate("", item.ID, update)
	if err != nil {
		return MediaItem{}, err
	}
	var score candidateScore
	score.add("manual_match", 100, "Selected by a user in the metadata editor")
	_ = s.recordMatchCandidate(item.ID, "tvdb", externalID, providerType, "manual-match", score, true, item.Title, result)
	return s.getMediaDetailShell(userID, updated.ID)
}

func (s *Server) tvdbDetails(ctx context.Context, providerType string, id int) (tvdbExtendedRecord, error) {
	if id <= 0 {
		return tvdbExtendedRecord{}, errors.New("TheTVDB ID must be positive")
	}
	resource := "series"
	if providerType == "movie" {
		resource = "movies"
	}
	var payload tvdbEnvelope[tvdbExtendedRecord]
	if err := s.getTVDB(ctx, fmt.Sprintf("/%s/%d/extended", resource, id), map[string]string{"meta": "translations"}, &payload); err != nil {
		return tvdbExtendedRecord{}, err
	}
	return payload.Data, nil
}

func (s *Server) tvdbDetailsForItem(ctx context.Context, item MediaItem, providerType string, id int) (tvdbExtendedRecord, error) {
	details, err := s.tvdbDetails(ctx, providerType, id)
	if err != nil {
		return details, err
	}
	if item.Type == "season" {
		seasonNumber := seasonNumberForMetadata(item)
		for _, season := range details.Seasons {
			if season.Number != seasonNumber || (season.Type.Type != "" && !strings.EqualFold(season.Type.Type, "official")) {
				continue
			}
			var payload tvdbEnvelope[tvdbExtendedRecord]
			if err := s.getTVDB(ctx, fmt.Sprintf("/seasons/%d/extended", season.ID), nil, &payload); err != nil {
				return tvdbExtendedRecord{}, err
			}
			payload.Data.ID = id // the durable provider identity remains the parent series
			if strings.TrimSpace(payload.Data.Name) == "" {
				payload.Data.Name = fmt.Sprintf("Season %d", seasonNumber)
			}
			return payload.Data, nil
		}
		return tvdbExtendedRecord{}, fmt.Errorf("%w: TheTVDB returned no matching season", errNoMetadataMatch)
	}
	if item.Type != "episode" || item.EpisodeNumber <= 0 {
		return details, nil
	}
	seasonNumber := seasonNumberForMetadata(item)
	for page := 0; page < 20; page++ {
		var payload tvdbEnvelope[tvdbEpisodePage]
		if err := s.getTVDB(ctx, fmt.Sprintf("/series/%d/episodes/default", id), map[string]string{"page": strconv.Itoa(page)}, &payload); err != nil {
			return tvdbExtendedRecord{}, err
		}
		for _, episode := range payload.Data.Episodes {
			if episode.Number == item.EpisodeNumber && episode.SeasonNumber == seasonNumber {
				details.Name, details.Overview, details.FirstAired, details.Year = episode.Name, episode.Overview, episode.Aired, episode.Year
				details.Image, details.Score = episode.Image, episode.Score
				return details, nil
			}
		}
		if len(payload.Data.Episodes) == 0 {
			break
		}
	}
	return tvdbExtendedRecord{}, fmt.Errorf("%w: TheTVDB returned no matching episode", errNoMetadataMatch)
}

func (s *Server) tvdbProviderIDForItem(item MediaItem, providerType string) (string, bool) {
	for _, mediaID := range metadataCascadeIDs(item) {
		if id, ok := s.mediaProviderID(mediaID, "tvdb", providerType); ok {
			return id, true
		}
		// Older NFO imports may have been stored before a precise external type
		// was known; accept that identity without discarding its provenance.
		if id, ok := s.mediaProviderID(mediaID, "tvdb", ""); ok {
			return id, true
		}
	}
	return "", false
}

func tvdbUpdateForResult(item MediaItem, result tvdbExtendedRecord) UpdateMediaRequest {
	update := UpdateMediaRequest{}
	if title := strings.TrimSpace(result.Name); title != "" {
		update.Title = &title
		sortTitle := sortableTitle(title)
		update.SortTitle = &sortTitle
	}
	if overview := strings.TrimSpace(result.Overview); overview != "" {
		update.Summary = &overview
	}
	if year := firstPositiveInt(yearFromTMDBDate(result.FirstAired), parseTVDBYear(result.Year)); year > 0 {
		update.Year = &year
	}
	genres := make([]string, 0, len(result.Genres))
	for _, genre := range result.Genres {
		if strings.TrimSpace(genre.Name) != "" {
			genres = append(genres, strings.TrimSpace(genre.Name))
		}
	}
	if len(genres) > 0 {
		update.Genres = &genres
	}
	if image := strings.TrimSpace(result.Image); image != "" {
		artwork := map[string]string{"source": "tvdb", "posterURL": image}
		update.Artwork = &artwork
	}
	people := make([]MediaPerson, 0, len(result.Characters))
	for index, character := range result.Characters {
		name := firstNonEmpty(strings.TrimSpace(character.PersonName), strings.TrimSpace(character.Name))
		if name == "" {
			continue
		}
		role := "Actor"
		if character.Type == 3 {
			role = "Director"
		}
		providerIDs := map[string]string{}
		if character.PeopleID > 0 {
			providerIDs["tvdb"] = strconv.Itoa(character.PeopleID)
		}
		people = append(people, MediaPerson{Name: name, Role: role, Character: character.Name, SortOrder: index, ImageURL: character.Image, ProviderIDs: providerIDs})
	}
	if len(people) > 0 {
		update.People = &people
	}
	return update
}

func tvdbIdentityProposals(result tvdbExtendedRecord, providerType string, confidence float64) []metadataProviderIdentityProposal {
	identities := []metadataProviderIdentityProposal{{Provider: "tvdb", ExternalID: strconv.Itoa(result.ID), ExternalType: providerType, Confidence: confidence}}
	for _, remote := range result.RemoteIDs {
		provider := normalizedMetadataProvider(remote.SourceName)
		if provider == "themoviedatabase" {
			provider = "tmdb"
		}
		if provider == "imdb" || provider == "tmdb" {
			identities = append(identities, metadataProviderIdentityProposal{Provider: provider, ExternalID: strings.TrimSpace(remote.ID), ExternalType: providerType, Confidence: confidence * .9})
		}
	}
	return identities
}

func bestTVDBResult(results []tvdbSearchResult, item MediaItem, titles []string, year int) tvdbSearchResult {
	var best tvdbSearchResult
	var bestScore candidateScore
	var runnerScore *candidateScore
	hasBest := false
	for _, result := range results {
		score := tvdbCandidateScore(result, item, titles, year)
		if !hasBest || score.Score > bestScore.Score {
			if hasBest && result.TVDBID != best.TVDBID {
				previous := bestScore
				runnerScore = &previous
			}
			best, bestScore, hasBest = result, score, true
		} else if result.TVDBID != best.TVDBID && (runnerScore == nil || score.Score > runnerScore.Score) {
			candidate := score
			runnerScore = &candidate
		}
	}
	if !hasBest || !automaticMetadataMatchAccepted(bestScore, runnerScore) {
		return tvdbSearchResult{}
	}
	return best
}

func tvdbCandidateScore(result tvdbSearchResult, item MediaItem, titles []string, year int) candidateScore {
	var score candidateScore
	expectedType := tvdbSearchType(item.Type)
	actualType := strings.ToLower(strings.TrimSpace(firstNonEmpty(result.Type, result.PrimaryType)))
	if actualType != "" && expectedType != "" && actualType != expectedType {
		score.add("wrong_kind", -100, fmt.Sprintf("expected %s got %s", expectedType, actualType))
	}
	if delta, reasons := titleScoreReasons(titles, result.Name, 70); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	if delta, reasons := yearScoreReason(year, parseTVDBYear(result.Year), 24, 8, 30); delta != 0 {
		score.add(reasons[0].Code, delta, reasons[0].Detail)
	}
	addIdentityEvidenceScore(&score, item, map[string]string{"title": result.Name, "year": result.Year})
	return score
}

func parseTVDBYear(value string) int {
	year, _ := strconv.Atoi(strings.TrimSpace(value))
	if year < 1800 || year > 3000 {
		return 0
	}
	return year
}
