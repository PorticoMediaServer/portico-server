//go:build tray

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/slytomcat/systray"
)

var errTrayUnavailable = errors.New("tray support is unavailable")

func runTray(options trayOptions) error {
	done := make(chan struct{})
	systray.Run(func() {
		onTrayReady(options)
	}, func() {
		close(done)
	})
	<-done
	return nil
}

func stopTray() {
	systray.Quit()
}

func trayAvailable() bool {
	return true
}

func onTrayReady(options trayOptions) {
	if icon, err := loadTrayIcon(); err == nil {
		systray.SetIcon(icon)
	}
	systray.SetTitle("Portico")
	systray.SetTooltip("Portico Media Server - " + options.serverURL)

	status := systray.AddMenuItem("Portico Media Server", options.serverURL)
	status.Disable()
	address := systray.AddMenuItem("Address: "+options.address, options.serverURL)
	address.Disable()
	systray.AddSeparator()
	openServer := systray.AddMenuItem("Open Server", "Open Portico in your browser")
	openSettings := systray.AddMenuItem("Server Settings", "Open Portico server settings")
	restartServer := systray.AddMenuItem("Restart Server", "Restart the Portico server process")
	systray.AddSeparator()
	exitServer := systray.AddMenuItem("Exit Server", "Stop Portico and remove the tray icon")

	go func() {
		for {
			select {
			case <-openServer.ClickedCh:
				_ = openBrowser(options.serverURL)
			case <-openSettings.ClickedCh:
				_ = openBrowser(options.settingsURL)
			case <-restartServer.ClickedCh:
				restartServer.Disable()
				restartServer.SetTitle("Restarting Server...")
				options.onRestart()
			case <-exitServer.ClickedCh:
				exitServer.Disable()
				exitServer.SetTitle("Exiting Server...")
				options.onExit()
				systray.Quit()
				return
			}
		}
	}()
}

func loadTrayIcon() ([]byte, error) {
	roots := []string{}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	if executable, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(executable)
		roots = append(roots, exeDir, filepath.Dir(exeDir))
	}
	for _, root := range roots {
		for _, relative := range []string{
			filepath.Join("web", "public", "brand", "portico-app-icon.png"),
			filepath.Join("web", "dist", "brand", "portico-app-icon.png"),
			filepath.Join("dist", "web", "brand", "portico-app-icon.png"),
		} {
			if icon, err := os.ReadFile(filepath.Join(root, relative)); err == nil {
				return icon, nil
			}
		}
	}
	return nil, os.ErrNotExist
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return command.Start()
}
