//go:build darwin || windows

package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/slytomcat/systray"
)

func runDesktopUI(ctx context.Context, app *application) {
	systray.Run(func() {
		icon := playPIconPNG()
		regular := icon
		if runtime.GOOS == "windows" {
			regular = pngAsICO(icon)
		}
		systray.SetTemplateIcon(icon, regular)
		systray.SetTitle("")
		systray.SetTooltip("Portico Media Server")

		heading := systray.AddMenuItem("Portico Media Server", fmt.Sprintf("Version %s (%s)", version, buildNumber))
		heading.Disable()
		serverStatus := systray.AddMenuItem("Server: Checking", "Current local Server status")
		serverStatus.Disable()
		remoteStatus := systray.AddMenuItem("Remote Access: Checking", "Last status reported by the Server")
		remoteStatus.Disable()
		systray.AddSeparator()
		openLocal := systray.AddMenuItem("Open Portico Server", "Open the local Server on this computer")
		openHosted := systray.AddMenuItem("Open Portico Web", "Open web.getportico.tv")
		openSettings := systray.AddMenuItem("Server Settings", "Open local Server settings")
		systray.AddSeparator()
		restart := systray.AddMenuItem("Restart Server", "Restart the managed Portico Server service")
		startStop := systray.AddMenuItem("Stop Server", "Stop the managed Portico Server service")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit Portico Menu", "Hide this menu without stopping the Server")

		apply := func(status desktopStatus) {
			serverStatus.SetTitle("Server: " + serverStatusLabel(status.Server))
			remoteStatus.SetTitle("Remote Access: " + status.RemoteLabel)
			if status.Server == "running" {
				startStop.SetTitle("Stop Server")
				restart.Enable()
				openLocal.Enable()
				openSettings.Enable()
			} else {
				startStop.SetTitle("Start Server")
				restart.Disable()
				openLocal.Disable()
				openSettings.Disable()
			}
		}
		apply(app.snapshot())

		go func() {
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case status := <-app.updates:
					apply(status)
				case <-openLocal.ClickedCh:
					_ = openBrowser(app.localURL)
				case <-openHosted.ClickedCh:
					_ = openBrowser(app.hostedURL)
				case <-openSettings.ClickedCh:
					_ = openBrowser(app.localURL + "/settings/server/status?tab=identity")
				case <-restart.ClickedCh:
					app.runServiceAction(serviceRestart)
				case <-startStop.ClickedCh:
					if app.snapshot().Server == "running" {
						app.runServiceAction(serviceStop)
					} else {
						app.runServiceAction(serviceStart)
					}
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
