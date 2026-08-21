// Package apiroute owns the main server's JSON API route catalog and runtime
// registration. The embedded contract is generated; handlers cannot mount an
// /api route without passing through Registry.
package apiroute

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

//go:embed contract.json
var contractJSON []byte

type Auth string

const (
	AuthPublic  Auth = "public"
	AuthSession Auth = "session"
	AuthMedia   Auth = "media-grant-or-session"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

type routeContextKey struct{}

// WithRoute records the generated contract route selected for a request. The
// application auth adapter uses this single route authority for permission,
// rate-policy, and audit metadata checks.
func WithRoute(req *http.Request, route Route) *http.Request {
	if req == nil {
		return nil
	}
	return req.WithContext(context.WithValue(req.Context(), routeContextKey{}, route))
}

// RouteFromRequest returns the generated route selected by Registry. Requests
// entering handlers outside the API registry intentionally have no route
// metadata and must use their dedicated non-registry authorization path.
func RouteFromRequest(req *http.Request) (Route, bool) {
	if req == nil {
		return Route{}, false
	}
	route, ok := req.Context().Value(routeContextKey{}).(Route)
	return route, ok
}

type Route struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	OperationID    string   `json:"operationId"`
	Auth           Auth     `json:"auth"`
	Permission     string   `json:"permission"`
	Audience       string   `json:"audience"`
	Surfaces       []string `json:"surfaces"`
	RatePolicy     string   `json:"ratePolicy"`
	AuditEvent     string   `json:"auditEvent"`
	RequestSchema  string   `json:"requestSchema,omitempty"`
	ResponseSchema string   `json:"responseSchema"`
	SuccessStatus  int      `json:"successStatus"`
	TypedAdapter   bool     `json:"typedAdapter"`
}

type contractDocument struct {
	Paths map[string]json.RawMessage `json:"paths"`
}

type Registry struct {
	mux       *http.ServeMux
	session   Middleware
	media     Middleware
	mu        sync.RWMutex
	routes    []Route
	mounted   map[string]struct{}
	handlers  map[string]http.HandlerFunc
	contract  contractDocument
	pathItems map[string]map[string]json.RawMessage
}

func New(mux *http.ServeMux, session Middleware, media Middleware) *Registry {
	var doc contractDocument
	if err := json.Unmarshal(contractJSON, &doc); err != nil {
		panic(fmt.Errorf("decode generated API contract: %w", err))
	}
	r := &Registry{mux: mux, session: session, media: media, mounted: map[string]struct{}{}, handlers: map[string]http.HandlerFunc{}, contract: doc, pathItems: map[string]map[string]json.RawMessage{}}
	for path, raw := range doc.Paths {
		r.pathItems[path] = r.resolvePathItem(raw)
	}
	mux.HandleFunc("/api", r.ServeHTTP)
	mux.HandleFunc("/api/", r.ServeHTTP)
	return r
}

func (r *Registry) resolvePathItem(raw json.RawMessage) map[string]json.RawMessage {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		panic(fmt.Errorf("decode API path item: %w", err))
	}
	if refRaw, ok := item["$ref"]; ok {
		var ref string
		_ = json.Unmarshal(refRaw, &ref)
		const prefix = "#/paths/"
		if strings.HasPrefix(ref, prefix) {
			key := strings.TrimPrefix(ref, prefix)
			key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
			if target, exists := r.contract.Paths[key]; exists {
				return r.resolvePathItem(target)
			}
		}
	}
	return item
}

func (r *Registry) Public(pattern string, handler http.HandlerFunc) {
	r.registerFamily(pattern, AuthPublic, handler)
}

func (r *Registry) Session(pattern string, handler http.HandlerFunc) {
	r.registerFamily(pattern, AuthSession, handler)
}

func (r *Registry) Media(pattern string, handler http.HandlerFunc) {
	r.registerFamily(pattern, AuthMedia, handler)
}

