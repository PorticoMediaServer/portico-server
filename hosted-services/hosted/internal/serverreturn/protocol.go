// Package serverreturn is the first-party registered-server handoff. It does
// not accept app redirect URIs or carry Hosted account credentials.
package serverreturn

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"portico.local/apikit"
	"portico.local/hosted/trust"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid server return")
var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var secret = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

func Purpose(v string) bool { return v == "setup" || v == "switch" || v == "change-owner" }
func Binding(purpose, current, previous string) bool {
	if !Purpose(purpose) {
		return false
	}
	if purpose == "setup" {
		return current == "" && previous == ""
	}
	if !identifier.MatchString(current) {
		return false
	}
	if purpose == "switch" {
		return previous == ""
	}
	return previous == "admin" || previous == "member" || previous == "remove"
}

// Origin accepts an exact HTTPS origin, including its registered port. ASCII
// canonical spelling prevents URL-parser normalization from widening authority.
func Origin(v string) bool {
	for _, c := range v {
		if c < 33 || c > 126 {
			return false
		}
	}
	u, e := url.Parse(v)
	return e == nil && u.Scheme == "https" && u.User == nil && u.Host != "" && u.Host == strings.ToLower(u.Host) && u.Hostname() != "" && !strings.HasSuffix(u.Host, ":") && u.Path == "" && u.RawQuery == "" && u.ForceQuery == false && u.Fragment == "" && u.Opaque == "" && u.String() == v && !strings.HasSuffix(u.Hostname(), ".")
}
func RegistrationID(server, operation string, claim, credential, generation int64, origin string) string {
	raw, _ := json.Marshal([]any{server, operation, claim, credential, generation, origin})
	hash := sha256.Sum256(raw)
	return "ret_" + base64.RawURLEncoding.EncodeToString(hash[:])
}
func Challenge(v string) string {
	hash := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

type Registration struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
}
type Start struct {
	ServerID         string `json:"serverId"`
	Purpose          string `json:"purpose"`
	ClaimOperationID string `json:"claimOperationId,omitempty"`
	RegistrationID   string `json:"registrationId"`
	State            string `json:"state"`
	CodeChallenge    string `json:"codeChallenge"`
	ServerChallenge  string `json:"serverChallenge"`
	InstallationID   string `json:"installationId"`
}

func (q Start) Valid() bool {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.CodeChallenge)
	return identifier.MatchString(q.ServerID) && Purpose(q.Purpose) && (q.ClaimOperationID == "" || identifier.MatchString(q.ClaimOperationID)) && identifier.MatchString(q.RegistrationID) && secret.MatchString(q.State) && e == nil && len(raw) == 32 && secret.MatchString(q.ServerChallenge) && secret.MatchString(q.InstallationID)
}

type Complete struct{}

type Started struct {
	TransactionID string `json:"transactionId"`
}
type Return struct {
	Location string `json:"location"`
}
type Exchange struct {
	Code            string `json:"code"`
	TransactionID   string `json:"transactionId"`
	Verifier        string `json:"verifier"`
	RegistrationID  string `json:"registrationId"`
	ServerChallenge string `json:"serverChallenge"`
	InstallationID  string `json:"installationId"`
	Purpose         string `json:"purpose"`
}

func (q Exchange) Valid() bool {
	return secret.MatchString(q.Code) && identifier.MatchString(q.TransactionID) && secret.MatchString(q.Verifier) && identifier.MatchString(q.RegistrationID) && secret.MatchString(q.ServerChallenge) && secret.MatchString(q.InstallationID) && Purpose(q.Purpose)
}
func (Exchange) String() string     { return "[server return redacted]" }
func (q Exchange) GoString() string { return q.String() }

const ReturnPath = "/auth/portico-return"

// Assertion appends the immutable return tuple to genuine account identity.
// This separate kind cannot be used as an ordinary sign-in/custody assertion.
type Assertion struct {
	Kind                  string    `json:"kind"`
	AccountID             string    `json:"accountId"`
	Username              string    `json:"username"`
	ServerID              string    `json:"serverId"`
	Challenge             string    `json:"challenge"`
	Nonce                 string    `json:"nonce"`
	IssuedAt              time.Time `json:"issuedAt"`
	ExpiresAt             time.Time `json:"expiresAt"`
	Purpose               string    `json:"purpose"`
	InstallationID        string    `json:"installationId"`
	RegistrationID        string    `json:"registrationId"`
	ClaimOperationID      string    `json:"claimOperationId"`
	ClaimGeneration       int64     `json:"claimGeneration,string"`
	CredentialGeneration  int64     `json:"credentialGeneration,string"`
	CurrentOwnerAccountID string    `json:"currentOwnerAccountId"`
	PreviousOwnerRole     string    `json:"previousOwnerRole"`
}
type Grant struct {
	Assertion trust.Envelope `json:"assertion"`
}

// These are the schemas emitted by the actual producer generator. Purpose
// alternatives retain their discriminator constraints in the runtime decoder.
type startShape Start
type exchangeShape Exchange

func (Start) JSONSchema() map[string]any {
	shape, e := apikit.Schema(reflect.TypeFor[startShape]())
	if e != nil {
		panic(e)
	}
	props := shape["properties"].(map[string]any)
	for _, field := range []string{"state", "serverChallenge", "installationId"} {
		props[field] = map[string]any{"type": "string", "minLength": 32, "maxLength": 128, "pattern": "^[A-Za-z0-9_-]+$"}
	}
	props["codeChallenge"] = map[string]any{"type": "string", "minLength": 43, "maxLength": 43, "pattern": "^[A-Za-z0-9_-]+$"}
	variants := []any{}
	for _, purpose := range []string{"setup", "switch", "change-owner"} {
		variant := map[string]any{}
		for k, v := range shape {
			variant[k] = v
		}
		copiedProps := map[string]any{}
		for k, v := range props {
			copiedProps[k] = v
		}
		variant["properties"] = copiedProps
		variant["properties"].(map[string]any)["purpose"] = map[string]any{"type": "string", "const": purpose}
		variants = append(variants, variant)
	}
	return map[string]any{"oneOf": variants}
}
func (Exchange) JSONSchema() map[string]any {
	shape, e := apikit.Schema(reflect.TypeFor[exchangeShape]())
	if e != nil {
		panic(e)
	}
	props := shape["properties"].(map[string]any)
	props["purpose"] = map[string]any{"type": "string", "enum": []string{"setup", "switch", "change-owner"}}
	for _, field := range []string{"code", "verifier", "serverChallenge", "installationId"} {
		props[field] = map[string]any{"type": "string", "minLength": 32, "maxLength": 128, "pattern": "^[A-Za-z0-9_-]+$"}
	}
	return shape
}
