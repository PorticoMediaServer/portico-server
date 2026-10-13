package httpapi

import (
	"encoding/json"
	"portico.local/server/internal/playbackv1"
	"testing"
)

func TestAdministratorControlsAnotherProfilesSessionWithoutMediaURLs(t *testing.T) {
	f := newV1Fixture(t, 1)
	member, item := f.member()
	w := f.raw("POST", "/v1/playback/sessions", member, map[string]string{"Idempotency-Key": "member-control-start"}, startBody(item, nil))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var session playbackv1.SessionView
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	var controls playbackv1.SessionControls
	f.call("GET", "/v1/admin/sessions/"+session.ID+"/controls", nil, nil, 200, &controls)
	f.call("POST", "/v1/admin/sessions/"+session.ID+"/controls", nil, playbackv1.ControlCommand{Action: "pause", ExpectedRevision: controls.Revision}, 200, &controls)
	if controls.State != "paused" {
		t.Fatal(controls)
	}
	position := int64(9000)
	f.call("POST", "/v1/admin/sessions/"+session.ID+"/controls", nil, playbackv1.ControlCommand{Action: "seek", PositionMs: &position, ExpectedRevision: controls.Revision}, 200, &controls)
	if controls.PositionMs != 9000 {
		t.Fatal(controls)
	}
	f.call("POST", "/v1/admin/sessions/"+session.ID+"/controls", nil, playbackv1.ControlCommand{Action: "resume", ExpectedRevision: controls.Revision}, 200, &controls)
	if controls.State != "playing" {
		t.Fatal(controls)
	}
	w = f.raw("POST", "/v1/admin/sessions/"+session.ID+"/controls", member, nil, map[string]any{"action": "pause", "expectedRevision": controls.Revision})
	if w.Code != 403 {
		t.Fatal("member controlled admin transport", w.Code, w.Body.String())
	}
}
