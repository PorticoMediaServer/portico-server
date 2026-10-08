//go:build !linux && !darwin

package storage

import "errors"

func inventoryCommandHelper(request) error {
	return errors.New("retained-descriptor scan execution is unsupported on this server platform")
}

// Retained-descriptor execution doesn't exist here; these keep the shared
// request builder compiling.
const descriptorInput = "/dev/fd/3"
const scanCPUSeconds = 300