func (r *Registry) registerFamily(pattern string, auth Auth, handler http.HandlerFunc) {
	if !strings.HasPrefix(pattern, "/api/") && pattern != "/api" {
		panic("apiroute: non-API pattern passed to registry: " + pattern)
	}
	wrapper := Middleware(func(h http.HandlerFunc) http.HandlerFunc { return h })
	if auth == AuthSession && r.session != nil {
		wrapper = r.session
	}
	if auth == AuthMedia && r.media != nil {
		wrapper = r.media
	}
	matched := 0
	paths := make([]string, 0, len(r.pathItems))
	for path := range r.pathItems {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, contractPath := range paths {
		fullPath := "/api" + contractPath
		if !familyMatches(pattern, fullPath) {
			continue
		}
		for method, raw := range r.pathItems[contractPath] {
			method = strings.ToUpper(method)
			if !isHTTPMethod(method) {
				continue
			}
			// A more-specific family may have already claimed this operation with
			// a different auth transport (for example, a credential-only
			// continuation nested under the session family). Do not re-validate or
			// remount the same generated operation through the broader family.
			r.mu.RLock()
			_, alreadyMounted := r.mounted[method+" "+fullPath]
			r.mu.RUnlock()
			if alreadyMounted {
				continue
			}
			r.mount(RouteFromOperation(method, fullPath, raw, auth), wrapper(handler))
			matched++
		}
	}
	if matched == 0 {
		panic("apiroute: runtime registration has no explicit OpenAPI operation: " + pattern)
	}
}

func familyMatches(pattern, candidate string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(candidate, pattern)
	}
	return candidate == pattern
}

func RouteFromOperation(method, path string, raw json.RawMessage, auth Auth) Route {
	var op struct {
		OperationID string   `json:"operationId"`
		RuntimePath string   `json:"x-portico-runtime-path"`
		Auth        Auth     `json:"x-portico-auth"`
		Permission  string   `json:"x-portico-permission"`
		Audience    string   `json:"x-portico-audience"`
		Surfaces    []string `json:"x-portico-surfaces"`
		RatePolicy  string   `json:"x-portico-rate-policy"`
		AuditEvent  string   `json:"x-portico-audit-event"`
		Responses   map[string]struct {
			Content map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"responses"`
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"content"`
		} `json:"requestBody"`
	}
	_ = json.Unmarshal(raw, &op)
	authMatches := op.Auth == auth || (auth == AuthMedia && op.Auth == AuthSession)
	if !authMatches {
		panic(fmt.Sprintf("apiroute: auth mismatch for %s %s: contract=%s registration=%s", method, path, op.Auth, auth))
	}
	if strings.TrimSpace(op.Permission) == "" || strings.TrimSpace(op.RatePolicy) == "" || strings.TrimSpace(op.AuditEvent) == "" {
		panic(fmt.Sprintf("apiroute: incomplete policy metadata for %s %s", method, path))
	}
	if err := validateAudienceSurfaces(op.Audience, op.Surfaces); err != nil {
		panic(fmt.Sprintf("apiroute: invalid client audience metadata for %s %s: %v", method, path, err))
	}
	if strings.TrimSpace(op.RuntimePath) != "" {
		path = op.RuntimePath
	}
	status, response := successSchema(op.Responses)
	request := ""
	if media, ok := op.RequestBody.Content["application/json"]; ok {
		request = schemaName(media.Schema.Ref)
	}
	if response == "" {
		panic(fmt.Sprintf("apiroute: missing response schema for %s %s", method, path))
	}
	return Route{Method: method, Path: path, OperationID: op.OperationID, Auth: op.Auth, Permission: op.Permission, Audience: op.Audience, Surfaces: append([]string(nil), op.Surfaces...), RatePolicy: op.RatePolicy, AuditEvent: op.AuditEvent, RequestSchema: request, ResponseSchema: response, SuccessStatus: status, TypedAdapter: true}
}

func validateAudienceSurfaces(audience string, surfaces []string) error {
	allowed := map[string]bool{}
	switch audience {
	case "viewer":
		allowed = map[string]bool{"web": true, "mobile": true, "television": true}
	case "management":
		allowed = map[string]bool{"web-admin": true}
	default:
		return fmt.Errorf("audience %q is not viewer or management", audience)
	}
	if len(surfaces) == 0 {
		return fmt.Errorf("surfaces must not be empty")
	}
	seen := map[string]bool{}
	for _, surface := range surfaces {
		if !allowed[surface] {
			return fmt.Errorf("surface %q is not valid for %s", surface, audience)
		}
		if seen[surface] {
			return fmt.Errorf("surface %q is duplicated", surface)
		}
		seen[surface] = true
	}
	return nil
}

