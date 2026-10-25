package mailoutbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureDelivery() Delivery {
	return Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: Registration, ResourceID: "registration_fixture", Revision: 1, ExpiresAt: time.Date(2026, 9, 6, 12, 30, 0, 0, time.UTC), Payload: Payload{To: "recipient@example.test", Code: "012345"}}, CreatedAt: time.Date(2026, 9, 6, 12, 20, 0, 0, time.UTC), Lease: "lease", Attempts: 1}
}
func TestRenderStableSafeMultipart(t *testing.T) {
	d := fixtureDelivery()
	first, e := Render(d, "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	second, e := Render(d, "sender@example.test", "")
	if e != nil || !bytes.Equal(first, second) {
		t.Fatal("retry changes message bytes")
	}
	message, e := mail.ReadMessage(bytes.NewReader(first))
	if e != nil {
		t.Fatal(e)
	}
	if message.Header.Get("Message-ID") != "<mail_fixture_0123456789@example.test>" {
		t.Fatal("message identity lost")
	}
	_, params, e := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if e != nil {
		t.Fatal(e)
	}
	parts := multipart.NewReader(message.Body, params["boundary"])
	n := 0
	for {
		part, e := parts.NextRawPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		text, e := io.ReadAll(quotedprintable.NewReader(part))
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Contains(text, []byte("#token=")) || bytes.Contains(text, []byte("?token=")) {
			t.Fatal("code missing or leaked in link")
		}
		n++
	}
	if n != 2 {
		t.Fatal("text and HTML required")
	}
	d.Payload.To = "person@example.test\r\nBcc: other@example.test"
	if _, e = Render(d, "sender@example.test", ""); e == nil {
		t.Fatal("header injection")
	}
	d = fixtureDelivery()
	d.Payload.Code = "<script>alert(1)</script>"
	raw, e := Render(d, "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	message, _ = mail.ReadMessage(bytes.NewReader(raw))
	_, params, _ = mime.ParseMediaType(message.Header.Get("Content-Type"))
	parts = multipart.NewReader(message.Body, params["boundary"])
	_, _ = parts.NextRawPart()
	part, _ := parts.NextRawPart()
	html, _ := io.ReadAll(quotedprintable.NewReader(part))
	if strings.Contains(string(html), "<script>") {
		t.Fatal("unescaped HTML")
	}
}

type fakeRepo struct {
	d                *Delivery
	eligible         bool
	eligibilityError error
	result           string
	terminal         bool
	finished         int
}

func (r *fakeRepo) ClaimMail(context.Context) (*Delivery, error) { return r.d, nil }
func (r *fakeRepo) MailEligible(context.Context, Delivery) (bool, error) {
	return r.eligible, r.eligibilityError
}
func (r *fakeRepo) FinishMail(_ context.Context, _ Delivery, result string, terminal bool) error {
	r.result = result
	r.terminal = terminal
	r.finished++
	return nil
}

type fakeSender struct {
	calls int
	err   error
}

func (s *fakeSender) FromAddress() string                        { return "sender@example.test" }
func (s *fakeSender) ReplyToAddress() string                     { return "" }
func (s *fakeSender) Send(context.Context, string, []byte) error { s.calls++; return s.err }
func TestWorkerChecksEligibilityAndClassifiesDelivery(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		eligible                    bool
		sendError, errorEligibility error
		result                      string
		terminal                    bool
		calls                       int
	}{
		{"cancelled before send", false, nil, nil, "ineligible", true, 0},
		{"lookup unavailable", false, nil, errors.New("database"), "unavailable", false, 0},
		{"accepted", true, nil, nil, "sent", true, 1},
		{"ambiguous data reply", true, io.EOF, nil, "unavailable", false, 1},
		{"temporary SMTP", true, &textproto.Error{Code: 451, Msg: "temporary"}, nil, "unavailable", false, 1},
		{"permanent SMTP", true, &textproto.Error{Code: 550, Msg: "private detail"}, nil, "rejected", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixtureDelivery()
			repo := &fakeRepo{d: &d, eligible: tc.eligible, eligibilityError: tc.errorEligibility}
			sender := &fakeSender{err: tc.sendError}
			worker := Worker{Repo: repo, Sender: sender}
			if !worker.Once(context.Background()) || repo.finished != 1 || repo.result != tc.result || repo.terminal != tc.terminal || sender.calls != tc.calls {
				t.Fatalf("wrong outcome %#v %#v", repo, sender)
			}
		})
	}
}
func TestSMTPConfigurationPrivateAndTLSMandatory(t *testing.T) {
	if config, e := LoadSMTP(""); e != nil || config != nil {
		t.Fatal("absent SMTP should be explicit, nonfatal configuration")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "smtp.json")
	raw := []byte(`{"host":"smtp.example.test","port":587,"tls":"starttls","from":"sender@example.test"}`)
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadSMTP(path); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadSMTP(path); e == nil {
		t.Fatal("world-readable config accepted")
	}
	c := SMTPConfig{Host: "smtp.example.test", Port: 25, From: "sender@example.test", TLS: "none"}
	if e := c.Send(context.Background(), "recipient@example.test", nil); e == nil {
		t.Fatal("plaintext configuration accepted")
	}
	c.TLS = "starttls"
	c.Username = "account"
	if c.Validate() == nil {
		t.Fatal("partial authentication accepted")
	}
}

