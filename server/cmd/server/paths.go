package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"portico.local/server/internal/storagepaths"
)

func startupLayout() (storagepaths.Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return storagepaths.Layout{}, err
	}
	mounts := startupMounts()
	_, docker := os.Stat("/.dockerenv")
	layout, err := storagepaths.Resolve(storagepaths.Options{Home: home, LocalAppData: os.Getenv("LOCALAPPDATA"), ProgramData: os.Getenv("ProgramData"), DataDirectory: os.Getenv("PORTICO_DATA_DIR"), Service: os.Getenv("PORTICO_SERVICE") == "1", Container: docker == nil || os.Getenv("PORTICO_CONTAINER") == "1", Mounts: mounts})
	if err == nil && layout.Container && !storagepaths.IsMount(layout.Data, mounts) {
		return layout, fmt.Errorf("container data folder %s must be a persistent mount", layout.Data)
	}
	return layout, err
}

func startupMounts() []storagepaths.Mount {
	if file, err := os.Open("/proc/self/mountinfo"); err == nil {
		defer file.Close()
		return storagepaths.ParseMountInfo(file)
	}
	return nil
}
func startupDirectory(variable, fallback string) (string, error) {
	directory := os.Getenv(variable)
	if directory == "" {
		return fallback, nil
	}
	if !filepath.IsAbs(directory) {
		return "", fmt.Errorf("%s must name an absolute directory", variable)
	}
	directory = filepath.Clean(directory)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("%s: %w", variable, err)
	}
	return directory, nil
}

// Deleting stale work requires the same exclusive ownership as staged restore.
// Unsupported filesystem locking retains the existing warn-and-continue start.
func resetStartupWork(layout storagepaths.Layout, ownsLock bool) error {
	if !ownsLock {
		return nil
	}
	if err := layout.ResetTemp(); err != nil {
		if !errors.Is(err, storagepaths.ErrCleanupUnavailable) {
			return err
		}
		log.Printf("Temp cleanup skipped: %v", err)
	}
	if err := layout.ApplyCacheEmpty(); err != nil {
		if !errors.Is(err, storagepaths.ErrCleanupUnavailable) {
			return err
		}
		log.Printf("Cache cleanup postponed: %v", err)
	}
	return nil
}
