//go:build darwin

package administration

import "os"

func platformName() string { return "darwin" }

// FilesystemRoots publishes the boot volume, the mounted-volume directory and
// the signed-in account's home folder.
func FilesystemRoots() []FilesystemRoot {
	out := []FilesystemRoot{{Path: "/", Name: "Macintosh HD", Kind: "root", Description: "The boot volume"}}
	if info, err := os.Stat("/Volumes"); err == nil && info.IsDir() {
		out = append(out, FilesystemRoot{Path: "/Volumes", Name: "Volumes", Kind: "volumes", Description: "External and network volumes"})
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, FilesystemRoot{Path: home, Name: "Home", Kind: "home", Description: "This account's home folder"})
	}
	return out
}
