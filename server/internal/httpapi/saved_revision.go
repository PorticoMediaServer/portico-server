package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

// Decode a bounded JSON object once, then require explicit CAS evidence. Missing
// revision must not be interpreted as the valid revision-zero initial state.
func decodeSavedRevision(w http.ResponseWriter, r *http.Request, v any) error {
	var raw json.RawMessage
	if e := decode(w, r, &raw); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(raw, &fields); e != nil || fields == nil {
		return errors.New("JSON object required")
	}
	value, ok := fields["expectedRevision"]
	if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errors.New("expectedRevision is required")
	}
	var revision int64
	if e := json.Unmarshal(value, &revision); e != nil || revision < 0 || revision > 9007199254740991 {
		return errors.New("invalid expectedRevision")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("invalid JSON request")
	}
	return nil
}
