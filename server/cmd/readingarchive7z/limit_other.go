//go:build !linux && !darwin

package main

import "errors"

// limitProcess refuses to run where the resource limits this isolation relies on
// cannot be set; there is no silent unbounded fallback.
func limitProcess() error {
	return errors.New("bounded archive decoding is unavailable on this platform")
}
