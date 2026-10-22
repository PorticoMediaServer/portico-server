//go:build darwin

package administration

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func platformName() string { return "darwin" }

// FilesystemRoots publishes the boot volume, the mounted-volume directory and
// the signed-in account's home folder.
func FilesystemRoots() []FilesystemRoot {
	out := []FilesystemRoot{{Path: "/", Name: "Macintosh HD", Kind: "root", Description: "The boot volume"}}
	if info, err := os.Stat("/Volumes"); err == nil && info.IsDir() {
		out = append(out, FilesystemRoot{Path: "/Volumes", Name: "Volumes", Kind: "volumes", Description: "External and network volumes"})
		if entries, e := os.ReadDir("/Volumes"); e == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					out = append(out, FilesystemRoot{Path: filepath.Join("/Volumes", entry.Name()), Name: entry.Name(), Kind: "volume", Description: "Mounted volume"})
				}
				if len(out) >= 63 {
					break
				}
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, FilesystemRoot{Path: home, Name: "Home", Kind: "home", Description: "This account's home folder"})
	}
	return out
}

func platformHidden(string) bool          { return false }
func filesystemWritable(path string) bool { return unix.Access(path, unix.W_OK|unix.X_OK) == nil }