func successSchema(responses map[string]struct {
	Content map[string]struct {
		Schema struct {
			Ref string `json:"$ref"`
		} `json:"schema"`
	} `json:"content"`
}) (int, string) {
	for _, code := range []string{"200", "201", "202", "204", "206", "302"} {
		response, ok := responses[code]
		if !ok {
			continue
		}
		if code == "204" {
			return http.StatusNoContent, "NoContent"
		}
		mediaTypes := make([]string, 0, len(response.Content))
		for mediaType := range response.Content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		if _, ok := response.Content["application/json"]; ok {
			mediaTypes = append([]string{"application/json"}, mediaTypes...)
		}
		seen := map[string]bool{}
		for _, mediaType := range mediaTypes {
			if seen[mediaType] {
				continue
			}
			seen[mediaType] = true
			if name := schemaName(response.Content[mediaType].Schema.Ref); name != "" {
				return atoiStatus(code), name
			}
		}
		return atoiStatus(code), ""
	}
	return 0, ""
}

func atoiStatus(code string) int {
	status := 0
	for _, ch := range code {
		status = status*10 + int(ch-'0')
	}
	return status
}

func schemaName(ref string) string { return strings.TrimPrefix(ref, "#/components/schemas/") }

func (r *Registry) mount(route Route, handler http.HandlerFunc) {
	if route.OperationID == "" {
		panic("apiroute: generated operation is missing operationId: " + route.Method + " " + route.Path)
	}
	key := route.Method + " " + route.Path
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.mounted[key]; exists {
		// A more-specific family registration can claim an operation before its
		// resource catch-all family. The first (most-specific, by routes.go
		// ordering) registration owns dispatch and metadata.
		return
	}
	r.mounted[key] = struct{}{}
	r.routes = append(r.routes, route)
	r.handlers[key] = handler
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	best := -1
	bestScore := -1
	allowed := map[string]struct{}{}
	for index, route := range r.routes {
		if !matchPath(route.Path, req.URL.Path) {
			continue
		}
		allowed[route.Method] = struct{}{}
		if route.Method != req.Method {
			continue
		}
		if score := pathSpecificity(route.Path); score > bestScore {
			best, bestScore = index, score
		}
	}
	if best >= 0 {
		route := r.routes[best]
		handler := r.handlers[route.Method+" "+route.Path]
		r.mu.RUnlock()
		handler(w, WithRoute(req, route))
		return
	}
	if len(allowed) > 0 {
		methods := make([]string, 0, len(allowed))
		for method := range allowed {
			methods = append(methods, method)
		}
		r.mu.RUnlock()
		sort.Strings(methods)
		w.Header().Set("Allow", strings.Join(methods, ", "))
		writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "The API resource does not support this HTTP method.")
		return
	}
	r.mu.RUnlock()
	writeProblem(w, http.StatusNotFound, "not_found", "The API resource was not found.")
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	requestID := strings.TrimSpace(w.Header().Get("X-Request-ID"))
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":      "https://portico.media/problems/" + strings.ReplaceAll(code, "_", "-"),
		"title":     http.StatusText(status),
		"status":    status,
		"code":      code,
		"detail":    detail,
		"requestId": requestID,
	})
}

func matchPath(template, actual string) bool {
	want := strings.Split(strings.Trim(template, "/"), "/")
	got := strings.Split(strings.Trim(actual, "/"), "/")
	for index := 0; index < len(want); index++ {
		if index >= len(got) {
			return false
		}
		segment := want[index]
		open, close := strings.IndexByte(segment, '{'), strings.IndexByte(segment, '}')
		if open < 0 || close <= open {
			if segment != got[index] {
				return false
			}
			continue
		}
		if strings.HasSuffix(segment[open+1:close], "...") {
			return true
		}
		if !strings.HasPrefix(got[index], segment[:open]) || !strings.HasSuffix(got[index], segment[close+1:]) || len(got[index]) < open+len(segment)-close-1 {
			return false
		}
	}
	return len(want) == len(got)
}

func pathSpecificity(path string) int {
	score := len(strings.Split(strings.Trim(path, "/"), "/")) * 10
	inWildcard := false
	for _, ch := range path {
		switch ch {
		case '{':
			inWildcard = true
		case '}':
			inWildcard = false
		default:
			if !inWildcard {
				score++
			}
		}
	}
	return score
}

func (r *Registry) Routes() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := append([]Route(nil), r.routes...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Method < result[j].Method
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func (r *Registry) DeclaredOperationCount() int {
	count := 0
	for _, item := range r.pathItems {
		for method := range item {
			if isHTTPMethod(strings.ToUpper(method)) {
				count++
			}
		}
	}
	return count
}

func isHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}
