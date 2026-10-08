package apikit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/apikit/apierror"
)

type principalKey struct{}

func TestContextualAuthorizerReachesTheHandler(t *testing.T) {
	r := NewContextual(func(ctx context.Context, q *http.Request, m Metadata) (context.Context, error) {
		if q.Header.Get("Authorization") == "" {
			return nil, &apierror.Error{Code: "unauthorized"}
		}
		return context.WithValue(ctx, principalKey{}, "viewer-1"), nil
	}, nil)
	m := Metadata{ID: "who", Method: "GET", Path: "/v1/who", Summary: "Who", Access: Device, Lane: Default, Cost: Constant, Status: 200}
	if err := Register(r, Route[struct{}, testResponse]{Metadata: m, Handler: func(ctx context.Context, _ *http.Request, _ struct{}) (testResponse, error) {
		return testResponse{Name: ctx.Value(principalKey{}).(string), Items: []string{}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/who", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	q := httptest.NewRequest("GET", "/v1/who", nil)
	q.Header.Set("Authorization", "Bearer x")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "viewer-1") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestHandlersSetStatusHeadersAndRevisionETags(t *testing.T) {
	r := New(func(context.Context, *http.Request, Metadata) error { return nil }, nil)
	start := Metadata{ID: "start", Method: "POST", Path: "/v1/things", Summary: "Start", Access: Device, Lane: Default, Cost: Constant, BodyLimit: 128, Status: 201}
	if err := Register(r, Route[testRequest, testResponse]{Metadata: start, Handler: func(ctx context.Context, _ *http.Request, in testRequest) (testResponse, error) {
		if in.Name == "later" {
			SetStatus(ctx, 202)
			SetHeader(ctx, "Retry-After", "1")
		}
		SetStatus(ctx, 500) // ignored: only 2xx
		return testResponse{Name: in.Name, Items: []string{}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	read := Metadata{ID: "read", Method: "GET", Path: "/v1/things/{id}", Summary: "Read", Access: Device, Lane: Default, Cost: Constant, Status: 200}
	if err := Register(r, Route[struct{}, testResponse]{Metadata: read, Handler: func(ctx context.Context, q *http.Request, _ struct{}) (testResponse, error) {
		SetHeader(ctx, "ETag", `"12"`)
		return testResponse{Name: q.PathValue("id"), Items: []string{}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	stop := Metadata{ID: "stop", Method: "DELETE", Path: "/v1/things/{id}", Summary: "Stop", Access: Device, Lane: Default, Cost: Constant, BodyLimit: 128, OptionalBody: true, Status: 204}
	var seen testRequest
	if err := Register(r, Route[testRequest, struct{}]{Metadata: stop, Handler: func(ctx context.Context, _ *http.Request, in testRequest) (struct{}, error) {
		seen = in
		SetHeader(ctx, "Report-Every-Ms", "10000")
		return struct{}{}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	post := func(body string) *httptest.ResponseRecorder {
		q := httptest.NewRequest("POST", "/v1/things", strings.NewReader(body))
		q.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		return w
	}
	if w := post(`{"name":"now"}`); w.Code != 201 {
		t.Fatal(w.Code)
	}
	if w := post(`{"name":"later"}`); w.Code != 202 || w.Header().Get("Retry-After") != "1" {
		t.Fatal(w.Code, w.Header())
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/things/a1", nil))
	if w.Header().Get("ETag") != `"12"` {
		t.Fatal(w.Header())
	}
	q := httptest.NewRequest("GET", "/v1/things/a1", nil)
	q.Header.Set("If-None-Match", `"12"`)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 304 {
		t.Fatal(w.Code)
	}
	// An optional body: absent is the zero request, present is still strict.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/v1/things/a1", nil))
	if w.Code != 204 || w.Header().Get("Report-Every-Ms") != "10000" || seen.Name != "" {
		t.Fatal(w.Code, w.Header())
	}
	q = httptest.NewRequest("DELETE", "/v1/things/a1", strings.NewReader(`{"name":"x"}`))
	q.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 204 || seen.Name != "x" {
		t.Fatal(w.Code, seen)
	}
	q = httptest.NewRequest("DELETE", "/v1/things/a1", strings.NewReader(`{"nope":1}`))
	q.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestRawRoutesKeepAccessAndQueryRules(t *testing.T) {
	r := New(func(_ context.Context, q *http.Request, _ Metadata) error {
		if q.Header.Get("Authorization") == "" {
			return &apierror.Error{Code: "unauthorized"}
		}
		return nil
	}, nil)
	m := Metadata{ID: "events", Method: "GET", Path: "/v1/events", Summary: "Events", Access: Device, Lane: Realtime, Cost: Constant, Query: []string{"after"}, Status: 200}
	if err := RegisterRaw(r, RawRoute[struct{}, testResponse]{Metadata: m, Serve: func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "id: 1\nevent: hello\ndata: {}\n\n")
	}}); err != nil {
		t.Fatal(err)
	}
	if RegisterRaw(r, RawRoute[struct{}, testResponse]{Metadata: Metadata{ID: "x", Method: "GET", Path: "/v1/x", Summary: "X", Access: Device, Lane: Realtime, Cost: Constant, Status: 200}}) == nil {
		t.Fatal("raw route without Serve accepted")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/events", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	q := httptest.NewRequest("GET", "/v1/events?bogus=1", nil)
	q.Header.Set("Authorization", "Bearer x")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	q = httptest.NewRequest("GET", "/v1/events?after=3", nil)
	q.Header.Set("Authorization", "Bearer x")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "event: hello") {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(r.Definitions()) != 1 {
		t.Fatal("raw route not listed for contract generation")
	}
}
