package httpapi

import (
	"errors"
	"reflect"

	"portico.local/apikit"
	"portico.local/server/internal/playbackv1"
)

// playbackStartResponse keeps a pre-session wait distinct from a real session.
// The minimal branch is declared for adapter compatibility before admission is
// wired; the current handler only constructs the existing session-backed branch.
// Embedding preserves the real SessionView's JSON and nullable-field handling.
type playbackStartResponse struct {
	*playbackv1.SessionView
	RetryAfterMs int `json:"retryAfterMs,omitempty"`
}

func startSessionResponse(v playbackv1.SessionView, preparing bool) (playbackStartResponse, error) {
	if v.ID == "" || v.Presentation.Generation < 1 || preparing && (v.RetryAfterMs < 1 || v.RetryAfterMs > 10000) || !preparing && v.RetryAfterMs != 0 {
		return playbackStartResponse{}, errors.New("start did not return its real session response")
	}
	return playbackStartResponse{SessionView: &v, RetryAfterMs: v.RetryAfterMs}, nil
}

func startSessionSchema() map[string]any {
	shape, err := apikit.Schema(reflect.TypeFor[playbackv1.SessionView]())
	if err != nil {
		panic(err)
	}
	properties := shape["properties"].(map[string]any)
	properties["retryAfterMs"] = startRetrySchema()
	for _, key := range []string{"id", "revision"} {
		properties[key].(map[string]any)["minLength"] = 1
	}
	presentation := properties["presentation"].(map[string]any)["properties"].(map[string]any)
	presentation["generation"].(map[string]any)["minimum"] = 1
	return shape
}

func startRetrySchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": 10000}
}

func startWaitSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"retryAfterMs": startRetrySchema()},
		"required":   []string{"retryAfterMs"},
	}
}

func (playbackStartResponse) JSONSchema() map[string]any {
	return map[string]any{"anyOf": []any{startSessionSchema(), startWaitSchema()}}
}

func (playbackStartResponse) JSONSchemaForStatus(status int) map[string]any {
	switch status {
	case 201:
		shape := startSessionSchema()
		delete(shape["properties"].(map[string]any), "retryAfterMs")
		return shape
	case 202:
		shape := startSessionSchema()
		shape["required"] = append(shape["required"].([]string), "retryAfterMs")
		return map[string]any{"oneOf": []any{shape, startWaitSchema()}}
	default:
		return nil
	}
}
