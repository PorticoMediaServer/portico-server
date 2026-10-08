package storage

import (
	"os"
	"path/filepath"
	"portico.local/server/internal/artifactlease"
	"regexp"
	"strings"
)

var recordingObjectID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (c *Client) managedRecording(path string) bool {
	if c.RecordingRoot == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	relative, e := filepath.Rel(c.RecordingRoot, path)
	if e != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	return (len(parts) == 3 && recordingObjectID.MatchString(parts[0]) && parts[1] == "objects" && recordingObjectID.MatchString(parts[2])) ||
		(len(parts) == 5 && recordingObjectID.MatchString(parts[0]) && parts[1] == "captures" && recordingObjectID.MatchString(parts[2]) && parts[3] == "objects" && recordingObjectID.MatchString(parts[4]))
}
func lockRecordingFile(req request, f *os.File) error {
	if !req.ManagedArtifact {
		return nil
	}
	if e := artifactlease.Shared(f); e != nil {
		return ErrPlaybackSource
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0222 != 0 {
		return ErrPlaybackSource
	}
	path := req.Path
	if req.ObservedRelativePath != "" {
		path = filepath.Join(path, req.ObservedRelativePath)
	}
	current, e := os.Lstat(path)
	if e != nil || !os.SameFile(info, current) {
		return ErrPlaybackSource
	}
	return nil
}
