package redaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

const privatePathLabel = "<private-path>"
const secretLabel = "<redacted-secret>"

// reusablePorticoCredentialPattern is deliberately prefix-allowlisted. These
// prefixes identify bearer credentials issued by Server or Hosted lifecycle
// boundaries; arbitrary ptc_* public identifiers must remain diagnosable.
// Eight characters is below every production credential's entropy-bearing
// suffix while avoiding short logical labels such as ptc_api_id.
var reusablePorticoCredentialPattern = regexp.MustCompile(`ptc_(?:api|clt|loc|lrf|srv|mg|dg|pb|cb|cr|sdp)_[A-Za-z0-9_-]{8,}`)

// RedactPorticoCredentials removes standalone reusable Portico credentials
// without requiring them to appear in an Authorization header or key/value
// pair. It preserves surrounding punctuation and benign identifiers.
func RedactPorticoCredentials(value string) string {
	matches := reusablePorticoCredentialPattern.FindAllStringIndex(value, -1)
	if len(matches) == 0 {
		return value
	}
	var output strings.Builder
	output.Grow(len(value))
	cursor := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		if (start > 0 && isPorticoCredentialCharacter(value[start-1])) ||
			(end < len(value) && isPorticoCredentialCharacter(value[end])) {
			continue
		}
		output.WriteString(value[cursor:start])
		output.WriteString(secretLabel)
		cursor = end
	}
	if cursor == 0 {
		return value
	}
	output.WriteString(value[cursor:])
	return output.String()
}

func isPorticoCredentialCharacter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_'
}

// Policy is the process-wide operational logging policy. Paths are replaced
// with a generic marker, known credentials are replaced by value, and
// structured attributes are filtered by key so settings loaded later from
// SQLite do not become a secret-serialization escape hatch.
type Policy struct {
	SensitivePaths  []string
	SensitiveValues []string
}

// Basename is the only path-derived value that ordinary operational logs may
// expose. Callers should prefer a logical label when even a file name is not
// useful to an operator.
func Basename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unset"
	}
	return filepath.Base(filepath.Clean(value))
}

// OperationID provides a stable, opaque identifier for an operation without
// putting a configured path or secret into a log or API diagnostic.
func OperationID(kind, value string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + value))
	return strings.TrimSpace(kind) + "-" + hex.EncodeToString(sum[:])[:16]
}

func (p Policy) RedactString(value string) string {
	value = RedactPorticoCredentials(value)
	for _, sensitive := range p.SensitivePaths {
		sensitive = strings.TrimSpace(sensitive)
		if sensitive == "" {
			continue
		}
		value = strings.ReplaceAll(value, sensitive, privatePathLabel)
		if cleaned := filepath.Clean(sensitive); cleaned != sensitive {
			value = strings.ReplaceAll(value, cleaned, privatePathLabel)
		}
	}
	for _, sensitive := range p.SensitiveValues {
		sensitive = strings.TrimSpace(sensitive)
		if sensitive == "" {
			continue
		}
		value = strings.ReplaceAll(value, sensitive, secretLabel)
		if escaped := url.QueryEscape(sensitive); escaped != sensitive {
			value = strings.ReplaceAll(value, escaped, secretLabel)
		}
	}
	value = p.redactURLs(value)
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return value
	}
	redactSensitiveHeaderValues(fields)
	for index, field := range fields {
		prefix, core, suffix := trimPunctuation(field)
		if key, _, ok := strings.Cut(core, "="); ok && IsSensitiveKey(key) {
			fields[index] = prefix + key + "=" + secretLabel + suffix
			continue
		}
		if strings.Contains(core, privatePathLabel) || isAbsolutePathToken(core) {
			fields[index] = prefix + privatePathLabel + suffix
		}
	}
	return strings.Join(fields, " ")
}

