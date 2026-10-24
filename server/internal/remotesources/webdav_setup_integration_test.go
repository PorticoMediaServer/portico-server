//go:build rcloneintegration && linux

package remotesources

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/childprocess"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
)

// This qualifies the real process/configuration/diagnostic component. Its
// context-only fixture guard is NOT production owner, source or DB authority.
func TestResolvedWebDAVSetupUsesActualRetainedProcess(t *testing.T) {
	program := lockedRcloneFixture(t)
	binary, err := os.ReadFile(program)
	if err != nil {
		t.Fatal(err)
	}
	digest := hex.EncodeToString(sha256Sum(binary))
	clear(binary)
	for _, mode := range []string{"imported/supported", "imported/ignored", "imported/wrong", "crypt/supported", "crypt/ignored", "crypt/wrong", "imported/unknown"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			payload := []byte(strings.Repeat("fixture", 16))
			name := "one.bin"
			crypt := strings.HasPrefix(mode, "crypt/")
			if crypt {
				name, payload = actualCiphertextFixture(t, ctx, program)
			}
			var gets atomic.Int32
			server := newPrivateSourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "fixture" || password != "potato" {
					w.WriteHeader(401)
					return
				}
				switch r.Method {
				case "PROPFIND":
					if r.URL.Path != "/media/cipher" && r.URL.Path != "/media/cipher/" {
						w.WriteHeader(404)
						return
					}
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(207)
					fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/media/cipher/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
					if r.Header.Get("Depth") == "1" {
						fmt.Fprintf(w, `<d:response><d:href>/media/cipher/%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength><d:getetag>"fixture"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, name, len(payload))
					}
					fmt.Fprint(w, `</d:multistatus>`)
				case "GET":
					gets.Add(1)
					if ownedRcloneChildren(t) != 0 {
						t.Error("diagnostic preceded temporary serve physical Wait")
					}
					if r.URL.Path != "/media/cipher/"+name || !strings.HasPrefix(r.UserAgent(), "rclone/") || r.Header.Get("Referer") == "" {
						t.Error("resolved saved request changed")
						w.WriteHeader(400)
						return
					}
					var start, end int64
					if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil || start < 0 || end >= int64(len(payload)) || end-start >= 16 {
						t.Error("range bound changed")
						w.WriteHeader(400)
						return
					}
					if strings.HasSuffix(mode, "/ignored") {
						w.WriteHeader(200)
						return
					}
					total := len(payload)
					if strings.HasSuffix(mode, "/wrong") {
						total++
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
					w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
					w.WriteHeader(206)
					_, _ = w.Write(payload[start : end+1])
				default:
					w.WriteHeader(405)
				}
			}))
			raw := "[leaf]\ntype=webdav\nurl=" + server.URL + "/media/\nuser=fixture\npass=YWFhYWFhYWFhYWFhYWFhYXMaGgIlEQ\n"
			remote := "leaf:cipher"
			if crypt {
				raw += "[outer]\ntype=crypt\nremote=leaf:cipher\npassword=YWFhYWFhYWFhYWFhYWFhYXMaGgIlEQ\nfilename_encryption=off\ndirectory_name_encryption=false\n"
				remote = "outer:"
			}
			if strings.HasSuffix(mode, "/unknown") {
				raw += "pacer_min_sleep=20ms\n"
			}
			approval, err := (remotemedia.Policy{}).PrepareEndpointApproval(ctx, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			policy := remotemedia.Policy{SourceEndpoints: true, Approvals: []remotemedia.Approval{approval}}
			profile, _ := SourceReadPreset("list")
			profile.Limits.Connections = 1
			profile.Limits.RequestsPerSecond = 50
			check := func(ctx context.Context) error { return ctx.Err() }
			var observations []webDAVSetupObservation
			observe, err := candidateWebDAVObserver([]byte(raw), remote, profile, policy, check, &observations)
			if err != nil {
				t.Fatal(err)
			}
			service := &Service{private: t.TempDir()}
			if err = service.runCandidateServe(ctx, "", "1", profile, []byte(raw), remote, rcloneprogram.Program{Path: program}, digest, policy, check, observe); err != nil {
				t.Fatal("retained setup component failed", err)
			}
			want, count := remotemedia.SetupRangeSupported, int32(3)
			if strings.HasSuffix(mode, "/ignored") || strings.HasSuffix(mode, "/wrong") {
				want, count = remotemedia.SetupRangeUnsupported, 1
			}
			if strings.HasSuffix(mode, "/unknown") {
				want, count = remotemedia.SetupRangeUnknown, 0
			}
			if len(observations) != 1 || aggregateWebDAVSetup(observations) != want || gets.Load() != count {
				t.Fatal("actual setup fact mismatch", aggregateWebDAVSetup(observations), gets.Load(), want, count)
			}
			if count > 0 && (observations[0].Object != name || observations[0].Size != int64(len(payload))) {
				t.Fatal("raw listed object identity changed")
			}
			if ownedRcloneChildren(t) != 0 {
				t.Fatal("owned temporary process remains")
			}
		})
	}
}

func actualCiphertextFixture(t *testing.T, ctx context.Context, program string) (string, []byte) {
	t.Helper()
	root := t.TempDir()
	output := filepath.Join(root, "cipher")
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "plain.bin")
	if err := os.WriteFile(source, []byte(strings.Repeat("procedural-fixture", 8)), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "fixture.conf")
	if err := os.WriteFile(config, []byte("[fixturecrypt]\ntype=crypt\nremote="+output+"\npassword=YWFhYWFhYWFhYWFhYWFhYXMaGgIlEQ\nfilename_encryption=off\ndirectory_name_encryption=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, program, "copyto", source, "fixturecrypt:one.bin", "--config", config, "--ask-password=false", "--log-level=ERROR")
	command.Env = []string{"HOME=" + root, "TMPDIR=" + root}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	childprocess.Configure(command)
	if err := command.Run(); err != nil {
		t.Fatal("owned offline ciphertext fixture generation failed", err)
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 1 {
		t.Fatal("missing actual ciphertext fixture", err)
	}
	body, err := os.ReadFile(filepath.Join(output, entries[0].Name()))
	if err != nil || len(body) < 32 || string(body[:8]) != "RCLONE\x00\x00" {
		t.Fatal("not actual rclone ciphertext", err)
	}
	return entries[0].Name(), body
}
