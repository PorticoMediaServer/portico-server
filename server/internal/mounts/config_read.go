package mounts

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrRemoteNotConfigured reports a remote name the server account's own
// configuration does not describe, or describes in a form Portico cannot import.
var ErrRemoteNotConfigured = errors.New("the remote is not in the server account's rclone configuration")

const (
	accountConfigLimit  = 1 << 20 // 1 MiB of INI is far more than a real account needs.
	sectionLimit        = 64 << 10
	configRemoteLimit   = 200
	encryptedValueMark  = "RCLONE_ENCRYPT_"
	configSectionMaxLen = 64
)

// ConfigRemote is one remote in the server account's own rclone configuration:
// its name, its declared type, and why Portico can or cannot import it. No value
// from its section is ever part of this projection.
type ConfigRemote struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
}

// AccountRcloneConfigPath is the server account's own rclone configuration, the
// same fixed place rclone looks. It is never chosen by a request.
func AccountRcloneConfigPath() (string, bool) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "", false
	}
	return filepath.Join(dir, "rclone", "rclone.conf"), true
}

// ReadAccountRcloneConfig lists the account configuration's remotes. A missing
// file is an empty list, not an error; anything unreadable is an error.
func ReadAccountRcloneConfig() (string, []ConfigRemote, error) {
	path, ok := AccountRcloneConfigPath()
	if !ok {
		return "", []ConfigRemote{}, nil
	}
	raw, err := readAccountConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, []ConfigRemote{}, nil
		}
		return path, nil, err
	}
	return path, parseConfigRemotes(string(raw)), nil
}

// ReadAccountRcloneSection returns the named section in plain rclone INI form,
// ready for the ordinary candidate test and save. Values are passed through
// untouched and never returned to a client.
func ReadAccountRcloneSection(remote string) (string, error) {
	if !configSectionName(remote) {
		return "", ErrConfigInvalid
	}
	path, ok := AccountRcloneConfigPath()
	if !ok {
		return "", ErrRemoteNotConfigured
	}
	raw, err := readAccountConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrRemoteNotConfigured
		}
		return "", err
	}
	lines := strings.Split(string(raw), "\n")
	inside := false
	typed := false
	encrypted := false
	section := make([]string, 0, 16)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inside {
				break
			}
			inside = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]") == remote
			continue
		}
		if !inside || trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "RCLONE_ENCRYPT_") || strings.Contains(trimmed, "="+encryptedValueMark) || strings.Contains(trimmed, "= "+encryptedValueMark) {
			encrypted = true
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			return "", ErrConfigInvalid
		}
		if strings.TrimSpace(key) == "type" && configTypeToken(strings.TrimSpace(value)) {
			typed = true
		}
		section = append(section, trimmed)
	}
	if !inside || !typed || encrypted || len(section) == 0 {
		return "", ErrRemoteNotConfigured
	}
	config := "[" + remote + "]\n" + strings.Join(section, "\n") + "\n"
	if len(config) > sectionLimit {
		return "", ErrConfigInvalid
	}
	if err = validateConfig(config, remote+":"); err != nil {
		return "", ErrConfigInvalid
	}
	return config, nil
}

func readAccountConfig(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, accountConfigLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > accountConfigLimit || !utf8.Valid(raw) {
		return nil, ErrConfigInvalid
	}
	return raw, nil
}

func parseConfigRemotes(text string) []ConfigRemote {
	out := make([]ConfigRemote, 0, 8)
	name, kind := "", ""
	encrypted := false
	flush := func() {
		if name == "" || len(out) >= configRemoteLimit {
			return
		}
		reason := ""
		switch {
		case encrypted:
			reason = "encrypted"
		case kind == "" || !configurationKinds[kind]:
			reason = "kind"
		}
		out = append(out, ConfigRemote{Name: name, Kind: kind, Supported: reason == "", Reason: reason})
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flush()
			candidate := strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			name, kind, encrypted = "", "", false
			if configSectionName(candidate) {
				name = candidate
			}
			continue
		}
		if name == "" || trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.Contains(trimmed, encryptedValueMark) {
			encrypted = true
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "type" {
			if token := strings.TrimSpace(value); configTypeToken(token) {
				kind = token
			}
		}
	}
	flush()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func configSectionName(name string) bool {
	if len(name) < 1 || len(name) > configSectionMaxLen {
		return false
	}
	for _, r := range name {
		if !(r == '-' || r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func configTypeToken(kind string) bool {
	if len(kind) < 1 || len(kind) > 32 {
		return false
	}
	for _, r := range kind {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
