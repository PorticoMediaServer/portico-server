package certificates

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"time"

	"portico.local/dns/directdns"
)

var sanOID = asn1.ObjectIdentifier{2, 5, 29, 17}

func ValidateCSR(der []byte, namespace string) (*x509.CertificateRequest, error) {
	if !directdns.ValidNamespace(namespace) || len(der) < 100 || len(der) > 8192 {
		return nil, ErrInvalid
	}
	csr, e := x509.ParseCertificateRequest(der)
	if e != nil || csr.CheckSignature() != nil {
		return nil, ErrInvalid
	}
	key, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() || !key.Curve.IsOnCurve(key.X, key.Y) {
		return nil, ErrInvalid
	}
	want := "*." + namespace + ".direct.getportico.tv"
	if len(csr.DNSNames) != 1 || csr.DNSNames[0] != want || len(csr.IPAddresses) != 0 || len(csr.EmailAddresses) != 0 || len(csr.URIs) != 0 || (csr.Subject.CommonName != "" && csr.Subject.CommonName != want) {
		return nil, ErrInvalid
	}
	if len(csr.Extensions) != 1 || !exactWildcardSAN(csr.Extensions, want) {
		return nil, ErrInvalid
	}
	return csr, nil
}
func ValidateChain(chain []byte, csrDER []byte, namespace string, roots *x509.CertPool, now time.Time) (*x509.Certificate, error) {
	csr, e := ValidateCSR(csrDER, namespace)
	if e != nil {
		return nil, e
	}
	if len(chain) == 0 || len(chain) > 64<<10 {
		return nil, ErrInvalid
	}
	var certs []*x509.Certificate
	for len(bytes.TrimSpace(chain)) > 0 {
		chain = bytes.TrimSpace(chain)
		if !bytes.HasPrefix(chain, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, ErrInvalid
		}
		block, rest := pem.Decode(chain)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 || len(certs) >= 8 {
			return nil, ErrInvalid
		}
		c, e := x509.ParseCertificate(block.Bytes)
		if e != nil {
			return nil, ErrInvalid
		}
		certs = append(certs, c)
		chain = rest
	}
	if len(certs) == 0 {
		return nil, ErrInvalid
	}
	leaf := certs[0]
	if leaf.IsCA || !exactWildcardSAN(leaf.Extensions, csr.DNSNames[0]) || !bytes.Equal(leaf.RawSubjectPublicKeyInfo, csr.RawSubjectPublicKeyInfo) || len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != csr.DNSNames[0] || len(leaf.IPAddresses) != 0 || len(leaf.EmailAddresses) != 0 || len(leaf.URIs) != 0 || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, ErrInvalid
	}
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}
	_, e = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now, DNSName: "probe." + namespace + ".direct.getportico.tv", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if e != nil {
		return nil, ErrInvalid
	}
	return leaf, nil
}

func exactWildcardSAN(extensions []pkix.Extension, name string) bool {
	want, e := asn1.Marshal([]asn1.RawValue{{Class: 2, Tag: 2, Bytes: []byte(name)}})
	if e != nil {
		return false
	}
	count := 0
	for _, extension := range extensions {
		if extension.Id.Equal(sanOID) {
			count++
			if !bytes.Equal(want, extension.Value) {
				return false
			}
		}
	}
	return count == 1
}

// validateReceiptChain is exclusively for provider-receipt reconciliation and
// deny-only cleanup. An expired leaf can still prove CSR ownership. Normal
// issuance, download, local installation and handshakes always use current time.
func validateReceiptChain(chain, csr []byte, namespace string, roots *x509.CertPool, now time.Time) (*x509.Certificate, error) {
	if len(chain) == 0 || len(chain) > 64<<10 {
		return nil, ErrInvalid
	}
	first, _ := pem.Decode(bytes.TrimSpace(chain))
	if first == nil || first.Type != "CERTIFICATE" {
		return nil, ErrInvalid
	}
	leaf, err := x509.ParseCertificate(first.Bytes)
	if err != nil || !leaf.NotBefore.Before(leaf.NotAfter) || now.Before(leaf.NotBefore) {
		return nil, ErrInvalid
	}
	at := now
	if !now.Before(leaf.NotAfter) {
		at = leaf.NotAfter.Add(-time.Nanosecond)
	}
	return ValidateChain(chain, csr, namespace, roots, at)
}
