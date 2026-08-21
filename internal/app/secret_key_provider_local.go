//go:build !darwin && !windows

package app

import "path/filepath"

// NewSecretKeyProvider selects the private file provider for Linux and other
// headless/portable targets. The key is deliberately kept outside SQLite and
// the database backup directory.
func NewSecretKeyProvider(appDataDir string) SecretKeyProvider {
	return NewLocalSecretKeyProvider(filepath.Join(appDataDir, "keys", "hosted-authority.key"))
}