type progressedRepo struct{ fakeRepo }

func (*progressedRepo) ClaimMail(context.Context) (*Delivery, error) { return nil, ErrQueueProgress }
func TestDiscardedQueueWorkKeepsLaneBusy(t *testing.T) {
	w := Worker{Repo: &progressedRepo{}}
	if !w.Once(context.Background()) {
		t.Fatal("discarded row treated as idle queue")
	}
	w.Repo = &fakeRepo{}
	if w.Once(context.Background()) {
		t.Fatal("empty queue treated as busy")
	}
}

type queueProgressRepo struct {
	claimed  atomic.Int32
	finished chan string
}

func (r *queueProgressRepo) ClaimMail(context.Context) (*Delivery, error) {
	n := r.claimed.Add(1)
	if n <= 6 {
		return nil, ErrQueueProgress
	}
	if n == 7 {
		d := fixtureDelivery()
		return &d, nil
	}
	return nil, nil
}
func (*queueProgressRepo) MailEligible(context.Context, Delivery) (bool, error) { return true, nil }
func (r *queueProgressRepo) FinishMail(_ context.Context, _ Delivery, result string, _ bool) error {
	r.finished <- result
	return nil
}
func TestWorkerDrainsBehindDiscardedRowsWithoutIdleDelay(t *testing.T) {
	repo := &queueProgressRepo{finished: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Worker{Repo: repo, Sender: &fakeSender{}, SafetyInterval: time.Hour}).Run(ctx)
	}()
	select {
	case result := <-repo.finished:
		if result != "sent" {
			t.Fatal(result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("valid mail behind discarded rows slept as idle")
	}
	cancel()
	<-done
}

func TestEveryEmailUsesCentralLinkContract(t *testing.T) {
	for _, kind := range []Kind{Registration, OIDCContact, PasswordReset, EmailChange} {
		d := Delivery{Intent: Intent{ID: "mail_1234567890123456", Kind: kind, ResourceID: "resource_1234567890", Revision: 1, ExpiresAt: time.Now().Add(time.Hour), Payload: Payload{To: "recipient@example.test", Code: "secret_abcdefghijklmnopqrstuv"}}, CreatedAt: time.Now()}
		raw, e := Render(d, "portico@example.test", "")
		if e != nil {
			t.Fatal(kind, e)
		}
		if !bytes.Contains(raw, []byte("multipart/alternative")) {
			t.Fatal("missing plain text alternative")
		}
		link := Link(d)
		u, e := url.Parse(link)
		if e != nil || u.Scheme != "https" || u.Host != "web.getportico.tv" {
			t.Fatal(kind, link)
		}
		// Every mail kind has its own Link Contract route and fragment; none
		// falls back to the bare account page.
		want := map[Kind][2]string{
			Registration:  {VerifyEmailPath, "token=" + VerificationToken(Registration, d.ResourceID, d.Payload.Code)},
			OIDCContact:   {VerifyEmailPath, "token=" + VerificationToken(OIDCContact, d.ResourceID, d.Payload.Code)},
			EmailChange:   {VerifyEmailPath, "token=" + VerificationToken(EmailChange, d.ResourceID, d.Payload.Code)},
			PasswordReset: {ResetPath, "token=" + d.Payload.Code},
		}[kind]
		if u.Path != want[0] || u.Fragment != want[1] || u.RawQuery != "" {
			t.Fatalf("%s: %s, want %s#%s", kind, link, want[0], want[1])
		}
		if back, resource, code, err := ParseVerificationToken(strings.TrimPrefix(u.Fragment, "token=")); u.Path == VerifyEmailPath && (err != nil || back != kind || resource != d.ResourceID || code != d.Payload.Code) {
			t.Fatal("verification token does not round-trip", kind, err)
		}
	}
	if ClaimPath != "/claim" || TogetherJoinPath != "/together/join" || DevicePath != "/device" || MediaPath != "/media/" {
		t.Fatal("public link route contract")
	}
}

func decodeMailParts(t *testing.T, raw []byte) (message *mail.Message, text, html string) {
	t.Helper()
	var e error
	message, e = mail.ReadMessage(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	_, params, e := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if e != nil {
		t.Fatal(e)
	}
	parts := multipart.NewReader(message.Body, params["boundary"])
	got := []string{}
	for {
		part, e := parts.NextRawPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(quotedprintable.NewReader(part))
		if e != nil {
			t.Fatal(e)
		}
		got = append(got, string(body))
	}
	if len(got) != 2 {
		t.Fatal("text and HTML required")
	}
	return message, got[0], got[1]
}

// MAIL-05: code emails carry the code in the subject, open with the code
// before any instructions, show a hidden preheader with the real TTL, and end
// the text part with the origin-bound autofill line.
func TestCodeEmailsPutCodeFirst(t *testing.T) {
	for _, kind := range []Kind{Registration, OIDCContact, EmailChange} {
		created := time.Date(2026, 9, 6, 12, 20, 0, 0, time.UTC)
		d := Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: kind, ResourceID: "resource_1234567890", Revision: 1, ExpiresAt: created.Add(10 * time.Minute), Payload: Payload{To: "recipient@example.test", Code: "482913"}}, CreatedAt: created}
		raw, e := Render(d, "sender@example.test", "")
		if e != nil {
			t.Fatal(kind, e)
		}
		message, text, html := decodeMailParts(t, raw)
		if got := message.Header.Get("Subject"); got != "Your Portico code is 482913" {
			t.Fatalf("%s subject %q", kind, got)
		}
		wantPreheader := "Enter it to confirm your email. It expires in 10 minutes."
		if !strings.Contains(html, wantPreheader) || !strings.Contains(html, "display:none") {
			t.Fatalf("%s missing hidden preheader", kind)
		}
		if !strings.HasPrefix(strings.TrimLeft(text, "\r\n"), "482913") {
			t.Fatalf("%s text does not open with the code: %q", kind, text)
		}
		codeIdxText := strings.Index(text, "482913")
		for _, marker := range []string{"Enter it to confirm", "Valid until", "http"} {
			if i := strings.Index(text, marker); i >= 0 && i < codeIdxText {
				t.Fatalf("%s instruction %q precedes the code", kind, marker)
			}
		}
		codeIdxHTML := strings.Index(html, "482913")
		for _, marker := range []string{"Confirm your", "Valid until", "Open Portico"} {
			if i := strings.Index(html, marker); i >= 0 && i < codeIdxHTML {
				t.Fatalf("%s HTML instruction %q precedes the code", kind, marker)
			}
		}
		lines := strings.Split(strings.TrimRight(text, "\r\n"), "\n")
		if got := strings.TrimSpace(lines[len(lines)-1]); got != "@"+WebHost()+" #482913" {
			t.Fatalf("%s last text line %q", kind, got)
		}
		if WebHost() != "web.getportico.tv" {
			t.Fatalf("web host %q", WebHost())
		}
	}
}

