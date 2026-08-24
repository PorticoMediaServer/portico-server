package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const rcloneInventoryPageLimit = 10000

type managedRclone struct {
	Binary    string
	Config    string
	Remote    string
	Root      string
	Scheduler *remoteStorageScheduler
	command   func(context.Context, string, ...string) *exec.Cmd
}

type managedRcloneMount struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan error
}

// installRcloneConfig atomically installs an isolated config. The caller is
// expected to pass an rclone-encrypted config where supported; filesystem
// permissions remain a second, mandatory boundary.
func installRcloneConfig(directory string, contents []byte) (string, error) {
	if len(contents) == 0 || len(contents) > 4<<20 {
		return "", errors.New("rclone config must be between 1 byte and 4 MiB")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(directory, ".rclone-config-")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(contents); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	destination := filepath.Join(directory, "rclone.conf")
	if err := os.Rename(tmpName, destination); err != nil {
		return "", err
	}
	return destination, nil
}

// Start launches a read-only compatibility mount with a bounded sparse VFS
// cache. Scanning still uses Inventory; this mount exists only for tools that
// require seekable filesystem semantics.
func (m *managedRcloneMount) Start(ctx context.Context, r *managedRclone, mountPoint, cacheDir string, cacheBytes int64) error {
	if r == nil {
		return errors.New("rclone backend is required")
	}
	target, err := r.target("")
	if err != nil {
		return err
	}
	for _, dir := range []string{mountPoint, cacheDir} {
		if strings.TrimSpace(dir) == "" {
			return errors.New("rclone mount and cache paths are required")
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	if cacheBytes < 1<<30 {
		cacheBytes = 1 << 30
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil {
		return errors.New("rclone mount is already running")
	}
	factory := r.command
	if factory == nil {
		factory = exec.CommandContext
	}
	cmd := factory(ctx, r.Binary, "--config", r.Config, "mount", target, mountPoint, "--read-only", "--vfs-cache-mode", "full", "--cache-dir", cacheDir, "--vfs-cache-max-size", strconv.FormatInt(cacheBytes, 10), "--vfs-cache-min-free-space", "1G", "--no-modtime")
	cmd.Env = append(os.Environ(), "RCLONE_ASK_PASSWORD=false")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd = cmd
	m.done = make(chan error, 1)
	go func() { m.done <- cmd.Wait() }()
	return nil
}
func (m *managedRcloneMount) Stop(ctx context.Context) error {
	m.mu.Lock()
	cmd, done := m.cmd, m.done
	m.cmd = nil
	m.done = nil
	m.mu.Unlock()
	if cmd == nil {
		return nil
	}
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return ctx.Err()
	}
}

type rcloneBinaryEvidence struct{ Path, Version, SHA256 string }

var rcloneVersionPattern = regexp.MustCompile(`(?m)^rclone v([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

// validateRcloneBinary accepts an owner-selected executable, but records its
// canonical location and content digest so replacement is detectable.
func validateRcloneBinary(ctx context.Context, binary string) (rcloneBinaryEvidence, error) {
	real, err := filepath.EvalSymlinks(strings.TrimSpace(binary))
	if err != nil {
		return rcloneBinaryEvidence{}, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return rcloneBinaryEvidence{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return rcloneBinaryEvidence{}, errors.New("rclone binary is not an executable regular file")
	}
	f, err := os.Open(real)
	if err != nil {
		return rcloneBinaryEvidence{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return rcloneBinaryEvidence{}, err
	}
	cmd := exec.CommandContext(ctx, real, "version")
	output, err := commandOutputLimit(cmd, 1<<20)
	if err != nil {
		return rcloneBinaryEvidence{}, fmt.Errorf("validate rclone version: %w", err)
	}
	match := rcloneVersionPattern.FindSubmatch(output)
	if len(match) != 2 {
		return rcloneBinaryEvidence{}, errors.New("selected executable did not identify itself as rclone")
	}
	return rcloneBinaryEvidence{Path: real, Version: string(match[1]), SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func verifyFileSHA256(path, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if expected == "" {
		return errors.New("approved rclone digest is missing")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return errors.New("rclone binary changed after owner approval")
	}
	return nil
}

func (r *managedRclone) Kind() string { return "rclone" }
func (r *managedRclone) target(object string) (string, error) {
	object = strings.TrimSpace(object)
	if object != "" {
		var err error
		object, err = normalizeRemoteObjectPath(object)
		if err != nil {
			return "", err
		}
	}
	root, err := normalizeRemoteStorageRoot(r.Root)
	if err != nil {
		return "", err
	}
	joined := strings.Trim(strings.Join([]string{root, object}, "/"), "/")
	remote := strings.TrimSpace(r.Remote)
	if remote == "" || strings.ContainsAny(remote, "/\\:\x00") {
		return "", errors.New("invalid rclone remote name")
	}
	return remote + ":" + joined, nil
}
func (r *managedRclone) run(ctx context.Context, maxOutput int64, args ...string) ([]byte, error) {
	if strings.TrimSpace(r.Config) == "" {
		return nil, errors.New("rclone config path is required")
	}
	if info, err := os.Stat(r.Config); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("rclone config is unavailable")
	}
	factory := r.command
	if factory == nil {
		factory = exec.CommandContext
	}
	// Secrets live in the mode-restricted config, never argv or logs. Only this
	// allowlisted adapter constructs subcommands; no generic RC/command surface.
	cmd := factory(ctx, r.Binary, append([]string{"--config", r.Config}, args...)...)
	cmd.Env = append(os.Environ(), "RCLONE_ASK_PASSWORD=false")
	out, err := commandOutputLimit(cmd, maxOutput)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		return nil, fmt.Errorf("rclone operation failed: %w", err)
	}
	return out, nil
}

func commandOutputLimit(cmd *exec.Cmd, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("command output limit is invalid")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxBytes+1))
	if int64(len(output)) > maxBytes {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return nil, errors.New("rclone command output exceeded its safety limit")
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	return output, waitErr
}

type rcloneListEntry struct {
	Path, Name, ID, MimeType string
	Size                     int64
	ModTime                  time.Time
	IsDir                    bool
	Hashes                   map[string]string
}

type rcloneInventoryCursor struct {
	Queue  []string `json:"queue"`
	Offset int      `json:"offset,omitempty"`
}

func rcloneStorageObject(entry rcloneListEntry, objectPath string) storageObject {
	hash := ""
	for _, value := range entry.Hashes {
		if value != "" {
			hash = value
			break
		}
	}
	revision := entry.ID + "\x00" + strconv.FormatInt(entry.Size, 10) + "\x00" + entry.ModTime.UTC().Format(time.RFC3339Nano) + "\x00" + hash
	return storageObject{Path: objectPath, ObjectID: entry.ID, Revision: revision, Hash: hash, Size: entry.Size, ModTime: entry.ModTime, ContentType: entry.MimeType}
}

func (r *managedRclone) Stat(ctx context.Context, object string) (storageObject, error) {
	objectPath, err := normalizeRemoteObjectPath(object)
	if err != nil {
		return storageObject{}, err
	}
	operationCtx, release, err := r.Scheduler.acquireOperation(ctx, false)
	if err != nil {
		return storageObject{}, err
	}
	defer release()
	target, err := r.target(objectPath)
	if err != nil {
		return storageObject{}, err
	}
	out, err := r.run(operationCtx, 4<<20, "lsjson", target, "--stat", "--metadata", "--hash")
	if err != nil {
		return storageObject{}, err
	}
	var entry rcloneListEntry
	if err := json.Unmarshal(out, &entry); err != nil {
		return storageObject{}, fmt.Errorf("decode rclone object stat: %w", err)
	}
	if entry.IsDir {
		return storageObject{}, errors.New("rclone object is a directory")
	}
	return rcloneStorageObject(entry, objectPath), nil
}

func (r *managedRclone) Inventory(ctx context.Context, cursor string, limit int) (storageInventoryPage, error) {
	if limit <= 0 || limit > rcloneInventoryPageLimit {
		limit = rcloneInventoryPageLimit
	}
	state := rcloneInventoryCursor{Queue: []string{""}}
	if cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &state); err != nil || len(state.Queue) == 0 || state.Offset < 0 {
			return storageInventoryPage{}, fmt.Errorf("%w: invalid rclone cursor", errRemoteInventoryCursorInvalid)
		}
	}
	directory := state.Queue[0]
	operationCtx, release, err := r.Scheduler.acquireOperation(ctx, false)
	if err != nil {
		return storageInventoryPage{}, err
	}
	defer release()
	target, err := r.target(directory)
	if err != nil {
		return storageInventoryPage{}, err
	}
	// Enumerate one directory at a time. The pending-directory queue is the
	// durable cursor, avoiding one enormous recursive JSON response and making
	// interruption resume at a deterministic provider request boundary.
	out, err := r.run(operationCtx, 64<<20, "lsjson", target, "--metadata", "--hash")
	if err != nil {
		return storageInventoryPage{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var entries []rcloneListEntry
	if err := dec.Decode(&entries); err != nil {
		return storageInventoryPage{}, fmt.Errorf("decode rclone inventory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return firstNonEmpty(entries[i].Name, entries[i].Path) < firstNonEmpty(entries[j].Name, entries[j].Path)
	})
	if state.Offset > len(entries) {
		return storageInventoryPage{}, fmt.Errorf("%w: rclone directory changed during resume", errRemoteInventoryCursorInvalid)
	}
	end := min(len(entries), state.Offset+limit)
	page := storageInventoryPage{Objects: make([]storageObject, 0, end-state.Offset)}
	queued := make(map[string]bool, len(state.Queue))
	for _, item := range state.Queue {
		queued[item] = true
	}
	for _, entry := range entries[state.Offset:end] {
		if entry.IsDir {
			child, e := normalizeRemoteObjectPath(pathpkg.Join(directory, firstNonEmpty(entry.Name, entry.Path)))
			if e != nil {
				return storageInventoryPage{}, e
			}
			if !queued[child] {
				state.Queue = append(state.Queue, child)
				queued[child] = true
			}
			continue
		}
		p, e := normalizeRemoteObjectPath(pathpkg.Join(directory, firstNonEmpty(entry.Name, entry.Path)))
		if e != nil {
			return storageInventoryPage{}, e
		}
		page.Objects = append(page.Objects, rcloneStorageObject(entry, p))
	}
	if end < len(entries) {
		state.Offset = end
	} else {
		state.Queue = state.Queue[1:]
		state.Offset = 0
	}
	if len(state.Queue) > 0 {
		encoded, _ := json.Marshal(state)
		page.NextCursor = string(encoded)
	} else {
		page.Authoritative = true
	}
	return page, nil
}
func (r *managedRclone) OpenRange(ctx context.Context, object string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 || length <= 0 {
		return nil, errors.New("invalid rclone read range")
	}
	operationCtx, release, err := r.Scheduler.acquireOperation(ctx, remoteStorageReadIsPlayback(ctx))
	if err != nil {
		return nil, err
	}
	target, err := r.target(object)
	if err != nil {
		release()
		return nil, err
	}
	factory := r.command
	if factory == nil {
		factory = exec.CommandContext
	}
	processCtx, cancelProcess := context.WithCancelCause(operationCtx)
	cmd := factory(processCtx, r.Binary, "--config", r.Config, "cat", target, "--offset", strconv.FormatInt(offset, 10), "--count", strconv.FormatInt(length, 10))
	cmd.Env = append(os.Environ(), "RCLONE_ASK_PASSWORD=false")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		cancelProcess(err)
		release()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		cancelProcess(err)
		release()
		return nil, err
	}
	return &rcloneReadCloser{pipe: pipe, cmd: cmd, expected: length, release: release, ctx: operationCtx, cancelProcess: cancelProcess, closeGrace: rcloneReadCloseGrace}, nil
}

const rcloneReadCloseGrace = 2 * time.Second

type rcloneReadCloser struct {
	pipe          io.ReadCloser
	cmd           *exec.Cmd
	expected      int64
	read          int64
	release       func()
	closeOnce     sync.Once
	closeErr      error
	ctx           context.Context
	cancelProcess context.CancelCauseFunc
	closeGrace    time.Duration
}

func (r *rcloneReadCloser) Read(buffer []byte) (int, error) {
	n, err := r.pipe.Read(buffer)
	r.read += int64(n)
	if err != nil && r.ctx != nil {
		if cause := context.Cause(r.ctx); cause != nil {
			return n, cause
		}
	}
	return n, err
}

func (r *rcloneReadCloser) Close() error {
	r.closeOnce.Do(func() {
		defer r.release()
		if r.cancelProcess != nil {
			defer r.cancelProcess(context.Canceled)
		}
		_ = r.pipe.Close()
		if r.read < r.expected {
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Signal(syscall.SIGTERM)
			}
		}
		waited := make(chan error, 1)
		go func() { waited <- r.cmd.Wait() }()
		grace := r.closeGrace
		if grace <= 0 {
			grace = rcloneReadCloseGrace
		}
		select {
		case r.closeErr = <-waited:
			return
		case <-time.After(grace):
			if r.cancelProcess != nil {
				r.cancelProcess(errors.New("rclone range reader close deadline exceeded"))
			}
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Kill()
			}
			r.closeErr = <-waited
		}
	})
	return r.closeErr
}
