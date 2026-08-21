package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RouterMappingResult struct {
	Status   string
	Protocol string
	Error    string
}

type RouterMapper interface {
	AddMapping(ctx context.Context, internalPort, externalPort int, description string) RouterMappingResult
	DeleteMapping(ctx context.Context, internalPort, externalPort int) RouterMappingResult
}

var routerMapper RouterMapper = MultiRouterMapper{Mappers: []namedRouterMapper{
	{Name: "upnp", Mapper: UPnPRouterMapper{}},
	{Name: "nat-pmp", Mapper: NATPMPRouterMapper{}},
	{Name: "pcp", Mapper: PCPRouterMapper{}},
}}

type namedRouterMapper struct {
	Name   string
	Mapper RouterMapper
}

type MultiRouterMapper struct {
	Mappers []namedRouterMapper
}

func (m MultiRouterMapper) AddMapping(ctx context.Context, internalPort, externalPort int, description string) RouterMappingResult {
	if err := validateRouterPorts(internalPort, externalPort); err != nil {
		return RouterMappingResult{Status: "failed", Error: err.Error()}
	}
	var failures []string
	for _, candidate := range m.Mappers {
		result := candidate.Mapper.AddMapping(ctx, internalPort, externalPort, description)
		if result.Status == "mapped" {
			if result.Protocol == "" {
				result.Protocol = candidate.Name
			}
			return result
		}
		failures = append(failures, routerMappingFailure(candidate.Name, result))
	}
	return RouterMappingResult{Status: "unavailable", Error: strings.Join(failures, "; ")}
}

func (m MultiRouterMapper) DeleteMapping(ctx context.Context, internalPort, externalPort int) RouterMappingResult {
	if err := validateRouterPorts(internalPort, externalPort); err != nil {
		return RouterMappingResult{Status: "failed", Error: err.Error()}
	}
	var removedBy []string
	var failures []string
	for _, candidate := range m.Mappers {
		result := candidate.Mapper.DeleteMapping(ctx, internalPort, externalPort)
		if result.Status == "removed" {
			protocol := result.Protocol
			if protocol == "" {
				protocol = candidate.Name
			}
			removedBy = append(removedBy, protocol)
			continue
		}
		failures = append(failures, routerMappingFailure(candidate.Name, result))
	}
	if len(removedBy) > 0 {
		return RouterMappingResult{Status: "removed", Protocol: strings.Join(removedBy, ","), Error: strings.Join(failures, "; ")}
	}
	return RouterMappingResult{Status: "unavailable", Error: strings.Join(failures, "; ")}
}

func routerMappingFailure(name string, result RouterMappingResult) string {
	value := name + "=" + result.Status
	if result.Error != "" {
		value += ":" + result.Error
	}
	return value
}

func (s *Server) applyRouterMapping(ctx context.Context, settings RemoteAccessSettings) RouterMappingResult {
	result := routerMapper.AddMapping(ctx, portFromAddress(s.cfg.Addr), settings.ManualPublicPort, "Portico Remote Access")
	now := time.Now().UTC().Format(time.RFC3339)
	settings.RouterMappingStatus = result.Status
	settings.LastRouterMappingAt = now
	settings.RouterMappingError = result.Error
	_ = s.saveRemoteAccessSettings(settings)
	return result
}

func (s *Server) removeRouterMapping(ctx context.Context, settings RemoteAccessSettings) RouterMappingResult {
	if settings.ManualPublicPort <= 0 {
		return RouterMappingResult{Status: "skipped"}
	}
	return routerMapper.DeleteMapping(ctx, portFromAddress(s.cfg.Addr), settings.ManualPublicPort)
}

func shouldRemoveRouterMapping(previous, next RemoteAccessSettings) bool {
	if !previous.RouterAutomationEnabled || previous.PublicPortMode != "automatic" {
		return false
	}
	return !next.Enabled || next.PublicPortMode != "automatic" || !next.RouterAutomationEnabled
}

const (
	upnpWANIPConnectionV2  = "urn:schemas-upnp-org:service:WANIPConnection:2"
	upnpWANIPConnectionV1  = "urn:schemas-upnp-org:service:WANIPConnection:1"
	upnpWANPPPConnectionV1 = "urn:schemas-upnp-org:service:WANPPPConnection:1"
)

type upnpService struct {
	ControlURL  string
	ServiceType string
}

var upnpHTTPClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		Proxy:       nil,
		DialContext: safeUPnPDialContext,
	},
	CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		return validateUPnPURL(req.URL)
	},
}

