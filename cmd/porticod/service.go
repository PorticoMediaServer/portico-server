package main

import "errors"

var errServiceUnsupported = errors.New("service mode is only supported on Windows; use launchd on macOS or systemd on Linux")
