//go:build !linux && !darwin && !windows

package storage

// RootedDir is unavailable where the platform cannot pin a directory by
// descriptor; callers refuse to browse rather than fall back to a lexical check.
type RootedDir struct{}

func OpenRootedDir(string) (*RootedDir, error)         { return nil, ErrRemoteConfig }
func (d *RootedDir) Child(string) (*RootedDir, error)  { return nil, ErrRemoteConfig }
func (d *RootedDir) Close() error                      { return nil }
func (d *RootedDir) Names(int) ([]string, bool, error) { return nil, false, ErrRemoteConfig }
func (d *RootedDir) IsDirectoryEntry(string) bool      { return false }
