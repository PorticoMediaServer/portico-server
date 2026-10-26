// Package mailoutbox owns typed mail intents and bounded SMTP delivery. Domain
// repositories enqueue in the same transaction as the fact authorizing a mail.
package mailoutbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Kind string

const (
	Registration  Kind = "registration"
	OIDCContact   Kind = "oidc_contact"
	PasswordReset Kind = "password_reset"
	EmailChange   Kind = "email_change"
)

// Account email is limited to registration/contact verification, password reset
// and email change. People invitations remain links and in-app facts.
// ResourceID/Revision are authoritative eligibility fences, not template data.
type Intent struct {
	ID           string
	Kind         Kind
	ResourceID   string
	Revision     int64
	AccountID    string
	AccountEpoch int64
	ExpiresAt    time.Time
	Payload      Payload
}
type Payload struct {
	To    string `json:"to"`
	Code  string `json:"code,omitempty"`
	Event string `json:"event,omitempty"`
}
type Delivery struct {
	Intent
	Lease     string
	Attempts  int
	CreatedAt time.Time
}

// ErrQueueProgress means a row was durably discarded; the lane should keep draining.
var ErrQueueProgress = errors.New("mail queue progressed")

type Repository interface {
	ClaimMail(context.Context) (*Delivery, error)
	MailEligible(context.Context, Delivery) (bool, error)
	FinishMail(context.Context, Delivery, string, bool) error
}
type Sender interface {
	Send(context.Context, string, []byte) error
	FromAddress() string
	// ReplyToAddress is the optional Reply-To config value; empty omits it.
	ReplyToAddress() string
}

func ValidAddress(value string) bool {
	if len(value) < 3 || len(value) > 320 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	a, e := mail.ParseAddress(value)
	return e == nil && a.Address == value && a.Name == ""
}

