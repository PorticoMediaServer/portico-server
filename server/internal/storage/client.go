package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/mediasource"
	"time"
)

type Snapshot struct {
	// Opaque adapter evidence; never a provider locator.
	Revision       string `json:"revision,omitempty"`
	ObjectID       string `json:"objectId,omitempty"`
	ObjectIdentity string `json:"objectIdentity,omitempty"`
	ChangeToken    string `json:"changeToken,omitempty"`
	Path           string `json:"path"`
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	ModifiedNS     int64  `json:"modifiedNs"`
	Directory      bool   `json:"directory"`
	Data           string `json:"data,omitempty"`
}
type request struct {
	ManagedArtifact bool `json:"managedArtifact,omitempty"`
	// Argv is a media tool invocation built by mediaexec in the server, reading
	// its input from descriptor 3 (inventory-command).
	Argv                   []string              `json:"argv,omitempty"`
	Inventory              *InventoryRequest     `json:"inventory,omitempty"`
	LyricsRoot             string                `json:"lyricsRoot,omitempty"`
	LyricsLanguage         string                `json:"lyricsLanguage,omitempty"`
	LyricsProbeArgv        []string              `json:"lyricsProbeArgv,omitempty"`
	ExpectedMountIdentity  string                `json:"expectedMountIdentity,omitempty"`
	ObservedRegistrationID string                `json:"observedRegistrationId,omitempty"`
	ObservedRelativePath   string                `json:"observedRelativePath,omitempty"`
	Operation              string                `json:"operation"`
	Identity               string                `json:"identity,omitempty"`
	Path                   string                `json:"path"`
	MountedRoot            string                `json:"mountedRoot,omitempty"`
	Limit                  int64                 `json:"limit,omitempty"`
	Size                   int64                 `json:"size,omitempty"`
	ModifiedNS             int64                 `json:"modifiedNs,omitempty"`
	VersionScope           string                `json:"versionScope,omitempty"`
	Version                *mediasource.Evidence `json:"version,omitempty"`
}
type Client struct {
	// RecordingRoot is a canonical, private, server-managed root, never a viewer path.
	RecordingRoot    string
	Remote           RemoteSources
	SourceOperations *OperationScope
	expectedIdentity string
	Binary           string
	Supervisor       *Supervisor
	Timeout          time.Duration
	Guard            func(string) error
	// SourceGuard applies only when a library source is added or changed.
	// Existing playback, scans and inspection use their own source authority.
	SourceGuard func(string) error
	MountedRoot func(string) string
	// VersionPolicy is configured only for roots with verified filesystem/mutation
	// assumptions. Nil denies new versioned access; legacy access is unchanged.
	VersionPolicy func(string) (LocalVersionPolicy, error)
}