// MAIL-05: the preheader lifetime comes from the code TTL, not a constant.
func TestCodePreheaderUsesRealTTL(t *testing.T) {
	created := time.Date(2026, 9, 6, 12, 20, 0, 0, time.UTC)
	d := Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: Registration, ResourceID: "resource_1234567890", Revision: 1, ExpiresAt: created.Add(7 * time.Minute), Payload: Payload{To: "recipient@example.test", Code: "771204"}}, CreatedAt: created}
	raw, e := Render(d, "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	_, text, html := decodeMailParts(t, raw)
	if !strings.Contains(text, "It expires in 7 minutes.") || !strings.Contains(html, "It expires in 7 minutes.") {
		t.Fatalf("preheader does not use the 7-minute TTL: %q", text)
	}
	if strings.Contains(text, "It expires in 10 minutes.") {
		t.Fatal("hard-coded 10-minute preheader")
	}
}

// MAIL-07: header basics. Date, MIME-Version and multipart/alternative are
// always present; Message-ID uses the sending domain; Reply-To is omitted
// unless configured; account mail never includes List-Unsubscribe.
func TestMailHeaderBasics(t *testing.T) {
	created := time.Date(2026, 9, 6, 12, 20, 0, 0, time.UTC)
	mk := func(kind Kind) Delivery {
		return Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: kind, ResourceID: "resource_1234567890", Revision: 1, ExpiresAt: created.Add(time.Hour), Payload: Payload{To: "recipient@example.test", Code: "482913"}}, CreatedAt: created}
	}
	raw, e := Render(mk(Registration), "accounts@send.example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	message, _, _ := decodeMailParts(t, raw)
	if message.Header.Get("Date") == "" || message.Header.Get("MIME-Version") != "1.0" {
		t.Fatal("Date/MIME-Version missing")
	}
	if ct := message.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/alternative") {
		t.Fatalf("Content-Type %q", ct)
	}
	if got := message.Header.Get("Message-ID"); got != "<mail_fixture_0123456789@send.example.test>" {
		t.Fatalf("Message-ID %q does not use the sending domain", got)
	}
	if message.Header.Get("Reply-To") != "" {
		t.Fatal("Reply-To invented without config")
	}
	if message.Header.Get("List-Unsubscribe") != "" {
		t.Fatal("List-Unsubscribe on a code email")
	}
	raw, e = Render(mk(Registration), "accounts@send.example.test", "support@example.test")
	if e != nil {
		t.Fatal(e)
	}
	message, _, _ = decodeMailParts(t, raw)
	if message.Header.Get("Reply-To") == "" {
		t.Fatal("configured Reply-To omitted")
	}
	if message.Header.Get("List-Unsubscribe") != "" {
		t.Fatal("unexpected List-Unsubscribe on account mail")
	}
	if _, e = Render(mk(Registration), "sender@example.test", "not-an-address"); e == nil {
		t.Fatal("invalid reply-to accepted")
	}
}

