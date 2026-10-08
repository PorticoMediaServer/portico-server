package playback

import (
	"context"
	"database/sql"
	"strings"

	"portico.local/server/internal/assets"
)

// SourceVideo and SourceAudio are the planner's view of one file. They are read
// from the scanner's stream rows; nothing here opens the file.
type SourceVideo struct {
	Index int    `json:"index"`
	Codec string `json:"codec"`
	assets.StreamDetail
}

type SourceAudio struct {
	Title      string `json:"title,omitempty"`
	Index      int    `json:"index"`
	Ordinal    int    `json:"ordinal"`
	Codec      string `json:"codec"`
	Channels   int    `json:"channels"`
	Layout     string `json:"layout,omitempty"`
	Language   string `json:"language,omitempty"`
	Default    bool   `json:"default,omitempty"`
	Commentary bool   `json:"commentary,omitempty"`
	assets.StreamDetail
}

// DeliverySource is everything planning knows about one file.
type DeliverySource struct {
	ID        string
	Container string
	Duration  float64
	// BitRate is the whole file's average rate, from size and duration when the
	// container did not state one. It is what a byte-range delivery costs.
	BitRate    int64
	MaxBitRate int64
	Video      *SourceVideo
	Audio      []SourceAudio
	// DetailKnown is false for a file probed before stream detail existed and
	// not refreshed since. Unobserved facts do not reject a route; they are noted.
	DetailKnown bool
}

// audioFamily folds the PCM and DTS variants ffprobe names individually into the
// one name a device declares.
func audioFamily(codec string) string {
	switch {
	case strings.HasPrefix(codec, "pcm_"):
		return "pcm"
	case codec == "dca":
		return "dts"
	case codec == "mp2", codec == "mp1":
		return "mp2"
	}
	return codec
}

func videoFamily(codec string) string {
	switch codec {
	case "h265":
		return "hevc"
	case "mpeg1video":
		return "mpeg2video"
	}
	return codec
}

// legacySource builds a source from the flat fields older callers and tests
// pass. Its detail is unobserved apart from height and colour.
func legacySource(in DeliveryInput) DeliverySource {
	s := DeliverySource{ID: in.SourceID, Container: in.Container, Duration: in.Duration}
	if in.VideoCodec != "" {
		v := &SourceVideo{Codec: videoFamily(in.VideoCodec)}
		v.Height = in.Height
		v.DynamicRange = assets.DynamicRangeOf(in.ColorTransfer, 0)
		s.Video = v
	}
	if in.AudioCodec != "" {
		s.Audio = []SourceAudio{{Index: -1, Codec: audioFamily(in.AudioCodec)}}
	}
	return s
}

type sourceQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// loadDeliverySource reads one asset's planning facts: one row for the asset and
// one indexed range read for its streams.
func loadDeliverySource(ctx context.Context, q sourceQuery, asset string) (DeliverySource, error) {
	var s DeliverySource
	var video, audio string
	var size int64
	var width, height, version int
	s.ID = asset
	err := q.QueryRowContext(ctx, `SELECT a.container,a.video_codec,a.audio_codec,a.duration,a.size,COALESCE(a.width,0),COALESCE(a.height,0),COALESCE((SELECT f.detail_version FROM asset_stream_facts f WHERE f.asset_id=a.token AND f.size=a.size AND f.modified_ns=a.modified_ns),0) FROM catalog_assets a WHERE a.token=?`, asset).Scan(&s.Container, &video, &audio, &s.Duration, &size, &width, &height, &version)
	if err != nil {
		return s, err
	}
	s.DetailKnown = version >= assets.StreamDetailVersion
	if s.Duration > 0 && size > 0 {
		s.BitRate = int64(float64(size) * 8 / s.Duration)
	}
	rows, err := q.QueryContext(ctx, `SELECT stream_index,type,codec,language,title,channels,channel_layout,is_default,color_transfer,detail_json FROM asset_streams WHERE asset_id=? AND type IN('video','audio') ORDER BY stream_index LIMIT 129`, asset)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var st assets.Stream
		var transfer, detail string
		if err = rows.Scan(&st.Index, &st.Type, &st.Codec, &st.Language, &st.Title, &st.Channels, &st.ChannelLayout, &st.Default, &transfer, &detail); err != nil {
			return s, err
		}
		st.DecodeDetail(detail)
		switch st.Type {
		case "video":
			if s.Video != nil {
				continue
			}
			v := &SourceVideo{Index: st.Index, Codec: videoFamily(st.Codec), StreamDetail: st.Detail}
			if v.Height == 0 {
				v.Height = height
				v.Width = width
			}
			if v.DynamicRange == "" {
				v.DynamicRange = assets.DynamicRangeOf(transfer, 0)
			}
			s.Video = v
		case "audio":
			s.Audio = append(s.Audio, SourceAudio{Index: st.Index, Ordinal: len(s.Audio), Codec: audioFamily(st.Codec), Channels: st.Channels, Layout: st.ChannelLayout, Language: st.Language, Title: publicAudioText(st.Title, 96), Default: st.Default, Commentary: st.Commentary, StreamDetail: st.Detail})
		}
	}
	if err = rows.Err(); err != nil {
		return s, err
	}
	if s.Video != nil && s.Video.MaxBitRate > 0 {
		s.MaxBitRate = s.Video.MaxBitRate
		for _, audio := range s.Audio {
			s.MaxBitRate += audio.BitRate
		}
	}
	// A file whose stream rows are missing (never probed per stream, or a remote
	// descriptor) still plans from the asset row's first-codec summary.
	if s.Video == nil && video != "" {
		v := &SourceVideo{Codec: videoFamily(video)}
		v.Height = height
		v.Width = width
		v.DynamicRange = assets.RangeSDR
		s.Video = v
	}
	if len(s.Audio) == 0 && audio != "" {
		// Index -1: the stream's position in the file is not known, so the
		// producer takes the first audio track rather than a guessed index.
		s.Audio = []SourceAudio{{Index: -1, Codec: audioFamily(audio)}}
	}
	return s, nil
}
