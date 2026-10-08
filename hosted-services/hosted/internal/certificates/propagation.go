package certificates

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/miekg/dns"
)

var errPropagation = errors.New("DNS propagation pending")

func queryDNS(ctx context.Context, resolver, name string, typ uint16) (*dns.Msg, error) {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), typ)
	c := dns.Client{Net: "udp", Timeout: 2 * time.Second}
	r, _, e := c.ExchangeContext(ctx, q, resolver)
	if e != nil || r == nil {
		return nil, errPropagation
	}
	if r.Truncated {
		c.Net = "tcp"
		r, _, e = c.ExchangeContext(ctx, q, resolver)
	}
	if e != nil || r == nil || r.Rcode != dns.RcodeSuccess || len(r.Answer) > 32 {
		return nil, errPropagation
	}
	return r, nil
}

// Every configured resolver must see the exact delegated target and our TXT.
// No sleeps while holding work/authority transactions; retries are persisted.
func (s *Service) propagated(ctx context.Context, c Challenge) error {
	alias := "_acme-challenge." + c.Namespace + ".direct.getportico.tv."
	target := c.Namespace + ".acme.getportico.tv."
	for _, resolver := range s.cfg.Resolvers {
		r, e := queryDNS(ctx, resolver, alias, dns.TypeCNAME)
		if e != nil {
			return e
		}
		found := false
		for _, rr := range r.Answer {
			if cn, ok := rr.(*dns.CNAME); ok && strings.EqualFold(cn.Hdr.Name, alias) && strings.EqualFold(cn.Target, target) {
				found = true
			}
		}
		if !found {
			return errPropagation
		}
		r, e = queryDNS(ctx, resolver, target, dns.TypeTXT)
		if e != nil {
			return e
		}
		found = false
		for _, rr := range r.Answer {
			if txt, ok := rr.(*dns.TXT); ok && strings.EqualFold(txt.Hdr.Name, target) && strings.Join(txt.Txt, "") == c.Value {
				found = true
			}
		}
		if !found {
			return errPropagation
		}
	}
	return nil
}
func (s *Service) checkDNSPolicy(ctx context.Context, namespace string) error {
	// A namespace is an empty non-terminal in the stateless authority. CAA lives
	// at direct.getportico.tv and must select the same issuer as the coordinator.
	for _, resolver := range s.cfg.Resolvers {
		r, e := queryDNS(ctx, resolver, "direct.getportico.tv", dns.TypeCAA)
		if e != nil {
			return e
		}
		issue, wild := false, false
		for _, rr := range r.Answer {
			caa, ok := rr.(*dns.CAA)
			if !ok {
				continue
			}
			if !strings.EqualFold(caa.Hdr.Name, "direct.getportico.tv.") {
				return ErrConfiguration
			}
			if caa.Tag != "issue" && caa.Tag != "issuewild" {
				if caa.Flag&128 != 0 {
					return ErrConfiguration
				}
				continue
			}
			fields := strings.Split(caa.Value, ";")
			if strings.TrimSpace(fields[0]) != s.cfg.issuerDomain() {
				return ErrConfiguration
			}
			for _, field := range fields[1:] {
				key, value, ok := strings.Cut(strings.TrimSpace(field), "=")
				if !ok {
					return ErrConfiguration
				}
				switch key {
				case "validationmethods":
					if value != "dns-01" {
						return ErrConfiguration
					}
				case "accounturi":
					if s.client == nil || value != string(s.client.KID) {
						return ErrConfiguration
					}
				default:
					return ErrConfiguration
				}
			}
			if caa.Tag == "issue" {
				issue = true
			} else {
				wild = true
			}
		}
		if !issue || !wild {
			return ErrConfiguration
		}
	}
	return nil
}
