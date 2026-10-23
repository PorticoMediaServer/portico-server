package playback

// The scanner stores FFmpeg's demuxer identity. Capability declarations may
// name a conventional container alias. WebM deliberately remains distinct:
// the matroska demuxer also reads Matroska files that a WebM-only player cannot.
func demuxerContainer(s string) string {
	switch s {
	case "mkv", "mka", "matroska":
		return "matroska"
	case "mp4", "m4v", "m4a", "m4b", "3gp", "3g2", "mov":
		return "mov"
	case "ts", "m2ts", "mts", "mpegts":
		return "mpegts"
	case "mpg", "vob", "mpeg":
		return "mpeg"
	case "wmv", "asf":
		return "asf"
	case "ogv", "ogm", "oga", "opus", "ogg":
		return "ogg"
	}
	return s
}
func transportContainsContainer(t ClientTransport, container string) bool {
	for _, c := range t.Containers {
		if c == container || demuxerContainer(c) == demuxerContainer(container) {
			return true
		}
	}
	return false
}
