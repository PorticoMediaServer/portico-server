package apikit

import (
	"fmt"
	"net/http"
	"reflect"
)

// RegisterHTTP declares the exact request and response types of an HTTP handler.
// The handler keeps its own access checks, decoding, headers and status selection.
// This is useful for multipart uploads, byte streams, and JSON handlers whose
// response semantics must survive a registration change. Credential scopes are
// still enforced by the registry before the handler is reached.
func RegisterHTTP[Request, Response any](r *Registry, metadata Metadata) error {
	metadata.HTTP = true
	if err := r.define(metadata, reflect.TypeFor[Request](), reflect.TypeFor[Response](), true); err != nil {
		return err
	}
	r.handlers = append(r.handlers, http.NotFound)
	return nil
}

// MountHTTP binds a declared HTTP contract to its existing handler. Undeclared
// or duplicate bindings fail at construction, before the server accepts traffic.
func (r *Registry) MountHTTP(mux *http.ServeMux, pattern string, handler http.Handler) error {
	if handler == nil {
		return fmt.Errorf("missing HTTP handler for %s", pattern)
	}
	for i, d := range r.routes {
		if !d.HTTP || d.Method+" "+d.Path != pattern {
			continue
		}
		if r.bound == nil {
			r.bound = make(map[string]bool)
		}
		if r.bound[d.ID] {
			return fmt.Errorf("HTTP contract already bound: %s", pattern)
		}
		r.bound[d.ID] = true
		m := d.Metadata
		serve := func(w http.ResponseWriter, q *http.Request) {
			if r.scopes != nil {
				if err := r.scopes(q, m.Requires); err != nil {
					r.errors.Write(w, "", err)
					return
				}
			}
			handler.ServeHTTP(w, q)
		}
		r.handlers[i] = serve
		mux.Handle(pattern, http.HandlerFunc(serve))
		return nil
	}
	return fmt.Errorf("HTTP handler has no typed contract: %s", pattern)
}

// MetadataForPattern reads an operation declaration by its registered pattern.
// It also covers HTTP path parameters whose syntax belongs to net/http.
func (r *Registry) MetadataForPattern(pattern string) (Metadata, bool) {
	for _, route := range r.routes {
		if route.Method+" "+route.Path == pattern {
			return route.Metadata, true
		}
	}
	return Metadata{}, false
}
