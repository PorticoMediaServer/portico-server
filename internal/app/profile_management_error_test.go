package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileManagementErrorsNeverExposeUnexpectedStorageDetails(t *testing.T) {
	recorder := httptest.NewRecorder()
	server := &Server{}
	server.writeProfileManagementError(recorder, errors.New("sqlite: secret profile row failed"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var problem struct {
		Code      string `json:"code"`
		Detail    string `json:"detail"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "profile_request_failed" || problem.RequestID == "" {
		t.Fatalf("unexpected normalized problem: %#v", problem)
	}
	if strings.Contains(strings.ToLower(problem.Detail), "sqlite") || strings.Contains(strings.ToLower(problem.Detail), "secret") {
		t.Fatalf("storage detail leaked to client: %q", problem.Detail)
	}
}

func TestProfileManagementRestrictionErrorsUseStableProductDetail(t *testing.T) {
	recorder := httptest.NewRecorder()
	server := &Server{}
	server.writeProfileManagementError(recorder, errors.Join(errInvalidProfileRestriction, errors.New("private validation implementation")))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "private validation") {
		t.Fatalf("validation implementation leaked to client: %s", recorder.Body.String())
	}
}
