package apikit

import (
	"bytes"
	_ "embed"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"portico.local/apikit/apierror"
	"reflect"
	"sort"
	"strings"
	"time"
)

//go:embed templates/runtime.ts.txt
var runtimeTemplate []byte

//go:embed templates/index.ts.txt
var indexTemplate []byte

var (
	timeType = reflect.TypeFor[time.Time]()
	rawType  = reflect.TypeFor[json.RawMessage]()
)

// Schema derives the wire shape from the exact Go type a registered handler uses.
// Unsupported encodings fail generation instead of publishing an invented type.
// time.Time is an RFC 3339 string; json.RawMessage is any JSON value, the empty schema.
func Schema(t reflect.Type) (map[string]any, error) { return schema(t, map[reflect.Type]bool{}) }

// SchemaProvider lets a wire type state its own schema (an opaque JSON value is
// {}: any value, generated as TypeScript `unknown`).
type SchemaProvider interface{ JSONSchema() map[string]any }

func schema(t reflect.Type, stack map[reflect.Type]bool) (map[string]any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case timeType:
		return map[string]any{"type": "string", "format": "date-time"}, nil
	case rawType:
		return map[string]any{}, nil
	}
	provider := reflect.TypeFor[SchemaProvider]()
	if t.Implements(provider) {
		return reflect.Zero(t).Interface().(SchemaProvider).JSONSchema(), nil
	}
	if reflect.PointerTo(t).Implements(provider) {
		return reflect.New(t).Interface().(SchemaProvider).JSONSchema(), nil
	}
	if stack[t] {
		return nil, fmt.Errorf("recursive type requires explicit schema: %s", t)
	}
	stack[t] = true
	defer delete(stack, t)
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "minimum": -9007199254740991, "maximum": 9007199254740991}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Interface:
		// Preference registry values have a published per-field type. Keep the
		// enclosing document typed while leaving those values open to that
		// registry, rather than pretending every value is a string.
		return map[string]any{"type": "any"}, nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil, fmt.Errorf("binary fields need an explicit wire type: %s", t)
		}
		item, err := schema(t.Elem(), stack)
		return map[string]any{"type": "array", "items": item}, err
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key must be string: %s", t)
		}
		value, err := schema(t.Elem(), stack)
		return map[string]any{"type": "object", "additionalProperties": value}, err
	case reflect.Struct:
		fields, err := structFields(t, stack, 0)
		if err != nil {
			return nil, err
		}
		props := map[string]any{}
		required := []string{}
		order := []string{}
		byName := map[string][]wireField{}
		for _, f := range fields {
			if byName[f.name] == nil {
				order = append(order, f.name)
			}
			byName[f.name] = append(byName[f.name], f)
		}
		for _, name := range order {
			f, err := dominant(byName[name])
			if err != nil {
				return nil, fmt.Errorf("%w: %s in %s", err, name, t)
			}
			props[name] = f.value
			if f.required {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": props, "required": required}, nil
	}
	return nil, fmt.Errorf("unsupported wire type %s", t)
}

