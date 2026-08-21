package app

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	dlnaRootDeviceURN         = "upnp:rootdevice"
	dlnaMediaServerURN        = "urn:schemas-upnp-org:device:MediaServer:1"
	dlnaContentDirectoryURN   = "urn:schemas-upnp-org:service:ContentDirectory:1"
	dlnaConnectionManagerURN  = "urn:schemas-upnp-org:service:ConnectionManager:1"
	dlnaContentDirectoryID    = "urn:upnp-org:serviceId:ContentDirectory"
	dlnaConnectionManagerID   = "urn:upnp-org:serviceId:ConnectionManager"
	dlnaMulticastAddr         = "239.255.255.250:1900"
	dlnaServerHeader          = "Portico/0.1 UPnP/1.1 DLNADOC/1.50"
	dlnaDefaultDiscoveryEvery = 60
	dlnaDefaultLeaseSeconds   = 1800
	dlnaDefaultProtocolInfo   = "http-get:*:video/mp4:DLNA.ORG_OP=01;DLNA.ORG_CI=0,http-get:*:audio/mpeg:DLNA.ORG_OP=01;DLNA.ORG_CI=0"
	dlnaDefaultBrowsePageSize = 100
	dlnaMaxBrowsePageSize     = 200
	dlnaMaxBrowsePerClient    = 4
	dlnaRendererProfileV1     = "dlna-renderer-profile-v1"
	dlnaRemoteHeaderTimeout   = 15 * time.Second
	dlnaRemoteTotalTimeout    = 12 * time.Hour
)

type dlnaRendererProfile struct {
	Version        string
	ID             string
	Client         PlaybackClientProfile
	ReachableRoute map[string]bool
}

type dlnaRendererProfileContextKey struct{}

type dlnaResource struct {
	SourceURL   string
	ContentType string
	Protocol    string
}

type dlnaConfig struct {
	Enabled           bool
	FriendlyName      string
	AdvertiseURL      string
	ExposedLibraries  map[string]bool
	DiscoveryInterval time.Duration
	LeaseSeconds      int
	ProtocolInfo      string
	ServerID          string
	UUID              string
}

func (s *Server) handleDLNAStatus(w http.ResponseWriter, r *http.Request, user User) {
	if !canInteractivelyManageServer(user) {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view DLNA settings.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	cfg, err := s.dlnaConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dlna_status_failed", "Unable to load DLNA status.")
		return
	}
	libraries, err := s.listLibraries()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "libraries_failed", "Unable to load libraries.")
		return
	}
	exposed := make([]DLNALibraryExposure, 0, len(libraries))
	for _, library := range libraries {
		exposed = append(exposed, DLNALibraryExposure{
			ID:      library.ID,
			Name:    library.Name,
			Type:    library.Type,
			Count:   library.Count,
			Exposed: cfg.libraryExposed(library.ID),
		})
	}
	baseURL := s.dlnaBaseURL(r, cfg)
	ssdpStatus := "disabled"
	if cfg.Enabled {
		ssdpStatus = "active when UDP port 1900 can be joined on an eligible LAN interface"
	}
	writeJSON(w, http.StatusOK, DLNAStatus{
		Enabled:                     cfg.Enabled,
		FriendlyName:                cfg.FriendlyName,
		AdvertiseURL:                s.dlnaAdvertiseURL(cfg),
		DeviceDescriptionURL:        baseURL + "/dlna/device.xml",
		ContentDirectoryURL:         baseURL + "/dlna/content-directory",
		MediaServerURN:              dlnaMediaServerURN,
		SSDPDiscovery:               ssdpStatus,
		ExposedLibraries:            exposed,
		UnauthenticatedLANAccess:    cfg.Enabled,
		ByteRangeStreamingSupported: true,
		DiscoveryIntervalSeconds:    int(cfg.DiscoveryInterval / time.Second),
		AnnouncementLeaseSeconds:    cfg.LeaseSeconds,
		ProtocolInfo:                cfg.ProtocolInfo,
		RendererProfileVersion:      dlnaRendererProfileV1,
		ReachableProtocols:          []string{"http"},
		Note:                        "DLNA is a local-network compatibility protocol. When enabled, selected libraries are browsable without Portico user authentication by devices that can reach this server.",
	})
}

func (s *Server) handleDLNADeviceDescription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := s.requireDLNA(w, r)
	if !ok {
		return
	}
	baseURL := s.dlnaBaseURL(r, cfg)
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <URLBase>%s/</URLBase>
  <device>
    <deviceType>%s</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>Portico</manufacturer>
    <manufacturerURL>https://portico.local/</manufacturerURL>
    <modelDescription>Portico Media Server DLNA bridge</modelDescription>
    <modelName>Portico Media Server</modelName>
    <modelNumber>0.1</modelNumber>
    <serialNumber>%s</serialNumber>
    <UDN>uuid:%s</UDN>
    <serviceList>
      <service>
        <serviceType>%s</serviceType>
        <serviceId>%s</serviceId>
        <SCPDURL>/dlna/content-directory.xml</SCPDURL>
        <controlURL>/dlna/content-directory</controlURL>
        <eventSubURL>/dlna/content-directory/events</eventSubURL>
      </service>
      <service>
        <serviceType>%s</serviceType>
        <serviceId>%s</serviceId>
        <SCPDURL>/dlna/connection-manager.xml</SCPDURL>
        <controlURL>/dlna/connection-manager</controlURL>
        <eventSubURL>/dlna/connection-manager/events</eventSubURL>
      </service>
    </serviceList>
  </device>
