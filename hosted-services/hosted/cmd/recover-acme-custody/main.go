//go:build linux

// recover-acme-custody is a root-only, stopped-worker operator recovery. It
// restores only a genuinely registered provider key/URL, never setup authority.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/postgres"
)

var refused = errors.New("ACME custody recovery refused")

func privateRead(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, refused
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 32768 {
		return nil, refused
	}
	raw := make([]byte, info.Size())
	if _, err = f.ReadAt(raw, 0); err != nil {
		clear(raw)
		return nil, refused
	}
	return raw, nil
}

func workersStopped(ctx context.Context) bool {
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(check, "systemctl", "show", "portico-astra-hosted.service", "--property=LoadState,ActiveState,MainPID").Output()
	if err != nil {
		return false
	}
	return stoppedUnit(raw)
}
func stoppedUnit(raw []byte) bool {
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	return values["LoadState"] == "loaded" && values["ActiveState"] == "inactive" && values["MainPID"] == "0"
}

func backupCandidate(path string, value postgres.ACMECustody) error {
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return refused
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return refused
	}
	return writeCandidateBackup(path, value)
}
func writeCandidateBackup(path string, value postgres.ACMECustody) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return refused
	}
	defer clear(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return refused
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		return refused
	}
	if err = f.Sync(); err != nil {
		return refused
	}
	if err = f.Close(); err != nil {
		return refused
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return refused
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return refused
	}
	return nil
}

func unchangedPrivateInputs(inputs map[string][]byte) bool {
	for path, want := range inputs {
		actual, err := privateRead(path)
		if err != nil {
			return false
		}
		same := bytes.Equal(actual, want)
		clear(actual)
		if !same {
			return false
		}
	}
	return true
}

func sameCustody(a, b postgres.ACMECustody) bool {
	return a.Issuer == b.Issuer && a.Directory == b.Directory && a.URL == b.URL && a.CreatedAt.Equal(b.CreatedAt) && bytes.Equal(a.Cipher, b.Cipher)
}

func run(ctx context.Context, sourceFile, currentFile, configFile, backupFile string, apply bool) error {
	if apply && !workersStopped(ctx) {
		return refused
	}
	sourceRaw, err := privateRead(sourceFile)
	if err != nil {
		return refused
	}
	defer clear(sourceRaw)
	currentRaw, err := privateRead(currentFile)
	if err != nil {
		return refused
	}
	defer clear(currentRaw)
	if bytes.Equal(bytes.TrimSpace(sourceRaw), bytes.TrimSpace(currentRaw)) {
		return refused
	}
	configRaw, err := privateRead(configFile)
	if err != nil {
		return refused
	}
	defer clear(configRaw)
	cfg, err := certificates.LoadConfig(configFile)
	if err != nil {
		return refused
	}
	issuer, directory, err := certificates.CustodyScope(cfg)
	if err != nil {
		return refused
	}
	sealRaw, err := privateRead(cfg.SealKeyFile)
	if err != nil {
		return refused
	}
	defer clear(sealRaw)
	source, err := postgres.Open(ctx, strings.TrimSpace(string(sourceRaw)))
	if err != nil {
		return refused
	}
	defer source.DB.Close()
	current, err := postgres.Open(ctx, strings.TrimSpace(string(currentRaw)))
	if err != nil {
		return refused
	}
	defer current.DB.Close()
	original, err := source.ReadACMECustody(ctx, issuer, directory)
	if err != nil || original.URL == "" {
		return refused
	}
	defer clear(original.Cipher)
	candidate, err := current.ReadACMECustody(ctx, issuer, directory)
	if err != nil || candidate.URL != "" {
		return refused
	}
	defer clear(candidate.Cipher)
	if err = current.CheckACMECandidate(ctx, candidate); err != nil {
		return refused
	}
	if !apply {
		return nil
	} // Inspection makes no provider request or database change.
	if backupFile == "" || backupCandidate(backupFile, candidate) != nil {
		return refused
	}
	if !workersStopped(ctx) {
		return refused
	}
	if certificates.VerifyCustodyRecovery(ctx, cfg, certificates.Account{Cipher: original.Cipher, URL: original.URL}, certificates.Account{Cipher: candidate.Cipher}) != nil {
		return refused
	}
	// Recheck every private input and the genuine source after provider I/O.
	if !unchangedPrivateInputs(map[string][]byte{sourceFile: sourceRaw, currentFile: currentRaw, configFile: configRaw, cfg.SealKeyFile: sealRaw}) {
		return refused
	}
	again, err := source.ReadACMECustody(ctx, issuer, directory)
	if err != nil {
		return refused
	}
	defer clear(again.Cipher)
	if !sameCustody(original, again) || !workersStopped(ctx) {
		return refused
	}
	if current.RecoverACMECustody(ctx, candidate, original) != nil {
		return refused
	}
	return nil
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Root operator required.")
		os.Exit(1)
	}
	source := flag.String("source-database-url-file", "", "Protected preserved registered-account database URL")
	current := flag.String("current-database-url-file", "", "Protected current database URL")
	config := flag.String("config-file", "", "Existing protected ACME configuration")
	backup := flag.String("candidate-backup-file", "", "New root-private encrypted candidate backup; required with apply")
	apply := flag.Bool("apply", false, "Recover verified custody while Hosted workers are stopped; default inspects only")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if *source == "" || *current == "" || *config == "" || flag.NArg() != 0 || run(ctx, *source, *current, *config, *backup, *apply) != nil {
		fmt.Fprintln(os.Stderr, "ACME custody recovery refused; preserved state must be reconciled.")
		os.Exit(1)
	}
	if *apply {
		fmt.Println("Existing provider-account custody recovered; encrypted candidate backup preserved.")
	} else {
		fmt.Println("Custody inspection passed; no provider requests or changes made.")
	}
}