func safeUPnPDialContext(ctx context.Context, network, endpoint string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse UPnP endpoint: %w", err)
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve UPnP endpoint: %w", err)
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	var lastDialErr error
	for _, address := range addresses {
		address = address.Unmap()
		if !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() {
			continue
		}
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastDialErr = dialErr
	}
	if lastDialErr != nil {
		return nil, fmt.Errorf("connect to private UPnP endpoint: %w", lastDialErr)
	}
	return nil, fmt.Errorf("UPnP endpoint %q did not resolve to a private or link-local address", host)
}

func validateUPnPURL(endpoint *url.URL) error {
	if endpoint == nil || endpoint.Scheme != "http" || endpoint.Hostname() == "" || endpoint.User != nil {
		return errors.New("UPnP endpoints must be unauthenticated HTTP URLs on the local network")
	}
	if address := net.ParseIP(endpoint.Hostname()); address != nil && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() {
		return errors.New("UPnP endpoints must resolve to a private or link-local address")
	}
	return nil
}

type UPnPRouterMapper struct {
	discover func(context.Context) (upnpService, error)
	localIP  func() (string, error)
	soap     func(context.Context, string, string, string) error
}

func (m UPnPRouterMapper) dependencies() (func(context.Context) (upnpService, error), func() (string, error), func(context.Context, string, string, string) error) {
	discover := m.discover
	if discover == nil {
		discover = discoverUPnPService
	}
	localIP := m.localIP
	if localIP == nil {
		localIP = outboundLocalIP
	}
	soap := m.soap
	if soap == nil {
		soap = upnpSOAP
	}
	return discover, localIP, soap
}

func (m UPnPRouterMapper) AddMapping(ctx context.Context, internalPort, externalPort int, description string) RouterMappingResult {
	if err := validateRouterPorts(internalPort, externalPort); err != nil {
		return RouterMappingResult{Status: "failed", Protocol: "upnp", Error: err.Error()}
	}
	discover, localIPForMapping, soap := m.dependencies()
	service, err := discover(ctx)
	if err != nil {
		return RouterMappingResult{Status: "unavailable", Protocol: "upnp", Error: err.Error()}
	}
	localIP, err := localIPForMapping()
	if err != nil {
		return RouterMappingResult{Status: "failed", Protocol: "upnp", Error: "determine internal client: " + err.Error()}
	}
	add := func(lease time.Duration) error {
		body := fmt.Sprintf(`<u:AddPortMapping xmlns:u="%s"><NewRemoteHost></NewRemoteHost><NewExternalPort>%d</NewExternalPort><NewProtocol>TCP</NewProtocol><NewInternalPort>%d</NewInternalPort><NewInternalClient>%s</NewInternalClient><NewEnabled>1</NewEnabled><NewPortMappingDescription>%s</NewPortMappingDescription><NewLeaseDuration>%d</NewLeaseDuration></u:AddPortMapping>`, service.ServiceType, externalPort, internalPort, xmlEscape(localIP), xmlEscape(description), int(lease.Seconds()))
		return soap(ctx, service.ControlURL, fmt.Sprintf(`"%s#AddPortMapping"`, service.ServiceType), body)
	}
	// Portico has no reason to let an active mapping silently expire. A zero
	// lease is the UPnP IGD permanent lease and is explicitly removed when
	// automatic remote access is disabled.
	err = add(0)
	if err != nil {
		return RouterMappingResult{Status: "failed", Protocol: "upnp", Error: err.Error()}
	}
	return RouterMappingResult{Status: "mapped", Protocol: "upnp"}
}

func (m UPnPRouterMapper) DeleteMapping(ctx context.Context, internalPort, externalPort int) RouterMappingResult {
	if err := validateRouterPorts(internalPort, externalPort); err != nil {
		return RouterMappingResult{Status: "failed", Protocol: "upnp", Error: err.Error()}
	}
	discover, _, soap := m.dependencies()
	service, err := discover(ctx)
	if err != nil {
		return RouterMappingResult{Status: "unavailable", Protocol: "upnp", Error: err.Error()}
	}
	body := fmt.Sprintf(`<u:DeletePortMapping xmlns:u="%s"><NewRemoteHost></NewRemoteHost><NewExternalPort>%d</NewExternalPort><NewProtocol>TCP</NewProtocol></u:DeletePortMapping>`, service.ServiceType, externalPort)
	err = soap(ctx, service.ControlURL, fmt.Sprintf(`"%s#DeletePortMapping"`, service.ServiceType), body)
	var soapErr *upnpSOAPError
	if err != nil && !(errors.As(err, &soapErr) && soapErr.Code == 714) {
		return RouterMappingResult{Status: "failed", Protocol: "upnp", Error: err.Error()}
	}
	return RouterMappingResult{Status: "removed", Protocol: "upnp"}
}

