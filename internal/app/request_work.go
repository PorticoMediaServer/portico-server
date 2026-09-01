package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app/apiroute"
	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

type requestWorkDescriptor struct {
	Class foundationcontract.WorkClass
	Lane  string
	Route apiroute.Route
}

var adminHeavyRequestOperations = map[string]bool{
	"getAuditEvents":              true,
	"getLogs":                     true,
	"getMetadataHealth":           true,
	"getPlaybackHistory":          true,
	"getPlaybackHistoryExportCsv": true,
}

var mediaTransformRequestOperations = map[string]bool{
	"getArtworkIdKind":                          true,
	"getPersonArtwork":                          true,
	"getMediaIdAttachmentsAttachmentId":         true,
	"getMediaIdImagesImageId":                   true,
	"getMediaIdTrickplay":                       true,
	"getMediaIdTrickplaySetIdTilesM3u8":         true,
	"getMediaIdTrickplaySetIdTilesTileIndexJpg": true,
}

var mediaBodyRequestOperations = map[string]bool{
	"getMediaIdSubtitlesStreamId": true,
}

var expensiveRequestOperations = map[string]bool{
	"getInstantMixId":           true,
	"getMediaIdRecommendations": true,
	"getPlaylistsPlaylistId":    true,
	"getSuggestions":            true,
	"postSearch":                true,
}

func (s *Server) requestWork(r *http.Request) requestWorkDescriptor {
	if r == nil || r.URL == nil {
		return requestWorkDescriptor{}
	}
	route, ok := apiroute.RouteFromRequest(r)
	if !ok && s != nil && s.apiRegistry != nil {
		route, ok = s.apiRegistry.Match(r)
	}
	if ok {
		return requestWorkForRoute(r, route)
	}
	if strings.HasPrefix(r.URL.Path, "/dlna/") {
		return requestWorkDescriptor{Class: foundationcontract.WorkClassInteractive, Lane: workloadLaneDLNA}
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		// Unknown/method-mismatched API requests remain bounded, but receive no
		// semantic elevation because no generated descriptor authorized it.
		return requestWorkDescriptor{Class: foundationcontract.WorkClassInteractive, Lane: workloadLaneDefault}
	}
	return requestWorkDescriptor{}
}

func requestWorkForRoute(r *http.Request, route apiroute.Route) requestWorkDescriptor {
	descriptor := requestWorkDescriptor{Class: route.WorkClass, Route: route}
	if !descriptor.Class.Valid() {
		descriptor.Class = foundationcontract.WorkClassInteractive
	}
	if descriptor.Class == foundationcontract.WorkClassSecurityFence {
		descriptor.Lane = workloadLaneSecurityFence
		return descriptor
	}
	// These are exact operation-descriptor exceptions where the physical cost is
	// narrower than the shared rate policy. Keep the exception owned by the
	// operation ID; never infer it from an arbitrary path prefix.
	switch {
	case adminHeavyRequestOperations[route.OperationID], dashboardHistoryRequest(route.OperationID, r):
		descriptor.Lane = workloadLaneAdminHeavy
		return descriptor
	case mediaTransformRequestOperations[route.OperationID]:
		descriptor.Lane = workloadLaneMedia
		return descriptor
	case mediaBodyRequestOperations[route.OperationID]:
		descriptor.Lane = workloadLaneMediaBody
		return descriptor
	case expensiveRequestOperations[route.OperationID]:
		descriptor.Lane = workloadLaneExpensive
		return descriptor
	case route.OperationID == "getDashboard":
		descriptor.Lane = workloadLaneAdmin
		return descriptor
	}
	switch route.RatePolicy {
	case "long-poll":
		descriptor.Lane = workloadLaneRealtime
	case "auth-sensitive":
		descriptor.Lane = workloadLaneAuth
	case "admin-expensive":
		descriptor.Lane = workloadLaneAdminHeavy
	case "search":
		descriptor.Lane = workloadLaneExpensive
	case "playback-control":
		descriptor.Lane = workloadLanePlayback
	case "media-delivery":
		switch {
		case route.OperationID == "getLogsStream":
			descriptor.Lane = workloadLaneRealtime
		case descriptor.Class == foundationcontract.WorkClassForegroundTransfer:
			descriptor.Lane = workloadLaneBulkTransfer
		default:
			descriptor.Lane = workloadLaneMediaBody
		}
	case "interactive-read":
		switch {
		case route.Audience == "management":
			descriptor.Lane = workloadLaneAdmin
		default:
			descriptor.Lane = workloadLaneBrowsing
		}
	case "state-mutation":
		if route.OperationID == "browseLibrary" {
			descriptor.Lane = workloadLaneBrowsing
		} else if route.Audience == "management" {
			descriptor.Lane = workloadLaneAdmin
		} else {
			descriptor.Lane = workloadLaneDefault
		}
	default:
		descriptor.Lane = workloadLaneDefault
	}
	return descriptor
}

func dashboardHistoryRequest(operationID string, r *http.Request) bool {
	if operationID != "getDashboard" || r == nil || r.URL == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	sections := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sections")))
	return mode == "history" || strings.Contains(sections, "topusers") ||
		strings.Contains(sections, "playhistory") || strings.Contains(sections, "topplayed")
}

func requestWorkMayQueue(r *http.Request, descriptor requestWorkDescriptor) bool {
	if r == nil {
		return false
	}
	if descriptor.Class == foundationcontract.WorkClassSecurityFence ||
		descriptor.Class == foundationcontract.WorkClassEstablishedPlayback ||
		descriptor.Class == foundationcontract.WorkClassPlaybackStart {
		return true
	}
	if descriptor.Lane == workloadLaneRealtime {
		return false
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return descriptor.Lane != workloadLaneMediaBody && descriptor.Lane != workloadLaneBulkTransfer
	}
	return descriptor.Route.RatePolicy == "search" || descriptor.Route.OperationID == "browseLibrary"
}

func requestBudgetForWork(descriptor requestWorkDescriptor) time.Duration {
	if descriptor.Route.RatePolicy == "long-poll" {
		return (longPollMaximumWaitSeconds + 5) * time.Second
	}
	switch descriptor.Lane {
	case workloadLaneRealtime, workloadLaneMediaBody, workloadLaneBulkTransfer:
		return 0
	case workloadLaneSecurityFence, workloadLanePlayback:
		return 10 * time.Second
	case workloadLaneExpensive, workloadLaneAdminHeavy:
		return 10 * time.Second
	case workloadLaneAuth, workloadLaneBrowsing, workloadLaneAdmin, workloadLaneDLNA, workloadLaneDefault, workloadLaneMedia:
		return 5 * time.Second
	default:
		return 0
	}
}
