package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseLaunchersSelectProductionHostedAuthority(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	for _, source := range [][]string{
		{"packaging/macos/launcher.sh", "packaging/macos/tv.getportico.server.service.plist"},
		{"packaging/linux/portico-media-server.service"},
		{"packaging/windows/installer.nsi"},
	} {
		var combined strings.Builder
		for _, relative := range source {
			raw, err := os.ReadFile(filepath.Join(root, relative))
			if err != nil {
				t.Fatalf("read %s: %v", relative, err)
			}
			combined.Write(raw)
		}
		contents := combined.String()
		label := strings.Join(source, " + ")
		for _, required := range []string{"PORTICO_ENVIRONMENT", "production", "PORTICO_HOSTED_API_AUTHORITY", "https://api.getportico.tv"} {
			if !strings.Contains(contents, required) {
				t.Errorf("%s does not set %s", label, required)
			}
		}
		if strings.Contains(contents, "http://localhost:8080") {
			t.Errorf("%s contains the development Hosted authority", label)
		}
	}
}

func TestDesktopCompanionDoesNotOwnCrashRecovery(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	read := func(relative string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		return string(raw)
	}

	macService := read("packaging/macos/tv.getportico.server.service.plist")
	macCompanion := read("packaging/macos/tv.getportico.server.companion.plist")
	linuxService := read("packaging/linux/portico-media-server.service")
	windowsInstaller := read("packaging/windows/installer.nsi")

	for label, contents := range map[string]string{
		"macOS Server LaunchAgent": macService,
		"Linux systemd service":    linuxService,
		"Windows SCM installer":    windowsInstaller,
	} {
		if !strings.Contains(contents, map[string]string{
			"macOS Server LaunchAgent": "<key>KeepAlive</key><true/>",
			"Linux systemd service":    "Restart=on-failure",
			"Windows SCM installer":    "sc.exe failure PorticoMediaServer",
		}[label]) {
			t.Errorf("%s does not configure OS-owned crash recovery", label)
		}
	}
	if !strings.Contains(macCompanion, "<key>KeepAlive</key><false/>") {
		t.Error("macOS companion must not become a second crash-recovery authority")
	}
}
