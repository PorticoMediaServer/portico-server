package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	version     = "0.0.0-dev"
	buildNumber = "0"
)

type desktopStatus struct {
	Server       string `json:"server"`
	RemoteAccess string `json:"remoteAccess"`
	RemoteLabel  string `json:"remoteLabel"`
	CheckedAt    string `json:"checkedAt"`
	Detail       string `json:"-"`
}

type application struct {
	localURL  string
	hostedURL string
	client    *http.Client

	mu      sync.RWMutex
	current desktopStatus
	updates chan desktopStatus
}

func main() {
	localURL := strings.TrimRight(envOr("PORTICO_LOCAL_URL", "http://127.0.0.1:32500"), "/")
	if err := requireLoopbackURL(localURL); err != nil {
		fmt.Fprintln(os.Stderr, "Portico companion:", err)
		os.Exit(2)
	}
	app := &application{
		localURL:  localURL,
		hostedURL: envOr("PORTICO_HOSTED_WEB_URL", "https://web.getportico.tv"),
		client:    &http.Client{Timeout: 3 * time.Second},
		current:   desktopStatus{Server: "checking", RemoteAccess: "unknown", RemoteLabel: "Checking status"},
		updates:   make(chan desktopStatus, 1),
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go app.monitor(ctx)
	runDesktopUI(ctx, app)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func requireLoopbackURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
		return errors.New("PORTICO_LOCAL_URL must be an HTTP loopback URL")
	}
	host := parsed.Hostname()
	if !strings.EqualFold(host, "localhost") {
		address := net.ParseIP(host)
		if address == nil || !address.IsLoopback() {
			return errors.New("PORTICO_LOCAL_URL must resolve directly to loopback")
		}
	}
	return nil
}

func (a *application) monitor(ctx context.Context) {
	a.refresh(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refresh(ctx)
		}
	}
}

func (a *application) refresh(ctx context.Context) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.localURL+"/desktop/status", nil)
	if err != nil {
		return
	}
	response, err := a.client.Do(request)
	status := desktopStatus{Server: "stopped", RemoteAccess: "unknown", RemoteLabel: "Unavailable"}
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(&status) == nil && status.Server == "running" {
			status.Detail = ""
		} else {
			status.Detail = fmt.Sprintf("Local status returned HTTP %d", response.StatusCode)
		}
	} else {
		status.Detail = err.Error()
	}
	a.publish(status)
}

func (a *application) publish(status desktopStatus) {
	a.mu.Lock()
	a.current = status
	a.mu.Unlock()
	select {
	case a.updates <- status:
	default:
		select {
		case <-a.updates:
		default:
		}
		select {
		case a.updates <- status:
		default:
		}
	}
}

func (a *application) snapshot() desktopStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.current
}

func (a *application) runServiceAction(action serviceAction) {
	pendingStatus := map[serviceAction]string{
		serviceStart:   "starting",
		serviceStop:    "stopping",
		serviceRestart: "restarting",
	}[action]
	a.publish(desktopStatus{Server: pendingStatus, RemoteAccess: "unknown", RemoteLabel: "Waiting for Server"})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := controlService(ctx, action); err != nil {
			a.publish(desktopStatus{Server: "error", RemoteAccess: "unknown", RemoteLabel: "Service action failed", Detail: err.Error()})
			return
		}
		for attempt := 0; attempt < 12; attempt++ {
			time.Sleep(time.Second)
			a.refresh(ctx)
			current := a.snapshot()
			if action == serviceStop && current.Server == "stopped" {
				return
			}
			if action != serviceStop && current.Server == "running" {
				return
			}
		}
	}()
}

type serviceAction string

const (
	serviceStart   serviceAction = "start"
	serviceStop    serviceAction = "stop"
	serviceRestart serviceAction = "restart"
)

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

func serverStatusLabel(status string) string {
	switch status {
	case "running":
		return "Running"
	case "starting":
		return "Starting"
	case "stopping":
		return "Stopping"
	case "restarting":
		return "Restarting"
	case "error":
		return "Attention required"
	default:
		return "Stopped"
	}
}
