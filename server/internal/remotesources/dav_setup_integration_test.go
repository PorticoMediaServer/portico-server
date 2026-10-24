//go:build rcloneintegration && linux

package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/remotemedia"
)

func newPrivateSourceServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var listener net.Listener
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err != nil {
			continue
		}
		ip := prefix.Addr().Unmap()
		if !ip.Is4() || !(ip.IsPrivate() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip)) || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		listener, err = net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
		if err == nil {
			break
		}
	}
	if listener == nil {
		t.Fatal("an existing permitted private IPv4 fixture interface is required")
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func ownedRcloneChildren(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		_, rest, ok := strings.Cut(string(stat), ") ")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 2 || fields[1] != strconv.Itoa(os.Getpid()) {
			continue
		}
		executable, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if err == nil && filepath.Base(executable) == "rclone" {
			count++
		}
	}
	return count
}

func TestDedicatedDAVSetupRunsBoundedOriginChecksAfterPhysicalStop(t *testing.T) {
	program := lockedRcloneFixture(t)
	t.Setenv("PORTICO_RCLONE", program)
	for _, mode := range []string{"supported", "ignored", "wrong", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			f, _, identities, owner, _, _ := nativeSaveFixture(t)
			authorize := func(tx *sql.Tx) error { return currentLocalOwnerTx(ctx, tx, identities, owner) }
			var gets atomic.Int32
			selectedMode := atomic.Value{}
			selectedMode.Store(mode)
			server := newPrivateSourceServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if !ok || user != "fixture" || password != "fixture-only" {
					w.WriteHeader(401)
					return
				}
				switch r.Method {
				case "PROPFIND":
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(207)
					fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/media/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
					if r.Header.Get("Depth") == "1" {
						fmt.Fprint(w, `<d:response><d:href>/media/one.bin</d:href><d:propstat><d:prop><d:getcontentlength>64</d:getcontentlength><d:getetag>"fixture"</d:getetag><d:getlastmodified>Thu, 01 Jan 2026 00:00:00 GMT</d:getlastmodified></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
					}
					fmt.Fprint(w, `</d:multistatus>`)
				case "GET":
					gets.Add(1)
					if r.URL.Path != "/media/one.bin" || r.UserAgent() != "Portico/1" {
						t.Error("GET was not the dedicated listed-file observer")
						w.WriteHeader(400)
						return
					}
					if count := ownedRcloneChildren(t); count != 0 {
						t.Error("origin diagnostic ran before temporary serve physical Wait", count)
					}
					var start, end int64
					if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil || end-start+1 > 16 {
						t.Error("unbounded setup GET")
						w.WriteHeader(400)
						return
					}
					switch selectedMode.Load().(string) {
					case "timeout":
						<-r.Context().Done()
						return
					case "ignored":
						w.WriteHeader(200)
						_, _ = w.Write([]byte(strings.Repeat("x", 64)))
						return
					case "wrong":
						start++
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/64", start, end))
					w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
					w.WriteHeader(206)
					_, _ = w.Write([]byte(strings.Repeat("x", int(end-start+1))))
				default:
					w.WriteHeader(405)
				}
			}))
			input := DAVInput{OperationID: nativeSaveOperation(200), Name: "Private DAV", Root: server.URL + "/media/", Username: "fixture", Password: "fixture-only", InsecureLocal: true}
			receipt, err := f.s.ConfigureDAV(ctx, "actual-owner-fixture", "", input, authorize)
			if mode == "timeout" {
				if !errors.Is(err, context.DeadlineExceeded) || gets.Load() != 1 {
					t.Fatal("timeout was not unknown/failure", gets.Load(), err)
				}
				var count int
				if err = f.s.db.QueryRow(`SELECT count(*) FROM remote_sources WHERE name='Private DAV'`).Scan(&count); err != nil || count != 0 {
					t.Fatal("timeout published source", count, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := "conditional"
			expectedGETs := int32(3)
			if mode != "supported" {
				expected = "unsupported"
				expectedGETs = 1
			}
			if receipt.Source.RangeSupport != expected || gets.Load() != expectedGETs {
				t.Fatal(receipt.Source.RangeSupport, gets.Load())
			}
			if mode == "supported" {
				selectedMode.Store("ignored")
				input.OperationID = nativeSaveOperation(201)
				input.ExpectedGeneration = receipt.Source.Generation
				changed, err := f.s.ConfigureDAV(ctx, "actual-owner-fixture", receipt.Source.ID, input, authorize)
				if err != nil || changed.Source.RangeSupport != "unsupported" || changed.Source.Generation != 2 || gets.Load() != 4 {
					t.Fatal("saved configuration reused old range fact", changed.Source.RangeSupport, gets.Load(), err)
				}
			}
			if count := ownedRcloneChildren(t); count != 0 {
				t.Fatal("setup left a temporary serve", count)
			}
		})
	}
}

func TestSourceDAVConfigReaderDoesNotResolveInsideTransaction(t *testing.T) {
	config := remotemedia.DAVConfig{Root: "https://nas.example/media/", Approval: remotemedia.Approval{Root: "https://nas.example:443/", Addresses: []string{"192.168.42.7"}}}
	resolver := panicSetupResolver{}
	if _, err := remotemedia.NewSourceDAV(config, resolver); err != nil {
		t.Fatal(err)
	}
}

type panicSetupResolver struct{}

func (panicSetupResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	panic("DB-only config reader performed DNS")
}
