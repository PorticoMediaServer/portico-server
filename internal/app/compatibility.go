package app

import (
	"regexp"
	"sort"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

type ProtocolRange struct {
	Minimum int `json:"minimum"`
	Maximum int `json:"maximum"`
}

type APIContractIdentity struct {
	DigestAlgorithm string `json:"digestAlgorithm"`
	Identity        string `json:"identity"`
	Digest          string `json:"digest"`
}

type CompatibilityCapability struct {
	ID                string   `json:"id"`
	Revision          int      `json:"revision"`
	State             string   `json:"state"`
	RequiredSemantics []string `json:"requiredSemantics"`
}

type BuildIdentity struct {
	Version   string  `json:"version"`
	Number    string  `json:"buildNumber"`
	Channel   string  `json:"channel"`
	Commit    string  `json:"commit"`
	Timestamp *string `json:"timestamp"`
}

type ForwardCompatibilityPolicy struct {
	UnknownOptionalCapabilities   string `json:"unknownOptionalCapabilities"`
	UnknownRequiredSemantics      string `json:"unknownRequiredSemantics"`
	AuthorizationOnPartialUpgrade string `json:"authorizationOnPartialUpgrade"`
	APIContractDigestMismatch     string `json:"apiContractDigestMismatch"`
}

type CompatibilityEnvelope struct {
	EnvelopeRevision        int                        `json:"envelopeRevision"`
	SupportedClientProtocol ProtocolRange              `json:"supportedClientProtocol"`
	APIContract             APIContractIdentity        `json:"apiContract"`
	Build                   BuildIdentity              `json:"build"`
	SemanticRevisions       map[string]int             `json:"semanticRevisions"`
	Capabilities            []CompatibilityCapability  `json:"capabilities"`
	RequiredSemantics       []string                   `json:"requiredSemantics"`
	ForwardCompatibility    ForwardCompatibilityPolicy `json:"forwardCompatibility"`
}

func (s *Server) compatibilityEnvelope() CompatibilityEnvelope {
	version := strings.TrimSpace(s.cfg.Release)
	if version == "" || version == "dev" {
		version = foundationcontract.DefaultBuildVersion
	}
	channel := strings.TrimSpace(s.cfg.BuildChannel)
	if channel == "" {
		channel = foundationcontract.DefaultBuildChannel
	}
	commit := strings.TrimSpace(s.cfg.BuildCommit)
	if commit == "" || commit == "unknown" {
		commit = foundationcontract.DefaultBuildCommit
	}
	var timestamp *string
	if value := strings.TrimSpace(s.cfg.BuildTimestamp); value != "" && value != "unknown" {
		timestamp = &value
	}
	revisions := make(map[string]int, len(foundationcontract.SemanticRevisions))
	for name, revision := range foundationcontract.SemanticRevisions {
		revisions[name] = revision
	}
	return CompatibilityEnvelope{
		EnvelopeRevision:        foundationcontract.EnvelopeRevision,
		SupportedClientProtocol: ProtocolRange{Minimum: foundationcontract.ClientProtocolMinimum, Maximum: foundationcontract.ClientProtocolMaximum},
		APIContract:             APIContractIdentity{DigestAlgorithm: foundationcontract.APIContractDigestAlgorithm, Identity: foundationcontract.APIContractIdentity, Digest: foundationcontract.APIContractDigest},
		Build:                   BuildIdentity{Version: version, Number: firstNonEmpty(strings.TrimSpace(s.cfg.BuildNumber), foundationcontract.DefaultBuildNumber), Channel: channel, Commit: commit, Timestamp: timestamp},
		SemanticRevisions:       revisions,
		Capabilities:            s.serverCapabilities(),
		RequiredSemantics:       []string{"product", "viewerProfileAuthority", "playback", "problemRecoveryAction", "externalAction", "paginationCursor"},
		ForwardCompatibility: ForwardCompatibilityPolicy{
			UnknownOptionalCapabilities:   foundationcontract.UnknownOptionalCapabilitiesPolicy,
			UnknownRequiredSemantics:      foundationcontract.UnknownRequiredSemanticsPolicy,
			AuthorizationOnPartialUpgrade: foundationcontract.AuthorizationOnPartialUpgradePolicy,
			APIContractDigestMismatch:     foundationcontract.APIContractDigestMismatchPolicy,
		},
	}
}

type serverCapabilityDefinition struct {
	ID                string
	Revision          int
	RequiredSemantics []string
	RequiredRoutes    []serverCapabilityRoute
	Availability      func(*Server) string
}

type serverCapabilityRoute struct {
	Method string
	Path   string
}

func capabilityRoute(method, path string) serverCapabilityRoute {
	return serverCapabilityRoute{Method: method, Path: path}
}

var serverCapabilityDefinitions = []serverCapabilityDefinition{
	{"downloads", 1, []string{"product"}, []serverCapabilityRoute{
		capabilityRoute("GET", "/api/download-preparations"), capabilityRoute("POST", "/api/download-preparations"),
		capabilityRoute("GET", "/api/download-preparations/{preparationId}"), capabilityRoute("PATCH", "/api/download-preparations/{preparationId}"), capabilityRoute("DELETE", "/api/download-preparations/{preparationId}"), capabilityRoute("POST", "/api/download-preparations/{preparationId}/grant"),
		capabilityRoute("GET", "/api/media/{id}/download-options"), capabilityRoute("GET", "/api/media/{id}/download"), capabilityRoute("HEAD", "/api/media/{id}/download"),
	}, nil},
	{"home.lazy-rows", 1, []string{"product"}, []serverCapabilityRoute{capabilityRoute("GET", "/api/home"), capabilityRoute("GET", "/api/home/rows/{id}")}, nil},
	{"library.canonical-browse", 1, []string{"product"}, []serverCapabilityRoute{capabilityRoute("POST", "/api/libraries/{libraryId}/browse")}, nil},
	{"library.capability-resolution", 1, []string{"product"}, []serverCapabilityRoute{capabilityRoute("GET", "/api/libraries/{libraryId}/browse-capabilities")}, nil},
	{"live-tv", 1, []string{"playback"}, []serverCapabilityRoute{capabilityRoute("GET", "/api/live-tv"), capabilityRoute("POST", "/api/live-tv/play"), capabilityRoute("GET", "/api/live-tv/hls/{channelId}/playlist.m3u8")}, liveTVCapabilityState},
	{"media.actions", 1, []string{"product"}, []serverCapabilityRoute{capabilityRoute("GET", "/api/product-contract")}, nil},
	{"notifications", 1, []string{"product"}, []serverCapabilityRoute{
		capabilityRoute("GET", "/api/notifications"), capabilityRoute("GET", "/api/notifications/events"), capabilityRoute("GET", "/api/notifications/events/poll"), capabilityRoute("POST", "/api/notifications/read-all"), capabilityRoute("POST", "/api/notifications/receipts"), capabilityRoute("PATCH", "/api/notifications/{notificationId}"),
	}, nil},
	{"playback.google-cast-custom-receiver", 1, []string{"playback"}, []serverCapabilityRoute{
		capabilityRoute("POST", "/api/playback/cast/bootstrap"), capabilityRoute("POST", "/api/playback/cast/reconnect"), capabilityRoute("POST", "/api/playback/cast/redeem"),
		capabilityRoute("GET", "/api/playback/cast/sessions/{sessionId}/state"), capabilityRoute("POST", "/api/playback/cast/sessions/{sessionId}/{operation}"), capabilityRoute("DELETE", "/api/playback/cast/sessions/{sessionId}/stop"),
	}, castCapabilityState},
	{"remote-access.direct", 1, []string{"viewerProfileAuthority"}, []serverCapabilityRoute{
		capabilityRoute("GET", "/api/remote-access/status"), capabilityRoute("GET", "/api/remote-access/health"), capabilityRoute("GET", "/api/remote-access/routes/local"),
		capabilityRoute("POST", "/api/remote-access/claim/start"), capabilityRoute("POST", "/api/remote-access/claim/cancel"), capabilityRoute("PATCH", "/api/remote-access/settings"), capabilityRoute("POST", "/api/remote-access/policy-sync"),
		capabilityRoute("POST", "/api/remote-access/certificates/renew"), capabilityRoute("PATCH", "/api/remote-access/members/{id}"), capabilityRoute("POST", "/api/remote-access/test-direct"), capabilityRoute("POST", "/api/remote-access/unclaim"),
	}, remoteAccessCapabilityState},
	{"saved.favorites-resource", 1, []string{"product"}, []serverCapabilityRoute{capabilityRoute("GET", "/api/favorites"), capabilityRoute("POST", "/api/media/{mediaId}/favorite")}, nil},
	{"saved.first-class-resources", 1, []string{"product"}, []serverCapabilityRoute{
		capabilityRoute("GET", "/api/playlists"), capabilityRoute("POST", "/api/playlists"), capabilityRoute("GET", "/api/playlists/{playlistId}"), capabilityRoute("PATCH", "/api/playlists/{playlistId}"), capabilityRoute("DELETE", "/api/playlists/{playlistId}"), capabilityRoute("GET", "/api/playlists/{playlistId}/items"), capabilityRoute("POST", "/api/playlists/{playlistId}/items:batch"),
		capabilityRoute("GET", "/api/collections"), capabilityRoute("POST", "/api/collections"), capabilityRoute("GET", "/api/collections/{collectionId}"), capabilityRoute("PATCH", "/api/collections/{collectionId}"), capabilityRoute("DELETE", "/api/collections/{collectionId}"), capabilityRoute("GET", "/api/collections/{collectionId}/items"), capabilityRoute("POST", "/api/collections/{collectionId}/memberships:batch"),
		capabilityRoute("GET", "/api/saved-views"), capabilityRoute("POST", "/api/saved-views"), capabilityRoute("GET", "/api/saved-views/{savedViewId}"), capabilityRoute("PATCH", "/api/saved-views/{savedViewId}"), capabilityRoute("DELETE", "/api/saved-views/{savedViewId}"), capabilityRoute("POST", "/api/saved-views/{savedViewId}/browse"),
	}, nil},
	{"search.grouped-cursors", 1, []string{"paginationCursor"}, []serverCapabilityRoute{capabilityRoute("POST", "/api/search")}, nil},
	{"settings.typed-revisioned", 1, []string{"product"}, []serverCapabilityRoute{capabilityRoute("GET", "/api/settings"), capabilityRoute("PATCH", "/api/settings")}, nil},
	{"watch-with-friends", 1, []string{"playback"}, []serverCapabilityRoute{
		capabilityRoute("GET", "/api/watch-with-friends/groups"), capabilityRoute("POST", "/api/watch-with-friends/groups"), capabilityRoute("GET", "/api/watch-with-friends/groups/{groupId}"), capabilityRoute("DELETE", "/api/watch-with-friends/groups/{groupId}"),
		capabilityRoute("POST", "/api/watch-with-friends/groups/{groupId}/join"), capabilityRoute("POST", "/api/watch-with-friends/groups/{groupId}/leave"), capabilityRoute("GET", "/api/watch-with-friends/groups/{groupId}/events"), capabilityRoute("GET", "/api/watch-with-friends/groups/{groupId}/events/poll"),
		capabilityRoute("PATCH", "/api/watch-with-friends/groups/{groupId}/state"), capabilityRoute("PATCH", "/api/watch-with-friends/groups/{groupId}/member/state"), capabilityRoute("PATCH", "/api/watch-with-friends/groups/{groupId}/settings"),
		capabilityRoute("POST", "/api/watch-with-friends/groups/{groupId}/queue"), capabilityRoute("PATCH", "/api/watch-with-friends/groups/{groupId}/queue"), capabilityRoute("DELETE", "/api/watch-with-friends/groups/{groupId}/queue/{mediaId}"),
	}, nil},
}

func (s *Server) serverCapabilities() []CompatibilityCapability {
	routes := map[string]bool{}
	for _, route := range s.APIRoutes() {
		routes[capabilityRouteKey(route.Method, route.Path)] = true
	}
	result := make([]CompatibilityCapability, 0, len(serverCapabilityDefinitions))
	for _, definition := range serverCapabilityDefinitions {
		state := serverCapabilityState(s, definition, routes)
		result = append(result, CompatibilityCapability{ID: definition.ID, Revision: definition.Revision, State: state, RequiredSemantics: append([]string(nil), definition.RequiredSemantics...)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func serverCapabilityState(server *Server, definition serverCapabilityDefinition, mountedRoutes map[string]bool) string {
	for _, required := range capabilityRequiredRoutes(definition) {
		if !mountedRoutes[capabilityRouteKey(required.Method, required.Path)] {
			return "unavailable"
		}
	}
	if definition.Availability != nil {
		return definition.Availability(server)
	}
	return "available"
}

func capabilityRequiredRoutes(definition serverCapabilityDefinition) []serverCapabilityRoute {
	routes := append([]serverCapabilityRoute(nil), definition.RequiredRoutes...)
	if definition.ID != "media.actions" {
		return routes
	}
	for _, action := range canonicalMediaActionCapabilities() {
		if action.Command.Kind == "api" {
			routes = append(routes, capabilityRoute(action.Command.Method, action.Command.PathTemplate))
		}
	}
	return routes
}

func capabilityRouteKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + regexp.MustCompile(`\{[^/{}]+\}`).ReplaceAllString(strings.TrimSpace(path), "{}")
}

func remoteAccessCapabilityState(s *Server) string {
	settings, err := s.remoteAccessSettings()
	if err != nil {
		return "degraded"
	}
	credential := strings.TrimSpace(s.secretSetting(remoteAccessCredentialKey))
	endpoint := s.remotePublicEndpoint(settings)
	certificateReady := settings.CertificateStatus == "valid" || settings.CertificateStatus == "custom_valid"
	if !settings.Enabled || settings.ClaimStatus != "claimed" || strings.TrimSpace(settings.ServerID) == "" || credential == "" || strings.TrimSpace(settings.AssignedHostname) == "" {
		return "requires_configuration"
	}
	// Capability state describes whether this build can serve the signed direct
	// access protocol. Reachability is independently verified by Hosted Services
	// for every published endpoint and is intentionally not a capability gate.
	// Coupling this state to the last asynchronous reachability diagnostic creates
	// a deadlock: a topology change advertises the capability as degraded, Hosted
	// then withholds the verified route document, and clients cannot reconnect even
	// after the replacement endpoints have passed their probes.
	if !certificateReady || endpoint.URL == "" {
		return "degraded"
	}
	return "available"
}

func liveTVCapabilityState(s *Server) string {
	if s.db == nil {
		return "degraded"
	}
	var configured, healthy int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM live_tv_sources WHERE enabled = 1`).Scan(&configured); err != nil {
		return "degraded"
	}
	if configured == 0 {
		return "requires_configuration"
	}
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT s.id) FROM live_tv_sources s JOIN live_tv_channels c ON c.source_id = s.id WHERE s.enabled = 1 AND c.enabled = 1 AND TRIM(s.last_error) = ''`).Scan(&healthy); err != nil || healthy == 0 {
		return "degraded"
	}
	return "available"
}

func castCapabilityState(s *Server) string {
	if canonicalCastServerOrigin(s.cfg.PublicOrigin) == "" || len(s.cfg.CastReceiverOrigins) == 0 {
		return "requires_configuration"
	}
	for _, origin := range s.cfg.CastReceiverOrigins {
		if s.castReceiverOriginAllowed(origin) {
			return "available"
		}
	}
	return "degraded"
}

func availableServerCapabilityIDs(capabilities []CompatibilityCapability) []string {
	result := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability.State == "available" {
			result = append(result, capability.ID)
		}
	}
	return result
}

func (s *Server) operationalCapabilityStatuses() map[string]OperationalCapabilityStatus {
	statuses := map[string]OperationalCapabilityStatus{}
	for _, capability := range s.compatibilityEnvelope().Capabilities {
		status := OperationalCapabilityStatus{
			Supported:    capability.State != "unavailable",
			State:        capability.State,
			Revision:     capability.Revision,
			CacheSeconds: 30,
		}
		switch capability.State {
		case "requires_configuration":
			status.ReasonCode = "configuration_required"
		case "degraded":
			status.ReasonCode = "dependency_unavailable"
		case "unavailable":
			status.Supported = false
			status.ReasonCode = "route_unavailable"
		}
		switch capability.ID {
		case "live-tv":
			status.Remediation = "Configure and test an enabled Live TV source."
		case "playback.google-cast-custom-receiver":
			status.Remediation = "Configure a valid public origin and an allowed Cast receiver origin."
		case "remote-access.direct":
			status.Remediation = "Claim the server and complete certificate and reachability checks."
		case "downloads":
			status.Remediation = "Enable downloads and verify writable private storage."
		}
		if status.State == "available" {
			status.Remediation = ""
		}
		statuses[capability.ID] = status
	}
	return statuses
}

func serverCapabilityCatalogIDs() []string {
	result := make([]string, 0, len(serverCapabilityDefinitions))
	for _, definition := range serverCapabilityDefinitions {
		result = append(result, definition.ID)
	}
	return result
}
