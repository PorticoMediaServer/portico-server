package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"portico.local/server/internal/mediaexec"
)

type scanInputKey struct{}
type scanInput struct {
	client *Client
	source string
	root   InventoryRequest
}

// WithScanInput binds optional analysis reads to the same admitted inventory
// root. It is not playback version evidence and does not affect provider I/O.
func WithScanInput(ctx context.Context, client *Client, source string, root InventoryRequest) context.Context {
	return context.WithValue(ctx, scanInputKey{}, scanInput{client, source, root})
}
func inventoryReadRequest(ctx context.Context, path string) (InventoryRequest, *scanInput, error) {
	input, ok := ctx.Value(scanInputKey{}).(scanInput)
	if !ok || input.client == nil {
		return InventoryRequest{}, nil, nil
	}
	relative, err := filepath.Rel(input.root.Root, path)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return InventoryRequest{}, nil, ErrInventoryChanged
	}
	r := input.root
	r.RelativePath = relative
	return r, &input, nil
}

// RunScanCommand runs a media tool by replacing the already-supervised helper
// process. No unmanaged grandchild can outlive the supervisor's physical Wait.
func RunScanCommand(ctx context.Context, binary string, args []string, path string, out io.Writer) (bool, error) {
	r, input, err := inventoryReadRequest(ctx, path)
	if err != nil {
		return true, err
	}
	if input == nil {
		return false, nil
	}
	if err = CheckScanRead(ctx, path); err != nil {
		return true, err
	}
	index := -1
	for i, arg := range args {
		if arg == path {
			if index != -1 {
				return true, errors.New("ambiguous scan input")
			}
			index = i
		}
	}
	if index < 0 || len(args) > 128 {
		return true, errors.New("invalid scan command")
	}
	c := input.client
	if c.IsRemote(path) {
		return true, ErrRemoteRange
	} // Native objects require an authenticated acquisition bridge.
	if c.Guard != nil {
		if err = c.Guard(path); err != nil {
			return true, err
		}
	}
	// The helper opens the admitted file and hands it over as descriptor 3; the
	// tool (sandboxed, or with the baseline) never gets the path (mediaexec).
	fdArgs := append([]string{}, args...)
	fdArgs[index] = descriptorInput
	argv, err := mediaexec.Argv(mediaexec.Job{Executable: binary, Args: fdArgs, Background: true, MaxCPUSeconds: scanCPUSeconds})
	if err != nil {
		return true, err
	}
	req := request{Operation: "inventory-command", Path: path, Inventory: &r, Argv: argv}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(path)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return true, err
	}
	cmd, err := scanHelperCommand(c.Binary, raw)
	if err != nil {
		return true, err
	}
	err = c.Supervisor.Run(ctx, "scan:"+input.source, cmd, func(reader io.Reader) error {
		n, e := io.Copy(out, io.LimitReader(reader, (4<<20)+1))
		if n > 4<<20 {
			return errors.New("scan output exceeds limit")
		}
		return e
	})
	return true, err
}

// openInventoryRoot compares the root actually retained by OpenRoot, not merely
// a stat done before an unrelated pathname open. Source replacement fails closed.
func openInventoryRoot(r InventoryRequest) (*os.Root, error) {
	if !filepath.IsAbs(r.Root) {
		return nil, ErrInventoryChanged
	}
	root, err := os.OpenRoot(r.Root)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || r.RootIdentity != "" && RootIdentity(snapshot(r.Root, info)) != r.RootIdentity {
		root.Close()
		return nil, ErrInventoryChanged
	}
	return root, nil
}
func openInventoryContent(r InventoryRequest) (*os.File, error) {
	path, err := inventoryPath(r)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(r.Root, path)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return nil, ErrInventoryChanged
	}
	root, err := openInventoryRoot(r)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, ErrInventoryChanged
	}
	return file, nil
}
func inventoryBlobHelper(r request, out io.Writer) error {
	if r.Inventory == nil || r.Limit <= 0 || r.Limit > 8<<20 {
		return errors.New("invalid inventory blob")
	}
	file, err := openInventoryContent(*r.Inventory)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > r.Limit {
		return errors.New("invalid local metadata file")
	}
	n, err := io.Copy(out, io.LimitReader(file, r.Limit+1))
	if n > r.Limit {
		return errors.New("local metadata exceeds limit")
	}
	return err
}

// scanHelperCommand starts the helper for one scan command. The helper moves
// the media file to descriptor 3 before it execs the tool. A placeholder holds
// 3 from the helper's start, so the Go runtime's own poller (kqueue on macOS)
// is never given 3 and then overwritten by that move: every probe failed so.
func scanHelperCommand(binary string, request []byte) (*exec.Cmd, error) {
	placeholder, err := descriptorPlaceholder()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(request)
	cmd.ExtraFiles = []*os.File{placeholder}
	return cmd, nil
}

var placeholderOnce struct {
	sync.Once
	file *os.File
	err  error
}

// descriptorPlaceholder is one read-only /dev/null shared by every scan
// helper start; each child gets its own copy as descriptor 3.
func descriptorPlaceholder() (*os.File, error) {
	placeholderOnce.Do(func() { placeholderOnce.file, placeholderOnce.err = os.Open(os.DevNull) })
	return placeholderOnce.file, placeholderOnce.err
}
