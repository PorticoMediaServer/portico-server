package certificates

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"strings"

	"portico.local/dns/dns01"
	"portico.local/hosted/internal/security"
)

// Config is read only from an operator-protected file. It is not an HTTP model.
// Google staging EAB is distinct from production; there is no automatic fallback.
type Config struct {
	Provider         string   `json:"provider"`
	Environment      string   `json:"environment"`
	ContactEmail     string   `json:"contactEmail"`
	AcceptTerms      bool     `json:"acceptTerms"`
	EABKeyID         string   `json:"eabKeyId"`
	EABMAC           string   `json:"eabMacKey"`
	SealKeyFile      string   `json:"accountSealKeyFile"`
	StagingRootsFile string   `json:"stagingRootsFile"`
	DNSAddress       string   `json:"dnsUpdateAddress"`
	TSIGSecret       string   `json:"dnsUpdateSecret"`
	Resolvers        []string `json:"propagationResolvers"`
	Workers          int      `json:"workers"`
	// OrdersPerMinute caps new orders at the certificate authority across every Hosted
	// instance and worker. Keep it under the account's negotiated quota (25 a minute).
	OrdersPerMinute int `json:"ordersPerMinute"`
}

func (Config) String() string     { return "[ACME configuration redacted]" }
func (c Config) GoString() string { return c.String() }

func privateFile(path string, limit int64) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > limit {
		return nil, ErrConfiguration
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrConfiguration
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(st, actual) {
		return nil, ErrConfiguration
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrConfiguration
	}
	return b, nil
}
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, nil
	}
	raw, e := privateFile(path, 32<<10)
	if e != nil {
		return nil, e
	}
	defer clear(raw)
	var c Config
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return nil, ErrConfiguration
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return nil, ErrConfiguration
	}
	if c.Provider == "" {
		c.Provider = "google"
	}
	if c.Environment == "" {
		c.Environment = "production"
	}
	if c.Workers == 0 {
		c.Workers = 2
	}
	if c.OrdersPerMinute == 0 {
		c.OrdersPerMinute = 20
	}
	if c.OrdersPerMinute < 1 || c.OrdersPerMinute > 1000 {
		return nil, ErrConfiguration
	}
	if c.Workers < 1 || c.Workers > 8 || !c.AcceptTerms {
		return nil, ErrConfiguration
	}
	if _, e = c.directory(); e != nil {
		return nil, e
	}
	if c.Environment == "production" && c.StagingRootsFile != "" {
		return nil, ErrConfiguration
	}
	if c.Environment == "staging" && c.StagingRootsFile == "" {
		return nil, ErrConfiguration
	}
	if c.EABMAC != "" {
		if _, e = decodeMAC(c.EABMAC); e != nil {
			return nil, e
		}
	}
	if (c.EABKeyID == "") != (c.EABMAC == "") {
		return nil, ErrConfiguration
	}
	if c.ContactEmail != "" && (!strings.Contains(c.ContactEmail, "@") || strings.ContainsAny(c.ContactEmail, "\r\n <>")) {
		return nil, ErrConfiguration
	}
	if len(c.Resolvers) < 1 || len(c.Resolvers) > 4 {
		return nil, ErrConfiguration
	}
	for _, r := range c.Resolvers {
		host, port, e := net.SplitHostPort(r)
		if e != nil || net.ParseIP(host) == nil || port != "53" {
			return nil, ErrConfiguration
		}
	}
	// Validate transport shape without making a DNS request.
	if e := c.dns().Validate(); e != nil {
		return nil, ErrConfiguration
	}
	return &c, nil
}
func decodeMAC(s string) ([]byte, error) {
	b, e := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if e != nil || len(b) < 16 {
		return nil, ErrConfiguration
	}
	return b, nil
}
func (c Config) ID() string { return c.Provider + "-" + c.Environment }
func (c Config) directory() (string, error) {
	switch c.ID() {
	case "google-production":
		return "https://dv.acme-v02.api.pki.goog/directory", nil
	case "google-staging":
		return "https://dv.acme-v02.test-api.pki.goog/directory", nil
	case "letsencrypt-production":
		return "https://acme-v02.api.letsencrypt.org/directory", nil
	case "letsencrypt-staging":
		return "https://acme-staging-v02.api.letsencrypt.org/directory", nil
	}
	return "", ErrConfiguration
}
func (c Config) dns() dns01.Client {
	return dns01.Client{Address: c.DNSAddress, Zone: "acme.getportico.tv", TSIGName: "acme-update.", TSIGSecret: c.TSIGSecret}
}
func (c Config) issuerDomain() string {
	if c.Provider == "google" {
		return "pki.goog"
	}
	return "letsencrypt.org"
}
func (c Config) sealing() (*security.Keys, error) {
	b, e := privateFile(c.SealKeyFile, 256)
	if e != nil {
		return nil, e
	}
	defer clear(b)
	raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if e != nil || len(raw) != 32 {
		return nil, ErrConfiguration
	}
	return &security.Keys{SealKey: raw}, nil
}
func (c Config) roots() (*x509.CertPool, error) {
	if c.Environment == "production" {
		return nil, nil
	} // Go's system public roots.
	b, e := privateFile(c.StagingRootsFile, 128<<10)
	if e != nil {
		return nil, ErrConfiguration
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(b) {
		return nil, ErrConfiguration
	}
	return roots, nil
}
