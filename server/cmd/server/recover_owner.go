package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

func recoverOwner(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("recover-owner", flag.ContinueOnError)
	state := flags.String("state", "", "Existing server state directory (required)")
	resetMFA := flags.Bool("reset-mfa", false, "Also remove the owner's second factor")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *state == "" || flags.NArg() != 0 {
		return errors.New("provide --state with the existing server state directory")
	}
	root, err := filepath.EvalSymlinks(*state)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("the state directory must be a directory")
	}
	if info.Mode().Perm()&0077 != 0 {
		// The server warns about loose permissions instead of refusing; the
		// recovery command does the same.
		fmt.Fprintln(out, "Warning: other accounts on this computer can read your Portico data, including sign-in secrets. Tighten the state folder when you are done.")
	}
	dbPath := filepath.Join(root, "server.sqlite")
	if info, err = os.Stat(dbPath); err != nil || !info.Mode().IsRegular() {
		return errors.New("existing server database required")
	}
	db, err := persistence.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	code, err := identity.RecoverOwnerCLI(context.Background(), db, *resetMFA)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "One-time owner recovery password: %s\nSign in and choose a new password. Previous sessions have been revoked.\n", code)
	return err
}
