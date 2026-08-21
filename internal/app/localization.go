package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleLocalization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	info, err := s.localizationInfoContext(r.Context())
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "localization_unavailable", "Localization options are unavailable.")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) localizationInfo() (LocalizationInfo, error) {
	return s.localizationInfoContext(context.Background())
}

func (s *Server) localizationInfoContext(ctx context.Context) (LocalizationInfo, error) {
	locales, err := s.localizationOptionsContext(ctx, "locale")
	if err != nil {
		return LocalizationInfo{}, err
	}
	languages, err := s.localizationOptionsContext(ctx, "language")
	if err != nil {
		return LocalizationInfo{}, err
	}
	countries, err := s.localizationOptionsContext(ctx, "country")
	if err != nil {
		return LocalizationInfo{}, err
	}
	timeZones, err := s.localizationTimeZonesContext(ctx)
	if err != nil {
		return LocalizationInfo{}, err
	}
	ratingSystems, err := s.localizationRatingSystemsContext(ctx)
	if err != nil {
		return LocalizationInfo{}, err
	}
	return LocalizationInfo{
		Locales:       locales,
		Languages:     languages,
		Countries:     countries,
		TimeZones:     timeZones,
		RatingSystems: ratingSystems,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) localizationOptions(kind string) ([]LocalizationOption, error) {
	return s.localizationOptionsContext(context.Background(), kind)
}

func (s *Server) localizationOptionsContext(ctx context.Context, kind string) ([]LocalizationOption, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT id, label, labels_json
		FROM localization_options
		WHERE kind = ?
		ORDER BY sort_order ASC, id ASC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := []LocalizationOption{}
	for rows.Next() {
		var option LocalizationOption
		var labelsJSON string
		if err := rows.Scan(&option.ID, &option.Label, &labelsJSON); err != nil {
			return nil, err
		}
		option.Labels = decodeLocalizationLabels(labelsJSON)
		options = append(options, option)
	}
	return options, rows.Err()
}

func (s *Server) localizationTimeZones() ([]string, error) {
	return s.localizationTimeZonesContext(context.Background())
}

func (s *Server) localizationTimeZonesContext(ctx context.Context) ([]string, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT id
		FROM localization_options
		WHERE kind = 'time_zone'
		ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	timeZones := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		timeZones = append(timeZones, id)
	}
	return timeZones, rows.Err()
}

func (s *Server) localizationRatingSystems() ([]LocalizationRatingSet, error) {
	return s.localizationRatingSystemsContext(context.Background())
}

func (s *Server) localizationRatingSystemsContext(ctx context.Context) ([]LocalizationRatingSet, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT country, system, label, labels_json
		FROM localization_rating_systems
		ORDER BY sort_order ASC, country ASC, system ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	systems := []LocalizationRatingSet{}
	for rows.Next() {
		var system LocalizationRatingSet
		var labelsJSON string
		if err := rows.Scan(&system.Country, &system.System, &system.Label, &labelsJSON); err != nil {
			return nil, err
		}
		system.Labels = decodeLocalizationLabels(labelsJSON)
		ratings, err := s.localizationRatingsContext(ctx, system.Country, system.System)
		if err != nil {
			return nil, err
		}
		system.Ratings = ratings
		systems = append(systems, system)
	}
	return systems, rows.Err()
}

func (s *Server) localizationRatings(country, system string) ([]LocalizationRating, error) {
	return s.localizationRatingsContext(context.Background(), country, system)
}

func (s *Server) localizationRatingsContext(ctx context.Context, country, system string) ([]LocalizationRating, error) {
	rows, err := s.queryUserRead(ctx, `
		SELECT rating, label, labels_json, rank, minimum_age
		FROM localization_rating_values
		WHERE country = ? AND system = ?
		ORDER BY sort_order ASC, rating ASC`, country, system)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ratings := []LocalizationRating{}
	for rows.Next() {
		var rating LocalizationRating
		var labelsJSON string
		if err := rows.Scan(&rating.ID, &rating.Label, &labelsJSON, &rating.Rank, &rating.MinimumAge); err != nil {
			return nil, err
		}
		rating.Labels = decodeLocalizationLabels(labelsJSON)
		ratings = append(ratings, rating)
	}
	return ratings, rows.Err()
}

func decodeLocalizationLabels(raw string) map[string]string {
	labels := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return map[string]string{}
	}
	for key, value := range labels {
		if key == "" || value == "" {
			delete(labels, key)
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}
