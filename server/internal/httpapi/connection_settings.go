package httpapi

import (
	"net/http"
	"portico.local/apikit"
	"portico.local/server/internal/operations"
)

type ConsoleSettingsScope struct {
	ServerID    string `json:"serverId"`
	ViewerFence string `json:"viewerFence"`
}
type ConsoleSettingsDocument struct {
	Scope ConsoleSettingsScope        `json:"scope"`
	Data  operations.SettingsDocument `json:"data"`
}

// Reuse the console's transactional authority recheck and complete-document
// validation. The registry supplies the typed contract and route admission.
func registerConnectionSettings(registry *apikit.Registry, d Dependencies) error {
	standalone := d
	standalone.registry = nil
	mux := http.NewServeMux()
	standalone.consoleRoutes(mux)
	err := apikit.RegisterRaw(registry, apikit.RawRoute[struct{}, ConsoleSettingsDocument]{Metadata: apikit.Metadata{
		ID: "get_console_settings", Method: "GET", Path: "/v1/admin/console/settings", Summary: "Read owner settings", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, Errors: []string{"unauthorized", "not_permitted", "internal"},
	}, Serve: mux.ServeHTTP})
	if err != nil {
		return err
	}
	return apikit.RegisterRaw(registry, apikit.RawRoute[operations.SettingsChange, ConsoleSettingsDocument]{Metadata: apikit.Metadata{
		ID: "change_console_settings", Method: "PATCH", Path: "/v1/admin/console/settings", Summary: "Save owner settings", Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, Status: 200, BodyLimit: 65536, Errors: []string{"unauthorized", "not_permitted", "invalid_request", "revision_mismatch", "request_in_progress", "internal"},
	}, Serve: mux.ServeHTTP})
}
