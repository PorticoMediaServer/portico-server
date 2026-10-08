//go:build !darwin && !linux && !windows

package administration

func fileIdentity(string) (string, error) { return "", ErrUnavailable }
