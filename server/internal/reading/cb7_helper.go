package reading

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"portico.local/server/internal/archive7z"
)

// CB7 decoding runs in the owned bounded helper, never in this process. The
// parent copies the leased bytes into its own private scratch root (bounded by
// MaxSource, read through the source lease; nothing is uploaded or fetched from
// a provider beyond that lease), gives the helper that file on stdin, owns the
// process group and deadline, closes the file and removes the scratch root only
// after the physical wait. The page mode's argv carries one already-validated
// member name — the archive's own path never does.
const (
	cb7ListMaxBytes  = 1 << 20
	cb7PageMaxBytes  = MaxMember
	cb7HelperTimeout = 90 * time.Second
)

type cb7ListReply struct {
	Error   string `json:"error"`
	Members []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"members"`
}

// cb7HelperPath is this same server executable: the worker is a hidden mode of
// the one installed binary, never a second file, PATH lookup or override.
func cb7HelperPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", ErrArchiveFormat
	}
	return executable, nil
}

// runCB7Helper starts the helper with a fixed argv, its own scratch root and a
// deadline, feeds the archive from the leased reader, and returns the bounded
// stdout. No filename from the archive or client reaches the command line.
func runCB7Helper(ctx context.Context, ra io.ReaderAt, size int64, args ...string) ([]byte, error) {
	if size <= 0 || size > MaxSource {
		return nil, ErrLimit
	}
	if !helperPlatformSupported() {
		// Windows cannot enforce the worker's resource limits; refuse before any
		// provider read or scratch creation rather than starting unsupported.
		return nil, ErrArchiveFormat
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
	// The archive is staged into the parent-owned scratch root under the same
	// deadline first: exec's stdin copy cannot hang on a blocked Source.ReadAt,
	// because the helper reads a local, fully written file.
	staged, err := stageCB7Archive(deadline, ra, size, scratch)
	if err != nil {
		_ = os.RemoveAll(scratch)
		return nil, err
	}
	input, err := os.Open(staged)
	if err != nil {
		_ = os.RemoveAll(scratch)
		return nil, ErrLimit
	}
	command := exec.CommandContext(deadline, path, append([]string{archive7z.Mode}, args...)...)
	command.Env = []string{"TMPDIR=" + scratch, "HOME=" + scratch, "PATH="}
	command.Stdin = input
	stdout, pipeErr := command.StdoutPipe()
	if pipeErr != nil {
		_ = input.Close()
		if cleanupErr := os.RemoveAll(scratch); cleanupErr != nil {
			return nil, errors.Join(ErrLimit, cleanupErr)
		}
		return nil, ErrArchiveFormat
	}
	command.Cancel = killHelper
	command.WaitDelay = 5 * time.Second
	if err = startHelper(command); err != nil {
		_ = input.Close()
		if cleanupErr := os.RemoveAll(scratch); cleanupErr != nil {
			return nil, errors.Join(ErrLimit, cleanupErr)
		}
		return nil, ErrArchiveFormat
	}
	limit := int64(cb7ListMaxBytes)
	if len(args) > 0 && args[0] == "page" {
		limit = cb7PageMaxBytes
	}
	body, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	waitErr := command.Wait()
	// Owned input closes after the physical wait and before removal, on every
	// path: an open file would keep the scratch root undeletable on Windows.
	closeErr := input.Close()
	removeErr := os.RemoveAll(scratch)
	if cleanupErr := errors.Join(closeErr, removeErr); cleanupErr != nil {
		return nil, errors.Join(ErrLimit, cleanupErr)
	}
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

// cb7Members lists a CB7's admitted image members through the helper.
func cb7Members(ctx context.Context, ra io.ReaderAt, size int64) ([]archiveMember, error) {
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
	out := make([]archiveMember, 0, len(reply.Members))
	seen := map[string]bool{}
	var expanded int64
	for _, member := range reply.Members {
		if !safePath(member.Name) || !isImageMember(member.Name) || member.Size <= 0 || member.Size > MaxMember {
			return nil, ErrArchiveFormat
		}
		// The parent repeats the identity and whole-publication budget checks:
		// a duplicate name is not two pages, and admitted members must fit
		// MaxExpanded together.
		if seen[member.Name] || int64(MaxExpanded)-expanded < member.Size {
			return nil, ErrArchiveFormat
		}
		seen[member.Name] = true
		expanded += member.Size
		name, memberSize := member.Name, member.Size
		out = append(out, archiveMember{Name: name, Size: memberSize, Open: func() (io.ReadCloser, error) {
			body, err := cb7MemberBytes(ctx, ra, size, name)
			if err != nil {
				return nil, err
			}
			return io.NopCloser(bytes.NewReader(body)), nil
		}})
	}
	return out, nil
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

// stageCB7Archive copies the leased bytes into the parent-owned scratch root
// through the source's context-aware extent seam, carrying both the captured
// retirement and the child deadline into every read. There is no goroutine and
// no bounded wait substituting for cancellation: the copy either finishes,
// including its close and sync, or the seam returns the cancellation itself.
func stageCB7Archive(ctx context.Context, ra io.ReaderAt, size int64, scratch string) (string, error) {
	reader, ok := ra.(interface {
		ReadAtContext(context.Context, []byte, int64) (int, error)
	})
	if !ok {
		return "", ErrArchiveFormat
	}
	path := filepath.Join(scratch, "archive.7z")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", ErrLimit
	}
	const chunk = 1 << 20
	buffer := make([]byte, chunk)
	written := int64(0)
	for written < size {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return "", err
		}
		want := int64(chunk)
		if size-written < want {
			want = size - written
		}
		read, readErr := reader.ReadAtContext(ctx, buffer[:want], written)
		if read > 0 {
			if _, writeErr := file.Write(buffer[:read]); writeErr != nil {
				_ = file.Close()
				return "", ErrLimit
			}
			written += int64(read)
		}
		if readErr != nil && readErr != io.EOF {
			_ = file.Close()
			return "", ErrArchiveFormat
		}
		if read == 0 {
			_ = file.Close()
			return "", ErrArchiveFormat
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", ErrLimit
	}
	if err := file.Close(); err != nil {
		return "", ErrLimit
	}
	return path, nil
}
