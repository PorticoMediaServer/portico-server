//go:build windows

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "PorticoMediaServer"

type porticoWindowsService struct {
	cfg     config.Config
	logger  *slog.Logger
	options runOptions
}

func runService(cfg config.Config, logger *slog.Logger, options runOptions) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		logger.Warn("windows service detection failed", "error", err)
	}
	if !options.service && !isService {
		return false, nil
	}
	logger.Info("starting windows service dispatcher", "service", windowsServiceName)
	return true, svc.Run(windowsServiceName, &porticoWindowsService{cfg: cfg, logger: logger, options: options})
}

func (service *porticoWindowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	options := service.options
	options.service = true
	go func() {
		errCh <- run(ctx, service.cfg, service.logger, options)
	}()
	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				service.logger.Error("windows service runtime stopped", "error", err)
				changes <- svc.Status{State: svc.Stopped}
				return false, 1
			}
			service.logger.Info("windows service runtime stopped")
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				service.logger.Warn("windows service stop requested", "command", uint32(request.Cmd))
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case err := <-errCh:
					if err != nil {
						service.logger.Error("windows service stopped with error", "error", err)
						changes <- svc.Status{State: svc.Stopped}
						return false, 1
					}
					changes <- svc.Status{State: svc.Stopped}
					return false, 0
				case <-time.After(20 * time.Second):
					service.logger.Error("windows service stop timed out")
					changes <- svc.Status{State: svc.Stopped}
					return false, 2
				}
			default:
				service.logger.Warn("unsupported windows service control request", "command", uint32(request.Cmd))
			}
		}
	}
}
