package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"portico.local/dns/directdns"
)

var buildID = "development"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func run() error {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(buildID)
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] == "dns-readiness" {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://127.0.0.1:5354/ready")
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return errors.New("authenticated address synchronization is not ready")
		}
		var status struct {
			Ready bool `json:"ready"`
		}
		if json.NewDecoder(response.Body).Decode(&status) != nil || !status.Ready {
			return errors.New("invalid synchronization status")
		}
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] == "dns-healthcheck" {
		// The apex SOA needs no table row, so health means "this authority
		// answers". It runs before the address table is opened: opening it is
		// crash recovery (orphaned journals removed, a partial tail truncated),
		// which only the serving process may do.
		zone := env("DIRECT_DNS_ZONE", "direct.getportico.tv")
		for _, transport := range []string{"udp", "tcp"} {
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(zone), dns.TypeSOA)
			client := dns.Client{Net: transport, Timeout: 2 * time.Second}
			r, _, e := client.Exchange(m, "127.0.0.1:5353")
			if e != nil {
				return e
			}
			if r.Rcode != dns.RcodeSuccess || !r.Authoritative || len(r.Answer) != 1 {
				return errors.New("authority healthcheck failed")
			}
		}
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] != "dns" && os.Args[1] != "dns-rebuild" {
		return errors.New("unknown command")
	}
	tableFile := env("DIRECT_DNS_TABLE_FILE", "/var/lib/portico-dns/addresses.json")
	// One owner at a time: the serving process and a rebuild both recover and
	// rewrite the table, so a second one refuses to start rather than race it.
	lock, err := os.OpenFile(tableFile+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("address table lock: %w", err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("another portico-dns process owns %s: %w", tableFile, err)
	}
	// The address table is this node's whole state. It is loaded from disk before anything
	// else so a restart answers at once, whether or not Hosted can be reached.
	table, err := directdns.OpenTable(tableFile, os.Getenv("DIRECT_DNS_ALLOW_PRIVATE") == "true")
	if err != nil {
		return fmt.Errorf("address table: %w", err)
	}
	cfg := directdns.Config{Table: table, Zone: env("DIRECT_DNS_ZONE", "direct.getportico.tv"), ListenAddr: env("DIRECT_DNS_ADDR", "0.0.0.0:5353"), Nameservers: strings.Split(env("DIRECT_DNS_NAMESERVERS", "ns1.portico-dns.getportico.tv,ns2.portico-dns.getportico.tv"), ","), ACMEDelegationZone: env("ACME_DNS_ZONE", "acme.getportico.tv"), CAAIssuerDomain: env("DIRECT_DNS_CAA_ISSUER_DOMAIN", "pki.goog"), CAAAdditionalIssuers: strings.Split(os.Getenv("DIRECT_DNS_CAA_ADDITIONAL_ISSUER_DOMAINS"), ","), CAAAccountURI: os.Getenv("DIRECT_DNS_CAA_ACCOUNT_URI"), TrustedProxyCIDRs: strings.Split(os.Getenv("DIRECT_DNS_TRUSTED_PROXY_CIDRS"), ","), MaxConcurrentQueries: 128}
	server, err := directdns.New(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	fmt.Fprintln(os.Stderr, "portico-dns", buildID)
	feedURL, secretFile := os.Getenv("DIRECT_DNS_FEED_URL"), os.Getenv("DIRECT_DNS_FEED_SECRET_FILE")
	if feedURL == "" || secretFile == "" {
		return errors.New("DIRECT_DNS_FEED_URL and DIRECT_DNS_FEED_SECRET_FILE are required")
	}
	secret, err := os.ReadFile(secretFile)
	if err != nil || len(strings.TrimSpace(string(secret))) < 32 {
		return errors.New("invalid address feed secret")
	}
	fmt.Fprintln(os.Stderr, "address table:", table.Len(), "servers at sequence", table.Sequence())
	feed := &directdns.Feed{URL: feedURL, Secret: strings.TrimSpace(string(secret)), Table: table, Client: &http.Client{Timeout: 25 * time.Second}, Log: func(m string) { fmt.Fprintln(os.Stderr, m) }}
	if len(os.Args) > 1 && os.Args[1] == "dns-rebuild" {
		rebuild, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err = feed.Rebuild(rebuild); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(table.Summary())
	}
	readiness := &directdns.Readiness{}
	feed.Readiness = readiness
	listener, err := net.Listen("tcp", "127.0.0.1:5354")
	if err != nil {
		return err
	}
	statusMux := http.NewServeMux()
	statusMux.Handle("GET /ready", readiness)
	statusMux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) { readiness.Metrics(w, r, table) })
	status := &http.Server{Handler: statusMux, ReadHeaderTimeout: 2 * time.Second}
	defer status.Close()
	go func() {
		if err := status.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cancel()
		}
	}()
	go feed.Run(ctx)
	return server.ListenAndServe(ctx)
}