</root>`, dlnaXMLText(strings.TrimRight(baseURL, "/")), dlnaMediaServerURN, dlnaXMLText(cfg.FriendlyName), dlnaXMLText(cfg.ServerID), dlnaXMLText(cfg.UUID), dlnaContentDirectoryURN, dlnaContentDirectoryID, dlnaConnectionManagerURN, dlnaConnectionManagerID)
	writeXML(w, http.StatusOK, body)
}

func (s *Server) handleDLNAContentDirectoryDescription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.requireDLNA(w, r); !ok {
		return
	}
	writeXML(w, http.StatusOK, dlnaContentDirectorySCPD)
}

func (s *Server) handleDLNAConnectionManagerDescription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.requireDLNA(w, r); !ok {
		return
	}
	writeXML(w, http.StatusOK, dlnaConnectionManagerSCPD)
}

func (s *Server) handleDLNAContentDirectoryControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := s.requireDLNA(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeDLNASOAPFault(w, 501, "Unable to read SOAP request")
		return
	}
	body := string(bodyBytes)
	action := strings.ToLower(r.Header.Get("SOAPAction") + " " + body)
	switch {
	case strings.Contains(action, "getsystemupdateid"):
		writeDLNASOAPResponse(w, "GetSystemUpdateID", dlnaContentDirectoryURN, "<Id>1</Id>")
	case strings.Contains(action, "getsortcapabilities"):
		writeDLNASOAPResponse(w, "GetSortCapabilities", dlnaContentDirectoryURN, "<SortCaps>dc:title,dc:date</SortCaps>")
	case strings.Contains(action, "getsearchcapabilities"):
		writeDLNASOAPResponse(w, "GetSearchCapabilities", dlnaContentDirectoryURN, "<SearchCaps>dc:title,upnp:class</SearchCaps>")
	case strings.Contains(action, "browse"):
		release, ok := s.tryAcquireDLNABrowseClient(r)
		if !ok {
			w.Header().Set("Retry-After", "1")
			writeDLNASOAPFault(w, 501, "Server is busy handling DLNA browse requests")
			return
		}
		defer release()
		objectID := soapValue(body, "ObjectID")
		if objectID == "" {
			objectID = "0"
		}
		flag := soapValue(body, "BrowseFlag")
		startingIndex := parseSOAPInt(soapValue(body, "StartingIndex"), 0)
		requestedCount := parseSOAPInt(soapValue(body, "RequestedCount"), 200)
		didl, totalMatches, numberReturned, err := s.dlnaBrowse(r, cfg, objectID, flag, startingIndex, requestedCount)
		if err != nil {
			writeDLNASOAPFault(w, 701, "No such object")
			return
		}
		result := fmt.Sprintf(
			"<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches><UpdateID>1</UpdateID>",
			dlnaXMLText(didl),
			numberReturned,
			totalMatches,
		)
		writeDLNASOAPResponse(w, "Browse", dlnaContentDirectoryURN, result)
	default:
		writeDLNASOAPFault(w, 401, "Invalid Action")
	}
}

func (s *Server) tryAcquireDLNABrowseClient(r *http.Request) (func(), bool) {
	clientID := clientIPFromRequest(r)
	if clientID == "" {
		clientID = "unknown"
	}
	s.dlnaMu.Lock()
	if s.dlnaBrowseActive == nil {
		s.dlnaBrowseActive = map[string]int{}
	}
	if s.dlnaBrowseActive[clientID] >= dlnaMaxBrowsePerClient {
		s.dlnaMu.Unlock()
		s.dlnaBrowseRejected.Add(1)
		return nil, false
	}
	s.dlnaBrowseActive[clientID]++
	s.dlnaMu.Unlock()
	return func() {
		s.dlnaMu.Lock()
		defer s.dlnaMu.Unlock()
		if s.dlnaBrowseActive[clientID] <= 1 {
			delete(s.dlnaBrowseActive, clientID)
			return
		}
		s.dlnaBrowseActive[clientID]--
	}, true
}

func (s *Server) handleDLNAConnectionManagerControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := s.requireDLNA(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	action := strings.ToLower(r.Header.Get("SOAPAction") + " " + string(bodyBytes))
	switch {
	case strings.Contains(action, "getprotocolinfo"):
		writeDLNASOAPResponse(w, "GetProtocolInfo", dlnaConnectionManagerURN, "<Source>"+dlnaXMLText(cfg.ProtocolInfo)+"</Source><Sink></Sink>")
	case strings.Contains(action, "getcurrentconnectionids"):
		writeDLNASOAPResponse(w, "GetCurrentConnectionIDs", dlnaConnectionManagerURN, "<ConnectionIDs>0</ConnectionIDs>")
	case strings.Contains(action, "getcurrentconnectioninfo"):
		writeDLNASOAPResponse(w, "GetCurrentConnectionInfo", dlnaConnectionManagerURN, "<RcsID>-1</RcsID><AVTransportID>-1</AVTransportID><ProtocolInfo></ProtocolInfo><PeerConnectionManager></PeerConnectionManager><PeerConnectionID>-1</PeerConnectionID><Direction>Output</Direction><Status>OK</Status>")
	default:
		writeDLNASOAPFault(w, 401, "Invalid Action")
	}
}

func (s *Server) handleDLNAMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg, ok := s.requireDLNA(w, r)
	if !ok {
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/dlna/media/"))
	if err != nil || strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return
	}
	item, err := s.dlnaMediaItemContext(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !cfg.libraryExposed(item.LibraryID) || dlnaContainerType(item.Type) {
		http.NotFound(w, r)
		return
	}
	profile, err := resolveDLNARendererProfile(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	resource, ok := dlnaResourceForRenderer(item, profile)
	if !ok {
		http.NotFound(w, r)
		return
	}
	sourceURL := resource.SourceURL
	parsed, err := url.Parse(sourceURL)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		s.proxyDLNAMedia(w, r, item, resource)
		return
	}
	path := sourceURL
	if err == nil && parsed.Scheme == "file" {
		path = parsed.Path
	}
	path, err = s.validateLocalMediaPath(item.LibraryID, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", resource.ContentType)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(path), stat.ModTime(), file)
}

func (s *Server) dlnaBrowse(r *http.Request, cfg dlnaConfig, objectID, browseFlag string, startingIndex, requestedCount int) (string, int, int, error) {
	profile, err := resolveDLNARendererProfile(r)
	if err != nil {
		return "", 0, 0, err
	}
	rendererContext := context.WithValue(r.Context(), dlnaRendererProfileContextKey{}, profile)
	browseMetadata := strings.EqualFold(browseFlag, "BrowseMetadata")
	baseURL := s.dlnaBaseURL(r, cfg)
	switch {
	case objectID == "0" || objectID == "":
		libraries, err := s.dlnaLibraries(cfg)
		if err != nil {
			return "", 0, 0, err
		}
		if browseMetadata {
			didl := dlnaDIDL([]string{dlnaContainer("0", "-1", cfg.FriendlyName, "object.container", len(libraries))})
			return didl, 1, 1, nil
		}
		selected := paginateLibraries(libraries, startingIndex, requestedCount)
		nodes := make([]string, 0, len(selected))
		for _, library := range selected {
			nodes = append(nodes, dlnaContainer("library:"+library.ID, "0", library.Name, "object.container.storageFolder", library.Count))
		}
		return dlnaDIDL(nodes), len(libraries), len(selected), nil
	case strings.HasPrefix(objectID, "library:"):
		libraryID := strings.TrimPrefix(objectID, "library:")
		library, err := s.getLibrary(libraryID)
		if err != nil || !cfg.libraryExposed(libraryID) {
			return "", 0, 0, errors.New("library not found")
		}
		if browseMetadata {
			didl := dlnaDIDL([]string{dlnaContainer("library:"+library.ID, "0", library.Name, "object.container.storageFolder", library.Count)})
			return didl, 1, 1, nil
		}
		items, total, err := s.dlnaLibraryItemsContext(rendererContext, libraryID, startingIndex, requestedCount)
		if err != nil {
			return "", 0, 0, err
		}
		nodes, err := s.dlnaMediaNodesContext(rendererContext, items, "library:"+libraryID, baseURL)
		if err != nil {
			return "", 0, 0, err
		}
		return dlnaDIDL(nodes), total, len(nodes), nil
	case strings.HasPrefix(objectID, "item:"):
		itemID := strings.TrimPrefix(objectID, "item:")
		item, err := s.dlnaMediaItemContext(rendererContext, itemID)
		if err != nil || !cfg.libraryExposed(item.LibraryID) {
			return "", 0, 0, errors.New("item not found")
		}
		parentID := "library:" + item.LibraryID
		if item.ParentID != "" {
			parentID = "item:" + item.ParentID
		}
		if browseMetadata || !dlnaContainerType(item.Type) {
			nodes, err := s.dlnaMediaNodesContext(rendererContext, []MediaItem{item}, parentID, baseURL)
			if err != nil {
				return "", 0, 0, err
			}
			return dlnaDIDL(nodes), len(nodes), len(nodes), nil
		}
		children, total, err := s.dlnaChildItemsContext(rendererContext, item.ID, startingIndex, requestedCount)
		if err != nil {
			return "", 0, 0, err
		}
		nodes, err := s.dlnaMediaNodesContext(rendererContext, children, "item:"+item.ID, baseURL)
		if err != nil {
			return "", 0, 0, err
		}
		return dlnaDIDL(nodes), total, len(nodes), nil
	default:
		return "", 0, 0, errors.New("object not found")
	}
}

func (s *Server) dlnaMediaNodes(items []MediaItem, parentID, baseURL string) ([]string, error) {
	return s.dlnaMediaNodesContext(context.Background(), items, parentID, baseURL)
}

func (s *Server) dlnaMediaNodesContext(ctx context.Context, items []MediaItem, parentID, baseURL string) ([]string, error) {
	nodes := make([]string, 0, len(items))
	childCounts, err := s.dlnaChildCountsContext(ctx, items)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		childCount := childCounts[item.ID]
		if dlnaContainerType(item.Type) || childCount > 0 {
			nodes = append(nodes, dlnaContainer("item:"+item.ID, parentID, item.Title, dlnaContainerClass(item.Type), childCount))
			continue
		}
		profile, ok := ctx.Value(dlnaRendererProfileContextKey{}).(dlnaRendererProfile)
		if !ok {
			var err error
			profile, err = resolveDLNARendererProfile(nil)
			if err != nil {
				continue
			}
		}
		resource, ok := dlnaResourceForRenderer(item, profile)
		if !ok {
			continue
		}
		nodes = append(nodes, dlnaItem("item:"+item.ID, parentID, item, strings.TrimRight(baseURL, "/")+"/dlna/media/"+url.PathEscape(item.ID), resource.Protocol))
	}
	return nodes, nil
}

func (s *Server) proxyDLNAMedia(w http.ResponseWriter, r *http.Request, item MediaItem, resource dlnaResource) {
	client, origin, err := dlnaRemoteHTTPClient(r.Context(), resource.SourceURL)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	requestContext, cancel := context.WithTimeout(r.Context(), dlnaRemoteTotalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, r.Method, origin.String(), nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Unable to open media stream.", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		http.Error(w, "Unable to open remote media stream.", http.StatusBadGateway)
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := resp.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	upstreamType := strings.ToLower(strings.TrimSpace(strings.Split(w.Header().Get("Content-Type"), ";")[0]))
	if upstreamType != "" && upstreamType != "application/octet-stream" && upstreamType != resource.ContentType {
		for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
			w.Header().Del(key)
		}
		http.Error(w, "Remote media type did not match analyzed media facts.", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", resource.ContentType)
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		if err := copyRemotePlaybackBody(requestContext, w, resp.Body); err != nil && requestContext.Err() == nil {
			s.recordLog("warn", "DLNA remote source copy failed", map[string]string{"mediaId": item.ID, "error": err.Error()})
		}
	}
}

func dlnaResourceForRenderer(item MediaItem, profile dlnaRendererProfile) (dlnaResource, bool) {
	var file MediaFileVersion
	for _, candidate := range item.MediaFiles {
		if !candidate.Available {
			continue
		}
		if file.ID == "" || candidate.Selected || (item.SourceURL != "" && candidate.Path == item.SourceURL) {
			file = candidate
		}
		if candidate.Selected || (item.SourceURL != "" && candidate.Path == item.SourceURL) {
			break
		}
	}
	if file.ID == "" || strings.TrimSpace(file.Path) == "" || file.Analysis != "analyzed" {
		return dlnaResource{}, false
	}
	container := strings.ToLower(strings.TrimSpace(file.Container))
	videoCodec := strings.ToLower(strings.TrimSpace(file.VideoCodec))
	audioCodec := strings.ToLower(strings.TrimSpace(file.AudioCodec))
	if videoCodec == "avc" || videoCodec == "avc1" {
		videoCodec = "h264"
	}
	if audioCodec == "mp4a" {
		audioCodec = "aac"
	}
	contentType := ""
	protocol := ""
	switch {
	case container == "mp4" && videoCodec == "h264" && audioCodec == "aac":
		contentType = "video/mp4"
		protocol = "http-get:*:video/mp4:DLNA.ORG_OP=01;DLNA.ORG_CI=0"
	case container == "mp3" && videoCodec == "" && audioCodec == "mp3":
		contentType = "audio/mpeg"
		protocol = "http-get:*:audio/mpeg:DLNA.ORG_OP=01;DLNA.ORG_CI=0"
	default:
		return dlnaResource{}, false
	}
	resolution, err := resolvePlaybackCapabilities(profile.Client)
	if err != nil {
		return dlnaResource{}, false
	}
	for _, tuple := range resolution.Tuples {
		if tuple.Protocol != "http" || tuple.Container != container {
			continue
		}
		if videoCodec != "" && (tuple.Video.Codec != videoCodec || (file.Width > 0 && tuple.Video.MaxWidth > 0 && file.Width > tuple.Video.MaxWidth) || (file.Height > 0 && tuple.Video.MaxHeight > 0 && file.Height > tuple.Video.MaxHeight)) {
			continue
		}
		if tuple.Audio.Codec == audioCodec && (file.AudioChannels <= 0 || tuple.Audio.MaxChannels <= 0 || file.AudioChannels <= tuple.Audio.MaxChannels) {
			return dlnaResource{SourceURL: file.Path, ContentType: contentType, Protocol: protocol}, true
		}
	}
	return dlnaResource{}, false
}

type dlnaIPLookup func(context.Context, string) ([]net.IPAddr, error)

func dlnaRemoteHTTPClient(ctx context.Context, rawURL string) (*http.Client, *url.URL, error) {
	origin, err := validateExternalURL(rawURL)
	if err != nil || origin.User != nil {
		return nil, nil, errors.New("DLNA remote source authority is not allowed")
	}
	pinned, err := resolveDLNAOrigin(ctx, origin, net.DefaultResolver.LookupIPAddr)
	if err != nil {
		return nil, nil, err
	}
	originAuthority := canonicalDLNAAuthority(origin)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = dlnaRemoteHeaderTimeout
	transport.MaxConnsPerHost = 8
	transport.MaxIdleConnsPerHost = 4
	transport.DialContext = func(dialContext context.Context, network, address string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(address)
		if splitErr != nil || canonicalDLNAHostPort(host, port, origin.Scheme) != originAuthority {
			return nil, errors.New("DLNA remote source attempted to change authority")
		}
		dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		return dialer.DialContext(dialContext, network, net.JoinHostPort(pinned.String(), port))
	}
	client := &http.Client{
		Transport:     transport,
		Timeout:       dlnaRemoteTotalTimeout,
		CheckRedirect: dlnaRedirectPolicy(originAuthority),
	}
	return client, origin, nil
}

func dlnaRedirectPolicy(originAuthority string) func(*http.Request, []*http.Request) error {
	return func(next *http.Request, via []*http.Request) error {
		if next == nil || len(via) >= 5 || next.URL.User != nil || canonicalDLNAAuthority(next.URL) != originAuthority {
			return errors.New("DLNA remote source redirect changed authority")
		}
		next.Header.Del("Authorization")
		next.Header.Del("Proxy-Authorization")
		next.Header.Del("Cookie")
		next.Header.Del("Referer")
		return nil
	}
}

func resolveDLNAOrigin(ctx context.Context, origin *url.URL, lookup dlnaIPLookup) (netip.Addr, error) {
	if origin == nil || lookup == nil {
		return netip.Addr{}, errors.New("DLNA remote source origin is missing")
	}
	host := strings.Trim(origin.Hostname(), "[]")
	if literal, err := netip.ParseAddr(host); err == nil {
		literal = literal.Unmap().WithZone("")
		if isUnsafeAddr(literal) {
			return netip.Addr{}, errors.New("DLNA remote source resolved to a blocked network")
		}
		return literal, nil
	}
	addresses, err := lookup(ctx, host)
	if err != nil || len(addresses) == 0 {
		return netip.Addr{}, errors.New("DLNA remote source could not be resolved")
	}
	var pinned netip.Addr
	for _, resolved := range addresses {
		address, ok := netip.AddrFromSlice(resolved.IP)
		if !ok {
			return netip.Addr{}, errors.New("DLNA remote source returned an invalid address")
		}
		address = address.Unmap().WithZone("")
		// Reject the whole resolution set on one unsafe answer. Selecting only a
		// safe member would permit mixed-answer rebinding.
		if isUnsafeAddr(address) {
			return netip.Addr{}, errors.New("DLNA remote source returned mixed or blocked addresses")
		}
		if !pinned.IsValid() {
			pinned = address
		}
	}
	if !pinned.IsValid() {
		return netip.Addr{}, errors.New("DLNA remote source had no usable address")
	}
	return pinned, nil
}

func canonicalDLNAAuthority(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else if parsed.Scheme == "http" {
			port = "80"
		}
	}
	return canonicalDLNAHostPort(parsed.Hostname(), port, parsed.Scheme)
}

func canonicalDLNAHostPort(host, port, scheme string) string {
	return strings.ToLower(strings.TrimSpace(scheme)) + "://" + net.JoinHostPort(strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]")), port)
}

func (s *Server) dlnaLibraries(cfg dlnaConfig) ([]Library, error) {
	libraries, err := s.listLibraries()
	if err != nil {
		return nil, err
	}
	exposed := make([]Library, 0, len(libraries))
	for _, library := range libraries {
		if cfg.libraryExposed(library.ID) {
			exposed = append(exposed, library)
		}
	}
	return exposed, nil
}

func (s *Server) dlnaLibraryItems(libraryID string, start, count int) ([]MediaItem, int, error) {
	return s.dlnaLibraryItemsContext(context.Background(), libraryID, start, count)
}

func (s *Server) dlnaLibraryItemsContext(ctx context.Context, libraryID string, start, count int) ([]MediaItem, int, error) {
	start, count = normalizeDLNABrowseWindow(start, count)
	items, hasMore, err := s.queryDLNAMediaItemsContext(ctx, `WHERE library_id = ? AND parent_id IS NULL ORDER BY sort_title ASC, id ASC LIMIT ? OFFSET ?`, []any{libraryID, count + 1, start}, count)
	return items, dlnaConservativeTotal(start, len(items), hasMore), err
}

func (s *Server) dlnaChildItems(parentID string, start, count int) ([]MediaItem, int, error) {
	return s.dlnaChildItemsContext(context.Background(), parentID, start, count)
}

func (s *Server) dlnaChildItemsContext(ctx context.Context, parentID string, start, count int) ([]MediaItem, int, error) {
	start, count = normalizeDLNABrowseWindow(start, count)
	items, hasMore, err := s.queryDLNAMediaItemsContext(ctx, `WHERE parent_id = ? ORDER BY index_number ASC, sort_title ASC, id ASC LIMIT ? OFFSET ?`, []any{parentID, count + 1, start}, count)
	return items, dlnaConservativeTotal(start, len(items), hasMore), err
}

func (s *Server) dlnaMediaItem(id string) (MediaItem, error) {
	return s.dlnaMediaItemContext(context.Background(), id)
}

func (s *Server) dlnaMediaItemContext(ctx context.Context, id string) (MediaItem, error) {
	items, _, err := s.queryDLNAMediaItemsContext(ctx, `WHERE id = ? LIMIT 1`, []any{id}, 1)
	if err != nil {
		return MediaItem{}, err
	}
	if len(items) == 0 {
		return MediaItem{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (s *Server) queryDLNAMediaItems(clause string, args []any, visibleLimit int) ([]MediaItem, bool, error) {
	return s.queryDLNAMediaItemsContext(context.Background(), clause, args, visibleLimit)
}

func (s *Server) queryDLNAMediaItemsContext(ctx context.Context, clause string, args []any, visibleLimit int) ([]MediaItem, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT id, library_id, COALESCE(parent_id, ''), type, title, sort_title, year,
		duration_seconds, season_number, episode_number, index_number, art_seed, source_url
		FROM media_items
		`+clause, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []MediaItem{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var item MediaItem
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.ParentID, &item.Type, &item.Title, &item.SortTitle, &item.Year, &item.DurationSeconds, &item.SeasonNumber, &item.EpisodeNumber, &item.IndexNumber, &item.ArtSeed, &item.SourceURL); err != nil {
			return nil, false, err
		}
		item.MediaFiles = s.primaryMediaFileForPlaybackContext(ctx, item.ID, item.SourceURL)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := visibleLimit > 0 && len(items) > visibleLimit
	if hasMore {
		items = items[:visibleLimit]
	}
	return items, hasMore, nil
}

