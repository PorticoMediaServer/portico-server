// Package apikit makes authorization and wire contracts mandatory when registering routes.
package apikit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"portico.local/apikit/apierror"
)

type Access string

const (
	Public        Access = "public"
	Device        Access = "device"
	ViewerLibrary Access = "viewer_library"
	ViewerItem    Access = "viewer_item_restrictions"
	Owner         Access = "owner"
	Admin         Access = "admin_scope"
)

type Lane string

const (
	Default  Lane = "default"
	Browsing Lane = "browsing"
	Realtime Lane = "realtime"
	Security Lane = "security"
	Media    Lane = "media"
)

type Cost string

const (
	Constant       Cost = "constant"
	PageSized      Cost = "page_sized"
	SelectionAsync Cost = "selection_sized_async"
)

type Metadata struct {
	ID        string   `json:"id"`
	Method    string   `json:"method"`
	Path      string   `json:"path"`
	Summary   string   `json:"summary"`
	Access    Access   `json:"access"`
	Scope     string   `json:"scope,omitempty"`
	Lane      Lane     `json:"lane"`
	Cost      Cost     `json:"cost"`
	BodyLimit int64    `json:"bodyLimit"`
	Query     []string `json:"query,omitempty"`
	Errors    []string `json:"errors"`
	Status    int      `json:"status"`
	// OptionalBody lets a route with a body contract also accept no body at all
	// (a DELETE whose body is optional); the handler then sees the zero Request.
	OptionalBody bool `json:"optionalBody,omitempty"`
	SSE          bool `json:"sse,omitempty"`
}
type Route[Request, Response any] struct {
	Metadata
	Handler func(context.Context, *http.Request, Request) (Response, error)
	// RawHandler is for a GET with both a typed JSON long-poll response and
	// text/event-stream frames. Access and query validation still run first.
	RawHandler http.HandlerFunc
}
type Definition struct {
	Metadata
	Request  reflect.Type
	Response reflect.Type
}
type Authorize func(context.Context, *http.Request, Metadata) error

// AuthorizeContext is an Authorize that also returns the context the handler
// runs in, so a route can reuse the principal it was admitted with.
type AuthorizeContext func(context.Context, *http.Request, Metadata) (context.Context, error)
type Registry struct {
	handlers     []http.HandlerFunc
	errors       *apierror.Registry
	authorize    Authorize
	authorizeCtx AuthorizeContext
	routes       []Definition
	frozen       bool
}

// RawRoute is for the few routes that write their own response (event
// streams): access, lane, cost and query rules still apply, and Response
// documents the JSON form the route also serves.
type RawRoute[Request, Response any] struct {
	Metadata
	Serve func(http.ResponseWriter, *http.Request)
}

type replyKey struct{}
type reply struct {
	status int
	header http.Header
}

// SetStatus overrides the route's declared success status for this request (a
// 202 while a resource is being prepared). Only 2xx values are accepted.
func SetStatus(ctx context.Context, status int) {
	if r, ok := ctx.Value(replyKey{}).(*reply); ok && status >= 200 && status < 300 {
		r.status = status
	}
}

// SetHeader adds a response header on success (Retry-After, a revision ETag,
// Report-Every-Ms). An ETag set here replaces the body-digest ETag of a GET.
func SetHeader(ctx context.Context, key, value string) {
	if r, ok := ctx.Value(replyKey{}).(*reply); ok && !strings.ContainsAny(key+value, "\r\n") {
		r.header.Set(key, value)
	}
}

func New(authorize Authorize, catalogue *apierror.Registry) *Registry {
	if catalogue == nil {
		catalogue, _ = apierror.New()
	}
	return &Registry{authorize: authorize, errors: catalogue}
}

// NewContextual is New with an authorizer that passes a context to the handler.
func NewContextual(authorize AuthorizeContext, catalogue *apierror.Registry) *Registry {
	r := New(nil, catalogue)
	if authorize != nil {
		r.authorizeCtx = authorize
		r.authorize = func(ctx context.Context, q *http.Request, m Metadata) error {
			_, err := authorize(ctx, q, m)
			return err
		}
	}
	return r
}

// Errors is the catalogue routes on this registry write errors from.
func (r *Registry) Errors() *apierror.Registry { return r.errors }

