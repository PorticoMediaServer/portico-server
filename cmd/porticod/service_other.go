//go:build !windows

package main

import (
	"log/slog"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func runService(_ config.Config, _ *slog.Logger, options runOptions) (bool, error) {
	if options.service {
		return true, errServiceUnsupported
	}
	return false, nil
}
