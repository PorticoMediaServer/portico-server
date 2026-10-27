package playbackv1

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// UnmarshalJSON distinguishes omission from null, and refuses duplicate
// generation fields rather than letting a conditional stop become ordinary.
func (s *StopRequest) UnmarshalJSON(raw []byte) error {
	type plain StopRequest
	var parsed plain
	d := json.NewDecoder(bytes.NewReader(raw))
	first, err := d.Token()
	if err != nil {
		return err
	}
	if first != json.Delim('{') {
		return errors.New("invalid stop request")
	}
	seen := false
	positionPresent := false
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("invalid stop request")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return err
		}
		if strings.EqualFold(key, "positionMs") {
			positionPresent = true
		}
		if strings.EqualFold(key, "expectedGeneration") {
			if key != "expectedGeneration" || seen || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return errors.New("invalid expected generation")
			}
			seen = true
			var generation int
			if err = json.Unmarshal(value, &generation); err != nil || generation < 1 || int64(generation) > 9007199254740991 {
				return errors.New("invalid expected generation")
			}
			parsed.ExpectedGeneration = &generation
		}
	}
	if _, err = d.Token(); err != nil {
		return err
	}
	if _, err = d.Token(); err != io.EOF {
		return errors.New("invalid stop request")
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if parsed.ExpectedGeneration != nil && positionPresent {
		return errors.New("conditional stop cannot report progress")
	}
	*s = StopRequest(parsed)
	return nil
}

func (StopRequest) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"positionMs":         map[string]any{"type": "integer", "minimum": -9007199254740991, "maximum": 9007199254740991},
		"expectedGeneration": map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991},
	}}
}
