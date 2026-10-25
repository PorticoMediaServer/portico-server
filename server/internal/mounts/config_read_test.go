package mounts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const accountConfigFixture = `# the server account's own rclone configuration
[nas]
type = sftp
host = storage.example
user = media

[archive]
type = s3
provider = AWS
access_key_id = public
secret_access_key = private-secret

[oldftp]
type = ftp
host = legacy.example

[wrapped]
type = crypt
remote = archive:private
password = RCLONE_ENCRYPT_V0:not-plaintext

[shortcut]
type = alias
remote = archive:private

[broken]
type = ftp
`

func writeAccountConfig(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rclone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rclone", "rclone.conf"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", root)
	return filepath.Join(root, "rclone", "rclone.conf")
}

func TestAccountConfigRemotesProjectNamesAndReasonsOnly(t *testing.T) {
	path := writeAccountConfig(t, accountConfigFixture)
	got, remotes, err := ReadAccountRcloneConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path %q, want %q", got, path)
	}
	want := []ConfigRemote{
		{Name: "archive", Kind: "s3", Supported: true},
		{Name: "broken", Kind: "ftp", Reason: "kind"},
		{Name: "nas", Kind: "sftp", Supported: true},
		{Name: "oldftp", Kind: "ftp", Reason: "kind"},
		{Name: "shortcut", Kind: "alias", Reason: "wrapper"},
		{Name: "wrapped", Kind: "crypt", Reason: "encrypted"},
	}
	if len(remotes) != len(want) {
		t.Fatalf("remotes %#v", remotes)
	}
	for index, remote := range remotes {
		if remote != want[index] {
			t.Fatalf("remote %d = %#v, want %#v", index, remote, want[index])
		}
	}
}

func TestAccountConfigRemotesAreEmptyWhenAbsent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, remotes, err := ReadAccountRcloneConfig()
	if err != nil {
		t.Fatal(err)
	}
	if path != "" || len(remotes) != 0 {
		t.Fatalf("path %q remotes %#v, want none", path, remotes)
	}
	if _, err = ReadAccountRcloneSection("archive"); !errors.Is(err, ErrRemoteNotConfigured) {
		t.Fatalf("section error %v, want ErrRemoteNotConfigured", err)
	}
}

func TestAccountConfigSectionIsPlainAndValidated(t *testing.T) {
	writeAccountConfig(t, accountConfigFixture)
	config, err := ReadAccountRcloneSection("archive")
	if err != nil {
		t.Fatal(err)
	}
	want := "[archive]\ntype = s3\nprovider = AWS\naccess_key_id = public\nsecret_access_key = private-secret\n"
	if config != want {
		t.Fatalf("config %q, want %q", config, want)
	}
	if err = validateConfig(config, "archive:"); err != nil {
		t.Fatalf("section is not valid for save: %v", err)
	}
	for _, remote := range []string{"oldftp", "wrapped", "missing", "broken", "bad name", "archive:"} {
		if _, err = ReadAccountRcloneSection(remote); err == nil {
			t.Fatalf("%q was accepted", remote)
		}
	}
}