// redactSensitiveHeaderValues treats Authorization and Proxy-Authorization as
// structured header fields, not as a one-word key/value pair. It handles
// normal slog text as well as quoted error wrappers such as
// error="Authorization: Bearer token" and stops at the next attribute token
// so harmless identifiers such as keyframe=4 remain visible.
func redactSensitiveHeaderValues(fields []string) {
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		lower := strings.ToLower(field)
		headerStart, headerName := sensitiveHeaderStart(lower)
		if headerStart < 0 {
			continue
		}
		colon := strings.Index(field[headerStart+len(headerName):], ":")
		if colon < 0 {
			continue
		}
		colon += headerStart + len(headerName)
		fields[index] = field[:colon+1] + redactHeaderInlineValue(field[colon+1:])
		for valueIndex := index + 1; valueIndex < len(fields); valueIndex++ {
			candidate := fields[valueIndex]
			_, core, suffix := trimPunctuation(candidate)
			if valueIndex > index+1 && isStructuredAttributeBoundary(core) {
				break
			}
			prefix, _, _ := trimPunctuation(candidate)
			fields[valueIndex] = prefix + secretLabel + suffix
			if strings.ContainsAny(suffix, "\"'") {
				break
			}
		}
	}
}

func sensitiveHeaderStart(value string) (int, string) {
	bestIndex := -1
	bestName := ""
	for _, name := range []string{"proxy-authorization", "authorization"} {
		index := strings.Index(value, name+":")
		if index >= 0 && (bestIndex < 0 || index < bestIndex) {
			bestIndex = index
			bestName = name
		}
	}
	return bestIndex, bestName
}

func redactHeaderInlineValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	prefix, _, suffix := trimPunctuation(value)
	return prefix + secretLabel + suffix
}

func isStructuredAttributeBoundary(core string) bool {
	if core == "" {
		return false
	}
	if key, _, ok := strings.Cut(core, "="); ok {
		return strings.TrimSpace(key) != ""
	}
	return strings.HasSuffix(core, ":")
}

// IsSensitiveKey is deliberately exact/suffix based. In particular, public
// identifiers such as keyframe and publicID are not secrets merely because
// they contain the letters "key" or "id".
func IsSensitiveKey(key string) bool {
	normalized := normalizeKey(key)
	if normalized == "" {
		return false
	}
	for _, exact := range []string{
		"token", "accesstoken", "refreshtoken", "idtoken", "secret", "clientsecret",
		"password", "passwd", "passphrase", "credential", "credentials", "apikey",
		"key", "accesskey", "authorization", "proxyauthorization", "cookie", "setcookie",
		"privatekey", "sessionid", "csrf", "xsrf",
	} {
		if normalized == exact {
			return true
		}
	}
	return strings.HasSuffix(normalized, "token") ||
		strings.HasSuffix(normalized, "secret") ||
		strings.HasSuffix(normalized, "password") ||
		strings.HasSuffix(normalized, "credential") ||
		strings.HasSuffix(normalized, "apikey") ||
		strings.Contains(normalized, "authorization") ||
		strings.HasSuffix(normalized, "cookie")
}

// Handler applies the same path policy to every attribute, including errors,
// used by the process logger. This protects future startup/restore messages
// that add an error without remembering to redact each call site manually.
type Handler struct {
	next   slog.Handler
	policy Policy
}

func NewHandler(next slog.Handler, policy Policy) slog.Handler {
	policy.SensitivePaths = append([]string(nil), policy.SensitivePaths...)
	policy.SensitiveValues = append([]string(nil), policy.SensitiveValues...)
	return &Handler{next: next, policy: policy}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	return h.handle(ctx, record)
}

func (h *Handler) handle(ctx context.Context, record slog.Record) (err error) {
	defer func() {
		if recover() != nil {
			err = h.handleFallback(ctx, record)
		}
	}()

	redacted := slog.NewRecord(record.Time, record.Level, h.policy.RedactString(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(h.redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

// handleFallback is deliberately lossy. If a user-supplied LogValuer, a
// malformed reflect.Value, or a downstream handler panics, logging must not
// turn into a process-wide denial of service or emit the original record.
func (h *Handler) handleFallback(ctx context.Context, record slog.Record) (err error) {
	defer func() {
		if recover() != nil {
			err = nil
		}
	}()
	fallback := slog.NewRecord(record.Time, record.Level, structuredRedactedLabel, record.PC)
	fallback.AddAttrs(slog.String("redaction", structuredRedactedLabel))
	return h.next.Handle(ctx, fallback)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, h.redactAttr(attr))
	}
	return &Handler{next: h.next.WithAttrs(redacted), policy: h.policy}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), policy: h.policy}
}

