package catalog

import (
	"portico.local/server/internal/assets"
)

// TitleFile is one file of a title as its Files list reads: where it is (owners only), how big
// it is and every track in it. It rides on the detail response only; browse rows keep the short
// assets.Source.
type TitleFile struct {
	ID        string           `json:"id"`
	Path      string           `json:"path,omitempty"`
	Size      int64            `json:"size"`
	Container string           `json:"container"`
	Duration  float64          `json:"duration"`
	BitRate   int64            `json:"bitRate,omitempty"`
	Available bool             `json:"available"`
	Video     *TitleFileVideo  `json:"video,omitempty"`
	Audio     []TitleFileTrack `json:"audio"`
	Subtitles []TitleFileTrack `json:"subtitles"`
}

type TitleFileVideo struct {
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// DynamicRange is one of the assets.Range constants ("sdr", "hdr10", "hlg", "dolby_vision").
	DynamicRange string  `json:"dynamicRange,omitempty"`
	HDR10Plus    bool    `json:"hdr10Plus,omitempty"`
	FrameRate    float64 `json:"frameRate,omitempty"`
	BitDepth     int     `json:"bitDepth,omitempty"`
	BitRate      int64   `json:"bitRate,omitempty"`
}

// TitleFileTrack is an audio or subtitle track. External is a subtitle file beside the media.
type TitleFileTrack struct {
	Language      string `json:"language,omitempty"`
	Codec         string `json:"codec"`
	Title         string `json:"title,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channelLayout,omitempty"`
	ObjectAudio   string `json:"objectAudio,omitempty"`
	BitRate       int64  `json:"bitRate,omitempty"`
	Default       bool   `json:"default,omitempty"`
	Forced        bool   `json:"forced,omitempty"`
	External      bool   `json:"external,omitempty"`
}

// titleFiles reads a title's files with their tracks: a handful of rows per file, the title's
// own. The path is the owner's to see.
func (s *Service) titleFiles(item string, owner bool) ([]TitleFile, error) {
	rows, err := s.read().Query(`SELECT a.token,a.path,a.size,a.container,a.duration,a.video_codec,a.width,a.height,link.available
 FROM catalog_asset_links link JOIN catalog_assets a ON a.id=link.asset_id
 WHERE link.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) ORDER BY link.part_index,a.token`, item)
	if err != nil {
		return nil, err
	}
	out := []TitleFile{}
	for rows.Next() {
		f := TitleFile{Audio: []TitleFileTrack{}, Subtitles: []TitleFileTrack{}}
		var codec string
		var width, height int
		if err = rows.Scan(&f.ID, &f.Path, &f.Size, &f.Container, &f.Duration, &codec, &width, &height, &f.Available); err != nil {
			rows.Close()
			return nil, err
		}
		if !owner {
			f.Path = ""
		}
		if f.Duration > 0 && f.Size > 0 {
			f.BitRate = int64(float64(f.Size) * 8 / f.Duration)
		}
		if codec != "" || height > 0 {
			f.Video = &TitleFileVideo{Codec: codec, Width: width, Height: height}
		}
		out = append(out, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		f := &out[i]
		streams, err := s.read().Query(`SELECT type,codec,language,title,channels,channel_layout,is_default,is_forced,color_transfer,detail_json FROM asset_streams WHERE asset_id=? AND type IN('video','audio') ORDER BY stream_index`, f.ID)
		if err != nil {
			return nil, err
		}
		video := false
		for streams.Next() {
			var kind, transfer, raw string
			var t TitleFileTrack
			if err = streams.Scan(&kind, &t.Codec, &t.Language, &t.Title, &t.Channels, &t.ChannelLayout, &t.Default, &t.Forced, &transfer, &raw); err != nil {
				streams.Close()
				return nil, err
			}
			var stream assets.Stream
			stream.DecodeDetail(raw)
			detail := stream.Detail
			if kind == "audio" {
				t.ObjectAudio, t.BitRate = detail.ObjectAudio, detail.BitRate
				f.Audio = append(f.Audio, t)
				continue
			}
			// The first video stream is the picture; later ones are attached covers.
			if video || f.Video == nil {
				continue
			}
			video = true
			f.Video.DynamicRange = detail.DynamicRange
			if f.Video.DynamicRange == "" {
				f.Video.DynamicRange = assets.DynamicRangeOf(transfer, detail.DolbyVisionProfile)
			}
			f.Video.HDR10Plus, f.Video.FrameRate, f.Video.BitDepth, f.Video.BitRate = detail.HDR10Plus, detail.FrameRate, detail.BitDepth, detail.BitRate
		}
		err = streams.Err()
		streams.Close()
		if err != nil {
			return nil, err
		}
		subs, err := s.read().Query(`SELECT format,language,title,is_default,is_forced,origin='sidecar' FROM asset_subtitles WHERE asset_id=? ORDER BY origin,stream_index,id`, f.ID)
		if err != nil {
			return nil, err
		}
		for subs.Next() {
			var t TitleFileTrack
			if err = subs.Scan(&t.Codec, &t.Language, &t.Title, &t.Default, &t.Forced, &t.External); err != nil {
				subs.Close()
				return nil, err
			}
			f.Subtitles = append(f.Subtitles, t)
		}
		err = subs.Err()
		subs.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