func validateRouterPorts(internalPort, externalPort int) error {
	if internalPort < 1 || internalPort > 65535 {
		return fmt.Errorf("internal port %d is outside 1-65535", internalPort)
	}
	if externalPort < 1 || externalPort > 65535 {
		return fmt.Errorf("external port %d is outside 1-65535", externalPort)
	}
	return nil
}

type NATPMPRouterMapper struct{}

func (NATPMPRouterMapper) AddMapping(ctx context.Context, internalPort, externalPort int, description string) RouterMappingResult {
	request := natPMPMappingRequest(internalPort, externalPort, 2*time.Hour)
	response, err := routerUDPRequest(ctx, request)
	if err != nil {
		return RouterMappingResult{Status: "unavailable", Error: err.Error()}
	}
	if len(response) < 16 || response[0] != 0 || response[1] != 130 {
		return RouterMappingResult{Status: "failed", Error: "invalid NAT-PMP response"}
	}
	resultCode := binary.BigEndian.Uint16(response[2:4])
	if resultCode != 0 {
		return RouterMappingResult{Status: "failed", Error: fmt.Sprintf("NAT-PMP result %d", resultCode)}
	}
	return RouterMappingResult{Status: "mapped"}
}

func (NATPMPRouterMapper) DeleteMapping(ctx context.Context, internalPort, externalPort int) RouterMappingResult {
	request := natPMPMappingRequest(internalPort, externalPort, 0)
	response, err := routerUDPRequest(ctx, request)
	if err != nil {
		return RouterMappingResult{Status: "unavailable", Error: err.Error()}
	}
	if len(response) < 16 || response[0] != 0 || response[1] != 130 {
		return RouterMappingResult{Status: "failed", Error: "invalid NAT-PMP response"}
	}
	if resultCode := binary.BigEndian.Uint16(response[2:4]); resultCode != 0 {
		return RouterMappingResult{Status: "failed", Error: fmt.Sprintf("NAT-PMP result %d", resultCode)}
	}
	return RouterMappingResult{Status: "removed"}
}

type PCPRouterMapper struct{}

func (PCPRouterMapper) AddMapping(ctx context.Context, internalPort, externalPort int, description string) RouterMappingResult {
	request, err := pcpMappingRequest(internalPort, externalPort, 2*time.Hour)
	if err != nil {
		return RouterMappingResult{Status: "failed", Error: err.Error()}
	}
	response, err := routerUDPRequest(ctx, request)
	if err != nil {
		return RouterMappingResult{Status: "unavailable", Error: err.Error()}
	}
	if len(response) < 24 || response[0] != 2 || response[1] != 129 {
		return RouterMappingResult{Status: "failed", Error: "invalid PCP response"}
	}
	if response[3] != 0 {
		return RouterMappingResult{Status: "failed", Error: fmt.Sprintf("PCP result %d", response[3])}
	}
	return RouterMappingResult{Status: "mapped"}
}

func (PCPRouterMapper) DeleteMapping(ctx context.Context, internalPort, externalPort int) RouterMappingResult {
	request, err := pcpMappingRequest(internalPort, externalPort, 0)
	if err != nil {
		return RouterMappingResult{Status: "failed", Error: err.Error()}
	}
	response, err := routerUDPRequest(ctx, request)
	if err != nil {
		return RouterMappingResult{Status: "unavailable", Error: err.Error()}
	}
	if len(response) < 24 || response[0] != 2 || response[1] != 129 {
		return RouterMappingResult{Status: "failed", Error: "invalid PCP response"}
	}
	if response[3] != 0 {
		return RouterMappingResult{Status: "failed", Error: fmt.Sprintf("PCP result %d", response[3])}
	}
	return RouterMappingResult{Status: "removed"}
}

