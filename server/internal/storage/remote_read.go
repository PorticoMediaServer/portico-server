package storage

import (
	"context"
	"io"
	"time"
)

// readRemoteSmall uses a single retained/conditional acquisition. The path stays
// server-private, rooted scan policy is checked before acquisition and after the
// bounded read, and native sources never fall through to their empty mount path.
func (c *Client) readRemoteSmall(ctx context.Context, path string, limit int64) ([]byte, error) {
	if limit <= 0 || limit > 8<<20 {
		return nil, ErrRemoteLimit
	}
	if err := CheckScanRead(ctx, path); err != nil {
		return nil, err
	}
	req, input, err := inventoryReadRequest(ctx, path)
	if err != nil {
		return nil, err
	}
	if input != nil {
		if _, err = c.InventoryStat(ctx, input.source, req); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	object, err := c.Remote.Acquire(ctx, path, "analysis")
	if err != nil {
		return nil, err
	}
	defer object.Close()
	before := object.Snapshot()
	if before.Directory || before.Size < 0 || before.Size > limit {
		return nil, ErrRemoteLimit
	}
	var raw []byte
	if before.Size > 0 {
		body, e := object.OpenRange(ctx, 0, before.Size)
		if e != nil {
			return nil, e
		}
		raw, err = io.ReadAll(io.LimitReader(body, limit+1))
		body.Close()
		if err != nil {
			return nil, err
		}
		if int64(len(raw)) != before.Size {
			return nil, ErrRemoteChanged
		}
	}
	if err = CheckScanRead(ctx, path); err != nil {
		return nil, err
	}
	after, err := object.Observe(ctx)
	if err != nil {
		return nil, err
	}
	if after.Revision != before.Revision {
		return nil, ErrRemoteChanged
	}
	return raw, nil
}