// wireField is one JSON member of a struct, before encoding/json's rules pick
// between same-named members of the struct and its embedded structs.
type wireField struct {
	name     string
	tagged   bool
	depth    int
	value    map[string]any
	required bool
}

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// structFields lists a struct's JSON members. An embedded exported struct
// without a JSON name contributes its own members one level deeper, as
// encoding/json promotes them. Embedding anything else unnamed (a pointer, a
// non-struct, or a type with its own JSON encoding, which would take over the
// outer struct's) still needs an explicit name.
func structFields(t reflect.Type, stack map[reflect.Type]bool, depth int) ([]wireField, error) {
	fields := []wireField{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("json"), ",")
		name := tag[0]
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			embedded := field.Type
			if embedded.Kind() != reflect.Struct || embedded.Implements(jsonMarshaler) || embedded.Implements(textMarshaler) || reflect.PointerTo(embedded).Implements(jsonMarshaler) || reflect.PointerTo(embedded).Implements(textMarshaler) {
				return nil, fmt.Errorf("embedded fields need explicit names: %s", t)
			}
			if stack[embedded] {
				return nil, fmt.Errorf("recursive type requires explicit schema: %s", embedded)
			}
			stack[embedded] = true
			promoted, err := structFields(embedded, stack, depth+1)
			delete(stack, embedded)
			if err != nil {
				return nil, err
			}
			fields = append(fields, promoted...)
			continue
		}
		tagged := name != ""
		if name == "" {
			name = field.Name
		}
		var value map[string]any
		var err error
		if strings.Contains(field.Tag.Get("json"), ",string") {
			value = map[string]any{"type": "string"}
		} else {
			value, err = schema(field.Type, stack)
		}
		if err != nil {
			return nil, err
		}
		required := !strings.Contains(field.Tag.Get("json"), ",omitempty")
		// A pointer that is always sent is null when unset: the member is
		// required and nullable. (An omitempty pointer is simply optional.)
		if required && field.Type.Kind() == reflect.Pointer && len(value) > 0 {
			nullable := map[string]any{"nullable": true}
			for k, v := range value {
				nullable[k] = v
			}
			value = nullable
		}
		fields = append(fields, wireField{name: name, tagged: tagged, depth: depth, value: value, required: required})
	}
	return fields, nil
}

// dominant applies encoding/json's rule to same-named members: the shallowest
// wins, then the only tagged one at that depth. Where encoding/json would drop
// every candidate, generation fails rather than publish a member that is never sent.
func dominant(candidates []wireField) (wireField, error) {
	shallowest := candidates[0].depth
	for _, f := range candidates {
		shallowest = min(shallowest, f.depth)
	}
	var top, tagged []wireField
	for _, f := range candidates {
		if f.depth == shallowest {
			top = append(top, f)
			if f.tagged {
				tagged = append(tagged, f)
			}
		}
	}
	if len(top) == 1 {
		return top[0], nil
	}
	if len(tagged) == 1 {
		return tagged[0], nil
	}
	return wireField{}, errors.New("ambiguous JSON member")
}

type Artifacts struct {
	Runtime    []byte
	Index      []byte
	Errors     []byte
	Limits     []byte
	Decoders   []byte
	OpenAPI    []byte
	Typescript []byte
	Routes     []byte
	Schemas    []byte
}

// Options names a generated contract. The zero value is the server's contract.
type Options struct {
	// Title is the OpenAPI info title; empty means "Portico API".
	Title string
	// Errors is the published error catalogue. Nil publishes apikit's catalogue
	// (the server's); a non-nil slice, even an empty one, publishes exactly those
	// definitions, for a service with its own error envelope.
	Errors []apierror.Definition
}

// Generate emits the server's contract, titled "Portico API".
func Generate(definitions []Definition) (Artifacts, error) {
	return GenerateWith(definitions, Options{})
}

