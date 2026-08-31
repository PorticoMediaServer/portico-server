package apiroute

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

func TestEveryGeneratedRouteHasOneValidWorkClassAndEveryOverrideIsUsed(t *testing.T) {
	registry := New(nilMux(), nil, nil)
	seenOperations := map[string]bool{}
	for contractPath, item := range registry.pathItems {
		for method, raw := range item {
			if !isHTTPMethod(strings.ToUpper(method)) {
				continue
			}
			var policy struct {
				Auth Auth `json:"x-portico-auth"`
			}
			if err := json.Unmarshal(raw, &policy); err != nil {
				t.Fatalf("decode %s %s: %v", method, contractPath, err)
			}
			route := RouteFromOperation(strings.ToUpper(method), "/api"+contractPath, raw, policy.Auth)
			if !route.WorkClass.Valid() {
				t.Fatalf("%s %s (%s) has invalid work class %q", method, contractPath, route.OperationID, route.WorkClass)
			}
			seenOperations[route.OperationID] = true
		}
	}
	for operationID, class := range operationWorkClass {
		if !seenOperations[operationID] {
			t.Errorf("work-class override %q does not name a generated operation", operationID)
		}
		if !class.Valid() {
			t.Errorf("work-class override %q uses invalid class %q", operationID, class)
		}
	}
}

func nilMux() *http.ServeMux {
	return http.NewServeMux()
}

func TestSecurityFenceOverridesUseTheHighestFoundationPriority(t *testing.T) {
	for _, operationID := range []string{
		"postAuthLogout", "deleteAccountSessionsId", "deleteAuthApiKeysId",
		"revokeNativeSession", "changeAccountPassword", "deleteDevicesId",
		"deletePlaybackSessionsSessionId", "revokePlaybackContinuation",
		"registerPlaybackReceiver", "revokePlaybackReceiverAuthorization",
	} {
		class := operationWorkClass[operationID]
		if class != foundationcontract.WorkClassSecurityFence || class.Priority() != 1 {
			t.Errorf("%s class=%q priority=%d", operationID, class, class.Priority())
		}
	}
}
