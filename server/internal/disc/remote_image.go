package disc

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"

	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/mediatools"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/supervise"
)

const remoteDiscRangeBytes = 2 << 20

// remoteImageCommand exposes only a caller-retained immutable RemoteObject,
// through two inherited pipes. The helper receives neither a locator nor any
// network, source-directory or credential authority. OpenRange retains the
// captured object's existing conditional reads and configuration guards.
//
// This does not admit remote discs into scanning/playback: those routes remain
// closed until the complete stream-mode path is qualified.
func (t Tool) remoteImageCommand(ctx context.Context, object storage.RemoteObject, mode string, arguments []string, maxRequests, maxBytes int64) (*exec.Cmd, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if runtime.GOOS == "windows" || object == nil || object.Version().ID() == "" || object.Access() != "finite_random" {
		return nil, nil, ErrRemoteUnqualified
	}
	size := object.Snapshot().Size
	if object.Snapshot().Directory || object.Snapshot().ErrorCode != "" || size <= 0 || size%2048 != 0 || maxRequests <= 0 || maxBytes <= 0 {
		return nil, nil, ErrProtocol
	}
	requests, helperRequests, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	helperData, data, err := os.Pipe()
	if err != nil {
		requests.Close()
		helperRequests.Close()
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	var once sync.Once
	closePipes := func() {
		once.Do(func() {
			requests.Close()
			helperRequests.Close()
			helperData.Close()
			data.Close()
		})
	}
	stop := context.AfterFunc(ctx, closePipes)
	finish := func() { cancel(); stop(); closePipes() }
	binaryPath := t.Binary
	if binaryPath == "" {
		binaryPath = mediatools.Resolve("portico-disc")
	}
	args := append([]string{mode, strconv.FormatInt(size, 10)}, arguments...)
	cmd, err := mediaexec.CommandContext(ctx, mediaexec.Job{Executable: binaryPath, Args: args, Files: []*os.File{helperRequests, helperData}, MaxFileBytes: mediaexec.NoOutputFiles})
	if err != nil {
		finish()
		return nil, nil, err
	}
	done := make(chan struct{})
	supervise.Go("disc.remote-image-ranges", func() {
		defer close(done)
		defer closePipes()
		serveRemoteDiscRanges(ctx, requests, data, object, size, maxRequests, maxBytes)
	})
	return cmd, func() { finish(); <-done }, nil
}

// Each request is PDR1, byte offset (uint64 BE), length (uint32 BE). A reply
// is PDA1 and the exact length (uint32 BE), then complete bytes. Refusal closes
// the channel; no partial range is published as a successful response.
func serveRemoteDiscRanges(ctx context.Context, requests io.Reader, data io.Writer, object storage.RemoteObject, size, maxRequests, maxBytes int64) {
	buffer := make([]byte, remoteDiscRangeBytes)
	var header [16]byte
	for count, used := int64(0), int64(0); count < maxRequests; count++ {
		if _, err := io.ReadFull(requests, header[:]); err != nil || ctx.Err() != nil || string(header[:4]) != "PDR1" {
			return
		}
		offset, length := binary.BigEndian.Uint64(header[4:12]), int64(binary.BigEndian.Uint32(header[12:]))
		if offset >= uint64(size) || length <= 0 || length > remoteDiscRangeBytes || length > size-int64(offset) || length > maxBytes-used {
			return
		}
		used += length
		readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		body, err := object.OpenRange(readCtx, int64(offset), length)
		if err != nil || body == nil {
			if body != nil {
				body.Close()
			}
			cancel()
			return
		}
		_, err = io.ReadFull(body, buffer[:length])
		closeErr := body.Close()
		current := readCtx.Err() == nil && ctx.Err() == nil
		cancel()
		if err != nil || closeErr != nil || !current {
			return
		}
		var reply [8]byte
		copy(reply[:4], "PDA1")
		binary.BigEndian.PutUint32(reply[4:], uint32(length))
		if n, e := data.Write(reply[:]); e != nil || n != len(reply) {
			return
		}
		if n, e := data.Write(buffer[:length]); e != nil || int64(n) != length {
			return
		}
	}
}

// ProbeRemoteImage runs the bounded reader against one original captured ISO.
// The caller owns and closes that object after this invocation. A size/mtime
// observation alone is deliberately insufficient authority to issue this job.
func (t Tool) ProbeRemoteImage(ctx context.Context, object storage.RemoteObject) (Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd, finish, err := t.remoteImageCommand(ctx, object, "probe-image-stream", nil, 1024, 16<<20)
	if err != nil {
		return Probe{}, err
	}
	defer finish()
	before := object.Snapshot()
	out := &outputBuffer{}
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = out, io.Discard, time.Second
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Probe{}, ctx.Err()
		}
		return Probe{}, ErrProtocol
	}
	if err = ctx.Err(); err != nil {
		return Probe{}, err
	}
	after, err := object.Observe(ctx)
	if err != nil || after != before {
		return Probe{}, storage.ErrRemoteChanged
	}
	probe, err := DecodeProbe(out.data)
	if ctx.Err() != nil {
		return Probe{}, ctx.Err()
	}
	return probe, err
}