func dlnaConservativeTotal(start, visible int, hasMore bool) int {
	if start < 0 {
		start = 0
	}
	total := start + visible
	if hasMore {
		total++
	}
	return total
}

func (s *Server) dlnaChildCounts(items []MediaItem) (map[string]int, error) {
	return s.dlnaChildCountsContext(context.Background(), items)
}

func (s *Server) dlnaChildCountsContext(ctx context.Context, items []MediaItem) (map[string]int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	counts := make(map[string]int, len(items))
	if len(items) == 0 {
		return counts, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		ids = append(ids, item.ID)
		counts[item.ID] = 0
	}
	if len(ids) == 0 {
		return counts, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT parent_id, COUNT(*)
		FROM media_items
		WHERE parent_id IN (`+sqlPlaceholders(len(ids))+`)
		GROUP BY parent_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var parentID string
		var childCount int
		if err := rows.Scan(&parentID, &childCount); err != nil {
			return nil, err
		}
		counts[parentID] = childCount
	}
	return counts, rows.Err()
}

func normalizeDLNABrowseWindow(start, count int) (int, int) {
	if start < 0 {
		start = 0
	}
	if count <= 0 {
		count = dlnaDefaultBrowsePageSize
	}
	if count > dlnaMaxBrowsePageSize {
		count = dlnaMaxBrowsePageSize
	}
	return start, count
}

