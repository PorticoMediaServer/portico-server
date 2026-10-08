package assets

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"portico.local/server/internal/storage"
)

// PlaybackInput is a validated source handed to a converter.
//
// The two platforms reach the same guarantee — the converter reads the exact
// file the session was created against, and nothing can substitute another one
// underneath it — by opposite means, so the difference has to be carried rather
// than assumed. On Unix a helper transfers an open descriptor and the child
// inherits it as file descriptor 3. On Windows the server holds the file open
// with writers and deleters locked out and the child opens the same path by
// name. Callers therefore ask what to put after `-i` and whether to inherit,
// instead of writing "/dev/fd/3" and hoping.
type PlaybackInput struct {
	// File must stay open until the child exits. On Unix it is the inherited
	// descriptor; on Windows it is the handle whose share mode is the fence.
	File *os.File
	// Argument is what follows `-i`.
	Argument string
	// Inherit says the file belongs in the child's ExtraFiles.
	Inherit bool
}

// ExtraFiles is the slice to assign to exec.Cmd.ExtraFiles: the descriptor on
// Unix, nothing on Windows, where ExtraFiles is not supported at all.
func (p *PlaybackInput) ExtraFiles() []*os.File {
	if p == nil || !p.Inherit || p.File == nil {
		return nil
	}
	return []*os.File{p.File}
}

// ReadPaths is what a sandboxed converter must be allowed to read by name: the
// source path where it is not handed over as a descriptor (Windows, and tests
// that pass a plain path).
func (p *PlaybackInput) ReadPaths() []string {
	if p == nil || p.Inherit || !filepath.IsAbs(p.Argument) || filepath.Clean(p.Argument) != p.Argument {
		return nil
	}
	return []string{p.Argument}
}

// Close releases the source. It is safe on a nil input, so a caller can defer it
// before checking the error.
func (p *PlaybackInput) Close() error {
	if p == nil || p.File == nil {
		return nil
	}
	return p.File.Close()
}

// OpenPlaybackInput resolves a session's pinned source and opens it the way this
// platform can hand it to a converter.
func OpenPlaybackInput(ctx context.Context, db *sql.DB, c *storage.Client, session string) (*PlaybackInput, error) {
	v, err := ResolvePlaybackSource(ctx, db, session, "")
	if err != nil {
		return nil, err
	}
	file, argument, inherit, err := c.OpenPlaybackSource(ctx, v.Path, v.Size, v.ModifiedNS)
	if err != nil {
		return nil, err
	}
	return &PlaybackInput{File: file, Argument: argument, Inherit: inherit}, nil
}
