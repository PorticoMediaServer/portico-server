package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubordinateProfileCannotManageAccountCredentialsSessionsOrImage(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	viewer := User{
		ID:               "account_owner",
		AccountID:        "account_owner",
		ProfileID:        "profile_child",
		ProfileIsPrimary: false,
		Role:             "owner",
		Permissions:      map[string]bool{"playMedia": true},
	}

	for _, test := range []struct {
		name    string
		method  string
		path    string
		handler authedHandler
	}{
		{name: "change account password", method: http.MethodPost, path: "/api/account/password", handler: server.handleAccountPassword},
		{name: "list account sessions", method: http.MethodGet, path: "/api/account/sessions", handler: server.handleAccountSessions},
		{name: "revoke account session", method: http.MethodDelete, path: "/api/account/sessions/session_other", handler: server.handleAccountSessionRoute},
		{name: "upload account image", method: http.MethodPost, path: "/api/account/image", handler: server.handleAccountImage},
		{name: "delete account image", method: http.MethodDelete, path: "/api/account/image", handler: server.handleAccountImage},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://portico.test"+test.path, nil)
			response := httptest.NewRecorder()

			test.handler(response, request, viewer)

			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "primary_profile_required") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
