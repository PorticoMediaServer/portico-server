package playback

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrHLSArtifactUnavailable = errors.New("stream artifact unavailable")

// OpenHLSFileContext coordinates production, then opens the authorized artifact
// beneath the pinned output directory. The caller owns the handle through HTTP.
func (s *Service) OpenHLSFileContext(ctx context.Context, grant, name string) (*os.File, error) {
	path, err := s.HLSFileContext(ctx, grant, name)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(s.hls.root, path)
	if err != nil || !filepath.IsLocal(relative) || filepath.Base(relative) != name {
		return nil, ErrHLSArtifactUnavailable
	}
	file, err := openHLSOutput(s.hls.outputRoot, filepath.Dir(relative), name)
	if err != nil {
		return nil, ErrHLSArtifactUnavailable
	}
	return file, nil
}

func openHLSOutput(root *os.Root, session, name string) (*os.File, error) {
	if root == nil || session == "" || session == "." || session == ".." || strings.ContainsAny(session, `/\`) || !artifactName.MatchString(name) {
		return nil, os.ErrInvalid
	}
	file, err := root.OpenFile(filepath.Join(session, name), hlsReadFlags, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, os.ErrInvalid
	}
	return file, nil
}
