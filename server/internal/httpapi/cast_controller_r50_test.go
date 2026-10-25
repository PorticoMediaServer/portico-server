package httpapi

import (
	"portico.local/server/internal/social"
	"strings"
	"testing"
)

// Source regression for Release's normal checkpoint: no execution in the
// per-item implementation loop. Real router/store/family checks, no auth mocks.
func TestReturningCastControllerRequiresOriginalAndCurrentBothFamilies(t *testing.T) {
	f := socialHTTP(t)
	var boot social.CastBootstrapResponse
	f.decode(f.as(f.token, "POST", "/v1/cast/bootstrap", social.CastBootstrapRequest{ProtocolVersion: social.Protocol, ReceiverID: "sdk-tv-1"}), 201, &boot)
	var target social.CastReceiverSession
	f.decode(f.as("", "POST", "/v1/cast/redeem", social.CastRedeemRequest{ProtocolVersion: social.Protocol, Code: boot.Bootstrap.Code, DeviceID: "cast-r50"}), 200, &target)
	var registered social.CastControllerResponse
	proof := strings.Repeat("a", 48)
	f.decode(f.as(target.Session.AccessToken, "POST", "/v1/cast/controllers/register", social.CastControllerRegister{BootstrapID: boot.Bootstrap.ID, ControlToken: proof}), 200, &registered)
	validate := social.CastControllerValidation{Binding: registered.Binding, AttemptID: "returning-1"}
	var result social.CastControllerResponse
	f.decode(f.as(f.token, "POST", "/v1/cast/controllers/validate", validate), 200, &result)
	check := social.CastControllerCheck{BindingID: result.Binding.ID, ControlToken: proof, AttemptID: result.AttemptID, Ticket: result.Ticket}
	f.decode(f.as(target.Session.AccessToken, "POST", "/v1/cast/controllers/check", check), 200, &social.CastControllerResponse{})
	// Neither another viewer nor the receiver may exercise sender validation.
	for _, bearer := range []string{f.member, target.Session.AccessToken} {
		if w := f.as(bearer, "POST", "/v1/cast/controllers/validate", validate); w.Code != 403 {
			t.Fatalf("foreign sender validation: %d", w.Code)
		}
	}
	changed := validate
	changed.Binding.TargetGeneration = "2"
	if w := f.as(f.token, "POST", "/v1/cast/controllers/validate", changed); w.Code != 403 {
		t.Fatalf("changed binding: %d", w.Code)
	}
	check.Ticket = strings.Repeat("b", 43)
	if w := f.as(target.Session.AccessToken, "POST", "/v1/cast/controllers/check", check); w.Code != 403 {
		t.Fatalf("invented ticket: %d", w.Code)
	}
	// A replacement receiver family/generation cannot recover the original proof.
	var renewed social.CastReceiverSession
	f.decode(f.as("", "POST", "/v1/cast/reconnect", social.CastReconnectRequest{ProtocolVersion: social.Protocol, DeviceToken: target.DeviceToken, DeviceID: "cast-r50"}), 200, &renewed)
	if w := f.as(f.token, "POST", "/v1/cast/controllers/validate", validate); w.Code != 403 {
		t.Fatalf("replaced target validation: %d", w.Code)
	}
	// Refusal leaves actual target state alone; no Stop/queue/media grant exists here.
	var state string
	if e := f.db.QueryRow(`SELECT state FROM social_cast_devices WHERE id=?`, target.Device.ID).Scan(&state); e != nil || state != "active" {
		t.Fatalf("refusal mutated target: %s %v", state, e)
	}
}
