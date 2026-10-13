//go:build devtrust && !release

// Command wake drives the real signed-wake worker across its injected transport
// boundary. The production endpoint registry and egress client refuse loopback;
// this fixture supplies one work item read from an isolated test database.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/wake"
	"portico.local/hosted/trust"
)

type fixture struct {
	job    wake.Work
	status int
}

func (f *fixture) Claim(context.Context, string, time.Time) (wake.Work, bool, error) {
	return f.job, true, nil
}
func (f *fixture) NextDue(context.Context) (time.Time, bool, error) { return time.Time{}, false, nil }
func (f *fixture) Finish(_ context.Context, _ wake.Work, status int, _ time.Time) error {
	f.status = status
	return nil
}
func run() error {
	database := flag.String("database-url-file", "", "isolated test database URL file")
	rootFile := flag.String("root-public-key-file", "", "test root public PEM")
	keyFile := flag.String("key-file", "", "test signing set")
	origin := flag.String("server-origin", "", "numeric loopback HTTP server")
	id := flag.String("server-id", "", "claimed server in the test database")
	flag.Parse()
	u, err := url.Parse(*origin)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(u.Hostname())
	port, pe := strconv.Atoi(u.Port())
	if err != nil || !ip.IsLoopback() || ip.Is4In6() || ip.Zone() != "" || u.Scheme != "http" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		pe != nil || port < 1 || port > 65535 || strconv.Itoa(port) != u.Port() {
		return errors.New("numeric loopback HTTP origin with an explicit port required")
	}
	raw, err := os.ReadFile(*database)
	if err != nil {
		return err
	}
	dsn := strings.TrimSpace(string(raw))
	dbURL, err := url.Parse(dsn)
	if err != nil || !strings.Contains(dbURL.Path, "test") || dbURL.Host != "" || !strings.HasPrefix(dbURL.Query().Get("host"), "/tmp/portico-e2e-socket-") {
		return errors.New("owned test Unix-socket database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.DB.Close()
	var revision int64
	if err = store.DB.QueryRowContext(ctx, `SELECT policy_revision FROM servers WHERE id=$1`, *id).Scan(&revision); err != nil {
		return err
	}
	raw, err = os.ReadFile(*rootFile)
	if err != nil {
		return err
	}
	root, err := trust.ParsePublicPEM(raw)
	if err != nil {
		return err
	}
	keys, err := security.LoadKeysWithRoot(*keyFile, root)
	if err != nil {
		return err
	}
	repo := &fixture{job: wake.Work{ServerID: *id, Kind: "check-in", BaseURL: *origin, Revision: revision}}
	worker := wake.Worker{Repo: repo, Keys: keys, Client: &http.Client{
		Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") },
	}}
	if _, err = worker.Once(ctx); err != nil {
		return err
	}
	if repo.status != http.StatusAccepted {
		return fmt.Errorf("wake response: %d", repo.status)
	}
	fmt.Printf("wake delivered: revision %d, status %d\n", revision, repo.status)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
