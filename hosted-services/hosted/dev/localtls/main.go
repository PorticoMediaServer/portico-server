// Command localtls puts a local development Hosted behind HTTPS on loopback, so
// a development Portico server (built with -tags devtrust, given the printed
// certificate as PORTICO_DEV_HOSTED_CA_FILE) can claim itself to it: server
// claims require an https Hosted origin. It makes a self-signed certificate for
// 127.0.0.1 and localhost in -dir on first run and reuses it after.
//
//	go run ./dev/localtls -dir <scratch dir> -listen 127.0.0.1:19443 -upstream http://127.0.0.1:19410
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("dir", "", "directory for cert.pem and key.pem")
	listen := flag.String("listen", "127.0.0.1:19443", "loopback HTTPS address")
	upstream := flag.String("upstream", "http://127.0.0.1:19410", "local Hosted API origin")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if *dir == "" || err != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("need -dir and a loopback -listen address")
	}
	certFile, keyFile := filepath.Join(*dir, "cert.pem"), filepath.Join(*dir, "key.pem")
	if _, err := os.Stat(certFile); err != nil {
		if err = makeCertificate(certFile, keyFile); err != nil {
			log.Fatal(err)
		}
	}
	target, err := url.Parse(*upstream)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("certificate:", certFile)
	server := &http.Server{Addr: *listen, Handler: httputil.NewSingleHostReverseProxy(target), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServeTLS(certFile, keyFile))
}

func makeCertificate(certFile, keyFile string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "Portico local Hosted"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, DNSNames: []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
}