func discoverUPnPService(ctx context.Context) (upnpService, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	deadline, ok := ctx.Deadline()
	timeout := 3 * time.Second
	if ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
	}
	if timeout <= 0 {
		return upnpService{}, ctx.Err()
	}
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return upnpService{}, fmt.Errorf("listen for UPnP discovery: %w", err)
	}
	defer conn.Close()
	addr, _ := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	searchTargets := []string{
		"urn:schemas-upnp-org:device:InternetGatewayDevice:2",
		"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
		upnpWANIPConnectionV2,
		upnpWANIPConnectionV1,
		upnpWANPPPConnectionV1,
	}
	var writeErrs []string
	for _, target := range searchTargets {
		search := strings.Join([]string{
			"M-SEARCH * HTTP/1.1",
			"HOST: 239.255.255.250:1900",
			`MAN: "ssdp:discover"`,
			"MX: 2",
			"ST: " + target,
			"",
			"",
		}, "\r\n")
		if _, err := conn.WriteTo([]byte(search), addr); err != nil {
			writeErrs = append(writeErrs, err.Error())
		}
	}
	if len(writeErrs) == len(searchTargets) {
		return upnpService{}, fmt.Errorf("send UPnP discovery: %s", strings.Join(writeErrs, "; "))
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	buf := make([]byte, 8192)
	seenLocations := make(map[string]struct{})
	var descriptionErrs []string
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return upnpService{}, fmt.Errorf("UPnP discovery: %w", ctx.Err())
			}
			if len(descriptionErrs) > 0 {
				return upnpService{}, fmt.Errorf("UPnP gateway found but no supported WAN service was usable: %s", strings.Join(descriptionErrs, "; "))
			}
			return upnpService{}, fmt.Errorf("UPnP gateway was not discovered")
		}
		location := routerSSDPHeader(string(buf[:n]), "location")
		if location == "" {
			continue
		}
		if _, seen := seenLocations[location]; seen {
			continue
		}
		seenLocations[location] = struct{}{}
		service, err := upnpServiceFromDescription(ctx, location)
		if err == nil {
			return service, nil
		}
		descriptionErrs = append(descriptionErrs, err.Error())
	}
}

func controlURLFromDescription(ctx context.Context, location string) (string, error) {
	service, err := upnpServiceFromDescription(ctx, location)
	if err != nil {
		return "", err
	}
	return service.ControlURL, nil
}

type upnpRootDescription struct {
	URLBase string                `xml:"URLBase"`
	Device  upnpDeviceDescription `xml:"device"`
}

type upnpDeviceDescription struct {
	Services []upnpServiceDescription `xml:"serviceList>service"`
	Devices  []upnpDeviceDescription  `xml:"deviceList>device"`
}

type upnpServiceDescription struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

