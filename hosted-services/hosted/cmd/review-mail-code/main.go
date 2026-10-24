//go:build linux

// review-mail-code reads one already-issued disposable registration mail. It
// does not create/verify accounts, alter mail delivery, or bypass any API guard.
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
)

var unavailable = errors.New("registration mail unavailable")

func privateRead(path string) ([]byte, os.FileInfo, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, unavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return nil, nil, unavailable
	}
	raw := make([]byte, info.Size())
	if _, err = f.ReadAt(raw, 0); err != nil {
		return nil, nil, unavailable
	}
	return raw, info, nil
}

// Replaces only the explicit protected registration request, atomically, with
// the issued code and expiry only. Use a fresh file per registration; the
// original registration fields are intentionally removed. Preserve the runner's ownership; no token is printed.
func publish(path string, original os.FileInfo, raw []byte) error {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return unavailable
	}
	f, err := os.CreateTemp(parent, ".registration-code-*")
	if err != nil {
		return unavailable
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if stat, ok := original.Sys().(*syscall.Stat_t); ok {
		if err = f.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
			return unavailable
		}
	} else {
		return unavailable
	}
	if _, err = f.Write(raw); err != nil {
		return unavailable
	}
	if err = f.Sync(); err != nil {
		return unavailable
	}
	if err = f.Close(); err != nil {
		return unavailable
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(original, current) {
		return unavailable
	}
	return os.Rename(f.Name(), path)
}

func run(databaseFile, sealFile, requestFile string) error {
	requestRaw, original, err := privateRead(requestFile)
	if err != nil {
		return err
	}
	defer clear(requestRaw)
	var request struct {
		RegistrationID string `json:"registrationId"`
		ContactAddress string `json:"contactAddress"`
		Origin         string `json:"origin"`
	}
	if json.Unmarshal(requestRaw, &request) != nil || !regexp.MustCompile(`^reg_[A-Za-z0-9_-]{16,128}$`).MatchString(request.RegistrationID) || request.Origin != "https://web.getportico.tv" || !mailoutbox.ValidAddress(request.ContactAddress) || !strings.HasSuffix(request.ContactAddress, "@m0.getportico.invalid") {
		return unavailable
	}
	dsn, _, err := privateRead(databaseFile)
	if err != nil {
		return err
	}
	defer clear(dsn)
	sealRaw, _, err := privateRead(sealFile)
	if err != nil {
		return err
	}
	defer clear(sealRaw)
	var seal security.SealFile
	if json.Unmarshal(sealRaw, &seal) != nil || seal.Version == 0 {
		return unavailable
	}
	key, err := base64.RawURLEncoding.Strict().DecodeString(seal.Key)
	if err != nil || len(key) != 32 {
		return unavailable
	}
	defer clear(key)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", strings.TrimSpace(string(dsn)))
	if err != nil {
		return unavailable
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return unavailable
	}
	defer tx.Rollback()
	var id string
	var cipher []byte
	var expires time.Time
	err = tx.QueryRowContext(ctx, `SELECT m.id,m.ciphertext,LEAST(m.expires_at,r.code_expires_at) FROM account_registrations r JOIN account_mail_outbox m ON m.resource_id=r.id AND m.kind='registration' AND m.revision=r.revision WHERE r.id=$1 AND r.address=$2 AND r.verified_at IS NULL AND r.completed_at IS NULL AND r.cancelled_at IS NULL AND r.attempts<5 AND r.expires_at>clock_timestamp() AND r.code_expires_at>clock_timestamp() AND m.ciphertext IS NOT NULL AND m.expires_at>clock_timestamp() ORDER BY m.created_at DESC LIMIT 1`, request.RegistrationID, request.ContactAddress).Scan(&id, &cipher, &expires)
	if err != nil {
		return unavailable
	}
	defer clear(cipher)
	keys := &security.Keys{SealKey: key, SealVersion: seal.Version, SealLegacy: seal.LegacyUnversioned}
	plain, err := keys.Open(cipher, "mail:"+id)
	if err != nil {
		return unavailable
	}
	var payload mailoutbox.Payload
	if json.Unmarshal([]byte(plain), &payload) != nil || payload.To != request.ContactAddress || !regexp.MustCompile(`^[0-9]{6}$`).MatchString(payload.Code) || !expires.After(time.Now()) {
		return unavailable
	}
	if err = tx.Commit(); err != nil {
		return unavailable
	}
	raw, err := json.Marshal(struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{payload.Code, expires.UTC()})
	if err != nil {
		return unavailable
	}
	defer clear(raw)
	return publish(requestFile, original, append(raw, '\n'))
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Root operator required.")
		os.Exit(1)
	}
	database := flag.String("database-url-file", "", "Protected active database URL")
	seal := flag.String("seal-file", "", "Protected existing sealing file")
	request := flag.String("registration-file", "", "Protected disposable registration request; replaced with its issued code")
	flag.Parse()
	if *database == "" || *seal == "" || *request == "" || flag.NArg() != 0 || run(*database, *seal, *request) != nil {
		fmt.Fprintln(os.Stderr, "Registration mail code unavailable.")
		os.Exit(1)
	}
	fmt.Println("Registration mail code written to the protected request file.")
}
