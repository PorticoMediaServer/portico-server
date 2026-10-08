package mounts

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"golang.org/x/crypto/nacl/secretbox"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrUnavailable = errors.New("managed storage is unavailable; start the mount and retry")
var ErrInvalid = errors.New("invalid managed mount configuration")
var ErrConfigInvalid = errors.New("Check the remote name, supported provider type, and rclone configuration format.")
var ErrExecutableInvalid = errors.New("Choose an available native rclone executable with secure file permissions that passes version validation.")
var remotePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}:[^\r\n\x00]*$`)

func validateConfig(config, remote string) error {
	if len(config) == 0 || len(config) > 64<<10 || !remotePattern.MatchString(remote) || len(remote) > 2048 || !utf8.ValidString(remote) || strings.IndexFunc(remote, unicode.IsControl) >= 0 {
		return ErrInvalid
	}
	wanted := strings.SplitN(remote, ":", 2)[0]
	section := ""
	found := false
	typed := false
	allowed := map[string]bool{"s3": true, "b2": true, "drive": true, "onedrive": true, "dropbox": true, "webdav": true, "http": true, "smb": true, "sftp": true, "crypt": true}
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if section != "" && !typed {
				return ErrInvalid
			}
			section = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			typed = false
			if section == wanted {
				found = true
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if !ok || section == "" || strings.ContainsAny(line, "\x00\r") || strings.Contains(key, "command") || key == "shell_type" || key == "serve" || key == "env_auth" && value != "false" {
			return ErrInvalid
		}
		if key == "type" {
			if !allowed[value] {
				return ErrInvalid
			}
			typed = true
		}
	}
	if !found || !typed {
		return ErrInvalid
	}
	return nil
}

// sealedConfigPrefix marks the removed encrypted format. Mount configs are
// rclone's plain config format now; files still sealed fail validation, and
// the startup migration converts them while the old key exists.
const sealedConfigPrefix = "RCLONE_ENCRYPT_V0:\n"

// migrateSealedConfigs converts sealed mount configs to plaintext while the
// old key exists, drops stale write candidates, and removes the key.
// Anything it cannot read is left; reads fail validation at use, and the
// server still starts.
func migrateSealedConfigs(private string) {
	keyPath := filepath.Join(private, "key")
	raw, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil || len(raw) != 32 {
		return
	}
	password := hex.EncodeToString(raw)
	entries, err := os.ReadDir(private)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".candidate") {
			_ = os.Remove(filepath.Join(private, name))
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".conf") {
			continue
		}
		path := filepath.Join(private, name)
		sealed, err := os.ReadFile(path)
		if err != nil || !strings.HasPrefix(string(sealed), sealedConfigPrefix) {
			continue
		}
		plain, err := openSealedConfig(sealed, password)
		if err != nil {
			continue
		}
		_ = os.WriteFile(path, plain, 0600)
	}
	// Best effort: a leftover key is inert and migrates again next start.
	_ = os.Remove(keyPath)
}
func executableDigest(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrInvalid
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != filepath.Clean(path) {
		return "", errors.New("rclone executable must be a canonical regular executable path")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 || info.Size() > 256<<20 {
		return "", ErrInvalid
	}
	var head [4]byte
	if _, err = io.ReadFull(f, head[:]); err != nil {
		return "", ErrInvalid
	}
	// Owner-selected native binary only: never a shell script.
	magic := hex.EncodeToString(head[:])
	if magic != "7f454c46" && magic != "cffaedfe" && magic != "feedfacf" && magic != "cafebabe" && magic != "bebafeca" {
		return "", errors.New("rclone must be a native executable")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// openSealedConfig decrypts one sealed-era config for the migration.
func openSealedConfig(sealed []byte, password string) ([]byte, error) {
	const prefix = "RCLONE_ENCRYPT_V0:\n"
	if !strings.HasPrefix(string(sealed), prefix) {
		return nil, ErrConfigInvalid
	}
	box, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sealed[len(prefix):])))
	if err != nil || len(box) < 24 {
		return nil, ErrConfigInvalid
	}
	key := sha256.Sum256([]byte("[" + password + "][rclone-config]"))
	var nonce [24]byte
	copy(nonce[:], box[:24])
	raw, ok := secretbox.Open(nil, box[24:], &nonce, &key)
	if !ok {
		return nil, ErrConfigInvalid
	}
	return raw, nil
}
