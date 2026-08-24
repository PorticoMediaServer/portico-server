package app

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLiveTVHLSReferenceIsOpaqueAndGrantSourceFenced(t *testing.T) {
	// Direct Server construction is supported by integration harnesses. The
	// issuing boundary must initialize its own private table rather than panic.
	server := &Server{}
	const (
		channel = "channel-1"
		source  = "https://provider.example/live/master.m3u8?provider_token=secret"
		item    = "https://provider.example/live/720/segment.ts?provider_token=secret"
		grant   = "ptc_mg_current"
	)
	publicURL := server.issueLiveTVHLSReference(channel, source, item, "720p-high", grant)
	if strings.Contains(publicURL, "provider.example") || strings.Contains(publicURL, "provider_token") || strings.Contains(publicURL, url.QueryEscape(item)) {
		t.Fatalf("public HLS child URL leaked provider identity: %q", publicURL)
	}
	parsed, err := url.Parse(publicURL)
	if err != nil {
		t.Fatal(err)
	}
	ref := parsed.Query().Get("ref")
	if ref == "" {
		t.Fatalf("public HLS child URL omitted its opaque reference: %q", publicURL)
	}
	if resolved, ok := server.resolveLiveTVHLSReference(channel, source, ref, grant); !ok || resolved != item {
		t.Fatalf("reference did not resolve under its exact fences: resolved=%q ok=%v", resolved, ok)
	}
	for _, mismatch := range []struct{ channel, source, grant string }{
		{"channel-2", source, grant},
		{channel, "https://provider.example/live/replacement.m3u8", grant},
		{channel, source, "ptc_mg_other"},
	} {
		if _, ok := server.resolveLiveTVHLSReference(mismatch.channel, mismatch.source, ref, mismatch.grant); ok {
			t.Fatalf("reference crossed a channel/source/grant fence: %#v", mismatch)
		}
	}
}

func TestLiveTVHLSReferenceExpiryIsTerminal(t *testing.T) {
	server := &Server{liveTVHLSReferences: map[string]liveTVHLSReference{}}
	publicURL := server.issueLiveTVHLSReference("channel", "https://provider.example/master.m3u8", "https://provider.example/segment.ts", "source", "ptc_mg_current")
	parsed, _ := url.Parse(publicURL)
	ref := parsed.Query().Get("ref")
	server.liveTVHLSReferenceMu.Lock()
	reference := server.liveTVHLSReferences[ref]
	reference.expiresAt = time.Now().UTC().Add(-time.Second)
	server.liveTVHLSReferences[ref] = reference
	server.liveTVHLSReferenceMu.Unlock()
	if _, ok := server.resolveLiveTVHLSReference("channel", "https://provider.example/master.m3u8", ref, "ptc_mg_current"); ok {
		t.Fatal("expired reference remained redeemable")
	}
}