func (s *Server) requireDLNA(w http.ResponseWriter, r *http.Request) (dlnaConfig, bool) {
	if !dlnaLANRequest(r) {
		http.NotFound(w, r)
		return dlnaConfig{}, false
	}
	cfg, err := s.dlnaConfig()
	if err != nil {
		http.Error(w, "DLNA settings unavailable.", http.StatusInternalServerError)
		return dlnaConfig{}, false
	}
	if !cfg.Enabled {
		http.NotFound(w, nil)
		return dlnaConfig{}, false
	}
	profile, err := resolveDLNARendererProfile(r)
	if err != nil || !profile.ReachableRoute["http"] {
		http.NotFound(w, r)
		return dlnaConfig{}, false
	}
	w.Header().Set("X-Portico-DLNA-Renderer-Profile", profile.Version+":"+profile.ID)
	return cfg, true
}

func resolveDLNARendererProfile(r *http.Request) (dlnaRendererProfile, error) {
	userAgent := ""
	if r != nil {
		userAgent = strings.ToLower(strings.TrimSpace(r.UserAgent()))
	}
	id := "generic-progressive"
	device := "generic-dlna-renderer"
	switch {
	case strings.Contains(userAgent, "samsung") || strings.Contains(userAgent, "sec_hhp"):
		id, device = "samsung-progressive", "samsung-dlna-renderer"
	case strings.Contains(userAgent, "webos") || strings.Contains(userAgent, "lge"):
		id, device = "lg-progressive", "lg-dlna-renderer"
	}
	client := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "dlna",
		ClientVersion:           dlnaRendererProfileV1,
		Platform:                "dlna",
		Device:                  device,
		SupportedContainers:     []string{"mp4", "mp3", "m4a"},
		SupportedVideoCodecs:    []string{"h264"},
		SupportedAudioCodecs:    []string{"aac", "mp3"},
		MaxWidth:                1920,
		MaxHeight:               1080,
		MaxAudioChannels:        2,
	}
	resolution, err := resolvePlaybackCapabilities(client)
	if err != nil {
		return dlnaRendererProfile{}, err
	}
	routes := map[string]bool{}
	for _, tuple := range resolution.Tuples {
		// DLNA URLs emitted by this server are progressive HTTP URLs. HLS or
		// any other planner route must not be inferred from a renderer family.
		if tuple.Protocol == "http" {
			routes[tuple.Protocol] = true
		}
	}
	return dlnaRendererProfile{Version: dlnaRendererProfileV1, ID: id, Client: client, ReachableRoute: routes}, nil
}

