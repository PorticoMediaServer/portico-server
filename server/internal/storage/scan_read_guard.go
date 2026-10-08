package storage

import "context"

type scanReadGuardKey struct{}
type ScanReadGuard func(context.Context, string) error

// WithScanReadGuard applies an ingestion-policy check at content-read admission.
// Ordinary playback/provider callers do not install this optional scan context.
func WithScanReadGuard(ctx context.Context, guard ScanReadGuard) context.Context {
	return context.WithValue(ctx, scanReadGuardKey{}, guard)
}
func CheckScanRead(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard, ok := ctx.Value(scanReadGuardKey{}).(ScanReadGuard); ok {
		return guard(ctx, path)
	}
	return nil
}
