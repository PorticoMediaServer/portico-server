package subtitlevideo

import (
	"context"
	"portico.local/server/internal/subtitles"
	"sync"
	"time"
)

// A track may take a long time on a NAS as long as input keeps moving. The idle
// limit catches stalled storage/decoders; the outer limit bounds hostile input.
type progressingInput struct {
	subtitles.RenderInput
	mu     sync.Mutex
	timer  *time.Timer
	cancel context.CancelFunc
}

func extractionProgress(ctx context.Context, input subtitles.RenderInput) (context.Context, *progressingInput, func()) {
	ctx, outer := context.WithTimeout(ctx, 2*time.Hour)
	ctx, cancel := context.WithCancel(ctx)
	p := &progressingInput{RenderInput: input, cancel: cancel}
	p.timer = time.AfterFunc(2*time.Minute, cancel)
	return ctx, p, func() { p.mu.Lock(); p.timer.Stop(); p.mu.Unlock(); cancel(); outer() }
}
func (p *progressingInput) ReadExtent(ctx context.Context, offset, n int64) ([]byte, error) {
	b, e := p.RenderInput.ReadExtent(ctx, offset, n)
	if e == nil && len(b) > 0 {
		p.mu.Lock()
		p.timer.Reset(2 * time.Minute)
		p.mu.Unlock()
	}
	return b, e
}
