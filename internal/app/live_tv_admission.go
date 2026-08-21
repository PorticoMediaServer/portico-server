package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type liveTVEndpointApproval struct {
	Scheme  string
	Host    string
	Port    string
	Purpose string
	Zone    string
}

type liveTVResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

var liveTVMetadataPrefixes = []netip.Prefix{
	netip.MustParsePrefix("169.254.169.254/32"), netip.MustParsePrefix("169.254.170.2/32"),
	netip.MustParsePrefix("100.100.100.200/32"), netip.MustParsePrefix("fd00:ec2::254/128"),
}

func approveLiveTVEndpoint(raw, purpose string) (liveTVEndpointApproval, *url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return liveTVEndpointApproval{}, nil, err
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return liveTVEndpointApproval{}, nil, errors.New("unsupported source scheme")
	}
	if parsed.User != nil || parsed.Hostname() == "" || parsed.Fragment != "" {
		return liveTVEndpointApproval{}, nil, errors.New("source authority is invalid")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if numeric, err := strconv.Atoi(port); err != nil || numeric < 1 || numeric > 65535 {
		return liveTVEndpointApproval{}, nil, errors.New("source port is invalid")
	}
	zone := ""
	if addr, err := netip.ParseAddr(host); err == nil {
		zone = addr.Zone()
		if addr.Is6() && addr.IsLinkLocalUnicast() && zone == "" {
			return liveTVEndpointApproval{}, nil, errors.New("IPv6 link-local source requires an explicit zone")
		}
		if !addr.IsLinkLocalUnicast() && zone != "" {
			return liveTVEndpointApproval{}, nil, errors.New("zone is only valid for IPv6 link-local sources")
		}
	}
	return liveTVEndpointApproval{Scheme: parsed.Scheme, Host: host, Port: port, Purpose: strings.TrimSpace(purpose), Zone: zone}, parsed, nil
}

func (approval liveTVEndpointApproval) validateURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("derived URL contains credentials or fragment")
	}
	candidate, _, err := approveLiveTVEndpoint(parsed.String(), approval.Purpose)
	if err != nil {
		return nil, err
	}
	if candidate.Scheme != approval.Scheme || candidate.Host != approval.Host || candidate.Port != approval.Port || candidate.Zone != approval.Zone {
		return nil, errors.New("derived URL escaped approved source authority")
	}
	return parsed, nil
}

func isLiveTVMetadataAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range liveTVMetadataPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func resolveLiveTVApproval(ctx context.Context, approval liveTVEndpointApproval, resolver liveTVResolver) ([]netip.Addr, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	lookupHost := approval.Host
	if literal, err := netip.ParseAddr(lookupHost); err == nil {
		return validateLiveTVResolvedSet(approval, []netip.Addr{literal})
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", lookupHost)
	if err != nil {
		return nil, errors.New("source host could not be resolved")
	}
	return validateLiveTVResolvedSet(approval, addresses)
}

func validateLiveTVResolvedSet(approval liveTVEndpointApproval, addresses []netip.Addr) ([]netip.Addr, error) {
	if len(addresses) == 0 {
		return nil, errors.New("source host has no addresses")
	}
	seen := map[netip.Addr]bool{}
	result := make([]netip.Addr, 0, len(addresses))
	class := -1
	for _, raw := range addresses {
		addr := raw.Unmap()
		if !addr.IsValid() || addr.IsLoopback() || addr.IsUnspecified() || addr.IsMulticast() || isLiveTVMetadataAddress(addr) {
			return nil, errors.New("source resolved to a forbidden address")
		}
		if addr.Is6() && addr.IsLinkLocalUnicast() {
			if approval.Zone == "" {
				return nil, errors.New("IPv6 link-local source requires an explicit zone")
			}
			addr = addr.WithZone(approval.Zone)
		}
		currentClass := 0
		if addr.IsPrivate() || addr.IsLinkLocalUnicast() {
			currentClass = 1
		}
		if class >= 0 && class != currentClass {
			return nil, errors.New("source returned mixed public and private addresses")
		}
		class = currentClass
		if !seen[addr] {
			seen[addr] = true
			result = append(result, addr)
		}
	}
	return result, nil
}

func newApprovedLiveTVHTTPClient(ctx context.Context, approval liveTVEndpointApproval, resolver liveTVResolver) (*http.Client, error) {
	addresses, err := resolveLiveTVApproval(ctx, approval, resolver)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.MaxConnsPerHost = 16
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 30 * time.Second
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if strings.ToLower(strings.TrimSuffix(host, ".")) != approval.Host || port != approval.Port {
			return nil, errors.New("connection escaped approved source authority")
		}
		var last error
		for _, addr := range addresses {
			conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(dialCtx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many provider redirects")
		}
		_, err := approval.validateURL(req.URL.String())
		return err
	}
	return client, nil
}
