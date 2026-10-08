package directdns

import (
	"context"
	"errors"
	"hash/maphash"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type Config struct {
	Zone                 string
	ListenAddr           string
	Nameservers          []string
	TTL                  uint32
	ACMEDelegationZone   string
	CAAIssuerDomain      string
	CAAAdditionalIssuers []string
	CAAAccountURI        string
	// Table answers each server's permanent name; nil answers none.
	Table                *Table
	MaxConcurrentQueries int
	QueryLimitPerMinute  int
	// TrustedProxyCIDRs identifies private DNS frontends such as dnsdist. A
	// query from one of these peers has already passed the frontend's client-IP
	// and global admission policy, so applying this backend's source limiter to
	// the proxy address would accidentally create one fleet-wide bucket.
	TrustedProxyCIDRs []string
}

type Server struct {
	cfg            Config
	querySlots     chan struct{}
	rateShards     [rateShardCount]queryRateShard
	rateSeed       maphash.Seed
	trustedProxies []netip.Prefix
}

type queryRateState struct {
	windowStarted time.Time
	count         int
}

const maxQueryRateKeys = 4096
const rateShardCount = 64
const rateShardCapacity = (maxQueryRateKeys + rateShardCount - 1) / rateShardCount

type queryRateShard struct {
	mu     sync.Mutex
	states map[string]queryRateState
	ring   []string
	index  map[string]int
	cursor int
}

const googlePublicCAIssuerDomain = "pki.goog"

func certificateAuthorityCAAValue(issuerDomain, accountURI string) string {
	issuerDomain = strings.ToLower(strings.TrimSpace(issuerDomain))
	accountURI = strings.TrimSpace(accountURI)
	if issuerDomain == "pki.goog" {
		return issuerDomain
	}
	if accountURI == "" {
		return issuerDomain + "; validationmethods=dns-01"
	}
	return issuerDomain + "; accounturi=" + accountURI + "; validationmethods=dns-01"
}

func New(cfg Config) (*Server, error) {
	cfg.Zone = normalizeZone(cfg.Zone)
	if cfg.Zone == "" {
		return nil, ErrInvalidZone
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":5353"
	}
	if cfg.TTL == 0 {
		cfg.TTL = 60
	}
	cfg.ACMEDelegationZone = normalizeZone(cfg.ACMEDelegationZone)
	if cfg.ACMEDelegationZone == "" || cfg.ACMEDelegationZone == cfg.Zone || strings.HasSuffix(cfg.ACMEDelegationZone, "."+cfg.Zone) {
		return nil, ErrInvalidACMEDelegationZone
	}
	cfg.CAAIssuerDomain = strings.ToLower(strings.TrimSpace(cfg.CAAIssuerDomain))
	if cfg.CAAIssuerDomain == "" {
		cfg.CAAIssuerDomain = googlePublicCAIssuerDomain
	}
	if strings.ContainsAny(cfg.CAAIssuerDomain, ";\r\n ") || strings.Contains(cfg.CAAIssuerDomain, "/") || !strings.Contains(cfg.CAAIssuerDomain, ".") {
		return nil, errors.New("CAA issuer domain must be a bare DNS issuer domain")
	}
	seenCAAIssuers := map[string]struct{}{cfg.CAAIssuerDomain: {}}
	additionalIssuers := make([]string, 0, len(cfg.CAAAdditionalIssuers))
	for _, issuer := range cfg.CAAAdditionalIssuers {
		issuer = strings.ToLower(strings.TrimSpace(issuer))
		if issuer == "" {
			continue
		}
		if strings.ContainsAny(issuer, ";\r\n ") || strings.Contains(issuer, "/") || !strings.Contains(issuer, ".") {
			return nil, errors.New("additional CAA issuer must be a bare DNS issuer domain")
		}
		if _, exists := seenCAAIssuers[issuer]; exists {
			continue
		}
		seenCAAIssuers[issuer] = struct{}{}
		additionalIssuers = append(additionalIssuers, issuer)
	}
	// A fallback is an operator-selected issuer change, not simultaneous
	// permission for several CAs. This also prevents accidental stale CAA rollout.
	if len(additionalIssuers) != 0 {
		return nil, errors.New("select one CAA issuer; additional issuers are not allowed")
	}
	if cfg.CAAIssuerDomain != "pki.goog" && cfg.CAAIssuerDomain != "letsencrypt.org" {
		return nil, errors.New("unsupported CAA issuer")
	}
	cfg.CAAAdditionalIssuers = additionalIssuers
	cfg.CAAAccountURI = strings.TrimSpace(cfg.CAAAccountURI)
	if cfg.CAAIssuerDomain == "pki.goog" && cfg.CAAAccountURI != "" {
		return nil, errors.New("Google CAA account URI pinning is not supported by this configuration")
	}
	if cfg.CAAAccountURI != "" && (!strings.HasPrefix(cfg.CAAAccountURI, "https://") || strings.ContainsAny(cfg.CAAAccountURI, ";\r\n")) {
		return nil, errors.New("CAA account URI must be an HTTPS URI without CAA separators")
	}
	if cfg.CAAAccountURI != "" && cfg.CAAIssuerDomain != "letsencrypt.org" {
		return nil, errors.New("CAA account URI pinning is only supported for the Let's Encrypt issuer")
	}
	if cfg.MaxConcurrentQueries <= 0 {
		cfg.MaxConcurrentQueries = 256
	}
	if cfg.QueryLimitPerMinute <= 0 {
		// Queries arrive from recursive resolvers, not end-user IPs. A low
		// per-source cap incorrectly denies large shared resolvers. Concurrency
		// remains the primary hard overload boundary; this is only a coarse
		// abusive-source circuit breaker.
		cfg.QueryLimitPerMinute = 60000
	}
	if len(cfg.Nameservers) == 0 {
		cfg.Nameservers = []string{"ns1." + cfg.Zone}
	}
	for i, ns := range cfg.Nameservers {
		cfg.Nameservers[i] = dns.Fqdn(normalizeZone(ns))
	}
	trustedProxies := make([]netip.Prefix, 0, len(cfg.TrustedProxyCIDRs))
	for _, raw := range cfg.TrustedProxyCIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, errors.New("trusted DNS proxy CIDR is invalid")
		}
		trustedProxies = append(trustedProxies, prefix.Masked())
	}
	server := &Server{
		cfg:            cfg,
		querySlots:     make(chan struct{}, cfg.MaxConcurrentQueries),
		rateSeed:       maphash.MakeSeed(),
		trustedProxies: trustedProxies,
	}
	for index := range server.rateShards {
		server.rateShards[index] = queryRateShard{
			states: make(map[string]queryRateState, rateShardCapacity),
			ring:   make([]string, rateShardCapacity),
			index:  make(map[string]int, rateShardCapacity),
		}
	}
	return server, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(dns.Fqdn(s.cfg.Zone), s.handleDNS)
	udp := &dns.Server{Addr: s.cfg.ListenAddr, Net: "udp", Handler: mux, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MsgAcceptFunc: acceptDNSMessage}
	tcp := &dns.Server{Addr: s.cfg.ListenAddr, Net: "tcp", Handler: mux, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MsgAcceptFunc: acceptDNSMessage}
	// Bind both transports before starting either serve loop. Starting the
	// miekg/dns ListenAndServe calls concurrently lets TCP report a bind error
	// while UDP is still between goroutine scheduling and its own bind; a
	// subsequent ShutdownContext then sees UDP as not started and can leave it
	// to bind after the paired service has already returned. Pre-binding makes
	// the TCP/UDP pair one atomic startup boundary: a failed second bind closes
	// the first socket and no DNS loop has begun serving.
	tcpListener, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	udpPacketConn, err := net.ListenPacket("udp", s.cfg.ListenAddr)
	if err != nil {
		_ = tcpListener.Close()
		return err
	}
	tcp.Listener = tcpListener
	udp.PacketConn = udpPacketConn
	errCh := make(chan error, 2)
	go func() { errCh <- udp.ActivateAndServe() }()
	go func() { errCh <- tcp.ActivateAndServe() }()
	remaining := 2
	var serveErr error
	select {
	case err := <-errCh:
		remaining--
		if !benignDNSServerError(err) {
			serveErr = err
		}
	case <-ctx.Done():
	}
	// UDP and TCP are one service boundary. If either listener fails (including
	// during initial bind), close both before returning so a half-running DNS
	// authority cannot retain the port or serve an incomplete transport set.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// ShutdownContext is responsible for active DNS connections, but it returns
	// immediately when ActivateAndServe has not reached its started state yet.
	// Run both shutdowns concurrently and close the pre-bound sockets as well;
	// the raw close covers that startup window and forces any late serve call to
	// fail instead of retaining a listener after this method returns.
	udpShutdownDone := make(chan struct{})
	tcpShutdownDone := make(chan struct{})
	go func() {
		_ = udp.ShutdownContext(shutdownCtx)
		close(udpShutdownDone)
	}()
	go func() {
		_ = tcp.ShutdownContext(shutdownCtx)
		close(tcpShutdownDone)
	}()
	_ = udpPacketConn.Close()
	_ = tcpListener.Close()
	<-udpShutdownDone
	<-tcpShutdownDone
	cancel()
	for remaining > 0 {
		err := <-errCh
		remaining--
		if serveErr == nil && !benignDNSServerError(err) {
			serveErr = err
		}
	}
	return serveErr
}

