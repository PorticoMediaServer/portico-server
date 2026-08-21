package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExplicitCookieSecureForcesSecureCookies(t *testing.T) {
	t.Setenv("PORTICO_COOKIE_SECURE", "true")

	cfg := Load()
	if !cfg.CookieSecure {
		t.Fatalf("expected explicit secure-cookie override to be respected")
	}
}

func TestDefaultAddressListensOnLAN(t *testing.T) {
	t.Setenv("PORTICO_ADDR", "")
	t.Setenv("PORTICO_PORT", "")

	cfg := Load()
	if cfg.Addr != "0.0.0.0:32500" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
}

func TestPublicOriginIsExplicitAndNormalized(t *testing.T) {
	t.Setenv("PORTICO_PUBLIC_ORIGIN", " https://media.example.test/ ")
	if got := Load().PublicOrigin; got != "https://media.example.test" {
		t.Fatalf("PublicOrigin = %q", got)
	}
}

func TestServicePortConfiguresDefaultAddress(t *testing.T) {
	t.Setenv("PORTICO_ADDR", "")
	t.Setenv("PORTICO_PORT", "32542")

	cfg := Load()
	if cfg.Addr != "0.0.0.0:32542" {
		t.Fatalf("Addr = %q", cfg.Addr)
	}
}

func TestInvalidServicePortFallsBackToPorticoDefault(t *testing.T) {
	for _, value := range []string{"not-a-port", "0", "65536"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("PORTICO_ADDR", "")
			t.Setenv("PORTICO_PORT", value)
			if cfg := Load(); cfg.Addr != "0.0.0.0:32500" {
				t.Fatalf("Addr = %q", cfg.Addr)
			}
		})
	}
}

func TestTrustedProxyCIDRsParseExactAddressesAndPrefixes(t *testing.T) {
	prefixes, err := parseTrustedProxyCIDRs("127.0.0.1, ::1/128, 172.16.0.0/12")
	if err != nil {
		t.Fatalf("parseTrustedProxyCIDRs: %v", err)
	}
	if len(prefixes) != 3 || prefixes[0].String() != "127.0.0.1/32" || prefixes[1].String() != "::1/128" || prefixes[2].String() != "172.16.0.0/12" {
		t.Fatalf("unexpected trusted proxy prefixes: %#v", prefixes)
	}
}

func TestTrustedProxyCIDRsRejectInvalidValues(t *testing.T) {
	if _, err := parseTrustedProxyCIDRs("127.0.0.1,not-a-cidr"); err == nil {
		t.Fatalf("expected invalid trusted proxy CIDR to fail")
	}
}

func TestBundledFFmpegDefaultsWhenPresent(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "third_party", "ffmpeg", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatalf("create bundled bin: %v", err)
	}
	ffmpegName := "ffmpeg"
	ffprobeName := "ffprobe"
	if runtime.GOOS == "windows" {
		ffmpegName += ".exe"
		ffprobeName += ".exe"
	}
	ffmpegPath := filepath.Join(bin, ffmpegName)
	ffprobePath := filepath.Join(bin, ffprobeName)
	if err := os.WriteFile(ffmpegPath, []byte("ffmpeg"), 0o700); err != nil {
		t.Fatalf("write ffmpeg: %v", err)
	}
	if err := os.WriteFile(ffprobePath, []byte("ffprobe"), 0o700); err != nil {
		t.Fatalf("write ffprobe: %v", err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})

	cfg := Load()
	ffmpegPath, _ = filepath.EvalSymlinks(ffmpegPath)
	ffprobePath, _ = filepath.EvalSymlinks(ffprobePath)
	if cfg.FFmpegPath != ffmpegPath {
		t.Fatalf("FFmpegPath = %q, expected bundled path %q", cfg.FFmpegPath, ffmpegPath)
	}
	if cfg.FFprobePath != ffprobePath {
		t.Fatalf("FFprobePath = %q, expected bundled path %q", cfg.FFprobePath, ffprobePath)
	}
}

func TestExplicitFFmpegPathOverridesBundledDefault(t *testing.T) {
	t.Setenv("PORTICO_FFMPEG_PATH", "/custom/ffmpeg")
	t.Setenv("PORTICO_FFPROBE_PATH", "/custom/ffprobe")

	cfg := Load()
	if cfg.FFmpegPath != "/custom/ffmpeg" || cfg.FFprobePath != "/custom/ffprobe" {
		t.Fatalf("explicit paths were not respected: ffmpeg=%q ffprobe=%q", cfg.FFmpegPath, cfg.FFprobePath)
	}
}

func TestHostedDocumentPublicKeysAreExplicitAndNormalized(t *testing.T) {
	t.Setenv("PORTICO_HOSTED_DOCUMENT_PUBLIC_KEYS_JSON", `{" hosted-key-1 ":" cHVibGljLWtleQ== ","":"ignored"}`) // gitleaks:allow -- synthetic public-key fixture
	cfg := Load()
	if len(cfg.HostedDocumentPublicKeys) != 1 || cfg.HostedDocumentPublicKeys["hosted-key-1"] != "cHVibGljLWtleQ==" {
		t.Fatalf("HostedDocumentPublicKeys = %#v", cfg.HostedDocumentPublicKeys)
	}

	t.Setenv("PORTICO_HOSTED_DOCUMENT_PUBLIC_KEYS_JSON", "not-json")
	cfg = Load()
	if len(cfg.HostedDocumentPublicKeys) != 0 {
		t.Fatalf("malformed key config must fail closed, got %#v", cfg.HostedDocumentPublicKeys)
	}
}