func (h *Handler) redactAttr(attr slog.Attr) (redacted slog.Attr) {
	defer func() {
		if recover() != nil {
			redacted = slog.Attr{Key: attr.Key, Value: slog.StringValue(structuredRedactedLabel)}
		}
	}()
	if IsSensitiveKey(attr.Key) {
		attr.Value = slog.StringValue(secretLabel)
	} else {
		attr.Value = h.redactValue(attr.Value)
	}
	return attr
}

func (h *Handler) redactValue(value slog.Value) slog.Value {
	if value.Kind() == slog.KindLogValuer {
		resolved := value.Resolve()
		// slog converts a panicking LogValuer into a diagnostic string. Do
		// not pass that diagnostic through: it can contain a stack and paths.
		if resolved.Kind() == slog.KindString && strings.HasPrefix(resolved.String(), "LogValue panicked") {
			return slog.StringValue(structuredRedactedLabel)
		}
		return h.redactValue(resolved)
	}
	switch value.Kind() {
	case slog.KindString:
		return slog.StringValue(h.policy.RedactString(value.String()))
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			if strings.HasPrefix(err.Error(), "LogValue panicked") {
				return slog.StringValue(structuredRedactedLabel)
			}
			return slog.StringValue(h.policy.RedactString(err.Error()))
		}
		return slog.AnyValue(h.policy.redactAny(value.Any(), "", map[visit]bool{}, 0))
	case slog.KindGroup:
		attrs := value.Group()
		for index := range attrs {
			attrs[index] = h.redactAttr(attrs[index])
		}
		return slog.GroupValue(attrs...)
	default:
		return value
	}
}

func normalizeKey(key string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(key)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (p Policy) redactURLs(value string) string {
	fields := strings.Fields(value)
	for index, field := range fields {
		prefix, core, suffix := trimPunctuation(field)
		if !strings.Contains(core, "://") {
			continue
		}
		parsed, err := url.Parse(core)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		changed := false
		if parsed.User != nil {
			parsed.User = url.User("redacted")
			changed = true
		}
		query := parsed.Query()
		for key := range query {
			if !IsSensitiveKey(key) {
				continue
			}
			query[key] = []string{secretLabel}
			changed = true
		}
		if changed {
			parsed.RawQuery = query.Encode()
			fields[index] = prefix + parsed.String() + suffix
		}
	}
	return strings.Join(fields, " ")
}

const (
	structuredRedactedLabel = "<structured-value-redacted>"
	structuredCycleLabel    = "<structured-cycle-redacted>"
	maxStructuredDepth      = 8
)

type visit struct {
	typ      reflect.Type
	kind     reflect.Kind
	ptr      uintptr
	identity string
	length   int
	capacity int
}

func (p Policy) redactAny(value any, key string, seen map[visit]bool, depth int) (redacted any) {
	defer func() {
		if recover() != nil {
			redacted = structuredRedactedLabel
		}
	}()
	if IsSensitiveKey(key) {
		return secretLabel
	}
	if value == nil {
		return nil
	}
	if depth > maxStructuredDepth {
		return structuredRedactedLabel
	}
	if err, ok := value.(error); ok {
		return p.RedactString(err.Error())
	}
	v := reflect.ValueOf(value)
	return p.redactReflect(v, key, seen, depth)
}

func (p Policy) redactReflect(value reflect.Value, key string, seen map[visit]bool, depth int) (redacted any) {
	defer func() {
		if recover() != nil {
			redacted = structuredRedactedLabel
		}
	}()
	if depth > maxStructuredDepth {
		return structuredRedactedLabel
	}
	if !value.IsValid() || IsSensitiveKey(key) {
		if IsSensitiveKey(key) {
			return secretLabel
		}
		return nil
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return p.redactReflect(value.Elem(), key, seen, depth+1)
	}
	if value.CanInterface() {
		if err, ok := value.Interface().(error); ok {
			return p.RedactString(err.Error())
		}
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return p.redactComposite(value, seen, func() any {
			return p.redactReflect(value.Elem(), key, seen, depth+1)
		})
	case reflect.String:
		return p.RedactString(value.String())
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		if !value.CanInterface() {
			return structuredRedactedLabel
		}
		return value.Interface()
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			if value.IsNil() {
				return nil
			}
			return p.RedactString(string(value.Bytes()))
		}
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		return p.redactComposite(value, seen, func() any {
			items := make([]any, value.Len())
			for index := 0; index < value.Len(); index++ {
				items[index] = p.redactReflect(value.Index(index), "", seen, depth+1)
			}
			return items
		})
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		if value.Type().Key().Kind() != reflect.String {
			return structuredRedactedLabel
		}
		return p.redactComposite(value, seen, func() any {
			items := map[string]any{}
			for _, mapKey := range value.MapKeys() {
				name := mapKey.String()
				items[name] = p.redactReflect(value.MapIndex(mapKey), name, seen, depth+1)
			}
			return items
		})
	case reflect.Struct:
		if value.CanInterface() {
			if stringer, ok := value.Interface().(fmt.Stringer); ok && value.Type().PkgPath() == "net/url" && value.Type().Name() == "URL" {
				return p.RedactString(stringer.String())
			}
			if value.Type().PkgPath() == "net/url" && value.Type().Name() == "Userinfo" {
				return secretLabel
			}
		}
		items := map[string]any{}
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if field.PkgPath != "" {
				continue
			}
			name := field.Name
			if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
				name = tag
			}
			items[name] = p.redactReflect(value.Field(index), name, seen, depth+1)
		}
		if len(items) == 0 {
			return structuredRedactedLabel
		}
		return items
	default:
		return structuredRedactedLabel
	}
}