func benignDNSServerError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "server closed") || strings.Contains(lower, "use of closed network connection")
}

func acceptDNSMessage(header dns.Header) dns.MsgAcceptAction {
	opcode := int(header.Bits>>11) & 0xF
	if opcode != dns.OpcodeUpdate {
		return dns.DefaultMsgAcceptFunc(header)
	}
	if header.Bits&(1<<15) != 0 {
		return dns.MsgIgnore
	}
	if header.Qdcount != 1 || header.Ancount != 0 || header.Nscount > 128 || header.Arcount > 2 {
		return dns.MsgReject
	}
	return dns.MsgAccept
}

func (s *Server) handleDNS(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	if req.Opcode != dns.OpcodeQuery {
		resp.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(resp)
		return
	}
	if !s.trustedProxy(w.RemoteAddr()) && !s.allowQuery(w.RemoteAddr(), time.Now().UTC()) {
		resp.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(resp)
		return
	}
	select {
	case s.querySlots <- struct{}{}:
		defer func() { <-s.querySlots }()
	default:
		resp.Rcode = dns.RcodeServerFailure
		_ = w.WriteMsg(resp)
		return
	}
	if len(req.Question) != 1 {
		resp.Rcode = dns.RcodeFormatError
		_ = w.WriteMsg(resp)
		return
	}
	q := req.Question[0]
	name := normalizeZone(q.Name)
	switch {
	case name == s.cfg.Zone:
		s.answerApex(resp, q)
	case strings.HasPrefix(name, acmeChallengePrefix) && strings.HasSuffix(name, "."+s.cfg.Zone):
		s.answerACMEDelegation(resp, q)
	case strings.HasSuffix(name, "."+s.cfg.Zone):
		s.answerDirectHost(resp, q)
	default:
		resp.Rcode = dns.RcodeNameError
	}
	if len(resp.Answer) == 0 && resp.Rcode == dns.RcodeSuccess {
		resp.Ns = append(resp.Ns, s.soa())
	}
	_ = w.WriteMsg(resp)
}

