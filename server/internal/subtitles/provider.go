package subtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"portico.local/server/internal/progressio"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/localonly"
	"portico.local/server/internal/operations"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/remotemedia"
)

type ProviderStatus struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}
type ProviderCandidate struct {
	FileID                       int64
	Language, Title, Attribution string
}
type Provider interface {
	ID() string
	Search(context.Context, string, string, int) ([]ProviderCandidate, error)
	Download(context.Context, int64) ([]byte, error)
}

func (s *Service) ProviderStatus() ProviderStatus {
	if s.provider == nil {
		return ProviderStatus{ID: "opensubtitles", Reason: "provider_not_configured"}
	}
	if status, ok := s.provider.(interface{ Status() ProviderStatus }); ok {
		return status.Status()
	}
	return ProviderStatus{ID: s.provider.ID(), Enabled: true}
}

// OpenSubtitles credentials stay in the server process. No provider URL, token,
// filename, HTML, or download capability is included in public search results.
type OpenSubtitles struct {
	key, token, agent        string
	username, password, host string
	loginRejected            atomic.Bool
	client                   *http.Client
	mu                       sync.Mutex
	next                     time.Time
	gate                     chan struct{}
	// The owner's saved credentials (SetCredentials) and the environment's,
	// which are used where the owner saved none. request adopts a change.
	credentials, environment openSubtitlesCredentials
	credentialMu             sync.Mutex
	// changes counts SetCredentials calls; adopted is the count request last
	// took its credentials at.
	changes, adopted int
}

type openSubtitlesCredentials struct{ key, username, password string }