func dlnaLANRequest(r *http.Request) bool {
	if r == nil || r.TLS != nil {
		return false
	}
	// DLNA is never proxy- or relay-reachable. Forwarding metadata is not an
	// identity signal and cannot make a peer eligible; its presence proves the
	// request did not arrive directly from a renderer socket.
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return false
		}
	}
	peer, err := dlnaSocketAddress(r.RemoteAddr)
	if err != nil {
		return false
	}
	local := netip.Addr{}
	if localAddress, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && localAddress != nil {
		local, err = dlnaSocketAddress(localAddress.String())
		if err != nil {
			return false
		}
	}
	return dlnaPeerEligible(peer, local, eligibleDLNAInterfacePrefixes())
}

func dlnaSocketAddress(socketAddress string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(socketAddress))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(socketAddress), "[]")
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, err
	}
	return address.Unmap().WithZone(""), nil
}

func dlnaPeerEligible(peer, local netip.Addr, eligible []netip.Prefix) bool {
	if !peer.IsValid() {
		return false
	}
	if peer.IsLoopback() {
		return !local.IsValid() || local.IsLoopback()
	}
	if local.IsValid() && local.IsLoopback() {
		return false
	}
	for _, prefix := range eligible {
		if prefix.Contains(peer) && (!local.IsValid() || prefix.Contains(local)) {
			return true
		}
	}
	return false
}

