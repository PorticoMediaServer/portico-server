//go:build !devtrust || release

package oidclogin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalBuildRefusesDevelopmentAccountOrigin(t *testing.T) {
	if developmentAccountOrigin(Config{WebOrigin: "http://127.0.0.1:19410"}) {
		t.Fatal("normal build accepted a development account origin")
	}
	file := filepath.Join(t.TempDir(), "account.json")
	if err := os.WriteFile(file, []byte(`{"webOrigin":"http://127.0.0.1:19410"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(context.Background(), file); err == nil {
		t.Fatal("normal build loaded a development account origin")
	}
}
