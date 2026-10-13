package httpapi

import (
	"net/http"
	"portico.local/apikit"
	"portico.local/server/internal/catalog"

	"portico.local/server/internal/operations"
	"strings"
)

type projectedScope struct {
	ServerID    string `json:"serverId"`
	ViewerFence string `json:"viewerFence"`
}
type projectedConsoleEnvelope[T any] struct {
	Scope projectedScope `json:"scope"`
	Data  T              `json:"data"`
}
type projectedListeningScope struct {
	ServerID  string `json:"serverId"`
	Authority string `json:"authority"`
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId"`
}
type projectedViewerEnvelope[T any] struct {
	Scope projectedListeningScope `json:"scope"`
	Data  T                       `json:"data"`
}
type projectedItems struct {
	Items      []catalog.Item `json:"items"`
	NextCursor string         `json:"nextCursor"`
}

var projectedPatterns = map[string]bool{
	"GET /v1/libraries/{id}/content":              true,
	"GET /v1/libraries/{id}/browse":               true,
	"GET /v1/libraries/{id}/discover":             true,
	"GET /v1/collections/{id}/items":              true,
	"GET /v1/items":                               true,
	"GET /v1/items/{id}":                          true,
	"GET /v1/items/{id}/detail":                   true,
	"PUT /v1/items/{id}/personal-state":           true,
	"PUT /v1/items/personal-state:batch":          true,
	"GET /v1/saved-resources/{id}/content":        true,
	"GET /v1/admin/play-history":                  true,
	"GET /v1/admin/play-history/summary":          true,
	"GET /v1/libraries/{id}/listening/selection":  true,
	"GET /v1/people/{id}":                         true,
	"GET /v1/notifications/inbox":                 true,
	"GET /v1/notifications/unread-count":          true,
	"POST /v1/feedback/reports":                   true,
	"POST /v1/admin/feedback/reports/{id}/status": true,
	"GET /v1/feedback/reports":                    true,
	"GET /v1/feedback/reports/{id}":               true,
	"GET /v1/admin/feedback/reports":              true,
	"GET /v1/admin/feedback/reports/{id}":         true,
	"GET /v1/feedback":                            true,
	"GET /v1/feedback/{id}":                       true,
	"GET /v1/admin/feedback":                      true,
	"GET /v1/admin/feedback/{id}":                 true,
	"POST /v1/feedback":                           true,
	"PATCH /v1/admin/feedback/{id}":               true,
	"GET /v1/notifications":                       true,
	"GET /v1/admin/console/jobs":                  true,
	"POST /v1/admin/console/jobs":                 true,
	"POST /v1/admin/console/jobs/{id}/{action}":   true,
	"GET /v1/admin/console/schedules":             true,
	"PUT /v1/admin/console/schedules":             true,
	"GET /v1/admin/console/panels/{panel}":        true,
	"GET /v1/admin/console/diagnostics/{lane}":    true,
	"POST /v1/admin/console/diagnostics/capture":  true,
	"POST /v1/admin/console/exports":              true,
	"GET /v1/admin/console/exports/{id}":          true,
	"GET /v1/admin/playback/history":              true,
	"GET /v1/admin/console/alerts":                true,
}

