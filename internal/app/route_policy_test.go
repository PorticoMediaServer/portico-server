package app

import (
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/app/apiroute"
)

func TestGeneratedRoutePrincipalMatrixUsesCentralPolicy(t *testing.T) {
	server := &Server{}
	server.Handler()
	if server.apiRegistry == nil {
		t.Fatal("server did not build the API registry")
	}

	owner := User{ID: "owner", AccountID: "owner", Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()}
	viewer := User{
		ID: "viewer", AccountID: "viewer", ProfileID: "viewer", ProfileIsPrimary: true, Role: "user", AuthProvider: "local",
		Permissions: map[string]bool{
			"playMedia": true, "downloadMedia": true, "editMetadata": true,
			"viewLiveTV": true, "playLiveTV": true, "viewDVR": true, "scheduleDVR": true,
		},
	}
	knownPermissions := map[string]bool{
		"public": true, "authenticated": true, "browse": true, "view-library": true,
		"manage-server": true, "manageServer": true, "manage-libraries": true, "manage-users": true,
		"download-media": true, "edit-metadata": true, "play-media": true,
		"play-live-tv": true, "view-live-tv": true, "view-dvr": true, "schedule-dvr": true,
	}
	viewerAllowed := map[string]bool{
		"authenticated": true, "browse": true, "view-library": true,
		"download-media": true, "edit-metadata": true, "play-media": true,
		"play-live-tv": true, "view-live-tv": true, "view-dvr": true, "schedule-dvr": true,
	}
	for _, route := range server.apiRegistry.Routes() {
		if !knownPermissions[route.Permission] {
			t.Fatalf("generated route has an unmapped permission %q: %#v", route.Permission, route)
		}
		if _, ok := routeRateLimitPolicy(route.RatePolicy); !ok {
			t.Fatalf("generated route has an unmapped rate class %q: %#v", route.RatePolicy, route)
		}
		if route.Auth == apiroute.AuthPublic {
			continue
		}
		if !routePermissionAllowed(owner, route.Permission) {
			t.Fatalf("owner was denied generated route permission %q: %#v", route.Permission, route)
		}
		if got := routePermissionAllowed(viewer, route.Permission); got != viewerAllowed[route.Permission] {
			t.Fatalf("viewer permission %q = %v, want %v", route.Permission, got, viewerAllowed[route.Permission])
		}
	}
}

func TestRoutePermissionPolicyFailsClosedForUnknownPermission(t *testing.T) {
	if routePermissionAllowed(User{Role: "owner"}, "future-permission") {
		t.Fatal("unknown route permission was allowed")
	}
}
