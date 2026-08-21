package config

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

// ConfigValidationError exposes only variable/authority names. Supplied
// values are intentionally omitted because this error is safe to include in
// local startup diagnostics and logs.
type ConfigValidationError struct {
	Fields []string
}

func (e ConfigValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "configuration is invalid"
	}
	return "invalid configuration: " + strings.Join(e.Fields, ", ")
}

// Validate distinguishes an absent value (which may use the documented safe
// default in Load) from a supplied value that cannot be trusted. The process
// host calls this before opening a listener or database; embedded callers may
// use it as the same fail-closed gate.
func (c Config) Validate() error {
	invalid := []string{}
	add := func(name string) { invalid = append(invalid, name) }

	if raw := strings.TrimSpace(os.Getenv("PORTICO_COOKIE_SECURE")); raw != "" {
		if _, err := strconv.ParseBool(raw); err != nil {
			add("PORTICO_COOKIE_SECURE")
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PORTICO_PORT")); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			add("PORTICO_PORT")
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PORTICO_ADDR")); raw != "" {
		if _, _, err := net.SplitHostPort(raw); err != nil {
			add("PORTICO_ADDR")
		}
	}
	validateOrigins := func(name, raw string) {
		for _, value := range splitCSV(raw) {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
				add(name)
				return
			}
		}
	}
	validateOrigins("PORTICO_PUBLIC_ORIGIN", os.Getenv("PORTICO_PUBLIC_ORIGIN"))
	validateOrigins("PORTICO_ALLOWED_ORIGINS", os.Getenv("PORTICO_ALLOWED_ORIGINS"))
	validateOrigins("PORTICO_CAST_RECEIVER_ORIGINS", os.Getenv("PORTICO_CAST_RECEIVER_ORIGINS"))
	if raw := strings.TrimSpace(os.Getenv("PORTICO_TRUSTED_PROXY_CIDRS")); raw != "" {
		if _, err := parseTrustedProxyCIDRs(raw); err != nil {
			add("PORTICO_TRUSTED_PROXY_CIDRS")
		}
	}
	validateDuration := func(name, raw string, minimum, maximum time.Duration) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || value < minimum || value > maximum {
			add(name)
		}
	}
	validateDuration("PORTICO_RESTORE_SAFETY_COPY_TIMEOUT", os.Getenv("PORTICO_RESTORE_SAFETY_COPY_TIMEOUT"), time.Minute, 24*time.Hour)
	validateDuration("PORTICO_RESTORE_IO_TIMEOUT", os.Getenv("PORTICO_RESTORE_IO_TIMEOUT"), time.Minute, 24*time.Hour)
	if raw := strings.TrimSpace(os.Getenv("PORTICO_RESTORE_MAX_DATABASE_BYTES")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1<<20 || value > 64<<30 {
			add("PORTICO_RESTORE_MAX_DATABASE_BYTES")
		}
	}
	environment := strings.TrimSpace(c.Environment)
	if environment == "" {
		environment = foundationEnvironmentFallback()
	}
	switch environment {
	case "development", "test", "staging", "production":
	default:
		add("PORTICO_ENVIRONMENT")
	}
	if environment == "staging" || environment == "production" {
		if strings.TrimSpace(os.Getenv("PORTICO_HOSTED_API_AUTHORITY")) == "" || strings.TrimSpace(c.HostedAPIAuthority) == "" {
			add("PORTICO_HOSTED_API_AUTHORITY")
		}
	}
	if strings.TrimSpace(c.ConfigPath) != "" {
		if _, err := LoadRuntimePaths(c.ConfigPath); err != nil {
			add("PORTICO_CONFIG_FILE")
		}
	}
	if len(invalid) > 0 {
		return ConfigValidationError{Fields: uniqueValidationFields(invalid)}
	}
	return nil
}

// Keep the default dependency local to this file so Config.Validate remains
// useful for zero-value test Configs without making callers provide it.
func foundationEnvironmentFallback() string {
	return foundationcontract.DefaultEnvironment
}

func uniqueValidationFields(fields []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) == "" || seen[field] {
			continue
		}
		seen[field] = true
		result = append(result, field)
	}
	return result
}

var _ error = ConfigValidationError{}
