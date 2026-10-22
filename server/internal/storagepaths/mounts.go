package storagepaths

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func refuseMountedChildren(directory string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer file.Close()
	for _, mount := range ParseMountInfo(file) {
		rel, err := filepath.Rel(directory, mount.Point)
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return fmt.Errorf("cannot empty a directory containing a mounted filesystem: %s", mount.Point)
		}
	}
	return nil
}