func New(binary string) *Client {
	return &Client{Binary: binary, Supervisor: &Supervisor{}, Timeout: 30 * time.Second}
}
func (c *Client) run(ctx context.Context, key, op, path string, visit func(Snapshot) error) error {
	if c.Guard != nil {
		if err := c.Guard(path); err != nil {
			return err
		}
	}
	duration := c.Timeout
	if duration <= 0 {
		duration = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	req := request{Operation: op, Path: path, Identity: c.expectedIdentity, ManagedArtifact: c.managedRecording(path)}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(path)
	}
	data, _ := json.Marshal(req)
	cmd := exec.Command(c.Binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(data)
	return c.Supervisor.Run(ctx, filepath.Clean(key), cmd, func(reader io.Reader) error {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			var value Snapshot
			if e := json.Unmarshal(scanner.Bytes(), &value); e != nil {
				return e
			}
			if e := visit(value); e != nil {
				return e
			}
		}
		return scanner.Err()
	})
}
func (c *Client) InspectRoot(ctx context.Context, path string) (Snapshot, error) {
	if c.IsRemote(path) {
		return c.Remote.InspectRoot(ctx, path)
	}
	var value Snapshot
	e := c.run(ctx, path, "root", path, func(v Snapshot) error { value = v; return nil })
	return value, e
}
func (c *Client) Stat(ctx context.Context, key, path string) (Snapshot, error) {
	if c.IsRemote(path) {
		return c.Remote.Stat(ctx, path)
	}
	var value Snapshot
	e := c.run(ctx, key, "stat", path, func(v Snapshot) error { value = v; return nil })
	return value, e
}
func (c *Client) Inventory(ctx context.Context, key, path string, visit func(Snapshot) error) error {
	// Native providers require durable ListPage/CommitPage; never a FUSE fallback.
	if c.IsRemote(path) {
		return ErrRemoteCursor
	}
	return c.run(ctx, key, "inventory", path, visit)
}
func Helper(in io.Reader, out io.Writer) (result error) {
	var r request
	decoder := json.NewDecoder(io.LimitReader(in, 64<<10))
	if e := decoder.Decode(&r); e != nil {
		return e
	}
	if r.MountedRoot != "" {
		check := func() error {
			ready, e := isMount(r.MountedRoot)
			if e != nil || !ready {
				return errors.New("managed mount unavailable")
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		defer func() {
			if result == nil {
				result = check()
			}
		}()
	}
	encoder := json.NewEncoder(out)
	switch r.Operation {
	case "inventory-page":
		if r.Inventory == nil {
			return errors.New("missing inventory request")
		}
		return inventoryPageHelper(*r.Inventory, out)
	case "inventory-stat":
		if r.Inventory == nil {
			return errors.New("missing inventory request")
		}
		v, e := inspectInventoryObject(*r.Inventory)
		if e != nil {
			return e
		}
		return encoder.Encode(v)
	case "inventory-directory":
		if r.Inventory == nil {
			return errors.New("missing inventory request")
		}
		v, e := inspectInventoryDirectory(*r.Inventory)
		if e != nil {
			return e
		}
		return encoder.Encode(v)
	case "lyrics-local":
		return lyricLocalHelper(r, out)
	case "mount-identity":
		identity, err := physicalMountIdentity(r.Path)
		if err != nil {
			return err
		}
		return encoder.Encode(Snapshot{Data: identity})
	case "register-root":
		return registerRootHelper(r, out)
	case "playback-observed":
		return observedPlaybackHelper(r, io.MultiReader(decoder.Buffered(), in), out)
	case "playback-discover-version", "playback-validate-version":
		return playbackVersionHelper(r, out)
	case "playback-stream":
		return playbackStreamHelper(r, io.MultiReader(decoder.Buffered(), in), out)
	case "playback-validate-descriptor":
		return playbackValidateDescriptorHelper(r, out)
	case "playback-descriptor":
		return playbackDescriptorHelper(r, out)
	case "root":
		path, e := filepath.EvalSymlinks(r.Path)
		if e != nil {
			return e
		}
		path, e = filepath.Abs(path)
		if e != nil {
			return e
		}
		info, e := os.Stat(path)
		if e != nil {
			return e
		}
		if !info.IsDir() {
			return errors.New("not a directory")
		}
		return encoder.Encode(snapshot(path, info))
	case "inventory-blob":
		return inventoryBlobHelper(r, out)
	case "inventory-command":
		return inventoryCommandHelper(r)
	case "blob":
		if r.Limit <= 0 || r.Limit > 8<<20 {
			return errors.New("invalid blob limit")
		}
		info, e := os.Lstat(r.Path)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Size() > r.Limit {
			return errors.New("invalid local metadata file")
		}
		f, e := os.Open(r.Path)
		if e != nil {
			return e
		}
		defer f.Close()
		if e = lockRecordingFile(r, f); e != nil {
			return e
		}
		n, e := io.Copy(out, io.LimitReader(f, r.Limit+1))
		if n > r.Limit {
			return errors.New("local metadata exceeds limit")
		}
		return e
	case "remove-mount-root":
		if r.Identity != "" {
			actual, e := DirectoryIdentity(r.Path)
			if errors.Is(e, os.ErrNotExist) {
				return encoder.Encode(Snapshot{})
			}
			if e != nil || actual != r.Identity {
				return errors.New("allocation identity changed")
			}
		}
		mounted, err := isMount(r.Path)
		if errors.Is(err, os.ErrNotExist) {
			return encoder.Encode(Snapshot{Path: r.Path})
		}
		if err != nil {
			return err
		}
		if mounted {
			return errors.New("mount remains active")
		}
		if err = os.Remove(r.Path); err != nil {
			return err
		}
		return encoder.Encode(Snapshot{Path: r.Path})
	case "mount-status":
		mounted, err := isMount(r.Path)
		if err != nil {
			return err
		}
		return encoder.Encode(Snapshot{Path: r.Path, Directory: mounted})
	case "descriptor":
		info, e := os.Lstat(r.Path)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return errors.New("invalid descriptor")
		}
		file, e := os.Open(r.Path)
		if e != nil {
			return e
		}
		defer file.Close()
		if e = lockRecordingFile(r, file); e != nil {
			return e
		}
		raw, e := io.ReadAll(io.LimitReader(file, (64<<10)+1))
		if e != nil || len(raw) > 64<<10 {
			return errors.New("descriptor exceeds budget")
		}
		result := snapshot(r.Path, info)
		result.Data = string(raw)
		return encoder.Encode(result)
	case "stat":
		info, e := os.Lstat(r.Path)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("not a regular file")
		}
		return encoder.Encode(snapshot(r.Path, info))
	case "inventory":
		f, e := os.Open(r.Path)
		if e != nil {
			return e
		}
		defer f.Close()
		if e = lockRecordingFile(r, f); e != nil {
			return e
		}
		for {
			entries, e := f.ReadDir(128)
			if e != nil && e != io.EOF {
				return e
			}
			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
					continue
				}
				if err = encoder.Encode(snapshot(filepath.Join(r.Path, entry.Name()), info)); err != nil {
					return err
				}
			}
			if e == io.EOF {
				return nil
			}
		}
	default:
		return errors.New("unknown storage operation")
	}
}
func snapshot(path string, info os.FileInfo) Snapshot {
	v := Snapshot{Path: path, Name: info.Name(), Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Directory: info.IsDir()}
	v.ObjectIdentity, v.ChangeToken, _ = observedObjectBinding(info)
	return v
}

