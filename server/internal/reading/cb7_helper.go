package reading

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CB7 decoding runs in the owned bounded helper, never in this process. The
// parent streams the leased bytes to the helper's stdin (bounded by MaxSource),
// gives it a private scratch root through TMPDIR, owns the process group and
// deadline, and removes the scratch root only after the physical wait.
const (
	cb7HelperName    = "readingarchive7z"
	cb7ListMaxBytes  = 1 << 20
	cb7PageMaxBytes  = MaxMember
	cb7HelperTimeout = 90 * time.Second
)

type cb7ListReply struct {
	Error   string   `json:"error"`
	Members []string `json:"members"`
}

func cb7HelperPath() (string, error) {
	if override := os.Getenv("PORTICO_ARCHIVE7Z"); override != "" {
		if filepath.IsAbs(override) {
			return override, nil
		}
		return "", ErrArchiveFormat
	}
	executable, err := os.Executable()
	if err != nil {
		return "", ErrArchiveFormat
	}
	return filepath.Join(filepath.Dir(executable), cb7HelperName), nil
}

// runCB7Helper starts the helper with a fixed argv, its own scratch root and a
// deadline, feeds the archive from the leased reader, and returns the bounded
// stdout. No filename from the archive or client reaches the command line.
func runCB7Helper(ctx context.Context, ra io.ReaderAt, size int64, args ...string) ([]byte, error) {
	if size <= 0 || size > MaxSource {
		return nil, ErrLimit
	}
	path, err := cb7HelperPath()
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, ErrArchiveFormat
	}
	scratch, err := os.MkdirTemp("", "portico-cb7-parent-")
	if err != nil {
		return nil, ErrLimit
	}
	deadline, cancel := context.WithTimeout(ctx, cb7HelperTimeout)
	defer cancel()
	command := exec.CommandContext(deadline, path, args...)
	command.Env = []string{"TMPDIR=" + scratch, "HOME=" + scratch, "PATH="}
	command.Stdin = io.NewSectionReader(ra, 0, size)
	stdout, pipeErr := command.StdoutPipe()
	if pipeErr != nil {
		_ = os.RemoveAll(scratch)
		return nil, ErrArchiveFormat
	}
	command.Cancel = killHelper
	command.WaitDelay = 5 * time.Second
	if err = startHelper(command); err != nil {
		_ = os.RemoveAll(scratch)
		return nil, ErrArchiveFormat
	}
	limit := int64(cb7ListMaxBytes)
	if len(args) > 0 && args[0] == "page" {
		limit = cb7PageMaxBytes
	}
	body, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	waitErr := command.Wait()
	// The helper writes only beneath the parent-owned scratch root; remove it
	// after the physical wait, never before.
	_ = os.RemoveAll(scratch)
	if readErr != nil || int64(len(body)) > limit {
		return nil, ErrLimit
	}
	if waitErr != nil {
		if deadline.Err() != nil {
			return nil, deadline.Err()
		}
		return nil, ErrArchiveFormat
	}
	return body, nil
}

// cb7MemberNames lists a CB7's admitted image members through the helper.
func cb7MemberNames(ctx context.Context, ra io.ReaderAt, size int64) ([]string, error) {
	body, err := runCB7Helper(ctx, ra, size, "list")
	if err != nil {
		return nil, err
	}
	var reply cb7ListReply
	if err = json.Unmarshal(body, &reply); err != nil {
		return nil, ErrArchiveFormat
	}
	if reply.Error != "" {
		return nil, errors.Join(ErrArchiveFormat, errors.New(reply.Error))
	}
	if len(reply.Members) == 0 || len(reply.Members) > MaxEntries {
		return nil, ErrArchiveFormat
	}
	for _, name := range reply.Members {
		if !safePath(name) || !isImageMember(name) {
			return nil, ErrArchiveFormat
		}
	}
	return reply.Members, nil
}

// cb7MemberBytes reads one page through the helper under the same deadline.
func cb7MemberBytes(ctx context.Context, ra io.ReaderAt, size int64, name string) ([]byte, error) {
	if !safePath(name) || !isImageMember(name) {
		return nil, ErrInvalid
	}
	body, err := runCB7Helper(ctx, ra, size, "page", name)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > MaxMember {
		return nil, ErrLimit
	}
	if _, err = inspectImage(body); err != nil {
		return nil, ErrInvalid
	}
	return body, nil
}