func upnpServiceFromDescription(ctx context.Context, location string) (upnpService, error) {
	locationURL, err := url.Parse(location)
	if err != nil {
		return upnpService{}, fmt.Errorf("parse UPnP description location: %w", err)
	}
	if err := validateUPnPURL(locationURL); err != nil {
		return upnpService{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, locationURL.String(), nil)
	if err != nil {
		return upnpService{}, fmt.Errorf("create UPnP description request: %w", err)
	}
	resp, err := upnpHTTPClient.Do(req)
	if err != nil {
		return upnpService{}, fmt.Errorf("fetch UPnP description: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return upnpService{}, fmt.Errorf("fetch UPnP description: HTTP %s", resp.Status)
	}
	var root upnpRootDescription
	err = xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&root)
	if err != nil {
		return upnpService{}, fmt.Errorf("decode UPnP description: %w", err)
	}
	services := collectUPnPServices(root.Device)
	var selected upnpServiceDescription
	for _, supportedType := range []string{upnpWANIPConnectionV2, upnpWANIPConnectionV1, upnpWANPPPConnectionV1} {
		for _, candidate := range services {
			if strings.TrimSpace(candidate.ServiceType) == supportedType && strings.TrimSpace(candidate.ControlURL) != "" {
				selected = candidate
				break
			}
		}
		if selected.ServiceType != "" {
			break
		}
	}
	if selected.ServiceType == "" {
		return upnpService{}, fmt.Errorf("supported WANIPConnection or WANPPPConnection service not found")
	}
	base := locationURL
	if strings.TrimSpace(root.URLBase) != "" {
		urlBase, err := url.Parse(strings.TrimSpace(root.URLBase))
		if err != nil {
			return upnpService{}, fmt.Errorf("parse UPnP URLBase: %w", err)
		}
		base = urlBase
	}
	parsed, err := url.Parse(strings.TrimSpace(selected.ControlURL))
	if err != nil {
		return upnpService{}, fmt.Errorf("parse UPnP controlURL: %w", err)
	}
	controlURL := base.ResolveReference(parsed)
	if err := validateUPnPURL(controlURL); err != nil {
		return upnpService{}, err
	}
	return upnpService{ControlURL: controlURL.String(), ServiceType: strings.TrimSpace(selected.ServiceType)}, nil
}

func collectUPnPServices(device upnpDeviceDescription) []upnpServiceDescription {
	services := append([]upnpServiceDescription(nil), device.Services...)
	for _, child := range device.Devices {
		services = append(services, collectUPnPServices(child)...)
	}
	return services
}

func upnpSOAP(ctx context.Context, controlURL, action, body string) error {
	envelope := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` + body + `</s:Body></s:Envelope>`
	endpoint, err := url.Parse(controlURL)
	if err != nil {
		return fmt.Errorf("parse UPnP control URL: %w", err)
	}
	if err := validateUPnPURL(endpoint); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader([]byte(envelope)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", action)
	resp, err := upnpHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("read UPnP SOAP response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		soapErr := parseUPnPSOAPError(responseBody)
		soapErr.HTTPStatus = resp.Status
		return soapErr
	}
	return nil
}

type upnpSOAPError struct {
	HTTPStatus string
	Code       int
	Detail     string
}

func (e *upnpSOAPError) Error() string {
	message := "UPnP SOAP returned " + e.HTTPStatus
	if e.Code != 0 {
		message += fmt.Sprintf(" (error %d", e.Code)
		if e.Detail != "" {
			message += ": " + e.Detail
		}
		message += ")"
	}
	return message
}

func parseUPnPSOAPError(body []byte) *upnpSOAPError {
	var fault struct {
		Code        int    `xml:"Body>Fault>detail>UPnPError>errorCode"`
		Description string `xml:"Body>Fault>detail>UPnPError>errorDescription"`
	}
	_ = xml.Unmarshal(body, &fault)
	return &upnpSOAPError{Code: fault.Code, Detail: strings.TrimSpace(fault.Description)}
}

func outboundLocalIP() (string, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("local UDP address unavailable")
	}
	return addr.IP.String(), nil
}

func natPMPMappingRequest(internalPort, externalPort int, lifetime time.Duration) []byte {
	request := make([]byte, 12)
	request[0] = 0
	request[1] = 2
	binary.BigEndian.PutUint16(request[4:6], uint16(internalPort))
	binary.BigEndian.PutUint16(request[6:8], uint16(externalPort))
	binary.BigEndian.PutUint32(request[8:12], uint32(lifetime.Seconds()))
	return request
}

func pcpMappingRequest(internalPort, externalPort int, lifetime time.Duration) ([]byte, error) {
	localIP, err := outboundLocalIP()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return pcpMappingRequestForIP(internalPort, externalPort, lifetime, localIP, nonce)
}

func pcpMappingRequestForIP(internalPort, externalPort int, lifetime time.Duration, localIP string, nonce []byte) ([]byte, error) {
	if len(nonce) != 12 {
		return nil, fmt.Errorf("PCP nonce must be 12 bytes")
	}
	request := make([]byte, 60)
	request[0] = 2
	request[1] = 1
	binary.BigEndian.PutUint32(request[4:8], uint32(lifetime.Seconds()))
	clientIP := net.ParseIP(localIP)
	if clientIP == nil {
		return nil, fmt.Errorf("local IP is invalid")
	}
	copy(request[8:24], clientIP.To16())
	copy(request[24:36], nonce)
	request[36] = 6
	binary.BigEndian.PutUint16(request[40:42], uint16(internalPort))
	binary.BigEndian.PutUint16(request[42:44], uint16(externalPort))
	return request, nil
}

func routerUDPRequest(ctx context.Context, request []byte) ([]byte, error) {
	gateways, err := likelyGatewayAddresses()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, gateway := range gateways {
		response, err := udpRequest(ctx, gateway+":5351", request)
		if err == nil {
			return response, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no likely gateway addresses")
	}
	return nil, lastErr
}

func udpRequest(ctx context.Context, address string, request []byte) ([]byte, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp4", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(1500 * time.Millisecond)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}
	response := make([]byte, 128)
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}
	return response[:n], nil
}

func likelyGatewayAddresses() ([]string, error) {
	localIP, err := outboundLocalIP()
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(localIP).To4()
	if ip == nil {
		return nil, fmt.Errorf("local IPv4 address unavailable")
	}
	candidates := []string{
		fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2]),
		fmt.Sprintf("%d.%d.%d.254", ip[0], ip[1], ip[2]),
	}
	return candidates, nil
}

func routerSSDPHeader(response, name string) string {
	name = strings.ToLower(name) + ":"
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), name) {
			return strings.TrimSpace(line[len(name):])
		}
	}
	return ""
}

func xmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	value = strings.ReplaceAll(value, "'", "&apos;")
	return value
}

func routerMappingStatusText(settings RemoteAccessSettings) string {
	if settings.RouterMappingStatus == "" {
		return "not_configured"
	}
	if settings.RouterMappingError != "" {
		return settings.RouterMappingStatus + ":" + strconv.Quote(settings.RouterMappingError)
	}
	return settings.RouterMappingStatus
}
