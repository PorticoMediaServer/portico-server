package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNATPMPMappingRequest(t *testing.T) {
	request := natPMPMappingRequest(32400, 32401, 2*time.Hour)

	if len(request) != 12 {
		t.Fatalf("request length = %d", len(request))
	}
	if request[0] != 0 || request[1] != 2 {
		t.Fatalf("unexpected NAT-PMP header: %#v", request[:2])
	}
	if reserved := binary.BigEndian.Uint16(request[2:4]); reserved != 0 {
		t.Fatalf("reserved field = %d", reserved)
	}
	if internal := binary.BigEndian.Uint16(request[4:6]); internal != 32400 {
		t.Fatalf("internal port = %d", internal)
	}
	if external := binary.BigEndian.Uint16(request[6:8]); external != 32401 {
		t.Fatalf("external port = %d", external)
	}
	if lifetime := binary.BigEndian.Uint32(request[8:12]); lifetime != 7200 {
		t.Fatalf("lifetime = %d", lifetime)
	}
}

func TestPCPMappingRequestForIP(t *testing.T) {
	nonce := []byte("abcdefghijkl")
	request, err := pcpMappingRequestForIP(32400, 32401, 2*time.Hour, "192.168.10.25", nonce)
	if err != nil {
		t.Fatalf("pcp request: %v", err)
	}

	if len(request) != 60 {
		t.Fatalf("request length = %d", len(request))
	}
	if request[0] != 2 || request[1] != 1 {
		t.Fatalf("unexpected PCP header: %#v", request[:2])
	}
	if lifetime := binary.BigEndian.Uint32(request[4:8]); lifetime != 7200 {
		t.Fatalf("lifetime = %d", lifetime)
	}
	if !bytes.Equal(request[20:24], []byte{192, 168, 10, 25}) {
		t.Fatalf("client IPv4 bytes = %#v", request[8:24])
	}
	if !bytes.Equal(request[24:36], nonce) {
		t.Fatalf("nonce = %q", string(request[24:36]))
	}
	if protocol := request[36]; protocol != 6 {
		t.Fatalf("protocol = %d", protocol)
	}
	if internal := binary.BigEndian.Uint16(request[40:42]); internal != 32400 {
		t.Fatalf("internal port = %d", internal)
	}
	if external := binary.BigEndian.Uint16(request[42:44]); external != 32401 {
		t.Fatalf("external port = %d", external)
	}
}

func TestPCPMappingRequestForIPRejectsInvalidNonce(t *testing.T) {
	if _, err := pcpMappingRequestForIP(32400, 32401, time.Hour, "192.168.10.25", []byte("short")); err == nil {
		t.Fatalf("expected nonce validation error")
	}
}

func TestUPnPServiceFromDescriptionPrefersWANIPV2AndResolvesURLBase(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <URLBase>%s/gateway/</URLBase>
  <device><deviceList><device><serviceList>
    <service><serviceType>%s</serviceType><controlURL>/wan-ip-v1</controlURL></service>
    <service><serviceType>%s</serviceType><controlURL>wan-ip-v2</controlURL></service>
  </serviceList></device></deviceList></device>
</root>`, server.URL, upnpWANIPConnectionV1, upnpWANIPConnectionV2)
	}))
	defer server.Close()

	service, err := upnpServiceFromDescription(t.Context(), server.URL+"/description.xml")
	if err != nil {
		t.Fatal(err)
	}
	if service.ServiceType != upnpWANIPConnectionV2 {
		t.Fatalf("expected WANIPConnection v2, got %q", service.ServiceType)
	}
	if service.ControlURL != server.URL+"/gateway/wan-ip-v2" {
		t.Fatalf("unexpected control URL %q", service.ControlURL)
	}
}

func TestUPnPServiceFromDescriptionSupportsWANPPP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<root><device><serviceList><service><serviceType>%s</serviceType><controlURL>/ppp/control</controlURL></service></serviceList></device></root>`, upnpWANPPPConnectionV1)
	}))
	defer server.Close()

	service, err := upnpServiceFromDescription(t.Context(), server.URL+"/description.xml")
	if err != nil {
		t.Fatal(err)
	}
	if service.ServiceType != upnpWANPPPConnectionV1 || service.ControlURL != server.URL+"/ppp/control" {
		t.Fatalf("unexpected service: %#v", service)
	}
}

