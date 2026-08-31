//go:build linux

package main

import (
	"context"
	"fmt"
	"os/exec"
)

func controlService(ctx context.Context, action serviceAction) error {
	verb := string(action)
	command := exec.CommandContext(ctx, "systemctl", verb, "portico-media-server.service")
	if err := command.Run(); err == nil {
		return nil
	}
	if _, err := exec.LookPath("pkexec"); err != nil {
		return fmt.Errorf("%s Portico Server: system authorization is required", action)
	}
	if err := exec.CommandContext(ctx, "pkexec", "systemctl", verb, "portico-media-server.service").Run(); err != nil {
		return fmt.Errorf("%s Portico Server: %w", action, err)
	}
	return nil
}
