package main

import (
	"fmt"
	"os"
	"path/filepath"
)

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
