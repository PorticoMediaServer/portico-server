package storagepaths

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Mount struct{ Point, Filesystem string }

// ParseMountInfo uses kernel mountinfo, including escaped mountpoints and
// optional fields. A directory's existence is not evidence it is a volume.
func ParseMountInfo(r io.Reader) []Mount {
	var out []Mount
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " - ", 2)
		if len(parts) != 2 {
			continue
		}
		a, b := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(a) < 6 || len(b) < 1 {
			continue
		}
		point := a[4]
		for _, escape := range []string{`\040`, `\011`, `\012`, `\134`} {
			v, _ := strconv.ParseInt(escape[1:], 8, 8)
			point = strings.ReplaceAll(point, escape, string(rune(v)))
		}
		out = append(out, Mount{point, b[0]})
	}
	if scanner.Err() != nil {
		return nil // An incomplete mount table cannot authorize cleanup.
	}
	return out
}
func IsMount(path string, mounts []Mount) bool {
	for _, m := range mounts {
		if filepath.Clean(m.Point) == filepath.Clean(path) {
			return true
		}
	}
	return false
}
func Filesystem(path string, mounts []Mount) string {
	best, kind := "", ""
	for _, m := range mounts {
		rel, e := filepath.Rel(m.Point, path)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && len(m.Point) > len(best) {
			best, kind = m.Point, m.Filesystem
		}
	}
	return kind
}
func SharedFilesystem(kind string) bool {
	switch kind {
	case "fuse.shfs", "nfs", "nfs4", "cifs", "smb3":
		return true
	}
	return false
}

func MemoryFilesystem(kind string) bool { return kind == "tmpfs" || kind == "ramfs" }

// CurrentMounts returns a complete platform mount/volume list or an error.
func CurrentMounts() ([]Mount, error) {
	mounts, err := cleanupMounts()
	if err != nil {
		return nil, err
	}
	if len(mounts) == 0 {
		return nil, ErrCleanupUnavailable
	}
	return mounts, nil
}

// DiskTranscodeDirectory keeps production FFmpeg output off RAM until the
// producer/serve path supports aggregate bounded RAM with live disk spill.
// The SpillFile primitive alone does not provide that integration.
func DiskTranscodeDirectory(requested, fallback string, mounts []Mount) (string, error) {
	kind := Filesystem(requested, mounts)
	if kind == "" {
		return "", errors.New("conversion filesystem could not be verified; conversion is unavailable until its disk backing can be identified")
	}
	if !MemoryFilesystem(kind) {
		return requested, nil
	}
	if fallback != "" && Filesystem(fallback, mounts) != "" && !MemoryFilesystem(Filesystem(fallback, mounts)) {
		return fallback, nil
	}
	return "", errors.New("RAM-backed transcode storage is unavailable until bounded producer spill is integrated; choose a disk directory")
}

var ErrCleanupUnavailable = errors.New("destructive cleanup cannot be confined to the owned filesystem")

func refuseMountedChildren(directory string) error {
	mounts, err := cleanupMounts()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCleanupUnavailable, err)
	}
	return refuseMountsWithin(directory, mounts)
}

func refuseMountsWithin(directory string, mounts []Mount) error {
	if len(mounts) == 0 {
		return ErrCleanupUnavailable
	}
	for _, mount := range mounts {
		rel, err := filepath.Rel(directory, mount.Point)
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return fmt.Errorf("%w: mounted filesystem at %s", ErrCleanupUnavailable, mount.Point)
		}
	}
	return nil
}
