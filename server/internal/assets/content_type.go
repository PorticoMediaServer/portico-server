package assets

// OriginalContentType names an original file's media type from its container.
// Every response says nosniff, so the label is all a player that trusts it has.
// QuickTime keeps the MP4 label it has always been served with: it is the same
// box structure, and a browser that plays it refuses the QuickTime type.
func OriginalContentType(container, video string) string {
	if video == "" {
		return audioContentType(container)
	}
	switch container {
	case "mp4", "m4v", "mov":
		return "video/mp4"
	case "mkv":
		return "video/x-matroska"
	case "webm":
		return "video/webm"
	case "avi":
		return "video/x-msvideo"
	case "ts", "m2ts", "mts":
		return "video/mp2t"
	case "mpg", "mpeg", "vob":
		return "video/mpeg"
	}
	return "application/octet-stream"
}

// audioContentType names an audio container's media type.
func audioContentType(container string) string {
	switch container {
	case "mp3":
		return "audio/mpeg"
	case "aac", "adts":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "ogg", "opus":
		return "audio/ogg"
	case "wav":
		return "audio/wav"
	case "aiff":
		return "audio/aiff"
	}
	return "audio/mp4"
}
