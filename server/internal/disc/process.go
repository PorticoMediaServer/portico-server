package disc

import (
	"context"
	"io"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/mediatools"
	"strconv"
	"time"
)

// outputBuffer fails closed on truncation. The helper also buffers at most
// 1MiB before publishing JSON, so oversized structures never look successful.
type outputBuffer struct{ data []byte }

func (b *outputBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > MaxOutputBytes {
		return 0, ErrProtocol
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

type Tool struct{ Binary string }

func (t Tool) run(ctx context.Context, path string, args ...string) ([]byte, error) {
	if !cleanLocalPath(path) {
		return nil, ErrProtocol
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	binary := t.Binary
	if binary == "" {
		binary = mediatools.Resolve("portico-disc")
	}
	cmd, err := mediaexec.CommandContext(ctx, mediaexec.Job{Executable: binary, Args: args, ReadPaths: []string{path}, MaxFileBytes: mediaexec.NoOutputFiles})
	if err != nil {
		return nil, err
	}
	out := &outputBuffer{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if err = cmd.Run(); err != nil {
		return nil, err
	}
	return out.data, nil
}
func (t Tool) ProbeLocal(ctx context.Context, path string) (Probe, error) {
	raw, err := t.run(ctx, path, "probe", path)
	if err != nil {
		return Probe{}, err
	}
	return DecodeProbe(raw)
}
func (t Tool) SeekLocal(ctx context.Context, path string, playlist int, ms int64) (Seek, error) {
	if playlist < 0 || playlist > 99999 || ms < 0 {
		return Seek{}, ErrProtocol
	}
	raw, err := t.run(ctx, path, "seek", path, strconv.Itoa(playlist), strconv.FormatInt(ms, 10))
	if err != nil {
		return Seek{}, err
	}
	return DecodeSeek(raw, playlist, ms)
}
