package assets

import (
	"context"
	"portico.local/server/internal/storage"
	"strings"
)

// Remote probes only see a capability-bound loopback byte bridge. Select a
// finite media demuxer explicitly: never permit playlists, external references,
// URL credentials or arbitrary local-file protocols through ffprobe.
func (p Probe) InspectRemote(ctx context.Context, bridge, extension string) (Facts, error) {
	ext := strings.TrimPrefix(strings.ToLower(extension), ".")
	format := ""
	switch ext {
	case "mp4", "m4v", "m4a", "m4b", "mov":
		format = "mov"
	case "mkv", "webm":
		format = "matroska"
	case "mp3", "flac", "wav", "aac", "ogg", "avi":
		format = ext
	case "ts", "m2ts":
		format = "mpegts"
	default:
		return Facts{}, storage.ErrRemoteRange
	}
	options := []string{"-f", format, "-protocol_whitelist", "http,tcp", "-probesize", "8388608", "-analyzeduration", "5000000"}
	if format == "mov" {
		options = append(options, "-enable_drefs", "0", "-use_absolute_path", "0")
	}
	facts, err := p.inspect(ctx, bridge, options)
	facts.Container = ext
	return facts, err
}
