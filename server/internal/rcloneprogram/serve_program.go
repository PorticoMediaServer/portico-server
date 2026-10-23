package rcloneprogram

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrServeProgram = errors.New("approved remote source program changed or is unavailable")

// checkServeProgram preserves the owner-selected NativeHelper executable
// approval at the new serve boundary. A version response alone cannot approve
// a replacement image. Included programs keep their installation-owned trust.
func checkServeProgram(program Program, approved string) error {
	if !filepath.IsAbs(program.Path) || len(approved) != sha256.Size*2 {
		return ErrServeProgram
	}
	if digest, err := hex.DecodeString(approved); err != nil || hex.EncodeToString(digest) != approved {
		return ErrServeProgram
	}
	canonical, err := filepath.EvalSymlinks(program.Path)
	if err != nil || canonical != filepath.Clean(program.Path) {
		return ErrServeProgram
	}
	file, err := os.Open(program.Path)
	if err != nil {
		return ErrServeProgram
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 || info.Size() > 256<<20 {
		return ErrServeProgram
	}
	var head [4]byte
	if _, err = io.ReadFull(file, head[:]); err != nil {
		return ErrServeProgram
	}
	switch hex.EncodeToString(head[:]) {
	case "7f454c46", "cffaedfe", "feedfacf", "cafebabe", "bebafeca":
	default:
		return ErrServeProgram
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return ErrServeProgram
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(file, (256<<20)+1)); err != nil || hex.EncodeToString(hash.Sum(nil)) != approved {
		return ErrServeProgram
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return ErrServeProgram
	}
	current, err := os.Lstat(program.Path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return ErrServeProgram
	}
	return nil
}
