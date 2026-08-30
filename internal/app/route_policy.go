package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app/apiroute"
)

// routeRateLimitPolicy is deliberately conservative and bounded. The contract
// names the class; this table is the one runtime implementation of that class
// so handlers cannot silently choose a different limiter.
func routeRateLimitPolicy(class string) (quickConnectRatePolicy, bool) {
	var limit int
	switch strings.TrimSpace(class) {
	case "auth-sensitive":
		limit = 120
	case "admin-expensive":
		limit = 120
	case "state-mutation":
		limit = 300
	case "interactive-read":
		limit = 600
	case "long-poll":
		limit = 180
	case "media-delivery":
		limit = 1200
	case "playback-control":
		limit = 600
	case "search":
		limit = 240
	default:
		return quickConnectRatePolicy{}, false
	}
	return quickConnectRatePolicy{
		limit:  limit,
		window: time.Minute,
		code:   "route_rate_limited",
		detail: "This route is being requested too quickly. Try again shortly.",
	}, true
}

func routePermissionAllowed(user User, permission string) bool {
	switch strings.TrimSpace(permission) {
	case "public", "authenticated", "browse", "view-library":
		// The route registry and withAuth establish identity. Browse and library
		// visibility remain resource-specific and are checked by the handler.
		return true
	case "manage-server", "manageServer", "manage-libraries", "manage-users":
		return canInteractivelyManageServer(user)
	case "download-media":
		return hasPermission(user, "downloadMedia")
	case "edit-metadata":
		// The generated contract groups metadata writes under one route
		// permission. Resource handlers retain the narrower distinction between
		// general metadata, lyrics, and subtitle operations.
		return hasPermission(user, "editMetadata") || hasPermission(user, "manageLyrics") || hasPermission(user, "manageSubtitles")
	case "play-media":
		return hasPermission(user, "playMedia")
	case "play-live-tv":
		return canPlayLiveTV(user)
	case "view-live-tv":
		return canViewLiveTV(user)
	case "view-dvr":
		return canViewDVR(user)
	case "schedule-dvr":
		return canScheduleDVR(user)
	default:
		// Unknown generated permissions fail closed. Adding a permission requires
		// changing this table and its route-principal matrix tests together.
		return false
	}
}

func routePrincipalKey(user User, r *http.Request, class string) string {
	principal := strings.TrimSpace(user.APIKeyID)
	if principal == "" {
		principal = strings.TrimSpace(accountIDForUser(user))
	}
	if principal == "" {
		principal = strings.TrimSpace(user.ID)
	}
	if principal == "" {
		principal = strings.TrimSpace(user.AuthSessionID)
	}
	if principal == "" {
		principal = "anonymous-principal"
	}
	profile := strings.TrimSpace(viewerProfileID(user))
	ip := ""
	if r != nil {
		ip = clientIPFromRequest(r)
	}
	return strings.Join([]string{"route", class, principal, profile, ip}, "\x00")
}

func routeAuditRequired(route apiroute.Route) bool {
	switch route.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return route.RatePolicy == "auth-sensitive" || route.RatePolicy == "admin-expensive"
}

// enforceRoutePolicy is the common API middleware boundary. Resource handlers
// still enforce ownership, library visibility, profile policy, and operation
// state; this function owns generated principal permission, named rate class,
// and central audit-attempt policy.
func (s *Server) enforceRoutePolicy(w http.ResponseWriter, r *http.Request, user User) bool {
	route, ok := apiroute.RouteFromRequest(r)
	if isPorticoPrincipal(user) && !s.enforceHostedPolicyContinuity(w, r, route, ok) {
		return false
	}
	if !ok {
		writeProductError(w, http.StatusInternalServerError, "route_policy_misconfigured", "The request route is missing its generated authorization policy.")
		return false
	}
	if route.Auth == apiroute.AuthPublic {
		writeProductError(w, http.StatusInternalServerError, "route_policy_misconfigured", "The generated route authorization policy is inconsistent.")
		return false
	}
	if !routePermissionAllowed(user, route.Permission) {
		writeError(w, http.StatusForbidden, "route_permission_denied", "This account is not permitted to use that operation.")
		return false
	}
	policy, known := routeRateLimitPolicy(route.RatePolicy)
	if !known || strings.TrimSpace(route.AuditEvent) == "" || strings.TrimSpace(route.OperationID) == "" {
		writeProductError(w, http.StatusInternalServerError, "route_policy_misconfigured", "The generated route authorization policy is incomplete.")
		return false
	}
	allowed, retryAfter := s.routeLimiter.allow(routePrincipalKey(user, r, route.RatePolicy), policy, time.Now().UTC())
	if !allowed {
		w.Header().Set("Retry-After", itoa(retryAfter))
		writeError(w, http.StatusTooManyRequests, "route_rate_limited", policy.detail)
		return false
	}
	if routeAuditRequired(route) {
		s.recordAudit(r, user, route.AuditEvent, "api_route", route.OperationID, "info", map[string]string{
			"method":       route.Method,
			"pathTemplate": route.Path,
			"ratePolicy":   route.RatePolicy,
		})
	}
	return true
}

func (s *Server) enforceHostedPolicyContinuity(w http.ResponseWriter, r *http.Request, route apiroute.Route, routeKnown bool) bool {
	if routeKnown && hostedContinuityDenyOnlyOperation(route.OperationID) {
		// Expired Hosted authority must not trap a user in their own local session.
		// These exact operations only remove the caller's own authority; route
		// permissions and resource ownership are still enforced by the normal
		// middleware and handler after this continuity check. Server-wide denial
		// remains available to a locally authoritative owner, never to a stale
		// Hosted owner projection.
		return true
	}
	state := remotePolicyContinuity(s.loadRemotePolicyState(), time.Now().UTC())
	if state == "valid" {
		return true
	}
	w.Header().Set("Retry-After", "300")
	code := "hosted_authority_stale"
	message := "Portico Account authority expired before Hosted Services could be reached. Reconnect this server before continuing."
	if state == "clock-invalid" {
		code = "hosted_authority_clock_invalid"
		message = "This server's clock moved behind its last trusted Hosted time. Correct the clock and reconcile Portico Account authority."
	}
	writeProductError(w, http.StatusServiceUnavailable, code, message)
	return false
}

func hostedContinuityDenyOnlyOperation(operationID string) bool {
	switch strings.TrimSpace(operationID) {
	case "postAuthLogout",
		"deleteAccountSessionsId",
		"revokeAutomaticProfileTrusts":
		return true
	default:
		return false
	}
}

func itoa(value int) string {
	if value < 1 {
		return "1"
	}
	if value > 3600 {
		return "3600"
	}
	return strconv.Itoa(value)
}
