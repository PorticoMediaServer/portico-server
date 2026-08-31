//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
)

func controlService(ctx context.Context, action serviceAction) error {
	var script string
	switch action {
	case serviceStart:
		script = `Start-Process -FilePath sc.exe -ArgumentList @('start','PorticoMediaServer') -Verb RunAs -Wait`
	case serviceStop:
		script = `Start-Process -FilePath sc.exe -ArgumentList @('stop','PorticoMediaServer') -Verb RunAs -Wait`
	case serviceRestart:
		script = `$p=Start-Process -FilePath sc.exe -ArgumentList @('stop','PorticoMediaServer') -Verb RunAs -Wait -PassThru; Start-Sleep -Seconds 2; Start-Process -FilePath sc.exe -ArgumentList @('start','PorticoMediaServer') -Verb RunAs -Wait`
	default:
		return fmt.Errorf("unsupported service action %q", action)
	}
	if err := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script).Run(); err != nil {
		return fmt.Errorf("%s Portico Server service: %w", action, err)
	}
	return nil
}
