package app

import "strings"

const logicalDiscPlaybackUnsupportedReason = "Main-title playback is unavailable because this server has not verified a compatible libdvdread/libbluray and FFmpeg path for this unencrypted disc source. Disc menus, encrypted discs, and OS auto-mounting are not supported."

// logicalDiscPlaybackSourceType returns the W3 catalog kind only for the
// selected logical source. Ordinary files, including VOB, FLV, and F4V, must
// continue through the normal planner and executor.
func logicalDiscPlaybackSourceType(item MediaItem) string {
	selectedID := selectedPlaybackVersionID(item)
	for _, version := range item.MediaFiles {
		if selectedID != "" && strings.TrimSpace(version.ID) != selectedID {
			continue
		}
		switch kind := strings.ToLower(strings.TrimSpace(version.SourceType)); kind {
		case "dvd-structure", "bluray-structure", "disc-image":
			return kind
		}
	}
	return ""
}

// resolveLogicalDiscPlaybackSource is the execution-boundary adapter for W3
// logical disc records. A directory or ISO is never passed to generic probing
// or FFmpeg as though it were a normal media file. Enabling the positive path
// requires one resolver that verifies the exact disc reader + FFmpeg build,
// proves that the structure is accessible and unencrypted under a W3 storage
// lease, and returns the inspected main-title input. Until all of those facts
// exist, the only truthful result is the stable unsupported error below.
func resolveLogicalDiscPlaybackSource(item MediaItem) error {
	if logicalDiscPlaybackSourceType(item) == "" {
		return nil
	}
	return errLogicalDiscPlaybackUnsupported
}