func TestUPnPRejectsNonLocalOrAuthenticatedControlURLs(t *testing.T) {
	for _, raw := range []string{
		"https://192.168.1.1/control",
		"http://user:password@192.168.1.1/control",
		"file:///etc/passwd",
	} {
		endpoint, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse test endpoint: %v", err)
		}
		if err := validateUPnPURL(endpoint); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<root><URLBase>http://203.0.113.10/</URLBase><device><serviceList><service><serviceType>%s</serviceType><controlURL>/control</controlURL></service></serviceList></device></root>`, upnpWANIPConnectionV1)
	}))
	defer server.Close()
	if _, err := upnpServiceFromDescription(t.Context(), server.URL+"/description.xml"); err == nil {
		t.Fatal("expected a public UPnP control endpoint to be rejected")
	}
}

func TestUPnPAddMappingUsesAdvertisedServiceAndPermanentLease(t *testing.T) {
	var actions []string
	var bodies []string
	mapper := UPnPRouterMapper{
		discover: func(context.Context) (upnpService, error) {
			return upnpService{ControlURL: "http://router.test/control", ServiceType: upnpWANIPConnectionV2}, nil
		},
		localIP: func() (string, error) { return "192.168.50.20", nil },
		soap: func(_ context.Context, controlURL, action, body string) error {
			if controlURL != "http://router.test/control" {
				t.Fatalf("unexpected control URL %q", controlURL)
			}
			actions = append(actions, action)
			bodies = append(bodies, body)
			return nil
		},
	}

	result := mapper.AddMapping(t.Context(), 32500, 32501, `Portico & Friends`)
	if result.Status != "mapped" || result.Protocol != "upnp" || result.Error != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(actions) != 1 || actions[0] != `"`+upnpWANIPConnectionV2+`#AddPortMapping"` {
		t.Fatalf("unexpected SOAP actions: %#v", actions)
	}
	if !strings.Contains(bodies[0], "<NewExternalPort>32501</NewExternalPort>") || !strings.Contains(bodies[0], "<NewInternalPort>32500</NewInternalPort>") {
		t.Fatalf("mapping ports missing from request: %s", bodies[0])
	}
	if !strings.Contains(bodies[0], "<NewLeaseDuration>0</NewLeaseDuration>") {
		t.Fatalf("expected permanent mapping request: %#v", bodies)
	}
	if !strings.Contains(bodies[0], "Portico &amp; Friends") {
		t.Fatalf("description was not XML escaped: %s", bodies[0])
	}
}

func TestUPnPDeleteMappingIsIdempotentForMissingEntry(t *testing.T) {
	var body string
	mapper := UPnPRouterMapper{
		discover: func(context.Context) (upnpService, error) {
			return upnpService{ControlURL: "http://router.test/control", ServiceType: upnpWANPPPConnectionV1}, nil
		},
		soap: func(_ context.Context, _, action, requestBody string) error {
			if action != `"`+upnpWANPPPConnectionV1+`#DeletePortMapping"` {
				t.Fatalf("unexpected action %q", action)
			}
			body = requestBody
			return &upnpSOAPError{HTTPStatus: "500 Internal Server Error", Code: 714, Detail: "NoSuchEntryInArray"}
		},
	}

	result := mapper.DeleteMapping(t.Context(), 32500, 32501)
	if result.Status != "removed" || result.Protocol != "upnp" || result.Error != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(body, "<NewExternalPort>32501</NewExternalPort>") || strings.Contains(body, "32500") {
		t.Fatalf("delete request must identify the external mapping: %s", body)
	}
}

func TestUPnPSOAPReportsFaultCodeAndDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>718</errorCode><errorDescription>ConflictInMappingEntry</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`))
	}))
	defer server.Close()

	err := upnpSOAP(t.Context(), server.URL, `"service#AddPortMapping"`, `<u:AddPortMapping/>`)
	var soapErr *upnpSOAPError
	if !errors.As(err, &soapErr) {
		t.Fatalf("expected typed SOAP error, got %T: %v", err, err)
	}
	if soapErr.Code != 718 || soapErr.Detail != "ConflictInMappingEntry" {
		t.Fatalf("unexpected SOAP fault: %#v", soapErr)
	}
	if !strings.Contains(err.Error(), "error 718: ConflictInMappingEntry") {
		t.Fatalf("fault details missing from error: %v", err)
	}
}

func TestUPnPMapperRejectsInvalidPortsBeforeDiscovery(t *testing.T) {
	called := false
	mapper := UPnPRouterMapper{discover: func(context.Context) (upnpService, error) {
		called = true
		return upnpService{}, nil
	}}

	result := mapper.AddMapping(t.Context(), 0, 32500, "Portico")
	if result.Status != "failed" || !strings.Contains(result.Error, "internal port") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if called {
		t.Fatal("discovery should not run for invalid ports")
	}
}