func (s *Server) trustedProxy(remote net.Addr) bool {
	if remote == nil || len(s.trustedProxies) == 0 {
		return false
	}
	host := strings.TrimSpace(remote.String())
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	address, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}
	address = address.Unmap()
	for _, prefix := range s.trustedProxies {
		candidate := prefix
		if address.Is4() && prefix.Addr().Is4In6() {
			candidate = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		if candidate.Contains(address) {
			return true
		}
	}
	return false
}

func (s *Server) answerACMEDelegation(resp *dns.Msg, q dns.Question) {
	target, err := ACMEChallengeTarget(q.Name, s.cfg.Zone, s.cfg.ACMEDelegationZone)
	if err != nil {
		resp.Rcode = dns.RcodeNameError
		return
	}
	if q.Qtype != dns.TypeTXT && q.Qtype != dns.TypeCNAME && q.Qtype != dns.TypeANY {
		return
	}
	resp.Answer = append(resp.Answer, &dns.CNAME{
		Hdr:    dns.RR_Header{Name: dns.Fqdn(normalizeZone(q.Name)), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: s.cfg.TTL},
		Target: dns.Fqdn(target),
	})
}

func (s *Server) allowQuery(remote net.Addr, now time.Time) bool {
	rawKey := "unknown"
	if remote != nil {
		rawKey = strings.TrimSpace(remote.String())
		if host, _, err := net.SplitHostPort(rawKey); err == nil {
			rawKey = host
		}
	}
	hash := maphash.String(s.rateSeed, rawKey)
	key := strconv.FormatUint(hash, 16)
	shard := &s.rateShards[int(hash%rateShardCount)]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	state, exists := shard.states[key]
	if !exists || now.Sub(state.windowStarted) >= time.Minute {
		state = queryRateState{windowStarted: now}
	}
	if state.count >= s.cfg.QueryLimitPerMinute {
		shard.states[key] = state
		return false
	}
	if !exists {
		// Each shard owns a fixed ring. A spoofed-source flood can evict one
		// coarse bucket in O(1), never causing a global O(n) scan.
		if len(shard.states) >= rateShardCapacity {
			evictKey := shard.ring[shard.cursor]
			if evictKey != "" {
				delete(shard.states, evictKey)
				delete(shard.index, evictKey)
			}
		}
		shard.ring[shard.cursor] = key
		shard.index[key] = shard.cursor
		shard.cursor = (shard.cursor + 1) % rateShardCapacity
	}
	state.count++
	shard.states[key] = state
	return true
}

