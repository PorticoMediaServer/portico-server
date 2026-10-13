//go:build devtrust && !release

package oidclogin

import (
	"net/netip"
	"net/url"
	"strconv"
)

// Local account pages must be able to exercise the canonical cookie and consent
// paths. This development override cannot configure external identity providers.
func developmentAccountOrigin(c Config) bool {
	if c.GoogleClientID != "" || c.GoogleClientSecret != "" || c.AppleClientID != "" ||
		c.AppleNativeClientID != "" || len(c.AppleNativeClientIDs) != 0 ||
		len(c.AppleNotificationAudiences) != 0 || c.AppleTeamID != "" ||
		c.AppleKeyID != "" || c.AppleKeyFile != "" {
		return false
	}
	u, err := url.Parse(c.WebOrigin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	ip, err := netip.ParseAddr(u.Hostname())
	port, portErr := strconv.Atoi(u.Port())
	return err == nil && ip.IsLoopback() && !ip.Is4In6() && ip.Zone() == "" &&
		portErr == nil && port > 0 && port <= 65535 && strconv.Itoa(port) == u.Port() &&
		u.String() == c.WebOrigin
}
