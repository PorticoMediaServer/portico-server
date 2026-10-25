package mailoutbox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	TLS          string `json:"tls"` // "starttls" (mandatory upgrade) or "implicit"
	From         string `json:"from"`
	ReplyTo      string `json:"replyTo,omitempty"`
	Username     string `json:"username"`
	PasswordFile string `json:"passwordFile"`
	password     string
}

func LoadSMTP(path string) (*SMTPConfig, error) {
	if path == "" {
		return nil, nil
	}
	info, e := os.Stat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("SMTP configuration must be a private regular file")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var c SMTPConfig
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid SMTP configuration")
	}
	if e = c.Validate(); e != nil {
		return nil, e
	}
	if c.Username != "" {
		info, e = os.Stat(c.PasswordFile)
		if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("SMTP password must be a private regular file")
		}
		raw, e = os.ReadFile(c.PasswordFile)
		if e != nil {
			return nil, e
		}
		c.password = strings.TrimRight(string(raw), "\r\n")
		if c.password == "" || len(c.password) > 4096 {
			return nil, errors.New("invalid SMTP password")
		}
	}
	return &c, nil
}
func (c SMTPConfig) Validate() error {
	if c.Host == "" || len(c.Host) > 253 || strings.ContainsAny(c.Host, " /\\\r\n\x00:@") || c.Port < 1 || c.Port > 65535 || !ValidAddress(c.From) || (c.TLS != "starttls" && c.TLS != "implicit") || len(c.Username) > 320 || strings.ContainsAny(c.Username, "\r\n\x00") || (c.Username == "") != (c.PasswordFile == "") {
		return errors.New("SMTP requires a host, port, verified-TLS mode, sender and paired authentication inputs")
	}
	if c.ReplyTo != "" && !ValidAddress(c.ReplyTo) {
		return errors.New("SMTP reply-to must be a valid address")
	}
	return nil
}
func (c *SMTPConfig) FromAddress() string    { return c.From }
func (c *SMTPConfig) ReplyToAddress() string { return c.ReplyTo }

// newClient identifies this sender with its configured domain. Go's default
// EHLO localhost is not a usable public sending identity; relays may refuse it
// before advertising STARTTLS. This changes no peer/TLS/authentication policy.
func (c *SMTPConfig) newClient(conn net.Conn) (*smtp.Client, error) {
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return nil, err
	}
	if err = client.Hello(c.From[strings.LastIndex(c.From, "@")+1:]); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}
func (c *SMTPConfig) Send(ctx context.Context, to string, message []byte) error {
	if e := c.Validate(); e != nil {
		return e
	}
	if !ValidAddress(to) {
		return errors.New("invalid mail recipient")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, e := dialer.DialContext(ctx, "tcp", address)
	if e != nil {
		return e
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if c.TLS == "implicit" {
		secure := tls.Client(conn, tlsConfig)
		if e = secure.HandshakeContext(ctx); e != nil {
			return e
		}
		conn = secure
	}
	client, e := c.newClient(conn)
	if e != nil {
		return e
	}
	defer client.Close()
	if c.TLS == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server requires TLS")
		}
		if e = client.StartTLS(tlsConfig); e != nil {
			return e
		}
	}
	if c.Username != "" {
		if e = client.Auth(smtp.PlainAuth("", c.Username, c.password, c.Host)); e != nil {
			return e
		}
	}
	if e = client.Mail(c.From); e != nil {
		return e
	}
	if e = client.Rcpt(to); e != nil {
		return e
	}
	writer, e := client.Data()
	if e != nil {
		return e
	}
	if _, e = writer.Write(message); e != nil {
		_ = writer.Close()
		return e
	}
	// DATA's final reply is the delivery boundary. A later QUIT failure does not
	// cause a duplicate. A lost DATA reply is ambiguous and retried with same ID.
	if e = writer.Close(); e != nil {
		return e
	}
	_ = client.Quit()
	return nil
}
