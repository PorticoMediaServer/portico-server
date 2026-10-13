//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"portico.local/server/internal/supervise"
)

type windowsService struct{}

func serviceCommand() (bool, error) {
	if len(os.Args) != 2 || !strings.HasPrefix(os.Args[1], "--service") {
		return false, nil
	}
	if os.Args[1] != "--service" {
		return true, manageWindowsService(os.Args[1])
	}
	executable, err := os.Executable()
	if err != nil {
		return true, err
	}
	for key, value := range map[string]string{
		"PORTICO_STATE_DIR":  filepath.Join(os.Getenv("ProgramData"), "Portico Media Server"),
		"PORTICO_WEB_DIR":    filepath.Join(filepath.Dir(executable), "web"),
		"PORTICO_LOG_STDERR": "off",
		"PORTICO_BIND":       "[::]:32500",
	} {
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
	return true, svc.Run("PorticoMediaServer", windowsService{})
}

func manageWindowsService(action string) error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService("PorticoMediaServer")
	if action == "--service-install" {
		executable, exeErr := os.Executable()
		if exeErr != nil {
			return exeErr
		}
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			service, err = manager.CreateService("PorticoMediaServer", executable, mgr.Config{StartType: mgr.StartAutomatic, DisplayName: "Portico Media Server", Description: "Organizes and streams your media. Data is kept in ProgramData\\Portico Media Server.", ServiceStartName: "NT AUTHORITY\\LocalService"}, "--service")
		} else if err == nil {
			config, configErr := service.Config()
			if configErr != nil {
				service.Close()
				return configErr
			}
			config.BinaryPathName = windows.EscapeArg(executable) + " --service"
			err = service.UpdateConfig(config)
		}
		if err != nil {
			if service != nil {
				service.Close()
			}
			return err
		}
		defer service.Close()
		return service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}, {Type: mgr.NoAction}}, 86400)
	}
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) && (action == "--service-stop" || action == "--service-uninstall") {
		return nil
	}
	if err != nil {
		return err
	}
	defer service.Close()
	if action == "--service-start" {
		return service.Start()
	}
	if action != "--service-stop" && action != "--service-uninstall" {
		return errors.New("unknown service command")
	}
	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped {
		if _, err = service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return err
		}
		deadline := time.Now().Add(50 * time.Second)
		for status.State != svc.Stopped {
			if time.Now().After(deadline) {
				return errors.New("Portico did not finish stopping; installation files were left in place")
			}
			time.Sleep(200 * time.Millisecond)
			status, err = service.Query()
			if err != nil {
				return err
			}
		}
	}
	if action == "--service-uninstall" {
		return service.Delete()
	}
	return nil
}

func (windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status <- svc.Status{State: svc.StartPending}
	executable, err := os.Executable()
	if err != nil {
		return true, 1
	}
	done := make(chan error, 1)
	supervise.Go("service.worker", func() { done <- supervise.Process(ctx, executable, nil) })
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				cancel()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for checkpoint := uint32(1); ; checkpoint++ {
					status <- svc.Status{State: svc.StopPending, CheckPoint: checkpoint, WaitHint: 45000}
					select {
					case <-done:
						return false, 0
					case <-ticker.C:
					}
				}
			}
		}
	}
}
