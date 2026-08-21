//go:build !tray

package main

import "errors"

var errTrayUnavailable = errors.New("tray support is not compiled into this binary")

func runTray(options trayOptions) error {
	return errTrayUnavailable
}

func stopTray() {}

func trayAvailable() bool {
	return false
}
