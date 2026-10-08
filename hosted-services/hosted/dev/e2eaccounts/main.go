// Command e2eaccounts creates throwaway Portico Accounts directly in a local
// development Hosted database, for end-to-end tests on one machine: a local
// Hosted has no mail transport, so the verified-email registration flow cannot
// finish there. It refuses any database whose name does not contain "e2e" or
// "test", and it prints each account's id.
//
//	go run ./dev/e2eaccounts -database-url-file <file> -password <throwaway> alice bob
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
)

func main() {
	file := flag.String("database-url-file", "", "file holding the local development database URL")
	password := flag.String("password", "", "throwaway password for every account (8+ characters)")
	flag.Parse()
	raw, err := os.ReadFile(*file)
	if err != nil || len(flag.Args()) == 0 || len(*password) < 8 {
		fmt.Fprintln(os.Stderr, "usage: e2eaccounts -database-url-file <file> -password <throwaway> <username>...")
		os.Exit(2)
	}
	dsn := strings.TrimSpace(string(raw))
	u, err := url.Parse(dsn)
	if err != nil || !(strings.Contains(u.Path, "e2e") || strings.Contains(u.Path, "test")) {
		fmt.Fprintln(os.Stderr, "refusing a database whose name does not contain e2e or test")
		os.Exit(2)
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer store.DB.Close()
	if err = store.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	ids := identity.New(store)
	for _, name := range flag.Args() {
		session, err := ids.Register(ctx, name, strings.ToUpper(name[:1])+name[1:], *password)
		if err != nil {
			fmt.Fprintln(os.Stderr, name+":", err)
			os.Exit(1)
		}
		fmt.Printf("%s %s\n", name, session.Account.ID)
	}
}
