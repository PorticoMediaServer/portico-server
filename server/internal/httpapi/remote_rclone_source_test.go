package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/apikit/apierror"
	"portico.local/server/internal/mounts"
)

func TestRcloneSourceRouteKeepsOwnerFirstAndNoSubsystemFacts(t *testing.T) {
	d, owner := logoutHTTPFixture(t)
	registry, err := FoundationRegistry(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token  string
		status int
	}{{"", 401}, {owner.AccessToken, 503}} {
		r := httptest.NewRequest("POST", "/v1/storage/sources/rclone", bytes.NewBufferString(`{"operationId":"invalid","name":"Cloud","remote":"cloud:","config":"[cloud]\ntype=s3\nsecret_access_key=private-secret\n"}`))
		r.Header.Set("Content-Type", "application/json")
		if test.token != "" {
			r.Header.Set("Authorization", "Bearer "+test.token)
		}
		w := httptest.NewRecorder()
		registry.ServeHTTP(w, r)
		if w.Code != test.status || strings.Contains(w.Body.String(), "private-secret") {
			t.Fatal("source save changed owner/absence boundary", w.Code)
		}
	}
}

func TestRcloneSourceFailureRetainsSafeReceiptCodes(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{mounts.ErrCommandConflict, "storage_command_conflict"},
		{mounts.ErrCommandExpired, "storage_operation_expired"},
		{mounts.ErrCommandCapacity, "storage_command_capacity"},
		{mounts.ErrAllocationCapacity, "storage_allocation_recovery_required"},
		{fmt.Errorf("https://owner:private-secret@private.example/: failed"), "internal"},
	} {
		var failure *apierror.Error
		projected := rcloneSourceFailure(test.err)
		if !errors.As(projected, &failure) || failure.Code != test.code || failure.Cause != nil || strings.Contains(projected.Error(), "private-secret") {
			t.Fatal("source save leaked a private cause or lost receipt code", test.code)
		}
	}
}
