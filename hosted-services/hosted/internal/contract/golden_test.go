package contract_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"portico.local/apikit"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/contract"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/trust"
)

var update = flag.Bool("update", false, "rewrite testdata golden responses")

// database opens a private schema in the isolated test database, as
// internal/postgres's fixture does. It skips without PORTICO_TEST_DATABASE_URL_FILE.
func database(t *testing.T) *postgres.Store {
	t.Helper()
	path := os.Getenv("PORTICO_TEST_DATABASE_URL_FILE")
	if path == "" {
		t.Skip("set PORTICO_TEST_DATABASE_URL_FILE to isolated PostgreSQL test URL file")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	dsn, e := url.Parse(strings.TrimSpace(string(raw)))
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	if !strings.Contains(dsn.Path, "test") {
		t.Fatal("refusing database not named test")
	}
	ctx := context.Background()
	admin, e := postgres.Open(ctx, dsn.String())
	if e != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	schema := "test" + strings.ToLower(strings.NewReplacer("-", "x", "_", "x").Replace(security.RandomID("")[:15]))
	if _, e = admin.DB.ExecContext(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	var store *postgres.Store
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.DB.ExecContext(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error("owned schema cleanup failed", err)
		}
		if err := admin.DB.Close(); err != nil {
			t.Error("fixture admin close failed", err)
		}
	})
	q := dsn.Query()
	q.Set("search_path", schema)
	dsn.RawQuery = q.Encode()
	store, e = postgres.Open(ctx, dsn.String())
	if e != nil {
		t.Fatal("open test schema")
	}
	t.Cleanup(func() {
		if err := store.DB.Close(); err != nil {
			t.Error("fixture store close failed", err)
		}
	})
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	return store
}

type golden struct {
	handler     http.Handler
	owner       identity.Session
	server      string
	serverToken string
}

const claimAudience = "https://hosted.example"

