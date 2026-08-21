//go:build windows

package app

import "os"

func validateNativeCredentialOwner(os.FileInfo) error {
	return nil
}
