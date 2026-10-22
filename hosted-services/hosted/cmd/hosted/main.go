package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"portico.local/hosted/internal/appstore"
	"portico.local/hosted/internal/buildinfo"
	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
	"portico.local/hosted/internal/wake"
	"portico.local/hosted/trust"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "keys" {
		if err := security.KeysCLI(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Info())
		return
	}
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "hosted startup/runtime failure:", e)
		os.Exit(1)
	}
}
func run() error {
	bind := flag.String("listen", "127.0.0.1:19410", "Loopback listen address; publish only through an HTTPS reverse proxy")
	rootPath := flag.String("root-public-key-file", "", "Explicit private-Hosted override of the compiled public root")
	keyPath := flag.String("key-file", "runtime/hosted/keys.json", "Private persisted signing/sealing key file")
	databaseFile := flag.String("database-url-file", "runtime/postgres/database-url", "Private PostgreSQL URL file")
	origins := flag.String("origins", "http://127.0.0.1:19412,http://localhost:19412,http://127.0.0.1:19411", "Explicit browser origin allowlist")
	publicOnly := flag.Bool("print-public-key", false, "Print public signing key config and exit")
	allowLoopback := flag.Bool("allow-loopback-enrollment", false, "Development only: allow IP-loopback HTTP device activation")
	verificationURI := flag.String("device-verification-uri", "https://web.getportico.tv/device", "Fixed browser activation URL; never supplied by requesting devices")
	trustedProxyCIDRs := flag.String("trusted-proxy-cidrs", "", "Explicit loopback-only proxy CIDRs; trust exactly one sanitized X-Forwarded-For IP from these peers")
	accessLifetime := flag.Duration("access-token-lifetime", 12*time.Hour, "Access credential lifetime; switch to15m after clients integrate refresh (5m..12h)")
	claimAudience := flag.String("claim-audience", "", "Required exact public HTTPS origin for current server claims")
	tvIssuer := flag.String("tv-issuer", "", "Origin TV sign-in grants name as their issuer, the one TVs talk to (default: -claim-audience; a loopback http origin needs -allow-loopback-enrollment)")
	restoreDrill := flag.Bool("restore-drill", false, "Serve a restored copy for a drill: no outbound or queue workers (no wakes, ACME, mail, exports, alerts or pruning)")
	httpMailFile := flag.String("http-mail-config-file", "", "Private generic HTTP transactional mail gateway configuration")
	smtpFile := flag.String("smtp-config-file", "", "Private account SMTP configuration; verified TLS is required")
	oidcFile := flag.String("oidc-config-file", "", "Private provider configuration")
	dnsKeyFile := flag.String("direct-dns-key-file", "", "Protected secret the DNS nodes present to read the address feed (base64, 32+ bytes)")
	appStoreFile := flag.String("appstore-config-file", "", "Private App Store Server Notifications configuration; absent leaves the intake endpoint unregistered")
	acmeFile := flag.String("acme-config-file", "", "Private Google Public CA / explicitly selected ACME issuer configuration")
	retiredFiles := flag.String("acme-retired-config-files", "", "Comma-separated private former issuer configs for cleanup-only workers during explicit issuer/environment transitions")
	reconcileID := flag.String("acme-reconcile-order", "", "Operator-only ambiguous local order receipt to reconcile, then exit")
	reconcileURL := flag.String("acme-order-url", "", "Exact provider order URL recovered using the configured ACME account")
	operatorAccount := flag.String("operator-account-id", "", "Portico Account whose in-app notifications warn 30 days before the signing certificate expires")
	migrateDryRun := flag.Bool("migrate-dry-run", false, "Run the schema migration against the database and roll it back, then exit; needs no keys")
	metricsBind := flag.String("metrics-listen", "127.0.0.1:19530", "Loopback address for /metrics and CT observations")
	metricsTokenFile := flag.String("metrics-token-file", "", "Private bearer token file that authorizes POST /ct-observations; unset disables it")
	flag.Parse()
	if *accessLifetime < 5*time.Minute || *accessLifetime > 12*time.Hour {
		return fmt.Errorf("access-token-lifetime must be between5m and12h")
	}
	if *migrateDryRun {
		raw, err := os.ReadFile(*databaseFile)
		if err != nil {
			return fmt.Errorf("read database URL file: %w", err)
		}
		store, err := postgres.Open(context.Background(), strings.TrimSpace(string(raw)))
		if err != nil {
			return fmt.Errorf("database unavailable")
		}
		defer store.DB.Close()
		before, after, err := store.MigrateDryRun(context.Background())
		if err != nil {
			return fmt.Errorf("migration dry run failed at schema version %d: %w", before, err)
		}
		fmt.Printf("migration dry run: schema version %d -> %d (rolled back)\n", before, after)
		return nil
	}
	root, e := trust.DefaultRoot()
	if e != nil {
		return e
	}
	if *rootPath != "" {
		raw, err := os.ReadFile(*rootPath)
		if err != nil {
			return err
		}
		root, e = trust.ParsePublicPEM(raw)
		if e != nil {
			return e
		}
	}
	if trust.IsDevelopmentRoot(root) && (productionHostedAddress(*claimAudience) || productionHostedAddress(*tvIssuer) || productionHostedAddress(*verificationURI)) {
		return errors.New("development root cannot use production Hosted origin")
	}
	keys, e := security.LoadKeysWithRoot(*keyPath, root)
	if e != nil {
		return e
	}
	if *publicOnly {
		fmt.Printf("PORTICO_HOSTED_PUBLIC_KEY=%s\nPORTICO_HOSTED_KEY_ID=%s\n", keys.PublicKey(), keys.KeyID)
		return nil
	}
	host, _, e := net.SplitHostPort(*bind)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("hosted listener must be explicit loopback behind the HTTPS edge")
	}
	// Configuration is checked before the database is migrated: a release that
	// can't start must fail while the previous binary can still run the schema.
	acmeConfig, e := certificates.LoadConfig(*acmeFile)
	if e != nil {
		return fmt.Errorf("invalid ACME configuration")
	}
	raw, e := os.ReadFile(*databaseFile)
	if e != nil {
		return fmt.Errorf("read database URL file: %w", e)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, e := postgres.Open(ctx, strings.TrimSpace(string(raw)))
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer store.DB.Close()
	if e = store.Migrate(ctx); e != nil {
		return fmt.Errorf("database migration failed: %w", e)
	}
	if !*restoreDrill {
		if e = store.RecordSigningCertificate(ctx, keys); e != nil {
			return fmt.Errorf("signing certificate rollout refused: %w", e)
		}
	}
	certificateService, e := certificates.New(store, acmeConfig)
	if e != nil {
		return fmt.Errorf("ACME account custody configuration failed")
	}
	if *reconcileID != "" || *reconcileURL != "" {
		if *reconcileID == "" || *reconcileURL == "" {
			return fmt.Errorf("both reconciliation flags are required")
		}
		reconcileCtx, done := context.WithTimeout(ctx, 45*time.Second)
		defer done()
		candidate, err := store.ReconciliationCandidate(reconcileCtx, *reconcileID)
		if err != nil {
			return fmt.Errorf("reconciliation candidate unavailable")
		}
		if err = certificateService.Reconcile(reconcileCtx, candidate, *reconcileURL); err != nil {
			return fmt.Errorf("provider receipt reconciliation failed")
		}
		fmt.Println("Provider receipt attached; workers will resume under current authority or perform deny-only cleanup.")
		return nil
	}

	// Validate all required issuer custody before serving requests. Removing a
	// config does not erase an account or turn unresolved orders into new issuance.
	issuerConfigs := map[string]bool{}
	workerCount := 0
	if acmeConfig != nil {
		issuerConfigs[acmeConfig.ID()] = true
		workerCount = acmeConfig.Workers
	}
	var retiredServices []*certificates.Service
	if *retiredFiles != "" {
		for _, path := range strings.Split(*retiredFiles, ",") {
			cfg, err := certificates.LoadConfig(strings.TrimSpace(path))
			if err != nil || cfg == nil || issuerConfigs[cfg.ID()] {
				return fmt.Errorf("invalid or duplicate retired ACME configuration")
			}
			issuerConfigs[cfg.ID()] = true
			workerCount += cfg.Workers
			retired, err := certificates.NewCleanupOnly(store, cfg)
			if err != nil {
				return fmt.Errorf("retired ACME account custody unavailable")
			}
			retiredServices = append(retiredServices, retired)
		}
	}
	if workerCount > 8 {
		return fmt.Errorf("total active and cleanup ACME workers must not exceed eight")
	}
	requiredIssuers, err := store.OutstandingCertificateIssuers(ctx)
	if err != nil {
		return fmt.Errorf("cannot check outstanding certificate custody")
	}
	for _, issuer := range requiredIssuers {
		// A restore drill never talks to an issuer, so it needs no custody.
		if !issuerConfigs[issuer] && !*restoreDrill {
			return fmt.Errorf("an outstanding ACME issuer requires its active or retired configuration")
		}
	}
	allowed := map[string]bool{}
	for _, origin := range strings.Split(*origins, ",") {
		allowed[strings.TrimSpace(origin)] = true
	}
	devices, e := device.New(store, keys, *verificationURI, *allowLoopback)
	if e != nil {
		return fmt.Errorf("invalid device verification URI")
	}
	if *tvIssuer == "" {
		*tvIssuer = *claimAudience
	}
	if e = devices.ConfigureTV(*tvIssuer, *allowLoopback); e != nil {
		return fmt.Errorf("invalid TV issuer origin: %w", e)
	}
	trustedProxies, e := httpapi.ParseTrustedProxies(*trustedProxyCIDRs)
	if e != nil {
		return e
	}
	if !trust.Development && len(trustedProxies) == 0 {
		return fmt.Errorf("release Hosted requires --trusted-proxy-cidrs")
	}
	devices.SetAccessLifetime(*accessLifetime)
	claimService, e := claims.New(store, keys, *claimAudience)
	if e != nil {
		return fmt.Errorf("invalid current claim audience configuration")
	}
	claimHandler := &httpapi.ClaimHandler{Web: &httpapi.ClaimWeb{Store: store, Keys: keys, Audience: *claimAudience}, Service: claimService, CaptureAuthority: store.CaptureClaimAuthority}
	oidcConfig, providers, e := oidclogin.Load(ctx, *oidcFile)
	if e != nil {
		return e
	}
	smtpConfig, e := mailoutbox.LoadSMTP(*smtpFile)
	if e != nil {
		return e
	}
	accountService := identity.NewWithAccessLifetime(store, *accessLifetime)
	httpMail, e := mailoutbox.LoadHTTP(*httpMailFile)
	if e != nil {
		return e
	}
	if smtpConfig != nil && httpMail != nil {
		return fmt.Errorf("select exactly one mail provider")
	}
	var mailSender mailoutbox.Sender
	if smtpConfig != nil {
		mailSender = smtpConfig
	}
	if httpMail != nil {
		mailSender = httpMail
	}
	accountSecurity := &postgres.AccountSecurity{Store: store, Keys: keys, Identity: accountService, Lifetime: *accessLifetime, MailEnabled: mailSender != nil}
	if oidcConfig.WebOrigin == "" {
		oidcConfig.WebOrigin = "https://web.getportico.tv"
	}
	login := &postgres.OIDCLogin{Store: store, Keys: keys, Providers: providers, Lifetime: *accessLifetime, Security: accountSecurity}
	// Always reconcile configured providers, including disabling removed ones.
	if e = login.Configure(ctx); e != nil {
		return e
	}

	// App Store Server Notifications. Absent configuration disables only this
	// feature; invalid configuration is a startup error, never a pretend success,
	// because an intake that cannot verify must not accept anything.
	var appStore *appstore.KeySet
	if *appStoreFile != "" {
		info, err := os.Stat(*appStoreFile)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("private App Store configuration required")
		}
		raw, err := os.ReadFile(*appStoreFile)
		if err != nil {
			return err
		}
		var file struct {
			BundleID        string `json:"bundleId"`
			Environment     string `json:"environment"`
			RootCertificate string `json:"rootCertificatePEM"`
		}
		if err = json.Unmarshal(raw, &file); err != nil {
			return fmt.Errorf("invalid App Store configuration")
		}
		set, err := appstore.LoadKeySet([]byte(file.RootCertificate), file.BundleID, file.Environment)
		if err != nil {
			return err
		}
		appStore = &set
	}

	var direct *directroute.Service
	if *dnsKeyFile != "" {
		info, err := os.Stat(*dnsKeyFile)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("private DNS key required")
		}
		raw, err := os.ReadFile(*dnsKeyFile)
		if err != nil {
			return err
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return fmt.Errorf("invalid DNS key encoding")
		}
		// The DNS nodes present this same value (as written in the file) to read the feed.
		direct, err = directroute.New(store, []byte(base64.StdEncoding.EncodeToString(key)))
		if err != nil {
			return err
		}
	}
	// A14: the signing certificate is checked continuously, not only at start.
	// Within 30 days of expiry the operator is alerted daily; once expired,
	// readiness fails so the deployment shows it, since servers reject every
	// document signed with an expired certificate.
	ready := func(ctx context.Context) error {
		if e := keys.ValidateSigner(time.Now()); e != nil {
			return fmt.Errorf("signing authority unavailable: %w", e)
		}
		return store.Ready(ctx)
	}
	// The server-list version map answers the devices' version checks from memory
	// (Spec — Hosted at Scale). Load it before serving; if that fails, checks answer
	// 503 until the follower below has loaded it.
	servers := directory.New(store, keys)
	loadCtx, loadCancel := context.WithTimeout(ctx, 2*time.Minute)
	if e := servers.RefreshServerListVersions(loadCtx); e != nil {
		fmt.Fprintln(os.Stderr, "server list versions not loaded yet:", e)
	}
	loadCancel()
	api := &httpapi.Handler{ServerReturns: &httpapi.ServerReturns{Store: store, Directory: servers}, AppAuthorizations: &httpapi.AppAuthorizations{Store: store, Directory: servers}, AccessLog: func(line string) { fmt.Fprintln(os.Stderr, line) }, Certificates: certificateService, DirectRoutes: direct, OIDC: login, Security: accountSecurity, AccountOrigin: oidcConfig.WebOrigin, Claims: claimHandler, Sessions: sessions.New(store, keys, *accessLifetime), DevelopmentCookies: *allowLoopback, TrustedProxies: trustedProxies, Devices: devices, Identity: accountService, Directory: servers, Origins: allowed, Ready: ready, AppStore: appStore}
	handler := api.Routes()
	// Start each worker exactly once, after all configuration has validated.
	// Cancel and join them before the deferred database close, including when
	// ListenAndServe fails before the signal context is cancelled.
	var workers sync.WaitGroup
	startWorker := func(run func(context.Context)) {
		workers.Add(1)
		go func() { defer workers.Done(); run(ctx) }()
	}
	defer func() { cancel(); workers.Wait() }()
	// One connection listens for the queues' own announcements, so the workers sleep until
	// there is something to do instead of asking on a clock. Every worker keeps a slow safety
	// tick, so losing this listener costs latency and never work.
	listener := store.Notifications()
	listener.Log = func(message string) { fmt.Fprintln(os.Stderr, message) }
	wakeNotify := listener.Subscribe(postgres.WakeChannel)
	certificateNotify := listener.Subscribe(postgres.CertificateChannel)
	mailNotify := listener.Subscribe(postgres.MailChannel)
	claimNotify := listener.Subscribe(postgres.ClaimChannel)
	exportNotify := listener.Subscribe(postgres.ExportChannel)
	rotationNotify := listener.Subscribe(postgres.LabelRotationChannel)
	serverListNotify := listener.Subscribe(postgres.ServerListChannel)
	startWorker(listener.Run)
	// Read-only, so it runs in a restore drill too.
	startWorker(func(ctx context.Context) { servers.FollowServerListVersions(ctx, serverListNotify) })
	// A17: a restore drill serves the restored copy for sign-in and readiness
	// only. It must not send signed wakes to real servers, order certificates,
	// mail anyone, alert the operator or prune the copy.
	if *restoreDrill {
		fmt.Fprintln(os.Stderr, "restore drill: outbound and queue workers are disabled")
	} else {
		startWorker(func(ctx context.Context) { store.RunClaimNotifications(ctx, claimNotify) })
		worker := &wake.Worker{Repo: store, Keys: keys, Notify: wakeNotify}
		startWorker(worker.Run)
		startWorker(func(ctx context.Context) { store.RunSigningRollout(ctx, keys.Certificate.Revision) })
		startWorker(func(ctx context.Context) { store.RunSignerRevocationFence(ctx, keys) })
		certificateService.Announce(certificateNotify)
		startWorker(certificateService.Run)
		for _, retired := range retiredServices {
			retired.Announce(certificateNotify)
			startWorker(retired.Run)
		}
		// Queue workers sleep until a trigger announces work (migration 0006), their
		// own next deadline, or a slow safety tick (A38, A66). Nothing polls.
		startWorker(func(ctx context.Context) {
			runQueue(ctx, exportNotify, 10*time.Minute, func(ctx context.Context) (time.Time, bool) {
				pending, err := accountSecurity.ExportPending(ctx)
				if err != nil || !pending {
					return time.Time{}, false
				}
				if err = accountSecurity.ProcessExport(ctx); err != nil {
					log.Printf("export: %v", err)
					return time.Now().Add(time.Minute), false
				}
				return time.Time{}, true
			})
		})
		// A87: member-label rotations queued by removals, each in its own tx.
		startWorker(func(ctx context.Context) {
			runQueue(ctx, rotationNotify, 10*time.Minute, func(ctx context.Context) (time.Time, bool) {
				more, err := store.RotateQueuedMemberLabels(ctx, 50)
				if err != nil {
					log.Printf("member label rotation: %v", err)
					return time.Now().Add(time.Minute), false
				}
				return time.Time{}, more
			})
		})
		// A75: the offline worker sleeps until the next server crosses the 24-hour
		// threshold, then reports every server past it.
		startWorker(func(ctx context.Context) {
			runQueue(ctx, nil, time.Hour, func(ctx context.Context) (time.Time, bool) {
				if err := accountSecurity.ScanOfflineServers(ctx); err != nil {
					log.Printf("offline scan: %v", err)
					return time.Now().Add(time.Minute), false
				}
				next, err := accountSecurity.NextOfflineDue(ctx)
				if err != nil {
					return time.Now().Add(time.Minute), false
				}
				return next, false
			})
		})
		if mailSender != nil {
			mailer := &mailoutbox.Worker{Repo: &postgres.AccountMail{Store: store, Keys: keys}, Sender: mailSender, Notify: mailNotify}
			startWorker(mailer.Run)
		}
		startWorker(func(ctx context.Context) {
			runQueue(ctx, nil, 24*time.Hour, func(ctx context.Context) (time.Time, bool) {
				left := time.Until(keys.Certificate.NotAfter)
				if left > postgres.SigningKeyAlertWindow {
					return keys.Certificate.NotAfter.Add(-postgres.SigningKeyAlertWindow), false
				}
				log.Printf("signing certificate %s expires %s; renew it (deployment/hosted/KEYS.md)", keys.KeyID, keys.Certificate.NotAfter.Format(time.RFC3339))
				if err := accountSecurity.QueueSigningKeyAlert(ctx, *operatorAccount, keys.KeyID, keys.Certificate.NotAfter); err != nil {
					log.Printf("signing key alert: %v", err)
				}
				return time.Time{}, false
			})
		})
		// A17/A95: a commit marker lets a restore drill measure how much data a
		// restore from the archive would lose. Every 30 s while other writes
		// happen; an idle primary writes nothing.
		startWorker(func(ctx context.Context) {
			marker := &postgres.RPOMarker{}
			timer := time.NewTicker(postgres.RPOMarkerCheck)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					markCtx, stop := context.WithTimeout(ctx, 5*time.Second)
					if _, e := marker.Tick(markCtx, store, time.Now()); e != nil {
						log.Printf("recovery-point marker: %v", e)
					}
					stop()
				}
			}
		})
		// Retention (A44): every ten minutes, work through each table in short
		// batches until it is caught up, yielding between batches, so pruning keeps
		// pace with row creation at fleet scale without holding anything long.
		startWorker(func(ctx context.Context) {
			timer := time.NewTicker(10 * time.Minute)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					pruneCtx, stop := context.WithTimeout(ctx, 5*time.Minute)
					if e := store.PruneUntilCaughtUp(pruneCtx, 100); e != nil {
						log.Printf("retention: %v", e)
					}
					if login != nil {
						_ = login.Prune(pruneCtx)
						_ = accountSecurity.Prune(pruneCtx)
					}
					stop()
				}
			}
		})
	}
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "portico_signing_certificate_expiry_timestamp_seconds %d\n", keys.Certificate.NotAfter.Unix())
		requests, rejections := api.RequestMetrics()
		fmt.Fprintf(w, "portico_http_requests_total %d\nportico_admission_rejections_total %d\n", requests, rejections)
		if e := store.Metrics(ctx, w); e != nil {
			log.Printf("metrics unavailable: %v", e)
		}
	})
	var metricsToken []byte
	if *metricsTokenFile != "" {
		raw, err := os.ReadFile(*metricsTokenFile)
		if err != nil || len(strings.TrimSpace(string(raw))) < 32 {
			return fmt.Errorf("metrics token file must hold at least 32 characters")
		}
		metricsToken = []byte(strings.TrimSpace(string(raw)))
	}
	metricsMux.HandleFunc("POST /ct-observations", func(w http.ResponseWriter, r *http.Request) {
		// A38: the observation feed writes to the database, so it needs the operator's
		// token even on loopback; without a configured token it isn't offered at all.
		given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if metricsToken == nil || !ok || subtle.ConstantTimeCompare([]byte(given), metricsToken) != 1 {
			http.NotFound(w, r)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, 65537))
		if e != nil || len(raw) > 65536 {
			http.Error(w, "invalid observation", 400)
			return
		}
		if block, _ := pem.Decode(raw); block != nil {
			raw = block.Bytes
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		known, e := store.ObserveCT(ctx, raw)
		if e != nil {
			http.Error(w, "invalid observation", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"known": known})
	})
	metricsHost, _, e := net.SplitHostPort(*metricsBind)
	if e != nil || !net.ParseIP(metricsHost).IsLoopback() {
		return fmt.Errorf("metrics-listen must be a loopback address")
	}
	metricsListener, e := net.Listen("tcp", *metricsBind)
	if e != nil {
		return e
	}
	metricsServer := &http.Server{Handler: metricsMux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	defer metricsServer.Close()
	startWorker(func(ctx context.Context) {
		go func() { <-ctx.Done(); metricsServer.Close() }()
		_ = metricsServer.Serve(metricsListener)
	})
	server := &http.Server{Addr: *bind, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	fmt.Println("Hosted listening on", *bind)
	select {
	case e = <-done:
		if e != http.ErrServerClosed {
			return e
		}
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		return server.Shutdown(shutdown)
	}
	return nil
}

func productionHostedAddress(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(u.Hostname(), "web.getportico.tv")
}

// runQueue runs step when notify fires, when the deadline step returned
// arrives, right away while step reports more work, or at the safety interval
// (a lost notification costs latency, never work). notify may be nil.
func runQueue(ctx context.Context, notify <-chan struct{}, safety time.Duration, step func(context.Context) (next time.Time, more bool)) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-notify:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
		c, cancel := context.WithTimeout(ctx, 30*time.Second)
		next, more := step(c)
		cancel()
		wait := safety
		if more {
			wait = 0
			runtime.Gosched()
		} else if !next.IsZero() {
			wait = min(max(time.Until(next), time.Second), safety)
		}
		timer.Reset(wait)
	}
}
