package identity

import (
	"context"
	"strings"
)

type DeviceContext struct{ Name, Platform, App, Version, InstallationID, IP string }
type deviceContextKey struct{}

func WithDeviceContext(ctx context.Context, d DeviceContext) context.Context {
	clean := func(s string, n int) string {
		s = strings.TrimSpace(s)
		if len(s) > n {
			return ""
		}
		return s
	}
	d.Name = clean(d.Name, 120)
	d.Platform = clean(d.Platform, 40)
	d.App = clean(d.App, 80)
	d.Version = clean(d.Version, 40)
	d.InstallationID = clean(d.InstallationID, 128)
	return context.WithValue(ctx, deviceContextKey{}, d)
}
func DeviceFromContext(ctx context.Context) DeviceContext {
	d, _ := ctx.Value(deviceContextKey{}).(DeviceContext)
	if d.Name == "" {
		d.Name = "Portico session"
	}
	if d.Platform == "" {
		d.Platform = "unknown"
	}
	return d
}
