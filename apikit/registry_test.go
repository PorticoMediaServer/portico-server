package apikit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/apikit/apierror"
)

type testRequest struct {
	Name string `json:"name"`
}
type testResponse struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func testMetadata() Metadata {
	return Metadata{ID: "test", Method: "POST", Path: "/v1/test", Summary: "Test route", Access: Device, Lane: Default, Cost: Constant, BodyLimit: 128, Status: 200}
}
func TestRegistryRequiresAccessAndEnforcesBeforeParsing(t *testing.T) {
	r := New(nil, nil)
	m := testMetadata()
	m.Access = ""
	route := Route[testRequest, testResponse]{Metadata: m, Handler: func(context.Context, *http.Request, testRequest) (testResponse, error) {
		t.Fatal("unapproved handler ran")
		return testResponse{}, nil
	}}
	if Register(r, route) == nil {
		t.Fatal("missing access accepted")
	}
	route.Access = Device
	if Register(r, route) == nil {
		t.Fatal("missing authorizer accepted")
	}
	r = New(func(context.Context, *http.Request, Metadata) error { return &apierror.Error{Code: "not_permitted"} }, nil)
	if err := Register(r, route); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/v1/test", strings.NewReader("invalid")))
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestStreamingRouteKeepsRegistryAccessAndQueryRules(t *testing.T) {
	authorized := false
	served := false
	r := New(func(context.Context, *http.Request, Metadata) error {
		if !authorized {
			return &apierror.Error{Code: "unauthorized"}
		}
		return nil
	}, nil)
	m := Metadata{ID: "events", Method: "GET", Path: "/v1/events", Summary: "Stream events", Access: Device, Lane: Realtime, Cost: Constant, Status: 200, SSE: true, Query: []string{"after"}}
	if err := Register(r, Route[struct{}, testResponse]{Metadata: m, RawHandler: func(w http.ResponseWriter, _ *http.Request) {
		served = true
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
	}}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/events?unknown=1", nil))
	if w.Code != 401 || served {
		t.Fatalf("stream bypassed access: %d %v", w.Code, served)
	}
	authorized = true
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/events?unknown=1", nil))
	if w.Code != 400 || served {
		t.Fatalf("stream bypassed query rule: %d %v", w.Code, served)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/events?after=0", nil))
	if w.Code != 200 || !served || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream refused: %d %v", w.Code, served)
	}
}
func TestRegistryStrictRequestsTolerantCaching(t *testing.T) {
	r := New(func(context.Context, *http.Request, Metadata) error { return nil }, nil)
	m := testMetadata()
	err := Register(r, Route[testRequest, testResponse]{Metadata: m, Handler: func(_ context.Context, _ *http.Request, in testRequest) (testResponse, error) {
		return testResponse{in.Name, []string{}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"name":"one","name":"two"}`, `{"name":"ok","NAME":"two"}`, `{"NAME":"two"}`, `{"name":"ok","unknown":1}`, `{} {}`} {
		q := httptest.NewRequest("POST", "/v1/test", strings.NewReader(body))
		q.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code != 400 {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	m.Method = "GET"
	m.ID = "read"
	m.BodyLimit = 0
	if err = Register(r, Route[struct{}, testResponse]{Metadata: m, Handler: func(context.Context, *http.Request, struct{}) (testResponse, error) {
		return testResponse{"ok", []string{}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/test", nil))
	q := httptest.NewRequest("GET", "/v1/test", nil)
	q.Header.Set("If-None-Match", w.Header().Get("ETag"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, q)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatal("conditional read", w.Code, w.Body)
	}
	r.Freeze()
	if Register(r, Route[struct{}, struct{}]{Metadata: m}) == nil {
		t.Fatal("frozen registry changed")
	}
}

func TestExactFieldNamesPromoteEmbeddedJSONFields(t *testing.T) {
	type Embedded struct {
		ItemID string `json:"itemId"`
	}
	type Request struct {
		Embedded
		Label string `json:"label"`
	}
	var out Request
	if err := decodeStrict([]byte(`{"itemId":"item","label":"ok"}`), &out); err != nil || out.ItemID != "item" {
		t.Fatalf("promoted field rejected: %+v %v", out, err)
	}
	for _, body := range []string{`{"ItemID":"item","label":"ok"}`, `{"itemId":"item","LABEL":"ok"}`} {
		if err := decodeStrict([]byte(body), &out); err == nil {
			t.Fatalf("case-mismatched field accepted: %s", body)
		}
	}
}
func TestCanonicalBodyAndSafeErrors(t *testing.T) {
	a, err := CanonicalJSON([]byte(`{"b":2,"a":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := CanonicalJSON([]byte(`{"a":9007199254740993,"b":2}`))
	if string(a) != string(b) {
		t.Fatal("key order changed digest")
	}
	c, _ := CanonicalJSON([]byte(`{"a":9007199254740992,"b":2}`))
	if string(a) == string(c) {
		t.Fatal("integer precision lost")
	}
	r, _ := apierror.New()
	w := httptest.NewRecorder()
	r.Write(w, "test-id", errors.New("secret path"))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body)
	}
}

func TestGeneratedErrorsUseRegistryCatalogue(t *testing.T) {
	catalogue, err := apierror.New(apierror.Definition{Code: "invitation_not_found", Status: 404, Message: "This invitation is not available.", Retry: apierror.Never})
	if err != nil {
		t.Fatal(err)
	}
	r := New(nil, catalogue)
	artifacts, err := GenerateWith(r.Definitions(), Options{Errors: r.ErrorDefinitions()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(artifacts.Errors), `"code": "invitation_not_found"`) {
		t.Fatal("generated contracts omitted a registered server error")
	}
}

func TestNamedCommandPathsAndEncodedSeparators(t *testing.T) {
	r := New(nil, nil)
	for _, verb := range []string{"move", "cancel"} {
		m := testMetadata()
		m.ID = verb
		m.Path = "/v1/items/{id}:" + verb
		m.Access = Public
		m.BodyLimit = 0
		if err := Register(r, Route[struct{}, testResponse]{Metadata: m, Handler: func(_ context.Context, r *http.Request, _ struct{}) (testResponse, error) {
			return testResponse{r.PathValue("id"), []string{}}, nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/v1/items/abc:move", "/v1/items/abc:cancel"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "abc") {
			t.Fatal(w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/v1/items/a%2Fb:move", nil))
	if w.Code != 404 {
		t.Fatal("encoded separator matched", w.Code)
	}
}
