package httpapi

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"portico.local/apikit/apierror"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

func TestDAVConnectionFailureDoesNotRetainCredentialsOrTransportText(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{fmt.Errorf("https://fixture:private-secret@nas.example/media/: dial failed"), "internal"},
		{fmt.Errorf("private-secret: %w", storage.ErrRemoteCredentials), "source_credentials_required"},
		{fmt.Errorf("private-secret: %w", storage.ErrRemoteConfig), "invalid_request"},
		{fmt.Errorf("private-secret: %w", identity.ErrUnauthorized), "unauthorized"},
		{fmt.Errorf("private-secret: %w", identity.ErrForbidden), "not_permitted"},
	} {
		out := davConnectionFailure(test.err)
		var public *apierror.Error
		if !errors.As(out, &public) || public.Code != test.code || public.Cause != nil || strings.Contains(out.Error(), "private-secret") {
			t.Fatal("connection error retained private transport text", public)
		}
	}
}
