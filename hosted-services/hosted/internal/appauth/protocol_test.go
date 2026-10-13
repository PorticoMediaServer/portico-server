package appauth

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestRedirectAddresses(t *testing.T) {
	for _, raw := range []string{"https://client.example/finish", "https://client.example:8443/finish?channel=tv", "https://192.168.1.2/finish", "http://127.0.0.1:50123/finish", "http://[::1]:50123/finish", "my-player://signin/finish", "tv.example.player:/signin"} {
		if !ValidRedirect(raw) {
			t.Errorf("refused %q", raw)
		}
	}
	for _, raw := range []string{"http://example.com/finish", "http://localhost:80/finish", "http://127.0.0.1/finish", "http://[::ffff:127.0.0.1]:80/finish", "https://user:pass@client.example/finish", "https://client.example/#x", "https://client.example/#", "https://client.example/finish?code=old", "https://client.example/finish?%73tate=x", "https://client.example/finish?error=x", "https://client.example:0/finish", "https://client.example:0443/finish", "https://client.example:/finish", "https://a\\b.example/finish", "https://client.example./finish", "https://a..example/finish", "https://[fe80::1%25en0]/finish", "https://client.example/?bad=%zz", "javascript:alert(1)", "javascript://client/x", "data://client/x", "file:///tmp/x", "intent://client/x", "tel://client/x", "my-player:signin", "//client.example/finish", "https://client.example/\r\nLocation:evil"} {
		if ValidRedirect(raw) {
			t.Errorf("accepted %q", raw)
		}
	}
}
func TestPKCEAndExactReturn(t *testing.T) {
	// RFC 7636 S256 vector.
	if Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatal("S256")
	}
	q := Request{ServerID: "srv_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32)), Challenge: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), RedirectURI: "https://client.example/callback?original=a%26b", State: strings.Repeat("state", 5), AppName: "My Player", CodeChallenge: Challenge(strings.Repeat("v", 43))}
	if e := q.Validate(); e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(ReturnLocation(q, "pac_test"))
	if u.Query().Get("state") != q.State || u.Query().Get("code") != "pac_test" || u.Query().Get("original") != "a&b" {
		t.Fatal(u)
	}
	u, _ = url.Parse(ReturnLocation(q, ""))
	if u.Query().Get("error") != "access_denied" || u.Query().Get("code") != "" {
		t.Fatal(u)
	}
	for _, name := range []string{"", strings.Repeat("x", 101), "Fake\nApp", "Fake\u202eApp", " Fake App"} {
		bad := q
		bad.AppName = name
		if bad.Validate() == nil {
			t.Fatal("app name", name)
		}
	}
	bad := q
	bad.State = "small"
	if bad.Validate() == nil {
		t.Fatal("weak state")
	}
	bad = q
	bad.CodeChallenge = "plain-verifier"
	if bad.Validate() == nil {
		t.Fatal("plain PKCE")
	}
}

// A local server issues 24 random bytes as its sign-in challenge. The request
// must accept exactly what the assertion issuer and the server's verifier do.
func TestServerIssuedChallengeSizes(t *testing.T) {
	q := Request{ServerID: "srv_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32)), RedirectURI: "https://client.example/callback", State: strings.Repeat("state", 5), AppName: "My Player", CodeChallenge: Challenge(strings.Repeat("v", 43))}
	for _, size := range []int{16, 24, 32, 96} {
		q.Challenge = base64.RawURLEncoding.EncodeToString(make([]byte, size))
		if e := q.Validate(); e != nil {
			t.Errorf("refused a %d-byte challenge", size)
		}
	}
	for _, bad := range []string{"", base64.RawURLEncoding.EncodeToString(make([]byte, 15)), base64.RawURLEncoding.EncodeToString(make([]byte, 97)), base64.URLEncoding.EncodeToString(make([]byte, 16)), strings.Repeat("A", 21) + "B", strings.Repeat("*", 32)} {
		q.Challenge = bad
		if q.Validate() == nil {
			t.Errorf("accepted challenge %q", bad)
		}
	}
}

// The person is sent to the address they were shown, with the answer added
// after it: nothing in the address itself is reordered or rewritten.
func TestReturnKeepsTheAddressAsShown(t *testing.T) {
	state := "st&te=1 " + strings.Repeat("s", 16)
	answer := "code=pac_one&state=st%26te%3D1+" + strings.Repeat("s", 16)
	for raw, want := range map[string]string{
		"https://client.example/finish?z=1&a=2&flag":   "https://client.example/finish?z=1&a=2&flag&" + answer,
		"HTTPS://Client.Example/a%2Fb?x=%7e":           "HTTPS://Client.Example/a%2Fb?x=%7e&" + answer,
		"http://127.0.0.1:49152/finish":                "http://127.0.0.1:49152/finish?" + answer,
		"my-player://signin":                           "my-player://signin?" + answer,
		"tv.example.player:/signin":                    "tv.example.player:/signin?" + answer,
		"https://client.example/finish?return=a%3Fb=c": "https://client.example/finish?return=a%3Fb=c&" + answer,
	} {
		if !ValidRedirect(raw) {
			t.Fatalf("refused %q", raw)
		}
		if got := ReturnLocation(Request{RedirectURI: raw, State: state}, "pac_one"); got != want {
			t.Errorf("return for %q:\n got %s\nwant %s", raw, got, want)
		}
	}
	if got := ReturnLocation(Request{RedirectURI: "my-player://signin?from=tv", State: "s"}, ""); got != "my-player://signin?from=tv&error=access_denied&state=s" {
		t.Error("denial", got)
	}
}
