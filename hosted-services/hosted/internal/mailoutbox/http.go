package mailoutbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// HTTPConfig targets an operator-configured transactional mail gateway accepting
// {from,to,rawMessage}. Credentials and response bodies are never logged.
type HTTPConfig struct {
	Endpoint  string `json:"endpoint"`
	From      string `json:"from"`
	ReplyTo   string `json:"replyTo,omitempty"`
	TokenFile string `json:"tokenFile"`
	token     string
	client    *http.Client
}

func LoadHTTP(path string) (*HTTPConfig, error) {
	if path == "" {
		return nil, nil
	}
	info, e := os.Stat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("mail HTTP configuration must be private")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var c HTTPConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil || dec.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid mail HTTP configuration")
	}
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || !ValidAddress(c.From) {
		return nil, errors.New("mail HTTP endpoint requires HTTPS and a sender")
	}
	if c.ReplyTo != "" && !ValidAddress(c.ReplyTo) {
		return nil, errors.New("mail HTTP reply-to must be a valid address")
	}
	info, e = os.Stat(c.TokenFile)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("mail HTTP token must be private")
	}
	raw, e = os.ReadFile(c.TokenFile)
	if e != nil {
		return nil, e
	}
	c.token = strings.TrimSpace(string(raw))
	if c.token == "" || len(c.token) > 4096 || strings.ContainsAny(c.token, "\r\n") {
		return nil, errors.New("invalid mail HTTP token")
	}
	c.client = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &c, nil
}
func (c *HTTPConfig) FromAddress() string    { return c.From }
func (c *HTTPConfig) ReplyToAddress() string { return c.ReplyTo }
func (c *HTTPConfig) Send(ctx context.Context, to string, message []byte) error {
	if !ValidAddress(to) || len(message) > 256<<10 {
		return errors.New("invalid message")
	}
	body, _ := json.Marshal(struct {
		From       string `json:"from"`
		To         string `json:"to"`
		RawMessage string `json:"rawMessage"`
	}{c.From, to, string(message)})
	r, e := http.NewRequestWithContext(ctx, "POST", c.Endpoint, bytes.NewReader(body))
	if e != nil {
		return e
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
	response, e := c.client.Do(r)
	if e != nil {
		return errors.New("mail provider unavailable")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("mail provider rejected delivery")
	}
	return nil
}
