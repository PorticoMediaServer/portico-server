package app

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

const porticoDiscoveryService = "_portico._tcp"

func (s *Server) runPorticoLocalDiscovery(ctx context.Context) {
	for {
		if err := s.servePorticoLocalDiscovery(ctx); err != nil && ctx.Err() == nil {
			s.recordLog("warn", "Portico local discovery stopped", map[string]string{"error": err.Error()})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func (s *Server) servePorticoLocalDiscovery(ctx context.Context) error {
	port := portFromAddress(s.cfg.Addr)
	if port <= 0 {
		return nil
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return err
	}
	settings, _ := s.remoteAccessSettings()
	system, _ := s.systemIdentity()
	name := firstNonEmpty(system.FriendlyName, settings.AssignedHostname, "Portico")
	text := porticoDiscoveryText(settings.ServerID, identity.Fingerprint, name)
	instance := sanitizeDiscoveryInstanceName(name)
	server, err := zeroconf.Register(instance, porticoDiscoveryService, "local.", port, text, nil)
	if err != nil {
		return err
	}
	s.recordLog("info", "Portico local discovery started", map[string]string{"service": porticoDiscoveryService, "port": strconv.Itoa(port)})
	<-ctx.Done()
	server.Shutdown()
	return nil
}

func porticoDiscoveryText(serverID, fingerprint, name string) []string {
	return []string{
		"txtVersion=1",
		"path=/",
		"scheme=http",
		"serverId=" + strings.TrimSpace(serverID),
		"fingerprint=" + strings.TrimSpace(fingerprint),
		"name=" + strings.TrimSpace(name),
	}
}

func sanitizeDiscoveryInstanceName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Portico"
	}
	replacer := strings.NewReplacer(".", "-", "_", "-", "/", "-", "\\", "-", "\n", " ", "\r", " ")
	value = replacer.Replace(value)
	if len(value) > 63 {
		value = value[:63]
	}
	return value
}

func localDiscoveryHTTPURLs(port int) []string {
	if port <= 0 {
		return nil
	}
	urls := []string{}
	for _, host := range localPrivateInterfaceHosts() {
		if strings.Contains(host, ":") {
			urls = append(urls, "http://"+net.JoinHostPort(host, strconv.Itoa(port)))
		} else {
			urls = append(urls, "http://"+host+":"+strconv.Itoa(port))
		}
	}
	return urls
}
