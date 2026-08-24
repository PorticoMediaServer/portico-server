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
	for _, relative := range []string{
		"packaging/macos/launcher.sh",
		"packaging/linux/portico-media-server.service",
		"packaging/windows/installer.nsi",
	} {
		raw, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		contents := string(raw)
		for _, required := range []string{"PORTICO_ENVIRONMENT", "production", "PORTICO_HOSTED_API_AUTHORITY", "https://api.getportico.tv"} {
			if !strings.Contains(contents, required) {
				t.Errorf("%s does not set %s", relative, required)
			}
		}
		if strings.Contains(contents, "http://localhost:8080") {
			t.Errorf("%s contains the development Hosted authority", relative)
		}
	}
}
