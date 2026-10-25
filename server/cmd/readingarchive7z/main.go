// Command readingarchive7z is the owned resource-bounded CB7 helper. The server
// never parses a 7z archive in its own address space: it starts this program
// with a fixed argument vector, feeds the leased bytes on stdin within a bounded
// spool, and reads a small typed result. The helper refuses encoded headers,
// external streams, oversized headers, members and output. The parent owns the
// process group, deadline, hard address-space/CPU/scratch/output limits and the
// private scratch root (TMPDIR): the helper only writes beneath that root, and a
// SIGKILL cannot clean up on its behalf, so the parent removes it after a
// physical wait.
package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

const (
	maxSource      = 2 << 30
	maxHeaderBytes = 16 << 20
	maxMemberBytes = 32 << 20
	maxMembers     = 4096
	maxNameLength  = 1024
)

type reply struct {
	Error   string   `json:"error,omitempty"`
	Members []string `json:"members,omitempty"`
	Size    int64    `json:"size,omitempty"`
}

func main() {
	code := 0
	if err := run(); err != nil {
		emit(reply{Error: err.Error()})
		code = 2
	}
	os.Exit(code)
}

// run keeps every owned resource unwinding through defers before main exits; a
// fail path returns an error instead of calling os.Exit inside a worker.
func run() error {
	// The isolation this helper provides depends on real resource limits: a
	// platform that cannot set them refuses to parse anything.
	if err := limitProcess(); err != nil {
		return err
	}
	if len(os.Args) < 2 {
		return errors.New("mode required")
	}
	switch os.Args[1] {
	case "list":
		return list()
	case "page":
		if len(os.Args) != 3 {
			return errors.New("member required")
		}
		return page(os.Args[2])
	default:
		return errors.New("unknown mode")
	}
}

// spool copies stdin into a private bounded file; 7z needs random access and the
// helper never receives an original filesystem path.
func spool() (string, int64, error) {
	directory, err := os.MkdirTemp("", "portico-cb7-")
	if err != nil {
		return "", 0, err
	}
	file, err := os.Create(filepath.Join(directory, "archive.7z"))
	if err != nil {
		os.RemoveAll(directory)
		return "", 0, err
	}
	written, err := io.Copy(file, io.LimitReader(os.Stdin, maxSource+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || written > maxSource {
		os.RemoveAll(directory)
		return "", 0, errors.New("archive too large")
	}
	return directory, written, nil
}

func admit(path string, size int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var head [32]byte
	if _, err = file.ReadAt(head[:], 0); err != nil {
		return errors.New("unreadable archive")
	}
	if string(head[:6]) != "7z\xbc\xaf'\x1c" {
		return errors.New("not a 7z archive")
	}
	nextOffset := binary.LittleEndian.Uint64(head[12:20])
	nextSize := binary.LittleEndian.Uint64(head[20:28])
	if size < 32 || nextOffset > uint64(size)-32 || nextSize == 0 || nextSize > maxHeaderBytes || nextSize > uint64(size)-32-nextOffset {
		return errors.New("unbounded header")
	}
	var marker [1]byte
	if _, err = file.ReadAt(marker[:], int64(32)+int64(nextOffset)); err != nil || marker[0] != 0x01 {
		return errors.New("unsupported header")
	}
	return nil
}

func open() (*sevenzip.ReadCloser, string, error) {
	directory, size, err := spool()
	if err != nil {
		return nil, "", err
	}
	archive := filepath.Join(directory, "archive.7z")
	if err = admit(archive, size); err != nil {
		os.RemoveAll(directory)
		return nil, "", err
	}
	reader, err := sevenzip.OpenReader(archive)
	if err != nil {
		os.RemoveAll(directory)
		return nil, "", errors.New("archive refused")
	}
	if len(reader.File) == 0 || len(reader.File) > maxMembers {
		reader.Close()
		os.RemoveAll(directory)
		return nil, "", errors.New("member count out of bounds")
	}
	return reader, directory, nil
}

func list() error {
	reader, directory, err := open()
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	defer reader.Close()
	names := []string{}
	for _, file := range reader.File {
		if !plainMember(file.Name, file.FileInfo().IsDir(), file.UncompressedSize64) {
			continue
		}
		names = append(names, file.Name)
	}
	emit(reply{Members: names})
	return nil
}

func page(name string) error {
	if !plainName(name) {
		return errors.New("invalid member")
	}
	reader, directory, err := open()
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != name || !plainMember(file.Name, file.FileInfo().IsDir(), file.UncompressedSize64) {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return errors.New("member refused")
		}
		defer body.Close()
		out, err := io.ReadAll(io.LimitReader(body, maxMemberBytes+1))
		if err != nil || int64(len(out)) != int64(file.UncompressedSize64) || len(out) > maxMemberBytes {
			return errors.New("member out of bounds")
		}
		if _, err = os.Stdout.Write(out); err != nil {
			return errors.New("output failed")
		}
		return nil
	}
	return errors.New("member not found")
}

func plainName(name string) bool {
	if name == "" || len(name) > maxNameLength || strings.ContainsAny(name, "\\\x00%?#:") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func plainMember(name string, dir bool, size uint64) bool {
	if dir || size == 0 || size > maxMemberBytes || !plainName(name) {
		return false
	}
	lower := strings.ToLower(name)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func emit(value reply) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}
