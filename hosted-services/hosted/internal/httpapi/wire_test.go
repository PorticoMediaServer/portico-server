package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// The certificate routes moved from []byte fields to base64 strings so the
// contract can name them; the wire and the accepted inputs must not change.
func TestCertificateBytesKeepTheirEncodingJSONWire(t *testing.T) {
	for _, body := range []string{
		`{"requestId":"r","csr":"AAEC/w=="}`,
		`{"requestId":"r","csr":"AAEC\n/w=="}`,
		`{"requestId":"r","csr":"AAEC/w"}`,
		`{"requestId":"r","csr":"AAEC_w=="}`,
		`{"requestId":"r","csr":""}`,
		`{"requestId":"r","csr":null}`,
	} {
		var old struct {
			RequestID string `json:"requestId"`
			CSR       []byte `json:"csr"`
		}
		oldErr := json.Unmarshal([]byte(body), &old)
		var current CertificateOrderRequest
		if e := json.Unmarshal([]byte(body), &current); e != nil {
			t.Fatal(body, e)
		}
		csr, newErr := base64.StdEncoding.DecodeString(current.CSR)
		if (oldErr == nil) != (newErr == nil) || oldErr == nil && !bytes.Equal(old.CSR, csr) {
			t.Errorf("%s: before %v %x, now %v %x", body, oldErr, old.CSR, newErr, csr)
		}
	}
	chain := []byte("-----BEGIN CERTIFICATE-----\n\x00\xff")
	before, _ := json.Marshal(struct {
		Chain []byte `json:"chain"`
	}{chain})
	after, _ := json.Marshal(CertificateChain{Chain: base64.StdEncoding.EncodeToString(chain)})
	if !bytes.Equal(before, after) {
		t.Fatalf("%s != %s", before, after)
	}
}