func TestInvitationEmailLiteralIsRetired(t *testing.T) {
	d := Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: Kind("invitation"), ResourceID: "fixture", ExpiresAt: time.Now().Add(time.Hour), Payload: Payload{To: "recipient@example.test", Code: "private"}}, CreatedAt: time.Now()}
	if d.Intent.Validate() == nil {
		t.Fatal("retired invitation kind accepted")
	}
	if raw, e := Render(d, "sender@example.test", ""); e == nil || len(raw) > 0 {
		t.Fatal("retired invitation rendered")
	}
	if strings.Contains(Link(d), "invite=") {
		t.Fatal("retired invitation link produced")
	}
}

// Frame 18 (design/ui-v1 account.html#18, notes/account.md 18): the drawn code
// email is one 560 px column with a wordmark chip and a rounded card, the code
// set large and first with its real expiry on the line below it, the white
// button (ink on light) instead of today's blue one, and a plain-text twin
// that keeps the preheader sentence rather than the app-only line.
func TestCodeEmailMatchesDrawnFrame18(t *testing.T) {
	created := time.Date(2026, 9, 6, 12, 20, 0, 0, time.UTC)
	d := Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: Registration, ResourceID: "resource_1234567890", Revision: 1, ExpiresAt: created.Add(10 * time.Minute), Payload: Payload{To: "recipient@example.test", Code: "482913"}}, CreatedAt: created}
	raw, e := Render(d, "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	_, text, html := decodeMailParts(t, raw)
	for _, want := range []string{
		`max-width:560px`,
		`width:128px;background:#071018;padding:12px;border-radius:8px`,
		`src="https://getportico.tv/brand/portico-wordmark-white.svg"`,
		`padding:40px;background:#10212d;border:1px solid #28404f;border-radius:16px`,
		`font-size:46px;line-height:1;font-weight:800;letter-spacing:.14em`,
		`class="exp" style="font-size:15px;font-weight:700;line-height:1.7;margin:0 0 26px;color:#5cb3e2">Enter it in Portico. It expires in 10 minutes.`,
		`<h1 style="margin:0 0 18px;font-size:28px;line-height:1.25;font-weight:800;letter-spacing:-.02em">Verify your email</h1>`,
		`<p style="font-size:16px;line-height:1.7;margin:0 0 28px">Confirm your email address to finish creating your Portico Account.</p>`,
		`background:#edf5fa;color:#071018;padding:14px 24px;border-radius:8px;font-weight:800;font-size:16px`,
		`color:#96a9b5;margin:32px 0 0`,
		// The light frame: ink button, lighter card edge, accent on the expiry.
		`.exp{color:#1c6f9e!important}`,
		`.btn{background:#071018!important;color:#edf5fa!important}`,
		`.small{color:#28404f!important}`,
		`border-color:#d3dee6!important`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("drawn frame 18 missing %q", want)
		}
	}
	if strings.Contains(html, "#409fd4") {
		t.Fatal("today's blue button survived")
	}
	last := -1
	for _, marker := range []string{">482913<", "Enter it in Portico.", ">Verify your email<"} {
		i := strings.Index(html, marker)
		if i <= last {
			t.Fatalf("drawn order broken at %q", marker)
		}
		last = i
	}
	if !strings.Contains(text, "Enter it to confirm your email. It expires in 10 minutes.") || strings.Contains(text, "Enter it in Portico.") {
		t.Fatalf("plain-text twin wording changed: %q", text)
	}
	// The reset email keeps its own branch: no code, no expiry line.
	reset := d
	reset.Kind = PasswordReset
	reset.Payload.Code = "reset_token_fixture"
	rawReset, e := Render(reset, "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	_, resetText, resetHTML := decodeMailParts(t, rawReset)
	if strings.Contains(resetHTML, `class="exp"`) || strings.Contains(resetHTML, "Enter it in Portico.") {
		t.Fatal("code line on the reset email")
	}
	if !strings.Contains(resetText, "Use this link to reset your password.") {
		t.Fatal("reset plain text changed")
	}
}

