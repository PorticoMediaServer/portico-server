//go:build linux

package decoder

import (
	"context"
	"portico.local/server/internal/mediaexec"
)

func inventoryIdentity(ctx context.Context, executable string) (string, bool, error) {
	return mediaexec.RuntimeIdentity(ctx, executable)
}
