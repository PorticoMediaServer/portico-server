// Package apispec checks real server answers against the published OpenAPI
// documents in server/api (contract drift, CD-*). The documents are
// what the clients are built against; a handler answer that does not match its
// response schema is drift, whichever side moved. Tests use it; nothing in the
// server's runtime imports it.
package apispec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Document is one parsed OpenAPI file.
type Document struct {
	File string
	root map[string]any
}

var (
	loadOnce sync.Once
	loaded   []Document
	loadErr  error
)

// Documents returns every server/api/*.openapi.yaml plus openapi.yaml.
func Documents() ([]Document, error) {
	loadOnce.Do(func() {
		_, here, _, _ := runtime.Caller(0)
		dir := filepath.Join(filepath.Dir(here), "..", "..", "api")
		files, _ := filepath.Glob(filepath.Join(dir, "*.openapi.yaml"))
		files = append(files, filepath.Join(dir, "openapi.yaml"))
		sort.Strings(files)
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				loadErr = err
				return
			}
			var doc any
			if err = yaml.Unmarshal(raw, &doc); err != nil {
				loadErr = fmt.Errorf("%s: %w", filepath.Base(file), err)
				return
			}
			root, _ := normalize(doc).(map[string]any)
			loaded = append(loaded, Document{File: filepath.Base(file), root: root})
		}
	})
	return loaded, loadErr
}

// normalize turns YAML's map[any]any (from integer keys such as status codes)
// into map[string]any all the way down.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = normalize(e)
		}
		return t
	case map[any]any:
		out := map[string]any{}
		for k, e := range t {
			out[fmt.Sprint(k)] = normalize(e)
		}
		return out
	case []any:
		for i, e := range t {
			t[i] = normalize(e)
		}
		return t
	}
	return v
}

// Response finds the JSON response schema for method, path template and
// status in whichever document describes it.
func Response(method, path string, status int) (Document, map[string]any, error) {
	docs, err := Documents()
	if err != nil {
		return Document{}, nil, err
	}
	for _, doc := range docs {
		paths, _ := doc.root["paths"].(map[string]any)
		item, _ := paths[path].(map[string]any)
		op, _ := item[strings.ToLower(method)].(map[string]any)
		if op == nil {
			continue
		}
		responses, _ := op["responses"].(map[string]any)
		response, _ := responses[fmt.Sprint(status)].(map[string]any)
		if response == nil {
			// OpenAPI: `default` describes every status not listed.
			response, _ = responses["default"].(map[string]any)
		}
		if response == nil {
			return doc, nil, fmt.Errorf("%s %s: no %d response in %s", method, path, status, doc.File)
		}
		response = doc.resolve(response)
		content, _ := response["content"].(map[string]any)
		media, _ := content["application/json"].(map[string]any)
		schema, _ := media["schema"].(map[string]any)
		if schema == nil {
			return doc, nil, fmt.Errorf("%s %s %d: no JSON schema in %s", method, path, status, doc.File)
		}
		return doc, schema, nil
	}
	return Document{}, nil, fmt.Errorf("%s %s: not in any API document", method, path)
}

// Schema returns a named components schema.
func Schema(name string) (Document, map[string]any, error) {
	docs, err := Documents()
	if err != nil {
		return Document{}, nil, err
	}
	for _, doc := range docs {
		components, _ := doc.root["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		if s, ok := schemas[name].(map[string]any); ok {
			return doc, s, nil
		}
	}
	return Document{}, nil, fmt.Errorf("schema %s: not in any API document", name)
}

func (d Document) resolve(v map[string]any) map[string]any {
	for i := 0; i < 16; i++ {
		ref, _ := v["$ref"].(string)
		if !strings.HasPrefix(ref, "#/") {
			return v
		}
		var cur any = d.root
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, _ := cur.(map[string]any)
			cur = m[part]
		}
		next, _ := cur.(map[string]any)
		if next == nil {
			return map[string]any{"x-unresolved": ref}
		}
		v = next
	}
	return v
}