// ProviderFromEnvironment is the OpenSubtitles provider, with the environment's
// credentials until the owner saves their own (Server › Metadata).
func ProviderFromEnvironment() *OpenSubtitles {
	env := openSubtitlesCredentials{strings.TrimSpace(os.Getenv("PORTICO_OPENSUBTITLES_API_KEY")), os.Getenv("PORTICO_OPENSUBTITLES_USERNAME"), os.Getenv("PORTICO_OPENSUBTITLES_PASSWORD")}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.MaxConnsPerHost = 2
	tr.ResponseHeaderTimeout = 10 * time.Second
	tr.MaxResponseHeaderBytes = 32 << 10
	agent := strings.TrimSpace(os.Getenv("PORTICO_OPENSUBTITLES_USER_AGENT"))
	if agent == "" {
		agent = "Portico v1"
	}
	return &OpenSubtitles{key: env.key, username: env.username, password: env.password, environment: env, credentials: env, host: "api.opensubtitles.com", agent: agent, gate: make(chan struct{}, 1), client: &http.Client{Transport: progressio.Transport{Base: tr, Idle: 30 * time.Second, Overall: 15 * time.Minute}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// SetCredentials installs the owner's saved key and account. An empty key keeps
// the environment's key; an empty username keeps the environment's account.
// The next request signs in again with what is now in force.
func (p *OpenSubtitles) SetCredentials(key, username, password string) {
	next := openSubtitlesCredentials{strings.TrimSpace(key), username, password}
	if next.key == "" {
		next.key = p.environment.key
	}
	if next.username == "" {
		next.username, next.password = p.environment.username, p.environment.password
	}
	p.credentialMu.Lock()
	p.credentials = next
	p.changes++
	p.credentialMu.Unlock()
	p.loginRejected.Store(false)
}

// adoptCredentials runs under the request gate: a change of credentials drops
// the session token and a refused sign-in, so the new ones are tried.
func (p *OpenSubtitles) adoptCredentials() {
	p.credentialMu.Lock()
	c, changes := p.credentials, p.changes
	p.credentialMu.Unlock()
	if changes == p.adopted {
		return
	}
	p.adopted = changes
	if c.key == p.key && c.username == p.username && c.password == p.password {
		return
	}
	p.key, p.username, p.password, p.token, p.host = c.key, c.username, c.password, "", "api.opensubtitles.com"
	p.loginRejected.Store(false)
}

func (p *OpenSubtitles) configured() bool {
	p.credentialMu.Lock()
	defer p.credentialMu.Unlock()
	return p.credentials.key != "" || p.changes == 0 && p.key != ""
}
func (p *OpenSubtitles) ID() string { return "opensubtitles" }
func (p *OpenSubtitles) request(ctx context.Context, method, path string, body, out any) error {
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	p.adoptCredentials()
	if p.key == "" {
		return ErrUnavailable
	}
	if p.token == "" && p.username != "" {
		if e := p.login(ctx); e != nil {
			return e
		}
	}
	e := p.requestOnce(ctx, method, path, body, out)
	if e == errProviderAuthorization && p.username != "" && !p.loginRejected.Load() {
		p.token = ""
		if e = p.login(ctx); e != nil {
			return e
		}
		return p.requestOnce(ctx, method, path, body, out)
	}
	return e
}

var errProviderAuthorization = errors.New("subtitle provider authorization expired")

func (p *OpenSubtitles) login(ctx context.Context) error {
	if p.loginRejected.Load() || p.username == "" || p.password == "" {
		return ErrUnavailable
	}
	p.host = "api.opensubtitles.com"
	var response struct {
		Token string `json:"token"`
		Host  string `json:"base_url"`
	}
	e := p.requestOnce(ctx, "POST", "/login", map[string]string{"username": p.username, "password": p.password}, &response)
	if e == errProviderAuthorization {
		p.loginRejected.Store(true)
	}
	if e != nil {
		return e
	}
	if response.Token == "" || len(response.Token) > 16384 || (response.Host != "api.opensubtitles.com" && response.Host != "vip-api.opensubtitles.com") {
		return ErrUnavailable
	}
	p.token, p.host = response.Token, response.Host
	return nil
}
func (p *OpenSubtitles) requestOnce(ctx context.Context, method, path string, body, out any) error {
	p.mu.Lock()
	delay := time.Until(p.next)
	p.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	p.mu.Lock()
	p.next = time.Now().Add(time.Second)
	p.mu.Unlock()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req, e := http.NewRequestWithContext(ctx, method, "https://"+p.providerHost()+"/api/v1"+path, bytes.NewReader(raw))
	if e != nil {
		return ErrUnavailable
	}
	req.Header.Set("Api-Key", p.key)
	req.Header.Set("User-Agent", p.agent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.token != "" && path != "/login" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	response, e := p.client.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 429 {
		delay := time.Minute
		if n, e := strconv.Atoi(response.Header.Get("Retry-After")); e == nil && n > 0 && n <= 86400 {
			delay = time.Duration(n) * time.Second
		}
		p.mu.Lock()
		p.next = time.Now().Add(delay)
		p.mu.Unlock()
	}
	if response.StatusCode == http.StatusUnauthorized {
		return errProviderAuthorization
	}
	if response.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	raw, e = io.ReadAll(io.LimitReader(response.Body, MaxInputBytes+1))
	if e != nil || len(raw) > MaxInputBytes {
		return ErrUnavailable
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (p *OpenSubtitles) providerHost() string {
	if p.host == "vip-api.opensubtitles.com" {
		return p.host
	}
	return "api.opensubtitles.com"
}
func (p *OpenSubtitles) Search(ctx context.Context, title, language string, page int) ([]ProviderCandidate, error) {
	return p.SearchMedia(ctx, ProviderQuery{Title: title, Language: language, Page: page, Season: -1, Episode: -1})
}
func (p *OpenSubtitles) SearchMedia(ctx context.Context, media ProviderQuery) ([]ProviderCandidate, error) {
	q := url.Values{"languages": {providerLanguage(media.Language)}}
	if media.Page > 1 {
		q.Set("page", strconv.Itoa(media.Page))
	}
	switch {
	case media.IMDbID != "":
		q.Set("imdb_id", media.IMDbID)
	case media.TMDBID != "":
		q.Set("tmdb_id", media.TMDBID)
	default:
		if media.ParentIMDbID != "" {
			q.Set("parent_imdb_id", media.ParentIMDbID)
		} else if media.ParentTMDBID != "" {
			q.Set("parent_tmdb_id", media.ParentTMDBID)
		} else {
			q.Set("query", strings.ToLower(media.Title))
			if media.Year > 0 && media.Episode < 0 {
				q.Set("year", strconv.Itoa(media.Year))
			}
		}
		if media.Episode >= 0 && media.Season >= 0 {
			q.Set("season_number", strconv.Itoa(media.Season))
			q.Set("episode_number", strconv.Itoa(media.Episode))
			q.Set("type", "episode")
		}
	}

	var response struct {
		Data []struct {
			Attributes struct {
				Language string `json:"language"`
				Release  string `json:"release"`
				Files    []struct {
					FileID int64 `json:"file_id"`
				} `json:"files"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if e := p.request(ctx, "GET", "/subtitles?"+q.Encode(), nil, &response); e != nil {
		return nil, e
	}
	out := []ProviderCandidate{}
	for _, v := range response.Data {
		language, e := Language(v.Attributes.Language)
		if e != nil {
			continue
		}
		for _, file := range v.Attributes.Files {
			if file.FileID <= 0 {
				continue
			}
			out = append(out, ProviderCandidate{file.FileID, language, safeTitle(v.Attributes.Release), "OpenSubtitles.com"})
			if len(out) >= 40 {
				return out, nil
			}
		}
	}
	return out, nil
}
func (p *OpenSubtitles) Download(ctx context.Context, id int64) ([]byte, error) {
	var response struct {
		Link string `json:"link"`
	}
	if e := p.request(ctx, "POST", "/download", map[string]any{"file_id": id, "sub_format": "srt"}, &response); e != nil {
		return nil, e
	}
	u, e := url.Parse(response.Link)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return nil, ErrUnavailable
	}
	// Reuse the existing public-network transport: no proxy, fresh DNS admission
	// at every dial/redirect, no private/link-local targets or TLS downgrade.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client, e := remotemedia.New(ctx, response.Link, remotemedia.Policy{})
	if e != nil {
		return nil, ErrUnavailable
	}
	defer client.Close()
	res, e := client.Open(ctx, "GET", "")
	if e != nil {
		return nil, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, MaxInputBytes+1))
	if e != nil || len(raw) > MaxInputBytes {
		return nil, ErrCapacity
	}
	return raw, nil
}

type SearchRequest struct {
	SourceID string `json:"sourceId"`
	Language string `json:"language"`
	Query    string `json:"query"`
	Page     int    `json:"page"`
}
type SearchCandidate struct {
	ID          string `json:"id"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	Attribution string `json:"attribution"`
}
type SearchResult struct {
	Version    int               `json:"version"`
	Candidates []SearchCandidate `json:"candidates"`
	Page       int               `json:"page"`
	ExpiresAt  string            `json:"expiresAt"`
}

func (s *Service) Search(ctx context.Context, p identity.Principal, item string, m SearchRequest) (SearchResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := SearchResult{Version: 1, Candidates: []SearchCandidate{}, Page: m.Page}
	language, e := Language(m.Language)
	if e != nil || language == "und" && m.Language != "" || m.Page < 1 || m.Page > 20 || !textField(m.Query, 160) {
		return out, ErrInput
	}
	if s.provider == nil {
		return out, ErrUnavailable
	}
	gated, p, e := s.readTx(ctx, p, item)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if m.Language == "" {
		values, _, _, err := operations.EffectivePreferences(tx, p.Viewer, "")
		if err != nil {
			return out, err
		}
		language = "en"
		for _, want := range values.List("playback.preferredSubtitleLanguages") {
			if normalized, err := Language(want); err == nil && normalized != "und" {
				language = normalized
				break
			}
		}
	}

	src, e := sourceQuery(ctx, tx, item, m.SourceID)
	if e != nil {
		return out, e
	}
	if local, err := localonly.Item(ctx, tx, item); err != nil || local {
		if err != nil {
			return out, err
		}
		return out, localonly.Err
	}
	if !src.Available {
		return out, ErrUnavailable
	}
	media, e := providerQuery(ctx, tx, item, strings.TrimSpace(m.Query), language, m.Page)
	if e != nil {
		return out, e
	}

	if e = gated.Commit(); e != nil {
		return out, e
	}
	var candidates []ProviderCandidate
	if advanced, ok := s.provider.(MediaSearchProvider); ok {
		candidates, e = advanced.SearchMedia(ctx, media)
	} else {
		candidates, e = s.provider.Search(ctx, safeTitle(media.Title), language, m.Page)
	}
	if e != nil {
		return out, e
	}
	var gated2 *dbwork.Write
	gated2, p, e = s.tx(ctx, p, item)
	if e != nil {
		return out, e
	}
	tx = gated2.Tx()
	defer gated2.Rollback()
	current, e := sourceQuery(ctx, tx, item, m.SourceID)
	if e != nil {
		return out, e
	}
	if !current.Available || current.size != src.size || current.modified != src.modified {
		return out, ErrConflict
	}
	// Bounded retained candidates per viewer; a later search deliberately retires
	// older un-applied capabilities. Publication receipts survive this retirement.
	if _, e = tx.ExecContext(ctx, `DELETE FROM subtitle_provider_candidates WHERE actor=? AND item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, actor(p), item); e != nil {
		return out, e
	}
	out.ExpiresAt = time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	for i, c := range candidates {
		if i >= 40 {
			break
		}
		id := identity.Token()
		if _, e = tx.ExecContext(ctx, `INSERT INTO subtitle_provider_candidates(id,actor,item_id,source_id,source_size,source_modified_ns,provider,file_id,language,title,attribution,expires_at) SELECT ?,?,id,?,?,?,?,?,?,?,?,? FROM catalog_entities WHERE public_id=pid_blob(?)`, id, actor(p), src.ID, src.size, src.modified, s.provider.ID(), c.FileID, c.Language, safeTitle(c.Title), c.Attribution, out.ExpiresAt, item); e != nil {
			return out, e
		}
		out.Candidates = append(out.Candidates, SearchCandidate{id, c.Language, safeTitle(c.Title), c.Attribution})
	}
	return out, gated2.Commit()
}

type ApplyRequest struct {
	OperationID string `json:"operationId"`
	CandidateID string `json:"candidateId"`
	Scope       string `json:"scope"`
	Rights      string `json:"rights"`
}

func (s *Service) Apply(ctx context.Context, p identity.Principal, item string, m ApplyRequest) (Receipt, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !validID(m.OperationID) || !validID(m.CandidateID) || (m.Scope != "personal" && m.Scope != "shared") || !textField(m.Rights, 500) {
		return Receipt{}, ErrInput
	}
	digest := requestDigest([]any{"provider", item, m})
	gated3, p, e := s.tx(ctx, p, item)
	if e != nil {
		return Receipt{}, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	old, e := receiptTx(ctx, tx, p, m.OperationID, digest, item)
	if e != nil {
		return Receipt{}, e
	}
	if old != nil {
		return *old, nil
	}
	if s.provider == nil {
		return Receipt{}, ErrUnavailable
	}
	if m.Scope == "shared" && !owner(p) {
		return Receipt{}, identity.ErrUnauthorized
	}
	var srcID, provider, language, title, attribution string
	var size, modified, file int64
	e = tx.QueryRowContext(ctx, `SELECT source_id,source_size,source_modified_ns,provider,file_id,language,title,attribution FROM subtitle_provider_candidates WHERE id=? AND actor=? AND item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND expires_at>?`, m.CandidateID, actor(p), item, time.Now().UTC().Format(time.RFC3339)).Scan(&srcID, &size, &modified, &provider, &file, &language, &title, &attribution)
	if e != nil {
		return Receipt{}, ErrConflict
	}
	src, e := sourceQuery(ctx, tx, item, srcID)
	if e != nil {
		return Receipt{}, e
	}
	if !src.Available || src.size != size || src.modified != modified || provider != s.provider.ID() {
		return Receipt{}, ErrConflict
	}
	if e = gated3.Commit(); e != nil {
		return Receipt{}, e
	}
	raw, e := s.provider.Download(ctx, file)
	if e != nil {
		return Receipt{}, e
	}
	canonical, e := Canonical(raw, "srt", int64(src.duration*1e6))
	if e != nil {
		return Receipt{}, e
	}
	mutation := Mutation{OperationID: m.OperationID, SourceID: srcID, Scope: m.Scope, Format: "srt", Language: language, Title: title, Rights: m.Rights, OffsetUS: "0"}
	return s.publish(ctx, p, item, mutation, digest, publication{source: src, origin: "provider", provider: provider, attribution: attribution, originalDigest: digestBytes(raw)}, canonical)
}

func (p *OpenSubtitles) Status() ProviderStatus {
	if !p.configured() {
		return ProviderStatus{ID: p.ID(), Reason: "provider_not_configured"}
	}
	if p.loginRejected.Load() {
		return ProviderStatus{ID: p.ID(), Reason: "provider_sign_in_failed"}
	}
	return ProviderStatus{ID: p.ID(), Enabled: true}
}
