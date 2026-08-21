package app

import (
	"context"
	"net/http"
	"strings"
)

// applyRequestedPlaybackVersion resolves a client-selected source only from the
// media files already indexed for the authorized media item. It deliberately
// accepts an opaque version id rather than a path, so clients cannot influence
// filesystem selection or probe another item's versions.
func (s *Server) applyRequestedPlaybackVersion(ctx context.Context, item *MediaItem, requestedVersionID string) *playbackStartHTTPError {
	requestedVersionID = strings.TrimSpace(requestedVersionID)
	if requestedVersionID == "" {
		return nil
	}
	if item == nil || strings.TrimSpace(item.ID) == "" {
		return &playbackStartHTTPError{status: http.StatusUnprocessableEntity, code: "media_version_unavailable", message: "The requested media version is not available."}
	}

	versions := s.mediaFilesForContext(ctx, item.ID, "")
	selectedPath := ""
	for index := range versions {
		selected := versions[index].ID == requestedVersionID
		versions[index].Selected = selected
		if selected && versions[index].Available && strings.TrimSpace(versions[index].Path) != "" {
			selectedPath = versions[index].Path
		}
	}
	if selectedPath == "" {
		return &playbackStartHTTPError{status: http.StatusUnprocessableEntity, code: "media_version_unavailable", message: "The requested media version is not available."}
	}

	item.SourceURL = selectedPath
	item.MediaFiles = versions
	return nil
}

func selectedPlaybackVersionID(item MediaItem) string {
	for _, version := range item.MediaFiles {
		if version.Selected {
			return strings.TrimSpace(version.ID)
		}
	}
	if len(item.MediaFiles) == 1 {
		return strings.TrimSpace(item.MediaFiles[0].ID)
	}
	return ""
}

// playbackStreamsForSelectedVersion keeps stream selection tied to the file
// that playback will actually open. A media item can expose streams from
// several indexed versions, but ffmpeg stream indexes and audio ordinals are
// only meaningful within one input file. Streams without a file id are kept
// for backwards compatibility with older scans and source-wide sidecars.
func playbackStreamsForSelectedVersion(item MediaItem) []Stream {
	selectedFileID := ""
	for _, version := range item.MediaFiles {
		if version.Selected {
			selectedFileID = strings.TrimSpace(version.ID)
			break
		}
	}
	if selectedFileID == "" && len(item.MediaFiles) == 1 {
		selectedFileID = strings.TrimSpace(item.MediaFiles[0].ID)
	}
	if selectedFileID == "" {
		return item.Streams
	}

	streams := make([]Stream, 0, len(item.Streams))
	for _, stream := range item.Streams {
		fileID := strings.TrimSpace(stream.FileID)
		if fileID == "" || fileID == selectedFileID {
			streams = append(streams, stream)
		}
	}
	return streams
}

func scopePlaybackStreamsToSelectedVersion(item *MediaItem) {
	if item == nil {
		return
	}
	item.Streams = playbackStreamsForSelectedVersion(*item)
}
