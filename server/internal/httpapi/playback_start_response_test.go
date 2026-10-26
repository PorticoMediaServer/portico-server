package httpapi

import (
	"encoding/json"
	"reflect"
	"testing"

	"portico.local/apikit"
	"portico.local/server/internal/apispec"
	"portico.local/server/internal/playbackv1"
)

func TestPlaybackStartMinimalWaitHasNoSessionAndIs202Only(t *testing.T) {
	response := playbackStartResponse{RetryAfterMs: 500}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"retryAfterMs":500}` {
		t.Fatalf("private preparation fabricated session fields: %s", raw)
	}
	validator := apispec.Document{}
	if issues := validator.ValidateJSON(response.JSONSchemaForStatus(202), raw); len(issues) != 0 {
		t.Fatal(issues)
	}
	if issues := validator.ValidateJSON(response.JSONSchemaForStatus(201), raw); len(issues) == 0 {
		t.Fatal("minimal wait accepted as a completed session")
	}
	for _, raw := range []string{
		`{}`, `{"retryAfterMs":0}`, `{"retryAfterMs":-1}`, `{"retryAfterMs":10001}`,
		`{"retryAfterMs":1.5}`, `{"retryAfterMs":"500"}`, `{"retryAfterMs":null}`,
		`{"retryAfterMs":500,"id":""}`, `{"retryAfterMs":500,"presentation":{}}`,
	} {
		if issues := validator.ValidateJSON(response.JSONSchemaForStatus(202), []byte(raw)); len(issues) == 0 {
			t.Fatalf("invalid pre-session response accepted: %s", raw)
		}
	}
}

func TestPlaybackStartProjectionRequiresGenuinePreparationOrRealSession(t *testing.T) {
	for _, wait := range []int{1, 500, 10000} {
		out, err := startPreparedResponse(playbackv1.PreparedStart{Preparing: true, RetryAfterMs: wait})
		if err != nil || out.SessionView != nil || out.RetryAfterMs != wait {
			t.Fatalf("pre-session wait gained a resource: %+v, %v", out, err)
		}
	}
	for _, invalid := range []playbackv1.PreparedStart{
		{}, {RetryAfterMs: 500}, {Preparing: true},
		{Preparing: true, RetryAfterMs: -1}, {Preparing: true, RetryAfterMs: 10001},
		{Session: &playbackv1.SessionView{}, Preparing: true, RetryAfterMs: 500},
	} {
		if _, err := startPreparedResponse(invalid); err == nil {
			t.Fatalf("invalid producer result became a start response: %+v", invalid)
		}
	}
}

func TestPlaybackStartWrapperKeepsActualSessionJSON(t *testing.T) {
	f := newV1Fixture(t, 1)
	var session playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "start-shape-00000001"}, startBody(f.items[0], nil), 201, &session)
	out, err := startSessionResponse(session, false)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := startPreparedResponse(playbackv1.PreparedStart{Session: &session})
	if err != nil || projected.SessionView == nil || projected.ID != session.ID || projected.RetryAfterMs != 0 {
		t.Fatalf("ordinary route result changed: %+v, %v", projected, err)
	}
	preparing := session
	preparing.RetryAfterMs = 500
	if _, err = startPreparedResponse(playbackv1.PreparedStart{Session: &preparing, Preparing: true, RetryAfterMs: 501}); err == nil {
		t.Fatal("a wait unrelated to the real session was accepted")
	}
	before, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(out)
	if err != nil || string(before) != string(after) {
		t.Fatalf("real session JSON changed: %v\n%s\n%s", err, before, after)
	}
	validator := apispec.Document{}
	for _, status := range []int{201, 202} {
		v := session
		if status == 202 {
			v.RetryAfterMs = 500
		}
		raw, _ := json.Marshal(v)
		projected, err := startPreparedResponse(playbackv1.PreparedStart{Session: &v, Preparing: status == 202, RetryAfterMs: v.RetryAfterMs})
		if err != nil {
			t.Fatalf("real session projection status %d: %v", status, err)
		}
		projectedJSON, err := json.Marshal(projected)
		if err != nil || string(projectedJSON) != string(raw) {
			t.Fatalf("real session projection status %d changed: %v\n%s\n%s", status, err, raw, projectedJSON)
		}
		if issues := validator.ValidateJSON(out.JSONSchemaForStatus(status), raw); len(issues) != 0 {
			t.Fatalf("real session status %d: %v", status, issues)
		}
	}
	if _, err := startSessionResponse(playbackv1.SessionView{}, true); err == nil {
		t.Fatal("empty SessionView became a preparation response")
	}
	registry, err := FoundationRegistry(f.deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range registry.Definitions() {
		if definition.ID == "start_playback_session" {
			if definition.Response != reflect.TypeFor[playbackStartResponse]() || !reflect.DeepEqual(definition.SuccessStatuses, []int{202}) {
				t.Fatal("start route lost its authoritative response declaration")
			}
			if _, err := apikit.Schema(definition.Response); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing start route")
}
