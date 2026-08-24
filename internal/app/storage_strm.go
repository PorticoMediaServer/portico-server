package app

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
)

const strmDescriptorLimit int64 = 64 << 10

// readSTRMLocator treats STRM as a descriptor, never as media. The locator is
// resolved only at playback time, is not copied into media_files/source_url,
// and is therefore absent from library APIs, logs, diagnostics and backups of
// catalog metadata. The descriptor itself remains owner-managed storage.
func (s *Server) readSTRMLocator(ctx context.Context, descriptor string) (string, error) {
	request := s.storageRequestForPath(ctx, descriptor, "read STRM descriptor")
	raw, err := s.storageReadFile(ctx, request, descriptor, strmDescriptorLimit)
	if err != nil {
		return "", err
	}
	return parseSTRMLocator(raw)
}

func parseSTRMLocator(raw []byte) (string, error) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	locator := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if locator != "" {
			return "", errors.New("STRM descriptor must contain exactly one locator")
		}
		locator = line
	}
	if locator == "" {
		return "", errors.New("STRM descriptor is empty")
	}
	parsed, err := url.Parse(locator)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("STRM descriptor must contain an HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return "", errors.New("STRM URL userinfo is not supported")
	}
	if _, err := validateExternalURL(locator); err != nil {
		return "", errors.New("STRM URL origin is not allowed")
	}
	return locator, nil
}

func isSTRMDescriptor(path string) bool {
	return strings.EqualFold(strings.TrimSpace(filepathExtension(path)), ".strm")
}
func filepathExtension(value string) string {
	idx := strings.LastIndex(strings.ReplaceAll(value, "\\", "/"), ".")
	if idx < 0 {
		return ""
	}
	ext := value[idx:]
	if strings.ContainsAny(ext, "/\\") {
		return ""
	}
	return ext
}

func strmDescriptorInfo(path string) (os.FileInfo, error) { return os.Stat(path) }
