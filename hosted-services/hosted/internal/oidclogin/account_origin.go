//go:build !devtrust || release

package oidclogin

func developmentAccountOrigin(Config) bool { return false }
