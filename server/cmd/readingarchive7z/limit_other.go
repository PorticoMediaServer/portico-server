//go:build !linux && !darwin

package main

// limitProcess is a no-op where the platform rlimits are unavailable; the
// parent's process-group kill, deadline and scratch removal remain mandatory.
func limitProcess() {}
