package storage

import "context"

// scanReadCheck is the one authoritative scan-read entry. The guard and the
// observation it already takes live together, so a caller that installs a newer
// guard replaces both: an observation taken under an older authority can never
// outlive or speak for the guard that replaced it.
type scanReadCheck struct {
	guard   ScanReadGuard
	client  *Client
	witness ScanReadWitnessFunc
}

type scanReadGuardKey struct{}
type ScanReadGuard func(context.Context, string) error

// ScanReadWitness is one fresh observation made under scan-read authority: the
// exact request that invocation observed and the snapshot it got back. It says
// what was seen at that instant; it is never a grant to read bytes, and an
// absent witness is not evidence of anything.
type ScanReadWitness struct {
	SourceID       string
	Root           string
	RootIdentity   string
	RelativePath   string
	FollowSymlinks bool
	Snapshot       Snapshot
}

// ScanReadWitnessFunc observes one path under the same policy and containment
// checks as the guard, and returns the request it actually observed.
type ScanReadWitnessFunc func(context.Context, string) (ScanReadWitness, error)

// ScanReadObservation is the witness that answered for one path, bound to the
// exact producer that took it.
type ScanReadObservation struct {
	Client *Client
	ScanReadWitness
}

// WithScanReadGuard applies an ingestion-policy check at content-read admission.
// Ordinary playback/provider callers do not install this optional scan context.
// It also replaces any observation installed with the guard it supersedes.
func WithScanReadGuard(ctx context.Context, guard ScanReadGuard) context.Context {
	return context.WithValue(ctx, scanReadGuardKey{}, scanReadCheck{guard: guard})
}

// WithScanReadWitness installs the scan-read guard together with the observation
// that guard invocation already takes, as a single authoritative entry. The
// witness must apply the guard's own policy and containment checks on every call
// and return the request it observed; client binds the observation to the exact
// producer so another storage namespace cannot satisfy it. Nothing is cached:
// each call is a fresh observation.
func WithScanReadWitness(ctx context.Context, client *Client, guard ScanReadGuard, witness ScanReadWitnessFunc) context.Context {
	return context.WithValue(ctx, scanReadGuardKey{}, scanReadCheck{guard: guard, client: client, witness: witness})
}

func CheckScanRead(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if check, ok := ctx.Value(scanReadGuardKey{}).(scanReadCheck); ok {
		return check.guard(ctx, path)
	}
	return nil
}

// ObserveScanRead reports the observation the current scan-read entry already
// took for path. ok is false when no observation is installed, and the caller
// then keeps its own explicit proof. A returned error is never usable as proof.
func ObserveScanRead(ctx context.Context, path string) (ScanReadObservation, bool, error) {
	if err := ctx.Err(); err != nil {
		return ScanReadObservation{}, false, err
	}
	check, ok := ctx.Value(scanReadGuardKey{}).(scanReadCheck)
	if !ok || check.witness == nil {
		return ScanReadObservation{}, false, nil
	}
	witness, err := check.witness(ctx, path)
	if err != nil {
		return ScanReadObservation{}, true, err
	}
	return ScanReadObservation{Client: check.client, ScanReadWitness: witness}, true, nil
}
