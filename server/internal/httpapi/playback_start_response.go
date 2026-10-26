package httpapi

import (
	"errors"
	"reflect"

	"portico.local/apikit"
	"portico.local/server/internal/playbackv1"
)

// playbackStartResponse keeps a pre-session wait distinct from a real session.
// The minimal branch belongs only to a still-current original preparation;
// ordinary starts construct the existing session-backed branch.
// Embedding preserves the real SessionView's JSON and nullable-field handling.
type playbackStartResponse struct {
	*playbackv1.SessionView
	RetryAfterMs int `json:"retryAfterMs,omitempty"`
}

func startPreparedResponse(v playbackv1.PreparedStart) (playbackStartResponse, error) {
	if v.Session != nil {
		if v.RetryAfterMs != v.Session.RetryAfterMs {
			return playbackStartResponse{}, errors.New("start retry does not match its real session")
		}
		return startSessionResponse(*v.Session, v.Preparing)
	}
	if !v.Preparing || v.RetryAfterMs < 1 || v.RetryAfterMs > 10000 {
		return playbackStartResponse{}, errors.New("start did not return a current preparation or a real session")
	}
	return playbackStartResponse{RetryAfterMs: v.RetryAfterMs}, nil
}

func startSessionResponse(v playbackv1.SessionView, preparing bool) (playbackStartResponse, error) {
	// The real channel session/lease precedes its asynchronous timeshift buffer.
	// Generation zero is that exact pending projection, never a made-up VOD generation.
	coldChannel := !preparing && (v.Kind == "live" || v.Kind == "channel") && v.Presentation.Mode == "stream" &&
		v.Presentation.Generation == 0 && v.Presentation.URL == "" &&
		v.Presentation.Linear != nil && v.Presentation.Linear.State == "preparing"
	if v.ID == "" || v.Revision == "" || v.Presentation.Generation < 1 && !coldChannel || preparing && (v.RetryAfterMs < 1 || v.RetryAfterMs > 10000) || !preparing && v.RetryAfterMs != 0 {
		return playbackStartResponse{}, errors.New("start did not return its real session response")
	}
	return playbackStartResponse{SessionView: &v, RetryAfterMs: v.RetryAfterMs}, nil
}

func startSessionSchema(coldChannel bool) map[string]any {
	shape, err := apikit.Schema(reflect.TypeFor[playbackv1.SessionView]())
	if err != nil {
		panic(err)
	}
	properties := shape["properties"].(map[string]any)
	properties["retryAfterMs"] = startRetrySchema()
	for _, key := range []string{"id", "revision"} {
		properties[key].(map[string]any)["minLength"] = 1
	}
	view := properties["presentation"].(map[string]any)
	presentation := view["properties"].(map[string]any)
	if coldChannel {
		// This is an accepted channel (201); its normal linear refresh observes
		// the later buffer. It is not a server-directed start retry (202).
		delete(properties, "retryAfterMs")
		properties["kind"].(map[string]any)["enum"] = []any{"live", "channel"}
		presentation["generation"].(map[string]any)["const"] = 0
		presentation["mode"].(map[string]any)["const"] = "stream"
		presentation["url"].(map[string]any)["const"] = ""
		view["required"] = append(view["required"].([]string), "linear")
		linear := presentation["linear"].(map[string]any)["properties"].(map[string]any)
		linear["state"].(map[string]any)["const"] = "preparing"
	} else {
		presentation["generation"].(map[string]any)["minimum"] = 1
	}
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

// Both alternatives retain the actual SessionView. Only their real media
// generation/linear state differs; neither is the private retry-only branch.
func startSessionSchemas(status int) []any {
	var out []any
	for _, cold := range []bool{false, true} {
		if cold && status == 202 {
			continue
		}
		shape := startSessionSchema(cold)
		if status == 201 {
			delete(shape["properties"].(map[string]any), "retryAfterMs")
		} else if status == 202 {
			shape["required"] = append(shape["required"].([]string), "retryAfterMs")
		}
		out = append(out, shape)
	}
	return out
}

func (playbackStartResponse) JSONSchema() map[string]any {
	return map[string]any{"anyOf": append(startSessionSchemas(0), startWaitSchema())}
}

func (playbackStartResponse) JSONSchemaForStatus(status int) map[string]any {
	switch status {
	case 201:
		return map[string]any{"oneOf": startSessionSchemas(201)}
	case 202:
		return map[string]any{"oneOf": append(startSessionSchemas(202), startWaitSchema())}
	default:
		return nil
	}
}
