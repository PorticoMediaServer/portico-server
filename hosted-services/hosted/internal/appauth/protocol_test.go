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
