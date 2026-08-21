//go:build windows

package app

// Windows does not expose Unix ESTALE/EIO errno values. Portable permission
// matching and filesystem error text are handled by storageErrorClass and
// storageErrorTransient; keep this hook so call sites remain build-neutral.
func storageErrnoClass(error) (string, bool) { return "", false }
