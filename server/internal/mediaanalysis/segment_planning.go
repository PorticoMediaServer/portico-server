package mediaanalysis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// segmentPlanning is captured once in each existing producer transaction.
// Its identity fences queued work and publication when the owner changes limits.
type segmentPlanning struct{ EpisodeMinutes, FilmMinutes int64 }

func defaultSegmentPlanning() segmentPlanning { return segmentPlanning{2, 10} }
func (p segmentPlanning) digest() string {
	return token("segment-planning-1", fmt.Sprint(p.EpisodeMinutes), fmt.Sprint(p.FilmMinutes))
}
func segmentPlanningTx(ctx context.Context, q queryer) (segmentPlanning, error) {
	p := defaultSegmentPlanning()
	var episode, film sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT "segments.shortestEpisode","segments.shortestFilm" FROM server_settings WHERE singleton=1`).Scan(&episode, &film)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	if episode.Valid {
		p.EpisodeMinutes = episode.Int64
	}
	if film.Valid {
		p.FilmMinutes = film.Int64
	}
	if p.EpisodeMinutes < 1 || p.EpisodeMinutes > 30 || p.FilmMinutes < 5 || p.FilmMinutes > 60 || p.FilmMinutes%5 != 0 {
		return p, ErrInput
	}
	return p, nil
}