func eligibleDLNAInterfacePrefixes() []netip.Prefix {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	prefixes := make([]netip.Prefix, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, raw := range addresses {
			prefix, err := netip.ParsePrefix(raw.String())
			if err != nil {
				continue
			}
			address := prefix.Addr().Unmap().WithZone("")
			if !(address.IsPrivate() || address.IsLinkLocalUnicast()) {
				continue
			}
			bits := prefix.Bits()
			if address.Is4() && prefix.Addr().Is6() {
				bits -= 96
			}
			prefixes = append(prefixes, netip.PrefixFrom(address, bits).Masked())
		}
	}
	return prefixes
}

func (s *Server) dlnaConfig() (dlnaConfig, error) {
	settings, err := s.loadSettings()
	if err != nil {
		return dlnaConfig{}, err
	}
	identity, err := s.systemIdentity()
	if err != nil {
		return dlnaConfig{}, err
	}
	group, _ := settings["dlna"].(map[string]any)
	friendlyName := strings.TrimSpace(settingString(group, "friendlyName", ""))
	if friendlyName == "" {
		friendlyName = identity.FriendlyName
	}
	exposed := map[string]bool{}
	for _, id := range settingStringList(group, "exposedLibraries") {
		exposed[id] = true
	}
	return dlnaConfig{
		Enabled:           settingBool(group, "enabled", false),
		FriendlyName:      friendlyName,
		AdvertiseURL:      strings.TrimRight(strings.TrimSpace(settingString(group, "advertiseUrl", "")), "/"),
		ExposedLibraries:  exposed,
		DiscoveryInterval: time.Duration(dlnaDefaultDiscoveryEvery) * time.Second,
		LeaseSeconds:      dlnaDefaultLeaseSeconds,
		ProtocolInfo:      dlnaDefaultProtocolInfo,
		ServerID:          identity.ServerID,
		UUID:              dlnaUUID(identity.ServerID),
	}, nil
}

func (cfg dlnaConfig) libraryExposed(libraryID string) bool {
	if len(cfg.ExposedLibraries) == 0 {
		return false
	}
	return cfg.ExposedLibraries[libraryID]
}

func (s *Server) dlnaBaseURL(r *http.Request, cfg dlnaConfig) string {
	// Never reflect Host or forwarding headers into unauthenticated renderer
	// URLs. The advertised LAN endpoint is selected from eligible interfaces.
	return s.dlnaAdvertiseURL(cfg)
}

func (s *Server) dlnaAdvertiseURL(cfg dlnaConfig) string {
	if eligibleDLNAAdvertiseURL(cfg.AdvertiseURL, eligibleDLNAInterfacePrefixes()) {
		return cfg.AdvertiseURL
	}
	host, port, err := net.SplitHostPort(s.cfg.Addr)
	if err != nil {
		port = "32500"
	}
	if port == "" {
		port = "32500"
	}
	ip := firstEligibleDLNAIPv4()
	if ip == "" && host != "" && host != "0.0.0.0" && host != "::" {
		ip = host
	}
	if ip == "" {
		ip = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(ip, port)
}

func eligibleDLNAAdvertiseURL(raw string, prefixes []netip.Prefix) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	address, err := netip.ParseAddr(strings.Trim(parsed.Hostname(), "[]"))
	if err != nil {
		return false
	}
	address = address.Unmap().WithZone("")
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (s *Server) runDLNASSDP(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		cfg, err := s.dlnaConfig()
		if err != nil {
			s.log.Warn("load dlna settings failed", "error", err)
			if !sleepContext(ctx, 10*time.Second) {
				return
			}
			continue
		}
		if !cfg.Enabled {
			if !sleepContext(ctx, 5*time.Second) {
				return
			}
			continue
		}
		if err := s.serveDLNASSDP(ctx, cfg); err != nil {
			s.log.Warn("dlna ssdp stopped", "error", err)
			s.recordLog("warn", "DLNA SSDP discovery stopped", map[string]string{"error": err.Error()})
			if !sleepContext(ctx, 15*time.Second) {
				return
			}
		}
	}
}

func (s *Server) serveDLNASSDP(ctx context.Context, initial dlnaConfig) error {
	iface, _ := firstEligibleDLNAIPv4Interface()
	if iface == nil {
		return errors.New("no explicitly eligible LAN interface is available for DLNA discovery")
	}
	addr, err := net.ResolveUDPAddr("udp4", dlnaMulticastAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	s.recordLog("info", "DLNA SSDP discovery started", map[string]string{"friendlyName": initial.FriendlyName})

	buf := make([]byte, 2048)
	nextNotify := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		cfg, err := s.dlnaConfig()
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			s.recordLog("info", "DLNA SSDP discovery stopped", map[string]string{"reason": "disabled"})
			return nil
		}
		if time.Now().After(nextNotify) {
			s.sendSSDPNotify(conn, cfg)
			nextNotify = time.Now().Add(cfg.DiscoveryInterval)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return err
		}
		request := string(buf[:n])
		peer, peerErr := dlnaSocketAddress(remote.String())
		if peerErr != nil || !dlnaPeerEligible(peer, netip.Addr{}, eligibleDLNAInterfacePrefixes()) || !isSSDPDiscoveryRequest(request) {
			continue
		}
		for _, st := range matchingSSDPTargets(ssdpHeader(request, "st")) {
			s.sendSSDPResponse(conn, remote, cfg, st)
		}
	}
}

