// Package driveoauth runs the owner's own Google OAuth sign-in for a Drive
// source: a short-lived loopback redirect, state and PKCE held on the server,
// and one code exchange at Google's fixed token endpoint. It never keeps a token
// after answering, never follows a redirect from the token endpoint, and never
// accepts a redirect target from the request.
package driveoauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid authorization request")
var ErrDenied = errors.New("authorization was refused")
var ErrExchange = errors.New("authorization exchange failed")

const (
	sessionTTL    = 10 * time.Minute
	acceptTimeout = 5 * time.Minute
	requestLimit  = 8192
	codeLimit     = 4096
	stateLimit    = 128
	tokenEndpoint = "https://oauth2.googleapis.com/token"
	exchangeLimit = 64 << 10
	documentPath  = "/"
	maxLifetime   = 366 * 24 * time.Hour
	evictInterval = time.Minute
)

// Token is the token shape rclone stores in its configuration.
type Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Expiry       string `json:"expiry"`
}

// Phase is what one session is doing now.
type Phase string

const (
	Pending Phase = "pending"
	Ready   Phase = "ready"
	Expired Phase = "expired"
	Denied  Phase = "denied"
)

// Result answers a status read. Token is rclone's JSON text, present only when
// the phase is ready.
type Result struct {
	Phase Phase  `json:"phase"`
	Token string `json:"token,omitempty"`
}

type session struct {
	actor        string
	clientID     string
	clientSecret string
	verifier     string
	redirectURI  string
	state        string
	port         int
	expires      time.Time
	code         string
	errorCode    string
	listener     net.Listener
	closed       bool
	exchanging   bool
	flight       context.CancelFunc
}

type exchangeFunc func(ctx context.Context, clientID, clientSecret, code, verifier, redirectURI string) (Token, error)

// Store holds the in-flight sign-ins. It is deliberately small and in-memory: a
// restart loses pending sign-ins, never a saved source.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*session
	order    []string
	limit    int
	exchange exchangeFunc
	stop     chan struct{}
	done     chan struct{}
}

// New starts the bounded expiry sweep. Close ends it.
func New(limit int) *Store {
	if limit < 1 {
		limit = 1
	}
	store := &Store{sessions: map[string]*session{}, limit: limit, exchange: googleExchange, stop: make(chan struct{}), done: make(chan struct{})}
	go store.sweep()
	return store
}

func (s *Store) sweep() {
	defer close(s.done)
	ticker := time.NewTicker(evictInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			s.mu.Lock()
			for state := range s.sessions {
				s.dropLocked(state)
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for state, current := range s.sessions {
				if now.After(current.expires) {
					s.dropLocked(state)
				}
			}
			s.mu.Unlock()
		}
	}
}

// Close ends the store: every session is dropped, its listener closed and its
// flight cancelled.
func (s *Store) Close() {
	if s == nil {
		return
	}
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}

