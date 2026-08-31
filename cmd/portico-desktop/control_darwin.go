//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const macServerLabel = "tv.getportico.server.service"

func controlService(ctx context.Context, action serviceAction) error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	plist := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", macServerLabel+".plist")
	switch action {
	case serviceStop:
		_ = exec.CommandContext(ctx, "launchctl", "disable", domain+"/"+macServerLabel).Run()
		if err := exec.CommandContext(ctx, "launchctl", "bootout", domain+"/"+macServerLabel).Run(); err != nil {
			return fmt.Errorf("stop Portico Server: %w", err)
		}
		return nil
	case serviceStart:
		if _, err := os.Stat(plist); err != nil {
			return fmt.Errorf("Portico Server launch agent is not installed: %w", err)
		}
		_ = exec.CommandContext(ctx, "launchctl", "enable", domain+"/"+macServerLabel).Run()
		_ = exec.CommandContext(ctx, "launchctl", "bootstrap", domain, plist).Run()
		if err := exec.CommandContext(ctx, "launchctl", "kickstart", domain+"/"+macServerLabel).Run(); err != nil {
			return fmt.Errorf("start Portico Server: %w", err)
		}
		return nil
	case serviceRestart:
		if err := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", domain+"/"+macServerLabel).Run(); err != nil {
			return fmt.Errorf("restart Portico Server: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported service action %q", action)
	}
}