func (s *Server) sendSSDPResponse(conn *net.UDPConn, remote *net.UDPAddr, cfg dlnaConfig, st string) {
	if st == "uuid:" {
		st = "uuid:" + cfg.UUID
	}
	location := s.dlnaAdvertiseURL(cfg) + "/dlna/device.xml"
	usn := ssdpUSN(cfg.UUID, st)
	response := strings.Join([]string{
		"HTTP/1.1 200 OK",
		fmt.Sprintf("CACHE-CONTROL: max-age=%d", cfg.LeaseSeconds),
		"DATE: " + time.Now().UTC().Format(http.TimeFormat),
		"EXT:",
		"LOCATION: " + location,
		"SERVER: " + dlnaServerHeader,
		"ST: " + st,
		"USN: " + usn,
		"",
		"",
	}, "\r\n")
	_, _ = conn.WriteToUDP([]byte(response), remote)
}

func (s *Server) sendSSDPNotify(conn *net.UDPConn, cfg dlnaConfig) {
	remote, err := net.ResolveUDPAddr("udp4", dlnaMulticastAddr)
	if err != nil {
		return
	}
	location := s.dlnaAdvertiseURL(cfg) + "/dlna/device.xml"
	for _, nt := range []string{dlnaRootDeviceURN, "uuid:" + cfg.UUID, dlnaMediaServerURN, dlnaContentDirectoryURN, dlnaConnectionManagerURN} {
		message := strings.Join([]string{
			"NOTIFY * HTTP/1.1",
			"HOST: " + dlnaMulticastAddr,
			fmt.Sprintf("CACHE-CONTROL: max-age=%d", cfg.LeaseSeconds),
			"LOCATION: " + location,
			"NT: " + nt,
			"NTS: ssdp:alive",
			"SERVER: " + dlnaServerHeader,
			"USN: " + ssdpUSN(cfg.UUID, nt),
			"",
			"",
		}, "\r\n")
		_, _ = conn.WriteToUDP([]byte(message), remote)
	}
}

func matchingSSDPTargets(st string) []string {
	st = strings.ToLower(strings.TrimSpace(st))
	switch st {
	case "", "ssdp:all":
		return []string{dlnaRootDeviceURN, dlnaMediaServerURN, dlnaContentDirectoryURN, dlnaConnectionManagerURN, "uuid:"}
	case strings.ToLower(dlnaRootDeviceURN):
		return []string{dlnaRootDeviceURN}
	case strings.ToLower(dlnaMediaServerURN):
		return []string{dlnaMediaServerURN}
	case strings.ToLower(dlnaContentDirectoryURN):
		return []string{dlnaContentDirectoryURN}
	case strings.ToLower(dlnaConnectionManagerURN):
		return []string{dlnaConnectionManagerURN}
	default:
		if strings.HasPrefix(st, "uuid:") {
			return []string{"uuid:"}
		}
		return nil
	}
}

func ssdpUSN(uuid, target string) string {
	if target == "uuid:" || strings.HasPrefix(target, "uuid:") {
		return "uuid:" + uuid
	}
	return "uuid:" + uuid + "::" + target
}

func isSSDPDiscoveryRequest(request string) bool {
	lower := strings.ToLower(request)
	return strings.HasPrefix(lower, "m-search") && strings.Contains(lower, "ssdp:discover")
}

func ssdpHeader(request, key string) string {
	key = strings.ToLower(key)
	for _, line := range strings.Split(request, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), key) {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func writeXML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func writeDLNASOAPResponse(w http.ResponseWriter, action, serviceURN, inner string) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:%sResponse xmlns:u="%s">%s</u:%sResponse>
  </s:Body>
</s:Envelope>`, action, serviceURN, inner, action)
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func writeDLNASOAPFault(w http.ResponseWriter, code int, description string) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>UPnPError</faultstring>
      <detail>
        <UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
          <errorCode>%d</errorCode>
          <errorDescription>%s</errorDescription>
        </UPnPError>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`, code, dlnaXMLText(description))
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(body))
}

func dlnaDIDL(nodes []string) string {
	return `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` + strings.Join(nodes, "") + `</DIDL-Lite>`
}

func dlnaContainer(id, parentID, title, class string, childCount int) string {
	return fmt.Sprintf(`<container id="%s" parentID="%s" restricted="1" searchable="1" childCount="%d"><dc:title>%s</dc:title><upnp:class>%s</upnp:class></container>`,
		dlnaXMLAttr(id),
		dlnaXMLAttr(parentID),
		childCount,
		dlnaXMLText(title),
		dlnaXMLText(class),
	)
}

func dlnaItem(id, parentID string, item MediaItem, mediaURL, protocolInfo string) string {
	duration := ""
	if item.DurationSeconds > 0 {
		duration = fmt.Sprintf(` duration="%s"`, dlnaXMLAttr(formatDLNADuration(item.DurationSeconds)))
	}
	year := ""
	if item.Year > 0 {
		year = fmt.Sprintf("<dc:date>%d-01-01</dc:date>", item.Year)
	}
	return fmt.Sprintf(`<item id="%s" parentID="%s" restricted="1"><dc:title>%s</dc:title>%s<upnp:class>%s</upnp:class><res protocolInfo="%s"%s>%s</res></item>`,
		dlnaXMLAttr(id),
		dlnaXMLAttr(parentID),
		dlnaXMLText(item.Title),
		year,
		dlnaXMLText(dlnaItemClass(item.Type)),
		dlnaXMLAttr(protocolInfo),
		duration,
		dlnaXMLText(mediaURL),
	)
}

func dlnaContainerType(mediaType string) bool {
	switch mediaType {
	case "show", "anime", "season", "album":
		return true
	default:
		return false
	}
}

func dlnaContainerClass(mediaType string) string {
	switch mediaType {
	case "show", "anime":
		return "object.container.series"
	case "season":
		return "object.container"
	case "album":
		return "object.container.album.musicAlbum"
	default:
		return "object.container.storageFolder"
	}
}

func dlnaItemClass(mediaType string) string {
	switch mediaType {
	case "movie":
		return "object.item.videoItem.movie"
	case "episode":
		return "object.item.videoItem"
	case "music", "track":
		return "object.item.audioItem.musicTrack"
	case "audiobook":
		return "object.item.audioItem.audioBook"
	default:
		return "object.item.videoItem"
	}
}

