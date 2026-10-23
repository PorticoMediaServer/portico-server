package webhooks

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/operations"
)

type integrationRoundTrip func(*http.Request) (*http.Response, error)

func (f integrationRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWebhookSecurePolicyRefusesBeforeSendingPayload(t *testing.T) {
	receiver := newReceiver(t)
	d := NewDispatcher(&Store{Now: time.Now})
	q := queued{url: receiver.server.URL, secret: "never-send-this-secret", event: Test}
	policy := operations.DefaultSettings().Integrations()
	if status, code := d.postWithPolicy(context.Background(), q, []byte("private payload"), policy); status != 204 || code != "" {
		t.Fatalf("allowed HTTP: %d %s", status, code)
	}
	policy.WebhookHTTPSOnly = true
	if status, code := d.postWithPolicy(context.Background(), q, []byte("private payload"), policy); status != 0 || code != "secure_address_required" {
		t.Fatalf("secure-only HTTP: %d %s", status, code)
	}
	if receiver.count() != 1 {
		t.Fatal("refused address received payload or signature")
	}
	if !policyURL("https://example.com/webhook", policy) {
		t.Fatal("secure policy refused HTTPS")
	}
}

func TestWebhookAttemptUsesChosenTimeoutWithoutChangingSharedClient(t *testing.T) {
	d := NewDispatcher(&Store{Now: time.Now})
	for _, seconds := range []int{5, 15, 30, 60} {
		policy := operations.DefaultSettings().Integrations()
		policy.WebhookTimeoutSeconds = seconds
		d.Client = &http.Client{Timeout: time.Second, Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			remaining := time.Until(deadline)
			if !ok || remaining > time.Duration(seconds)*time.Second || remaining < time.Duration(seconds)*time.Second-time.Second {
				t.Fatalf("%d second policy deadline: %v %v", seconds, remaining, ok)
			}
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		})}
		if status, code := d.postWithPolicy(context.Background(), queued{url: "https://example.com/webhook", secret: "secret"}, []byte("{}"), policy); status != 204 || code != "" {
			t.Fatalf("%d: %d %s", seconds, status, code)
		}
		if d.Client.Timeout != time.Second {
			t.Fatal("attempt changed the shared client's timeout")
		}
	}
}
