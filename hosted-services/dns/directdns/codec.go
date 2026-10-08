package directdns

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
)

var (
	ErrInvalidZone               = errors.New("invalid direct dns zone")
	ErrInvalidACMEDelegationZone = errors.New("invalid ACME delegation zone")
	ErrInvalidToken              = errors.New("invalid server token")
	ErrInvalidEndpoint           = errors.New("invalid endpoint name")
	ErrUnsafeAddress             = errors.New("unsafe endpoint address")
)

const acmeChallengePrefix = "_acme-challenge."

func ACMEChallengeTarget(name, directZone, delegationZone string) (string, error) {
	directZone = normalizeZone(directZone)
	delegationZone = normalizeZone(delegationZone)
	name = normalizeZone(name)
	if directZone == "" {
		return "", ErrInvalidZone
	}
	if delegationZone == "" || delegationZone == directZone || strings.HasSuffix(delegationZone, "."+directZone) {
		return "", ErrInvalidACMEDelegationZone
	}
	suffix := "." + directZone
	if !strings.HasPrefix(name, acmeChallengePrefix) || !strings.HasSuffix(name, suffix) {
		return "", ErrInvalidEndpoint
	}
	token := strings.TrimSuffix(strings.TrimPrefix(name, acmeChallengePrefix), suffix)
	if strings.Contains(token, ".") || !validServerToken(token) {
		return "", ErrInvalidToken
	}
	return token + "." + delegationZone, nil
}

func validateAnswerAddress(address netip.Addr) error {
	if !address.IsValid() || address.Is4In6() || address.Zone() != "" || address == netip.MustParseAddr("255.255.255.255") || address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsInterfaceLocalMulticast() {
		return ErrUnsafeAddress
	}
	return nil
}

func normalizeZone(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func validServerToken(value string) bool {
	if len(value) == 56 && strings.HasPrefix(value, "pca-") {
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(value[4:]))
		return err == nil && len(decoded) == 32 && "pca-"+strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(decoded)) == value
	}
	if len(value) != 24 || !strings.HasPrefix(value, "ptc-") {
		return false
	}
	for _, r := range value[len("ptc-"):] {
		if !((r >= 'a' && r <= 'z') || (r >= '2' && r <= '7')) {
			return false
		}
	}
	return true
}

// Deprecated: retained only to interpret pre-P02 diagnostics/tests. New allocation
// MUST use NewNamespace persisted under current claim authority.
// NamespaceFromServerID preserves all 256 bits of the current key-derived server
// identity. The base64url srv_ ID is case-sensitive; never lowercase before decode.
func NamespaceFromServerID(serverID string) (string, error) {
	if !strings.HasPrefix(serverID, "srv_") {
		return "", ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(serverID, "srv_"))
	if err != nil || len(raw) != 32 || "srv_"+base64.RawURLEncoding.EncodeToString(raw) != serverID {
		return "", ErrInvalidToken
	}
	return "pca-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}

// NewNamespace allocates 96 independent random bits, never derived from identity.
// Callers must durably reserve this value before returning it to a server.
func NewNamespace() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ptc-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])), nil
}

// ValidNamespace is stricter than the historical DNS interpreter: only new,
// canonical 96-bit namespaces can receive new certificates or DNS01 updates.
func ValidNamespace(s string) bool {
	if len(s) != 24 || !strings.HasPrefix(s, "ptc-") {
		return false
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(s[4:]))
	return err == nil && len(raw) == 12 && "ptc-"+strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)) == s
}
