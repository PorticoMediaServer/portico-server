package redaction

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactStringRemovesConfiguredAndAbsolutePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-data")
	message := "open " + filepath.Join(root, "portico.db") + ": permission denied"
	redacted := (Policy{SensitivePaths: []string{root}}).RedactString(message)
	if strings.Contains(redacted, root) || strings.Contains(redacted, "portico.db") {
		t.Fatalf("redacted message leaked a private path: %q", redacted)
	}
}

func TestHandlerRedactsErrorAttributesAndPreservesLogicalValues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-data")
	var buffer bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buffer, nil), Policy{SensitivePaths: []string{root}}))
	logger.Error("restore failed", "error", errors.New("copy "+filepath.Join(root, "restore-pending.db")+" failed"), "backup", "portico-20260805.db")
	output := buffer.String()
	if strings.Contains(output, root) || strings.Contains(output, "restore-pending.db") {
		t.Fatalf("logger leaked a private path: %q", output)
	}
	if !strings.Contains(output, "portico-20260805.db") {
		t.Fatalf("logger removed a permitted logical backup name: %q", output)
	}
}

func TestHandlerRedactsProviderSecretsSensitiveKeysAndURLCredentials(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-data")
	providerSecret := "tmdb-secret-123" // gitleaks:allow -- synthetic redaction fixture
	var buffer bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buffer, nil), Policy{
		SensitivePaths:  []string{root},
		SensitiveValues: []string{providerSecret},
	}))
	logger.Error(
		"relay failed",
		"token", "session-token-456",
		"password", "database-password-789",
		"apiKey", providerSecret,
		"settings", map[string]any{
			"acoustidApiKey": "settings-secret-abc",
			"publicID":       "public-resource-42",
		},
		"providerURL", "https://relay-user:relay-password@example.test/api?api_key="+providerSecret+"&page=2",
		"safeURL", "https://example.test/media?keyframe=3&title=Sun",
		"keyframe", "keyframe-001",
	)
	logger.Error("provider response", "error", errors.New("authorization: bearer-unknown password=inline-secret keyframe=4"))
	logger.Error("proxy response", "error", errors.New(`wrapped error="Proxy-Authorization: Bearer proxy-runtime-token" Content-Type: video/mp4`))
	output := buffer.String()
	for _, secret := range []string{providerSecret, "session-token-456", "database-password-789", "settings-secret-abc", "relay-user", "relay-password", "bearer-unknown", "inline-secret", "proxy-runtime-token"} {
		if strings.Contains(output, secret) {
			t.Fatalf("logger leaked secret %q: %q", secret, output)
		}
	}
	for _, permitted := range []string{"public-resource-42", "keyframe-001", "keyframe=4", "https://example.test/media?keyframe=3&title=Sun", "Content-Type:", "video/mp4"} {
		if !strings.Contains(output, permitted) {
			t.Fatalf("logger over-redacted permitted value %q: %q", permitted, output)
		}
	}
}

func TestSensitiveKeyPolicyDoesNotMatchPublicIdentifiers(t *testing.T) {
	for _, key := range []string{"keyframe", "publicID", "identifier", "media_keyframe"} {
		if IsSensitiveKey(key) {
			t.Fatalf("public identifier key %q was classified as sensitive", key)
		}
	}
	for _, key := range []string{"token", "refresh_token", "client_secret", "api_key", "password", "authorization", "cookie"} {
		if !IsSensitiveKey(key) {
			t.Fatalf("credential key %q was not classified as sensitive", key)
		}
	}
}

