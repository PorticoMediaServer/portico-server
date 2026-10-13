//go:build !devtrust || release

package oidclogin

import "testing"

func TestNormalBuildRefusesDevelopmentAccountOrigin(t *testing.T) {
	if developmentAccountOrigin(Config{WebOrigin: "http://127.0.0.1:19410"}) {
		t.Fatal("normal build accepted a development account origin")
	}
}