// goldenSetup builds the production handler over a real store: a certified
// directory key, an owner account and an installed server it owns.
func goldenSetup(t *testing.T) golden {
	t.Helper()
	store := database(t)
	ctx := context.Background()
	rootPublic, rootPrivate, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	public, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	keys := &security.Keys{RootPublicKey: rootPublic, PrivateKey: private, KeyID: trust.KeyID(public), SealKey: bytes.Repeat([]byte{7}, 32)}
	if keys.Certificate, e = trust.Certify(rootPrivate, public, keys.KeyID, "documents", time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour), 1, nil); e != nil {
		t.Fatal(e)
	}
	ids := identity.New(store)
	owner, e := ids.Register(ctx, "owner", "Owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	serverKey, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	server, e := claims.ServerID(serverKey)
	if e != nil {
		t.Fatal(e)
	}
	token := security.RandomID("hst_")
	sealed, e := keys.Seal(token, server)
	if e != nil {
		t.Fatal(e)
	}
	// An already installed current-format claim, as internal/postgres's fixture seeds it.
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO servers(id,owner_account_id,name,base_url,public_key,token_hash,token_ciphertext,claim_generation,credential_generation) VALUES($1,$2,'Local','http://127.0.0.1:19411',$3,$4,$5,1,1)`, []any{server, owner.Account.ID, base64.RawURLEncoding.EncodeToString(serverKey), security.Hash(token), sealed}},
		{`INSERT INTO claim_operations(id,server_id,account_id,public_key,local_generation,server_name,intent_digest,state,claim_generation,credential_generation,committed_token_hash,installed_at) VALUES('fixture_installed_claim',$1,$2,$3,0,'Local',$4,'installed',1,1,$5,clock_timestamp())`, []any{server, owner.Account.ID, []byte(serverKey), security.Hash("fixture installed claim"), security.Hash(token)}},
		{`INSERT INTO server_members(server_id,account_id,role) VALUES($1,$2,'owner')`, []any{server, owner.Account.ID}},
		{`INSERT INTO wake_state(server_id,kind,desired_revision) VALUES($1,'policy',1)`, []any{server}},
	} {
		if _, e = store.DB.ExecContext(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	service, e := claims.New(store, keys, claimAudience)
	if e != nil {
		t.Fatal(e)
	}
	accountSecurity := &postgres.AccountSecurity{Store: store, Keys: keys, Identity: ids, Lifetime: 15 * time.Minute}
	h := &httpapi.Handler{
		Identity:  ids,
		Directory: directory.New(store, keys),
		Security:  accountSecurity,
		Claims:    &httpapi.ClaimHandler{Web: &httpapi.ClaimWeb{Store: store, Keys: keys, Audience: claimAudience}, Service: service, CaptureAuthority: store.CaptureClaimAuthority},
	}
	return golden{h.Routes(), owner, server, token}
}

// signedWebClaim is a server's signed request to be claimed through the web.
func signedWebClaim(t *testing.T) claimweb.Signed {
	t.Helper()
	public, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.New()
	digest.Write([]byte("portico.server.identity.v1\x00"))
	digest.Write(public)
	sum := digest.Sum(nil)
	now := time.Now().UTC()
	raw, e := json.Marshal(claimweb.Request{Purpose: "setup", Kind: "portico.claim.web.v2", Audience: claimAudience, OperationID: "op_golden_claim", ServerID: "srv_" + base64.RawURLEncoding.EncodeToString(sum), PublicKey: base64.RawURLEncoding.EncodeToString(public), Name: "Living Room", Address: "http://192.168.1.10:32500", IssuedAt: now, ExpiresAt: now.Add(4 * time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	return claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, raw))}
}

func definition(t *testing.T, id string) apikit.Definition {
	t.Helper()
	for _, d := range contract.Definitions() {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no route %s", id)
	return apikit.Definition{}
}

// call sends a request to a declared route and returns its body after checking
// the status the contract declares.
func call(t *testing.T, h http.Handler, d apikit.Definition, params map[string]string, bearer string, body any) []byte {
	t.Helper()
	path := d.Path
	for key, value := range params {
		path = strings.ReplaceAll(path, "{"+key+"}", url.PathEscape(value))
	}
	var reader io.Reader
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		reader = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(d.Method, path, reader)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != d.Status {
		t.Fatalf("%s answered %d, the contract declares %d: %s", d.ID, w.Code, d.Status, w.Body.String())
	}
	return w.Body.Bytes()
}

// TestGoldenResponses checks real handler responses against the contract: each
// decodes strictly into the declared Go type, satisfies the generated schema
// (required members present and non-null, no undeclared members), and matches
// its normalized golden body under testdata. Run with -update to rewrite them.
func TestGoldenResponses(t *testing.T) {
	g := goldenSetup(t)
	spec := openAPI(t)
	check := func(id string, raw []byte, volatile ...string) {
		t.Helper()
		d := definition(t, id)
		target := reflect.New(d.Response).Interface()
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(target); e != nil {
			t.Errorf("%s: response does not decode strictly into %s: %v", id, d.Response, e)
		}
		if _, e := decoder.Token(); !errors.Is(e, io.EOF) {
			t.Errorf("%s: trailing data after the response", id)
		}
		var value any
		decoder = json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if e := decoder.Decode(&value); e != nil {
			t.Fatalf("%s: %v", id, e)
		}
		for _, problem := range conform(spec.response(t, d), value, "$") {
			t.Errorf("%s: %s", id, problem)
		}
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if e := encoder.Encode(normalize(value, volatile)); e != nil {
			t.Fatal(e)
		}
		normalized := buffer.Bytes()
		file := filepath.Join("testdata", id+".json")
		if *update {
			if e := os.WriteFile(file, normalized, 0644); e != nil {
				t.Fatal(e)
			}
			return
		}
		want, e := os.ReadFile(file)
		if e != nil || !bytes.Equal(want, normalized) {
			t.Errorf("%s: response differs from %s (run go test ./internal/contract -run TestGoldenResponses -update after reviewing the change):\n%s", id, file, normalized)
		}
	}
	check("system_get", call(t, g.handler, definition(t, "system_get"), nil, "", nil))
	check("keys_get", call(t, g.handler, definition(t, "keys_get"), nil, "", nil), "keyId", "publicKey", "root", "signature")
	check("account_notifications_list", call(t, g.handler, definition(t, "account_notifications_list"), nil, g.owner.AccessToken, nil), "id", "deviceId")
	check("servers_list", call(t, g.handler, definition(t, "servers_list"), nil, g.owner.AccessToken, nil), "id", "profileId", "accountId", "ownerAccountId", "payload", "signature", "keyId", "publicKey", "watch")
	pendingRaw := call(t, g.handler, definition(t, "server_claim_pending"), nil, "", signedWebClaim(t))
	check("server_claim_pending", pendingRaw, "code", "serverId")
	var pending claimweb.Pending
	if e := json.Unmarshal(pendingRaw, &pending); e != nil {
		t.Fatal(e)
	}
	check("server_claim_lookup", call(t, g.handler, definition(t, "server_claim_lookup"), nil, g.owner.AccessToken, httpapi.ClaimCodeRequest{Code: pending.Code}), "code", "serverId")
}

type spec struct {
	Paths map[string]map[string]struct {
		Responses map[string]struct {
			Content map[string]struct {
				Schema map[string]any `json:"schema"`
			} `json:"content"`
		} `json:"responses"`
	} `json:"paths"`
}

// openAPI reads the committed generated document, so the golden responses are
// held to what clients are given, not to a regenerated copy.
func openAPI(t *testing.T) spec {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(root, "hosted-services/hosted/api/hosted.openapi.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s spec
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	return s
}

func (s spec) response(t *testing.T, d apikit.Definition) map[string]any {
	t.Helper()
	schema := s.Paths[d.Path][strings.ToLower(d.Method)].Responses[fmt.Sprint(d.Status)].Content["application/json"].Schema
	if schema == nil {
		t.Fatalf("%s: no response schema in hosted.openapi.json", d.ID)
	}
	return schema
}

// conform validates a decoded JSON value against a generated schema. Unlike the
// tolerant client decoder it rejects members the contract does not declare, so a
// field added to a handler without reaching the contract fails here.
func conform(schema map[string]any, value any, path string) []string {
	if len(schema) == 0 {
		return nil
	}
	if expected, ok := schema["const"]; ok && !sameJSON(expected, value) {
		return []string{path + " does not match its const constraint"}
	}
	if alternatives, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range alternatives {
			matched = matched || sameJSON(candidate, value)
		}
		if !matched {
			return []string{path + " is not an allowed enum value"}
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		alternatives, ok := schema[keyword].([]any)
		if !ok {
			continue
		}
		matches := 0
		for _, alternative := range alternatives {
			branch, valid := alternative.(map[string]any)
			if valid && len(conform(branch, value, path)) == 0 {
				matches++
			}
		}
		if matches == 0 || (keyword == "oneOf" && matches != 1) {
			return []string{path + " does not match " + keyword + " alternatives"}
		}
		remainder := make(map[string]any, len(schema)-1)
		for key, constraint := range schema {
			if key != keyword {
				remainder[key] = constraint
			}
		}
		return conform(remainder, value, path)
	}
	if value == nil {
		if schema["nullable"] == true || schema["type"] == "null" {
			return nil
		}
		return []string{path + " is null"}
	}
	fail := func(want string) []string {
		return []string{fmt.Sprintf("%s is %T, the contract says %s", path, value, want)}
	}
	switch schema["type"] {
	case "null":
		return fail("null")
	case "string":
		s, ok := value.(string)
		if !ok {
			return fail("string")
		}
		if schema["format"] == "date-time" {
			if _, e := time.Parse(time.RFC3339Nano, s); e != nil {
				return []string{path + " is not a date-time"}
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fail("boolean")
		}
	case "integer":
		n, ok := value.(json.Number)
		if !ok {
			return fail("integer")
		}
		if _, e := n.Int64(); e != nil {
			return []string{path + " is not an integer"}
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return fail("number")
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fail("array")
		}
		var problems []string
		for i, item := range items {
			problems = append(problems, conform(schema["items"].(map[string]any), item, fmt.Sprintf("%s[%d]", path, i))...)
		}
		return problems
	case "object":
		members, ok := value.(map[string]any)
		if !ok {
			return fail("object")
		}
		var problems []string
		if additional, ok := schema["additionalProperties"].(map[string]any); ok {
			for key, member := range members {
				problems = append(problems, conform(additional, member, path+"."+key)...)
			}
			return problems
		}
		properties, _ := schema["properties"].(map[string]any)
		required, _ := schema["required"].([]any)
		for _, key := range required {
			if _, ok := members[key.(string)]; !ok {
				problems = append(problems, fmt.Sprintf("%s.%s is required but missing", path, key))
			}
		}
		keys := make([]string, 0, len(members))
		for key := range members {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			property, ok := properties[key].(map[string]any)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.%s is not in the contract", path, key))
				continue
			}
			problems = append(problems, conform(property, members[key], path+"."+key)...)
		}
		return problems
	default:
		return []string{fmt.Sprintf("%s: unsupported schema %v", path, schema)}
	}
	return nil
}

func sameJSON(a, b any) bool {
	left, leftError := json.Marshal(a)
	right, rightError := json.Marshal(b)
	return leftError == nil && rightError == nil && bytes.Equal(left, right)
}

func TestConformNullableDateTimeUnion(t *testing.T) {
	schema := map[string]any{"anyOf": []any{
		map[string]any{"type": "string", "format": "date-time"},
		map[string]any{"type": "null"},
	}}
	for _, value := range []any{nil, "2026-10-17T03:00:00Z"} {
		if problems := conform(schema, value, "$.at"); len(problems) != 0 {
			t.Fatalf("valid value %v: %v", value, problems)
		}
	}
	for _, value := range []any{true, json.Number("1"), "yesterday", map[string]any{}} {
		if len(conform(schema, value, "$.at")) == 0 {
			t.Fatalf("invalid value accepted: %v", value)
		}
	}
	schema["type"] = "string"
	if len(conform(schema, nil, "$.at")) == 0 {
		t.Fatal("anyOf bypassed its sibling type constraint")
	}
}

var timestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

// normalize replaces values that differ between runs: every timestamp, and the
// string values of the named members wherever they appear.
func normalize(value any, volatile []string) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, member := range v {
			if s, ok := member.(string); ok && contains(volatile, key) && s != "" {
				out[key] = "<" + key + ">"
				continue
			}
			out[key] = normalize(member, volatile)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalize(item, volatile)
		}
		return out
	case string:
		if timestamp.MatchString(v) {
			return "<time>"
		}
	}
	return value
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func TestConformRejectsContractDrift(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"id", "at"}, "properties": map[string]any{
		"id":    map[string]any{"type": "string"},
		"at":    map[string]any{"type": "string", "format": "date-time"},
		"count": map[string]any{"type": "integer"},
		"any":   map[string]any{},
	}}
	for raw, want := range map[string]int{
		`{"id":"a","at":"2026-09-23T10:00:00Z","any":null}`:            0,
		`{"id":"a","at":"2026-09-23T10:00:00Z","count":1.5}`:           1,
		`{"at":"2026-09-23T10:00:00Z"}`:                                1,
		`{"id":null,"at":"2026-09-23T10:00:00Z"}`:                      1,
		`{"id":"a","at":"yesterday"}`:                                  1,
		`{"id":"a","at":"2026-09-23T10:00:00Z","extra":true}`:          1,
		`{"id":"a","at":"2026-09-23T10:00:00Z","count":"1","extra":1}`: 2,
	} {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var value any
		if e := decoder.Decode(&value); e != nil {
			t.Fatal(e)
		}
		if got := conform(schema, value, "$"); len(got) != want {
			t.Errorf("%s: %d problems %v, want %d", raw, len(got), got, want)
		}
	}
}