func randomValue(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", ErrInvalid
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Start opens one owner-bound sign-in session and returns the authorization URL
// and its state. The client secret and PKCE verifier stay on the server.
func (s *Store) Start(actor, clientID, clientSecret string) (string, string, error) {
	if s == nil || actor == "" || len(clientID) < 8 || len(clientID) > 512 || len(clientSecret) < 8 || len(clientSecret) > 2048 {
		return "", "", ErrInvalid
	}
	if strings.IndexFunc(clientID+clientSecret, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return "", "", ErrInvalid
	}
	state, err := randomValue(32)
	if err != nil {
		return "", "", err
	}
	verifier, err := randomValue(32)
	if err != nil {
		return "", "", err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", ErrInvalid
	}
	port := listener.Addr().(*net.TCPAddr).Port
	current := &session{
		actor: actor, clientID: clientID, clientSecret: clientSecret, verifier: verifier,
		redirectURI: fmt.Sprintf("http://127.0.0.1:%d%s", port, documentPath), state: state, port: port,
		expires: time.Now().Add(sessionTTL), listener: listener,
	}
	s.mu.Lock()
	for len(s.order) >= s.limit {
		s.dropLocked(s.order[0])
	}
	s.sessions[state] = current
	s.order = append(s.order, state)
	s.mu.Unlock()
	go s.serve(current)
	challenge := base64.RawURLEncoding.EncodeToString(digest(verifier))
	query := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {current.redirectURI},
		"response_type":         {"code"},
		"scope":                 {"https://www.googleapis.com/auth/drive.readonly"},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + query.Encode(), state, nil
}

func digest(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

// serve answers the first valid loopback redirect for this session. Unrelated or
// malformed requests are refused without consuming the callback, within one
// bounded deadline.
func (s *Store) serve(current *session) {
	listener := current.listener
	deadline := time.Now().Add(acceptTimeout)
	if tcp, ok := listener.(*net.TCPListener); ok {
		_ = tcp.SetDeadline(deadline)
	}
	terminal := func() {
		_ = listener.Close()
		s.mu.Lock()
		if current.code != "" || current.errorCode != "" {
			current.closed = true
		}
		s.mu.Unlock()
	}
	defer terminal()
	for time.Now().Before(deadline) {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		code, state, failure, valid := readRedirect(conn)
		_ = conn.Close()
		if !valid {
			continue
		}
		if state != current.state {
			continue
		}
		if failure != "" {
			s.fail(current, failure)
			return
		}
		if s.accept(current, code) {
			return
		}
	}
}

// readRedirect parses one bounded HTTP request and reports its redirect query.
func readRedirect(conn net.Conn) (code, state, failure string, valid bool) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	head, err := readHead(conn, requestLimit)
	if err != nil {
		return "", "", "", false
	}
	line, _, ok := strings.Cut(head, "\r\n")
	if !ok || !strings.HasPrefix(line, "GET ") {
		writePage(conn, "400 Bad Request", "Portico could not use this address.")
		return "", "", "", false
	}
	target := strings.TrimPrefix(line, "GET ")
	target = strings.TrimSuffix(strings.TrimSuffix(target, " HTTP/1.1"), " HTTP/1.0")
	if !strings.HasPrefix(target, documentPath+"?") {
		writePage(conn, "400 Bad Request", "Portico could not use this address.")
		return "", "", "", false
	}
	values, err := url.ParseQuery(strings.TrimPrefix(target, documentPath+"?"))
	if err != nil || len(values["state"]) != 1 {
		writePage(conn, "400 Bad Request", "Portico could not use this address.")
		return "", "", "", false
	}
	state = values["state"][0]
	if len(state) < 1 || len(state) > stateLimit {
		writePage(conn, "400 Bad Request", "Portico could not use this address.")
		return "", "", "", false
	}
	if failures := values["error"]; len(failures) == 1 {
		writePage(conn, "403 Forbidden", "Google did not finish the sign-in. Return to Portico.")
		return "", state, failures[0], true
	}
	codes := values["code"]
	if len(codes) != 1 || len(codes[0]) < 1 || len(codes[0]) > codeLimit || strings.ContainsAny(codes[0], "\r\n\x00") {
		writePage(conn, "400 Bad Request", "Portico could not use this address.")
		return "", "", "", false
	}
	writePage(conn, "200 OK", "You can close this window and return to Portico.")
	return codes[0], state, "", true
}

func readHead(conn net.Conn, limit int) (string, error) {
	raw := make([]byte, 0, 512)
	buffer := make([]byte, 512)
	for len(raw) <= limit {
		read, err := conn.Read(buffer)
		if read > 0 {
			raw = append(raw, buffer[:read]...)
			if index := strings.Index(string(raw), "\r\n\r\n"); index >= 0 {
				return string(raw[:index]), nil
			}
		}
		if err != nil {
			return "", err
		}
	}
	return "", ErrInvalid
}

func writePage(conn net.Conn, status, body string) {
	_, _ = io.WriteString(conn, "HTTP/1.1 "+status+"\r\nContent-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n"+
		"<!doctype html><title>Portico</title><p>"+body+"</p>")
}

func (s *Store) fail(current *session, failure string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current.closed || s.sessions[current.state] != current || time.Now().After(current.expires) {
		return
	}
	current.errorCode = failure
	current.code = ""
}

// accept records the code exactly once for a live session.
func (s *Store) accept(current *session, code string) bool {
	if len(code) < 1 || len(code) > codeLimit || strings.ContainsAny(code, "\r\n\x00") {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current.closed || s.sessions[current.state] != current || current.code != "" || current.errorCode != "" || time.Now().After(current.expires) {
		return false
	}
	current.code = code
	return true
}

// Paste records the code from the address Google ended on, for the browser that
// cannot reach the server's loopback listener. Only this session's exact
// loopback origin is accepted; the address is parsed, never fetched.
func (s *Store) Paste(actor, state, redirectURL string) error {
	if s == nil || len(state) > stateLimit || len(redirectURL) > 8192 {
		return ErrInvalid
	}
	parsed, err := url.Parse(redirectURL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Path != documentPath || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery == "" {
		return ErrInvalid
	}
	s.mu.Lock()
	current := s.sessions[state]
	if current == nil || current.actor != actor || time.Now().After(current.expires) {
		s.mu.Unlock()
		return ErrInvalid
	}
	port := current.port
	s.mu.Unlock()
	if parsed.Port() != strconv.Itoa(port) {
		return ErrInvalid
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values["state"]) != 1 || values["state"][0] != state {
		return ErrInvalid
	}
	if failures := values["error"]; len(failures) == 1 {
		s.fail(current, failures[0])
		return ErrDenied
	}
	codes := values["code"]
	if len(codes) != 1 {
		return ErrInvalid
	}
	if !s.accept(current, codes[0]) {
		return ErrInvalid
	}
	return nil
}

// Status answers the owner who started the session. A recorded code is exchanged
// once, with its exact entry kept and counted until the answer arrives; the token
// is delivered only to that live session.
func (s *Store) Status(ctx context.Context, actor, state string) (Result, error) {
	if s == nil || len(state) > stateLimit {
		return Result{Phase: Expired}, nil
	}
	s.mu.Lock()
	current := s.sessions[state]
	if current == nil || current.actor != actor {
		s.mu.Unlock()
		return Result{Phase: Expired}, nil
	}
	if time.Now().After(current.expires) {
		s.dropLocked(state)
		s.mu.Unlock()
		return Result{Phase: Expired}, nil
	}
	if current.errorCode != "" {
		s.dropLocked(state)
		s.mu.Unlock()
		return Result{Phase: Denied}, nil
	}
	if current.code == "" || current.exchanging {
		s.mu.Unlock()
		return Result{Phase: Pending}, nil
	}
	flight, cancel := context.WithTimeout(ctx, 20*time.Second)
	current.exchanging = true
	current.flight = cancel
	code, clientID, clientSecret, verifier, redirectURI := current.code, current.clientID, current.clientSecret, current.verifier, current.redirectURI
	s.mu.Unlock()
	token, err := s.exchange(flight, clientID, clientSecret, code, verifier, redirectURI)
	cancel()
	s.mu.Lock()
	live := s.sessions[state] == current && !current.closed && current.actor == actor && time.Now().Before(current.expires)
	s.dropLocked(state)
	s.mu.Unlock()
	if !live || err != nil {
		if err == nil {
			err = ErrExchange
		}
		return Result{}, err
	}
	raw, err := json.Marshal(token)
	if err != nil {
		return Result{}, ErrExchange
	}
	return Result{Phase: Ready, Token: string(raw)}, nil
}

// Cancel ends the owner's session, aborts an exchange in flight and closes the
// listener.
func (s *Store) Cancel(actor, state string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.sessions[state]; current != nil && current.actor == actor {
		s.dropLocked(state)
	}
}

func (s *Store) dropLocked(state string) {
	current := s.sessions[state]
	if current == nil {
		return
	}
	current.closed = true
	if current.flight != nil {
		current.flight()
		current.flight = nil
	}
	if current.listener != nil {
		_ = current.listener.Close()
	}
	current.clientSecret = ""
	current.verifier = ""
	current.code = ""
	delete(s.sessions, state)
	for index, value := range s.order {
		if value == state {
			s.order = append(s.order[:index], s.order[index+1:]...)
			break
		}
	}
}

// googleExchange is the only outbound call this package makes: Google's fixed
// token endpoint, with the server-held client secret and verifier, and no
// redirect ever followed.
func googleExchange(ctx context.Context, clientID, clientSecret, code, verifier, redirectURI string) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, ErrExchange
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return Token{}, ErrExchange
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, exchangeLimit+1))
	if err != nil || len(raw) > exchangeLimit || response.StatusCode != http.StatusOK {
		return Token{}, ErrExchange
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return Token{}, ErrExchange
	}
	if payload.AccessToken == "" || len(payload.AccessToken) > 8192 || payload.RefreshToken == "" || len(payload.RefreshToken) > 8192 {
		return Token{}, ErrExchange
	}
	if !strings.EqualFold(payload.TokenType, "Bearer") {
		return Token{}, ErrExchange
	}
	if payload.ExpiresIn <= 0 || time.Duration(payload.ExpiresIn)*time.Second > maxLifetime {
		return Token{}, ErrExchange
	}
	if !strings.Contains(payload.Scope, "https://www.googleapis.com/auth/drive.readonly") {
		return Token{}, ErrExchange
	}
	expiry := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	return Token{AccessToken: payload.AccessToken, TokenType: "Bearer", RefreshToken: payload.RefreshToken, Expiry: expiry}, nil
}
