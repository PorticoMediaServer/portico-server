//go:build windows

package administration

import "os"

func platformName() string { return "windows" }

// FilesystemRoots publishes the drive letters that are actually present plus the
// user profile, which is where a single-machine install usually keeps media. A
// UNC path is typed rather than browsed: there is no portable way to enumerate
// network neighbours, and the picker accepts `\\server\share` directly.
func FilesystemRoots() []FilesystemRoot {
	out := []FilesystemRoot{}
	for letter := 'A'; letter <= 'Z'; letter++ {
		path := string(letter) + `:\`
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			out = append(out, FilesystemRoot{Path: path, Name: string(letter) + ":", Kind: "drive", Description: "Drive " + string(letter) + ":"})
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, FilesystemRoot{Path: home, Name: "Home", Kind: "home", Description: "This account's profile folder"})
	}
	out = append(out, FilesystemRoot{Path: `\\`, Name: "Network share", Kind: "unc", Description: `Type a UNC path such as \\server\share`})
	return out
}
