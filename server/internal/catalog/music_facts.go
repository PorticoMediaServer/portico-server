package catalog

import (
	"database/sql"
	"errors"
	"portico.local/server/internal/assets"
)

// MusicTechnicalFacts contains safe measured codec facts, never a filename,
// source address, grant, or owner-only file metadata. Zero means unknown.
type MusicTechnicalFacts struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sampleRate,omitempty"`
	BitDepth   int    `json:"bitDepth,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	Lossless   *bool  `json:"lossless,omitempty"`
}

func audioLossless(codec string) bool { return assets.LosslessAudio(codec) }

func persistMusicFacts(tx *sql.Tx, asset string, f assets.Facts) error {
	for _, s := range f.Streams {
		if s.Type != "audio" {
			continue
		}
		d := s.Detail
		if d.SampleRate <= 0 || s.Channels <= 0 {
			return nil
		}
		_, e := tx.Exec(`INSERT INTO music_technical_facts(asset_id,size,modified_ns,codec,sample_rate,bit_depth,channels,lossless) SELECT token,size,modified_ns,?,?,?,?,? FROM catalog_assets WHERE token=? ON CONFLICT(asset_id) DO UPDATE SET size=excluded.size,modified_ns=excluded.modified_ns,codec=excluded.codec,sample_rate=excluded.sample_rate,bit_depth=excluded.bit_depth,channels=excluded.channels,lossless=excluded.lossless`, s.Codec, d.SampleRate, d.BitsPerSample, s.Channels, musicLossless(s.Codec, f), asset)
		return e
	}
	return nil
}

const musicFactsQuery = `SELECT f.codec,f.sample_rate,f.bit_depth,f.channels,f.lossless FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id JOIN music_technical_facts f ON f.asset_id=a.token AND f.size=a.size AND f.modified_ns=a.modified_ns WHERE e.public_id=pid_blob(?) AND e.kind=7 AND l.available=1 AND a.available=1 ORDER BY l.part_index,a.id LIMIT 1`

func (s *Service) musicFacts(item string) (*MusicTechnicalFacts, error) {
	var f MusicTechnicalFacts
	e := s.read().QueryRow(musicFactsQuery, item).Scan(&f.Codec, &f.SampleRate, &f.BitDepth, &f.Channels, &f.Lossless)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	return &f, e
}
func (s *Service) musicFactsPage(items map[string]*Item) error {
	ids := []string{}
	for id, item := range items {
		if item.Kind == "song" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, e := s.read().Query(`SELECT pid(e.public_id),f.codec,f.sample_rate,f.bit_depth,f.channels,f.lossless FROM catalog_entities e JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id JOIN music_technical_facts f ON f.asset_id=a.token AND f.size=a.size AND f.modified_ns=a.modified_ns WHERE e.public_id IN(SELECT pid_blob(value) FROM json_each(?)) AND e.kind=7 AND l.available=1 AND a.available=1 ORDER BY l.part_index,a.id`, idList(ids))
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var f MusicTechnicalFacts
		if e = rows.Scan(&id, &f.Codec, &f.SampleRate, &f.BitDepth, &f.Channels, &f.Lossless); e != nil {
			return e
		}
		if item := items[id]; item != nil && item.AudioFacts == nil {
			item.AudioFacts = &f
		}
	}
	return rows.Err()
}

func musicLossless(codec string, f assets.Facts) *bool {
	if codec == "wavpack" && f.EmbeddedAudio != nil && f.EmbeddedAudio.Lossless != nil {
		return f.EmbeddedAudio.Lossless
	}
	return assets.KnownLosslessAudio(codec)
}
