package apikit

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type closedTransportMessage struct{}

func (closedTransportMessage) JSONSchema() map[string]any {
	closed := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	text := map[string]any{"type": "string", "minLength": 1}
	return map[string]any{"oneOf": []any{
		closed(map[string]any{"code": text, "state": text}, "code", "state"),
		closed(map[string]any{"transactionId": text, "ownerStart": closed(map[string]any{"payload": text, "signature": text}, "payload", "signature")}, "transactionId", "ownerStart"),
	}}
}

func TestGeneratedRuntimeRejectsUnknownFieldsOnlyOnExplicitlyClosedObjects(t *testing.T) {
	artifacts, err := Generate([]Definition{{Metadata: Metadata{ID: "closed", Method: "GET", Path: "/v1/closed", Summary: "Closed transport fixture", Access: Public, Requires: ScopeOpen, Lane: Default, Cost: Constant, Status: 200}, Request: reflect.TypeFor[struct{}](), Response: reflect.TypeFor[closedTransportMessage]()}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, raw := range map[string][]byte{"schemas.ts": artifacts.Schemas, "runtime.ts": artifacts.Runtime, "package.json": []byte(`{"type":"module"}`)} {
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := `import assert from 'node:assert/strict';import {decode} from './runtime.ts';
const normal={code:'code',state:'state'},owner={transactionId:'transaction',ownerStart:{payload:'proof',signature:'signature'}};
assert.equal(decode('ClosedResponse',normal).code,'code');assert.equal(decode('ClosedResponse',owner).ownerStart.payload,'proof');
for(const bad of [{...normal,extra:1},{...normal,extra:null},{...normal,extra:undefined},{...owner,extra:1},{...owner,code:'code'},{...owner,ownerStart:null},{...owner,ownerStart:{...owner.ownerStart,unknown:1}},{transactionId:'transaction'},{state:'state'}])assert.throws(()=>decode('ClosedResponse',bad));
// Existing reflected, nonclosed DTOs remain additive and strip unknown presentation fields.
assert.deepEqual(Object.keys(decode('ClosedRequest',{future:1})),[]);`
	if err := os.WriteFile(filepath.Join(dir, "check.mjs"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if raw, err := exec.Command("node", "--experimental-strip-types", filepath.Join(dir, "check.mjs")).CombinedOutput(); err != nil {
		t.Fatalf("generated closed-object decoder: %v %s", err, raw)
	}
}