func (i Intent) Validate() error {
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`).MatchString(i.ID) || len(i.ResourceID) > 128 || i.Revision < 0 || !ValidAddress(i.Payload.To) || len(i.Payload.Code) > 160 || len(i.Payload.Event) > 160 || i.ExpiresAt.IsZero() {
		return errors.New("invalid mail intent")
	}
	switch i.Kind {
	case Registration, OIDCContact, PasswordReset, EmailChange:
		return nil
	}
	return errors.New("unknown mail kind")
}
func MarshalPayload(p Payload) (string, error) { b, e := json.Marshal(p); return string(b), e }

func isCodeKind(k Kind) bool {
	return k == Registration || k == OIDCContact || k == EmailChange
}

// ttlMinutes is the real link lifetime from the intent's expiry and the
// outbox creation time (MAIL-05: never a hard-coded number). Both are set at
// enqueue time from the challenge's expires_at, including resends shortened by
// LEAST(expires_at, ...). A zero or inverted pair falls back to the kind's
// documented lifetime so a malformed delivery still renders; production rows
// always carry both.
func ttlMinutes(d Delivery, fallback int) int {
	if d.CreatedAt.IsZero() || d.ExpiresAt.IsZero() || !d.ExpiresAt.After(d.CreatedAt) {
		return fallback
	}
	mins := int((d.ExpiresAt.Sub(d.CreatedAt) + 30*time.Second) / time.Minute)
	if mins < 1 {
		return 1
	}
	return mins
}

// codeTTLMinutes is the verification code lifetime (10 minutes drawn).
func codeTTLMinutes(d Delivery) int { return ttlMinutes(d, 10) }

func minutePhrase(mins int) string {
	if mins == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", mins)
}

// sendingDomain is the domain of the configured sender, for Message-ID
// (MAIL-07: with the sending domain, never a literal).
func sendingDomain(from string) string {
	a, e := mail.ParseAddress(from)
	if e != nil {
		return ""
	}
	parts := strings.Split(a.Address, "@")
	if len(parts) != 2 || parts[1] == "" {
		return ""
	}
	return strings.ToLower(parts[1])
}

// brandWordmarkPNG is the canonical 512x117 brand raster, carried as the mail's
// related image part so the wordmark needs no network fetch.
//
//go:embed brand-wordmark.png
var brandWordmarkPNG []byte

// Tokens are confined to fragments. Mail retries keep the exact link and ID.
func Render(d Delivery, from, replyTo string) ([]byte, error) {
	if e := d.Intent.Validate(); e != nil {
		return nil, e
	}
	if !ValidAddress(from) {
		return nil, errors.New("invalid sender")
	}
	if replyTo != "" && !ValidAddress(replyTo) {
		return nil, errors.New("invalid reply-to")
	}
	domain := sendingDomain(from)
	if domain == "" {
		return nil, errors.New("invalid sender domain")
	}
	subject, heading, body, button := "Portico account security", "Your Portico Account", "A security setting changed on your Portico Account.", "Review account"
	switch d.Kind {
	case Registration:
		subject = "Verify your Portico Account"
		heading = "Verify your email"
		body = "Confirm your email address to finish creating your Portico Account."
		button = "Verify email"
	case OIDCContact:
		subject = "Verify your Portico contact email"
		heading = "Verify your contact email"
		body = "Confirm the contact address for your Portico Account. You can open this link on any device, then finish on the one where you started."
		button = "Verify email"
	case PasswordReset:
		subject = "Reset your Portico password"
		heading = "Choose a new password"
		body = "Use this link to reset your password. If two-step verification is on, you’ll also need your authenticator app or a recovery code."
		button = "Reset password"
	case EmailChange:
		subject = "Verify your new Portico email"
		heading = "Confirm your new email"
		body = "Your previous email stays active until you confirm this address."
		button = "Confirm email"
	}
	link := Link(d)
	expiry := d.ExpiresAt.UTC().Format(time.RFC1123)
	// A33: verification mails also show the code, for typing into the device
	// that started the flow when the link is opened somewhere else.
	code := ""
	switch d.Kind {
	case Registration, OIDCContact, EmailChange:
		code = d.Payload.Code
	}
	// MAIL-05: code emails put the code in the subject, open the body with
	// the code before any instructions, and end the text part with the
	// origin-bound autofill line. The preheader lifetime comes from the
	// code's real TTL.
	preheader := ""
	codeNote := ""
	note := ""
	if d.Kind == PasswordReset {
		// Frame 19: the reset email leads with a preheader and one plain line
		// carrying the link's real lifetime and the didn't-ask sentence.
		mins := ttlMinutes(d, 15)
		phrase := minutePhrase(mins)
		preheader = "This link works once and expires in " + phrase + "."
		note = "The link works once and expires in " + phrase + ". If you didn’t ask to reset your password, ignore this message and your password stays as it is."
	}
	text := ""
	if isCodeKind(d.Kind) && code != "" {
		mins := codeTTLMinutes(d)
		phrase := minutePhrase(mins)
		preheader = "Enter it to confirm your email. It expires in " + phrase + "."
		// Frame 18 draws one line under the code, worded for the app, with the
		// same real TTL as the hidden preheader; the plain-text twin keeps the
		// preheader sentence.
		codeNote = "Enter it in Portico. It expires in " + phrase + "."
		autofill := "@" + WebHost() + " #" + code
		text = code + "\n\n" + preheader + "\n\n" + heading + "\n\n" + body + "\n\n" + button + ": " + link + "\nValid until " + expiry + ".\n\nNever share this message. If you did not request a verification or password reset, ignore that request.\n" + autofill + "\n"
		subject = "Your Portico code is " + code
	} else {
		codeLine := ""
		if code != "" {
			codeLine = "Or enter this code in Portico: " + code + "\n"
		}
		text = heading + "\n\n" + body + "\n\n" + button + ": " + link + "\n" + codeLine + "Valid until " + expiry + ".\n\n"
		if note != "" {
			text += note + "\n\n"
		}
		text += "Never share this message. If you did not request a verification or password reset, ignore that request.\n"
	}
	var html bytes.Buffer
	e := template.Must(template.New("mail").Parse(`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark light"><style>@media(prefers-color-scheme:light){body,.surface,.em{background:#edf5fa!important;color:#071018!important}.em{box-shadow:none!important}.card{background:#ffffff!important;color:#071018!important;border-color:#d3dee6!important}.exp{color:#1c6f9e!important}.btn{background:#071018!important;color:#edf5fa!important}.small{color:#28404f!important}.muted{color:#28404f!important}}@media(max-width:480px){.card{padding:24px!important}}</style></head><body style="margin:0;background:#071018;color:#edf5fa;font-family:Manrope,Arial,sans-serif">{{if .Preheader}}<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all;">{{.Preheader}}</div>{{end}}<table role="presentation" width="100%" class="surface" style="background:#071018;border-collapse:collapse"><tr><td align="center" style="padding:0"><table role="presentation" width="100%" style="max-width:560px;border-collapse:collapse"><tr><td class="em" style="padding:32px 16px 36px;background:#071018;border-radius:14px;box-shadow:inset 0 0 0 1px rgba(237,245,250,.06)"><img class="wmk" src="{{.Wordmark}}" alt="Portico" width="128" height="29" style="display:block;width:128px;height:auto;background:#071018;padding:12px;border-radius:8px;margin:0 0 24px"><table role="presentation" width="100%" style="border-collapse:separate;border-spacing:0"><tr><td class="card" style="padding:40px;background:#10212d;border:1px solid #28404f;border-radius:16px">{{if .Code}}<div style="font-size:46px;line-height:1;font-weight:800;letter-spacing:.14em;font-variant-numeric:tabular-nums;margin:0 0 8px;word-break:break-all;">{{.Code}}</div>{{end}}{{if .CodeNote}}<p class="exp" style="font-size:15px;font-weight:700;line-height:1.7;margin:0 0 26px;color:#5cb3e2">{{.CodeNote}}</p>{{end}}<h1 style="margin:0 0 18px;font-size:28px;line-height:1.25;font-weight:800;letter-spacing:-.02em">{{.Heading}}</h1><p style="font-size:16px;line-height:1.7;margin:0 0 28px">{{.Body}}</p><a class="btn" href="{{.Link}}" style="display:inline-block;background:#edf5fa;color:#071018;padding:14px 24px;border-radius:8px;font-weight:800;font-size:16px;text-decoration:none">{{.Button}}</a>{{if .CodeFallback}}<p style="font-size:14px;line-height:1.6;margin:22px 0 0">Or enter this code in Portico:<br><span style="font-size:22px;font-weight:700;letter-spacing:.14em;font-variant-numeric:tabular-nums">{{.CodeFallback}}</span></p>{{end}}{{if .Note}}<p class="small" style="font-size:14px;line-height:1.6;margin:22px 0 0;color:#bdccd6">{{.Note}}</p>{{end}}<p class="foot muted" style="font-size:12px;line-height:1.6;color:#96a9b5;margin:32px 0 0">Valid until {{.Expiry}}.<br>Never share this message. If you did not request verification or a password reset, ignore that request.</p></td></tr></table></td></tr></table></td></tr></table></body></html>`)).Execute(&html, map[string]any{"Heading": heading, "Body": body, "Button": button, "Link": link, "Expiry": expiry, "Wordmark": template.URL(BrandWordmarkURL), "Code": codeFirstHTML(code, d.Kind), "CodeNote": codeNote, "CodeFallback": codeFallbackHTML(code, d.Kind), "Note": note, "Preheader": preheader})
	if e != nil {
		return nil, e
	}
	// The ID is restricted to safe characters by the enqueue repository.
	digest := sha256.Sum256([]byte(d.ID))
	boundary := "portico-" + hex.EncodeToString(digest[:20])
	related := "portico-rel-" + hex.EncodeToString(digest[:20])
	var alternative bytes.Buffer
	multi := multipart.NewWriter(&alternative)
	if e = multi.SetBoundary(boundary); e != nil {
		return nil, e
	}
	for _, part := range []struct{ kind, body string }{{"text/plain", text}, {"text/html", html.String()}} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", part.kind+"; charset=utf-8")
		header.Set("Content-Transfer-Encoding", "quoted-printable")
		w, err := multi.CreatePart(header)
		if err != nil {
			return nil, err
		}
		q := quotedprintable.NewWriter(w)
		if _, err = q.Write([]byte(part.body)); err != nil {
			return nil, err
		}
		if err = q.Close(); err != nil {
			return nil, err
		}
	}
	if e = multi.Close(); e != nil {
		return nil, e
	}
	// The wordmark rides along as a related part of the alternative body.
	var bodyBuffer bytes.Buffer
	outer := multipart.NewWriter(&bodyBuffer)
	if e = outer.SetBoundary(related); e != nil {
		return nil, e
	}
	altHeader := textproto.MIMEHeader{}
	altHeader.Set("Content-Type", "multipart/alternative; boundary="+strconv.Quote(boundary))
	w, err := outer.CreatePart(altHeader)
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(alternative.Bytes()); err != nil {
		return nil, err
	}
	imageHeader := textproto.MIMEHeader{}
	imageHeader.Set("Content-Type", `image/png; name="portico-wordmark.png"`)
	imageHeader.Set("Content-Transfer-Encoding", "base64")
	imageHeader.Set("Content-ID", "<"+WordmarkCID+">")
	imageHeader.Set("Content-Disposition", `inline; filename="portico-wordmark.png"`)
	imageHeader.Set("Content-Location", "portico-wordmark.png")
	if w, err = outer.CreatePart(imageHeader); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(brandWordmarkPNG)
	for len(encoded) > 76 {
		if _, err = w.Write([]byte(encoded[:76] + "\r\n")); err != nil {
			return nil, err
		}
		encoded = encoded[76:]
	}
	if _, err = w.Write([]byte(encoded + "\r\n")); err != nil {
		return nil, err
	}
	if e = outer.Close(); e != nil {
		return nil, e
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\n", (&mail.Address{Name: "Portico", Address: from}).String(), (&mail.Address{Address: d.Payload.To}).String(), mime.QEncoding.Encode("UTF-8", subject))
	if replyTo != "" {
		fmt.Fprintf(&out, "Reply-To: %s\r\n", (&mail.Address{Address: replyTo}).String())
	}
	fmt.Fprintf(&out, "Date: %s\r\nMessage-ID: <%s@%s>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=%q; type=\"multipart/alternative\"\r\n", d.CreatedAt.UTC().Format(time.RFC1123Z), d.ID, domain, related)
	fmt.Fprintf(&out, "\r\n")
	out.Write(bodyBuffer.Bytes())
	return out.Bytes(), nil
}

// codeFirstHTML renders the code as the first visible block for code emails
// (MAIL-05). Other kinds render no leading code; their code fallback (always
// empty now that every code kind is code-first) stays for the template shape.
func codeFirstHTML(code string, k Kind) string {
	if isCodeKind(k) && code != "" {
		return code
	}
	return ""
}

// codeFallbackHTML is the old "Or enter this code" block. Code emails show
// the code first instead, so there is no fallback; it remains only to keep
// the template's non-code path explicit.
func codeFallbackHTML(code string, k Kind) string {
	if isCodeKind(k) {
		return ""
	}
	return code
}