func (s *Server) answerApex(resp *dns.Msg, q dns.Question) {
	switch q.Qtype {
	case dns.TypeSOA:
		resp.Answer = append(resp.Answer, s.soa())
	case dns.TypeNS:
		for _, ns := range s.cfg.Nameservers {
			resp.Answer = append(resp.Answer, &dns.NS{Hdr: dns.RR_Header{Name: dns.Fqdn(s.cfg.Zone), Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: s.cfg.TTL}, Ns: ns})
		}
	case dns.TypeCAA:
		issuers := append([]string{s.cfg.CAAIssuerDomain}, s.cfg.CAAAdditionalIssuers...)
		for _, issuer := range issuers {
			accountURI := ""
			if issuer == s.cfg.CAAIssuerDomain {
				accountURI = s.cfg.CAAAccountURI
			}
			for _, tag := range []string{"issue", "issuewild"} {
				resp.Answer = append(resp.Answer, &dns.CAA{
					Hdr:   dns.RR_Header{Name: dns.Fqdn(s.cfg.Zone), Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: s.cfg.TTL},
					Flag:  0,
					Tag:   tag,
					Value: certificateAuthorityCAAValue(issuer, accountURI),
				})
			}
		}
	default:
		resp.Ns = append(resp.Ns, s.soa())
	}
}

// answerDirectHost answers a server's permanent name from the address table. Anything else
// under the zone does not exist: the zone can only ever resolve to a registered server's
// current address, so it is no use to anyone wanting a name for an arbitrary address.
func (s *Server) answerDirectHost(resp *dns.Msg, q dns.Question) {
	namespace, ok := NamespaceFromCurrentHostname(q.Name, s.cfg.Zone)
	if !ok || s.cfg.Table == nil {
		resp.Rcode = dns.RcodeNameError
		return
	}
	addresses, ok := s.cfg.Table.Lookup(namespace)
	name := normalizeZone(q.Name)
	if ok {
		previous := ""
		if addresses.previousValid(time.Now()) {
			previous = addresses.PreviousLabel
		}
		switch {
		case name == CurrentHostname(namespace, s.cfg.Zone, addresses.Label):
		case name == LANHostname(namespace, s.cfg.Zone, addresses.Label):
			addresses.IPv4, addresses.IPv6 = addresses.LANIPv4, addresses.LANIPv6
		case previous != "" && name == CurrentHostname(namespace, s.cfg.Zone, previous):
		case previous != "" && name == LANHostname(namespace, s.cfg.Zone, previous):
			addresses.IPv4, addresses.IPv6 = addresses.LANIPv4, addresses.LANIPv6
		default:
			ok = false
		}
	}
	if !ok {
		resp.Rcode = dns.RcodeNameError
		return
	}
	header := dns.RR_Header{Name: dns.Fqdn(normalizeZone(q.Name)), Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: s.cfg.TTL}
	switch {
	case q.Qtype == dns.TypeA && addresses.IPv4.IsValid():
		resp.Answer = append(resp.Answer, &dns.A{Hdr: header, A: net.IP(addresses.IPv4.AsSlice())})
	case q.Qtype == dns.TypeAAAA && addresses.IPv6.IsValid():
		resp.Answer = append(resp.Answer, &dns.AAAA{Hdr: header, AAAA: net.IP(addresses.IPv6.AsSlice())})
	}
}

func (s *Server) soa() dns.RR {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: dns.Fqdn(s.cfg.Zone), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: s.cfg.TTL},
		Ns:      s.cfg.Nameservers[0],
		Mbox:    "hostmaster." + dns.Fqdn(s.cfg.Zone),
		Serial:  uint32(time.Now().UTC().Unix() / 60),
		Refresh: 600,
		Retry:   300,
		Expire:  86400,
		Minttl:  60,
	}
}