func dlnaContentType(item MediaItem, source string) string {
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(source))); contentType != "" {
		return contentType
	}
	switch item.Type {
	case "music", "track", "audiobook":
		return "audio/mpeg"
	default:
		return "video/mp4"
	}
}

func formatDLNADuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%d:%02d:%02d.000", hours, minutes, secs)
}

func paginateLibraries(items []Library, start, count int) []Library {
	if start < 0 {
		start = 0
	}
	if start >= len(items) {
		return []Library{}
	}
	if count <= 0 || start+count > len(items) {
		count = len(items) - start
	}
	return items[start : start+count]
}

func paginateMedia(items []MediaItem, start, count int) []MediaItem {
	if start < 0 {
		start = 0
	}
	if start >= len(items) {
		return []MediaItem{}
	}
	if count <= 0 || start+count > len(items) {
		count = len(items) - start
	}
	return items[start : start+count]
}

func soapValue(body, name string) string {
	pattern := regexp.MustCompile(`(?is)<(?:[A-Za-z0-9_]+:)?` + regexp.QuoteMeta(name) + `(?:\s[^>]*)?>(.*?)</(?:[A-Za-z0-9_]+:)?` + regexp.QuoteMeta(name) + `>`)
	match := pattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(match[1]))
}

func parseSOAPInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func settingBool(group map[string]any, key string, fallback bool) bool {
	if group == nil {
		return fallback
	}
	value, ok := group[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
	case float64:
		return typed != 0
	case int:
		return typed != 0
	}
	return fallback
}

func settingString(group map[string]any, key, fallback string) string {
	if group == nil {
		return fallback
	}
	if value, ok := group[key].(string); ok {
		return value
	}
	return fallback
}

func settingInt(group map[string]any, key string, fallback int) int {
	if group == nil {
		return fallback
	}
	switch value := group[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func settingStringList(group map[string]any, key string) []string {
	if group == nil {
		return nil
	}
	raw, ok := group[key]
	if !ok {
		return nil
	}
	var values []string
	switch list := raw.(type) {
	case []any:
		for _, item := range list {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
		}
	case []string:
		for _, value := range list {
			if strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
		}
	}
	return values
}

func firstEligibleDLNAIPv4() string {
	_, address := firstEligibleDLNAIPv4Interface()
	return address
}

func firstEligibleDLNAIPv4Interface() (*net.Interface, string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip != nil && !ip.IsLoopback() && (ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				selected := iface
				return &selected, ip.String()
			}
		}
	}
	return nil, ""
}

func dlnaUUID(serverID string) string {
	sum := sha1.Sum([]byte(serverID))
	bytes := sum[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func dlnaXMLText(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func dlnaXMLAttr(value string) string {
	return dlnaXMLText(value)
}

const dlnaContentDirectorySCPD = `<?xml version="1.0" encoding="utf-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action><name>Browse</name><argumentList><argument><name>ObjectID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ObjectID</relatedStateVariable></argument><argument><name>BrowseFlag</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_BrowseFlag</relatedStateVariable></argument><argument><name>Filter</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Filter</relatedStateVariable></argument><argument><name>StartingIndex</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Index</relatedStateVariable></argument><argument><name>RequestedCount</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>SortCriteria</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_SortCriteria</relatedStateVariable></argument><argument><name>Result</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable></argument><argument><name>NumberReturned</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>TotalMatches</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument><argument><name>UpdateID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_UpdateID</relatedStateVariable></argument></argumentList></action>
    <action><name>GetSearchCapabilities</name><argumentList><argument><name>SearchCaps</name><direction>out</direction><relatedStateVariable>SearchCapabilities</relatedStateVariable></argument></argumentList></action>
    <action><name>GetSortCapabilities</name><argumentList><argument><name>SortCaps</name><direction>out</direction><relatedStateVariable>SortCapabilities</relatedStateVariable></argument></argumentList></action>
    <action><name>GetSystemUpdateID</name><argumentList><argument><name>Id</name><direction>out</direction><relatedStateVariable>SystemUpdateID</relatedStateVariable></argument></argumentList></action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ObjectID</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_BrowseFlag</name><dataType>string</dataType><allowedValueList><allowedValue>BrowseMetadata</allowedValue><allowedValue>BrowseDirectChildren</allowedValue></allowedValueList></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Filter</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Result</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_SearchCriteria</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_SortCriteria</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Index</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Count</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_UpdateID</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="yes"><name>SystemUpdateID</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>SearchCapabilities</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>SortCapabilities</name><dataType>string</dataType></stateVariable>
  </serviceStateTable>
</scpd>`

const dlnaConnectionManagerSCPD = `<?xml version="1.0" encoding="utf-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action><name>GetProtocolInfo</name><argumentList><argument><name>Source</name><direction>out</direction><relatedStateVariable>SourceProtocolInfo</relatedStateVariable></argument><argument><name>Sink</name><direction>out</direction><relatedStateVariable>SinkProtocolInfo</relatedStateVariable></argument></argumentList></action>
    <action><name>GetCurrentConnectionIDs</name><argumentList><argument><name>ConnectionIDs</name><direction>out</direction><relatedStateVariable>CurrentConnectionIDs</relatedStateVariable></argument></argumentList></action>
    <action><name>GetCurrentConnectionInfo</name><argumentList><argument><name>ConnectionID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable></argument><argument><name>RcsID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_RcsID</relatedStateVariable></argument><argument><name>AVTransportID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_AVTransportID</relatedStateVariable></argument><argument><name>ProtocolInfo</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ProtocolInfo</relatedStateVariable></argument><argument><name>PeerConnectionManager</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionManager</relatedStateVariable></argument><argument><name>PeerConnectionID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable></argument><argument><name>Direction</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Direction</relatedStateVariable></argument><argument><name>Status</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionStatus</relatedStateVariable></argument></argumentList></action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="yes"><name>SourceProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="yes"><name>SinkProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="yes"><name>CurrentConnectionIDs</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ConnectionStatus</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ConnectionManager</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Direction</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ConnectionID</name><dataType>i4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_AVTransportID</name><dataType>i4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_RcsID</name><dataType>i4</dataType></stateVariable>
  </serviceStateTable>
</scpd>`