// admit applies access and the query allow-list. It returns the request the
// handler must use, or false after writing the error.
func (r *Registry) admit(w http.ResponseWriter, q *http.Request, m Metadata) (*http.Request, bool) {
	fail := func(err error) { r.errors.Write(w, "", err) }
	// Access precedes body parsing and conditional responses.
	if m.Access != Public {
		if r.authorizeCtx != nil {
			ctx, err := r.authorizeCtx(q.Context(), q, m)
			if err != nil {
				fail(err)
				return nil, false
			}
			if ctx != nil {
				q = q.WithContext(ctx)
			}
		} else if err := r.authorize(q.Context(), q, m); err != nil {
			fail(err)
			return nil, false
		}
	}
	allowed := map[string]bool{}
	for _, key := range m.Query {
		allowed[key] = true
	}
	var fields []apierror.Field
	for key := range q.URL.Query() {
		if !allowed[key] {
			fields = append(fields, apierror.Field{Path: key, Code: "unknown_field", Message: "This field is not recognized."})
		}
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	if len(fields) > 0 {
		fail(&apierror.Error{Code: "invalid_request", Fields: fields})
		return nil, false
	}
	return q, true
}

func (r *Registry) define(m Metadata, request, response reflect.Type, access bool) error {
	if r.frozen {
		return errors.New("registry frozen")
	}
	if err := validate(m); err != nil {
		return err
	}
	if !access || m.Access != Public && r.authorize == nil {
		return errors.New("handler and access enforcement required")
	}
	for _, code := range m.Errors {
		if _, ok := r.errors.Definition(code); !ok {
			return errors.New("unregistered route error")
		}
	}
	for _, d := range r.routes {
		if d.ID == m.ID || d.Method == m.Method && d.Path == m.Path {
			return errors.New("duplicate route")
		}
	}
	r.routes = append(r.routes, Definition{m, request, response})
	return nil
}

// RegisterRaw registers a route that writes its own response after admission.
func RegisterRaw[Request, Response any](r *Registry, route RawRoute[Request, Response]) error {
	m := route.Metadata
	if err := r.define(m, reflect.TypeFor[Request](), reflect.TypeFor[Response](), route.Serve != nil); err != nil {
		return err
	}
	r.handlers = append(r.handlers, func(w http.ResponseWriter, q *http.Request) {
		if q, ok := r.admit(w, q, m); ok {
			route.Serve(w, q)
		}
	})
	return nil
}
func (r *Registry) Definitions() []Definition               { return append([]Definition{}, r.routes...) }
func (r *Registry) ErrorDefinitions() []apierror.Definition { return r.errors.Definitions() }
func (r *Registry) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	best, score := -1, -1
	var values map[string]string
	for i, route := range r.routes {
		if route.Method != q.Method {
			continue
		}
		matched, rank, ok := matchRoute(route.Path, q.URL.EscapedPath())
		if ok && rank > score {
			best, score, values = i, rank, matched
		}
	}
	if best < 0 {
		r.errors.Write(w, "", &apierror.Error{Code: "not_found"})
		return
	}
	for key, value := range values {
		q.SetPathValue(key, value)
	}
	q.Pattern = r.routes[best].Method + " " + r.routes[best].Path
	r.handlers[best](w, q)
}
func matchRoute(pattern, path string) (map[string]string, int, bool) {
	want, got := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(want) != len(got) {
		return nil, 0, false
	}
	values := map[string]string{}
	score := 0
	for i, part := range want {
		actual, err := url.PathUnescape(got[i])
		if err != nil || strings.Contains(actual, "/") {
			return nil, 0, false
		}
		if strings.HasPrefix(part, "{") {
			end := strings.IndexByte(part, '}')
			if end < 2 {
				return nil, 0, false
			}
			suffix := part[end+1:]
			if !strings.HasSuffix(actual, suffix) {
				return nil, 0, false
			}
			id := strings.TrimSuffix(actual, suffix)
			if id == "" || len(id) > 128 {
				return nil, 0, false
			}
			for _, c := range id {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
					return nil, 0, false
				}
			}
			values[part[1:end]] = id
			score += len(suffix)
		} else {
			if part != actual {
				return nil, 0, false
			}
			score += len(part) + 100
		}
	}
	return values, score, true
}
func (r *Registry) Freeze() { r.frozen = true }
func validate(m Metadata) error {
	if m.ID == "" || m.Summary == "" || !strings.HasPrefix(m.Path, "/v1/") || m.Status < 200 || m.Status >= 300 || m.BodyLimit < 0 || m.BodyLimit > 48<<20 {
		return errors.New("route metadata incomplete")
	}
	switch m.Access {
	case Public, Device, ViewerLibrary, ViewerItem, Owner:
	case Admin:
		if m.Scope == "" {
			return errors.New("admin scope required")
		}
	default:
		return errors.New("access rule required")
	}
	switch m.Lane {
	case Default, Browsing, Realtime, Security, Media:
	default:
		return errors.New("lane required")
	}
	switch m.Cost {
	case Constant, PageSized, SelectionAsync:
	default:
		return errors.New("cost class required")
	}
	switch m.Method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE":
	default:
		return errors.New("invalid method")
	}
	return nil
}
func Register[Request, Response any](r *Registry, route Route[Request, Response]) error {
	m := route.Metadata
	// Integration merge: lane C's SSE raw handler (886f5011) on BE-playback's
	// define/admit registry; exactly one of Handler and RawHandler.
	if (route.Handler == nil) == (route.RawHandler == nil) {
		return errors.New("handler and access enforcement required")
	}
	if route.RawHandler != nil && (!m.SSE || m.Method != "GET" || m.BodyLimit != 0) || route.RawHandler == nil && m.SSE {
		return errors.New("streaming route must be a bodyless GET with a raw handler")
	}
	if err := r.define(m, reflect.TypeFor[Request](), reflect.TypeFor[Response](), true); err != nil {
		return err
	}
	r.handlers = append(r.handlers, func(w http.ResponseWriter, q *http.Request) {
		fail := func(err error) { r.errors.Write(w, "", err) }
		q, ok := r.admit(w, q, m)
		if !ok {
			return
		}
		if route.RawHandler != nil {
			if q.ContentLength != 0 || len(q.TransferEncoding) != 0 {
				fail(&apierror.Error{Code: "invalid_request"})
				return
			}
			route.RawHandler(w, q)
			return
		}
		var request Request
		empty := q.ContentLength == 0 && len(q.TransferEncoding) == 0
		if m.BodyLimit > 0 && !(m.OptionalBody && empty && q.Header.Get("Content-Type") == "") {
			typ, _, err := mime.ParseMediaType(q.Header.Get("Content-Type"))
			if err != nil || (typ != "application/json" && !(m.Method == "PATCH" && typ == "application/merge-patch+json")) {
				fail(&apierror.Error{Code: "unsupported_media_type"})
				return
			}
			raw, err := io.ReadAll(http.MaxBytesReader(w, q.Body, m.BodyLimit))
			if err != nil {
				fail(&apierror.Error{Code: "body_too_large"})
				return
			}
			if err = decodeStrict(raw, &request); err != nil {
				fail(&apierror.Error{Code: "invalid_request", Cause: err})
				return
			}
		} else if m.BodyLimit == 0 && !empty {
			fail(&apierror.Error{Code: "invalid_request"})
			return
		}
		out := &reply{header: http.Header{}}
		q = q.WithContext(context.WithValue(q.Context(), replyKey{}, out))
		response, err := route.Handler(q.Context(), q, request)
		if err != nil {
			fail(err)
			return
		}
		status := m.Status
		if out.status != 0 {
			status = out.status
		}
		for key, values := range out.header {
			w.Header()[key] = values
		}
		if status == 204 {
			w.WriteHeader(204)
			return
		}
		raw, err := marshalResponse(response)
		if err != nil {
			fail(err)
			return
		}
		if m.Method == "GET" {
			tag := w.Header().Get("ETag")
			if tag == "" {
				sum := sha256.Sum256(raw)
				tag = "\"" + hex.EncodeToString(sum[:]) + "\""
				w.Header().Set("ETag", tag)
			}
			w.Header().Set("Cache-Control", "private, no-cache")
			for _, candidate := range strings.Split(q.Header.Get("If-None-Match"), ",") {
				if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == tag || strings.TrimSpace(candidate) == "*" {
					w.WriteHeader(304)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(raw)
	})
	return nil
}
func marshalResponse(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Enforce the no-null response contract instead of silently inventing values.
	if err = nullFree(reflect.ValueOf(v), "response"); err != nil {
		return nil, err
	}
	if len(raw) > 512<<10 {
		return nil, errors.New("response exceeds page budget")
	}
	return raw, nil
}
func decodeStrict(raw []byte, out any) error {
	// A token walk rejects duplicate keys, which encoding/json would otherwise overwrite.
	tokens := json.NewDecoder(bytes.NewReader(raw))
	tokens.UseNumber()
	if err := uniqueValue(tokens); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	var document any
	values := json.NewDecoder(bytes.NewReader(raw))
	values.UseNumber()
	if err := values.Decode(&document); err != nil {
		return err
	}
	if err := exactFieldNames(document, reflect.TypeOf(out)); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

// encoding/json accepts case-folded struct fields even with DisallowUnknownFields.
// Strict request contracts require the exact published JSON names, recursively.
func exactFieldNames(value any, kind reflect.Type) error {
	if kind == nil {
		return nil
	}
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	if reflect.PointerTo(kind).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return nil
	}
	switch kind.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return nil // The typed decoder reports the value-kind error.
		}
		fields := make(map[string]reflect.Type, kind.NumField())
		var collect func(reflect.Type, int)
		collect = func(current reflect.Type, depth int) {
			for current.Kind() == reflect.Pointer {
				current = current.Elem()
			}
			if current.Kind() != reflect.Struct || depth > 16 {
				return
			}
			for i := range current.NumField() {
				field := current.Field(i)
				if !field.IsExported() {
					continue
				}
				name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if name == "-" {
					continue
				}
				if field.Anonymous && name == "" {
					collect(field.Type, depth+1)
					continue
				}
				if name == "" {
					name = field.Name
				}
				fields[name] = field.Type
			}
		}
		collect(kind, 0)
		for name, child := range object {
			field, ok := fields[name]
			if !ok {
				return errors.New("unknown or case-mismatched key")
			}
			if err := exactFieldNames(child, field); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if array, ok := value.([]any); ok {
			for _, child := range array {
				if err := exactFieldNames(child, kind.Elem()); err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		if object, ok := value.(map[string]any); ok {
			for _, child := range object {
				if err := exactFieldNames(child, kind.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func uniqueValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || keys[s] {
				return errors.New("duplicate or invalid key")
			}
			keys[s] = true
			if err = uniqueValue(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueValue(d); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}

// CanonicalJSON rejects duplicate keys and trailing values before normalizing key order.
func CanonicalJSON(raw []byte) ([]byte, error) {
	var value any
	if err := decodeStrict(raw, &value); err != nil {
		return nil, err
	}
	// UseNumber preserves integers above float64's exact range for validation by
	// the typed handler; normalization must not collapse distinct large inputs.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// nullFree finds an accidental null in a response: a nil slice, map or
// interface, which encode as null where the contract promises a list, an object
// or a value. A nil pointer is the one intended null: the contract generator
// declares an always-sent pointer "required and nullable" (an unset ETA, an
// unknown count). Fields follow encoding/json: "-" is skipped, an omitempty
// field that is empty is not sent, embedded structs are flattened. A value with
// its own MarshalJSON is checked by scanning what it produces.
func nullFree(v reflect.Value, path string) error {
	if !v.IsValid() {
		return fmt.Errorf("response contains null at %s", path)
	}
	if v.Kind() != reflect.Pointer && v.Kind() != reflect.Interface && v.Type().Implements(marshalerType) {
		return marshaledNullFree(v.Interface(), path)
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		if v.Type().Implements(marshalerType) {
			return marshaledNullFree(v.Interface(), path)
		}
		return nullFree(v.Elem(), path)
	case reflect.Interface:
		if v.IsNil() {
			return fmt.Errorf("response contains null at %s", path)
		}
		return nullFree(v.Elem(), path)
	case reflect.Slice:
		if v.IsNil() {
			return fmt.Errorf("response contains null at %s", path)
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		fallthrough
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := nullFree(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if v.IsNil() {
			return fmt.Errorf("response contains null at %s", path)
		}
		entries := v.MapRange()
		for entries.Next() {
			if err := nullFree(entries.Value(), fmt.Sprintf("%s.%v", path, entries.Key())); err != nil {
				return err
			}
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() && !field.Anonymous {
				continue
			}
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, options, _ := strings.Cut(tag, ",")
			value := v.Field(i)
			if field.Anonymous && name == "" {
				if value.Kind() == reflect.Pointer {
					if value.IsNil() {
						continue
					}
					value = value.Elem()
				}
				if value.Kind() == reflect.Struct {
					if err := nullFree(value, path); err != nil {
						return err
					}
					continue
				}
			}
			if !field.IsExported() {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if strings.Contains(","+options+",", ",omitempty,") && emptyJSON(value) {
				continue
			}
			if err := nullFree(value, path+"."+name); err != nil {
				return err
			}
		}
	}
	return nil
}

var marshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

func marshaledNullFree(v any, path string) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if token == nil {
			return fmt.Errorf("response contains null at %s", path)
		}
	}
}

// emptyJSON is encoding/json's omitempty test.
func emptyJSON(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}
