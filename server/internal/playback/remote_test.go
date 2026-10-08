package playback

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/decodertest"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storage"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--portico-storage-helper" {
		if storage.Helper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestRealSTRMApprovedLANProbeRangeAndRevocation(t *testing.T) {
	ffmpeg := decodertest.QualifiedFFmpeg(t)
	root := t.TempDir()
	media := filepath.Join(root, "remote.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-f", "lavfi", "-i", "sine=frequency=440", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart", "-y", media)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("fixture %s %v", out, e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media/redirect.mp4" {
			http.Redirect(w, r, "/media/movie.mp4?credential=private", 302)
			return
		}
		if r.URL.Path != "/media/movie.mp4" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials leaked to upstream")
		}
		f, e := os.Open(media)
		if e != nil {
			t.Error(e)
			return
		}
		defer f.Close()
		info, _ := f.Stat()
		http.ServeContent(w, r, "movie.mp4", info.ModTime(), f)
	}))
	defer server.Close()
	descriptor := filepath.Join(root, "Movie (2026).strm")
	if e := os.WriteFile(descriptor, []byte(server.URL+"/media/redirect.mp4?credential=private\n"), 0600); e != nil {
		t.Fatal(e)
	}
	info, _ := os.Stat(descriptor)
	db, e := persistence.Open(filepath.Join(root, "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES('owner','owner',X'00','profile')`)
	if e != nil {
		t.Fatal(e)
	}
	_, item, _ := catalogFixture(t, db, "lib", "/test", compactcatalog.Movie, "item", "Movie", compactcatalog.Asset{Path: descriptor, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: "strm"})
	hash := identity.Digest("access")
	insertFixtureSession(t, db, hash, "owner", "profile", "owner", time.Now().UTC().Add(time.Hour).Format(time.RFC3339))
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	store := storage.New(binary)
	remote := NewRemote(db, store, assets.Probe{Supervisor: store.Supervisor})
	s := New(db)
	s.ConfigureRemote(remote)
	p := identity.Principal{Hash: hash, Viewer: identity.Viewer{AccountID: "owner", ProfileID: "profile", Authority: "local", Role: "owner"}}
	if _, e = s.CreateContext(ctx, p, item, "auto", "denied"); e == nil {
		t.Fatal("unapproved LAN descriptor played")
	}
	if _, e = s.ApproveNetworkRoots(ctx, "lib", []string{server.URL + "/media/"}); e != nil {
		t.Fatal(e)
	}
	session, e := s.CreateContext(ctx, p, item, "auto", "allowed")
	if e != nil {
		t.Fatal(e)
	}
	if session.Mode != "direct" || session.Duration < 1 || strings.Contains(session.StreamURL, "private") || strings.Contains(session.StreamURL, server.URL) {
		t.Fatalf("unsafe or unprobed session %+v", session)
	}
	grant := strings.TrimPrefix(session.StreamURL, "/v1/media/")
	response, close, yes, e := s.OpenRemote(ctx, grant, "GET", "bytes=0-15", false)
	if e != nil || !yes {
		t.Fatalf("range %v %v", yes, e)
	}
	raw, e := io.ReadAll(response.Body)
	close()
	original, e2 := os.ReadFile(media)
	if e != nil || e2 != nil || response.StatusCode != 206 || !bytes.Equal(raw, original[:16]) {
		t.Fatalf("remote range %d %q %v", response.StatusCode, raw, e)
	}
	again, e := s.CreateContext(ctx, p, item, "auto", "allowed")
	if e != nil || again.ID != session.ID {
		t.Fatal("STRM receipt retry failed", e)
	}
	if _, e = s.ApproveNetworkRoots(ctx, "lib", nil); e != nil {
		t.Fatal(e)
	}
	if _, _, _, e = s.OpenRemote(ctx, grant, "GET", "", false); e == nil {
		t.Fatal("network approval removal did not revoke media")
	}
}
