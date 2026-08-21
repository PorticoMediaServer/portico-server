package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRejectsSuppliedSecurityValuesInsteadOfDowngrading(t *testing.T) {
	t.Setenv("PORTICO_COOKIE_SECURE", "tru")
	t.Setenv("PORTICO_PUBLIC_ORIGIN", "not-an-origin")
	t.Setenv("PORTICO_TRUSTED_PROXY_CIDRS", "10.0.0.0/33")

	err := (Config{Environment: "development"}).Validate()
	if err == nil {
		t.Fatal("invalid supplied security configuration was accepted")
	}
	message := err.Error()
	for _, field := range []string{"PORTICO_COOKIE_SECURE", "PORTICO_PUBLIC_ORIGIN", "PORTICO_TRUSTED_PROXY_CIDRS"} {
		if !strings.Contains(message, field) {
			t.Fatalf("validation error %q does not name %s", message, field)
		}
	}
}

func TestValidateRejectsMalformedRuntimePathAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portico.config.json")
	if err := os.WriteFile(path, []byte(`{"databasePath":`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (Config{Environment: "development", ConfigPath: path}).Validate()
	if err == nil || !strings.Contains(err.Error(), "PORTICO_CONFIG_FILE") {
		t.Fatalf("malformed runtime path document validation error = %v", err)
	}
}

func TestValidateAllowsAbsentValuesWithSafeDefaults(t *testing.T) {
	for _, key := range []string{
		"PORTICO_COOKIE_SECURE", "PORTICO_PORT", "PORTICO_ADDR", "PORTICO_PUBLIC_ORIGIN",
		"PORTICO_ALLOWED_ORIGINS", "PORTICO_CAST_RECEIVER_ORIGINS", "PORTICO_TRUSTED_PROXY_CIDRS",
		"PORTICO_RESTORE_SAFETY_COPY_TIMEOUT", "PORTICO_RESTORE_IO_TIMEOUT", "PORTICO_RESTORE_MAX_DATABASE_BYTES",
		"PORTICO_HOSTED_API_AUTHORITY",
	} {
		t.Setenv(key, "")
	}
	if err := (Config{Environment: "development"}).Validate(); err != nil {
		t.Fatalf("absent values should use safe defaults: %v", err)
	}
}
