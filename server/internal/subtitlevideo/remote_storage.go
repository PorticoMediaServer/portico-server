package subtitlevideo

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
)

// RemoteObject is the structural P04B storage.RemoteObject contract. Keeping this
// local interface lets I04 compile against the immutable baseline; the assembler
// passes RemoteSources.Acquire via a one-line covariant return adapter, not a URL.
type RemoteObject interface {
	Snapshot() storage.Snapshot
	Version() mediasource.Version
	Access() string
	Observe(context.Context) (storage.Snapshot, error)
	OpenRange(context.Context, int64, int64) (io.ReadCloser, error)
	Close() error
}
type RemoteStorageAdapter struct {
	Match   func(string) bool
	Acquire func(context.Context, string, string) (RemoteObject, error)
}

func (a RemoteStorageAdapter) Handles(path string) bool { return a.Match != nil && a.Match(path) }
func (a RemoteStorageAdapter) Open(ctx context.Context, item, source, path string, size, modified int64) (subtitles.RenderInput, error) {
	if a.Acquire == nil {
		return nil, subtitles.ErrUnavailable
	}
	o, e := a.Acquire(ctx, path, "playback")
	if e != nil {
		return nil, e
	}
	good := false
	defer func() {
		if !good {
			o.Close()
		}
	}()
	snap, e := o.Observe(ctx)
	if e != nil {
		return nil, e
	}
	if snap.Directory || snap.Size < 1 || size >= 0 && snap.Size != size || (modified != 0 && snap.ModifiedNS != modified) {
		return nil, subtitles.ErrConflict
	}
	if o.Access() != "finite_random" && o.Access() != "finite_sequential" {
		return nil, subtitles.ErrUnsupported
	}
	evidence := "observed:" + identity.Token()
	if o.Version().ID() != "" {
		evidence = "strong:" + o.Version().ID()
	}
	encoded, e := json.Marshal(snap)
	if e != nil {
		return nil, e
	}
	p := &remoteStorageInput{object: o, size: snap.Size, observation: string(encoded), evidence: evidence}
	if e = p.Validate(ctx); e != nil {
		return nil, e
	}
	good = true
	return p, nil
}

type remoteStorageInput struct {
	object                RemoteObject
	size                  int64
	observation, evidence string
	mu                    sync.Mutex
	closed, lost          bool
}

func (p *remoteStorageInput) Size() int64      { return p.size }
func (p *remoteStorageInput) Evidence() string { return p.evidence }
func (p *remoteStorageInput) validate(ctx context.Context) error {
	if p.closed || p.lost {
		return subtitles.ErrUnavailable
	}
	now, e := p.object.Observe(ctx)
	if e != nil {
		return e
	}
	raw, e := json.Marshal(now)
	if e != nil {
		return e
	}
	if string(raw) != p.observation {
		p.lost = true
		return subtitles.ErrConflict
	}
	return nil
}
func (p *remoteStorageInput) Validate(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.validate(ctx)
}
func (p *remoteStorageInput) ReadExtent(ctx context.Context, offset, length int64) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if offset < 0 || length < 1 || length > 1<<20 || offset > p.size-length {
		return nil, subtitles.ErrInput
	}
	if p.closed || p.lost {
		return nil, subtitles.ErrUnavailable
	}
	if !strings.HasPrefix(p.evidence, "strong:") {
		if e := p.validate(ctx); e != nil {
			return nil, e
		}
	}
	body, e := p.object.OpenRange(ctx, offset, length)
	if e != nil {
		return nil, e
	}
	raw, e := io.ReadAll(io.LimitReader(body, length+1))
	closeErr := body.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(raw)) != length {
		p.lost = true
		return nil, subtitles.ErrConflict
	}
	if !strings.HasPrefix(p.evidence, "strong:") {
		if e = p.validate(ctx); e != nil {
			return nil, e
		}
	}
	return raw, nil
}
func (p *remoteStorageInput) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return p.object.Close()
}
