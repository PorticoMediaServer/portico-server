//go:build linux || darwin

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"portico.local/server/internal/mediaexec"
)

// The descriptor is confined by os.Root before FFprobe receives it as
// descriptor 3. argv was built by mediaexec in the server (sandboxed where the
// platform can); it carries no provider URL, secret or untrusted path.
func probeLyricTags(source *os.File, argv []string) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if len(argv) < 2 || argv[len(argv)-1] != descriptorInput {
		return nil, errors.New("invalid lyric probe")
	}
	cmd, err := mediaexec.CommandArgv(ctx, argv, source)
	if err != nil {
		return nil, err
	}
	cmd.WaitDelay = time.Second
	cmd.Stderr = io.Discard
	buf := &lyricProbeBuffer{}
	cmd.Stdout = buf
	if e := cmd.Run(); e != nil {
		return nil, e
	}
	var payload struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Tags map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if json.Unmarshal(buf.data, &payload) != nil || len(payload.Streams) > 128 {
		return nil, errors.New("invalid lyric tags")
	}
	out := []map[string]string{payload.Format.Tags}
	for _, s := range payload.Streams {
		out = append(out, s.Tags)
	}
	return out, nil
}

type lyricProbeBuffer struct{ data []byte }

func (b *lyricProbeBuffer) Write(p []byte) (int, error) {
	if len(p) > 1<<20-len(b.data) {
		return 0, errors.New("embedded lyrics exceed limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