func (c *Client) ReadDescriptor(ctx context.Context, key, path string) (Snapshot, error) {
	var out Snapshot
	e := c.run(ctx, "playback:"+key, "descriptor", path, func(v Snapshot) error { out = v; return nil })
	return out, e
}

// Mounted uses an isolated helper without the data-availability guard: it is the
// observation that drives the guard itself.
func (c *Client) Mounted(ctx context.Context, path string) (bool, error) {
	raw := *c
	raw.Guard = nil
	raw.MountedRoot = nil
	var value Snapshot
	err := raw.run(ctx, "mount-status:"+path, "mount-status", path, func(v Snapshot) error { value = v; return nil })
	return value.Directory, err
}

func (c *Client) ReadSmall(ctx context.Context, key, path string, limit int64) ([]byte, error) {
	if c.IsRemote(path) {
		return c.readRemoteSmall(ctx, path, limit)
	}
	if err := CheckScanRead(ctx, path); err != nil {
		return nil, err
	}
	if c.Guard != nil {
		if err := c.Guard(path); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req := request{Operation: "blob", Path: path, Limit: limit, ManagedArtifact: c.managedRecording(path)}
	root, rooted, err := inventoryReadRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	if rooted != nil {
		req.Operation = "inventory-blob"
		req.Inventory = &root
	}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(path)
	}
	input, _ := json.Marshal(req)
	cmd := exec.Command(c.Binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(input)
	var raw []byte
	err = c.Supervisor.Run(ctx, key, cmd, func(r io.Reader) error {
		var err error
		raw, err = io.ReadAll(io.LimitReader(r, limit+1))
		if int64(len(raw)) > limit {
			return errors.New("local metadata exceeds limit")
		}
		return err
	})
	return raw, err
}

// RemoveMountRoot is idempotent empty-directory removal behind the same bounded
// subprocess boundary as mount observation. It never recursively deletes files.
func (c *Client) RemoveMountRoot(ctx context.Context, path string) error {
	raw := *c
	raw.Guard = nil
	raw.MountedRoot = nil
	return raw.run(ctx, "mount-remove:"+path, "remove-mount-root", path, func(Snapshot) error { return nil })
}

func (c *Client) RemoveOwnedMountRoot(ctx context.Context, path, identity string) error {
	if identity == "" {
		return os.ErrInvalid
	}
	copy := *c
	copy.expectedIdentity = identity
	return copy.RemoveMountRoot(ctx, path)
}
