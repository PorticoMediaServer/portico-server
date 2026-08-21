package app

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const mediaDiskReservationMinimum = int64(64 << 20)

// reserveMediaDisk accounts predicted concurrent output on the actual target
// filesystem. Free-space checks alone allow several jobs to observe the same
// bytes and all fail later; reservations close that admission race.
func (g *mediaResourceGovernor) reserveMediaDisk(path string, predictedBytes, safetyBytes int64) (func(), error) {
	if g == nil {
		return nil, errMediaResourcesBusy
	}
	if predictedBytes < 0 {
		return nil, errors.New("predicted media output cannot be negative")
	}
	if predictedBytes < mediaDiskReservationMinimum {
		predictedBytes = mediaDiskReservationMinimum
	}
	if safetyBytes < mediaDiskReservationMinimum {
		safetyBytes = mediaDiskReservationMinimum
	}
	existing, err := nearestExistingDirectory(path)
	if err != nil {
		return nil, err
	}
	key, err := filesystemReservationKey(existing)
	if err != nil {
		return nil, err
	}
	available, _, err := filesystemSpace(existing)
	if err != nil {
		return nil, err
	}

	g.mu.Lock()
	if g.diskReservedBytes == nil {
		g.diskReservedBytes = map[string]int64{}
	}
	already := g.diskReservedBytes[key]
	if predictedBytes > available || safetyBytes > available-predictedBytes || already > available-predictedBytes-safetyBytes {
		g.mu.Unlock()
		return nil, errMediaStoragePressure
	}
	g.diskReservedBytes[key] = already + predictedBytes
	g.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			remaining := g.diskReservedBytes[key] - predictedBytes
			if remaining > 0 {
				g.diskReservedBytes[key] = remaining
			} else {
				delete(g.diskReservedBytes, key)
			}
			g.mu.Unlock()
		})
	}, nil
}

func nearestExistingDirectory(path string) (string, error) {
	path = filepath.Clean(path)
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				path = filepath.Dir(path)
				continue
			}
			return path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", os.ErrNotExist
		}
		path = parent
	}
}
