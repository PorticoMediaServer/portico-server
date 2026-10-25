package administration

import (
	"path/filepath"
	"portico.local/server/internal/storagepaths"
)

// RecordingDestination uses the existing owner picker scope. It never admits a
// library URL, managed mount locator, symlink, system or server-data directory.
func (s *Service) RecordingDestination(path string) (string, error) {
	if !safeText(path, 4096) || !filepath.IsAbs(path) || filepath.Clean(path) != path { return "", ErrInput }
	if systemPath(path) || s.pickerPolicy().CheckBrowse(path) != nil { return "", ErrDenied }
	if !filesystemWritable(path) { return "", ErrDenied }
	if err := storagepaths.At(s.StateDirectory()).ValidateRecordings(path); err != nil { return "", ErrDenied }
	return storagepaths.EmptyRecordingRootIdentity(path)
}