func (p Policy) redactComposite(value reflect.Value, seen map[visit]bool, recurse func() any) any {
	marker, track := compositeVisit(value)
	if track {
		if seen[marker] {
			return structuredCycleLabel
		}
		seen[marker] = true
		defer delete(seen, marker)
	}
	return recurse()
}

func compositeVisit(value reflect.Value) (visit, bool) {
	if !value.IsValid() {
		return visit{}, false
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return visit{}, false
		}
		pointer := value.Pointer()
		if pointer == 0 {
			return visit{}, false
		}
		return visit{typ: value.Type(), kind: value.Kind(), ptr: pointer}, true
	case reflect.Map:
		if value.IsNil() || !value.CanInterface() {
			return visit{}, false
		}
		// reflect has no portable map-pointer accessor. Formatting the map
		// through %p obtains its identity without iterating or invoking a
		// user String method; the identity never reaches the output.
		return visit{
			typ:      value.Type(),
			kind:     value.Kind(),
			identity: fmt.Sprintf("%p", value.Interface()),
		}, true
	case reflect.Slice:
		if value.IsNil() || value.Len() == 0 {
			return visit{}, false
		}
		pointer := value.Pointer()
		if pointer == 0 {
			return visit{}, false
		}
		return visit{
			typ:      value.Type(),
			kind:     value.Kind(),
			ptr:      pointer,
			length:   value.Len(),
			capacity: value.Cap(),
		}, true
	default:
		return visit{}, false
	}
}

func trimPunctuation(value string) (string, string, string) {
	prefixLen := 0
	for prefixLen < len(value) && strings.ContainsRune("([{\"'", rune(value[prefixLen])) {
		prefixLen++
	}
	suffixLen := len(value)
	for suffixLen > prefixLen && strings.ContainsRune(",.;:!?)]}\"'", rune(value[suffixLen-1])) {
		suffixLen--
	}
	return value[:prefixLen], value[prefixLen:suffixLen], value[suffixLen:]
}

func isAbsolutePathToken(value string) bool {
	if value == "" || strings.Contains(value, "://") {
		return false
	}
	if filepath.IsAbs(value) {
		return true
	}
	// filepath.IsAbs follows the host platform. Retain a deterministic check
	// for Windows drive/UNC paths when logs are inspected on another platform.
	if len(value) >= 3 && unicode.IsLetter(rune(value[0])) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	return strings.HasPrefix(value, `\\`)
}

// Error is useful at boundaries that need to retain an error value while
// applying the same policy as the logger.
func Error(err error, sensitivePaths ...string) error {
	if err == nil {
		return nil
	}
	return errors.New((Policy{SensitivePaths: sensitivePaths}).RedactString(err.Error()))
}