func (d Dependencies) projectedHandle(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	if d.registry != nil && projectedPatterns[pattern] {
		return
	}
	mux.HandleFunc(pattern, handler)
}
func addProjection[Request, Response any](registry *apikit.Registry, mux *http.ServeMux, pattern string, query []string) error {
	method, path, _ := strings.Cut(pattern, " ")
	scope := routeScopes[pattern]
	lane := apikit.Default
	if routeLanes[pattern] == laneBrowsing {
		lane = apikit.Browsing
	}
	id := strings.ToLower(method) + "_" + strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_", ":", "_").Replace(strings.TrimPrefix(path, "/v1/"))
	return apikit.RegisterRaw(registry, apikit.RawRoute[Request, Response]{Metadata: apikit.Metadata{ID: id, Method: method, Path: path, Summary: "Read or update the selected projection", Access: apikit.Device, Requires: scope, Lane: lane, Cost: apikit.PageSized, Status: 200, BodyLimit: 1048576, Query: query, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}, Serve: func(w http.ResponseWriter, r *http.Request) {
		// A projection whose handler this server did not mount (no scheduler,
		// no console) is an unknown route and answers like one.
		if _, mounted := mux.Handler(r); mounted == "" {
			write(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": "not_found", "message": "The request could not be completed.", "retryable": false}})
			return
		}
		mux.ServeHTTP(w, r)
	}})
}
func registerUnitProjections(registry *apikit.Registry, d Dependencies) error {
	private := d
	private.registry = nil
	mux := http.NewServeMux()
	private.discoveryRoutes(mux)
	private.playHistoryRoutes(mux)
	private.peopleRoutes(mux)
	private.listeningRoutes(mux)
	private.consoleRoutes(mux)
	private.notificationRoutes(mux)
	mux.HandleFunc("GET /v1/items", private.itemsReadHandler)
	mux.HandleFunc("GET /v1/items/{id}", private.itemReadHandler)
	if err := addProjection[noBody, catalog.ContentEnvelope](registry, mux, "GET /v1/libraries/{id}/content", []string{"view", "entityId", "sort", "direction", "category", "q", "cursor", "limit", "start", "section"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedItems](registry, mux, "GET /v1/libraries/{id}/browse", []string{"sort", "direction", "category", "q", "cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.Discovery](registry, mux, "GET /v1/libraries/{id}/discover", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedItems](registry, mux, "GET /v1/collections/{id}/items", []string{"sort", "direction", "category", "q", "cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedItems](registry, mux, "GET /v1/items", []string{"libraryId", "cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.Item](registry, mux, "GET /v1/items/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.Detail](registry, mux, "GET /v1/items/{id}/detail", []string{"related"}); err != nil {
		return err
	}
	if err := addProjection[catalog.PersonalMutation, catalog.PersonalReceipt](registry, mux, "PUT /v1/items/{id}/personal-state", nil); err != nil {
		return err
	}
	if err := addProjection[catalog.PersonalBatchMutation, catalog.PersonalBatchReceipt](registry, mux, "PUT /v1/items/personal-state:batch", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.SavedResourceContent](registry, mux, "GET /v1/saved-resources/{id}/content", []string{"cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.PlayHistoryPage](registry, mux, "GET /v1/admin/play-history", []string{"accountId", "libraryId", "period", "cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.PlaySummary](registry, mux, "GET /v1/admin/play-history/summary", []string{"period"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedViewerEnvelope[catalog.ListeningSelection]](registry, mux, "GET /v1/libraries/{id}/listening/selection", []string{"kind", "entityId", "cursor", "limit", "mode", "seed", "resume", "startItem"}); err != nil {
		return err
	}
	if err := addProjection[noBody, catalog.PersonPage](registry, mux, "GET /v1/people/{id}", []string{"role", "cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.NoticeInbox]](registry, mux, "GET /v1/notifications/inbox", []string{"audience", "state", "cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.NoticeUnread]](registry, mux, "GET /v1/notifications/unread-count", []string{"audience"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.FeedbackList]](registry, mux, "GET /v1/feedback/reports", []string{"cursor", "limit", "status", "kind", "category", "itemId", "reporter"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.FeedbackReport]](registry, mux, "GET /v1/feedback/reports/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.FeedbackList]](registry, mux, "GET /v1/admin/feedback/reports", []string{"cursor", "limit", "status", "kind", "category", "itemId", "reporter"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.FeedbackReport]](registry, mux, "GET /v1/admin/feedback/reports/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Page[operations.Report]]](registry, mux, "GET /v1/feedback", []string{"cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Report]](registry, mux, "GET /v1/feedback/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Page[operations.Report]]](registry, mux, "GET /v1/admin/feedback", []string{"cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Report]](registry, mux, "GET /v1/admin/feedback/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[operations.SubmitReport, projectedConsoleEnvelope[operations.Report]](registry, mux, "POST /v1/feedback", nil); err != nil {
		return err
	}
	if err := addProjection[operations.Triage, projectedConsoleEnvelope[operations.Report]](registry, mux, "PATCH /v1/admin/feedback/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Inbox]](registry, mux, "GET /v1/notifications", []string{"cursor", "limit"}); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Page[operations.Job]]](registry, mux, "GET /v1/admin/console/jobs", []string{"cursor"}); err != nil {
		return err
	}
	if err := addProjection[operations.RunJob, projectedConsoleEnvelope[operations.Job]](registry, mux, "POST /v1/admin/console/jobs", nil); err != nil {
		return err
	}
	if err := addProjection[operations.JobCommand, projectedConsoleEnvelope[operations.Job]](registry, mux, "POST /v1/admin/console/jobs/{id}/{action}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[[]operations.Schedule]](registry, mux, "GET /v1/admin/console/schedules", nil); err != nil {
		return err
	}
	if err := addProjection[operations.ScheduleChange, projectedConsoleEnvelope[operations.Schedule]](registry, mux, "PUT /v1/admin/console/schedules", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Measurement]](registry, mux, "GET /v1/admin/console/panels/{panel}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Page[operations.Record]]](registry, mux, "GET /v1/admin/console/diagnostics/{lane}", []string{"cursor"}); err != nil {
		return err
	}
	if err := addProjection[operations.Capture, projectedConsoleEnvelope[operations.Capture]](registry, mux, "POST /v1/admin/console/diagnostics/capture", nil); err != nil {
		return err
	}
	if err := addProjection[operations.ExportRequest, projectedConsoleEnvelope[operations.Export]](registry, mux, "POST /v1/admin/console/exports", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Export]](registry, mux, "GET /v1/admin/console/exports/{id}", nil); err != nil {
		return err
	}
	if err := addProjection[noBody, projectedConsoleEnvelope[[]operations.Alert]](registry, mux, "GET /v1/admin/console/alerts", nil); err != nil {
		return err
	}
	if err := addProjection[operations.FeedbackSubmission, projectedConsoleEnvelope[operations.FeedbackSubmissionResult]](registry, mux, "POST /v1/feedback/reports", nil); err != nil {
		return err
	}
	if err := addProjection[operations.FeedbackTransition, projectedConsoleEnvelope[operations.FeedbackReport]](registry, mux, "POST /v1/admin/feedback/reports/{id}/status", nil); err != nil {
		return err
	}
	historyService := &telemetryService{d: private}
	historyService.register(mux, "GET /v1/admin/playback/history", []string{"period", "limit", "cursor"}, func(r *http.Request, a operations.Authorize) (any, error) {
		period, limit, err := historyQuery(r)
		if err != nil {
			return nil, err
		}
		return private.Console.PlaybackHistory(r.Context(), a, period, r.URL.Query().Get("cursor"), limit)
	})
	if err := addProjection[noBody, projectedConsoleEnvelope[operations.Page[operations.PlaybackHistoryEntry]]](registry, mux, "GET /v1/admin/playback/history", []string{"period", "limit", "cursor"}); err != nil {
		return err
	}
	return nil
}