// Validate checks a decoded JSON value against a schema and lists every
// mismatch (empty when it conforms). Supported: $ref, allOf, oneOf/anyOf,
// type (single or list), required, properties, items, maxItems, minLength/maxLength, enum, const.
func (d Document) Validate(schema map[string]any, value any) []string {
	var out []string
	d.validate(schema, value, "$", &out)
	return out
}

// ValidateJSON decodes body and validates it.
func (d Document) ValidateJSON(schema map[string]any, body []byte) []string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return []string{"body is not JSON: " + err.Error()}
	}
	return d.Validate(schema, v)
}

func (d Document) validate(schema map[string]any, v any, at string, out *[]string) {
	schema = d.resolve(schema)
	if ref, ok := schema["x-unresolved"].(string); ok {
		*out = append(*out, at+": unresolved "+ref)
		return
	}
	for _, key := range []string{"allOf"} {
		if list, ok := schema[key].([]any); ok {
			for _, s := range list {
				if m, ok := s.(map[string]any); ok {
					d.validate(m, v, at, out)
				}
			}
		}
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if list, ok := schema[key].([]any); ok {
			matched := false
			for _, s := range list {
				m, _ := s.(map[string]any)
				var sub []string
				d.validate(m, v, at, &sub)
				if len(sub) == 0 {
					matched = true
					break
				}
			}
			if !matched {
				*out = append(*out, at+": matches none of "+key)
			}
		}
	}
	if types := schemaTypes(schema); len(types) > 0 && !typeMatches(types, v) {
		*out = append(*out, fmt.Sprintf("%s: %s is not %s", at, kind(v), strings.Join(types, "|")))
		return
	}
	if c, ok := schema["const"]; ok && fmt.Sprint(c) != fmt.Sprint(v) {
		*out = append(*out, fmt.Sprintf("%s: %v is not %v", at, v, c))
	}
	if enum, ok := schema["enum"].([]any); ok && v != nil {
		found := false
		for _, e := range enum {
			if fmt.Sprint(e) == fmt.Sprint(v) {
				found = true
			}
		}
		if !found {
			*out = append(*out, fmt.Sprintf("%s: %v is not one of %v", at, v, enum))
		}
	}
	switch t := v.(type) {
	case string:
		n := utf8.RuneCountInString(t)
		if min, ok := schema["minLength"].(int); ok && n < min {
			*out = append(*out, fmt.Sprintf("%s: %d code points is below %d", at, n, min))
		}
		if max, ok := schema["maxLength"].(int); ok && n > max {
			*out = append(*out, fmt.Sprintf("%s: %d code points exceeds %d", at, n, max))
		}
	case map[string]any:
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, present := t[fmt.Sprint(r)]; !present {
					*out = append(*out, at+": missing required "+fmt.Sprint(r))
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for k, e := range t {
			if p, ok := props[k].(map[string]any); ok {
				d.validate(p, e, at+"."+k, out)
			}
		}
	case []any:
		if max, ok := schema["maxItems"].(int); ok && len(t) > max {
			*out = append(*out, fmt.Sprintf("%s: %d items exceeds %d", at, len(t), max))
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, e := range t {
				d.validate(items, e, fmt.Sprintf("%s[%d]", at, i), out)
			}
		}
	}
}

func schemaTypes(schema map[string]any) []string {
	switch t := schema["type"].(type) {
	case string:
		out := []string{t}
		if n, _ := schema["nullable"].(bool); n {
			out = append(out, "null")
		}
		return out
	case []any:
		out := []string{}
		for _, e := range t {
			out = append(out, fmt.Sprint(e))
		}
		return out
	}
	return nil
}

func kind(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if t == float64(int64(t)) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

func typeMatches(types []string, v any) bool {
	k := kind(v)
	for _, t := range types {
		if t == k || t == "number" && k == "integer" {
			return true
		}
	}
	return false
}