func TestReusablePorticoCredentialsAreRedactedInEveryTextPosition(t *testing.T) {
	prefixes := []string{"api", "clt", "loc", "lrf", "srv", "mg", "dg", "pb", "cb", "cr", "sdp"}
	for _, prefix := range prefixes {
		credential := "ptc_" + prefix + "_" + strings.Repeat("A", 42) + "_"
		messages := []string{
			credential,
			"(" + credential + "),",
			`{"credential":"` + credential + `"}`,
			"https://example.test/callback?proof=" + credential + "&page=2",
			"AuThOrIzAtIoN: bEaReR " + credential,
			"token=" + credential,
		}
		for _, message := range messages {
			redacted := RedactPorticoCredentials(message)
			if strings.Contains(redacted, credential) || !strings.Contains(redacted, secretLabel) {
				t.Fatalf("credential prefix %q was not redacted", prefix)
			}
		}
	}
}

func TestReusablePorticoCredentialRedactionPreservesHarmlessIdentifiers(t *testing.T) {
	harmless := []string{
		"ptc_api_id",
		"ptc_media_" + strings.Repeat("A", 43),
		"public_ptc_api_" + strings.Repeat("A", 43),
		"apikey_" + strings.Repeat("A", 43),
	}
	for _, value := range harmless {
		if redacted := RedactPorticoCredentials(value); redacted != value {
			t.Fatalf("harmless identifier was redacted")
		}
	}
}

func TestPolicyRedactsAdjacentReusablePorticoCredentials(t *testing.T) {
	first := "ptc_api_" + strings.Repeat("A", 43)
	second := "ptc_mg_" + strings.Repeat("B", 43)
	redacted := (Policy{}).RedactString(first + "," + second)
	if strings.Contains(redacted, first) || strings.Contains(redacted, second) || strings.Count(redacted, secretLabel) != 2 {
		t.Fatal("adjacent credentials were not independently redacted")
	}
}

func TestOperationIDIsOpaqueAndStable(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "restore-pending.db")
	first := OperationID("restore", secretPath)
	second := OperationID("restore", secretPath)
	if first == "" || first != second || strings.Contains(first, secretPath) || strings.Contains(first, "restore-pending") {
		t.Fatalf("operation id is not opaque/stable: %q %q", first, second)
	}
}

func TestHandlerFailsClosedForCyclicStructuredValues(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buffer, nil), Policy{}))

	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := make([]any, 1)
	cyclicSlice[0] = cyclicSlice

	logger.Info("cyclic values", "map", cyclicMap, "slice", cyclicSlice)
	output := buffer.String()
	if !strings.Contains(output, structuredCycleLabel) {
		t.Fatalf("cyclic structured values were not marked: %q", output)
	}
}

func TestHandlerBoundsDeepAcyclicStructuredValues(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buffer, nil), Policy{}))

	var nested any = "leaf"
	for index := 0; index < maxStructuredDepth+4; index++ {
		nested = map[string]any{"next": nested}
	}

	logger.Info("deep value", "value", nested)
	if !strings.Contains(buffer.String(), structuredRedactedLabel) {
		t.Fatalf("deep structured value was not bounded: %q", buffer.String())
	}
}

func TestHandlerHandlesTypedNilAndUnexportedFields(t *testing.T) {
	type privateFields struct {
		secret string
		Public string
	}
	var typedNil *privateFields

	var buffer bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buffer, nil), Policy{}))
	logger.Info("safe values", "typedNil", typedNil, "fields", privateFields{secret: "hidden-secret", Public: "visible-public"})

	output := buffer.String()
	if strings.Contains(output, "hidden-secret") {
		t.Fatalf("unexported field leaked through structured sanitizer: %q", output)
	}
	if !strings.Contains(output, "visible-public") {
		t.Fatalf("exported field was lost unexpectedly: %q", output)
	}
}

type panicLogValuer struct{}

func (panicLogValuer) LogValue() slog.Value {
	panic("log-valued panic")
}

func TestHandlerNeverPanicsOnPanickingLogValuer(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&buffer, nil), Policy{}))

	logger.Info("panic value", "value", panicLogValuer{})
	if !strings.Contains(buffer.String(), structuredRedactedLabel) {
		t.Fatalf("panicking LogValuer was not replaced with a redacted marker: %q", buffer.String())
	}
}