// Frame 18 panel 4: the new-address message keeps the drawn heading, body and
// button, and the code block escapes markup while the text twin stays literal.
func TestCodeEmailCopyAndEscaping(t *testing.T) {
	d := fixtureDelivery()
	d.Kind = EmailChange
	d.Payload.Code = `<i>482913</i>`
	raw, e := Render(d, "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	_, text, html := decodeMailParts(t, raw)
	for _, want := range []string{"Confirm your new email", "Your previous email stays active until you confirm this address.", "Confirm email"} {
		if !strings.Contains(html, want) || !strings.Contains(text, want) {
			t.Fatalf("drawn new-address copy missing %q", want)
		}
	}
	if strings.Contains(html, "<i>482913</i>") || !strings.Contains(html, "&lt;i&gt;482913&lt;/i&gt;") {
		t.Fatal("code block let markup through")
	}
	if !strings.Contains(text, "<i>482913</i>") {
		t.Fatal("text part altered the code")
	}
}

// Frame 19 (design/ui-v1 account.html#19, notes/account.md 19 and mail item 4):
// the reset email carries no code. It gains the drawn preheader and one plain
// line with the link's real lifetime and the didn't-ask sentence; the subject,
// body, button and never-share footer stay as they are.
func TestResetEmailMatchesDrawnFrame19(t *testing.T) {
	created := time.Date(2026, 10, 22, 14, 32, 7, 0, time.UTC)
	mk := func(mins int) Delivery {
		return Delivery{Intent: Intent{ID: "mail_fixture_0123456789", Kind: PasswordReset, ResourceID: "resource_1234567890", Revision: 1, ExpiresAt: created.Add(time.Duration(mins) * time.Minute), Payload: Payload{To: "recipient@example.test", Code: "reset_token_fixture"}}, CreatedAt: created}
	}
	raw, e := Render(mk(15), "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	message, text, html := decodeMailParts(t, raw)
	if got := message.Header.Get("Subject"); got != "Reset your Portico password" {
		t.Fatalf("subject %q", got)
	}
	note := "The link works once and expires in 15 minutes. If you didn’t ask to reset your password, ignore this message and your password stays as it is."
	for _, want := range []string{
		`<p class="small" style="font-size:14px;line-height:1.6;margin:22px 0 0;color:#bdccd6">` + note + `</p>`,
		`display:none;max-height:0;overflow:hidden`,
		"This link works once and expires in 15 minutes.",
		"Choose a new password",
		"Use this link to reset your password. If two-step verification is on, you’ll also need your authenticator app or a recovery code.",
		">Reset password<",
		"Valid until Thu, 22 Oct 2026 14:47:07 UTC.",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("drawn frame 19 missing %q", want)
		}
	}
	if strings.Contains(html, `class="exp"`) || !strings.Contains(text, "Valid until Thu, 22 Oct 2026 14:47:07 UTC.") {
		t.Fatal("reset email took the code branch")
	}
	last := -1
	for _, marker := range []string{"Reset password: https://", "Valid until ", note, "Never share this message."} {
		i := strings.Index(text, marker)
		if i <= last {
			t.Fatalf("drawn text order broken at %q", marker)
		}
		last = i
	}
	// The drawn line and preheader use the real lifetime, not the 15 drawn as text.
	raw, e = Render(mk(12), "sender@example.test", "")
	if e != nil {
		t.Fatal(e)
	}
	_, text, html = decodeMailParts(t, raw)
	if !strings.Contains(html, "This link works once and expires in 12 minutes.") || !strings.Contains(text, "The link works once and expires in 12 minutes.") {
		t.Fatalf("reset lifetime not from the delivery: %q", text)
	}
	if strings.Contains(text, "15 minutes") || strings.Contains(html, "15 minutes") {
		t.Fatal("hard-coded 15-minute reset lifetime")
	}
}
