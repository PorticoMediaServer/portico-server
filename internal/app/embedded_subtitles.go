package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// playbackSubtitleStreams exposes browser-playable text tracks without
// pretending that extraction has changed the media's scanned metadata. The
// route materializes a local WebVTT cache lazily; image-based subtitles remain
// server burn-in selections.
func playbackSubtitleStreams(mediaID string, streams []Stream) []Stream {
	prepared := append([]Stream(nil), streams...)
	for index := range prepared {
		stream := &prepared[index]
		if stream.Kind != "subtitle" || stream.ID == "sub_none" || strings.TrimSpace(stream.SourceURL) != "" || isImageSubtitleCodec(stream.Codec) {
			continue
		}
		if _, ok := embeddedSubtitleStreamIndex(*stream); !ok {
			continue
		}
		stream.SourceURL = "/api/media/" + url.PathEscape(mediaID) + "/subtitles/" + url.PathEscape(stream.ID)
	}
	return prepared
}

func (s *Server) ensureEmbeddedTextSubtitleWebVTT(ctx context.Context, mediaID, streamID string) (string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	streamID = s.resolveMediaStreamIDContext(ctx, mediaID, streamID)
	var item MediaItem
	var stream Stream
	if err := s.queryUserRow(ctx, `
		SELECT m.id, COALESCE(m.library_id, ''), COALESCE(m.type, ''), COALESCE(m.source_url, ''),
		       COALESCE(ms.codec, ''), COALESCE(ms.source_url, ''), COALESCE(ms.source_kind, 'unknown'), COALESCE(ms.stream_index, -1), COALESCE(NULLIF(ms.storage_key, ''), ms.id)
		FROM media_items m
		JOIN media_streams ms ON ms.media_id = m.id
		WHERE m.id = ? AND ms.id = ? AND ms.kind = 'subtitle'
		LIMIT 1`, mediaID, streamID).Scan(&item.ID, &item.LibraryID, &item.Type, &item.SourceURL, &stream.Codec, &stream.SourceURL, &stream.SourceKind, &stream.Index, &stream.StorageKey); err != nil {
		return "", 0, err
	}
	stream.ID = streamID
	if strings.TrimSpace(stream.SourceURL) != "" {
		return s.subtitleStreamPathAndOffset(mediaID, streamID)
	}
	if isImageSubtitleCodec(stream.Codec) {
		return "", 0, errors.New("image subtitles require burn-in")
	}
	streamIndex, ok := embeddedSubtitleStreamIndex(stream)
	if !ok {
		return "", 0, errors.New("subtitle is not an embedded text stream")
	}
	sourcePath, err := s.localSourcePathForTranscode(item)
	if err != nil {
		return "", 0, err
	}
	outputRoot := filepath.Join(s.cfg.AppDataDir, "subtitles")
	outputDir := filepath.Join(outputRoot, safePathComponent(mediaID))
	outputPath := filepath.Join(outputDir, safePathComponent(stream.StorageKey)+".vtt")
	if !pathInsideRoot(outputPath, outputRoot) {
		return "", 0, errors.New("subtitle path escaped app data")
	}
	if outputInfo, statErr := os.Stat(outputPath); statErr == nil && !outputInfo.IsDir() {
		if sourceInfo, sourceErr := os.Stat(sourcePath); sourceErr != nil || !outputInfo.ModTime().Before(sourceInfo.ModTime()) {
			return outputPath, 0, nil
		}
	}
	if _, err := exec.LookPath(s.cfg.FFmpegPath); err != nil && filepath.Base(s.cfg.FFmpegPath) == s.cfg.FFmpegPath {
		return "", 0, errors.New("FFmpeg is not available on PATH")
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return "", 0, err
	}
	tempPath := outputPath + "." + safePathComponent(randomID("extract")) + ".tmp.vtt"
	defer os.Remove(tempPath)
	extractCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(extractCtx, s.cfg.FFmpegPath,
		"-hide_banner", "-nostdin", "-y", "-threads", "1",
		"-protocol_whitelist", "file,pipe", "-i", sourcePath,
		"-map", fmt.Sprintf("0:%d", streamIndex), "-c:s", "webvtt", "-f", "webvtt", tempPath,
	)
	if output, err := managedCommandCombinedOutput(extractCtx, cmd); err != nil {
		return "", 0, fmt.Errorf("extract embedded subtitle: %w: %s", err, strings.TrimSpace(string(output)))
	}
	body, err := readBoundedRegularFile(tempPath, playbackSubtitleFileLimit)
	if err != nil {
		return "", 0, err
	}
	if err := publishPrivateArtifact(outputRoot, outputPath, body); err != nil {
		// Another request may have completed the same deterministic cache first.
		if _, statErr := os.Stat(outputPath); statErr != nil {
			return "", 0, err
		}
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return "", 0, err
	}
	return outputPath, 0, nil
}
