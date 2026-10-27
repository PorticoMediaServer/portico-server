package playbackv1

import (
	"encoding/json"
	"testing"
)

func TestStopRequestGenerationPresenceNeverFallsBack(t *testing.T) {
	for _, raw := range []string{`{"ExpectedGeneration":0}`, `{"expectedGeneration":1,"ExpectedGeneration":2}`, `{"expectedGeneration":null}`, `{"expectedGeneration":0}`, `{"expectedGeneration":-1}`, `{"expectedGeneration":1.5}`, `{"expectedGeneration":"1"}`, `{"expectedGeneration":9007199254740992}`, `{"expectedGeneration":999999999999999999999999}`, `{"expectedGeneration":1,"expectedGeneration":2}`, `{"expectedGeneration":1,"positionMs":5}`, `{"expectedGeneration":1,"positionMs":null}`} {
		var in StopRequest
		if err := json.Unmarshal([]byte(raw), &in); err == nil {
			t.Fatalf("accepted invalid conditional stop %s", raw)
		}
	}
	var ordinary StopRequest
	if err := json.Unmarshal([]byte(`{"positionMs":5}`), &ordinary); err != nil || ordinary.ExpectedGeneration != nil || ordinary.PositionMs == nil {
		t.Fatalf("ordinary stop: %+v %v", ordinary, err)
	}
	var guarded StopRequest
	if err := json.Unmarshal([]byte(`{"expectedGeneration":2}`), &guarded); err != nil || guarded.ExpectedGeneration == nil || *guarded.ExpectedGeneration != 2 || guarded.PositionMs != nil {
		t.Fatalf("conditional stop: %+v %v", guarded, err)
	}
}
