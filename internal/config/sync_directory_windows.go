//go:build windows

package config

// Windows does not expose a portable directory fsync through os.File. The
// temporary file is flushed before the replace; the platform rename is the
// publication boundary available to the process.
func syncRuntimePathsDirectory(string) error { return nil }