// GenerateWith emits a contract for another Portico service from its route definitions.
func GenerateWith(definitions []Definition, options Options) (Artifacts, error) {
	if options.Title == "" {
		options.Title = "Portico API"
	}
	definitions = append([]Definition{}, definitions...)
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ID < definitions[j].ID })
	paths := map[string]any{}
	schemas := map[string]any{}
	routes := []Metadata{}
	var ts strings.Builder
	var decoders strings.Builder
	decoders.WriteString("// Generated by apigen. Do not edit.\nimport { decode } from './runtime.ts';\nimport type * as types from './types.ts';\n")
	ts.WriteString("// Generated by apigen. Do not edit.\n")
	anyJSON := false
	nullable := false
	for _, d := range definitions {
		request, err := Schema(d.Request)
		if err != nil {
			return Artifacts{}, err
		}
		response, err := Schema(d.Response)
		if err != nil {
			return Artifacts{}, err
		}
		anyJSON = anyJSON || usesAny(request) || usesAny(response)
		nullable = nullable || usesNullable(request) || usesNullable(response)
		name := exportName(d.ID)
		fmt.Fprintf(&decoders, "export function decode%sResponse(input: unknown): types.%sResponse { return decode(%q, input) as types.%sResponse; }\n", name, name, name+"Response", name)
		schemas[name+"Request"] = request
		schemas[name+"Response"] = response
		ts.WriteString("export type " + name + "Request = " + tsType(request) + ";\n")
		ts.WriteString("export type " + name + "Response = " + tsType(response) + ";\n")
		operation := map[string]any{"operationId": d.ID, "summary": d.Summary, "x-access": d.Access, "x-lane": d.Lane, "x-cost": d.Cost, "responses": map[string]any{fmt.Sprint(d.Status): map[string]any{"description": "Success", "content": map[string]any{"application/json": map[string]any{"schema": response}}}}}
		if d.SSE {
			field, ok := d.Response.FieldByName("Events")
			if !ok || field.Type.Kind() != reflect.Slice {
				return Artifacts{}, fmt.Errorf("SSE response %s must expose Events", d.ID)
			}
			event, err := Schema(field.Type.Elem())
			if err != nil {
				return Artifacts{}, err
			}
			operation["responses"].(map[string]any)[fmt.Sprint(d.Status)].(map[string]any)["content"].(map[string]any)["text/event-stream"] = map[string]any{"schema": map[string]any{"type": "string"}, "x-event-schema": event}
		}
		if d.BodyLimit > 0 {
			operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": request}}}
			operation["x-max-body-bytes"] = d.BodyLimit
		}
		if paths[d.Path] == nil {
			paths[d.Path] = map[string]any{}
		}
		paths[d.Path].(map[string]any)[strings.ToLower(d.Method)] = operation
		routes = append(routes, d.Metadata)
	}
	spec := map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": options.Title, "version": "1"}, "paths": paths}
	openapi, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return Artifacts{}, err
	}
	rawSchemas, _ := json.MarshalIndent(schemas, "", "  ")
	rawRoutes, _ := json.MarshalIndent(routes, "", "  ")
	definitionsErrors := options.Errors
	if definitionsErrors == nil {
		catalogue, _ := apierror.New()
		definitionsErrors = catalogue.Definitions()
	} else {
		definitionsErrors = append([]apierror.Definition{}, definitionsErrors...)
	}
	sort.Slice(definitionsErrors, func(i, j int) bool { return definitionsErrors[i].Code < definitionsErrors[j].Code })
	rawErrors, _ := json.MarshalIndent(definitionsErrors, "", "  ")
	runtime := append([]byte{}, runtimeTemplate...)
	if anyJSON {
		if runtime, err = anyRuntime(runtime); err != nil {
			return Artifacts{}, err
		}
	}
	if nullable {
		if runtime, err = nullableRuntime(runtime); err != nil {
			return Artifacts{}, err
		}
	}
	return Artifacts{Runtime: runtime, Index: append([]byte{}, indexTemplate...), OpenAPI: append(openapi, '\n'), Typescript: []byte(ts.String()), Routes: []byte("// Generated by apigen. Do not edit.\nexport const routes = " + string(rawRoutes) + " as const;\n"), Schemas: []byte("// Generated by apigen. Do not edit.\nexport const schemas = " + string(rawSchemas) + " as const;\n"), Errors: []byte("// Generated by apigen. Do not edit.\nexport const errors = " + string(rawErrors) + " as const;\n"), Limits: []byte("// Generated by apigen. Do not edit.\nexport const limits = { responseBytes: 524288, cursorChars: 4096, itemIds: 500 } as const;\n"), Decoders: []byte(decoders.String())}, nil
}
func exportName(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		if r == '_' || r == '-' || r == '.' {
			upper = true
			continue
		}
		if upper {
			b.WriteString(strings.ToUpper(string(r)))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// usesAny reports whether a schema contains the empty (any JSON value) schema.
func usesAny(s map[string]any) bool {
	if len(s) == 0 {
		return true
	}
	if items, ok := s["items"].(map[string]any); ok && usesAny(items) {
		return true
	}
	if additional, ok := s["additionalProperties"].(map[string]any); ok && usesAny(additional) {
		return true
	}
	if props, ok := s["properties"].(map[string]any); ok {
		for _, p := range props {
			if usesAny(p.(map[string]any)) {
				return true
			}
		}
	}
	return false
}

// usesNullable reports whether a schema has a nullable member.
func usesNullable(s map[string]any) bool {
	if s["nullable"] == true {
		return true
	}
	if items, ok := s["items"].(map[string]any); ok && usesNullable(items) {
		return true
	}
	if additional, ok := s["additionalProperties"].(map[string]any); ok && usesNullable(additional) {
		return true
	}
	if props, ok := s["properties"].(map[string]any); ok {
		for _, p := range props {
			if usesNullable(p.(map[string]any)) {
				return true
			}
		}
	}
	return false
}

// nullableRuntime lets the decoder accept null for nullable members. Like
// anyRuntime it applies only when a contract has one, so other contracts keep
// the original runtime byte for byte.
func nullableRuntime(runtime []byte) ([]byte, error) {
	for _, edit := range [][2]string{
		{"minimum?: number; maximum?: number}", "minimum?: number; maximum?: number; nullable?: boolean}"},
		{"  const reject = (): never => { throw new Error(`Invalid contract value at ${path}`); };\n", "  const reject = (): never => { throw new Error(`Invalid contract value at ${path}`); };\n  if (input === null && schema.nullable) return null;\n"},
	} {
		if bytes.Count(runtime, []byte(edit[0])) != 1 {
			return nil, errors.New("runtime template changed; update nullableRuntime")
		}
		runtime = bytes.Replace(runtime, []byte(edit[0]), []byte(edit[1]), 1)
	}
	return runtime, nil
}

// anyRuntime lets the decoder pass through values of the empty schema. It applies
// only when a contract uses one, so contracts without free-form JSON keep the
// original runtime byte for byte.
func anyRuntime(runtime []byte) ([]byte, error) {
	for _, edit := range [][2]string{
		{"type Schema = {type: string;", "type Schema = {type?: string;"},
		{"  switch (schema.type) {\n", "  // The empty schema is any JSON value.\n  if (schema.type === undefined) return input;\n  switch (schema.type) {\n"},
	} {
		if bytes.Count(runtime, []byte(edit[0])) != 1 {
			return nil, errors.New("runtime template changed; update anyRuntime")
		}
		runtime = bytes.Replace(runtime, []byte(edit[0]), []byte(edit[1]), 1)
	}
	return runtime, nil
}
func tsType(s map[string]any) string {
	if s["nullable"] == true {
		plain := map[string]any{}
		for k, v := range s {
			if k != "nullable" {
				plain[k] = v
			}
		}
		return tsType(plain) + " | null"
	}
	switch s["type"] {
	case nil:
		return "unknown"
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "any":
		return "unknown"
	case "array":
		return "Array<" + tsType(s["items"].(map[string]any)) + ">"
	case "object":
		if additional, ok := s["additionalProperties"].(map[string]any); ok {
			return "Record<string, " + tsType(additional) + ">"
		}
		props := s["properties"].(map[string]any)
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		required := map[string]bool{}
		for _, k := range s["required"].([]string) {
			required[k] = true
		}
		fields := []string{}
		for _, key := range keys {
			suffix := "?"
			if required[key] {
				suffix = ""
			}
			fields = append(fields, fmt.Sprintf("%q%s: %s", key, suffix, tsType(props[key].(map[string]any))))
		}
		return "{ " + strings.Join(fields, "; ") + " }"
	}
	panic("unsupported schema")
}
