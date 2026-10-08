// Package dns01 restricts the privileged ACME DNS update transport to the exact
// durably allocated random namespace. Present requires current claim authority.
// Cleanup is deny-only: its durable exact-value ownership survives revocation.
package dns01

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"portico.local/dns/directdns"
)

type Client struct{ Address, Zone, TSIGName, TSIGSecret string }

func (c Client) change(ctx context.Context, namespace, value string, remove bool) error {
	if !directdns.ValidNamespace(namespace) {
		return directdns.ErrInvalidToken
	}
	ns := namespace
	if e := c.Validate(); e != nil {
		return e
	}
	raw, e := base64.RawURLEncoding.DecodeString(value)
	if e != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != value {
		return errors.New("invalid DNS01 digest")
	}
	rr := &dns.TXT{Hdr: dns.RR_Header{Name: ns + "." + c.Zone + ".", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 30}, Txt: []string{value}}
	m := new(dns.Msg)
	m.SetUpdate(dns.Fqdn(c.Zone))
	if remove {
		m.Remove([]dns.RR{rr})
	} else {
		m.Insert([]dns.RR{rr})
	}
	m.SetTsig(c.TSIGName, dns.HmacSHA256, 300, time.Now().Unix())
	dc := dns.Client{Net: "tcp", Timeout: 5 * time.Second, TsigSecret: map[string]string{c.TSIGName: c.TSIGSecret}}
	response, _, e := dc.ExchangeContext(ctx, m, c.Address)
	if e != nil {
		return errors.New("ACME DNS update transport failed")
	}
	if response == nil || response.Rcode != dns.RcodeSuccess || response.IsTsig() == nil || strings.ToLower(response.IsTsig().Hdr.Name) != c.TSIGName {
		return errors.New("ACME DNS update rejected or unsigned")
	}
	return nil
}
func (c Client) Present(ctx context.Context, namespace, value string) error {
	return c.change(ctx, namespace, value, false)
}

// Cleanup removes only this order's TXT value, preserving concurrent orders.
func (c Client) Cleanup(ctx context.Context, namespace, value string) error {
	return c.change(ctx, namespace, value, true)
}

func (c Client) Validate() error {
	// Config is intentionally fixed to the dedicated validation child zone.
	if c.Zone != "acme.getportico.tv" {
		return errors.New("invalid ACME zone")
	}
	host, port, e := net.SplitHostPort(c.Address)
	if e != nil {
		return errors.New("invalid update endpoint")
	}
	ip, e := netip.ParseAddr(host)
	if e != nil || !ip.IsPrivate() || ip.Is4In6() || ip.Zone() != "" {
		return errors.New("update endpoint must be private IP")
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return errors.New("invalid update port")
	}
	secret, e := base64.StdEncoding.DecodeString(c.TSIGSecret)
	if e != nil || len(secret) < 32 {
		return errors.New("invalid update credential")
	}
	if c.TSIGName != "acme-update." {
		return errors.New("invalid update key name")
	}
	return nil
}
