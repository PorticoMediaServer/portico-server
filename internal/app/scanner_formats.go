package app

import (
	"os"
	"path/filepath"
	"strings"
)

const logicalDiscUnsupportedReason = "Catalogued disc source. Playback requires a compatible sandboxed disc reader configured on this server. Menus, encrypted discs, and OS auto-mounting are not supported."

// logicalDiscSourceForPath recognizes disc media without mounting images or
// claiming that folder/file presence implies readable, decrypted playback.
func logicalDiscSourceForPath(library Library, root, path string) (scannerMediaFile, bool) {
	if library.Type != "movie" {
		return scannerMediaFile{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return scannerMediaFile{}, false
	}
	kind := ""
	titlePath := path
	container := ""
	if info.IsDir() {
		switch strings.ToUpper(filepath.Base(path)) {
		case "VIDEO_TS":
			kind, container, titlePath = "dvd-structure", "dvd", filepath.Dir(path)
		case "BDMV":
			kind, container, titlePath = "bluray-structure", "bluray", filepath.Dir(path)
		default:
			return scannerMediaFile{}, false
		}
	} else if info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(path), ".iso") {
		kind, container = "disc-image", "iso"
	} else {
		return scannerMediaFile{}, false
	}

	base := strings.TrimSuffix(filepath.Base(titlePath), filepath.Ext(titlePath))
	title := cleanMediaTitle(base)
	year := movieYearFromName(base)
	local := scannerLocalMetadata{Year: year, Source: "scanner:logical-disc"}
	id := scannedID("scan_movie", movieScannerIdentity(library.ID, title, year))
	version := parseMediaVersionInfo(titlePath)
	version.Source = logicalDiscUnsupportedReason
	version.Label = strings.ToUpper(container) + " (catalogued; playback unavailable)"
	return scannerMediaFile{
		ID: id, FileID: scannerFileID(id, path), LibraryID: library.ID,
		Type: "movie", Title: title, SortTitle: sortableTitle(title),
		SourcePath: path, DisplayPath: path, SourceType: kind,
		FileSize: fileSize(info), FileModTime: fileModTime(info),
		Version: version, ArtSeed: container, LocalMetadata: local,
	}, true
}
