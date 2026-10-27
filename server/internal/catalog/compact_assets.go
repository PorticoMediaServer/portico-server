package catalog

import (
	"portico.local/server/internal/assets"
)

// compactItemAssetSummary keeps compactItemAssets' exact availability and
// maximum duration without transferring or allocating every linked source.
// Unavailable links still contribute their duration; an empty or all-negative
// duration set keeps the original loop's zero floor.
func (s *Service) compactItemAssetSummary(id string) (bool, float64, error) {
	var available bool
	var duration float64
	err := s.read().QueryRow(`SELECT COALESCE(MAX(l.available),0),COALESCE(MAX(CASE WHEN a.duration>0 THEN a.duration ELSE 0 END),0)
 FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id
 JOIN catalog_assets a ON a.id=l.asset_id
 WHERE e.public_id=pid_blob(?)`, id).Scan(&available, &duration)
	return available, duration, err
}

// compactItemAssets reads one item's asset-link facts and the assets'
// independently recorded facts. Catalogue facts are written synchronously, so
// there is no pending projection to fence; per-item availability is derived
// (domain 20) and served from catalog_item_availability instead.
func (s *Service) compactItemAssets(id string) ([]assets.Source, bool, float64, error) {
	rows, err := s.read().Query(`SELECT a.token,a.container,a.video_codec,a.audio_codec,a.width,a.height,a.duration,l.available
 FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id
 JOIN catalog_assets a ON a.id=l.asset_id
 WHERE e.public_id=pid_blob(?) ORDER BY l.part_index,a.token`, id)
	if err != nil {
		return nil, false, 0, err
	}
	defer rows.Close()
	sources := []assets.Source{}
	var available bool
	var duration float64
	for rows.Next() {
		var source assets.Source
		var sourceAvailable bool
		if err = rows.Scan(&source.ID, &source.Container, &source.VideoCodec, &source.AudioCodec, &source.Width, &source.Height, &source.Duration, &sourceAvailable); err != nil {
			return nil, false, 0, err
		}
		sources = append(sources, source)
		available = available || sourceAvailable
		if source.Duration > duration {
			duration = source.Duration
		}
	}
	return sources, available, duration, rows.Err()
}
